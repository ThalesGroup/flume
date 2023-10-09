package flume

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/assert"
	"log/slog"
	"os"
	"runtime"
	"testing"
)

func TestHandlerStateWeakRef(t *testing.T) {

	h := &handler{newHandlerState(&slog.LevelVar{}, slog.NewJSONHandler(os.Stdout, nil), nil, "")}
	logger := slog.New(h)

	logger.Info("Hi")

	doit(t, logger, h)

	runtime.GC()
	runtime.GC()

	// need to lock before checking size of children or race detector complains
	h.Lock()
	defer h.Unlock()

	assert.Empty(t, h.children)

}

func doit(t *testing.T, logger *slog.Logger, dynHandler *handler) {
	child := logger.WithGroup("colors").With("blue", true)
	child.Info("There")
	dynHandler.setDelegateHandler(slog.NewTextHandler(os.Stdout, nil), true)
	logger.Info("Hi again")
	child.Info("There")

	assert.Len(t, dynHandler.children, 1)
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
				return f.newHandler("h1")
			},
		},
		{
			name:  "default debug",
			level: slog.LevelDebug,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				return f.newHandler("h1")
			},
		},
		{
			name: "change default after construction",
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				h := f.newHandler("h1")
				f.SetDefaultLevel(slog.LevelWarn)
				return h
			},
		},
		{
			name: "change default before construction",
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				f.SetDefaultLevel(slog.LevelWarn)
				return f.newHandler("h1")
			},
		},
		{
			name:     "set handler specific after construction",
			level:    slog.LevelDebug,
			wantJSON: `{"level":  "DEBUG", "logger": "h1", "msg":"hi"}`,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				h := f.newHandler("h1")
				f.SetLevel("h1", slog.LevelDebug)
				return h
			},
		},
		{
			name:     "set handler specific before construction",
			level:    slog.LevelDebug,
			wantJSON: `{"level":  "DEBUG", "logger": "h1", "msg":"hi"}`,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				f.SetLevel("h1", slog.LevelDebug)
				return f.newHandler("h1")
			},
		},
		{
			name:  "set a different handler specific after construction",
			level: slog.LevelDebug,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				h := f.newHandler("h1")
				f.SetLevel("h2", slog.LevelDebug)
				return h
			},
		},
		{
			name:  "set a different handler specific before construction",
			level: slog.LevelDebug,
			handlerFunc: func(t *testing.T, f *Factory) slog.Handler {
				f.SetLevel("h2", slog.LevelDebug)
				return f.newHandler("h1")
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
