package flume

import (
	"log/slog"
	"sync"
)

const (
	// LoggerKey is the key which stores the name of the logger.  The name was the argument
	// passed to Controller.NewLogger() or Controller.NewHandler()
	LoggerKey   = "logger"
	allHandlers = "*"
)

// Controller is the core of flume.  It creates and controls a set of flume slog.Handlers.
//
// Flume handlers have a name (assigned during creation), and they
// add a `logger=<name>` attribute to each log entry.
//
// The Controller controls the log level and sink
// for all the flume handlers it controls, and can reconfigure them at runtime.
//
// Flume handlers ultimately delegate log records to an underlying `sink` slog.Handler.  The
// Controller has a default sink to which all flume handlers managed by the Controller
// will send their log records.  The Controller also allows overriding the default sink
// for particular flume handlers.
//
// Package-level functions mirror of most of Controller's methods, which delegate to a
// default package-level Controller.
type Controller struct {
	defaultLevel      slog.Level
	defaultSink       slog.Handler
	defaultMiddleware []Middleware

	confs map[string]*conf
	mutex sync.Mutex
}

// NewController creates a new flume Controller with the specified default sink
// handler.
func NewController(sink slog.Handler) *Controller {
	return &Controller{defaultSink: sink}
}

// Logger is a convenience method for creating new slog.Loggers.  Equiavalent
// to `slog.New(c.Handler(name))`.
// Example:
//
//	logger := c.Logger(name)
func (c *Controller) Logger(name string) *slog.Logger {
	return slog.New(c.Handler(name))
}

// Handler creates a new handler with the given name.
// Example:
//
//	logger := slog.New(c.Handler(name))
func (c *Controller) Handler(name string) slog.Handler {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.conf(name).handler()
}

// conf locates or creates a new conf for the given name.  The Controller
// maintains one conf instance for each name registered with the controller.
// The conf stores the configuration for that name (sink, middleware, log level),
// and maintains a set of weak refs to handlers which have been created, signalling
// those handlers when the configuration changes.
func (c *Controller) conf(name string) *conf {
	cfg, ok := c.confs[name]
	if !ok {
		levelVar := &slog.LevelVar{}
		levelVar.Set(c.defaultLevel)
		cfg = &conf{
			name:             name,
			lvl:              levelVar,
			globalMiddleware: c.defaultMiddleware,
		}
		cfg.setSink(c.defaultSink, true)
		if c.confs == nil {
			c.confs = map[string]*conf{}
		}
		c.confs[name] = cfg
	}

	return cfg
}

// SetSink configures flume handlers with the given name to use the given sink, overriding
// the default sink.
func (c *Controller) SetSink(name string, sink slog.Handler) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if name != allHandlers {
		c.conf(name).setSink(sink, false)
		return
	}

	c.defaultSink = sink

	for _, h := range c.confs {
		h.setSink(sink, true)
	}
}

func (c *Controller) Sink(name string) slog.Handler {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if name == allHandlers {
		return c.defaultSink
	}

	return c.conf(name).sink
}

// SetDefaultSink configures the default sink handler for all flume handlers managed
// by this controller.
func (c *Controller) SetDefaultSink(handler slog.Handler) {
	c.SetSink(allHandlers, handler)
}

func (c *Controller) DefaultSink() slog.Handler {
	return c.Sink(allHandlers)
}

// SetLevel sets the log level for flume handlers with the given name, overriding
// the default level.
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

func (c *Controller) Level(name string) slog.Level {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if name == allHandlers {
		return c.defaultLevel
	}

	return c.conf(name).lvl.Level()
}

// SetDefaultLevel sets the default log level for all flume handlers managed by this
// controller.
func (c *Controller) SetDefaultLevel(l slog.Level) {
	c.SetLevel(allHandlers, l)
}

func (c *Controller) DefaultLevel() slog.Level {
	return c.Level(allHandlers)
}

// Use applies middleware to the sink for flume handlers with the given name.
//
// This middleware will be applied *in addition to* the default middleware.
// See [Controller.UseDefault]
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

// UseDefault applies middleware to the sink for all flume handlers managed by this controller.
//
// Default middleware will be applied *in addition to* the middleware applied to individual
// handlers with Use().
//
// During a call to Handle(), default middleware is invoked first, then this middleware.
// The middleware will be invoked in the order it is passed to Use().  Example:
//
//	ctl.Use("http", Format(), Summarize())
//	ctl.UseDefault(Redact(), Filter())
//
// In this example, each Handle() call would invoke Redact, then Filter, then Format, then Summarize,
// then the sink.
func (c *Controller) UseDefault(middleware ...Middleware) {
	c.Use(allHandlers, middleware...)
}
