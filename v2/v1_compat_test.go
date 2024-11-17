package flume

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"log/slog"
	"testing"
)

func appendString(key, value string) func(groups []string, a slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
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

			f := NewFactory(slog.NewTextHandler(buf, &slog.HandlerOptions{ReplaceAttr: ChainReplaceAttrs(replaceAttrFuncs...)}))

			l := slog.New(f.NewHandler("blue"))
			l.Info("hi", test.args...)
			assert.Equal(t, test.wantText, buf.String())
		})
	}
}
