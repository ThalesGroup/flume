// Package flumetest configures flume to integrate with golang tests.
//
// - Snapshot(): save and restore flume's configuration
// - Start(): buffers logs during tests and dumps them to t.Log() if the test fails
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
//	FLUMETEST_VERBOSE=true     // Start() flushes captured logs at test end even on success.
//	                           // aliased to FLUME_TEST_VERBOSE for backward compatibility with v1
//	FLUMETEST_ARTIFACTS=true   // Save logs to artifact files.  When unset, go1.26's native
//	                           // -test.artifacts flag is used as a fallback.  When set, it
//	                           // takes precedence over the native flag.
//	FLUMETEST_BUFFER_LIMIT=<size>  // Maximum bytes retained per test capture buffer.
//	                               // Accepts a bare integer (bytes) or a human-friendly suffix:
//	                               // B, K/KB/KiB, M/MB/MiB, G/GB/GiB (all binary; KB == KiB == 1024).
//	                               // Default: 1MiB (1048576 bytes). Set to 0 for unlimited.
//	                               // On invalid value, a warning is written to stderr and the
//	                               // default is used.
package flumetest

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ThalesGroup/flume/v2"
)

const defaultBufferLimit = 1 << 20 // 1 MiB

var (
	disabledPtr    *bool
	verbosePtr     *bool
	artifactsPtr   *bool
	bufferLimitPtr *int

	initializeOnce = &sync.Once{}

	// flagSet is the flag set used for registering and looking up command line flags.
	// Defaults to flag.CommandLine; tests can replace it with a fresh *flag.FlagSet.
	flagSet = flag.CommandLine
)

// BufferLimit returns the maximum number of bytes retained per test capture buffer.
// A value of 0 means unlimited.
func BufferLimit() int {
	initialize()
	return *bufferLimitPtr
}

// SetBufferLimit sets the maximum number of bytes retained per test capture buffer.
// A value of 0 means unlimited. This takes effect for newly started tests; tests
// already in progress retain the cap that was in effect at Subscribe time.
func SetBufferLimit(n int) {
	initialize()

	*bufferLimitPtr = n
}

// Sentinel errors for parseByteSize.
var (
	errByteSizeEmpty    = errors.New("empty string")
	errByteSizeFormat   = errors.New("must be a non-negative integer with optional suffix (B, K, KB, KiB, M, MB, MiB, G, GB, GiB)")
	errByteSizeOverflow = errors.New("value overflows int")
)

// byteSizeRe matches an optional integer followed by an optional suffix.
var byteSizeRe = regexp.MustCompile(`^(\d+)\s*([a-z]*)$`)

// parseByteSize parses a human-friendly byte size string into an integer number of bytes.
// Supported suffixes (case-insensitive): B, K/KB/KiB, M/MB/MiB, G/GB/GiB.
// KB and KiB are treated identically as 1024 (likewise MB/MiB, GB/GiB).
// A bare integer means bytes. Returns an error for unknown suffixes, negatives,
// fractions, overflow, or empty input.
func parseByteSize(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errByteSizeEmpty
	}

	lower := strings.ToLower(s)

	m := byteSizeRe.FindStringSubmatch(lower)
	if m == nil {
		return 0, fmt.Errorf("invalid byte size %q: %w", s, errByteSizeFormat)
	}

	n, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q: %w", s, err)
	}

	var multiplier uint64

	switch m[2] {
	case "", "b":
		multiplier = 1
	case "k", "kb", "kib":
		multiplier = 1024
	case "m", "mb", "mib":
		multiplier = 1024 * 1024
	case "g", "gb", "gib":
		multiplier = 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("invalid byte size %q: unknown suffix %q: %w", s, m[2], errByteSizeFormat)
	}

	result := n * multiplier
	if result > math.MaxInt {
		return 0, fmt.Errorf("invalid byte size %q: %w", s, errByteSizeOverflow)
	}

	return int(result), nil
}

func Disabled() bool {
	initialize()
	return disabledPtr != nil && *disabledPtr
}

func SetDisabled(disabled bool) {
	initialize()

	*disabledPtr = disabled
}

func Verbose() bool {
	initialize()
	return verbosePtr != nil && *verbosePtr
}

func SetVerbose(verbose bool) {
	initialize()

	*verbosePtr = verbose
}

func Artifacts() bool {
	initialize()
	return artifactsPtr != nil && *artifactsPtr
}

func SetArtifacts(artifacts bool) {
	initialize()

	*artifactsPtr = artifacts
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

		if artifactsPtr == nil {
			var b bool

			// FLUMETEST_ARTIFACTS takes precedence over the native flag.
			// When unset, fall back to go1.26's -test.artifacts flag.
			if s, ok := os.LookupEnv("FLUMETEST_ARTIFACTS"); ok {
				b, _ = strconv.ParseBool(s)
			} else {
				b = nativeArtifactsFlag()
			}

			artifactsPtr = &b
		}

		if bufferLimitPtr == nil {
			limit := defaultBufferLimit

			if raw := os.Getenv("FLUMETEST_BUFFER_LIMIT"); raw != "" {
				v, err := parseByteSize(raw)
				if err != nil {
					fmt.Fprintf(os.Stderr, "flumetest: invalid FLUMETEST_BUFFER_LIMIT=%q: %v; using default %d\n", raw, err, defaultBufferLimit)
				} else {
					limit = v
				}
			}

			bufferLimitPtr = &limit
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
	disabledPtr = flagSet.Bool("disable-flumetest", false, "Disables all flumetest features: logging will happen as normal")
	verbosePtr = flagSet.Bool("vv", false, "Flush captured logs at test end even on success")
}

// Start captures all logs written during the test.  If the test succeeds, the
// captured logs are discarded.  If the test fails, the captured logs are dumped
// to the t.Log() method.
//
// In parallel tests, Start captures all flume output emitted while the test is
// active. When tests overlap, each active test receives the full shared output
// for the overlap window. Logs from other concurrently running tests may appear
// in the captured output.
//
// Nested Start() calls (e.g. in subtests) take over from ancestor captures:
// during a subtest's window, the parent's capture is suspended and the child's
// is active. The parent resumes when the subtest ends.
//
// If Start is called more than once for the same test (same t.Name()), logs captured
// so far are flushed or discarded, and capturing resumes.
//
// If Verbose is true, captured logs are flushed at test end even on success.
// (Previously: live-streamed to t.Log during execution. That behavior has changed.)
// If Disable is true, Start does nothing.
//
//	func TestSomething(t *testing.T) {
//	  flumetest.Start(t)
//	  ...
//	}
//
// The return value is a function which terminates capturing and flushes the captured
// logs.  This function is called automatically when the test ends, but may be used
// to end one capture and start another mid-test.
//
// If Start() is called again before the first Start()'s cleanup function is called,
// the second Start()'s cleanup function is a no-op.
func Start(t testingTB) func() {
	if Disabled() {
		// no op
		return func() {}
	}

	ensureMuxInstalled()

	verbose := Verbose()
	artifacts := Artifacts()

	id, sub, oldSub, replaced := globalMux.Subscribe(t.Name())

	// If replaced is true, an old subscriber was returned. We must flush its buffer
	// now (not when its cleanup runs) because the old subscriber is being retired.
	if replaced && oldSub != nil {
		flushBuffer(oldSub, t, artifacts, verbose, t.Failed())

		return func() {}
	}

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

		// Unsubscribe first so no new writes land in sub after this point.
		// Idempotent: no-op if already removed by same-test rollover.
		globalMux.Unsubscribe(id)

		// make sure that if the test panics, we re-panic after cleanup
		recovered := recover()

		// Flush this subscriber's buffer.
		// Use recovered != nil || t.Failed() to match original panic-handling behavior.
		flushBuffer(sub, t, artifacts, verbose, recovered != nil || t.Failed())

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
	// that the test can call via defer.  t.Cleanup ensures we at least clean up state,
	// even if the test itself doesn't call the revert cleanup function,
	// but returning the cleanup function as well means tests that *do* call it via defer
	// will correctly handle test panics.
	//
	// Even if that bug is fixed, having the option to flush the logs with defer is useful.
	// For example, if you want to discard logs from setup code, then capture logs for the
	// body of the test.
	return revert
}

// Capture all logs written during the test.  If the test succeeds, the
// captured logs are discarded.  If the test fails, the captured logs are dumped
// to the t.Log() method.
//
// In parallel tests, captures all flume output emitted while the test is
// active. When tests overlap, each active test receives the full shared output
// for the overlap window. Logs from other concurrently running tests may appear
// in the captured output.
//
// Nested Capture() calls (e.g. in subtests) take over from ancestor captures:
// during a subtest's window, the parent's capture is suspended and the child's
// is active. The parent resumes when the subtest ends.
//
// If Capture() is called more than once for the same test (same t.Name()), logs captured
// so far are flushed or discarded, and capturing resumes.
//
// If Verbose is true, captured logs are flushed at test end even on success.
// (Previously: live-streamed to t.Log during execution. That behavior has changed.)
// If Disable is true, Capture does nothing.
//
//	func TestSomething(t *testing.T) {
//	  flumetest.Capture(t)
//	  ...
//	}
//
// As a convenience, returns flumetest.Context(t).
//
// Note: if the test panics, logs may be lost due to a golang bug:
// https://github.com/golang/go/issues/49929
// Until this bug is fixed, the recommendation is to first fix or avoid the panic,
// then re-run the tests.  Generally, it is an anti-pattern for tests to panic anyway.
func Capture(t testingTB) context.Context {
	Start(t)
	return addTestNameToLogs(t, nil)
}

// Context returns a context, derived from t.Context(), with the test name injected as a "testname"
// attribute. This also installs a middleware in default slog handler which adds "testname" to log
// records.
//
//	ctx := flumetest.Context(t)
//	log.DebugCtx(ctx, "my message")
//
// This is useful for correlating logs when running parallel tests.
func Context(t testingTB) context.Context {
	return addTestNameToLogs(t, nil)
}

func writeArtifact(t testingTB, src io.WriterTo) {
	artifactFile := openArtifactFile(t)
	if artifactFile == nil {
		return
	}
	defer artifactFile.Close()

	_, _ = src.WriteTo(artifactFile)
}

// flushBuffer flushes the subscriber's captured logs to t.Log or artifact file,
// or discards them if the test passed and neither verbose nor artifacts require flushing.
// This is used both by the normal cleanup path and by the same-test rollover path
// when a second Start() replaces an existing subscriber.
// The failed parameter should be true if the test has failed OR if a panic occurred.
func flushBuffer(sub *subscriber, t testingTB, artifacts bool, verbose bool, failed bool) {
	defer sub.Free()
	// Save to artifact file on failure, or always when verbose.
	// On success without verbose, discard logs (don't create artifact dir).
	saveArtifact := artifacts && (failed || verbose)

	flushed := false

	if saveArtifact && sub.Len() > 0 {
		writeArtifact(t, sub)

		flushed = true
	} else if sub.Len() > 0 && (failed || verbose) {
		// no artifact file: dump to t.Log on failure or when verbose
		t.Log(sub.String())

		flushed = true
	}

	if flushed && sub.Dropped() > 0 {
		t.Log("flumetest: log buffer truncated;", sub.Dropped(), "bytes dropped (FLUMETEST_BUFFER_LIMIT=", BufferLimit(), ")")
	}
}

// Snapshot returns a function which will revert the configuration
// of the given handler to its state at the time Snapshot() was called.
// The state includes the current output writer, and the handler opts.
//
// Example:
//
//	t.Cleanup(flumetest.Snapshot(flume.Default()))
//	// or...
//	defer flumetest.Snapshot(flume.Default())()
func Snapshot(h *flume.Handler) func() {
	if h == nil {
		h = flume.Default()
	}

	w := h.Out()
	opts := h.HandlerOptions()

	return func() {
		h.SetOut(w)
		h.SetHandlerOptions(opts)
	}
}

var installTestNameMiddlewareOnce sync.Once

func addTestNameToLogs(t testingTB, h *flume.Handler) context.Context {
	if h == nil {
		h = flume.Default()
	}

	// lazy install the middleware
	installTestNameMiddlewareOnce.Do(func() {
		opts := h.HandlerOptions()
		opts.Middleware = append(opts.Middleware, flume.ContextAttrsMiddleware())
		h.SetHandlerOptions(opts)
	})

	return flume.ContextWithAttrs(t.Context(), slog.String("testname", t.Name()))
}

type testingTB interface {
	Failed() bool
	Log(args ...any)
	Cleanup(func())
	Name() string
	Context() context.Context
}
