package flume

import (
	"context"
	"log/slog"
)

const badKey = "!BADKEY"

// BareAttr
func BareAttr() func(slog.Handler) slog.Handler {
	return NewMiddleware(func(ctx context.Context, record slog.Record, next slog.Handler) error {
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

			if _, ok := asError(attr.Value); ok {
				attr.Key = "error"
			} else {
				attr.Key = "value"
			}

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

func NewMiddleware(handleFn func(ctx context.Context, record slog.Record, next slog.Handler) error) func(slog.Handler) slog.Handler {
	return func(inner slog.Handler) slog.Handler {
		return &HandlerMiddleware{
			inner:    inner,
			handleFn: handleFn,
		}
	}
}

type HandlerMiddleware struct {
	inner    slog.Handler
	handleFn func(ctx context.Context, record slog.Record, next slog.Handler) error
}

func (h *HandlerMiddleware) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *HandlerMiddleware) Handle(ctx context.Context, record slog.Record) error {
	return h.handleFn(ctx, record, h.inner)
}

func (h *HandlerMiddleware) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &HandlerMiddleware{
		inner:    h.inner.WithAttrs(attrs),
		handleFn: h.handleFn,
	}
}

func (h *HandlerMiddleware) WithGroup(name string) slog.Handler {
	return &HandlerMiddleware{
		inner:    h.inner.WithGroup(name),
		handleFn: h.handleFn,
	}
}
