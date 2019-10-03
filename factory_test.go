package flume

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
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
