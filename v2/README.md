Flume [![GoDoc](https://godoc.org/github.com/ThalesGroup/flume?status.png)](https://godoc.org/github.com/ThalesGroup/flume/v2) [![Go Report Card](https://goreportcard.com/badge/github.com/ThalesGroup/flume)](https://goreportcard.com/report/ThalesGroup/flume) [![Build](https://github.com/ThalesGroup/flume/workflows/Build/badge.svg)](https://github.com/ThalesGroup/flume/actions?query=branch%3Amaster+workflow%3ABuild+)
=====

Flume is a handler for the log/slog package.  

# Usage

    go get github.com/ThalesGroup/flume/v2

Example:

    import github.com/ThalesGroup/flume/v2

    // configure logging from the environment, e.g.:
    // FLUME={"level":"INFO", "levels":{"http":"DEBUG"}, "handler":"json"}
    flume.MustConfigFromEnv()

    l := flume.New("http")  // equivalent to slog.New(flume.Default()).With(flume.LoggerKey, "http")
    l.Debug("hello world")


# Features

A flume handler has a couple nifty capabilities:

- It enables different levels based on the value of a special `logger` attribute in the record.
  For example, the default level might be INFO, but records with the attribute `logger=http` can be enabled at the debug level.  (The attribute name is configurable).
- Flume handlers forward records to another, "sink" Handler.  The sink can be changed at runtime,
  in an atomic, concurrency-safe way, even after loggers are created and while they are being used.  
  So you can switch from text to json at runtime without re-creating loggers, enable/disable the source
  attribute, or add/remove ReplaceAttr functions.
- Middleware: you can configure Handler middleware, which can do things like augmenting
  log records with additional attributes from the context.  As with the sink handler, middleware
  can be added or swapped at runtime.
- Flume's HandlerOptions can be configured from a JSON configuration spec, and has convenience 
  methods for reading this configuration from environment variables.
- Integrates with [github.com/ansel1/console-slog](https://github.com/ansel1/console-slog), providing
  a very human-friendly output format 

Migration from v1
-----------------

Flume v1 was based on zap internally, but its logging functions (e.g. `log.Info()`) had the same signatures as `slog.Logger`.

Migration steps:

- Replace flume imports with flume/v2
- Replace `flume.New(` with `flume.New(` (or with calls to `slog.New()` passing a flume/v2 handler)
- Replace `.IsDebug()` with `.Enabled(ctx, slog.LevelDebug)`
- Compiler errors: fix unmatched arguments to logging function (e.g. `l.Info("temp", temp)` -> `l.Info("temp", "value", temp)`
- Consider [zap2slog](https://github.com/ansel1/zap2slog) to do the transition incrementally

Contributing
------------

To build, be sure to have a recent go SDK, golangci-lint, and [just](https://github.com/casey/just).  Then run `just`.

Merge requests are welcome!