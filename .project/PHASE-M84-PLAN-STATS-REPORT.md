# Phase Report — Milestone M84 (`agt plan stats`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-02/SPEC-08 observability.

## Why

M83 added `agt plan history` (per-execution list). The aggregate was missing:
"across all plan runs, what's the success rate and typical duration?". M84 adds
`agt plan stats`, completing the plan **history / stats** pair — the same
list+aggregate shape `runs` and `tool` already have, and the plan analogue of
`agt runs stats`.

## What shipped

- **Server `handlePlanStats`** — folds the plan lifecycle (`plan.started` +
  terminal `plan.completed`/`plan.failed`) into total / completed / failed /
  running counts, a success rate over terminal plans, and a duration distribution
  (avg/min/max/p50/p95 via the shared nearest-rank `durationStats`). Primary-only,
  like `CmdPlan`.
- **CLI `agt plan stats [--json]`** — renders the counts, success rate, and a
  duration block identical in shape to `runs stats`.

## Design decisions

- **Reuse `durationStats`.** The same percentile helper behind `runs stats`,
  `tool stats` latency, and the M60 spend block — every distribution across the
  CLI reads alike.
- **Success over terminal.** The rate is completed/(completed+failed), excluding
  still-running plans, matching `runs stats`' definition.

## Tests

- `TestPlanStats_Aggregates` — 2 completed + 1 failed + 1 running → total 4,
  completed 2, failed 1, running 1, success ≈ 0.667 (2/3 terminal).

Test count: **1327 → 1328**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt plan stats
  plan stats (over 2 execution(s)):
    completed : 1
    failed    : 1
    running   : 0
    success   : 50.0% (1/2 terminal)
    duration (over 2 terminal plan(s)):
      avg : 11ms  min : 2ms  p50 : 2ms  p95 : 21ms  max : …
```
