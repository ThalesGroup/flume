package flume

import (
	"context"
	"log/slog"
)

const badKey = "!BADKEY"

// BareAttr
func BareAttr() Middleware {
	return HandlerMiddlewareFunc(func(ctx context.Context, record slog.Record, next slog.Handler) error {
		// if the record only has a single attr, and that attr was added without a matching
		// key, slog will set the key to "!BADKEY".  In flume v1, we added attrs like that
		// with an underscore as the key.

		if record.NumAttrs() != 1 {
			return next.Handle(ctx, record)
		}

		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key != badKey {
				return false
			}

			// slog.Record has no means to
			record = slog.NewRecord(record.Time, record.Level, record.Message, record.PC)

			if _, ok := asError(attr.Value); ok {
				attr.Key = "error"
			} else {
				attr.Key = "value"
			}

			record.AddAttrs(attr)

			// stop immediately, we only care about the first attr
			return false
		})

		return next.Handle(ctx, record)
	})
}

func asError(value slog.Value) (error, bool) { //nolint:revive
	if value.Kind() != slog.KindAny {
		return nil, false
	}

	err, ok := value.Any().(error)
	return err, ok
}

func AbbreviateLevel(_ []string, attr slog.Attr) slog.Attr {
	if attr.Value.Kind() == slog.KindAny {
		if lvl, ok := attr.Value.Any().(slog.Level); ok {
			strLvl := lvl.String()
			if len(strLvl) > 0 {
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
			}
			attr.Value = slog.StringValue(strLvl)
		}
	}

	return attr
}

func FormatTimes(format string) func([]string, slog.Attr) slog.Attr {
	return func(_ []string, a slog.Attr) slog.Attr {
		if a.Value.Kind() == slog.KindTime {
			a.Value = slog.StringValue(a.Value.Time().Format(format))
		}
		return a
	}
}

func SimpleTime() func(s []string, a slog.Attr) slog.Attr {
	return FormatTimes("15:04:05.000")
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
	for i, fn := range fns {
		if fn == nil {
			continue
		}
		a = fn(groups, a)
		a.Value = a.Value.Resolve()
		// if a is a group, and there are still more functions to apply,
		// apply the rest of the functions to the members of the group.
		// ReplaceAttrs functions are never applied to Group attrs themselves,
		// only to the children
		if a.Value.Kind() == slog.KindGroup && i < len(fns)-1 {
			if a.Key != "" {
				groups = append(groups, a.Key)
			}
			childAttrs := a.Value.Group()
			for i2, childAttr := range childAttrs {
				childAttr.Value = childAttr.Value.Resolve()
				childAttrs[i2] = applyReplaceAttrs(groups, childAttr, fns[i+1:])
			}
			// since we recursed to apply the rest of the functions, stop now
			return a
		}
	}

	return a
}

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

// func er(_ []string, a slog.Attr) slog.Attr {
// 	if err, ok := asError(a.Value); ok {
// 		a.Value = slog.StringValue(fmt.Sprintf("%+v", err))
// 	}
// 	return a
// }

// logger name is logged with attribute "name"
// l.Info("msg", err) => error:boom	errorVerbose:boom\n\n...
// l.Info("msg", "err", err) => err:boom\n\n...
// l.Info("msg", "blue") => _:blue

// func ErrorDetails() func(slog.Handler) slog.Handler {
// 	return NewMiddleware(func(ctx context.Context, record slog.Record, next slog.Handler) error {
// 		record.Attrs(func(attr slog.Attr) bool {
// 			attr.Value = resolve(attr.Value)
//
//
//
// 			if attr.Key != badKey {
// 				return false
// 			}
//
// 			v := attr.
//
// 			if err := attr.Value.Resolve().Any()
// 			if attr.Key == badKey && attr.Value.Resolve().
//
//
// 			// stop immediately, we only care about the first attr
// 			return false
// 		})
// 	})
// }

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
