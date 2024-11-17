package flume

import (
	"context"
	"log/slog"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
)

type handler struct {
	*handlerState
}

func newHandlerState(lv *slog.LevelVar, delegate slog.Handler, attrs []slog.Attr, group string) *handlerState {
	hs := handlerState{
		attrs: attrs,
		group: group,
		level: lv,
	}
	hs.setDelegateHandler(delegate, true)
	return &hs
}

type handlerState struct {
	// attrs associated with this handler.  immutable after initial construction.
	attrs []slog.Attr
	// group associated with this handler.  immutable after initial construction.
	group string

	// atomic pointer to handler delegate
	delegateHandlerPtr atomic.Pointer[slog.Handler]
	// indicates if the default delegate handler has been overridden with a handler-specific delegate
	delegateSet bool

	// map of child handlerStates.  Calls to WithAttrs() or WithGroup() create new children
	// which we need to hold a reference to so we can cascade setDelegateHandler() to them.
	children map[*handlerState]bool
	sync.Mutex

	level *slog.LevelVar
	// indicates if the default level has been overridden with a handler-specific level
	levelSet bool
}

func (s *handlerState) setDelegateHandler(delegate slog.Handler, isDefault bool) {
	s.Lock()
	defer s.Unlock()

	if s.delegateSet && isDefault {
		return
	}

	s.delegateSet = !isDefault

	if delegate == nil {
		delegate = noop
	}

	// re-apply groups and attrs settings
	// don't need to check if s.attrs is empty: it will never be empty because
	// all flume handlers have at least a "logger_name" attribute
	delegate = delegate.WithAttrs(slices.Clone(s.attrs))
	if s.group != "" {
		delegate = delegate.WithGroup(s.group)
	}

	s.delegateHandlerPtr.Store(&delegate)

	for holder := range s.children {
		// pass isDefault=false to force children to accept the new delegate.
		// only the top handler should do the default handling
		holder.setDelegateHandler(delegate, false)
	}
}

func (s *handlerState) delegateHandler() slog.Handler {
	handlerRef := s.delegateHandlerPtr.Load()
	return *handlerRef
}

func (s *handlerState) setLevel(l slog.Level, isDefault bool) {
	// this is optimized for making Enabled() as fast as possible, which can be
	// implemented with a single atomic load.

	// when setting the level, which need to know if this handler is tracking
	// the default level, or has been overridden with a handler-specific level.
	// if the default level has been overridden, we ignore future changes to the
	// default level.
	s.Lock()
	defer s.Unlock()

	switch {
	case isDefault && !s.levelSet:
		s.level.Set(l)
	case !isDefault:
		s.levelSet = true
		s.level.Set(l)
	}
}

func (s *handlerState) newChild(attrs []slog.Attr, group string) *handler {
	s.Lock()
	defer s.Unlock()

	hs := newHandlerState(s.level, s.delegateHandler(), attrs, group)

	if s.children == nil {
		s.children = map[*handlerState]bool{}
	}

	s.children[hs] = true

	h := &handler{
		handlerState: hs,
	}

	// when the handler goes out of scope, run a finalizer which
	// removes the state reference from its parent state, allowing
	// it to be gc'd
	runtime.SetFinalizer(h, func(_ *handler) {
		s.Lock()
		defer s.Unlock()

		delete(s.children, hs)
	})

	return h
}

func (s *handlerState) WithAttrs(attrs []slog.Attr) slog.Handler {
	return s.newChild(attrs, "")
}

func (s *handlerState) WithGroup(name string) slog.Handler {
	return s.newChild(nil, name)
}

func (s *handlerState) Enabled(_ context.Context, level slog.Level) bool {
	return level >= s.level.Level()
}

func (s *handlerState) Handle(ctx context.Context, record slog.Record) error {
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
