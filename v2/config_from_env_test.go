package flume

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_UnmarshalEnv(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		envvars     []string
		expected    HandlerOptions
		expectError string
		want        string
	}{
		{
			name: "defaults",
			env: map[string]string{
				"FLUME": `{"level":"WRN"}`,
			},
			envvars: DefaultConfigEnvVars,
			expected: HandlerOptions{
				Level: slog.LevelWarn,
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
			expected: HandlerOptions{
				Level: slog.LevelWarn,
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

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for k, v := range test.env {
				t.Setenv(k, v)
			}
			var opts HandlerOptions
			err := UnmarshalEnv(&opts, test.envvars...)
			if test.expectError != "" {
				assert.ErrorContains(t, err, test.expectError)
				return
			}

			require.NoError(t, err)
			assertHandlerOptionsEqual(t, test.expected, opts, test.want)
		})
	}
}

func TestConfigFromEnv(t *testing.T) {
	tests := []struct {
		name          string
		env           map[string]string
		envvars       []string
		expectedLevel slog.Leveler
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
			expectedLevel: nil,
		},
		{
			name: "parse error",
			env: map[string]string{
				"FLUME": `not json`,
			},
			expectError: "parsing configuration from environment variable FLUME: invalid character",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// make sure the default handler is restored after the test
			ogOpts := Default().HandlerOptions()
			t.Cleanup(func() {
				Default().SetHandlerOptions(ogOpts)
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
			assert.Equal(t, test.expectedLevel, Default().HandlerOptions().Level)
		})
	}
}

func TestMustConfigFromEnv(t *testing.T) {
	testCases := []struct {
		name          string
		env           map[string]string
		envvars       []string
		expectedLevel slog.Leveler
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
			expectedLevel: nil,
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
			// make sure the default handler is restored after the test
			ogOpts := Default().HandlerOptions()
			t.Cleanup(func() {
				Default().SetHandlerOptions(ogOpts)
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
			assert.Equal(t, tC.expectedLevel, Default().HandlerOptions().Level)
		})
	}
}
func TestRegisterHandlerFn(t *testing.T) {
	tests := []struct {
		name        string
		handlerName string
		handlerFn   HandlerFn
		want        string
		wantPanic   bool
	}{
		{
			name: "register constructor",
			handlerFn: func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
				return slog.NewTextHandler(w, opts).WithAttrs([]slog.Attr{slog.String("test", "test")})
			},
			want:        "level=INFO msg=hi test=test\n",
			handlerName: "blue",
		},
		{
			name:        "register nil constructor should panic",
			wantPanic:   true,
			handlerName: "blue",
		},
		{
			name:        "register constructor with empty name",
			handlerName: "",
			handlerFn:   func(_ string, _ io.Writer, _ *slog.HandlerOptions) slog.Handler { return noop },
			wantPanic:   true,
		},
		{
			name:        "re-register constructor",
			handlerName: TextHandler,
			handlerFn: func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
				return slog.NewTextHandler(w, opts).WithAttrs([]slog.Attr{slog.String("test", "test")})
			},
			want: "level=INFO msg=hi test=test\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// first, reset the handlerFns map
			resetBuiltInHandlerFns()
			// also reset after the test
			t.Cleanup(resetBuiltInHandlerFns)
			if tt.wantPanic {
				assert.Panics(t, func() {
					RegisterHandlerFn(tt.handlerName, tt.handlerFn)
				})
				return
			}

			RegisterHandlerFn(tt.handlerName, tt.handlerFn)

			// Verify the handlerFn
			handlerTest{
				want: tt.want,
				handlerFn: func(buf *bytes.Buffer) slog.Handler {
					return LookupHandlerFn(tt.handlerName)("", buf, nil)
				},
			}.Run(t)
		})
	}

	builtIns := map[string]string{
		TermHandler:      "blue         INF | hi\n",
		TermColorHandler: "\x1b[1;90mblue        \x1b[0m \x1b[32mINF\x1b[0m \x1b[1;90m|\x1b[0m \x1b[1mhi\x1b[0m\n",
		TextHandler:      "level=INFO msg=hi logger=blue\n",
		JSONHandler:      `{"level":"INFO","msg":"hi","logger":"blue"}` + "\n",
		ConsoleHandler:   "level=INFO msg=hi logger=blue\n",
		NoopHandler:      "",
	}
	for name, want := range builtIns {
		t.Run("builtin "+name, func(t *testing.T) {
			handlerTest{
				want: want,
				handlerFn: func(buf *bytes.Buffer) slog.Handler {
					return LookupHandlerFn(name)("", buf, nil).WithAttrs([]slog.Attr{slog.String(LoggerKey, "blue")})
				},
			}.Run(t)
		})
	}
}
