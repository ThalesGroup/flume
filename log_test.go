package flume

import (
	"bytes"
	"encoding/json"
	"github.com/ansel1/merry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"strconv"
	"testing"
	"time"
)

type Widget struct {
	Color    string
	Size     int
	Duration time.Duration
	Tags     []string
}

func TestNew(t *testing.T) {
	SetOut(LogFuncWriter(t.Log, true))
	ConfigString(`{"development":true,"level":"DBG"}`)
	//SetAddCaller(true)
	//encoderConfig := NewEncoderConfig()
	//encoderConfig.TimeKey = ""
	//pkgFactory.encoder = NewConsoleEncoder(encoderConfig)
	//pkgFactory.refreshLoggers()
	l := New("core")
	l.Debug("hi mom", "color", 1)
	l.Info("hi mom", "color", 2)
	l.Error("this log message\nhas newlines.")

	// samples with a single value and blank key
	l.Info("your favorite color", "blue")
	l.Info("url used", "http://test.com")
	l.Info("temp of main unit", "16")

	l.Info("this log message has multi-line fields", "logs", "hi\nthere\nmister rogers")
	//
	//ConfigString("*")
	//
	//l.Debug("hi mom", "color", 3)
	l2 := l.With("size", "red")
	l2.Info("curried", "age", 5)
	l.Info("hi mom", "color", 4)
	//
	l.Info("asdf", "widget", Widget{
		Color:    "red",
		Size:     4,
		Duration: 3 * time.Minute,
		Tags:     []string{"high", "low"},
	}, "array", []string{"high", "low"})
	l.Info("asdf")
	l.Info("asdfg")
	l.Info("asdfgh")
	l.Info("asdfghi")
	l.Info("asdfghij")
	l.Info("asdfghijk")
	l.Info("asdfghijkl")
	l.Info("asdfghijklm")
	l.Info("asdfghijklmn")
	l.Info("asdfghijklmno")
	l.Info("asdfghijklmnop")
	l.Error("message with error", "error", merry.New("boom!"))
	//
	//l.Info("asdf2", "arraymarshaller", zapcore.ArrayMarshalerFunc(func(ae zapcore.ArrayEncoder) error {
	//	ae.AppendInt(4)
	//	ae.AppendString("hi there")
	//	ae.AppendObject(zapcore.ObjectMarshalerFunc(func(oe zapcore.ObjectEncoder) error {
	//		oe.AddBool("enabled", false)
	//		oe.AddString("color", "red")
	//		return nil
	//	}))
	//	ae.AppendReflected(Widget{
	//		Color:    "red",
	//		Size:     4,
	//		Duration: 3 * time.Minute,
	//		Tags:     []string{"high", "low"},
	//	})
	//	return nil
	//}))
	//
	//l.Info("asdf3", "objectmarshaller", zapcore.ObjectMarshalerFunc(func(oe zapcore.ObjectEncoder) error {
	//	oe.AddBool("enabled", false)
	//	oe.AddString("color", "red")
	//	return nil
	//}))
	//l.Info("this just has a single, unpaired value", "red")
	//
	//LevelsString("*")
	//l.Debug("This is a debug")
	//l.Error("This is an error")
	l.Info("hi mom", "color", 5, "bits", []byte("hello"))

	ConfigString(`{}`)
	//config := NewEncoderConfig()
	//config.LevelKey = "l"
	//SetEncoder(zapcore.NewJSONEncoder(zapcore.EncoderConfig(config)))
	//SetAddCaller(true)
	l.Info("hi mom", "color", 5, "bits", []byte("hello"))

}

func TestTwo(t *testing.T) {
	//f := NewFactory()
	f := pkgFactory
	config := NewEncoderConfig()
	config.LevelKey = "l"
	f.SetEncoder(zapcore.NewJSONEncoder(zapcore.EncoderConfig(*config)))
	f.SetAddCaller(true)
	f.SetDefaultLevel(InfoLevel)
	l := f.NewLogger("test")
	l.Info("hi mom", "color", 5)
}

func TestBinary(t *testing.T) {
	f := NewFactory()
	l := f.NewLogger("")
	f.SetDefaultLevel(DebugLevel)
	l.Info("binary", "bin", []byte("hello"))
	l.Info("bear binary", []byte("hello"))
}

func TestLevelJSON(t *testing.T) {
	// Level should marshal to and from JSON
	l := ErrorLevel
	out, err := json.Marshal(l)
	require.NoError(t, err)
	require.Equal(t, `"ERR"`, string(out))

	c := Config{}
	err = json.Unmarshal([]byte(`{"level":"DBG"}`), &c)
	require.NoError(t, err)
	require.Equal(t, DebugLevel, c.DefaultLevel)
}

func TestNewCore(t *testing.T) {

	c := NewCore("green")
	assert.NotNil(t, c)
	assert.Equal(t, pkgFactory.NewCore("green"), c)

	f := NewFactory()
	err := f.LevelsString("*=INF,http=DBG")
	require.NoError(t, err)

	c = f.NewCore("green")

	assert.False(t, c.IsEnabled(DebugLevel))
	assert.True(t, c.IsEnabled(InfoLevel))

	require.NoError(t, err)
	c = f.NewCore("http")
	assert.True(t, c.IsEnabled(DebugLevel))
	assert.True(t, c.IsEnabled(InfoLevel))

	buf := bytes.NewBuffer(nil)
	f.SetOut(buf)

	c.Log(InfoLevel, "color %v", []interface{}{"red"}, []interface{}{"size", 5})

	assert.Contains(t, buf.String(), "size:5")
	assert.Contains(t, buf.String(), "color red")
	assert.Contains(t, buf.String(), "level:INF")
	assert.Contains(t, buf.String(), "name:http")

}

func TestAddCaller(t *testing.T) {
	f := NewFactory()
	f.SetDefaultLevel(DebugLevel)
	f.SetAddCaller(true)

	c := f.NewCore("green")

	buf := bytes.NewBuffer(nil)
	f.SetOut(buf)

	c.Log(InfoLevel, "asdf", nil, nil)
	assert.Contains(t, buf.String(), "caller:flume/log_test.go:")

	buf.Reset()

	c.Debug("asdf")
	assert.Contains(t, buf.String(), "caller:flume/log_test.go:")

	buf.Reset()

	c.Info("asdf")
	assert.Contains(t, buf.String(), "caller:flume/log_test.go:")

	buf.Reset()

	c.Error("asdf")
	assert.Contains(t, buf.String(), "caller:flume/log_test.go:")

	buf.Reset()

	logSomething(c)
	assert.Contains(t, buf.String(), "caller:"+logSomethingFile+":"+strconv.Itoa(logSomethingLine))
}

func TestAddCallerSkip(t *testing.T) {
	f := NewFactory()
	f.SetDefaultLevel(DebugLevel)
	f.SetAddCaller(true)

	c := f.NewCore("green")

	buf := bytes.NewBuffer(nil)
	f.SetOut(buf)

	logSomething(c)
	assert.Contains(t, buf.String(), "caller:"+logSomethingFile+":"+strconv.Itoa(logSomethingLine))

	c = f.NewCore("green", AddCallerSkip(1))
	logSomething(c)
	assert.Contains(t, buf.String(), "caller:flume/log_test.go:")
}

func TestSingleArgument(t *testing.T) {
	f := NewFactory()
	f.SetDefaultLevel(DebugLevel)
	f.SetAddCaller(true)

	c := f.NewCore("green")

	buf := bytes.NewBuffer(nil)
	f.SetOut(buf)

	c.Info("green", "red")
	assert.NotContains(t, buf.String(), _oddNumberErrMsg)

	buf.Reset()

	c.Info("green", "color", "red", "yellow")
	assert.Contains(t, buf.String(), _oddNumberErrMsg)
}
