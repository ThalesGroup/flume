package flume

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/ansel1/merry/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

			c := NewController(slog.NewTextHandler(buf, &slog.HandlerOptions{ReplaceAttr: ChainReplaceAttrs(replaceAttrFuncs...)}))

			l := c.Logger("blue")
			l.Info("hi", test.args...)
			assert.Equal(t, test.wantText, buf.String())
		})
	}
}

func TestDetailedErrors(t *testing.T) {
	buf := bytes.NewBuffer(nil)

	ctl := NewController(
		slog.NewJSONHandler(
			buf,
			&slog.HandlerOptions{
				ReplaceAttr: ChainReplaceAttrs(removeKeys(slog.TimeKey), DetailedErrors),
			},
		))

	l := ctl.Logger("main")
	err := merry.New("boom")

	l.Info("an error", "error", err)

	marshaledErr, merr := json.Marshal(fmt.Sprintf("%+v", err))
	require.NoError(t, merr)
	assert.JSONEq(t, fmt.Sprintf(`{"level":"INFO","logger":"main","msg":"an error","error":%v}`, string(marshaledErr)), buf.String())
}

func TestReplaceAttrs(t *testing.T) {
	tests := []handlerTest{
		{
			name: "basic",
			want: "level=INFO msg=hi logger=h1 size=big\n",
			args: []any{"color", "red", "size", "big"},
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(removeKeys("color")))

				return ctl.Handler("h1")
			},
		},
		{
			name: "nil function",
			want: "level=INFO msg=hi logger=h1 color=red size=big\n",
			args: []any{"color", "red", "size", "big"},
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(nil))

				return ctl.Handler("h1")
			},
		},
		{
			name: "withattrs",
			want: "level=INFO msg=hi logger=h1 size=big\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(removeKeys("color")))

				return ctl.Handler("h1").WithAttrs([]slog.Attr{slog.String("color", "red"), slog.String("size", "big")})
			},
		},
		{
			name: "nested groups",
			want: "level=INFO msg=hi logger=h1 props.size=big\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(removeKeys("color")))

				return ctl.Handler("h1").WithGroup("props").WithAttrs([]slog.Attr{slog.String("color", "red"), slog.String("size", "big")})
			},
		},
		{
			name: "msg",
			want: "level=INFO msg=bye logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(replaceKey(slog.MessageKey, slog.String("doesn'tmatter", "bye"))))

				return ctl.Handler("h1")
			},
		},
		{
			name: "delete msg",
			want: "level=INFO msg=<nil> logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(removeKeys(slog.MessageKey)))

				return ctl.Handler("h1")
			},
		},
		{
			name: "replace msg",
			want: "level=INFO msg=5 logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(replaceKey(slog.MessageKey, slog.Int("size", 5))))

				return ctl.Handler("h1")
			},
		},
		{
			name: "time",
			want: "time=2020-10-23T03:04:05.000Z level=INFO msg=hi logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, _ *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, nil))

				t1 := time.Date(2020, 10, 23, 3, 4, 5, 0, time.UTC)

				ctl.UseDefault(ReplaceAttrs(replaceKey(slog.TimeKey, slog.Time(slog.TimeKey, t1))))

				return ctl.Handler("h1")
			},
		},
		{
			name: "delete time",
			want: "level=INFO msg=hi logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, nil))

				ctl.UseDefault(ReplaceAttrs(opts.ReplaceAttr))

				return ctl.Handler("h1")
			},
		},
		{
			name: "multiple ReplaceAttr funcs",
			want: "time=2021-10-23T03:04:05.000Z level=INFO msg=hi logname=frank\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, _ *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, nil))

				t1 := time.Date(2020, 10, 23, 3, 4, 5, 0, time.UTC)
				t2 := time.Date(2021, 10, 23, 3, 4, 5, 0, time.UTC)

				ctl.UseDefault(ReplaceAttrs(
					replaceKey(slog.TimeKey, slog.Time(slog.TimeKey, t1)),
					replaceKey(slog.TimeKey, slog.Time(slog.TimeKey, t2)),
					replaceKey(LoggerKey, slog.String("logname", "frank")),
				))

				return ctl.Handler("h1")
			},
		},
		{
			name: "replace time with not a time",
			want: "time=2020-10-23T03:04:05.000Z level=INFO msg=hi logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, _ *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, nil))

				// in order to test this, I first need to replace the time with a fixed value which
				// I can assert against, then replace it *again* with something else, which should
				// be ignored because it's not a valid Time
				t1 := time.Date(2020, 10, 23, 3, 4, 5, 0, time.UTC)

				ctl.UseDefault(ReplaceAttrs(replaceKey(slog.TimeKey, slog.Time(slog.TimeKey, t1))))

				ctl.UseDefault(ReplaceAttrs(replaceKey(slog.TimeKey, slog.String("size", "big"))))

				return ctl.Handler("h1")
			},
		},
		{
			name: "level",
			want: "level=ERROR msg=hi logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(replaceKey(slog.LevelKey, slog.Any(slog.LevelKey, slog.LevelError))))

				return ctl.Handler("h1")
			},
		},
		{
			name:  "delete level",
			want:  "level=INFO msg=hi logger=h1\n",
			level: slog.LevelError,
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(replaceKey(slog.LevelKey, slog.Attr{})))

				return ctl.Handler("h1")
			},
		},
		{
			name:  "replace level with not a level",
			want:  "level=ERROR msg=hi logger=h1\n",
			level: slog.LevelError,
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(replaceKey(slog.LevelKey, slog.String("color", "red"))))

				return ctl.Handler("h1")
			},
		},
		{
			name: "WithAttrs",
			want: "level=INFO msg=hi logger=h1 size=big\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(removeKeys("color")))

				return ctl.Handler("h1").WithAttrs([]slog.Attr{slog.String("color", "red"), slog.String("size", "big")})
			},
		},
		{
			name: "WithAttrs with all the attrs removed",
			want: "level=INFO msg=hi logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(removeKeys("color")))

				return ctl.Handler("h1").WithAttrs([]slog.Attr{slog.String("color", "red")})
			},
		},
		{
			name: "apply ReplaceAttrs recursively to groups",
			want: "level=INFO msg=hi logger=h1 colors.color=blue\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(replaceKey("color", slog.String("color", "blue"))))

				return ctl.Handler("h1").WithAttrs([]slog.Attr{slog.Group("colors", slog.String("color", "red"))})
			},
		},
		{
			name: "elide empty groups",
			want: "level=INFO msg=hi logger=h1\n",
			handlerFn: func(_ *testing.T, buf *bytes.Buffer, opts *slog.HandlerOptions) slog.Handler {
				ctl := NewController(slog.NewTextHandler(buf, opts))

				ctl.UseDefault(ReplaceAttrs(
					removeKeys("color"),
				))

				return ctl.Handler("h1").WithAttrs([]slog.Attr{slog.Group("colors", slog.String("color", "red"))})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.Run)
	}
}

func TestReplaceAttrs_Enabled(t *testing.T) {
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
		ReplaceAttr: removeKeys(slog.TimeKey),
		Level:       slog.LevelDebug,
	})

	middleware := HandlerMiddlewareFunc(func(ctx context.Context, record slog.Record, next slog.Handler) error {
		record.AddAttrs(slog.String("foo", "bar"))
		return next.Handle(ctx, record)
	})

	outerHandler := middleware.Apply(innerHandler)
	outerHandler.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelDebug, "hi", 0))
	assert.Equal(t, "level=DEBUG msg=hi foo=bar\n", buf.String())

	// make sure it handles WithGroup and WithAttrs correctly
	buf.Reset()
	outerHandler.WithGroup("props").WithAttrs([]slog.Attr{slog.String("bar", "baz")}).Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelDebug, "hi", 0))
	assert.Equal(t, "level=DEBUG msg=hi props.bar=baz props.foo=bar\n", buf.String())

	// make sure Enabled passes through to next handler
	assert.True(t, outerHandler.Enabled(context.Background(), slog.LevelDebug))
}
