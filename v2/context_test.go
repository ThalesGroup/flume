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

func TestContextWithAttrs_emptyReturnsSameContext(t *testing.T) {
	ctx := context.Background()
	got := ContextWithAttrs(ctx)
	assert.Equal(t, ctx, got)
}

func TestContextWithAttrs_singleCall(t *testing.T) {
	ctx := ContextWithAttrs(context.Background(),
		slog.String("color", "blue"),
		slog.Int("size", 42),
	)

	attrs := attrsFromContext(ctx)
	require.Len(t, attrs, 2)
	assert.Equal(t, "color", attrs[0].Key)
	assert.Equal(t, "blue", attrs[0].Value.String())
	assert.Equal(t, "size", attrs[1].Key)
	assert.Equal(t, int64(42), attrs[1].Value.Int64())
}

func TestContextWithAttrs_nestedCallsAppend(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithAttrs(ctx, slog.String("a", "1"))
	ctx = ContextWithAttrs(ctx, slog.String("b", "2"))
	ctx = ContextWithAttrs(ctx, slog.String("c", "3"))

	attrs := attrsFromContext(ctx)
	require.Len(t, attrs, 3)
	assert.Equal(t, "a", attrs[0].Key)
	assert.Equal(t, "b", attrs[1].Key)
	assert.Equal(t, "c", attrs[2].Key)
}

func TestContextWithAttrs_parentNotMutated(t *testing.T) {
	parent := ContextWithAttrs(context.Background(), slog.String("a", "1"))

	childA := ContextWithAttrs(parent, slog.String("b", "2"))
	childB := ContextWithAttrs(parent, slog.String("c", "3"))

	assert.Len(t, attrsFromContext(parent), 1)

	childAAttrs := attrsFromContext(childA)
	require.Len(t, childAAttrs, 2)
	assert.Equal(t, "b", childAAttrs[1].Key)

	childBAttrs := attrsFromContext(childB)
	require.Len(t, childBAttrs, 2)
	assert.Equal(t, "c", childBAttrs[1].Key)
}

func TestAttrsFromContext_nilContext(t *testing.T) {
	var ctx context.Context
	assert.Nil(t, attrsFromContext(ctx))
}

func TestAttrsFromContext_noAttrs(t *testing.T) {
	assert.Nil(t, attrsFromContext(context.Background()))
}

func TestContextAttrsMiddleware_appendsToRecord(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	h := NewHandler(buf, &HandlerOptions{
		Middleware: []Middleware{ContextAttrsMiddleware()},
	})

	ctx := ContextWithAttrs(context.Background(),
		slog.String("request_id", "req-123"),
		slog.String("tenant", "acme"),
	)

	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0)
	rec.AddAttrs(slog.String("color", "blue"))

	require.NoError(t, h.Handle(ctx, rec))

	assert.Equal(t,
		"level=INFO msg=hi color=blue request_id=req-123 tenant=acme\n",
		buf.String(),
	)
}

func TestContextAttrsMiddleware_duplicateMiddleware(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	h := NewHandler(buf, &HandlerOptions{
		Middleware: []Middleware{
			ContextAttrsMiddleware(),
			ContextAttrsMiddleware(),
			ContextAttrsMiddleware(),
		},
	})

	ctx := ContextWithAttrs(context.Background(),
		slog.String("request_id", "req-123"),
	)

	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0)
	require.NoError(t, h.Handle(ctx, rec))

	// attrs should appear exactly once, not three times
	assert.Equal(t,
		"level=INFO msg=hi request_id=req-123\n",
		buf.String(),
	)
}

func TestContextAttrsMiddleware_noAttrsPassesThrough(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	h := NewHandler(buf, &HandlerOptions{
		Middleware: []Middleware{ContextAttrsMiddleware()},
	})

	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0)
	rec.AddAttrs(slog.String("color", "blue"))

	require.NoError(t, h.Handle(context.Background(), rec))

	assert.Equal(t,
		"level=INFO msg=hi color=blue\n",
		buf.String(),
	)
}
