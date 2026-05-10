# Plan: Make repeated `flumetest.Start` flush and restart same-test capture

## Context

- Scope: **v2 only** (`v2/flumetest`). The legacy root `flumetest` package should not be changed.
- `v2/flumetest.Start(t)` currently calls `globalMux.Subscribe(t.Name())`.
- If a subscriber for the same `t.Name()` is already active, `Subscribe` reports `existing=true`; `Start` logs `flumetest: Start already active...` and returns a no-op cleanup.
- Desired behavior: a repeated `Start()` in the **same test** should flush the currently active same-test buffer, then begin a fresh buffer for subsequent logs.
- Existing subtest behavior must remain: the first `Start()` in a child test should suspend the parent capture and activate a child capture, resuming the parent when the child cleanup runs.

## Approach

- Treat a repeated `Start()` for the same `t.Name()` as a capture rollover instead of a no-op.
- Add/refactor a mux operation that atomically replaces the active same-name subscriber with a fresh subscriber, closes/removes the old subscriber from future snapshots, and returns the old subscriber for flushing outside the mux lock.
- Reuse the existing cleanup/flush behavior so rollover flushing matches normal end-of-capture flushing:
  - discard when the test has not failed and verbose/artifact output does not require surfacing;
  - write to `t.Log` or artifact output when the test is already failing, panicking, or verbose as today;
  - specifically cover the case where `t.Failed()` is already true before the second same-test `Start()`: the first capture's buffered logs must be surfaced immediately on rollover, either through `t.Log` when artifacts are off or through artifact output when artifacts are on;
  - emit the existing truncation notice when surfaced output was truncated.
- Preserve returned cleanup semantics: an old cleanup retained by `defer` should become harmless after its buffer was rolled over/flushed, while the newest cleanup controls the active replacement subscriber.
- Keep nested/subtest semantics based on test-name ancestry unchanged.

## Files to modify

- `v2/flumetest/flumetest.go`
- `v2/flumetest/mux.go`
- `v2/flumetest/flumetest_test.go`
- `v2/flumetest/mux_test.go`

## Reuse

- `Start` cleanup logic in `v2/flumetest/flumetest.go` for unsubscribe, failure/panic detection, artifact writing, `t.Log`, truncation notice, and buffer freeing.
- `multiplexWriter.Subscribe` / `Unsubscribe` and ancestry helpers in `v2/flumetest/mux.go`.
- Existing tests:
  - `TestDoubleStartIsNoOp` documents the behavior to replace.
  - `TestNestedSuppression` protects parent/child subtest behavior.
  - `TestParallelOverlapCaptures` protects sibling overlap fan-out.

## Steps

- [ ] Refactor `Start` so buffer surfacing is in a helper usable by both cleanup and same-test rollover. Keep panic recovery directly in the returned cleanup so deferred `recover()` semantics remain intact.
- [ ] Add a mux operation for replacing an active same-name subscriber with a new subscriber while preserving ancestry/subtest suppression semantics. The operation should distinguish:
  - same-name active capture: replace and return the old subscriber to flush;
  - child/subtest capture: keep existing ancestor-suspension behavior;
  - sibling/parallel capture: keep fan-out behavior.
- [ ] Remove the same-test warning/no-op path in `Start`.
- [ ] Ensure rollover closes/removes the old subscriber before flushing it, then frees it after surfacing/discarding so retained old cleanup closures do not double-output.
- [ ] Update tests: replace `TestDoubleStartIsNoOp` with expectations for flushing/discarding previous buffer and starting a fresh active buffer.
- [ ] Add/adjust tests for success-state rollover, verbose rollover if needed, idempotent old cleanup behavior, and unchanged subtest suppression.
- [ ] Add explicit failure-state rollover tests for a second same-test `Start()` after the test is already marked failed:
  - artifacts disabled: logs buffered by the first `Start()` are flushed to `t.Log` when the second `Start()` happens;
  - artifacts enabled: logs buffered by the first `Start()` are flushed to artifact output when the second `Start()` happens, accepting either a distinct artifact file per `Start()` in the same artifact directory or appending to one per-test artifact file.
- [ ] Update comments in `v2/flumetest/flumetest.go` and `v2/flumetest/mux.go` that currently describe duplicate same-test `Start` as a no-op.

## Verification

- Run focused tests for `v2/flumetest`, especially `TestDoubleStart...`, the new already-failing second-`Start()` tests with artifacts both disabled and enabled, `TestNestedSuppression`, `TestParallelOverlapCaptures`, artifact tests, and buffer-limit tests.
- Run `go test ./v2/flumetest` or the module-appropriate equivalent.
- Run broader `go test ./...` if feasible from repo root/workspace.
