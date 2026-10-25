# W3.48 — the workflow library is the daemon's file

## What the slice is

The kernel still reached **14** modules after W3.47. This slice takes **two**
of those edges — and they are the same family:

| Edge | Why it existed |
|---|---|
| `cross-module kernel/runtime -> kernel/modules/workflows/workflow` | the kernel held the concrete `*workflow.Store`, **opened the file** itself and **built the engine** |
| `adapter-bypass kernel/controlplane -> kernel/modules/workflows/workflow` | the control plane named the module's rows |

The ratchet went **51 → 49**.

The workflow edge was the **widest** one left: the kernel held the store AND
the engine, and it named four module rows (`workflow.Engine`, `workflow.Ports`,
`workflow.Provenance`, `workflow.Trigger`) that had **no contract home at
all**. This is the first slice where a port had to be built around an
*implementation* the kernel was not allowed to name.

## What the kernel used to do

```go
// kernel/runtime/runtime_kernel.go
workflows *workflow.Store
wfEngine  *workflow.Engine
// kernel/runtime/compose.go
wfstore, err := workflow.OpenStore(filepath.Join(cfg.BaseDir, "workflows"))
// kernel/runtime/workflow_host.go
func (k *Kernel) Workflows() *workflow.Store { return k.workflows }
```

The engine was the kernel's own, built over the kernel's own ports:

```go
k.wfEngine = workflow.NewEngine(k.workflows, workflow.Ports{ ... })
```

That is the part that made this slice different from W3.47. The board's store
was a **file**; the workflow edge carried an **engine** — and the engine's
ports are built from the kernel's own state (its bus, its governor, its tool
runner, its approvals). So the composition root cannot build the engine
*before* the kernel exists, and the kernel cannot build it *without* becoming
a module.

## What it does now

### The port is the contract's — the engine is a PORT

```go
// kernel/contract/workflowapi/engine.go
type Engine interface {
    Save(corr string, w Workflow) (Workflow, bool, error)
    Restore(corr string, w Workflow, reason string) (Workflow, bool, error)
    SetEnabled(corr, ref string, enabled bool) (Workflow, error)
    Remove(corr, ref string) (bool, error)
    Run(ctx context.Context, corr, ref string, payload any, prov Provenance) (RunResult, error)
    TestNode(ctx context.Context, corr string, w Workflow, nodeID string, data map[string]any, payload any) (NodeTestResult, error)
    Draft(ctx context.Context, corr, name, description string) (Workflow, error)
    Refine(ctx context.Context, corr string, base Workflow, instruction string) (Workflow, error)
}
```

**Eight** verbs — the width the host actually drives: the canvas saves,
restores, arms, disarms, removes, runs and tests a graph, and the copilot
drafts and refines one.

The module answers the port with a compile-time assertion, so a port change
breaks in the module that implements it:

```go
// kernel/modules/workflows/workflow/aliases.go
var _ workflowapi.Engine = (*Engine)(nil)
```

### The read port stays narrow

The **plugin** (the agent-facing `workflow` tool) keeps the **read side only**
— `workflowapi.Store`, two verbs (`Get`, `List`). A plugin that could remove
or disable a stored workflow would be editing the daemon's library.

The **host** holds `workflowapi.HostStore` (the read side plus the four
lifecycle verbs), and that is what `cfg.Workflows` carries — because the
composition root builds the engine over exactly that store.

The kernel's own **face** is the read side, and the test says so *statically*:
a widened face would still satisfy the read port (the host port is a
superset), so only the declared return type can tell the two apart.

### The rows that travel with the port moved to the contract

| Row | Why it moved |
|---|---|
| `workflowapi.Ports` | the host capabilities the engine calls out to — ten closures, all the host's own governed paths |
| `workflowapi.Provenance` | why a run exists; the host fills it in from its own wake context |
| `workflowapi.Trigger` | what fired a workflow; the host's trigger runner carries it |
| `workflowapi.ErrHalted` | the HOST reports the halt, and the host may not name a module's error |
| `approvalapi.SubmitSpec` / `approvalapi.Outcome` | the approval rows the engine's port carries; the **platform** (`kernel/approval`) keeps its names as aliases |

The module keeps every name as an **alias** and every rule as a
**delegation**, so there is one row and one spelling of it.

### The engine is the daemon's — and it is built AFTER Open

```go
// cmd/agezt/main_toolinvoker.go
if cfg.Workflows == nil {
    store, err := workflow.OpenStore(filepath.Join(cfg.BaseDir, "workflows"))
    if err != nil {
        return nil, err
    }
    cfg.Workflows = store
}
// ... after Open, because the ports are the kernel's own:
k.SetWorkflowEngine(workflow.NewEngine(cfg.Workflows, k.WorkflowPorts()))
```

The engine is built **after** `Open` on purpose: its ports are the kernel's
own (bus, governor, tool runner, approvals, model resolution), so a workflow
run keeps the host's policy checks, its audit arc and its spend attribution.
The library's file is the daemon's, and a library that cannot be opened is a
**startup error** — the workflow library is part of the daemon, not a
convenience.

### A missing engine is an answer, not a panic — in TWO layers

The engine used to be built by the kernel, so `k.wfEngine` was **never nil**
and the code never had to say what happens without one. Now a host may open a
kernel without an engine, so two layers had to learn the rule:

- the **kernel**: all eight lifecycle operations go through
  `workflowEngine()`, which refuses with *"the workflow engine is not
  available on this daemon"*;
- the **app layer** (`kernel/app/workflow`): the reads, the execution (run and
  webhook) and the copilot hold the library, and each of them refuses with
  *"the workflow library is not available on this daemon"* instead of
  dereferencing nil.

The two sentences are deliberately different: the **engine** is what the
kernel drives, the **library** is what the app layer reads. One rule, two
surfaces, each naming its own missing thing.

## The proof

### Permanent tests

| Test | What it holds |
|---|---|
| `kernel/contract/workflowapi`: `TestTheEnginePortCarriesTheEightVerbs` | the engine port is **exactly** eight verbs wide |
| `kernel/contract/workflowapi`: `TestTheReadPortIsExactlyTwoVerbs` | the plugin's read port is **exactly two** verbs wide |
| `kernel/contract/workflowapi`: `TestTheHostStoreIsWiderThanTheReadPort` | the host store is a **superset** of the read port — the two shapes cannot drift apart |
| `kernel/contract/workflowapi`: `TestTheHostPortsCarryTheTenCapabilities` | the engine's host capabilities are **exactly ten**, with their signatures pinned |
| `kernel/contract/workflowapi`: `TestTheProvenanceRowCarriesTheWakeContext` | the provenance carries its **seven** wake fields |
| `kernel/contract/workflowapi`: `TestTheTriggerRowCarriesWhatFired` | the trigger carries its **three** fields |
| `kernel/contract/approvalapi`: `TestTheSubmitRowCarriesWhatTheOperatorMustSee` | the approval rows the port carries keep their **eighteen** fields |
| `kernel/approval`: `TestTheSubmitRowsAreTheContractRows` | the platform's names **are** the contract's rows |
| `kernel/modules/workflows/workflow`: `TestTheEngineIsTheContractsPort` | the module answers the contract's engine port |
| `kernel/modules/workflows/workflow`: `TestTheStoreIsTheContractsHostPort` | the module's store answers the host port |
| `kernel/modules/workflows/workflow`: `TestThePortsRowsAreTheContractRows` | `Ports`, `Provenance` and `Trigger` **are** the contract's rows |
| `kernel/runtime`: `TestTheWorkflowRefusesWithoutAnEngine` | **all eight** kernel lifecycle operations refuse instead of panicking |
| `kernel/runtime`: `TestTheWorkflowEngineDrivesTheHostPort` | the kernel drives the engine through the port, and the row lands in the **live** library |
| `kernel/runtime`: `TestTheWorkflowReadPortIsTheReadSide` | the kernel's face is **statically** the read side, not the host port |
| `kernel/runtime`: `TestTheWorkflowLibraryStaysANilPortWithoutOne` | a kernel without a library hands over a **nil** port |
| `kernel/app/workflow`: `TestTheAppLayerRefusesWithoutALibrary` | the reads, the execution and the copilot refuse instead of panicking |
| `cmd/agezt`: `TestOpenAppKernelWiresTheWorkflowLibrary` | the daemon end: the engine is live and the library's file is **on disk, under the daemon's base directory** |
| `cmd/agezt`: `TestTheWorkflowEngineIsBuiltOverTheInjectedLibrary` | the engine is built over the **injected** library, not over one opened behind the caller's back |

### The mutation run

**8 mutations: 8 caught, 0 survived, 0 skipped, 0 build errors** — in **two**
independent passes, both honest (every catch named the expected test or the
expected edge).

| # | Mutation | Caught by |
|---|---|---|
| 1 | the control plane names the module's rows again | **archcheck** — `adapter-bypass kernel/controlplane -> kernel/modules/workflows/workflow` |
| 2 | the runtime names the module's rows again | **archcheck** — `cross-module kernel/runtime -> kernel/modules/workflows/workflow` |
| 3 | a stale allowlist entry comes back | **archcheck** — a listed edge that no longer occurs |
| 4 | the kernel stops refusing without an engine | `TestTheWorkflowRefusesWithoutAnEngine` |
| 5 | the engine setter drops the engine | `TestTheWorkflowEngineDrivesTheHostPort` |
| 6 | the daemon does not hand the engine over | `TestOpenAppKernelWiresTheWorkflowLibrary` |
| 7 | the daemon builds the engine over nothing | `TestOpenAppKernelWiresTheWorkflowLibrary` |
| 8 | the kernel's face widens to the host port | `TestTheWorkflowReadPortIsTheReadSide` |

**Three of the eight are gate-measured.** "The kernel's reach is smaller" is
not something a test can see: a test would happily accept a kernel that
builds the engine itself, as long as the engine works.

The run was honest on the **first** pass — which is the harness earning the
W3.43/W3.46 fixes: the package and the `-run` filter are separate arguments,
a build error is not a catch, and an anchor that does not hold is a skip.

Mutation **8** is the one worth naming: it widens the kernel's face from the
read side to the host port — a change that **still compiles** (the host port
is a superset) and that no runtime behaviour would notice. Only a **static**
check on the declared return type catches it. A port's width is a contract,
and a contract has to be measured, not inferred.

### Gates

| Gate | Result |
|---|---|
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | **OK: 311 packages placed; 49 allowlisted import violations and 13 allowlisted call sites remain (ratchet)** — **down from 51** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `staticcheck ./...` | clean |
| `go run ./tools/structure-md -check` | clean |
| `go run ./tools/changelog-lint` | clean |
| `go run ./tools/docclaimscheck -base origin/main` | clean |
| `go test ./... -count=1` | **all green** |

## The rule the slice is really about

**A port's width is decided by the weakest holder of it.** The plugin reads
the library (`Get`, `List`); the host drives it (`HostStore`, eight verbs on
the engine). Both shapes are pinned by a test, and the *kernel's own face* is
pinned **statically** — because the width is a contract, and a contract that
is only inferred from behaviour is a contract that can drift.

**An implementation that needs the host's own capabilities cannot be built by
the host.** The engine's ports are the kernel's bus, governor, tool runner and
approvals — so the engine is the **composition root's**, built after `Open`
over the kernel's ports, and handed over as a port. The kernel holds the
*shape*; the daemon holds the *thing*. That is the same arc as the reasoning
module and the reaper scanner, one layer up: the host names the port, and the
composition root names the implementation.

**Every test that expected a store became a host.** The kernel opened the
library before; now every test that needs one opens it, injects the port and
builds the engine over its own kernel's ports. That is the same shift W3.43
(W3.47) made for the artifacts and the board — and it is the test suite that
shows the change is real: a test cannot be a host and a user at the same time.

## What remains

The kernel still reaches **12** modules. The next candidates, in the order the
measurement suggests:

- **`cadence`, `standing`, `mcp`** — many callers each; the shape is known (a
  contract port the host opens), the work is in the port's width.
- **`memory`, `worldmodel`, `roster`, `skill`, `okr`, `taste`, `catalog`** — the
  kernel holds each module's concrete store. Each one is the same slice: a
  contract port, the daemon opening the file, and the kernel's accessor
  carrying the port.
