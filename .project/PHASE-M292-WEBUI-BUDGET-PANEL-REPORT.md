# M292 — Web UI: a Budget panel

## Why
The cost arc (M289–M291) made spend accounting cache-aware but the headline
number — *what have I spent today, against what ceiling* — lived only in the CLI
(`agt budget`); `agt status` / the Web UI Status panel surface delegation caps,
not the daily spend or per-task caps. This brings the daily budget to the visible
dashboard and is the natural payoff for the cache-cost work (the spend it shows is
now cache-discounted).

## What
- **`kernel/webui/webui.go`**: a new read-only API route
  `"/api/budget" → controlplane.CmdBudget` in `apiRoutes` (parameterless read,
  same proxy path as status/stats/providers).
- **`kernel/webui/dashboard.html`**:
  - A `Budget` panel (after Stats) with a `budget` renderer showing `date`,
    `spent`, `ceiling` (or `unlimited`), `used %`, a `strict pricing on` marker,
    and per-task caps (spend chip + task type + `/ $cap`). Tolerates the
    no-governor error string.
  - `liveRefresh` routes `budget.*` events to the panel; `budget` added to the
    initial-load `PANELS` list and the 10s periodic refresh (CmdBudget is a cheap
    snapshot read, not a journal scan).

## Files
- `kernel/webui/webui.go` — `/api/budget` route (edited).
- `kernel/webui/dashboard.html` — panel, renderer, liveRefresh, PANELS, periodic
  (edited).
- `kernel/webui/webui_test.go`:
  - **new** `TestBudgetRouteProxiesBudget` (the route proxies `budget`).
  - `budget` added to the `TestAPIReadOnly` read-only allowlist.
  - the dashboard-wiring test now guards the `budget` panel + renderer.

## Verification
- `go test ./kernel/webui/` — green; full suite **1902**, 68 packages, `go test
  ./...` exit 0.
- `gofmt -l` clean; `go vet ./kernel/webui/` clean; `GOOS=linux` build clean;
  `go.mod` / `go.sum` unchanged.
- **Live-proven end-to-end**: a daemon with `AGEZT_DEMO_CACHED=1` ran one intent →
  `GET /api/budget` returned `spent_mc:907500 ceiling_mc:20000000000` (the
  cache-aware cost from M289–M291).
- **Live-proven in a real browser** (Playwright): the Budget panel rendered
  `date 2026-06-04 / spent $0.0009 / ceiling $20.0000 / used 0%` — the spend is
  the cache-discounted figure.

## Scope notes
- Read-only over the existing `CmdBudget` snapshot; no new endpoint logic, no new
  event, no dependency. The handler returns a clear error string when the
  daemon's provider isn't a governor (test rigs); the renderer surfaces it.
- This makes the daily-spend view reach parity with `agt budget`. A follow-up
  could add a *cached-savings* aggregate (fold `budget.consumed`
  cached/cache-write tokens → tokens-served-from-cache / estimated $ saved); that
  needs a new journal fold, deferred.
