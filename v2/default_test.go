package flume

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []handlerTest{
		{
			// this is a bit of a hack to test that the default handler is a noop handler
			// for the very first test, the test loop will not configure the default handler,
			// so it should be a noop handler
			name: "default noop",
			handlerFn: func(_ *bytes.Buffer) slog.Handler {
				return Default()
			},
			want: "",
		},
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
				// the test is will have replaced os.Stdout, but the default handler
				// is still writing to the old os.Stdout
				h.SetOut(nil)
				return h
			},
			want: "level=INFO msg=hi\n",
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if i != 0 {
				ogOpts := Default().HandlerOptions()
				defer Default().SetHandlerOptions(ogOpts)

				Default().SetHandlerOptions(nil)
			}
			tt.stdout = true
			tt.Run(t)
		})
	}
}
