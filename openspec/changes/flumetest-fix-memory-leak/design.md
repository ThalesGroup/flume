## Context

`v2/flumetest` integrates `flume` with Go's `testing` package. `Start(t)`
installs a multiplex writer in front of the default flume handler so that
all log output written during a test is fanned out to (a) the original
writer (typically `os.Stdout`) and (b) a per-test capture buffer
(`subscriber`). When the test ends, `revert` decides whether to flush the
captured buffer to `t.Log`, write it to an artifact file, or discard it.

A user reported their test process growing to many GB of resident memory
when running hundreds of verbose tests. Audit findings:

1. **Closure retention** (`flumetest.go:189-237`): the `revert` closure
   captures `*subscriber` by reference and is registered with `t.Cleanup`.
   `*testing.T` retains its `cleanups []func()` slice for the lifetime of
   the test struct, and `*testing.T` values themselves (subtests included)
   are retained by the testing framework until the binary exits. So every
   per-test `subscriber` (and its `bytes.Buffer` payload) survives until
   the entire `go test` process completes — even when the test passed and
   the captured logs were intentionally discarded.

2. **Stale slice slot** (`mux.go:151-161`): `Unsubscribe` filters
   `m.state.records` in place via `newRecords := m.state.records[:0]`.
   This shrinks the slice but leaves stale `*subscriber` pointers in the
   trailing slots of the underlying array. The slot gets overwritten on
   the next `Subscribe`, but until then it pins the dead subscriber.

3. **Unbounded per-test buffer** (`mux.go:200-218`): each `subscriber`
   wraps an unbounded `bytes.Buffer`. A single chatty test can accumulate
   hundreds of MB before its capture is even released.

Combined, (1) and (3) explain GB-scale growth across hundreds of verbose
tests; (2) is a smaller correctness leak.

The package's existing configuration pattern (`Disabled`, `Verbose`,
`Artifacts`) uses sticky `*bool` globals primed lazily by `initialize()`
from environment variables, with command-line flag bindings via
`RegisterFlags()`. The new buffer-limit knob will follow the same shape
so users have a single mental model.

## Goals / Non-Goals

**Goals:**

- Eliminate retention of per-test capture buffers beyond the lifetime of
  `Start`'s `revert` function.
- Eliminate stale subscriber pointers in `multiplexWriter.state.records`.
- Cap per-test buffer growth at a configurable size (default 1 MiB) while
  preserving the most recent log content.
- Surface truncation events to the test output (via `t.Log`) only when
  the user is going to see the buffer (failure / verbose / artifact).
- Preserve the existing public API surface — additive only.

**Non-Goals:**

- Restructuring `Start`/`revert` so it does not capture `*subscriber` at
  all. `Free()` is a smaller, equivalent fix.
- Reviewing `flume.Handler.delegates` map growth, `Snapshot` writer
  retention, or other parts of the broader flume v2 module.
- Aligning truncation on log-record boundaries. The truncation notice
  warns that the prefix may be partial; we do not parse log records.
- Supporting unit suffixes beyond the binary/SI prefixes listed below.

## Decisions

### D1. Release buffers via `subscriber.Free()` + `defer` in `revert`

`subscriber` gains a `Free()` method that, under `s.mu`, sets `closed=true`
and nils `s.buf`. All buffer accessors (`Write`, `Len`, `String`,
`WriteTo`) become nil-safe: `Len`/`String`/`WriteTo` return zero-value
results when `s.buf == nil`; `Write` returns `len(p), nil` (matching the
existing closed-subscriber behavior).

`Start` adds `defer sub.Free()` as the first statement in `revert` after
the `ran.CompareAndSwap` guard. This guarantees `Free` runs after any
consume branch reads from the buffer, after any panic-recovery branch
re-raises, and even when `revert` is invoked twice (Cleanup + deferred
caller — the existing `ran` guard makes the second call a no-op so
`Free` runs exactly once).

**Alternatives considered:**

- **Restructure `Start`/`revert` to not capture `*subscriber`.** Move the
  consume-and-free logic into a separate function `finishCapture(sub,
  …)`; `revert` captures only an `*atomic.Pointer[subscriber]` it can
  swap to nil. Equivalent effect, larger diff, harder to review. Rejected
  in favor of `Free()`.
- **Drop `Cleanup` registration entirely** and rely on the deferred
  return value. Breaks the existing workaround for [golang/go#49929]
  where `t.Failed()` reads false inside Cleanup on panic. Not viable.

### D2. Use `slices.IndexFunc` + `slices.Delete` in `Unsubscribe`

Find the matching record with `slices.IndexFunc`, capture it for the
ancestor-re-enable logic, then remove it with `slices.Delete`. The stdlib
`slices.Delete` implementation zeros the obsolete trailing element,
dropping the `*subscriber` reference contained in `subscriberRecord.sub`.

```go
idx := slices.IndexFunc(m.state.records, func(r subscriberRecord) bool {
    return r.id == id
})
if idx == -1 {
    m.mu.Unlock()
    return
}
removed := m.state.records[idx]
m.state.records = slices.Delete(m.state.records, idx, idx+1)
```

This relies on the existing uniqueness invariant: `id` is assigned by
the monotonic `m.nextID++` under `m.mu`, so no two records ever share an
id. The "find one, remove one" shape mirrors that invariant directly.

**Alternatives considered:**

- **`slices.DeleteFunc` after a pre-scan to capture `removed`.**
  Equivalent retention behavior, but the predicate-based deletion
  semantics are misleading given the known-unique key. Rejected as
  slightly less clear.
- **Hand-rolled zero-fill after the existing in-place loop.** Equivalent,
  but requires reviewers to verify the manual nil-out. Rejected.
- **Switch records to a map keyed by id.** Larger refactor, changes
  iteration order semantics relied on by ancestor lookup. Rejected.

### D3. Bounded buffer via `bytes.Buffer` + post-write trim

After each `Write`, if `cap > 0 && buf.Len() > cap`, drop the oldest
`buf.Len() - cap` bytes via `buf.Next(buf.Len() - cap)` and accumulate
the count in `s.dropped`. Reads (`String`, `WriteTo`) return whatever
remains.

This is simpler than a bespoke ring buffer and adequate for the
write-many / read-once access pattern. Worst-case overhead is the
internal `bytes.Buffer` re-slicing cost, which is amortized O(1) per
trim because `Next` only advances the read offset.

**Alternatives considered:**

- **True ring buffer over a fixed `[]byte`.** Saves one `Next` call per
  write but requires linearizing on read and reimplementing `WriteTo`.
  Premature optimization. Rejected.
- **Cap by line count instead of bytes.** Requires parsing newlines
  during `Write`. Rejected — bytes are simpler and sufficient.
- **Reject any write that would exceed the cap.** Loses recent output,
  which is the part most likely to be relevant on failure. Rejected.

### D4. Cap captured at Subscribe time

`Subscribe` calls `BufferLimit()` once and stores the result on the new
`subscriber`. Concurrent calls to `SetBufferLimit` after a test starts
have no effect on that test. Rationale: avoids subtle behavior changes
mid-test, simplifies the `Write` hot path (no atomic load per call).

### D5. Configuration surface mirrors `Verbose`/`Artifacts`

Add globals and accessors:

```go
var bufferLimitPtr *int
func BufferLimit() int        { initialize(); return *bufferLimitPtr }
func SetBufferLimit(n int)    { initialize(); *bufferLimitPtr = n }
```

In `initialize()`, parse `FLUMETEST_BUFFER_LIMIT` only if
`bufferLimitPtr == nil` (i.e. not already set programmatically or via a
flag). On parse error, write a one-line warning to `os.Stderr` and use
the default of `1 << 20`.

We do **not** add a new command-line flag at this time. The
`RegisterFlags` API can grow one later if needed.

### D6. Env-var grammar for `FLUMETEST_BUFFER_LIMIT`

Parser `parseByteSize(s string) (int, error)`:

- Trim surrounding whitespace; lowercase.
- Match `^([0-9]+)\s*([a-z]*)$`.
- Number must be a non-negative integer that fits in `int`.
- Suffix lookup table:

  | suffix | multiplier |
  |--------|------------|
  | (empty) | 1          |
  | `b`    | 1          |
  | `k`, `kb`, `kib` | 1024 |
  | `m`, `mb`, `mib` | 1024 × 1024 |
  | `g`, `gb`, `gib` | 1024 × 1024 × 1024 |

- Treat `KB`/`KiB` (and `MB`/`MiB`, `GB`/`GiB`) as identical at 1024.
  This is the common dev-tooling convention (`docker`, `journalctl`,
  Kubernetes resource limits) and avoids forcing users to remember the
  base-10 vs base-2 distinction. Document this choice in the package
  comment.
- Reject unknown suffixes, fractional numbers, negative numbers,
  overflow, and the empty string with descriptive errors.

### D7. Truncation notice via `t.Log` only, once per test

When `revert` is going to surface the buffer (failure path, verbose
path, or artifact-write path) AND `sub.Dropped() > 0`, emit exactly one
line via `t.Log`:

```
flumetest: log buffer truncated; <N> bytes dropped (FLUMETEST_BUFFER_LIMIT=<limit>)
```

`<N>` is the cumulative total of bytes dropped during the test. The
notice is emitted **once per test**, from `revert`, regardless of how
many `Write` calls triggered truncation (a single chatty test can
trigger thousands of trims; users see exactly one summary line, not a
flood). `subscriber.Write` does not emit anything; its only effect on
truncation is to advance the `dropped` counter.

The notice goes through `t.Log` so it is interleaved with other test
output rather than mingled with captured log records. The captured
buffer and any artifact file remain pure log content — important for
log-parsing tooling and for diff-stable artifact contents.

When the buffer is silently discarded (passing test with verbose off
and artifacts off), no notice is emitted regardless of `Dropped()`.

**Alternatives considered:**

- **Live notice on every truncating `Write`.** Rejected: spammy
  (potentially thousands of lines per test) and adds I/O to the hot
  path. The end-of-test summary is just as informative.
- **Threshold-based notice (e.g. log every Nth dropped byte).**
  Rejected: still on the hot path, and the cumulative end-of-test
  count already gives the user the impact figure.

## Risks / Trade-offs

- **[Behavior change: default 1 MiB cap]** → Mitigation: opt-out via
  `FLUMETEST_BUFFER_LIMIT=0`; truncation notice tells users when the
  prefix was lost; release notes call this out.
- **[`Free()` makes accessors zero-valued, changing post-revert API
  semantics]** → Mitigation: today no caller invokes accessors after
  `revert` returns; document `Free()` as "call only after consume is
  complete"; `Free()` is unexported (lowercase method on unexported
  type), so external API is unchanged.
- **[Stderr warning on bad env var could noise CI logs]** → Mitigation:
  one line, prefixed `flumetest:`, only emitted once during
  `initialize()`.
- **[Per-write trim adds work to the hot path]** → Mitigation: trim is
  amortized O(1); cap-of-zero short-circuits; benchmarks would confirm.
  Can revisit with a ring buffer if profiling shows hotspot.
- **[Cap captured at Subscribe time means `SetBufferLimit` does not
  affect running tests]** → Mitigation: documented; matches the
  `Verbose`/`Artifacts` "configure before running" expectation.
- **[Three commits stacked via jj could complicate revert]** → Mitigation:
  each commit is independently meaningful and includes its own tests, so
  any single revision can be `jj abandon`ed without breaking the others.

## Migration Plan

No data migration. Rollout is normal version bump of `v2/flumetest`.

Suggested release notes line:

> `flumetest`: per-test capture buffers are now released as soon as
> `Start`'s revert runs, and capped at 1 MiB by default. Set
> `FLUMETEST_BUFFER_LIMIT` (e.g. `2MiB`, `0` for unlimited) or call
> `flumetest.SetBufferLimit(n)` to override. When truncation occurs,
> a notice is emitted via `t.Log`.

Rollback: revert the relevant jj revision(s). The three fixes are
independent and can be rolled back individually.

## Open Questions

- Should we add a `RegisterFlags` flag (`-buffer-limit`) in a follow-up?
  Defer until requested.
- Should the truncation notice include a relative pointer (e.g. "first
  N bytes lost")? Current format reports dropped-bytes count, which is
  unambiguous; keep simple for v1.

[golang/go#49929]: https://github.com/golang/go/issues/49929
