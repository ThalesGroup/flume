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

func TestTimeFormats(t *testing.T) {
	now := time.Date(2023, 12, 25, 13, 14, 15, 123456789, time.FixedZone("test", 1*60*60))
	tests := []struct {
		name      string
		formatter func([]string, slog.Attr) slog.Attr
		want      string
	}{
		// this is the format slog's default json handlers uses
		{"slog json default", FormatTimes(time.RFC3339Nano), "2023-12-25T13:14:15.123456789+01:00"},
		// this is the format slog's default text handler uses
		{"slog text default", RFC3339MillisTime(), "2023-12-25T13:14:15.123+01:00"},
		{"ISO8601Time", ISO8601Time(), "2023-12-25T13:14:15.123+0100"},
		{"SimpleTime", SimpleTime(), "13:14:15.123"},
		{"FormatTimes:Kitchen", FormatTimes(time.Kitchen), "1:14PM"},
		{"FormatTimes:custom", FormatTimes("2006-01-02 15:04:05"), "2023-12-25 13:14:15"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.formatter(nil, slog.Time("timestamp", now))
			assert.Equal(t, slog.KindString, result.Value.Kind(), "kind should have changed to string")
			assert.Equal(t, tt.want, result.Value.String())
			assert.Equal(t, "timestamp", result.Key, "key name should not have changed")

			result = tt.formatter(nil, slog.Duration("foo", time.Second))
			assert.Equal(t, slog.KindDuration, result.Value.Kind(), "kind for non-timestamp attr should not have changed")
			assert.Equal(t, time.Second, result.Value.Duration(), "value for non-timestamp attr should not have changed")
			assert.Equal(t, "foo", result.Key, "key for non-timestamp attr should not have changed")
		})
	}
}

func TestSecondsDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		// this is the format slog's default json handlers uses
		{time.Second, "1"},
		{time.Second + time.Millisecond, "1.001"},
	}

	for _, tt := range tests {
		t.Run(tt.in.String(), func(t *testing.T) {
			result := SecondsDuration()(nil, slog.Duration("duration", tt.in))
			assert.Equal(t, slog.KindFloat64, result.Value.Kind(), "kind should have changed to float64")
			assert.Equal(t, tt.want, result.Value.String())
			assert.Equal(t, "duration", result.Key, "key name should not have changed")
		})
	}

	t.Run("ignore non-duration values", func(t *testing.T) {
		result := SecondsDuration()(nil, slog.String("foo", "bar"))
		assert.Equal(t, slog.KindString, result.Value.Kind(), "kind should not have changed")
		assert.Equal(t, "bar", result.Value.String(), "value should not have changed")
		assert.Equal(t, "foo", result.Key, "key should not have changed")
	})
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
