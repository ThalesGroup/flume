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
	// attrs associated with this handler.  immutable
	attrs []slog.Attr
	// group associated with this handler.  immutable
	groups []string

	// atomic pointer to handler delegate
	delegatePtr atomic.Pointer[slog.Handler]

	// should be reference to the levelVar in the parent conf
	level *slog.LevelVar

	conf *conf
}

func (s *state) setDelegate(delegate slog.Handler) {
	// re-apply groups and attrs settings
	// don't need to check if s.attrs is empty: it will never be empty because
	// all flume handlers have at least a "logger_name" attribute
	// TODO: I think this logic is wrong.  this assumes
	// that all these attrs are either nested in *no* groups, or
	// nested in *all* the groups.  Really, each attr will only
	// be nested in whatever set of groups was active when that
	// attr was first added.
	// TODO: Need to go back to each state holding pointers to
	// children, and trickling delegate or conf changes down
	// to to children.  And children need to point to parents...
	// need to ensure that *only leaf states* are eligible for
	// garbage collection, and not states which still have
	// referenced children.
	// Also, I think each state only needs to hold the set of
	// attrs which were used to create that state using WithAttrs.
	// It doesn't need the list of all attrs cached by parent
	// states.  So long as those parents already embedded those attrs
	// in their respective delegate handlers, and those delegate
	// handlers have already been trickled down to this state,
	// this state doesn't care about those parent attrs.  The state
	// only needs to hold on to its own attrs in case a new delegate
	// trickles down, and the state needs to rebuild its own delegate
	delegate = delegate.WithAttrs(slices.Clone(s.attrs))
	for _, g := range s.groups {
		delegate = delegate.WithGroup(g)
	}

	s.delegatePtr.Store(&delegate)
}

func (s *state) delegate() slog.Handler {
	handlerRef := s.delegatePtr.Load()
	return *handlerRef
}

func (s *state) WithAttrs(attrs []slog.Attr) slog.Handler {
	// TODO: I think I need to clone or clip this slice
	// TODO: this is actually pretty inefficient.  Each time this is
	// called, we end up re-calling WithAttrs and WithGroup on the delegate
	// several times.
	// TODO: consider adding native support for ReplaceAttr, and calling it
	// here...that way I can implement ReplaceAttrs in flume, and it
	// doesn't matter if the sinks implement it.  I'd need to add calls
	// to it in Handle() as well.
	return s.conf.newHandler(append(s.attrs, attrs...), s.groups)
}

func (s *state) WithGroup(name string) slog.Handler {
	// TODO: I think I need to clone or clip this slice
	return s.conf.newHandler(s.attrs, append(s.groups, name))
}

func (s *state) Enabled(_ context.Context, level slog.Level) bool {
	return level >= s.level.Level()
}

func (s *state) Handle(ctx context.Context, record slog.Record) error {
	return s.delegate().Handle(ctx, record)
}

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
