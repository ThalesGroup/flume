//nolint:err113
package flume

import (
	"errors"
	"log/slog"
	"time"
)

//nolint:testableexamples
func ExampleNew() {
	ogOpts := Default().HandlerOptions()
	defer Default().SetHandlerOptions(ogOpts)

	// The default handler is a noop handler that discards all log messages.
	// To enable logging, using a default slog text handler writing to os.Stdout,
	// call Default().SetHandlerOptions(nil)
	Default().SetHandlerOptions(nil)

	// This creates a named logger with the name "my-app", using the default handler.
	log := New("my-app")
	log.Info("Hello, World!")

	// Output will be something like:
	// time=2025-02-26T09:47:59.129-06:00 level=INFO msg="Hello, World!" logger=my-app
}

func ExampleTermHandler() {
	fixedTime := time.Date(2024, 1, 1, 20, 41, 28, 515*1e6, time.UTC)

	l := slog.New(NewHandler(nil, &HandlerOptions{
		Level:     LevelInfo,
		HandlerFn: LookupHandlerFn(TermHandler),
		ReplaceAttrs: []func([]string, slog.Attr) slog.Attr{
			func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					a.Value = slog.AnyValue(fixedTime)
				}
				return a
			},
		},
	}).Named("main"))

	l.Info("Hello, World!")
	l.Warn("High temp", "temp", 30)
	l.Error("Failed to read file", "error", errors.New("file not found"))

	l.With(LoggerKey, "server").Info("Request received", "method", "GET", "url", "/users/123")

	// Output:
	// 20:41:28.515 main     INF | Hello, World!
	// 20:41:28.515 main     WRN | High temp temp=30
	// 20:41:28.515 main     ERR | Failed to read file error=file not found
	// 20:41:28.515 server   INF | Request received method=GET url=/users/123
}
