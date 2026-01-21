package bridge

import (
	"bytes"
	"log/slog"
	"testing"

	flumev2 "github.com/ThalesGroup/flume/v2"
	"github.com/ThalesGroup/flume/v2/flumetest"
	flumev1 "github.com/gemalto/flume"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetGlobalState resets all global state modified by the bridge functions
func resetGlobalState(
	origV1AddCaller bool,
	origV2Opts *flumev2.HandlerOptions,
	origSlogDefault *slog.Logger,
) {
	// Reset flumev1 state
	flumev1.DefaultFactory().SetNewCoreFn(nil)
	flumev1.SetAddCaller(origV1AddCaller)

	// Reset flumev2 state
	flumev2.Default().SetHandlerOptions(origV2Opts)

	// Reset slog default
	slog.SetDefault(origSlogDefault)
}

// saveGlobalState captures the current global state before running tests
func saveGlobalState() (
	origV1AddCaller bool,
	origV2Opts *flumev2.HandlerOptions,
	origSlogDefault *slog.Logger,
) {
	origV1AddCaller = flumev1.DefaultFactory().AddCaller()
	origV2Opts = flumev2.Default().HandlerOptions()
	origSlogDefault = slog.Default()
	return
}

// testConfig resets the global v1 and v2 logger configuration to a common baseline state,
// then calls flumev1 and flumev2 ConfigFromEnv() using the given config value.
// After the test completes, the original state of the global logger configuration is restored.
func testConfig(t *testing.T, config string) *bytes.Buffer {
	// reset state after test
	t.Cleanup(flumetest.Snapshot(flumev2.Default()))
	origV1AddCaller := flumev1.DefaultFactory().AddCaller()
	origSlogDefault := slog.Default()
	t.Cleanup(func() {
		flumev1.DefaultFactory().SetNewCoreFn(nil)
		flumev1.SetAddCaller(origV1AddCaller)
		slog.SetDefault(origSlogDefault)
	})

	t.Setenv("red", config)
	require.NoError(t, flumev2.ConfigFromEnv("red"))
	require.NoError(t, flumev1.ConfigFromEnv("red"))

	buf := bytes.NewBuffer(nil)
	flumev2.Default().SetOut(buf)
	t.Cleanup(flumev1.SetOut(buf))
	return buf
}

func testToV1(t *testing.T, buf *bytes.Buffer, v1Log flumev1.Logger, v2Log *slog.Logger) {
	err := ToV1()
	require.NoError(t, err)

	// Now confirm both messages are handled by v1
	buf.Reset()
	v1Log.Info("v1 message")
	v2Log.Info("v2 message")

	assert.Contains(t, buf.String(), "msg:v1 message")
	assert.Contains(t, buf.String(), "msg:v2 message")

	// Also confirm that slog's default logger is
	// handled by v1 now
	buf.Reset()
	slog.Info("slog message")

	assert.Contains(t, buf.String(), "msg:slog message")
}

func testToV2(t *testing.T, buf *bytes.Buffer, v1Log flumev1.Logger, v2Log *slog.Logger) {
	err := ToV2()
	require.NoError(t, err)

	// Now confirm both messages are handled by v1
	buf.Reset()
	v1Log.Info("v1 message")
	v2Log.Info("v2 message")

	assert.Contains(t, buf.String(), "msg=\"v1 message\"")
	assert.Contains(t, buf.String(), "msg=\"v2 message\"")

	// Also confirm that slog's default logger is
	// handled by v1 now
	buf.Reset()
	slog.Info("slog message")

	assert.Contains(t, buf.String(), "msg=\"slog message\"")
}

func TestToV1(t *testing.T) {
	// for this test, configure logging to use text formatting.  The
	// "encoding":"ltsv" value setting is native to v1, but is
	// also understood by v2 for backward compatibility (as an alias
	// for "handler":"text")
	buf := testConfig(t, `{"level":"inf","encoding":"ltsv"}`)

	v1Log := flumev1.New("v1test")
	v2Log := flumev2.New("v2test")

	// First, confirm both loggers are going to different handlers
	v1Log.Info("v1 message")
	v2Log.Info("v2 message")

	// v1 ltsv uses k:v, v2 text uses k=v
	assert.Contains(t, buf.String(), "msg:v1 message")
	assert.Contains(t, buf.String(), "msg=\"v2 message\"")

	testToV1(t, buf, v1Log, v2Log)

	// test changing it
	testToV2(t, buf, v1Log, v2Log)
}

func TestToV2(t *testing.T) {
	// for this test, configure logging to use text formatting.  The
	// "encoding":"ltsv" value setting is native to v1, but is
	// also understood by v2 for backward compatibility (as an alias
	// for "handler":"text")
	buf := testConfig(t, `{"level":"inf","encoding":"ltsv"}`)

	v1Log := flumev1.New("v1test")
	v2Log := flumev2.New("v2test")

	// First, confirm both loggers are going to different handlers
	v1Log.Info("v1 message")
	v2Log.Info("v2 message")

	// v1 ltsv uses k:v, v2 text uses k=v
	assert.Contains(t, buf.String(), "msg:v1 message")
	assert.Contains(t, buf.String(), "msg=\"v2 message\"")

	testToV2(t, buf, v1Log, v2Log)

	// test changing it
	testToV1(t, buf, v1Log, v2Log)
}
