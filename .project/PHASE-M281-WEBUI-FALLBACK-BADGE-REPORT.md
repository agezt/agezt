# M281 — Web UI: a header warning badge for provider fallbacks

## Why
M280 surfaced provider fallbacks in `agt status`/`doctor` and as a status field
the Web UI Status panel renders — but in the panel it appears only as a raw
`provider_fallbacks {"count":3,…}` line among many. A degraded provider deserves
a prominent, at-a-glance signal. This milestone adds a header badge so an
operator sees a silently-failing provider the moment they open the dashboard.

## What
- **`kernel/webui/dashboard.html`**:
  - A header chip `#fbBadge` (red `.fbbadge` CSS), hidden by default.
  - `updateFallbackBadge(d)` reads `d.provider_fallbacks.count` and, when > 0,
    shows `⚠ N fallback(s)` with the last reason as the hover title; hides it at
    zero. Called from the `status` renderer, so it updates on every status fetch
    (initial load + the 10s periodic refresh + manual ↻).

## Files
- `kernel/webui/dashboard.html` — badge element, CSS, `updateFallbackBadge`,
  status-renderer hook (edited).
- `kernel/webui/webui_test.go` — the dashboard-wiring test now guards the badge
  element + `updateFallbackBadge` (no new top-level test func; test count
  unchanged at 1890).

## Verification
- `go test ./kernel/webui/` — green; full suite **1890**, 68 packages, `go test
  ./...` exit 0.
- `gofmt -l` clean on the Go files; `go vet ./kernel/webui/` clean; `GOOS=linux
  build` clean; `go.mod` / `go.sum` unchanged.
- **Live-proven in a real browser**: a daemon pointed at the real gateway with a
  wrong key (→ 401 on every call → mock fallback) rendered a red `⚠ 3 fallbacks`
  badge in the header next to HALT/Resume, tooltip "a provider errored; runs
  served by a backup / last: openai: status 401: Invalid API key". A healthy
  daemon shows no badge.

## Scope notes
- Pure front-end over the existing M280 status field; no backend change, no new
  dependency, no new control-plane call.
- Completes the M279→M280→M281 real-API arc: fix the dotted-tool-name 400 (M279),
  make the resulting silent fallback visible in CLI surfaces (M280), and surface
  it prominently in the live dashboard (M281).
- Demo artifacts (`webui-*.png`) remain gitignored.
