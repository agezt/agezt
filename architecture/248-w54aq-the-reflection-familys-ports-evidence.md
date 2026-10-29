# W5.4aq — the reflection family, and the engine the daemon always has

## What the slice is

The **thirtieth** slice of the port programme, and the **twenty-first** one of
its last phase: the handle itself.

The reflection engine is the daemon's own — the pass that folds the journal
into observations, applies the world model's decay and derives advisory
proposals, so the decay is explainable through `agt why`. The adapter reached
for the kernel's engine in **one** place.

## What the slice found

| file | sites | whose engine |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 1 | the daemon's own |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **7** | **6** |
| distinct kernel verbs the adapter names | **5** | **4** |
| method values handed over | **4** | **4** |

The **application** layer held no engine handle either: `kernel/app/reflection`
already took the engine as an argument, and it still does — the port only
changed **where** the value comes from.

The number of control-plane production files that **name** the kernel runtime
did **not** move: the reflection family's binding was written without naming
the runtime, like every binding since W5.4v. The ratchet stays at **twelve**.

## What changed

The reflection family's ports moved into the application
(`kernel/app/reflection/ports.go`):

```go
type Ports struct {
	engine func() Engine
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{engine: func() Engine { return k.Reflect() }}
}

func (p Ports) Engine() Engine {
	if p.engine == nil {
		return nil
	}
	return p.engine()
}
```

and the adapter keeps the binding
(`kernel/controlplane/reflection_ports.go`) — which does not name the runtime:

```go
func (s *Server) reflectionPorts() appreflection.Ports {
	return appreflection.PortsFromKernel(s.k)
}
```

There is **no request-level binding**, and the reason is the shape of the read
path: the reflection surface drives the **daemon's own** engine, so it asks the
daemon's kernel. A binding nobody calls is dead code.

### The port's names are not the kernel's

The port answers `Engine` — never `Reflect`. A port that carried the name of
the verb it stands for would let the adapter name the kernel's verb through the
port's door.
`TestTheReflectionPortNamesAreNotTheKernelVerbs` holds that by reflection over
the port's own method set and the kernel's.

## The rule

The reflection family's verbs are the application's, and the rule is **by whole
word**:

> a kernel verb whose name carries "Reflect" as a **complete word** is the
 reflection family's business.

The sweep takes in **one** verb today: `Reflect`.

There is no exception list for this family: no other kernel verb carries the
word, and `TestTheReflectionFamilyClaimsNoOtherFamilysVerbs` holds that the
reflection family's list claims nothing that belongs to somebody else — and
that it is a strict subset of the kernel's verbs, because a rule that only ever
grows is a rule that eventually guards the whole kernel, and a guard that guards
the whole kernel guards nothing.

The guard now holds **148** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheReflectionPortsReachTheKernelEngine` | the port hands over the daemon's own engine |
| `TestTheReflectionPortsSurviveAHostWithNoKernel` | a host with no kernel hands over no engine |
| `TestTheReflectionPortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsTheReflectionPortsToTheDaemon` | the daemon-level binding answers for the **daemon's** engine |
| `TestTheGuardNamesEveryReflectionVerbTheKernelHas` | the guard names every reflection verb the kernel has |
| `TestTheReflectionFamilyClaimsNoOtherFamilysVerbs` | the reflection family's list claims no other family's verb |

**6** permanent tests.

### One half, and why there is only one

The reflection engine is **not** a daemon's file: the composition root always
builds one (`reflect.New(j, wgraph, kbus, reflect.Config{})`), so a daemon that
was given a kernel always holds an engine. There is no "daemon without an
engine" to test against — the case is unreachable in production.

That is the difference from the lake, the seat catalog and the taste library,
where the daemon's file may be missing and the port's empty half is a real
state. Here the port's absent case is only the host that holds **no kernel at
all**, and the test says so in words rather than pretending to test an
unreachable half.

## The mutations

**11** mutations, **11** caught, in two independent passes with **byte-identical**
logs (SHA-256 `49826F05477126705861D0FCF65B302A7C59812C9CEA65E054F00A98037BD663`).

M01, M02 and M03 are the ones that bite: the port holds **no** engine, a host
with no kernel asks the kernel **anyway**, and the port hands over **nothing**.

M04 gives the port a method that carries a kernel verb's own name, M05 empties
the daemon's binding, M06 puts the kernel back in the operations and the **verb
guard** catches it, M07 drops the pass's own "reflect-" correlation prefix and
the surface over the socket catches it, M08 takes the family out of the guard,
M09 hands the family **another** family's verbs, M10 makes the binding name the
runtime, and M11 makes the surface stop reading the engine's latest report.

### One mutation that did not compile, and why that is the mutation's fault

M07 first replaced the mint at the call site with a function that returns an
empty string. That did not compile: the `ulid` import became unused, and a
mutation that does not compile proves nothing about the test. The mutation was
rewritten to drop the pass's own "reflect-" prefix inside the family instead —
where the surface test's correlation rule sees it.

**BUILDERR is the mutation's fault**, exactly as a surviving mutation is.

## What the slice does not do

The handle stays, and the adapter still calls **6** kernel verbs on it and hands
over **4** method values: the run decorators, the reaper, the research model
and the plan runner.

The **runs family** still holds the kernel handle — for the run decorators, the
roster, the warden and the catalog. It is the next family, and it is the
largest one left.

The daemon's **overview** (`kernel/app/system`) still holds the handle for
**thirteen** verbs across **eight** families. That surface is not one family but a
**roll-up** of many, and it will have to ask each of them rather than the kernel.
