# W3.14 — the roster's rows move to the contract layer: the overseer tool stops reaching into the roster module

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 step 8 — the agents
module's remaining item — and the plugin layer's "reach into a module" edges.

**Scope:** the first half of the overseer slice. The roster's profile row and
the board's message row move down to the contract layer, so the **overseer
tool** (a plugin) no longer imports the roster module at all.

## What moved

| Before | After | What it is |
|---|---|---|
| `kernel/modules/agents/roster.Profile` (+ `RetryPolicy`, `HealthPolicy`, `SelfRepairPolicy`, `NoisePolicy`, `AgentLifecycle`, `AgentTask`, `RepairResult`) | `kernel/contract/agentapi` (new, L1) | the agent's identity row and the policies that hang off it, plus the repair outcome |
| `kernel/modules/board/board.Message` | `kernel/contract/boardapi` (new, L1) | the board's message row |
| `kernel/modules/agents/roster` | unchanged in shape | the store, the validation, the normalization — and **aliases** of the contract's rows |
| `kernel/modules/board/board` | unchanged in shape | the store — and an **alias** of the contract's row |
| `plugins/tools/overseertool` | imports the contract | the tool's `Source` interface and its helpers are typed on the contract's rows |

The Profile's three rules (`Kind`, `AllowsDirectCall`, `AllowsDelegationFrom`)
moved with the row: they are pure rules over the row, so they belong with it.

## Why

Archcheck's plugin rule: a plugin (L6) may reach the **contract** and the
**platform** layers, and nothing else. The overseer tool reached into
`kernel/modules/agents/roster` — and for **types only**: its `Source` interface
returns `[]roster.Profile`, takes a `roster.Profile` as the edit payload, and
its helpers take a profile. A row the overseer must name is a contract between
the module that owns the roster and the tool that steers it, so it belongs at
the bottom.

## The ratchet went DOWN

**279 packages, 119 import violations, 13 call sites** — from 120/13, because
`plugin-reach plugins/tools/overseertool -> kernel/modules/agents/roster` is
gone. No allowlist entry was added; one was **deleted** (a stale entry fails
the archcheck gate — the debt was paid, the list must shrink).

The tool still reaches into `kernel/runtime` (it holds the live kernel) and
into the board module (it opens the board store). Those two edges need the
**implementation** to move — see "What is deliberately left" — and they are the
second half of this slice.

## The proof

### Permanent tests

The identity assertions (the W3.13 recipe, applied twice):
`kernel/modules/agents/roster/identity_test.go` and
`kernel/modules/board/board/identity_test.go` state that the module's row and
the contract's row are the **same type** — as a **function type**, because an
assignment would also accept a copy with the same fields while a function type
requires identity. So a module that ever declares its own copy is reported at
the seam instead of at the far end of the wiring.

And the **wire shape** got measured, which it never had: the profile's and the
message's JSON keys are what the Web UI, the SDK and the persisted files read
by name, so a renamed key is a silent break for every consumer that does not
live in the package — and no test in the package would notice, because the
package writes and reads the same struct. `TestProfileWireShape` and
`TestMessageWireShape` pin the key sets (and refuse a camelCase twin).

`TestAllowsDirectCall` closes the third case of the direct-call policy (an
explicit `true`), which the mutation run showed was the one uncovered.

### Mutation run

**8 mutations: 6 caught, 2 build errors (recorded separately).**

| # | Mutation | Result |
|---|---|---|
| M01 | `Kind()` forgets the system guardians | caught |
| M02 | `Kind()` forgets the managed sub-agents | caught |
| M03 | an explicitly callable agent is treated as managed | caught |
| M04 | every agent accepts delegation from anyone | caught |
| M05 | the roster copies the contract row instead of aliasing it | **build error** — the identity assertion fires at the seam |
| M06 | the board copies the contract row instead of aliasing it | **build error** — the identity assertion fires at the seam |
| M07 | the profile's `task_type` key changes | caught (after the wire-shape test was added) |
| M08 | the message's `reply_to` key changes | caught (after the wire-shape test was added) |

M07 and M08 **escaped the first round**: nothing in the package measured the
wire shape, so a renamed key passed every test. The two wire-shape tests are
what closed them — the same shape of gap as W3.13's M10, one layer down.

### Gates

The build, gofmt, vet, dead-code, archcheck (**279 packages, 119/13**),
structure (and its check), changelog-lint, docclaimscheck and staticcheck are
green, and so is the full test suite.

## The operator-visible wire change

The alias approach has one honest cost, and it was paid here: the JSON decode
error for a malformed profile or task now names the **contract** package, not
the module. `{"profile":"x"}` is rejected with `args.profile: json: cannot
unmarshal string into Go value of type agentapi.Profile` where it used to say
`roster.Profile`; the task row says `agentapi.AgentTask` where it used to say
`roster.AgentTask`. The rejection, its wording and its cause are unchanged —
only the type's package name in the message moved.

That is a small operator-visible change, so it is recorded rather than hidden:
three expectations in `kernel/app/roster` were updated to the new text, and the
CHANGELOG says so. The alternative — keeping a module-local named type — would
have preserved the message but broken the "one row, one home" rule that makes
the identity assertions possible at all.

## What is deliberately left

- The overseer tool still holds the live kernel and opens the board store, so
  two plugin-reach edges remain (`kernel/runtime`, the board module). Closing
  them means moving the **implementation** out of the plugin — to the app
  layer, which is the only layer that can see the roster, the board and the
  run controls at once. That is the second half of the overseer slice, and it
  is the roadmap's end state for the agents module: the tool becomes an L6
  caller of typed app operations.
- The other plugin-reach edges (the config tool, the schedule tool, the
  guardians, the built-in tools, the introspector) each need their own slice.
- The **dated** reports (`.project/PHASE-*`, the dated scans and audits, the
  earlier evidence) describe the tree as it was and are left alone.
