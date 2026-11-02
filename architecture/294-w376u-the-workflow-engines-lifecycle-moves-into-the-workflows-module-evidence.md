# W3.76u — the workflow engine's lifecycle moves into the workflows module

**Slice:** W3.76u (the twenty-ninth of the runtime's dissolution)
**Base:** `45528d64` (the W3.76t merge)
**Date:** 2026-11-02

---

## 1. The family

The **workflow engine** is the family that runs the durable, named graphs: it
saves and restores a workflow, arms and disarms its triggers, removes it, runs
it (a trigger node, a tool node, an LLM node, an approval gate, a subflow…),
tests a single node against a data map, and asks the copilot to draft or refine
a graph from a plain-language instruction.

The engine is the **workflows module's** — `kernel/modules/workflows/workflow`
— and the module's `*Engine` has answered the contract's `workflowapi.Engine`
port since W3.48 (`var _ workflowapi.Engine = (*Engine)(nil)`).

What the daemon still held was **eight forwarders** over that engine's own
operations: `SaveWorkflow`, `RestoreWorkflow`, `SetWorkflowEnabled`,
`RemoveWorkflow`, `RunWorkflow`, `TestWorkflowNode`, `DraftWorkflow` and
`RefineWorkflow`. Each one did the same three lines: take the engine out of
the daemon, call it, return its answer.

## 2. The port the contract already had — twice

The **contract** named this family **twice**, in two spellings:

- `workflowapi.Engine` — the wide port, eight verbs: `Save`, `Restore`,
  `SetEnabled`, `Remove`, `Run`, `TestNode`, `Draft`, `Refine`;
- `workflowapi.Runner` — the narrow port the agent-facing plugin drives, three
  verbs: `SaveWorkflow`, `SetWorkflowEnabled`, `RunWorkflow`.

And the daemon added a **third** copy of the long ones. Three spellings of one
family, and a caller had to know which one it was holding.

**One vocabulary, two widths** is the shape the contract now has: the
**widest** port's spelling is the family's, and the narrow port is a **subset**
of it — `Runner` embeds into `Engine` rather than restating it. The plugin's
port is now the engine's port with five verbs missing, and the module's engine
answers both without an adapter.

The daemon's eight forwarders were the third copy, and they are **gone**.

## 3. The one thing that did not move: the provenance

The engine's `Run` took a `Provenance` argument — **why** the run exists. That
row is filled from the host's **wake context**, and a module may not read
another package's context keys (archcheck's principle P5). So the row cannot
travel as a parameter the module asks for; the **host** has to hand it over.

The slice made that a **port**: `workflowapi.Ports` gained
`Provenance func(ctx) Provenance` (the eleventh capability), and the daemon
fills it with the mapping it already had (`workflowProvenance`, reading the
wake context). The module asks the host; the host answers.

The engine's `Run` signature therefore lost its parameter — and the daemon's
`RunWorkflow` forwarder, which had hidden the mapping, is gone with it.

## 4. What moved

- **the module** answers the contract's engine vocabulary (its methods already
  did; only `Run`'s signature changed, because the provenance now comes through
  the ports);
- **the contract** has one spelling: `Runner` (three verbs) is a subset of
  `Engine` (eight verbs), and the refusal row `ErrNoEngine` moved in from the
  daemon's private error;
- **the daemon** keeps three things and no more: the library's **FILE**
  (`Workflows`), the engine's **WIRING** (`WorkflowPorts` — the host's own
  governed paths: its bus, its governor, its tool runner, its approvals) and
  the engine's **FACE** (`WorkflowEngine`). The daemon's own trigger path
  (`FireWorkflow`) stays: the wake context is the host's, so the host runs the
  trigger's workflow;
- **the application** binds the module's engine (`PortsFromKernel` fills
  `Writer` and `Designer` with `k.WorkflowEngine()`, and the run/test closures
  drive the engine) and a daemon with no engine holds **no** writer — the
  application then refuses with the contract's row (`workflowapi.ErrNoEngine`)
  instead of calling into nothing;
- **the plugin** binds the module's engine too (`workflowPort(k)` answers
  `k.WorkflowEngine()`), and the composition root's cadence path drives the
  engine.

## 5. The measurement

| what | before | after |
|---|---|---|
| the daemon's public verbs | 173 | **166** |
| the daemon's production lines | 5,899 | **5,813** |
| the daemon's production files | 54 | **54** |
| the app packages that import `kernel/runtime` | 40 | **40** |
| the workflows module's production lines | 1,951 | **1,972** |
| the workflow contract's production lines | 893 | **924** |
| the workflow application's production lines | 1,215 | **1,285** |

**Seven verbs off the daemon's surface in one slice** — the eight forwarders
gone, the module's face (`WorkflowEngine`) added.

The lines that went **up** are the ones that carry the conditions: the module
grew by twenty-one (the provenance port and its nil-guard), the contract by
thirty-one (the second port collapsed into the first, the refusal row moved
in), the application by seventy (the nil-guards and the halt-identity mapping
that the daemon's forwarders used to carry).

The app package count did not move, and the reason is on the record:
`kernel/app/workflow` still names the daemon for the families it owns besides
the engine — the library's file, the journal and the correlation mint — so the
engine's **binding** was narrowed, not the package's.

## 6. The ratchets — and the probes

Two ratchets, **both probes fired**:

- the daemon's new workflow ratchet (`kernel/runtime/workflow_ratchet_test.go`)
  pins that the eight forwarders do not come back, and that the daemon still
  holds the library's file, the engine's wiring and the engine's face;
- the application's new binding ratchet
  (`kernel/app/workflow/binding_ratchet_test.go`) pins that the workflow
  package does not bind the **daemon** as the engine's writer again.

The daemon's own surface ceiling (`kernel/runtime/dead_surface_ratchet_test.go`)
followed the floor again: **173 → 166** (220 → 201 → 194 → 179 → 173 → 166).

**Probe A** put the eight long-named forwarders back on `*Kernel`: the
retired-verb ratchet, the surface ceiling (174 > 166) and the workflow
ratchet all went red at once.

**Probe B** gave the daemon the engine's **short** names and bound the daemon
as the application's writer: the application's binding ratchet went red.

There is a third, sharper fact between the two probes: with the daemon's
forwarders gone, binding the daemon as the application's writer **does not
compile** — the daemon cannot satisfy the application's port, and the type
system says so before any test runs.

## 7. The coverage moved with the subject

The daemon's workflow coverage drove the daemon's verbs, so it went where the
verbs went:

- the **contract's** own tests now pin the family: the runner port has three
  methods, the engine port has eight, the runner is a **subset** of the
  engine, and the host's ports carry **eleven** capabilities (the provenance
  is the eleventh);
- the **module's** tests exercise the engine under the contract's names;
- the **daemon's** tests pin what the daemon still does — it holds the file,
  the wiring and the face, and it holds **no** verb per operation;
- the **application's** tests pin the refusal: a daemon with no engine holds
  no writer, and every service (save, arm, run, test a node, draft) refuses
  with the contract's row.

## 8. Verification

- the compiler (`go build ./...`, `go vet ./...`);
- the contract's own tests, the module's tests, the application's tests, the
  daemon's tests, the plugin's tests, the composition root's tests;
- the **full test suite** and **the thirteen gates** (`TOTAL FAILURES: 0`).

## 9. What is left

The workflow family is done: its file is the host's, its rules and its
lifecycle are the module's, and the application binds the module.

The same shape is waiting for the families that still have forwarders in the
daemon: the **MCP registry** (12), the **standing library** (9) and the
**roster's** remaining read doors (9). The MCP registry is the mixed one: the
registry's lifecycle is the module's, but the **attach/detach** verbs are the
host's (they hold live child processes).
