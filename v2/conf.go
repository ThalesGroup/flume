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
	delegate       slog.Handler
	customDelegate bool
	sync.Mutex
	states map[*state]struct{}
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
