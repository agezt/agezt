# M410 — `agt standing add --budget`: make the per-run cost ceiling settable (SPEC-16 §4)

## Context
M404 wired `Initiative.BudgetPerRunMc` as a per-run cost cap on a fired standing
order (via `WithMaxCost`), and M408 added the trust ceiling. But `agt standing
add` never exposed a budget flag — so `BudgetPerRunMc` was always 0 and the budget
ceiling, though enforced, could not actually be configured. This closes that gap
(an enforced-but-unsettable path).

## What
- **`cmd/agt/standing.go`** — `--budget <USD>` on `agt standing add`: parsed with
  the existing `usdToMicrocents` ($1 = 1e9 microcents), validated (a bad amount
  exits 2 with a clear message), and set as `initiative.budget_per_run_mc`. The
  initiative block is now built field-by-field so budget can be set with or
  without mode/max-trust. Usage text updated.

## Verification
- **`kernel/standing/standing_test.go`** `TestStore_PreservesInitiative`: an order
  with mode + max_trust + budget round-trips through Add → Get → reopen
  (budget = 1e9 microcents = $1 preserved).
- **Live demo** (mock): `agt standing add --max-trust L2 --budget 0.50` →
  `agt standing list --json` shows `"budget_per_run_mc": 500000000` and
  `"max_trust": "L2"`; `--budget abc` is rejected with
  "want a dollar amount like 0.01".
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2257** passing (was 2256; +1). CHANGELOG (Added, user-visible).

## Scope notes
- All three initiative knobs (mode, max_trust, budget) are now settable from the
  CLI and enforced at fire time. SPEC-16 §4 Chronos remains functionally complete;
  the only outstanding enhancement is the richer observers/salience scoring (a
  Pulse integration whose named-observer registry the DSL references but does not
  define — deliberately not invented).
