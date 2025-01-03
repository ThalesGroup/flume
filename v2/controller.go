package flume

import (
	"log/slog"
	"runtime"
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

type conf struct {
	name           string
	lvl            *slog.LevelVar
	customLvl      bool
	delegate       slog.Handler
	customDelegate bool
	sync.Mutex
	states map[*handlerState]struct{}
}

func (c *conf) setDelegate(delegate slog.Handler, isDefault bool) {
	c.Lock()
	defer c.Unlock()

	if c.customDelegate && isDefault {
		return
	}

	c.customDelegate = !isDefault

	if delegate == nil {
		delegate = noop
	}

	c.delegate = delegate

	for s := range c.states {
		s.setDelegate(delegate)
	}
}

func (c *conf) setLevel(l slog.Level, isDefault bool) {
	// don't need a mutex here.  this is already protected
	// by the Controller mutex, and the `lvl` pointer itself
	// is immutable.
	switch {
	case isDefault && !c.customLvl:
		c.lvl.Set(l)
	case !isDefault:
		c.customLvl = true
		c.lvl.Set(l)
	}
}

func (c *conf) newHandler(attrs []slog.Attr, groups []string) *handler {
	c.Lock()
	defer c.Unlock()

	s := &handlerState{
		attrs:  attrs,
		groups: groups,
		level:  c.lvl,
		conf:   c,
	}
	s.setDelegate(c.delegate)

	c.states[s] = struct{}{}

	h := &handler{
		handlerState: s,
	}

	// when the handler goes out of scope, run a finalizer which
	// removes the state reference from its parent state, allowing
	// it to be gc'd
	runtime.SetFinalizer(h, func(_ *handler) {
		c.Lock()
		defer c.Unlock()

		delete(c.states, s)
	})

	return h
}

func NewController(delegateHandler slog.Handler) *Controller {
	return &Controller{defaultDelegate: delegateHandler}
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
			states: map[*handlerState]struct{}{},
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
func (r *Controller) SetDelegate(handlerName string, delegate slog.Handler) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.conf(handlerName).setDelegate(delegate, false)
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
