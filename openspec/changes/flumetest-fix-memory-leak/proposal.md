## Why

`flumetest.Start(t)` retains every per-test capture buffer for the entire lifetime
of the test binary, even when the test passes and the captured logs are
intentionally discarded. In suites with hundreds of verbose tests, this
linearly accumulates into many gigabytes of resident memory, sometimes
crashing the test process. The buffer is also unbounded per test, so a single
chatty test can consume an arbitrary amount of memory before its capture is
even released.

## What Changes

- Add `subscriber.Free()` and call it via `defer` at the top of `Start`'s
  `revert` function so each test's capture buffer is released as soon as
  `revert` finishes consuming it. Buffer accessors (`Write`, `Len`, `String`,
  `WriteTo`) become nil-safe.
- Replace the in-place filter in `multiplexWriter.Unsubscribe` with
  `slices.DeleteFunc` so trailing slots in the records slice's backing array
  no longer retain stale `*subscriber` pointers.
- Add a configurable per-subscriber buffer cap that retains only the most
  recent N bytes:
  - New env var `FLUMETEST_BUFFER_LIMIT` accepting human-friendly sizes
    (`1MB`, `512KiB`, `1048576`, etc.). Default `1MiB`. `0` means unlimited
    (preserves prior behavior).
  - New programmatic accessors `BufferLimit() int` and
    `SetBufferLimit(int)` mirroring the existing `Disabled`/`Verbose`/
    `Artifacts` pattern.
  - On invalid env-var value, write a one-line warning to `os.Stderr` and
    fall back to the default.
  - When a test's buffer was truncated, `revert` emits a single
    `t.Log("flumetest: log buffer truncated; N bytes dropped …")` line. The
    notice is **not** written into the captured buffer or the artifact file.

## Capabilities

### New Capabilities

- `flumetest-capture`: Per-test log capture lifecycle, buffer release,
  subscriber bookkeeping in the multiplex writer, and the configurable
  capture buffer cap.

### Modified Capabilities

<!-- none; flumetest had no prior spec -->

## Impact

- **Code**: `v2/flumetest/flumetest.go`, `v2/flumetest/mux.go`, plus tests
  in `v2/flumetest/flumetest_test.go` and `v2/flumetest/mux_test.go`.
- **Public API**: additive only — new `BufferLimit()` / `SetBufferLimit(int)`
  exported functions; new `FLUMETEST_BUFFER_LIMIT` env var. No existing
  signatures change.
- **Behavior change**: captured per-test log buffers are now capped at
  1 MiB by default. Tests producing more than 1 MiB of log output will
  see the prefix dropped; the most recent 1 MiB is preserved. Opt out
  with `FLUMETEST_BUFFER_LIMIT=0`. A `t.Log` notice is emitted whenever
  truncation occurred and the buffer is being surfaced.
- **No changes** to `Snapshot`, `AddTestNameToLogs`, the artifacts-dir
  code path, or the `flume.Handler.delegates` map.
