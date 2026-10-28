# W5.4r — the workflow family's ports, and the handle that finally reaches the application

## What the slice is

The **fifth** family of the port programme, and the first one where the
**handle itself** reaches the application.

The four families before it had a shape the application could hold without
the kernel: the council's panel, the roster's profiles, the standing library's
store, the schedule library's store. The workflow family is different — the
workflow services need to **save, restore, enable, remove, draft, refine, run
and test** workflows, and the kernel is what does those. There is no narrower
thing for the application to hold except the handle itself, seen through the
interfaces it already declares.

So the workflow family's ports moved into the application:

```go
// kernel/app/workflow/ports.go
type Ports struct {
	Reader      Reader                                          // the workflow library; NIL when there is no library
	Journal     Journal                                         // the journal the workflow's own story is folded from
	Writer      Writer                                          // save, restore, enable, remove
	Designer    Designer                                        // draft, refine
	Correlation func() string                                   // the correlation mint
	Run         func(context.Context, string, string, any) (RunResult, error)
	TestNode    func(context.Context, string, graphs.Workflow, string, map[string]any, any) (NodeResult, error)
	Entries     func() []graphs.Workflow                        // nil-safe listing
	Get         func(ref string) (graphs.Workflow, bool)        // nil-safe read
}

func PortsFromKernel(k *runtime.Kernel) Ports
```

The adapter keeps one binding — `kernel/controlplane/workflow_ports.go`:

```go
func (s *Server) workflowPorts() appworkflow.Ports {
	return appworkflow.PortsFromKernel(s.k)
}
```

## What moved

| | Before | After |
|---|---|---|
| the workflow reads | `appworkflow.NewReads(s.k.Workflows(), s.k.Journal(), nil)` | `appworkflow.NewReads(ports.Reader, ports.Journal, nil)` |
| the workflow lifecycle | `appworkflow.NewLifecycle(s.k)` — the **whole handle** | `appworkflow.NewLifecycle(ports.Writer)` |
| the workflow copilot | `appworkflow.NewCopilot(s.k.Workflows(), s.k, s.k.NewCorrelation)` | `appworkflow.NewCopilot(ports.Reader, ports.Designer, ports.Correlation)` |
| the workflow execution | `appworkflow.NewExecution(s.k.Workflows(), …)` + the kernel's run/test verbs spelled in the host | `appworkflow.NewExecution(ports.Reader, …)` + `ports.Run` / `ports.TestNode` |
| the impact derivation's workflows | a nil-check and a `List()` spelled in the adapter | `ports.Entries` |
| the schedule admission's workflow name | `s.k.Workflows().Get(ref)` | `ports.Get(ref)` |

**Thirteen** kernel verb sites left the adapter's production code, and the
adapter no longer hands the handle to an application constructor at all.

Two things are worth naming:

- **The host's own verbs stayed.** The execution host's `WithWake` (the
  contract's wake row) and `Panic` (the protocol's stderr line and the bus
  event) are the *adapter's* business — the protocol surface, not the
  application's.
- **The application's results are the application's.** The port's `Run` and
  `TestNode` answer in `RunResult` and `NodeResult`, the application's own
  rows: the kernel's `RunWorkflowResult` and `NodeTestResult` stop at the
  application's edge.

## The guard's fourth rule

The guard now holds **38** application-owned verbs: the council's four
(hand-written), the roster's nine (the family's accessor file), the standing
family's eight, the schedule family's five and the workflow family's
**twelve** (the name).

The workflow rule matters most, because this is the family where the handle
is in play: the application may reach the handle's workflow verbs **through
its port**, and the adapter may not reach them at all.

## The proof

### Permanent tests — 4 new

| Test | What it holds |
|---|---|
| `TestTheWorkflowPortsReachTheKernelWorkflows` (`kernel/app/workflow`) | the ports are **filled** (the writer, the designer, the correlation, the run ports) and the workflows the kernel holds are the workflows the application reads, shows and pauses |
| `TestTheWorkflowPortsSurviveADaemonWithNoWorkflowLibrary` (`kernel/app/workflow`) | the **nil-port discipline**: with no library the reader port is nil, the listing is empty and a read says "not found" |
| `TestTheAdapterBindsTheWorkflowPortsToItsOwnKernel` (`kernel/controlplane`) | the binding: the adapter's ports read the kernel's workflows, the roster impact's workflow port sees the same library, and the daemon's workflow reads service serves them |
| `TestTheGuardNamesEveryWorkflowVerbTheKernelHas` (`kernel/controlplane`) | the derivation — read from the kernel's own method set, independently of the helper the guard uses |

### The mutation run — 9 mutations, 9 caught

Two independent passes with **byte-identical** logs (SHA-256
`E4D5444FBC446AB297D80E785E7307EEAC64F88E91F2C33A2F5D3E7A6AD663C9`; the
elapsed times, ULIDs, epoch stamps, addresses and goroutine ids are
normalized, because they are not the mutation's fingerprint).

| Mutation | What it breaks | Caught by |
|---|---|---|
| M01 | the reader port is nil — the reads service answers "no workflows" | the ports test |
| M02 | the listing port answers nothing | the ports test |
| M03 | the read port answers "not found" for a workflow that is there | the ports test |
| M04 | the writer port is nil — the application cannot save a workflow | the ports test |
| M05 | the designer port is nil — the application cannot draft a workflow | the ports test |
| M06 | the correlation port answers empty — a run would have no name | the ports test |
| M07 | the adapter's reads get no reader | the binding test |
| M08 | the workflow rule loses its case — the family disappears from the guard | the guard test |
| M09 | the shared rule narrows to the setters — most verbs fall out of the guard | the guard test |

M04 and M05 are worth reading together: they are the two ports that are the
handle, narrowed. A port that is *nil* is not a narrow view — it is no view at
all, and an application that cannot save or draft a workflow fails at the first
operator command. The test says so in one line each: **the ports are filled**.

### The gates

| Gate | Result |
|---|---|
| `gofmt -l cmd internal kernel plugins sdk tools` | clean |
| `go vet ./...` | clean |
| `go run ./tools/archcheck` | OK — **2 allowlisted import violations and 0 allowlisted call sites remain** |
| `go run ./tools/deadcodecheck` | OK |
| `go run ./tools/depscheck` | OK |
| `go run ./tools/sdkparity -check docs/SDK-PARITY.md` | OK |
| `go run ./tools/structure-md -out .project/STRUCTURE.generated` + `-check` | clean |
| `go run ./tools/changelog-lint` | OK |
| `go run ./tools/docclaimscheck -base origin/main` | OK |
| `go run ./tools/docsindex` | OK |
| `staticcheck ./...` | clean |
| `go test ./... -count=1 -timeout=40m` | green |

## What the slice does not do

The **ratchet does not move yet**: the control plane still imports the runtime
in **15** files.

Five families are done — council, roster, standing, schedule, workflow — and
the shape of the remaining work is now clear. The next families (the tools and
MCP surface, the workboard and OKR surface) are the ones the family map calls
**verbs without libraries**: the adapter asks the kernel a question and the
application decides what the answer means. Those are the same shape as the
council's, and the council's slice is the template.

The four blockers the family map named — `Reflect()`, `LoopRunner()`,
`RunPlan()` and the raw handle — are the ones that will need a **port** rather
than a **wiring**: the application's port will have to be the module's own
interface, not the kernel's method set.
