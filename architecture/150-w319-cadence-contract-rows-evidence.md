# W3.19 — the schedule entry's row moves to the contract layer

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the same shape as W3.18, on the cadence side. The **introspector**
and the **guardians** both name the schedule entry's row; both are plugins, and
a plugin may reach the contract and the platform layers and nothing else — so a
row a plugin has to name cannot live in a module's implementation package.

## What moved

| Before | After |
|---|---|
| `cadence.Entry` and its rules | → `kernel/contract/cadenceapi` (new, L1): the row, `Validate`, `Forecast`, `Advance`, `SafeInterval`, `UsesInterval` |
| `cadence.FormatDays`, `cadence.ParseDays` and the day-mask helpers | → `kernel/contract/cadenceapi` (`MaskWeekdays`, `MaskWeekends` are exported now — the module's own tests read them) |
| `cadence.MinInterval`, the modes, the targets, the sources | → `kernel/contract/cadenceapi` |
| the system-task catalog (`SystemTaskInfo`, the task names, `SystemTasks`, `SystemTaskInfos`, `IsSystemTask`) | → `kernel/contract/cadenceapi` — `Entry.Validate` checks a system-task target, so the catalog has to be where the row is |
| `plugin-reach plugins/tools/introspecttool -> kernel/cadence` | **gone** |
| `plugin-reach plugins/builtinguardians -> kernel/cadence` | **gone** |

`kernel/cadence` keeps every name as an **alias** (and thin wrappers for the
functions), so the store, the engine, the ticker, the CLI and the schedule tool
are all unchanged. The store and the engine stay in the module — they are the
implementation.

Three rules about the row had to be **exported** rather than kept unexported:
`usesInterval`, `safeInterval` and `advance` are called by the module's store
and engine, and a method can only be defined in its type's package. They are
now `UsesInterval`, `SafeInterval` and `Advance` — the same rules about the
same row, with the W3.14 recipe's shape (the profile's rules live on the
contract row too).

## The ratchet went DOWN

**283 packages, 108 import violations, 13 call sites** — from 110/13. The
**two stale allowlist entries were deleted** — a leftover entry fails the
archcheck gate (the debt was paid, the list must shrink).

## The proof

### Permanent tests

- **The identity assertions** (`kernel/cadence/cadence_test.go`): the
  module's `Entry` and `SystemTaskInfo` ARE the contract's, as **function
  types** — an assignment would also accept a copy with the same fields, while
  a function type requires identity.
- **The wire shape** (`kernel/contract/cadenceapi/cadenceapi_test.go`) —
  **new, and never measured before**: the schedule entry's JSON keys are what
  the control plane serves, the Web UI reads and the store persists, and they
  are read **by name**. A renamed key is a silent break for every consumer
  outside the package, and no test in the package would notice. The test pins
  the exact key set (22 keys) and that the unset fields stay out of the wire.

### Mutation run

**9 mutations: 8 caught, 1 build error (recorded separately).**

| # | Mutation | Result |
|---|---|---|
| M01 | the schedule entry loses a field on the wire | caught |
| M02 | a wire key is renamed (`next_run_unix` → `next_run`) | caught |
| M03 | an empty field stops being omitted | caught |
| M04 | the module's row stops being the contract's row | **build error** — the identity assertion fires at the seam |
| M05 | the safe interval stops flooring (a busy-loop guard) | caught |
| M06 | the daily schedule stops using the daily slot | caught |
| M07 | the windowed schedule stops using the window slot | caught |
| M08 | the introspector reaches into the cadence module again | caught — **by the archcheck gate** |
| M09 | the guardians reach into the cadence module again | caught — by the archcheck gate |

M05–M07 are caught by the module's own timing tests — the DST fall-back guard,
the interval floor and the window walk — which now run against the contract's
implementation of the same rules. That is the point of moving a rule with its
row: the tests that guard it did not have to move with it.

### Gates

The build, gofmt, vet, dead-code, archcheck (**283 packages, 108/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **schedule tool** still reaches into the cadence module: it needs the
  store (`cadence.Store`), not just the rows.
- The **guardians** still reach into the roster module: they need
  `roster.Validate`, and the validation is not pure — it reads the trust
  ceiling through `kernel/edict`, and a contract package may not import a
  platform package. That is a slice of its own.
- The remaining plugin-reach edges: the standing tool, the database tool, the
  workboard tool, the skill tool, the forgetool, the MCP tool, the artifacts
  tool, the board tool, the introspector's kernel edge, the market plugin, the
  built-in tools and the built-in channels.
