# M497 — Mutation testing the governor: pin the spend-enforcement boundary

## Context
Eighth package in the mutation pass (a scope expansion of the HARDENING.md mutation
criterion): `kernel/governor`, which enforces per-day and per-task spend ceilings —
the daemon's cost-runaway safety control. Its pricing arithmetic is already fuzzed
(`FuzzCostMicrocents`, M496); this targets the budget *decision* logic. Run with
`GOMAXPROCS=3` to keep the CPU usable per operator feedback. Score 0.582 (215/514
survived — dominated by error-message and event-publish mutants).

## The genuine gap (closed)
The two enforcement checks use `>=` — spend that has *reached* the ceiling is over
budget:
- `budgetExceeded`: `g.spentToday >= g.cfg.DailyCeilingMicrocents`
- `taskBudgetExceeded`: `spent >= cap`

Both `>=` → `>` mutants **survived**. The existing end-to-end tests
(`TestBudgetCeiling_RefusesNewCalls`, `TestTaskBudget_BlocksAfterCap`) overshoot: the
first call "blows well past" the cap, so they only ever assert blocking when spend is
*strictly greater* than the ceiling. The exact boundary — spend equal to the ceiling
must block — was unpinned, so a `>=` → `>` regression (one extra call allowed once
spend reaches the ceiling) would pass the whole suite.

(The `DailyCeilingMicrocents <= 0` = unlimited case is already covered — several tests
set a 0 global ceiling and expect calls to proceed, so the `<=` → `<` mutant there is
killed.)

## Fix
`kernel/governor/budget_boundary_internal_test.go` (internal `package governor`):
- `TestBudgetExceeded_AtExactCeilingBlocks`: spend == ceiling → over budget; spend ==
  ceiling-1 → not.
- `TestTaskBudgetExceeded_AtExactCapBlocks`: same at the per-task cap.

The Governor is built directly (not via `New`, which requires a registry) because the
two checks touch only the spend ledger; `today` is pre-set to the current UTC day so
`rolloverIfNeededLocked` does not reset the installed spend. (First draft used `New`
and aborted at "registry required" — caught and corrected before relying on it.)

## Negative control (manual, CPU-capped)
Applying both survivors — `spentToday >= ceiling → >` and `spent >= cap → >` — makes
both new tests fail; restored byte-for-byte (`git diff --ignore-all-space` on
governor.go empty); tests pass again.

## Verification / gate
- New tests pass; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (run at `GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum`
  unchanged.

## Mutation pass — eight packages (M490–M497)
redact, journal, edict, netguard, event, creds, warden, governor — plus the
controlplane primary-token auth gate verified solid out-of-band. Genuine
security/safety-relevant gaps were found and closed where they existed (redact,
journal, edict, creds-legacy-KDF, warden-blank-argv0, governor-spend-boundary); the
rest were verified already solid with equivalent/error-message survivors.
