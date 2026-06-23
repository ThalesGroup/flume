package flumetest

import (
	"bytes"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ThalesGroup/flume/v2"
)

var (
	installMuxOnce sync.Once
	globalMux      *multiplexWriter
)

// multiplexWriter fans out writes to all currently enabled subscribers.
// The subscriber list is published as an immutable snapshot via an atomic pointer,
// so the hot Write path acquires no lock. Subscribe/Unsubscribe hold mu to
// maintain canonical state and publish new snapshots.
type multiplexWriter struct {
	mu     sync.Mutex
	base   io.Writer
	nextID uint64
	state  *muxState
	snap   atomic.Pointer[subscriberSnapshot]
}

// muxState is the canonical, mutable subscriber state. Only touched under multiplexWriter.mu.
type muxState struct {
	records []subscriberRecord
}

type subscriberRecord struct {
	id       uint64
	testName string
	depth    int
	sub      *subscriber
	enabled  bool
}

// subscriberSnapshot is an immutable snapshot of currently-enabled subscribers.
// Published atomically; readers never lock.
type subscriberSnapshot struct {
	items []*subscriber
}

// subscriber holds a single test's capture buffer.
type subscriber struct {
	mu      sync.Mutex
	buf     *bytes.Buffer
	closed  atomic.Bool
	bufCap  int // maximum bytes retained; 0 means unlimited
	dropped int // cumulative bytes discarded due to cap
}

// Compile-time interface assertions.
var (
	_ io.Writer   = (*multiplexWriter)(nil)
	_ io.Writer   = (*subscriber)(nil)
	_ io.WriterTo = (*subscriber)(nil)
)

func newMultiplexWriter(base io.Writer) *multiplexWriter {
	m := &multiplexWriter{
		base:  base,
		state: &muxState{},
	}
	m.snap.Store(&subscriberSnapshot{}) // non-nil sentinel; avoids nil check in Write

	return m
}

// ensureMuxInstalled installs the global multiplexWriter on flume.Default() exactly once.
func ensureMuxInstalled() *multiplexWriter {
	installMuxOnce.Do(func() {
		h := flume.Default()
		base := h.Out()
		globalMux = newMultiplexWriter(base)
		h.SetOut(globalMux)
	})

	return globalMux
}

func (m *multiplexWriter) Write(p []byte) (int, error) {
	snap := m.snap.Load()
	for _, sub := range snap.items {
		_, _ = sub.Write(p)
	}

	if m.base != nil {
		_, _ = m.base.Write(p)
	}

	return len(p), nil
}

func isAncestor(ancestorName, childName string) bool {
	return strings.HasPrefix(childName, ancestorName+"/")
}

func nameDepth(name string) int {
	return strings.Count(name, "/")
}

// Subscribe registers a capture buffer for the given test name, returning the
// rolled-over buffer if one already existed for the same test name.
// The returned bool indicates whether a rollover occurred.
//
// Returns: id, subscriber, oldSubscriber, replaced
//   - If a rollover occurred: id and sub are the existing subscription, oldSub is
//     the detached previous buffer (flushed by caller), replaced is true.
//   - If no same-name subscriber exists: handles child/subtest ancestor suspension
//     or new subscriber creation. oldSub is nil, replaced is false.
func (m *multiplexWriter) Subscribe(testName string) (id uint64, sub, oldSub *subscriber, replaced bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.state.records {
		if m.state.records[i].testName == testName {
			id = m.state.records[i].id
			sub = m.state.records[i].sub
			oldSub = sub.Rollover()

			return id, sub, oldSub, true
		}
	}

	for i := range m.state.records {
		if m.state.records[i].enabled && isAncestor(m.state.records[i].testName, testName) {
			m.state.records[i].enabled = false
		}
	}

	sub = newSubscriber(BufferLimit())

	m.nextID++
	id = m.nextID
	m.state.records = append(m.state.records, subscriberRecord{
		id:       id,
		testName: testName,
		depth:    nameDepth(testName),
		sub:      sub,
		enabled:  true,
	})

	m.rebuildSnapshot()

	return id, sub, nil, false
}

// Rollover detaches the current buffer for an active subscription and starts a
// fresh one. It returns false if the subscription has already ended.
func (m *multiplexWriter) Rollover(id uint64) (*subscriber, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := slices.IndexFunc(m.state.records, func(r subscriberRecord) bool {
		return r.id == id
	})

	if idx == -1 {
		return nil, false
	}

	return m.state.records[idx].sub.Rollover(), true
}

func (m *multiplexWriter) Unsubscribe(id uint64) {
	m.mu.Lock()

	idx := slices.IndexFunc(m.state.records, func(r subscriberRecord) bool {
		return r.id == id
	})

	if idx == -1 {
		m.mu.Unlock()
		return
	}

	removed := m.state.records[idx]
	m.state.records = slices.Delete(m.state.records, idx, idx+1)

	bestIdx := -1

	for i := range m.state.records {
		if isAncestor(m.state.records[i].testName, removed.testName) {
			if bestIdx == -1 || m.state.records[i].depth > m.state.records[bestIdx].depth {
				bestIdx = i
			}
		}
	}

	if bestIdx != -1 {
		m.state.records[bestIdx].enabled = true
	}

	m.rebuildSnapshot()
	m.mu.Unlock()

	removed.sub.Close()
}

func (m *multiplexWriter) rebuildSnapshot() {
	items := make([]*subscriber, 0, len(m.state.records))
	for i := range m.state.records {
		if m.state.records[i].enabled {
			items = append(items, m.state.records[i].sub)
		}
	}

	m.snap.Store(&subscriberSnapshot{items: items})
}

// newSubscriber constructs a fresh per-test capture buffer with the given byte limit.
// A bufCap of 0 means unlimited.
func newSubscriber(bufCap int) *subscriber {
	return &subscriber{buf: bytes.NewBuffer(nil), bufCap: bufCap}
}

// Rollover detaches the current buffer and starts a fresh one on the same
// subscriber so the original cleanup still owns the capture lifetime.
func (s *subscriber) Rollover() *subscriber {
	s.mu.Lock()
	defer s.mu.Unlock()

	old := &subscriber{
		buf:     s.buf,
		bufCap:  s.bufCap,
		dropped: s.dropped,
	}

	s.buf = bytes.NewBuffer(nil)
	s.dropped = 0

	return old
}

// Write appends p to the subscriber's buffer. No-ops if the subscriber is closed or freed.
// If a bufCap is configured and the buffer exceeds it after writing, the oldest bytes are
// discarded to bring the buffer back to the cap. Safe to call concurrently.
func (s *subscriber) Write(p []byte) (int, error) {
	if s.closed.Load() {
		return len(p), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() || s.buf == nil {
		return len(p), nil
	}

	n, _ := s.buf.Write(p) // bytes.Buffer.Write never returns a non-nil error

	if s.bufCap > 0 && s.buf.Len() > s.bufCap {
		drop := s.buf.Len() - s.bufCap
		s.buf.Next(drop)
		s.dropped += drop
	}

	return n, nil
}

// Close marks the subscriber as closed. Subsequent Write calls are no-ops.
// The buffer is retained so callers may still read via String or WriteTo.
func (s *subscriber) Close() {
	s.closed.Store(true)
}

// Free marks the subscriber as closed and releases the backing buffer storage,
// making the captured bytes eligible for garbage collection. All subsequent
// accessor calls return zero-length results. Free is idempotent and safe to
// call concurrently with any other method.
func (s *subscriber) Free() {
	s.closed.Store(true)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.buf = nil
}

// Dropped returns the cumulative number of bytes discarded due to the buffer cap.
func (s *subscriber) Dropped() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.dropped
}

// Len returns the current byte count in the buffer.
func (s *subscriber) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buf == nil {
		return 0
	}

	return s.buf.Len()
}

// String returns the buffer contents as a string.
func (s *subscriber) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buf == nil {
		return ""
	}

	return s.buf.String()
}

// WriteTo writes the buffer contents to w. Non-destructive: the buffer is not drained.
// Implements io.WriterTo.
func (s *subscriber) WriteTo(w io.Writer) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buf == nil {
		return 0, nil
	}

	n, err := w.Write(s.buf.Bytes())

	return int64(n), err
}
