# M286 — Web UI: Providers panel → per-call routing timeline

## Why
M285 added a Providers panel showing the *aggregate* routing picture (who served
how many calls, fallback rate). The natural companion is the per-call *timeline*:
which provider was chosen for each call, on what chain, and exactly when a
fallback fired — the same drill-down the fallback badge got in M283. The control
plane already serves this (`CmdProviderLog`, M89); this milestone surfaces it.

## What
- **`kernel/webui/webui.go`**: a new read-only args route
  `"/api/provider_log" → controlplane.CmdProviderLog` (forwarding `limit`,
  `fallbacks`), added to `readArgsRoutes` (GET, read-only — never mutates).
- **`kernel/webui/dashboard.html`**:
  - `openProviderLog()` reuses the modal shell + `/api/provider_log?limit=40` and
    renders each event newest-first: routing decisions as `route → <provider>`
    with `task type · chain`, fallbacks as `fallback <failed> → <next>` with the
    reason, each stamped with its local time (`fmtTime`).
  - The Providers panel's per-provider rows (and the count line) are now
    `item click` → `openProviderLog`, with a "· click for routing log" hint.

## Files
- `kernel/webui/webui.go` — `/api/provider_log` route (edited).
- `kernel/webui/dashboard.html` — `openProviderLog`, clickable rows + hint
  (edited).
- `kernel/webui/webui_test.go`:
  - **new** `TestProviderLogRouteForwardsLimit` (route forwards `limit`, drops a
    stray arg).
  - the dashboard-wiring test now guards `function openProviderLog` +
    `/api/provider_log`.

## Verification
- `go test ./kernel/webui/` — green; full suite **1896**, 68 packages, `go test
  ./...` exit 0.
- `gofmt -l` clean on the Go files; `go vet ./kernel/webui/` clean; `GOOS=linux`
  build clean; `go.mod` / `go.sum` unchanged.
- **Live-proven end-to-end**: a daemon with `AGEZT_DEMO_FAIL_PRIMARY=1` ran 2
  intents → `GET /api/provider_log` returned 4 events — 2 `route` (primary
  `mock-failshim`, chain `mock-failshim,mock`) interleaved with 2 `fallback`
  (`mock-failshim → mock`, `demo-shim: simulated primary failure`).
- **Live-proven in a real browser** (Playwright): clicking a Providers row opened
  the "Provider routing log" modal listing all 4 events newest-first with
  timestamps, `route → mock-failshim · chain: mock-failshim,mock`, and
  `fallback mock-failshim → mock · demo-shim: simulated primary failure`.

## Scope notes
- Read-only over an existing control-plane command; no new endpoint logic, no
  new event, no dependency. `provider_log` is in `readArgsRoutes` (GET) because
  it never mutates.
- Completes the Providers surface: aggregate view (M285) + per-call timeline
  (M286), mirroring the fallback badge (M281) + fallback detail modal (M283)
  pairing. Both feed off the same `routing.decision` / `provider.fallback`
  journal events the real-API arc (M279→M280) made trustworthy.
