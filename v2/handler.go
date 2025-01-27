package flume

import (
	"context"
	"log/slog"
	"slices"
	"sync/atomic"
)

type handler struct {
	// atomic pointer to the base delegate
	basePtr *atomic.Pointer[slog.Handler]

	// atomic pointer to a memoized copy of the base
	// delegate, plus any transformations (i.e. WithGroup or WithAttrs calls)
	memoPrt atomic.Pointer[[2]*slog.Handler]

	// list of WithGroup/WithAttrs transformations.  Can be re-applied
	// to the base delegate any time it changes
	transformers []func(slog.Handler) slog.Handler

	// should be reference to the levelVar in the parent conf
	level *slog.LevelVar
}

func (s *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	transformer := func(h slog.Handler) slog.Handler {
		return h.WithAttrs(attrs)
	}
	return &handler{
		basePtr:      s.basePtr,
		level:        s.level,
		transformers: slices.Clip(append(s.transformers, transformer)),
	}
}

func (s *handler) WithGroup(name string) slog.Handler {
	transformer := func(h slog.Handler) slog.Handler {
		return h.WithGroup(name)
	}

	return &handler{
		basePtr:      s.basePtr,
		level:        s.level,
		transformers: slices.Clip(append(s.transformers, transformer)),
	}
}

func (s *handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= s.level.Level()
}

func (s *handler) Handle(ctx context.Context, record slog.Record) error {
	return s.delegate().Handle(ctx, record)
}

func (s *handler) delegate() slog.Handler {
	base := s.basePtr.Load()
	if base == nil {
		return noop
	}
	memo := s.memoPrt.Load()
	if memo != nil && memo[0] == base {
		hPtr := memo[1]
		if hPtr != nil {
			return *hPtr
		}
	}
	// build and memoize
	delegate := *base
	for _, transformer := range s.transformers {
		delegate = transformer(delegate)
	}
	if delegate == nil {
		delegate = noop
	}
	s.memoPrt.Store(&[2]*slog.Handler{base, &delegate})
	return delegate
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
