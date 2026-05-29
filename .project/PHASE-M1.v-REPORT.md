# Phase Report — Milestone 1.v (Planner v1 — LLM-generated DAGs)

> Status: **shipped** · Date: 2026-05-29
> Per SPEC-02 §4 (multi-step plans), DECISIONS B0d (scheduler sits
> above the agent loop), and the "scheduler is in but plans must
> be hand-authored" gap that's been open since M1.e.
> Continues [PHASE-M1.u-REPORT.md](PHASE-M1.u-REPORT.md).

## Scope

The DAG scheduler shipped in M1.e and the control-plane plumbing
to drive it shipped in M1.e too. But until M1.v, operators had to
write the plan JSON by hand — there was no path from a
natural-language intent to a multi-step DAG. M1.v closes that:

```
agt plan generate "research and summarise topic X"
agt plan run      "research and summarise topic X"
```

A single LLM call, asked to emit JSON in the same shape `agt plan
<file>` has been executing all along, with strict post-hoc
validation so a bad plan from the model surfaces as a clear
"your planner emitted X — fix the prompt" rather than a confusing
scheduler error mid-run.

Deliberately **not** an agentic-meta thing. The planner is a
single Provider.Complete call: no recursion, no tools-during-
planning, no re-plan mid-execution. Future iterations can add
those, but each is a separate audit-story problem.

| Concern | Status |
|---|---|
| `planner.Generate(ctx, cfg, intent) (rawJSON, Plan, err)` | ✅ |
| System prompt with explicit JSON schema + node-kind rules | ✅ |
| Two node kinds emittable: `loop` and `gate` | ✅ tested |
| Fenced ` ```json ` block extraction + bare-JSON fallback | ✅ tested |
| Validation: ≥1 node, unique non-empty ids | ✅ tested |
| Validation: every `deps` ref exists | ✅ tested |
| Validation: no self-deps | ✅ tested |
| Validation: no cycles (Kahn topological walk) | ✅ tested |
| Validation: kind ∈ {loop, gate}; loops need intent; gates need description | ✅ tested |
| `Config.SystemOverride` so operators can inject custom prompts | ✅ tested |
| Control-plane command `CmdPlanGenerate` (single LLM call → JSON) | ✅ tested |
| `Kernel.Provider()` accessor exposing the configured provider | ✅ |
| CLI: `agt plan generate "<intent>"` prints JSON | ✅ |
| CLI: `agt plan run "<intent>"` generates AND executes | ✅ |
| CLI: `agt plan <file.json>` (backwards-compatible) still works | ✅ |
| Help text updated | ✅ |

## Changes

### 1. `kernel/planner/planner.go` — new package (~290 LoC)

The planner is one focused package. Public surface is small:

```go
var SystemPrompt = "..."          // overridable
type Config struct{ Provider; Model; MaxTokens; SystemOverride }
type Plan struct{ Name; MaxParallel; Nodes []Node }
type Node struct{ ID; Kind; Deps; Intent; Capability; Description }

func Generate(ctx, Config, intent) (rawJSON string, Plan, error)
```

Three internal layers:

**A. LLM call.** A single `Provider.Complete` with `system=SystemPrompt`,
`user=intent`, `max_tokens=2048`. The prompt asks for "EXACTLY one
JSON object inside a fenced code block" and enumerates the schema
+ rules.

**B. JSON extraction.** `extractJSONBlock` strips the ` ```json ` or
` ``` ` fence; falls back to "treat the whole reply as JSON" when
the model omits fences (some tool-use-trained models do). Refuses
anything that isn't either fenced or a bare JSON object.

**C. Validation** — `parseAndValidate` + `validateDAG`. Every
failure mode is in the test table below.

Two design choices worth recording:

**Why duplicate validation that the scheduler already does.** The
scheduler's topological sort *will* refuse a cyclic plan — but at
that point we're past the planner boundary and the operator gets
a scheduler error mid-`agt plan run`. Catching the same issue in
`planner.Generate` makes the error say "your planner emitted a
cyclic plan; the prompt may need a 'no cycles' nudge" instead of
"scheduler: cycle detected." Two error messages for the same root
cause, but they point at different fixes — and the planner-side
one tells the operator something about *their* planner-prompt
quality, not their hand-written DAG quality.

**Why no LLM-side schema enforcement** (Anthropic structured
outputs, OpenAI JSON mode, etc.). Three reasons: (a) catalog
coverage isn't uniform — Ollama models don't have a JSON mode,
and we don't want to silently disable the planner on local
setups; (b) the failure mode of "model emitted JSON but it's
invalid" is *common enough* that schema mode wouldn't save many
trips; (c) keeping it provider-agnostic means swapping
providers (which the catalog encourages) doesn't change planner
behaviour.

### 2. `kernel/planner/planner_test.go` — new file (18 tests)

| Test | Locks in |
|---|---|
| `TestGenerate_HappyPath_TwoNodes` | End-to-end: fenced JSON → parsed Plan with name, 2 nodes, deps wired |
| `TestGenerate_SingleNodePlan` | Trivial 1-node plan works |
| `TestGenerate_BareJSONWithoutFence` | Fence-less response parses cleanly |
| `TestGenerate_GateNode` | Gate node round-trips with description preserved |
| `TestGenerate_RejectsMissingProvider` | Clear "Provider required" error before any LLM call |
| `TestGenerate_RejectsEmptyIntent` | Whitespace-only intent rejected at boundary |
| `TestGenerate_RejectsEmptyLLMResponse` | Empty completion → clear "empty response" error |
| `TestGenerate_RejectsNonJSONResponse` | "hi i'm friendly" → "not JSON" error with snippet |
| `TestGenerate_RejectsUnclosedFence` | ` ```json ` without closer → "no closing fence" |
| `TestGenerate_RejectsEmptyNodes` | `{"nodes":[]}` → "plan has no nodes" |
| `TestGenerate_RejectsDuplicateIDs` | Two nodes with same id → "duplicate id" |
| `TestGenerate_RejectsUnknownDep` | dep references missing id → "dep \"X\" does not exist" |
| `TestGenerate_RejectsSelfDep` | `deps: [self]` → "depends on itself" |
| `TestGenerate_RejectsCycle` | 2-node cycle → "plan has a cycle" |
| `TestGenerate_RejectsUnknownKind` | kind=frobnicate → "unknown kind" |
| `TestGenerate_RejectsEmptyLoopIntent` | loop with whitespace intent → "intent is empty" |
| `TestGenerate_RejectsEmptyGateDescription` | gate with empty description → "description is empty" |
| `TestGenerate_HonorsSystemOverride` | `Config.SystemOverride` reaches the provider's `req.System` |

The validation tests are intentionally exhaustive — every
failure mode is a category of bug a real model will produce
sooner or later, and each error message is what the operator
sees when their generated plan fails. Locking the messages
in keeps the operator-facing surface predictable.

### 3. `kernel/runtime/runtime.go` — `Provider()` accessor

```go
// Provider exposes the live agent.Provider so callers (notably
// the planner, which needs an LLM round-trip to generate a DAG)
// can reuse the kernel's configured routing without re-wiring
// catalog lookup. ... Hot reload via Replace updates this
// pointer's underlying chain atomically, so cached callers stay
// correct.
func (k *Kernel) Provider() agent.Provider { return k.cfg.Provider }
```

Critically, this returns the Governor (not the raw catalog-
constructed provider), so all the M1.s subscription-first
routing + M1.r hot-reload behaviour applies to planner calls
too. The planner does NOT get a special "use the cheap one"
override — that would shadow operator intent. If operators want
the planner to use a cheaper model, they pass `Config.Model`.

### 4. `kernel/controlplane/protocol.go` + `planner.go`

```go
const CmdPlanGenerate = "plan_generate"

func (s *Server) handlePlanGenerate(ctx, conn, req) {
    intent := req.Args["intent"]
    rawJSON, plan, err := planner.Generate(ctx,
        planner.Config{Provider: s.k.Provider()},
        intent)
    // → result: { plan_json: "<the JSON>", node_count: N }
}
```

Why a separate command rather than fold into `CmdPlan`: the CLI
composes `generate` then `run` cleanly with the existing
single-purpose endpoints. Combining them would either require a
"confirm before execute?" prompt over the wire (doesn't compose
with `agt pulse`, `--json` tooling) or fire blindly. Two small
endpoints keep operator control sharp.

### 5. `cmd/agt/main.go` — `agt plan` subcommand router

`agt plan` now dispatches:

```
agt plan generate "<intent>"  → cmdPlanGenerate → CmdPlanGenerate; print JSON
agt plan run      "<intent>"  → cmdPlanRun      → CmdPlanGenerate + CmdPlan
agt plan <file>               → cmdPlanExecuteFile → CmdPlan (backwards compatible)
```

The `run` path generates the plan, prints it, then forwards the
JSON to the existing `CmdPlan` machinery — same scheduler,
same events streamed, same `node_outputs` block at the end. The
operator sees both halves in the same terminal so they always
know what plan they're executing.

### 6. `kernel/controlplane/planner_test.go` — 3 integration tests

| Test | Coverage |
|---|---|
| `TestPlanGenerate_StreamsValidPlan` | Round-trip wiring works: intent → mock LLM → JSON back to client |
| `TestPlanGenerate_RejectsMissingIntent` | Missing arg → clear server error |
| `TestPlanGenerate_SurfacesPlannerValidation` | Planner rejection (bad dep) surfaces as a control-plane error, not a silent success |

The planner has thorough unit tests; the integration tests just
prove the server wiring forwards correctly.

## Test summary

```
go test ./kernel/planner/ -v -count=1
(18 passing — full validation table)

go test ./kernel/controlplane/ -v -count=1 -run TestPlanGenerate
(3 passing — wiring)

go test ./... -count=1
(all packages PASS)
```

Total suite: **475 passing** (from 454 after M1.u). +21 from
M1.v (18 planner + 3 controlplane).

## Operator workflow examples

**Inspect a plan before running it** (recommended for the first
few uses of a new planner-prompt or a new model):
```
agt plan generate "audit my repo for hard-coded secrets and propose fixes"
# prints JSON; operator reviews; optionally pipes to a file
agt plan generate "audit my repo for hard-coded secrets" > audit.plan.json
agt plan audit.plan.json
```

**Generate-and-execute in one go** (once the operator trusts the
planner for a class of intents):
```
agt plan run "summarise yesterday's commits and email the team"
generated 3-node plan:
{ "nodes": [
  {"id": "fetch_commits", "kind": "loop", "intent": "...", "deps": []},
  {"id": "summarise",     "kind": "loop", "intent": "...", "deps": ["fetch_commits"]},
  {"id": "send_email",    "kind": "loop", "intent": "...", "deps": ["summarise"]}
]}

--- executing ---
  [evt seq=N kind=plan.started ...]
  ...
```

**Pipe to jq for inspection**:
```
agt plan generate "X" | jq '.nodes | map(.id)'
[ "step1", "step2", "step3" ]
```

**Watch the plan as it runs**, in another terminal:
```
# terminal 1:
agt plan run "<intent>"
# terminal 2:
agt pulse --subject plan.>
```

## What's intentionally NOT in M1.v (Planner v1 → v2 deferrals)

- **Re-planning mid-execution.** If a loop fails or a tool result
  reveals the plan was wrong, v1 just fails the plan. Future:
  on node failure, optionally call the planner again with the
  current plan + the failure context to get a patched plan. Big
  audit-story implications (a plan that mutates itself); not v1.
- **Sub-planner / hierarchical plans.** A loop node deciding to
  spawn its own sub-plan. Same recursion concern as above.
- **Planner-issued tool calls.** Currently the planner cannot call
  tools (e.g. read files, query git, etc.) to inform the plan.
  Means today's planner is working from intent text alone. Adding
  it requires policy decisions about what the planner can touch.
- **Validation rules per-node-kind** (e.g. gate must have a `deps`
  list). v1 checks the universal rules; per-kind structural
  schema lives in the scheduler's node constructors.
- **Cost estimation / budget guard.** A generated plan with 10
  loop nodes silently costs ~10× a normal `agt run`. Future:
  pre-execution estimate so the operator sees "this plan will
  cost ≈$X based on average tokens-per-loop in the journal."
- **Plan templates / few-shot examples in the system prompt.**
  Adding good few-shots tends to lock the planner into specific
  patterns; we'd want to expose template choice as a flag rather
  than bake them in. Future.
- **Visual plan rendering** (mermaid / DOT). Easy add-on; just
  a different output format from `agt plan generate`. Out of
  scope for the engine wedge.

## Files touched

- [kernel/planner/planner.go](../kernel/planner/planner.go) — new (~290 LoC).
- [kernel/planner/planner_test.go](../kernel/planner/planner_test.go) — new (~250 LoC, 18 tests).
- [kernel/controlplane/planner.go](../kernel/controlplane/planner.go) — new (~65 LoC).
- [kernel/controlplane/planner_test.go](../kernel/controlplane/planner_test.go) — new (~70 LoC, 3 tests).
- [kernel/controlplane/protocol.go](../kernel/controlplane/protocol.go) — one new const.
- [kernel/controlplane/server.go](../kernel/controlplane/server.go) — one dispatch case.
- [kernel/runtime/runtime.go](../kernel/runtime/runtime.go) — one accessor (`Provider()`).
- [cmd/agt/main.go](../cmd/agt/main.go) — `cmdPlan` becomes a router; 3 helper funcs added; 2 help lines updated.

No changes to the scheduler, the agent loop, the bus, the
journal, the catalog, the providers, or the existing tools.
Planner sits cleanly on top of the existing primitives.

## Deferrals after M1.v

- **Planner v2** (re-planning, sub-planners, planner tools, cost
  estimation, templates — as above).
- **Bedrock SigV4** + non-Anthropic body shapes (M1.m.x).
- **OS-keychain vault encryption.**
- **Browser tool.**
- **Out-of-process plugin host.**

Picking up **Bedrock SigV4 + non-Anthropic body shapes** next —
the last per-provider feature gap that meaningfully limits
operator choice (Bedrock without SigV4 only works with the
bearer-token preview, and only for Anthropic models).
