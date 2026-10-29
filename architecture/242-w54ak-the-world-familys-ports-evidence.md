# W5.4ak — the world family, and the one site that asked for the whole model

## What the slice is

The **twenty-fourth** slice of the port programme, and the **fifteenth** one of
its last phase: the handle itself.

The world model is the daemon's own graph of entities and the relations between
them — what the operator's world is made of. The adapter reached for the
kernel's graph in **one** place.

## What the slice found

| file | sites | whose world |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 1 | the routed one |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **14** | **13** |
| distinct kernel verbs the adapter names | **12** | **11** |
| method values handed over | **8** | **8** |

The **application** layer held no world handle either: `kernel/app/world`
already took the contract's `worldmodelapi.Graph` as an argument.

## What changed

The world family's ports moved into the application
(`kernel/app/world/ports.go`):

```go
type Ports struct {
	graph func() worldmodelapi.Graph
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{graph: func() worldmodelapi.Graph { return k.World() }}
}

func (p Ports) Graph() worldmodelapi.Graph {
	if p.graph == nil {
		return nil
	}
	return p.graph()
}
```

and the adapter keeps the binding (`kernel/controlplane/world_ports.go`) — which
does not name the runtime, like every binding since W5.4v:

```go
func requestWorldPorts(ctx context.Context) appworld.Ports {
	return appworld.PortsFromKernel(ctx.Value(appHostKey{}).(appHost).kernel)
}
```

There is **no daemon-level binding**, and the reason is the shape of the read
path: every surface that reads the world reads it on the **request's** path, so
the routed kernel is the one that answers. A binding nobody calls is dead code.

### The port's names are not the kernel's

The port answers `Graph` — never `World`. A port that carried the name of the
verb it stands for would let the adapter name the kernel's verb through the
port's door.
`TestTheWorldPortNamesAreNotTheKernelVerbs` holds that by reflection over the
port's own method set and the kernel's.

### The daemon that holds no world model

The world model is the daemon's **file**: the daemon opens the store and hands
the kernel the contract's port and the **builder**. A daemon that was given
neither holds **no** graph, and the port hands over nothing — the surfaces
already refuse with their own sentence (`errNoWorldModel`), and the port does
not invent one.

## The rule

The world's verbs are the application's, and the rule is **by whole word**:

> a kernel verb whose name carries "World" as a **complete word** is the world
> family's business.

The sweep takes in **one** verb today: `World`.

The guard now holds **139** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheWorldPortsReachTheKernelGraph` | the port hands over the daemon's own graph |
| `TestTheWorldPortsSurviveAHostWithNoWorld` | a daemon that was given no world model hands over none |
| `TestTheWorldPortsSurviveAHostWithNoKernel` | a host with no kernel hands over no graph |
| `TestTheWorldPortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsTheWorldPortsToTheRoutedKernel` | the request's ports follow the **routed** kernel, and the daemon's own request answers for the daemon's own |
| `TestTheAdapterBindsTheWorldPortsToAHostWithNoWorld` | a daemon given no world model hands over none |
| `TestTheGuardNamesEveryWorldVerbTheKernelHas` | the guard names every world verb the kernel has |
| `TestTheWorldFamilyClaimsNoOtherFamilysVerbs` | the world's list claims no other family's verb and does not claim the whole kernel |

**8** permanent tests.

## The mutations

**11** mutations, **11** caught, in two independent passes with **byte-identical**
logs (SHA-256 `6FECE8AED97E2A8E431D2A02B5BEBAC59C66F3E727C31893AA4C93179C612662`).

M01, M02 and M03 are the ones that bite: the port holds **no** graph, a host
with no kernel asks the kernel **anyway**, and the port hands over **nothing**.

M04 gives the port a method that carries a kernel verb's own name, M05 empties
the request's binding, M06 makes it follow the **daemon's** kernel — a tenant's
world would become the daemon's — M07 builds the service from **nothing** and
the world add over the socket refuses, M08 puts the kernel back and the **verb
guard** catches it, M09 takes the family out of the guard, M10 hands the family
**another** family's verbs, and M11 makes the binding name the runtime.

## What the slice does not do

The handle stays, and the adapter still calls **13** kernel verbs on it and hands
over **8** method values: the run decorators, the state, the taste, the data
lake, the seats, the reflect, the reaper, the update drain and the rest.

The **runs family** still holds the kernel handle — for the run decorators, the
roster, the warden, the model and the catalog. It is the next family, and it is
the largest one left.

The daemon's **overview** (`kernel/app/system`) still holds the handle for
**thirteen** verbs across **eight** families. That surface is not one family but a
**roll-up** of many, and it will have to ask each of them rather than the kernel.
