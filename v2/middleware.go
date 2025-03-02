package flume

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"
)

// const badKey = "!BADKEY"

// Looks like this isn't needed.  New govet rules make the compiler enforce arg pairs to slog methods,
// so it's pretty hard now to pass a bare arg.
// func BareAttr() Middleware {
// 	return HandlerMiddlewareFunc(func(ctx context.Context, record slog.Record, next slog.Handler) error {
// 		// if the record only has a single attr, and that attr was added without a matching
// 		// key, slog will set the key to "!BADKEY".  In flume v1, we added attrs like that
// 		// with an underscore as the key.
//
// 		if record.NumAttrs() != 1 {
// 			return next.Handle(ctx, record)
// 		}
//
// 		record.Attrs(func(attr slog.Attr) bool {
// 			if attr.Key != badKey {
// 				return false
// 			}
//
// 			// slog.Record has no means to
// 			record = slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
//
// 			if _, ok := asError(attr.Value); ok {
// 				attr.Key = "error"
// 			} else {
// 				attr.Key = "value"
// 			}
//
// 			record.AddAttrs(attr)
//
// 			// stop immediately, we only care about the first attr
// 			return false
// 		})
//
// 		return next.Handle(ctx, record)
// 	})
// }

// func asError(value slog.Value) (error, bool) { //nolint:revive
// 	if value.Kind() != slog.KindAny {
// 		return nil, false
// 	}
//
// 	err, ok := value.Any().(error)
// 	return err, ok
// }

// AbbreviateLevel is a ReplaceAttr function that abbreviates log level names.
//
// It modifies the attribute if it's a log level (slog.Level) and changes the level name to its abbreviation.
// The abbreviations are:
//
//   - "DEBUG" becomes "DBG"
//   - "INFO" becomes "INF"
//   - "WARN" becomes "WRN"
//   - "ERROR" becomes "ERR"
//
// If the attribute's value is not a slog.Level, it is returned unchanged.
//
// Example:
//
//	// Create a logger with the ReplaceAttr function.
//	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
//		ReplaceAttr: AbbreviateLevel,
//	}))
//
//	// Log a message.
//	logger.Debug("This is a debug message.") // Output will be: level=DBG msg="This is a debug message."
func AbbreviateLevel(_ []string, attr slog.Attr) slog.Attr {
	if attr.Value.Kind() != slog.KindAny {
		return attr
	}

	if lvl, ok := attr.Value.Any().(slog.Level); ok {
		strLvl := lvl.String()
		switch strLvl[0] {
		case 'D':
			strLvl = dbgAbbrev + strLvl[5:]
		case 'W':
			strLvl = wrnAbbrev + strLvl[4:]
		case 'E':
			strLvl = errAbbrev + strLvl[5:]
		case 'I':
			strLvl = infAbbrev + strLvl[4:]
		}
		attr.Value = slog.StringValue(strLvl)
	}

	return attr
}

// FormatTimes returns a ReplaceAttr function that formats time values according to the specified format.
//
// It modifies an attribute if its value is a time.Time. The time is formatted using the provided format string,
// and the attribute's value is updated to the formatted time string.
//
// If the attribute's value is not a time.Time, it is returned unchanged.
//
// Args:
//
//	format string: The format string to use for formatting time values (e.g., time.DateTime, "2006-01-02", "15:04:05").
//
// Returns:
//
//	func([]string, slog.Attr) slog.Attr: A ReplaceAttr function that formats time values.
//
// Example:
//
//		// Create a ReplaceAttr function that formats times using a custom format.
//		customTimeFormat := FormatTimes("2006-01-02 15:04:05")
//
//		// Create a logger with the ReplaceAttr function.
//		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
//		    ReplaceAttr: customTimeFormat,
//		}))
//
//		// Log a message with a time attribute.
//		logger.Info("Time example", slog.Time("now", time.Now()))
//		// Output might be: level=INFO msg="Time example" now="2023-10-27 10:30:00"
//
//	 // Create a ReplaceAttr function that formats times using a predefined format.
//	 dateTimeFormat := FormatTimes(time.DateTime)
//
//	 // Create a logger with the ReplaceAttr function.
//	 logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
//	     ReplaceAttr: dateTimeFormat,
//	 }))
//
//	 // Log a message with a time attribute.
//	 logger.Info("Time example", slog.Time("now", time.Now()))
//	 // Output might be: level=INFO msg="Time example" now="2023-10-27 10:30:00.000"
func FormatTimes(format string) func([]string, slog.Attr) slog.Attr {
	return func(_ []string, a slog.Attr) slog.Attr {
		if a.Value.Kind() == slog.KindTime {
			a.Value = slog.StringValue(a.Value.Time().Format(format))
		}
		return a
	}
}

// SimpleTime returns a ReplaceAttr function that formats time values to a simple time format: "15:04:05.000".
//
// It's a convenience function that uses FormatTimes internally with a predefined format.
//
// Example:
//
//	// Create a logger with the ReplaceAttr function.
//	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
//	    ReplaceAttr: SimpleTime(),
//	}))
//
//	// Log a message with a time attribute.
//	logger.Info("Time example", slog.Time("now", time.Now()))
//	// Output might be: level=INFO msg="Time example" now="10:30:00.000"
func SimpleTime() func([]string, slog.Attr) slog.Attr {
	return FormatTimes("15:04:05.000")
}

type detailedJSONError struct {
	error
	fmt.Formatter
}

func (e detailedJSONError) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("%+v", e.error)) //nolint:wrapcheck
}

// DetailedErrors is a ReplaceAttr function which ensures that errors
// implement fmt.Formatter are rendered the same by the JSONHandler as
// by the TextHandler.  By default, the slog.JSONHandler renders errors
// as err.Error(), where as the slog.TextHandler will render it with
// fmt.Sprintf("%+v", err).
//
// When using DetailedErrors, if any error values implement fmt.Formatter (
// and therefore *may* implement some form of detailed error printing), and
// *do not* already implement json.Marshaler, then the value is wrapped in
// an error implementation which implements json.Marshaler, and marshals
// the error to a string using fmt.Sprintf("%+v", err).
func DetailedErrors(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() != slog.KindAny {
		return a
	}

	if err, ok := a.Value.Any().(error); ok {
		if formatter, ok := err.(fmt.Formatter); ok {
			if _, ok = err.(json.Marshaler); !ok {
				a.Value = slog.AnyValue(detailedJSONError{err, formatter})
			}
		}
	}

	return a
}

// func V1Compat() func(slog.Handler) slog.Handler {
// 	return func(h slog.Handler) slog.Handler {
//
// 	}
// }

// func resolve(value slog.Value) slog.Value {
// 	v := value.Resolve()
// 	if v.Kind() == slog.KindGroup {
// 		grp := v.Group()
// 		for i := range grp {
// 			grp[i].Value = resolve(grp[i].Value)
// 		}
// 	}
// 	return v
// }

func applyReplaceAttrs(groups []string, a slog.Attr, fns []func([]string, slog.Attr) slog.Attr) slog.Attr {
	// assume that the value has already been resolved by the calling handler.  That's part
	// of the contract
	for _, fn := range fns {
		if fn == nil {
			continue
		}
		a = fn(groups, a)

		if a.Equal(slog.Attr{}) {
			// one of the functions returned an empty attr, which
			// is the equivalent of deleting the attr, so we're done.
			return a
		}

		a.Value = a.Value.Resolve()
		if a.Value.Kind() == slog.KindGroup {
			// ReplaceAttr is only run on non-group attrs.  We can stop.
			// The handler will take care of calling us again on the members.
			return a
		}
	}

	return a
}

// ChainReplaceAttrs composes a series of ReplaceAttr functions into a single ReplaceAttr function.
func ChainReplaceAttrs(fns ...func([]string, slog.Attr) slog.Attr) func([]string, slog.Attr) slog.Attr {
	switch len(fns) {
	case 0:
		// no op, return nil func
		return nil
	case 1:
		return fns[0]
	default:
		return func(groups []string, a slog.Attr) slog.Attr {
			return applyReplaceAttrs(groups, a, fns)
		}
	}
}

// ReplaceAttrs is middleware which adds ReplaceAttr support to other Handlers
// which don't natively have it.
// Because this can only act on the slog.Record as it passes through the middleware,
// it has limitations regarding the built-in fields:
//
//   - slog.SourceKey: skipped
//   - slog.MessageKey: ignores changes to the key name.  If the returned value is empty, the message will
//     be set to the value `<nil>`.  Non-string values are coerced to a string like `fmt.Print`
//   - slog.TimeKey: ignores changes to the key name.  If the value returned is not empty or a time.Time{}, it is
//     ignored.
//   - slog.LevelKey: ignores changes to the key name.  If the value returned is not empty or a slog.Level, it is
//     ignored.
func ReplaceAttrs(fns ...func([]string, slog.Attr) slog.Attr) Middleware {
	fn := ChainReplaceAttrs(fns...)
	if fn == nil {
		// no-op
		return MiddlewareFunc(func(h slog.Handler) slog.Handler {
			return h
		})
	}

	return MiddlewareFunc(func(h slog.Handler) slog.Handler {
		return &replaceAttrsMiddleware{
			next:        h,
			replaceAttr: ChainReplaceAttrs(fns...),
		}
	})
}

type replaceAttrsMiddleware struct {
	next        slog.Handler
	groups      []string
	replaceAttr func([]string, slog.Attr) slog.Attr
}

func (h *replaceAttrsMiddleware) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *replaceAttrsMiddleware) Handle(ctx context.Context, record slog.Record) error {
	// TODO: apply ReplaceAttrs to the built-in attrs as well

	var replacedBuiltIns [3]slog.Attr
	replaced := replacedBuiltIns[:0]

	attr := h.replaceAttr(nil, slog.String(slog.MessageKey, record.Message))
	record.Message = attr.Value.String()

	attr = h.replaceAttr(nil, slog.Time(slog.TimeKey, record.Time))
	if attr.Value.Kind() == slog.KindTime {
		record.Time = attr.Value.Time()
	} else if attr.Value.Equal(slog.Value{}) {
		record.Time = time.Time{}
	}

	attr = h.replaceAttr(nil, slog.Any(slog.LevelKey, record.Level))
	if attr.Value.Kind() == slog.KindAny && attr.Key == slog.LevelKey {
		if lvl, ok := attr.Value.Any().(slog.Level); ok {
			record.Level = lvl
		}
	} else if attr.Value.Equal(slog.Value{}) {
		record.Level = slog.LevelInfo
	}

	if record.NumAttrs() == 0 && len(replaced) == 0 {
		return h.next.Handle(ctx, record)
	}

	newRecord := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	newRecord.AddAttrs(replaced...)
	record.Attrs(func(a slog.Attr) bool {
		a = h.applyReplaceAttrRecurse(h.groups, a)
		if !a.Equal(slog.Attr{}) {
			newRecord.AddAttrs(a)
		}
		return true
	})
	return h.next.Handle(ctx, newRecord)
}

func (h *replaceAttrsMiddleware) WithAttrs(attrs []slog.Attr) slog.Handler {
	attrs = h.applyReplaceAttr(h.groups, attrs)
	if len(attrs) == 0 {
		// all attrs resolved to empty, so no-op
		return h
	}
	return &replaceAttrsMiddleware{
		next:        h.next.WithAttrs(attrs),
		replaceAttr: h.replaceAttr,
		groups:      h.groups,
	}
}

func (h *replaceAttrsMiddleware) WithGroup(name string) slog.Handler {
	return &replaceAttrsMiddleware{
		next:        h.next.WithGroup(name),
		replaceAttr: h.replaceAttr,
		groups:      slices.Clip(append(h.groups, name)),
	}
}

func (h *replaceAttrsMiddleware) applyReplaceAttrRecurse(groups []string, attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()

	if attr.Value.Kind() != slog.KindGroup {
		attr = h.replaceAttr(h.groups, attr)
		attr.Value = attr.Value.Resolve()
		return attr
	}

	members := h.applyReplaceAttr(append(groups, attr.Key), attr.Value.Group())

	if len(members) == 0 {
		// empty group, elide
		return slog.Attr{}
	}
	attr.Value = slog.GroupValue(members...)
	return attr
}

func (h *replaceAttrsMiddleware) applyReplaceAttr(groups []string, attrs []slog.Attr) []slog.Attr {
	newAttrs := attrs[:0]
	for _, attr := range attrs {
		attr = h.applyReplaceAttrRecurse(groups, attr)
		if !attr.Equal(slog.Attr{}) {
			newAttrs = append(newAttrs, attr)
		}
	}
	return newAttrs
}

type Middleware interface {
	Apply(slog.Handler) slog.Handler
}

type MiddlewareFunc func(slog.Handler) slog.Handler

func (f MiddlewareFunc) Apply(h slog.Handler) slog.Handler {
	return f(h)
}

func (f HandlerMiddlewareFunc) Apply(h slog.Handler) slog.Handler {
	return &middlewareHandler{
		next:       h,
		middleware: f,
	}
}

type HandlerMiddlewareFunc func(ctx context.Context, record slog.Record, next slog.Handler) error

type middlewareHandler struct {
	next       slog.Handler
	middleware func(ctx context.Context, record slog.Record, next slog.Handler) error
}

func (h *middlewareHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *middlewareHandler) Handle(ctx context.Context, record slog.Record) error {
	return h.middleware(ctx, record, h.next)
}

func (h *middlewareHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &middlewareHandler{
		next:       h.next.WithAttrs(attrs),
		middleware: h.middleware,
	}
}

func (h *middlewareHandler) WithGroup(name string) slog.Handler {
	return &middlewareHandler{
		next:       h.next.WithGroup(name),
		middleware: h.middleware,
	}
}
