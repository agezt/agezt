# W5.4u — the workboard family's ports, and the last of the board's handle

## What the slice is

The **eighth** family of the port programme, and the **largest** one: the task
board, with **seventeen** kernel verbs, **six** services and **three** adapter
sites.

The adapter's workboard surface was not a list of verb calls — it was three
places where the **whole handle** was handed to the application:

```go
appworkboard.NativeServices{Reads: appworkboard.New(k.Workboard()), Lifecycle: appworkboard.NewLifecycle(k, k.Workboard()), Relations: appworkboard.NewRelations(k), Watch: appworkboard.NewWatch(k.Workboard(), k.Journal()), ...}
appworkboard.NewExecution(s.k, s.k.Seats(), s.k.Workboard(), ...)
appworkboard.NewDispatch(s.k.Workboard(), s.k, ...)
```

So the workboard family's ports moved into the application:

```go
// kernel/app/workboard/ports.go
type Ports struct {
	Reader     Reader            // the board — NIL when the daemon holds none
	Store      WatchStore        // the board's watch surface
	SeatSetter SeatSetter        // the seat assignment
	Relations  RelationHost      // the task relations
	Lifecycle  LifecycleHost     // the task lifecycle
	Dispatch   DispatchHost      // the claim and link verbs
	Execution  ExecutionHost     // the execution verbs
	SeatIndex  ExecutionSeats    // the seats
	SeatStore  seatapi.Store     // the seat catalog
	TaskIndex  ExecutionTasks    // the tasks
	Events     journalview.Reader // the event journal
}

func PortsFromKernel(k *runtime.Kernel) Ports
```

and the adapter keeps the binding (`kernel/controlplane/workboard_ports.go`):
`workboardPortsFor(k)` for a kernel, `s.workboardPorts()` for the daemon's own,
`requestWorkboardPorts(ctx)` for the **routed** one.

## What moved

| | Before | After |
|---|---|---|
| the board's reads | `appworkboard.New(k.Workboard())` | `appworkboard.New(ports.Reader)` |
| the task lifecycle | `appworkboard.NewLifecycle(k, k.Workboard())` — the **whole handle** | `appworkboard.NewLifecycle(ports.Lifecycle, ports.SeatSetter)` |
| the task relations | `appworkboard.NewRelations(k)` — the **whole handle** | `appworkboard.NewRelations(ports.Relations)` |
| the board's watch | `appworkboard.NewWatch(k.Workboard(), k.Journal())` | `appworkboard.NewWatch(ports.Store, ports.Events)` |
| the seat validation | `seatValidator(k.Seats())` | `seatValidator(ports.SeatStore)` |
| the dispatch | `appworkboard.NewDispatch(s.k.Workboard(), s.k, …)` — the **whole handle** | `appworkboard.NewDispatch(ports.Store, ports.Dispatch, …)` |
| the execution | `appworkboard.NewExecution(s.k, s.k.Seats(), s.k.Workboard(), …)` — the **whole handle** | `appworkboard.NewExecution(ports.Execution, ports.SeatIndex, ports.TaskIndex, …)` |

**Seven** sites left the adapter's production code, and the adapter no longer
hands the handle to the workboard application at all.

The dispatcher's **agent** closure and the execution's **run** and **publish**
closures stay in the adapter: they are the host's protocol surface (who the
seat is, how the run is decorated, what is published) and not the
application's business.

## The rule reads a GLOB of files

The workboard family's verbs live in **two** files — the task lifecycle in
`kernel/runtime/workboard.go`, the proof in `kernel/runtime/workboard_proof.go`
— so the rule reads them all, and it finds them with a **glob**:
`../runtime/workboard*.go`. A verb that lands in a new workboard file is
guarded the day it lands.

The rule reads the **exported** methods only: the files also hold private
helpers, and a private method is not a verb the adapter can name.

And the rule reads the **name** only as far as it must: the proof verb is
called `ProveTask` and says nothing about the board — so the rule cannot be a
name rule, the way the objective family's could not be (W5.4t).

The guard now holds **92** application-owned verbs.

## The port's field names — the third lesson

The port's field names must not collide with kernel verbs, and the workboard
family had the same trap twice:

- `Events` (the journal) — the kernel's journal verb is `Journal`, so the
  field is `Events` ✓;
- `SeatStore` (the seat catalog) — the kernel's seat verb is `Seats`, so the
  field is `SeatStore` ✓.

The **seat** port deserves a word: the workboard family asks "is this seat
real?", and the seat family owns the answer. The port holds the contract's
seat store, and the adapter's validator reads it — the same shape as the
registry being the tools family's answer for four other families.

## The proof

### Permanent tests — 3 new

| Test | What it holds |
|---|---|
| `TestTheWorkboardPortsReachTheKernelBoard` (`kernel/app/workboard`) | the ports are **filled** (all eleven), the reader port sees the kernel's tasks, and the lifecycle port writes **through** the kernel |
| `TestTheWorkboardPortsSurviveADaemonWithNoBoard` (`kernel/app/workboard`) | the **nil-port discipline**: a daemon with no board holds a nil reader and a nil store, and the application answers "the task board is not available on this daemon" rather than calling into nothing |
| `TestTheAdapterBindsTheWorkboardPortsToItsOwnKernel` (`kernel/controlplane`) | the binding, end to end: the daemon's task list answers from the kernel's board, and the request's ports follow the **routed** kernel |

The adapter's workboard behaviour was already covered by the existing
`workboard_app_test.go` (the reads, the lanes, the creates, the claims and the
audit), so this slice adds the **binding** on top of it.

### The mutation run — 19 mutations, 19 caught

Two independent passes with **byte-identical** logs (SHA-256
`2802D351DF11DF68083229098A69A18B58B44EA94FCC058E8DBD1122F7C05A6C`; the
elapsed times, ULIDs, epoch stamps, addresses, goroutine ids, temporary paths
and ephemeral ports are normalized, because they are not the mutation's
fingerprint).

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01–M11 | one of the eleven ports is nil — the application cannot read the board, watch it, seat a task, link a task, run a task's life, claim a task, execute a task, read the seats, read the tasks or read the journal | the ports test |
| M12 | the adapter's binding fills no ports at all | the binding test |
| M13 | the request's binding fills no ports — the tenant's board is gone | the binding test |
| M14 | the daemon's board service gets no reader | the binding test |
| M15 | the adapter names the board verb again | the **verb guard** |
| M16 | the dispatch hands the handle to the application | the **handle rule** |
| M17 | the execution hands the handle to the application | the **handle rule** |
| M18 | the derivation narrows — the workboard verbs fall out of the guard | the guard test |
| M19 | the derivation loses a file — the proof verb falls out of the guard | the guard test |

M15, M16 and M17 are the mutations that matter most, and none of them is a
behavioural regression: the wiring would work **exactly as well**, it would
simply be in the wrong layer. No behaviour test can see those — only the two
rules can.

M19 is worth reading twice. The rule's **glob** loses the proof file, and the
guard's own test catches it — because that test finds the files **itself**,
with its own glob, rather than through the helper the guard uses. A
derivation that checks itself is not a derivation.

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

The task board is the **host's** file (W3.47), so the host still opens it and
hands the kernel the contract's port — that is the host's business, and the
slice does not move it.

Eight families are done — council, roster, standing, schedule, workflow, tools,
objectives, workboard — and the family map's **verbs without libraries** are
done with them: the adapter no longer names a workboard, an objective, a tool
or an MCP server. What is left is the **handle** itself: the control plane
still imports the runtime in fifteen files, and the families that hold it
(the seat catalog, the journal, the sandbox, the providers) are the ones the
roadmap calls *libraries* — they own a store, and the store is the host's
file.
