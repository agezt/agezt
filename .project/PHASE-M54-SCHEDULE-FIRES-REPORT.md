# Phase Report — Milestone M54 (`agt schedule fires` — autonomy firing history)

> Status: **shipped** · Date: 2026-06-01
> SPEC-08 (journal) × autonomy/cadence. Opens a fresh axis: the first
> operator view of what scheduled work has actually *done*, not just what's
> scheduled.

## Why

The cadence subsystem (`AGEZT_SCHEDULE` + `agt schedule add/list/edit/...`) was
mature: an operator could see what's *scheduled* (`agt schedule list`). But there
was no view of what *fired* — did a schedule run, succeed, fail, when, how long,
at what cost? The firing was journaled (`schedule.fired` carries the run's
correlation, and the intent runs through the normal governed loop producing
`task.completed`/`task.failed`), but nothing surfaced it. Run-observability is deep
for manual runs (`agt runs`); autonomy had no equivalent. M54 adds the first one.

## What shipped

- **`CmdScheduleFires` + `handleScheduleFires`
  (`kernel/controlplane/schedule_fires.go`)** — walks the journal for
  `schedule.fired` events and joins each with its run's outcome from the **shared
  `collectRuns` fold**: status, reason, duration, spend (M47), answer preview
  (M52). Newest-first, optional `limit`. Tenant-scoped via the M39
  `kernelFor(tenantOf(req))` seam for free.
- **`agt schedule fires [N]` (`cmd/agt/schedule.go`)** — a new subcommand (alias
  `history`) on the existing `agt schedule` dispatcher. Renders one line per
  firing: `<fired-time>  <status>  (<dur>, <$spend>)  <correlation>  "<intent>"`.
  `--json` for pipelines. Drill into any firing with `agt runs show <correlation>`.

## Design decisions

- **Reuse `collectRuns`, don't re-fold.** A schedule firing IS a run — the same
  `runEntry` the `agt runs` family already produces. `handleScheduleFires` calls
  the shared fold and joins by correlation, so a firing's status/duration/spend/
  answer never disagrees between `agt schedule fires` and `agt runs show`. The only
  schedule-specific walk is collecting the `schedule.fired` events themselves.
- **The autonomy analogue of `agt runs list`.** Same shape (newest-first, `[N]`
  limit, `--json`, graceful "running" for an in-flight firing), same renderers
  (`fmtDuration`/`fmtUSD`), same drill-down (`runs show <corr>`). An operator who
  knows `agt runs` already knows this.
- **Filter to scheduled firings only.** A manual `agt run` is not a scheduled
  firing and must not appear; the view is keyed off `schedule.fired` events, so
  only autonomy-launched runs list — proven by the test's manual-run exclusion.
- **`fires` over enriching `schedule list`.** `schedule.fired` carries the run
  correlation but not the schedule id, so a firing can't yet be mapped back to its
  schedule *entry*. Rather than change the firing path's schema, M54 surfaces the
  firings as their own timeline (identified by intent + correlation + time);
  linking firings to their schedule id is a clean follow-on.

## Tests

- `kernel/controlplane/schedule_test.go::TestScheduleFires_JoinsRunOutcome` — a
  `schedule.fired` + its run (received → budget.consumed → completed-with-answer)
  lists with `status=completed`, `intent`, `spent_mc=2_100_000`,
  `answer_preview="all done"`; a manual run is excluded.
- `TestScheduleFires_EmptyWhenNoFirings` — runs but no firings → empty (non-nil)
  array.
- `cmd/agt/schedule_fires_test.go` — `--help` exits 0 with usage,
  a bad positional is a usage error (exit 2), and the `fires`/`history` aliases
  both dispatch to `cmdScheduleFires`.

Test count: **1285 → 1290**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof (offline daemon, AGEZT_SCHEDULE='1s=report the current status')

```
$ agt schedule list
  sched-…  every 1s  [env,enabled] next …  "report the current status"

# after one 10s engine tick fires the due schedule:
$ agt schedule fires 5
  2026-06-01 11:55:20  completed          (22ms)  run-01KT16A32R…  "report the current status"
```

`agt schedule list` shows what's scheduled; `agt schedule fires` now shows what
fired and how it turned out — the first autonomy-observability surface.

## What's next

The autonomy axis now has its first observability view. Sharpest next steps:

1. **Link firings to their schedule** (LOW-MED) — add `schedule_id` to the
   `schedule.fired` payload (the cadence Engine's `RunFunc` would need the entry
   id) so `agt schedule fires --id <sched>` filters, and `agt schedule list` can
   show each schedule's last-firing outcome inline. The natural M55.
2. **`agt schedule fires` stats** (LOW) — fire count, success rate, total spend
   over scheduled runs (an autonomy `agt runs stats`).
3. **Boot-banner the delegation caps** (LOW) — off-axis legibility win.
4. **`agt runs list` answer preview column** (LOW) — `answer_preview` is on every
   row (M52); show it in the flat list too.
