# M285 — Web UI: a Providers routing panel

## Why
The fallback-observability arc made a *degraded* provider visible — M280 surfaced
the fallback count in `agt status`/`doctor`, M281 badged it in the header, M283
made the badge drill into the individual `provider.fallback` events. But the Web
UI still had no view of the *healthy* routing picture: which provider is actually
serving the traffic, and what share of calls fall back. The control plane already
aggregates exactly this (`CmdProviderStats`, M90) — this milestone surfaces it as
a panel.

## What
- **`kernel/webui/webui.go`**: a new read-only API route
  `"/api/providers" → controlplane.CmdProviderStats` in `apiRoutes` (parameterless
  read, same proxy path as status/runs/stats).
- **`kernel/webui/dashboard.html`**:
  - A `Providers` panel (after Stats) with a `providers` renderer showing
    `routed` / `fallbacks` / `fallback rate`, a per-provider `by_primary` list
    (count chip + name, sorted by volume), and a red `fallbacks by provider`
    breakdown when any provider has fallen back.
  - `liveRefresh` refreshes the panel on `routing.*` / `provider.*` events, and
    `providers` is added to the initial-load `PANELS` list.

## Files
- `kernel/webui/webui.go` — `/api/providers` route (edited).
- `kernel/webui/dashboard.html` — panel, renderer, liveRefresh hook, PANELS
  (edited).
- `kernel/webui/webui_test.go`:
  - **new** `TestProvidersRouteProxiesProviderStats` (the route proxies
    `provider_stats`).
  - `provider_stats` added to the `TestAPIReadOnly` read-only allowlist.
  - the dashboard-wiring test now guards the `providers` panel + renderer.

## Verification
- `go test ./kernel/webui/` — green; full suite **1895**, 68 packages, `go test
  ./...` exit 0.
- `gofmt -l` clean on the Go files; `go vet ./kernel/webui/` clean; `GOOS=linux`
  build clean; `go.mod` / `go.sum` unchanged.
- **Live-proven end-to-end**: a daemon with `AGEZT_DEMO_FAIL_PRIMARY=1` ran 3
  intents → `GET /api/providers` returned `routed:3 fallbacks:3 rate:1.0
  by_primary:{mock-failshim:3} fallbacks_by_primary:{mock-failshim:3}`.
- **Live-proven in a real browser** (Playwright): the Providers panel rendered
  `routed 3 / fallbacks 3 / fallback rate 100% / 1 provider(s) serving /
  mock-failshim ×3 / fallbacks by provider: mock-failshim ×3`. A fourth run
  bumped `routed` to 4 automatically (the `routing.*`/`provider.*` live refresh),
  with no manual reload.

## Scope notes
- Read-only over an existing control-plane aggregate; no new endpoint logic, no
  new event, no dependency. The `provider_stats` fold is tenant-scoped and
  `--since`-capable server-side; the panel uses the all-time/primary view (matches
  the other parameterless panels).
- Completes the fallback-observability arc on the Web UI: status badge (M281) →
  fallback detail modal (M283) → aggregate routing view (M285). The panel answers
  the healthy-state question ("who is serving my traffic?") the badge/modal
  couldn't.
