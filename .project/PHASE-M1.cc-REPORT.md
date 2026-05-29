# Phase Report — Milestone 1.cc (Per-task-type routing override)

> Status: **shipped** · Date: 2026-05-29
> Closes the last DECISIONS C2 gap deferred since M1.b.
> Continues [PHASE-M1.bb-REPORT.md](PHASE-M1.bb-REPORT.md).

## Scope

DECISIONS C2 reads:

> Routing default order: subscription-first → **quality-floor-for-task-type** → cost → latency.

M1.b shipped subscription-first; the task-type slot was left empty
("Planner / task-type metadata" — deferred to a later phase). Every
phase report since M1.s has carried the deferral. M1.cc closes it.

The need is operator-visible: planning calls and chat-loop calls
have different latency budgets, different stakes if they go wrong,
and often different ideal models. An operator who wants their
*planner* pinned to Claude (for the JSON-formatting reliability)
while everything else round-robins through cheaper providers
should be able to express that without recompiling.

```
AGEZT_TASK_ROUTES="plan=anthropic;code=anthropic,openai;embed=ollama"
```

The Governor now consults this map when a CompletionRequest carries
a non-empty `TaskType`. Matching task types get the named providers
hoisted to the front of the routing chain (in the listed order,
registered-only). The rest of the chain follows in the default
subscription-first order.

| Concern | Status |
|---|---|
| `CompletionRequest.TaskType` field — opt-in, opaque to providers | ✅ |
| `Governor.Config.TaskRoutes` map[string][]string | ✅ |
| `applyTaskRoute` hoists registered providers in listed order | ✅ tested |
| Unknown provider names in route → silent skip (degrade gracefully) | ✅ tested |
| Empty route OR empty TaskType → no behaviour change vs. M1.s baseline | ✅ tested |
| Hoisted provider fails → chain continues through remaining providers | ✅ tested |
| `AGEZT_TASK_ROUTES` env var parser (`plan=foo;code=bar,baz`) | ✅ tested |
| Parser hard-errors on syntactic mistakes (no `=`, empty key) | ✅ tested |
| Whitespace tolerant; empty value deletes prior route | ✅ tested |
| Planner stamps `TaskType="plan"` on its LLM call | ✅ |
| Routing audit event includes `task_type` so `agt pulse` shows it | ✅ |
| Daemon banner shows route count when configured | ✅ |
| Zero impact on default behaviour when env unset | ✅ tested |

## Why hoist (soft preference), not filter (hard pin)

The implementation reorders the chain rather than restricting it:
even if `AGEZT_TASK_ROUTES="plan=anthropic"` is set, a Claude
outage still falls through to whichever other provider can answer.
Two reasons:

1. **Fallback story is already the kernel's promise.** The
   Governor's existing chain-walk on transient errors is what
   makes agezt robust when a single provider has a bad afternoon.
   Restricting the chain by task type would break that promise
   selectively (only for tasks that opted into routing) — surprising
   the operator at exactly the moment they need fallback most.
2. **Task types are operator intent, not safety constraints.** If
   the operator needs hard provider pinning (e.g. compliance:
   "code must NOT leave our VPC, so embed=local-only-or-fail"),
   that's a separate edict layer — the policy engine should reject
   the tool call before routing happens. Doing pinning at routing
   time hides the rejection in a place operators don't expect to
   find it.

Hard-pinning is therefore explicitly NOT in M1.cc. If demand
arises, a future `governor.TaskRouteRequire` map (parallel to
`TaskRoutes`) would express it cleanly without re-litigating
the existing semantics.

## Files

### `kernel/agent/agent.go` — added one field

```go
type CompletionRequest struct {
    Model     string
    System    string
    Messages  []Message
    Tools     []ToolDef
    MaxTokens int
    TaskType  string  // NEW: opaque to providers, consulted by Governor
}
```

Opaque to providers, zero-value backwards compatible. Every
existing test and provider compiles unchanged.

### `kernel/governor/routes.go` — new file (~150 LoC)

- `TaskRoutes` type: `map[string][]string` (task-type → ordered
  preference list of provider names).
- `parseTaskRoutesEnv(spec string)` / exported `ParseTaskRoutesEnv`:
  decodes the `AGEZT_TASK_ROUTES` syntax (`a=p1,p2;b=p3`).
  Returns hard errors only on syntax problems (missing `=`,
  empty key); unknown provider names are tolerated.
- `applyTaskRoute(chain, routes, taskType)`: pure function that
  returns a reordered chain. Listed-and-registered providers get
  hoisted to the front in listed order; the rest of the chain
  follows in original order, with no duplicates.

### `kernel/governor/governor.go` — three small edits

- `Config.TaskRoutes` field added.
- `routeChain` now calls `applyTaskRoute` after the
  subscription-first sort, when both TaskRoutes is non-empty AND
  req.TaskType is non-empty.
- The `routing.decision` audit event payload now includes
  `task_type`, so `agt pulse --kind routing.decision` shows which
  task-type overrides actually fired (and which didn't).

### `kernel/planner/planner.go` — exports `TaskType`, stamps it

```go
const TaskType = "plan"

// In Generate:
req := agent.CompletionRequest{
    ...,
    TaskType: TaskType,
}
```

Exported as a constant so a future tool that wants to invoke the
planner via its public interface can re-use the same key without
guessing at the string. (The string itself is operator-facing —
it's what they write in `AGEZT_TASK_ROUTES`.)

### `cmd/agezt/main.go` — env wiring

Parses `AGEZT_TASK_ROUTES` once at daemon startup. Syntactically
malformed entries are a hard startup error (operator gets fast
feedback). Unknown provider names are passed through; the silent-
skip semantics live in `applyTaskRoute` itself, which is the right
place because the registered providers may change at runtime via
the M1.r hot-reload path.

Banner shows `task_routes=N` when non-empty so operators see
the routing is actually loaded.

### `kernel/governor/routes_test.go` — new (~280 LoC, 10 tests)

| Test | Locks in |
|---|---|
| `TestParseTaskRoutesEnv_Basic` | Happy-path parse of `a=p1;b=p2,p3;c=p4` |
| `TestParseTaskRoutesEnv_Whitespace` | Tolerates shell-quote whitespace around tokens |
| `TestParseTaskRoutesEnv_Empty` | Empty / whitespace-only spec → nil, no error |
| `TestParseTaskRoutesEnv_BadEntry` | Hard error on no-`=`, empty-key entries |
| `TestParseTaskRoutesEnv_EmptyValueDeletes` | `plan=anthropic;plan=` removes prior route |
| `TestGovernor_TaskRouteHoistsPreferredProvider` | Route overrides subscription-first ordering for matching task; default applies for empty TaskType |
| `TestGovernor_TaskRouteFallsThroughOnFailure` | Hoisted provider's error still falls through to next in chain (fallback story intact) |
| `TestGovernor_TaskRouteIgnoresUnknownProvider` | Route listing nonexistent provider → degrade to default chain (no daemon failure) |
| `TestGovernor_TaskRoutePreservesOrderInList` | Multi-provider route tries them in listed order, then default-order remainder |
| `TestGovernor_TaskRouteOnlyAppliesToMatchingType` | "plan" route doesn't bleed into "code" tasks |

## Operator workflow examples

**Pin the planner to Claude, let everything else use the default chain:**

```
export AGEZT_TASK_ROUTES="plan=anthropic"
agezt
```

Planner-emitted DAGs go to Anthropic; chat-loop calls inside the
DAG nodes still run through whichever provider the subscription-
first chain selects.

**Different providers per task type:**

```
AGEZT_TASK_ROUTES="plan=anthropic;code=anthropic,openai;embed=ollama" \
agezt
```

`plan` → Anthropic. `code` → Anthropic then OpenAI on fallback.
`embed` → local Ollama. Any other task type uses default.

**Verify a route is firing:**

```
agt pulse --kind routing.decision | jq '.event.payload | {task_type, chain}'
{ "task_type": "plan", "chain": ["anthropic", "openai", "mock"] }
{ "task_type": "code", "chain": ["anthropic", "openai", "mock"] }
{ "task_type": "",     "chain": ["anthropic", "openai", "mock"] }
```

The third line shows a default-routed call — anthropic still
first because subscription-first put it there, not because of any
route. Useful for confirming that "plan=anthropic" isn't just
mirroring what default would have done anyway.

## Test summary

```
go test ./kernel/governor/ -v -count=1 -run 'TestParseTaskRoutes|TestGovernor_TaskRoute'
(10 tests — all PASS)

go test ./... -count=1
(35 packages — all PASS, 477 unique top-level tests)
```

(+10 from M1.cc on top of M1.bb's 467.)

## What's intentionally NOT in M1.cc

- **Hard provider pinning** (`TaskRouteRequire`). See the "soft
  preference" rationale above — if a real compliance use case lands,
  add it as a separate orthogonal field, not by changing the
  existing TaskRoutes semantics.
- **Per-task-type model override.** Routing picks the provider;
  the model id still comes from `req.Model`. A future config could
  let `code=anthropic:claude-opus-4-7` set both — but that
  conflates routing and model selection in one knob, which makes
  the failure mode "model X not available on provider Y" harder
  to surface clearly. Keep model selection where it is (catalog
  + planner / loop config).
- **Per-task-type budget caps.** Daily ceiling is still global.
  Per-task-type budgets ("max $5/day on `code`") would be a
  legitimate extension; not in scope here.
- **TaskType validation.** The string is free-form. Operators
  could write `AGEZT_TASK_ROUTES="typo=anthropic"` and silently
  never have it apply. We considered validating against a known
  set of task types (`plan`/`code`/`embed`/etc.) but rejected it:
  the set is application-specific (a custom planner can emit
  whatever TaskType it wants), and a validation list would either
  be too restrictive or so permissive it caught nothing.
- **`agt route` CLI.** No CLI inspector for current routing config.
  Operators read it back from the daemon banner / via
  `agt pulse --kind routing.decision`. A future `agt route show`
  could pretty-print the configured map.
- **Reverse-engineering task type from the prompt.** The kernel
  never tries to infer task type from the message content. Setting
  TaskType is callers' responsibility — the planner does it
  explicitly; the chat loop leaves it empty (and routes by
  subscription tier alone). This keeps the routing decision
  auditable: TaskType is operator-set, not LLM-decided.

## Files touched

- [kernel/agent/agent.go](../kernel/agent/agent.go) — added `TaskType` field to `CompletionRequest`.
- [kernel/governor/routes.go](../kernel/governor/routes.go) — new, TaskRoutes type + parser + `applyTaskRoute`.
- [kernel/governor/governor.go](../kernel/governor/governor.go) — `Config.TaskRoutes`, chain reorder, audit event field.
- [kernel/governor/routes_test.go](../kernel/governor/routes_test.go) — new, 10 tests.
- [kernel/planner/planner.go](../kernel/planner/planner.go) — exported `TaskType="plan"`, stamps on req.
- [cmd/agezt/main.go](../cmd/agezt/main.go) — `AGEZT_TASK_ROUTES` env parsing + banner.

Zero changes to providers, bus, journal, agent loop, scheduler, or
any plugin. Routing is still a pure Governor concern.

## Deferrals after M1.cc

The DECISIONS C2 routing slot is now filled. Unchanged from M1.bb's
list, minus the per-task-type item just shipped:

- Pulse v3+ (TUI dashboard, until/last flags, replay rate limit,
  subject indexing).
- Planner v2 (re-planning, sub-planners, planner-side tools).
- Plugin sandboxing, signing, hot-reload, streaming, callbacks.
- Browser: JS rendering, screenshots, search, cookies.
- AWS credential-provider chain.
- Non-Anthropic body shapes on Bedrock.
- Vault: OS-keychain integration, passphrase rotation, argon2.
- MCP bridge v2 (resources/sampling/progress/cancellation/SSE/image content).
- Routing extensions noted above (`TaskRouteRequire`,
  per-task-type budgets, per-task-type model overrides) — only if
  demand surfaces.
