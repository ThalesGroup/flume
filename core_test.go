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
