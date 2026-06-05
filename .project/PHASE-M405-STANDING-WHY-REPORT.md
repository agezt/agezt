# M405 — `agt standing why`: a standing order's life story (SPEC-16 §4)

## Context
M403–M404 gave standing orders a management surface and an event-trigger runner.
The remaining operator gap was observability: no way to see what a given order has
done — when it was created, paused/resumed, and every time it fired. This adds
`agt standing why <id>`, mirroring `agt skill history`.

## What
- **`kernel/controlplane/standing.go`** — `handleStandingWhy` folds the journal
  for every `standing.*` event whose payload `id` matches the order, returning
  them chronologically (seq, kind, correlation, ts, payload).
- **`kernel/controlplane/protocol.go` / `server.go`** — `CmdStandingWhy` +
  routing.
- **`cmd/agt/standing.go`** — `agt standing why <id> [--json]`: renders each event
  as `seq=N kind (action) ← trigger_subject`, so a fire shows the subject that
  triggered it.

## Verification
- **`kernel/controlplane/standing_test.go`** `TestStanding_Why`: after
  create + pause, `why` returns ≥2 events all scoped to that order id; an unknown
  id returns 0.
- **Negative control:** removing the `p["id"] != id` scope filter in
  `handleStandingWhy` → the unknown-id assertion FAILs (2 events leak); restored
  byte-identical.
- **Live demo** (echo mock): an order created → paused → resumed, then a run
  fired it; `agt standing why <id>` printed:
  `standing.created` / `standing.updated (paused)` / `standing.updated (resumed)`
  / `standing.fired ← agent.agent-run-….task`.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2249** passing (was 2248; +1). CHANGELOG (the `why` subcommand).

## Scope notes
- Chronos arc: management (M403), event runner (M404), `agt standing why` (M405).
  Remaining: cron-trigger wiring (needs a cron→schedule-engine bridge — the
  cadence store is interval/daily/window, so a full 5-field cron needs a
  translation or a small cron parser), the `max_trust` initiative ceiling (budget
  cap is applied; trust needs a per-run edict cap), a web Standing panel, and
  observers/salience/briefing wiring to Pulse.
