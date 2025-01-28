package flume

import (
	"bytes"
	"context"
	"log/slog"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func replaceKey(key string, newAttr slog.Attr) func([]string, slog.Attr) slog.Attr {
	return func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == key {
			return newAttr
		}

		return a
	}
}

func TestHandlers(t *testing.T) {
	tests := []handlerTest{
		{
			name: "nil",
			handlerFn: func(_ *testing.T, _ *bytes.Buffer, _ *slog.HandlerOptions) slog.Handler {
				return NewController(nil).Handler("h1")
			},
		},
		{
			name: "factory constructor",
			want: `{"level": "INFO", "logger": "h1", "msg":"hi"}`,
			json: true,
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				return NewController(slog.NewJSONHandler(buf, opts)).Handler("h1")
			},
		},
		{
			name: "change default sink before handler",
			want: `{"level": "INFO", "logger": "h1", "msg":"hi"}`,
			json: true,
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(nil)
				f.SetDefaultSink(slog.NewJSONHandler(buf, opts))

				return f.Handler("h1")
			},
		},
		{
			name: "change default sink after handler",
			want: "level=INFO msg=hi logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				h := f.Handler("h1")
				f.SetDefaultSink(slog.NewTextHandler(buf, opts))

				return h
			},
		},
		{
			name: "change other sink before handler",
			want: `{"level":  "INFO", "logger": "h1", "msg":"hi"}`,
			json: true,
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				f.SetSink("h2", slog.NewTextHandler(buf, opts))

				return f.Handler("h1")
			},
		},
		{
			name: "change sink before handler",
			want: "level=INFO msg=hi logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				f.SetSink("h1", slog.NewTextHandler(buf, opts))

				return f.Handler("h1")
			},
		},
		{
			name: "change sink after handler",
			want: "level=INFO msg=hi logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				h := f.Handler("h1")
				f.SetSink("h1", slog.NewTextHandler(buf, opts))

				return h
			},
		},
		{
			name: "WithXXX",
			want: "level=INFO msg=hi logger=h1 props.color=red\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewTextHandler(buf, opts))
				h := f.Handler("h1")
				h = h.WithGroup("props")
				h = h.WithAttrs([]slog.Attr{slog.String("color", "red")})
				return h
			},
		},
		{
			name: "change sink after WithXXX",
			want: "level=INFO msg=hi logger=h1 size=big props.color=red props.address.street=mockingbird\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				h := f.Handler("h1")
				h = h.WithAttrs([]slog.Attr{slog.String("size", "big")})
				h = h.WithGroup("props").WithAttrs([]slog.Attr{slog.String("color", "red")}).WithGroup("address").WithAttrs([]slog.Attr{slog.String("street", "mockingbird")})
				f.SetSink("h1", slog.NewTextHandler(buf, opts))

				return h
			},
		},
		{
			name: "change sink before WithXXX",
			want: "level=INFO msg=hi logger=h1 size=big props.color=red props.address.street=mockingbird\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				f.SetSink("h1", slog.NewTextHandler(buf, opts))
				h := f.Handler("h1")
				h = h.WithAttrs([]slog.Attr{slog.String("size", "big")})
				h = h.WithGroup("props").WithAttrs([]slog.Attr{slog.String("color", "red")}).WithGroup("address").WithAttrs([]slog.Attr{slog.String("street", "mockingbird")})

				return h
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
			name: "set other logger to nil",
			want: `{"level":  "INFO", "logger": "h1", "msg":"hi"}`,
			json: true,
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				f := NewController(slog.NewJSONHandler(buf, opts))
				h := f.Handler("h1")
				f.SetSink("h2", nil)

				return h
			},
		},
		{
			name: "default",
			want: `{"level":  "INFO", "logger": "def1", "msg":"hi"}`,
			json: true,
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				Default().SetDefaultSink(slog.NewJSONHandler(buf, opts))
				return Handler("def1")
			},
		},
		{
			name: "default with text",
			want: "level=INFO msg=hi logger=def1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				Default().SetDefaultSink(slog.NewTextHandler(buf, opts))
				return Handler("def1")
			},
		},
		{
			name: "default with specific logger",
			want: "level=INFO msg=hi logger=def2\n",
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
			name:  "set default log level",
			level: slog.LevelDebug,
			want:  "level=DEBUG msg=hi logger=def1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				Default().SetDefaultSink(slog.NewTextHandler(buf, opts))
				Default().SetDefaultLevel(slog.LevelDebug)

				return Handler("def1")
			},
		},
		{
			name:  "set specific log level",
			level: slog.LevelDebug,
			want:  "level=DEBUG msg=hi logger=TestHandlers/set_specific_log_level\n",
			handlerFn: func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				Default().SetDefaultSink(slog.NewTextHandler(buf, opts))
				Default().SetDefaultLevel(slog.LevelInfo)
				Default().SetLevel(t.Name(), slog.LevelDebug)

				return Handler(t.Name())
			},
		},
		{
			name: "ensure cloned slices",
			want: "level=INFO msg=hi logger=h1 props.flavor=lemon props.color=red\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))
				h1 := ctl.Handler("h1").WithGroup("props").WithAttrs([]slog.Attr{slog.String("flavor", "lemon")})
				h2 := h1.WithAttrs([]slog.Attr{slog.String("color", "red")}) // this appended a group to an internal slice
				h3 := h1.WithAttrs([]slog.Attr{slog.String("size", "big")})  // so did this
				runtime.KeepAlive(h3)
				// need to make sure that h2 and h3 have completely independent states, and one group didn't over the other's group
				// to test this, I need to install a ReplaceAttr function, since that's all the group slice is
				// used for
				return h2
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.Run)
	}
}

const emptyMsg = "<<<EMPTY>>>"

type handlerTest struct {
	name string
	json bool
	want string
	// defaults to "hi".  set to emptyMsg to use an empty message.
	msg   string
	level slog.Level
	args  []any

	handlerFn func(t *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler
}

func (ht handlerTest) Run(t *testing.T) {
	t.Helper()
	buf := bytes.NewBuffer(nil)
	h := ht.handlerFn(t, buf, &slog.HandlerOptions{ReplaceAttr: removeKeys(slog.TimeKey)})
	l := slog.New(h)

	msg := ht.msg
	switch msg {
	case "":
		msg = "hi"
	case emptyMsg:
		msg = ""
	}

	l.Log(context.Background(), ht.level, msg, ht.args...)
	switch {
	case ht.want == "":
		assert.Empty(t, buf.String())
	case ht.json:
		assert.JSONEq(t, ht.want, buf.String())
	default:
		assert.Equal(t, ht.want, buf.String())
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
			wantJSON: `{"level":  "INFO", "logger": "h1", "msg":"hi"}`,
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
			wantJSON: `{"level":  "DEBUG", "logger": "h1", "msg":"hi"}`,
			handlerFunc: func(_ *testing.T, f *Controller) slog.Handler {
				h := f.Handler("h1")
				f.SetLevel("h1", slog.LevelDebug)

				return h
			},
		},
		{
			name:     "set handler specific before construction",
			level:    slog.LevelDebug,
			wantJSON: `{"level":  "DEBUG", "logger": "h1", "msg":"hi"}`,
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
			wantJSON: `{"level":  "DEBUG", "logger": "h1", "msg":"hi", "color":"red"}`,
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
			wantJSON: `{"level":  "DEBUG", "logger": "h1", "msg":"hi", "color":"red"}`,
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
