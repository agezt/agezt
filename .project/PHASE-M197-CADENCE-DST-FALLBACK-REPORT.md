# M197 — Daily-scheduling DST fall-back hardening + regression coverage

## Why
The cadence review flagged a potential CRITICAL: a daily/window schedule whose `at` time
lands in the fall-back DST repeated hour (e.g. 01:30 in US zones) could DOUBLE-FIRE,
because that wall-clock time occurs twice and the second occurrence is `After(now)`.

**Investigation result (honest):** empirically it does NOT manifest. Go's `time.Date`
resolves an ambiguous wall time to the *earlier* offset. So after a daily-at-01:30 fires
at 01:30 EDT (05:30 UTC), `nextDaily`'s same-day candidate `time.Date(…,1,30,…)` resolves
back to 01:30 EDT (= the just-fired `now`), making `cand.After(now)` false — the loop
already advances to the next day. Confirmed with a standalone check against
America/New_York 2026-11-01. The review's premise (Go picks the standard offset) was
incorrect, so this is NOT a live double-fire bug.

It is still worth hardening: the single-fire guarantee currently rests on an *implicit*
detail of how the Go runtime / installed tzdata resolves an ambiguous time. A different
platform, tzdata build, or future runtime that resolved the fold to the later offset
would double-fire. And the existing tests had no fall-back coverage at all.

## What
- **Explicit fold guard in `nextDaily`** — for the same-day candidate (`i==0`), require
  the slot to be strictly later in the day than `now` (`atMinutes > nowMin`). The fold
  re-entry (a candidate sharing `now`'s wall-clock minutes) is rejected, advancing to the
  next permitted day. In normal time this rejects nothing real: a same/earlier-minute
  today slot already fails `cand.After(now)`. The single-fire property no longer depends
  on the runtime's ambiguous-time resolution.
- `nextWindowSlot` is unaffected and unchanged: its slots are absolute (`startT + k*iv`
  with `k` chosen strictly after `now`), so it advances by real interval and cannot
  re-fire the same slot across a fold.

## Tests
`kernel/cadence/dst_test.go`:
- `TestNextDaily_NoFallBackDoubleFire` — using America/New_York's 2026-11-01 fall-back, a
  daily-at-01:30 that fired at the first 01:30 (EDT) advances to 2026-11-02 01:30 (~25h
  later), never ~1h. (Skips if tzdata is unavailable.)
- `TestNextDaily_NormalDayAdvances24h` — a normal daily firing exactly at its slot advances
  ~24h, not 0.

These lock in the correct single-fire behavior across DST — coverage the suite lacked.

## Verification
- `go test ./...` — 1613 passing, 0 failing.
- `go vet ./kernel/cadence/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/cadence/cadence.go` — explicit fold guard in `nextDaily`.
- `kernel/cadence/dst_test.go` — new DST fall-back + normal-day regression tests.

## Note on honesty
This milestone is framed as defense-in-depth + regression coverage, NOT a critical
bug-fix, because the double-fire does not occur with the current Go/tzdata behavior. The
guard removes the reliance on that implicit behavior; the tests document the guarantee.

## Remaining cadence review follow-up
- **H2** — the fire/advance boundary advances+persists `NextRunUnix` (and removes a `once`
  entry) before the run completes, so a crash in that ~10s window silently drops the
  firing. A deliberate at-most-once choice worth documenting, or deferring `once` removal
  until after a successful run.
