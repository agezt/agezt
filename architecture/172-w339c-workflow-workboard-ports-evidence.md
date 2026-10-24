# W3.39c — the workflow and workboard runners become the contract's ports

**Date:** 2026-10-24
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the tool registry's **workflow row** and **workboard row** — the
two dependencies the workflow tool and the workboard tool drive.

## The same leak, twice

W3.39b moved the reasoning runners. The same shape was sitting in two more
places, and this slice takes both:

```go
// plugins/tools/workflowtool/tool.go — before
type Kernel interface {
	SaveWorkflow(corr string, w workflowapi.Workflow) (workflowapi.Workflow, bool, error)
	SetWorkflowEnabled(corr, ref string, enabled bool) (workflowapi.Workflow, error)
	RunWorkflow(ctx context.Context, corr, ref string, payload any) (workflowapi.RunResult, error)
}

// plugins/tools/workboardtool/workboard.go — before
type Kernel interface {
	CreateWorkboardTask(corr string, spec workboardapi.CreateSpec) (workboardapi.Task, bool, error)
	… thirteen verbs …
}
```

Both interfaces were already the right shape — a port, naming only the
contract's rows. Both lived in the **tool's** package, which is a plugin
(L6), and the registry is a module (L3): a module may not name a plugin's
type, so the registry's row had to be the **live kernel** instead of the
runner.

These two were the cleanest candidates of the five families: the workflow
tool's port names three verbs and the workboard tool's thirteen, and **every
row they name is already the contract's** (`workflowapi.Workflow`,
`workflowapi.RunResult`, `workboardapi.Task`, `workboardapi.CreateSpec`,
`workboardapi.RetryPolicy`, `workboardapi.RetryDecision`).

## What moved

- **New** `kernel/contract/workflowapi/runner.go`: `Runner` — three verbs.
- **New** `kernel/contract/workboardapi/runner.go`: `Runner` — thirteen verbs.
- The two tools keep their names as **aliases**, so every caller and every
  fake in every tool's test is unchanged:
  - `workflowtool.Kernel = workflowapi.Runner`
  - `workboardtool.Kernel = workboardapi.Runner`
- `kernel/toolreg`: `KernelDeps` gains `WorkflowRunner` and `WorkboardRunner`.
- `plugins/builtintools`: the two specs bind the new rows instead of `d.K`.
- `cmd/agezt`: the daemon supplies both ports (`workflowPort(k)`,
  `workboardPort(k)`) under the same nil-port rule the other ports follow.

The **read** side was already a port (`workflowapi.Store`, `workboardapi.Store`
— the W3.39a slice), and it stays one: the store is the read side, the runner
is the write-and-run side, and the tool needs both.

### The workboard port is thirteen verbs wide, and that is worth naming

Thirteen is a wide port. It is not a "host" interface that swallows the
kernel: it is the board's whole surface, and the workboard tool is a
first-class actor on it (it creates, claims, moves, links and reclaims
tasks). Every one of the thirteen names a contract row — the module's rows
are the contract's rows, by alias.

## The ratchet did NOT move — again, and for the same reason

**307 packages, 78 import violations, 13 call sites** — unchanged.

The registry's **rows** are still in the wrong layer. The last `plugin-reach`
edge (`plugins/builtintools -> kernel/toolreg`) falls only when the rows
(the `Spec`, the `Built`, the `BuildDeps`, the `KernelDeps`, the `LateDeps`
and the registry itself) live where a plugin may name them, and that needs
**every** field of every row to be a contract row or a contract port.

This slice removes the **second** of the five families that block it. Three
remain: the MCP row, the forge row (whose `approval.Decision` is a platform
type and would have to move), the schedule/standing/skill/journal row, and
the platform rows themselves — the warden, the bus and the journal, where the
bus and the journal carry `event.Event` and therefore need an **adapter**
rather than an alias.

## The proof

### Permanent tests

- **The contracts** (`kernel/contract/workflowapi/runner_test.go`,
  `kernel/contract/workboardapi/runner_test.go`): the workflow port has
  **exactly three** methods and the workboard port **exactly thirteen** — a
  new verb would be a new reach into the daemon and would have to be earned
  here; every type each port names is a contract row, a builtin or a
  standard-library type, and each signature is pinned argument by argument
  and answer by answer.
- **The tools** — the ports are **aliases**, so the tool's runner and the
  contract's port are the *same type*. The identity is asserted in
  `plugins/builtintools/workflow_workboard_ports_test.go` by comparing the
  **types themselves** (`reflect.TypeOf`), not by an assignment: an
  assignment would accept a copy with the same method set, while two
  interface types with identical method sets are still different types.
- **The plugin** (same file): both specs bind the port, and the fakes
  **record the call** — "the tool did not complain" is not enough, a tool
  could report unavailability for another reason and still look wired; and
  without the port the two tools say they are unavailable rather than
  calling into nothing.
- **The daemon** (`cmd/agezt/tooldeps_test.go`): the composition root
  supplies both ports, they **are the live kernel's**, and they stay **nil**
  ports when there is no kernel.

### The tests that had to change, and why

Two existing tests (`workflow_ports_test.go`, `workboard_ports_test.go`) had
to be re-pointed at the new row: they measured the **store** port's wiring
and had been reaching the live kernel through `d.K` to get the runner as
well. The runner now arrives through its own port, so the tests supply it
there. What they measure — the store port carrying the live implementation —
is unchanged, and their assertions are unchanged.

### Mutation run

**8 mutations: 8 caught.** No mutation survived, none produced a compile
error after the re-authoring below, and none was skipped on a stale anchor.

| # | Mutation | Result |
|---|---|---|
| M01 | the workflow spec does not bind the port | caught |
| M02 | the workboard spec does not bind the port | caught |
| M03 | the daemon does not supply the workflow port | caught |
| M04 | the daemon does not supply the workboard port | caught |
| M05 | the workflow port stops being nil-safe | caught |
| M06 | the workflow tool declares its own runner copy | caught |
| M07 | the workboard tool declares its own runner copy | caught |
| M08 | the workflow port loses a verb | caught |

M07 was **re-authored** after the first round: the mutation declared only one
of the thirteen verbs, which produced a *compile error* rather than a
measured failure — the tool calls the other twelve. The re-written mutation
declares the whole port (and the `time` import it needs), and the identity
assertion caught it.

There is **no gate-measured mutation in this slice**, for the same reason as
W3.39b: the registry's row for the live kernel is a **same-module** edge
(`kernel/toolreg` and `kernel/runtime` are both the "runs" module), so
archcheck cannot see it — there is no violation for it to see. The shape is
pinned by the **compiler** instead (the kernel must implement the ports, or
nothing builds) and by the shape tests.

### Gates

The build, gofmt, vet, dead-code, archcheck (**307 packages, 78/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so is
the full test suite.

## What is deliberately left — the three families

1. **the MCP row** — `mcptool.Kernel` already names only `mcpapi` rows, so it
   is a clean candidate like these two;
2. **the schedule, standing and skill rows** — the tools that read the
   daemon's schedules and standing orders;
3. **the platform rows** — the warden (whose rows would move to the contract
   layer as aliases), the bus and the journal, which carry `event.Event` and
   therefore need an **adapter** rather than an alias.

The forge row (`forgetool.Kernel`) is a fourth: its port names the toolforge
rows plus `approval.Decision`, which is a platform type — the port can move
only after that row moves with it.
