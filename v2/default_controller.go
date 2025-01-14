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
	return deflt().Logger(name)
}

// NewHandler creates a named flume Handler, managed by the default flume.Controller.
//
// These package-level functions are typically used to initialize
// package-level loggers in var initializers, which can later
// be configured in main().
// See [Controller.Handler]
//
// Example:
//
//	package http
//	var logger = slog.New(flume.NewHandler("http"))
func NewHandler(name string) slog.Handler {
	return deflt().Handler(name)
}

// SetSink sets the sink handler for flume handlers with the given
// name, managed by the default flume.Controller.
// See [Controller.SetSink]
func SetSink(name string, sink slog.Handler) {
	deflt().SetSink(name, sink)
}

// SetDefaultSink sets the default sink for the default flume.Controller.
// See [Controller.SetDefaultSink]
func SetDefaultSink(sink slog.Handler) {
	deflt().SetDefaultSink(sink)
}

// SetLevel sets the log level for flume handlers with the give name,
// managed by the default flume.Controller.
// See [Controller.SetLevel]
func SetLevel(name string, l slog.Level) {
	deflt().SetLevel(name, l)
}

// SetDefaultLevel sets the default log level for the default
// flume.Controller.
// See [Controller.SetDefaultLevel]
func SetDefaultLevel(l slog.Level) {
	deflt().SetDefaultLevel(l)
}

// Use applies middleware to flume handlers with the given name, managed
// by the default flume.Controller.
// See [Controller.Use]
func Use(name string, middleware ...Middleware) {
	deflt().Use(name, middleware...)
}

// UseDefault applies middleware to all flume handlers managed by
// the default flume.Controller.
// See [Controller.Use.Default]
func UseDefault(middleware ...Middleware) {
	deflt().UseDefault(middleware...)
}

var defaultFactory atomic.Pointer[Controller]

//nolint:gochecknoinits
func init() {
	defaultFactory.Store(NewController(slog.Default().Handler()))
}

func deflt() *Controller {
	return defaultFactory.Load()
}
