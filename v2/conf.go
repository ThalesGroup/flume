package flume

import (
	"log/slog"
	"runtime"
	"sync"
)

type conf struct {
	name           string
	lvl            *slog.LevelVar
	customLvl      bool
	sink, delegate slog.Handler
	customSink     bool
	sync.Mutex
	states           map[*state]struct{}
	middleware       []Middleware
	globalMiddleware []Middleware
}

func (c *conf) setSink(sink slog.Handler, isDefault bool) {
	c.Lock()
	defer c.Unlock()

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

	c.delegate = h

	for s := range c.states {
		s.setDelegate(c.delegate)
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

func (c *conf) use(middleware ...Middleware) {
	c.Lock()
	defer c.Unlock()

	c.middleware = append(c.middleware, middleware...)

	c.rebuildDelegate()
}

func (c *conf) setGlobalMiddleware(middleware []Middleware) {
	c.Lock()
	defer c.Unlock()

	c.globalMiddleware = middleware

	c.rebuildDelegate()
}

func (c *conf) newHandler(attrs []slog.Attr, groups []string) *handler {
	c.Lock()
	defer c.Unlock()

	s := &state{
		attrs:  attrs,
		groups: groups,
		level:  c.lvl,
		conf:   c,
	}
	s.setDelegate(c.delegate)

	c.states[s] = struct{}{}

	h := &handler{
		state: s,
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
