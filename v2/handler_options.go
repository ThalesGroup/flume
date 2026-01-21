package flume

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Define static error variables
var (
	ErrInvalidLevels       = errors.New("invalid levels value")
	ErrInvalidLevel        = errors.New("invalid log level")
	ErrUnregisteredHandler = errors.New("unregistered handler")
)

// HandlerFn is a constructor for slog handlers.  The function should return a slog.Handler
// configured with the given writer and options.  `w` and `opts` will never be nil.
//
// `name` is the name of the logger for which the handler is being created, e.g. via
// flume.New("<name>") or logger.With(flume.LoggerKey, "<name>").  This parameter may
// be used to return different handlers or different middleware for particular loggers.
type HandlerFn func(name string, w io.Writer, opts *slog.HandlerOptions) slog.Handler

type HandlerOptions struct {
	// default level for all loggers, defaults to slog.LevelInfo
	Level slog.Leveler
	// per-logger levels
	Levels map[string]slog.Leveler
	// add source to log records
	AddSource bool
	// replace attributes
	ReplaceAttrs []func(groups []string, a slog.Attr) slog.Attr
	// If set, will be called to construct handler instances.
	// Defaults to TextHandlerFn()
	HandlerFn HandlerFn
	// middleware applied to all sinks
	Middleware []Middleware
}

func DevDefaults() *HandlerOptions {
	return &HandlerOptions{
		HandlerFn: TermColorHandlerFn(),
		AddSource: true,
	}
}

func (o *HandlerOptions) Clone() *HandlerOptions {
	if o == nil {
		return nil
	}

	ret := &HandlerOptions{
		Level:        o.Level,
		Levels:       maps.Clone(o.Levels),
		AddSource:    o.AddSource,
		HandlerFn:    o.HandlerFn,
		ReplaceAttrs: slices.Clone(o.ReplaceAttrs),
		Middleware:   slices.Clone(o.Middleware),
	}

	return ret
}

func (o *HandlerOptions) UnmarshalJSON(bytes []byte) error {
	s := struct {
		Development bool   `json:"development"`
		Handler     string `json:"handler"`
		Level       any    `json:"level"`
		Levels      any    `json:"levels"`
		AddSource   *bool  `json:"addSource"`
		AddCaller   *bool  `json:"addCaller"`
		Encoding    string `json:"encoding"`
	}{}

	err := json.Unmarshal(bytes, &s)
	if err != nil {
		return fmt.Errorf("invalid json config: %w", err)
	}

	var opts *HandlerOptions

	switch {
	case s.Development:
		opts = DevDefaults()
	case o == nil:
		opts = &HandlerOptions{}
	default:
		opts = o.Clone()
	}

	if s.Level != nil {
		level, err := parseLevel(s.Level)
		if err != nil {
			return err
		}

		opts.Level = level
	}

	switch lvls := s.Levels.(type) {
	case nil:
	case string:
		var l Levels

		err := l.UnmarshalText([]byte(lvls))
		if err != nil {
			return err
		}

		opts.Levels = l
	case map[string]any:
		if len(lvls) > 0 {
			opts.Levels = Levels{}

			for n, l := range lvls {
				lvl, err := parseLevel(l)
				if err != nil {
					return err
				}

				opts.Levels[n] = lvl
			}
		}
	default:
		return fmt.Errorf("%w '%v': must be a levels string or map", ErrInvalidLevels, s.Levels)
	}

	// for backward compatibility with flumev1, if there is a level named "*"
	// in the levels map, treat it like setting the default level.  Again,
	// to match v1 behavior, this overrides the default level option.
	if lvl, ok := opts.Levels["*"]; ok {
		opts.Level = lvl
		delete(opts.Levels, "*")
	}

	// for backward compat with v1, allow "addCaller" as
	// an alias for "addSource"
	if s.AddSource == nil {
		s.AddSource = s.AddCaller
	}

	if s.AddSource != nil {
		opts.AddSource = *s.AddSource
	}

	// for backward compat with v1, allow "encoding" as
	// an alias for "handler"
	if s.Handler == "" {
		s.Handler = s.Encoding
		// for backward compatibility with v1, add aliases
		// for the other values of "encoding".
		switch s.Handler {
		case "ltsv":
			s.Handler = TextHandler
		case "console":
			s.Handler = TermHandler
		}
	}

	if s.Handler != "" {
		fn := LookupHandlerFn(s.Handler)
		if fn == nil {
			return fmt.Errorf("%w: '%v'", ErrUnregisteredHandler, s.Handler)
		}

		opts.HandlerFn = fn
	}

	*o = *opts

	return nil
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
		return 0, fmt.Errorf("%w: should be string, number, or bool", ErrInvalidLevel)
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
	if err != nil {
		return 0, fmt.Errorf("%w '%v': %w", ErrInvalidLevel, v, err)
	}

	return l, nil
}

type Levels map[string]slog.Leveler

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
			directives = append(directives, name+"="+level.Level().String())
		}
	}

	return []byte(strings.Join(directives, ",")), nil
}

func parseLevels(s string) (map[string]slog.Leveler, error) {
	if s == "" {
		return nil, nil //nolint:nilnil
	}

	items := strings.Split(s, ",")
	m := map[string]slog.Leveler{}

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

	if errs != nil {
		return nil, fmt.Errorf("%w '%v': %w", ErrInvalidLevels, s, errs)
	}

	return m, nil
}
