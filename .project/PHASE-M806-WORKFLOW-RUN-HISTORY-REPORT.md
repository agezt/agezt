# Phase M806 — workflow run history (canvas replay from the journal)

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** workflow polish #2.

## What — every past run, replayable on the canvas

The journal already carries every run's full arc
(workflow.started→node…→completed|failed under subject workflow.<name>);
M806 folds it into a browsable history. Nothing new is stored — the
journal is the truth.

- **controlplane `workflow_runs`** {ref, limit (default 20, max 100)}:
  `Journal().Range` fold (the approvals-log pattern) grouped by
  correlation — per run: status (running|completed|failed), started/
  finished ms, error, executed, and the ordered node events
  ({node, type, label, ok, port, handled, error, ts}). Newest first.
  Unknown ref is an honest error.
- **`agt workflow runs <ref> [N] [--json]`** — one line per run: local
  time, status, node count, duration, correlation, clipped error.
- **Console Runs drawer** — a "Runs" toggle next to Copilot opens the
  history (GET `/api/workflows/runs`, a readArgsRoute): status badge,
  time, node count, duration, error gist per row. Clicking a row
  **replays it on the canvas**: `runToStatus` (pure, exported) folds the
  node events into the same done/failed map the live SSE replay uses —
  ok!==false OR handled → green ring (an error-port rescue reads as
  handled, exactly like live).

## Tests (wire + 3 vitest; full battery green)

- Wire round-trip extended: two runs → runs fold returns 2 newest-first,
  correct correlations, 3 node events, started/finished present; unknown
  ref refused
- runToStatus mirrors the live ok/handled rule (failed node → failed,
  error-port-handled → done)
- RunsDrawer lists the fold and fires onReplay with the clicked run
- RunsDrawer empty state
- 488 vitest total; full Go suite, vet, staticcheck, linux build green

## Smoke (isolated AGEZT_HOME, real daemon)

Saved a 5-node condition workflow; ran it twice (payload v=9 → "true"
branch, v=2 → "false" branch). `agt workflow runs hist-demo` listed both
(newest first, 4–5ms durations). Browser: Runs drawer showed 2 rows;
replaying the v=9 run lit start/shape/check/**big** green and left
`small` uncolored; replaying the v=2 run flipped to **small** green,
`big` uncolored — branch-accurate replay from the journal. 0 console
errors.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; vitest 488; dist rebuilt LF; go.mod unchanged; no
new env vars (workflow_runs is read-only — readArgsRoute, not in the
readOnly apiRoutes map by design).

## Next

Workflow polish #3: templates gallery. Memory side: provider embeddings
opt-in. Forge: agent-requested promotion queue.
