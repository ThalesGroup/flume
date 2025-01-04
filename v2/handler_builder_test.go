package flume

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"testing"

	maps "github.com/ansel1/vespucci/v4"
	"github.com/ansel1/vespucci/v4/mapstest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevDefaults(t *testing.T) {
	require.Equal(t, Config{Encoding: EncodingTermColor, AddSource: true}, DevDefaults())
}

func TestParseLevel(t *testing.T) {
	tests := map[slog.Level][]any{
		slog.LevelDebug:    {"DBG", "DEBUG", float64(-4), "-4"},
		slog.LevelWarn:     {"WRN", "WARN", float64(4), "4"},
		slog.LevelInfo:     {"INF", "INFO", float64(0), "0", "", nil},
		slog.LevelError:    {"ERR", "ERROR", "erRor", "eRr", float64(8), "8"},
		slog.LevelWarn + 3: {"WRN+3", "WARN+3"},
		slog.LevelWarn - 2: {"WRN-2", "WARN-2"},
		math.MaxInt:        {"OFF"},
		math.MinInt:        {"ALL"},
	}

	for level, aliases := range tests {
		for _, alias := range aliases {
			t.Run(fmt.Sprint(alias), func(t *testing.T) {
				l, err := parseLevel(alias)
				require.NoError(t, err)

				require.Equal(t, level, l)
			})
		}
	}
}

func TestConfig_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name          string
		confJSON      string
		expected      Config
		expectedError string
	}{
		{
			name:     "defaults",
			confJSON: `{}`,
			expected: Config{},
		},
		{
			name:     "standard",
			confJSON: `{"level":"WRN", "levels":"blue=WRN", "encoding":"text", "addSource":true}`,
			expected: Config{
				DefaultLevel: slog.LevelWarn,
				Levels:       Levels{"blue": slog.LevelWarn},
				Encoding:     "text",
				AddSource:    true,
			},
		},
		{
			name:     "dev defaults",
			confJSON: `{"development":true}`,
			expected: DevDefaults(),
		},
		{
			name:     "int level",
			confJSON: `{"level":2}`,
			expected: Config{
				DefaultLevel: slog.Level(2),
			},
		},
		{
			name:     "string int level",
			confJSON: `{"level":"3"}`,
			expected: Config{
				DefaultLevel: slog.Level(3),
			},
		},
		{
			name:     "named level",
			confJSON: `{"level":"err"}`,
			expected: Config{
				DefaultLevel: slog.LevelError,
			},
		},
		{
			name:     "level as alias for defaultLevel",
			confJSON: `{"level":"err"}`,
			expected: Config{DefaultLevel: slog.LevelError},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var c Config
			err := json.Unmarshal([]byte(test.confJSON), &c)
			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)
				return
			}

			require.NoError(t, err)

			assert.Equal(t, test.expected, c)
		})
	}
}

func TestConfig_Configure(t *testing.T) {
	buf := bytes.NewBuffer(nil)

	tests := []struct {
		name     string
		conf     Config
		logFn    func(*testing.T, *slog.Logger, *Controller)
		assertFn func(*testing.T, *bytes.Buffer)
	}{
		{
			name: "defaults",
			logFn: func(t *testing.T, l *slog.Logger, _ *Controller) {
				assert.True(t, l.Enabled(context.Background(), slog.LevelInfo))
				assert.False(t, l.Enabled(context.Background(), slog.LevelDebug))
				l.Info("hi")
				logLine := json.RawMessage(buf.Bytes())
				mapstest.AssertContains(t, logLine, map[string]any{"msg": "hi"}, "should have been json, was %v", buf.String())
				mapstest.AssertNotContains(t, logLine, map[string]any{"source": nil}, "AddSource should have been false, but log includes source: %v", buf.String())
			},
		},
		{
			name: "default level debug",
			conf: Config{
				DefaultLevel: slog.LevelDebug,
			},
			logFn: func(t *testing.T, l *slog.Logger, _ *Controller) {
				assert.True(t, l.Enabled(context.Background(), slog.LevelDebug))
			},
		},
		{
			name: "default level warn",
			conf: Config{
				DefaultLevel: slog.LevelWarn,
			},
			logFn: func(t *testing.T, l *slog.Logger, _ *Controller) {
				assert.True(t, l.Enabled(context.Background(), slog.LevelWarn))
				assert.False(t, l.Enabled(context.Background(), slog.LevelInfo))
			},
		},
		{
			name: "text encoder",
			conf: Config{
				Encoding: "text",
			},
			logFn: func(t *testing.T, l *slog.Logger, _ *Controller) {
				l.Info("hi")
				assert.Contains(t, buf.String(), "msg=hi")
			},
		},
		{
			name: "add source",
			conf: Config{
				AddSource: true,
			},
			logFn: func(t *testing.T, l *slog.Logger, _ *Controller) {
				l.Info("hi")
				mapstest.AssertContains(
					t,
					json.RawMessage(buf.Bytes()),
					map[string]any{"source": map[string]any{"file": "handler_builder_test.go", "function": "TestConfig_Configure", "line": nil}},
					maps.StringContains(),
					"AddSource should have been enabled: %v",
					buf.String(),
				)
			},
		},
		{
			name: "levels",
			conf: Config{
				Levels: Levels{"*": slog.LevelWarn, "blue": slog.LevelInfo},
			},
			logFn: func(t *testing.T, l *slog.Logger, f *Controller) {
				l.Info("hi")
				assert.Empty(t, buf.String(), "default logger should only log warn and higher")

				buf.Reset()
				l.Warn("bye")
				mapstest.AssertContains(t, json.RawMessage(buf.Bytes()), map[string]any{"msg": "bye"}, "warn should be have been logged, was %v", buf.String())

				buf.Reset()
				l2 := slog.New(f.Handler("blue"))
				l2.Info("cya")
				mapstest.AssertContains(t, json.RawMessage(buf.Bytes()), map[string]any{"msg": "cya", "logger": "blue"}, "blue logger should log info level, was %v", buf.String())
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf.Reset()
			test.conf.Out = buf

			ctl := NewController(nil)

			err := test.conf.Configure(ctl)
			require.NoError(t, err)

			l := slog.New(ctl.Handler(""))

			if test.logFn != nil {
				test.logFn(t, l, ctl)
			}
		})
	}
}

func TestLevelsMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		levels   Levels
		expected string
	}{
		{
			name:     "empty",
			levels:   Levels{},
			expected: "",
		},
		{
			name:     "nil",
			levels:   nil,
			expected: "",
		},
		{
			name: "values",
			levels: Levels{
				"info":   slog.LevelInfo,
				"warn":   slog.LevelWarn,
				"error":  slog.LevelError,
				"debug":  slog.LevelDebug,
				"off":    math.MaxInt,
				"all":    math.MinInt,
				"offset": slog.LevelDebug + 2,
			},
			expected: "info=INFO,warn=WARN,error=ERROR,debug=DEBUG,-off,all,offset=DEBUG+2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := test.levels.MarshalText()
			require.NoError(t, err)

			// order of directives in the encoded string is non-deterministic
			assert.ElementsMatch(t, strings.Split(string(actual), ","), strings.Split(test.expected, ","))

			// test unmarshaling
			var levels Levels
			err = levels.UnmarshalText(actual)
			require.NoError(t, err)

			if len(test.levels) == 0 {
				assert.Nil(t, levels)
			} else {
				assert.Equal(t, test.levels, levels)
			}
		})
	}
}

func TestLevelsUnmarshaling(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected Levels
	}{
		{
			name:     "empty",
			text:     "",
			expected: nil,
		},
		{
			name: "values",
			text: "info=INFO,warn=WARN,error=ERROR,debug=DEBUG,-disable,on,offset=DEBUG+2,offsetMinus=DBG-2,inf=INF,wrn=WRN,err=ERR,dbg=DBG,int=-4,def=,all=all,off=off",
			expected: Levels{
				"info":        slog.LevelInfo,
				"warn":        slog.LevelWarn,
				"error":       slog.LevelError,
				"debug":       slog.LevelDebug,
				"disable":     math.MaxInt,
				"on":          math.MinInt,
				"offset":      slog.LevelDebug + 2,
				"offsetMinus": slog.LevelDebug - 2,
				"inf":         slog.LevelInfo,
				"wrn":         slog.LevelWarn,
				"err":         slog.LevelError,
				"dbg":         slog.LevelDebug,
				"int":         slog.LevelDebug,
				"def":         slog.LevelInfo,
				"all":         math.MinInt,
				"off":         math.MaxInt,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var levels Levels
			err := levels.UnmarshalText([]byte(test.text))
			require.NoError(t, err)

			assert.Equal(t, test.expected, levels)

			// test roundtrip
			actual, err := levels.MarshalText()
			require.NoError(t, err)

			var levels2 Levels
			err = levels2.UnmarshalText(actual)
			require.NoError(t, err)

			assert.Equal(t, levels, levels2)
		})
	}
}
