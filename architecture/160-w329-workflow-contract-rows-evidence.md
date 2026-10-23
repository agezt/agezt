# W3.29 — the workflow rows move to the contract layer

**Date:** 2026-10-23
**Roadmap:** `architecture/21-migration-roadmap.md` §W3 — the plugin layer's
"reach into a module" edges.

**Scope:** the **workflow rows** — one durable, named graph, the typed nodes
it is made of, the edges that wire them, the trigger configuration that starts
it, the per-node config payloads, and the outcome of a run — and the **rule**
that decides whether a graph may run at all. The tool that authors and runs
workflows (`plugins/tools/workflowtool`) is a plugin, and a plugin may reach
the contract and the platform layers and nothing else.

## What moved

| Before | After |
|---|---|
| `kernel/modules/workflows/workflow` named `Workflow`, `Node`, `Edge`, `TriggerConfig`, the fifteen config types, `RunResult`, `NodeTestResult` and the node-type constants | → `kernel/contract/workflowapi` — the rows' new home |
| the graph rule (`Validate` and its validators: trigger configs, edge ports, reliability bounds, per-type configs, the acyclicity walk) | → the contract's. The rule is **pure** — stdlib only — so it belongs where the row is, and it is the rule the whole daemon enforces |
| the tool's own `Kernel` interface, naming `workflow.Workflow` and `runtime.RunWorkflowResult` | → the contract's rows. The plugin may not name the module and may not name the kernel |
| the tool's read path (`k.Workflows().List()` / `.Get()`) — a **concrete** store type reached through the kernel | → the contract's **port** (`workflowapi.Store`: `Get`, `List`), which the daemon hands over as a second argument to `Bind(k, store)` |
| `plugin-reach plugins/tools/workflowtool -> kernel/modules/workflows/workflow` | **gone** |
| `plugin-reach plugins/tools/workflowtool -> kernel/runtime` | **gone** |

This is the first slice that closes **two** plugin-reach edges at once: the
tool no longer names the module that keeps the rows, and it no longer names
the kernel either — the outcome it reports (`RunResult`) is a row of the
contract too.

The **store**, the **engine** (the graph walk, the per-node dispatchers, the
copilot, the single-node test), the **trigger runner** and the **templates**
stay in the module: they are the implementation. The module keeps the names
its callers use as **aliases** and its rule as a one-line **delegation** — so
there is one row and one spelling of it. The module's store now carries a
compile-time promise that it satisfies the contract's port.

## The ratchet went DOWN

**291 packages, 93 import violations, 13 call sites** — from 95/13. The two
**stale allowlist entries were deleted**.

## The wire matters here too

A workflow is **persisted** (`<home>/workflows/workflows.json`) and read back
by key — the CLI (`agt workflow list/show`), the console's canvas and the Web
UI all read it. So the row's JSON keys are a contract with every workflow file
already on disk, and a renamed key would silently drop that field.

The wire is now pinned key for key (the workflow's always-fields, the node's
always-fields and the optional ones that must stay **off** the wire when they
are empty, the edge's three fields), and the fifteen **node types** are pinned
too — a renamed type would orphan every stored graph that uses it.

## The rules are the daemon's rules, so they are measured rule by rule

The graph rule decides what may run: exactly one trigger, known node types,
edges that resolve, legal ports, per-type configs, and an acyclic graph. Every
refusal is measured individually — with its **own message**, because a rule
that refuses for the wrong reason is a rule nobody can debug — and every
**limit** is measured **at its boundary** (a name of exactly 64 characters
passes, 65 does not; a graph of exactly 100 nodes passes, 101 does not; a
node config of exactly 32 KiB passes, one byte more does not), because a cap
that is only measured at cap+1 would also pass an implementation that had no
cap at all. The limit **numbers** are pinned separately: a test that measures
a cap against the constant itself passes any value at all.

The rule's tests moved **with** the rule — they are the rule's proof, and a
proof that stays behind while the rule travels would be a proof of nothing.

## The interesting gap was the wiring

The read ops (`op=list`, `op=show`) need the library. The tool used to reach
it through the kernel (`k.Workflows()`), which is how a plugin got hold of a
concrete implementation. Now the library arrives as the contract's port — and
a port that nobody hands over is a port that is **silently nil**: the tool
would answer "not available on this daemon" on every live box, while every
test that builds the tool by hand stayed green.

So the daemon's wiring was pulled out into a named function
(`cmd/agezt/tooldeps.go` → `workflowStore`) and **measured there**: the daemon
must supply the port, and the port must answer the **live** library. The
**nil-port rule** is stated once, and the check is on the **concrete** type on
purpose — a nil store handed to an interface parameter is already a non-nil
interface.

## The proof

### Permanent tests

- **The wire shape** (`kernel/contract/workflowapi/wire_test.go`): the
  workflow's exact key set, the node's and the edge's, the always-fields, the
  optional fields that must fall out of the wire, the fifteen node types, the
  **port's** method set, and the limit numbers.
- **The rule** (`kernel/contract/workflowapi/workflow_test.go`, moved with
  the rule): every refusal with its own message — a bad name, no nodes, a
  missing or a second trigger, a duplicate id, a bad node id, an unknown
  type, an edge to a ghost, a self-edge, a trigger with incoming edges, a
  condition without a port, a port on a plain node, a cycle, and every
  per-type config (tool, llm, condition, transform, delay, http, code, map,
  filter, switch, merge, approval, subworkflow, pipeline) — plus the trigger
  configs (cron, event, webhook) and the **limits at their boundaries**.
- **The identity assertions** (`kernel/modules/workflows/workflow/contract_type_test.go`):
  the module's rows ARE the contract's rows, its node types ARE the
  contract's, its `ErrNotFound` IS the contract's, and its store IS the
  contract's port.
- **One rule, not two** (same file): the module's `Validate` and the
  contract's agree on every verdict — **and on the message**.
- **The plugin names the contract** (`plugins/tools/workflowtool/contract_type_test.go`):
  the tool walks its own kernel interface and its `Bind` signature and asserts
  that neither the module nor the kernel appears in them — a plugin may reach
  the contract and nothing else.
- **The tool's wire** (`plugins/tools/workflowtool/wire_test.go`): the view the
  agent reads is pinned key for key, the list carries its count, and the run
  reports the executed nodes and the node outputs.
- **The wiring, twice** (`cmd/agezt/tooldeps_test.go`,
  `plugins/builtintools/workflow_ports_test.go`): the daemon must supply the
  port and it must answer the **live** library; a workflow the agent saved
  through the tool must show up in that same library; and a daemon that
  handed over no library must leave the read ops reporting that — not calling
  into nil.

### Mutation run

**79 mutations: 77 caught, 2 equivalent (both proven unreachable).**

Seven mutations **survived the first round** — and every one of them was a
**test gap**, not a rule gap: an edge *from* a ghost node, a trigger feeding a
trigger, the node timeout bound, retries on a node that cannot fail, a
timeout on a trigger, the transform's empty template, the switch's `default`
port. The tests were strengthened (each case with **its own message** — a
substring that another rule also produces is not a measurement) and the
mutations were run again → caught.

Two mutations were **equivalent**: they aim at rules that cannot fire. The
"trigger cannot feed a trigger" check sits behind the "exactly one trigger"
rule, so a graph that would reach it is refused earlier; and the pipeline
step's `output_schema` size check sits behind the node's own config cap, so
the step's schema can never be the thing that is too big. Both were recorded
as equivalent and **re-authored** as reachable rules → caught. The dead
checks were left in place: this slice MOVES the rule, it does not change it.

One mutation had a **stale anchor** (the constant lives in the other file)
and was re-authored → caught. Two were **compile errors** (the mutation left
a variable unused) and were re-authored so the code still compiles → caught.
One was **misclassified** by the harness — a test message containing the word
"undeclared" was read as a compiler error — so the classifier was fixed to
look for a compiler message (`file:line:col`) and the mutation was run
again → caught.

| # | Mutation | Result |
|---|---|---|
| M01 | the name pattern's length limit is shortened | caught |
| M02 | the name pattern loses its length limit | caught |
| M03 | the node-id pattern's length limit is shortened | caught |
| M04 | a graph with no nodes is accepted | caught |
| M05 | the node cap is not enforced | caught |
| M06 | the edge cap is not enforced | caught |
| M07 | a bad node id is accepted | caught |
| M08 | a duplicate node id is accepted | caught |
| M09 | the node config cap is not enforced | caught |
| M10 | the per-type config check is skipped | caught |
| M11 | two trigger nodes are accepted | caught |
| M12 | an edge **from** a ghost node is accepted | **survived the first round** — the case was missing → caught |
| M13 | an edge to a ghost node is accepted | caught |
| M14 | a self-edge is accepted | caught |
| M15 | the edge-port check is skipped | compile error → re-authored → caught |
| M16 | "trigger cannot feed a trigger" is dropped | **equivalent** — unreachable behind the trigger-count rule → re-authored as the unknown-node-type rule → caught |
| M17 | a trigger with incoming edges is accepted | caught |
| M18 | the cycle check is removed | caught |
| M19 | the node cap's number is changed | caught |
| M20 | the edge cap's number is changed | caught |
| M21 | the config cap's number is changed | caught |
| M22 | the delay cap's number is changed | caught |
| M23 | the cron interval floor is not enforced | caught |
| M24 | the cron interval floor's number is changed | stale anchor → re-authored → caught |
| M25 | a bad `daily_at` is accepted | caught |
| M26 | the `daily_at` pattern accepts anything | caught |
| M27 | an event trigger without a subject is accepted | caught |
| M28 | a too-broad or self-referential subject is accepted | caught |
| M29 | the webhook secret's length is not enforced | caught |
| M30 | the webhook secret's length number is changed | caught |
| M31 | an unknown trigger kind is accepted | caught |
| M32 | a condition's port may be anything | caught |
| M33 | a switch's undeclared port is accepted | misclassified by the harness → classifier fixed → caught |
| M34 | a tool node stops being failable | caught |
| M35 | the node timeout bound is not enforced | **survived the first round** — the case was missing → caught |
| M36 | retries on a node that cannot fail are accepted | **survived the first round** → caught |
| M37 | a timeout on a trigger is accepted | **survived the first round** → caught |
| M38 | a tool node without a tool name is accepted | caught |
| M39 | an llm node without a prompt is accepted | caught |
| M40 | an unknown condition op is accepted | caught |
| M41 | a transform without a template is accepted | **survived the first round** → caught |
| M42 | a delay outside (0, 600] is accepted | caught |
| M43 | an http method other than GET/POST is accepted | compile error → re-authored → caught |
| M44 | an http node without a url is accepted | caught |
| M45 | a code node without language or code is accepted | caught |
| M46 | a map node without items or template is accepted | caught |
| M47 | a switch without cases is accepted | caught |
| M48 | a switch may use the reserved "error" port | caught |
| M49 | an unknown merge mode is accepted | caught |
| M50 | an approval without a description is accepted | caught |
| M51 | a subworkflow without a workflow name is accepted | caught |
| M52 | a pipeline without steps is accepted | caught |
| M53 | a pipeline step with a bad id is accepted | caught |
| M54 | a duplicate pipeline step id is accepted | caught |
| M55 | a pipeline step without a tool is accepted | caught |
| M56 | the pipeline step's `output_schema` size is not enforced | **equivalent** — unreachable behind the node's config cap → re-authored as the retries bound → caught |
| M57 | the `trigger` node type is renamed | caught |
| M58 | the node's `type` key is renamed | caught |
| M59 | the workflow's `enabled` key is renamed | caught |
| M60 | the edge's `from` key is renamed | caught |
| M61 | the node's `timeout_sec` stays on the wire when it is zero | caught |
| M62 | the workflow's `description` stays on the wire when it is empty | caught |
| M63 | the node's `label` stays on the wire when it is empty | caught |
| M64 | the module's rule stops delegating to the contract | caught |
| M65 | the module's `trigger` value disagrees with the contract | caught |
| M66 | `Bind` drops the library | caught |
| M67 | the view always reports "disabled" | caught |
| M68 | the view's node count is always zero | caught |
| M69 | the read ops stop reporting a missing library | caught |
| M70 | the run reports no executed nodes | caught |
| M71 | the daemon stops supplying the port | caught |
| M72 | the daemon's nil-kernel guard is removed | caught |
| M73 | the registry hands the tool a nil library | caught |
| M74 | the retry-delay bound is not enforced | caught |
| M75 | the switch's `default` port is refused | **survived the first round** — no accept case → caught |
| M76 | a pipeline step's `output_schema` may be any JSON | caught |
| M77 | the `error` port is refused on a failable node | caught |
| M16b | an unknown node type is accepted (M16 re-authored) | caught |
| M56′ | the retries bound is not enforced (M56 re-authored) | caught |

### Gates

The build, gofmt, vet, dead-code, archcheck (**291 packages, 93/13**),
structure, changelog-lint, docclaimscheck and staticcheck are green, and so
is the full test suite.

## One thing that moved with the rows: the parity ledger

The evidence ledger (`cmd/agt/compare_data.go`) names the files that back
each parity claim, and it asserts that those files **exist**. The workflow
rows' old path was named there — so the ledger was repointed at the rows' new
home. A stale path in that ledger is not a cosmetic bug: the ledger would
assert a claim about a file that is no longer there.

## What is deliberately left

- The **guardians** still reach into the roster module: they need
  `roster.Validate`, and the validation is not pure — it reads the trust
  ceiling through `kernel/edict`, and a contract package may not import a
  platform package. That is a slice of its own.
- The **runtime** still reaches into the workflows module, because it drives
  the engine (the graph walk, the copilot, the single-node test) and the
  store's lifecycle. Moving the engine to the app layer is a slice of its own.
- The remaining plugin-reach edges: the database tool, the workboard tool,
  the config tool, the ACP agent, the built-in tools and the built-in
  channels.
