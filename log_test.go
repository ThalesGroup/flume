package flume

import (
	"encoding/json"
	"github.com/ansel1/merry"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
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
	//l.Warn("This is a much longer message\nwith new lines\n in it", "stack", "this is\n a value\n\twith new\nlines and tabs")
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
	f.SetEncoder(zapcore.NewJSONEncoder(zapcore.EncoderConfig(config)))
	f.SetAddCaller(true)
	f.SetDefaultLevel(InfoLevel)
	l := f.NewLogger("test")
	l.Info("hi mom", "color", 5)
}

func TestBinary(t *testing.T) {
	f := NewFactory()
	l := f.NewLogger("")
	f.SetDefaultLevel(AllLevel)
	l.Info("binary", "bin", []byte("hello"))
	l.Info("bear binary", []byte("hello"))
}

func TestLevelJSON(t *testing.T) {
	// Level should marshal to and from JSON
	l := ErrorLevel
	out, err := json.Marshal(l)
	require.NoError(t, err)
	require.Equal(t, `"ERR"`, string(out))

	err = json.Unmarshal([]byte(`"WRN"`), &l)
	require.NoError(t, err)
	require.Equal(t, WarnLevel, l)

	c := Config{}
	err = json.Unmarshal([]byte(`{"level":"DBG"}`), &c)
	require.NoError(t, err)
	require.Equal(t, DebugLevel, c.DefaultLevel)
}
