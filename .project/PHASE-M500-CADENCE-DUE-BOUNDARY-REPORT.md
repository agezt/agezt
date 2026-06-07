# M500 — Mutation testing cadence: pin the due-check firing boundary

## Context
Eleventh package in the mutation pass: `kernel/cadence` (interval/window/once
scheduling — where two HIGH bugs were fixed earlier, M420). Run with `GOMAXPROCS=3`
(CPU-capped per operator feedback). Score 0.568 over 604 mutants (a large package;
most survivors are error/event-emit and equivalent mutants).

## The genuine gap (closed)
`Store.Due(now)` decides which entries fire with `if now.Unix() < e.NextRunUnix
{ continue }` — an entry is due when `now >= NextRunUnix`, i.e. it must fire AT its
scheduled instant. The mutation `< → <=` **survived**: under it, `now == NextRunUnix`
is treated as not-yet-due, delaying every entry by one tick.

`TestStore_Due_AdvancesAndPersists` only probes `now < nextRun` (not due) and
`now = nextRun + 1s` (due) — it never lands on the exact boundary, so the off-by-one
went undetected. For a scheduler, firing on the scheduled instant is the core contract.

## Fix
`kernel/cadence/cadence_test.go` — `TestStore_Due_FiresAtExactScheduledTime`: an entry
added with `next = base+1h`, queried with `Due(base.Add(time.Hour))` (now ==
NextRunUnix exactly), must be due.

## Negative control (manual, CPU-capped)
Applying the survivor (`now.Unix() < e.NextRunUnix → <=`) makes the new test fail
(entry not due at its scheduled time); restored byte-for-byte
(`git diff --ignore-all-space` on cadence.go empty); passes again.

## Verification / gate
- New test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — eleven packages (M490–M500)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence — plus the controlplane primary-token auth gate verified solid. Genuine gaps
closed where they existed (redact, journal, edict, creds-legacy-KDF, warden-blank-argv0,
governor-spend-boundary, scheduler-correlation-id, bus-matcher-over-delivery,
cadence-due-boundary); the rest verified solid. Each gap was a single-token regression
that the existing suite missed by construction (overshooting boundaries, one-sided
guards, untested generation/normalization paths) — exactly the class mutation testing
exists to surface.
