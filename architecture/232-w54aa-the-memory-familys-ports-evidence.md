# W5.4aa — the memory family's ports, and the distillation that held the handle

## What the slice is

The **fourteenth** slice of the port programme, and the **fifth** one of its
last phase: the handle itself.

The memory is the daemon's own file (W3.57) — the records the operator and the
agents keep. The adapter reached for the kernel's memory in **five** places
across **three** files, and handed the kernel's **handle** to the memory
family's own distillation service.

## What the slice found

| file | sites | whose memory |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 3 | one routed, two the daemon's own |
| `kernel/controlplane/roster_teardown_state.go` | 2 | the daemon's own |
| `kernel/controlplane/roster_cascade.go` | 1 | the daemon's own |

Two of the five were the same verb seen twice: the memory's **manager**
(`Memory`, twice) and its **active records** (`MemoryActive`, twice), plus the
**forget** (`MemoryForget`).

The sixth site was not a verb at all: the memory family's **distillation
service** was handed the kernel handle.

| | before | after |
|---|---|---|
| call sites on the handle | **50** | **45** |
| distinct kernel verbs the adapter names | **27** | **24** |

## What changed

The memory family's ports moved into the application
(`kernel/app/memory/ports.go`):

```go
type Ports struct {
	manager   memoryapi.Manager
	distiller Distiller
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		manager:   k.Memory(),
		distiller: kernelDistiller{k: k},
	}
}
```

and the adapter keeps the binding (`kernel/controlplane/memory_ports.go`) —
which does not name the runtime, like every binding since W5.4v:

```go
func (s *Server) memoryPorts() appmemory.Ports { return appmemory.PortsFromKernel(s.k) }

func requestMemoryPorts(ctx context.Context) appmemory.Ports {
	return appmemory.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

The memory may be absent, and the port answers for that case: the **active**
records are none and a **forget** refuses with `ErrNoMemory` — the same answer
the kernel's own memory verbs gave, spelled once now, in the application.

### The distillation service was handed the handle

The memory family's distillation service took the kernel itself:

```go
return appmemory.NewDistillation(ctx.Value(appHostKey{}).(appHost).kernel)
```

The **handle rule** caught it the moment the memory family declared its own
ports: a family that declares its ports takes ports, not the handle they were
filled from — and the rule reads the tree, so a family is covered the day it
writes its `ports.go`.

The fix is not a rename. The distillation needs two things the memory manager
alone cannot give: the **provider** and the **model** a distillation runs
under, and the kernel is the only thing that knows them. So the port carries a
**capability** instead of the handle:

```go
type kernelDistiller struct{ k *runtime.Kernel }

func (d kernelDistiller) NewCorrelation() string { return d.k.NewCorrelation() }

func (d kernelDistiller) DistillBrain(ctx context.Context, corr string) (memoryapi.BrainDistillReport, error) {
	return d.k.MemoryDistillBrain(ctx, corr)
}
```

The family sees a `Distiller`, never a kernel.

### The door the handle opened was not the door the port opened

The port's distillation reaches the kernel through its **checked** surface:

```go
func (d kernelDistiller) DistillBrain(ctx context.Context, corr string) (memoryapi.BrainDistillReport, error) {
	return d.k.DistillBrain(ctx, corr)
}
```

The first version of the port reached for the accessor behind it
(`MemoryDistillBrain`) — the same work, one layer down — and the **gate caught
it**: a halted daemon answered the consolidation with a success. The two are
different doors to the same room, and only one of them answers when the
daemon is going down:

```go
func (k *Kernel) DistillBrain(ctx context.Context, corr string) (memoryapi.BrainDistillReport, error) {
	...
	if halted {
		return memoryapi.BrainDistillReport{}, ErrHalted
	}
	return k.MemoryDistillBrain(ctx, corr)
}
```

The lesson is general: **a port must reach the same surface the handle
reached.** A capability is not only what it can do, it is also what it
refuses.

## The rule

The memory's verbs are the application's, and the rule is **by whole word** —
the rule the forge slice had already introduced:

> a kernel verb whose name carries "Memory" as a **complete word** is the
> memory family's business.

The sweep takes in the distillation verbs — the brain, the facts and the
profile: they consolidate the memory's own records, so they are the memory's
business even though the runtime calls them. The family answers to **two**
names, "Memory" and "Distill", because the checked distillation surface
(`DistillBrain`, `DistillProfile`) does not say "memory" in its name. The
guard now holds **120** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheMemoryPortsReachTheKernelMemory` | the ports reach the kernel's memory: the port lists the records the memory holds and its forget really forgets |
| `TestTheMemoryPortsSurviveAHostWithNoMemory` | a host with no memory lists nothing, refuses a forget and mints no correlation |
| `TestTheAdapterBindsTheMemoryPortsToItsOwnKernel` | the daemon's memory answers for the daemon's kernel and the request's for the routed one |
| `TestTheGuardNamesEveryMemoryVerbTheKernelHas` | the guard names every memory verb the kernel has |

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `132356820D1025E08B8C112D466A8CC6CD93E50F5B70C050DD830F5C37051BCD`).

M11, M12 and M13 put the kernel back — the routed consumer reaches for the
kernel's memory again, the daemon's consumer does the same, and the
distillation service is handed the handle again — and the **three rules**
catch every one of them. None of them is a behavioural regression: the wiring
would work **exactly as well**, it would simply be in the wrong layer.

M06, M07 and M09 are the ones that bite: the port lists **nothing**, the port
forgets **nothing**, and the port's refusal becomes a silent success.

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

The handle stays, and the adapter still calls **45** kernel verbs on it: the
providers (**7**), the base directory (**6**), the policy engine (**4**), the
correlation mint (**3**) and the rest.

The memory came over in the shape the rest will take: the application fills its
own ports, the adapter asks the family, and the **families that read the
memory stop reaching for the kernel**.
