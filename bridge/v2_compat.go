// Package bridge routes all logs from log, log/slog, github.com/gemalto/flume,
// and github.com/ThalesGroup/flume/v2 to a single logging backend: either
// github.com/gemalto/flume or github.com/ThalesGroup/flume/v2.
//
// This is useful for code bases which use a mix of logging packages, or
// are in the process of transitions from flume to flume/v2.
//
// Call either ToV1() or ToV2() early in your programs main function.
package bridge

import (
	"io"
	"log/slog"

	flumev2 "github.com/ThalesGroup/flume/v2"
	"github.com/ansel1/zap2slog"
	flumev1 "github.com/gemalto/flume"
	"go.uber.org/zap/zapcore"
)

// ToV1 routes all logging to github.com/gemalto/flume, e.g.:
//
//	import (
//	  flumev1 "github.com/gemalto/flume"
//	  flumev2 "github.com/ThalesGroup/flume/v2"
//	  "github.com/gemalto/flume/bridge"
//	)
//
//	func main() {
//	  flumev1.ConfigFromEnv()
//	  bridge.ToV1()
//
//	  v1Log := flumev1.New("main")
//	  v2Log := flumev2.New("main")
//
//	  // both these call will ultimately be handled, formatted,
//	  // and output by flumev1/zap
//	  v1Log.Info("hi")
//	  v2Log.InfoContext("hi")
//	}
func ToV1() error {
	// By calling this method, we're signaling that we intend to use flumev1/zap
	// as the final logging destination.  So configure flumev2 to redirect all its
	// logs to flumev1.  Make sure flumev1 isn't redirecting back to flumev2
	flumev1.DefaultFactory().SetNewCoreFn(nil)
	flumev2.Default().SetHandlerOptions(&flumev2.HandlerOptions{
		HandlerFn: func(name string, _ io.Writer, opts *slog.HandlerOptions) slog.Handler {
			zc := flumev1.NewCore(name).ZapCore()
			return zap2slog.NewZapHandler(zc, &zap2slog.ZapHandlerOptions{
				AddSource:     opts.AddSource,
				LoggerNameKey: flumev2.LoggerKey,
				ReplaceAttr:   opts.ReplaceAttr,
			})
		},
	})

	// Configure the slog/log package to redirect to flume (slog -> flumev2 -> flumev1)
	slog.SetDefault(flumev2.New("slog"))

	return nil
}

// ToV2 routes all logging to github.com/gemalto/flume, e.g.:
//
//	import (
//	  flumev1 "github.com/gemalto/flume"
//	  flumev2 "github.com/ThalesGroup/flume/v2"
//	  "github.com/gemalto/flume/bridge"
//	)
//
//	func main() {
//	  flumev2.ConfigFromEnv()
//	  bridge.ToV2()
//
//	  v1Log := flumev1.New("main")
//	  v2Log := flumev2.New("main")
//
//	  // both these call will ultimately be handled, formatted,
//	  // and output by flumev2/slog
//	  v1Log.Info("hi")
//	  v2Log.InfoContext("hi")
//	}
func ToV2() error {
	// By calling this method, we're signaling that we intend to use flumev2/slog
	// as the final logging destination.  So configure flumev1 redirect all its
	// logs to flumev2.
	flumev1.SetAddCaller(flumev2.Default().HandlerOptions().AddSource)

	// sanity test: if ToV1() has already been called, then setting the flumev1 core function will
	// cause a loop (v1 will call v2's HandlerFn, which will call back to the NewCoreFn).
	// try to detect this
	handlerFn := flumev2.Default().HandlerOptions().HandlerFn
	if handlerFn != nil {
		h := handlerFn("test", io.Discard, &slog.HandlerOptions{})
		if _, ok := h.(*zap2slog.ZapHandler); ok {
			// set handlerFn to nil first, to avoid a loop
			opts := flumev2.Default().HandlerOptions()
			opts.HandlerFn = nil
			flumev2.Default().SetHandlerOptions(opts)
		}
	}

	flumev1.DefaultFactory().SetNewCoreFn(func(name string, _ zapcore.Encoder, _ zapcore.WriteSyncer, _ zapcore.LevelEnabler) zapcore.Core {
		return zap2slog.NewSlogCore(flumev2.Default().Named(name), &zap2slog.SlogCoreOptions{LoggerNameKey: flumev2.LoggerKey})
	})

	// Configure the slog/log package to redirect to flumev2
	slog.SetDefault(flumev2.New("slog"))

	return nil
}
