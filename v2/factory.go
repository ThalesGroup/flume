package flume

import (
	"log/slog"
	"sync"
)

const LoggerNameKey = "logger"

// Factory is a log management core.  It spawns handlers.  The Factory has
// methods for dynamically reconfiguring all the handlers spawned from Factory.
//
// The flume package has mirrors of most of the functions which delegate to a
// default, package-level factory.
type Factory struct {
	defaultLevel slog.Level

	rootHandler slog.Handler

	handlers map[string]*handler
	mutex    sync.Mutex
}

func NewFactory(rootHandler slog.Handler) *Factory {
	return &Factory{rootHandler: rootHandler}
}

func (r *Factory) SetRootHandler(handler slog.Handler) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.rootHandler = handler

	for _, h := range r.handlers {
		h.setDelegateHandler(handler)
	}
}

func (r *Factory) NewHandler(name string) slog.Handler {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.newHandler(name)
}

func (r *Factory) newHandler(name string) *handler {
	h, ok := r.handlers[name]
	if !ok {
		levelVar := &slog.LevelVar{}
		levelVar.Set(r.defaultLevel)
		h = &handler{newHandlerState(levelVar, r.rootHandler, []slog.Attr{slog.String(LoggerNameKey, name)}, "")}
		r.handlers[name] = h
	}

	return h
}

// SetLevel sets the log level for a particular named logger.  All handlers with this same
// are affected, in the past or future.
func (r *Factory) SetLevel(name string, l slog.Level) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	h := r.newHandler(name)
	h.setLevel(l, false)
}

// SetDefaultLevel sets the default log level for all handlers which don't have a specific level
// assigned to them
func (r *Factory) SetDefaultLevel(l slog.Level) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.defaultLevel = l

	// iterating over all handlers, inside a mutex, is slow, and made slower still
	// by each handler locking its own mutex.  But setting levels happens very rarely,
	// while reading the handler's level happens each time a log function is called.  So
	// we optimize for that path, which requires only a single atomic load.
	for _, h := range r.handlers {
		h.setLevel(l, true)
	}
}
