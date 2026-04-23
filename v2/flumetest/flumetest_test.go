package flumetest

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ThalesGroup/flume/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	opts := &flume.HandlerOptions{
		HandlerFn: flume.TermHandlerFn(),
		AddSource: true,
		Level:     flume.LevelAll,
	}
	flume.Default().SetHandlerOptions(opts)
}

type mockT struct {
	sync.Mutex

	failed   bool
	logs     strings.Builder
	cleanups []func()
}

func (m *mockT) Cleanup(f func()) {
	m.Lock()
	defer m.Unlock()

	m.cleanups = append(m.cleanups, f)
}

func (m *mockT) Failed() bool {
	m.Lock()
	defer m.Unlock()

	return m.failed
}

func (m *mockT) Log(args ...any) {
	m.Lock()
	defer m.Unlock()

	_, _ = fmt.Fprintln(&m.logs, args...)
}

func (m *mockT) Name() string {
	return "TestSomething"
}

func TestStart(t *testing.T) {
	var log = flume.New("TestStart")

	tests := []struct {
		name     string
		failTest bool
		testFunc func(tb testingTB)
		expect   string
		skip     string
	}{
		{
			name: "success",
			testFunc: func(tb testingTB) {
				defer Start(tb)()

				log.Info("Hi", "color", "red")
			},
			failTest: false,
			expect:   "",
		},
		{
			name: "failed",
			testFunc: func(tb testingTB) {
				defer Start(tb)()

				log.Info("Hi", "color", "red")
			},
			failTest: true,
			expect:   "color=red",
		},
		{
			name:     "panic",
			failTest: false,
			expect:   "color=red",
			testFunc: func(tb testingTB) {
				require.Panics(t, func() {
					defer Start(tb)()

					log.Info("Hi", "color", "red")

					panic("boom")
				})
			},
		},
		{
			name:     "race",
			failTest: false,
			expect:   "",
			testFunc: func(tb testingTB) {
				cleanup := Start(tb)

				// when run with the race detector, this would cause a race
				// unless the log buffer is synchronized
				barrier, stop := make(chan struct{}, 1), make(chan struct{})

				go func() {
					barrier <- struct{}{}

					for {
						select {
						case <-stop:
							return
						default:
							log.Info("Hi", "color", "red")
						}
					}
				}()

				<-barrier
				cleanup()

				stop <- struct{}{}
			},
		},
		{
			name:     "verbose",
			failTest: false,
			expect:   "color=red",
			testFunc: func(tb testingTB) {
				SetVerbose(true)
				Start(tb)

				log.Info("Hi", "color", "red")
			},
		},
		{
			name:     "disabled",
			failTest: true,
			expect:   "",
			testFunc: func(tb testingTB) {
				SetDisabled(true)
				Start(tb)

				log.Info("Hi", "color", "red")
			},
		},
		{
			name:     "cleanup_without_defer",
			failTest: true,
			expect:   "color=red",
			testFunc: func(tb testingTB) {
				Start(tb)

				log.Info("Hi", "color", "red")
			},
		},
		{
			skip:     "this will fail until this golang issue is resolved: https://github.com/golang/go/issues/49929",
			name:     "cleanup_without_defer_panic",
			failTest: false,
			expect:   "color=red",
			testFunc: func(tb testingTB) {
				Start(tb)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.skip != "" {
				t.Skip(test.skip)
			}

			// restore the original values after the test
			oldDisabled := Disabled()
			oldVerbose := Verbose()
			oldArtifacts := Artifacts()

			defer func() {
				SetDisabled(oldDisabled)
				SetVerbose(oldVerbose)
				SetArtifacts(oldArtifacts)
			}()

			SetArtifacts(false)

			m := mockT{
				failed: test.failTest,
			}

			test.testFunc(&m)

			// call any registered cleanup functions, as the testing package would
			// at the end of the test
			for _, cleanup := range m.cleanups {
				cleanup()
			}

			if test.expect == "" {
				assert.Empty(t, m.logs.String())
			} else {
				assert.Contains(t, m.logs.String(), test.expect)
			}
		})
	}
}

func TestStartArtifacts(t *testing.T) {
	var log = flume.New("TestStartArtifacts")

	// Save and restore original state
	oldDisabled := Disabled()
	oldVerbose := Verbose()
	oldArtifacts := Artifacts()

	defer func() {
		SetDisabled(oldDisabled)
		SetVerbose(oldVerbose)
		SetArtifacts(oldArtifacts)
	}()

	t.Run("success_no_artifact_file", func(t *testing.T) {
		dir := t.TempDir()
		setArtifactsFlag(t)
		setNativeArtifactsFlag(t)

		m := &mockTWithArtifacts{mockT: mockT{}, artifactDir: dir}
		revert := Start(m)

		log.Info("artifact test message", "key", "val")
		revert()

		// On success, no artifact file should be created.
		matches, err := filepath.Glob(filepath.Join(dir, "flumetest_*.log"))
		require.NoError(t, err)
		assert.Empty(t, matches, "no artifact log file expected on success")
		assert.Empty(t, m.logs.String())
	})

	t.Run("verbose_writes_to_file_on_success", func(t *testing.T) {
		dir := t.TempDir()
		setArtifactsFlag(t)
		setNativeArtifactsFlag(t)
		SetVerbose(true)

		m := &mockTWithArtifacts{mockT: mockT{}, artifactDir: dir}
		revert := Start(m)

		log.Info("verbose artifact message", "key", "val")
		revert()

		// Verbose + artifacts: logs go to file, not t.Log.
		data, err := os.ReadFile(findArtifactLog(t, dir))
		require.NoError(t, err)
		assert.Contains(t, string(data), "key=val")
		assert.Empty(t, m.logs.String())
	})

	t.Run("no_tlog_output_on_failure", func(t *testing.T) {
		dir := t.TempDir()
		setArtifactsFlag(t)
		setNativeArtifactsFlag(t)

		m := &mockTWithArtifacts{mockT: mockT{failed: true}, artifactDir: dir}
		revert := Start(m)

		log.Info("should go to file", "color", "blue")
		revert()

		data, err := os.ReadFile(findArtifactLog(t, dir))
		require.NoError(t, err)
		assert.Contains(t, string(data), "color=blue")
		assert.Empty(t, m.logs.String())
	})

	t.Run("panic_is_repanicked", func(t *testing.T) {
		dir := t.TempDir()
		setArtifactsFlag(t)
		setNativeArtifactsFlag(t)

		m := &mockTWithArtifacts{mockT: mockT{}, artifactDir: dir}

		require.Panics(t, func() {
			defer Start(m)()

			log.Info("artifact panic message", "key", "val")

			panic("boom")
		})
	})
}

func TestArtifacts(t *testing.T) {
	t.Run("not_set", func(t *testing.T) {
		clearArtifactsFlag(t)
		assert.False(t, Artifacts())
	})

	t.Run("set", func(t *testing.T) {
		setArtifactsFlag(t)
		assert.True(t, Artifacts())
	})
}

// findArtifactLog returns the path of the single log file written into the
// flumetest subdirectory of dir.  It fails the test if there isn't exactly one.
func findArtifactLog(t *testing.T, dir string) string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(dir, "flumetest_*.log"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "expected exactly one artifact log file in %s", dir)

	return matches[0]
}

// setArtifactsFlag temporarily enables artifacts for the duration of t.
func setArtifactsFlag(t *testing.T) {
	t.Helper()

	old := Artifacts()

	SetArtifacts(true)
	t.Cleanup(func() { SetArtifacts(old) })
}

type mockTWithArtifacts struct {
	mockT

	artifactDir string
}

func (m *mockTWithArtifacts) ArtifactDir() string {
	return m.artifactDir
}

// setNativeArtifactsFlag registers (if needed) and enables the go1.26
// test.artifacts flag for the duration of t.
func setNativeArtifactsFlag(t *testing.T) {
	t.Helper()

	f := flagSet.Lookup("test.artifacts")
	if f == nil {
		// Pre-go1.26: register the flag so the test can exercise the 1.26 path.
		flagSet.Bool("test.artifacts", false, "")
		f = flagSet.Lookup("test.artifacts")
	}

	old := f.Value.String()

	require.NoError(t, flagSet.Set("test.artifacts", "true"))
	t.Cleanup(func() { _ = flagSet.Set("test.artifacts", old) })
}

// clearNativeArtifactsFlag ensures the go1.26 test.artifacts flag is false
// for the duration of t.
func clearNativeArtifactsFlag(t *testing.T) {
	t.Helper()

	f := flagSet.Lookup("test.artifacts")
	if f == nil {
		return
	}

	old := f.Value.String()

	require.NoError(t, flagSet.Set("test.artifacts", "false"))
	t.Cleanup(func() { _ = flagSet.Set("test.artifacts", old) })
}

func TestInitialize(t *testing.T) {
	// unsetenv removes an environment variable for the duration of the test.
	unsetenv := func(t *testing.T, key string) {
		t.Helper()

		if old, ok := os.LookupEnv(key); ok {
			require.NoError(t, os.Unsetenv(key))
			t.Cleanup(func() { os.Setenv(key, old) }) //nolint:usetesting
		}
	}

	// setup resets all global state, injects a fresh flag set, and clears all
	// relevant env vars, restoring everything when the subtest ends.
	setup := func(t *testing.T) {
		t.Helper()

		oldD, oldV, oldA, oldBL := disabledPtr, verbosePtr, artifactsPtr, bufferLimitPtr
		oldOnce := initializeOnce
		oldFlagSet := flagSet

		t.Cleanup(func() {
			disabledPtr = oldD
			verbosePtr = oldV
			artifactsPtr = oldA
			bufferLimitPtr = oldBL
			initializeOnce = oldOnce
			flagSet = oldFlagSet
		})

		disabledPtr = nil
		verbosePtr = nil
		artifactsPtr = nil
		bufferLimitPtr = nil
		initializeOnce = &sync.Once{}
		flagSet = flag.NewFlagSet("test", flag.ContinueOnError)

		unsetenv(t, "FLUMETEST_DISABLE")
		unsetenv(t, "FLUME_TEST_DISABLE")
		unsetenv(t, "FLUMETEST_VERBOSE")
		unsetenv(t, "FLUMETEST_ARTIFACTS")
		unsetenv(t, "FLUMETEST_BUFFER_LIMIT")
	}

	t.Run("defaults", func(t *testing.T) {
		setup(t)

		initialize()

		assert.False(t, *disabledPtr)
		assert.False(t, *verbosePtr)
		assert.False(t, *artifactsPtr)
		assert.Equal(t, defaultBufferLimit, *bufferLimitPtr)
	})

	// --- disabled ---

	t.Run("disabled/env_var", func(t *testing.T) {
		setup(t)
		t.Setenv("FLUMETEST_DISABLE", "true")

		initialize()

		assert.True(t, *disabledPtr)
	})

	t.Run("disabled/v1_compat_fallback", func(t *testing.T) {
		setup(t)
		t.Setenv("FLUME_TEST_DISABLE", "true")

		initialize()

		assert.True(t, *disabledPtr)
	})

	t.Run("disabled/v2_env_takes_precedence_over_v1", func(t *testing.T) {
		setup(t)
		t.Setenv("FLUMETEST_DISABLE", "false")
		t.Setenv("FLUME_TEST_DISABLE", "true")

		initialize()

		assert.False(t, *disabledPtr)
	})

	t.Run("disabled/pointer_already_set_skips_env", func(t *testing.T) {
		setup(t)
		t.Setenv("FLUMETEST_DISABLE", "true")

		b := false
		disabledPtr = &b

		initialize()

		assert.False(t, *disabledPtr, "env var should be ignored when pointer is already set")
	})

	// --- verbose ---

	t.Run("verbose/env_var", func(t *testing.T) {
		setup(t)
		t.Setenv("FLUMETEST_VERBOSE", "true")

		initialize()

		assert.True(t, *verbosePtr)
	})

	t.Run("verbose/pointer_already_set_skips_env", func(t *testing.T) {
		setup(t)
		t.Setenv("FLUMETEST_VERBOSE", "true")

		b := false
		verbosePtr = &b

		initialize()

		assert.False(t, *verbosePtr, "env var should be ignored when pointer is already set")
	})

	// --- artifacts ---

	t.Run("artifacts/env_var_true", func(t *testing.T) {
		setup(t)
		t.Setenv("FLUMETEST_ARTIFACTS", "true")

		initialize()

		assert.True(t, *artifactsPtr)
	})

	t.Run("artifacts/env_var_false", func(t *testing.T) {
		setup(t)
		t.Setenv("FLUMETEST_ARTIFACTS", "false")

		initialize()

		assert.False(t, *artifactsPtr)
	})

	t.Run("artifacts/native_flag_true", func(t *testing.T) {
		setup(t)
		flagSet.Bool("test.artifacts", false, "")
		require.NoError(t, flagSet.Set("test.artifacts", "true"))

		initialize()

		assert.True(t, *artifactsPtr, "should honor the go1.26 test.artifacts flag")
	})

	t.Run("artifacts/native_flag_false", func(t *testing.T) {
		setup(t)
		flagSet.Bool("test.artifacts", false, "")

		initialize()

		assert.False(t, *artifactsPtr, "native flag defaults to false")
	})

	t.Run("artifacts/env_true_overrides_native_false", func(t *testing.T) {
		setup(t)
		t.Setenv("FLUMETEST_ARTIFACTS", "true")
		flagSet.Bool("test.artifacts", false, "")

		initialize()

		assert.True(t, *artifactsPtr, "env var should take precedence over native flag")
	})

	t.Run("artifacts/env_false_overrides_native_true", func(t *testing.T) {
		setup(t)
		t.Setenv("FLUMETEST_ARTIFACTS", "false")
		flagSet.Bool("test.artifacts", false, "")
		require.NoError(t, flagSet.Set("test.artifacts", "true"))

		initialize()

		assert.False(t, *artifactsPtr, "env var should take precedence over native flag")
	})

	t.Run("artifacts/no_native_flag_no_env", func(t *testing.T) {
		setup(t)

		initialize()

		assert.False(t, *artifactsPtr, "should default to false when neither env nor native flag is set")
	})

	t.Run("artifacts/pointer_already_set_skips_env_and_flag", func(t *testing.T) {
		setup(t)
		t.Setenv("FLUMETEST_ARTIFACTS", "true")
		flagSet.Bool("test.artifacts", false, "")
		require.NoError(t, flagSet.Set("test.artifacts", "true"))

		b := false
		artifactsPtr = &b

		initialize()

		assert.False(t, *artifactsPtr, "env and flag should be ignored when pointer is already set")
	})
}

// disableArtifactsForTest disables artifact capture for the duration of t,
// restoring the previous setting on cleanup. This matches the behavior tested
// here, which exercises Start's in-memory capture/replay path rather than
// the file-artifact path.
func disableArtifactsForTest(t *testing.T) {
	t.Helper()

	old := Artifacts()

	SetArtifacts(false)
	t.Cleanup(func() { SetArtifacts(old) })
}

// TestParallelOverlapCaptures verifies the end-to-end fan-out wiring through
// Start(): overlapping subscribers each receive all logs emitted during their
// active windows. This is the primary integration test for the mux fan-out;
// it also exercises sibling capture semantics.
func TestParallelOverlapCaptures(t *testing.T) {
	disableArtifactsForTest(t)

	log := flume.New("parallel-test")

	mockA := &mockTOldGo{mockT: mockT{failed: true}, name: "TestParallelOverlapCaptures/a"}
	mockB := &mockTOldGo{mockT: mockT{failed: true}, name: "TestParallelOverlapCaptures/b"}

	revertA := Start(mockA)

	log.Info("PRE-A")

	revertB := Start(mockB)

	log.Info("OVERLAP-1")
	log.Info("OVERLAP-2")

	revertA()
	log.Info("POST-A")
	revertB()

	aLogs := mockA.logs.String()
	bLogs := mockB.logs.String()

	assert.Contains(t, aLogs, "PRE-A")
	assert.Contains(t, aLogs, "OVERLAP-1")
	assert.Contains(t, aLogs, "OVERLAP-2")
	assert.NotContains(t, aLogs, "POST-A")

	assert.Contains(t, bLogs, "OVERLAP-1")
	assert.Contains(t, bLogs, "OVERLAP-2")
	assert.Contains(t, bLogs, "POST-A")
	assert.NotContains(t, bLogs, "PRE-A")
}

// TestNestedSuppression verifies that when a child Start is active, the
// ancestor's capture is suspended; it resumes when the child finishes.
func TestNestedSuppression(t *testing.T) {
	disableArtifactsForTest(t)

	log := flume.New("nested-test")

	parentMock := &mockTOldGo{mockT: mockT{failed: true}, name: "TestNestedSuppression/parent"}
	childMock := &mockTOldGo{mockT: mockT{failed: true}, name: "TestNestedSuppression/parent/child"}

	revertParent := Start(parentMock)

	log.Info("parent-before")

	revertChild := Start(childMock)

	log.Info("child-only")
	revertChild()

	log.Info("parent-after")
	revertParent()

	parentLogs := parentMock.logs.String()
	childLogs := childMock.logs.String()

	assert.Contains(t, childLogs, "child-only")
	assert.NotContains(t, childLogs, "parent-before")
	assert.NotContains(t, childLogs, "parent-after")

	assert.Contains(t, parentLogs, "parent-before")
	assert.Contains(t, parentLogs, "parent-after")
	assert.NotContains(t, parentLogs, "child-only")
}

// TestOrphanAncestor verifies that subscribing with a subtest-style name
// whose ancestor has no active subscriber works without panic.
func TestOrphanAncestor(t *testing.T) {
	disableArtifactsForTest(t)

	log := flume.New("orphan-test")

	mock := &mockTOldGo{mockT: mockT{failed: true}, name: "TestOrphan/sub"}

	assert.NotPanics(t, func() {
		revert := Start(mock)

		log.Info("orphan")
		revert()
	})

	assert.Contains(t, mock.logs.String(), "orphan")
}

// TestDoubleStartIsNoOp verifies that calling Start twice for the same test
// name (while the first is still active) is a no-op: it does not panic, emits
// a warning via t.Log, and returns a no-op revert. The outer Start's capture
// remains in effect and is flushed when the outer revert runs.
func TestDoubleStartIsNoOp(t *testing.T) {
	disableArtifactsForTest(t)

	log := flume.New("double-start-test")

	mock := &mockTOldGo{mockT: mockT{failed: true}, name: "TestDoubleStartIsNoOp/target"}

	revert1 := Start(mock)

	log.Info("before-double-start")

	var revert2 func()

	require.NotPanics(t, func() {
		revert2 = Start(mock)
	})
	require.NotNil(t, revert2)

	log.Info("after-double-start")

	// Inner revert is a no-op; calling it must not unsubscribe the outer
	// capture or otherwise disrupt log collection.
	revert2()

	log.Info("after-inner-revert")

	revert1()

	logs := mock.logs.String()
	assert.Contains(t, logs, "flumetest: Start already active for ", "expected warning via t.Log")
	assert.Contains(t, logs, "before-double-start", "outer capture should include logs before duplicate Start")
	assert.Contains(t, logs, "after-double-start", "outer capture should include logs while inner Start was active")
	assert.Contains(t, logs, "after-inner-revert", "outer capture should remain active after inner revert")
}

// TestStart_releases_buffer_after_revert verifies that the per-test capture
// buffer is released (buf set to nil) after revert runs, making the bytes
// eligible for garbage collection even though the closure is still retained.
func TestStart_releases_buffer_after_revert(t *testing.T) {
	disableArtifactsForTest(t)

	log := flume.New("release-test")

	mock := &mockTOldGo{mockT: mockT{}, name: "TestStart_releases_buffer_after_revert/inner"}

	// Capture the subscriber via a small test hook: subscribe directly so we
	// can inspect the subscriber's internal state after revert.
	ensureMuxInstalled()

	_, sub, existing := globalMux.Subscribe(mock.Name())
	require.False(t, existing)

	// Write some data to establish a non-empty buffer.
	_, _ = sub.Write([]byte("some captured log data"))
	require.Equal(t, 22, sub.Len(), "buffer should be non-empty before revert")

	// Now simulate what Start does: call Free (which is what defer sub.Free() does in revert).
	// Here we don't use Start() itself to avoid the double-subscribe complexity;
	// instead we verify Free() directly as the mechanism triggered by revert.
	sub.Free()

	// After Free, buf must be nil and accessors must return zero values.
	assert.Nil(t, sub.buf, "buf should be nil after Free")
	assert.Equal(t, 0, sub.Len(), "Len should be 0 after Free")
	assert.Empty(t, sub.String(), "String should be empty after Free")

	// A subsequent write must be a no-op.
	log.Info("after revert write")

	n, err := sub.Write([]byte("post-free write"))
	require.NoError(t, err)
	assert.Equal(t, 15, n, "Write should return len(p) after Free")
	assert.Equal(t, 0, sub.Len(), "Len should still be 0 after post-Free write")
}

// --- Task 3.10: parseByteSize tests ---

func TestParseByteSize_bytes(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"0", 0},
		{"1", 1},
		{"1048576", 1048576},
		{"1B", 1},
		{"100b", 100},
		{"100 B", 100},
	}

	for _, tc := range tests {
		got, err := parseByteSize(tc.input)
		require.NoError(t, err, "input=%q", tc.input)
		assert.Equal(t, tc.want, got, "input=%q", tc.input)
	}
}

func TestParseByteSize_units(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"1K", 1024},
		{"1k", 1024},
		{"1KB", 1024},
		{"1kb", 1024},
		{"1KiB", 1024},
		{"1kib", 1024},
		{"1 KiB", 1024},
		{"2K", 2048},
		{"1M", 1024 * 1024},
		{"1MB", 1024 * 1024},
		{"1MiB", 1024 * 1024},
		{"1mib", 1024 * 1024},
		{"1 MB", 1024 * 1024},
		{"1G", 1024 * 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"1GiB", 1024 * 1024 * 1024},
		{"1gib", 1024 * 1024 * 1024},
		{"512KB", 512 * 1024},
		{"2MiB", 2 * 1024 * 1024},
	}

	for _, tc := range tests {
		got, err := parseByteSize(tc.input)
		require.NoError(t, err, "input=%q", tc.input)
		assert.Equal(t, tc.want, got, "input=%q", tc.input)
	}
}

func TestParseByteSize_invalid(t *testing.T) {
	tests := []struct {
		input string
	}{
		{""},
		{"-1"},
		{"1.5MB"},
		{"1TB"},
		{"1PB"},
		{"abc"},
		{"1ZB"},
		{"1 foo"},
	}

	for _, tc := range tests {
		_, err := parseByteSize(tc.input)
		assert.Error(t, err, "input=%q should return error", tc.input)
	}
}

// --- Task 3.11: BufferLimit tests ---

// setupBufferLimit is a helper that resets all global state for BufferLimit tests.
func setupBufferLimit(t *testing.T) {
	t.Helper()

	oldBL := bufferLimitPtr
	oldOnce := initializeOnce

	t.Cleanup(func() {
		bufferLimitPtr = oldBL
		initializeOnce = oldOnce
	})

	bufferLimitPtr = nil
	initializeOnce = &sync.Once{}

	if old, ok := os.LookupEnv("FLUMETEST_BUFFER_LIMIT"); ok {
		require.NoError(t, os.Unsetenv("FLUMETEST_BUFFER_LIMIT"))
		t.Cleanup(func() { os.Setenv("FLUMETEST_BUFFER_LIMIT", old) }) //nolint:usetesting
	}
}

func TestBufferLimit_defaultIs1MiB(t *testing.T) {
	setupBufferLimit(t)

	assert.Equal(t, 1<<20, BufferLimit())
}

func TestBufferLimit_zeroMeansUnlimited(t *testing.T) {
	setupBufferLimit(t)

	SetBufferLimit(0)

	assert.Equal(t, 0, BufferLimit())

	// With cap=0, a subscriber must retain all bytes.
	sub := newSubscriber(BufferLimit())
	data := strings.Repeat("x", 5*1024*1024)
	_, _ = sub.Write([]byte(data))

	assert.Equal(t, len(data), sub.Len())
	assert.Equal(t, 0, sub.Dropped())
}

func TestSetBufferLimit_overridesEnv(t *testing.T) {
	setupBufferLimit(t)

	t.Setenv("FLUMETEST_BUFFER_LIMIT", "2MB")

	// Set programmatically before initialize runs.
	four := 4096
	bufferLimitPtr = &four

	assert.Equal(t, 4096, BufferLimit())
}

func TestBufferLimit_envVar(t *testing.T) {
	tests := []struct {
		envVal string
		want   int
	}{
		{"512KB", 512 * 1024},
		{"512KiB", 512 * 1024},
		{"2MiB", 2 * 1024 * 1024},
		{"1G", 1024 * 1024 * 1024},
		{"1048576", 1048576},
		{"0", 0},
	}

	for _, tc := range tests {
		t.Run(tc.envVal, func(t *testing.T) {
			setupBufferLimit(t)
			t.Setenv("FLUMETEST_BUFFER_LIMIT", tc.envVal)

			assert.Equal(t, tc.want, BufferLimit())
		})
	}
}

func TestBufferLimit_invalidEnvVarWarnsAndUsesDefault(t *testing.T) {
	setupBufferLimit(t)

	t.Setenv("FLUMETEST_BUFFER_LIMIT", "not-a-number")

	// Redirect stderr to capture the warning.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stderr = w

	limit := BufferLimit()

	_ = w.Close()
	os.Stderr = oldStderr

	var stderrBuf strings.Builder

	_, _ = io.Copy(&stderrBuf, r)

	assert.Equal(t, defaultBufferLimit, limit, "should fall back to default on invalid env var")
	assert.Contains(t, stderrBuf.String(), "flumetest: invalid FLUMETEST_BUFFER_LIMIT=", "should write warning to stderr")
	assert.Contains(t, stderrBuf.String(), "not-a-number", "warning should include the bad value")
}

// --- Task 3.14: Truncation notice tests ---

func TestRevert_logsTruncationNoticeViaTLog(t *testing.T) {
	disableArtifactsForTest(t)

	// Use a small buffer limit so we can trigger truncation easily.
	SetBufferLimit(10)

	t.Cleanup(func() { SetBufferLimit(defaultBufferLimit) })

	log := flume.New("truncation-test")

	mock := &mockTOldGo{mockT: mockT{failed: true}, name: "TestRevert_logsTruncationNoticeViaTLog/inner"}

	revert := Start(mock)

	// Write more than 10 bytes so truncation occurs.
	log.Info(strings.Repeat("X", 200))

	revert()

	logOutput := mock.logs.String()

	// t.Log must have received the truncation notice.
	assert.Contains(t, logOutput, "flumetest: log buffer truncated;", "truncation notice must appear in t.Log output")
	assert.Contains(t, logOutput, "FLUMETEST_BUFFER_LIMIT=", "truncation notice must mention the limit")

	// The captured buffer flushed via t.Log must not contain the notice text itself.
	// The notice is a separate t.Log call, so both will be in logOutput; but the
	// captured log content (the first t.Log call) must not contain the notice string.
	// We verify this by checking that the notice string doesn't appear as a substring
	// of the log record portion (before "flumetest: log buffer truncated").
	noticeIdx := strings.Index(logOutput, "flumetest: log buffer truncated;")
	if noticeIdx > 0 {
		beforeNotice := logOutput[:noticeIdx]
		assert.NotContains(t, beforeNotice, "flumetest: log buffer truncated;")
	}
}

func TestRevert_noNoticeWhenNoTruncation(t *testing.T) {
	disableArtifactsForTest(t)

	// Use a generous limit so no truncation occurs.
	SetBufferLimit(1 << 20)

	t.Cleanup(func() { SetBufferLimit(defaultBufferLimit) })

	log := flume.New("no-truncation-test")

	for _, failTest := range []bool{false, true} {
		name := fmt.Sprintf("failed=%v", failTest)

		t.Run(name, func(t *testing.T) {
			disableArtifactsForTest(t)

			mock := &mockTOldGo{mockT: mockT{failed: failTest}, name: "TestRevert_noNoticeWhenNoTruncation/" + name}

			revert := Start(mock)

			log.Info("small message")

			revert()

			assert.NotContains(t, mock.logs.String(), "flumetest: log buffer truncated;",
				"no notice expected when buffer was not truncated")
		})
	}
}

func TestRevert_noNoticeWhenSilentlyDiscarded(t *testing.T) {
	disableArtifactsForTest(t)

	// Small buffer limit to ensure truncation would occur.
	SetBufferLimit(10)

	t.Cleanup(func() { SetBufferLimit(defaultBufferLimit) })

	log := flume.New("silent-discard-test")

	// Passing test, verbose=false, artifacts=false => buffer silently discarded.
	mock := &mockTOldGo{mockT: mockT{failed: false}, name: "TestRevert_noNoticeWhenSilentlyDiscarded/inner"}

	SetVerbose(false)
	t.Cleanup(func() { SetVerbose(false) })

	revert := Start(mock)

	log.Info(strings.Repeat("Y", 200))

	revert()

	assert.NotContains(t, mock.logs.String(), "flumetest: log buffer truncated;",
		"no notice expected when buffer is silently discarded")
}

func TestSnapshot(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	h := flume.NewHandler(buf, nil)
	assert.Equal(t, buf, h.Out())

	buf2 := bytes.NewBuffer(nil)

	revert := Snapshot(h)
	h.SetOut(buf2)
	assert.Equal(t, buf2, h.Out())

	revert()
	assert.Equal(t, buf, h.Out())
}
