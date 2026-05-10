package flume

import (
	"context"
	"log/slog"
)

// Experimental: the APIs in this file are experimental and may change or be
// removed in a future release.
//
// ContextWithAttrs and ContextAttrsMiddleware provide a way to attach
// slog.Attrs to a context.Context and have them automatically appended to
// every log record whose handler chain includes ContextAttrsMiddleware.
//
// This is useful for propagating request-scoped attributes (e.g. request IDs,
// tenant IDs, user IDs) through call stacks without having to thread a
// *slog.Logger everywhere.

type ctxAttrsKey struct{}

// ContextWithAttrs returns a new context with attrs appended to any attrs
// already attached to ctx.  Calls are cumulative: attrs from earlier calls
// are preserved and new attrs are appended after them.
//
// If attrs is empty, ctx is returned unchanged.
//
// Attrs stored on the context are surfaced in log records by
// ContextAttrsMiddleware.
func ContextWithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	if len(attrs) == 0 {
		return ctx
	}

	existing := attrsFromContext(ctx)

	merged := make([]slog.Attr, 0, len(existing)+len(attrs))
	merged = append(merged, existing...)
	merged = append(merged, attrs...)

	return context.WithValue(ctx, ctxAttrsKey{}, merged)
}

func attrsFromContext(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}

	attrs, _ := ctx.Value(ctxAttrsKey{}).([]slog.Attr)

	return attrs
}

// ContextAttrsMiddleware returns a Middleware which appends any attrs stored
// on the context via ContextWithAttrs to each log record.
//
// Records with no context-attached attrs are passed through unchanged.
func ContextAttrsMiddleware() Middleware {
	return SimpleMiddlewareFn(func(ctx context.Context, record slog.Record, next slog.Handler) error {
		if attrs := attrsFromContext(ctx); len(attrs) > 0 {
			record.AddAttrs(attrs...)
		}

		return next.Handle(ctx, record)
	})
}
