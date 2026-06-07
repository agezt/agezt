# Phase M577 — Web UI run-detail cards

**Type:** Feature (Web UI; documented M566/M567 follow-up)
**Date:** 2026-06-07
**Branch:** `feat-webui-run-detail`

## Goal

Make the Web UI's Runs view actually useful at a glance. Before, expanding a run
showed only a flat list of journaled events (ts · kind · subject). This adds
structured **run-detail cards** derived from the same event arc: a summary, a
tool-call breakdown with the Edict capability + policy verdict per call, and the
final answer — with the raw timeline kept one click away.

## What shipped

### `frontend/src/views/Runs.tsx` (rewritten view, pure frontend)
On expand, the row fetches the run's arc (`/api/journal?correlation_id=…`,
already the existing call) and `deriveDetail()` folds it into a `RunDetail`:
- **Summary** (`KeyValue`): status (coloured badge), model, iterations
  (max `iter`+1 over `llm.request`/`llm.response`), tokens (in / out / cached
  from `budget.consumed`), cost (`money(Σ cost_microcents)`), duration. Token/
  cost rows show "—" when the run journaled no budget event (e.g. the offline
  mock).
- **Tool calls**: events are grouped by `call_id` across `policy.decision`
  (capability, allow, hard_denied), `tool.invoked` (tool, input), and
  `tool.result` (error, output). Each renders the tool name, the **capability**
  badge (e.g. `homeassistant.call`), an allowed / denied / hard-denied verdict
  badge, an error flag, and a clipped output preview.
- **Final answer / Error**: from `task.completed` / `task.failed`.
- **Raw events**: the original flat timeline, now under a collapsible toggle.

The kernel is untouched — the UI only *derives* this from the journal arc it
already had, so the journal stays the single source of truth.

### Rebuilt embedded assets
`kernel/webui/dist/` rebuilt (`npm run build`) and committed as LF
(`.gitattributes`), so the Linux `frontend-dist-in-sync` CI job reproduces it
byte-identically from the same source + lockfile.

## Verification

- **Build:** `cd frontend && npm run build` → `tsc --noEmit` (strict) clean +
  Vite emitted `kernel/webui/dist`. `go test ./kernel/webui` (embed compiles +
  serves) pass; `kernel/controlplane` pass. No Go source changed.
- **Runtime smoke (criterion-7):** booted the real daemon + a mock HA, drove
  `agt run "turn off the living room light"` (the M575 HA-tool demo, which
  produces real tool calls + policy decisions), then opened the Web UI in a
  browser (Playwright) and expanded the run. The cards rendered:
  status=completed, model=mock, iterations=3; **Tool calls (2)** —
  `homeassistant` / `homeassistant.call` / allowed / "called light.turn_off ok …"
  and `homeassistant` / `homeassistant.read` / allowed / state JSON; the final
  answer; and a "raw events (20)" toggle. **0 console errors** under the strict
  CSP. Cleaned up the daemon + temp afterwards.

## Counts

- Go packages/tests unchanged (80 / 2463 — frontend-only change).

## Out of scope (documented follow-ups)

- Live recolour of an in-progress run's cards from the SSE stream (currently the
  arc is fetched once on expand; Flow Studio already does live node recolour).
- Vitest/Playwright CI for the React side (still the standing front-end test gap).
