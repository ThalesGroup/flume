package flume

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/ansel1/merry/v2"
)

type Config struct {
	// DefaultLevel is the default log level for all loggers not
	// otherwise configured by Levels.  Defaults to Info.
	DefaultLevel slog.Level `json:"defaultLevel,omitempty"`
	// Levels configures log levels for particular named loggers.  See
	// LevelsString for format.
	Levels string `json:"levels,omitempty"`
	// Encoding sets the logger's encoding. Valid values are "json",
	// "text", "ltsv", "term", and "term-color".
	//
	// For compatibility with flume v1, "console" is also accepted, and
	// is an alias for "text"
	Encoding string `json:"encoding,omitempty"`
	// AddSource causes the handler to compute the source code position
	// of the log statement and add a SourceKey attribute to the output.
	// Defaults to true when the Development flag is set, false otherwise.
	AddSource bool `json:"addSource,omitempty"`
	Out       io.Writer
}

const (
	EncodingJSON      = "json"
	EncodingText      = "text"
	EncodingTermColor = "term-color"
)

func DevDefaults() Config {
	return Config{
		Encoding:  "term-color",
		AddSource: true,
	}
}

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
		// allow raw integer values for level
		return slog.Level(v), nil
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
		return slog.Level(math.MinInt), nil
	case "":
		return slog.LevelInfo, nil // default
	case "OFF":
		return slog.Level(math.MaxInt), nil
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
		DefaultLevel any   `json:"defaultLevel"`
		Level        any   `json:"level"`
		AddSource    *bool `json:"addSource"`
		AddCaller    *bool `json:"addCaller"`
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

	// for backward compat with v1, allow "addCaller" as
	// an alias for "addSource"
	if s.AddSource == nil {
		s.AddSource = s.AddCaller
	}

	if s.AddSource != nil {
		s.config.AddSource = *s.AddSource
	}

	*c = Config(s.config)

	return nil
}

func (c *Config) Configure(f *Controller) error {
	f.SetDefaultLevel(c.DefaultLevel)

	out := c.Out
	if out == nil {
		out = os.Stdout
	}

	opts := slog.HandlerOptions{
		AddSource: c.AddSource,
	}

	var handler slog.Handler

	switch c.Encoding {
	case "text":
		handler = slog.NewTextHandler(out, &opts)
	case "json":
		fallthrough
	default:
		handler = slog.NewJSONHandler(out, &opts)
	}

	f.SetDefaultDelegate(handler)

	return nil
}
