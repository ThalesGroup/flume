package flume

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestSweetenFields(t *testing.T) {

	// single value, instead of key/value pairs
	c := NewCore("asdf")
	fields := c.sweetenFields([]interface{}{1})

	assert.Equal(t, []zap.Field{zap.Int("", 1)}, fields)

	// if the bare value is an error, use the key "error"
	err := errors.New("blue")
	fields = c.sweetenFields([]interface{}{err})
	assert.Equal(t, []zap.Field{zap.NamedError("error", err)}, fields)
}

func TestCore_IsDebug(t *testing.T) {
	f := NewFactory()
	f.SetDefaultLevel(InfoLevel)

	l := f.NewLogger("asdf")
	assert.False(t, l.IsDebug())
	f.SetDefaultLevel(DebugLevel)
	assert.True(t, l.IsDebug())
	f.SetDefaultLevel(ErrorLevel)
	assert.False(t, l.IsDebug())
	f.SetLevel("asdf", DebugLevel)
	assert.True(t, l.IsDebug())
}

func TestCore_IsInfo(t *testing.T) {
	f := NewFactory()
	f.SetDefaultLevel(ErrorLevel)

	l := f.NewLogger("asdf")
	assert.False(t, l.IsInfo())
	f.SetDefaultLevel(InfoLevel)
	assert.True(t, l.IsInfo())
	f.SetDefaultLevel(ErrorLevel)
	assert.False(t, l.IsInfo())
	f.SetLevel("asdf", DebugLevel)
	assert.True(t, l.IsInfo())
}
func TestCore_ZapCore(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	f := NewFactory()
	f.SetOut(buf)
	f.SetDefaultLevel(DebugLevel)
	f.SetAddCaller(true)
	f.SetEncoder(zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		MessageKey: "msg",
		LevelKey:   "level",
		TimeKey:    "time",
		CallerKey:  "caller",
		EncodeTime: func(t time.Time, pae zapcore.PrimitiveArrayEncoder) {
			if t.IsZero() {
				pae.AppendString("zerotime")
			} else {
				pae.AppendString("atime")
			}
		},
		EncodeCaller: func(ec zapcore.EntryCaller, pae zapcore.PrimitiveArrayEncoder) {
			pae.AppendString("acaller: " + ec.File)
		},
	}))

	c := f.NewCore("test").WithArgs(zap.String("color", "red"))
	zc := c.ZapCore()
	assert.NotNil(t, zc)

	// zc should forward its calls to the same core that underlies the flume
	// core.  We can test this by comparing calls to zc to calls to the flume
	// core.
	c.Debug("debug", "letter", "d")
	c.Info("info", "letter", "i")
	c.Error("error", "letter", "e")
	want := buf.String()
	buf.Reset()
	zl := zap.New(zc, zap.AddCaller())
	// .With(zap.String("size", "large"))
	zl.Debug("debug", zap.String("letter", "d"))
	zl.Info("info", zap.String("letter", "i"))
	zl.Error("error", zap.String("letter", "e"))
	got := buf.String()
	assert.Equal(t, want, got)

}
