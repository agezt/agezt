# Phase M564 — Flow Studio (visual plan editor in the Web UI)

**Type:** Feature (deferred-roadmap surface; user-selected "Flow Studio")
**Date:** 2026-06-07
**Branch:** `feat-flow-studio`

## Goal

Turn the existing plan toolchain (`agt plan generate` / `refine` / `run` /
`history`, control-plane verbs `plan_generate` / `plan_refine` / `plan` /
`plan_history` / `plan_stats`) into a visual authoring + run surface in the
existing server-rendered Web UI (`kernel/webui`) — faithful to SPEC-07 §0 ("one
event truth, many views; the UI never holds authoritative state, it subscribes
and renders") and the page's stdlib-first, no-build-chain, strict-CSP posture.

## Key design constraint

The dashboard's CSP is `default-src 'none'; script-src 'nonce-…'` with no
external connections — so a CDN diagram library (mermaid.js etc.) is impossible.
The page already hand-rolls an inline-SVG node-link graph (`worldGraph` +
`svgEl`, textContent-only, no `innerHTML`), so Flow Studio renders the plan DAG
the same CSP-safe way. The plan JSON (`nodes[]` with `deps`) carries everything
needed to draw; node events carry `correlation_id` + `payload.node_id` for live
highlighting. No new dependency; `go.mod`/`go.sum` unchanged.

## What shipped

### Backend — `kernel/webui/webui.go`
- `Caller` interface extended with `Stream(...)` (already implemented by
  `*controlplane.Client`). `CmdPlan` streams `RespEvent` frames before its
  terminal result, so it cannot go through `Call` (single response); the run
  route drives it with `Stream`. Streamed events are discarded — the browser
  already sees them live on the SSE `/events` firehose — but `Stream` runs to
  completion so the control-plane connection stays open for the run's whole
  duration (closing early cancels the run's context, killing the plan).
- `decodeAllowedBody` + `jsonProxy`: POST-only handlers that read a JSON request
  **body** (size-capped at 1 MiB via `http.MaxBytesReader`) and forward only the
  route's allowlisted keys — needed because a plan JSON / multi-line intent
  won't fit in a query string. Same allowlist discipline as `writeProxy`.
- New routes: `/api/plan/generate` + `/api/plan/refine` (jsonProxy →
  `CmdPlanGenerate` / `CmdPlanRefine`), `/api/plan/run` (`planRunProxy` →
  `CmdPlan` via Stream, 30-min timeout), `/api/plan_history` (readArgs, limit +
  status), `/api/plan_stats` (read).

### Frontend — `kernel/webui/dashboard.html`
- Full-width "Flow Studio" panel: intent + optional model inputs + Generate;
  editable plan-JSON textarea (with format button + debounced live redraw);
  inline-SVG DAG; feedback input + Refine; Run button; recent-plans list.
- `planDag(plan)`: top-down layered layout by dependency depth (longest-path
  relaxation, cycle-safe via a bounded pass count so an edited plan with a cycle
  draws flat rather than hanging), loop = rounded rect, gate = hexagon, arrows
  dep→node, all built with `svgEl`/textContent (CSP-safe).
- `postJSON` helper (JSON body, token on query string); `flowOnEvent` recolours
  the matching node in place from `node.*` events and resets on `plan.started`.
- CSS for the panel + node-state colours (running/done/failed), nonce'd.

## Verification

- **Unit:** `kernel/webui` — added 10 tests (jsonProxy forwards only allowlisted
  body keys, POST-only, rejects bad/oversized body; run drives Stream + forwards
  only plan_json; plan_history forwards limit/status; plan_stats proxies; a
  dashboard-markers guard). `TestAPIReadOnly` allowlist updated for `plan_stats`.
  All pass.
- **Full gate:** `GOMAXPROCS=3 go test ./... -p 2 -count=1` — exit 0 (75 pkgs).
  gofmt (staged LF blobs) / vet / staticcheck clean.
- **Runtime smoke (executable proof, criterion-7):** booted the real daemon
  (`AGEZT_DEMO_ECHO=1`, `AGEZT_WEB_ADDR`), then over HTTP:
  - dashboard serves the Flow Studio markers; unauth → 401; GET on a POST route → 405.
  - `GET /api/plan_history` → `{"count":0,"plans":[]}`; `GET /api/plan_stats` aggregates.
  - `POST /api/plan/generate` with the echo provider → graceful JSON error (not a
    panic — echo isn't a planner).
  - `POST /api/plan/run` with a hand-written plan → executed end to end:
    `{"node_outputs":{"a":"[echo]\necho hi"},"plan_id":"plan-…"}`, journaling
    `plan.started` / `node.started` / `node.completed` / `plan.completed` — the
    exact events the DAG highlights from. `plan_stats` then showed 1 completed,
    success_rate 1.
  - bad JSON body → 400. **0 panics** across both boots; graceful shutdown.
- **go.mod / go.sum:** unchanged.

## Counts

- Packages: 75 (unchanged — Flow Studio lives in the existing `kernel/webui`).
- Tests (funcs + subtests): 2421 → **2431**.

## Out of scope (documented follow-ups)

- Drag-to-edit node authoring (add/remove nodes/edges by mouse) — current editor
  is the JSON textarea + live diagram preview.
- Per-node drill-down (click a node → its run output / journal arc).
- Saving named plans to a catalog for reuse (plans are authored ad hoc).
