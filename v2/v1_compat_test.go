package flume

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// formattableError implements fmt.Formatter but not json.Marshaler
type formattableError struct {
	msg    string
	detail string
}

func (f *formattableError) Error() string {
	return f.msg
}

func (f *formattableError) Format(s fmt.State, _ rune) {
	_, _ = fmt.Fprint(s, f.msg)
	if s.Flag('+') {
		_, _ = fmt.Fprint(s, f.detail)
	}
}

// jsonError implements both fmt.Formatter and json.Marshaler
type jsonError struct {
	msg    string
	detail string
}

func (j *jsonError) Error() string {
	return j.msg
}

func (j *jsonError) Format(s fmt.State, _ rune) {
	_, _ = fmt.Fprint(s, j.msg)
	if s.Flag('+') {
		_, _ = fmt.Fprint(s, j.detail)
	}
}

func (j *jsonError) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{
		"message": j.msg,
		"detail":  j.detail,
	})
}

func TestV1VerboseErrors(t *testing.T) {
	tests := []struct {
		name     string
		attr     slog.Attr
		validate func(t *testing.T, result slog.Attr)
	}{
		{
			name: "regular error passes through unchanged",
			attr: slog.Any("error", errors.New("simple error")),
			validate: func(t *testing.T, result slog.Attr) {
				assert.Equal(t, "error", result.Key)
				assert.Equal(t, slog.KindAny, result.Value.Kind())
				err, ok := result.Value.Any().(error)
				require.True(t, ok)
				assert.Equal(t, "simple error", err.Error())
			},
		},
		{
			name: "formattable error creates two attributes",
			attr: slog.Any("error", &formattableError{msg: "boom", detail: " with details"}),
			validate: func(t *testing.T, result slog.Attr) {
				// Should return a group with empty key
				assert.Empty(t, result.Key)
				assert.Equal(t, slog.KindGroup, result.Value.Kind())

				// Extract the group members
				attrs := result.Value.Group()
				require.Len(t, attrs, 2)

				// First attribute should be "error" with just the message
				assert.Equal(t, "error", attrs[0].Key)
				err1, ok := attrs[0].Value.Any().(error)
				require.True(t, ok)
				assert.Equal(t, "boom", err1.Error())

				// Second attribute should be "errorVerbose" with formatted details
				assert.Equal(t, "errorVerbose", attrs[1].Key)
				assert.Equal(t, "boom with details", attrs[1].Value.String())
			},
		},
		{
			name: "json.Marshaler error passes through unchanged",
			attr: slog.Any("error", &jsonError{msg: "json error", detail: "details"}),
			validate: func(t *testing.T, result slog.Attr) {
				assert.Equal(t, "error", result.Key)
				assert.Equal(t, slog.KindAny, result.Value.Kind())
				err, ok := result.Value.Any().(*jsonError)
				require.True(t, ok)
				assert.Equal(t, "json error", err.Error())
			},
		},
		{
			name: "non-error attribute passes through unchanged",
			attr: slog.String("message", "hello"),
			validate: func(t *testing.T, result slog.Attr) {
				assert.Equal(t, "message", result.Key)
				assert.Equal(t, "hello", result.Value.String())
			},
		},
		{
			name: "non-Any attribute passes through unchanged",
			attr: slog.Int("count", 42),
			validate: func(t *testing.T, result slog.Attr) {
				assert.Equal(t, "count", result.Key)
				assert.Equal(t, int64(42), result.Value.Int64())
			},
		},
		{
			name: "nil error passes through unchanged",
			attr: slog.Any("error", nil),
			validate: func(t *testing.T, result slog.Attr) {
				assert.Equal(t, "error", result.Key)
				assert.Nil(t, result.Value.Any())
			},
		},
		{
			name: "formattable error with different key name",
			attr: slog.Any("myerr", &formattableError{msg: "failed", detail: "\nstack trace here"}),
			validate: func(t *testing.T, result slog.Attr) {
				// Should return a group with empty key
				assert.Empty(t, result.Key)
				assert.Equal(t, slog.KindGroup, result.Value.Kind())

				// Extract the group members
				attrs := result.Value.Group()
				require.Len(t, attrs, 2)

				// First attribute should be "error" (note: hardcoded, not "myerr")
				assert.Equal(t, "error", attrs[0].Key)
				err1, ok := attrs[0].Value.Any().(error)
				require.True(t, ok)
				assert.Equal(t, "failed", err1.Error())

				// Second attribute should be "errorVerbose"
				assert.Equal(t, "errorVerbose", attrs[1].Key)
				assert.Equal(t, "failed\nstack trace here", attrs[1].Value.String())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := V1VerboseErrors(nil, tt.attr)
			tt.validate(t, result)
		})
	}
}

func TestV1JSONHandler(t *testing.T) {
	// Save the original JSON handler to restore it at the end
	originalHandler := JSONHandlerFn()
	require.NotNil(t, originalHandler)

	// Ensure we restore the original handler at the end
	defer func() {
		RegisterHandlerFn(JSONHandler, originalHandler)
	}()

	// Call V1JSONHandler to configure the v1-compatible handler
	V1JSONHandler()

	// Get the configured handler function
	handlerFn := JSONHandlerFn()
	require.NotNil(t, handlerFn)

	// Test that the handler applies all v1 transformations
	tests := []struct {
		name        string
		handlerOpts *slog.HandlerOptions
		logFunc     func(logger *slog.Logger)
		expected    map[string]any
	}{
		{
			name: "abbreviates level",
			logFunc: func(logger *slog.Logger) {
				logger.Info("test message")
			},
			expected: map[string]any{
				"level": "INF",
				"msg":   "test message",
			},
		},
		{
			name: "converts duration to seconds",
			logFunc: func(logger *slog.Logger) {
				logger.Info("duration test", "elapsed", 2500*time.Millisecond)
			},
			expected: map[string]any{
				"level":   "INF",
				"msg":     "duration test",
				"elapsed": 2.5,
			},
		},
		{
			name: "handles formattable error with verbose output",
			logFunc: func(logger *slog.Logger) {
				err := &formattableError{msg: "boom", detail: " with stack trace"}
				logger.Info("error occurred", "error", err)
			},
			expected: map[string]any{
				"level":        "INF",
				"msg":          "error occurred",
				"error":        "boom",
				"errorVerbose": "boom with stack trace",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			// Use provided handler options or default
			opts := tt.handlerOpts
			if opts == nil {
				opts = &slog.HandlerOptions{}
			}

			// Create a handler using the registered handler function
			handler := handlerFn("test", &buf, opts)
			logger := slog.New(handler)

			// Execute the log function
			tt.logFunc(logger)

			// Parse the JSON output
			var result map[string]any

			err := json.Unmarshal(buf.Bytes(), &result)
			require.NoError(t, err)

			// Verify expected fields are present and correct
			for key, expectedValue := range tt.expected {
				actualValue, ok := result[key]
				require.True(t, ok, "expected key %q not found in output", key)
				assert.Equal(t, expectedValue, actualValue, "value mismatch for key %q", key)
			}
		})
	}
}
