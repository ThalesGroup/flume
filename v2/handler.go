package flume

import (
	"context"
	"log/slog"
	"slices"
	"sync/atomic"
)

type handler struct {
	*state
}

type state struct {
	// attrs associated with this handler.  immutable after initial construction.
	attrs []slog.Attr
	// group associated with this handler.  immutable after initial construction.
	groups []string

	// atomic pointer to handler delegate
	delegateHandlerPtr atomic.Pointer[slog.Handler]

	// should be reference to the levelVar in the parent conf
	level *slog.LevelVar

	conf *conf
}

func (s *state) setDelegate(delegate slog.Handler) {
	// re-apply groups and attrs settings
	// don't need to check if s.attrs is empty: it will never be empty because
	// all flume handlers have at least a "logger_name" attribute
	delegate = delegate.WithAttrs(slices.Clone(s.attrs))
	for _, g := range s.groups {
		delegate = delegate.WithGroup(g)
	}

	s.delegateHandlerPtr.Store(&delegate)
}

func (s *state) delegateHandler() slog.Handler {
	handlerRef := s.delegateHandlerPtr.Load()
	return *handlerRef
}

func (s *state) WithAttrs(attrs []slog.Attr) slog.Handler {
	return s.conf.newHandler(append(s.attrs, attrs...), s.groups)
}

func (s *state) WithGroup(name string) slog.Handler {
	return s.conf.newHandler(s.attrs, append(s.groups, name))
}

func (s *state) Enabled(_ context.Context, level slog.Level) bool {
	return level >= s.level.Level()
}

func (s *state) Handle(ctx context.Context, record slog.Record) error {
	return s.delegateHandler().Handle(ctx, record)
}

// Env Config options:
// 1. format
// 2. time encoding
// 3. duration encoding
// 4. level encoding
// 5. default level
// 6. per-logger level
// 7. add caller
//
// Programmatic options
// 1. Set out
// 2. ReplaceAttr hooks

var noop = noopHandler{}

type noopHandler struct{}

func (n noopHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return false
}

func (n noopHandler) Handle(_ context.Context, _ slog.Record) error {
	return nil
}

func (n noopHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return n
}

func (n noopHandler) WithGroup(_ string) slog.Handler {
	return n
}
