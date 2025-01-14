package flume

import (
	"log/slog"
	"math"
	"sync/atomic"
)

const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
	LevelOff   = slog.Level(math.MaxInt)
	LevelAll   = slog.Level(math.MinInt)
)

// New is a convenience for `slog.New(NewHandler(name))`.
// The handler is managed by the default flume.Controller.
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
	return Default().Logger(name)
}

// Handler creates a named flume Handler, managed by the default flume.Controller.
//
// These package-level functions are typically used to initialize
// package-level loggers in var initializers, which can later
// be configured in main().
// See [Controller.Handler]
//
// Example:
//
//	package http
//	var logger = slog.New(flume.Handler("http"))
func Handler(name string) slog.Handler {
	return Default().Handler(name)
}

var defaultFactory atomic.Pointer[Controller]

//nolint:gochecknoinits
func init() {
	defaultFactory.Store(NewController(slog.Default().Handler()))
}

func Default() *Controller {
	return defaultFactory.Load()
}
