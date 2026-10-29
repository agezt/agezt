# W5.4at — the lifecycle family, and the last call on the handle

## What the slice is

The **thirty-third** slice of the port programme, and the **twenty-fourth** one
of its last phase: the handle itself.

The daemon's own lifecycle capability — drain the runs in flight, wait for
them, halt — is what the update flow asks for: an update that lands while runs
are still going would be applied under a daemon that is changing underneath it.
The adapter reached for the kernel's drain in **one** place.

**This slice empties the adapter's call sites on the handle.** After it, the
adapter names **no** kernel verb on the handle at all.

## What the slice found

| file | sites | whose daemon |
|---|---|---|
| `kernel/controlplane/update_control.go` | 1 | the daemon's own |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **1** | **0** |
| distinct kernel verbs the adapter names | **1** | **0** |
| method values handed over | **3** | **3** |

The number of control-plane production files that **name** the kernel runtime
did **not** move: the lifecycle family's binding was written without naming the
runtime, like every binding since W5.4v. The ratchet stays at **twelve**.

## What changed

The lifecycle family's ports moved into the application
(`kernel/app/lifecycle/ports.go`):

```go
type Ports struct {
	drain func(timeout time.Duration) (bool, int)
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{drain: func(timeout time.Duration) (bool, int) {
		return k.DrainAndHalt(timeout)
	}}
}

func (p Ports) Drain(timeout time.Duration) (bool, int, error) {
	if p.drain == nil {
		return false, 0, errNoDrain
	}
	timedOut, active := p.drain(timeout)
	return timedOut, active, nil
}
```

and the adapter keeps the binding (`kernel/controlplane/lifecycle_ports.go`) —
which does not name the runtime:

```go
func (s *Server) lifecyclePorts() applifecycle.Ports {
	return applifecycle.PortsFromKernel(s.k)
}
```

There is **no request-level binding**, and the reason is the shape of the read
path: the update flow drains the **daemon's own** runs, so it asks the daemon's
kernel. A binding nobody calls is dead code.

### The port's names are not the kernel's

The port answers `Drain` — never `DrainAndHalt`. A port that carried the name
of the verb it stands for would let the adapter name the kernel's verb through
the port's door.
`TestTheLifecyclePortNamesAreNotTheKernelVerbs` holds that by reflection over
the port's own method set and the kernel's.

### The refusal is a refusal, and the update listens to it

A host with no kernel holds no daemon to drain, and the port says so. The
adapter's update wiring maps that refusal to a **drain timeout**, and the
update flow's own contract is exact about what a timeout means: the staging
file is left in place, the binary is **not** swapped, the daemon is **not**
restarted. A drain that could not happen is therefore treated as a drain that
did not finish — the conservative reading, and the only one that keeps a new
binary from landing under a running daemon.

## A new family, and the gate that said why

The slice first put the port in `kernel/app/system` — the daemon's **overview**
— and the gate said no:

> `app_operations.go hands the kernel handle to the system application (s.k) —
> the family takes ports, not the handle it filled them from`

The rule fires the day a family declares its ports: from that moment the
adapter may not hand that family the handle. The daemon's overview does not
take a port today — it takes the **handle**, for **thirteen** verbs across
**eight** families, and the adapter hands it over in two places.

That is a real boundary, and the slice did not loosen it. Two things were
possible: give the overview a port that covers all thirteen verbs — which is
the **roll-up's** own work, the phase after this one — or put the drain in the
family that actually owns it.

The drain belongs to the daemon's **lifecycle**, and the daemon's overview is
not that: the overview *reports* what the daemon is doing, and the lifecycle
*does* what the daemon does to itself when it must stop. So the slice opened
`kernel/app/lifecycle` — a family of its own, with the daemon's own capability
in it — and the overview's thirteen verbs stayed exactly where they were, for
the next phase.

The rule did its job: it caught a boundary violation that the measurement could
not see, because the measurement counts *calls* and this was about *ownership*.

## The rule

The lifecycle family's verbs are the application's, and the rule is **by whole
word**:

> a kernel verb whose name carries "Drain" as a **complete word** is the
 lifecycle family's business.

The sweep takes in **one** verb today: `DrainAndHalt`.

The daemon's **other** lifecycle verbs — the halt, the suspend, the halted flag,
the runs in flight — are asked for by the daemon's own **overview**
(`kernel/app/system`), and they are named there and not here: this slice moves
the drain, and the overview's own asks are the phase after it. The rule says
so out loud rather than pretending the family is only one verb.

There is no exception list for this family: no other kernel verb carries the
word, and `TestTheLifecycleFamilyClaimsNoOtherFamilysVerbs` holds that the
lifecycle family's list claims nothing that belongs to somebody else — and that
it is a strict subset of the kernel's verbs, because a rule that only ever grows
is a rule that eventually guards the whole kernel, and a guard that guards the
whole kernel guards nothing.

The guard now holds **154** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheLifecyclePortsDrainTheDaemon` | the port really drains — and the daemon's own **halted flag** says so |
| `TestTheLifecyclePortsReportADrainThatGaveUp` | a run that would not stop makes the drain report its **timeout** |
| `TestTheLifecyclePortsRefuseForAHostWithNoKernel` | a host with no kernel is **refused**, and the refusal answers nothing |
| `TestTheLifecyclePortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsTheLifecyclePortsToTheDaemon` | the daemon-level binding drains the **daemon's** runs |
| `TestTheGuardNamesEveryLifecycleVerbTheKernelHas` | the guard names every lifecycle verb the kernel has |
| `TestTheLifecycleFamilyClaimsNoOtherFamilysVerbs` | the lifecycle family's list claims no other family's verb |

**7** permanent tests.

### The drain's hard case, observed

A drain with nothing in flight is a clean drain, and a clean drain proves
almost nothing. The second test therefore hangs a run on a provider that never
answers until the test lets it, and holds the drain's **timeout**: the drain
gave up before the run was done, and it says so.

## The mutations

**11** mutations, **11** caught, in two independent passes with **byte-identical**
logs (SHA-256 `C7B2810A247FB772748A08D5B383DB46DF9305702C7B8D6F3D39BED1DDE91C4A`).

M01, M02 and M03 are the ones that bite: the port holds **no** drain, a host
with no kernel asks the kernel **anyway**, and the port hands over **no drain**.

M04 gives the port a method that carries a kernel verb's own name, M05 empties
the daemon's binding, M06 puts the kernel back in the update flow and the
**verb guard** catches it, M07 swallows the port's **refusal**, M08 takes the
family out of the guard, M09 hands the family **another** family's verbs, M10
makes the binding name the runtime, and M11 makes the port answer the drain
**wrong** — the timeout it reported becomes the active count, and the test that
waits for a run that would not stop sees it.

## What the slice does not do

The adapter names **no** kernel verb on the handle any more. It still hands
over **3** method values — the research model, the loop runner and the plan
runner — and those are the last of the handle's reach that the measurement can
see.

The **application** layer still holds the kernel in a dozen files: the daemon's
own overview (`kernel/app/system`, thirteen verbs across eight families), the
runs, the overseer, the introspector, the execution profile, the guardians, the
config tool, the memory and the tools. That is the phase after this one, and
it is the same programme's next step: the handle itself, on the application's
side.
