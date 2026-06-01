# Phase Report — Milestone M56 (Per-schedule last outcome in `agt schedule list`)

> Status: **shipped** · Date: 2026-06-01
> SPEC-08 × autonomy/cadence. The M55 follow-on: the primary `schedule list`
> view now shows not just what's scheduled but how each schedule last went.

## Why

`agt schedule list` showed the cadence (when a schedule fires) but nothing about
how it had *gone* — an operator had to cross-reference `agt schedule fires`. Now
that firings carry `schedule_id` (M55), each schedule can be annotated with its
most-recent firing's outcome inline.

## What shipped

- **`latestFiringBySchedule` fold (`kernel/controlplane/schedule_fires.go`)** —
  walks the journal for `schedule.fired` events, keeps the newest per
  `schedule_id`, and joins its correlation with the run outcome (status/reason)
  from the shared `collectRuns` fold.
- **`handleScheduleList` annotates each row** with `last_status` / `last_reason` /
  `last_fired_unix_ms` when a firing is known. Best-effort: a journal-walk error
  just omits the annotation, never fails the list.
- **`cmdScheduleList` renders it** — appends `  last: completed 06-01 12:16`
  (or `failed (timeout) …`) to a schedule's row.

## Design decisions

- **Reuse the M54/M55 fold + collectRuns.** No new event or store field — the
  last-outcome is derived from the same `schedule.fired` + run events the firing
  history already reads. A schedule's `last_status` never disagrees with
  `agt schedule fires --id <sched>`.
- **Newest firing wins.** Keyed by `schedule_id`, keeping the max `fired_unix_ms`
  — proven by the test where a later *failed* firing supersedes an earlier
  completed one.
- **Quiet for never-fired schedules.** A schedule with no firing (just added, or
  pre-M55 firings only) gets no annotation — the row reads exactly as before.

## Tests

- `kernel/controlplane/schedule_test.go::TestScheduleList_ShowsLastFiringOutcome`
  — a schedule with two firings (later one failed/timeout) reports
  `last_status=failed`, `last_reason=timeout` on its list row.

Test count: **1292 → 1293**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_SCHEDULE='1s=report the current status' agezt &   # wait one 10s tick
$ agt schedule list
  sched-…  every 1s  [env,enabled] next …  "report the current status"  last: completed 06-01 12:16
```

## What's next

1. **`agt schedule stats`** (LOW) — fire count / success rate / total spend over
   scheduled runs (autonomy `agt runs stats`).
2. **Boot-banner the delegation caps** (LOW).
3. **`agt runs list` answer preview column** (LOW).
