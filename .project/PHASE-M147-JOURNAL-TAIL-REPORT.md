# M147 — `agt journal tail` reads only the tail, not the whole journal

## Why
The journal is append-only with **full retention** — segments rotate at 64 MB but
are never deleted, so on a long-lived daemon it grows without bound. Yet
`handleJournalTail` (`agt journal tail N`) forward-walked **every** segment via
`Range`, filtering by seq, just to keep the last N events. The handler's own
comment admitted it: "O(total)". `journal tail` is a common debugging command; on
a multi-gigabyte journal it would read every byte to return 20 events. This was
flagged adjacent to the M145 review (the broader "folds scan the whole journal"
concern). Unlike the predicate folds (inbox / conversation history), which can't be
safely seq-windowed without silently truncating low-traffic results, `journal tail`
is purely "the last N by seq" — so it CAN be made efficient with no correctness
risk.

## What
- **`kernel/journal/journal.go`** — a new `Tail(n int) ([]*event.Event, error)`
  primitive. It lists segments and reads them newest→oldest, prepending each
  segment's events (so the result stays in seq order) and stopping as soon as it has
  gathered ≥ n events. For a small N this reads only the last segment; for the whole
  journal in one segment it's equivalent to before. `n <= 0` → nil; fewer than n
  total → all. A small `readSegment` helper scans one segment file into a slice
  (factored out of `Range`'s loop). Concurrency matches `Range` exactly — no lock is
  held, so a tail read runs alongside `Append` and reflects whatever was durably
  written when it reached EOF.
- **`kernel/controlplane/journal.go`** — `handleJournalTail` now calls
  `Journal().Tail(n)` instead of `Range` + seq-cutoff filtering. The response shape
  is unchanged (`events`, `count`, `head`).

## Why it's correct
`Tail` returns exactly the last N events in seq order — byte-identical output to the
old forward-walk-and-filter, which the unchanged existing tests
(`TestJournalTail_*`) confirm. The only difference is how much of the journal is
read to produce it.

## Files
- `kernel/journal/journal.go` — `Tail`, `readSegment`.
- `kernel/controlplane/journal.go` — use `Tail`; drop the now-unused `event` import.
- `kernel/journal/journal_test.go` — `TestTail_ReturnsLastNInOrderAcrossSegments`,
  `TestTail_EdgeCases`.

## Tests (+2, all passing; existing `TestJournalTail_*` unchanged and green)
- `TestTail_ReturnsLastNInOrderAcrossSegments` — 50 events forced across multiple
  segments (200-byte rotation); asserts ≥3 segments exist, then `Tail(5)` returns
  seqs 45..49 in order (proving the cross-segment stitch and seq ordering).
- `TestTail_EdgeCases` — empty journal → []; `Tail(0)` → nil; `Tail(100)` on 3
  events → all 3 in order.

## Live proof (offline mock, real booted daemon)
```
$ agt journal head
  head seq=14 …
$ agt journal tail 4 --json
  count=4  seqs=[11, 12, 13, 14]      ← last 4 by seq, in order, via Tail()
```

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./...` — **FAIL 0**, **1472 tests** (was 1470; +2), 61 packages.

## Result
`agt journal tail N` now costs ≈ the last segment instead of the entire journal,
with identical output and matched concurrency semantics — the first of the
"folds scan everything" concerns closed where it could be done without trading away
correctness. (The predicate folds — inbox, conversation history — remain full-scan
by design, since seq-windowing them would drop low-traffic results; revisiting those
needs a real index, noted for later.)
