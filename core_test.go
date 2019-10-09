package flume

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"testing"
)

func TestSweetenFields(t *testing.T) {

	// single value, instead of key/value pairs
	c := NewCore("asdf")
	fields := c.sweetenFields([]interface{}{1})

	assert.Equal(t, []zap.Field{zap.Int("", 1)}, fields)

	// if the bare value is an error, use the key "error"
	err := errors.New("blue")
	fields = c.sweetenFields([]interface{}{err})
	assert.Equal(t, []zap.Field{zap.NamedError("error", err)}, fields)
}

func TestCore_IsDebug(t *testing.T) {
	f := NewFactory()
	f.SetDefaultLevel(InfoLevel)

	l := f.NewLogger("asdf")
	assert.False(t, l.IsDebug())
	f.SetDefaultLevel(DebugLevel)
	assert.True(t, l.IsDebug())
	f.SetDefaultLevel(ErrorLevel)
	assert.False(t, l.IsDebug())
	f.SetLevel("asdf", DebugLevel)
	assert.True(t, l.IsDebug())
}

func TestCore_IsInfo(t *testing.T) {
	f := NewFactory()
	f.SetDefaultLevel(ErrorLevel)

	l := f.NewLogger("asdf")
	assert.False(t, l.IsInfo())
	f.SetDefaultLevel(InfoLevel)
	assert.True(t, l.IsInfo())
	f.SetDefaultLevel(ErrorLevel)
	assert.False(t, l.IsInfo())
	f.SetLevel("asdf", DebugLevel)
	assert.True(t, l.IsInfo())
}
