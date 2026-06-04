# M369 — Lock in the fail-closed half of durable-before-publish (SPEC-02 §2.3)

## SPEC audit (read-vs-code)
SPEC-02 §2.3 (Journal write path) states the ordering invariant:

> The bus publish happens **after** durable append, so subscribers never see an
> event that wasn't persisted.

**Verified vs `bus.Publish`:** the code is correct — `j.Append(spec)` runs first,
and on error it `return nil, err` **before** any subscriber fan-out; subscribers
are only delivered to after a successful durable append.

**The gap (test coverage, priority A):** `TestDurableBeforePublish` covers only
the *success* half — when a subscriber receives an event, the journal already
has it. The **fail-closed** half — "if the append FAILS, no subscriber receives
the event and Publish returns the error" — was untested. That half is the more
security-critical one: a subscriber acting on an event that was never persisted
would lose it on the next crash, with no journal record explaining the action. A
refactor that moved the fan-out before the append-error check would silently
break it.

## What
Test-only, no production change. `kernel/bus/bus_test.go`:
- **`TestPublish_AppendFailureDoesNotDeliver`** — injects an append failure by
  closing the journal out from under the still-open bus (the next `Append`
  writes to a nil file handle and returns `ErrInvalid` — clean error, no panic),
  then asserts `Publish` returns a non-nil error, a nil event, AND the subscriber
  receives nothing.

## Verification
- **Negative control (proves the test bites):** patched `Publish` to ignore the
  append error and fall through to fan-out → the test FAILED ("subscriber
  received event though the durable append failed"); restored `bus.go`
  byte-identical (git diff empty) → green.
- `go test ./kernel/bus -run TestPublish_AppendFailureDoesNotDeliver` — pass.
  `gofmt -l` clean; `go vet` clean; `GOOS=linux go build ./...` exit 0. Full
  suite **2122** passing (was 2121; +1), `go test ./...` 0 failures across 3
  clean runs. `go.mod`/`go.sum` unchanged. No CHANGELOG (test-only).

## Scope notes
- Injection method recorded for reuse: closing a `*journal.Journal` sets its
  `curFile` to nil; a subsequent `Append`→`writeAndSync` hits `(*os.File)(nil).
  Write`, which returns `os.ErrInvalid` (Go's nil-receiver guard) — an error, not
  a panic — so the bus's error path is exercised cleanly without a fake-journal
  interface (the Bus holds a concrete `*journal.Journal`).
- SPEC-02 §2.3 now covered on both halves (success: TestDurableBeforePublish;
  failure: this). Other SPEC-02 items already solid/tested: §2.2 hash chain
  (journal verify), §2.4 projection rebuild, §3 plugin host (pin spawn+reload),
  §6 governor. Remaining SPEC-02 to audit later: §2.5 compaction/snapshots,
  §2.6 `agt journal revert` reversibility.
