package flume

import (
	"log/slog"
	"sync/atomic"
)

var defaultFactory atomic.Pointer[Factory]

//nolint:gochecknoinits
func init() {
	defaultFactory.Store(NewFactory(slog.Default().Handler()))
}

func deflt() *Factory {
	return defaultFactory.Load()
}

func NewHandler(loggerName string) slog.Handler {
	return deflt().NewHandler(loggerName)
}

func SetLoggerHandler(loggerName string, handler slog.Handler) {
	deflt().SetLoggerHandler(loggerName, handler)
}

func SetDefaultHandler(handler slog.Handler) {
	deflt().SetDefaultHandler(handler)
}

func SetLoggerLevel(loggerName string, l slog.Level) {
	deflt().SetLoggerLevel(loggerName, l)
}

func SetDefaultLevel(l slog.Level) {
	deflt().SetDefaultLevel(l)
}
