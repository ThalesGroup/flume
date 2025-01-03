package flume

import (
	"log/slog"
	"sync/atomic"
)

var defaultFactory atomic.Pointer[Controller]

//nolint:gochecknoinits
func init() {
	defaultFactory.Store(NewController(slog.Default().Handler()))
}

func deflt() *Controller {
	return defaultFactory.Load()
}

func NewHandler(loggerName string) slog.Handler {
	return deflt().Handler(loggerName)
}

func SetLoggerHandler(loggerName string, handler slog.Handler) {
	deflt().SetDelegate(loggerName, handler)
}

func SetDefaultHandler(handler slog.Handler) {
	deflt().SetDefaultDelegate(handler)
}

func SetLoggerLevel(loggerName string, l slog.Level) {
	deflt().SetLevel(loggerName, l)
}

func SetDefaultLevel(l slog.Level) {
	deflt().SetDefaultLevel(l)
}
