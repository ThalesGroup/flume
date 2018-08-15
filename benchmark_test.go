package flume

import (
	"errors"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"testing"
	"time"
)

func BenchmarkDisabledWithoutFields(b *testing.B) {
	b.Logf("Logging at a disabled level without any structured context.")
	SetLevel("test", ErrorLevel)
	logger := New("test")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info(getMessage(0))
		}
	})
}

func BenchmarkDisabledAccumulatedContext(b *testing.B) {
	b.Logf("Logging at a disabled level with some accumulated context.")
	SetLevel("test", ErrorLevel)
	logger := New("test").With(fakeFields()...)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info(getMessage(0))
		}
	})
}

func BenchmarkDisabledAddingFields(b *testing.B) {
	b.Logf("Logging at a disabled level, adding context at each log site.")
	SetLevel("test", ErrorLevel)
	logger := New("test")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info(getMessage(0), fakeFields()...)
		}
	})
}

func BenchmarkWithoutFields(b *testing.B) {
	b.Logf("Logging without any structured context.")
	SetLevel("test", DebugLevel)
	SetOut(&zaptest.Discarder{})
	logger := New("test")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info(getMessage(0))
		}
	})
}

func getMessage(iter int) string {
	return _messages[iter%1000]
}

func fakeMessages(n int) []string {
	messages := make([]string, n)
	for i := range messages {
		messages[i] = fmt.Sprintf("Test logging, but use a somewhat realistic message length. (#%v)", i)
	}
	return messages
}

var _messages = fakeMessages(1000)
var errExample = errors.New("fail")

func fakeFields() []interface{} {
	return []interface{}{
		//"int", 1,
		//"int64",int64(2),
		//"float", 3.0,
		//"string", "four!",
		//"bool", true,
		//"time", time.Unix(0, 0),
		//"error", errExample,
		//"duration", time.Second,
		//"user-defined type", _jane,
		//"another string", "done!",
		zap.Int("int", 1),
		zap.Int64("int64", 2),
		zap.Float64("float", 3.0),
		zap.String("string", "four!"),
		zap.Bool("bool", true),
		zap.Time("time", time.Unix(0, 0)),
		zap.Error(errExample),
		zap.Duration("duration", time.Second),
		zap.Object("user-defined type", _jane),
		zap.String("another string", "done!"),
	}
}

type user struct {
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func (u user) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("name", u.Name)
	enc.AddString("email", u.Email)
	enc.AddInt64("created_at", u.CreatedAt.UnixNano())
	return nil
}

var _jane = user{
	Name:      "Jane Doe",
	Email:     "jane@test.com",
	CreatedAt: time.Date(1980, 1, 1, 12, 0, 0, 0, time.UTC),
}
