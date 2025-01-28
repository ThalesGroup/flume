package flume

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/ansel1/merry/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func appendString(key, value string) func(groups []string, a slog.Attr) slog.Attr {
	return func(_ []string, a slog.Attr) slog.Attr {
		if a.Value.Kind() == slog.KindString && a.Key == key {
			a.Value = slog.StringValue(a.Value.String() + value)
		}

		return a
	}
}

// func createGroup(key, named string) func(groups []string, a slog.Attr) slog.Attr {
// 	return func(groups []string, a slog.Attr) slog.Attr {
// 		if a.Key == key {
// 			a.Value = slog.GroupValue(a)
// 			a.Value = slog.StringValue(a.Value.String() + value)
// 		}
// 		return a
// 	}
// }

// func TestBareAttr(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		wantText string
// 		args     []any
// 	}{
// 		{
// 			name:     "bare value",
// 			wantText: "level=INFO msg=hi logger=blue value=chocolate\n",
// 			args:     []any{"chocolate"},
// 		},
// 		{
// 			name:     "bare error",
// 			wantText: "level=INFO msg=hi logger=blue error=boom\n",
// 			args:     []any{errors.New("boom")},
// 		},
// 		{
// 			name:     "only works when len(args)==1",
// 			wantText: "level=INFO msg=hi logger=blue size=5 !BADKEY=chocolate\n",
// 			args:     []any{"size", 5, "chocolate"},
// 		},
// 	}
//
// 	for test := range tests {
// 		t.Run(tests[test].name, func(t *testing.T) {
// 			buf := bytes.NewBuffer(nil)
//
// 			ctl := NewController(nil)
// 			c := Config{
// 				Encoding: "text",
// 				ReplaceAttrs: []func(groups []string, a slog.Attr) slog.Attr{
// 					removeKeys(slog.TimeKey),
// 				},
// 				Out: buf,
// 			}
// 			c.Configure(ctl)
//
// 			ctl.Use("*", BareAttr())
//
// 			ctl.Logger("blue").Info("hi", tests[test].args...)
// 			l := ctl.Logger("blue")
// 			l.Info("hi", "chocolate")
// 			assert.Equal(t, tests[test].wantText, buf.String())
// 		})
// 	}
// }

func TestChainReplaceAttrs(t *testing.T) {
	tests := []struct {
		name             string
		wantText         string
		args             []any
		replaceAttrFuncs []func(groups []string, a slog.Attr) slog.Attr
	}{
		{
			name:     "bare value",
			wantText: "level=INFO msg=hi logger=blue color=greenbluegreen\n",
			args:     []any{"color", "green"},
			replaceAttrFuncs: []func(groups []string, a slog.Attr) slog.Attr{
				appendString("color", "blue"),
				appendString("color", "green"),
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

	l.Info("an error", "error", detailedJSONError{err})

	marshaledErr, merr := json.Marshal(fmt.Sprintf("%+v", err))
	require.NoError(t, merr)
	assert.JSONEq(t, fmt.Sprintf(`{"level":"INFO","logger":"main","msg":"an error","error":%v}`, string(marshaledErr)), buf.String())

	// mapstest.AssertEquivalent(t, map[string]any{"level": "INFO", LoggerKey: "main", "msg": "an error", "error": merry.Details(err)}, json.RawMessage(buf.String()))
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
	}

	for _, test := range tests {
		t.Run(test.name, test.Run)
	}
}
