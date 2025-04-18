// Package flumetest configures flume to integrate with golang tests.
//
// Using SetDefaults() in your TestMain() or an init() function in your test code
// will enabled all log levels, but discard all log output.  This ensures all your logging
// paths are tested, but test logs are not cluttered.  An additional command-line flag
// can be used to re-enable logging output.
//
// Additionally, at the start of each test, the following will capture logs during the test
// and dump them to the t.Log() function if the test fails:
//
//	defer flumetest.Start(t)()
//
// The deferred function call resets the log output.
//
// This has the benefit of interleaving the application logs with your test logs, and leveraging
// the test packages behavior of only printing the logs if the test fails.
package flumetest

import (
	"bytes"
	"flag"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gemalto/flume"
)

var Verbose bool
var ConfigString string

//nolint:gochecknoinits
func init() {
	flag.BoolVar(&Verbose, "vv", false, "super verbose: Output all logs (warning: logs may be big).  -v flag must also be set.")
	flag.StringVar(&ConfigString, "log-config", "", "logging config: Overrides default log settings with configuration string.  Same format as flume.ConfigString(). ")

	Verbose, _ = strconv.ParseBool(os.Getenv("FLUME_TEST_VERBOSE"))
	ConfigString = os.Getenv("FLUME_TEST_CONFIG_STRING")
}

// SetDefaults sets default options on the package-level flume factory which are appropriate for tests.
// Enables all logging, turns on call site logging, but discards all logs.
//
// To enable logging *all* output to stdout, pass the `-vv` flag to `go test`.
// This will log at all levels, in all tests, which will be *very* verbose.  Use
// with caution:
//
//	$ go test -v -vv
//
// Log output can also be enabled by setting the FLUME_TEST_VERBOSE environment variable.
//
// Uses a colorized console encoder with abbreviated times.
func SetDefaults() error {
	if !Verbose {
		flume.SetOut(io.Discard)
	}

	if ConfigString != "" {
		return flume.ConfigString(ConfigString)
	}

	// Use defaults
	return flume.Configure(flume.Config{
		Development:  true,
		DefaultLevel: flume.DebugLevel,
	})
}

// MustSetDefaults calls SetDefaults, and panics on error.
func MustSetDefaults() {
	if err := SetDefaults(); err != nil {
		panic(err)
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
//	  defer flumetest.Start(t)()
//	  ...
//	}
//
// Be sure to call the returned function at the end of the test to reset the log
// output to its original setting.
func Start(t testing.TB) func() {
	// delegate to an inner method which is testable
	return start(t)
}

type lockedBuf struct {
	buf *bytes.Buffer
	sync.Mutex
}

func (l *lockedBuf) Write(p []byte) (n int, err error) {
	l.Lock()
	defer l.Unlock()
	return l.buf.Write(p)
}

func (l *lockedBuf) Len() int {
	l.Lock()
	defer l.Unlock()
	return l.buf.Len()
}

func (l *lockedBuf) String() string {
	l.Lock()
	defer l.Unlock()
	return l.buf.String()
}

type testingTB interface {
	Failed() bool
	Log(args ...interface{})
	Cleanup(func())
}

func start(t testingTB) func() {
	var revert func()
	if Verbose {
		revert = flume.SetOut(flume.LogFuncWriter(t.Log, true))
	} else {
		// need to use a synchronized version of buf, since
		// logs may be written on other goroutines than this one,
		// and bytes.Buffer is not concurrent safe.
		buf := &lockedBuf{
			buf: bytes.NewBuffer(nil),
		}
		revertOut := flume.SetOut(buf)

		// since we're calling this function via t.Cleanup *and* returning
		// the function so the caller can call it with defer, there is a good
		// chance it will be called twice.  I can't use sync.Once here, because
		// if recover() is called inside the Once func, it doesn't work.  recover()
		// must be called directly in the deferred function
		ran := atomic.Bool{}
		revert = func() {
			if !ran.CompareAndSwap(false, true) {
				return
			}
			revertOut()
			// make sure that if the test panics or fails, we dump the logs
			recovered := recover()
			if buf.Len() > 0 && (recovered != nil || t.Failed()) {
				t.Log(buf.String())
			}
			if recovered != nil {
				panic(recovered)
			}
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
	// Since that bug is expected to be fixed soon in go, flume v2 may only rely on
	// on t.Cleanup(), to make the API simpler, and live with the limitation until go fixes
	// it.
	return revert
}
