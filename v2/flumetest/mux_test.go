package flumetest

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/ThalesGroup/flume/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriber_WriteAndRead(t *testing.T) {
	sub := newSubscriber(0)

	n, err := sub.Write([]byte("hello"))

	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, 5, sub.Len())
	assert.Equal(t, "hello", sub.String())
}

func TestSubscriber_WriteAfterClose(t *testing.T) {
	sub := newSubscriber(0)

	n, err := sub.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	sub.Close()
	before := sub.Len()

	n, err = sub.Write([]byte("world"))

	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, before, sub.Len())
	assert.Equal(t, "hello", sub.String())
}

func TestSubscriber_WriteToNonDestructive(t *testing.T) {
	sub := newSubscriber(0)

	n, err := sub.Write([]byte("content"))
	require.NoError(t, err)
	assert.Equal(t, 7, n)

	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}

	written1, err := sub.WriteTo(buf1)
	require.NoError(t, err)
	assert.EqualValues(t, 7, written1)

	written2, err := sub.WriteTo(buf2)
	require.NoError(t, err)
	assert.EqualValues(t, 7, written2)

	assert.Equal(t, "content", buf1.String())
	assert.Equal(t, "content", buf2.String())
}

func TestSubscriber_ConcurrentWrites(t *testing.T) {
	sub := newSubscriber(0)

	const (
		goroutines         = 100
		writesPerGoroutine = 100
	)

	var wg sync.WaitGroup

	start := make(chan struct{})

	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()

			<-start

			for range writesPerGoroutine {
				n, err := sub.Write([]byte("x"))
				assert.NoError(t, err)
				assert.Equal(t, 1, n)
			}
		}()
	}

	close(start)
	wg.Wait()

	assert.Equal(t, goroutines*writesPerGoroutine, sub.Len())
}

func TestSubscriber_CloseIsIdempotent(t *testing.T) {
	sub := newSubscriber(0)

	assert.NotPanics(t, func() {
		sub.Close()
		sub.Close()
		sub.Close()
	})

	n, err := sub.Write([]byte("hello"))

	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, 0, sub.Len())
}

func TestMux_InstallOnce(t *testing.T) {
	first := ensureMuxInstalled()

	assert.Same(t, globalMux, flume.Default().Out())

	for range 10 {
		assert.Same(t, first, ensureMuxInstalled())
	}
}

func TestMux_WriteZeroSubscribers(t *testing.T) {
	mux := newMultiplexWriter(nil)

	n, err := mux.Write([]byte("hello"))

	require.NoError(t, err)
	assert.Equal(t, 5, n)
}

func TestMux_ConcurrentWrites(t *testing.T) {
	mux := newMultiplexWriter(nil)

	const (
		goroutines         = 50
		writesPerGoroutine = 100
	)

	var wg sync.WaitGroup

	start := make(chan struct{})

	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()

			<-start

			for range writesPerGoroutine {
				n, err := mux.Write([]byte("x"))
				assert.NoError(t, err)
				assert.Equal(t, 1, n)
			}
		}()
	}

	close(start)
	wg.Wait()
}

func TestIsAncestor(t *testing.T) {
	tests := []struct {
		ancestor, child string
		want            bool
	}{
		{"TestFoo", "TestFoo/sub", true},
		{"TestFoo", "TestFooBar", false},
		{"TestFoo", "TestFoo", false},
		{"TestFoo/a", "TestFoo/b", false},
		{"TestFoo/a", "TestFoo/a/b", true},
		{"", "TestFoo", false},
	}
	for _, tc := range tests {
		got := isAncestor(tc.ancestor, tc.child)
		assert.Equal(t, tc.want, got, "isAncestor(%q, %q)", tc.ancestor, tc.child)
	}
}

func TestMux_UnsubscribeUnknownID(t *testing.T) {
	m := newMultiplexWriter(nil)

	assert.NotPanics(t, func() {
		m.Unsubscribe(9999)
	})
}

func TestMux_WriteToBase(t *testing.T) {
	base := &bytes.Buffer{}
	m := newMultiplexWriter(base)

	n, err := m.Write([]byte("hello"))

	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", base.String())
}

func TestMux_ResubscribeAfterUnsubscribe(t *testing.T) {
	m := newMultiplexWriter(nil)

	id1, _, _ := m.Subscribe("TestFoo")
	m.Unsubscribe(id1)

	assert.NotPanics(t, func() {
		id2, sub2, existing := m.Subscribe("TestFoo")
		require.False(t, existing)

		_, _ = m.Write([]byte("y"))

		assert.Contains(t, sub2.String(), "y")
		m.Unsubscribe(id2)
	})
}

func TestSubscriber_capRetainsMostRecentBytes(t *testing.T) {
	const bufCap = 1024

	sub := newSubscriber(bufCap)

	// Write 4096 bytes total (3072 then 1024).
	first := strings.Repeat("A", 3072)
	last := strings.Repeat("Z", 1024)

	_, _ = sub.Write([]byte(first))
	_, _ = sub.Write([]byte(last))

	got := sub.String()

	assert.Len(t, got, bufCap, "buffer should contain exactly bufCap bytes")
	assert.Equal(t, last, got, "buffer should contain the most recent bytes")
	assert.Equal(t, 3072, sub.Dropped(), "dropped should equal bytes trimmed")
}

func TestSubscriber_capCapturedAtSubscribeTime(t *testing.T) {
	// Set buffer limit to 1024, then subscribe.
	SetBufferLimit(1024)
	t.Cleanup(func() { SetBufferLimit(defaultBufferLimit) })

	m := newMultiplexWriter(nil)

	_, sub, existing := m.Subscribe("TestFoo/D4")
	require.False(t, existing)

	// Now change the limit; this must not affect the already-subscribed sub.
	SetBufferLimit(8192)

	// Write 2048 bytes — if the original cap of 1024 is in effect, the buffer
	// must be capped at 1024 (not 2048 or 8192).
	_, _ = sub.Write([]byte(strings.Repeat("X", 2048)))

	assert.Equal(t, 1024, sub.Len(), "subscriber cap must be fixed at Subscribe time")
	assert.Equal(t, 1024, sub.Dropped(), "dropped bytes must reflect original cap")
}

func TestUnsubscribe_clears_backing_array_refs(t *testing.T) {
	m := newMultiplexWriter(nil)

	const n = 5

	ids := make([]uint64, n)
	for i := range n {
		id, _, existing := m.Subscribe(fmt.Sprintf("TestFoo/sub%d", i))
		require.False(t, existing)

		ids[i] = id
	}

	// Unsubscribe the middle subscriber (index 2).
	targetID := ids[2]
	m.Unsubscribe(targetID)

	// After deletion, len(records) == n-1.
	// The backing array has capacity >= n, so slot at index n-1
	// (i.e., records[:cap(records)][n-1]) must be zero-valued.
	m.mu.Lock()
	full := m.state.records[:cap(m.state.records)]
	m.mu.Unlock()

	// The trailing slot must have a nil sub field.
	trailing := full[len(m.state.records):]
	for _, slot := range trailing {
		assert.Nil(t, slot.sub, "trailing backing-array slot must have nil sub after Unsubscribe")
	}
}

func TestSubscriber_Free_is_safe(t *testing.T) {
	t.Run("Free is idempotent", func(t *testing.T) {
		sub := newSubscriber(0)

		assert.NotPanics(t, func() {
			sub.Free()
			sub.Free()
			sub.Free()
		})
	})

	t.Run("post-Free Write is no-op returning len(p) nil", func(t *testing.T) {
		sub := newSubscriber(0)
		sub.Free()

		n, err := sub.Write([]byte("hello"))

		require.NoError(t, err)
		assert.Equal(t, 5, n)
	})

	t.Run("post-Free Len returns 0", func(t *testing.T) {
		sub := newSubscriber(0)
		_, _ = sub.Write([]byte("some content"))
		sub.Free()

		assert.Equal(t, 0, sub.Len())
	})

	t.Run("post-Free String returns empty", func(t *testing.T) {
		sub := newSubscriber(0)
		_, _ = sub.Write([]byte("some content"))
		sub.Free()

		assert.Empty(t, sub.String())
	})

	t.Run("post-Free WriteTo returns 0 nil", func(t *testing.T) {
		sub := newSubscriber(0)
		_, _ = sub.Write([]byte("some content"))
		sub.Free()

		var buf bytes.Buffer

		n, err := sub.WriteTo(&buf)

		require.NoError(t, err)
		assert.EqualValues(t, 0, n)
		assert.Empty(t, buf.String())
	})

	t.Run("concurrent Free and accessor calls are race-free", func(_ *testing.T) {
		sub := newSubscriber(0)

		var wg sync.WaitGroup

		wg.Add(4)

		go func() {
			defer wg.Done()

			for range 100 {
				sub.Free()
			}
		}()

		go func() {
			defer wg.Done()

			for range 100 {
				_, _ = sub.Write([]byte("x"))
			}
		}()

		go func() {
			defer wg.Done()

			for range 100 {
				_ = sub.Len()
				_ = sub.String()
			}
		}()

		go func() {
			defer wg.Done()

			var buf bytes.Buffer

			for range 100 {
				_, _ = sub.WriteTo(&buf)
				buf.Reset()
			}
		}()

		wg.Wait()
	})
}

func BenchmarkMuxWrite(b *testing.B) {
	m := newMultiplexWriter(nil)

	subs := make([]*subscriber, 10)
	for i := range subs {
		_, subs[i], _ = m.Subscribe(fmt.Sprintf("bench/sub%d", i))
	}

	payload := []byte("benchmark log entry\n")

	b.ResetTimer()

	for b.Loop() {
		_, _ = m.Write(payload)
	}
}
