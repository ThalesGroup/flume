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
			// todo: need to add a branch here to handle when the environment variable is
			// set to a raw levels string, and isn't JSON
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
	registerHandlerFn(TextHandler, textHandlerFn)
	// for v1 compatibility, "console" is an alias for "text"
	registerHandlerFn(ConsoleHandler, textHandlerFn)
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

func LookupHandlerFn(name string) HandlerFn {
	initHandlerFns()
	v, ok := handlerFns.Load(name)
	if !ok {
		return nil
	}
	// fn := v.(func(string, io.Writer, *slog.HandlerOptions) slog.Handler) //nolint:forcetypeassert // if it's not a HandlerFn, we should panic
	fn := v.(HandlerFn) //nolint:forcetypeassert // if it's not a HandlerFn, we should panic
	return fn
}

func RegisterHandlerFn(name string, fn HandlerFn) {
	initHandlerFns()
	registerHandlerFn(name, fn)
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
