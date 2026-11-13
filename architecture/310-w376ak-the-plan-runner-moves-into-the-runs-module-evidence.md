# W3.76ak — the plan runner moves into the runs module

**Slice:** W3.76ak (the forty-fifth of the runtime's dissolution)
**Base:** `cd2567b1` (the W3.76aj merge)
**Date:** 2026-11-13

---

## 1. The family

A **plan** is the runs module's own work unit: the operator submits a plan, the
scheduler executes its nodes, and the plan is a **live run** while it runs —
the module's registry holds its cancel, the halt latch refuses new plans, and
the plan's id is its correlation.

```go
result, err := planSvc.Run(ctx, pl, planID)
```

Both halves of that were already the module's:

- the **SCHEDULER** that executes a plan is the runs module's family — the
  architecture gate places `kernel/scheduler` in the runs module
  (`tools/archcheck/layers.json`: `"pattern": "kernel/scheduler", "layer": 3,
  "module": "runs"`);
- the plan's **registration** is over the module's own live-run **registry**
  (`kernel/modules/runs/registry`, the module's store since W3.76d).

What stayed in the daemon was the machinery's **second handle** and two verbs
in the daemon's own words:

```go
// kernel/runtime/accessors_misc.go
func (k *Kernel) LoopRunner() scheduler.LoopRunner { … }
func (k *Kernel) RunPlan(ctx context.Context, plan scheduler.Plan, planID string) (*scheduler.PlanResult, error) {
	k.registry.Lock()
	if k.registry.Halted() { … }
	k.registry.Register(planID, cancel)
	k.registry.Unlock()
	defer func() { k.registry.Unregister(planID); cancel() }()
	return k.scheduler.Run(runCtx, plan, planID)
}
```

**The daemon held the module's registry and the module's scheduler, and
spoke for the family in words of its own.** The application above learned two
names the module already had.

## 2. What moved

The plan runner is the module's own service now — **`kernel/modules/runs/plan`**:

```go
type Service struct { registry *registry.Registry; scheduler *scheduler.Executor; runner Runner }

func (s *Service) Run(ctx context.Context, pl scheduler.Plan, planID string) (*scheduler.PlanResult, error)
func (s *Service) LoopRunner() scheduler.LoopRunner
```

Its collaborators are all the module's own objects: the **registry** (the
plan's registration and the halt latch), the **scheduler** (the plan's
execution) and the **run engine** (the loop a node runs), and it reaches the
daemon for **nothing**.

The daemon keeps **one** thing over the family: the module's **FACE**.

```go
func (k *Kernel) PlanService() api.PlanService { return k.plan }
```

`api.PlanService` is the module's own port (`kernel/modules/runs/api/plan.go`),
in the module's own names — `Run` and `LoopRunner` — because the module's
scheduler already carries them. The **application** binds the plan runner's
ports from that face (`kernel/app/runs/ports.go`):

```go
return PlanRunnerPorts{
	LoopRunner: k.PlanService().LoopRunner,
	RunPlan:    k.PlanService().Run,
}
```

and the **control plane's** runs family now names the face in the place the
two verbs held (`PlanService`, next to `RunnerService`).

The halt refusal is the module's own sentinel now: `state.ErrHalted` — the
daemon's alias points at the same value, so a halted daemon refuses a plan
exactly as it refuses a run.

## 3. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 107 | **106** |
| the daemon's production lines | 5,213 | **5,213** |
| the daemon's production files | 52 | **52** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the plan runner's package | — | **1 file / 100 lines** |
| the runs module's api package | 3 files / 122 lines | **4 files / 152 lines** |
| the scheduler's package | 5 files / 768 lines | **5 files / 768 lines** |

**One verb off the daemon's surface, and its lines unchanged** — and that is
the honest number: the daemon's `accessors_misc.go` lost **20** lines (the
plan's body and the loop hook) while the module's face and the composition
root's wiring note (**19** lines across two files) came in. The body did not
shrink; it **moved** — 100 lines now live in `kernel/modules/runs/plan`, and
the module's api package carries the face.

The **scheduler did not move**: it is still `kernel/scheduler`, and the slice
moved the plan runner's **lifecycle** onto it, not the package itself. The
scheduler's path home is the same module's directory; that is a **path move**
(a separate slice), and this one is not it.

## 4. The ratchets — and the probe

**The daemon's side** (`kernel/runtime/plan_ratchet_test.go`, new): the daemon
holds **no** verb per plan operation (`RunPlan`, `LoopRunner`), registers **no**
plan of its own (`k.registry.Register` / `k.registry.Unregister`), drives **no**
scheduler (`k.scheduler.Run`), and hands over the module's face — the
composition root builds the plan runner over the module's own parts.

**The module's side** (`kernel/modules/runs/plan/plan_ratchet_test.go`, new):
the service holds the body (the registration, the unregistration, the halt
check, the scheduler call and the module's sentinel), answers the module's own
port (`api.PlanService`), and never names the daemon.

The daemon's own surface ceiling followed the floor again: **107 → 106** (220
→ 201 → 194 → 179 → 173 → 166 → 164 → 156 → 152 → 148 → 143 → 135 → 129 →
125 → 121 → 118 → 116 → 114 → 111 → 108 → 107 → 106), and the two retired
names joined the retired-verb list.

**The probe** put the world back before this slice — the daemon's two verbs
with their **real** bodies and the application asking the daemon again: the
retired-verb ratchet, the surface ceiling (107 > 106), the daemon's three plan
ratchets, the re-aimed registry ratchet (the daemon registers a plan again)
and the accessor-surface ratchet (the two verbs are on the kernel again) all
went red at once — **seven** ratchets in one shot, and nothing else broke; the
probe was reverted and the suites went green again.

## 5. The guard

Two earlier ratchets were **re-aimed**, not replaced:

- the W3.76ae registry ratchet read `accessors_misc.go` and demanded that the
  daemon register **through** the module's store — it named the plan's
  registration as the daemon's last one. The plan's registration is the
  module's now (W3.76ak), so the ratchet demands the **absence** instead: the
  daemon registers nothing at all, not a run and not a plan;
- the accessor-surface ratchet in `submanager_test.go` pinned `LoopRunner` and
  `RunPlan` as part of the kernel's surface. They left the daemon, so the
  ratchet names the module's face (`PlanService`) and pins their **absence** —
  a door that comes back comes back with a second spelling of the family.

The **control plane's** runs family names `PlanService` in the place the two
verbs held: a verb the kernel has and the guard does not name is a verb nobody
guards.

## 6. Verification

- the compiler (`go build ./...`, `go vet ./...`) and `gofmt`;
- the daemon's tests, the plan runner's tests, the application's runs tests and
  the control plane's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 7. What is left

The daemon holds **106** public verbs and **5,213** production lines; **forty**
application packages still name it; the run engine's host port carries **23**
entries.

What is left in the daemon is the run's **remaining machinery**, and it is
small: the two doors the run engine still opens for the **host's own state**
(`ResolveRunModel` — the run context's model falls back to the daemon's
effective config; `BuildLoopConfig` — the module's rule built from the
daemon's live state), and the host's own identity (`NewCorrelation`,
`SubjectForRun`).

The **standing package** itself still lives at `kernel/standing`, and its
eventual home is the **triggers module** (`kernel/modules/triggers`) — the
architecture gate already places both `kernel/cadence` and `kernel/standing`
in that module, so the family's boundary is drawn; the module's directory does
not exist yet, and the move is a path move (four production importers), not a
lifecycle one.
