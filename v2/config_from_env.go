package flume

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/ansel1/console-slog"
)

const (
	TextHandler      = "text"
	JSONHandler      = "json"
	ConsoleHandler   = "console"
	TermHandler      = "term"
	TermColorHandler = "term-color"
	NoopHandler      = "noop"
)

// DefaultConfigEnvVars is a list of the environment variables
// that ConfigFromEnv will search by default.
var DefaultConfigEnvVars = []string{"FLUME"}

// ConfigFromEnv configures flume from environment variables.
// It should be called from main():
//
//	func main() {
//	    flume.ConfigFromEnv()
//	    ...
//	 }
//
// It searches envvars for the first environment
// variable that is set, and attempts to parse the value.
//
// If no environment variable is set, it silently does nothing.
//
// If an environment variable with a value is found, but parsing
// fails, an error is returned.
//
// If envvars is empty, it defaults to DefaultConfigEnvVars.
func ConfigFromEnv(envvars ...string) error {
	if len(envvars) == 0 {
		envvars = DefaultConfigEnvVars
	}
	var c HandlerOptions
	err := UnmarshalEnv(&c, envvars...)
	if err != nil {
		return err
	}

	Default().SetHandlerOptions(&c)

	return nil
}

func MustConfigFromEnv(envvars ...string) {
	err := ConfigFromEnv(envvars...)
	if err != nil {
		panic(err)
	}
}

func UnmarshalEnv(o *HandlerOptions, envvars ...string) error {
	for _, v := range envvars {
		if configString := os.Getenv(v); configString != "" {
			err := json.Unmarshal([]byte(configString), o)
			if err != nil {
				return fmt.Errorf("parsing configuration from environment variable %v: %w", v, err)
			}
			return nil
		}
	}
	return nil
}

var handlerFns sync.Map

var initHandlerFnsOnce sync.Once

func resetBuiltInHandlerFns() {
	handlerFns = sync.Map{}
	textHandlerFn := func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewTextHandler(w, opts)
	}
	handlerFns.Store(TextHandler, textHandlerFn)
	// for v1 compatibility, "console" is an alias for "text"
	handlerFns.Store(ConsoleHandler, textHandlerFn)
	handlerFns.Store(JSONHandler, func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewJSONHandler(w, opts)
	})
	handlerFns.Store(TermHandler, termHandlerFn(false))
	handlerFns.Store(TermColorHandler, termHandlerFn(true))
	handlerFns.Store(NoopHandler, func(_ string, _ io.Writer, _ *slog.HandlerOptions) slog.Handler {
		return noop
	})
}

func initHandlerFns() {
	initHandlerFnsOnce.Do(func() {
		resetBuiltInHandlerFns()
	})
}

func LookupHandlerFn(name string) func(string, io.Writer, *slog.HandlerOptions) slog.Handler {
	initHandlerFns()
	v, ok := handlerFns.Load(name)
	if !ok {
		return nil
	}
	return v.(func(string, io.Writer, *slog.HandlerOptions) slog.Handler) //nolint:forcetypeassert // if it's not a HandlerFn, we should panic
}

func RegisterHandlerFn(name string, fn func(string, io.Writer, *slog.HandlerOptions) slog.Handler) {
	initHandlerFns()
	if fn == nil {
		panic(fmt.Sprintf("constructor for sink %q is nil", name))
	}
	if name == "" {
		panic("constructor registered with empty name")
	}
	handlerFns.Store(name, fn)
}

func termHandlerFn(color bool) func(string, io.Writer, *slog.HandlerOptions) slog.Handler {
	return func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		if opts == nil {
			opts = &slog.HandlerOptions{}
		}
		return console.NewHandler(w, &console.HandlerOptions{
			NoColor:            !color,
			AddSource:          opts.AddSource,
			Theme:              console.NewDefaultTheme(),
			ReplaceAttr:        opts.ReplaceAttr,
			TimeFormat:         "15:04:05.000",
			HeaderFormat:       "%t %[" + LoggerKey + "]8h %l | %m",
			TruncateSourcePath: 2,
		})
	}
}
