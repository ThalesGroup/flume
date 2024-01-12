package flumetest

import (
	"fmt"
	"github.com/gemalto/flume"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"sync"
	"testing"
)

func init() {
	MustSetDefaults()
}

type mockT struct {
	failed   bool
	logs     strings.Builder
	cleanups []func()
	sync.Mutex
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

func (m *mockT) Log(args ...interface{}) {
	m.Lock()
	defer m.Unlock()
	_, _ = fmt.Fprint(&m.logs, args...)
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
				defer start(tb)()

				log.Info("Hi", "color", "red")
			},
			failTest: false,
			expect:   "",
		},
		{
			name: "failed",
			testFunc: func(tb testingTB) {
				defer start(tb)()

				log.Info("Hi", "color", "red")
			},
			failTest: true,
			expect:   "color:red",
		},
		{
			name:     "panic",
			failTest: false,
			expect:   "color:red",
			testFunc: func(tb testingTB) {
				require.Panics(t, func() {
					defer start(tb)()

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
				cleanup := start(tb)

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
			expect:   "color:red",
			testFunc: func(tb testingTB) {
				oldVerbose := Verbose
				Verbose = true
				defer func() {
					Verbose = oldVerbose
				}()
				start(tb)

				log.Info("Hi", "color", "red")
			},
		},
		{
			name:     "cleanup_without_defer",
			failTest: true,
			expect:   "color:red",
			testFunc: func(tb testingTB) {
				start(tb)

				log.Info("Hi", "color", "red")
			},
		},
		{
			skip:     "this will fail until this golang issue is resolved: https://github.com/golang/go/issues/49929",
			name:     "cleanup_without_defer_panic",
			failTest: false,
			expect:   "color:red",
			testFunc: func(tb testingTB) {
				start(tb)

			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.skip != "" {
				t.Skip(test.skip)
			}

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

	// fakeTestRun := func(succeed bool) string {
	// 	m := mockT{
	// 		failed: !succeed,
	// 	}
	// 	finish := start(&m)
	//
	// 	log.Info("Hi", "color", "red")
	//
	// 	finish()
	// 	m.Lock()
	// 	defer m.Unlock()
	// 	return m.lastLog
	// }
	// assert.Empty(t, fakeTestRun(true), "should not have logged, because the test didn't fail")
	// assert.Contains(t, fakeTestRun(false), "color:red", "should have logged since test failed")

	// this test is meant to trigger the race detector if we're not synchronizing correctly on the message
	// buffer
	// m := mockT{
	// 	failed: true,
	// }
	// finish := start(&m)
	//
	// barrier := make(chan struct{}, 1)
	// go func() {
	// 	barrier <- struct{}{}
	// 	log.Info("logging on a different goroutine, for race detector")
	//
	// }()
	// <-barrier
	// finish()

}
