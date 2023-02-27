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
	"github.com/gemalto/flume"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
	"testing"
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
		flume.SetOut(ioutil.Discard)
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
}

func start(t testingTB) func() {
	var revert func()
	if Verbose {
		revert = flume.SetOut(flume.LogFuncWriter(t.Log, true))
	} else {
		// need to use a synchronized version of buf, since
		// logs may be written on other goroutines than this one,
		// and bytes.Buffer is not concurrent safe.
		buf := lockedBuf{
			buf: bytes.NewBuffer(nil),
		}
		revertOut := flume.SetOut(&buf)
		revert = func() {
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

	return revert
}
