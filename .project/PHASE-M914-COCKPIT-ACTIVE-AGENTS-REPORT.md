# Phase M914 — Live "Active agents" panel on the Cockpit

## Gap
The Cockpit (Dashboard) is dense — gauges, the de-staled "Needs attention" strip,
run counters, spend-by-model, a live ticker — but it showed the fleet only as a
bare number ("running now: N"). The operator (who lives in the Agents monitor)
had no glanceable window into *which* agents are working right now without leaving
the home screen.

## What shipped — `frontend/src/views/Dashboard.tsx`
A live **"Active agents (N)"** panel, placed right under "Needs attention" so the
cockpit leads with what needs you and what's running:
- Fetches `/api/runs` (added to the existing parallel refresh) and folds it through
  the Agents gallery's **`summarizeRoots`** (reuse, M909), keeping only the leads
  whose status is `running` — each with its whole sub-agent subtree aggregated.
- Renders up to 6 mini-cards: a pulsing live dot, the intent, and a chip row of
  **agents / sub-agents / iterations / tree-spend** + the model. The header and
  each card link to the Agents monitor.
- Hidden entirely when nothing is running (like the attention strip), so an idle
  cockpit stays calm.

No new pure logic — the running-fleet derivation is `summarizeRoots(runs).filter(
running)`, already unit-tested via `summarizeRoots`. Live updates ride the existing
`task.received/completed/failed` refresh nudge (a spawning sub-agent is a
`task.received`).

## Gate
`tsc` ✓ · full vitest **542 pass** (80 files) ✓ · `vite build` → embedded dist
(LF) ✓ · `go build ./...` + `kernel/webui` green ✓ · go.mod unchanged · frontend +
dist only.

## Process
Built in an isolated git worktree (`AGEZT-cockpit`, branch
`feat/m914-cockpit-active-agents`) from `origin/main` so the dist is clean by
construction. Reused `summarizeRoots`/`RootSummary` from `@/views/Agents` and
`money`/`clip`/`fmtTime` rather than re-deriving (reuse-over-create).
