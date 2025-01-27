package flume

import (
	"log/slog"
	"sync/atomic"
)

type conf struct {
	name                  string
	lvl                   *slog.LevelVar
	customLvl, customSink bool
	// sink is the ultimate, final handler
	// delegate is the sink wrapped with middleware
	sink             slog.Handler
	delegatePtr      atomic.Pointer[slog.Handler]
	middleware       []Middleware
	globalMiddleware []Middleware
}

func (c *conf) setSink(sink slog.Handler, isDefault bool) {
	if c.customSink && isDefault {
		return
	}

	c.customSink = !isDefault

	if sink == nil {
		sink = noop
	}

	c.sink = sink

	c.rebuildDelegate()
}

// rebuildDelegate updates the delegate handler for all states associated with this configuration.
// This function is not thread safe, and should only be called while holding the conf mutex.
func (c *conf) rebuildDelegate() {
	// apply middleware, first the local middleware, then global in reverse order
	h := c.sink
	for i := len(c.middleware) - 1; i >= 0; i-- {
		h = c.middleware[i].Apply(h)
	}
	for i := len(c.globalMiddleware) - 1; i >= 0; i-- {
		h = c.globalMiddleware[i].Apply(h)
	}

	h = h.WithAttrs([]slog.Attr{slog.String(LoggerKey, c.name)})

	c.delegatePtr.Store(&h)
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

func (c *conf) use(middleware ...Middleware) {
	c.middleware = append(c.middleware, middleware...)

	c.rebuildDelegate()
}

func (c *conf) setGlobalMiddleware(middleware []Middleware) {
	c.globalMiddleware = middleware

	c.rebuildDelegate()
}

func (c *conf) handler() slog.Handler {
	return &handler{
		basePtr: &c.delegatePtr,
		level:   c.lvl,
	}
}
