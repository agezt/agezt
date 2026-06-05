# M417 — Journal torn-tail truncation on reopen (HIGH data-integrity)

## Context
A rigorous review of the foundational integrity core (journal hash-chain + crash
recovery, bus pub/sub, governor money math) found a single HIGH-severity bug in the
journal — the source of truth for the entire event-sourced system. The bus and the
governor were found clean (hash chain, durability ordering, concurrency, wildcard
matching, backpressure, microcents saturation math, cap semantics all verified).

## The bug
`kernel/journal/journal.go`, `Open` (the reopen-into-last-segment branch):

A crash mid-`Append` can leave a torn, newline-less fragment at the end of the last
segment (the `\n` is written as part of the line, fsync'd as a unit). Readers and
recovery tolerate this: `scanCompleteLines` *discards* an unterminated trailing line.
But it is never removed from disk, and on reopen `Open` did:

```go
j.curBytes = info.Size()          // includes the torn fragment's bytes
j.openCurrent(true)               // O_APPEND
```

`O_APPEND` always writes at end-of-file, so the next real `Append` wrote its
`\n`-terminated line *immediately after* the fragment:

```
…event1\n{"id":…,"seq":2,"prev_ha{"id":…,"seq":2,…}\n
```

That glued content is a single line up to the next `\n` and fails `event.Decode`.
From that point `Range`, `Tail`, `Verify`, and even `Open`/`scanSegment` return a hard
decode error — the journal will not reopen and the source-of-truth log is unreadable,
unrecoverable without manual file surgery. Silent until the next read/restart.

The existing torn-line tests only *read* after a torn tail (correctly tolerated);
none *appended* after one, so the corruption was uncovered.

## The fix
On reopen of a partially-written last segment, truncate to the end of the last
complete (`\n`-terminated) line before appending, restoring the invariant *the append
offset equals the end of the last committed record*. New helper
`lastCompleteOffset(path)` returns the length of the newline-terminated prefix
(`bytes.LastIndexByte`); `Open` truncates the file to that offset (only when it
differs from the size) and sets `curBytes` to it. Only the append branch needed this
— the rotate-to-new-segment branch opens a fresh `O_EXCL` file and never appends to
the torn segment, and per-segment readers already drop a trailing torn line.

## Verification
- **`kernel/journal/journal_test.go`** `TestAppendAfterTornLine_StaysReadable`:
  append 2 events, inject a torn fragment, reopen, append a real event, then reopen
  again and assert `Verify()` passes, `Range` yields exactly 3 events, and head seq
  is 2 — i.e. the journal stays readable and chain-verifiable and the fragment is
  gone. (Existing `TestOpen_RecoversPastTornFinalLine` / `TestReaders_TolerateTornFinalLine`
  still pass — read tolerance unchanged.)
  - **Negative control:** reverting to `curBytes = info.Size()` without truncation →
    the test FAILs at the final reopen with `decode … unexpected end of JSON input`
    (the exact wedged-journal corruption). Restored byte-identical.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2266** passing (was 2265; +1). CHANGELOG
  Reliability entry added.

## Review status
This closes the one finding from the integrity-core review. `kernel/bus` (durable-
before-publish, non-blocking backpressure, race-safe Subscribe/Cancel, correct
NATS-wildcard matching) and `kernel/governor` (saturating microcents math, no
negative/over-charge, correct `>=` cap semantics, fail-open/strict unpriced handling)
were both found clean.
