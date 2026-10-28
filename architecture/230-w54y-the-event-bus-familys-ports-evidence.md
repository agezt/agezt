# W5.4y — the event bus family's ports, and the eleven families that publish through it

## What the slice is

The **twelfth** slice of the port programme, and the **third** one of its last
phase: the handle itself.

The bus is the one capability every family **publishes** through. The adapter
reached for the kernel's bus in **nineteen** places across **seven** files — the
second largest debt in the control plane, after the journal.

## What the slice found

| file | sites | whose bus |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 6 | four routed, two the daemon's own |
| `kernel/controlplane/app_host.go` | 3 | the routed one — the operation audit |
| `kernel/controlplane/dispatch_audit.go` | 3 | the caller's kernel |
| `kernel/controlplane/roster_escalation.go` | 2 | the caller's kernel |
| `kernel/controlplane/workboard_dispatch_helpers.go` | 2 | the caller's kernel |
| `kernel/controlplane/workflow.go` | 2 | the daemon's own |
| `kernel/controlplane/pulse.go` | 1 | the daemon's own |

Seven of the nineteen were **routed** — a tenant's bus is the tenant's bus — and
five read the daemon's own. The slice keeps that split exactly:
`requestBusPorts(ctx)` answers for the routed kernel, `s.busPorts()` for the
root. The seven that take the kernel as a **parameter** (the dispatch audit, the
roster's operator action, the workboard's dispatch) keep following the kernel
they were handed: they call the application's own `PortsFromKernel(k)`.

The adapter's handle-verb debt, measured across the whole control plane:

| | before | after |
|---|---|---|
| call sites on the handle | **76** | **57** |
| distinct kernel verbs the adapter names | **29** | **28** |

## What changed

The event bus family's ports moved into the application
(`kernel/app/eventbus/ports.go`):

```go
type Ports struct {
	bus *bus.Bus
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{bus: k.Bus()}
}
```

and the adapter keeps the binding (`kernel/controlplane/bus_ports.go`) — which
does not name the runtime, like every binding since W5.4v:

```go
func (s *Server) busPorts() appeventbus.Ports { return appeventbus.PortsFromKernel(s.k) }

func requestBusPorts(ctx context.Context) appeventbus.Ports {
	return appeventbus.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

### The port carries operations, not the bus

The bus is the first family whose port does **not** carry the capability as a
named field: the bus is held **unexported**, and the port offers the three
things the families actually use — `Publish`, `Subscribe` and `Redactor`.

That is not a style choice. The bus has a **nil discipline** — a daemon can
hold no bus — and the adapter used to spell that **five** times across **five**
files (`if k == nil || k.Bus() == nil`). Five spellings that could disagree
with each other. The application spells it **once**:

```go
func (p Ports) Publish(spec event.Spec) (*event.Event, error) {
	if p.bus == nil {
		return nil, ErrNoBus
	}
	return p.bus.Publish(spec)
}
```

Because the object is hidden, the adapter **cannot** publish into nothing and
cannot forget the check — there is no check to forget. And because there is no
named field, the **verb-name collision cannot happen at all**: this programme
learned that lesson four times (`MCPAttached` → `Attachment`, `Rollup` →
`Progress`, the workboard's `Journal` → `Events`, the workflow's `Journal` →
`Events`), and this family simply never had the chance.

The families still narrow the bus themselves — the roster's operator action, the
workboard's dispatch, the pulse stream's subscription, the redaction family's
redactor — and those narrow views are the consumers' business, not the family's.

### The operation audit holds the ports, not the handle

The application layer's operation audit published its `op.invoked` and
`op.completed` events through the routed kernel's bus, reached through the
handle it was given. It now holds the **ports** — the same bus, one layer down:

```go
span := &appAuditSpan{host: host, ports: requestBusPorts(ctx), operation: record.Operation, start: time.Now()}
```

## The rule

The bus's verbs are the application's, and the rule is **by name** — the
kernel's verb says what it is. The family answers to three names, and they
sweep in everything that publishes:

| name | verbs |
|---|---|
| `Bus` | `Bus`, `PublishBus`, `PublishBusEvent` |
| `Event` | `PublishEvent` |
| `Publish` | `PublishContextFailureAnalysis`, `PublishHeuristicBypass`, `PublishIntentInterpreted` |

The `Publish` name is the interesting one: the runtime's own publishes — the
intent interpretation, the heuristic bypass, the context failure analysis —
publish **through the bus**, so the adapter must not name them either. The
adapter publishes by **asking** the family, never by reaching for the kernel.
The guard now holds **101** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheEventBusPortsReachTheKernelBus` | the ports reach the kernel's bus, in both directions |
| `TestTheEventBusPortsSurviveAHostWithNoBus` | a host with no bus publishes nothing, subscribes to nothing and has no redactor |
| `TestTheAdapterBindsTheBusPortsToItsOwnKernel` | the daemon's bus answers for the daemon's kernel and the request's for the routed one |
| `TestTheGuardNamesEveryBusVerbTheKernelHas` | the guard names every bus verb the kernel has |

## The mutations

**13** mutations, **13** caught, in two independent passes with **byte-identical**
logs (SHA-256 `94F0438265E5EBED737C4ED4C248282CA73BB3E11F639F70725E4C45AE2302E0`).

M11, M12 and M13 put the kernel back — the routed publisher reaches for the
kernel's bus again, the daemon's subscriber does the same, and the binding file
names the runtime again — and the **three rules** catch every one of them. None
of them is a behavioural regression: the wiring would work **exactly as well**,
it would simply be in the wrong layer, and no behaviour test can see that.

M06 and M07 are the ones that bite: the port publishes **nothing** (the daemon's
event never reaches the daemon's journal) and the port subscribes to
**nothing** (a subscriber opened through the port never sees the kernel's
events). M08 removes the nil discipline and the host with no bus **panics**
instead of answering; M09 makes the port claim it is ready when there is no
bus at all.

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

The handle stays, and the adapter still calls **57** kernel verbs on it: the
skill forge (**7**), the providers (**7**), the base directory (**6**), the
memory (**5**), the policy engine (**4**), the correlation mint (**3**) and the
rest.

The bus was the second largest of them, and it came over in the shape the rest
will take: the application fills its own ports, the adapter asks the family,
and the **eleven families that publish through it stop reaching for the
kernel**.
