# M408 — Standing-order max_trust initiative ceiling (SPEC-16 §4)

## Context
A standing order carries `initiative.max_trust` — "the ceiling for autonomous
action here". M404/M407 fired orders bounded by the budget cap, but the trust
ceiling was not enforced: a fired order ran at the daemon's normal policy levels.
This wires the ceiling so a persistent goal can act on its own yet stay bounded.

## What
- **`kernel/edict`** — `DecideWithCeiling(cap, input, ceiling)`: clamps the
  looked-up capability level to at most `ceiling` before the level→decision
  mapping, so an L4 (auto-allow) capability becomes Ask, or at ceiling L0, Deny.
  The clamp note is appended to the reason. The hard-deny floor and
  unknown-capability default-deny are unaffected — a ceiling only ever tightens.
  `Decide` now delegates to `DecideWithCeiling(…, LevelAllow)` (no clamp; identical
  behaviour).
- **`kernel/runtime`** — `WithTrustCeiling(ctx, level)` / `trustCeilingFromCtx`;
  `policyHook` uses `DecideWithCeiling` when the run carries a ceiling, else
  `Decide`.
- **`cmd/agezt/main.go`** — the standing-order FireFunc parses the order's
  `MaxTrust` (`edict.ParseTrustLevel`) and sets `WithTrustCeiling` on the run, so
  every tool call in a fired order is gated against the ceiling.

## Verification
- **`kernel/edict/ceiling_test.go`** `TestDecideWithCeiling`: no ceiling → allow;
  ceiling L2 → clamped to Ask (WouldAsk); ceiling L0 → deny; a ceiling never
  loosens an L0 cap; hard-deny holds regardless of ceiling; `Decide` ==
  `DecideWithCeiling(LevelAllow)`.
- **`kernel/runtime/ceiling_internal_test.go`** `TestPolicyHook_TrustCeiling`
  (white-box): an L4 shell call is allowed with no ceiling, denied (reason names
  the ceiling) under `WithTrustCeiling(LevelDeny)`.
- **Negative control:** replacing the `policyHook` ceiling branch with a plain
  `Decide` → the runtime test FAILs (call allowed instead of denied); restored
  byte-identical.
- **Live demo** (mock `AGEZT_DEMO_LOOP=1`): a cron standing order
  (`--max-trust L0`) fired; its shell calls were denied with
  `"capability set to L0 (deny) (clamped to ceiling L0)"` in the journal.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2255** passing (was 2253; +2). CHANGELOG (Added, user-visible).

## Scope notes
- Chronos standing orders now: model+CRUD (M403), event runner (M404), `why`
  (M405), web panel (M406), cron triggers (M407), **budget + trust ceilings
  (M408)**. The remaining piece is observers/salience/briefing wiring to Pulse —
  today a fired order runs its plan as a normal governed run rather than driving
  the observe→salience→brief pipeline. The "bounded persistent goal that fires on
  time or event" is complete.
