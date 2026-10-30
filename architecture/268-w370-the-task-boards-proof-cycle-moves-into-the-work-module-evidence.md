# W3.70 — the task board's proof cycle moves into the work module

## What the slice is

The **third** slice of the runtime's dissolution, and the first one to move a
**judgment** rather than a lifecycle.

The task board's **proof cycle** lived in the daemon
(`kernel/runtime/workboard_proof.go`, 218 lines). It was the loop that decides
whether a task actually did what it promised: it gathers the evidence the
proof rests on (the artifacts and the journal range produced under the task's
correlation id), asks a judge whether the answer satisfies the task's
acceptance criteria, reads the judge's reply, records the proof on the task
and announces the verdict.

That is the **board's** business — the module's own words say so: "the
durable, checkable evidence that a **workboard task** actually satisfied its
acceptance criteria". The task carries the proof, and the proof carries the
verdict. A module that keeps the task and the store but leaves the judgment
in the daemon keeps the data and gives away the meaning of the data.

After W3.68 (the lifecycle), W3.69 (the goals) and W3.70 (the proof), the work
module keeps the whole of the board's business: its lifecycle, its goals and
its judgment.

## What the slice found

The measurement that named the candidate:

- the daemon's proof file: **218 lines**, **1** public verb (`ProveTask`);
- the module's proof package: **29 lines** — the rows and nothing else (the
  aliases of `proofapi`'s `Criterion`, `Evidence` and `Proof`);
- the daemon's reach into the host: **six** capabilities — the board store,
  the model, the artifact index, the journal, the bus and the host's own
  plain completion judge.

The **shape** of the candidate decided the slice's design. The proof is not a
separate family with its own face — it is a **verb of the board**. So the
proof did not become a new service with a new injection point: it became
`BoardService.Prove`, and the board's dependencies grew the ports the judge
asks through.

**That decision is the slice's real finding**, and it is what makes the slice
cheap: the board was **already wired** in every host that had one.

## What changed

### The module owns the judgment

`kernel/modules/work/workboard/workboard_proof.go` is the cycle: the evidence
to gather, the criterion prompt, the reply to parse, the verdict to publish
and the proof to record.

### The board's face grows one verb and five ports

```go
// kernel/modules/work/api/workboard.go
type BoardService interface {
	// … the fifteen lifecycle verbs …
	Prove(ctx context.Context, corr, id, answer string) (workboardapi.Task, error)
}

type BoardDependencies struct {
	Publish            func(spec event.Spec)
	TaskCompleted      func(corr, taskID string)
	TaskProved         func(corr, taskID string)
	Complete           func(ctx context.Context, corr, taskType string, req llm.CompletionRequest) (*llm.CompletionResponse, error)
	Model              func() string
	VerifyCompletion   func(ctx context.Context, corr, task, answer string) (assureapi.Verdict, error)
	Artifacts          ArtifactLister
	Journal            EventScanner
}
```

The `Complete` port is the reasoning module's port, **letter for letter**: the
governor's task routing, the task budgets and the per-run spend attribution
keep working, because the judge asks through the host's funnel rather than
around it.

`ArtifactLister` and `EventScanner` are the module's own narrow shapes
(`kernel/modules/work/api/proof.go`). The host's artifact index and the host's
journal satisfy them **structurally** — nothing in the contract names the
host's journal package, so the module never reaches it. The module holds a way
to ASK and does the walking and the filtering itself, which is what keeps the
**rule** ("scan by correlation id, not a fixed tail window") in the module
where it belongs.

### The daemon keeps its name and forwards

```go
// kernel/runtime/workboard_proof.go
func (k *Kernel) ProveTask(ctx context.Context, corr, id, answer string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.Prove(ctx, corr, id, answer)
}
```

**No caller outside the runtime changed**: **0 production files** in
`kernel/app` changed.

### What the slice cost

**Nothing to wire.** The board was already injected in every host that has
one, so the proof came with it. The two prior slices paid **11** and **6**
test sites for their new injection points; this one paid **0**.

That is the finding worth keeping: **a move into an existing face costs
nothing, and a move that invents a new face costs a wiring in every host.**
The proof could have been a new service — a `ProofService` with its own
`SetProofService` — and it would have cost six more wirings and a second
refusal row to answer the same question.

## The rule

The daemon holds the module's **face**; the module holds the module's
**body**.

The rule is held by the rule's own observer: a fake board records what the
daemon asks. A daemon that grew the judgment back would answer on its own and
the fake would record nothing.

## What did NOT move, and why

- **The judge stays the host's.** The model, the governor's routing and the
  spend attribution are the host's machinery. The module asks through a port.
- **The host's plain completion judge stays the host's.** A task with no
  acceptance criteria has no criteria to judge; the host's own judge answers
  whether the answer accomplished the task at all. The module asks for that
  answer (`VerifyCompletion`) and never judges completion itself.
- **The refusal did not become a contract row.** The board's existing row
  (`workboardapi.ErrNoStore`) already answers "no board" — whether the store
  was never opened or the module was never wired — and the proof answers with
  it. The **judge's** refusal (`errNoJudge`) is the module's own: only the
  module answers it, so a shared row would be a row with one reader.

## The tests

**21 permanent tests**, in three places:

- `kernel/modules/work/workboard/workboard_proof_test.go` — **16 tests**: the
  board refuses to prove without a store (the contract's row), it proves a
  criterialess task with the host's plain judge, it **holds the gate** when
  the answer falls short, it judges **each** criterion and reads the reply, it
  holds the gate when the reply cannot be read, it holds the gate when the
  host has no judge, it gathers the evidence under the **right** correlation
  id, it gathers no evidence when the host has none, it publishes the
  verdict, it tells the host that a task was proved, it synthesizes an answer
  when there is none, it refuses a task it cannot find, it is quiet without a
  bus, and the judge's reply is read even when it is fenced in prose.
- `kernel/runtime/workboard_proof_module_test.go` — **4 tests**: the daemon
  asks the module (a fake records the arguments and the answer comes from the
  module), the daemon refuses when the module is not wired, the daemon hands
  the proof's ports over, and the daemon hands its journal over.
- `cmd/agezt/workboard_proof_module_test.go` — **1 test**: the root wires the
  proof cycle.

## The mutations

**13 mutations, 13 caught**, in two independent passes with **byte-identical**
logs.

SHA-256: `C734D7BFCA4D490ABED5EF80141387D6140AF46B435223FCEB6577DB0F88D4EA`

The first calibration reported **one BUILDERR**, **one** mutation that
survived and **two** that were pointed at the wrong observer — and all four
were the **harness's** faults, not the code's:

- the mutation that stopped the daemon asking the module left `svc` unused, so
  Go refused to compile — a compile error is not a caught mutation, it is a
  mutation that never ran;
- the mutation that made the module read a broken reply as a pass was pointed
  at the **second** of the two "not valid JSON" branches, and the test that
  watches the gate exercises the **first**;
- the mutation that dropped the artifact listing was pointed at the test that
  expects **no** evidence — a host with no artifact index — and the test that
  watches the artifacts is the other one;
- the mutation that emptied the evidence's correlation id was **correct** and
  **survived**, because the test did not yet say whose correlation the
  evidence belongs to. The test was wrong, not the mutation: the evidence's
  correlation id IS the board's rule, and it is now pinned.

- **M01**, **M02** the daemon stops asking the module, or swallows the
  refusal — the **fake board** and the **refusal** catch them.
- **M03**, **M04** the module answers the wrong row, or proves a task that is
  not there — the module's own refusals catch them.
- **M05** the module judges without a judge — the **gate** catches it.
- **M06**, **M07**, **M08**, **M09** the module lets a failing answer
  through, judges only the first criterion, reads a broken reply as a pass, or
  cannot find the JSON in the reply — the **gate**, the **criteria**, the
  **unreadable reply** and the **fence** catch them.
- **M10**, **M11** the module gathers the evidence under the wrong correlation
  id, or does not ask the host for the artifacts — the **evidence** catches
  them.
- **M12**, **M13** the module does not tell the host, or synthesizes nothing
  — the **telling** and the **answer** catch them.

## The measurement that moved

| | before | after |
|---|---|---|
| the daemon's calls into the board's store (the proof cycle) | **3** | **0** |
| the daemon's calls into the module (the proof cycle) | **0** | **1** |
| the module's calls into the board's store (the proof cycle) | **0** | **2** |
| the daemon's public proof names | **1** | **1** |
| the kernel's verbs | **240** | **240** |
| the application layer's production files that changed | — | **0** |
| the test sites that had to be wired | — | **0** |
| the contract's new rows | — | **0** |
| `kernel/runtime` (lines) | **9,590** | **9,395** |
| `kernel/modules/work` (lines) | **2,323** | **2,673** |
| the daemon's proof file (lines) | **218** | **24** |
| the module's proof file (lines) | — | **250** |
| the daemon's helpers the move made dead | — | **2** |

The **kernel's verb count did not move at all** (240 → 240), and that is the
point: `ProveTask` is still the daemon's name and always was. The board's
service gained a verb, the daemon kept its one, and **what moved is the body
behind the name**.

The lines are counted the way the previous two slices counted them:
**non-blank** lines of the package's production files. The daemon's runtime
shrank by 195 lines, and the module grew by 350 — the module's proof file
alone is 250 lines, because the judgment does not fit in a forwarder.

### The move made two daemon helpers dead

The daemon's `workboardStore()` and `publishWorkboard()` were the proof
cycle's **only** callers: the store was reached only to read the task and to
record the proof, and the publisher only announced the verdict. When the
cycle moved, both helpers lost their last caller, and the dead-code gate
said so.

Both are gone with it. The module's own `publish` — the board's publisher,
already there since W3.68 — publishes the **same** event: the same subject,
the same kind and the same payload row
(`workboardapi.TaskEventPayload`), because the tools read one shape.

That is the second thing this slice's shape decided: a move that takes the
last caller of a helper takes the helper too, and the **event's shape** does
not change on the way — only the place that speaks it.

The **wiring cost of zero** is the number worth reading twice: the board was
already injected everywhere, so the proof came with it. The two prior slices
paid **11** and **6** test sites for their new injection points.

## What is left

The work module now keeps the whole of the board's business: the lifecycle,
the goals and the judgment.

What is left of the runtime's dissolution stands at **240 verbs** in
**9,395 lines**. The nearest families are the **board's sixteen** (the
lifecycle's fifteen and the proof's one), the **workflow host's twelve** and
the **reasoning module's six** — and the next candidate is not another slice
of the work module but the **scope's leaf contract**: the surface family that
the leaf's own contract should own, whose first slice is the **files** family.

The self-repair module's edge into the runtime is still in the archcheck
allowlist — it is the other program's work, and it waits there.
