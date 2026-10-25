# W3.51 — the catalog directory and its syncer are the daemon's

## What the slice is

After W3.50 the kernel still reached **8** modules. This slice takes **four**
of those edges — the most of any slice so far, and the best ratio the
measurement had (four edges for ten call sites):

| Edge | Why it existed |
|---|---|
| `cross-module kernel/runtime -> kernel/modules/providers/catalog` | the kernel held the module's `*catalog.Store` and the `*catalog.Catalog` row |
| `cross-module kernel/runtime/runexec -> kernel/modules/providers/catalog` | the run-executor interface named the module's store and row |
| `cross-module kernel/cadence/systemtasks -> kernel/modules/providers/catalog` | the scheduled sync built the module's syncer itself |
| `upward kernel/governor -> kernel/modules/providers/catalog` | the governor's pricing table held a `*catalog.Catalog` |

The ratchet went **43 → 39**. The kernel still reaches **7** modules.

The catalog's rows were **already** in the contract (`catalogapi.Provider`,
`Model`, `Family`, `Cost`, `Limit`, `Modalities`) and the module already kept
them as aliases — so this slice was mostly about the two things that were
missing: **the aggregate row** and **the ports**.

## What the kernel used to do

```go
// kernel/runtime/runtime_kernel.go
catalogStore *catalog.Store
catalog      *catalog.Catalog
// kernel/runtime/compose.go
catStore := catalog.NewStore(catDir)
cat, err := catStore.Load()
```

The kernel opened the catalog's directory, held the module's concrete store,
and handed the module's concrete row to the governor and to the run executor.

## What it does now

### The contract carries the row AND its rules

```go
// kernel/contract/catalogapi/catalog.go
type Catalog struct {
    Providers map[string]*Provider
    SyncedAt  time.Time
    Sources   []string
}
```

The `Catalog` aggregate and its **nine rules** (`ProviderList`, `FindModel`,
`StrictToolArgsNative`, `ToolCapableAlternative`,
`ToolCapableAlternativeAmong`, `VisionCapableAmong`, `BestModelsAcross`,
`DuplicateCredentialEnvs`, `Merge`) moved to the contract layer, and with them
the four helpers they lean on and `MarshalAPI`.

**The rules had to travel, and that is the point.** A Go type alias cannot
carry methods, so a rule that stayed in the module could not be reached by the
layers above it — and the rules decide **which model a run routes to**: the
down-routing that picks a tool-capable sibling, the vision sidecar's choice,
the council's seats, the pricing table's lookup. Keeping them in the module
would have left the governor reaching into a module for the price of a model.

The module keeps every name as an **alias**, so there is one row and one
spelling of it, and every existing caller is unchanged.

### The two ports

```go
// kernel/contract/catalogapi/store.go
type Store interface {
    Load() (*Catalog, error)
    SaveAPI(raw []byte, sourceURL string) error
    SaveLocal(c *Catalog, source string) error
    UpsertCustomProvider(p *Provider) (added bool, err error)
    LoadMeta() (Meta, error)
}

// kernel/contract/catalogapi/sync.go
type Syncer interface {
    Sync(ctx context.Context, url string, timeout time.Duration) (raw []byte, cat *Catalog, res SyncResult, err error)
}
```

The store's width is **what the layers above actually drive** — five verbs.
The module's own store has more (`SaveCustom`, `SaveMeta`); those stay on the
module's type and are **not** part of the contract, because no layer above the
module drives them.

**The syncer's URL and timeout are PARAMETERS, not fields.** A port cannot
carry mutable state that a caller would have to reach through, and a caller
that sets a URL on a port it does not own is reaching into someone else's
implementation. The two surfaces that sync (the app layer's operation and the
scheduled system task) each pass their own URL — the operator's override, or
the environment's — and the daemon's syncer honours them.

### The directory is the daemon's; the syncer is the daemon's

```go
// cmd/agezt/main_toolinvoker.go
store := catalog.NewStore(filepath.Join(cfg.BaseDir, "catalog"))
cfg.CatalogStore = store
cfg.CatalogSyncer = catalog.NewSyncer()
```

The kernel opens **no** catalog directory any more. The scheduled system task
reaches the syncer **through the kernel** (`k.CatalogSyncer()`), which is what
closed the `systemtasks -> catalog` edge — the module that drives a sync may
not name the module that fetches.

### A missing library: two different answers, on purpose

This slice has to answer a question the earlier ones did not: what does a
daemon with **no** catalog say?

- The **read** surface answers with the **EMPTY** catalog — "no providers" is
  an honest answer, and a nil snapshot would make every reader a potential
  panic. `k.Catalog()` is never nil.
- The **write** surfaces (a sync, a discovery, a custom provider) **refuse**:
  *"the catalog library is not available on this daemon"*. There is nowhere to
  write, so the refusal is the answer.

The app layer learned the same rule, and the discovery refuses **before** the
network call — a daemon with no library should not go and ask Ollama what it
finds when it has nowhere to put the answer.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/catalogapi`: `TestTheStorePortIsExactlyFiveVerbsWide` | the store port is **exactly five** verbs |
| `kernel/contract/catalogapi`: `TestTheStorePortNamesOnlyContractRows` | the five verbs keep their exact signatures and name only the contract's rows |
| `kernel/contract/catalogapi`: `TestTheSyncerPortIsOneVerbAndTakesTheCallersURL` | the syncer port is **one** verb, and the URL and timeout are **parameters** |
| `kernel/contract/catalogapi`: `TestTheDefaultSyncURLIsTheContracts` | the sync source is the contract's |
| `kernel/contract/catalogapi`: `TestTheCatalogRowCarriesItsRules` | the row carries the rules about it |
| `kernel/modules/providers/catalog`: the compile-time assertions | the module's store and syncer answer **both** contract ports, and its rows are the contract's |
| `kernel/runtime`: `TestTheCatalogFaceIsTheContractsPorts` | the kernel's face is **statically** the contract's ports |
| `kernel/runtime`: `TestTheCatalogPortsStayNilWithoutALibrary` | a kernel without a library hands over **nil** ports |
| `kernel/runtime`: `TestTheCatalogSnapshotIsNeverNil` | the read surface answers the **empty** catalog, never nil |
| `kernel/runtime`: `TestTheCatalogRefusesToReloadWithoutALibrary` | the reload refuses instead of panicking |
| `kernel/runtime`: `TestTheCatalogPortsAnswerTheLiveLibrary` | the ports answer the **live** rows |
| `kernel/app/catalog`: `TestTheAppLayerRefusesWithoutALibrary` | both app surfaces refuse instead of panicking |
| `cmd/agezt`: `TestOpenAppKernelWiresTheCatalogLibrary` | the daemon end: the ports are live and the catalog is a **directory on disk** |
| `cmd/agezt`: `TestTheCatalogLibraryIsTheInjectedOne` | the daemon uses the **injected** library, not one of its own |

### The mutation run

**8 mutations: 8 caught, 0 survived, 0 skipped, 0 build errors** — in **two**
independent passes, both honest (every catch named the expected test or the
expected edge).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the kernel names the module again | **archcheck** — `cross-module kernel/runtime -> kernel/modules/providers/catalog` |
| 2 | the governor names the module again | **archcheck** — `upward kernel/governor -> kernel/modules/providers/catalog` |
| 3 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |
| 4 | the contract's store port loses a verb | `TestTheStorePortIsExactlyFiveVerbsWide` |
| 5 | the kernel's store face is an anonymous shape, not the contract's port | `TestTheCatalogFaceIsTheContractsPorts` |
| 6 | the kernel stops answering the live rows | `TestTheCatalogPortsAnswerTheLiveLibrary` |
| 7 | the app layer stops refusing without a library | `TestTheAppLayerRefusesWithoutALibrary` |
| 8 | the daemon does not hand the library to the kernel | `TestOpenAppKernelWiresTheCatalogLibrary` |

**Three of the eight are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that opens
the catalog directory itself, as long as the prices work.

The run earned two honest corrections, and both are worth naming:

- The first attempt at mutation **5** narrowed the kernel's face to a
  one-verb interface — which **broke the callers** and was a **build error**.
  A build error is not a catch: the mutation was wrong, not the test. The
  second attempt drifts the face to an **anonymous** interface with the **same
  five verbs** — that still compiles, and no runtime behaviour would notice.
- Mutation **6** first landed on the kernel's **Open** path, where the empty
  catalog is *correct* — so the test never saw it. The mutation now lands on
  the **reload**, which is where the live rows actually come from.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 311 packages placed; 39 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 43** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rule the slice is really about

**A rule that lives in the module cannot be reached by the layers above it.**
This is the first slice where the logic had to move, not just the shape — and
the rule it exposes is about the places that decide things: the down-routing
that picks a tool-capable sibling, the vision sidecar, the council's seats,
the pricing table's lookup. A "row only" migration would have left the
governor reaching into a module for the price of a model, and the whole point
of the contract layer is that it cannot.

**A port cannot carry mutable state that a caller has to reach through.** The
syncer's URL and timeout became parameters, and that is not a detail: a
caller that sets a field on a port it does not own is reaching into someone
else's implementation.

**"No providers" and "no library" are different answers.** The read surface
answers the empty catalog; the write surfaces refuse. Collapsing them — say,
answering nil everywhere — would have turned one honest answer into a panic
in every reader.

## What remains

The kernel still reaches **7** modules. The next candidates, in the order the
measurement suggests:

- **`skill`** — the kernel holds the forge and its file store; five edges;
  `skillapi` exists (the `Forge` and `BundleStore` ports are already there).
- **`roster`** — the kernel holds the store; four edges and 73 importer files
  by far the largest fan-in; the rows are already in `agentapi`, only the
  store port and the sentinels are missing.
- **`memory`, `worldmodel`, `taste`, `okr`** — each one the same slice (a
  contract port, the daemon opening the file, the kernel's accessor carrying
  the port).
