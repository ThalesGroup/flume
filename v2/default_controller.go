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

func New(name string) *slog.Logger {
	return deflt().Logger(name)
}

func NewHandler(name string) slog.Handler {
	return deflt().Handler(name)
}

func SetDelegate(name string, handler slog.Handler) {
	deflt().SetDelegate(name, handler)
}

func SetDefaultDelegate(handler slog.Handler) {
	deflt().SetDefaultDelegate(handler)
}

func SetLevel(name string, l slog.Level) {
	deflt().SetLevel(name, l)
}

func SetDefaultLevel(l slog.Level) {
	deflt().SetDefaultLevel(l)
}
