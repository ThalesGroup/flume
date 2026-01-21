package flumetest

import (
	"bytes"
	"fmt"
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
