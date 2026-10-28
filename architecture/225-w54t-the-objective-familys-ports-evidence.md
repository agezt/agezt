# W5.4t — the objective family's ports, and the rule that reads a file

## What the slice is

The **seventh** family of the port programme: the **objectives**. The daemon
holds the objective library — the contract's store port, opened by the host
(W3.53) — and the kernel answers four things about it: read the library, roll
an objective's progress up, and the five writes.

The adapter's objective surface was small — **two** sites — but it was the
shape the port programme is about:

```go
appokr.New(k.OKR(), k)                        // the library, and the handle as the rollup
appokr.NewLifecycle(k, appokr.New(k.OKR(), k)) // the handle as the mutation host
```

So the objective family's ports moved into the application:

```go
// kernel/app/okr/ports.go
type Ports struct {
	Reader   Reader          // the objective library — NIL when the daemon holds none
	Progress Rollupper       // the rollup of an objective's progress
	Mutation MutationHost    // create, key result, link, unlink and archive
}

func PortsFromKernel(k *runtime.Kernel) Ports
```

and the adapter keeps the binding (`kernel/controlplane/okr_ports.go`):
`okrPortsFor(k)` for a kernel, `requestOKRPorts(ctx)` for the **routed** one —
a tenant's objectives are the tenant's objectives.

## The rule reads a FILE, not a name

The objective family's verbs are:

| Verb | What it is |
|---|---|
| `OKR` | the library's store port |
| `Rollup` | the progress rollup of an objective |
| `ObjectiveRollup` | the board's rollup of an objective |
| `CreateObjective` | write |
| `AddObjectiveKeyResult` | write |
| `LinkObjectiveTask` | write |
| `UnlinkObjectiveTask` | write |
| `ArchiveObjective` | write |

**Eight** verbs, and a name rule would have missed one of them: the rollup verb
is called `Rollup` and says nothing about objectives. A verb the rule misses
is a verb the adapter may name — so the objective family is found by reading
the kernel's own objective file (`kernel/runtime/okr.go`), the way the roster
family is found by reading the roster accessor file.

The rule reads the **exported** methods of that file only: the file also holds
two private helpers, and a private method is not a verb the adapter can name.

The guard now holds **75** application-owned verbs.

### The port's field names must not collide with kernel verbs — again

The first draft called the rollup port `Rollup`, because the app's own
interface is `Rollupper`. The guard caught the adapter the moment it did:

```
app_operations.go reaches the kernel's Rollup — that verb belongs to the application layer
```

The field is `Progress` now. This is the **second** time this slice family
learned the same lesson (the tools family learned it with `MCPAttached` →
`Attachment`), which is why the lesson is written down in the evidence rather
than only in the code.

## The handle rule now covers the WHOLE programme

The sixth rule (added with the tools family) says: an application takes
**ports**, never the handle they were filled from. It was written for the tools
application alone, and the objective family needs it too — `appokr.NewLifecycle(k, …)`
names no verb at all, so the verb rule cannot see it.

So the rule now reads the programme's own membership from the tree: **every
application that declares its own `ports.go` is in scope**. A family is covered
the day it declares its ports, and the list cannot rot.

## What moved

| | Before | After |
|---|---|---|
| the objective service | `appokr.New(k.OKR(), k)` | `appokr.New(ports.Reader, ports.Progress)` |
| the objective lifecycle | `appokr.NewLifecycle(k, appokr.New(k.OKR(), k))` | `appokr.NewLifecycle(ports.Mutation, appokr.New(ports.Reader, ports.Progress))` |

**Two** kernel verb sites left the adapter's production code, and the adapter
no longer hands the handle to the objective application at all.

## The proof

### Permanent tests — 3 new

| Test | What it holds |
|---|---|
| `TestTheObjectivePortsReachTheKernelObjectives` (`kernel/app/okr`) | the ports are **filled**, the reader port sees the kernel's objectives, and the mutation port writes **through** the kernel |
| `TestTheObjectivePortsSurviveADaemonWithNoObjectiveLibrary` (`kernel/app/okr`) | the **nil-port discipline**: a daemon with no library holds a nil reader, and the application answers "no objectives" rather than calling into nothing |
| `TestTheAdapterBindsTheObjectivePortsToItsOwnKernel` (`kernel/controlplane`) | the binding, end to end: the daemon's objective list answers from the kernel's library, and the request's ports follow the **routed** kernel |

The adapter's objective behaviour was already covered by the existing
`okr_app_test.go` (the reads, the creates and the audit), so this slice adds
the **binding** on top of it.

### The mutation run — 10 mutations, 10 caught

Two independent passes with **byte-identical** logs (SHA-256
`D528C9DD1E373B04A6D720FC1E1637D5E9EA8DFCD4F63F25EC09F67447986B70`; the
elapsed times, ULIDs, epoch stamps, addresses, goroutine ids, temporary paths
and ephemeral ports are normalized, because they are not the mutation's
fingerprint).

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the reader port is nil — the application cannot read the objectives | the ports test |
| M02 | the progress port is nil | the ports test |
| M03 | the mutation port is nil | the ports test |
| M04 | the adapter's binding fills no ports at all | the binding test |
| M05 | the request's binding fills no ports — the tenant's objectives are gone | the binding test |
| M06 | the daemon's objective service gets no reader | the binding test |
| M07 | the adapter names the objective verb again | the **verb guard** |
| M08 | the adapter hands the handle to the objective lifecycle | the **handle rule** |
| M09 | the derivation narrows — the objective verbs fall out of the guard | the guard test |
| M10 | the handle rule goes blind | the rule's own test |

M07 and M08 are the mutations that matter most, and neither is a behavioural
regression: the wiring would work **exactly as well**, it would simply be in
the wrong layer.

M09 is the one worth reading twice. The derivation narrows from "every
exported objective verb" to "the ones whose name starts with Objective" — and
the guard's own test catches it, because that test reads the kernel's file
**independently** of the helper the guard uses. A derivation that checks
itself is not a derivation.

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

The **ratchet does not move yet**: the control plane still imports the runtime
in **15** files.

The objective library is the **daemon's** file, so the host still opens it
and hands the kernel the contract's port — that is the host's business (W3.53),
and the slice does not move it.

Seven families are done — council, roster, standing, schedule, workflow, tools,
objectives — and the **workboard** is next: it is the largest of them (six
services, seventeen verbs) and the last of the family map's *verbs without
libraries* that the adapter still names.
