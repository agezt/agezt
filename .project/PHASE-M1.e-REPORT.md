# Phase Report — Milestone 1.e (DAG Scheduler v0)

> Status: **shipped** · Date: 2026-05-29
> Per [SPEC-02 §4 (DAG Scheduler)](SPEC-02-KERNEL.md),
> [TASKS P1-SCHED-01..03](TASKS.md), and
> [DECISIONS B0d](DECISIONS.md) (DAG is a second layer over a first-party
> single-agent tool-loop).
> Continues [PHASE-M1.d-REPORT.md](PHASE-M1.d-REPORT.md).

## Scope

M1.e ships the **scheduler layer** that sits above the tool-loop: a
named graph of nodes, topologically scheduled with a bounded worker
pool. The agent loop becomes one node type (`LoopNode`); the HITL
approval queue from M1.d gets a `GateNode` wrapper that pauses an
edge until the operator decides.

What this phase delivers (and what it leaves for later):

| ROADMAP §2.1 MVP essential | Status after M1.e |
|---|---|
| #1 Kernel core | ✅ M0.5 |
| #2 DAG scheduler + Planner | 🟡 **scheduler shipped; LLM-driven Planner deferred** |
| #3 Governor + ≥2 providers | ✅ M1.b |
| #4 4 tools: shell, file, http, browser | ✅ shell/file/http (browser → next) |
| #5 1 channel: Telegram | ⏸ next |
| #6 Pulse v1 | ⏸ next |
| #7 Safety: Edict v1, Warden v1, HITL, halt | ✅ M1.a/c/d |

The **LLM-driven Planner** (intent → DAG via capability inventory,
SPEC-02 §4.1) is deliberately out of scope for M1.e. The scheduler is
the harder piece — the Planner is "an LLM call that returns JSON we
already accept on the wire." Building it without the executor would
have been pointless; the inverse ships value today (programmatic
plans, integration tests, e2e demos) and unlocks the Planner as a
straight follow-on.

## What shipped

### New package: `kernel/scheduler` (1083 LoC, 12 tests)

| File | LoC | Role |
|---|---:|---|
| `scheduler.go` | 495 | `Node` interface, `Plan`, `Executor`, topological scheduling, bounded pool, cycle/dup/unknown-dep validation, event publishers |
| `nodes.go` | 163 | `LoopNode` (wraps `agent.Run` via a kernel-supplied `LoopRunner`), `GateNode` (delegates to `approval.Registry`), ctx-propagation helpers |
| `scheduler_test.go` | 425 | 12 tests covering single-node, linear chain, parallel branches, failure propagation, validation paths, and Loop/Gate integration |

Public surface:

```go
type NodeKind string
const (KindLoop NodeKind = "loop"; KindGate NodeKind = "gate")

type Node interface {
    ID() string
    Kind() NodeKind
    DependsOn() []string
    Run(ctx context.Context, inputs Inputs) (Result, error)
}

type Plan struct {
    Name        string
    Nodes       []Node
    MaxParallel int  // 0 → DefaultMaxParallel = 8 (SPEC-02 §4.3)
}

type Executor struct{ /* ... */ }
func New(Config) *Executor
func (*Executor) Run(ctx, Plan, correlationID) (*PlanResult, error)
```

Concrete node types:

```go
type LoopNode struct {
    NodeID, Intent string
    Deps           []string
    Runner         LoopRunner             // closure over kernel's agent.Run
    IntentFn       func(Inputs) string    // optional: derive intent from upstream
}

type GateNode struct {
    NodeID, Capability, Description string
    Deps                             []string
    Approvals                        *approval.Registry
}
```

### Executor semantics (SPEC-02 §4.3)

- **Topological scheduling**: nodes with all deps satisfied are
  launched immediately and concurrently up to `MaxParallel`. Idle
  worker slots backpressure via a semaphore channel.
- **Failure propagation**: one node error fails the plan; downstream
  nodes whose any upstream errored are **skipped** (not started), so
  no `node.started` event fires for them. Sibling branches that don't
  depend on the failed node still run to completion. Compensation
  paths (SPEC-02 §4.3 "compensation branch") are deferred to M1.f+.
- **Dynamic scheduling**: the executor doesn't pre-compute a topo
  order — it re-polls "what's ready now?" after every completion. This
  is what lets fan-out work without any extra coordination.
- **Validation upfront**: `ErrEmptyPlan`, `ErrDuplicateNodeID`,
  `ErrUnknownDependency`, `ErrCycle` are all returned before any node
  runs (Kahn's algorithm for the cycle check).
- **Determinism (SPEC-02 §4.4)**: stable ordering inside `pickReady`
  (sorted by node ID) makes the launch order reproducible; the only
  remaining nondeterminism is the LLM's, which is itself journaled.

### New event kinds (`kernel/event/kinds.go`)

| Kind | Subject | Payload |
|---|---|---|
| `plan.started` | `plan.<planID>.lifecycle` | `{plan_name, node_count, node_ids[]}` |
| `plan.completed` | `plan.<planID>.lifecycle` | `{plan_name, node_count, results_keys[]}` |
| `plan.failed` | `plan.<planID>.lifecycle` | `{plan_name, failed_ids[]}` |
| `node.started` | `plan.<planID>.node.<nodeID>` | `{node_id, node_kind, deps[]}` |
| `node.completed` | `plan.<planID>.node.<nodeID>` | `{node_id, node_kind, output_bytes, ...detail}` |
| `node.failed` | `plan.<planID>.node.<nodeID>` | `{node_id, node_kind, error}` |

Subjects follow the existing wildcard convention: a client subscribing
to `plan.<planID>.>` sees everything for one plan; `plan.>.node.>`
sees every node event across every plan; `plan.>.lifecycle` is just
the plan headers.

### Runtime integration (`kernel/runtime`)

```go
type Kernel struct {
    /* ... */
    scheduler *scheduler.Executor   // NEW
}

func (k *Kernel) Scheduler() *scheduler.Executor
func (k *Kernel) LoopRunner() scheduler.LoopRunner   // adapter for LoopNode
func (k *Kernel) RunPlan(ctx, Plan, planID) (*scheduler.PlanResult, error)
```

`RunPlan` participates in the existing halt path — refusing to start
when the kernel is halted, and propagating cancellation through ctx
to in-flight nodes when Halt is called mid-plan.

`LoopRunner()` returns a closure that calls the kernel's `RunWith`
with the right correlation derivation
(`<planID>.loop.<nodeID>`), so the agent loop's existing
`agent.agent-<actor>.>` subjects nest cleanly under the plan's
correlation. `agt why` walks the full chain end-to-end.

### Control plane + `agt` CLI

```
CmdPlan = "plan"   // args: plan_json (string of JSON encoding planSpec)
```

JSON wire shape (lives in `controlplane/server.go`):

```json
{
  "name": "approve-then-execute",
  "max_parallel": 2,
  "nodes": [
    {"id": "approve", "kind": "gate",
     "capability": "plan.execute",
     "description": "About to run the execute loop with shell+file+http. Proceed?"},
    {"id": "execute", "kind": "loop", "deps": ["approve"],
     "intent": "list the files here and tell me what this project is"}
  ]
}
```

New CLI:

```
agt plan <file.json>     # submit a Plan, stream plan.* + node.* events
                         # final result prints each node's output
```

Existing `agt approve <id>` / `agt deny <id>` are how the operator
resolves a gate-node's request — the same UX as M1.d's tool-call HITL.

### Example: gate→loop plan (shipped in repo)

[.project/examples/plan-gate-execute.json](.project/examples/plan-gate-execute.json):

```json
{
  "name": "approve-then-execute",
  "max_parallel": 2,
  "nodes": [
    {
      "id": "approve",
      "kind": "gate",
      "capability": "plan.execute",
      "description": "About to run the execute loop with shell+file+http tools. Proceed?"
    },
    {
      "id": "execute",
      "kind": "loop",
      "deps": ["approve"],
      "intent": "list the files here and tell me what this project is"
    }
  ]
}
```

## Demo transcript — grant path

```
$ AGEZT_HOME=/tmp/agezt-m1e-demo AGEZT_PROVIDER=mock ./bin/agezt

Agezt 0.0.0-m0 — daemon ready (protocol v1)
  base dir         : /tmp/agezt-m1e-demo
  governor         : primary=mock(offline; scripted shell+final), daily_ceiling=$20.00
  tools            : shell(warden=requested-namespace), file(...), http(hosts=0)
  policy engine    : edict (defaults from DECISIONS F3; AskAllow)
  warden           : requested=namespace, effective=none (M1.c facade)
  control plane    : 127.0.0.1:57296

# Shell A — submit the plan; the gate node blocks waiting for approval:
$ ./bin/agt plan .project/examples/plan-gate-execute.json
  [evt seq=0 kind=plan.started      subject=plan.…lifecycle]
  [evt seq=1 kind=node.started      subject=plan.…node.approve]
  ▶ (blocking — gate-node submitted to approval queue)

# Shell B — operator inspects and grants:
$ ./bin/agt approvals
1 pending approval(s):
  id         : appr-01KSS786TYD6VGH27YNYMK7WYB
  capability : plan.execute
  tool       : scheduler.gate
  reason     : About to run the execute loop with shell+file+http tools. Proceed?
  actor      : scheduler

$ ./bin/agt approve appr-01KSS786TYD6VGH27YNYMK7WYB "ship it"
{ "decision": "grant", "id": "appr-01KSS786TYD6VGH27YNYMK7WYB", "ok": true }
```

The full daemon event stream for the grant path (23 events, all
journaled and BLAKE3-chained):

```
[evt seq=0  kind=plan.started               subject=plan.…lifecycle]
[evt seq=1  kind=node.started               subject=plan.…node.approve]
[evt seq=2  kind=approval.requested         subject=approval.request]
[evt seq=3  kind=approval.granted           subject=approval.resolve]
[evt seq=4  kind=node.completed             subject=plan.…node.approve]
[evt seq=5  kind=node.started               subject=plan.…node.execute]
[evt seq=6  kind=task.received              subject=agent.agent-plan-…loop.execute.task]
[evt seq=7  kind=llm.request                subject=agent.…llm]
[evt seq=8  kind=routing.decision           subject=governor.route]
[evt seq=9  kind=budget.consumed            subject=governor.budget]
[evt seq=10 kind=llm.response               subject=agent.…llm]
[evt seq=11 kind=policy.decision            subject=agent.…policy]
[evt seq=12 kind=tool.invoked               subject=agent.…tool]
[evt seq=13 kind=warden.profile_downgraded  subject=warden.profile]
[evt seq=14 kind=warden.executed            subject=warden.exec]
[evt seq=15 kind=tool.result                subject=agent.…tool]
[evt seq=16 kind=llm.request                subject=agent.…llm]
[evt seq=17 kind=routing.decision           subject=governor.route]
[evt seq=18 kind=budget.consumed            subject=governor.budget]
[evt seq=19 kind=llm.response               subject=agent.…llm]
[evt seq=20 kind=task.completed             subject=agent.…task]
[evt seq=21 kind=node.completed             subject=plan.…node.execute]
[evt seq=22 kind=plan.completed             subject=plan.…lifecycle]
```

Client output after the grant lands:

```
--- plan completed ---
plan_id: plan-…

[approve]
gate granted by operator

[execute]
[offline-mock] I ran a directory listing via the shell tool. This
project is Agezt — an open-source, MIT-licensed agentic operating
system written in Go. …

$ ./bin/agt journal verify
{ "ok": true }
```

## Demo transcript — deny path

Same plan, operator denies:

```
$ ./bin/agt deny appr-01KSS79BSBAXKCNQG0948ZNTB2 "not yet"

# Plan client sees:
[evt seq=0 kind=plan.started      subject=plan.…lifecycle]
[evt seq=1 kind=node.started      subject=plan.…node.approve]
[evt seq=4 kind=node.failed       subject=plan.…node.approve]
[evt seq=5 kind=plan.failed       subject=plan.…lifecycle]

agt plan: controlplane: plan "approve-then-execute" failed:
  node "approve": gate denied: deny (not yet)
```

Daemon log (full 6 events):

```
[evt seq=0 kind=plan.started        subject=plan.…lifecycle]
[evt seq=1 kind=node.started        subject=plan.…node.approve]
[evt seq=2 kind=approval.requested  subject=approval.request]
[evt seq=3 kind=approval.denied     subject=approval.resolve]
[evt seq=4 kind=node.failed         subject=plan.…node.approve]
[evt seq=5 kind=plan.failed         subject=plan.…lifecycle]
```

The `execute` LoopNode is **never started** — the agent loop's
expensive LLM round and shell invocation are skipped entirely. That's
the architectural payoff of plan-approval-before-execution.

## Verified invariants

| Invariant | Test |
|---|---|
| 1-node plan runs to completion; plan.started + plan.completed both fire | `TestRun_SingleNodePlan` |
| Linear chain a→b→c runs in dependency order | `TestRun_LinearChainPreservesOrder` |
| Fan-out branches actually run in parallel (max-inflight ≥ 3 with 3 siblings) | `TestRun_ParallelBranchesRunConcurrently` |
| Sibling branches survive when one branch fails; downstream of failure is skipped | `TestRun_FailureAbortsDownstreamButNotSiblings` |
| Cyclic plan rejected with `ErrCycle` before any node runs | `TestRun_DetectsCycle` |
| Duplicate node IDs rejected with `ErrDuplicateNodeID` | `TestRun_RejectsDuplicateID` |
| Dependency on missing ID rejected with `ErrUnknownDependency` | `TestRun_RejectsUnknownDependency` |
| Empty plan rejected with `ErrEmptyPlan` | `TestRun_RejectsEmptyPlan` |
| GateNode grant releases the downstream branch | `TestGateNode_GrantedReleasesDownstream` |
| GateNode deny fails the gate and skips downstream | `TestGateNode_DeniedAbortsDownstream` |
| LoopNode delegates to the supplied runner with the right intent + correlation | `TestLoopNode_DelegatesToRunner` |
| `IntentFn` can derive a downstream loop's intent from upstream outputs | `TestLoopNode_IntentFnReadsUpstream` |

All 12 scheduler tests pass. Existing 168 tests unaffected. Total
module: **180 passing tests** across **25 packages**, vet clean,
depscheck clean.

## Cumulative status

```
25 packages | ~12,300 lines source+tests | 180 tests passing | 2 deps (allowlisted)
```

| Subsystem | LoC | Tests |
|---|---:|---:|
| `kernel/{ulid,event,journal,state,bus,agent,runtime,controlplane}` | ~4,500 | 65 |
| `kernel/edict` | ~600 | 16 |
| `kernel/governor` | 899 | 12 |
| `kernel/warden` | 726 | 9 |
| `kernel/approval` | 578 | 8 |
| `kernel/scheduler` | **1,083** | **12** |
| `plugins/providers/{mock,anthropic,ollama}` | 1,034 | 13 |
| `plugins/tools/{shell,file,http}` | ~1,360 | 35 |
| `cmd/{agezt,agt}` | ~1,060 | 8 |
| `internal/{brand,paths}` | 102 | 1 |
| `tools/{jsonschemagen,depscheck}` | 633 | (jsonschemagen: 3 + e2e) |

## Deviations from spec (intentional)

1. **No LLM-driven Planner.** SPEC-02 §4.1 describes a Planner that
   reads the capability inventory and emits `EVT_PLAN_PROPOSED`. M1.e
   ships the executor; plans are constructed programmatically (Go
   code) or via the JSON wire shape (`agt plan <file.json>`). The
   Planner is a single LLM call that returns the same JSON — its
   "missing-capability" path needs the catalog (TASKS P1-CONDUIT-04)
   which lands separately.
2. **Two node types only.** `tool-node`, `agent-node` (parallel
   sub-agent spawn), and `coding-node` from SPEC-01 §6 are deferred.
   The `Node` interface is intentionally narrow so they slot in
   without changing the executor — adding a tool-node is ~30 lines
   plus tests.
3. **No compensation branches.** A single node failure currently
   stops the plan (after sibling branches finish). SPEC-02 §4.3
   mentions per-node retry + compensation. Retry is a wrapper around
   the existing Node; compensation needs a way to express "if X
   fails, run Y instead" — comes with the M1.f Planner work.
4. **No `loop-node` budget ceiling enforcement** (SPEC-02 §4.3
   "loop-node enforces max iterations and a budget ceiling"). The
   existing `agent.Run` already honours its own `MaxIter` and the
   Governor already enforces a daily ceiling, so this isn't *quite*
   ungated — but per-plan / per-loop budget caps aren't yet wired
   through `LoopNode`. Trivial to add when there's a concrete need.
5. **No path-scoped concurrency** (SPEC-02 §4.3 "nodes touching the
   same path serialize"). The executor only cares about explicit
   DAG dependencies; resource-based serialization needs a notion of
   per-node "claims" that I'd rather build atop a real workload.
6. **JSON wire shape is minimal.** Only `loop`/`gate` kinds and the
   bare-essential fields (`id`, `kind`, `deps`, `intent`,
   `capability`, `description`) are accepted. No JSON-schema
   validation, no per-node retry policy in the wire shape, no
   plan-level timeout. Future node kinds extend the spec; existing
   plans keep working.

## Open items for M1.f

- **LLM-driven Planner** (TASKS P1-PLAN-01) — intent → DAG via
  capability inventory + relevant world-model context. Emits
  `EVT_PLAN_PROPOSED`; missing-capability path either requests Forge
  to create a skill or surfaces a "missing capability" event.
- **Plan approval gate-node front** (TASKS P1-PLAN-02) — auto-inject
  a `gate` node at the front of any plan whose cost/scope estimate
  exceeds a threshold.
- **Additional node types**: `tool-node` (one direct tool call without
  a full loop), `agent-node` (parallel sub-agent spawn), `coding-node`.
- **Retry + compensation per node** (TASKS P1-SCHED-03).
- **Live model catalog sync** (TASKS P1-CONDUIT-04 / SPEC-15) — feeds
  the Planner's capability inventory.
- **Browser tool** (TASKS P1-TOOL-04) — requests ProfileContainer
  through Warden.
- **Telegram channel** (TASKS P4-CHAN-01) + out-of-process plugin
  host (DECISIONS B0a).
- **Warden Linux backend** (TASKS P1-WARD-01).
- **Pulse v1** (TASKS P3-*).

## Pointers

- Tests: `go test ./...` (180 pass, vet clean, depscheck OK)
- Plan demo (grant path):
  ```
  # shell A — daemon:
  AGEZT_HOME=/tmp/d AGEZT_PROVIDER=mock ./bin/agezt

  # shell B — submit the plan; blocks at the gate:
  ./bin/agt plan .project/examples/plan-gate-execute.json

  # shell C — operator decides:
  ./bin/agt approvals
  ./bin/agt approve <id>     # → execute runs
  # or:
  ./bin/agt deny <id> "nope" # → plan aborts; execute never starts
  ```
- Construct a plan in Go (no JSON needed):
  ```go
  plan := scheduler.Plan{
      Name: "approve-then-execute",
      Nodes: []scheduler.Node{
          &scheduler.GateNode{
              NodeID: "approve", Approvals: kernel.Approvals(),
              Capability: "plan.execute",
          },
          &scheduler.LoopNode{
              NodeID: "execute", Deps: []string{"approve"},
              Intent: "list the files", Runner: kernel.LoopRunner(),
          },
      },
  }
  result, err := kernel.RunPlan(ctx, plan, "")
  ```
- Backwards-compatibility: `agt run "<intent>"` is unchanged. The
  scheduler is purely additive — anyone who doesn't construct a Plan
  sees no behavioural difference.
- Next milestone report: `PHASE-M1.f-REPORT.md`
