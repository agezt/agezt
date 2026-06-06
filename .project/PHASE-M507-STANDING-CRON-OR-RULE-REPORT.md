# M507 — Mutation testing standing: pin cron's dom/dow OR-when-both-restricted rule

## Context
Eighteenth package in the mutation pass: `kernel/standing` (the Chronos standing-order
runner — cron/event triggers, cooldown, shutdown gating fixed in M458). Run with
`GOMAXPROCS=3` (CPU-capped). Score 0.598.

## The genuine gap (closed)
`matchesCron` implements the classic cron day rule:

```
case domRestricted && dowRestricted: return domMatch || dowMatch  // match if EITHER
case domRestricted:                  return domMatch
case dowRestricted:                  return dowMatch
default:                             return true
```

`TestMatchesCron` covered daily/`*/15`/weekday/weekend and both Sunday encodings
(0 and 7), but **every** case left day-of-month as `*` — so the both-restricted branch
(`domMatch || dowMatch`) was never exercised. The mutation `|| → &&` **survived**:
under it a schedule with both DOM and DOW restricted (e.g. `0 8 13 * 5` — 8am on the
13th OR on Friday) would fire only when *both* match, the wrong (and surprising) cron
semantics — silently skipping most intended firings.

## Fix
Extended `TestMatchesCron` with the both-restricted cases (June 2026: the 7th is Sunday
per the existing tests, so the 12th is Friday, the 13th Saturday):
- `0 8 13 * 5` on the 13th (Sat) → match (DOM matches)
- `0 8 13 * 5` on Friday the 12th → match (DOW matches)
- `0 8 13 * 5` on Wed the 10th → no match (neither)

## Negative control (manual, CPU-capped)
Applying the survivor (`domMatch || dowMatch → &&`) fails the table:
`matchesCron("0 8 13 * 5", Fri 08:00) = false, want true` (the `Fri` label also
confirms June 12 2026 is a Friday). Restored byte-for-byte
(`git diff --ignore-all-space` on cron.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — eighteen packages (M490–M507)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel, approval, memory, skill, standing — plus the
controlplane primary-token auth gate verified solid. Recurring closeable class: an
inclusive boundary / multi-condition rule that end-to-end tests skip because their
inputs never exercise the both-conditions / exactly-at-threshold corner.
