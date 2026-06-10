# Phase M799 — workflow triggers (cron + event + manual)

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** workflow engine, step 2.

## What

The trigger node gains a `kind`: **manual** (default — run-on-demand only),
**cron** (`interval_sec` ≥30 XOR `daily_at` "HH:MM" local), **event**
(`subject` glob with bus semantics: `*` one token, `>` rest — e.g.
`task.failed`, `board.dm.*`, `memory.>`). The matched journal event rides
into the run as `{{trigger.payload}}`: {kind, subject, event, data:<event
payload>}; cron fires carry {kind:"cron", fired_at}.

`kernel/workflow/runner.go` (standing-runner shape): the daemon injects a
FireFunc (closure over Kernel.RunWorkflow); the runner decides WHEN —
- **event**: one bus subscription (">"), glob match per enabled workflow,
  per-workflow cooldown (default 30s) so event storms can't launch run
  floods; `workflow.*` subjects are NEVER trigger fuel (and validation
  refuses such subjects + bare `>`/`*` outright — defense in both layers,
  killing the feedback-loop foot-gun).
- **cron**: coarse ticker (default 15s), interval anchored at arm time,
  daily_at once per local day. Injectable clock + tick for tests.
- The store is consulted LIVE on every tick/event: canvas/CLI saves,
  enables, removes take effect without a restart.

Trigger state is in-memory by design — a restart re-arms cleanly (interval
anchors reset; daily_at refires at the next HH:MM).

## Surfaces

Daemon banner: "workflows : N defined (X cron + Y event trigger(s) armed)".
List views (controlplane + `agt workflow list`) now carry trigger_kind +
trigger_detail ("every 30s" / "daily at 09:00" / "on memory.>") so a row
says HOW a workflow starts without the graph body.

## Tests (4 new + validation table extension)

- SubjectMatch table (literals, *, trailing >, length mismatches).
- Event trigger e2e on a REAL bus+journal: fire carries subject/kind/data;
  cooldown suppresses then re-arms; non-matching subjects, workflow.*
  events, and disabled workflows never fire.
- Cron e2e with injected clock: interval fires once per elapsed interval
  (anchored at arm), daily fires exactly once per day after HH:MM; both
  workflows tallied by name.
- Validation: 9 new reject cases (cron without/with-both timings, interval
  <30, bad HH:MM, event without subject, workflow.* subject, bare `>`,
  unknown kind).

## Smoke (isolated AGEZT_HOME, real daemon)

Saved `heartbeat` (cron 30s) + `on-memory` (event memory.>) via CLI — list
showed "cron (every 30s)" / "event (on memory.>)". `agt memory add` →
journal: memory.written → **workflow.started on-memory → 2×node →
completed** (real event fire, no restart needed — runner reads the store
live). 38s later: **heartbeat completed TWICE** (~30s apart). Restart
positive control: banner "2 defined (1 cron + 1 event trigger(s) armed)".

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; frontend untouched; go.mod unchanged; no new env
vars. CI org-billing still blocked → local battery + arc-authority merge.

## Next

M800: node library — http, code (ScriptRunner sandbox), forEach, switch,
parallel+merge, approval gate (approval.Registry), subworkflow, notify
(channel sinks), error branch.
