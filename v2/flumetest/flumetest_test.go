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

	defer func() {
		SetDisabled(oldDisabled)
		SetVerbose(oldVerbose)
	}()

	t.Run("success_no_artifact_file", func(t *testing.T) {
		dir := t.TempDir()
		setArtifactsFlag(t)

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

		m := &mockTWithArtifacts{mockT: mockT{}, artifactDir: dir}

		require.Panics(t, func() {
			defer Start(m)()

			log.Info("artifact panic message", "key", "val")

			panic("boom")
		})
	})
}

func TestArtifactsEnabled(t *testing.T) {
	// Clean up any flags we register during the test.
	// We can't unregister flags, so we test with fresh names via Lookup behavior.
	t.Run("not_set", func(t *testing.T) {
		clearArtifactsFlag(t)
		assert.False(t, artifactsEnabled())
	})

	t.Run("artifacts_flag", func(t *testing.T) {
		setArtifactsFlag(t)
		assert.True(t, artifactsEnabled())
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

// setArtifactsFlag enables whichever artifacts flag is registered.
func setArtifactsFlag(t *testing.T) {
	t.Helper()

	for _, name := range []string{"test.artifacts", "artifacts"} {
		if f := flag.CommandLine.Lookup(name); f != nil {
			require.NoError(t, flag.CommandLine.Set(name, "true"))
			t.Cleanup(func() { _ = flag.CommandLine.Set(name, "false") })

			return
		}
	}

	t.Fatal("no artifacts flag found")
}

type mockTWithArtifacts struct {
	mockT

	artifactDir string
}

func (m *mockTWithArtifacts) ArtifactDir() string {
	return m.artifactDir
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
