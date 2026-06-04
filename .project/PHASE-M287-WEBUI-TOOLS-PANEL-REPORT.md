# M287 — Web UI: a Tools execution panel

## Why
The Web UI had aggregate views for runs (Stats) and routing (Providers, M285)
but nothing for the layer that does the actual work: tools. Every agent run
invokes tools (`file`, `shell`, `http`, `memory`, …), and the CLI already exposes
`agt tool stats` (M67) — but an operator watching the dashboard couldn't see how
many tool calls ran, what's erroring, or which tool is slow. This panel closes
that CLI↔Web-UI gap with an always-populated execution view.

## What
- **`kernel/webui/webui.go`**: a new read-only API route
  `"/api/tools" → controlplane.CmdToolStats` in `apiRoutes` (parameterless read,
  same proxy path as status/runs/stats/providers).
- **`kernel/webui/dashboard.html`**:
  - A `Tools` panel (after Providers) with a `tools` renderer showing
    `calls` / `errored` / `error rate`, the distinct tool count, and a per-tool
    list (count chip + name, error count in red, average latency), sorted by
    volume.
  - `liveRefresh` routes `tool.*` events to the panel (with `tool.awaiting_approval`
    still going to Approvals, checked first); `tools` added to the initial-load
    `PANELS` list.

## Files
- `kernel/webui/webui.go` — `/api/tools` route (edited).
- `kernel/webui/dashboard.html` — panel, renderer, liveRefresh hook, PANELS
  (edited).
- `kernel/webui/webui_test.go`:
  - **new** `TestToolsRouteProxiesToolStats` (the route proxies `tool_stats`).
  - `tool_stats` added to the `TestAPIReadOnly` read-only allowlist.
  - the dashboard-wiring test now guards the `tools` panel + renderer.

## Verification
- `go test ./kernel/webui/` — green; full suite **1897**, 68 packages, `go test
  ./...` exit 0.
- `gofmt -l` clean on the Go files; `go vet ./kernel/webui/` clean; `GOOS=linux`
  build clean; `go.mod` / `go.sum` unchanged.
- **Live-proven end-to-end**: a daemon with `AGEZT_DEMO_FILE_EDIT=1` ran intents
  that drove the `file` tool → `GET /api/tools` returned
  `total:2 errored:0 tools:1 by_tool:{file:{calls:2,errors:0,avg_ms:1}}`.
- **Live-proven in a real browser** (Playwright): the Tools panel rendered
  `calls 2 / errored 0 / error rate 0% / 1 tool(s) used / file ×2 1ms`. The panel
  stays consistent with the backend, and its `tool.*` live-refresh wiring is the
  same path browser-proven to update the Providers panel live in M285.

## Scope notes
- Read-only over an existing control-plane aggregate (`tool_stats`); no new
  endpoint logic, no new event, no dependency.
- The handler also returns `errors_by_message` and a duration distribution
  (p50/p95); the panel surfaces the headline numbers + per-tool rows and leaves
  the deeper breakdown to `agt tool stats` / a future drill-down modal (mirroring
  the Providers panel → routing-log modal pairing, M285→M286).
