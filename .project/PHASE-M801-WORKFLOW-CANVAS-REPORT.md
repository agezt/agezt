# Phase M801 — workflow canvas editor

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** workflow engine, step 4.

## What — the React Flow canvas: see, build, run workflows visually

New **Workflows** view (Automation group, first entry) with two modes:

- **List** — every stored workflow as a row: name, description, node
  count, trigger chip (`manual` / `cron (every 30s)` / `event (on
  memory.>)`), enabled state; enable/disable toggle and remove (with
  confirm) per row; "New workflow" inline form with the kernel's name
  rules enforced client-side before the canvas ever opens.
- **Canvas** — a full React Flow editor on @xyflow/react v12 (already in
  the bundle for Flow Studio, zero new deps):
  - **Node palette** — one button per node type (Tool, LLM, If/Else,
    Transform, Delay, HTTP, Code, Map, Filter, Switch, Merge, Approval,
    Sub-Workflow); a new workflow starts with its trigger node already
    placed (exactly one, not deletable).
  - **Custom node card** (`WfNodeView`) — type accent + label + a
    per-type config gist (`summarize`: "cron every 30s", "GET https://…",
    "{{a}} gt 5"); target handle on top, one **source handle per output
    port** spread along the bottom (`portsForNode`: condition → true/false,
    switch → declared case ports + default, failable → out + a red error
    handle), port captions under multi-port nodes.
  - **Drag-to-connect** — edges carry `sourceHandle` = the kernel port;
    the canvas "out" handle folds back to the kernel's default "" port on
    save (lossless `toFlow`/`fromFlow` round-trip, vitest-proven).
  - **Side panel** (`NodePanel`) — label + per-type fields from
    `FIELD_SPECS` (text/textarea/number/select/JSON; JSON parsed and
    rejected with a toast before it can corrupt the graph); delete node.
  - **Save / Save & Run** — `fromFlow` → POST /api/workflows/save (positions
    persist as node x/y); run POSTs /api/workflows/run and reports
    "run completed — N node(s)".
  - **Live run replay** — subscribes to the SSE firehose; `workflow.node`
    events for the open workflow recolor each node (green ring ok /
    red failed, error-port rescues count as handled); `workflow.started`
    clears the slate for the next run.

## Tests (7 new vitest, 480 total green)

- portsForNode per type (condition/switch/failable/default)
- summarize gists (trigger kinds, http, condition, merge default)
- toFlow/fromFlow lossless round-trip ("out" ↔ default-port fold,
  label + config + positions preserved)
- list rendering with trigger chips + disabled state
- enable toggle posts the flipped flag
- illegal workflow name rejected before the canvas opens
- empty state

## Smoke (isolated AGEZT_HOME, real daemon, headed browser)

Built "canvas-demo" entirely on the canvas: New workflow → trigger placed
→ Add Transform → side-panel config (template "hello from the canvas",
label "Greeter") → dragged trigger.out onto the transform's target handle
→ Save (2 nodes / 1 edge) → Run: `{"executed":["start","transform_1"],
"outputs":{"transform_1":"hello from the canvas"}}` and both nodes lit
green from the live `workflow.node` events. `agt workflow show
canvas-demo` cross-checked the persisted graph (edge, label, config,
positions). 0 console errors.

E2e lesson: a React Flow drag needs fresh handle coordinates — the first
attempts reused boxes captured before a zoom changed the viewport
transform, so the pointer landed off-handle. Read the handle rects and
drag in the same script step.

## Gate

vitest 72 files / 480 green; full `GOMAXPROCS=3 go test -p 2 ./...`
green; vet + staticcheck clean; linux cross-build OK; dist rebuilt and
committed LF; go.mod unchanged; no new env vars. CI org-billing still
blocked → local battery + arc-authority merge.

## Next

M802: the workflow copilot — the internal assistant drafts a workflow
JSON from a description and applies it to the canvas; an agent-facing
`workflow` tool so agents can author and run workflows themselves.
