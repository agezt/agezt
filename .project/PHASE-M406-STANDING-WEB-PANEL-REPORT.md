# M406 — Web Standing panel (SPEC-16 §4 / SPEC-07)

## Context
The Live Monitor had panels for runs, skills, schedules, etc., but not for
Chronos standing orders. This adds a read-only Standing panel so an operator can
see their persistent goals at a glance (management stays in `agt standing`).

## What
- **`kernel/webui/webui.go`** — `/api/standing` → `CmdStandingList` (read-only).
- **`kernel/webui/dashboard.html`** — a Standing panel section + a `standing`
  renderer: enabled/total count, an on/off chip per order, name, trigger count,
  and initiative mode (textContent only — XSS-safe). A `standing.*` event triggers
  a panel refresh, like the other panels.

## Verification
- **`kernel/webui/webui_test.go`** `TestDashboard_RendersStandingPanel` (panel +
  body + renderer + count field + empty state); `TestAPIReadOnly` updated to
  whitelist `standing_list` (it's a read); `TestDashboard_NoUnsafeDOMSinks` green.
- **Negative control:** renaming the panel's empty-state string in the dashboard
  → the lock-in test FAILs; restored byte-identical.
- **Live Playwright demo:** added an order (`--cron … --event "github.>" --mode
  act_or_ask`); the Standing panel rendered `1 enabled · 1 total`, an `on` chip,
  `portfolio watch`, `2 trigger(s)`, `act_or_ask`.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2250** passing (was 2249; +1). No CHANGELOG beyond the dashboard surface.

## Scope notes
- Chronos operator surface is now CLI (`agt standing list|add|pause|resume|remove|
  why`) + web (Standing panel). Remaining Chronos work: cron-trigger wiring (needs
  a cron→schedule bridge), the `max_trust` initiative ceiling, and observers/
  salience/briefing wiring to Pulse.
