# PHASE M867 — Overseer goes live (SSE) + activity ticker

**Status:** shipped
**Milestone:** M867 (numbered to stay clear of concurrent in-progress
M858/M859 work in the tree).
**Theme:** Product/live-monitoring layer (the owner's stated priority — humane UI
+ live monitoring over more infra). Upgrades the M862 Overseer from a 5s poll to a
real-time supervisory surface driven by the event stream.

## What shipped

`frontend/src/views/Overseer.tsx` now subscribes to the shared SSE event stream
(`useEvents()`):

- **Live refresh.** A state-changing event (`task.received/completed/failed/
  continued`, `subagent.spawned`, `council.consensus`, `board.posted`) triggers a
  debounced (700 ms) refetch, so the panels reflect reality within ~1s instead of
  waiting out a poll. A 15s fallback poll remains so the view self-heals if the
  stream drops or an event is missed.
- **Connection indicator.** A `live` / `offline` pill (green pulsing dot when the
  stream is connected) tells the operator whether they're seeing real-time state.
- **Recent activity panel.** A "what just happened" ticker derived from the live
  feed — the last 10 supervisory-significant events, each with a typed icon/tone
  (run started/completed/failed, sub-agent spawned, council consensus, help
  requested/board post), actor, and time.
- **Click-through.** Active-run cards are now buttons that jump to the Runs view
  (`location.hash = "runs"`) for the full run detail.

## Why this milestone is conflict-free

Purely frontend. Touches **only** `frontend/src/views/Overseer.tsx` and the
rebuilt `kernel/webui/dist`. It reuses the existing `useEvents()` provider and the
same read routes as M862 — no new control-plane route, no Go change. The
concurrent session's in-flight kernel edits (M858/M859) are untouched.

## Verification

- **Frontend gate:** `tsc --noEmit` clean; `vite build` emits the committed-LF
  dist (1855 modules). Removed the now-unused `Muted` import. Event-kind strings
  were cross-checked against `kernel/event/kinds.go` (read-only) so the ticker
  labels match what the daemon actually emits.
- No Go change → contested kernel packages not compiled; full `go build ./...`
  deliberately skipped (it would compile the concurrent in-progress Go edits).
- Degrades gracefully: each route call stays `.catch`-guarded, and when the
  stream is down the pill shows `offline` while the fallback poll keeps the data
  fresh.

## Notes
- The Overseer is now the live nerve-center for supervision; intervention controls
  (stop/modify a run) still live in their own views, reachable via the click-through.
