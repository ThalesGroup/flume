package flume

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ansel1/merry"
	"go.uber.org/zap/zapcore"
	"io"
	"strconv"
	"strings"
)

type (
	// Logger is the basic logging interface.  Construct instances of Logger with a Factory,
	// or with the package functions (which use a package level Factory).
	Logger interface {
		Debug(msg string, args ...interface{})
		Info(msg string, args ...interface{})
		Error(msg string, args ...interface{})

		IsDebug() bool
		IsInfo() bool

		// With creates a new Logger with some context already attached.  All
		// entries logged with the child logger will include this context.
		With(args ...interface{}) Logger
	}

	// DeprecatedLogger is fully compatible with the logxi.Logger interface.  It includes the
	// base Logger interface, then adds some additional methods to fill out the logxi interface.
	// These additional methods are deprecated.
	DeprecatedLogger interface {
		Logger

		// deprecated: probably never appropriate. If you want to die, log an error, then quit
		// this is mapped to panic
		Fatal(msg string, args ...interface{})

		// deprecated: use Error or Info instead
		Warn(msg string, args ...interface{})

		// deprecated: trace exists to support legacy logger impl, but now maps
		// to debug
		Trace(msg string, args ...interface{})
		// deprecated: trace exists to support legacy logger impl, but now maps
		// to debug
		IsTrace() bool
		// deprecated: see Warn()
		IsWarn() bool
	}

	// Level is a log level
	Level zapcore.Level
)

const (
	// AllLevel enables all logs
	AllLevel = Level(-127)
	// OffLevel disables all logs
	OffLevel = Level(127)
	// DebugLevel should be used for low-level, non-production logs.  Typically intended only for developers.
	DebugLevel = Level(zapcore.DebugLevel)
	// InfoLevel should be used for production level logs.  Typically intended for end-users and developers.
	InfoLevel = Level(zapcore.InfoLevel)
	// ErrorLevel should be used for errors.  Generally, this should be reserved for events which truly
	// need to be looked at by an admin, and might be reported to an error-tracking system.
	ErrorLevel = Level(zapcore.ErrorLevel)

	// PanicLevel should used for errors, and will immediately cause a panic after logging.
	// deprecated: use ErrorLevel.  flume prefers not having the logging package do panics or system exits.
	PanicLevel = Level(zapcore.PanicLevel)
	// WarnLevel can be used for important messages which might be critical to troubleshooting, but aren't
	// really errors.
	// deprecated: use InfoLevel.  InfoLevel logs shouldn't be so noisy that important stuff would be lost
	// in the noise.  And this level tempts developers to punt on considering whether a condition is
	// really an error, or a normal operating condition.
	WarnLevel = Level(zapcore.WarnLevel)
)

var pkgFactory = NewFactory()

// New creates a new Logger
func New(name string) Logger {
	return pkgFactory.NewLogger(name)
}

// NewDeprecated creates a new DeprecatedLogger logger.
//
// Recommended usage: start with this, then work on eliminating
// the usages of the deprecated methods, then change to the Logger interface.
func NewDeprecated(name string) DeprecatedLogger {
	return pkgFactory.NewDeprecatedLogger(name)
}

// ConfigString configures the package level Factory.  The
// string can either be a JSON-serialized Config object, or
// just a LevelsString (see Factory.LevelsString for format).
//
// Note: this will reconfigure the logging levels for all
// loggers.
func ConfigString(s string) error {
	if strings.HasPrefix(strings.TrimSpace(s), "{") {
		// it's json, treat it like a full config string
		cfg := Config{}
		err := json.Unmarshal([]byte(s), &cfg)
		if err != nil {
			return err
		}
		return Configure(cfg)
	}
	return pkgFactory.LevelsString(s)
}

// Configure configures the package level Factory from
// the settings in the Config object.  See Config for
// details.
//
// Note: this will reconfigure the logging levels for all
// loggers.
func Configure(cfg Config) error {
	return pkgFactory.Configure(cfg)
}

// SetOut sets the output writer for all logs produced by the default factory.
// Returns a function which sets the output writer back to the prior setting.
func SetOut(w io.Writer) func() {
	return pkgFactory.SetOut(w)
}

// SetDefaultLevel sets the default log level on the package-level Factory.
func SetDefaultLevel(l Level) {
	pkgFactory.SetDefaultLevel(l)
}

// SetLevel sets a log level for a named logger on the package-level Factory.
func SetLevel(name string, l Level) {
	pkgFactory.SetLevel(name, l)
}

// SetAddCaller enables/disables call site logging on the package-level Factory
func SetAddCaller(b bool) {
	pkgFactory.SetAddCaller(b)
}

// SetEncoder sets the encoder for the package-level Factory
func SetEncoder(e Encoder) {
	pkgFactory.SetEncoder(e)
}

// SetDevelopmentDefaults sets useful default settings on the package-level Factory
// which are appropriate for a development setting.  Default log level is
// set to INF, all loggers are reset to the default level, call site information
// is logged, and the encoder is a colorized, multi-line friendly console
// encoder with a simplified time stamp format.
func SetDevelopmentDefaults() error {
	return Configure(Config{
		Development: true,
	})
}

// String implements stringer and a few other interfaces.
func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "DBG"
	case InfoLevel:
		return "INF"
	case WarnLevel:
		return "WRN"
	case ErrorLevel:
		return "ERR"
	case PanicLevel:
		return "PAN"
	case AllLevel:
		return "ALL"
	case OffLevel:
		return "OFF"
	default:
		return fmt.Sprintf("Level(%d)", l)
	}
}

// MarshalText implements encoding.TextMarshaler
func (l Level) MarshalText() ([]byte, error) {
	return []byte(l.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (l *Level) UnmarshalText(text []byte) error {
	if l == nil {
		return merry.New("can't unmarshal a nil *Level")
	}
	if !l.unmarshalText(text) {
		return fmt.Errorf("unrecognized level: %q", text)
	}
	return nil
}

func (l *Level) unmarshalText(text []byte) bool {
	text = bytes.ToLower(text)
	switch string(text) {
	case "debug", "dbg":
		*l = DebugLevel
	case "info", "inf", "": // make the zero value useful
		*l = InfoLevel
	case "warn", "wrn":
		*l = WarnLevel
	case "error", "err":
		*l = ErrorLevel
	case "panic", "pan":
		*l = PanicLevel
	case "off":
		*l = OffLevel
	case "all":
		*l = AllLevel
	default:
		if i, err := strconv.Atoi(string(text)); err != nil {
			if i >= -127 && i <= 127 {
				*l = Level(i)
			} else {
				return false
			}
		}
		return false
	}
	return true
}

// Set implements flags.Value
func (l *Level) Set(s string) error {
	return l.UnmarshalText([]byte(s))
}

// Get implements flag.Getter
func (l *Level) Get() interface{} {
	return *l
}
