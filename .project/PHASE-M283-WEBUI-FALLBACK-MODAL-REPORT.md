# M283 — Web UI: clickable fallback badge → recent provider.fallback events

## Why
M280 surfaced provider fallbacks in `agt status`/`doctor` + a status field, and
M281 turned that field into a prominent header badge (`⚠ N fallbacks`). But the
badge is a dead-end: it shows a *count* and the *last* reason on hover — an
operator still can't see the individual fallback events (which provider failed,
when, and why each one happened) without a journal dig. This milestone makes the
badge actionable: clicking it drills straight into the underlying errors.

## What
- **`kernel/webui/dashboard.html`**:
  - `.fbbadge` CSS `cursor: help` → `cursor: pointer` (+ a hover brighten) so the
    chip reads as clickable; `updateFallbackBadge` tooltip gains a
    "click for details" hint.
  - `openFallbacks()` reuses the run-detail modal shell and the read-only
    `/api/journal` route, fetching `kind=provider.fallback&limit=30`. It renders
    the events newest-first: each row shows `seq`, `failed → next`, and
    `<local time> · <reason>` (via a new `fmtTime(ms)` helper over `ts_unix_ms`).
    Empty / error states handled.
  - The header badge is wired to `openFallbacks` at boot.

## Files
- `kernel/webui/dashboard.html` — CSS, `fmtTime`, `openFallbacks`, badge
  click-wiring, tooltip hint (edited).
- `kernel/webui/webui_test.go`:
  - **new** `TestJournalRouteForwardsKind` — asserts the `kind` + `limit` args
    reach `journal_grep` through the read-args proxy and a stray param is dropped
    (the route the modal depends on).
  - the dashboard-wiring test now also guards `function openFallbacks` +
    `provider.fallback` in the served page.

## Verification
- `go test ./kernel/webui/` — green; full suite **1893**, 68 packages, `go test
  ./...` exit 0.
- `gofmt -l` clean on the Go files; `go vet ./kernel/webui/` clean; `GOOS=linux`
  build clean; `go.mod` / `go.sum` unchanged.
- **Live-proven end-to-end**: a daemon with `AGEZT_DEMO_FAIL_PRIMARY=1` (the
  primary wrapped in an always-failing shim → mock fallback on every run)
  generated 3 `provider.fallback` events over 3 runs. `agt status` showed
  `fallbacks : 2`; `GET /api/journal?kind=provider.fallback` returned the 3
  events (`failed: mock-failshim → next: mock`,
  `reason: demo-shim: simulated primary failure`, each with `seq`/`ts_unix_ms`).
- **Live-proven in a real browser** (Playwright): opened the dashboard, clicked
  the `⚠ 3 fallbacks` badge → modal titled "Provider fallbacks" listing
  `3 fallback event(s)`, three rows newest-first (seq 17/10/3),
  `mock-failshim → mock` + `9:26:00 AM · demo-shim: simulated primary failure`.
  Only console error was a benign favicon 401. Screenshots gitignored.

## Scope notes
- Pure front-end over an existing read route; no backend change, no new
  control-plane command, no new dependency.
- Extends the real-API observability arc: M279 fixed the dotted-tool-name 400 →
  M280 surfaced the resulting silent fallback in CLI/status → M281 badged it in
  the Web UI → M283 makes that badge drillable to the per-event detail.
- The `/api/journal` route already allowlisted `kind`; this is the first Web UI
  surface to use it for anything other than the per-run correlation filter.
