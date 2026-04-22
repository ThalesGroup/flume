Each numbered group below corresponds to one `jj` revision. After
completing a group, run `cd v2 && just` (which runs `fmt tidy build
lint test`), `jj describe -m "<msg>"`, then `jj new` before starting
the next group.

## 1. Revision 1 — Release per-test buffers after revert

- [x] 1.1 In `v2/flumetest/mux.go`, add `func (s *subscriber) Free()` that sets `s.closed.Store(true)`, then under `s.mu` sets `s.buf = nil`.
- [x] 1.2 In `v2/flumetest/mux.go`, make `subscriber.Write`, `Len`, `String`, and `WriteTo` nil-safe on `s.buf` (treat `nil` as zero-length: `Write` returns `len(p), nil`; `Len` returns `0`; `String` returns `""`; `WriteTo` returns `0, nil`).
- [x] 1.3 In `v2/flumetest/flumetest.go` `Start`, add `defer sub.Free()` as the first statement of `revert` after the `ran.CompareAndSwap` guard.
- [x] 1.4 Add unit test `TestSubscriber_Free_is_safe` in `v2/flumetest/mux_test.go` covering: Free is idempotent; post-Free `Write` is no-op returning `len(p), nil`; `Len`/`String`/`WriteTo` return zero values; concurrent `Free` + accessor calls are race-free under `-race`.
- [x] 1.5 Add unit test `TestStart_releases_buffer_after_revert` in `v2/flumetest/flumetest_test.go` that asserts `sub.buf == nil` (or equivalently `sub.Len() == 0` after a write) after `revert` runs. Use a fake `testingTB` if needed to access the subscriber, or expose a small test hook.
- [x] 1.6 Run `cd v2 && just`; resolve any failures (covers fmt, tidy, build, lint, test).
- [x] 1.7 `jj describe -m "fix(flumetest): release per-test buffers after revert"` then `jj new`.

## 2. Revision 2 — Drop stale subscriber refs in mux records

- [x] 2.1 In `v2/flumetest/mux.go`, add `import "slices"` if not already imported.
- [x] 2.2 In `v2/flumetest/mux.go` `Unsubscribe`: (a) call `slices.IndexFunc` to find the index of the record with matching `id`; (b) early-return if `-1`; (c) capture `removed := m.state.records[idx]` for the ancestor-re-enable logic; (d) `m.state.records = slices.Delete(m.state.records, idx, idx+1)`.
- [x] 2.3 Verify the existing ancestor re-enable logic still operates against the updated `m.state.records` and that the removed record's ancestor lookup is unaffected by deletion ordering.
- [x] 2.4 Add unit test `TestUnsubscribe_clears_backing_array_refs` in `v2/flumetest/mux_test.go` that subscribes N tests, unsubscribes one, then peeks into the backing array via `records[:cap(records)]` and asserts the trailing slot has zero-value `subscriberRecord` (i.e. `sub == nil`).
- [ ] 2.5 Run `cd v2 && just`; resolve any failures (covers fmt, tidy, build, lint, test).
- [ ] 2.6 `jj describe -m "fix(flumetest): clear stale subscriber refs in mux records"` then `jj new`.

## 3. Revision 3 — Configurable bounded buffer (default 1 MiB)

- [ ] 3.1 In `v2/flumetest/flumetest.go`, add a global `bufferLimitPtr *int` alongside `disabledPtr`/`verbosePtr`/`artifactsPtr`.
- [ ] 3.2 Add exported `BufferLimit() int` and `SetBufferLimit(n int)` mirroring the existing `Verbose`/`Artifacts` accessors (call `initialize()` first, then read/write `*bufferLimitPtr`).
- [ ] 3.3 Add helper `parseByteSize(s string) (int, error)` supporting bare integers and case-insensitive suffixes `B`, `K`/`KB`/`KiB`, `M`/`MB`/`MiB`, `G`/`GB`/`GiB`. Treat `KB`==`KiB`==1024 (and likewise `MB`/`MiB`, `GB`/`GiB`). Trim whitespace; reject negatives, fractions, overflow, and unknown suffixes.
- [ ] 3.4 In `initialize()`, when `bufferLimitPtr == nil`, parse `os.Getenv("FLUMETEST_BUFFER_LIMIT")`. On success store the value; on parse error write a one-line warning to `os.Stderr` (`fmt.Fprintf(os.Stderr, "flumetest: invalid FLUMETEST_BUFFER_LIMIT=%q: %v; using default %d\n", raw, err, defaultLimit)`) and store the default. Default = `1 << 20` (1 MiB).
- [ ] 3.5 In `v2/flumetest/mux.go`, add `cap int` and `dropped int` fields to `subscriber`. Modify `newSubscriber` to take `cap int`. Update `Subscribe` to pass `BufferLimit()` to `newSubscriber`.
- [ ] 3.6 In `subscriber.Write`, after writing, if `s.cap > 0 && s.buf.Len() > s.cap`, compute `n := s.buf.Len() - s.cap`, advance via `s.buf.Next(n)`, and add `n` to `s.dropped`. Hold `s.mu` while doing this.
- [ ] 3.7 Add `Dropped() int` accessor on `subscriber` (read under `s.mu`).
- [ ] 3.8 Update `v2/flumetest/flumetest.go` package doc comment to document `FLUMETEST_BUFFER_LIMIT`, the size grammar, and the default of 1 MiB.
- [ ] 3.9 In `Start`'s `revert`, after the existing consume branches (failure / verbose flush to `t.Log`, or artifact write), if `sub.Dropped() > 0` AND the buffer was surfaced (failure / verbose / artifact), call `t.Log("flumetest: log buffer truncated; ", sub.Dropped(), " bytes dropped (FLUMETEST_BUFFER_LIMIT=", BufferLimit(), ")")`. Do NOT emit when the buffer was silently discarded. Ensure the notice is emitted regardless of whether the artifact-write or t.Log path was taken.
- [ ] 3.10 Add unit tests in `v2/flumetest/flumetest_test.go` for the byte-size parser: `TestParseByteSize_bytes`, `TestParseByteSize_units` (covers all suffix variants including case-insensitive and whitespace), `TestParseByteSize_invalid` (unknown suffix, fractional, negative, empty, overflow).
- [ ] 3.11 Add `TestBufferLimit_defaultIs1MiB`, `TestBufferLimit_zeroMeansUnlimited`, `TestSetBufferLimit_overridesEnv`, `TestBufferLimit_envVar` (with multiple suffix variants), `TestBufferLimit_invalidEnvVarWarnsAndUsesDefault` (assert stderr output via redirection) in `v2/flumetest/flumetest_test.go`.
- [ ] 3.12 Add `TestSubscriber_capRetainsMostRecentBytes` in `v2/flumetest/mux_test.go`: with `cap=1024`, write 4096 bytes, assert `String()` returns the last 1024 bytes and `Dropped() == 3072`.
- [ ] 3.13 Add `TestSubscriber_capCapturedAtSubscribeTime` in `v2/flumetest/mux_test.go` covering D4: subscribe, then change `SetBufferLimit`, verify the active subscriber's effective cap did not change.
- [ ] 3.14 Add `TestRevert_logsTruncationNoticeViaTLog` (with truncation + failure; assert `t.Log` saw the notice and the captured buffer / artifact does NOT contain the notice) and `TestRevert_noNoticeWhenNoTruncation` and `TestRevert_noNoticeWhenSilentlyDiscarded` in `v2/flumetest/flumetest_test.go`.
- [ ] 3.15 Run `cd v2 && just`; resolve any failures (covers fmt, tidy, build, lint, test).
- [ ] 3.16 `jj describe -m "feat(flumetest): cap captured log buffer with FLUMETEST_BUFFER_LIMIT (default 1 MiB)"`.
