package flume

import (
	"context"
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
//
// The middleware may be further configured to skip processing built-in attributes, or skipping processing
// for particular records.
func ReplaceAttrs(fns ...func([]string, slog.Attr) slog.Attr) *ReplaceAttrsMiddleware {
	return &ReplaceAttrsMiddleware{
		replaceAttr: ChainReplaceAttrs(fns...),
	}
}

var _ Middleware = (*ReplaceAttrsMiddleware)(nil)

type ReplaceAttrsMiddleware struct {
	// If SkipRecord is set, it will be called in the Handle function.  If it returns
	// false, all attr processing will be skipped.
	//
	// This may be used if attr processing is conditional on some aspect of the record,
	// like the level.  Note that attrs passed to WithAttrs() are always processed.
	SkipRecord func(slog.Record) bool

	// If true, built-in attributes (message, level, and time) will not be processed.
	SkipBuiltins bool

	next        slog.Handler
	groups      []string
	replaceAttr func([]string, slog.Attr) slog.Attr
}

func (r *ReplaceAttrsMiddleware) clone(next slog.Handler) *ReplaceAttrsMiddleware {
	return &ReplaceAttrsMiddleware{
		next:         next,
		groups:       slices.Clip(r.groups),
		replaceAttr:  r.replaceAttr,
		SkipRecord:   r.SkipRecord,
		SkipBuiltins: r.SkipBuiltins,
	}
}

func (r *ReplaceAttrsMiddleware) Apply(next slog.Handler) slog.Handler {
	return r.clone(next)
}

func (r *ReplaceAttrsMiddleware) Enabled(ctx context.Context, level slog.Level) bool {
	return r.next.Enabled(ctx, level)
}

func (r *ReplaceAttrsMiddleware) Handle(ctx context.Context, record slog.Record) error {
	if r.replaceAttr == nil || (r.SkipRecord != nil && r.SkipRecord(record)) {
		return r.next.Handle(ctx, record)
	}

	if !r.SkipBuiltins {
		record.Message = r.applyToMessage(record.Message)
		record.Time = r.applyToTime(record.Time)
		record.Level = r.applyToLevel(record.Level)
	}

	if record.NumAttrs() == 0 {
		return r.next.Handle(ctx, record)
	}

	newRecord := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(a slog.Attr) bool {
		a = r.applyReplaceAttrRecurse(r.groups, a)
		if !a.Equal(slog.Attr{}) {
			newRecord.AddAttrs(a)
		}
		return true
	})
	return r.next.Handle(ctx, newRecord)
}

func (r *ReplaceAttrsMiddleware) applyToMessage(msg string) string {
	attr := r.replaceAttr(nil, slog.String(slog.MessageKey, msg))
	return attr.Value.String()
}

func (r *ReplaceAttrsMiddleware) applyToTime(t time.Time) time.Time {
	attr := r.replaceAttr(nil, slog.Time(slog.TimeKey, t))
	if attr.Value.Kind() == slog.KindTime {
		return attr.Value.Time()
	}
	if attr.Value.Equal(slog.Value{}) {
		return time.Time{}
	}
	return t
}

func (r *ReplaceAttrsMiddleware) applyToLevel(l slog.Level) slog.Level {
	attr := r.replaceAttr(nil, slog.Any(slog.LevelKey, l))
	if attr.Value.Equal(slog.Value{}) {
		return slog.LevelInfo
	}
	if attr.Value.Kind() == slog.KindAny && attr.Key == slog.LevelKey {
		if lvl, ok := attr.Value.Any().(slog.Level); ok {
			return lvl
		}
	}
	return l
}

func (r *ReplaceAttrsMiddleware) WithAttrs(attrs []slog.Attr) slog.Handler {
	if r.replaceAttr == nil {
		return r.clone(r.next.WithAttrs(attrs))
	}

	attrs = r.applyReplaceAttr(r.groups, attrs)
	if len(attrs) == 0 {
		// all attrs resolved to empty, so no-op
		return r
	}
	return r.clone(r.next.WithAttrs(attrs))
}

func (r *ReplaceAttrsMiddleware) WithGroup(name string) slog.Handler {
	r = r.clone(r.next.WithGroup(name))
	r.groups = append(r.groups, name)
	return r
}

func (r *ReplaceAttrsMiddleware) applyReplaceAttrRecurse(groups []string, attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()

	if attr.Value.Kind() != slog.KindGroup {
		attr = r.replaceAttr(r.groups, attr)
		attr.Value = attr.Value.Resolve()
		return attr
	}

	members := r.applyReplaceAttr(append(groups, attr.Key), attr.Value.Group())

	if len(members) == 0 {
		// empty group, elide
		return slog.Attr{}
	}
	attr.Value = slog.GroupValue(members...)
	return attr
}

func (r *ReplaceAttrsMiddleware) applyReplaceAttr(groups []string, attrs []slog.Attr) []slog.Attr {
	newAttrs := attrs[:0]
	for _, attr := range attrs {
		attr = r.applyReplaceAttrRecurse(groups, attr)
		if !attr.Equal(slog.Attr{}) {
			newAttrs = append(newAttrs, attr)
		}
	}
	return newAttrs
}

type Middleware interface {
	Apply(slog.Handler) slog.Handler
}

// MiddlewareFn adapts a function to the Middleware interface.
type MiddlewareFn func(slog.Handler) slog.Handler

func (f MiddlewareFn) Apply(h slog.Handler) slog.Handler {
	return f(h)
}

// Apply implements Middleware
func (f SimpleMiddlewareFn) Apply(h slog.Handler) slog.Handler {
	return &middlewareHandler{
		next:       h,
		middleware: f,
	}
}

// SimpleMiddlewareFn defines a simple middleware function which intercepts
// *slog.Handler.Handle() calls.  SimpleMiddlewareFn adapts this to the Middleware
// interface.
type SimpleMiddlewareFn func(ctx context.Context, record slog.Record, next slog.Handler) error

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
