# W5.4ao — the seats family, and the plural word the splitter reads as one

## What the slice is

The **twenty-eighth** slice of the port programme, and the **nineteenth** one
of its last phase: the handle itself.

The seat catalog is the daemon's own file — the execution seats a workboard
task can be dispatched under, the seeded built-ins and the operator's own
custom ones, each pinning an isolation profile, a model chain and an optional
tool allowlist. The adapter reached for the kernel's catalog in **one** place.

## What the slice found

| file | sites | whose catalog |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 1 | the daemon's own |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **9** | **8** |
| distinct kernel verbs the adapter names | **7** | **6** |
| method values handed over | **4** | **4** |

The **application** layer held no seat handle either: `kernel/app/seats`
already took the catalog as an argument, and it still does — the port only
changed **where** the value comes from.

The number of control-plane production files that **name** the kernel runtime
did **not** move: the seat family's binding was written without naming the
runtime, like every binding since W5.4v, and the file that asked the kernel
(`app_operations.go`) already named it for other reasons. The ratchet stays at
**twelve**.

## What changed

The seats family's ports moved into the application
(`kernel/app/seats/ports.go`):

```go
type Ports struct {
	store func() Store
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{store: func() Store { return k.Seats() }}
}

func (p Ports) Store() Store {
	if p.store == nil {
		return nil
	}
	return p.store()
}
```

and the adapter keeps the binding (`kernel/controlplane/seats_ports.go`) — which
does not name the runtime:

```go
func (s *Server) seatPorts() appseats.Ports {
	return appseats.PortsFromKernel(s.k)
}
```

There is **no request-level binding**, and the reason is the shape of the read
path: the seat surface inspects the **daemon's own** catalog, so it asks the
daemon's kernel. A binding nobody calls is dead code.

### The port's names are not the kernel's

The port answers `Store` — never `Seats`. A port that carried the name of the
verb it stands for would let the adapter name the kernel's verb through the
port's door.
`TestTheSeatPortNamesAreNotTheKernelVerbs` holds that by reflection over the
port's own method set and the kernel's.

## The rule

The seats' verbs are the application's, and the rule is **by whole word**:

> a kernel verb whose name carries "Seats" as a **complete word** is the seats
 family's business.

The sweep takes in **one** verb today: `Seats`.

### The word is the plural, and that is not a detail

The word in the rule is **"Seats"**, not "Seat". The camel-case splitter reads
a trailing plural "s" as part of the word it belongs to — `Artifacts` is one
word, `Seats` is one word — so a rule written as "Seat" matches **nothing at
all**, and the guard's helper fails on the very day it is written. The slice
found this the hard way: the rule was written as "Seat" first, and the guard
test failed with *"no seat verbs found"* before the rule was corrected.

There is no exception list for this family: no other kernel verb carries the
word, and `TestTheSeatFamilyClaimsNoOtherFamilysVerbs` holds that the seats'
list claims nothing that belongs to somebody else — and that it is a strict
subset of the kernel's verbs, because a rule that only ever grows is a rule
that eventually guards the whole kernel, and a guard that guards the whole
kernel guards nothing.

The guard now holds **146** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheSeatPortsReachTheKernelStore` | the port hands over the daemon's own catalog |
| `TestTheSeatPortsHoldNoStoreWhenTheDaemonHasNoCatalog` | a daemon with no catalog hands over none |
| `TestTheSeatPortsSurviveAHostWithNoKernel` | a host with no kernel hands over no catalog |
| `TestTheSeatPortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsTheSeatPortsToTheDaemon` | the daemon-level binding answers for the **daemon's** catalog |
| `TestTheAdapterBindsNoSeatStoreToADaemonWithoutACatalog` | a daemon with no catalog gets none |
| `TestTheGuardNamesEverySeatVerbTheKernelHas` | the guard names every seat verb the kernel has |
| `TestTheSeatFamilyClaimsNoOtherFamilysVerbs` | the seats' list claims no other family's verb |

**8** permanent tests.

### Both halves, or the test proves nothing

The catalog is the daemon's **file**: a daemon that was given none holds none,
so a test that only builds a library-less daemon cannot tell a working port
from a port that always answers "no catalog". The slice therefore holds
**both** halves — a daemon that was given a catalog (the port must hand over
exactly that one) and a daemon that was given none (the port must hand over
nothing) — on the port side and on the binding side.

This is not a formality: it is what turns M01 and M03 from invisible mutations
into caught ones.

## The mutations

**11** mutations, **11** caught, in two independent passes with **byte-identical**
logs (SHA-256 `880B91EB58B8CD3A63F4FD441707720BEA535D6BB9C599844E86912D7EDFCDD6`).

M01, M02 and M03 are the ones that bite: the port holds **no** store, a host
with no kernel asks the kernel **anyway**, and the port hands over **nothing**.

M04 gives the port a method that carries a kernel verb's own name, M05 empties
the daemon's binding, M06 puts the kernel back in the operations and the **verb
guard** catches it, M07 builds the surface from **nothing** and the seat list
over the socket refuses, M08 takes the family out of the guard, M09 hands the
family **another** family's verbs, M10 makes the binding name the runtime, and
M11 writes the rule as the **singular** — "Seat" — so it matches nothing and the
guard goes blind.

### Two mutations that were rewritten, and why

The slice did not accept its first two drafts of two mutations, because a
mutation that survives for a **good** reason is still a mutation that does not
teach the test anything:

- M09 first widened the guard **test's own** rule to claim the provider
  family's verbs. It **survived** — and not because the test was weak: the
  provider verbs really are guarded, so the widened rule was satisfied by the
  truth. The mutation was measuring nothing. It was rewritten to widen the
  **helper's** rule, which is the rule the guard actually runs, and the sharp
  guard test catches that one.
- M11 first widened the same test-side rule in the other direction. It
  **survived** for the same reason. It was rewritten to write the rule as the
  **singular**, which is the trap this slice actually fell into.

A mutation that survives is either a missing test or a mutation that measures
nothing. Both are the mutation's fault, and both are the slice's fault to fix.

## What the slice does not do

The handle stays, and the adapter still calls **8** kernel verbs on it and hands
over **4** method values: the run decorators, the reaper, the research model
and the plan runner.

The **runs family** still holds the kernel handle — for the run decorators, the
roster, the warden and the catalog. It is the next family, and it is the
largest one left.

The daemon's **overview** (`kernel/app/system`) still holds the handle for
**thirteen** verbs across **eight** families. That surface is not one family but a
**roll-up** of many, and it will have to ask each of them rather than the kernel.
