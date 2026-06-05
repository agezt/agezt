# M411 — Standing-order scope grounding (SPEC-16 §4 scope.entities)

## Context
The `Order.ScopeEntities` field existed but was neither settable from the CLI nor
used at runtime. The SPEC frames scope as "what entities (world-model refs)" an
order watches. This makes scope meaningful: a fired order's run is grounded in
its scope, and the scope is settable via the CLI.

## What
- **`kernel/standing/runner.go`** — `ScopedIntent(o, intent)`: when the order
  names scope entities, prefix the intent with a one-line scope note ("Scope (what
  this standing order watches): …"); no entities → intent unchanged.
- **`cmd/agezt/main.go`** — the FireFunc grounds the run via `ScopedIntent` before
  applying budget/trust ceilings.
- **`cmd/agt/standing.go`** — `--scope ent1,ent2` on `agt standing add` sets
  `scope_entities`; usage updated.

## Verification
- **`kernel/standing/runner_test.go`** `TestScopedIntent`: entities → prefixed
  note + the plan preserved; no entities → unchanged.
- **Negative control:** forcing `ScopedIntent` to return the intent unchanged →
  the test FAILs; restored byte-identical.
- **Live demo** (echo mock): `agt standing add --scope "project:portfolio,
  repo:agezt"` then a triggering run → the fired order's `standing.fired` intent
  was `"Scope (what this standing order watches): project:portfolio, repo:agezt.\n\nreview"`.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2258** passing (was 2257; +1). CHANGELOG (Added, user-visible).

## Scope notes
- Standing-order CLI now sets every meaningful field: triggers (cron/event),
  plan, mode, max_trust, budget, scope, channel. The remaining `observers`
  field stays unexposed because it references named observers the DSL doesn't
  define a registry for — wiring it would require inventing that config, which the
  build rules forbid. SPEC-16 §4 Chronos is complete for everything buildable +
  verifiable offline.
