# Phase M810 — async workflow runs

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** production-grade
workflows #3 — long runs stop being hostage to wire timeouts.

## What

The latent defect this fixes: the canvas's Save & Run held an HTTP
connection through the webui's JSON proxy, which caps at **120s** —
while the engine legitimately allows **15m**. A two-minute workflow
died at the proxy even though the daemon was happily running it.

- **controlplane `workflow_run` gains `async`** (bool or "true"): the
  ref is resolved BEFORE detaching (a typo stays a synchronous, honest
  error), then the run fires on its own 15m deadline and
  {accepted, async, correlation_id, workflow} returns immediately.
  Failures land in workflow.failed — the journal carries the arc, as
  with the M809 webhook path. Sync mode unchanged (CLI scripts that
  want outputs keep them).
- **Canvas Save & Run is now async**: immediate "run started — watching
  it live" toast; the SSE subscription colours nodes as they execute
  (that machinery already existed); the new terminal handler toasts
  "run completed — N node(s)" / "run failed: …" off
  workflow.completed/failed for the OPEN workflow and releases the Run
  button. A `uiRef` keeps the long-lived SSE subscription from
  re-subscribing per render.
- **CLI `agt workflow run --async`** prints the correlation + the
  follow-up commands.

## Tests (wire + 1 vitest; 491 vitest, full battery green)

- Wire: async run → {accepted, async, correlation_id} immediately;
  workflow.completed observed on the bus; async run of an unknown ref
  is a synchronous error
- vitest: canvas Run posts {ref, payload, async:true} after the save
- Full Go suite, vet, staticcheck, linux cross-build green

## Smoke (isolated AGEZT_HOME, real daemon)

A 10-second workflow (delay node):
- CLI `--async` returned in **43ms** ("started — follow it with…").
- Browser: clicked Save & Run → "run started" toast immediately, button
  "Running…"; a MID-RUN snapshot showed start green while wait/done were
  still uncoloured (true live streaming, not a post-hoc paint); ~10s
  later the "run completed — 3 node(s)" toast landed, all three nodes
  green, button released. 0 console errors.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; vitest 491; dist rebuilt LF; go.mod unchanged; no
new env vars.

## Next (production-grade backlog)

Per-node "test this node" on the canvas. Then: webhook reply mode
(sync response with outputs) if the owner wants n8n's "respond to
webhook" pattern.
