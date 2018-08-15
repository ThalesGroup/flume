// Package context provides functions to acquire a flume.Logger from a context.Context.
// Useful for injecting a logger into a request path.
package context

import (
	"context"
	"gitlab.protectv.local/regan/flume.git"
)

// DefaultLogger is returned by FromContext if no other logger has been
// injected into the context.
var DefaultLogger = flume.New("")

type ctxKey struct{}

var loggerKey = &ctxKey{}

// WithLogger returns a new context with the specified logger injected into it.
func WithLogger(ctx context.Context, l flume.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// FromContext returns a logger from the context.  If the context
// doesn't contain a logger, the DefaultLogger will be returned.
func FromContext(ctx context.Context) flume.Logger {
	if l, ok := ctx.Value(loggerKey).(flume.Logger); ok {
		return l
	}
	return DefaultLogger
}
