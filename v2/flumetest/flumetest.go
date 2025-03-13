// Package flumetest configures flume to integrate with golang tests.
//
// It offers two main features:
//   - Configuring the default flume handler with suggested settings for tests, with the ability to
//     customize the configuration via command-line flags or environment variables.
//   - Capturing logs during tests, and either forwarding them to the t.Log(), or buffering them, and
//     only forwarding them to t.Log() if the test fails.
//
// Using SetDefaults() in your TestMain() or an init() function in your test code
// will enabled all log levels, but discard all log output.  This ensures all your logging
// paths are tested, but test logs are not cluttered.
//
// At the start of each test, the following will capture logs during the test
// and either dump them to the t.Log() function if the test fails, or discard them if the test passes.
//
//	flumetest.Start(t)
//
// Calls to flumetest.Start() can be nested, and it is conventional to call Start() at the start of each
// subtest.
package flumetest

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync/atomic"

	"github.com/ThalesGroup/flume/v2"
)

var Disabled bool
var Verbose bool
var configString string

//nolint:gochecknoinits
func init() {
	Disabled, _ = strconv.ParseBool(os.Getenv("FLUME_TEST_DISABLE"))
	Verbose, _ = strconv.ParseBool(os.Getenv("FLUME_TEST_VERBOSE"))
	configString = os.Getenv("FLUME_TEST_CONFIG_STRING")
}

// RegisterFlags registers command line flag options related flume:
//
//	-disable-flumetest
//	-vv
//	-log-config
//
// These options may also be set via environment variables.
//
// If you wish to use these flags in your tests, you should call this in TestMain().
func RegisterFlags() {
	flag.BoolVar(&Disabled, "disable-flumetest", false, "Disables all flumetest features: logging will happen as normal")
	flag.BoolVar(&Verbose, "vv", false, "During tests, forwards all logs immediately to t.Log()")
	flag.StringVar(&configString, "log-config", "", "logging config: Overrides default log settings with configuration string")
}

// SetDefaults configures the default flume handler with suggested settings
// for tests. Enables all logging, turns on call site logging, but discards all logs.
//
// Pass the `-disable-flumetest` flag to `go test`, or set the `FLUME_TEST_DISABLE` environment variable,
// to disable flumetest features and log as normal.
//
// Pass the `-log-config` flag to `go test`, or set the `FLUME_TEST_CONFIG_STRING` environment variable,
// to configure the flume handler with a custom configuration.
//
// Example:
//
//	$ go test -v -vv -log-config='{"level":"DBG"}'
func SetDefaults() error {
	if Disabled {
		return nil
	}

	if configString != "" {
		var opts flume.HandlerOptions
		err := json.Unmarshal([]byte(configString), &opts)
		if err != nil {
			return fmt.Errorf("failed to unmarshal log-config: %w", err)
		}
		flume.Default().SetHandlerOptions(&opts)
	} else {
		flume.Default().SetHandlerOptions(TestDefaults())
	}

	flume.Default().SetOut(io.Discard)

	return nil
}

// MustSetDefaults calls SetDefaults, and panics on error.
func MustSetDefaults() {
	if err := SetDefaults(); err != nil {
		panic(err)
	}
}

func TestDefaults() *flume.HandlerOptions {
	return &flume.HandlerOptions{
		HandlerFn: flume.TermHandlerFn(),
		AddSource: true,
		Level:     flume.LevelAll,
	}
}

// Start captures all logs written during the test.  If the test succeeds, the
// captured logs are discard.  If the test fails, the captured logs are dumped
// to the t.Log() method.
//
// If the -vv flag, or FLUME_TEST_VERBOSE env var is set, logs are forwarded
// directly to t.Log() as they occur.
//
//	func TestSomething(t *testing.T) {
//	  flumetest.Start(t)
//	  ...
//	}
//
// Buffered logs are automatically flushed when the test ends.  This can be
// overridden by calling the returned function, which will flush the logs immediately.
// This may be useful to discard logs from setup code, then starting a new capture for the
// body of the test.
func Start(t testingTB) func() {
	if Disabled {
		// no op
		return func() {}
	}

	revertToSnapshot := Snapshot(flume.Default())
	if Verbose {
		t.Cleanup(revertToSnapshot)
		flume.Default().SetOut(flume.LogFuncWriter(t.Log, true))
		return revertToSnapshot
	}

	buf := bytes.NewBuffer(nil)
	flume.Default().SetOut(buf)

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
		// make sure that if the test panics or fails, we dump the logs
		recovered := recover()
		if buf.Len() > 0 && (recovered != nil || t.Failed()) {
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
	Log(args ...interface{})
	Cleanup(func())
}
