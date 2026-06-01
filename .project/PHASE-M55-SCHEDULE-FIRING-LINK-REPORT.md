# Phase Report — Milestone M55 (Link firings to their schedule)

> Status: **shipped** · Date: 2026-06-01
> SPEC-08 (journal) × autonomy/cadence. The M54 follow-on: a firing now knows
> which schedule produced it.

## Why

M54 added `agt schedule fires` — the firing history — but `schedule.fired`
carried only `{intent, model}` and the run correlation, not the schedule id. So a
firing couldn't be mapped back to its schedule *entry*: you couldn't filter "show
me just *this* schedule's firings", and `agt schedule list` couldn't show a
schedule's last outcome. The cadence Engine knew the entry id when it fired
(`fireDue` iterates `Entry` values) but threw it away — its `RunFunc` only
received `(intent, model)`. M55 threads the id through.

## What shipped

- **`RunFunc` carries the schedule id (`kernel/cadence/cadence.go`)** — the
  signature widened from `func(ctx, intent, model)` to `func(ctx, id, intent,
  model)`, and `fireDue` now passes `ent.ID`. One-field widening of the one
  callback the engine invokes.
- **`schedule_id` on the firing event (`cmd/agezt/main.go`)** — the daemon's
  cadence `run` closure stamps `schedule_id` onto the `schedule.fired` payload
  alongside intent/model, so the firing is attributable to its schedule from the
  journal.
- **`schedule_id` in the fold + an `id` filter (`kernel/controlplane/schedule_fires.go`)**
  — `extractScheduleFired` now returns the id; each `agt schedule fires` row
  carries `schedule_id`; and `args.id` restricts the listing to one schedule's
  firings.
- **`agt schedule fires --id <sched>` (`cmd/agt/schedule.go`)** — both `--id X`
  and `--id=X`, documented in `--help`. The operator gets the id from
  `agt schedule list`.

## Design decisions

- **Widen the callback, don't smuggle the id.** The engine already had the entry;
  passing its id is the honest, minimal change — no context value, no store
  lookup in the daemon. The `RunFunc` is the single seam between the engine and
  the run, and it's the right place for firing metadata.
- **`""` for pre-M55 firings, never a crash.** `extractScheduleFired` returns an
  empty id for firings journaled before M55 (and for env/malformed payloads); they
  still list, just unattributed, and the `--id` filter simply won't match them.
  Backward-compatible over the append-only journal.
- **Filter server-side.** `args.id` is applied during the journal walk, so a
  narrow `--id` query doesn't ship every firing to the client. Mirrors how
  `--tenant`/`--since` scope other control-plane reads.
- **Folded a stale-format cleanup.** Adding M48's long `SubAgentMaxSpendMicrocents`
  key to the daemon's `kernelruntime.Config` literal had left the whole literal's
  gofmt alignment stale (a mechanical oversight, caught now by the full-file
  `gofmt -l`). This commit re-aligns it — whitespace only, no behaviour change.

## Tests

- `kernel/cadence/cadence_test.go::TestEngine_Start_FiresLive` extended — the
  recorder now captures the schedule id and asserts the engine threads the firing
  entry's id to the `RunFunc`.
- `kernel/controlplane/schedule_test.go::TestScheduleFires_JoinsRunOutcome`
  extended to assert each row carries `schedule_id`.
- `TestScheduleFires_FilterByScheduleID` — three firings across two schedules;
  `args.id` returns only the matching schedule's two firings.
- `cmd/agt/schedule_fires_test.go::TestCmdScheduleFires_IdFlagNeedsValue` —
  `--id` with no value is a usage error; `--help` documents `--id`.

Test count: **1290 → 1292**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean (and `cmd/agezt/main.go` is now fully
gofmt-clean).

## Live proof (offline daemon, AGEZT_SCHEDULE='1s=report the current status')

```
$ agt schedule list --json | grep -o 'sched-[0-9A-Z]*'
  sched-01KT175JCT…

$ agt schedule fires 1 --json
  "schedule_id": "sched-01KT175JCT…"          # the firing is linked to its schedule

$ agt schedule fires --id sched-01KT175JCT…
  2026-06-01 12:10:31  completed  (18ms)  run-…  "report the current status"

$ agt schedule fires --id sched-NONEXISTENT
  no scheduled firings yet …                   # filter excludes everything else
```

A firing now names its schedule, and `--id` narrows the history to one schedule.

## What's next

The schedule↔firing link is closed. Sharpest next steps:

1. **Per-schedule last outcome in `agt schedule list`** (LOW-MED) — now that
   firings carry `schedule_id`, fold the most-recent firing per schedule and show
   its status/time inline on each `schedule list` row (`… last: completed 2m ago`).
   Builds on the M54 fold + M55 link.
2. **`agt schedule fires` stats** (LOW) — fire count, success rate, total spend
   over scheduled runs (an autonomy `agt runs stats`).
3. **Boot-banner the delegation caps** (LOW) — off-axis legibility win.
4. **`agt runs list` answer preview column** (LOW) — `answer_preview` is on every
   row (M52); show it in the flat list too.
