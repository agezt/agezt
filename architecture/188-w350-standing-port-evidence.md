# W3.50 — the standing-order library is the daemon's file

## What the slice is

After W3.49 the kernel still reached **9** modules. This slice takes
**three** of those edges — the same family again:

| Edge | Why it existed |
|---|---|
| `cross-module kernel/runtime -> kernel/standing` | the kernel held the concrete `*standing.Store` and **opened the file** itself |
| `cross-module kernel/runtime/runexec -> kernel/standing` | the run-executor interface named the module's store |
| `adapter-bypass kernel/controlplane -> kernel/standing` | the control plane named the module's rows |

The ratchet went **46 → 43**.

Standing was the **widest** edge left by the measurement and the **cheapest**
one to close: only **eight** call sites in the runtime, but **three** edges.
The rows were already in the contract (`standingapi.Order`, `Trigger`,
`Initiative`, `Validate`, the `TriggerCron`/`TriggerEvent` and
`Initiative*` vocabularies); what was missing was a **port**.

## What the kernel used to do

```go
// kernel/runtime/runtime_kernel.go
standing *standing.Store
// kernel/runtime/compose.go
ststore, err := standing.Open(filepath.Join(cfg.BaseDir, "standing"))
// kernel/runtime/accessors_standing.go
func (k *Kernel) Standing() *standing.Store { return k.standing }
```

The kernel opened the library's file, held the module's concrete store, and
handed the module's concrete type to the run engine and to the control plane.

## What it does now

### The contract carries the port

```go
// kernel/contract/standingapi/store.go
var ErrNotFound = errors.New("standing: order not found")

type Store interface {
    Add(o Order) (Order, error)
    SetEnabled(id string, enabled bool) (Order, error)
    Update(id string, mutate func(*Order)) (Order, error)
    Remove(id string) (bool, error)
    Get(id string) (Order, bool)
    List() []Order
    Count() int
}
```

**Seven** verbs — the module's whole public surface, and no more. A wider port
would let a caller reach past the library's own rules (the store validates,
assigns ids and protects the identity fields the mutator must not touch); a
narrower one would force a second shape to be kept in step with this one.

**`ErrNotFound` moved to the contract.** The kernel compares against it (the
edit path turns "not found" into "the order does not exist"), and the kernel
may not name a module. The module keeps the name as an **alias**, so every
existing caller is unchanged — and `errors.Is(err, standing.ErrNotFound)` in
the kernel still holds, because the two are the same error value.

### The library's file is the daemon's; the residents are the daemon's too

```go
// cmd/agezt/main.go
standingStore, err := standing.Open(filepath.Join(baseDir, "standing"))
if err != nil { ... }
cfg.Standing = standingStore
// cmd/agezt/main_standing.go
evOK := standing.StartRunner(ctx, k.Bus(), store, standing.RunnerConfig{}, fire)
cronOK := standing.StartCron(ctx, store, nil, fire)
```

A library that cannot be opened is a **startup error** — the standing library
is part of the daemon, not a convenience. The two residents (the **event
runner** and the **cron ticker**) are built by the composition root over **the
daemon's own store**, so the kernel's port and the residents read the same
rows. A second instance over the same file would have been a silent
split-brain; the slice's test (`TestTheStandingLibraryIsTheInjectedOne`) is
what makes that impossible to reintroduce quietly.

### A missing library is an answer, not a panic — in FOUR layers

Before the slice the kernel opened the library itself, so `k.Standing()` was
**never nil** and nothing had to say what happens without one. Now a host may
open a kernel without a library, so four layers had to learn the rule:

- the **kernel**: the port is nil, and every one of its five standing
  operations refuses with *"the standing library is not available on this
  daemon"*;
- the **app layer** (`kernel/app/standing`): the read (list), the creation
  (add), the editing (edit) and the lifecycle (enable, remove) all refuse;
- the **guardians' host**: the standing half of the seeder's port answers the
  same way;
- the **control plane**'s two roster surfaces (status view, teardown) read the
  port defensively: a host without a library sees **no** standing orders rather
  than calling into nothing.

The kernel's own accessor is the port — `Standing() standingapi.Store` — and
its **width is checked statically** (see the proof), because a widened or
narrowed face would still compile and still pass every behaviour test.

### The rows the layers above name are the contract's

`kernel/app/standing`, `kernel/app/roster` and `kernel/controlplane` name
`standingapi.Order` now, not `standing.Order`. The two are the **same type**
(the module keeps its name as an alias), so this is not a copy — it is one
row with one spelling, and the app layer no longer reaches a module to say so.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/standingapi`: `TestTheStandingStorePortHasExactlySevenMethods` | the store port is **exactly seven** verbs wide |
| `kernel/contract/standingapi`: `TestTheStandingStoreNamesOnlyContractRows` | the seven verbs keep their exact signatures and name only the contract's rows |
| `kernel/contract/standingapi`: `TestTheNotFoundErrorNamesItsSubject` | the error says **what** was not found |
| `kernel/contract/standingapi`: `TestTheNotFoundErrorIsComparable` | the error survives wrapping (`errors.Is`) |
| `kernel/standing`: `TestTheStoreIsTheContractsPort` | the module's store answers the contract's port |
| `kernel/standing`: `TestTheRowsAreTheContractRows` | the module's rows and vocabularies **are** the contract's |
| `kernel/standing`: `TestTheNotFoundErrorIsTheContractsError` | the module's error **is** the contract's error |
| `kernel/runtime`: `TestTheStandingLibraryStaysANilPortWithoutOne` | a kernel without a library hands over a **nil** port |
| `kernel/runtime`: `TestTheStandingFaceIsTheStorePort` | the kernel's face is **statically** the store port |
| `kernel/runtime`: `TestTheStandingPortAnswersTheLiveLibrary` | the port answers the **live** rows, not a copy |
| `kernel/runtime`: `TestTheKernelRefusesWithoutALibrary` | all **five** kernel standing operations refuse instead of panicking |
| `kernel/runtime`: `TestTheKernelEditOfAnUnknownOrderIsNotFound` | an unknown id is "not found", not a failure |
| `kernel/runtime`: `TestTheKernelForwardsTheLibrarysError` | the port forwards the contract's error |
| `kernel/app/standing`: `TestTheAppLayerRefusesWithoutALibrary` | all **five** app surfaces refuse instead of panicking |
| `cmd/agezt`: `TestOpenAppKernelWiresTheStandingLibrary` | the daemon end: the port is live and the library's file is **on disk** |
| `cmd/agezt`: `TestTheStandingLibraryIsTheInjectedOne` | the daemon uses the **injected** library, not one of its own |

### The mutation run

**8 mutations: 8 caught, 0 survived, 0 skipped, 0 build errors** — in **two**
independent passes, both honest (every catch named the expected test or the
expected edge).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the runtime names the module again | **archcheck** — `cross-module kernel/runtime -> kernel/standing` |
| 2 | the control plane names the module's rows again | **archcheck** — `adapter-bypass kernel/controlplane -> kernel/standing` |
| 3 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |
| 4 | the contract's store port loses a verb | `TestTheStandingStorePortHasExactlySevenMethods` |
| 5 | the port stops answering the live rows | `TestTheStandingPortAnswersTheLiveLibrary` |
| 6 | the kernel's face is an anonymous shape, not the contract's port | `TestTheStandingFaceIsTheStorePort` |
| 7 | the app layer stops refusing without a library | `TestTheAppLayerRefusesWithoutALibrary` |
| 8 | the daemon does not hand the library to the kernel | `TestOpenAppKernelWiresTheStandingLibrary` |

**Three of the eight are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that opens
the library itself, as long as the orders work.

The run earned two honest corrections, and both are worth naming:

- The first attempt at mutation **5** replaced a whole function signature with
  a bare expression and left the old body dangling — a **build error**, and a
  build error is not a catch: the mutation was wrong, not the test. The second
  attempt **survived** for a different reason: the harness replaces the
  **first** matching line, and `for _, o := range s.orders` sits in **three**
  functions (`SetEnabled`, `Update` and `List`) — so the mutation landed on
  `SetEnabled` and the test never saw it. The anchor was narrowed to the sort
  inside `List()`, which is unique.
- Mutation **6** is the one worth naming: it replaces the kernel's face with
  an **anonymous** interface that has the same seven verbs. That **still
  compiles** — the run engine's own declaration moves with it, so nothing
  breaks — and no runtime behaviour would notice. Only a **static** check on
  the declared return type catches it. A port's width is a contract, and a
  contract that is only inferred from behaviour is a contract that can drift.

The harness itself earned a correction too: a case whose anchor does not hold
used to report "no failure" and look like a survivor. Every case now returns
the **real** result of its mutation, so a missed anchor is a **SKIP** and can
never be mistaken for a pass.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 311 packages placed; 43 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 46** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rule the slice is really about

**The port is the module's whole public surface, and it is measured in verbs.**
Seven — not "whatever the store happens to have today", but the width the
host actually drives, pinned by a test so a new verb has to be earned.

**An error the layers above compare against belongs to the contract.**
`ErrNotFound` is the kind of rule that looks like a detail and is not: the
kernel's edit path answers "the order does not exist" by comparing against it,
and a copy of the error in the module would have made every such comparison
silently false.

**A library's file belongs to the daemon, and so do the residents.** The daemon
opens the file, hands the kernel the port and builds the residents over the
same store — so the kernel's port and the residents read the same rows.

**Every test that expected a store became a host.** The kernel opened the
library before; now every test that needs one opens it, injects the port and
— where the residents are involved — builds the residents over that very
store. That is the same shift W3.43/W3.47/W3.48/W3.49 made for the artifacts,
the board, the workflow library and the schedule library: a test cannot be a
host and a user at the same time.

## What remains

The kernel still reaches **8** modules. The next candidates, in the order the
measurement suggests:

- **`skill`** — the kernel holds the forge and its file store; three edges plus
  the market's; `skillapi` exists.
- **`catalog`** — the kernel holds the store and the snapshot; two edges;
  `catalogapi` exists.
- **`memory`, `worldmodel`, `roster`, `okr`, `taste`** — the kernel holds each
  module's concrete store, and each one is the same slice (a contract port,
  the daemon opening the file, the kernel's accessor carrying the port).
