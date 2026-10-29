# W5.4ap — the data family, and the lake the daemon may not have

## What the slice is

The **twenty-ninth** slice of the port programme, and the **twentieth** one of
its last phase: the handle itself.

The personal data lake is the daemon's own file — the collections the agents
build and share, each with a schema and its records. The adapter reached for
the kernel's lake in **one** place.

## What the slice found

| file | sites | whose lake |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 1 | the daemon's own |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **8** | **7** |
| distinct kernel verbs the adapter names | **6** | **5** |
| method values handed over | **4** | **4** |

The **application** layer held no lake handle either: `kernel/app/data` already
took the lake as an argument, and it still does — the port only changed **where**
the value comes from.

The number of control-plane production files that **name** the kernel runtime
did **not** move: the data family's binding was written without naming the
runtime, like every binding since W5.4v. The ratchet stays at **twelve**.

## What changed

The data family's ports moved into the application
(`kernel/app/data/ports.go`):

```go
type Ports struct {
	lake func() Lake
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{lake: func() Lake { return k.DataLake() }}
}

func (p Ports) Lake() Lake {
	if p.lake == nil {
		return nil
	}
	return p.lake()
}
```

and the adapter keeps the binding (`kernel/controlplane/data_ports.go`) — which
does not name the runtime:

```go
func (s *Server) dataPorts() appdata.Ports {
	return appdata.PortsFromKernel(s.k)
}
```

There is **no request-level binding**, and the reason is the shape of the read
path: the data surface inspects the **daemon's own** lake, so it asks the
daemon's kernel. A binding nobody calls is dead code.

The two branches the old call site carried — "a lake, or no lake at all" — are
gone: the port answers the same question in both cases, and the service refuses
when the answer is "none".

### The port's names are not the kernel's

The port answers `Lake` — never `DataLake`. A port that carried the name of the
verb it stands for would let the adapter name the kernel's verb through the
port's door.
`TestTheDataPortNamesAreNotTheKernelVerbs` holds that by reflection over the
port's own method set and the kernel's.

## The rule

The data family's verbs are the application's, and the rule is **by two whole
words**:

> a kernel verb whose name carries "Data" **and** "Lake" as **complete words**
 is the data family's business.

The lake's own name is one word to the camel-case splitter — "DataLake" reads
as "Data" + "Lake" — and the rule says so out loud rather than trusting the
shape.

The sweep takes in **one** verb today: `DataLake`.

There is no exception list for this family: no other kernel verb carries both
words, and `TestTheDataFamilyClaimsNoOtherFamilysVerbs` holds that the data
family's list claims nothing that belongs to somebody else — and that it is a
strict subset of the kernel's verbs, because a rule that only ever grows is a
rule that eventually guards the whole kernel, and a guard that guards the whole
kernel guards nothing.

The guard now holds **147** application-owned verbs.

### The test's rule is a second reading, not the helper's

The guard test states the rule **again**, in its own words, with its own
`hasWord` helper — and it does **not** ask the guard's helper what the guard
owns. A test that asks the helper cannot see the helper reach for something
that is not its own: the expectation and the thing under test would be the same
sentence.

## The tests

| test | holds |
|---|---|
| `TestTheDataPortsReachTheKernelLake` | the port hands over the daemon's own lake |
| `TestTheDataPortsHoldNoLakeWhenTheDaemonHasNoLake` | a daemon with no lake hands over none |
| `TestTheDataPortsSurviveAHostWithNoKernel` | a host with no kernel hands over no lake |
| `TestTheDataPortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsTheDataPortsToTheDaemon` | the daemon-level binding answers for the **daemon's** lake |
| `TestTheAdapterBindsNoLakeToADaemonWithoutOne` | a daemon with no lake gets none |
| `TestTheGuardNamesEveryDataVerbTheKernelHas` | the guard names every data verb the kernel has |
| `TestTheDataFamilyClaimsNoOtherFamilysVerbs` | the data family's list claims no other family's verb |
| `TestTheDataSurfaceRefusesWhenTheDaemonHoldsNoLake` | the surface **refuses** a daemon with no lake, and says why |

**9** permanent tests.

### Both halves, and the refusal

The lake is the daemon's **file**: a daemon that was given none holds none, so
a test that only builds a library-less daemon cannot tell a working port from
a port that always answers "no lake". The port test and the binding test
therefore hold **both** halves.

The third half is the one the slice added on purpose: the **refusal**. The
surface must not answer a daemon with no lake with an empty collection list —
that reads like "there is no data", and the operator is told the lake is
unavailable instead. `TestTheDataSurfaceRefusesWhenTheDaemonHoldsNoLake` drives
the surface over the socket and holds both the refusal and its reason.

## The mutations

**11** mutations, **11** caught, in two independent passes with **byte-identical**
logs (SHA-256 `51D1AB73439D5C67BD4E8E8CC059FC5AD7EAC963D6CD4375C4120053FA9D10FD`).

M01, M02 and M03 are the ones that bite: the port holds **no** lake, a host
with no kernel asks the kernel **anyway**, and the port hands over **nothing**.

M04 gives the port a method that carries a kernel verb's own name, M05 empties
the daemon's binding, M06 puts the kernel back in the operations and the **verb
guard** catches it, M07 builds the surface from **nothing** and the data
operations over the socket refuse, M08 takes the family out of the guard, M09
makes the rule reach for **another family's word** and the sharp guard test
catches it, M10 makes the binding name the runtime, and M11 makes the surface
answer an **empty list** where it owes a refusal.

### One mutation that was rewritten, and why

M09 first widened the rule's word check with the provider family's word. It
**survived** — and the reason is the rule's own shape: the data family's rule
is "Data **and** Lake", so a rule that also accepts "Provider" still demands
"Data" as well, and no other verb in the kernel carries it. The mutation
measured nothing. It was rewritten to make the rule reach for the provider's
word **on its own**, and the sharp guard test catches that one.

A mutation that survives for a good reason is still a mutation that does not
teach the test anything — the same lesson the seats slice learned twice.

## What the slice does not do

The handle stays, and the adapter still calls **7** kernel verbs on it and hands
over **4** method values: the run decorators, the reaper, the research model
and the plan runner.

The **runs family** still holds the kernel handle — for the run decorators, the
roster, the warden and the catalog. It is the next family, and it is the
largest one left.

The daemon's **overview** (`kernel/app/system`) still holds the handle for
**thirteen** verbs across **eight** families. That surface is not one family but a
**roll-up** of many, and it will have to ask each of them rather than the kernel.
