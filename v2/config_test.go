package flume

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"
	"testing"

	maps "github.com/ansel1/vespucci/v4"
	"github.com/ansel1/vespucci/v4/mapstest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevDefaults(t *testing.T) {
	dd := DevDefaults()
	dd.ReplaceAttrs = nil
	require.Equal(t, Config{DefaultSink: TermColorSink, AddSource: true}, dd)
}

func TestParseLevel(t *testing.T) {
	tests := map[slog.Level][]any{
		slog.LevelDebug:    {"DBG", "DEBUG", float64(-4), "-4", int(-4)},
		slog.LevelWarn:     {"WRN", "WARN", float64(4), "4", int(4)},
		slog.LevelInfo:     {"INF", "INFO", float64(0), "0", int(0), "", nil},
		slog.LevelError:    {"ERR", "ERROR", "erRor", "eRr", float64(8), "8", int(8)},
		slog.LevelWarn + 3: {"WRN+3", "WARN+3", int(slog.LevelWarn + 3)},
		slog.LevelWarn - 2: {"WRN-2", "WARN-2", int(slog.LevelWarn - 2)},
		math.MaxInt:        {"OFF", int(math.MaxInt), false},
		math.MinInt:        {"ALL", int(math.MinInt), true},
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

func TestParseLevel_error(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantError string
	}{
		{
			name:      "invalid string",
			input:     "INVALID",
			wantError: "invalid log level 'INVALID': slog: level string \"INVALID\": unknown name",
		},
		{
			name:      "map",
			input:     map[string]string{},
			wantError: "levels must be a string or int value",
		},
		{
			name:      "invalid level modifier",
			input:     "WRN+invalid",
			wantError: "invalid log level 'WRN+invalid': slog: level string \"WARN+INVALID\": strconv.Atoi: parsing \"+INVALID\": invalid syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseLevel(tt.input)
			assert.ErrorContains(t, err, tt.wantError)
		})
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
			confJSON: `{"defaultLevel":"WRN", "levels":"blue=WRN", "defaultSink":"text", "addSource":true}`,
			expected: Config{
				DefaultLevel: slog.LevelWarn,
				Levels:       Levels{"blue": slog.LevelWarn},
				DefaultSink:  TextSink,
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
			confJSON: `{"defaultLevel":2}`,
			expected: Config{
				DefaultLevel: slog.Level(2),
			},
		},
		{
			name:     "string int level",
			confJSON: `{"defaultLevel":"3"}`,
			expected: Config{
				DefaultLevel: slog.Level(3),
			},
		},
		{
			name:     "named level",
			confJSON: `{"defaultLevel":"err"}`,
			expected: Config{
				DefaultLevel: slog.LevelError,
			},
		},
		{
			name:     "levels as map",
			confJSON: `{"levels":{"inf":"INF","number":"1","nil":null,"debug":"DEBUG","true":true,"false":false,"rawInt":"-1","offset":"DBG-2","all":"all","empty":"","off":"off"}}`,
			expected: Config{Levels: Levels{
				"inf":    slog.LevelInfo,
				"number": slog.LevelInfo + 1,
				"nil":    slog.LevelInfo,
				"debug":  slog.LevelDebug,
				"true":   LevelAll,
				"false":  LevelOff,
				"rawInt": slog.LevelInfo - 1,
				"offset": slog.LevelDebug - 2,
				"all":    LevelAll,
				"empty":  slog.LevelInfo,
				"off":    LevelOff,
			}},
		},
		{
			name:     "encoding as alias for defaultSink",
			confJSON: `{"encoding":"text"}`,
			expected: Config{
				DefaultSink: TextSink,
			},
		},
		{
			name:     "sink as alias for defaultSink",
			confJSON: `{"sink":"text"}`,
			expected: Config{
				DefaultSink: TextSink,
			},
		},
		{
			name:     "defaultSink has higher precedence than sink",
			confJSON: `{"defaultSink":"text", "sink":"json"}`,
			expected: Config{
				DefaultSink: TextSink,
			},
		},
		{
			name:     "sink has higher precedence than encoding",
			confJSON: `{"sink":"text", "encoding":"json"}`,
			expected: Config{
				DefaultSink: TextSink,
			},
		},
		{
			name:     "level as alias for defaultLevel",
			confJSON: `{"level":"ERR"}`,
			expected: Config{
				DefaultLevel: slog.LevelError,
			},
		},
		{
			name:     "defaultLevel has higher precedence than level",
			confJSON: `{"level":"ERR", "defaultLevel":"WARN"}`,
			expected: Config{
				DefaultLevel: slog.LevelWarn,
			},
		},
		{
			name:     "out to stdout",
			confJSON: `{"out":"stdout"}`,
			expected: Config{
				Out: os.Stdout,
			},
		},
		{
			name:     "out to stderr",
			confJSON: `{"out":"stderr"}`,
			expected: Config{
				Out: os.Stderr,
			},
		},
		{
			name:          "invalid JSON",
			confJSON:      `{out:"stderr"}`,
			expectedError: "invalid character 'o' looking for beginning of object key string",
		},
		{
			name:          "invalid level",
			confJSON:      `{"level":"INVALID"}`,
			expectedError: "invalid log level 'INVALID': slog: level string \"INVALID\": unknown name",
		},
		{
			name:          "invalid levels string",
			confJSON:      `{"levels":"*=INVALID"}`,
			expectedError: "invalid log level 'INVALID': slog: level string \"INVALID\": unknown name",
		},
		{
			name:          "invalid levels map",
			confJSON:      `{"levels":{"*":"INVALID"}}`,
			expectedError: "invalid log level 'INVALID': slog: level string \"INVALID\": unknown name",
		},
		{
			name:          "invalid levels type",
			confJSON:      `{"levels":1}`,
			expectedError: "invalid levels value: 1",
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

			// can't compare functions with equals, so the best we can do to check
			// the equality of the ReplaceAttrs slice is compare the len
			//
			assert.Len(t, c.ReplaceAttrs, len(test.expected.ReplaceAttrs))
			c.ReplaceAttrs = nil

			test.expected.ReplaceAttrs = nil
			assert.Equal(t, test.expected, c)
		})
	}
}

func TestConfig_Configure(t *testing.T) {
	buf := bytes.NewBuffer(nil)

	tests := []struct {
		name      string
		conf      Config
		logFn     func(*testing.T, *slog.Logger, *Controller)
		assertFn  func(*testing.T, *bytes.Buffer)
		wantError string
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
				DefaultSink: "text",
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
					map[string]any{"source": map[string]any{"file": "config_test.go", "function": "TestConfig_Configure", "line": nil}},
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
			logFn: func(t *testing.T, l *slog.Logger, c *Controller) {
				l.Info("hi")
				assert.Empty(t, buf.String(), "default logger should only log warn and higher")

				buf.Reset()
				l.Warn("bye")
				mapstest.AssertContains(t, json.RawMessage(buf.Bytes()), map[string]any{"msg": "bye"}, "warn should be have been logged, was %v", buf.String())

				buf.Reset()
				l2 := c.Logger("blue")
				l2.Info("cya")
				mapstest.AssertContains(t, json.RawMessage(buf.Bytes()), map[string]any{"msg": "cya", LoggerKey: "blue"}, "blue logger should log info level, was %v", buf.String())
			},
		},
		{
			name: "should replace levels",
			conf: Config{
				DefaultSink:  TextSink,
				DefaultLevel: slog.LevelWarn,
				Levels:       Levels{"blue": slog.LevelInfo},
			},
			logFn: func(t *testing.T, _ *slog.Logger, c *Controller) {
				blue, red, white := c.Logger("blue"), c.Logger("red"), c.Logger("white")
				blue.Info("hiblue")
				red.Info("hired")
				white.Info("hiwhite")
				assert.Contains(t, buf.String(), "msg=hiblue")
				assert.NotContains(t, buf.String(), "msg=hired")
				assert.NotContains(t, buf.String(), "msg=hiwhite")

				buf.Reset()
				Config{DefaultSink: TextSink, DefaultLevel: slog.LevelWarn, Levels: Levels{"red": slog.LevelInfo}, Out: buf}.Configure(c)
				blue.Info("hiblue")
				red.Info("hired")
				white.Info("hiwhite")
				assert.NotContains(t, buf.String(), "msg=hiblue")
				assert.Contains(t, buf.String(), "msg=hired")
				assert.NotContains(t, buf.String(), "msg=hiwhite")

				buf.Reset()
				Config{DefaultSink: TextSink, DefaultLevel: slog.LevelWarn, Out: buf}.Configure(c)
				blue.Info("hiblue")
				red.Info("hired")
				white.Info("hiwhite")
				assert.Empty(t, buf.String())

				buf.Reset()
				Config{DefaultSink: TextSink, DefaultLevel: slog.LevelInfo, Out: buf}.Configure(c)
				blue.Info("hiblue")
				red.Info("hired")
				white.Info("hiwhite")
				assert.Contains(t, buf.String(), "msg=hiblue")
				assert.Contains(t, buf.String(), "msg=hired")
				assert.Contains(t, buf.String(), "msg=hiwhite")
			},
		},
		{
			name: "invalid sink",
			conf: Config{
				DefaultSink: "INVALID",
			},
			wantError: "unknown sink constructor: INVALID",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf.Reset()
			test.conf.Out = buf

			ctl := NewController(nil)

			err := test.conf.Configure(ctl)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}

			require.NoError(t, err)

			l := ctl.Logger("")

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

func TestLevels_UnmarshalText(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		expected  Levels
		wantError string
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
		{
			name:      "invalid level",
			text:      "invalid=INVALID",
			wantError: "invalid log level 'INVALID': slog: level string \"INVALID\": unknown name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var levels Levels
			err := levels.UnmarshalText([]byte(test.text))
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}

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

func TestConfig_UnmarshalEnv(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		envvars     []string
		expected    Config
		expectError string
	}{
		{
			name: "defaults",
			env: map[string]string{
				"FLUME": `{"level":"WRN"}`,
			},
			envvars: DefaultConfigEnvVars,
			expected: Config{
				DefaultLevel: slog.LevelWarn,
			},
		},
		{
			name: "empty env vars should be a no-op",
			env: map[string]string{
				"FLUME": `{"level":"WRN"}`,
			},
		},
		{
			name: "search list of env vars",
			env: map[string]string{
				"EMPTY":     ``,
				"LOGCONFIG": `{"level":"WRN"}`,
			},
			envvars: []string{"EMPTY", "LOGCONFIG"},
			expected: Config{
				DefaultLevel: slog.LevelWarn,
			},
		},
		{
			name: "parse error",
			env: map[string]string{
				"FLUME": `not json`,
			},
			envvars:     DefaultConfigEnvVars,
			expectError: "parsing configuration from environment variable FLUME: invalid character",
		},
	}

	for _, test := range tests { //nolint:paralleltest
		t.Run(test.name, func(t *testing.T) {
			for k, v := range test.env {
				t.Setenv(k, v)
			}
			var c Config
			err := c.UnmarshalEnv(test.envvars...)
			if test.expectError != "" {
				assert.ErrorContains(t, err, test.expectError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expected, c)
		})
	}
}

func TestConfigFromEnv(t *testing.T) {
	tests := []struct {
		name          string
		env           map[string]string
		envvars       []string
		expectedLevel slog.Level
		expectError   string
	}{
		{
			name: "default",
			env: map[string]string{
				"FLUME": `{"level":"WRN"}`,
			},
			expectedLevel: slog.LevelWarn,
		},
		{
			name: "search envvars",
			env: map[string]string{
				"EMPTY":     "",
				"LOGCONFIG": `{"level":"WRN"}`,
			},
			envvars:       []string{"EMPTY", "LOGCONFIG"},
			expectedLevel: slog.LevelWarn,
		},
		{
			name:          "no-op",
			envvars:       []string{"EMPTY", "LOGCONFIG"},
			expectedLevel: slog.LevelInfo,
		},
		{
			name: "parse error",
			env: map[string]string{
				"FLUME": `not json`,
			},
			expectError: "parsing configuration from environment variable FLUME: invalid character",
		},
	}

	for _, test := range tests { //nolint:paralleltest
		t.Run(test.name, func(t *testing.T) {
			// make sure the default controller is restored after the test
			ctl := Default()
			t.Cleanup(func() {
				SetDefault(ctl)
			})

			for k, v := range test.env {
				t.Setenv(k, v)
			}
			err := ConfigFromEnv(test.envvars...)
			if test.expectError != "" {
				assert.ErrorContains(t, err, test.expectError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expectedLevel, Default().DefaultLevel())
		})
	}
}

func TestMustConfigFromEnv(t *testing.T) {
	testCases := []struct {
		name          string
		env           map[string]string
		envvars       []string
		expectedLevel slog.Level
		expectPanic   string
	}{
		{
			name: "default",
			env: map[string]string{
				"FLUME": `{"level":"WRN"}`,
			},
			expectedLevel: slog.LevelWarn,
		},
		{
			name: "search envvars",
			env: map[string]string{
				"EMPTY":     "",
				"LOGCONFIG": `{"level":"WRN"}`,
			},
			envvars:       []string{"EMPTY", "LOGCONFIG"},
			expectedLevel: slog.LevelWarn,
		},
		{
			name:          "no-op",
			envvars:       []string{"EMPTY", "LOGCONFIG"},
			expectedLevel: slog.LevelInfo,
		},
		{
			name: "parse error",
			env: map[string]string{
				"FLUME": `not json`,
			},
			expectPanic: "parsing configuration from environment variable FLUME: invalid character 'o' in literal null (expecting 'u')",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.name, func(t *testing.T) {
			// make sure the default controller is restored after the test
			ctl := Default()
			t.Cleanup(func() {
				SetDefault(ctl)
			})

			for k, v := range tC.env {
				t.Setenv(k, v)
			}

			if tC.expectPanic != "" {
				assert.PanicsWithError(t, tC.expectPanic, func() {
					MustConfigFromEnv(tC.envvars...)
				})
				return
			}

			MustConfigFromEnv(tC.envvars...)
			assert.Equal(t, tC.expectedLevel, Default().DefaultLevel())
		})
	}
}
func TestRegisterSinkConstructor(t *testing.T) {
	tests := []struct {
		name            string
		constructorName string
		constructor     SinkConstructor
		want            string
		wantPanic       bool
		wantError       string
	}{
		{
			name: "register constructor",
			constructor: func(c Config) (slog.Handler, error) {
				return slog.NewTextHandler(c.Out, nil).WithAttrs([]slog.Attr{slog.String("test", "test")}), nil
			},
			want:            "test=test",
			constructorName: "blue",
		},
		{
			name:            "register nil constructor should panic",
			wantPanic:       true,
			constructorName: "blue",
		},
		{
			name:            "register constructor with empty name",
			constructorName: "",
			constructor:     func(_ Config) (slog.Handler, error) { return noop, nil },
			wantPanic:       true,
		},
		{
			name:            "re-register constructor",
			constructorName: TextSink,
			constructor: func(c Config) (slog.Handler, error) {
				return slog.NewTextHandler(c.Out, nil).WithAttrs([]slog.Attr{slog.String("test", "test")}), nil
			},
			want: "test=test",
		},
		{
			name:            "register a constructor which returns an error",
			constructorName: "blue",
			constructor: func(_ Config) (slog.Handler, error) {
				return nil, errors.New("test error")
			},
			wantError: "test error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// first, reset the sink constructors map
			resetBuiltInSinks()

			if tt.wantPanic {
				assert.Panics(t, func() {
					RegisterSinkConstructor(tt.constructorName, tt.constructor)
				})
				return
			}

			RegisterSinkConstructor(tt.constructorName, tt.constructor)

			// Verify the constructor was registered by configuring a sink
			var buf bytes.Buffer
			conf := Config{
				DefaultSink: tt.constructorName,
				Out:         &buf,
			}
			ctl := NewController(nil)
			err := conf.Configure(ctl)
			if tt.wantError != "" {
				assert.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)

			// Test the configured sink works
			logger := ctl.Logger("")
			logger.Info("test message")
			if tt.want != "" {
				assert.Contains(t, buf.String(), tt.want)
			} else {
				assert.Empty(t, buf.String())
			}
		})
	}

	builtIns := map[string]string{
		TermSink:      "> test message\n",
		TermColorSink: "\x1b[0m \x1b[36m> \x1b[0m\x1b[1mtest message\x1b[0m\n",
		TextSink:      "level=INFO msg=\"test message\" logger=blue\n",
		JSONSink:      `{"level":"INFO","msg":"test message","logger":"blue"}` + "\n",
		ConsoleSink:   "level=INFO msg=\"test message\" logger=blue\n",
	}
	for name, want := range builtIns {
		t.Run("builtin "+name, func(t *testing.T) {
			var buf bytes.Buffer
			conf := Config{
				DefaultSink:  name,
				Out:          &buf,
				ReplaceAttrs: []func([]string, slog.Attr) slog.Attr{removeKeys(slog.TimeKey)},
			}
			ctl := NewController(nil)
			err := conf.Configure(ctl)
			require.NoError(t, err)

			logger := ctl.Logger("blue")
			logger.Info("test message")
			assert.Contains(t, buf.String(), want)
		})
	}
}
