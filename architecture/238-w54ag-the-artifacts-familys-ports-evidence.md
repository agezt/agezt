# W5.4ag — the artifacts family, and the plural the splitter cannot see

## What the slice is

The **twentieth** slice of the port programme, and the **eleventh** one of its
last phase: the handle itself.

The artifact store is the content-addressed store the loop offloads oversized
tool outputs into, and the artifact index is the browsable, deletable metadata
over it. The adapter reached for the kernel's artifact surface in **two**
places, in **one** file.

## What the slice found

| file | sites | whose artifacts |
|---|---|---|
| `kernel/controlplane/artifact.go` | 2 | the daemon's own |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **22** | **20** |
| distinct kernel verbs the adapter names | **17** | **15** |
| method values handed over | **9** | **9** |

The **application** layer held no artifact handle at all: `kernel/app/artifacts`
already took its store and its index as arguments. So this slice is the clean
shape of the programme — one family, one file, two sites.

## What changed

The artifacts family's ports moved into the application
(`kernel/app/artifacts/ports.go`):

```go
type Ports struct {
	store func() BlobStore
	index func() Index
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		store: func() BlobStore { return k.Artifacts() },
		index: func() Index { return k.ArtifactIndex() },
	}
}
```

and the adapter keeps the binding (`kernel/controlplane/artifacts_ports.go`) —
which does not name the runtime, like every binding since W5.4v:

```go
func (s *Server) artifactPorts() appartifacts.Ports { return appartifacts.PortsFromKernel(s.k) }
```

### The nil checks left the adapter

The adapter used to spell the absence of a store and the absence of an index
**twice**, in the same file:

```go
var store appartifacts.BlobStore
var index appartifacts.Index
if actual := s.k.Artifacts(); actual != nil {
	store = actual
}
if actual := s.k.ArtifactIndex(); actual != nil {
	index = actual
}
```

The port answers for that case now — it hands over nothing when the daemon holds
nothing — and the adapter's service is built from the port:

```go
ports := s.artifactPorts()
return appartifacts.New(ports.Store(), ports.Index(), time.Now)
```

### The port's names are not the kernel's

The port answers `Store` and `Index` — never `Artifacts`, `ArtifactIndex` or
`ArtifactStore`. A port that carried the name of the verb it stands for would
let the adapter name the kernel's verb through the port's door.
`TestTheArtifactPortNamesAreNotTheKernelVerbs` holds that by reflection over the
port's own method set and the kernel's.

## The rule

The artifacts' verbs are the application's, and the rule is **by whole word** —
and the family answers to **two** names:

> a kernel verb whose name carries "Artifact" **or** "Artifacts" as a
> **complete word** is the artifacts family's business.

The two names are not a whim: the camel-case splitter reads `ArtifactIndex` as
**"Artifact" + "Index"**, while `Artifacts` is **ONE** word to it — a plural
`s` does not start a new word. A family that looked for a single name would
guard either the store and the index, **or** the plural, and never both — and
the one it left out would be a verb the adapter may name.

The sweep takes in **three** verbs: `Artifacts`, `ArtifactIndex` and
`ArtifactStore`.

The guard now holds **134** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheArtifactPortsReachTheKernelStore` | the port hands over the daemon's own store and the daemon's own index |
| `TestTheArtifactPortsSurviveAHostWithNoKernel` | a host with no kernel hands over neither |
| `TestTheArtifactPortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsTheArtifactPortsToItsOwnKernel` | the daemon's binding answers for the daemon's own kernel |
| `TestTheAdapterBindsTheArtifactPortsToAHostWithNoStore` | a daemon opened without a store hands over none |
| `TestTheGuardNamesEveryArtifactVerbTheKernelHas` | the guard names every artifact verb the kernel has |
| `TestTheArtifactFamilyClaimsNoOtherFamilysVerbs` | the artifact's list claims no other family's verb and does not claim the whole kernel |

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `9929404FC9700496982DCCC35A5FE495DFB97E77B5B926E43F86D845A9F531A5`).

M06 puts the kernel back — the adapter's artifact service reaches for the
kernel's store and index again — and the **verb guard** catches it. It is not a
behavioural regression: the wiring would work **exactly as well**, it would
simply be in the wrong layer.

M01 through M04 are the ones that bite: the port holds **no** store, the store
answers with **nothing** for every caller, the port holds **no** index, the
index answers with **nothing**. M05 empties the daemon's binding, M10 gives the
port a method that carries a kernel verb's own name, M11 makes the binding name
the runtime, M12 builds the service from **nothing** — and the artifact list
over the socket answers *"artifact index unavailable"* — M13 drops the singular
half of the rule, M08 drops the plural half, M07 takes the family out of the
guard, and M09 hands the family **another** family's verbs.

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

The handle stays, and the adapter still calls **20** kernel verbs on it and hands
over **9** method values: the run decorators, the approvals, the market, the
plugins, the state, the world, the taste, the data lake, the seats, the reflect,
the reaper, the update drain and the rest.

The **application** layer still holds the handle in a handful of places, and this
slice names them rather than pretending they are gone:

- `kernel/app/runs` — the run engine and the run context's kernel,
- `kernel/app/system` — the system service,
- `kernel/app/memory` — the distiller,
- `kernel/app/tools` — the tool registry,
- `kernel/app/execprofile` and `kernel/app/introspect` — two narrow adapters that
  take the kernel as an argument.
