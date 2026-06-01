# Phase Report — Milestone M69 (per-round budget in the task arc)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 observability.

## Why

`agt runs show` renders a run's task arc, and its header shows the run's TOTAL
spend (M50). But the per-call cost — which round, which model, how many tokens —
fell through to the arc's generic default branch, rendering as bare
`budget.consumed (seq=3)` noise. The governor already journals everything needed
(`{model, cost_microcents, input_tokens, output_tokens}`); it just wasn't shown.

M69 renders `budget.consumed` properly so the arc answers "where did the spend
accrue?" round by round, not just "what did the run cost in total?".

## What shipped (client-side, in `renderTaskArc`)

- **`budget.consumed` case** — renders `budget: <model> $<cost> (in=N, out=M
  tokens)`, indented inside the round when mid-round. Reuses `mcFromAny` +
  `fmtUSD` (the same spend formatting as the header and `agt runs stats`) and
  `intOfStatus` for tokens, so the figures agree across surfaces. The token
  suffix is omitted when both are zero (an unpriced/offline call).

## Design decisions

- **Header = total, arc line = breakdown.** The two are complementary: M50's
  header `spend:` is the run total; this is the per-call accrual. Together they
  reconcile (sum of round costs = header total), which is exactly what an
  operator auditing cost wants to verify.
- **Read the governor's fields verbatim.** Grounded in the
  `KindBudgetConsumed` payload in `governor.go:588` — same discipline as the M68
  fix.

## Tests

- `TestRenderTaskArc_BudgetConsumedShowsCost` — a budget.consumed event renders
  `budget: claude-sonnet-4-6 $0.0084 (in=120, out=45 tokens)` and explicitly is
  NOT left as the generic `budget.consumed (seq=…)` line.

Test count: **1311 → 1312**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean (added lines).

## Live proof

```
$ agt runs last        # AGEZT_DEMO_DELEGATE=3 → priced mock usage
  status     : completed (1 iters, —)
  spend      : $0.0021
  round 1 (seq=37)
      budget: claude-sonnet-4-6 $0.0021 (in=2000, out=1000 tokens)
    llm.response  (input=2000, output=1000 tokens)
```
