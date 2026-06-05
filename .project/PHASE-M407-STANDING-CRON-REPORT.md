# M407 — Chronos cron triggers: standing orders that fire on schedule (SPEC-16 §4)

## Context
M404 made event-triggered standing orders fire; cron triggers (the spec's
canonical "brief me every morning", `cron: "0 8 * * *"`) were stored but inert.
The schedule engine (`kernel/cadence`) is interval/daily/window-based, not a
full 5-field cron, so this adds a small stdlib cron matcher + a minute ticker
rather than bending an order's arbitrary cron onto cadence.

## What
- **`kernel/standing/cron.go`** — `matchesCron(spec, t)`: a dependency-free
  5-field cron matcher (minute hour dom month dow) supporting `*`, values,
  ranges, steps (`*/n`, `a-b/n`), comma lists; Sunday as 0 or 7; the cron
  OR-rule when both dom and dow are restricted; a malformed spec never matches.
  `tickCron` fires every enabled cron-triggered order whose schedule matches the
  current minute, at most once per minute (dedup by minute stamp), dispatching
  `fire` on its own goroutine. `StartCron` runs the ticker (every 30s, so each
  minute is caught) until ctx cancellation; panic-recovered.
- **`cmd/agezt/main.go`** — `buildStandingRunner` now starts both the event
  runner (M404) and the cron ticker with the same FireFunc; banner reads
  "event + cron triggers".

## Verification
- **`kernel/standing/cron_test.go`**: `TestMatchesCron` (15 cases: daily,
  step `*/15`, weekday `1-5`, weekend, Sunday 0/7, day-of-month, and the
  malformed/out-of-range/4-field rejections); `TestTickCron_FiresOncePerMinute`
  (fires at 08:00, not twice in the same minute, not at 09:00);
  `TestTickCron_SkipsDisabled`.
- **Negative control:** breaking the minute check in `matchesCron`
  (`t.Minute()+100`, out of range) → `TestMatchesCron` FAILs; restored
  byte-identical.
- **Live demo** (echo mock): added an order with cron `* * * * *`; within the
  wait window two `standing.fired` events appeared with
  `trigger_subject:"cron:* * * * *"` — once per minute boundary, confirming both
  firing and the per-minute dedup end to end.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2253** passing (was 2250; +3). CHANGELOG (Added, user-visible).

## Scope notes
- Both standing-order trigger types now fire: event (M404) and cron (M407), with
  the full operator surface (CLI + `why` + web panel). Remaining Chronos polish:
  the `max_trust` initiative ceiling (the budget ceiling is applied; trust needs
  a per-run edict cap) and observers/salience/briefing wiring to Pulse (today an
  order's plan runs as a normal governed run). The core "persistent goal that
  fires on time or on an event" is complete.
