package flume

import "log/slog"

var defaultFactory *Factory

//nolint:gochecknoinits
func init() {
	defaultFactory = NewFactory(slog.Default().Handler())
}

func NewHandler(loggerName string) slog.Handler {
	return defaultFactory.NewHandler(loggerName)
}

func SetLoggerHandler(loggerName string, handler slog.Handler) {
	defaultFactory.SetLoggerHandler(loggerName, handler)
}

func SetDefaultHandler(handler slog.Handler) {
	defaultFactory.SetDefaultHandler(handler)
}

func SetLoggerLevel(loggerName string, l slog.Level) {
	defaultFactory.SetLoggerLevel(loggerName, l)
}

func SetDefaultLevel(l slog.Level) {
	defaultFactory.SetDefaultLevel(l)
}
