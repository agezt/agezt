# Phase Report — Milestone M47 (Per-delegation spend attribution)

> Status: **shipped** · Date: 2026-06-01
> SPEC-12 (multi-agent orchestration). Seventh step on the multi-agent axis,
> and the second *governance* increment: M46 bounded the delegation COUNT;
> M47 attributes and surfaces its COST.

## Why

A sub-agent run executes through the **same governor instance** as its lead
(`subagent.go:141` — `Provider: k.cfg.Provider`), and the governor's
`budget.consumed` event recorded *what* was spent (tokens, cost) but not *which
run* spent it — it was published with `Actor: "governor"` and no correlation. So
spend was visible only globally and per-task-type; you could see that delegation
*happened* (M41–M45) and bound *how much* (M46), but not what any delegation
**cost**. M47 closes that: it threads the spending run's correlation onto every
`budget.consumed` event, then folds per-run spend into `agt runs stats` — total
and the share attributable to sub-agents.

## What shipped — a three-layer vertical

**1. Attribution (the enabler).**
- `agent.CompletionRequest` gains a `CorrelationID string` field — a Governor-only
  hint, opaque to providers, mirroring the existing `TaskType` field exactly. The
  agent loop sets it from `cfg.CorrelationID` when building each request
  (`kernel/agent/agent.go`).
- The governor stamps `budget.consumed` with `CorrelationID: req.CorrelationID`
  (event envelope + a `correlation_id` payload field) in `recordUsage`
  (`kernel/governor/governor.go`). A sub-agent's calls carry the child
  correlation; the lead's carry the lead's — automatically, because each
  `agent.Run` already runs under its own correlation.

**2. Aggregation (the fold).**
- `runEntry` gains `SpentMicrocents int64`; `collectRuns` folds each
  `budget.consumed`'s `cost_microcents` into the matching run — **existing entries
  only**, so an orphan spend event (an out-of-run governor call) never conjures a
  phantom run that would miscount as "running" in stats. `extractCostMicrocents`
  mirrors the existing `extractIters` helper (`kernel/controlplane/runs.go`).

**3. Surface (the view).**
- `handleRunsStats` sums `spent_microcents` over the windowed run set and
  `delegated_spent_microcents` over the runs carrying a `ParentCorrelation` (M41).
- `cmdRunsStats` renders `spend: $0.0126 (delegated: $0.0042)` — printed only when
  priced usage was journaled, reusing the `agt budget` `fmtUSD` formatter so spend
  reads identically across surfaces (`cmd/agt/runs.go`).

**Demo enablement.**
- `mock.WithUsage(resp, usage)` attaches token usage to a scripted response so a
  governor-wrapped mock journals non-zero `budget.consumed` events offline. The
  `AGEZT_DEMO_DELEGATE=N` hook now stamps each scripted call with synthetic usage
  on a priced model, so the live spend path is exercisable with no real provider.

## Design decisions

- **Mirror `TaskType`, don't invent context plumbing.** Correlation reaches the
  governor as a request field, exactly like the existing `TaskType` routing hint —
  no new context key, no cross-package key sharing (the governor already imports
  `agent`; it can't import `runtime` without a cycle). The field is documented as
  provider-opaque, so real providers ignore it.
- **Stamp the event envelope, not just the payload.** Setting the event's
  `CorrelationID` ties spend to its run the *same way every other event does*, so
  `collectRuns` groups it with the standard `e.CorrelationID` key — no special
  case. The payload `correlation_id` is a redundant convenience for `agt journal`
  greppers.
- **Fold into existing entries only.** A `budget.consumed` for a correlation with
  no `task.received` is dropped from per-run spend (still counted in the governor's
  global total). This guarantees the spend fold can never invent a run — proven by
  `TestRunsStats_SpendIgnoresUnknownCorrelation`.
- **Derive from events, no new spend store.** Per-run/per-delegation spend is a
  pure journal fold over data the governor already emits — no projection, no new
  endpoint, consistent with M45's delegation-count fold. The governor's in-memory
  counters (global/per-task) are untouched.
- **Omit the line at $0.** A free/local model or the offline mock spends nothing;
  the spend line appears only when priced usage exists, so it never shows a
  misleading `$0.0000`.

## Tests

- `kernel/governor/governor_test.go::TestComplete_BudgetConsumedCarriesCorrelation`
  — a request with `CorrelationID` produces a `budget.consumed` whose envelope and
  payload both carry it, with cost > 0.
- `kernel/controlplane/runs_test.go::TestRunsStats_SpendAttribution` — lead spends
  100+50 and delegates to a child spending 30 → `spent_microcents=180`,
  `delegated_spent_microcents=30`.
- `kernel/controlplane/runs_test.go::TestRunsStats_SpendIgnoresUnknownCorrelation`
  — orphan spend doesn't create a run (`total=1`) nor inflate the total
  (`spent=40`, not 1039).

Test count: **1269 → 1272**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof (offline mock + AGEZT_DEMO_DELEGATE=3 + AGEZT_SUBAGENT_FANOUT=2)

```
$ agt runs stats
  …
  delegations: 2 (from 1 run(s), max fan-out 2)
  spend      : $0.0126 (delegated: $0.0042)

$ grep -oE '"kind":"budget.consumed".*"correlation_id":"[^"]*"' $AGEZT_HOME/journal/*.jsonl
  # 4 events under the lead correlation, 1 under each of the 2 child correlations
```

The six priced calls (4 lead + 2 sub-agent) are each attributed to their run; the
stats line shows the window's total spend and the third of it the delegations
cost.

## What's next

The multi-agent axis now has count governance (M46) and spend *visibility* (M47).
Sharpest remaining frontiers:

1. **Per-delegation spend CAP** (MED) — pair M46's count cap with a microcents
   ceiling: refuse a `delegate` (or fail the run) once a lead's sub-agents have
   spent more than `AGEZT_SUBAGENT_SPEND_CAP`. The attribution M47 added (spend
   keyed by correlation) is the enabler; this acts on it. Note the cap would be
   enforced in the governor/runtime, not just surfaced.
2. **Per-run spend in `agt runs list` / `show`** (LOW) — `runEntry.SpentMicrocents`
   is already computed; surface it per row and in the run header / per-delegation
   `↳` outcome line (M44), so spend is visible at the individual-run level too, not
   only in aggregate.
3. **Journal the run answer** (MED) — `llm.response`/`task.completed` carry
   `text_chars`/`usage`, not the body; adding it lights up the M44 outcome and the
   "final answer:" arc section.
4. **Tenant-scoped `why`** (LOW-MED) — route `handleWhy` via `kernelFor` + a
   tenant-token allowlist; the last non-tenant-aware control surface.
