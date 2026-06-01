# Phase Report — Milestone M57 (`agt schedule stats` — autonomy aggregate)

> Status: **shipped** · Date: 2026-06-01
> SPEC-08 × autonomy/cadence. The autonomy analogue of `agt runs stats`.

## Why

M54 added per-firing history and M55/M56 linked firings to schedules, but there
was no AGGREGATE: how many scheduled firings, what fraction succeeded, total
spend? `agt runs stats` answers this for manual runs; M57 brings the same to
autonomy.

## What shipped

- **`CmdScheduleStats` + `handleScheduleStats`** — folds `schedule.fired` events,
  joins each with its run outcome (`collectRuns`), and reports `total`,
  `completed`/`failed`/`running`/`abandoned`, `success_rate`, `spent_microcents`,
  `failed_by_reason`, and `schedules` (distinct that fired). Optional `id` scopes
  to one schedule; `since_ms` windows by firing time. Tenant-scoped via `kernelFor`.
- **`agt schedule stats [--id <sched>] [--since <dur>] [--json]`** — renders the
  aggregate, reusing `failedByReasonStr`/`fmtUSD` from the `agt runs` family.

## Design decisions

- **Mirror `agt runs stats`.** Same shape (counts, success rate, failed-by-reason,
  windowed), same renderers, filtered to scheduled correlations — an operator who
  knows `runs stats` reads this instantly.
- **Reuse the fold.** No new event; the aggregate is derived from the same
  `schedule.fired` + run events the firing history reads, so the numbers never
  disagree with `agt schedule fires`.

## Tests

- `TestScheduleStats_AggregatesFirings` — 2 completed (one with spend) + 1 failed
  across 2 schedules → total=3, completed=2, failed=1, schedules=2, spent=150,
  success≈0.667.
- `TestScheduleStats_FilterByScheduleID` — `--id` scopes to one schedule (2 of 3).

Test count: **1293 → 1295**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt schedule stats
schedule firings (over 1 firing(s)):
  schedules : 1 distinct fired
  completed : 1
  success   : 100.0% (1/1 terminal)
```

## What's next
1. Boot-banner the delegation caps (LOW).
2. `agt runs list` answer preview column (LOW).
