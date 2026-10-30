# W5.5a — the adapter holds the handle without naming it

## What the slice is

The **forty-third** slice of the port programme, and the **first** slice of
its **next phase**: the **adapter's** handle.

The phase before it was the handle itself: the application's own files stopped
naming it, and the ratchet is at **zero**. This phase is the other side of the
same door. The **adapter** — the control plane — held the handle too, and it
named it, in **twelve** production files.

## What the slice found

The adapter's twelve files held the handle **by type**:

```go
type Server struct {
	k             *runtime.Kernel
```

and forty-five binding files filled the families' ports from it by handing it
over:

```go
func (s *Server) toolsPorts() apptools.Ports { return apptools.PortsFromKernel(s.k) }
```

The shape worked, and the tree said so in its own rule — the binding files
already did **not** name the runtime, because "a binding that names the
runtime has to invent a helper whose only purpose is to spell the type". But
the twelve files that HELD the handle did name it ✗, and the archcheck had an
allowlist entry for exactly that edge:

```
adapter-bypass kernel/controlplane -> kernel/runtime
```

The measurement that moved:

| | before | after |
|---|---|---|
| control-plane production files that name the runtime | **12** | **0** |
| the archcheck import allowlist's edges | **2** | **1** |
| application-owned verbs the guard holds | **168** | **170** |
| the adapter's call sites on the handle | **0** | **0** |
| application files that name the handle **outside** a port file | **0** | **0** |

## What changed

### The handle is unnamed in the adapter

The `Server` holds it as `any`, the `DispatchCtx` carries it as `any`, the
request's app host carries it as `any` — and the **application** fills the
families' ports from it through a **bridge**:

```go
// kernel/app/binding/ports.go
func Fill[T any](handle any, fill func(*runtime.Kernel) T) T {
	k, ok := handle.(*runtime.Kernel)
	if !ok {
		var zero T
		return zero
	}
	return fill(k)
}
```

The bridge lives in the **application** layer, because the application owns the
families' ports. It is the hand-over and nothing more: the adapter gives the
handle to the bridge, the bridge gives it to the family, and the family fills
its **own** ports.

A handle that is not the daemon's own yields the family's **empty** ports —
and an empty port refuses rather than calling into nothing.

### Forty-five binding files read one line

```go
func (s *Server) toolsPorts() apptools.Ports { return binding.Fill(s.k, apptools.PortsFromKernel) }
```

### Five families gained an entry point

The adapter used to reach five capabilities through the handle itself. Each
family now fills its own:

| family | the entry point | what it is |
|---|---|---|
| the council | `PortsFromHandle(h any, baseDir string)` | the council's own ports |
| the files | `RunnerFromHandle(h any) Runner` | the tool runner the file family drives |
| the research | `EnginesFromHandle(h any, mint func() string)` | the daemon's research capability |
| the steer | `RunsFromHandle(h any) Runs` | the daemon's run controls |
| the runs | `PlanRunnerPortsFromHandle(h any) PlanRunnerPorts` | the daemon's plan machinery |

Each one refuses when the handle is not the daemon's own: the council has no
seats, the runner runs nothing, the research answers nobody, the run controls
steer nothing, and the plan cannot run.

### The plan runner takes ports

The plan runner used to take **four loose functions** — two of them the
daemon's own verbs — and the adapter named them (`k.LoopRunner`, `k.RunPlan`).
The runner now takes a **`PlanRunnerPorts`** struct, and the runs family fills
it ✓. The adapter's own verb rule ("the adapter calls no application-owned
verb on the kernel") holds without a special case ✓.

### The archcheck edge is gone

The allowlist's `adapter-bypass kernel/controlplane -> kernel/runtime` entry
is **deleted**, and the check **fails** when a listed edge no longer occurs ✓ —
so the debt cannot come back by accident.

## The rule

The runs family's rule grew by **two** exact names: `LoopRunner` and
`RunPlan`, the plan runner's own machinery. The plan runner is the runs
family's, so the daemon's plan loop and the daemon's plan verb are the runs
family's too — and a word rule cannot find either of them ("Run" **and**
"With" ✓).

The guard now holds **170** application-owned verbs, up from **168**.

## The tests

**8 permanent tests**:

- `TestTheBridgeFillsTheFamilysPortsFromTheHandle` — the hand-over really
  happens.
- `TestTheBridgeAnswersWithEmptyPortsForAHandleThatIsNotTheDaemons` — a
  foreign handle yields the family's **empty** value and the family is never
  called.
- `TestTheBridgeHandsTheFamilysRefusalThrough` — the bridge is a hand-over,
  not a decision.
- `TestTheCouncilPortsAreEmptyWithoutADaemon` — the council refuses.
- `TestTheRunnerRefusesWithoutADaemon` — the file family's runner refuses.
- `TestTheResearchEnginesRefuseWithoutADaemon` — the research engines
  refuse.
- `TestTheRunControlsRefuseWithoutADaemon` — the steer family's controls
  refuse.
- `TestThePlanRunnerPortsRefuseWithoutADaemon` — the plan machinery refuses.

The adapter's own ratchet moved with the slice:
`controlPlaneRuntimeNamingFiles` is **12 → 0**.

## The mutations

**14 mutations, 14 caught**, in two independent passes with **byte-identical**
logs (SHA-256 `C3985CB2B34660A8D2C8FD248E003267641FC94A6AB1242D171356905BCD17ED`):

- **M01** the bridge hands a foreign handle to the family.
- **M02** the bridge fills nothing.
- **M03** the adapter names the runtime again — the **ratchet** catches it.
- **M04** the ratchet constant moves.
- **M05** the runs family loses the plan verbs — the guard's broad half
  catches it.
- **M06** the rule goes blind — the rule's own test catches it.
- **M07** the council's ports panic without a daemon.
- **M08** the steer controls claim a daemon.
- **M09** the file runner claims a daemon.
- **M10** the research engines claim a daemon.
- **M11** the plan ports claim a daemon.
- **M12** the tenant's handle becomes the primary's — the tenant's own test
  catches it.
- **M13** the request reads the primary's tools — the tenant's own test
  catches it.
- **M14** the adapter-bypass edge comes back — the **archcheck** catches it.

**Nine mutations were rewritten**, and each rewrite was the mutation's fault,
not the tests':

- Four of them flipped `if !ok {` into `if false {` and left `k` and `ok`
  unused, so they did not compile (**BUILDERR**). They were rewritten to flip
  the condition instead — the same defect, and a compiling one.
- One of them filled the bridge with nothing and left the handle unused
  (**BUILDERR**) — rewritten to keep the binding and answer the empty value.
- One of them replaced a struct literal's opening and left the literal's
  fields behind, so the file did not parse (**BUILDERR**) — rewritten to
  replace the field itself.
- One of them reached for the primary's handle in a **free** function that
  has no server to ask (**BUILDERR**) — rewritten to ask the system host.
- One of them dropped the tenant handle and left the tenant unused
  (**BUILDERR**) — rewritten to keep the binding and drop the handle.

## What the slice does not do

- The adapter still **holds** the handle — it simply no longer **names** it.
  The next step in this phase is the composition root holding it and the
  adapter receiving the ports, so the handle leaves the adapter's memory too.
- The **selfrepair** cross-module edge is still in the archcheck allowlist: it
  is another slice's business, and the allowlist keeps it honest until then.
- The families' own ports did not change shape: the port programme's rule
  still holds — the family owns its ports, and the adapter binds them.
