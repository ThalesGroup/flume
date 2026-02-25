package flume

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/ansel1/console-slog"
)

const (
	TextHandler      = "text"
	JSONHandler      = "json"
	TermHandler      = "term"
	TermColorHandler = "term-color"
	NoopHandler      = "noop"
)

var defaultConfigEnvVars = []string{"FLUME"}

// DefaultConfigEnvVars returns a copy of the default environment variable
// names that ConfigFromEnv will search.
func DefaultConfigEnvVars() []string {
	return append([]string(nil), defaultConfigEnvVars...)
}

// ConfigFromEnv enables logging and configures flume from environment variables.
// It should be called from main():
//
//	func main() {
//	    flume.ConfigFromEnv()
//	    ...
//	 }
//
// Calling this function always enables logging by switching the default handler
// from noop to a text handler writing to stdout at LevelInfo.
//
// If an environment variable is set, its value overrides the defaults.
// It searches envvars for the first environment variable that is set,
// and attempts to parse the value.
//
// If an environment variable with a value is found, but parsing
// fails, an error is returned.
//
// If envvars is empty, it defaults to DefaultConfigEnvVars().
func ConfigFromEnv(envvars ...string) error {
	if len(envvars) == 0 {
		envvars = defaultConfigEnvVars
	}

	var c HandlerOptions

	err := UnmarshalEnv(&c, envvars...)
	if err != nil {
		return err
	}

	Default().SetHandlerOptions(&c)

	return nil
}

// MustConfigFromEnv is like ConfigFromEnv, but panics on error.
// Like ConfigFromEnv, it always enables logging even when no environment variable is set.
func MustConfigFromEnv(envvars ...string) {
	err := ConfigFromEnv(envvars...)
	if err != nil {
		panic(err)
	}
}

// UnmarshalEnv reads handler options from an environment variable.  The first environment
// variable in the list with a non-empty value will be unmarshaled into the options arg.
//
// The first argument must not be nil.
//
// The value of the environment variable can be either json, or a levels string:
//
//	FLUME={"level":"inf"}            // json
//	FLUME=*=inf                      // levels string
//
// This is the full schema for the json encoding:
//
//	{
//	  "development": <bool>,
//	  "handler": <str>,       // looks up HandlerFn using LookupHandlerFn()
//	  "encoding": <str>,      // v1 alias for "handler"; if both set, "handler" wins
//	  "level": <str>,         // e.g. "INF", "INFO", "INF-1"
//	  "levels": <str or obj>, // either a levels string, or an object where the keys
//	                          // are logger names, and the values are levels (in the same
//	                          // format as the "level" property)
//	  "addSource": <bool>,
//	  "addCaller": <bool>,    // v1 alias for "addSource"; if both set, "addSource" wins
//	}
//
// Level strings are in the form:
//
//		 Levels    = Directive {"," Directive} .
//	  Directive = logger | "-" logger | logger "=" Level | "*" .
//	  Level     = LevelName [ "-" offset ] | int .
//	  LevelName = "DEBUG" | "DBG" | "INFO" | "INF" |
//	              "WARN" | "WRN" | "ERROR" | "ERR" |
//	              "ALL" | "OFF" | ""
//
// Where `logger` is the name of a logger.  "*" sets the default level.  LevelName is
// case-insensitive.
//
// Example:
//
//	*=INF,http,-sql,boot=DEBUG,authz=ERR,authn=INF+1,keys=4
//
// - sets default level to info
// - enables all log levels on the http logger
// - disables all logging from the sql logger
// - sets the boot logger to debug
// - sets the authz logger to ERR
// - sets the authn logger to level 1 (slog.LevelInfo + 1)
// - sets the keys logger to WARN (slog.LevelWarn == 4)
func UnmarshalEnv(o *HandlerOptions, envvars ...string) error {
	for _, v := range envvars {
		configString := os.Getenv(v)
		if configString == "" {
			continue
		}

		if strings.HasPrefix(configString, "{") {
			err := json.Unmarshal([]byte(configString), o)
			if err != nil {
				return fmt.Errorf("parsing configuration from environment variable %v: %w", v, err)
			}

			return nil
		}

		// parse the value like a levels string
		var levels Levels

		err := levels.UnmarshalText([]byte(configString))
		if err != nil {
			return fmt.Errorf("parsing levels string from environment variable %v: %w", v, err)
		}

		opts := HandlerOptions{}
		if defLvl, ok := levels["*"]; ok {
			opts.Level = defLvl

			delete(levels, "*")
		}

		opts.Levels = levels
		*o = opts
	}

	return nil
}

var handlerFns sync.Map

var initHandlerFnsOnce sync.Once

func resetBuiltInHandlerFns() {
	handlerFns = sync.Map{}

	registerHandlerFn(TextHandler, func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewTextHandler(w, opts)
	})
	registerHandlerFn(JSONHandler, func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewJSONHandler(w, opts)
	})
	registerHandlerFn(TermHandler, func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		termOpts := termHandlerOptions(opts)
		termOpts.NoColor = true

		return console.NewHandler(w, termOpts)
	})
	registerHandlerFn(TermColorHandler, func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return console.NewHandler(w, termHandlerOptions(opts))
	})
	registerHandlerFn(NoopHandler, func(_ string, _ io.Writer, _ *slog.HandlerOptions) slog.Handler {
		return noop
	})
}

func initHandlerFns() {
	initHandlerFnsOnce.Do(func() {
		resetBuiltInHandlerFns()
	})
}

// LookupHandlerFn looks for a handler registered with the given name.  Registered
// handlers are stored in an internal, package level map, which is initialized with some
// built-in handlers.  Handlers can be added or replaced via RegisterHandlerFn.
//
// Returns nil if name is not found.
//
// LookupHandlerFn is used when unmarshaling HandlerOptions from json, to resolve the
// "handler" property to a handler function.
func LookupHandlerFn(name string) HandlerFn {
	initHandlerFns()

	v, ok := handlerFns.Load(name)
	if !ok {
		return nil
	}

	fn := v.(HandlerFn) //nolint:forcetypeassert // if it's not a HandlerFn, we should panic

	return fn
}

// RegisterHandlerFn registers a handler with a name.  The handler can be looked up
// with LookupHandlerFn.  If a handler function was already registered with the given
// name, the old handler function is replaced.  Built-in handler functions can also
// be replaced in this manner.
func RegisterHandlerFn(name string, fn HandlerFn) {
	initHandlerFns()
	registerHandlerFn(name, fn)
}

// JSONHandlerFn is shorthand for LookupHandlerFn("json").  Will never be nil.
func JSONHandlerFn() HandlerFn {
	return LookupHandlerFn(JSONHandler)
}

// TextHandlerFn is shorthand for LookupHandlerFn("text").  Will never be nil.
func TextHandlerFn() HandlerFn {
	return LookupHandlerFn(TextHandler)
}

// TermHandlerFn is shorthand for LookupHandlerFn("term").  Will never be nil.
func TermHandlerFn() HandlerFn {
	return LookupHandlerFn(TermHandler)
}

// TermColorHandlerFn is shorthand for LookupHandlerFn("term-color").  Will never be nil.
func TermColorHandlerFn() HandlerFn {
	return LookupHandlerFn(TermColorHandler)
}

// NoopHandlerFn is shorthand for LookupHandlerFn("noop").  Will never be nil.
func NoopHandlerFn() HandlerFn {
	return LookupHandlerFn(NoopHandler)
}

func registerHandlerFn(name string, fn HandlerFn) {
	if fn == nil {
		panic(fmt.Sprintf("constructor for sink %q is nil", name))
	}

	if name == "" {
		panic("constructor registered with empty name")
	}

	handlerFns.Store(name, fn)
}

func termHandlerOptions(opts *slog.HandlerOptions) *console.HandlerOptions {
	// todo: it would be nice if consumers could tweak this, either programatically
	// or via configuration, but that would mean exposing the dependency on console-slog,
	// which is currently opaque.  For now, I want to keep this opaque.  That means, if
	// a consumer was a slightly different configuration of console-slog, they will
	// have to construct it from scratch themselves.
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}

	theme := console.NewDefaultTheme()
	theme.Name = "flume"
	theme.Source = console.ToANSICode(console.BrightBlack, console.Italic)
	theme.AttrKey = console.ToANSICode(console.Green, console.Faint)

	return &console.HandlerOptions{
		AddSource:          opts.AddSource,
		ReplaceAttr:        opts.ReplaceAttr,
		Level:              opts.Level,
		Theme:              theme,
		TimeFormat:         "15:04:05.000",
		HeaderFormat:       "%t %[" + LoggerKey + "]8h |%l| %m %a %(source){→ %s%}",
		TruncateSourcePath: 2,
	}
}
