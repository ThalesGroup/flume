package flumetest

import (
	"fmt"
	"github.com/gemalto/flume"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
)

func init() {
	MustSetDefaults()
}

type mockT struct {
	failed  bool
	lastLog string
	sync.Mutex
}

func (m *mockT) Failed() bool {
	m.Lock()
	defer m.Unlock()
	return m.failed
}

func (m *mockT) Log(args ...interface{}) {
	m.Lock()
	defer m.Unlock()
	m.lastLog = fmt.Sprint(args...)
}

func TestStart(t *testing.T) {
	var log = flume.New("TestStart")

	fakeTestRun := func(succeed bool) string {
		m := mockT{
			failed: !succeed,
		}
		finish := start(&m)

		log.Info("Hi", "color", "red")

		finish()
		m.Lock()
		defer m.Unlock()
		return m.lastLog
	}
	assert.Empty(t, fakeTestRun(true), "should not have logged, because the test didn't fail")
	assert.Contains(t, fakeTestRun(false), "color:red", "should have logged since test failed")

	// this test is meant to trigger the race detector if we're not synchronizing correctly on the message
	// buffer
	m := mockT{
		failed: true,
	}
	finish := start(&m)

	var wg sync.WaitGroup
	wg.Add(1)
	stop := make(chan struct{})
	go func() {
		wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				log.Info("logging on a different goroutine, for race detector")
			}
		}

	}()
	wg.Wait()
	finish()
	stop <- struct{}{}

}
