# W3.39a — the registry's two module dependencies become contract's ports

**Date:** 2026-10-24
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the tool registry's **dependency rows** — the `KernelDeps` and
`LateDeps` the daemon hands to the tools in the post-Open and late phases.

## Two edges, both of them a row naming a module

```
cross-module kernel/toolreg -> kernel/modules/artifacts/artifact
cross-module kernel/toolreg -> kernel/modules/board/board
```

The registry is a module (L3, "runs"), and the artifact index and the board
store are modules of their own. The registry did not need their
**implementations** — it needed to *carry* them to the tools, in two rows of
its dependency struct:

```go
type KernelDeps struct {
	Artifacts *artifact.Index   // ← a module's concrete type
	…
}
type LateDeps struct {
	Board       *board.Store     // ← a module's concrete type
	BoardNotify func(m board.Message, corr string)
}
```

A row is not an implementation. The registry holds them, hands them over and
never calls a method on them — the **tools** do the calling. So the rows were
carrying a module's type into every consumer of the row, including the
plugin that names the registry.

The tools themselves were never the problem: each one already held a **narrow
private interface** (`artifacts.Index`, `fetch.Indexer`,
`browser.actionArtifactIndexer`, `codeexec.artifactIndexer`) and received the
concrete index by assignment. The leak was entirely in the row that carried
it.

## What moved

- **New** `kernel/contract/artifactapi` (L1, pure): `Index` — the artifact
  index's port, with the **four verbs** the four tools need (`PutEntry`,
  `List`, `Bytes`, `Delete`) and the contract's own rows
  (`remoteapi.Artifact`, `remoteapi.Filter`).
- **`boardapi.Store`** already existed — the board module's store answers it
  (the board tool has been taking the store through that port since it was
  written). The registry now names the **port** rather than the module's
  store.
- `kernel/toolreg` names the two ports, so its imports of
  `kernel/modules/artifacts/artifact` and `kernel/modules/board/board` are
  **gone**.

The tools keep their own narrow interfaces: an interface value satisfies a
narrower interface when its method set covers it, so the daemon hands over the
one port and each tool still receives exactly the slice it declares.

## The ratchet went DOWN by TWO

**307 packages, 78 import violations, 13 call sites** — from 80/13. Both
`cross-module` edges are gone and **both stale allowlist entries were
deleted**.

## The finding: a nil pointer is not a nil port

The rows were concrete pointers, so `d.Board != nil` meant "there is a board".
They are interfaces now, and a nil pointer handed to an interface parameter is
a **non-nil interface** — the trap the codebase already knew about for the
data lake (`lakePort`) and the forge store.

This is not a theoretical hazard here: **`lateBoard` really is nil on a live
path** — the daemon opens the board late and leaves the store nil when it
could not be opened (`cmd/agezt/main.go`). With a naive change, the board tool
would have bound a store that was not there and **panicked on the first post**
instead of reporting that the board is unavailable.

So the daemon grew the same nil-port rule the other ports follow:

- `artifactPort(*artifact.Index) artifactapi.Index`
- `boardPort(*board.Store) boardapi.Store`

and the late-phase wiring uses them. The rule is now stated and measured at
**both** ends: the daemon hands over a nil port, and the tool reports that
the board/index is unavailable rather than calling into nothing.

## The proof

### Permanent tests

- **The contract** (`kernel/contract/artifactapi/artifactapi_test.go`): the
  port has **exactly four** methods; every type it names is a contract row or
  a builtin, and each signature is pinned argument by argument and answer by
  answer (a module's type in a port's signature would drag that module into
  every plugin that holds the port — the leak the contract layer exists to
  stop).
- **The plugin** (`plugins/builtintools/board_artifact_ports_test.go`): the
  module's index and store **answer the ports** (compile-time assertions), the
  live artifact is listed through the port, the live store receives the
  message, and — the new rule — an **unwired** port leaves the tool
  reporting "unavailable" instead of binding nothing.
- **The daemon** (`cmd/agezt/tooldeps_test.go`): the composition root supplies
  the artifact port and it answers the **live** index; the board port answers
  the **live** store; and both stay **nil** ports when there is nothing to
  hand over.

### Mutation run

**7 mutations: 7 caught.** No mutation survived, none produced a compile
error, and none was skipped on a stale anchor.

| # | Mutation | Result |
|---|---|---|
| M01 | the deps name the artifact module again | caught — **by the archcheck gate** |
| M02 | the deps name the board module again | caught — **by the archcheck gate** |
| M03 | the daemon's board port stops being nil-safe | caught |
| M04 | the daemon's artifact port stops being nil-safe | caught |
| M05 | the contract's index port loses a verb | caught |
| M06 | the artifacts spec does not bind the index | caught |
| M07 | the board spec does not bind the store | caught |

M01 and M02 are measured by the **gate**: the module's index and store are
concrete, so a registry that named them again would compile and pass every
test in the package — the gate is the only thing that can see it, the same
truth the W3.32, W3.33, W3.34, W3.35, W3.37 and W3.38 slices recorded.

M03 and M04 are the mutations that matter most: they are the difference
between "the board is unavailable" and "the daemon panics on the first post".

### Gates

The build, gofmt, vet, dead-code, archcheck (**307 packages, 78/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so is
the full test suite.

## What is deliberately left

- The **other rows still name modules**: `KernelDeps.K` (the live kernel),
  `KernelDeps.Bus`, `KernelDeps.Journal` and `BuildDeps.Warden`. They are the
  next sub-slices — the kernel row is the heavy one (eleven specs, nineteen
  call sites).
- The **registry itself** stays a module: it drives the lifecycle phases, and
  the daemon drives the registry. What the plugin needs from it — the spec
  row and the dependency rows — is what moves, one row family at a time, until
  the plugin's last edge (`plugins/builtintools -> kernel/toolreg`) falls.
