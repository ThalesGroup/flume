package flume

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/ansel1/merry"
	"github.com/stretchr/testify/assert"
)

func TestHandlerStateWeakRef(t *testing.T) {
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

	h := &handler{newHandlerState(&slog.LevelVar{}, slog.NewJSONHandler(os.Stdout, nil), nil, "")}
	logger := slog.New(h)

	logger.Info("Hi")

	// useChildLogger creates a child logger, uses it, then throws it away
	useChildLogger := func(t *testing.T, logger *slog.Logger, dynHandler *handler) {
		child := logger.WithGroup("colors").With("blue", true)
		child.Info("There")
		dynHandler.setDelegateHandler(slog.NewTextHandler(os.Stdout, nil), true)
		logger.Info("Hi again")
		child.Info("There")

		assert.Len(t, dynHandler.children, 1)
	}

	useChildLogger(t, logger, h)

	runtime.GC()
	runtime.GC()

	// need to lock before checking size of children or race detector complains
	h.Lock()
	defer h.Unlock()

	assert.Empty(t, h.children)

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
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				return NewFactory(nil).NewHandler("h1")
			},
		},
		{
			name:     "factory constructor",
			wantJSON: `{"level":  "INFO", "logger": "h1", "msg":"hi"}`,
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				return NewFactory(slog.NewJSONHandler(buf, opts)).NewHandler("h1")
			},
		},
		{
			name:     "change default before construction",
			wantJSON: `{"level":  "INFO", "logger": "h1", "msg":"hi"}`,
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewFactory(nil)
				f.SetDefaultHandler(slog.NewJSONHandler(buf, opts))
				return f.NewHandler("h1")
			},
		},
		{
			name:     "change default after construction",
			wantText: "level=INFO msg=hi logger=h1\n",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewFactory(slog.NewJSONHandler(buf, opts))
				h := f.NewHandler("h1")
				f.SetDefaultHandler(slog.NewTextHandler(buf, opts))
				return h
			},
		},
		{
			name:     "change other handler before construction",
			wantJSON: `{"level":  "INFO", "logger": "h1", "msg":"hi"}`,
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewFactory(slog.NewJSONHandler(buf, opts))
				f.SetLoggerHandler("h2", slog.NewTextHandler(buf, opts))
				return f.NewHandler("h1")
			},
		},
		{
			name:     "change specific before construction",
			wantText: "level=INFO msg=hi logger=h1\n",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewFactory(slog.NewJSONHandler(buf, opts))
				f.SetLoggerHandler("h1", slog.NewTextHandler(buf, opts))
				return f.NewHandler("h1")
			},
		},
		{
			name:     "change specific after construction",
			wantText: "level=INFO msg=hi logger=h1\n",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewFactory(slog.NewJSONHandler(buf, opts))
				h := f.NewHandler("h1")
				f.SetLoggerHandler("h1", slog.NewTextHandler(buf, opts))
				return h
			},
		},
		{
			name:     "cascade to children after construction",
			wantText: "level=INFO msg=hi logger=h1 color=red\n",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewFactory(slog.NewJSONHandler(buf, opts))
				h := f.NewHandler("h1")
				c := h.WithAttrs([]slog.Attr{slog.String("color", "red")})
				f.SetLoggerHandler("h1", slog.NewTextHandler(buf, opts))
				return c
			},
		},
		{
			name:     "cascade to children before construction",
			wantText: "level=INFO msg=hi logger=h1 color=red\n",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewFactory(slog.NewJSONHandler(buf, opts))
				f.SetLoggerHandler("h1", slog.NewTextHandler(buf, opts))
				h := f.NewHandler("h1")
				c := h.WithAttrs([]slog.Attr{slog.String("color", "red")})
				return c
			},
		},
		{
			name: "set default to nil",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewFactory(slog.NewJSONHandler(buf, opts))
				h := f.NewHandler("h1")
				f.SetDefaultHandler(nil)
				return h
			},
		},
		{
			name: "set logger to nil",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewFactory(slog.NewJSONHandler(buf, opts))
				h := f.NewHandler("h1")
				f.SetLoggerHandler("h1", nil)
				return h
			},
		},
		{
			name:     "set other logger to nil",
			wantJSON: `{"level":  "INFO", "logger": "h1", "msg":"hi"}`,
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewFactory(slog.NewJSONHandler(buf, opts))
				h := f.NewHandler("h1")
				f.SetLoggerHandler("h2", nil)
				return h
			},
		},
		{
			name:     "default",
			wantJSON: `{"level":  "INFO", "logger": "def1", "msg":"hi"}`,
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				SetDefaultHandler(slog.NewJSONHandler(buf, opts))
				return NewHandler("def1")
			},
		},
		{
			name:     "default with text",
			wantText: "level=INFO msg=hi logger=def1\n",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				SetDefaultHandler(slog.NewTextHandler(buf, opts))
				return NewHandler("def1")
			},
		},
		{
			name:     "default with specific logger",
			wantText: "level=INFO msg=hi logger=def2\n",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				SetDefaultHandler(slog.NewJSONHandler(buf, opts))
				SetLoggerHandler("def2", slog.NewTextHandler(buf, opts))
				return NewHandler("def2")
			},
		},
		{
			name:  "default log level",
			level: slog.LevelDebug,
			// wantText: "level=INFO msg=hi logger=def1\n",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				SetDefaultHandler(slog.NewTextHandler(buf, opts))
				return NewHandler("def1")
			},
		},
		{
			name:     "set default log level",
			level:    slog.LevelDebug,
			wantText: "level=DEBUG msg=hi logger=def1\n",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				SetDefaultHandler(slog.NewTextHandler(buf, opts))
				SetDefaultLevel(slog.LevelDebug)
				return NewHandler("def1")
			},
		},
		{
			name:     "set specific log level",
			level:    slog.LevelDebug,
			wantText: "level=DEBUG msg=hi logger=TestHandlers/set_specific_log_level\n",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				SetDefaultHandler(slog.NewTextHandler(buf, opts))
				SetDefaultLevel(slog.LevelInfo)
				SetLoggerLevel(t.Name(), slog.LevelDebug)
				return NewHandler(t.Name())
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
		handlerFunc func(t *testing.T, f *Factory) slog.Handler
	}{
		{
			name:     "default info",
			wantJSON: `{"level":  "INFO", "logger": "h1", "msg":"hi"}`,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				return f.NewHandler("h1")
			},
		},
		{
			name:  "default debug",
			level: slog.LevelDebug,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				return f.NewHandler("h1")
			},
		},
		{
			name: "change default after construction",
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				h := f.NewHandler("h1")
				f.SetDefaultLevel(slog.LevelWarn)
				return h
			},
		},
		{
			name: "change default before construction",
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				f.SetDefaultLevel(slog.LevelWarn)
				return f.NewHandler("h1")
			},
		},
		{
			name:     "set handler specific after construction",
			level:    slog.LevelDebug,
			wantJSON: `{"level":  "DEBUG", "logger": "h1", "msg":"hi"}`,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				h := f.NewHandler("h1")
				f.SetLoggerLevel("h1", slog.LevelDebug)
				return h
			},
		},
		{
			name:     "set handler specific before construction",
			level:    slog.LevelDebug,
			wantJSON: `{"level":  "DEBUG", "logger": "h1", "msg":"hi"}`,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				f.SetLoggerLevel("h1", slog.LevelDebug)
				return f.NewHandler("h1")
			},
		},
		{
			name:  "set a different handler specific after construction",
			level: slog.LevelDebug,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				h := f.NewHandler("h1")
				f.SetLoggerLevel("h2", slog.LevelDebug)
				return h
			},
		},
		{
			name:  "set a different handler specific before construction",
			level: slog.LevelDebug,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				f.SetLoggerLevel("h2", slog.LevelDebug)
				return f.NewHandler("h1")
			},
		},
		{
			name:     "cascade to children",
			level:    slog.LevelDebug,
			wantJSON: `{"level":  "DEBUG", "logger": "h1", "msg":"hi", "color":"red"}`,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				f.SetLoggerLevel("h1", slog.LevelDebug)
				h := f.NewHandler("h1")
				c := h.WithAttrs([]slog.Attr{slog.String("color", "red")})
				return c
			},
		},
		{
			name:     "update after creating child",
			level:    slog.LevelDebug,
			wantJSON: `{"level":  "DEBUG", "logger": "h1", "msg":"hi", "color":"red"}`,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				h := f.NewHandler("h1")
				c := h.WithAttrs([]slog.Attr{slog.String("color", "red")})
				f.SetLoggerLevel("h1", slog.LevelDebug)
				return c
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := bytes.NewBuffer(nil)
			f := NewFactory(slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: removeKeys(slog.TimeKey)}))

			l := slog.New(test.handlerFunc(t, f))
			l.Log(context.Background(), test.level, "hi")
			if test.wantJSON == "" {
				assert.Empty(t, buf.String())
			} else {
				assert.JSONEq(t, test.wantJSON, buf.String())
			}
		})
	}
}

func TestAttrs(t *testing.T) {
	tests := []struct {
		name     string
		wantText string
		args     []any
	}{
		{
			name:     "bare value",
			wantText: "level=INFO msg=hi logger=blue !BADKEY=green\n",
			args:     []any{"green"},
		},
		{
			name:     "bare error",
			wantText: "level=INFO msg=hi logger=blue !BADKEY=boom\n",
			args:     []any{errors.New("boom")},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := bytes.NewBuffer(nil)
			f := NewFactory(slog.NewTextHandler(buf, &slog.HandlerOptions{ReplaceAttr: removeKeys(slog.TimeKey)}))

			l := slog.New(f.NewHandler("blue"))
			l.Info("hi", test.args...)
			assert.Equal(t, test.wantText, buf.String())
		})
	}
}

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
