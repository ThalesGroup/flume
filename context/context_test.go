package context

import (
	"context"
	"github.com/stretchr/testify/assert"
	"gitlab.protectv.local/regan/flume.git"
	"testing"
)

func TestWithLogger(t *testing.T) {
	ctx := context.Background()

	l := flume.New("hi")
	ctx2 := WithLogger(ctx, l)
	v := ctx2.Value(loggerKey)
	assert.EqualValues(t, l, v)
}

func TestFromContext(t *testing.T) {
	ctx := context.Background()

	l := flume.New("hi")
	ctx2 := WithLogger(ctx, l)
	l2 := FromContext(ctx2)
	assert.EqualValues(t, l, l2)

	t.Run("default", func(t *testing.T) {
		l := FromContext(context.Background())
		assert.EqualValues(t, l, DefaultLogger)

		defL := DefaultLogger
		defer func() {
			DefaultLogger = defL
		}()

		l2 := flume.New("l2")
		DefaultLogger = l2
		l3 := FromContext(context.Background())
		assert.EqualValues(t, l2, l3)
	})
}
