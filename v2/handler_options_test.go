package flume

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"testing"

	"github.com/ansel1/console-slog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevDefaults(t *testing.T) {
	dd := DevDefaults()

	assert.Nil(t, dd.Level)
	assert.Empty(t, dd.Levels)
	assert.True(t, dd.AddSource)
	assert.Empty(t, dd.ReplaceAttrs)
	assert.Empty(t, dd.Middleware)

	// can't assert equality because HandlerFn is a function
	require.NotNil(t, dd.HandlerFn)

	buf := &bytes.Buffer{}
	h := dd.HandlerFn("test", buf, &slog.HandlerOptions{})
	if _, ok := h.(*console.Handler); !ok {
		t.Fatalf("expected console handler, got %T", h)
	}
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
				"off":    LevelOff,
				"all":    LevelAll,
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
				"disable":     LevelOff,
				"on":          LevelAll,
				"offset":      slog.LevelDebug + 2,
				"offsetMinus": slog.LevelDebug - 2,
				"inf":         slog.LevelInfo,
				"wrn":         slog.LevelWarn,
				"err":         slog.LevelError,
				"dbg":         slog.LevelDebug,
				"int":         slog.LevelDebug,
				"def":         slog.LevelInfo,
				"all":         LevelAll,
				"off":         LevelOff,
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

func TestHandlerOptions_UnmarshalJSON(t *testing.T) {
	theme := console.NewDefaultTheme()
	tests := []struct {
		name     string
		confJSON string
		want     string
		expected HandlerOptions
		wantErr  string
	}{
		{
			name:     "defaults",
			confJSON: `{}`,
			expected: HandlerOptions{},
		},
		{
			name:     "standard",
			confJSON: `{"level":"WRN", "levels":"blue=WRN", "handler":"text", "addSource":true}`,
			expected: HandlerOptions{
				Level:     slog.LevelWarn,
				Levels:    Levels{"blue": slog.LevelWarn},
				HandlerFn: TextHandlerFn(),
				AddSource: true,
			},
		},
		{
			name:     "dev defaults",
			confJSON: `{"development":true}`,
			expected: *DevDefaults(),
			want: styled("|", theme.Header) + styled("INF", theme.LevelInfo) + styled("|", theme.Header) + " " +
				styled("hi", theme.Message) + "\n",
		},
		{
			name:     "int level",
			confJSON: `{"level":2}`,
			expected: HandlerOptions{
				Level: slog.Level(2),
			},
		},
		{
			name:     "string int level",
			confJSON: `{"level":"3"}`,
			expected: HandlerOptions{
				Level: slog.Level(3),
			},
		},
		{
			name:     "named level",
			confJSON: `{"level":"err"}`,
			expected: HandlerOptions{
				Level: slog.LevelError,
			},
		},
		{
			name:     "levels as map",
			confJSON: `{"levels":{"inf":"INF","number":"1","nil":null,"debug":"DEBUG","true":true,"false":false,"rawInt":"-1","offset":"DBG-2","all":"all","empty":"","off":"off"}}`,
			expected: HandlerOptions{
				Levels: Levels{
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
				},
			},
		},
		{
			name:     "encoding as alias for defaultSink",
			confJSON: `{"encoding":"text"}`,
			expected: HandlerOptions{
				HandlerFn: TextHandlerFn(),
			},
		},
		{
			name:     "handler has higher precedence than encoding",
			confJSON: `{"handler":"text", "encoding":"json"}`,
			expected: HandlerOptions{
				HandlerFn: TextHandlerFn(),
			},
		},
		{
			name:     "addCaller as alias for addSource",
			confJSON: `{"addCaller":true}`,
			expected: HandlerOptions{
				AddSource: true,
			},
		},
		{
			name:     "addSource has higher precedence than addCaller",
			confJSON: `{"addSource":false, "addCaller":true}`,
			expected: HandlerOptions{
				AddSource: false,
			},
		},
		{
			name:     "text handler",
			confJSON: `{"handler":"text"}`,
			expected: HandlerOptions{
				HandlerFn: TextHandlerFn(),
			},
			want: "level=INFO msg=hi\n",
		},
		{
			name:     "json handler",
			confJSON: `{"handler":"json"}`,
			expected: HandlerOptions{
				HandlerFn: JSONHandlerFn(),
			},
			want: `{"level":"INFO","msg":"hi"}` + "\n",
		},
		{
			name:     "invalid JSON",
			confJSON: `{out:"stderr"}`,
			wantErr:  "invalid character 'o' looking for beginning of object key string",
		},
		{
			name:     "invalid level",
			confJSON: `{"level":"INVALID"}`,
			wantErr:  "invalid log level 'INVALID': slog: level string \"INVALID\": unknown name",
		},
		{
			name:     "invalid levels string",
			confJSON: `{"levels":"*=INVALID"}`,
			wantErr:  "invalid log level 'INVALID': slog: level string \"INVALID\": unknown name",
		},
		{
			name:     "invalid levels map",
			confJSON: `{"levels":{"*":"INVALID"}}`,
			wantErr:  "invalid log level 'INVALID': slog: level string \"INVALID\": unknown name",
		},
		{
			name:     "invalid levels type",
			confJSON: `{"levels":1}`,
			wantErr:  "invalid levels value: 1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var opts HandlerOptions
			err := json.Unmarshal([]byte(test.confJSON), &opts)
			if test.wantErr != "" {
				assert.Error(t, err, test.wantErr)
				return
			}

			require.NoError(t, err)

			assertHandlerOptionsEqual(t, test.expected, opts, test.want)
		})
	}
}

func assertHandlerOptionsEqual(t *testing.T, want, got HandlerOptions, sample string) {
	t.Helper()

	// hard to compare HandlerOptions with ==.  We'll check what we can,
	// and then use a test message to compare the rest
	if want.HandlerFn != nil {
		assert.NotNil(t, got.HandlerFn)
	} else {
		assert.Nil(t, got.HandlerFn)
	}

	assert.Equal(t, want.Level, got.Level)

	assert.Equal(t, want.Levels, got.Levels)

	assert.Equal(t, want.AddSource, got.AddSource)

	if want.ReplaceAttrs != nil {
		assert.NotNil(t, got.ReplaceAttrs)
		assert.Equal(t, len(want.ReplaceAttrs), len(got.ReplaceAttrs))
	} else {
		assert.Empty(t, got.ReplaceAttrs)
	}

	if want.Middleware != nil {
		assert.NotNil(t, got.Middleware)
		assert.Equal(t, len(want.Middleware), len(got.Middleware))
	} else {
		assert.Empty(t, got.Middleware)
	}

	if sample != "" {
		handlerTest{
			opts: &got,
			want: sample,
		}.Run(t)
	}
}
