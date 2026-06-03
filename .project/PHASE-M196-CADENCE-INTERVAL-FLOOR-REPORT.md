# M196 — Scheduler interval floor (no busy-loop on corrupt interval)

## Why
A reliability review of the cadence engine (the time-based scheduler that fires
unattended autonomous runs) found a HIGH busy-loop bug. `advance` computes the next run
for interval/window modes as:

```go
return now.Add(e.Interval()).Unix()   // Interval() = IntervalSec seconds, no floor
```

`Add`/`Reschedule`/`ParseJobs`/`SyncEnv` all enforce `interval >= MinInterval`, but two
paths bypass that:
- `OpenStore` (`schedules.json` is plain JSON on disk) `json.Unmarshal`s entries with NO
  validation.
- A hand-edited or corrupt file with `"interval_sec": 0` — or negative — therefore loads
  as-is.

With `IntervalSec == 0`, `advance(now)` returns `now`; with a negative value it returns a
time in the PAST. Either way `NextRunUnix <= now`, so every ticker wake
(`DefaultResolution`, ~10s) finds the entry due and fires a run — **forever**, an
unattended run every 10 seconds (bounded only by the per-entry overlap guard, so it
hammers the run path as fast as runs complete). One bad value DoSes the daemon.

## What
- **`Entry.safeInterval()` / `safeIntervalSec()`** — `Interval()` clamped to
  `MinInterval`. `advance` uses them for both interval mode (`now.Add(safeInterval())`)
  and window mode (`nextWindowSlot(..., safeIntervalSec(), ...)`), so a zero/negative
  `IntervalSec` can never put the next run on `now`/the past, regardless of how the entry
  was constructed.
- **`OpenStore` repair** — after unmarshal, any interval/window entry with a sub-minimum
  `IntervalSec` is clamped to `MinInterval`. This makes the fix durable (re-persisted on
  the next save) and visible in `agt schedule list`, not just a runtime patch.
- **`usesInterval()`** distinguishes interval/window (where `IntervalSec` is load-bearing)
  from daily/once (where `IntervalSec == 0` is legitimate), so the clamp never touches a
  daily/once entry.

A bad value now degrades to the slowest safe firing rate instead of a busy-loop.

## Tests
`kernel/cadence/interval_floor_test.go` (white-box):
- `TestAdvance_FloorsSubMinimumInterval` — `advance` for `IntervalSec` of 0/-1/-100 (and a
  window entry with `IntervalSec: 0`) returns a time strictly after `now`, at least
  `now + MinInterval`.
- `TestOpenStore_RepairsSubMinimumInterval` — a `schedules.json` with `interval_sec: 0`
  loads with the interval clamped to `>= MinInterval`.
- `TestDue_SubMinimumIntervalDoesNotBusyLoop` — the end-to-end property: a loaded
  zero-interval entry is due once, then NOT due again on the same instant (the old code
  would re-fire every tick).

## Verification
- `go test ./...` — 1611 passing, 0 failing.
- `go vet ./kernel/cadence/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/cadence/cadence.go` — `safeInterval`/`safeIntervalSec`/`usesInterval`,
  floored `advance`, repair in `OpenStore`.
- `kernel/cadence/interval_floor_test.go` — new.

## Follow-ups (same cadence review)
- **C2 (CRITICAL)** — a daily/window schedule whose `at` time falls in the fall-back DST
  repeated hour (e.g. 01:30 in US zones on the Nov transition) can DOUBLE-FIRE: after
  firing at 01:30 EDT, `nextDaily` resolves the ambiguous wall time to 01:30 EST (~1h
  later, same calendar day) and fires again. Fix: in `nextDaily`/`nextWindowSlot` require
  an `i==0` candidate's wall clock to be strictly later in the day than `now`'s, so the
  fold re-entry is rejected. Next milestone candidate.
- **H2** — the fire/advance boundary advances+persists `NextRunUnix` (and removes a
  `once` entry) BEFORE the run completes, so a crash in that ~10s window silently drops
  the firing. A deliberate at-most-once choice; document it, or defer `once` removal until
  after a successful run.
