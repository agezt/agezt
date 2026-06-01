# Phase Report — Milestone M107 (`agt budget check`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / cost control.

## Why

`agt budget` shows spend-to-date vs the ceiling, but an operator running
cost-controlled workloads had no quick answer to "can I submit this run, or am I
out of budget?" — they had to eyeball spend, guess the cost, and submit and hope.
`agt budget check` reports the remaining headroom directly, with an exit code a
CI gate can branch on.

## What shipped

- **`agt budget check [--task-type <t>] [--json]`** — reports the binding
  remaining spend before a run. Without a task type it reports global headroom;
  with one it folds in that task's per-type cap and reports whichever binds
  (the smaller). Exit 0 = headroom remains (or uncapped), 3 = exhausted, 2 =
  usage. Reuses the existing `CmdBudget` snapshot — no new control-plane command.
- **`effectiveHeadroom(dims)`** — the pure binding-constraint logic: the minimum
  remaining across capped dimensions, or "all uncapped". Unit-tested.

## Design notes

- **The smallest cap binds.** A run must satisfy every cap, so the operator's
  real headroom is the minimum remaining across the global ceiling and the
  task-type cap — exactly what `effectiveHeadroom` computes.
- **Uncapped is not exhausted.** A dimension with no ceiling never binds; only a
  capped, fully-spent dimension yields exit 3.

## Tests

- `TestEffectiveHeadroom` — global-only; task cap binds over a larger global;
  all-uncapped → unlimited; exhausted (==0 and overspent <0); capped task beside
  an uncapped global.

Test count: **1364 → 1365**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt budget check
global   : $20.0000 remaining of $20.0000 ceiling
→ headroom: $20.0000 before the binding cap.                      (exit 0)
$ AGEZT_TASK_BUDGETS="code=500000000" … ; agt budget check --task-type code
task "code": $0.5000 remaining of $0.5000 cap
→ headroom: $0.5000 before the binding cap.                       (exit 0)
```
