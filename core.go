package flume

import (
	"fmt"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sync/atomic"
)

var _ Logger = (*Core)(nil)

type atomicLogger struct {
	innerLoggerPtr atomic.Value
}

func (af *atomicLogger) get() *zap.Logger {
	return af.innerLoggerPtr.Load().(*zap.Logger)
}

func (af *atomicLogger) set(logger *zap.Logger) {
	af.innerLoggerPtr.Store(logger)
}

// Core is the concrete implementation of Logger.  It has some additional
// lower-level methods which can be used by other logging packages which wrap
// flume, to build alternate logging interfaces.
type Core struct {
	*atomicLogger
	context []zap.Field
}

// Log is the core logging method, used by the convenience methods Debug(), Info(), and Error().
//
// Returns true if the log was actually logged.
func (l *Core) Log(lvl Level, template string, fmtArgs, context []interface{}) bool {
	raw := l.get()
	if !raw.Core().Enabled(zapcore.Level(lvl)) {
		return false
	}

	msg := template
	if msg == "" && len(fmtArgs) > 0 {
		msg = fmt.Sprint(fmtArgs...)
	} else if msg != "" && len(fmtArgs) > 0 {
		msg = fmt.Sprintf(template, fmtArgs...)
	}

	if ce := raw.Check(zapcore.Level(lvl), msg); ce != nil {
		ce.Write(append(l.context, l.sweetenFields(context)...)...)
		return true
	}

	return false
}

// IsEnabled returns true if the specified level is enabled.
func (l *Core) IsEnabled(lvl Level) bool {
	return l.get().Core().Enabled(zapcore.Level(lvl))
}

const (
	_oddNumberErrMsg    = "Ignored key without a value."
	_nonStringKeyErrMsg = "Ignored key-value pairs with non-string keys."
)

func (l *Core) sweetenFields(args []interface{}) []zap.Field {
	if len(args) == 0 {
		return nil
	}

	// Allocate enough space for the worst case; if users pass only structured
	// fields, we shouldn't penalize them with extra allocations.
	fields := make([]zap.Field, 0, len(args))
	var invalid invalidPairs

	for i := 0; i < len(args); {
		// This is a strongly-typed field. Consume it and move on.
		if f, ok := args[i].(zap.Field); ok {
			fields = append(fields, f)
			i++
			continue
		}

		if len(args) == 1 {
			// passed a bare arg with no key.  We'll handle this
			// as a special case
			fields = append(fields, zap.Any("", args[0]))
		}

		// Make sure this element isn't a dangling key.
		if i == len(args)-1 {
			l.get().Error(_oddNumberErrMsg, zap.Any("ignored", args[i]))
			break
		}

		// Consume this value and the next, treating them as a key-value pair. If the
		// key isn't a string, add this pair to the slice of invalid pairs.
		key, val := args[i], args[i+1]
		if keyStr, ok := key.(string); !ok {
			// Subsequent errors are likely, so allocate once up front.
			if cap(invalid) == 0 {
				invalid = make(invalidPairs, 0, len(args)/2)
			}
			invalid = append(invalid, invalidPair{i, key, val})
		} else {
			fields = append(fields, zap.Any(keyStr, val))
		}
		i += 2
	}

	// If we encountered any invalid key-value pairs, log an error.
	if len(invalid) > 0 {
		l.get().Error(_nonStringKeyErrMsg, zap.Array("invalid", invalid))
	}
	return fields
}

type invalidPair struct {
	position   int
	key, value interface{}
}

func (p invalidPair) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt64("position", int64(p.position))
	zap.Any("key", p.key).AddTo(enc)
	zap.Any("value", p.value).AddTo(enc)
	return nil
}

type invalidPairs []invalidPair

func (ps invalidPairs) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	var err error
	for i := range ps {
		err = multierr.Append(err, enc.AppendObject(ps[i]))
	}
	return err
}

// Debug logs at DBG level.  args should be alternative keys and values.  keys should be strings.
func (l *Core) Debug(msg string, args ...interface{}) {
	l.Log(DebugLevel, msg, nil, args)
}

// Info logs at INF level. args should be alternative keys and values.  keys should be strings.
func (l *Core) Info(msg string, args ...interface{}) {
	l.Log(InfoLevel, msg, nil, args)
}

// Error logs at ERR level.  args should be alternative keys and values.  keys should be strings.
func (l *Core) Error(msg string, args ...interface{}) {
	l.Log(ErrorLevel, msg, nil, args)
}

// IsDebug returns true if DBG level is enabled.
func (l *Core) IsDebug() bool {
	return l.IsEnabled(DebugLevel)
}

// With returns a new Logger with some context baked in.  All entries
// logged with the new logger will include this context.
//
// args should be alternative keys and values.  keys should be strings.
//
//     reqLogger := l.With("requestID", reqID)
//
func (l *Core) With(args ...interface{}) Logger {
	return l.WithArgs(args...)
}

// WithArgs is the same as With() but returns the concrete type.  Useful
// for other logging packages which wrap this one.
func (l *Core) WithArgs(args ...interface{}) *Core {
	l2 := l.clone()
	switch len(args) {
	case 0:
	default:
		l2.context = append(l2.context, l.sweetenFields(args)...)
	}
	return l2
}

func (l *Core) clone() *Core {
	l2 := *l
	l2.context = nil
	if len(l.context) > 0 {
		l2.context = append(l2.context, l.context...)
	}
	return &l2
}
