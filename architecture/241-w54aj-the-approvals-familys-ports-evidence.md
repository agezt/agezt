# W5.4aj — the approvals family, and the queue two kernels answer for

## What the slice is

The **twenty-third** slice of the port programme, and the **fourteenth** one of its
last phase: the handle itself.

The approval registry is the daemon's own human-in-the-loop queue — the requests
waiting for an operator and the decision that answers them. The adapter reached
for the kernel's registry in **three** places in **one** file.

## What the slice found

| file | sites | whose queue |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 3 | one routed, two the daemon's own |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **16** | **14** |
| distinct kernel verbs the adapter names | **13** | **12** |
| method values handed over | **9** | **8** |

The **application** layer held no approval handle either: `kernel/app/approvals`
already took a `Broker` of its own. What the adapter did was reach for the
kernel's registry in three directions — the live list, the mission control's
pending count and the plan runner's node spec — and spell the **nil case** of
its own.

## What changed

The approvals family's ports moved into the application
(`kernel/app/approvals/ports.go`):

```go
type Ports struct {
	registry func() *approval.Registry
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{registry: func() *approval.Registry { return k.Approvals() }}
}

func (p Ports) Broker() Broker { ... }
func (p Ports) Registry() *approval.Registry { ... }
```

and the adapter keeps the bindings (`kernel/controlplane/approvals_ports.go`) —
which do not name the runtime, like every binding since W5.4v:

```go
func (s *Server) approvalPorts() appapprovals.Ports {
	return appapprovals.PortsFromKernel(s.k)
}

func requestApprovalPorts(ctx context.Context) appapprovals.Ports {
	return appapprovals.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

There are **two** bindings, and they answer for **two different kernels**:

- The live approvals — the pending list and the decision — run on the
  **request's** path, so the **routed** kernel is the one that answers: a tenant
  decides its own queue.
- The mission control's pending count and the plan runner's node spec are the
  **daemon's own** surfaces, so they ask the daemon's kernel.

### Two views of one queue

The port answers in **two** shapes, because two kinds of consumer want it:

- `Broker()` — the family's own narrow view: the waiting requests and the
  decision over them. This is what the live approvals read.
- `Registry()` — the registry itself. The **scheduler's** node spec carries the
  queue, so the plan runner needs the queue and not a reader of it.

The port's names are **not** the kernel's: a port that carried the name of the
verb it stands for would let the adapter name the kernel's verb through the
port's door. `TestTheApprovalPortNamesAreNotTheKernelVerbs` holds that by
reflection over the port's own method set and the kernel's.

### The refusal is written once

The queue is the daemon's own file, and a daemon that was given none holds none.
The live approvals **refuse** now — `approvals.ErrNoBroker` — instead of
reading from nothing, and the refusal is written **once**, in the family that
answers for the approvals.

## The rule

The approvals' verbs are the application's, and the rule is **by whole word**:

> a kernel verb whose name carries "Approvals" as a **complete word** is the
> approvals family's business.

The sweep takes in **one** verb today: `Approvals`.

The guard now holds **138** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheApprovalPortsReachTheKernelRegistry` | the port hands over the daemon's own queue |
| `TestTheApprovalPortsSurviveAHostWithNoKernel` | a host with no kernel hands over no queue |
| `TestTheApprovalPortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheLiveApprovalsRefuseWithoutABroker` | a daemon with no queue refuses, and it refuses the same way every time |
| `TestTheAdapterBindsTheApprovalPortsToTheRoutedKernel` | the request's ports follow the **routed** kernel, and the daemon's own request answers for the daemon's own queue |
| `TestTheAdapterBindsTheApprovalPortsToTheDaemon` | the daemon-level binding answers for the **daemon's** queue, not a tenant's |
| `TestTheLiveApprovalsReadThroughThePort` | the application reads the queue through the port |
| `TestTheGuardNamesEveryApprovalVerbTheKernelHas` | the guard names every approval verb the kernel has |
| `TestTheApprovalFamilyClaimsNoOtherFamilysVerbs` | the approvals' list claims no other family's verb and does not claim the whole kernel |

**9** permanent tests.

## The mutations

**14** mutations, **14** caught, in two independent passes with **byte-identical**
logs (SHA-256 `3383752078807A7EE0857D3DB012FE4BF926E6DFF41965ED530B4265B022A87E`).

M01, M02 and M03 are the ones that bite: the port holds **no** registry, a host
with no kernel asks the kernel **anyway**, and the port hands over **nothing**.

M04 gives the port a method that carries a kernel verb's own name, M05 empties
the request's binding, M06 makes it follow the **daemon's** kernel — a tenant's
queue would become the daemon's — M07 empties the daemon's binding, M08, M09 and
M10 put the kernel back — the mission control, the live approvals and the plan
runner each reach for the kernel's registry again — and the **verb guard** catches
every one of them; none is a behavioural regression, because the wiring would
work **exactly as well**, it would simply be in the wrong layer. M11 takes the
family out of the guard, M12 hands the family **another** family's verbs, M13
takes the refusal away, and M14 makes the binding name the runtime.

## What the slice does not do

The handle stays, and the adapter still calls **14** kernel verbs on it and hands
over **8** method values: the run decorators, the state, the world, the taste,
the data lake, the seats, the reflect, the reaper, the update drain and the rest.

The **runs family** still holds the kernel handle — for the run decorators, the
roster, the warden, the model and the catalog. It is the next family, and it is
the largest one left.

The daemon's **overview** (`kernel/app/system`) still holds the handle for
**thirteen** verbs across **eight** families. That surface is not one family but a
**roll-up** of many, and it will have to ask each of them rather than the kernel.
