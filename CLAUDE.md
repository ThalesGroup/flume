# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Flume is a Go structured logging library. This is a multi-version monorepo:
- **Root directory**: Flume v1 (built on `go.uber.org/zap`)
- **v2/ directory**: Flume v2 (ground-up rewrite as a `slog.Handler`, minimal dependencies)
- **go.work**: Ties both modules together as a Go workspace

v2 is actively developed and near release. v1 is mature/legacy.

## Build Commands

### v1 (root directory, uses Make)

| Task | Command |
|------|---------|
| All | `make all` |
| Build | `make build` |
| Test | `make test` |
| Lint | `make lint` |
| Format | `make fmt` |
| Coverage | `make cover` (output: `build/coverage.html`) |

### v2 (v2/ directory, uses just)

| Task | Command |
|------|---------|
| All | `just all` |
| Build | `just build` |
| Test | `just test` |
| Single test | `just tests -run TestName` |
| Lint | `just lint` |
| Format | `just fmt` |
| Coverage | `just cover` (output: `build/coverage.html`) |

Both use `go test -race` by default. Linting uses `golangci-lint`.

## Architecture

### v2 Core (primary development target)

- **`handler.go`**: Core `Handler` type implementing `slog.Handler`. Wraps an inner handler with atomic swapping for runtime reconfiguration.
- **`handler_options.go`**: `HandlerOptions` config and factory functions (`NewHandler`, `NewJSONHandler`, `NewTextHandler`, etc.).
- **`default.go`**: Global default handler singleton.
- **`middleware.go`**: Middleware chain support for processing log records.
- **`config_from_env.go`**: Configuration via `FLUME` environment variable (JSON or levels string).
- **`adapter.go`**: Bridge between slog and zap cores (for v1 interop).
- **`v1_compat.go`**: v1 compatibility layer and migration tools.
- **`flumetest/`**: Test helpers for capturing/asserting log output.

### Key Design Principles

- **Silent by default**: Logs are discarded unless explicitly configured (library-friendly).
- **Named loggers**: Per-logger level configuration via a `logger` attribute key.
- **Thread-safe**: Atomic handler swapping for runtime reconfiguration.
- **Context integration**: Loggers bound to `context.Context`.

### Handler Types

`TextHandler`, `JSONHandler`, `TermHandler`, `TermColorHandler`, `NoopHandler`

## Testing

- Uses `github.com/stretchr/testify` for assertions.
- Both modules require Go 1.21+.
- CI tests against multiple Go versions (oldstable, stable).


