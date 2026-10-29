# W5.4ar — the runs family, and the two verbs that start a run

## What the slice is

The **thirty-first** slice of the port programme, and the **twenty-second** one
of its last phase: the handle itself.

The daemon's run capability — the two verbs that start a run under a
correlation, plainly and with a retry policy — is what two adapter surfaces
ask for: the roster's operator wake and the workboard's dispatch. The adapter
reached for the kernel's verbs in **four** places.

## What the slice found

| file | sites | whose runner |
|---|---|---|
| `kernel/controlplane/roster_escalation.go` | 2 | the daemon's own |
| `kernel/controlplane/workboard_dispatch.go` | 2 | the daemon's own |

| | before | after |
|---|---|---|
| call sites on the handle (the adapter) | **6** | **2** |
| distinct kernel verbs the adapter names | **4** | **2** |
| method values handed over | **4** | **4** |

This is the **biggest** single drop so far: four of the six remaining call
sites went in one slice.

The number of control-plane production files that **name** the kernel runtime
did **not** move: the runs family's binding was written without naming the
runtime, like every binding since W5.4v. The ratchet stays at **twelve**.

## What changed

The runs family's ports moved into the application
(`kernel/app/runs/ports.go`):

```go
type Ports struct {
	launch          func(ctx context.Context, corr, intent string) (string, error)
	launchWithRetry func(ctx context.Context, corr, intent string, pol agentapi.RetryPolicy) (string, error)
}

func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		launch: func(ctx context.Context, corr, intent string) (string, error) {
			return k.RunWith(ctx, corr, intent)
		},
		launchWithRetry: func(ctx context.Context, corr, intent string, pol agentapi.RetryPolicy) (string, error) {
			return k.RunWithRetry(ctx, corr, intent, pol)
		},
	}
}

func (p Ports) Launch(ctx context.Context, corr, intent string, retry *agentapi.RetryPolicy) (string, error) {
	if p.launch == nil || p.launchWithRetry == nil {
		return "", errNoRunner
	}
	if retry != nil {
		return p.launchWithRetry(ctx, corr, intent, *retry)
	}
	return p.launch(ctx, corr, intent)
}
```

and the adapter keeps the binding (`kernel/controlplane/runs_ports.go`) — which
does not name the runtime:

```go
func (s *Server) runPorts() appruns.Ports {
	return appruns.PortsFromKernel(s.k)
}
```

There is **no request-level binding**, and the reason is the shape of the read
path: the two surfaces that launch runs are the daemon's own, so they ask the
daemon's kernel. A binding nobody calls is dead code.

### The port's names are not the kernel's

The port answers `Launch` — never `RunWith`. A port that carried the name of
the verb it stands for would let the adapter name the kernel's verb through the
port's door.
`TestTheRunPortNamesAreNotTheKernelVerbs` holds that by reflection over the
port's own method set and the kernel's.

### The choice between plain and retrying stays with the caller

The two surfaces decide differently, and the port does not overrule them: the
roster asks for a plain run unless it was handed a policy, the workboard asks
for a plain run unless the policy really retries (`MaxAttempts > 1`). The port
only carries the decision the caller made.

## The rule

The runs family's verbs are the application's, and the rule is **by two whole
words**:

> a kernel verb whose name carries "Run" **and** "With" as **complete words**
 is the runs family's business.

The sweep takes in **two** verbs today: `RunWith` and `RunWithRetry`.

**One** verb carries both words and is **not** the runs family's:
`RunToolWithLookup` is the **tools** family's — it looks a tool up — and a rule
that swept it in would make the runs family claim another family's verb. The
exception is written out in the family's own rule, and
`TestTheRunsFamilyClaimsNoOtherFamilysVerbs` says so out loud.

The guard now holds **150** application-owned verbs.

## The tests

| test | holds |
|---|---|
| `TestTheRunPortsLaunchTheDaemonRuns` | the port starts the daemon's run, brings the answer back, and journals it under the correlation |
| `TestTheRunPortsLaunchARetryingRun` | a policy makes the run make as many attempts as the policy allows |
| `TestTheRunPortsLaunchAPlainRunWithoutRetrying` | no policy means **one** attempt, however badly the run went |
| `TestTheRunPortsRefuseForAHostWithNoKernel` | a host with no kernel is **refused**, and the refusal is a refusal |
| `TestTheRunPortNamesAreNotTheKernelVerbs` | no method of the port carries the name of a kernel verb |
| `TestTheAdapterBindsTheRunPortsToTheDaemon` | the daemon-level binding starts the **daemon's** run |
| `TestTheGuardNamesEveryRunVerbTheKernelHas` | the guard names every run verb the kernel has |
| `TestTheRunsFamilyClaimsNoOtherFamilysVerbs` | the runs family's list claims no other family's verb, **and not** `RunToolWithLookup` |

**8** permanent tests.

### The two paths tell each other apart

The plain path and the retrying path look the same when the run succeeds — and
a test that cannot tell them apart cannot hold either. The slice therefore
gives the provider **no scripted responses**: every attempt fails, and the
number of attempts is the difference. A plain run makes **one** attempt; a
retrying run under `MaxAttempts: 2` makes **two**.

The provider's own counter cannot count those attempts — the mock does not
advance its index when it is exhausted — so the count is taken at the
`OnRequest` hook, which fires before the exhaustion check.

## The mutations

**12** mutations, **12** caught, in two independent passes with **byte-identical**
logs (SHA-256 `0B776A753926D6E5BB70071B47DAB79EB5B541AEF95920C7DC0DF0C72CB34E2B`).

M01, M02 and M03 are the ones that bite: the plain launcher answers **nothing**,
a host with no kernel asks the kernel **anyway**, and the port hands over **no
runner**.

M04 gives the port a method that carries a kernel verb's own name, M05 empties
the daemon's binding, M06 puts the kernel back in the workboard's dispatch and
the **verb guard** catches it, M07 drops the **correlation** the run is
journaled under, M08 takes the family out of the guard, M09 sweeps the **tools'
verb** into the runs family's list and the sharp guard test catches it, M10
makes the binding name the runtime, M11 swallows the port's **refusal**, and
M12 carries the **retry policy away** — the retrying run then makes one attempt
instead of two, and the attempt count sees it.

## What the slice does not do

The handle stays, and the adapter still calls **2** kernel verbs on it and hands
over **4** method values: the reaper, the research model, the plan runner and
the loop runner.

The two remaining call sites are **not** the runs family's: `ReaperScan` (with
its method value) is the **reaper's**, and `DrainAndHalt` is the **daemon
lifecycle's**. They are two small families, and they are next.

The daemon's **overview** (`kernel/app/system`) still holds the handle for
**thirteen** verbs across **eight** families. That surface is not one family but a
**roll-up** of many, and it will have to ask each of them rather than the kernel.
