package flume

import (
	"testing"
)

func TestNewLTSVEncoder(t *testing.T) {
	SetOut(LogFuncWriter(t.Log, true))
	ConfigString(`{"encoding":"ltsv","level":"INF"}`)
	SetAddCaller(true)
	l := New("core")
	l.Debug("hi mom", "color", 1)
	l.Info("hi mom", "color", 2)
	l.Info("what's up?")
	l.Info("that's cool")
}
