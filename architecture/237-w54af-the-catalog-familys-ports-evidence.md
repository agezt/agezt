# W5.4af — the catalog family, and the refusal written once

## What the slice is

The **nineteenth** slice of the port programme, and the **tenth** one of its last
phase: the handle itself.

The catalog is the directory the daemon keeps its provider and model catalog in
(W3.51) — the daemon's own file, and the store the sync writes into, the
snapshot the routing reads, the syncer the discovery fetches with, and the
refresh that re-reads the directory and rebuilds the live provider registry.

The adapter reached for the kernel's catalog in **two** places. The **application**
layer reached for it in **twenty-one** places across **four** files — and that is
where the real debt was, because the application layer is where the handle
travels *past* the binding.

## What the slice found

| file | sites | whose catalog |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 1 | the daemon's own |
| `kernel/controlplane/settings_write_ports.go` | 1 | the daemon's own |
| `kernel/app/catalog/catalog.go` | 9 | the routed one |
| `kernel/app/providers/providers.go` | 9 | the routed one |
| `kernel/app/providers/oauth.go` | 2 | the daemon's own |
| `kernel/app/providers/oauth_callback.go` | 1 | the daemon's own |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **24** | **22** |
| distinct kernel verbs the adapter names | **19** | **17** |
| method values handed over | **9** | **9** |
| application-layer sites that named a catalog verb | **21** | **0** |

## What changed

The catalog family's ports moved into the application
(`kernel/app/catalog/ports.go`):

```go
type Ports struct {
	snapshot func() *catalogapi.Catalog
	store    func() catalogapi.Store
	syncer   func() catalogapi.Syncer
	refresh  func() (*catalogapi.Catalog, bool, error)
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		snapshot: k.Catalog,
		store:    k.CatalogStore,
		syncer:   k.CatalogSyncer,
		refresh:  k.Reload,
	}
}
```

and the adapter keeps the binding (`kernel/controlplane/catalog_ports.go`) — which
does not name the runtime, like every binding since W5.4v:

```go
func (s *Server) catalogPorts() appcatalog.Ports { return appcatalog.PortsFromKernel(s.k) }

func requestCatalogPorts(ctx context.Context) appcatalog.Ports {
	return appcatalog.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

### The port's names are not the kernel's

The port answers `Snapshot`, `Store`, `Syncer` and `Refresh` — never `Catalog`,
`CatalogStore`, `CatalogSyncer` or `Reload`. A port that carried the name of the
verb it stands for would let the adapter name the kernel's verb through the
port's door, and the guard would read the port as the handle.

`TestTheCatalogPortNamesAreNotTheKernelVerbs` holds that by reflection over the
port's own method set and the kernel's: the two sets do not meet.

### The applications stopped holding the handle

The catalog application (`kernel/app/catalog`) and the providers application
(`kernel/app/providers`) held the kernel handle for exactly the catalog's verbs —
and nothing else. The catalog application now holds its family's ports and a
**narrow view of the bus**:

```go
type Publisher interface {
	Publish(spec event.Spec) (*event.Event, error)
}
```

The bus belongs to the bus family; the catalog family declares only the part it
uses. That is the consumer's business, not the bus family's — and the host hands
over a bus that answers it (`requestBusPorts(ctx)`), so the catalog application
never names the bus's port package either.

The providers application now holds `appcatalog.Ports` and nothing else: the
catalog directory is the daemon's file, and the providers write into it.

### The refusal is written once

The providers application used to spell its own refusal — *"providers: the catalog
library is not available on this daemon"* — and the catalog application spelled
another one. Two spellings of the same refusal are two answers to the same
question, and the two could drift apart.

The refusal now belongs to the family that answers for the catalog:

```go
var ErrNoLibrary = errors.New("catalog: the catalog library is not available on this daemon")
```

and the families that ask the catalog inherit it.

## The rule

The catalog's verbs are the application's, and the rule is **by whole word** —
and the family answers to **two** names:

> a kernel verb whose name carries "Catalog" **or** "Reload" as a **complete
> word** is the catalog family's business.

The two names are **unioned**, not chosen between: the memory family answers to
"Memory" and "Distill" for the same reason — the checked surface does not say
the family's name in it. `ReloadCatalog` carries both words, and a family that
took only one of them would leave the other unguarded.

The sweep takes in **five** verbs: `Catalog`, `CatalogStore`, `CatalogSyncer`,
`ReloadCatalog` and `Reload`.

The guard now holds **131** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheCatalogPortsReachTheKernelCatalog` | the port reports the daemon's own snapshot and hands over the daemon's own store |
| `TestTheCatalogPortsRefreshThroughTheKernel` | the refresh runs the daemon's refresh, and reports honestly that no provider registry was rebuilt |
| `TestTheCatalogPortsSurviveAHostWithNoKernel` | a host with no kernel holds the empty catalog, no store, no syncer, and the refresh refuses |
| `TestTheCatalogPortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheCatalogServicePublishesItsEventsThroughTheBusItWasGiven` | the family still publishes what happened |
| `TestTheCatalogServiceSurvivesAHostWithNoBus` | a host with no bus publishes nothing and does not panic |
| `TestTheProvidersServiceRefusesWithoutACatalogLibrary` | the providers refuse to connect and to refresh, and they inherit the catalog's refusal |
| `TestTheProviderLoginSurvivesAHostWithNoCatalog` | the login does not panic on a host with no catalog |
| `TestTheAdapterBindsTheCatalogPortsToItsOwnKernel` | the daemon's binding answers for the daemon's kernel, and the request's binding follows the **routed** kernel |
| `TestTheGuardNamesEveryCatalogVerbTheKernelHas` | the guard names every catalog verb the kernel has |
| `TestTheCatalogFamilyClaimsNoOtherFamilysVerbs` | the catalog's list claims no other family's verb and does not claim the whole kernel |

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `99B1013889210EA8B07E13E5EFFB7F05256AB158CA6BC41089AAB0C0015ACF79`).

M12 and M13 put the kernel back — the routing's model check and the settings
writer's refresh each reach for the kernel's catalog again — and the **verb
guard** catches every one of them. None of them is a behavioural regression: the
wiring would work **exactly as well**, it would simply be in the wrong layer.

M01 through M05 are the ones that bite: the port holds **no** snapshot, the
snapshot answers with the **empty** catalog for every caller, the port holds
**no** store, the refresh **swallows** the refusal, and the service publishes
**nothing**. M06 removes the providers' refusal, M07 empties the daemon's binding,
M08 makes the request's binding follow the **daemon's** kernel — a tenant's
operations would run against the daemon's catalog — and M09 through M11 take the
family out of the guard, drop the "Reload" half of the rule, and hand the family
another family's verb.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — **2 allowlisted import violations and 0 allowlisted call sites remain** (ratchet) |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `go run ./tools/docsindex` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## What the slice does not do

The handle stays, and the adapter still calls **22** kernel verbs on it and hands
over **9** method values: the run decorators, the approvals, the market, the
plugins, the state, the world, the taste, the artifacts and the rest.

The **application** layer still holds the handle in a handful of places, and this
slice names them rather than pretending they are gone:

- `kernel/app/runs` — the run engine and the run context's kernel,
- `kernel/app/system` — the system service,
- `kernel/app/memory` — the distiller,
- `kernel/app/tools` — the tool registry,
- `kernel/app/execprofile` and `kernel/app/introspect` — two narrow adapters that
  take the kernel as an argument.

Each of them is a family's own service, and each will have to answer the handle
rule before it can answer the verb rule — the way the memory family's
distillation service had to in W5.4aa.
