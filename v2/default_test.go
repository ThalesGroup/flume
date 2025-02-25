package flume

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []handlerTest{
		{
			name: "New",
			handlerFn: func(_ *bytes.Buffer) slog.Handler {
				return New("blue").Handler()
			},
			want: "level=INFO msg=hi logger=blue\n",
		},
		{
			name: "Default",
			handlerFn: func(_ *bytes.Buffer) slog.Handler {
				h := Default()
				// need something to trigger rebuilding the handlers
				// so the captured stdout is correct
				h.SetOut(nil)
				return h
			},
			want: "level=INFO msg=hi\n",
		},
	}
	for _, tt := range tests {
		tt.stdout = true
		tt.Run(t)
	}
}
