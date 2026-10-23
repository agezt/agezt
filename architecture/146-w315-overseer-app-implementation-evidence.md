# W3.15 — the overseer's implementation moves to the app layer: the plugin reaches only the contract

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 step 8 — the agents
module's last item — and the plugin layer's "reach into a module" edges.

**Scope:** the second half of the overseer slice. W3.14 moved the rows the
overseer names down to the contract layer, so the tool stopped importing the
roster module. What was left were the two edges that could not be closed by
moving a type: the tool held the **live kernel** and opened the **board store**
itself. This slice moves the **implementation** that answers the port into the
app layer.

## What moved

| Before | After | What it is |
|---|---|---|
| `plugins/tools/overseertool.Source` (a 25-method interface) | `kernel/contract/overseerapi.Source` (new, L1) | the supervisory port: read, intervene, edit, search, bulk, wake, repair |
| `plugins/tools/overseertool.BulkResult`, `.SearchFilter` | `kernel/contract/overseerapi` (new, L1) | the two rows the port carries |
| `plugins/tools/overseertool/kernelsource.go` | `kernel/app/overseer/source.go` | the kernel-backed implementation: the read/intervene adapters, PATCH `EditAgent`, `CreateAgent`, `DeleteAgent`, `GetAgent`, `CloneAgent`, the fleet lock |
| `plugins/tools/overseertool/kernelsource_ops.go` | `kernel/app/overseer/ops.go` | bulk ops, `WakeAgent`, `SearchAgents`, `RepairAgent`, `RollbackRouting`, `ApplyRoutingChain`, `OpenHelp` |
| `plugins/tools/overseertool/kernelsource_chains.go` | `kernel/app/overseer/chains.go` | the task-model-chain read/set and its persistence |
| `plugins/tools/overseertool/repair.go` | `kernel/app/overseer/repair.go` | the repair brief, the reaper lookups, the proposal parse/clean/apply |
| `plugins/tools/overseertool` | keeps the tool | the op dispatcher, the schema, the views, and the port's aliases |
| `toolreg.KernelDeps` | gains `Overseer` | the port the daemon supplies |
| `plugins/builtintools` | binds `d.Overseer` | the tool no longer builds its own source |

## Why

Archcheck's plugin rule: a plugin (L6) may reach the **contract** and the
**platform** layers and nothing else. The overseer tool broke it twice: it
imported `kernel/runtime` (to hold the live kernel) and
`kernel/modules/board/board` (to open the board store). Moving the *rows* down
(W3.14) could not close those, because they are not types — they are the
implementation.

Where the implementation has to live is decided by who can see what:

- the live kernel is the **runs** module (L3),
- the board store is the **board** module (L3),
- a module may not reach another module's internals (P5),
- the **app** layer (L4) sits above both and may reach both.

So the port goes to the **contract** (L1) and the implementation goes to the
**app** (L4). The plugin keeps the tool and names the contract; the daemon
builds the implementation and injects it. That is the W3.12 ports recipe
applied to a plugin instead of a module.

## The ratchet went DOWN

**281 packages, 117 import violations, 13 call sites** — from 119/13. Both
`plugin-reach` edges of the overseer tool are gone (`kernel/runtime` and
`kernel/modules/board/board`), and the **two stale allowlist entries were
deleted** — a leftover entry fails the archcheck gate (the debt was paid, the
list must shrink).

## The proof

### Permanent tests

- **The identity assertion** (`plugins/tools/overseertool/identity_test.go`)
  states that the tool's `Source`/`BulkResult`/`SearchFilter` ARE the
  contract's, as **function types**: an assignment would also accept a copy
  with the same method set, while a function type requires identity. So a
  tool that ever declares its own port is reported at the seam.
- **The injected port** (`plugins/builtintools/overseer_test.go`) builds the
  real spec, hands it a fake port through `toolreg.KernelDeps.Overseer` and
  drives an op: the fake's answers must reach the tool. This is the test that
  would fail if the tool ever went back to building its own source — the
  wiring is the thing the slice changed, so the wiring is what is measured.
  Its sibling asserts that an **unwired** port is a clear error, not a panic
  and not a silent success.
- **The live kernel** (`kernel/app/overseer/source_test.go`) drives the moved
  implementation against a REAL kernel: the roster is visible through the
  port, an unknown agent is refused, a pause reaches the live roster, and the
  daemon-wide halt and resume travel through the kernel. The tool's own suite
  proves the op → port mapping with a fake; nothing proved that the
  implementation *behind* the port still reads and steers the live system.
- **The protections** — the same file — close a gap the move exposed: the
  guardian guard, the forced `System=false` on create and the PATCH semantics
  of the agent-facing edit live in the implementation, and the tool's suite
  drives a fake, so they were **never** measured. They are now.

### Mutation run

**12 mutations: 10 caught, 2 build errors (recorded separately).**

| # | Mutation | Result |
|---|---|---|
| M01 | the tool binds nothing instead of the injected port | caught |
| M02 | the tool declares its own `Source` instead of aliasing the contract's | **build error** — the identity assertion fires at the seam |
| M03 | `Agents` answers with nothing | caught |
| M04 | `SetAgentEnabled` ignores the flag it was given | caught |
| M05 | `GetAgent` reports an unknown agent as found | caught |
| M06 | `Halt` does not halt | caught |
| M07 | the guardian guard is removed (first anchor deleted the block and left `cur` unused) | **build error** — re-anchored (M07b) and caught |
| M08 | `CreateAgent` no longer forces `System=false` | caught |
| M09 | the edit stops being a PATCH and blanks the soul | caught |
| M10 | the port is never bound, even when the daemon supplies one | caught |
| M11 | the fleet lock no longer refuses a delete | caught |

M07's first anchor was a **build error**, not an escape: a build failure is
recorded separately and never counted as one. The re-anchored mutation (the
guard's condition neutered instead of the block deleted) is caught.

### Gates

The build, gofmt, vet, dead-code, archcheck (**281 packages, 117/13**),
structure (and its check), changelog-lint, docclaimscheck and staticcheck are
green, and so is the full test suite.

## What is deliberately left

- The overseer's **operations** are still tool-facing: the tool dispatches
  them, and they are not (yet) transport-facing typed operations in the
  control plane's registry. The ratchet is the proof of the layer rule, and
  the port is now where a typed operation would sit.
- The other plugin-reach edges (the config tool, the schedule tool, the
  guardians, the built-in tools, the introspector) each need their own slice.
- The **dated** reports (`.project/PHASE-*`, the dated scans and audits, the
  earlier evidence) describe the tree as it was and are left alone.
