package flume

import (
	"context"
	"io"
	"log/slog"
	"os"
	"slices"
	"sync"
	"sync/atomic"
)

const (
	// LoggerKey is the key which stores the name of the logger.  The name was the argument
	// passed to New() or Named()
	LoggerKey = "logger"
)

func (o *HandlerOptions) handler(name string, w io.Writer) slog.Handler {
	if w == nil {
		w = os.Stdout
	}

	if o == nil {
		// special case: default to a text handler
		return TextHandlerFn()(name, w, &slog.HandlerOptions{})
	}

	opts := &slog.HandlerOptions{
		Level:       o.Level,
		AddSource:   o.AddSource,
		ReplaceAttr: ChainReplaceAttrs(o.ReplaceAttrs...),
	}

	if name != "" {
		if lvl, ok := o.Levels[name]; ok {
			opts.Level = lvl
		}
	}

	var sink slog.Handler

	handlerFn := o.HandlerFn
	if handlerFn == nil {
		handlerFn = TextHandlerFn()
	}

	sink = handlerFn(name, w, opts)
	if sink == nil {
		sink = noop
	}

	for i := len(o.Middleware) - 1; i >= 0; i-- {
		sink = o.Middleware[i].Apply(sink)
	}

	return sink
}

type Handler struct {
	opts      *HandlerOptions
	w         io.Writer
	mutex     sync.Mutex
	handler   *innerHandler
	delegates map[string]*atomic.Pointer[slog.Handler]
}

func NewHandler(w io.Writer, opts *HandlerOptions) *Handler {
	h := &Handler{
		delegates: map[string]*atomic.Pointer[slog.Handler]{},
		w:         w,
		opts:      opts,
	}
	h.handler = &innerHandler{
		root:    h,
		basePtr: h.delegatePtr(""),
	}

	return h
}

func (h *Handler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.handler.Enabled(ctx, lvl)
}

func (h *Handler) Handle(ctx context.Context, rec slog.Record) error {
	return h.handler.Handle(ctx, rec)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h.handler.WithAttrs(attrs)
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return h.handler.WithGroup(name)
}

// Named is a convenience for `h.WithAttrs([]slog.Attr{slog.String(LoggerKey, name)})`.
func (h *Handler) Named(name string) slog.Handler {
	return h.WithAttrs([]slog.Attr{slog.String(LoggerKey, name)})
}

// HandlerOptions returns a copy of the current handler options.
// This will never return nil.
func (h *Handler) HandlerOptions() *HandlerOptions {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	opts := h.opts.Clone()
	if opts == nil {
		return &HandlerOptions{}
	}

	return opts
}

// SetHandlerOptions sets the options passed to HandlerFn
// when sink handlers are created.  This triggers rebuilding
// all the sink handlers with the new opts,
// so affects on logging will apply immediately.
func (h *Handler) SetHandlerOptions(opts *HandlerOptions) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.opts = opts.Clone()
	h.reset()
}

func (h *Handler) Out() io.Writer {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.w
}

// SetOut sets the output writer passed to HandlerFn when sink handlers
// are created.  This triggers rebuilding all
// the sink handlers with the new opts,
// so affects on logging will apply immediately.
func (h *Handler) SetOut(w io.Writer) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.w = w
	h.reset()
}

func (h *Handler) reset() {
	for name, ptr := range h.delegates {
		h := h.opts.handler(name, h.w)
		ptr.Store(&h)
	}
}

func (h *Handler) delegatePtr(name string) *atomic.Pointer[slog.Handler] {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	ptr, ok := h.delegates[name]
	if !ok {
		ptr = &atomic.Pointer[slog.Handler]{}
		h.delegates[name] = ptr
	}

	if ptr.Load() == nil {
		h := h.opts.handler(name, h.w)
		ptr.Store(&h)
	}

	return ptr
}

type innerHandler struct {
	root *Handler
	// atomic pointer to the base delegate
	basePtr *atomic.Pointer[slog.Handler]

	// atomic pointer to a memoized copy of the base
	// delegate, plus any transformations (i.e. WithGroup or WithAttrs calls)
	memoPtr atomic.Pointer[[2]*slog.Handler]

	// list of WithGroup/WithAttrs transformations.  Can be re-applied
	// to the base delegate any time it changes
	transformers []func(slog.Handler) slog.Handler

	openGroups int
}

func (s *innerHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var delegate *atomic.Pointer[slog.Handler]

	// scan attrs for a logger name, but only if there is no group open
	// the logger name attribute is not allowed to be nested in a group
	if s.openGroups == 0 {
		if name := loggerName(attrs); name != "" {
			delegate = s.root.delegatePtr(name)
		}
	}

	if delegate == nil {
		delegate = s.basePtr
	}

	transformer := func(h slog.Handler) slog.Handler {
		return h.WithAttrs(attrs)
	}

	return &innerHandler{
		root:         s.root,
		basePtr:      delegate,
		transformers: slices.Clip(append(s.transformers, transformer)),
	}
}

func (s *innerHandler) WithGroup(name string) slog.Handler {
	transformer := func(h slog.Handler) slog.Handler {
		return h.WithGroup(name)
	}

	return &innerHandler{
		root:         s.root,
		basePtr:      s.basePtr,
		transformers: slices.Clip(append(s.transformers, transformer)),
		openGroups:   s.openGroups + 1,
	}
}

func (s *innerHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return s.delegate().Enabled(ctx, level)
}

func (s *innerHandler) Handle(ctx context.Context, record slog.Record) error {
	return s.delegate().Handle(ctx, record)
}

func (s *innerHandler) delegate() slog.Handler {
	base := s.basePtr.Load()

	memo := s.memoPtr.Load()
	if memo != nil && memo[0] == base && memo[1] != nil {
		return *memo[1]
	}

	// build and memoize
	delegate := *base
	for _, transformer := range s.transformers {
		delegate = transformer(delegate)
	}

	s.memoPtr.Store(&[2]*slog.Handler{base, &delegate})

	return delegate
}

func loggerName(attrs []slog.Attr) string {
	// find the last logger name attribute
	for i := len(attrs) - 1; i >= 0; i-- {
		if attrs[i].Key == LoggerKey && attrs[i].Value.Kind() == slog.KindString {
			return attrs[i].Value.String()
		}
	}

	return ""
}

var noop slog.Handler = noopHandler{}

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
