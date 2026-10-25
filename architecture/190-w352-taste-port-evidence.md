# W3.52 — the curated exemplar library is the daemon's

## What the slice is

After W3.51 the kernel still reached **7** modules and the ratchet stood at
**39**. This slice takes **one** of those edges — the smallest slice so far —
and it takes it **completely**:

| Edge | Why it existed |
|---|---|
| `cross-module kernel/runtime -> kernel/modules/knowledge/taste` | the kernel held the module's concrete `*taste.Store` and opened the module's file itself |

The ratchet went **39 → 38**. The kernel still reaches **7** modules.

The slice is small on purpose. The measurement had six candidates left
(`skill`, `roster`, `memory`, `worldmodel`, `taste`, `okr`), and the **first
thing measured was the width of each port**, not the number of edges:

| Candidate | Edges | What the port would have to carry |
|---|---|---|
| `skill` | 5 | the **Forge** — 15+ verbs, incl. `Propose`, `ShadowEvaluate`, `RecordOutcome`, `ActivateFor`, `SetAutoQuarantine`, `Bundles()` |
| `roster` | 4 | the roster store across **73 importer files** |
| `memory` | 6 | the memory manager **plus** its bus-bound journal |
| `worldmodel` | 4 | the graph engine |
| `okr` | 1 | the okr store, 30 call sites |
| **`taste`** | **1** | **the exemplar store — five verbs** |

`taste` is the same shape as the slices that came before it (`standing`, the
catalog), at a tenth of the width. It was the right thing to finish first: the
pattern is what the bigger slices will reuse, and a pattern proven on a
one-edge slice is cheaper than a pattern proven on a fifteen-verb port.

## What the kernel used to do

```go
// kernel/runtime/runtime_kernel.go
taste *taste.Store

// kernel/runtime/compose.go
tastestore, err := taste.OpenStore(filepath.Join(cfg.BaseDir, "taste"))
if err != nil {
    return fail("runtime: taste", err)
}
```

The kernel opened the library's file and held the module's **concrete** store.
The app layer held the same concrete type, and the control plane reached it
through the kernel's accessor.

## What it does now

### The contract carries the rows and the port

```go
// kernel/contract/tasteapi/tasteapi.go
type Exemplar struct { ID, Title, Body, Scope string; Tags []string; CreatedMS, UpdatedMS int64 }
type CreateSpec struct { Title, Body, Scope string; Tags []string }
type Filter struct { Scope, Tag string; Limit int }

type Store interface {
    Create(spec CreateSpec, nowMS int64) (Exemplar, error)
    Get(id string) (Exemplar, bool)
    List(f Filter) []Exemplar
    ForScope(scope string, limit int) []Exemplar
    Delete(id string) error
}
```

The module keeps every name as an **alias**, so there is one row and one
spelling of it, and every existing caller is unchanged:

```go
// kernel/modules/knowledge/taste/taste.go
type Exemplar = tasteapi.Exemplar
type CreateSpec = tasteapi.CreateSpec
type Filter = tasteapi.Filter
var ErrNotFound = tasteapi.ErrNotFound
var _ tasteapi.Store = (*Store)(nil)
```

**The port carries no clock.** `Create` takes `nowMS int64`, not a
`time.Time`: the contract layer may not name a clock (archcheck's principle
P4), and a port that reads one cannot be tested deterministically. The module's
own `Create` changed shape with it — the caller supplies the time, the store
records it.

**The port is exactly the module's whole public surface.** Five verbs, no
more: a wider port would let a caller reach past the library's own rules (the
store validates, assigns ids and timestamps, and keeps its own on-disk shape);
a narrower one would force a second shape to be kept in step.

### The library is the daemon's

```go
// cmd/agezt/main_toolinvoker.go
if cfg.TasteStore == nil {
    store, err := taste.OpenStore(filepath.Join(cfg.BaseDir, "taste"))
    if err != nil {
        return nil, err
    }
    cfg.TasteStore = store
}
```

The kernel opens **no** library any more; it holds `cfg.TasteStore`, the
contract's port.

The composition root is the **daemon's** `openAppKernel` — the same place that
opens the workboard and the tool-forge stores — so the library lives under the
**base dir**. A tenant's kernel has its own base dir, and therefore **its own
exemplars**: a tenant cannot curate the primary's taste anchors. (The config
copy in the tenant closure carries the primary's port; the composition root
drops it and opens the tenant's own, so the copy cannot leak.)

### A missing library: two different answers, on purpose

- The **read** surface answers the **empty** list — "no exemplars" is an
  honest answer, and a nil list would make every reader a potential panic.
- The **write** surfaces (create, delete) **refuse**: *"the exemplar library is
  not available on this daemon"*. There is nowhere to write, so the refusal is
  the answer.

The kernel's own rule is the same: a kernel with no library injects **no**
exemplars into a run (`k.taste != nil` was already the guard) rather than
calling into nothing.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/tasteapi`: `TestTheStorePortIsExactlyFiveVerbsWide` | the store port is **exactly five** verbs |
| `kernel/contract/tasteapi`: `TestTheStorePortNamesOnlyContractRows` | the five verbs keep their exact signatures and name only the contract's rows |
| `kernel/contract/tasteapi`: `TestThePortCarriesNoClock` | no verb takes a `time.Time` — the clock is the caller's |
| `kernel/contract/tasteapi`: `TestTheNotFoundSentinelIsTheContracts` | the sentinel the layers compare against is the contract's |
| `kernel/contract/tasteapi`: `TestTheRowsAreTheContractsOwn` | the rows are plain data |
| `kernel/modules/knowledge/taste`: `TestTheRowsAreTheContractsRows` | the module's rows are the contract's rows — by **type identity** |
| `kernel/modules/knowledge/taste`: `TestTheNotFoundSentinelIsTheContracts` | the module's sentinel **is** the contract's |
| `kernel/modules/knowledge/taste`: the compile-time assertion | the module's store answers the contract's port |
| `kernel/runtime`: `TestTheTasteFaceIsTheContractsPort` | the kernel's face is **statically** the contract's port |
| `kernel/runtime`: `TestTheTastePortStaysNilWithoutALibrary` | a kernel without a library hands over a **nil** port |
| `kernel/runtime`: `TestTheTastePortAnswersTheLiveLibrary` | the port answers the **live** rows |
| `kernel/app/taste`: `TestTheAppLayerRefusesWithoutALibrary` | both write surfaces refuse instead of panicking |
| `kernel/app/taste`: `TestTheReadSurfaceAnswersEmptyWithoutALibrary` | the read surface answers the empty list, present and not nil |
| `kernel/app/taste`: `TestTheRefusalNamesItsReason` | the refusal says **why** |
| `cmd/agezt`: `TestOpenAppKernelWiresTheExemplarLibrary` | the daemon end: the port is live and the library is a **directory on disk** |
| `cmd/agezt`: `TestTheExemplarLibraryIsTheInjectedOne` | the daemon uses the **injected** library, not one of its own |

### The mutation run

**8 mutations: 8 caught, 0 survived, 0 skipped, 0 build errors** — in **two**
independent passes, both honest (every catch named the expected test or the
expected edge).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel names the module again | **archcheck** — `cross-module kernel/runtime -> kernel/modules/knowledge/taste` |
| 2 | the control plane names the module again | **archcheck** — `adapter-bypass kernel/controlplane -> kernel/modules/knowledge/taste` |
| 3 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |
| 4 | the contract's store port grows a sixth verb | `TestTheStorePortIsExactlyFiveVerbsWide` |
| 5 | the kernel's taste face is an anonymous shape, not the contract's port | `TestTheTasteFaceIsTheContractsPort` |
| 6 | the kernel ignores the library it was handed | `TestTheTastePortAnswersTheLiveLibrary` |
| 7 | the app layer stops refusing without a library | `TestTheAppLayerRefusesWithoutALibrary` |
| 8 | the daemon does not hand the library to the kernel | `TestOpenAppKernelWiresTheExemplarLibrary` |

**Three of the eight are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that opens
the library itself, as long as the exemplars still show up in the prompt.

Mutation **4** is worth naming, because the obvious version of it does not
work: **removing** a verb from the port breaks the app layer that calls it,
which is a **build error**, not a catch. The mutation therefore **grows** the
port by a sixth verb and implements it in the module too — the code still
compiles, and only the static width test can tell the two apart.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 312 packages placed; 38 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 39** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rule the slice is really about

**A port that carries a clock cannot be tested deterministically.** The
exemplar's `Create` took a `time.Time` and now takes `nowMS int64`; the
caller owns the clock, and the contract layer names none. This is the same
rule the catalog's syncer taught — the URL and the timeout became parameters —
wearing a different shape: **a port may not own what a caller must supply.**

**The library's file is the daemon's business, and so is its scope.** The
library lives under the **base dir**, so the primary and each tenant curate
their **own** exemplars. A port copied into a tenant's configuration without
its own file would have let a tenant curate the primary's taste anchors — a
leak that no test on the primary would ever see.

**"No exemplars" and "no library" are different answers.** The read surface
answers the empty list; the write surfaces refuse. Collapsing them — answering
nil everywhere — would have turned one honest answer into a panic in every
reader.

## What remains

The kernel still reaches **7** modules, and the ratchet stands at **38**. The
next candidates, in the order the measurement suggests:

- **`skill`** — the kernel holds the forge and its file store; five edges;
  `skillapi` exists (the `Forge` and `BundleStore` ports are already there).
  The **width** is the work: the forge's surface is 15+ verbs, and it is
  journaled through the bus, so the port and the publisher have to travel
  together.
- **`roster`** — the kernel holds the store; four edges and 73 importer files
  by far the largest fan-in; the rows are already in `agentapi`, only the
  store port and the sentinels are missing.
- **`memory`, `worldmodel`, `okr`** — each one the same slice this one was (a
  contract port, the daemon opening the file, the kernel's accessor carrying
  the port).
