# W5.4an — the state family, and the run's state that is not the state's

## What the slice is

The **twenty-seventh** slice of the port programme, and the **eighteenth** one
of its last phase: the handle itself.

The state store is the daemon's own key/value file — the namespaces and the keys
under them, which the agents and the scheduler write to and no operator can
read. The adapter reached for the kernel's store in **one** place.

## What the slice found

| file | sites | whose store |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 1 | the daemon's own |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **10** | **9** |
| distinct kernel verbs the adapter names | **8** | **7** |
| method values handed over | **4** | **4** |

The **application** layer held no state handle either: `kernel/app/state`
already took the read side of the store as an argument.

## What changed

The state family's ports moved into the application
(`kernel/app/state/ports.go`):

```go
type Ports struct {
	store func() Store
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{store: func() Store { return k.State() }}
}

func (p Ports) Store() Store {
	if p.store == nil {
		return nil
	}
	return p.store()
}
```

and the adapter keeps the binding (`kernel/controlplane/state_ports.go`) — which
does not name the runtime, like every binding since W5.4v:

```go
func (s *Server) statePorts() appstate.Ports {
	return appstate.PortsFromKernel(s.k)
}
```

There is **no request-level binding**, and the reason is the shape of the read
path: the state surface inspects the **daemon's own** file, so it asks the
daemon's kernel. A binding nobody calls is dead code.

### The port's names are not the kernel's

The port answers `Store` — never `State`. A port that carried the name of the
verb it stands for would let the adapter name the kernel's verb through the
port's door.
`TestTheStatePortNamesAreNotTheKernelVerbs` holds that by reflection over the
port's own method set and the kernel's.

## The rule

The state's verbs are the application's, and the rule is **by whole word**:

> a kernel verb whose name carries "State" as a **complete word** is the state
 family's business.

The sweep takes in **one** verb today: `State`.

**Three** verbs carry the word and are **not** the state family's:
`CleanupRunState`, `SetupRunState` and `RunControlState` are the **run's** own
business — the state a run leaves behind and the control that stops it. A rule
that swept them in would make the state family claim three other families'
verbs, and `TestTheStateFamilyClaimsNoOtherFamilysVerbs` says so out loud: it
holds that the state's list carries none of them.

The guard now holds **145** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheStatePortsReachTheKernelStore` | the port hands over the daemon's own store |
| `TestTheStatePortsSurviveAHostWithNoKernel` | a host with no kernel hands over no store |
| `TestTheStatePortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsTheStatePortsToTheDaemon` | the daemon-level binding answers for the **daemon's** store |
| `TestTheGuardNamesEveryStateVerbTheKernelHas` | the guard names every state verb the kernel has |
| `TestTheStateFamilyClaimsNoOtherFamilysVerbs` | the state's list claims no other family's verb, **and none of the run's state verbs** |

**6** permanent tests.

## The mutations

**11** mutations, **11** caught, in two independent passes with **byte-identical**
logs (SHA-256 `0DF94C4E5E51195E92FB77E546EA0D9873EB89FE0A59CFB4FA92A97657A7DC94`).

M01, M02 and M03 are the ones that bite: the port holds **no** store, a host
with no kernel asks the kernel **anyway**, and the port hands over **nothing**.

M04 gives the port a method that carries a kernel verb's own name, M05 empties
the daemon's binding, M06 puts the kernel back in the operations and the **verb
guard** catches it, M07 builds the surface from **nothing** and the state read
over the socket refuses, M08 takes the family out of the guard, M09 hands the
family **another** family's verbs, M10 makes the binding name the runtime, and
M11 sweeps the **run's** state verbs into the state's list — and the sharp
guard test catches it.

### One false positive, fixed in the slice

The guard reads the adapter's **raw source**, comments included, so a
commented-out call would still be a reach for the kernel. That is the rule's
point, and it has a price: a piece of prose that happened to end a sentence
with the family's word — "the kernel state store. State is normally invisible
to operators" — read as a call. The slice reworded the comment rather than
loosen the guard, because a guard that skips comments is a guard that a
commented-out call walks past.

## What the slice does not do

The handle stays, and the adapter still calls **9** kernel verbs on it and hands
over **4** method values: the run decorators, the reaper, the research model
and the plan runner.

The **runs family** still holds the kernel handle — for the run decorators, the
roster, the warden and the catalog. It is the next family, and it is the
largest one left.

The daemon's **overview** (`kernel/app/system`) still holds the handle for
**thirteen** verbs across **eight** families. That surface is not one family but a
**roll-up** of many, and it will have to ask each of them rather than the kernel.
