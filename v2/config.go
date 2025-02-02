package flume

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/ansel1/console-slog"
	"github.com/ansel1/merry/v2"
)

type SinkConstructor func(Config) (slog.Handler, error)

var sinkConstructors sync.Map

const (
	TextSink      = "text"
	JSONSink      = "json"
	ConsoleSink   = "console"
	TermSink      = "term"
	TermColorSink = "term-color"
)

func init() { //nolint:gochecknoinits
	resetBuiltInSinks()
}

func resetBuiltInSinks() {
	sinkConstructors = sync.Map{}
	RegisterSinkConstructor(TextSink, textSinkConstructor)
	// for v1 compatibility, "console" is an alias for "text"
	RegisterSinkConstructor(ConsoleSink, textSinkConstructor)
	RegisterSinkConstructor(JSONSink, jsonSinkConstructor)
	RegisterSinkConstructor(TermSink, termSinkConstructor(false))
	RegisterSinkConstructor(TermColorSink, termSinkConstructor(true))
}

func RegisterSinkConstructor(name string, constructor SinkConstructor) {
	if constructor == nil {
		panic(fmt.Sprintf("constructor for sink %q is nil", name))
	}
	if name == "" {
		panic("constructor registered with empty name")
	}
	sinkConstructors.Store(name, constructor)
}

func textSinkConstructor(c Config) (slog.Handler, error) {
	opts := slog.HandlerOptions{
		AddSource:   c.AddSource,
		ReplaceAttr: ChainReplaceAttrs(c.ReplaceAttrs...),
	}
	return slog.NewTextHandler(c.Out, &opts), nil
}

func jsonSinkConstructor(c Config) (slog.Handler, error) {
	opts := slog.HandlerOptions{
		AddSource:   c.AddSource,
		ReplaceAttr: ChainReplaceAttrs(c.ReplaceAttrs...),
	}
	return slog.NewJSONHandler(c.Out, &opts), nil
}

func termSinkConstructor(color bool) SinkConstructor {
	return func(c Config) (slog.Handler, error) {
		return console.NewHandler(c.Out, &console.HandlerOptions{
			NoColor:     !color,
			AddSource:   c.AddSource,
			Theme:       console.NewDefaultTheme(),
			ReplaceAttr: ChainReplaceAttrs(c.ReplaceAttrs...),
			TimeFormat:  "15:04:05.000",
			Headers:     []string{LoggerKey},
			HeaderWidth: 13,
		}), nil
	}
}

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
	var c Config
	err := c.UnmarshalEnv(envvars...)
	if err != nil {
		return err
	}

	return c.Configure(Default())
}

func MustConfigFromEnv(envvars ...string) {
	err := ConfigFromEnv(envvars...)
	if err != nil {
		panic(err)
	}
}

func (c *Config) UnmarshalEnv(envvars ...string) error {
	for _, v := range envvars {
		if configString := os.Getenv(v); configString != "" {
			err := json.Unmarshal([]byte(configString), c)
			if err != nil {
				return fmt.Errorf("parsing configuration from environment variable %v: %w", v, err)
			}
			return nil
		}
	}
	return nil
}

type Config struct {
	// DefaultLevel is the default log level for all loggers not
	// otherwise configured by Levels.  Defaults to Info.
	DefaultLevel slog.Level `json:"defaultLevel,omitempty"`
	// Encoding sets the logger's encoding. Valid values are "json",
	// "text", "ltsv", "term", and "term-color".
	//
	// For compatibility with flume v1, "console" is also accepted, and
	// is an alias for "text"
	DefaultSink string `json:"defaultSink,omitempty"`
	// Levels configures log levels for particular named loggers.
	Levels Levels `json:"levels,omitempty"`
	// AddSource causes the handler to compute the source code position
	// of the log statement and add a SourceKey attribute to the output.
	// Defaults to true when the Development flag is set, false otherwise.
	AddSource    bool `json:"addSource,omitempty"`
	Out          io.Writer
	ReplaceAttrs []func([]string, slog.Attr) slog.Attr
}

func DevDefaults() Config {
	return Config{
		DefaultSink: TermColorSink,
		AddSource:   true,
	}
}

const (
	dbgAbbrev = "DBG"
	infAbbrev = "INF"
	wrnAbbrev = "WRN"
	errAbbrev = "ERR"
)

var levelAbbreviations = map[slog.Level]string{
	slog.LevelDebug: "DBG",
	slog.LevelInfo:  "INF",
	slog.LevelWarn:  "WRN",
	slog.LevelError: "ERR",
}

func parseLevel(v any) (slog.Level, error) {
	var s string

	switch v := v.(type) {
	case nil:
		return slog.LevelInfo, nil
	case string:
		s = v
	case float64:
		// allow numbers to be used as level values
		return slog.Level(v), nil
	case int:
		// allow raw integer values for level
		return slog.Level(v), nil
	case bool:
		if v {
			return LevelAll, nil
		}
		return LevelOff, nil
	default:
		return 0, errors.New("levels must be a string or int value")
	}

	// allow raw integer values for level
	if i, err := strconv.Atoi(s); err == nil {
		return slog.Level(i), nil
	}

	s = strings.ToUpper(s)

	// some special values
	switch s {
	case "ALL":
		return LevelAll, nil
	case "":
		return slog.LevelInfo, nil // default
	case "OFF":
		return LevelOff, nil
	}

	// convert abbreviations to full length values
	// also support the level offset convention slog supports, i.e. WRN+3 = WARNING+3 = 4+3 = 7
	if len(s) == 3 || strings.IndexAny(s, "+-") == 3 {
		for level, abbr := range levelAbbreviations {
			if strings.HasPrefix(s, abbr) {
				s = level.String() + s[3:]
			}
		}
	}

	var l slog.Level
	err := l.UnmarshalText([]byte(s))

	return l, merry.Prependf(err, "invalid log level '%v'", v)
}

type config Config

func (c *Config) UnmarshalJSON(bytes []byte) error {
	// first unmarshal the development defaults flag
	s1 := struct {
		Development bool `json:"development"`
	}{}

	if err := json.Unmarshal(bytes, &s1); err != nil {
		return merry.Prepend(err, "invalid json config")
	}

	s := struct {
		config
		Sink         string `json:"sink"`
		DefaultLevel any    `json:"defaultLevel"`
		Level        any    `json:"level"`
		Levels       any    `json:"levels"`
		AddSource    *bool  `json:"addSource"`
		AddCaller    *bool  `json:"addCaller"`
		Encoding     string `json:"encoding"`
		Out          string `json:"out"`
	}{}

	if s1.Development {
		s.config = config(DevDefaults())
	}

	if err := json.Unmarshal(bytes, &s); err != nil {
		return merry.Prependf(err, "invalid json config")
	}

	// for backward compat with v1, allow "level" as an alias
	// for "defaultLevel"
	if s.DefaultLevel == nil {
		s.DefaultLevel = s.Level
	}

	level, err := parseLevel(s.DefaultLevel)
	if err != nil {
		return err
	}
	s.config.DefaultLevel = level

	switch lvls := s.Levels.(type) {
	case nil:
	case string:
		err := s.config.Levels.UnmarshalText([]byte(lvls))
		if err != nil {
			return err
		}
	case map[string]any:
		if len(lvls) > 0 {
			s.config.Levels = Levels{}
			for n, l := range lvls {
				lvl, err := parseLevel(l)
				if err != nil {
					return err
				}
				s.config.Levels[n] = lvl
			}
		}
	default:
		return merry.Errorf("invalid levels value: %v", s.Levels)
	}

	// for backward compat with v1, allow "addCaller" as
	// an alias for "addSource"
	if s.AddSource == nil {
		s.AddSource = s.AddCaller
	}

	if s.AddSource != nil {
		s.config.AddSource = *s.AddSource
	}

	// allow "sink" as alias for "defaultSink"
	if s.DefaultSink == "" {
		s.DefaultSink = s.Sink
	}

	// for backward compat with v1, allow "encoding" as
	// an alias for "defaultSink"
	if s.DefaultSink == "" {
		s.DefaultSink = s.Encoding
	}

	switch s.Out {
	case "stdout":
		s.config.Out = os.Stdout
	case "stderr":
		s.config.Out = os.Stderr
	}

	*c = Config(s.config)

	return nil
}

func (c Config) Handler() (slog.Handler, error) {
	if c.Out == nil {
		c.Out = os.Stdout
	}

	if c.DefaultSink == "" {
		c.DefaultSink = JSONSink
	}

	v, ok := sinkConstructors.Load(c.DefaultSink)
	if !ok {
		return nil, errors.New("unknown sink constructor: " + c.DefaultSink)
	}
	constructor, _ := v.(SinkConstructor)
	return constructor(c)
}

// Controller returns a new controller configured with the given config.
func (c Config) Controller() (*Controller, error) {
	ctl := NewController(nil)
	err := c.Configure(ctl)
	if err != nil {
		return nil, err
	}
	return ctl, nil
}

// Configure configures a controller with the given config.
//
// It sets the default level, levels, and sink.  Level settings
// replace any current level settings.
func (c Config) Configure(ctl *Controller) error {
	ctl.SetDefaultLevel(c.DefaultLevel)
	ctl.SetLevels(c.Levels, true)

	h, err := c.Handler()
	if err != nil {
		return err
	}
	ctl.SetDefaultSink(h)

	return nil
}

type Levels map[string]slog.Level

func (l *Levels) UnmarshalText(text []byte) error {
	m, err := parseLevels(string(text))
	if err != nil {
		return err
	}
	*l = m
	return nil
}

func (l *Levels) MarshalText() ([]byte, error) {
	if l == nil || *l == nil || len(*l) == 0 {
		return []byte{}, nil
	}

	directives := make([]string, 0, len(*l))
	for name, level := range *l {
		switch level {
		case LevelOff:
			directives = append(directives, "-"+name)
		case LevelAll:
			directives = append(directives, name)
		default:
			directives = append(directives, name+"="+level.String())
		}
	}

	return []byte(strings.Join(directives, ",")), nil
}

func parseLevels(s string) (map[string]slog.Level, error) {
	if s == "" {
		return nil, nil //nolint:nilnil
	}

	items := strings.Split(s, ",")
	m := map[string]slog.Level{}

	var errs error

	for _, setting := range items {
		parts := strings.Split(setting, "=")

		switch len(parts) {
		case 1:
			name := parts[0]
			if strings.HasPrefix(name, "-") {
				m[name[1:]] = LevelOff
			} else {
				m[name] = LevelAll
			}
		case 2:
			var err error
			m[parts[0]], err = parseLevel(parts[1])
			errs = errors.Join(errs, err)
		}
	}
	return m, merry.Prepend(errs, "invalid log levels")
}
