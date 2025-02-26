package flume

import (
	"log/slog"
	"math"
	"sync"
)

const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
	LevelOff   = slog.Level(math.MaxInt)
	LevelAll   = slog.Level(math.MinInt)
)

// New is a convenience for creating a named logger using the default handler.
//
// These package-level functions are typically used to initialize
// package-level loggers in var initializers, which can later
// be configured in main().
// See [Controller.Logger]
//
// Example:
//
//	package http
//	var logger = flume.New("http")
func New(name string) *slog.Logger {
	return slog.New(Default().Named(name))
}

var defaultHandler *Handler

var initDefaultHandlerOnce sync.Once

// Default returns the default Handler.  It will never be
// nil.  By default, it is a noop handler that discards all log messages.
//
// The simplest way to enable logging is to call Default().SetHandlerOptions(nil)
func Default() *Handler {
	initDefaultHandlerOnce.Do(func() {
		defaultHandler = NewHandler(nil, &HandlerOptions{
			HandlerFn: LookupHandlerFn(NoopHandler),
		})
	})
	return defaultHandler
}
