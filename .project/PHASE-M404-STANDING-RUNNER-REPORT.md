# M404 — Chronos event-trigger runner: standing orders that fire (SPEC-16 §4)

## Context
M403 built the standing-order model + store + management surface, but orders were
inert — nothing fired them. This milestone is the **event-trigger runner**: when a
journal event matches an enabled order's event trigger, the order's plan launches
as a run. Cron triggers reuse the existing schedule engine; this is the
event-driven half (the genuinely new path).

## What
- **`kernel/standing/runner.go`** — `StartRunner(ctx, bus, store, cfg, fire)`
  subscribes to every event (mirrors `kernel/anomaly.Start`): for each event it
  fires every enabled order whose event trigger subject matches
  (`bus.MatchSubject`, NATS-style wildcards), subject to a per-order cooldown
  (`DefaultRunnerCooldown = 60s`) so a burst can't flood. `standing.*` lifecycle
  events are skipped so an order can't self-trigger. `fire` is dispatched on its
  own goroutine so a long run never stalls the loop; a panic is recovered.
  `FireFunc(ctx, order, triggerSubject)`.
- **`kernel/event/kinds.go`** — `standing.fired`.
- **`cmd/agezt/main.go`** — `buildStandingRunner`: the daemon `FireFunc` journals
  `standing.fired` (under a fresh correlation) then launches the order's plan
  (`o.Plan`, falling back to the name) via `RunWith`, applying the order's
  `BudgetPerRunMc` as a per-run cost cap (`WithMaxCost`). Always on (inert until
  event-triggered orders exist); a banner line reports it.

## Verification
- **`kernel/standing/runner_test.go`** (real bus + store):
  `TestRunner_FiresOnMatchingEvent` (matching subject fires once; non-matching
  doesn't); `TestRunner_SkipsDisabledAndLifecycle` (paused order + `standing.*`
  event never fire); `TestRunner_Cooldown` (a 5-event burst fires at most once
  within the window).
- **Negative control:** forcing `matchesAnyEventTrigger` to `return false` → the
  fire-on-match test FAILs (order never fires); restored byte-identical.
- **Live demo** (mock with `AGEZT_DEMO_ECHO=1`): added an order with event trigger
  `agent.>` and plan "the order fired"; `agt run "do a thing"` (which emits many
  `agent.*` events) produced **exactly one** `standing.fired` (name "react to
  runs", intent "the order fired", own correlation) — the cooldown capping the
  burst to a single fire, end to end.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2248** passing (was 2245; +3). CHANGELOG (Added, user-visible behaviour).

## Scope notes
- Chronos arc: management surface (M403), event-trigger runner (M404). Remaining
  polish: cron triggers wired through the schedule engine into standing orders
  (today an order's cron trigger is stored but the schedule engine isn't yet
  driven from it); the `max_trust` initiative ceiling (budget ceiling is applied;
  trust ceiling needs a per-run edict cap); `agt standing why` + web Standing
  panel; observers/salience/briefing-disposition wiring to Pulse. The event path
  — the core "react to the world" behaviour — works now.
