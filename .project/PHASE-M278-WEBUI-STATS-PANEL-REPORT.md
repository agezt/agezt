# M278 — Web UI: a run-stats panel with an outcome bar

## Why
The Runs panel (M275) is per-run; the missing companion is the **aggregate** —
how the fleet is doing at a glance (the `agt runs stats` view). This milestone
adds a Stats panel with the headline numbers and the Web UI's first chart-like
element beyond the world graph: a proportional outcome bar.

## What
- **`kernel/webui/webui.go`** — added `"/api/stats": controlplane.CmdRunsStats`
  to the read-only `apiRoutes`.
- **`kernel/webui/dashboard.html`**:
  - A **Stats** panel section, placed after Runs.
  - A `stats` renderer: a kv of `runs` / `success` (success_rate×100 as %, shown
    only when there are terminal runs) / `spend` ($ from spent_microcents), then
    an `outcomeBar`, then a `N completed · M failed · K running [· A abandoned]`
    line.
  - `outcomeBar(completed, failed, running)` — a flex bar whose segments are
    sized by share (completed green, running accent, failed red) with per-segment
    tooltips; new `.obar` / `.oseg.*` CSS.
  - `liveRefresh` refactored to a `refreshSoon(panel)` debounce that can update
    multiple panels: `task.*` now refreshes **both** Runs and Stats. `stats`
    added to the initial-load + periodic-refresh sets.

## Files
- `kernel/webui/webui.go` — `/api/stats` route (edited).
- `kernel/webui/dashboard.html` — Stats panel, renderer, `outcomeBar`, live
  refresh (edited).
- `kernel/webui/webui_test.go` — `TestStatsRouteProxiesRunsStats` (new); the
  read-only allowlist gained `runs_stats`; the dashboard-wiring test guards the
  Stats panel.
- `README.md` — Web UI panel list refreshed to name the new panels.

## Verification
- `go test ./kernel/webui/` — green; full suite **1883 → 1884** (+1), 68
  packages, `go test ./...` exit 0.
- `gofmt -l` (CRLF-normalised) clean on the Go files; `go vet ./kernel/webui/`
  clean; `GOOS=linux build` clean; `go.mod` / `go.sum` unchanged.
- **Live-proven in a real browser** with a mixed run set (1 completed + 3 failed
  via the mock's scripted-exhaustion path): the Stats panel showed `runs 4`,
  `success 25%`, `spend $0.0000`, and the outcome bar rendered a ~1/4 green +
  ~3/4 red split matching the Runs panel's chips; `1 completed · 3 failed · 0
  running` below.

## Scope notes
- Reuses `CmdRunsStats` (same data as `agt runs stats`), so the Web UI and CLI
  stay consistent; no new control-plane command, no new dependency (the bar is
  plain flex divs, no SVG/library).
- The Web UI now offers both per-run (Runs + click→arc) and aggregate (Stats +
  bar) views, all refreshed live off the event stream.
- Demo artifacts (`webui-*.png`) remain gitignored.
