package flume

import (
	"log/slog"
	"maps"
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
		cfg.setSink(c.defaultSink, true, true)
		if c.confs == nil {
			c.confs = map[string]*conf{}
		}
		c.confs[name] = cfg
	}

	return cfg
}

// SetSink configures handlers with the given name to use the given sink, overriding
// the default sink.
// If name is `*`, this sets the default sink, and is equivalent to SetDefaultSink().
func (c *Controller) SetSink(name string, sink slog.Handler) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if name != allHandlers {
		c.conf(name).setSink(sink, false, false)
		return
	}

	c.defaultSink = sink

	for _, h := range c.confs {
		h.setSink(sink, true, false)
	}
}

// Sink returns the sink used by the named handler.
func (c *Controller) Sink(name string) slog.Handler {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if name == allHandlers {
		return c.defaultSink
	}

	return c.conf(name).sink
}

func (c *Controller) ClearSink(name string) {
	if name == allHandlers {
		// noop
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.conf(name).setSink(c.defaultSink, true, true)
}

// SetDefaultSink configures the default sink used by handlers.
func (c *Controller) SetDefaultSink(handler slog.Handler) {
	c.SetSink(allHandlers, handler)
}

// DefaultSink returns the default sink used by handlers.
func (c *Controller) DefaultSink() slog.Handler {
	return c.Sink(allHandlers)
}

// SetSinks configures sinks for named handlers in a batch.  All handlers will be
// updated in a single atomic operation. e.g.:
//
//	ctl.SetSinks(map[string]slog.Handler{"blue":slog.NewTextHandler(os.Stdout, nil)}, false)
//
// The default sink can optionally be set using the key `*`.
//
// If `replace` is true, then all existing named sinks will be cleared first, then the new
// sinks will be applied.  If `replace` is false, the new sinks will be applied on
// top of whatever sinks are already configured.  `replace` does not affect the
// default sink.  The default sink will only change if the key `*` is in the map,
// regardless of the value of `replace`.
func (c *Controller) SetSinks(newSinks map[string]slog.Handler, replace bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	newSinks = maps.Clone(newSinks)

	if defSink, ok := newSinks[allHandlers]; ok {
		c.defaultSink = defSink
		delete(newSinks, allHandlers)
	}

	for name, conf := range c.confs {
		if sink, ok := newSinks[name]; ok {
			conf.setSink(sink, false, replace)
			delete(newSinks, name)
		} else {
			conf.setSink(c.defaultSink, true, replace)
		}
	}

	for name, sink := range newSinks {
		c.conf(name).setSink(sink, false, replace)
	}
}

// ClearSinks removes all named sinks.  It will not affect the default sink.
func (c *Controller) ClearSinks() {
	c.SetSinks(nil, true)
}

// SetLevel sets the log level for handlers with the given name.
func (c *Controller) SetLevel(name string, l slog.Level) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if name != allHandlers {
		c.conf(name).setLevel(l, false, false)
		return
	}

	c.defaultLevel = l

	for _, h := range c.confs {
		h.setLevel(l, true, false)
	}
}

func (c *Controller) ClearLevel(name string) {
	if name == allHandlers {
		// noop
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.conf(name).setLevel(c.defaultLevel, true, true)
}

// Level returns the configured log level for the named handler.
func (c *Controller) Level(name string) slog.Level {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if name == allHandlers {
		return c.defaultLevel
	}

	return c.conf(name).lvl.Level()
}

// SetDefaultLevel sets the default log level for handlers.
func (c *Controller) SetDefaultLevel(l slog.Level) {
	c.SetLevel(allHandlers, l)
}

func (c *Controller) DefaultLevel() slog.Level {
	return c.Level(allHandlers)
}

func (c *Controller) SetLevels(newLevels map[string]slog.Level, replace bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	newLevels = maps.Clone(newLevels)

	if defLvl, ok := newLevels[allHandlers]; ok {
		c.defaultLevel = defLvl
		delete(newLevels, allHandlers)
	}

	for name, conf := range c.confs {
		if lvl, ok := newLevels[name]; ok {
			conf.setLevel(lvl, false, replace)
			delete(newLevels, name)
		} else {
			conf.setLevel(c.defaultLevel, true, replace)
		}
	}

	for name, lvl := range newLevels {
		c.conf(name).setLevel(lvl, false, replace)
	}
}

func (c *Controller) ClearLevels() {
	c.SetLevels(nil, true)
}

// Use applies middleware to the sink for flume handlers with the given name.
//
// This middleware will be applied *in addition to* the default middleware.
// See [Controller.UseDefault]
func (c *Controller) Use(name string, middleware ...Middleware) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if name != allHandlers {
		c.conf(name).use(c.defaultMiddleware, false, middleware...)
		return
	}

	c.defaultMiddleware = append(c.defaultMiddleware, middleware...)

	for _, conf := range c.confs {
		conf.use(c.defaultMiddleware, false)
	}
}

// UseDefault applies middleware to the sink for all handlers.
//
// Default middleware will be applied *in addition to* the middleware applied to named
// handlers with Use().
//
// During a call to Handle(), default middleware is invoked first, then specific middleware.
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

// SetMiddleware appends, or replaces, middleware in a single atomic operation.
// If `replace` is true,
func (c *Controller) SetMiddleware(newMW map[string][]Middleware, replace bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	newMW = maps.Clone(newMW)
	defMW := newMW[allHandlers]
	if replace {
		c.defaultMiddleware = defMW
	} else {
		c.defaultMiddleware = append(c.defaultMiddleware, defMW...)
	}
	delete(newMW, allHandlers)

	for name, conf := range c.confs {
		conf.use(c.defaultMiddleware, replace, newMW[name]...)
		delete(newMW, name)
	}

	for name, mw := range newMW {
		c.conf(name).use(c.defaultMiddleware, replace, mw...)
	}
}

// ClearMiddleware removes all global and named middleware.
func (c *Controller) ClearMiddleware() {
	c.SetMiddleware(nil, true)
}
