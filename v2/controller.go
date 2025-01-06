package flume

import (
	"log/slog"
	"sync"
)

const (
	loggerNameKey = "logger"
	allHandlers   = "*"
)

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
	defaultLevel      slog.Level
	defaultDelegate   slog.Handler
	defaultMiddleware []Middleware

	confs map[string]*conf
	mutex sync.Mutex
}

func NewController(delegateHandler slog.Handler) *Controller {
	return &Controller{defaultDelegate: delegateHandler}
}

func (c *Controller) Logger(name string) *slog.Logger {
	return slog.New(c.Handler(name))
}

func (c *Controller) Handler(name string) slog.Handler {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.conf(name).newHandler([]slog.Attr{slog.String(loggerNameKey, name)}, nil)
}

func (c *Controller) conf(name string) *conf {
	cfg, ok := c.confs[name]
	if !ok {
		levelVar := &slog.LevelVar{}
		levelVar.Set(c.defaultLevel)
		cfg = &conf{
			name:             name,
			lvl:              levelVar,
			states:           map[*state]struct{}{},
			globalMiddleware: c.defaultMiddleware,
		}
		cfg.setCoreDelegate(c.defaultDelegate, true)
		if c.confs == nil {
			c.confs = map[string]*conf{}
		}
		c.confs[name] = cfg
	}

	return cfg
}

// SetDelegate configures the default delegate handler
func (c *Controller) SetDelegate(handlerName string, handler slog.Handler) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if handlerName != allHandlers {
		c.conf(handlerName).setCoreDelegate(handler, false)
		return
	}

	c.defaultDelegate = handler

	for _, h := range c.confs {
		h.setCoreDelegate(handler, true)
	}
}

func (c *Controller) SetDefaultDelegate(handler slog.Handler) {
	c.SetDelegate(allHandlers, handler)
}

// SetLevel sets the log level for a particular named logger.  All handlers with this same
// are affected, in the past or future.
func (c *Controller) SetLevel(name string, l slog.Level) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if name != allHandlers {
		c.conf(name).setLevel(l, false)
		return
	}

	c.defaultLevel = l

	// iterating over all handlers, inside a mutex, is slow, and made slower still
	// by each handler locking its own mutex.  But setting levels happens very rarely,
	// while reading the handler's level happens each time a log function is called.  So
	// we optimize for that path, which requires only a single atomic load.
	for _, h := range c.confs {
		h.setLevel(l, true)
	}
}

// SetDefaultLevel sets the default log level for all handlers which don't have a specific level
// assigned to them
func (c *Controller) SetDefaultLevel(l slog.Level) {
	c.SetLevel(allHandlers, l)
}

func (c *Controller) Use(name string, middleware ...Middleware) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if name != allHandlers {
		c.conf(name).use(middleware...)
		return
	}

	c.defaultMiddleware = append(c.defaultMiddleware, middleware...)

	for _, conf := range c.confs {
		conf.setGlobalMiddleware(c.defaultMiddleware)
	}
}

func (c *Controller) UseDefault(middleware ...Middleware) {
	c.Use(allHandlers, middleware...)
}
