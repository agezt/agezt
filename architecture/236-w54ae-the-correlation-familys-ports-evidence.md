# W5.4ae — the correlation family, and the binding a value cannot hold

## What the slice is

The **eighteenth** slice of the port programme, and the **ninth** one of its
last phase: the handle itself.

The correlation is the identity one operation carries — the journal's rows, the
audit's rows and the wake's lineage all name it — and the daemon mints a fresh
one for every mutating request. The adapter reached for the kernel's
correlation in **nine** places across **four** files.

## What the slice found

| file | sites | whose correlation |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 6 | the daemon's own |
| `kernel/controlplane/dispatch_audit.go` | 1 | routed |
| `kernel/controlplane/schedule_helpers.go` | 1 | the daemon's own |
| `kernel/controlplane/app_operations.go` (the router) | 1 | routed |

Six of the nine were **method values** — the kernel's mint handed over as a
value (`s.k.NewCorrelation`) rather than called — and three were calls.

| | before | after |
|---|---|---|
| call sites on the handle | **28** | **24** |
| distinct kernel verbs the adapter names | **21** | **19** |
| method values handed over | **15** | **9** |

The correlation was the **largest** cluster the adapter had left: nine
mentions of one verb, and six of them invisible to the call measurement.

## What changed

The correlation family's ports moved into the application
(`kernel/app/correlation/ports.go`):

```go
type Ports struct {
	mint  func() string
	actor func(ctx context.Context, actor, corr string) context.Context
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		mint:  k.NewCorrelation,
		actor: k.WithActorCorrelation,
	}
}
```

and the adapter keeps the binding (`kernel/controlplane/correlation_ports.go`)
— which does not name the runtime, like every binding since W5.4v:

```go
func (s *Server) correlationPorts() appcorrelation.Ports { return appcorrelation.PortsFromKernel(s.k) }
```

There is **no requested binding**, and the reason is the shape of the request
path: the router mints the request's correlation **before** the request's
kernel is in the context, and the protocol audit runs on a context that never
carries one. Both ask the family directly, with the kernel they hold — the
same shape the council family has always had.

### The port holds capabilities, not the kernel

The kernel's correlation surface is two verbs, and the port holds them as
**capabilities** — the mint and the actor decoration. A family that asks for a
correlation never names the kernel's verb to get one, and the port cannot
drift into being a handle: it holds two functions, not a kernel.

### The router is the adapter's own protocol surface

The router mints the request's correlation **before** the request's kernel is
in the context — the app host is put there afterwards. So the router builds
the family's ports from the kernel it holds, the way the council family has
always been built:

```go
correlation := appcorrelation.PortsFromKernel(k)
host.correlation = correlation.Mint()
ctx = correlation.WithActor(ctx, string(principal.Kind), host.correlation)
```

## The rule

The correlation's verbs are the application's, and the rule is **by whole
word**:

> a kernel verb whose name carries "Correlation" as a **complete word** is the
> correlation family's business.

The sweep takes in **three** verbs: the mint (`NewCorrelation`), the actor
decoration (`WithActorCorrelation`) and the wake provenance's parent
correlation (`WakeContextParentCorrelation`). The third says nothing about
minting — it is a reader — but it reads the same identity, so it is the same
family's business.

The guard now holds **126** application-owned verbs.

## What the slice could NOT hold

The binding test for this family is weaker than the ones before it, and the
slice says so rather than pretending otherwise.

The correlation is `"run-" + ulid.New()` — a fresh ULID every time, from a
generator the kernel does not own per kernel. The daemon's mint and the
tenant's mint are therefore **indistinguishable by their value**: a binding
that answered with the wrong kernel would hand out a perfectly good ULID.

So the test holds what CAN be held: the daemon's binding reaches a live kernel
(a binding that answered for nobody would mint nothing), and every mint is its
own (two operations carrying one correlation cannot be traced). What it cannot
hold is which kernel minted — and the slice records that as a limit rather
than dressing it up.

The kernel's correlation lives in a **private** context key, and the kernel
publishes no reader for it (`ActorFromCtx` is the only public one), so the
application's test reads the **actor** instead — the part of the decoration
the kernel does publish.

## The tests

| test | holds |
|---|---|
| `TestTheCorrelationPortsReachTheKernelMint` | the port mints the daemon's own correlation, two mints are two identities, and the actor is stamped into the context |
| `TestTheCorrelationPortsSurviveAHostWithNoKernel` | a host with no kernel mints nothing and leaves the context alone |
| `TestTheAdapterBindsTheCorrelationPortsToItsOwnKernel` | the daemon's binding reaches a live kernel, and every mint is its own |
| `TestTheGuardNamesEveryCorrelationVerbTheKernelHas` | the guard names every correlation verb the kernel has |
| `TestTheCorrelationFamilyClaimsNoOtherFamilysVerbs` | the correlation's list claims no other family's verb and does not claim the whole kernel |

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `FF14A51BFB8821855CE2797E05FD752C925DC346971EF36C3103ECA187528A74`).

M08 through M12 put the kernel back — the roster's repair, the roster's wake,
the router, the operation audit and the schedule's action each reach for the
kernel's correlation again — and the **verb guard** catches every one of them.
None of them is a behavioural regression: the wiring would work **exactly as
well**, it would simply be in the wrong layer.

M01, M02, M06 and M07 are the ones that bite: the port holds **no** mint,
the daemon's binding answers for **nobody**, the port hands out **one**
correlation for every operation, and the port stamps **no** actor.

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

The handle stays, and the adapter still calls **24** kernel verbs on it and
hands over **9** method values: the run decorators (**4**), the approvals, the
market, the plugins, the catalog and the rest.

Two doors are still open in the adapter's own layer: the **catalog** and the
**reload** — and the OAuth/catalog family (`kernel/app/providers`) still holds
the handle for both, along with the catalog store. That family will have to
answer the handle rule before it can answer the verb rule, exactly as the
memory family's distillation service had to.
