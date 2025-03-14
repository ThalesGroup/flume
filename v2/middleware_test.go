package flume

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReplaceAttrs(t *testing.T) {
	tests := []handlerTest{
		{
			name: "basic",
			opts: &HandlerOptions{
				Middleware: []Middleware{
					ReplaceAttrs(removeKeys("color")),
				},
			},
			recFn: func(rec slog.Record) slog.Record {
				rec.Add("color", "red", "size", "big")
				return rec
			},
			want: "level=INFO msg=hi size=big\n",
		},
		{
			name: "nil function",
			opts: &HandlerOptions{
				Middleware: []Middleware{
					ReplaceAttrs(nil),
				},
			},
			recFn: func(rec slog.Record) slog.Record {
				rec.Add("color", "red", "size", "big")
				return rec
			},
			want: "level=INFO msg=hi color=red size=big\n",
		},
		{
			name: "withattrs",
			want: "level=INFO msg=hi size=big\n",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				h := NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{
						ReplaceAttrs(removeKeys("color")),
					},
				})

				return h.WithAttrs([]slog.Attr{slog.String("color", "red"), slog.String("size", "big")})
			},
		},
		{
			name: "nested groups",
			want: "level=INFO msg=hi props.size=big\n",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				h := NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{
						ReplaceAttrs(removeKeys("color")),
					},
				})

				return h.WithGroup("props").WithAttrs([]slog.Attr{slog.String("color", "red"), slog.String("size", "big")})
			},
		},
		{
			name: "msg",
			want: "level=INFO msg=bye\n",
			opts: &HandlerOptions{
				Middleware: []Middleware{
					ReplaceAttrs(replaceKey(slog.MessageKey, slog.String("doesn'tmatter", "bye"))),
				},
			},
		},

		{
			name: "delete msg",
			want: "level=INFO msg=<nil>\n",
			opts: &HandlerOptions{
				Middleware: []Middleware{
					ReplaceAttrs(removeKeys(slog.MessageKey)),
				},
			},
		},
		{
			name: "replace msg",
			want: "level=INFO msg=5\n",
			opts: &HandlerOptions{
				Middleware: []Middleware{
					ReplaceAttrs(replaceKey(slog.MessageKey, slog.Int("size", 5))),
				},
			},
		},
		{
			name: "time",
			want: "time=2020-10-23T03:04:05.000Z level=INFO msg=hi\n",
			opts: &HandlerOptions{
				Middleware: []Middleware{
					ReplaceAttrs(replaceKey(slog.TimeKey, slog.Time(slog.TimeKey, time.Date(2020, 10, 23, 3, 4, 5, 0, time.UTC)))),
				},
			},
		},
		{
			name: "delete time",
			want: "level=INFO msg=hi\n",
			recFn: func(rec slog.Record) slog.Record {
				rec.Time = time.Now()
				return rec
			},
			opts: &HandlerOptions{
				Middleware: []Middleware{
					ReplaceAttrs(removeKeys(slog.TimeKey)),
				},
			},
		},

		{
			name: "multiple ReplaceAttr funcs",
			want: "time=2021-10-23T03:04:05.000Z level=INFO msg=hi logname=frank\n",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				h := NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{
						ReplaceAttrs(
							replaceKey(slog.TimeKey, slog.Time(slog.TimeKey, time.Date(2020, 10, 23, 3, 4, 5, 0, time.UTC))),
							replaceKey(slog.TimeKey, slog.Time(slog.TimeKey, time.Date(2021, 10, 23, 3, 4, 5, 0, time.UTC))),
							replaceKey(LoggerKey, slog.String("logname", "frank")),
						),
					},
				})

				return h.WithAttrs([]slog.Attr{slog.String(LoggerKey, "h1")})
			},
		},
		{
			name: "replace time with not a time",
			want: "time=2020-10-23T03:04:05.000Z level=INFO msg=hi\n",
			recFn: func(rec slog.Record) slog.Record {
				// in order to test this, I first need to use a fixed time which
				// I can assert against, then replace it *again* with something else, which should
				// be ignored because it's not a valid Time
				rec.Time = time.Date(2020, 10, 23, 3, 4, 5, 0, time.UTC)
				return rec
			},
			opts: &HandlerOptions{
				Middleware: []Middleware{
					ReplaceAttrs(
						replaceKey(slog.TimeKey, slog.String("size", "big")),
					),
				},
			},
		},
		{
			name: "level",
			want: "level=ERROR msg=hi\n",
			opts: &HandlerOptions{
				Middleware: []Middleware{
					ReplaceAttrs(replaceKey(slog.LevelKey, slog.Any(slog.LevelKey, slog.LevelError))),
				},
			},
		},
		{
			name: "delete level",
			want: "level=INFO msg=hi\n",
			recFn: func(rec slog.Record) slog.Record {
				rec.Level = slog.LevelError
				return rec
			},
			opts: &HandlerOptions{
				Middleware: []Middleware{
					ReplaceAttrs(replaceKey(slog.LevelKey, slog.Attr{})),
				},
			},
		},
		{
			name: "replace level with not a level",
			want: "level=ERROR msg=hi\n",
			recFn: func(rec slog.Record) slog.Record {
				rec.Level = slog.LevelError
				return rec
			},
			opts: &HandlerOptions{
				Middleware: []Middleware{
					ReplaceAttrs(replaceKey(slog.LevelKey, slog.String("color", "red"))),
				},
			},
		},
		{
			name: "WithAttrs",
			want: "level=INFO msg=hi size=big\n",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				return NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{
						ReplaceAttrs(removeKeys("color")),
					},
				}).WithAttrs([]slog.Attr{slog.String("color", "red"), slog.String("size", "big")})
			},
		},
		{
			name: "WithAttrs with all the attrs removed",
			want: "level=INFO msg=hi\n",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				return NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{
						ReplaceAttrs(removeKeys("color")),
					},
				}).WithAttrs([]slog.Attr{slog.String("color", "red")})
			},
		},
		{
			name: "apply ReplaceAttrs recursively to groups",
			want: "level=INFO msg=hi colors.color=blue\n",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				return NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{
						ReplaceAttrs(replaceKey("color", slog.String("color", "blue"))),
					},
				}).WithAttrs([]slog.Attr{slog.Group("colors", slog.String("color", "red"))})
			},
		},
		{
			name: "elide empty groups",
			want: "level=INFO msg=hi\n",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				return NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{
						ReplaceAttrs(removeKeys("color")),
					},
				}).WithAttrs([]slog.Attr{slog.Group("colors", slog.String("color", "red"))})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.Run)
	}
}

func TestReplaceAttrs_SkipRecord(t *testing.T) {
	t.Run("check arg passed to SkipRecord", func(t *testing.T) {
		ht := handlerTest{
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				mw := ReplaceAttrs()
				mw.SkipRecord = func(r slog.Record) bool {
					rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0)
					rec.Add("color", "red")
					assert.Equal(t, rec, r)
					return true
				}
				return NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{mw},
				})
			},
			recFn: func(rec slog.Record) slog.Record {
				rec.Add("color", "red")
				return rec
			},
			want: "level=INFO msg=hi color=red\n",
		}
		ht.Run(t)
	})

	tests := []handlerTest{
		{
			name: "SkipRecord=true",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				mw := ReplaceAttrs(removeKeys("color"))
				mw.SkipRecord = func(_ slog.Record) bool {
					return true
				}
				return NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{mw},
				})
			},
			recFn: func(rec slog.Record) slog.Record {
				rec.Add("color", "red")
				return rec
			},
			want: "level=INFO msg=hi color=red\n",
		},
		{
			name: "SkipRecord=false",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				mw := ReplaceAttrs(removeKeys("color"))
				mw.SkipRecord = func(_ slog.Record) bool {
					return false
				}
				return NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{mw},
				})
			},
			recFn: func(rec slog.Record) slog.Record {
				rec.Add("color", "red")
				return rec
			},
			want: "level=INFO msg=hi\n",
		},
		{
			name: "SkipRecord does not affect WithAttrs",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				mw := ReplaceAttrs(removeKeys("color"))
				mw.SkipRecord = func(_ slog.Record) bool {
					return true
				}
				return NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{mw},
				}).WithAttrs([]slog.Attr{slog.String("color", "red"), slog.String("size", "big")})
			},
			want: "level=INFO msg=hi size=big\n", // processing should have been skipped
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.Run)
	}
}

func TestReplaceAttrs_SkipBuiltins(t *testing.T) {
	tests := []handlerTest{
		{
			name: "SkipBuiltins=true",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				mw := ReplaceAttrs(replaceKey(slog.MessageKey, slog.String(slog.MessageKey, "bye")))
				mw.SkipBuiltins = true
				return NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{mw},
				})
			},
			want: "level=INFO msg=hi\n",
		},
		{
			name: "SkipBuiltins=false",
			handlerFn: func(buf *bytes.Buffer) slog.Handler {
				mw := ReplaceAttrs(replaceKey(slog.MessageKey, slog.String(slog.MessageKey, "bye")))
				mw.SkipBuiltins = false
				return NewHandler(buf, &HandlerOptions{
					Middleware: []Middleware{mw},
				})
			},
			want: "level=INFO msg=bye\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.Run)
	}
}

func TestReplaceAttrs_Enabled(t *testing.T) {
	// This test just ensures that the ReplaceAttrs middleware doesn't break the
	// Enabled method.
	buf := bytes.NewBuffer(nil)
	var handler slog.Handler = slog.NewTextHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	handler = ReplaceAttrs(removeKeys(slog.TimeKey)).Apply(handler)

	assert.True(t, handler.Enabled(context.Background(), slog.LevelDebug))
}

func TestHandlerMiddlewareFunc(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	innerHandler := slog.NewTextHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	middleware := SimpleMiddlewareFn(func(ctx context.Context, record slog.Record, next slog.Handler) error {
		record.AddAttrs(slog.String("foo", "bar"))
		return next.Handle(ctx, record)
	})

	assert.Implements(t, (*Middleware)(nil), middleware)

	outerHandler := middleware.Apply(innerHandler)
	outerHandler.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelDebug, "hi", 0))
	assert.Equal(t, "level=DEBUG msg=hi foo=bar\n", buf.String())

	// make sure it handles WithGroup and WithAttrs correctly
	buf.Reset()
	outerHandler.WithGroup("props").WithAttrs([]slog.Attr{slog.String("bar", "baz")}).Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelDebug, "hi", 0))
	assert.Equal(t, "level=DEBUG msg=hi props.bar=baz props.foo=bar\n", buf.String())

	// make sure Enabled passes through to next handler
	assert.True(t, outerHandler.Enabled(context.Background(), slog.LevelDebug))
}
