package flumetest

import (
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	artifactDirsMu sync.Mutex
	artifactDirs   = make(map[testingTB]string)
)

func artifactLogFileName() string {
	return "flumetest_" + time.Now().Format("20060102T150405.000") + ".log"
}

type hasArtifactDir interface {
	ArtifactDir() string
}

// nativeArtifactsFlag reports whether Go 1.26's -test.artifacts flag is
// registered and set to true.
func nativeArtifactsFlag() bool {
	if f := flagSet.Lookup("test.artifacts"); f != nil {
		b, _ := strconv.ParseBool(f.Value.String())
		return b
	}

	return false
}

// getArtifactDir returns the artifact directory for t.
// When go1.26's native -test.artifacts flag is active it delegates to
// t.ArtifactDir(); otherwise it emulates the same behavior using flag
// inspection and os.MkdirTemp.
func getArtifactDir(t testingTB) (string, bool) {
	if !Artifacts() {
		return "", false
	}

	// go1.26+: if the native flag is also enabled, delegate to t.ArtifactDir()
	// so the go tool manages the directory lifecycle.
	if a, ok := t.(hasArtifactDir); ok && nativeArtifactsFlag() {
		return a.ArtifactDir(), true
	}

	artifactDirsMu.Lock()
	defer artifactDirsMu.Unlock()

	if dir, ok := artifactDirs[t]; ok {
		return dir, true
	}

	// Get output dir from -test.outputdir flag, default to "."
	outputDir := "."
	if f := flagSet.Lookup("test.outputdir"); f != nil && f.Value.String() != "" {
		outputDir = f.Value.String()
	}

	artifactDir := filepath.Join(outputDir, "_artifacts")

	artifactBase := filepath.Join(artifactDir, relArtifactBase(t, debug.ReadBuildInfo))
	if err := os.MkdirAll(artifactBase, 0o777); err != nil {
		return "", false
	}

	dir, err := os.MkdirTemp(artifactBase, "")
	if err != nil {
		return "", false
	}

	// t.Output() added in 1.25
	if tOutput, ok := t.(interface{ Output() io.Writer }); ok {
		fmt.Fprintln(tOutput.Output(), "=== ARTIFACTS", t.Name(), dir)
	} else {
		t.Log("=== ARTIFACTS", t.Name(), dir)
	}

	artifactDirs[t] = dir

	t.Cleanup(func() {
		artifactDirsMu.Lock()
		defer artifactDirsMu.Unlock()

		delete(artifactDirs, t)
	})

	return dir, true
}

func relArtifactBase(t testingTB, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	name := t.Name()

	// Sanitize name similarly to go1.26's makeArtifactDir
	const maxNameSize = 64

	safeName := strings.ReplaceAll(name, "/", "__")
	if len(safeName) > maxNameSize {
		h := fmt.Sprintf("%0x", hashName(name))
		safeName = safeName[:maxNameSize-len(h)] + h
	}

	pkg := "unknown"
	if info, ok := readBuildInfo(); ok && strings.HasSuffix(info.Path, ".test") {
		pkg = strings.TrimSuffix(info.Path, ".test")
		// trim module first, then leading slash.  if path == main.path
		// we want to trim everything
		pkg = strings.TrimPrefix(pkg, info.Main.Path)
		pkg = strings.TrimPrefix(pkg, "/")
	}

	base := safeName
	if pkg != "" {
		base = pkg + "/" + safeName
	}

	base = removeSymbolsExcept(base, "!#$%&()+,-.=@^_ { } ~ /")

	var err error

	base, err = filepath.Localize(base)
	if err != nil {
		// This name can't be safely converted into a local filepath.
		// Drop it and just use _artifacts/<random>.
		base = ""
	}

	return base
}

func removeSymbolsExcept(s, allowed string) string {
	mapper := func(r rune) rune {
		if unicode.IsLetter(r) ||
			unicode.IsNumber(r) ||
			strings.ContainsRune(allowed, r) {
			return r
		}

		return -1 // disallowed symbol
	}

	return strings.Map(mapper, s)
}

func hashName(name string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(name))

	return h.Sum64()
}

func openArtifactFile(t testingTB) *os.File {
	dir, ok := getArtifactDir(t)
	if !ok {
		return nil
	}

	f, err := os.Create(filepath.Join(dir, artifactLogFileName()))
	if err != nil {
		return nil
	}

	return f
}
