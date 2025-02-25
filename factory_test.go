package flume

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestFactory_Configure(t *testing.T) {
	f := NewFactory()

	err := f.Configure(Config{
		DefaultLevel: DebugLevel,
	})
	require.NoError(t, err)

	assert.True(t, f.NewLogger("asdf").IsDebug())

	err = f.Configure(Config{
		DefaultLevel: DebugLevel,
		Levels:       "*=INF",
	})

	require.NoError(t, err)
	assert.False(t, f.NewLogger("asdf").IsDebug())

}

func TestFactory_SetNewCoreFn(t *testing.T) {
	f := NewFactory()

	called := false
	name := ""
	f.SetNewCoreFn(func(n string, encoder zapcore.Encoder, out zapcore.WriteSyncer, levelEnabler zapcore.LevelEnabler) zapcore.Core {
		called = true
		name = n
		return zapcore.NewCore(encoder, out, levelEnabler)
	})

	// Creating a new logger should trigger the custom core func
	l := f.NewLogger("test")
	assert.True(t, called)
	assert.Equal(t, "test", name)
	l.Info("test")

	// Setting to nil should revert to default behavior
	f.SetNewCoreFn(nil)
	called = false
	l = f.NewLogger("test2")
	assert.False(t, called)
	l.Info("test")
}
