package flume

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestAbbreviateLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    slog.Attr
		expected slog.Attr
	}{
		{
			name:     "info level",
			input:    slog.Any(slog.LevelKey, slog.LevelInfo),
			expected: slog.String(slog.LevelKey, "INF"),
		},
		{
			name:     "error level",
			input:    slog.Any(slog.LevelKey, slog.LevelError),
			expected: slog.String(slog.LevelKey, "ERR"),
		},
		{
			name:     "debug level",
			input:    slog.Any(slog.LevelKey, slog.LevelDebug),
			expected: slog.String(slog.LevelKey, "DBG"),
		},
		{
			name:     "warn level",
			input:    slog.Any(slog.LevelKey, slog.LevelWarn),
			expected: slog.String(slog.LevelKey, "WRN"),
		},
		{
			name:     "non-level attr",
			input:    slog.Any("foo", "bar"),
			expected: slog.Any("foo", "bar"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AbbreviateLevel(nil, tt.input)
			assert.Equal(t, tt.expected.String(), result.String())
			assert.True(t, tt.expected.Equal(result))
		})
	}
}

func TestFormatTimes(t *testing.T) {
	now := time.Date(2023, 12, 25, 13, 14, 15, 123456789, time.UTC)
	tests := []struct {
		name     string
		format   string
		input    slog.Attr
		expected slog.Attr
	}{
		{
			name:     "formats time with custom format",
			format:   "2006-01-02 15:04:05",
			input:    slog.Time("timestamp", now),
			expected: slog.String("timestamp", "2023-12-25 13:14:15"),
		},
		{
			name:     "formats time with kitchen format",
			format:   time.Kitchen,
			input:    slog.Time("timestamp", now),
			expected: slog.String("timestamp", "1:14PM"),
		},
		{
			name:     "non-time attr is unchanged",
			format:   time.RFC3339,
			input:    slog.String("foo", "bar"),
			expected: slog.String("foo", "bar"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := FormatTimes(tt.format)
			result := formatter(nil, tt.input)
			assert.Equal(t, tt.expected.String(), result.String())
			assert.True(t, tt.expected.Equal(result))
		})
	}
}

func TestSimpleTime(t *testing.T) {
	now := time.Date(2023, 12, 25, 13, 14, 15, 123456789, time.UTC)
	tests := []struct {
		name     string
		input    slog.Attr
		expected slog.Attr
	}{
		{
			name:     "formats time with simple format",
			input:    slog.Time("timestamp", now),
			expected: slog.String("timestamp", "13:14:15.123"),
		},
		{
			name:     "non-time attr is unchanged",
			input:    slog.String("foo", "bar"),
			expected: slog.String("foo", "bar"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := SimpleTime()
			result := formatter(nil, tt.input)
			assert.Equal(t, tt.expected.String(), result.String())
			assert.True(t, tt.expected.Equal(result))
		})
	}
}

func TestChainReplaceAttrs(t *testing.T) {
	tests := []struct {
		name             string
		wantText         string
		args             []any
		replaceAttrFuncs []func(groups []string, a slog.Attr) slog.Attr
	}{
		{
			name:     "nil function",
			wantText: "level=INFO msg=hi logger=blue color=red\n",
			args:     []any{"color", "red"},
			replaceAttrFuncs: []func(groups []string, a slog.Attr) slog.Attr{
				nil,
			},
		},
		{
			name:     "single function",
			wantText: "level=INFO msg=hi logger=blue color=blue\n",
			args:     []any{"color", "red"},
			replaceAttrFuncs: []func(groups []string, a slog.Attr) slog.Attr{
				replaceKey("color", slog.String("color", "blue")),
			},
		},
		{
			name:     "multiple functions",
			wantText: "level=INFO msg=hi logger=blue color=blue size=small\n",
			args:     []any{"color", "red", "size", "big"},
			replaceAttrFuncs: []func(groups []string, a slog.Attr) slog.Attr{
				nil,
				replaceKey("color", slog.String("color", "blue")),
				nil,
				replaceKey("size", slog.String("size", "small")),
				nil,
			},
		},
		{
			name:     "stop processing after first empty attr",
			wantText: "level=INFO msg=hi logger=blue\n",
			args:     []any{"color", "red"},
			replaceAttrFuncs: []func(groups []string, a slog.Attr) slog.Attr{
				removeKeys("color"),
				func(_ []string, a slog.Attr) slog.Attr {
					if a.Key == "color" {
						panic("should not have been called")
					}
					return a
				},
			},
		},
		{
			name:     "stop processing if value is a group",
			wantText: "level=INFO msg=hi logger=blue color.red=blue\n",
			args:     []any{"color", "red"},
			replaceAttrFuncs: []func(groups []string, a slog.Attr) slog.Attr{
				replaceKey("color", slog.Group("color", slog.String("red", "blue"))),
				func(_ []string, a slog.Attr) slog.Attr {
					if a.Key == "color" {
						panic("should not have been called")
					}
					return a
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := bytes.NewBuffer(nil)
			replaceAttrFuncs := test.replaceAttrFuncs
			replaceAttrFuncs = append(replaceAttrFuncs, removeKeys(slog.TimeKey))

			h := slog.NewTextHandler(buf, &slog.HandlerOptions{ReplaceAttr: ChainReplaceAttrs(replaceAttrFuncs...)})
			rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0)
			rec.Add(slog.String("logger", "blue"))
			rec.Add(test.args...)
			err := h.Handle(context.Background(), rec)
			require.NoError(t, err)
			assert.Equal(t, test.wantText, buf.String())
		})
	}
}

type detailError struct {
	msg    string
	detail string
}

func (d *detailError) Error() string {
	return d.msg
}

func (d *detailError) Format(s fmt.State, _ rune) {
	_, _ = fmt.Fprint(s, d.msg)
	if s.Flag('+') {
		_, _ = fmt.Fprint(s, d.detail)
	}
}

func TestDetailedErrors(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	opts := &HandlerOptions{
		HandlerFn: func(_ string, out io.Writer, opts *slog.HandlerOptions) slog.Handler {
			return slog.NewJSONHandler(out, opts)
		},
		ReplaceAttrs: []func([]string, slog.Attr) slog.Attr{
			DetailedErrors,
		},
	}
	h := NewHandler(buf, opts)

	boom := &detailError{msg: "boom", detail: "it exploded"}
	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "an error", 0)
	rec.Add("error", boom)
	err := h.Handle(context.Background(), rec)
	require.NoError(t, err)

	assert.JSONEq(t, `{"level":"INFO","msg":"an error","error":"boomit exploded"}`, buf.String())

	// make sure text renders the same way
	buf.Reset()
	opts.HandlerFn = func(_ string, out io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewTextHandler(out, opts)
	}
	h.SetHandlerOptions(opts)

	rec = slog.NewRecord(time.Time{}, slog.LevelInfo, "an error", 0)
	rec.Add("error", boom)
	err = h.Handle(context.Background(), rec)
	require.NoError(t, err)
	assert.Equal(t, "level=INFO msg=\"an error\" error=\"boomit exploded\"\n", buf.String())
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

	middleware := HandlerMiddlewareFunc(func(ctx context.Context, record slog.Record, next slog.Handler) error {
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
