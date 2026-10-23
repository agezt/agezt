# W3.24 — the introspector's rows move to the contract layer, and its implementation to the app layer

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **introspector's rows** — the daemon's own at-a-glance health
snapshot and the reaper's fleet scan — and the **live-state surface** the
tool reads them through. The tool is the **introspector**
(`plugins/tools/introspecttool`), and it is a plugin: a plugin may reach the
contract and the platform layers and nothing else.

## What moved

| Before | After |
|---|---|
| `plugins/tools/introspecttool` named `Overview`, `ReaperOverview`, `ReaperAgent`, `ReaperDegradedAgent`, `ReaperMisconfiguredAgent` and `ReaperReport` | → `kernel/contract/introspectapi` — the rows' new home |
| `plugins/tools/introspecttool/kernelsource.go` built the rows **from the live kernel** (`*runtime.Kernel`) | → `kernel/app/introspect/source.go` — the app layer's adapter. The app layer is the one that can see both the kernel and the contract; the plugin may not name the kernel at all |
| the tool's `Source` interface (declared in the plugin) | → the contract's **port** (`introspectapi.Source`), with the four reads it has always had: the overview, the reaper scan, the schedules, the standing orders |
| the plugin bound the adapter itself (`it.Bind(introspecttool.NewKernelSource(d.K))`) | → the **daemon** hands the adapter over: `toolreg.KernelDeps.Introspect` is the port, and the composition root fills it with the app layer's implementation |
| `plugin-reach plugins/tools/introspecttool -> kernel/runtime` | **gone** |

The tool's **wire is byte-identical** — the contract's rows carry exactly the
tags the plugin's rows carried — so nothing an operator or an agent reads
changes. The wire is now **pinned** (see below), so it cannot drift either.

## The ratchet went DOWN

**287 packages, 99 import violations, 13 call sites** — from 100/13. The
**stale allowlist entry was deleted** — a leftover entry fails the archcheck
gate (the debt was paid, the list must shrink).

## The port is a contract between the daemon and the tool — and it was unwitnessed

The tool cannot name the kernel, so the daemon must hand it the live state.
That hand-over is a **wiring**, and a wiring nobody witnesses is a wiring that
can be dropped silently: the tool would answer "introspection is not available
on this daemon" on every live box, and every test that builds the tool by hand
would still pass.

So the composition root's dependency assembly was pulled out of `runDaemon`
into a named function (`cmd/agezt/tooldeps.go`, `toolKernelDeps`) and
**measured there**: the daemon must supply the introspect port, the overseer
port, and both must answer the **live** kernel.

## The proof

### Permanent tests

- **The wire shape** (`kernel/contract/introspectapi/introspectapi_test.go`):
  the exact key set of the overview (17 keys), of the reaper report and of the
  fleet snapshot, plus the always-fields that must stay on the wire even when
  they are empty (a missing `halted` would read as "false" and hide a halted
  daemon from the agent). The three agent rows inside the report are pinned
  too.
- **The port's shape** (same file): the port has exactly four reads. A port
  that grows a method grows the plugin's reach.
- **The identity assertions** (`plugins/tools/introspecttool/contract_type_test.go`):
  the plugin's six rows ARE the contract's, and the plugin's `Source` IS the
  contract's port — as function types, so a copy with the same fields would
  not pass.
- **The adapter answers the live kernel** (`kernel/app/introspect/source_test.go`):
  the delegation ceilings are the kernel's own, the journal head is the
  kernel's own, the tool names are the kernel's own (and sorted), and a
  schedule and a standing order the operator added are listed by the port.
- **The projection is measured field for field** (same file): the mapping from
  the reaper's own row to the contract's row is a hand-written copy — a field
  left out of it would be silent, so every field (the stale counters, the
  failure threshold, the last-active stamp, the misconfiguration issues) is
  asserted.
- **The wiring, twice** (`plugins/builtintools/introspect_ports_test.go` and
  `cmd/agezt/tooldeps_test.go`): the plugin binds the port and publishes the
  live kernel's state; the daemon supplies it; and an unwired port leaves the
  tool reporting that it is unwired instead of dereferencing nil.

### Mutation run

**23 mutations: 23 caught.** One mutation **survived the first round** — the
sorted tool list, because the adapter's test registered only **one** tool, and
a reversed list of one name looks exactly like a sorted one. The test now
registers two and checks the membership as well, and the mutation was run
again → caught. Six mutations were **first authored as build errors** (a
compile error is not a catch): they were re-authored so the code still
compiles, and the gap each of them probed was closed by a test.

| # | Mutation | Result |
|---|---|---|
| M01 | the overview's `delegation` key is renamed | caught — by the wire test |
| M02 | the overview's `halted` key is renamed | caught |
| M03 | the report's `stale_bytes` key is renamed | caught |
| M04 | the fleet snapshot's `misconfigured_agents` key is renamed | caught |
| M05 | the degraded agent's `last_failure_ms` key is renamed | caught |
| M06 | the empty-journal clamp is removed | caught — the overview would report a negative head |
| M07 | the journal head is off by one | caught |
| M08 | the tool names are sorted the wrong way | **survived the first round** — the adapter's test registered one tool, so a reversed list looked sorted; two tools and a membership check were added → caught |
| M09 | the tool list is dropped | caught |
| M10 | the delegation ceilings are dropped | **build error** at first (the value became unused) — re-authored as a wrong value → caught |
| M11 | the schedules read returns nothing | caught |
| M12 | the standing-orders read returns nothing | caught |
| M13 | the plugin never binds the port | caught |
| M14 | the daemon binds the introspect port to nothing | **build error** at first (the import became unused) — re-authored → caught |
| M15 | the daemon binds the overseer port to nothing | **build error** at first — re-authored → caught |
| M16 | the plugin's row drifts from the contract's | **build error** at first (the plugin's own code no longer fits) — re-authored against the plugin's behaviour → caught |
| M17 | the plugin publishes an empty overview instead of the source's | caught |
| M18 | an unbound tool does not report that it is unbound | caught |
| M19 | the reaper projection drops the stale bytes | caught |
| M20 | the reaper projection drops the last-active stamp | caught |
| M21 | the reaper projection drops the failure threshold | caught |
| M22 | the reaper projection drops the misconfiguration issues | caught |
| M23 | the fleet snapshot reports a slug that is not the agent's | **build error** at first — re-authored → caught |

### Gates

The build, gofmt, vet, dead-code, archcheck (**287 packages, 99/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## What is deliberately left

- The **guardians** still reach into the roster module: they need
  `roster.Validate`, and the validation is not pure — it reads the trust
  ceiling through `kernel/edict`, and a contract package may not import a
  platform package. That is a slice of its own.
- The remaining plugin-reach edges: the database tool, the workboard tool,
  the skill tool, the forgetool, the market plugin, the built-in tools and
  the built-in channels.
