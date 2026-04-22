## ADDED Requirements

### Requirement: Per-test capture buffer release

`flumetest.Start(t)` SHALL release the per-test capture buffer's backing
storage as soon as the returned `revert` function completes, regardless of
whether the buffer was flushed to `t.Log`, written to an artifact file, or
discarded. After release, subsequent reads from the same subscriber MUST
return zero-length output and writes MUST be silent no-ops.

#### Scenario: Passing test with verbose off and artifacts off

- **WHEN** a test calls `flumetest.Start(t)`, writes log output, and passes
- **AND** `Verbose()` is false and `Artifacts()` is false
- **THEN** after `revert` runs, the subscriber's buffer storage MUST be
  released so that the captured bytes are eligible for garbage collection
  even though the `t.Cleanup` closure that captured `*subscriber` is still
  retained by the testing framework

#### Scenario: Failing test that flushes to t.Log

- **WHEN** a test fails and `revert` flushes the buffer via `t.Log`
- **THEN** after `revert` returns, the subscriber's buffer storage MUST be
  released and subsequent calls to `String()` / `WriteTo` / `Len` MUST
  behave as if the buffer were empty

#### Scenario: Subscriber method safety after release

- **WHEN** `Free()` has been called on a subscriber
- **AND** another goroutine concurrently calls `Write`, `Len`, `String`, or
  `WriteTo`
- **THEN** those calls MUST NOT panic, MUST return zero-length results
  (or report `len(p)` written for `Write`), and MUST be safe under the
  race detector

### Requirement: Mux records release stale subscriber references

`multiplexWriter.Unsubscribe(id)` SHALL NOT retain any reference to the
removed subscriber in the records slice's backing array after the call
returns. The slice's underlying array MUST be cleared at the trailing
slot(s) so that no `*subscriber` pointer survives in unreachable storage.

#### Scenario: Unsubscribe leaves no stale pointer in trailing array slot

- **WHEN** `Subscribe` is called N times producing records `r0…r(N-1)`
- **AND** `Unsubscribe(rK.id)` is then called for some `K`
- **THEN** the trailing slot of the records slice's backing array (i.e.
  the slot at index `len(records)` after deletion) MUST contain a
  zero-value `subscriberRecord` with a nil `sub` field

### Requirement: Configurable capture buffer cap

Each per-test capture buffer SHALL respect a configurable maximum size in
bytes. When more than the cap's worth of bytes are written during a test,
the buffer MUST retain only the most recent `cap` bytes; older bytes MUST
be discarded. The cap MUST be configurable in three ways, in order of
precedence:

1. Programmatic `flumetest.SetBufferLimit(n int)` — highest precedence.
2. Env var `FLUMETEST_BUFFER_LIMIT` — read once during `initialize()` if
   the limit was not already set programmatically.
3. Built-in default of `1 MiB` (1048576 bytes).

A cap value of `0` MUST mean "unlimited" (no truncation). Negative values
MUST be rejected (treated as parse errors).

The env var MUST accept a human-friendly size grammar (case-insensitive,
optional whitespace between number and unit):

- bare integer → bytes (e.g., `1048576`)
- suffixes: `B`, `K`/`KB`/`KiB`, `M`/`MB`/`MiB`, `G`/`GB`/`GiB`
- `KB` and `KiB` MUST be treated identically as 1024 (likewise for `MB`/`MiB` and `GB`/`GiB`)

The cap in effect for a particular test MUST be captured at `Subscribe`
time so that mutating the limit mid-suite does not affect already-running
tests.

#### Scenario: Default limit is 1 MiB

- **WHEN** neither `FLUMETEST_BUFFER_LIMIT` is set nor `SetBufferLimit` has
  been called
- **THEN** `BufferLimit()` MUST return `1048576`

#### Scenario: Zero means unlimited

- **WHEN** `SetBufferLimit(0)` is called (or `FLUMETEST_BUFFER_LIMIT=0`)
- **AND** a test writes 5 MiB of log output
- **THEN** the subscriber MUST retain all 5 MiB; no bytes MUST be dropped

#### Scenario: Cap retains most recent bytes

- **WHEN** `SetBufferLimit(1024)` is called
- **AND** a test writes 4096 bytes of log output
- **THEN** the subscriber's `String()` MUST return the most recent 1024
  bytes of output and MUST NOT contain any of the first 3072 bytes

#### Scenario: Env var with KB suffix

- **WHEN** `FLUMETEST_BUFFER_LIMIT=512KB` is set in the environment
- **AND** `initialize()` runs for the first time
- **THEN** `BufferLimit()` MUST return `524288`

#### Scenario: Env var with MiB suffix

- **WHEN** `FLUMETEST_BUFFER_LIMIT=2MiB` is set
- **THEN** `BufferLimit()` MUST return `2097152`

#### Scenario: Programmatic setter overrides env var

- **WHEN** `FLUMETEST_BUFFER_LIMIT=2MB` is set
- **AND** `SetBufferLimit(4096)` is called before `initialize()` runs
- **THEN** `BufferLimit()` MUST return `4096`

#### Scenario: Limit captured at subscribe time

- **WHEN** test A subscribes with `BufferLimit() == 1024`
- **AND** while test A is active, `SetBufferLimit(8192)` is called
- **AND** test A then writes 2048 bytes
- **THEN** test A's buffer MUST be capped at 1024 bytes (the value at
  Subscribe time)

#### Scenario: Invalid env var value falls back to default with stderr warning

- **WHEN** `FLUMETEST_BUFFER_LIMIT=not-a-number` is set
- **AND** `initialize()` runs for the first time
- **THEN** a one-line warning MUST be written to `os.Stderr` describing
  the parse error
- **AND** `BufferLimit()` MUST return the default `1048576`
- **AND** the test process MUST NOT panic or exit

### Requirement: Truncation notice surfaced via t.Log only

`Start`'s `revert` SHALL emit a truncation notice line via `t.Log` when,
and only when, both of the following hold: (a) the capture buffer
experienced truncation (bytes were dropped due to the cap) and (b)
`revert` is going to surface the buffer (failure path, verbose path, or
artifact-write path). The notice MUST be emitted **exactly once per
test**, regardless of how many `Write` calls triggered truncation during
the test. The cumulative count `<N>` reports the total bytes dropped
across the entire capture window. The notice MUST take exactly one line
of the form:

```
flumetest: log buffer truncated; <N> bytes dropped (FLUMETEST_BUFFER_LIMIT=<limit>)
```

`subscriber.Write` MUST NOT emit any notice itself; the only side effect
of an in-test truncation MUST be incrementing the cumulative `dropped`
counter on the subscriber.

The notice MUST NOT be written into the captured buffer itself, MUST NOT
be written into any artifact file, and MUST NOT be emitted when no bytes
were dropped, nor when the buffer is being silently discarded (passing
test, verbose off, artifacts off).

#### Scenario: Failing test with truncation

- **WHEN** a failing test produced output exceeding the cap
- **AND** `revert` runs
- **THEN** `t.Log` MUST receive a single truncation notice line
- **AND** the captured buffer flushed via `t.Log` MUST NOT contain the
  notice text

#### Scenario: Artifact file does not contain the notice

- **WHEN** artifacts are enabled and a failing test produced output
  exceeding the cap
- **AND** `revert` writes the buffer to an artifact file
- **THEN** the artifact file's contents MUST NOT contain the truncation
  notice line
- **AND** `t.Log` MUST receive the truncation notice line

#### Scenario: No notice when no truncation occurred

- **WHEN** a test produced output below the cap
- **AND** `revert` runs (regardless of pass/fail/verbose)
- **THEN** no truncation notice MUST be emitted via `t.Log`

#### Scenario: No notice when buffer is silently discarded

- **WHEN** a passing test with `Verbose()=false` and `Artifacts()=false`
  produced output exceeding the cap
- **AND** `revert` runs
- **THEN** no truncation notice MUST be emitted via `t.Log` (the user
  did not ask to see anything)

#### Scenario: One notice per test even when many writes trigger truncation

- **WHEN** a failing test issues many `Write` calls each of which
  individually exceeds the cap (e.g. 1000 writes, each dropping 100
  bytes)
- **AND** `revert` runs
- **THEN** `t.Log` MUST receive exactly one truncation notice line
- **AND** the `<N>` value in that line MUST equal the cumulative total
  of all bytes dropped across the entire test (e.g. `100000` for the
  example above)
