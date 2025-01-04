package flume

import (
	"log/slog"
	"sync"
)

const loggerNameKey = "logger"

// Controller is a log management core.  It spawns named slog.Handlers.  These named handlers
// add a `logger=<name>` attribute to each log entry.  The Controller can dynamically (at runtime)
// reconfigure these named handlers.  For example, the Controller can change the log level of a
// particular named handler.
//
// Named handlers ultimately delegate handling log entries to an underlying delegate slog.Handler.  The
// Controller has a default delegate slog.Handler to which all named handlers spawned by the Controller
// will send their log entries.  The Controller also allows overriding the default delegate handler
// for particular named handlers.
//
// Package-level functions mirror of most of Controller's methods, which delegate to a
// default package-level Controller.
type Controller struct {
	defaultLevel slog.Level

	defaultDelegate slog.Handler

	confs map[string]*conf
	mutex sync.Mutex
}

func NewController(delegateHandler slog.Handler) *Controller {
	return &Controller{defaultDelegate: delegateHandler}
}

func (r *Controller) Logger(name string) *slog.Logger {
	return slog.New(r.Handler(name))
}

func (r *Controller) Handler(name string) slog.Handler {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.conf(name).newHandler([]slog.Attr{slog.String(loggerNameKey, name)}, nil)
}

func (r *Controller) conf(name string) *conf {
	cfg, ok := r.confs[name]
	if !ok {
		levelVar := &slog.LevelVar{}
		levelVar.Set(r.defaultLevel)
		cfg = &conf{
			name:   name,
			lvl:    levelVar,
			states: map[*state]struct{}{},
		}
		cfg.setDelegate(r.defaultDelegate, true)
		if r.confs == nil {
			r.confs = map[string]*conf{}
		}
		r.confs[name] = cfg
	}

	return cfg
}

// SetDelegate configures the default delegate handler
func (r *Controller) SetDelegate(handlerName string, handler slog.Handler) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.conf(handlerName).setDelegate(handler, false)
}

func (r *Controller) SetDefaultDelegate(handler slog.Handler) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.defaultDelegate = handler

	for _, h := range r.confs {
		h.setDelegate(handler, true)
	}
}

// SetLevel sets the log level for a particular named logger.  All handlers with this same
// are affected, in the past or future.
func (r *Controller) SetLevel(handlerName string, l slog.Level) {
	if handlerName == "*" {
		r.SetDefaultLevel(l)
		return
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.conf(handlerName).setLevel(l, false)
}

// SetDefaultLevel sets the default log level for all handlers which don't have a specific level
// assigned to them
func (r *Controller) SetDefaultLevel(l slog.Level) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.defaultLevel = l

	// iterating over all handlers, inside a mutex, is slow, and made slower still
	// by each handler locking its own mutex.  But setting levels happens very rarely,
	// while reading the handler's level happens each time a log function is called.  So
	// we optimize for that path, which requires only a single atomic load.
	for _, h := range r.confs {
		h.setLevel(l, true)
	}
}
