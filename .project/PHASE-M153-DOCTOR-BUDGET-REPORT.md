# M153 — Budget-headroom check in `agt doctor`

## Why
`agt doctor` covers daemon health, journal, tools, sandbox, provider, approvals,
catalog, webhooks, disk, exposure, channels, and halt — but not **spend**. The
global daily ceiling ($20/day by default) is a hard, terminal limit: once it's hit,
the governor rejects calls with `ErrBudgetExceeded`, which reaches the agent loop as
a fatal run error with no fallback (the mock only rescues *ordinary* provider
errors, not budget exhaustion). So an operator's first sign of trouble is a
confusing "all providers failed" mid-run. The spend is already known (`agt budget`),
it just wasn't in the go-to diagnostic. A doctor check turns "you're about to start
failing" into a proactive WARN — exactly like the disk check warns before the disk
fills.

## What
- **`cmd/agt/doctor.go`** — `checkBudget(ctx, client)` calls `CmdBudget` (like
  `checkProvider` / `checkApprovals`) and delegates to the pure
  `budgetCheckFromBudget(res)`:
  - no ceiling configured → OK (`$X spent today (no daily ceiling)`);
  - spend ≥ ceiling → WARN ("daily ceiling reached" — runs blocked until the UTC
    spend window resets);
  - spend ≥ 90% of ceiling (`budgetWarnPct`) → WARN ("near the daily ceiling");
  - otherwise OK (`$X / $Y today (Z%)`).
  A failed/absent budget call is an informational OK, never a FAIL (the check must
  not itself break `agt doctor`). Wired into `runDoctorChecks` next to
  `checkApprovals`. The hint deliberately names no env knob — the primary daily
  ceiling is fixed and resets at UTC midnight (only tenants have a configurable
  ceiling), so it tells the truth: reduce usage or wait for rollover.

## Files
- `cmd/agt/doctor.go` — `budgetWarnPct`, `checkBudget`, `budgetCheckFromBudget`;
  wired into `runDoctorChecks`.
- `cmd/agt/doctor_test.go` — `TestBudgetCheckFromBudget`.

## Tests (+1, all passing)
- `TestBudgetCheckFromBudget` — no ceiling → OK; 25% → OK; 95% → WARN ("near the
  daily ceiling"); 100% → WARN ("ceiling reached").

## Live proof (offline mock, real booted daemon)
```
$ agt doctor
  …
  [OK  ] budget           : $0.0000 / $20.0000 today (0%)
```
(At ≥90% the line becomes a WARN naming the near/at-ceiling condition and the
UTC-rollover remedy.)

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./...` — **FAIL 0**, **1482 tests** (was 1481; +1), 61 packages.

## Result
`agt doctor` now warns before the daily budget runs out, so a cost-capped run
failure is something the operator sees coming in their health check rather than
discovers as an opaque mid-run error.
