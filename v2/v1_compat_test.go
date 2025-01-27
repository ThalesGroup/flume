package flume

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

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
