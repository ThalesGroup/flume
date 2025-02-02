package flume

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Cleanup(func() {
		SetDefault(nil)
	})

	buf := bytes.NewBuffer(nil)
	ctl, err := Config{
		DefaultSink: TextSink,
	}.Controller()
	require.NoError(t, err)
	SetDefault(ctl)
	New("blue").Info("hi")
	assert.Contains(t, "level=INFO msg=hi logger=blue\n", buf.String())
}

func TestHandler(t *testing.T) {
	t.Cleanup(func() {
		SetDefault(nil)
	})

	buf := bytes.NewBuffer(nil)
	ctl, err := Config{
		DefaultSink: TextSink,
	}.Controller()
	require.NoError(t, err)
	SetDefault(ctl)

	h := Handler("blue")
	require.NotNil(t, h)

	h.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0))
	assert.Contains(t, "level=INFO msg=hi logger=blue\n", buf.String())
}

func TestDefault(t *testing.T) {
	t.Cleanup(func() {
		SetDefault(nil)
	})

	ctl, err := Config{
		DefaultSink: TextSink,
	}.Controller()
	require.NoError(t, err)
	SetDefault(ctl)

	assert.Equal(t, ctl, Default())
}
