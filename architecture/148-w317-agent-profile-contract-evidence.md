# W3.17 — two more tools name the agent profile where it lives, and the "who manages this agent?" rule gets one home

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the same shape as W3.16, on the agents module this time. The config
tool and the schedule tool each reached into the roster module for **one type**
— the agent profile — and nothing else. That row moved to the contract layer
in W3.14, so the module only kept an alias; the two tools named the alias, and
the alias lives in a module's implementation package.

The slice also found something the rename exposed: **the "who manages this
agent?" rule was implemented twice** — once in the schedule tool and once in
the overseer's repair surface — with the same trigger and the same lookup. Two
copies of one rule are two copies that can drift apart, so the rule moved to
the row it is about.

## What changed

| Before | After |
|---|---|
| `plugins/tools/{config,schedule}` → `roster.Profile` | → `agentapi.Profile` (`kernel/contract/agentapi`) — the row's real home |
| `plugin-reach plugins/tools/config -> kernel/modules/agents/roster` | **gone** |
| `plugin-reach plugins/tools/schedule -> kernel/modules/agents/roster` | **gone** |
| the manager lookup, twice (schedule tool + overseer app) | → `agentapi.Profile.ManagedBy()` — one rule, one home |

The two surfaces keep their own wording (a schedule is refused for a different
reason than a repair), but they now agree on **who** manages the agent and on
**when** an agent counts as managed — which is the part that must not drift.

## The ratchet went DOWN

**281 packages, 112 import violations, 13 call sites** — from 114/13. The
**two stale allowlist entries were deleted** — a leftover entry fails the
archcheck gate (the debt was paid, the list must shrink).

## The proof

### Permanent tests

- **The rule** (`kernel/contract/agentapi/agentapi_test.go`): `ManagedBy`
  returns the parent agent, or the owner when there is no parent, the parent
  wins over the owner, nothing manages an unmanaged profile, a blank slug is
  not a manager, and the manager is trimmed. Six cases, one home.
- **The schedule hint** (`plugins/tools/schedule/coverage_more_test.go`)
  already asserted that the hint names the manager — it now gets that
  manager from the rule.
- **The overseer hint** (`kernel/app/overseer/impl_test.go`) asserted only
  that the hint was **not empty**. That is the "something happened" kind of
  assertion: a hint that dropped the manager's slug — the one piece of
  information the caller needs — would have passed. The three cases now
  assert the manager's slug (and the "managed sub-agent" wording when there
  is no manager).

### Mutation run

**9 mutations: 9 caught, 0 build errors.**

| # | Mutation | Result |
|---|---|---|
| M01 | the manager is the parent only (the owner fallback is gone) | caught |
| M02 | nothing ever manages anybody | caught |
| M03 | the manager is not trimmed | caught |
| M04 | the owner wins over the parent | caught |
| M05 | the schedule's hint names a hardcoded manager | caught |
| M06 | the overseer's hint names a hardcoded manager | caught |
| M07 | the schedule's hint stops naming the manager | caught |
| M08 | the config tool reaches into the roster module again | caught — **by the archcheck gate**; the build still passes, because the module's alias is the same type, so the gate is the only thing that can see it |

M08 is the honest proof of the slice's shape: a plugin that names the module's
alias instead of the contract row compiles and behaves **identically**. Only
the layer rule can tell the difference, which is why the ratchet and the
allowlist are the measurement, and why a stale allowlist entry has to go.

### Gates

The build, gofmt, vet, dead-code, archcheck (**281 packages, 112/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **standing tool** still reaches into the roster module: it needs the
  store (`roster.Open`, `roster.Store`), not just the row.
- The **guardians** still reach into it: they need `roster.Validate`, and
  the validation is not pure — it reads the trust ceiling through
  `kernel/edict`, and a contract package may not import a platform package.
  That is a slice of its own.
- The remaining plugin-reach edges: the database tool, the workboard tool, the
  skill tool, the forgetool, the MCP tool, the introspector, the market
  plugin, the built-in tools and the built-in channels.
