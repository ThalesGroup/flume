// Package flumetest configures flume to integrate with golang tests.  It buffers logs during tests
// and only dumps them to t.Log() if the test fails.
//
// At the start of each test, add:
//
//	flumetest.Start(t)
//
// Calls to Start() can be nested, and it is conventional to call Start() at the start of each subtest.
//
// Environment variables can be used to customize behavior:
//
//	FLUMETEST_DISABLE=true     // Makes Start() a no-op
//	FLUMETEST_VERBOSE=true     // Start() will forward each log message to t.Log() immediately
//	                           // aliased to FLUME_TEST_VERBOSE for backward compatibility with v1
//
// Command line flags:
//
//	-artifacts
//		Save logs to artifact files, rather than dumping them to t.Log().  Artifacts are
//		stored in folders under $outputdir/_artifacts/.
//
// Note: This is a native flag in go1.26, but we backfill for older versions of go.
// In older versions, because it is a backfilled flag, it must come after the package argument:
//
// pre go1.26:
//
//	go test -v -outputdir build . -artifacts
//
// go1.26+:
//
//	go test -v -artifacts -outputdir build
package flumetest

import (
	"bytes"
	"flag"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ThalesGroup/flume/v2"
)

var (
	disabledPtr *bool
	verbosePtr  *bool

	initializeOnce sync.Once
)

func Disabled() bool {
	initialize()
	return disabledPtr != nil && *disabledPtr
}

func SetDisabled(disabled bool) {
	*disabledPtr = disabled
}

func Verbose() bool {
	initialize()
	return verbosePtr != nil && *verbosePtr
}

func SetVerbose(verbose bool) {
	*verbosePtr = verbose
}

// do not read the environment in init().  Using init() to read the environment
// doesn't give consumers a chance to load .env files first, or otherwise set up
// the environment.
func initialize() {
	initializeOnce.Do(func() {
		// only read these from the env if they weren't already set by from the command
		// line args
		if disabledPtr == nil {
			var b bool
			if s, ok := os.LookupEnv("FLUMETEST_DISABLE"); ok {
				b, _ = strconv.ParseBool(s)
			} else {
				b, _ = strconv.ParseBool(os.Getenv("FLUME_TEST_DISABLE"))
			}

			disabledPtr = &b
		}

		if verbosePtr == nil {
			var b bool

			b, _ = strconv.ParseBool(os.Getenv("FLUMETEST_VERBOSE"))
			verbosePtr = &b
		}
	})
}

// RegisterFlags registers command line flag options related flume:
//
//	-disable-flumetest
//	-vv
//
// These options may also be set via environment variables.
//
// If you wish to use these flags in your tests, you should call this in TestMain().
func RegisterFlags() {
	disabledPtr = flag.Bool("disable-flumetest", false, "Disables all flumetest features: logging will happen as normal")
	verbosePtr = flag.Bool("vv", false, "During tests, forwards all logs immediately to t.Log()")
}

// Start captures all logs written during the test.  If the test succeeds, the
// captured logs are discard.  If the test fails, the captured logs are dumped
// to the t.Log() method.
//
// If Verbose is true, logs are forwarded directly to t.Log() as they occur.
// If Disable is true, Start does nothing.
//
//	func TestSomething(t *testing.T) {
//	  flumetest.Start(t)
//	  ...
//	}
//
// The return value is a function which flushes the buffer, either to t.Log() if
// the test is failing, or discarding them if the test is not failing.
// This function is called automatically when the test ends, but it's sometimes
// useful to flush ear logs from setup code, then starting a new buffer for the
// body of the test.
func Start(t testingTB) func() {
	if Disabled() {
		// no op
		return func() {}
	}

	revertToSnapshot := Snapshot(flume.Default())

	verbose := Verbose()
	artifacts := artifactsEnabled()

	if verbose && !artifacts {
		t.Cleanup(revertToSnapshot)
		flume.Default().SetOut(flume.LogFuncWriter(t.Log, true))

		return revertToSnapshot
	}

	var (
		mu  sync.Mutex
		buf = bytes.NewBuffer(nil)
	)

	flume.Default().SetOut(&syncWriter{w: buf, mu: &mu})

	// since we're calling this function via t.Cleanup *and* returning
	// the function so the caller can call it with defer, there is a good
	// chance it will be called twice.  I can't use sync.Once here, because
	// if recover() is called inside the Once func, it doesn't work.  recover()
	// must be called directly in the deferred function
	ran := atomic.Bool{}
	revert := func() {
		if !ran.CompareAndSwap(false, true) {
			return
		}

		revertToSnapshot()

		mu.Lock()
		defer mu.Unlock()

		// make sure that if the test panics, we re-panic after cleanup
		recovered := recover()

		failed := recovered != nil || t.Failed()

		// Save to artifact file on failure/panic, or always when verbose.
		// On success without verbose, discard logs (don't create artifact dir).
		saveArtifact := artifacts && (failed || verbose)

		if saveArtifact && buf.Len() > 0 {
			writeArtifact(t, buf.Bytes())
		} else if buf.Len() > 0 && failed {
			// no artifact file: dump to t.Log on failure
			t.Log(buf.String())
		}

		if recovered != nil {
			panic(recovered)
		}
	}

	t.Cleanup(revert)
	// Calling Cleanup() to revert these changes should be sufficient, but isn't due to
	// this bug: https://github.com/golang/go/issues/49929
	// Due to that issue, if the test panics:
	// 1. t.Failed() returns false inside the cleanup function
	// 2. the revert doesn't know the test failed
	// 3. the revert function doesn't flush its captured logs as it should when a test fails
	//
	// So we do both: call the revert function via t.Cleanup, as well as return a function
	// that the test can call via defer.  t.Cleanup ensures we as least return the state
	// of the system, even if the test itself doesn't call the revert cleanup function,
	// but returning the cleanup function as well means tests that *do* call it via defer
	// will correctly handle test panics.
	//
	// Even if that bug is fixed, having the option to flush the logs with defer is useful.
	// For example, if you want to discard logs from setup code, then capture logs for the
	// body of the test.
	return revert
}

func writeArtifact(t testingTB, data []byte) {
	artifactFile := openArtifactFile(t)
	if artifactFile == nil {
		return
	}
	defer artifactFile.Close()

	_, _ = artifactFile.Write(data)
}

// Snapshot returns a function which will revert the configuration
// of the given handler to its state at the time Snapshot() was called.
// The state includes the current output writer, and the handler opts.
// Useful in tests to temporarily change the state of the handler for the
// duration of the test, e.g.:
//
//	t.Cleanup(flumetest.Snapshot(flume.Default()))
//	// or...
//	defer flumetest.Snapshot(flume.Default())()
func Snapshot(h *flume.Handler) func() {
	w := h.Out()
	opts := h.HandlerOptions()

	return func() {
		h.SetOut(w)
		h.SetHandlerOptions(opts)
	}
}

type testingTB interface {
	Failed() bool
	Log(args ...any)
	Cleanup(func())
	Name() string
}

type syncWriter struct {
	w  io.Writer
	mu sync.Locker
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.w.Write(p) //nolint:wrapcheck
}
