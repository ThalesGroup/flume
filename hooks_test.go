package flume

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/ansel1/merry"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

var suppressErrVerboseOnInfoHook = func(entry *CheckedEntry, fields []Field) []Field {
	for i := range fields {
		if fields[i].Type == zapcore.ErrorType && entry.Level == zapcore.InfoLevel {
			if err, ok := fields[i].Interface.(error); ok {
				if _, ok := err.(fmt.Formatter); ok {
					err = errors.New(err.Error())
					fields[i].Interface = err
				}
			}
		}
	}
	return fields
}

func TestHook(t *testing.T) {
	f := NewFactory()
	f.SetDefaultLevel(DebugLevel)
	f.Hooks(suppressErrVerboseOnInfoHook)

	buf := bytes.NewBuffer(nil)
	f.SetOut(buf)

	// The hook should suppress the errVerbose key when the level is INF.
	logger := f.NewLogger("")
	logger.Error("boom", merry.New("boom"))
	assert.Contains(t, buf.String(), "errorVerbose")

	buf.Reset()
	logger.Info("boom", merry.New("boom"))
	assert.NotContains(t, buf.String(), "errorVerbose")

	t.Run("ClearHooks", func(t *testing.T) {
		f.ClearHooks()

		buf.Reset()
		logger.Info("boom", merry.New("boom"))
		assert.Contains(t, buf.String(), "errorVerbose")
	})
}

func TestAddHooks(t *testing.T) {
	f := NewFactory()
	f.SetDefaultLevel(DebugLevel)
	buf := bytes.NewBuffer(nil)
	f.SetOut(buf)

	withoutHook := f.NewCore("withouthook")
	withHook := f.NewCore("withhook", AddHooks(suppressErrVerboseOnInfoHook))

	withoutHook.Info("boom", merry.New("boom"))
	assert.Contains(t, buf.String(), "errorVerbose")

	buf.Reset()
	withHook.Info("boom", merry.New("boom"))
	assert.NotContains(t, buf.String(), "errorVerbose")
}

func BenchmarkNoHook(b *testing.B) {
	b.ReportAllocs()
	factory := NewFactory()
	factory.SetDefaultLevel(DebugLevel)
	factory.SetOut(io.Discard)
	logger := factory.NewLogger("")
	err := merry.New("boom")
	for i := 0; i < b.N; i++ {
		logger.Error("boom", err)
	}
}

func BenchmarkHookNoop(b *testing.B) {
	b.ReportAllocs()
	factory := NewFactory()
	factory.SetDefaultLevel(DebugLevel)
	factory.SetOut(io.Discard)
	logger := factory.NewLogger("")
	err := merry.New("boom")
	factory.Hooks(suppressErrVerboseOnInfoHook)
	for i := 0; i < b.N; i++ {
		logger.Error("boom", err)
	}
}

func BenchmarkHook(b *testing.B) {
	b.ReportAllocs()
	factory := NewFactory()
	factory.SetDefaultLevel(DebugLevel)
	factory.SetOut(io.Discard)
	logger := factory.NewLogger("")
	err := merry.New("boom")
	factory.Hooks(suppressErrVerboseOnInfoHook)
	for i := 0; i < b.N; i++ {
		logger.Info("boom", err)
	}
}
