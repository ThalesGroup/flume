package flume

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// removeKeys returns a function suitable for HandlerOptions.ReplaceAttr
// that removes all Attrs with the given keys.
func removeKeys(keys ...string) func([]string, slog.Attr) slog.Attr {
	return func(_ []string, a slog.Attr) slog.Attr {
		if slices.Contains(keys, a.Key) {
			return slog.Attr{}
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

func TestHandler(t *testing.T) {
	jsonHandlerFunc := func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewJSONHandler(w, opts)
	}
	textHandlerFunc := func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewTextHandler(w, opts)
	}
	tests := []handlerTest{
		{
			// just ensures that NewHandler doesn't panic
			// should return a handler that writes to stdout
			name: "nil",
			handlerFn: func(_ *bytes.Buffer) slog.Handler {
				return NewHandler(nil, nil)
			},
			want:   "level=INFO msg=hi\n",
			stdout: true,
		},
		{
			name: "nil options",
			want: "level=INFO msg=hi\n",
		},
		{
			name: "default options",
			opts: &HandlerOptions{},
			want: "level=INFO msg=hi\n",
		},
		{
			name: "json",
			opts: &HandlerOptions{
				HandlerFn: jsonHandlerFunc,
			},
			want: `{"level":"INFO","msg":"hi"}` + "\n",
		},
		{
			name: "WithXXX",
			want: "level=INFO msg=hi logger=h1 props.color=red\n",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				return NewHandler(buf, &HandlerOptions{}).Named("h1").
					WithGroup("props").
					WithAttrs([]slog.Attr{slog.String("color", "red")})
			},
		},
		{
			name: "SetHandlerOptions after WithXXX",
			want: `{"level":"INFO","msg":"hi","logger":"h1","props":{"color":"red"}}` + "\n",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				// demonstrates that the handler options can be changed
				// after the handler has been created and used
				h := NewHandler(buf, &HandlerOptions{
					HandlerFn: textHandlerFunc,
				})

				h1 := h.Named("h1").
					WithGroup("props").
					WithAttrs([]slog.Attr{slog.String("color", "red")})
				h1.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0))
				assert.Equal(t, "level=INFO msg=hi logger=h1 props.color=red\n", buf.String())

				buf.Reset()
				h.SetHandlerOptions(&HandlerOptions{
					HandlerFn: jsonHandlerFunc,
				})

				return h1
			},
		},
		{
			name: "ensure cloned slices",
			want: "level=INFO msg=hi logger=h1 props.flavor=lemon props.color=red\n",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				h1 := NewHandler(buf, &HandlerOptions{}).Named("h1").
					WithGroup("props").
					WithAttrs([]slog.Attr{slog.String("flavor", "lemon")})
				h2 := h1.WithAttrs([]slog.Attr{slog.String("color", "red")}) // this appended a group to an internal slice
				h3 := h1.WithAttrs([]slog.Attr{slog.String("size", "big")})  // so did this
				runtime.KeepAlive(h3)
				// need to make sure that h2 and h3 have completely independent states, and one group didn't over the other's group
				// to test this, I need to install a ReplaceAttr function, since that's all the group slice is
				// used for
				return h2
			},
		},
		{
			name: "with attrs",
			want: "level=INFO msg=hi color=red\n",
			recFn: func(rec slog.Record) slog.Record {
				rec.Add(slog.String("color", "red"))
				return rec
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.Run)
	}
}

func TestHandlerOptions_HandlerFn(t *testing.T) {
	// buf := bytes.NewBuffer(nil)
	// redSink := slog.NewTextHandler(buf, nil).WithAttrs([]slog.Attr{slog.String("sink", "red")})
	// blueSink := slog.NewTextHandler(buf, nil).WithAttrs([]slog.Attr{slog.String("sink", "blue")})
	// yellowSink := slog.NewTextHandler(buf, nil).WithAttrs([]slog.Attr{slog.String("sink", "yellow")})
	// defSink := slog.NewTextHandler(buf, nil).WithAttrs([]slog.Attr{slog.String("sink", "def")})
	testCases := []struct {
		desc      string
		hFunc     func(string, io.Writer, *slog.HandlerOptions) slog.Handler
		want      map[string]string
		wantSinks map[string]slog.Handler
	}{
		{
			desc: "unique handlers for each logger",
			hFunc: func(name string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
				if name == "blue" || name == "red" {
					return slog.NewTextHandler(w, opts).WithAttrs([]slog.Attr{slog.String("sink", name)})
				}

				return slog.NewTextHandler(w, opts).WithAttrs([]slog.Attr{slog.String("sink", "def")})
			},
			want: map[string]string{
				"blue":   "sink=blue",
				"red":    "sink=red",
				"yellow": "sink=def",
			},
		},
		{
			desc: "nil handler defaults to discard handler",
			hFunc: func(name string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
				if name == "blue" {
					return nil
				}

				return slog.NewTextHandler(w, opts).WithAttrs([]slog.Attr{slog.String("sink", "def")})
			},
			want: map[string]string{
				"blue":   "",
				"yellow": "sink=def",
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			buf := bytes.NewBuffer(nil)
			h := NewHandler(buf, &HandlerOptions{
				HandlerFn: tC.hFunc,
			})
			// ctl := tC.ctlFn()
			for logger, want := range tC.want {
				buf.Reset()
				slog.New(h).With(LoggerKey, logger).Info("hi")
				assert.Contains(t, buf.String(), want, "logger: %s", logger)
			}
		})
	}
}

func TestHandlerOptions_Levels(t *testing.T) {
	levels := []slog.Level{
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
	}

	for _, level := range levels {
		opts := &HandlerOptions{
			Level:  level,
			Levels: map[string]slog.Leveler{},
		}
		for _, h1Level := range levels {
			opts.Levels["h1"] = h1Level
			for _, h2Level := range levels {
				opts.Levels["h2"] = h2Level
				t.Run(fmt.Sprintf("level=%s h1=%s h2=%s opts in constructor", level, h1Level, h2Level), func(t *testing.T) {
					h := NewHandler(io.Discard, opts)
					assert.True(t, h.Enabled(context.Background(), level))
					assert.False(t, h.Enabled(context.Background(), level-1))
					h1 := h.WithAttrs([]slog.Attr{slog.String(LoggerKey, "h1")})
					assert.True(t, h1.Enabled(context.Background(), h1Level))
					assert.False(t, h1.Enabled(context.Background(), h1Level-1))
					h2 := h1.WithAttrs([]slog.Attr{slog.String(LoggerKey, "h2")})
					assert.True(t, h2.Enabled(context.Background(), h2Level))
					assert.False(t, h2.Enabled(context.Background(), h2Level-1))
				})
				t.Run(fmt.Sprintf("level=%s h1=%s h2=%s opts set later", level, h1Level, h2Level), func(t *testing.T) {
					h := NewHandler(io.Discard, &HandlerOptions{})
					h1 := h.WithAttrs([]slog.Attr{slog.String(LoggerKey, "h1")})
					h2 := h1.WithAttrs([]slog.Attr{slog.String(LoggerKey, "h2")})

					h.SetHandlerOptions(opts)
					assert.True(t, h.Enabled(context.Background(), level))
					assert.False(t, h.Enabled(context.Background(), level-1))
					assert.True(t, h1.Enabled(context.Background(), h1Level))
					assert.False(t, h1.Enabled(context.Background(), h1Level-1))
					assert.True(t, h2.Enabled(context.Background(), h2Level))
					assert.False(t, h2.Enabled(context.Background(), h2Level-1))
				})
			}
		}
	}
}

func TestHandlerOptions_AddSource(t *testing.T) {
	pc, file, line, _ := runtime.Caller(0)
	sourceField := fmt.Sprintf("%s:%d", file, line)

	buf := bytes.NewBuffer(nil)
	h := NewHandler(buf, &HandlerOptions{
		AddSource: true,
	})
	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", pc)

	h.Handle(context.Background(), rec.Clone())
	assert.Equal(t, `level=INFO source=`+sourceField+" msg=hi\n", buf.String())

	h.SetHandlerOptions(&HandlerOptions{
		AddSource: false,
	})
	buf.Reset()
	h.Handle(context.Background(), rec.Clone())
	assert.Equal(t, "level=INFO msg=hi\n", buf.String())
}

func TestHandlerOptions_ReplaceAttrs(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	h := NewHandler(buf, &HandlerOptions{
		ReplaceAttrs: []func(groups []string, a slog.Attr) slog.Attr{
			replaceKey("color", slog.String("color", "red")),
		},
	})
	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0)
	rec.Add(slog.String("color", "blue"))

	h.Handle(context.Background(), rec.Clone())
	assert.Equal(t, "level=INFO msg=hi color=red\n", buf.String())

	// test that HandlerOptions can be applied later
	h.SetHandlerOptions(&HandlerOptions{
		ReplaceAttrs: []func(groups []string, a slog.Attr) slog.Attr{
			replaceKey("color", slog.String("color", "green")),
		},
	})
	buf.Reset()
	h.Handle(context.Background(), rec.Clone())
	assert.Equal(t, "level=INFO msg=hi color=green\n", buf.String())

	// test that multiple ReplaceAttrs functions are applied in order
	h.SetHandlerOptions(&HandlerOptions{
		ReplaceAttrs: []func(groups []string, a slog.Attr) slog.Attr{
			replaceKey("color", slog.String("color", "yellow")),
			replaceKey("size", slog.String("size", "small")),
		},
	})
	buf.Reset()

	rec = rec.Clone()
	rec.Add(slog.String("size", "big"))
	h.Handle(context.Background(), rec)
	assert.Equal(t, "level=INFO msg=hi color=yellow size=small\n", buf.String())
}

func TestHandlerOptions_Middleware(t *testing.T) {
	addAttrMW := func(attr slog.Attr) Middleware {
		return SimpleMiddlewareFn(func(ctx context.Context, record slog.Record, next slog.Handler) error {
			record.AddAttrs(attr)
			return next.Handle(ctx, record)
		})
	}

	buf := bytes.NewBuffer(nil)
	h := NewHandler(buf, &HandlerOptions{
		Middleware: []Middleware{addAttrMW(slog.String("color", "red"))},
	})
	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0)
	h.Handle(context.Background(), rec)
	assert.Equal(t, "level=INFO msg=hi color=red\n", buf.String())
}
func TestHandlerOptions_Clone(t *testing.T) {
	testCases := []struct {
		name string
		opts *HandlerOptions
	}{
		{
			name: "non-nil",
			opts: &HandlerOptions{
				Level: slog.LevelInfo,
				Levels: map[string]slog.Leveler{
					"foo": slog.LevelWarn,
				},
				AddSource: true,
				ReplaceAttrs: []func(groups []string, a slog.Attr) slog.Attr{
					func(_ []string, a slog.Attr) slog.Attr {
						return a
					},
				},
				Middleware: []Middleware{
					SimpleMiddlewareFn(func(ctx context.Context, record slog.Record, next slog.Handler) error {
						return next.Handle(ctx, record)
					}),
				},
				HandlerFn: func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
					return slog.NewTextHandler(w, opts)
				},
			},
		},
		{
			name: "nil",
			opts: nil,
		},
	}

	for _, tC := range testCases {
		t.Run(tC.name, func(t *testing.T) {
			clone := tC.opts.Clone()
			if tC.opts == nil {
				assert.Nil(t, clone)
				return
			}

			assertHandlerOptionsEqual(t, *clone, *tC.opts, "")

			// modify original and verify clone is not modified
			tC.opts.Level = slog.LevelDebug
			tC.opts.Levels["bar"] = slog.LevelError
			tC.opts.AddSource = false
			tC.opts.ReplaceAttrs = append(tC.opts.ReplaceAttrs, func(_ []string, _ slog.Attr) slog.Attr {
				return slog.Attr{}
			})
			tC.opts.Middleware = append(tC.opts.Middleware, SimpleMiddlewareFn(func(_ context.Context, _ slog.Record, _ slog.Handler) error {
				return nil
			}))
			tC.opts.HandlerFn = nil

			assert.Equal(t, slog.LevelInfo, clone.Level)
			assert.Equal(t, Levels{
				"foo": slog.LevelWarn,
			}, clone.Levels)
			assert.True(t, clone.AddSource)
			assert.Len(t, clone.ReplaceAttrs, 1)
			assert.Len(t, clone.Middleware, 1)
			assert.NotNil(t, clone.HandlerFn)
		})
	}
}

func TestNoopHandler(t *testing.T) {
	// noop should always return false for Enabled
	assert.False(t, noop.Enabled(context.Background(), LevelAll))

	// noop should return itself for WithAttrs and WithGroup
	assert.Equal(t, noop, noop.WithAttrs([]slog.Attr{slog.String("foo", "bar")}))
	assert.Equal(t, noop, noop.WithGroup("group"))

	// noop should do nothing for Handle
	assert.NoError(t, noop.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0)))
}

type handlerTest struct {
	name string
	want string
	// defaults to slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0)
	recFn     func(rec slog.Record) slog.Record
	handlerFn func(buf *bytes.Buffer) slog.Handler
	opts      *HandlerOptions
	stdout    bool
}

func (ht handlerTest) Run(t *testing.T) {
	t.Helper()

	buf := bytes.NewBuffer(nil)

	var r, w *os.File

	if ht.stdout {
		// special case: handler will write to stdout
		oldStdout := os.Stdout

		var err error

		r, w, err = os.Pipe()
		require.NoError(t, err)

		os.Stdout = w

		defer func() {
			os.Stdout = oldStdout
		}()
	}

	var h slog.Handler

	hFun := ht.handlerFn
	if hFun == nil {
		h = NewHandler(buf, ht.opts)
	} else {
		h = hFun(buf)
	}

	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0)
	if ht.recFn != nil {
		rec = ht.recFn(rec)
	}

	err := h.Handle(context.Background(), rec)
	require.NoError(t, err)

	if ht.stdout {
		w.Close()
		io.Copy(buf, r)
	}

	assert.Equal(t, ht.want, buf.String())
}

func TestHandler_Out(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	h := NewHandler(buf, nil)
	assert.Equal(t, buf, h.Out())

	buf2 := bytes.NewBuffer(nil)
	h.SetOut(buf2)
	assert.Equal(t, buf2, h.Out())
}
