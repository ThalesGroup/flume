package flumev1compat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/ThalesGroup/flume/v2"
)

// These are helpers to configure v2 to behave like v1.

// V1JSONHandler configures the default json to behave like flume v1:
// - abbreviations for levels
// - durations in seconds
// - "errorsVerbose" key for formattable errors
func V1JSONHandler() {
	// Alters the default JSON handler to mimic the behavior of yugolog/flumev1/zap
	flume.RegisterHandlerFn(flume.JSONHandler, func(_ string, w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		opts.ReplaceAttr = flume.ChainReplaceAttrs(opts.ReplaceAttr, flume.AbbreviateLevel, flume.SecondsDuration(), V1VerboseErrors)
		return slog.NewJSONHandler(w, opts)
	})
}

// V1VerboseErrors is a ReplaceAttr function which is meant to mimic how errors were rendered in flumev1.
// In flumev1, an error which also implemented fmt.Formatter would result in two attributes being added
// to the log message: "error" and "errorVerbose".  "error" would only contain the error's message, and
// "errorVerbose" would contain the error object formatted with fmt.Sprintf(), which typically includes
// the error's stacktrace or other metadata.
//
// flumev2 has a DetailedErrors function, but that just replaces the "error" value in place with the formatted
// error value.  We need something different to mimic flumev1/zap.  See zap.NamedError() for more details.
func V1VerboseErrors(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() != slog.KindAny {
		return a
	}

	if err, ok := a.Value.Any().(error); ok {
		if _, ok := err.(fmt.Formatter); ok {
			if _, ok = err.(json.Marshaler); !ok {
				// this is an error which implements fmt.Formatter, and does not
				// implement json.Marshaler.  So we'll turn this into two attributes:
				// one with just the message, and the other with the details.
				// We turn one attribute into two by returning a group Attr with an empty Key,
				// which causes the members of the group to be inlined.
				// Careful not to include the original error: ReplaceAttr functions are called recursively
				// on the members of this group, so if we return the original error, we get a StackOverflow
				return slog.Group("", slog.Any("error", errors.New(err.Error())), slog.String("errorVerbose", fmt.Sprintf("%+v", err))) //nolint:err113
			}
		}
	}

	return a
}
