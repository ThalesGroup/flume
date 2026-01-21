package flume

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggerFuncWriter_Write(t *testing.T) {
	testcases := []struct {
		name string
		msg  string
	}{
		{
			name: "newline",
			msg:  "hello world\n",
		},
		{
			name: "no newline",
			msg:  "hello world",
		},
		{
			name: "long message",
			msg:  strings.Repeat("a", 2000),
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer

			lw := LoggerFuncWriter(func(msg string, _ ...any) {
				buf.WriteString(msg)
			})

			n, err := lw.Write([]byte(tc.msg))
			require.NoError(t, err, "Write failed")
			assert.Equal(t, len(tc.msg), n, "Write returned incorrect length")
			assert.Equal(t, tc.msg, buf.String(), "logger func not called with correct message")
		})
	}
}

func TestLogFuncWriter_Write(t *testing.T) {
	testcases := []struct {
		name      string
		msg       string
		trimSpace bool
		expected  string
	}{
		{
			name:      "newline",
			msg:       "hello world\n",
			trimSpace: false,
			expected:  "hello world\n",
		},
		{
			name:      "no newline",
			msg:       "hello world",
			trimSpace: false,
			expected:  "hello world",
		},
		{
			name:      "long message",
			msg:       strings.Repeat("a", 2000),
			trimSpace: false,
			expected:  strings.Repeat("a", 2000),
		},
		{
			name:      "trim space",
			msg:       "  hello world  \n",
			trimSpace: true,
			expected:  "hello world",
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer

			lw := LogFuncWriter(func(args ...any) {
				fmt.Fprint(&buf, args...)
			}, tc.trimSpace)

			n, err := lw.Write([]byte(tc.msg))
			require.NoError(t, err, "Write failed")
			assert.Equal(t, len(tc.msg), n, "Write returned incorrect length")
			assert.Equal(t, tc.expected, buf.String(), "logger func not called with correct message")
		})
	}
}
