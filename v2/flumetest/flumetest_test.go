package flumetest

import (
	"bytes"
	"flag"
	"fmt"
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

			defer func() {
				SetDisabled(oldDisabled)
				SetVerbose(oldVerbose)
			}()

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

		oldD, oldV, oldA := disabledPtr, verbosePtr, artifactsPtr
		oldOnce := initializeOnce
		oldFlagSet := flagSet

		t.Cleanup(func() {
			disabledPtr = oldD
			verbosePtr = oldV
			artifactsPtr = oldA
			initializeOnce = oldOnce
			flagSet = oldFlagSet
		})

		disabledPtr = nil
		verbosePtr = nil
		artifactsPtr = nil
		initializeOnce = &sync.Once{}
		flagSet = flag.NewFlagSet("test", flag.ContinueOnError)

		unsetenv(t, "FLUMETEST_DISABLE")
		unsetenv(t, "FLUME_TEST_DISABLE")
		unsetenv(t, "FLUMETEST_VERBOSE")
		unsetenv(t, "FLUMETEST_ARTIFACTS")
	}

	t.Run("defaults", func(t *testing.T) {
		setup(t)

		initialize()

		assert.False(t, *disabledPtr)
		assert.False(t, *verbosePtr)
		assert.False(t, *artifactsPtr)
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
