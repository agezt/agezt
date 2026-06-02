# M166 — `agt run --max-cost`: per-run cost cap

## Why
The per-run override family bounds a run by model, system, tools, and wall-clock
time (`--timeout`, M154). The missing axis was **money**. For an autonomous or
scheduled run — exactly the unattended case where a runaway tool loop is most
expensive — there was no way to say "spend at most $X on this run." The Governor's
ceilings are per-day (global) and per-task-type, not per-run. `--max-cost` is the
money analogue of `--timeout`: a single run, bounded, without touching daemon-wide
config.

## Design — zero new concurrency surface
The obvious place to enforce a per-run cap is the Governor (it computes cost), but
that means a per-correlation accumulator map with a lifecycle (clear-on-run-end)
on the Governor's already-contended hot path — real concurrency risk.

Instead the cap is enforced **inside the agent loop**, where it's naturally
per-run: a **local stack variable** accumulates spend across the loop's calls.
No shared map, no mutex, no cleanup, no lifecycle — the accumulator dies with the
run. The loop stays decoupled from pricing via an **injected `CostFn`**; the kernel
wires `governor.CostMicrocents` (already exported, M1.oo), so pricing stays the
Governor's single source of truth and the loop never imports it.

## What
### kernel/agent/agent.go
- `LoopConfig.MaxRunCostMicrocents int64` (0 = uncapped) + `LoopConfig.CostFn
  func(model string, in, out int) int64` (nil = cost accounting off).
- `var ErrRunBudgetExceeded` — terminal, like `ErrMaxIter`.
- In `Run`'s loop, after each call's `llm.response`: `spentMicrocents +=
  CostFn(billed, usage.in, usage.out)` (billed = `resp.Usage.Model` or the request
  model — same rule as the Governor), and if it reaches the cap, return
  `ErrRunBudgetExceeded`. Post-call check ⇒ bounded overshoot of at most one call,
  exactly like the daily ceiling. The existing deferred terminal emitter journals
  `task.failed`, and `failureReason` now tags `ErrRunBudgetExceeded` → `cost_budget`.

### kernel/runtime/runtime.go
- `WithMaxCost(ctx, mc int64)` / `maxCostFromCtx` ctx override (the cost sibling of
  `WithRunTimeout`).
- `RunWith` wires `MaxRunCostMicrocents: maxCostFromCtx(runCtx)` and `CostFn:
  governor.CostMicrocents` into the `LoopConfig`.

### kernel/controlplane/server.go + args.go
- New `argInt64` typed accessor (numeric arg; present-but-wrong-type → usage error,
  per the M161 contract).
- `handleRun` parses a `max_cost` arg (microcents) → `runtime.WithMaxCost`.

### cmd/agt/main.go
- `--max-cost <usd>` flag (`--max-cost 0.50` / `--max-cost=$0.50`).
  `parseUSDToMicrocents` converts dollars → microcents ($1 = 1e9, matching
  `governor.DefaultDailyCeilingMicrocents`) and rejects non-positive/garbage
  client-side, so a bad value is a usage error, never a silently-uncapped run.
- Usage line updated.

## Tests (+9, all passing)
- `kernel/agent/runcost_test.go` — `TestRun_PerRunCostCap_Terminates` (a call over
  the cap → `ErrRunBudgetExceeded` + `task.failed(reason=cost_budget)`, no
  `task.completed`), `_UnderBudget` (stays under → completes), `_InertWithoutCostFn`
  (nil CostFn or 0 cap → cap disabled even at absurd usage).
- `kernel/controlplane/args_test.go` — `TestArgInt64` (absent / present / wrong-type).
- `cmd/agt/run_test.go` — `TestParseUSDToMicrocents` (dollar forms, `$` strip,
  non-positive/garbage rejected).
- `kernel/controlplane/runcost_server_test.go` —
  `TestRun_PerRunCostCap_EndToEnd` (3 subtests, the live end-to-end through the real
  control plane): a run billing 100k tokens of a catalog-priced model
  (`claude-sonnet-4-6` ≈ $0.03) past a $0.01 cap fails with `cost budget` +
  journals `cost_budget`; an uncapped run completes; a string `max_cost` is a usage
  error.

## Live proof
- **End-to-end (server-level):** the `TestRun_PerRunCostCap_EndToEnd` suite drives a
  real `controlplane.Server` + `Kernel` + agent loop + `governor.CostMicrocents`
  pricing — the cap fires and journals `task.failed(reason=cost_budget)`. (The offline
  mock daemon is unpriced — `"mock": {0,0}` in the price table — so the cap can't fire
  through the CLI binary; the priced-model server test is the authoritative live proof,
  the same pattern as M154/M163.)
- **Binary (CLI plumbing):** `agt run --max-cost 0.50 --no-tools "hello"` on a mock
  daemon completes (exit 0; unpriced, under any cap); `agt run --max-cost abc` and
  `--max-cost 0` are rejected client-side (exit 2) with a clear message.

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command or event kind (reuses
  `task.failed` with a new reason tag).
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./... -count=1` — **FAIL 0**, **1539 tests** (was 1530; +9), 61 packages.

## Result
A run can now be bounded by money as cleanly as by time: `agt run --max-cost 0.50`
stops the moment cumulative spend reaches the cap, terminating with a distinct
`cost_budget` reason that `agt runs`/`agt runs stats` can group — and it does so
without adding a single byte of shared mutable state to the hot path. The per-run
override family is now model / system / timeout / tools / **cost**.
