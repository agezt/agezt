# W3.69 — the objective library's verbs move into the work module

## What the slice is

The **second** slice of the runtime's dissolution, and the first one to move a
family whose **rule** — not just its operations — lived in the daemon.

The **objective library** was split in half, exactly like the task board was
one slice ago. Its **DATA** — the store, the durable goals, the rollup rule's
row — already lived in the work module (`kernel/modules/work/okr`, 380 lines
before this slice). Its **OPERATIONS** — the eight verbs that create a goal,
add a key result, link a task, unlink a task, archive a goal and roll a goal
up — lived in the daemon (`kernel/runtime/okr.go`), spelling out the lifecycle
over the store the daemon held.

That is the split P5 forbids: the module that keeps the data must also keep
the operations over it, and the daemon may hold its **face**, never its
**body**.

The board's move was the first half of the work module's dissolution. This one
closes the other half of the same family: after it, the work module keeps the
board AND the goals, and the daemon holds two faces instead of two bodies.

## What the slice found

The measurement that named the candidate:

- the daemon's objective file: **175 lines**, **8** public verbs;
- the module's objective package: **380 lines** — the store, the durable
  library, the aliases;
- the callers of the daemon's objective verbs: **6** call sites, **all** of them
  in the objective family's own application (`kernel/app/okr`);
- the target architecture's own mapping: `work` owns `OKR.*`, and the roadmap
  lists `workboard dispatch + proof + OKR → modules/work` as the same move.

The split was not visible from the daemon's side, because the daemon's file
*looked* like a thin layer: it called the store and published an event. What
it actually held was **every decision** about what a goal's PROGRESS means —
including the rule that decides whether a linked task counts toward a Key
Result (it counts when it is **done**), and the rule that maintains the
durable "achieved" signal when a task crosses the target or regresses below
it.

## What changed

### The module owns the service

`kernel/modules/work/api` already carried the board's vocabulary. The
objective library's vocabulary now sits beside it in the same package, with its
own names — a package that carries two families must not give them one name.

```go
// kernel/modules/work/api/okr.go
type ObjectiveService interface {
	Create(corr string, spec okrapi.CreateSpec) (okrapi.Objective, error)
	AddKeyResult(corr, objID, title string, target int) (okrapi.Objective, error)
	LinkTask(corr, objID, krID, taskID string) (okrapi.Objective, error)
	UnlinkTask(corr, objID, krID, taskID string) (okrapi.Objective, error)
	Archive(corr, objID string) (okrapi.Objective, error)
	Progress(o okrapi.Objective) okrapi.ObjectiveProgress
	ProgressOf(id string) (okrapi.Objective, okrapi.ObjectiveProgress, bool)
	RecomputeForTask(corr, taskID string)
}

type ObjectiveDependencies struct {
	Publish    func(spec event.Spec)
	TaskStatus func(taskID string) (workboardapi.Status, bool)
}
```

`kernel/modules/work/okr/service.go` is the implementation: the eight verbs
over the contract's host port, the publish, the rollup and the recompute.

### The daemon keeps its names and forwards

```go
// kernel/runtime/okr.go
func (k *Kernel) LinkObjectiveTask(corr, objID, krID, taskID string) (okrapi.Objective, error) {
	svc, err := k.objectiveLibraryOrRefusal()
	if err != nil {
		return okrapi.Objective{}, err
	}
	return svc.LinkTask(corr, objID, krID, taskID)
}
```

**Every** caller outside the runtime is untouched: the daemon's public names
did not move, so the application layer — which never named the daemon's
objective verbs anyway — did not change at all. **0 production files** in
`kernel/app` changed.

### The module is wired by the composition root

```go
// cmd/agezt/main_toolinvoker.go
k.SetObjectiveLibrary(okr.NewService(k.OKR(), k.ObjectiveLibraryDependencies()))
```

The root is the one place that knows both the store it opened and the module
that drives it — the same shape the board's move used, and the one the layer
rule forces: a module may import another module's `api` package and nothing
else, so the runtime can hold the module's face but never name its
implementation.

### The refusal both sides need became a contract row

The daemon may not name the module and the module may not name the daemon, so
the answer they share lives in the contract:

- `okrapi.ErrNoLibrary` — the refusal a daemon with no library gives. The
  board has its own row (`workboardapi.ErrNoStore`), and the library has its
  own: a daemon that holds neither must be able to say **which** one is
  missing — one row for both would make the answer a guess.

### What the slice cost

- **6 test sites** had to wire the library, because the module is injected
  after `Open` — the price the board's move paid in 11 places, and the
  reasoning module's move paid in 6. The price is the slice's real cost, and
  it is counted here rather than discovered later.

## The rule

The daemon holds the module's **face**; the module holds the module's
**body**.

The rule is held by the rule's own observer: a fake module records what the
daemon asks it. A daemon that grew the lifecycle back would answer on its own
and the fake would record nothing.

## The tests

**14 permanent tests**, in four places:

- `kernel/modules/work/okr/service_test.go` — the library refuses without a
  store (all six refusing verbs, the contract's row), publishes what happened
  to a goal (kind, subject, correlation, payload), rolls up from the live
  status of the linked tasks, counts nothing without a board, recomputes the
  durable "achieved" signal in both directions, and is quiet without a bus.
- `kernel/runtime/okr_module_test.go` — the daemon asks the module (a fake
  records the arguments and the answer comes from the module), the daemon
  refuses when the module is not wired, the daemon still holds the host's
  library file, the daemon rolls up **through** the module, and the daemon
  counts nothing without it.
- `kernel/contract/okrapi/errnolibrary_test.go` — the refusal is the
  contract's, and it is its **own** row (not the not-found row, and it names
  the objective library).
- `cmd/agezt/okr_module_test.go` — the root wires the objective library's
  module.

## The mutations

**13 mutations, 13 caught**, in two independent passes with **byte-identical**
logs.

SHA-256: `9FD92FCD93468EC6CF5BC1CAE4AF462E144B591CD235B47A06D0F8FF96088963`

The first calibration reported **one BUILDERR**, and it was the **mutation's**
defect: the mutation that made every task count as done dropped the
`workboardapi` reference, and Go refused to compile — a compile error is not a
caught mutation, it is a mutation that never ran. It was rewritten to keep the
import alive while changing the rule (`ok || status == workboardapi.StatusDone`
— every task the host knows about counts, whether it is done or not).

- **M01**, **M02**, **M03** the daemon's create, link and archive answer on
  their own — the **fake module** catches all three.
- **M04** the daemon swallows the refusal — the **refusal** catches it.
- **M05** the daemon rolls up on its own instead of asking the module — the
  **rollup** catches it.
- **M06** the module answers a different row — the module's own refusal catches
  it.
- **M07**, **M08**, **M11** the module publishes the wrong kind, the wrong
  subject, or stays silent on a creation — the **publish** catches all three.
- **M09** the module counts every task as done — the **rollup** catches it.
- **M10** the module does not flip the achieved signal — the **recompute**
  catches it.
- **M12** the contract says something else than "not available" — the
  **contract's own tests** catch it.
- **M13** the root does not wire the library — the **root's** test catches it.

## The measurement that moved

| | before | after |
|---|---|---|
| the daemon's calls into the library's store | **10** | **0** |
| the daemon's calls into the module | **0** | **8** |
| the module's calls into the library's store | **0** | **10** |
| the daemon's public objective names | **8** | **10** |
| the kernel's verbs | **238** | **240** |
| the application layer's production files that changed | — | **0** |
| `kernel/runtime` (lines) | **9,588** | **9,590** |
| `kernel/modules/work` (lines) | **2,049** | **2,323** |
| the daemon's objective file (lines) | **175** | **172** |
| the contract's objective rows | — | **1** |
| the daemon's answer when the module is not wired | the store's answer | the **contract's refusal** |

The **kernel's verb count moved by two** (238 → 240): the eight objective
verbs stayed — the names are the host's public API — and the slice added the
two names of the **seam** (`SetObjectiveLibrary` and
`ObjectiveLibraryDependencies`). What moved is the **body** behind the names,
not the names.

The lines are counted the way the previous slice counted them: **non-blank**
lines of the package's production files. The daemon's runtime grew by two
lines, because the forwarders are shorter than the bodies they replaced — the
daemon's objective file is now 172 lines of forwarders and their refusals, and
the module's objective package grew from 380 to 577 lines, where the two rules
that were nowhere else now live.

## What is left

The goals' verbs live in the module that keeps the goals.

What is left of the work module's dissolution is the board's **proof**
(`kernel/runtime/workboard_proof.go`): the loop that judges a task's answer
against its acceptance criteria. It still lives in the daemon because it
judges with the daemon's **model** and reads the daemon's artifact index and
journal — the same shape as the reasoning module's service, and the same move:
give it an `api` package, hand it the host's ports, and let the daemon
forward.

The rest of the runtime's dissolution stands at **240 verbs** in
**9,590 lines**, of which the board's fifteen and the objective library's ten
and the workflow host's twelve and the reasoning module's six are the nearest.
