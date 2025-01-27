package flume

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStateWeakRef(t *testing.T) {
	// This ensures that child handlers are garbage collected.  slog methods like WithGroup() create
	// copies of their handlers, and flume's handlers keep references to those child handlers, so that
	// changes from the Factory can propagate down through all the handler clones.
	//
	// But when the child loggers are garbage collected, the handlers inside them should be collected
	// to.  The reference from the parent handler to the child handler shouldn't prevent the child handler
	// from being collected, so it needs to be something like a weakref in java.  Golang doesn't have weakref
	// yet, but we use some fancy finalizers to mimic them.
	//
	// Test: create a logger/handler.  Pass it to another function, which creates a child logger/handler, uses
	// it, then discards it.
	//
	// After this function returns, the child logger should be garbage collected, and the parent *handler* should
	// no longer be holding a ref to the child handler.

	ctl := NewController(slog.NewTextHandler(os.Stdout, nil))
	h := ctl.Handler("")

	conf := ctl.conf("")
	lenStates := func() int {
		// need to lock before checking size of children or race detector complains
		conf.Lock()
		defer conf.Unlock()

		return len(conf.states)
	}

	logger := slog.New(h)

	logger.Info("Hi")

	// useChildLogger creates a child logger, uses it, then throws it away
	useChildLogger := func(t *testing.T, logger *slog.Logger) *slog.Logger {
		child := logger.WithGroup("colors").With("blue", true)
		child.Info("There", "flavor", "vanilla")

		assert.Equal(t, 3, lenStates())

		grandchild := child.With("size", "big")

		assert.Equal(t, 4, lenStates())

		return grandchild
	}

	grandchild := useChildLogger(t, logger)

	// Need to run 2 gc cycles.  The first one should collect the handler and run the finalizer, then the second
	// should collect the state orphaned by the finalizer.
	runtime.GC()
	runtime.GC()

	// after gc, there should be two states left, the original referenced by `h`, and the grandchild.
	assert.Equal(t, 2, lenStates())

	// to make this test reliable, we need to ensure that the compiler doesn't allow h to be gc'd before we've
	// asserted the length of states.
	runtime.KeepAlive(h)

	// make sure changes to the root ancestor still cascade down to all ancestors,
	// even if links in the tree have already been released.
	assert.IsType(t, &slog.TextHandler{}, grandchild.Handler().(*handler).delegate())
	ctl.SetDefaultSink(slog.NewJSONHandler(os.Stdout, nil))
	assert.IsType(t, &slog.JSONHandler{}, grandchild.Handler().(*handler).delegate())
}

// removeKeys returns a function suitable for HandlerOptions.ReplaceAttr
// that removes all Attrs with the given keys.
func removeKeys(keys ...string) func([]string, slog.Attr) slog.Attr {
	return func(_ []string, a slog.Attr) slog.Attr {
		for _, k := range keys {
			if a.Key == k {
				return slog.Attr{}
			}
		}

		return a
	}
}

func TestHandlers(t *testing.T) {
	tests := []struct {
		name      string
		wantJSON  string
		wantText  string
		level     slog.Level
		handlerFn func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler
	}{
		{
			name: "nil",
			handlerFn: func(_ *testing.T, _ *bytes.Buffer, _ *slog.HandlerOptions) slog.Handler {
				return NewController(nil).Handler("h1")
			},
		},
		{
			name:     "factory constructor",
			wantJSON: `{"level":  "INFO", LoggerKey: "h1", "msg":"hi"}`,
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				return NewController(slog.NewJSONHandler(buf, opts)).Handler("h1")
			},
		},
		{
			name:     "change default before construction",
			wantJSON: `{"level":  "INFO", LoggerKey: "h1", "msg":"hi"}`,
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(nil)
				f.SetDefaultSink(slog.NewJSONHandler(buf, opts))

				return f.Handler("h1")
			},
		},
		{
			name:     "change default after construction",
			wantText: "level=INFO msg=hi logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				h := f.Handler("h1")
				f.SetDefaultSink(slog.NewTextHandler(buf, opts))

				return h
			},
		},
		{
			name:     "change other handler before construction",
			wantJSON: `{"level":  "INFO", LoggerKey: "h1", "msg":"hi"}`,
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				f.SetSink("h2", slog.NewTextHandler(buf, opts))

				return f.Handler("h1")
			},
		},
		{
			name:     "change specific before construction",
			wantText: "level=INFO msg=hi logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				f.SetSink("h1", slog.NewTextHandler(buf, opts))

				return f.Handler("h1")
			},
		},
		{
			name:     "change specific after construction",
			wantText: "level=INFO msg=hi logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				h := f.Handler("h1")
				f.SetSink("h1", slog.NewTextHandler(buf, opts))

				return h
			},
		},
		{
			name:     "cascade to children after construction",
			wantText: "level=INFO msg=hi logger=h1 color=red\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				h := f.Handler("h1")
				c := h.WithAttrs([]slog.Attr{slog.String("color", "red")})
				f.SetSink("h1", slog.NewTextHandler(buf, opts))

				return c
			},
		},
		{
			name:     "cascade to children before construction",
			wantText: "level=INFO msg=hi logger=h1 color=red\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				f.SetSink("h1", slog.NewTextHandler(buf, opts))
				h := f.Handler("h1")
				c := h.WithAttrs([]slog.Attr{slog.String("color", "red")})

				return c
			},
		},
		{
			name: "set default to nil",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				h := f.Handler("h1")
				f.SetDefaultSink(nil)

				return h
			},
		},
		{
			name: "set logger to nil",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				h := f.Handler("h1")
				f.SetSink("h1", nil)

				return h
			},
		},
		{
			name:     "set other logger to nil",
			wantJSON: `{"level":  "INFO", LoggerKey: "h1", "msg":"hi"}`,
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				h := f.Handler("h1")
				f.SetSink("h2", nil)

				return h
			},
		},
		{
			name:     "default",
			wantJSON: `{"level":  "INFO", LoggerKey: "def1", "msg":"hi"}`,
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				Default().SetDefaultSink(slog.NewJSONHandler(buf, opts))
				return Handler("def1")
			},
		},
		{
			name:     "default with text",
			wantText: "level=INFO msg=hi logger=def1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				Default().SetDefaultSink(slog.NewTextHandler(buf, opts))
				return Handler("def1")
			},
		},
		{
			name:     "default with specific logger",
			wantText: "level=INFO msg=hi logger=def2\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				Default().SetDefaultSink(slog.NewJSONHandler(buf, opts))
				Default().SetSink("def2", slog.NewTextHandler(buf, opts))

				return Handler("def2")
			},
		},
		{
			name:  "default log level",
			level: slog.LevelDebug,
			// wantText: "level=INFO msg=hi logger=def1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				Default().SetDefaultSink(slog.NewTextHandler(buf, opts))
				return Handler("def1")
			},
		},
		{
			name:     "set default log level",
			level:    slog.LevelDebug,
			wantText: "level=DEBUG msg=hi logger=def1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				Default().SetDefaultSink(slog.NewTextHandler(buf, opts))
				Default().SetDefaultLevel(slog.LevelDebug)

				return Handler("def1")
			},
		},
		{
			name:     "set specific log level",
			level:    slog.LevelDebug,
			wantText: "level=DEBUG msg=hi logger=TestHandlers/set_specific_log_level\n",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				Default().SetDefaultSink(slog.NewTextHandler(buf, opts))
				Default().SetDefaultLevel(slog.LevelInfo)
				Default().SetLevel(t.Name(), slog.LevelDebug)

				return Handler(t.Name())
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := bytes.NewBuffer(nil)
			h := test.handlerFn(t, buf, &slog.HandlerOptions{ReplaceAttr: removeKeys(slog.TimeKey)})
			l := slog.New(h)
			l.Log(context.Background(), test.level, "hi")
			switch {
			case test.wantJSON != "":
				assert.JSONEq(t, test.wantJSON, buf.String())
			case test.wantText != "":
				assert.Equal(t, test.wantText, buf.String())
			default:
				assert.Empty(t, buf.String())
			}
		})
	}
}

func TestLevels(t *testing.T) {
	tests := []struct {
		name        string
		wantJSON    string
		level       slog.Level
		handlerFunc func(t *testing.T, f *Controller) slog.Handler
	}{
		{
			name:     "default info",
			wantJSON: `{"level":  "INFO", LoggerKey: "h1", "msg":"hi"}`,
			handlerFunc: func(_ *testing.T, f *Controller) slog.Handler {
				return f.Handler("h1")
			},
		},
		{
			name:  "default debug",
			level: slog.LevelDebug,
			handlerFunc: func(_ *testing.T, f *Controller) slog.Handler {
				return f.Handler("h1")
			},
		},
		{
			name: "change default after construction",
			handlerFunc: func(_ *testing.T, f *Controller) slog.Handler {
				h := f.Handler("h1")
				f.SetDefaultLevel(slog.LevelWarn)

				return h
			},
		},
		{
			name: "change default before construction",
			handlerFunc: func(_ *testing.T, f *Controller) slog.Handler {
				f.SetDefaultLevel(slog.LevelWarn)
				return f.Handler("h1")
			},
		},
		{
			name:     "set handler specific after construction",
			level:    slog.LevelDebug,
			wantJSON: `{"level":  "DEBUG", LoggerKey: "h1", "msg":"hi"}`,
			handlerFunc: func(_ *testing.T, f *Controller) slog.Handler {
				h := f.Handler("h1")
				f.SetLevel("h1", slog.LevelDebug)

				return h
			},
		},
		{
			name:     "set handler specific before construction",
			level:    slog.LevelDebug,
			wantJSON: `{"level":  "DEBUG", LoggerKey: "h1", "msg":"hi"}`,
			handlerFunc: func(_ *testing.T, f *Controller) slog.Handler {
				f.SetLevel("h1", slog.LevelDebug)
				return f.Handler("h1")
			},
		},
		{
			name:  "set a different handler specific after construction",
			level: slog.LevelDebug,
			handlerFunc: func(_ *testing.T, f *Controller) slog.Handler {
				h := f.Handler("h1")
				f.SetLevel("h2", slog.LevelDebug)

				return h
			},
		},
		{
			name:  "set a different handler specific before construction",
			level: slog.LevelDebug,
			handlerFunc: func(_ *testing.T, f *Controller) slog.Handler {
				f.SetLevel("h2", slog.LevelDebug)
				return f.Handler("h1")
			},
		},
		{
			name:     "cascade to children",
			level:    slog.LevelDebug,
			wantJSON: `{"level":  "DEBUG", LoggerKey: "h1", "msg":"hi", "color":"red"}`,
			handlerFunc: func(_ *testing.T, f *Controller) slog.Handler {
				f.SetLevel("h1", slog.LevelDebug)
				h := f.Handler("h1")
				c := h.WithAttrs([]slog.Attr{slog.String("color", "red")})

				return c
			},
		},
		{
			name:     "update after creating child",
			level:    slog.LevelDebug,
			wantJSON: `{"level":  "DEBUG", LoggerKey: "h1", "msg":"hi", "color":"red"}`,
			handlerFunc: func(_ *testing.T, f *Controller) slog.Handler {
				h := f.Handler("h1")
				c := h.WithAttrs([]slog.Attr{slog.String("color", "red")})
				f.SetLevel("h1", slog.LevelDebug)

				return c
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := bytes.NewBuffer(nil)
			ctl := NewController(slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: removeKeys(slog.TimeKey)}))

			l := slog.New(test.handlerFunc(t, ctl))
			l.Log(context.Background(), test.level, "hi")
			if test.wantJSON == "" {
				assert.Empty(t, buf.String())
			} else {
				assert.JSONEq(t, test.wantJSON, buf.String())
			}
		})
	}
}

//nolint:dupword
// func chainReplaceAttrs(funcs ...func(groups []string, a slog.Attr) slog.Attr) func(groups []string, a slog.Attr) slog.Attr {
// 	return func(groups []string, a slog.Attr) slog.Attr {
// 		for _, f := range funcs {
// 			a = f(groups, a)
// 		}
// 		return a
// 	}
// }
//
// func TestLogging(t *testing.T) {
// 	SetDefaultHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: chainReplaceAttrs(
// 		removeKeys(slog.TimeKey),
// 		func(_ []string, a slog.Attr) slog.Attr {
// 			if a.Value.Kind() == slog.KindAny {
// 				if e, ok := a.Value.Any().(error); ok {
// 					a.Value = slog.AnyValue(ErrorLogValue{err: e})
// 				}
// 			}
// 			return a
// 		},
// 	)}))
// 	logger := slog.New(NewHandler("main"))
// 	logger.Info("hi", "color", merry.New("boom"))
// }
//
// type ErrorLogValue struct {
// 	err error
// }
//
// func (e ErrorLogValue) LogValue() slog.Value {
// 	return slog.GroupValue(
// 		slog.String("msg", e.err.Error()),
// 		slog.String("verbose", merry.Details(e.err)),
// 	)
// }
