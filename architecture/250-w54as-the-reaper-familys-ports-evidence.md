# W5.4as — the reaper family, and the module the daemon may not have

## What the slice is

The **thirty-second** slice of the port programme, and the **twenty-third** one
of its last phase: the handle itself.

The reaper's scan is the daemon's own — the read-only pass that finds dead,
degraded, misconfigured and routing-troubled agents and stale artifacts, the
detail behind the operator's report. The adapter reached for the kernel's scan
in **two** places: the reaper's own operations, and the roster's status
collection.

## What the slice found

| file | sites | whose scan |
|---|---|---|
| `kernel/controlplane/app_operations.go` | 1 | the daemon's own |
| `kernel/controlplane/roster_status_views.go` | 1 | the daemon's own |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **2** | **1** |
| distinct kernel verbs the adapter names | **2** | **1** |
| method values handed over | **4** | **3** |

The reaper's scan was also a **method value** — the kernel's own method handed
over as a function — and it is a method value no more: the port's own `Scan`
is what the adapter hands over now.

The number of control-plane production files that **name** the kernel runtime
did **not** move: the reaper family's binding was written without naming the
runtime, like every binding since W5.4v. The ratchet stays at **twelve**.

## What changed

The reaper family's ports moved into the application
(`kernel/app/reaper/ports.go`):

```go
type Ports struct {
	scan func(agentIdleCutoffMs, artifactStaleCutoffMs int64) api.ReaperReport
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{scan: func(agentIdleCutoffMs, artifactStaleCutoffMs int64) api.ReaperReport {
		return k.ReaperScan(agentIdleCutoffMs, artifactStaleCutoffMs)
	}}
}

func (p Ports) Scan(agentIdleCutoffMs, artifactStaleCutoffMs int64) api.ReaperReport {
	if p.scan == nil {
		return api.ReaperReport{}
	}
	return p.scan(agentIdleCutoffMs, artifactStaleCutoffMs)
}
```

and the adapter keeps the binding (`kernel/controlplane/reaper_ports.go`) — which
does not name the runtime:

```go
func (s *Server) reaperPorts() appreaper.Ports {
	return appreaper.PortsFromKernel(s.k)
}
```

There is **no request-level binding**, and the reason is the shape of the read
path: the two surfaces that scan are the daemon's own, so they ask the
daemon's kernel. A binding nobody calls is dead code.

### The port's names are not the kernel's

The port answers `Scan` — never `ReaperScan`. A port that carried the name of
the verb it stands for would let the adapter name the kernel's verb through the
port's door.
`TestTheReaperPortNamesAreNotTheKernelVerbs` holds that by reflection over the
port's own method set and the kernel's.

### The empty report is the kernel's own answer

The reaper is a **module** the composition root injects, and a daemon that was
given none holds no scanner. The kernel answers such a daemon with an **empty
report** — no dead agents, no stale artifacts — and the port answers the same
way for a host with no kernel at all. The port does not invent a refusal here:
the scan's own contract is "read-only, and empty means nothing found".

## The rule

The reaper family's verbs are the application's, and the rule is **by whole
word**:

> a kernel verb whose name carries "Reaper" as a **complete word** is the
 reaper family's business.

The sweep takes in **three** verbs today: `ReaperScan` (the scan itself),
`ReaperPorts` (the host's own facts the scanner needs) and `SetReaperScanner`
(the setter that injects the scanner).

There is no exception list for this family: no other kernel verb carries the
word, and `TestTheReaperFamilyClaimsNoOtherFamilysVerbs` holds that the reaper
family's list claims nothing that belongs to somebody else — and that it is a
strict subset of the kernel's verbs, because a rule that only ever grows is a
rule that eventually guards the whole kernel, and a guard that guards the whole
kernel guards nothing.

The guard now holds **153** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheReaperPortsReachTheKernelScan` | the port hands over the daemon's **own** scan, and the two cutoffs arrive in order |
| `TestTheReaperPortsAnswerEmptyWhenTheDaemonHasNoReaper` | a daemon with no reaper module answers an **empty** report |
| `TestTheReaperPortsSurviveAHostWithNoKernel` | a host with no kernel answers an empty report |
| `TestTheReaperPortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsTheReaperPortsToTheDaemon` | the daemon-level binding answers for the **daemon's** scan |
| `TestTheAdapterBindsNoReaperToADaemonWithoutOne` | a daemon with no reaper gets an empty report |
| `TestTheGuardNamesEveryReaperVerbTheKernelHas` | the guard names every reaper verb the kernel has |
| `TestTheReaperFamilyClaimsNoOtherFamilysVerbs` | the reaper family's list claims no other family's verb |

**8** permanent tests.

### Telling the two halves apart

A daemon with no reaper module answers an **empty** report, and a daemon with a
reaper module over an empty fleet answers an empty report too. The two are
indistinguishable through the report alone — so the test gives the daemon a
**fake scanner** that answers with a known dead agent, and the filled half is
the one that carries it.

The fake also remembers the two cutoffs it was asked about, because they are
two different things — the agents' idle cutoff and the artifacts' stale
cutoff — and a port that swapped them would still answer a report.

## The mutations

**11** mutations, **11** caught, in two independent passes with **byte-identical**
logs (SHA-256 `64CA924864EDD3D135799F8973C3C866122C0C130858D19A7CDEAD0E4916D62A`).

M01, M02 and M03 are the ones that bite: the port holds **no** scan, a host
with no kernel asks the kernel **anyway**, and the port hands over **no scan**.

M04 gives the port a method that carries a kernel verb's own name, M05 empties
the daemon's binding, M06 puts the kernel back in the reaper's operations and
the **verb guard** catches it, M07 puts the kernel back in the roster's status
and the **verb guard** catches it too, M08 takes the family out of the guard,
M09 hands the family **another** family's verbs, M10 makes the binding name the
runtime, and M11 **swaps the two cutoffs** — the fake remembers them, and the
test sees the swap.

## What the slice does not do

The handle stays, and the adapter still calls **1** kernel verb on it — the
daemon lifecycle's drain — and hands over **3** method values: the research
model, the loop runner and the plan runner.

The **application** layer reaches for the same reaper scan in two more places
(`kernel/app/overseer`, `kernel/app/introspect`), but both of those packages
hold the kernel for many other reasons as well — the roster's verbs, the
provider's chains, the schedules, the standing orders, the journal. Those are
the **application's** own handle, and they are the phase after this one.

The daemon's **overview** (`kernel/app/system`) still holds the handle for
**thirteen** verbs across **eight** families. That surface is not one family but a
**roll-up** of many, and it will have to ask each of them rather than the kernel.
