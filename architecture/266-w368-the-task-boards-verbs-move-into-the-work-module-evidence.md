# W3.68 — the task board's verbs move into the work module

## What the slice is

The **first** slice of the **runtime's dissolution**, and the first time the
port programme's work is *spent*: the port layers were the preparation, and
this is one of the moves they were preparing.

The **task board** was split in half. Its **DATA** — the store, the
transitions, the retry rule — already lived in the work module
(`kernel/modules/work/workboard`, 1,014 lines). Its **OPERATIONS** — the
fifteen verbs that create, claim, complete and archive a task — lived in the
daemon (`kernel/runtime/workboard.go`), spelling out the lifecycle over the
store the daemon held.

That is the split P5 forbids: the module that keeps the data must also keep
the operations over it, and the daemon may hold its **face**, never its
**body**.

## What the slice found

The measurement that named the candidate:

- the daemon's board file: **297 lines**, **15** lifecycle verbs;
- the module's board package: **1,014 lines** — the store, the transitions,
  the retry rule, the aliases;
- the callers of the daemon's verbs: **24** call sites, **all** of them in the
  board's own family (`kernel/app/workboard`) and the root;
- the target architecture's own mapping: `work` owns `Task.*`, and
  `workboard dispatch + proof + OKR → modules/work`.

The split was not visible from the daemon's side, because the daemon's file
*looked* like a thin layer: it called the store, published an event and
recursed into the OKR rollup. What it actually held was **every decision**
about what a task's lifecycle MEANS — including the two rules that are
nowhere else: the retry decision's action name (`escalate` when the budget is
spent) and the sweep's per-task action.

## What changed

### The module owns the service

`kernel/modules/work/api` is the module's new public vocabulary: the
**Service** the daemon drives, and the **Dependencies** the module needs from
the host.

```go
// kernel/modules/work/api/workboard.go
type Service interface {
	Create(corr string, spec workboardapi.CreateSpec) (workboardapi.Task, bool, error)
	Claim(corr, id, agent, runID string) (workboardapi.Task, error)
	… // fifteen verbs, the daemon's own names
}

type Dependencies struct {
	Publish       func(spec event.Spec)
	TaskCompleted func(corr, taskID string)
}
```

`kernel/modules/work/workboard/service.go` is the implementation: the fifteen
verbs over the contract's host port, the publish, and the objective rollup.

### The daemon keeps its names and forwards

```go
// kernel/runtime/workboard.go
func (k *Kernel) ClaimWorkboardTask(corr, id, agent, runID string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.Claim(corr, id, agent, runID)
}
```

**Every** caller outside the runtime is untouched: the daemon's public names
did not move, so the application layer — which never named the daemon's board
verbs anyway — did not change at all. **0 production files** in
`kernel/app` changed.

### The module is wired by the composition root

```go
// cmd/agezt/main_toolinvoker.go
k.SetWorkboard(workboard.NewService(k.Workboard(), k.WorkboardDependencies()))
```

The root is the one place that knows both the store it opened and the module
that drives it. This is the same shape the reasoning module uses
(`k.SetReasoning(reasoning.New(k.ReasoningDependencies()))`), and it is the
shape the layer rule forces: a module may import another module's `api`
package and nothing else, so the runtime can hold the module's face but never
name its implementation.

### The two answers both sides need became contract rows

The daemon may not name the module and the module may not name the daemon, so
the answers they share live in the contract:

- `workboardapi.ErrNoStore` — the refusal a daemon with no board gives;
- `workboardapi.TaskEventPayload` — the payload the board publishes (the
  daemon's proof path publishes the same shape, so the tools read one).

### What the slice cost, and what it fixed on the way

- **11 test sites** had to wire the board, because the module is injected
  after `Open` — the same price the reasoning module's move paid in 6 places.
- The daemon's board publish had **no nil-bus check** (`k.bus.Publish` on a
  daemon with no bus): a latent panic, now structurally impossible — the
  host's port is nil-safe and the board stays quiet instead.

## The rule

The daemon holds the module's **face**; the module holds the module's
**body**.

The rule is held by the rule's own observer: a fake module records what the
daemon asks it. A daemon that grew the lifecycle back would answer on its own
and the fake would record nothing.

## The tests

**11 permanent tests**, in four places:

- `kernel/modules/work/workboard/service_test.go` — the board refuses without
  a store (all fifteen verbs, the contract's row), publishes what happened to
  a task (kind, subject, correlation, payload), tells the host that a task
  reached done, and is quiet without a bus.
- `kernel/runtime/workboard_module_test.go` — the daemon asks the module (a
  fake records the arguments and the answer comes from the module), the
  daemon refuses when the module is not wired, and the daemon still holds the
  host's board file.
- `kernel/contract/workboardapi/event_test.go` — the refusal is the
  contract's, the payload carries the task and the action, and a blank extra
  does not reach the payload.
- `cmd/agezt/workboard_module_test.go` — the root wires the board's module.

## The mutations

**13 mutations, 13 caught**, in two independent passes with **byte-identical**
logs.

SHA-256: `79D7F91071F5866007BE92FE876ABEBEAE87A482A75D18C513536C3AB8160D7E`

The first calibration reported **three BUILDERRs**, and all three were the
**mutation's** defect: a forwarder that stopped calling the module left the
service **declared and not used**, and Go refused to compile — a compile error
is not a caught mutation, it is a mutation that never ran. All three were
rewritten to keep the service alive (`_ = svc`) while answering on their own.

- **M01**, **M02**, **M12** the daemon's claim, complete and create answer on
  their own — the **fake module** catches all three.
- **M03** the daemon swallows the refusal — the **refusal** catches it.
- **M04** the module swallows the refusal — the module's own refusal catches
  it.
- **M05**, **M06**, **M08**, **M13** the module publishes the wrong kind, drops
  the action, publishes the wrong subject, or stays silent on a claim — the
  **publish** catches all four.
- **M07** the module does not tell the host that a task reached done — the
  **hook** catches it.
- **M09**, **M10** the contract drops a field of the payload, or says
  something else than "not available" — the **contract's own tests** catch
  them.
- **M11** the root does not wire the board — the **root's** test catches it.

## The measurement that moved

| | before | after |
|---|---|---|
| the daemon's calls into the board's store | **15** | **0** |
| the daemon's calls into the module | **0** | **15** |
| the module's calls into the board's store | **0** | **15** |
| the daemon's public board names | **15** | **15** |
| the kernel's verbs | **236** | **238** |
| the application layer's production files that changed | — | **0** |
| `kernel/runtime` (lines) | **9,649** | **9,588** |
| `kernel/modules/work` (lines) | **1,664** | **2,049** |
| the daemon's board file (lines) | **297** | **251** |
| the contract's board rows | — | **2** |
| the daemon's answer when the module is not wired | the store's answer | the **contract's refusal** |

The **kernel's verb count moved by two** (236 → 238): the fifteen board verbs
stayed — the names are the host's public API — and the slice added the two
names of the **seam** (`SetWorkboard` and `WorkboardDependencies`). What moved
is the **body** behind the names, not the names — and the daemon's board file
is now 46 lines of forwarders and their refusals.

## What is left

The board's verbs live in the module that keeps the board.

What is left of the board is the **proof** (`kernel/runtime/workboard_proof.go`):
the loop that judges a task's answer against its acceptance criteria. It
still lives in the daemon because it judges with the daemon's **model** and
reads the daemon's artifact index and journal — the same shape as the
reasoning module's service, and the same move: give it an `api` package, hand
it the host's ports, and let the daemon forward.

The rest of the runtime's dissolution stands at **238 verbs** in
**9,588 lines**, of which the board's fifteen and the workflow host's twelve
and the reasoning module's six are the nearest.
