package flumetest

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/ThalesGroup/flume/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTOldGo simulates *testing.T on pre-go1.26: provides Name() but not ArtifactDir().
type mockTOldGo struct {
	mockT

	name string
}

func (m *mockTOldGo) Name() string { return m.name }

// clearArtifactsFlag temporarily disables artifacts for the duration of t.
func clearArtifactsFlag(t *testing.T) {
	t.Helper()

	old := Artifacts()

	SetArtifacts(false)
	t.Cleanup(func() { SetArtifacts(old) })
}

// setOutputDir temporarily points the test.outputdir flag at a fresh temp directory
// for the duration of t, and returns that directory's path.  The test is skipped
// if the flag is not registered in this binary.
func setOutputDir(t *testing.T) string {
	t.Helper()

	f := flagSet.Lookup("test.outputdir")
	if f == nil {
		t.Skip("test.outputdir flag not registered in this test binary")
	}

	dir := t.TempDir()
	old := f.Value.String()

	require.NoError(t, flagSet.Set("test.outputdir", dir))
	t.Cleanup(func() { _ = flagSet.Set("test.outputdir", old) })

	return dir
}

// findArtifactLogRecursive walks baseDir and returns the single flumetest_*.log
// file found anywhere beneath it.  Fails the test if there isn't exactly one.
func findArtifactLogRecursive(t *testing.T, baseDir string) string {
	t.Helper()

	var found []string

	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() &&
			strings.HasPrefix(d.Name(), "flumetest_") &&
			strings.HasSuffix(d.Name(), ".log") {
			found = append(found, path)
		}

		return nil
	})
	require.NoError(t, err)
	require.Len(t, found, 1, "expected exactly one artifact log file under %s", baseDir)

	return found[0]
}

// assertNoArtifactLogs walks baseDir and fails the test if any flumetest_*.log
// files are found.
func assertNoArtifactLogs(t *testing.T, baseDir string) {
	t.Helper()

	var found []string

	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() &&
			strings.HasPrefix(d.Name(), "flumetest_") &&
			strings.HasSuffix(d.Name(), ".log") {
			found = append(found, path)
		}

		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, found, "expected no artifact log files under %s", baseDir)
}

// mockTOldGoWithOutput simulates *testing.T on go1.25: provides Name() and Output() but not ArtifactDir().
type mockTOldGoWithOutput struct {
	mockTOldGo

	output strings.Builder
}

func (m *mockTOldGoWithOutput) Output() io.Writer { return &m.output }

// TestGetArtifactDir exercises the pre-go1.26 emulation paths in getArtifactDir:
// artifact flag checks, name sanitisation, default name fallback, and long-name
// truncation with a hash suffix.
func TestGetArtifactDir(t *testing.T) {
	t.Run("artifacts_disabled", func(t *testing.T) {
		// No ArtifactDir() method and the -artifacts flag is not set.
		clearArtifactsFlag(t)

		m := &mockTOldGo{name: "SomeTest"}
		dir, ok := getArtifactDir(m)
		assert.False(t, ok)
		assert.Empty(t, dir)
	})

	t.Run("with_name_creates_dir_inside_outputdir", func(t *testing.T) {
		setArtifactsFlag(t)
		outDir := setOutputDir(t)

		m := &mockTOldGo{name: "TestGetArtifactDir/with_name"}
		dir, ok := getArtifactDir(m)
		require.True(t, ok)
		assert.DirExists(t, dir)

		// The returned dir must live inside outDir.
		rel, err := filepath.Rel(outDir, dir)
		require.NoError(t, err)
		assert.False(t, strings.HasPrefix(rel, ".."), "dir should be inside outDir")

		// The sanitised test name must appear as a path component.
		assert.Contains(t, dir, "TestGetArtifactDir__with_name")
	})

	t.Run("nice_name_uses_name_as_prefix", func(t *testing.T) {
		setArtifactsFlag(t)
		outDir := setOutputDir(t)

		m := &mockTOldGo{name: "NiceTestName"}
		dir, ok := getArtifactDir(m)
		require.True(t, ok)
		assert.NotEmpty(t, dir)

		// Should be created under outDir/_artifacts/flumetest/NiceTestName/<tempdir>.
		prefix := filepath.Join(outDir, "_artifacts/flumetest/NiceTestName") + string(filepath.Separator)
		assert.True(t, strings.HasPrefix(dir, prefix),
			"expected dir %q to start with %q", dir, prefix)
	})

	t.Run("consecutive_calls_return_same_dir", func(t *testing.T) {
		setArtifactsFlag(t)
		setOutputDir(t)

		m := &mockTOldGo{name: "TestGetArtifactDir/consecutive"}
		dir1, ok1 := getArtifactDir(m)
		require.True(t, ok1)

		dir2, ok2 := getArtifactDir(m)
		require.True(t, ok2)

		assert.Equal(t, dir1, dir2, "expected consecutive calls to return the same directory")
	})

	t.Run("go126_native_flag_delegates_to_ArtifactDir", func(t *testing.T) {
		setArtifactsFlag(t)
		setNativeArtifactsFlag(t)

		dir := t.TempDir()
		m := &mockTWithArtifacts{mockT: mockT{}, artifactDir: dir}

		got, ok := getArtifactDir(m)
		require.True(t, ok)
		assert.Equal(t, dir, got, "should delegate to t.ArtifactDir() when native flag is set")
	})

	t.Run("go126_native_flag_false_uses_backfill", func(t *testing.T) {
		setArtifactsFlag(t)
		clearNativeArtifactsFlag(t)
		outDir := setOutputDir(t)

		m := &mockTWithArtifacts{
			mockT:       mockT{},
			artifactDir: "/should/not/be/used",
		}

		got, ok := getArtifactDir(m)
		require.True(t, ok)
		assert.DirExists(t, got)

		// Should be under outDir/_artifacts/, NOT the mock's ArtifactDir.
		rel, err := filepath.Rel(outDir, got)
		require.NoError(t, err)
		assert.False(t, strings.HasPrefix(rel, ".."),
			"backfill dir %q should be inside output dir %q", got, outDir)
	})

	t.Run("artifacts_disabled_ignores_ArtifactDir", func(t *testing.T) {
		clearArtifactsFlag(t)

		m := &mockTWithArtifacts{
			mockT:       mockT{},
			artifactDir: t.TempDir(),
		}

		dir, ok := getArtifactDir(m)
		assert.False(t, ok)
		assert.Empty(t, dir, "should not return a dir when artifacts are disabled")
	})
}

// TestStartOldGo covers the Start() paths that are only reached on pre-go1.26:
// routing logs to an artifact file, reporting the artifact dir on failure/panic,
// and staying quiet on success.
func TestStartOldGo(t *testing.T) {
	var log = flume.New("TestStartOldGo")

	// Restore Verbose/Disabled after each sub-test (Start reads them).
	setup := func(t *testing.T) {
		t.Helper()

		oldV, oldD := Verbose(), Disabled()

		t.Cleanup(func() { SetVerbose(oldV); SetDisabled(oldD) })
	}

	t.Run("failure_logs_artifacts_path_to_tlog", func(t *testing.T) {
		setup(t)
		setArtifactsFlag(t)
		outDir := setOutputDir(t)

		m := &mockTOldGo{mockT: mockT{failed: true}, name: "TestStartOldGo/failure"}
		revert := Start(m)

		log.Info("failure log message", "color", "red")
		revert()

		// On failure the artifact directory path is reported via t.Log.
		assert.Contains(t, m.logs.String(), "=== ARTIFACTS")
		assert.Contains(t, m.logs.String(), "TestStartOldGo/failure")

		// The log message itself goes to the artifact file, not t.Log.
		assert.NotContains(t, m.logs.String(), "color=red")

		// Verify the artifact file exists and contains the log message.
		logFile := findArtifactLogRecursive(t, outDir)
		data, err := os.ReadFile(logFile)
		require.NoError(t, err)
		assert.Contains(t, string(data), "color=red")
	})

	t.Run("success_no_artifacts_created", func(t *testing.T) {
		setup(t)
		setArtifactsFlag(t)
		outDir := setOutputDir(t)

		m := &mockTOldGo{name: "TestStartOldGo/success"}
		revert := Start(m)

		log.Info("success message", "taste", "sweet")
		revert()

		// No "=== ARTIFACTS" line on a passing test.
		assert.NotContains(t, m.logs.String(), "=== ARTIFACTS")

		// Logs are discarded on success, not sent to t.Log.
		assert.NotContains(t, m.logs.String(), "taste=sweet")

		// No artifact file should be created on a passing test.
		assertNoArtifactLogs(t, outDir)
	})

	t.Run("verbose_success_saves_artifacts", func(t *testing.T) {
		setup(t)
		SetVerbose(true)
		setArtifactsFlag(t)
		outDir := setOutputDir(t)

		m := &mockTOldGo{name: "TestStartOldGo/verbose_success"}
		revert := Start(m)

		log.Info("verbose success message", "flavor", "umami")
		revert()

		// Verbose + artifacts: logs go to file, not t.Log.
		assert.NotContains(t, m.logs.String(), "flavor=umami")
		assert.Contains(t, m.logs.String(), "=== ARTIFACTS "+m.name)

		// Artifact file should exist and contain the log.
		logFile := findArtifactLogRecursive(t, outDir)
		data, err := os.ReadFile(logFile)
		require.NoError(t, err)
		assert.Contains(t, string(data), "flavor=umami")
	})

	t.Run("panic_logs_artifacts_path_to_tlog", func(t *testing.T) {
		setup(t)
		setArtifactsFlag(t)
		setOutputDir(t)

		m := &mockTOldGo{name: "TestStartOldGo/panic"}

		require.Panics(t, func() {
			defer Start(m)()

			log.Info("panic message", "shape", "circle")
			panic("boom")
		})

		// A panic should also trigger the "=== ARTIFACTS" diagnostic.
		assert.Contains(t, m.logs.String(), "=== ARTIFACTS")
		// The log message went to the file, not t.Log.
		assert.NotContains(t, m.logs.String(), "shape=circle")
	})

	t.Run("artifacts_path_written_to_output_when_available", func(t *testing.T) {
		setup(t)
		setArtifactsFlag(t)
		setOutputDir(t)

		m := &mockTOldGoWithOutput{mockTOldGo: mockTOldGo{
			mockT: mockT{failed: true},
			name:  "TestStartOldGo/output",
		}}
		revert := Start(m)

		log.Info("output path message", "key", "val")
		revert()

		// The "=== ARTIFACTS" diagnostic should go to Output(), not t.Log().
		assert.Contains(t, m.output.String(), "=== ARTIFACTS")
		assert.Contains(t, m.output.String(), "TestStartOldGo/output")
		assert.NotContains(t, m.logs.String(), "=== ARTIFACTS")
	})
}

func TestRelArtifactBase(t *testing.T) {
	tests := []struct {
		desc        string
		name        string
		buildInfo   *debug.BuildInfo
		buildInfoOk bool
		expected    string
	}{
		{
			desc:        "nice name",
			name:        "NiceTestName",
			buildInfoOk: false,
			expected:    filepath.Join("unknown", "NiceTestName"),
		},
		{
			desc:        "sub test name",
			name:        "TestName/sub_Test",
			buildInfoOk: false,
			expected:    filepath.Join("unknown", "TestName__sub_Test"),
		},
		{
			desc:        "long name",
			name:        strings.Repeat("a", 80),
			buildInfoOk: false,
			expected:    filepath.Join("unknown", strings.Repeat("a", 64-16)+fmt.Sprintf("%0x", hashName(strings.Repeat("a", 80)))),
		},
		{
			desc:        "name with invalid chars",
			name:        "Test_*?<>|Name",
			buildInfoOk: false,
			expected:    filepath.Join("unknown", "Test_Name"),
		},
		{
			desc: "test is in root package of module (no pkg path segment)",
			name: "NiceTestName",
			buildInfo: &debug.BuildInfo{
				Path: "github.com/example/mod.test",
				Main: debug.Module{Path: "github.com/example/mod"},
			},
			buildInfoOk: true,
			expected:    "NiceTestName",
		},
		{
			desc: "test is in package one level deep",
			name: "NiceTestName",
			buildInfo: &debug.BuildInfo{
				Path: "github.com/example/mod/pkg.test",
				Main: debug.Module{Path: "github.com/example/mod"},
			},
			buildInfoOk: true,
			expected:    filepath.Join("pkg", "NiceTestName"),
		},
		{
			desc: "test is in package multiple levels deep",
			name: "NiceTestName",
			buildInfo: &debug.BuildInfo{
				Path: "github.com/example/mod/pkg/subpkg/deep.test",
				Main: debug.Module{Path: "github.com/example/mod"},
			},
			buildInfoOk: true,
			expected:    filepath.Join("pkg", "subpkg", "deep", "NiceTestName"),
		},
		{
			desc:        "package is unknown",
			name:        "NiceTestName",
			buildInfo:   nil,
			buildInfoOk: false,
			expected:    filepath.Join("unknown", "NiceTestName"),
		},
		{
			desc:        "file name can't be made valid (should return empty)",
			name:        "..",
			buildInfoOk: false,
			expected:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			m := &mockTOldGo{name: tc.name}

			readBuildInfo := func() (*debug.BuildInfo, bool) {
				return tc.buildInfo, tc.buildInfoOk
			}
			result := relArtifactBase(m, readBuildInfo)

			assert.Equal(t, tc.expected, result)
		})
	}
}
