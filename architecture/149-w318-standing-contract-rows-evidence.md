# W3.18 — the standing order's row moves to the contract layer

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the standing order is an autonomous agent the operator leaves
switched on — "when this event fires, do this". The **introspector** (the tool
that reports the daemon's own live state) and the **guardians** (the plugin
that seeds and reconciles the autonomous fleet) both reach into the standing
module for its rows. Both are plugins, and a plugin may reach the contract and
the platform layers and nothing else — so a row a plugin has to name cannot
live in a module's implementation package.

## What moved

| Before | After |
|---|---|
| `standing.Order`, `standing.Trigger`, `standing.Initiative`, `standing.TriggerType`, `standing.InitiativeMode` | → `kernel/contract/standingapi` (new, L1) |
| `standing.Validate` | → `standingapi.Validate` (the module keeps a thin wrapper) |
| `(InitiativeMode).MaxAutonomyTrust()` | → `standingapi.InitiativeMode.MaxAutonomyTrust()` (a method can only be defined in its type's package) |
| `plugin-reach plugins/tools/introspecttool -> kernel/standing` | **gone** |
| `plugin-reach plugins/builtinguardians -> kernel/standing` | **gone** |

`kernel/standing` keeps every name as an **alias**, so the store, the runner,
the control plane and the standing tool are all unchanged. The store and the
runner stay in the module — they are the implementation.

The same recipe as W3.14 and W3.16: the row moves to the contract, the module
keeps its names, the plugin names the contract.

## The ratchet went DOWN

**282 packages, 110 import violations, 13 call sites** — from 112/13. The
**two stale allowlist entries were deleted** — a leftover entry fails the
archcheck gate (the debt was paid, the list must shrink).

## The proof

### Permanent tests

- **The identity assertions** (`kernel/standing/standing_test.go`): the
  module's five row types ARE the contract's, as **function types** — an
  assignment would also accept a copy with the same fields, while a function
  type requires identity.
- **The wire shape** (`kernel/contract/standingapi/standingapi_test.go`) —
  **new, and never measured before**: the standing order's JSON keys are what
  the control plane serves, the Web UI reads and the store persists, and they
  are read **by name**. A renamed key is a silent break for every consumer
  outside the package, and no test in the package would notice, because the
  package writes and reads the same struct. The test pins the exact key set
  of the order, of the trigger and of the initiative, and that the unset
  fields stay out of the wire.

### Mutation run

**9 mutations: 8 caught, 1 build error (recorded separately).**

| # | Mutation | Result |
|---|---|---|
| M01 | the standing order loses a field on the wire | caught |
| M02 | a wire key is renamed | caught |
| M03 | an empty field stops being omitted (`budget_per_run_mc`) | **escaped the first round** — the omits-empty assertion looked at the order's top level and the initiative's `max_trust`, but not at the initiative's second field; the test was strengthened and the mutation re-run → caught |
| M04 | the module's row stops being the contract's row | **build error** — the identity assertion fires at the seam (`cannot use func(Order) {} as func(standingapi.Order)`) |
| M05 | the module's `Validate` stops validating | caught |
| M06 | the autonomy ceiling moves (`ask` → `L2`) | caught |
| M07 | the introspector reaches into the standing module again | caught — **by the archcheck gate**; the build still passes, because the alias is the same type |
| M08 | the guardians reach into the standing module again | caught — by the archcheck gate |
| M09 | the trigger's empty subject stops being omitted | caught |

M03's escape is the discipline working: a mutation that survives is a missing
assertion, so the assertion was added and the mutation was re-run. A build
error is recorded separately and never counted as an escape.

### Gates

The build, gofmt, vet, dead-code, archcheck (**282 packages, 110/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **standing tool** still reaches into the standing module: it needs the
  store (`standing.Open`, `standing.Store`), not just the rows.
- The **cadence rows** (`cadence.Entry` and its modes) are the same shape of
  work — the introspector and the guardians name them too.
- The **guardians** still reach into the roster module: they need
  `roster.Validate`, and the validation is not pure — it reads the trust
  ceiling through `kernel/edict`, and a contract package may not import a
  platform package. That is a slice of its own.
