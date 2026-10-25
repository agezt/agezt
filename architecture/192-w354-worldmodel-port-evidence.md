# W3.54 — the world model is the daemon's

## What the slice is

After W3.53 the kernel still reached **7** modules and the ratchet stood at
**37**. This slice takes **four** of those edges — the first multi-edge slice
since the catalog — and it takes them **completely**:

| Edge | Why it existed |
|---|---|
| `cross-module kernel/runtime -> kernel/modules/knowledge/worldmodel` | the kernel held the module's concrete `*worldmodel.Graph` and opened the file itself |
| `cross-module kernel/runtime/runexec -> kernel/modules/knowledge/worldmodel` | the run-executor declared the module's concrete graph in its `KernelAPI` |
| `cross-module kernel/contextselect -> kernel/modules/knowledge/worldmodel` | the context selector named the module's rows |
| `cross-module kernel/reflect -> kernel/modules/knowledge/worldmodel` | the reflection engine held the module's concrete graph |

The ratchet went **37 → 33**. The kernel still reaches **7** modules.

This slice is the **pilot for the bus-bound modules**: the graph journals every
mutation through the kernel's bus, so the graph cannot be built by the layer
that holds the bus without that layer naming the module. The pattern it
establishes — a **builder injected by the daemon**, a **publisher port** the
kernel hands over — is what the remaining bus-bound modules (the skill forge,
the memory manager) will reuse.

## What the kernel used to do

```go
// kernel/runtime/runtime_kernel.go
world    *worldmodel.Graph
worldDir *worldmodel.FileStore

// kernel/runtime/compose.go
wstore, err := worldmodel.Open(filepath.Join(cfg.BaseDir, "worldmodel"))
if err != nil {
    return fail("runtime: worldmodel", err)
}
closers = append(closers, wstore)
wgraph := worldmodel.NewGraph(wstore, kbus)
```

The kernel opened the graph's file, built the graph over its own bus, and held
the module's **concrete** type — which the reflection engine and the run
executor then had to name as well.

## What it does now

### The contract carries the rows, the rules and the ports

```go
// kernel/contract/worldmodelapi/worldmodel.go
type Kind string   // + nine constants, the open vocabulary
type Verb string   // + seven constants
type Entity struct { ... }        // with its Active() RULE
type Relation struct { ... }      // with its Active() RULE
type ScoredEntity struct { Entity Entity; Score float64 }
type Neighbor struct { Relation Relation; Outgoing bool; Other Entity }
type UpsertSpec struct { ... }
type DecayOptions struct { ... }
```

```go
// kernel/contract/worldmodelapi/store.go — the store port: eight verbs.
type Store interface { PutEntity, GetEntity, AllEntities, PutRelation, GetRelation, AllRelations, Count, Close }

// kernel/contract/worldmodelapi/graph.go — the graph port: fourteen verbs.
type Graph interface { Upsert, EditEntity, Relate, Resolve, ResolveQuiet, IsActiveSubject, Neighbors, Forget, Get, Entities, Relations, Count, Decay, Tool }
```

The module keeps every name as an **alias**, so there is one row and one
spelling of it, and every existing caller is unchanged.

**The RULES travel with the ROWS.** `Entity.Active()` and `Relation.Active()`
decide whether a node still counts — a Go type alias cannot carry methods, so
had the rules stayed in the module, the aliases would have lost them and the
layers above would have had to reach back into the module to ask "does this
entity still count?".

**The content-addressing helpers stay in the module.** `EntityID` and
`RelationID` hash with BLAKE3, and the contract layer may not reach for a
third-party module (principle P4). Nothing above the module needs them — the
module computes the ids itself.

**The publisher is a port, not the bus.**

```go
// kernel/contract/worldmodelapi/graph.go
type Publisher interface {
    Publish(spec eventapi.Spec) (id string, err error)
}
```

The bus answers with the durable **envelope**, and the envelope carries the
hash chain — the contract layer may not name it. The port answers with the
event's **id**, which is exactly what the graph records as the entity's
provenance (`SourceEvent`) and what `agt why` reads.

### The graph is built by a builder the daemon injects

```go
// cmd/agezt/main_toolinvoker.go
if cfg.WorldStore == nil {
    store, err := worldmodel.Open(filepath.Join(cfg.BaseDir, "worldmodel"))
    if err != nil {
        return nil, err
    }
    cfg.WorldStore = store
}
if cfg.WorldGraph == nil {
    cfg.WorldGraph = func(store worldmodelapi.Store, pub worldmodelapi.Publisher) worldmodelapi.Graph {
        return worldmodel.NewGraph(store, pub)
    }
}
```

```go
// kernel/runtime/compose.go
var wgraph worldmodelapi.Graph
if cfg.WorldStore != nil && cfg.WorldGraph != nil {
    wgraph = cfg.WorldGraph(cfg.WorldStore, worldPublisher{b: kbus})
}
```

**The daemon names the module; the kernel does not.** The daemon opens the file
and hands over the *builder*; the kernel calls that builder with the store it
was given and its own publisher, so the graph journals through the kernel's bus
without either side naming the other's implementation.

This is the pattern the remaining bus-bound modules will reuse: the daemon
names the implementation, the kernel names the ports.

### A missing world model: the tool is not registered, the surfaces refuse

- A kernel with **no** graph registers **no world tool** — a tool that calls
  into nothing would answer the agent with a panic.
- The reflection engine with **no** graph decays nothing and reports what it
  could see.
- The app layer **refuses** every surface with *"the world model is not
  available on this daemon"*.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/worldmodelapi`: `TestTheStorePortIsExactlyEightVerbsWide` | the store port is **exactly eight** verbs |
| `kernel/contract/worldmodelapi`: `TestTheGraphPortIsExactlyFourteenVerbsWide` | the graph port is **exactly fourteen** verbs |
| `kernel/contract/worldmodelapi`: `TestThePublisherPortIsOneVerb` | the publisher answers the event's **id** and nothing else |
| `kernel/contract/worldmodelapi`: `TestTheGraphPortNamesOnlyContractRows` | the fourteen verbs keep their exact signatures and name only the contract's rows |
| `kernel/contract/worldmodelapi`: `TestTheActiveRulesTravelWithTheRows` | the active rules are **on the contract's rows** and decide right |
| `kernel/contract/worldmodelapi`: `TestTheNormalizersAreTheContracts` | the open vocabularies keep their permissive rules |
| `kernel/contract/worldmodelapi`: `TestTheSentinelIsTheContracts` | the empty-name sentinel is the contract's |
| `kernel/modules/knowledge/worldmodel`: `TestTheRowsAreTheContractsRows` | the module's rows and constants are the contract's — by **type identity** |
| `kernel/modules/knowledge/worldmodel`: `TestTheActiveRulesAreTheContracts` | the aliased rows **keep** the contract's rules |
| `kernel/modules/knowledge/worldmodel`: `TestTheGraphDoesNotKnowTheBus` | the graph's core **does not import the kernel bus** |
| `kernel/modules/knowledge/worldmodel`: the compile-time assertions | the module's graph and store answer the contract's ports |
| `kernel/runtime`: `TestTheWorldFaceIsTheContractsPort` | the kernel's face is **statically** the contract's port |
| `kernel/runtime`: `TestTheWorldPortStaysNilWithoutALibrary` | a kernel without a world model hands over a **nil** port |
| `kernel/runtime`: `TestTheWorldToolIsRegisteredOnlyWithAGraph` | the world tool is registered **only** with a graph |
| `kernel/runtime`: `TestTheWorldPortAnswersTheLiveLibrary` | the port answers the **live** rows |
| `kernel/app/world`: `TestTheAppLayerRefusesWithoutAGraph` | every surface refuses instead of panicking |
| `kernel/app/world`: `TestTheRefusalNamesItsReason` | the refusal says **why** |
| `cmd/agezt`: `TestOpenAppKernelWiresTheWorldModel` | the daemon end: the port is live and the model is a **directory on disk** |
| `cmd/agezt`: `TestTheWorldModelIsTheInjectedOne` | the daemon uses the **injected** model, not one of its own |

### The mutation run

**9 mutations: 9 caught, 0 survived, 0 skipped, 0 build errors** — in **two**
independent passes, both honest (every catch named the expected test or the
expected edge).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel names the module again | **archcheck** — `cross-module kernel/runtime -> kernel/modules/knowledge/worldmodel` |
| 2 | the context selector names the module again | **archcheck** — `cross-module kernel/contextselect -> kernel/modules/knowledge/worldmodel` |
| 3 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |
| 4 | the contract's graph port grows a fifteenth verb | `TestTheGraphPortIsExactlyFourteenVerbsWide` |
| 5 | the kernel's world face is an anonymous shape, not the contract's port | `TestTheWorldFaceIsTheContractsPort` |
| 6 | the kernel ignores the world model it was handed | `TestTheWorldPortAnswersTheLiveLibrary` |
| 7 | the app layer stops refusing without a graph | `TestTheAppLayerRefusesWithoutAGraph` |
| 8 | the daemon does not hand the world model to the kernel | `TestOpenAppKernelWiresTheWorldModel` |
| 9 | **the active rule is wrong** (a forgotten entity counts again) | `TestTheActiveRulesTravelWithTheRows` |

**Three of the nine are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that opens
the file itself, as long as the world entities still show up in the prompt.

**Mutation 5 needed the run executor's declaration to travel with it.** The
kernel's face is also declared in the run executor's `KernelAPI`; drifting only
the kernel's face is a **compile error**, not a catch. The mutation therefore
drifts both declarations — the code compiles, the interface still matches, and
only a **static** check on the declared return type catches it. (The same
lesson as the catalog slice, met again: a face that the run executor declares
cannot drift alone.)

**Mutation 9 is the one this slice added.** The active rules are a
**behaviour**, not a shape: a rule that quietly forgets the tombstone lets a
forgotten entity back into resolve, and no structural check can see it.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 314 packages placed; 33 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 37** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rules the slice is really about

**A bus-bound module cannot be built by the layer that holds the bus.** The
graph journals every mutation; the bus lives inside the kernel; the kernel may
not name the module. The way out is a **builder the daemon injects** and a
**publisher port the kernel hands over** — the daemon names the
implementation, the kernel names the ports, and neither names the other.

**A port that needs the durable envelope cannot carry it.** The publisher
answers with the event's **id** — the provenance the graph records and `agt
why` reads — and leaves the hashed envelope to the platform.

**A tool that calls into nothing must not be registered.** The world tool is
the graph's agent-facing face; a kernel with no graph registers no world tool,
because a tool that answers the agent with a panic is worse than a missing
tool.

## What remains

The kernel still reaches **7** modules, and the ratchet stands at **33**. The
next candidates:

- **`skill`** — five edges; `skillapi` exists (the `Forge` and `BundleStore`
  ports are already there). The **width** is the work: the forge's surface is
  15+ verbs, and it is journaled through the bus — so it needs the same
  builder-plus-publisher pattern this slice established.
- **`roster`** — four edges and 73 importer files by far the largest fan-in;
  the rows are already in `agentapi`, only the store port and the sentinels are
  missing. The app layer (28 production files) is the bulk of the work.
- **`memory`** — six edges; the manager is bus-bound and LLM-bound (distill,
  embeddings), so its port is the widest of all.
