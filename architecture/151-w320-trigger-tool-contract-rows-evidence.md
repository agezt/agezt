# W3.20 — the two trigger tools name their rows where they live

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the last two trigger tools. The **schedule tool** (the agent's own
cron) and the **standing tool** (the agent's own durable event/cron wake
rules) each reached into a module — one for the **rows** they speak and one
for the **store** they drive. Both are plugins, and a plugin may reach the
contract and the platform layers and nothing else.

## What moved

| Before | After |
|---|---|
| `plugins/tools/schedule` named `cadence.Entry`, `cadence.SystemTasks`, `cadence.IsSystemTask`, `cadence.ParseDays` and the target constants | → `kernel/contract/cadenceapi` — the rows already lived there (W3.19); the tool now names their real home |
| `plugins/tools/schedule` held the store as `*cadence.Store` (the module's concrete type) | → `cadenceapi.Store`, a **port** in the contract layer: the eleven methods the tool drives. The module's `*cadence.Store` implements it |
| `plugins/tools/standingtool` named `standing.Order`, `standing.Trigger`, `standing.Initiative`, `standing.InitiativeMode` and the two mode constants | → `kernel/contract/standingapi` — the rows already lived there (W3.18) |
| `plugins/tools/standingtool` named `roster.Profile` | → `agentapi.Profile` — the profile row's home (W3.14) |
| `plugins/tools/standingtool` reached for the roster through the host's `Roster() *roster.Store` | → the daemon hands it the **profile lookup** (`BindAgentLookup`), the same shape the schedule tool already used (W3.17) |
| `plugins/tools/standingtool` read the store through the host's `Standing() *standing.Store` | → the host's `StandingList() []standingapi.Order` — the kernel already had it |
| `plugin-reach plugins/tools/schedule -> kernel/cadence` | **gone** |
| `plugin-reach plugins/tools/standingtool -> kernel/standing` | **gone** |
| `plugin-reach plugins/tools/standingtool -> kernel/modules/agents/roster` | **gone** |

Nothing moved **out** of a module this time: W3.18 and W3.19 had already put
the rows in the contract layer, and the modules kept their names as aliases.
What was left was the plugin's half — it still named the *alias*, and the
alias lives in a module's implementation package, so each tool paid a
`plugin-reach` edge for a row that was never theirs to name.

Two rules travelled with the rows, and one of them was a **duplicate**:

- The standing tool read the manager of a managed sub-agent by hand
  (`ParentAgent`, else `OwnerAgent`). W3.17 had already moved that rule onto
  the profile row as `agentapi.Profile.ManagedBy()` — the schedule tool uses
  it, and so does the overseer's repair surface. The standing tool now uses
  the one rule instead of its third copy.
- The standing tool's three refusals (retired, paused, managed) are the
  schedule tool's three refusals, in the same order, over the same row. They
  stay separate surfaces — a standing order and a schedule are refused for
  different reasons — but they now read the same row.

## The ratchet went DOWN

**283 packages, 105 import violations, 13 call sites** — from 108/13. The
**three stale allowlist entries were deleted** — a leftover entry fails the
archcheck gate (the debt was paid, the list must shrink).

## The proof

### Permanent tests

- **The identity assertions** (`plugins/tools/standingtool/standing_test.go`):
  the module's `Order`, `Trigger`, `Initiative` and `InitiativeMode` and the
  roster's `Profile` ARE the contract's, as **function types** — an assignment
  would also accept a copy with the same fields, while a function type
  requires identity.
- **The store port** (`kernel/cadence/store_port_test.go`): a compile-time
  assertion that the module's store IS the contract's port, and a test that
  the store still works when it is *held as the port* — the tool never sees
  the concrete type, so a method that only worked on `*Store` would slip
  through every other test.
- **The wiring** (`plugins/builtintools/trigger_ports_test.go`): against a
  **real kernel**, the standing spec's managed sub-agent is still refused
  with its manager named (so the roster lookup really crossed the wire), and
  a schedule the tool creates lands in the **kernel's own** cadence store (so
  the port really carries the live store). Two compile-time assertions pin
  the shapes: the kernel IS the standing tool's host, and the kernel's
  cadence store IS the contract's port.
- **The day mask** (`plugins/tools/schedule/schedule_test.go`): the mask the
  tool parses is asserted **exactly** — "Mon-Fri" is the weekday mask, "weekends"
  is Sunday and Saturday. The old assertion was "non-zero", which any wrong
  mask satisfies.

### Mutation run

**18 mutations: 17 caught, 1 build error (recorded separately).** One
mutation **survived the first round** — the day mask (M12) — and the test was
strengthened before it was run again.

| # | Mutation | Result |
|---|---|---|
| M01 | the standing tool stops refusing a managed sub-agent | caught |
| M02 | the retired-agent refusal is dropped | caught |
| M03 | the paused-agent refusal is dropped | caught |
| M04 | the hint stops naming the manager | caught |
| M05 | the conservative default mode changes from `ask` | caught |
| M06 | the acting agent is not bound to the order | caught |
| M07 | the list op returns nothing | caught |
| M08 | the daemon stops wiring the roster lookup | caught — **by the wiring test** |
| M09 | the schedule tool's system-task enum is wrong | caught |
| M10 | the schedule tool's `Bind` drops the store | caught — **by the wiring test** |
| M11 | the day mask is dropped (every day) | caught |
| M12 | the day mask is wrong but non-zero | **survived the first round** — the test asserted only "non-zero"; it was strengthened and M12 was run again → caught |
| M13 | the contract's store port loses a method | **build error** — the port is a promise, not a comment |
| M14 | the allowlist gets the paid debt back | caught — **by the archcheck gate** |
| M15 | the unwired roster lookup stops being tolerated | caught |
| M16 | the weekend mask is read as the weekday mask | caught — after the strengthening |
| M17 | the managed-sub-agent refusal is inverted | caught |
| M18 | the daemon stops binding the standing host | caught — by the wiring test |

M08, M10 and M18 are caught only by the **wiring** tests: the tool's own
suite proves the tool, and nothing but a test against a real kernel proves
that the daemon still hands it what it needs. That is the gap this slice's
new tests close.

### Gates

The build, gofmt, vet, dead-code, archcheck (**283 packages, 105/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **guardians** still reach into the roster module: they need
  `roster.Validate`, and the validation is not pure — it reads the trust
  ceiling through `kernel/edict`, and a contract package may not import a
  platform package. That is a slice of its own.
- The remaining plugin-reach edges: the database tool, the workboard tool,
  the skill tool, the forgetool, the MCP tool, the artifacts tool, the board
  tool, the introspector's kernel edge, the market plugin, the built-in tools
  and the built-in channels.
