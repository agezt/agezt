# W3.76b — the steering moves into the runs module

## What the slice is

The **tenth** slice of the runtime's dissolution and the **second** slice of the
**run engine** programme (W3.76). W3.76a took the durable **tickets**; this
one takes the **steering**.

The target architecture names the runs module's job in one line: *run registry,
**resume tickets**, **steering/interventions***. This slice takes the second
half of that line.

The family was split across three packages:

- `kernel/runtime/steer.go` (**180** non-blank lines) held the **surface**:
  `runControl` — the per-run steering state (paused, the lease, the pending
  single-step, the queued directives, the wake channel, the remembered
  idempotency results) — and the per-correlation lookup.
- `kernel/runtime/steer_ops.go` (**183** lines) held the **operations**: pause,
  resume, step, steer, observe, intervene, and the two event publishers.
- `kernel/intervention/intervention.go` (**69** lines) held the **grammar**: the
  wire vocabulary of an intervention (`Primitive`, `Request`, `Result`,
  `Normalize`).

## What the slice found

The steering family is the target architecture's own phrase, so it was not a
guess — but the shape still had to be measured, and the measurement settled
where the seam goes:

- the operations touch the daemon in exactly **four** ways: the **live run**
  (the steering surface of a running correlation), the **bus** (the two
  publishers), the **cancel** (an abort), and the **clock** (the lease);
- the surface touches nothing but the loop's own `agent.Steerer` contract;
- the grammar touches nothing at all.

That is a **port-shaped** family: four doors, and everything behind them is the
runs module's business. So the module took all three parts, and the daemon kept
the doors.

## What changed

### The module owns the family

```
kernel/modules/runs/steer/control.go       (193)   the per-run steering surface
kernel/modules/runs/steer/service.go      (235)   the operations, over four ports
kernel/modules/runs/intervention/         (74)    the wire grammar
```

The surface, with its state machine intact:

```go
// kernel/modules/runs/steer/control.go
type Control struct { /* mu, paused, pauseUntil, stepOnce, directives, wake, results */ }
func NewControl() *Control
func (rc *Control) Wait(ctx context.Context) error   // agent.Steerer
func (rc *Control) Drain() []agent.Directive          // agent.Steerer
func (rc *Control) Pause(until time.Time) bool
func (rc *Control) Resume() bool
func (rc *Control) Step()
func (rc *Control) Inject(directive string, note bool)
func (rc *Control) Snapshot() (paused bool, pending int, until time.Time)
func (rc *Control) ResultForKey(key string) (intervention.Result, bool)
func (rc *Control) RememberResult(key string, res intervention.Result)
```

The operations, over four ports:

```go
// kernel/modules/runs/steer/service.go
type Dependencies struct {
    Lookup  func(corr string) (*Control, bool)  // the live run
    Publish func(spec event.Spec)               // the bus
    Cancel  func(corr string) bool              // cancel a live run
    Now     func() time.Time                    // the clock (the lease)
}
type Service struct { deps Dependencies }
func NewService(deps Dependencies) *Service
func (s *Service) Pause(corr string) bool
func (s *Service) Resume(corr string) bool
func (s *Service) Step(corr string) bool
func (s *Service) Steer(corr, directive string, note bool) bool
func (s *Service) State(corr string) (paused bool, pending int, ok bool)
func (s *Service) Intervene(req intervention.Request) (intervention.Result, error)
```

The `Now` port is not decoration: the lease arithmetic (`lease_expires_unix`
and the expiry that releases a paused run) is the one place where the module
needed the host's clock, and a port is how it asks without reaching.

### The daemon keeps the doors

```go
// kernel/runtime/steer.go — 40 non-blank lines, all of them doors
func (k *Kernel) PauseRun(corr string) bool { return k.steerSvc.Pause(corr) }
func (k *Kernel) ResumeRun(corr string) bool { return k.steerSvc.Resume(corr) }
func (k *Kernel) StepRun(corr string) bool { return k.steerSvc.Step(corr) }
func (k *Kernel) SteerRun(corr, directive string, note bool) bool { ... }
func (k *Kernel) RunControlState(corr string) (bool, int, bool) { ... }
func (k *Kernel) InterveneRun(req intervention.Request) (intervention.Result, error) { ... }
```

and the composition root hands the module its four doors:

```go
// kernel/runtime/compose.go
k.steerSvc = steer.NewService(steer.Dependencies{
    Lookup:  func(corr string) (*steer.Control, bool) { rc := k.controlFor(corr); return rc, rc != nil },
    Publish: func(spec event.Spec) { _, _ = kbus.Publish(spec) },
    Cancel:  k.CancelRun,
    Now:     time.Now,
})
```

The verbs did not change their names, their signatures or their meaning — the
application layer's port (`kernel/app/steer.Runs`) and the operator UI are
untouched.

## The rule

**The module owns the operations; the host owns the doors.** W3.76a's rule was
*the host keeps the doors; the module keeps the rows and the rules that judge
them*. This slice adds the third half — **the module owns the operations
themselves**, and the host reaches them through named ports.

The family made that easy because it only needed four doors. A family that
touches the daemon in forty ways (the delegation) cannot move this way; it
needs a real port design first.

## What the slice cost

- `kernel/runtime/steer.go`: **180 → 40** lines (the doors).
- `kernel/runtime/steer_ops.go`: **deleted** (its content is the module's
  service).
- `kernel/intervention/` → `kernel/modules/runs/intervention/` (the grammar).
- **Thirteen files** changed outside the moved packages: the daemon's eight
  (`kernel/runtime/{steer,compose,resume,runexec_lifecycle,runtime_kernel,subagent_prep,subagent_tool,runtime_steer_test}.go`),
  the application layer's four (`kernel/app/steer/{ports,steer,ports_test,steer_test}.go`)
  and the archcheck's layer rule for `kernel/intervention`.
- **No new contract row**: the grammar was already a standalone package, and
  the module now hosts it.
- **No new face**: the daemon's verbs keep their names and their signatures.

## The tests

**15 new permanent tests** and **13 moved** with their packages:

- `kernel/modules/runs/steer/service_test.go` — **11 tests**: the service
  drives the control and emits the steering events, an unknown run is not
  steered, an empty directive is rejected, the interventions are applied (halt
  with a lease, redirect, adjust, query), an abort cancels through the port,
  the idempotency key is honoured, an unknown run is refused, a malformed
  intervention is rejected, a port-less service refuses everything, and a
  second pause emits no duplicate event.
- `kernel/modules/runs/steer/control_internal_test.go` — the **7** white-box
  concurrency tests of the surface, moved from `kernel/runtime`, plus **1** new
  one (resuming a run that is not paused is a no-op).
- `kernel/runtime/steer_internal_test.go` — **2 tests**: the **ratchet** (the
  daemon's steer hook must ask the module, not re-implement it) and the doors
  (a daemon without a steering service steers nothing).
- `kernel/runtime/runtime_steer_test.go` — **1 new test**: the protocolized
  intervention **through the daemon's own door** (an unknown run is refused, a
  live run is halted under a lease). The daemon's `InterveneRun` had no
  end-to-end coverage before this slice — the application layer's tests use a
  fake — and the mutation run found it.

The grammar's **6** tests moved with the package (`kernel/intervention` →
`kernel/modules/runs/intervention`).

## The mutations

**31 mutations, 31 caught**, in two independent passes with **byte-identical**
logs.

SHA-256: `C4FC447BF2F151FB136369D5C190155258F71366497EDF37C99E217E359AC476`

The calibration recorded **three** compile errors, and every one of them was
the mutation's fault — the mutation left the code in a state Go refuses:

- the drain that returned nothing lost its type (`use of untyped nil in
  assignment`);
- the pause that emitted no event left `rc` unused;
- the resume that ignored the pause state needed a wider anchor, because the
  same line opens two branches.

Each now keeps the code compilable **and** changes the behaviour: the drain
returns an empty (typed) slice, the pause still parks the run but publishes
nothing, and the resume mutation sits on the resume branch only.

What the 31 mutations cover:

- **M01**–**M11** the surface: the pause's idempotency check, the pause that
  does not park, the resume that ignores the state, the resume that does not
  clear it, the step that never releases, the inject that drops the directive,
  the drain that returns nothing, the drain that does not consume, the
  snapshot that hides the pause, the remembered result that is lost, and the
  wait that blocks a running run.
- **M12**–**M23** the operations: the pause that emits no event, the unknown
  run that answers true, the empty directive that is queued, the malformed
  intervention that is accepted, the abort's wrong state, the ignored
  idempotency key, the un-honoured lease, the redirect that is a soft note, the
  adjustment that is forceful, the steering event that loses its subject, the
  intervention event that loses its kind, and the query that applies something.
- **M24**–**M28** the grammar: the correlation that is not required, the lease
  default, the scope default, the unknown primitive that is accepted, and the
  adjustment that needs no directive.
- **M29**–**M31** the daemon's doors: the pause door that refuses, the
  intervention door that refuses, and the state door that refuses.

## The measurement that moved

| | before | after |
|---|---|---|
| the daemon's copies of the steering operations | **6** | **0** |
| `kernel/runtime/steer.go` (lines) | **180** | **40** |
| `kernel/runtime/steer_ops.go` (lines) | **183** | **gone** |
| the module's lines (the family) | **0** | **502** |
| `kernel/runtime` (upper level, lines) | **8,695** | **8,391** |
| `kernel/modules` (recursive, lines) | **25,835** | **26,346** |
| the kernel's verbs | **242** | **242** |
| the contract's new rows | — | **0** |
| the daemon's new verbs | — | **0** |

The daemon's copies of the steering operations fell from **6** to **0** — and a
**ratchet test** holds them there.

## The measurement that did NOT move

The family's own line count went **up**: **432 → 542** non-blank lines.

That is the same shape W3.74 and W3.75 explained — **moving a rule moves the
CONDITIONS, not the lines** — with one addition of this slice's own: the
module's lines are not only the moved lines, they are the moved lines **plus
the four ports** and the documentation of what they are.

The daemon's `steer.go` did shrink (**180 → 40**), because the daemon's copy of
the family was *only* the operations — the surface and the grammar lived
elsewhere. What the daemon kept is the wiring that feeds the module: the six
verbs, the live-run lookup, and the composition root's four ports.

## A correction to the recorded measurements

While this slice was measured, the four run-engine families were re-measured
from the tree. Three of the recorded figures reproduce **exactly** — the
delegation (**881**), the prompt (**578**), the run context (**562**) and the
loop config (**99**). The fifth did not:

> the **loop** was recorded as **1,415** non-blank lines. No subset of
> `kernel/agent` sums to 1,415 **and** contains the two files that are the
> loop (`agent_loop.go`, `agent_run.go`): an exhaustive search over the 2²⁰
> subsets found **1,040** subsets that sum to 1,415, and **none** of them
> contains both. `kernel/agent` measures **2,629** non-blank lines across 20
> files, and it never measured 1,415 at any commit in this branch's history.

So the loop is roughly **1.9×** the size the programme planned for it, and the
figure was carried from a measurement that does not reproduce against the
current tree. The number in W3.76a's evidence is corrected here rather than
quietly carried forward.

## What is left

The runs module now holds two families: the **durable tickets** (W3.76a) and
the **steering** (this slice).

Of the run engine's four measured families, the **run context** (**562** lines,
5 files, **zero** `*Kernel` coupling, 17 production call-site files) is the
next cheapest coherent move by the evidence — it is the run's own vocabulary,
and the prompt, the delegation and the loop all read it. The **delegation**
(**881**) is the most entangled (nine `*Kernel` methods, three bus
publications, a journal read); the **prompt** (**578**) reads memory, world and
skills through contract ports and owns no state; the **loop** is the biggest
and the most entangled with the contract layer (`kernel/contract/llm`,
`kernel/contract/toolapi`) — 40 production call-site files, including 15
provider files under `plugins/providers`.
