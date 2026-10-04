# 04 — Agent Runtime (kernel/runtime, kernel/agent, tool execution, delegation, resume, proof)

**Scope:** `kernel/runtime` (81 non-test files) and its sub-packages `kernel/runtime/{accessors,compose,lifecycle,runexec,types}`; the agent loop `kernel/agent`; and the satellite packages `kernel/toolexec`, `kernel/toolreg`, `kernel/toolforge`, `kernel/contextselect`, `kernel/convo`, `kernel/delegation`, `kernel/intent`, `kernel/intervention`, `kernel/planner`, `kernel/reflect`, `kernel/resume`, `kernel/meshctx`, `kernel/proof`, `kernel/assure`.

Everything below was read from the working tree on disk (2026-10-02), not from `docs/`. Where a code comment contradicts the code, the code wins and the discrepancy is listed under "Gotchas".

Sibling docs: governor, Edict, approvals, warden: [05-governance-routing-security.md](05-governance-routing-security.md). Journal, bus, event kinds, memory, worldmodel, workboard, okr: [06-data-memory-state.md](06-data-memory-state.md). Control-plane `handleRun`, REST, OpenAI API: [03-control-plane-and-http.md](03-control-plane-and-http.md). Daemon boot, `runtime.Config` population from env, the boot-time resumer: [01-daemon-boot-cmd-agezt.md](01-daemon-boot-cmd-agezt.md). Concrete tools: [10-tools.md](10-tools.md). Providers: [08-providers.md](08-providers.md). Scheduler, cadence, standing, pulse, MCP, workflow: [07-autonomy-and-extensibility.md](07-autonomy-and-extensibility.md).

---

## 1. Responsibilities at a glance

| Concern | Owner | One-liner |
|---|---|---|
| Single-agent tool loop | `kernel/agent.Run` | model request, then tool gate/execute/finalize, then repeat. Panic firewall, iteration and cost caps, auto-continue, compaction, steering, checkpoint. |
| Provider and Tool contracts | `kernel/agent` (`Provider`, `StreamingProvider`, `Tool`, `ToolDef`, `Result`) | Every LLM adapter and every tool in the repo implements these. |
| Composition root | `runtime.Open` (`compose.go`) | Opens ~20 stores under `AGEZT_HOME`, builds `*Kernel`, registers in-process tools (memory/world/delegate/market/voice/image/rerank), starts the agentgw listener. |
| Run engine | `runexec.Runner.RunWith` (+ `RunAssured`, `RunWithRetry`) | Run registration, ctx stamping, intent framing, heuristic bypass, system-prompt assembly, model resolution, LoopConfig, resume ticket, post-run hooks (distill/forge/shadow-eval/lifecycle). |
| Tool-call policy | `(*Kernel).policyHook` (`policy.go`) | Adapts Edict to `agent.Policy`: capability resolution, trust ceiling, per-agent tool policy, epistemic gate, intent/regret gate, prompt-injection guard, auto-approve grant, live HITL via `approval.Registry.Submit`. |
| Halt / cancel / drain | `runtime/lifecycle.Manager` | `Halt`, `HaltWith`, `CancelRun`, `DrainAndHalt`, `Resume`, `NewCorrelation` (`run-<ulid>`). |
| Live steering | `runControl` (`steer.go`), `steer_ops.go`, `kernel/intervention` | pause / step / resume / steer / BTW note / lease-bounded intervene, applied at the loop's iteration boundary. |
| Delegation | `subagent_*.go` + `kernel/delegation` | `delegate` / `delegate_await` tools; depth, fan-out, tree-total and spend rails; async spawns. |
| Restart resume | `runtime/resume.go` + `kernel/resume` | Durable per-run ticket and per-iteration conversation snapshot; `Suspend` before cancel; the daemon re-dispatches on boot. |
| Completion assurance | `kernel/assure`, `Runner.VerifyCompletion`, `kernel/proof`, `ProveTask` | Verify-and-retry loop; acceptance-criteria proof gate for workboard tasks. |
| Shared direct/workflow tool path | `toolapi.Invoker` via `(*Kernel).RunTool` (default `toolexec.NewInvoker`) | Operator/CLI/research plus registered-tool, HTTP, pipeline and canvas-node workflow calls: merged lookup, schema, resolved definition → policy, audit, execution. |
| Tool registry (boot) | `kernel/toolreg` | Spec registry with Build / PreOpen / Configure / Late phases used by `plugins/builtintools`. |
| Script tools | `kernel/toolforge` + `scripttool.go` | Agent-authored scripts. Lifecycle draft, test, promote, quarantine. Exposed as `forge_<name>`. |
| Context selection audit | `kernel/contextselect` + `context_selection.go` | Candidate scoring and `context.selection` manifests for memory/world/skill injection; failure analysis. |
| Intent framing | `kernel/intent` | Deterministic utterance → `Frame` (ambiguity, harmful reading), action regret axes, confirmation gating. |
| Aux "multi-model" features | `council*.go`, `conductor*.go`, `research*.go`, `workflow*.go` | Council of Elders, Conductor (thinker/worker/verifier), deep research, workflow graph execution and drafting. All use `completeAux`. |

---

## 2. Package dependency map

From `internal-deps.txt` (`pkg -> deps`):

```
kernel/agent        -> kernel/bus kernel/event                 (leaf-ish; imported by ~70 pkgs incl. every provider and tool)
kernel/runtime      -> brand, agent, agentgw, approval, artifact, assure, bus, cadence, catalog, configcenter,
                       contextselect, datalake, delegation, edict, event, governor, imagetool, intent, intervention,
                       journal, market, mcp, memory, okr, proof, reflect, reranktool, resume, roster, runtime/accessors,
                       runtime/lifecycle, runtime/runexec, runtime/types, scheduler, seat, skill, standing, state,
                       taste, tenantctx, toolexec, toolforge, ulid, voicetool, warden, workboard, workflow, worldmodel
kernel/runtime/runexec   -> agent, agentgw, approval, artifact, assure, bus, cadence, catalog, configcenter, datalake,
                            edict, event, intent, journal, memory, reflect, resume, roster, scheduler, skill, standing,
                            state, voicetool, warden, worldmodel           (never imports kernel/runtime; KernelAPI interface)
kernel/runtime/lifecycle -> event, ulid
kernel/runtime/accessors -> (store packages) + runtime/types
kernel/runtime/compose   -> agent, edict, imagetool, mcp, reranktool, voicetool, warden    (NO importers: dead)
kernel/runtime/types     -> (none)
kernel/app/tools   -> contract/{toolapi,toolphaseapi}, platform/toolpipeline
kernel/toolexec     -> contract/toolapi, platform/toolpipeline (legacy forwarding only)
kernel/platform/toolpipeline -> contract/{llm,policyapi,toolapi,toolphaseapi}, event, platform/{policyctx,schema,toolaudit,toolinvoke,tooloutput}
kernel/toolreg      -> agent, artifact, board, bus, channel, datalake, journal, runtime, warden
kernel/toolforge    -> filestore, ulid
kernel/delegation   -> agent, edict
kernel/resume       -> internal/atomicfile, agent
kernel/planner      -> internal/strutil, agent, intent
kernel/reflect      -> bus, event, journal, worldmodel
kernel/contextselect-> memory, skill, worldmodel
kernel/proof        -> assure
kernel/assure, convo, intent, intervention, meshctx -> (none)
```

Reverse deps (who uses this area):

- `kernel/runtime` is imported by `cmd/agezt`, `kernel/controlplane`, `kernel/selfrepair`, `kernel/cadence/systemtasks`, `kernel/toolreg`, `plugins/builtinguardians`, `plugins/builtintools`, and the tools `conductor`, `config`, `council`, `introspecttool`, `overseertool`, `research`, `workflowtool`.
- `kernel/agent` is imported by every provider (`plugins/providers/*`), every tool (`plugins/tools/*`), `governor`, `memory`, `skill`, `scheduler`, `pulse`, `market`, `plugin`, `worldmodel`, `controlplane`, `cmd/agt`, `cmd/agezt`.
- `kernel/convo` is used by `controlplane`, `openaiapi`, `webui`. `kernel/meshctx` by `restapi`, `plugins/tools/peer`, `cmd/agt`. `kernel/planner` by `controlplane`, `cmd/agt`. `kernel/proof` by `workboard`. `kernel/toolforge` by `controlplane` and `plugins/tools/forgetool`. `kernel/resume` by `cmd/agezt` (boot resumer).

Layering notes:
- `kernel/toolreg` imports `kernel/runtime`. This is an upward edge: a "registry" sits above the composition root because `Spec.PreOpen` mutates `*runtime.Config` and `KernelDeps` carries `*runtime.Kernel`. It is not a cycle (runtime does not import toolreg).
- The kernel never imports `plugins/*`. Plugins are injected through interfaces: `toolforge.Runner`, `CodeExecutor`, `Voice`, `ImageGen`, `Reranker`, `memory.Embedder`, and the closures `VisionModel`, `ModelAvailable`, `CouncilMembers`.

---

## 3. kernel/agent — the loop

### 3.1 Purpose
The canonical single-agent tool loop (DECISIONS B0d). It defines `Provider` (LLM adapters) and `Tool` (tool adapters), plus `Run`, which wires them together. Each iteration is interleaved (one model turn, then its tools), not plan-then-execute. It depends only on `kernel/bus` and `kernel/event`. Pricing comes in through `CostFn`, policy through `Policy`, artifacts through `ArtifactPutter`, steering through `Steerer`. These are all injected.

### 3.2 Key types (agent.go, agent_loop.go, agent_steer.go, agent_context_policy.go)

| Type | Fields / methods (real names) |
|---|---|
| `Message` | `Role` (`system`/`user`/`assistant`/`tool`), `Content`, `ToolCalls []ToolCall`, `ToolCallID`, `Images []string` |
| `ToolCall` | `ID`, `Name`, `Input json.RawMessage` |
| `ToolDef` | `Name`, `Description`, `InputSchema`. Off the wire (`json:"-"`): `Effect ToolEffect`, `Capability ToolCapability` |
| `ToolCapability` | `Name` (fallback axis), `Field` (input field selecting the axis, e.g. `op`), `ByValue map[string]string`. `For(input)` resolves the axis per call; `IsZero()` |
| `ToolEffect` / `EffectClass` | `read_only` / `reversible` / `compensable` / `irreversible`; `PredictedEffects`, `AffectedResources`, `RollbackNotes`, `Confidence` |
| `CompletionRequest` | `Model, System, Messages, Tools, MaxTokens`. Governor-only hints: `TaskType, ModelChain, Agent, AgentDailyCeilingMc, CorrelationID`. Also `JSONMode`, `Params`, `ProviderOptions` |
| `CompletionResponse` | `Message`, `StopReason` (`end_turn`/`tool_use`/`max_tokens`), `Usage` (input/output/cached/cache-write tokens, `Model`), `ReasoningContent` |
| `Params` | pointer knobs: `Temperature, TopP, TopK, Stop, Seed, FrequencyPenalty, PresencePenalty, ReasoningEffort`; `IsZero()` |
| `Provider` | `Name() string`; `Complete(ctx, CompletionRequest) (*CompletionResponse, error)` |
| `StreamingProvider` | `CompleteStream(ctx, req, onChunk func(Chunk) error)`. It must return the same response as `Complete`. |
| `Chunk` | `TextDelta`, `ToolUseStart *ToolCall`, `ToolInputJSONDelta`, `ToolUseStop`, `ReasoningDelta`; `IsEmpty()` |
| `Tool` | `Definition() ToolDef`; `Invoke(ctx, json.RawMessage) (Result, error)`. Implementations must be goroutine-safe because the same instance serves concurrent runs and in-turn parallel calls. |
| `Result` | `Output`, `IsError`, `ObservationTrust` (`""`/`trusted`/`untrusted`), `ObservationSource` |
| `PolicyVerdict` | `Allow, Capability, Reason, WouldAsk, HardDenied, EffectClass, AffectedResources`, epistemic fields (`EpistemicAction/Reason/Signals/Confidence, FailureMatches, WeightedFailures, SchemaHash, InputShape, TemporalSensitive, NovelTool`), injection fields (`UntrustedObservation, ObservationSources, ObservationDirectiveLike, ObservationDirectiveMatches`) |
| `Policy` | `func(ctx, ToolCall) PolicyVerdict` |
| `Steerer` | `Wait(ctx) error`; `Drain() []Directive`. `Directive{Text, Note}` |
| `LoopConfig` | see 3.3 |
| `ToolSelector` / `ToolSelectionRequest` | `func(ctx, {Intent, Iter, Messages, Tools}) ([]ToolDef, error)` |
| `ToolMemo` | per-run TTL+LRU cache of successful read-only results. Keys are SHA-256 of name+input. Defaults `DefaultToolMemoTTL=5m`, `DefaultToolMemoMaxEntries=256` |
| `ArtifactPutter` | `Put([]byte) (ref, error)`. `kernel/artifact.Store` satisfies it |
| `Middleware` / `Wrap` | provider middleware (M997): `TransformRequest`, `WrapComplete`, `WrapStream`, `SynthesizeStream`. Helpers: `ExtractReasoningMiddleware`, `SimulateStreamingMiddleware`, `DefaultParamsMiddleware` |
| `GenerateObject` | JSON-schema-constrained one-shot with `DefaultObjectRepairs=2` repair rounds. Returns `*ObjectError` wrapping `ErrNoObjectGenerated` |

W2.3h moved these contracts verbatim to `kernel/contract/policyapi`; agent aliases retain exact callback/verdict type identity. The loop forwards policy journal rendering to `kernel/platform/toolaudit.PolicyDecisionPayload`, preserving its 23 fields and nil/zero JSON representation. W2.3i measured and repaired the direct invoker's eight-field map: `toolexec.Run` now uses the same 23-field renderer through both legacy and options entry points. Resource/epistemic/observation data and explicit nil/zero values are retained without changing the verdict or admission sequence. Same-verdict loop/direct/options and actual direct/workflow/canvas/code allow/deny journal tests reproduced the old loss; eight mutations guard wiring, metadata, identity and mandatory preflight error propagation. Source/actual-loop tests and four mutations guard the extraction.

W2.3j extracted the unchanged schema validator (`platform/schema`) and policy context helpers (`platform/policyctx`) so the direct invoker can stop depending on the loop implementation. `contract/policyapi` owns the observation-taint type; agent forwards its existing public helpers. Exact error/validation and cross-accessor/parent-context tests plus seven mutations protect the move. W2.3k repointed direct Run/RunWithOptions to the platform validators/context helpers; its production dependency closure no longer reaches agent. Old/new source invoker tests preserve unknown/schema refusal before policy/backend/audit/hooks and resolved metadata, provenance, identity and allow/deny semantics. Five mutations and 20-repeat entry-point tests guard the repointing. The loop's compatible helper calls and its memo/guard/batch admission sequence remain.

W2.3l adds the pure `toolapi.Invoker`/`Invocation` port. `runtime.Open` binds a service once through `Config.NewToolInvoker`, defaulting to the unchanged `toolexec.NewInvoker` compatibility pipeline. Each call forwards its lookup override and effective artifact store/threshold; per-kernel constructors bind own policy/audit/noise ports. Nil service results fail startup before gateway launch and release opened stores. Four actual-path offload, two-kernel policy/journal and nil/reopen tests plus seven mutations guard injection. Pipeline ownership and agent batch admission still remain legacy; L4 app/daemon binding follows.

W2.3m binds `app/tools.NewInvoker` at daemon composition through shared `openAppKernel` for primary and tenant kernels. The service forwards context and Invocation to the canonical legacy pipeline; each Open creates fresh host dependencies. Actual app-bound offload and full policy/provenance paths, two-kernel isolation and live approval identity remain covered, with six mutations. Standalone runtime.Open keeps its legacy default. The L4 entry now exists, while implementation ownership and agent admission convergence remain open.

W2.3n moved the generic governed direct engine into `platform/toolpipeline`, which imports only contracts/event/platform helpers and stdlib. Business policy/audit/hook hosts remain injected. The app constructor now builds that engine; legacy toolexec keeps type aliases and public forwarding adapters, including one shared terminal output decorator. Standalone fallback keeps old APIs reachable in production. Execution/recovery/output/constructor bodies compare unchanged; primitive/source contracts and eight mutations cover the move. App/runtime dependency declarations still use compatibility aliases until the separate repointing.

W2.3o repoints app/runtime dependency declarations to the platform ports. App dependency closure excludes toolexec/runtime/agent; runtime keeps only the explicit standalone legacy constructor fallback. Its public forwarding APIs and app entry execute the same engine. Behavior is preserved; agent batch admission/memo remains outside this direct engine pending a phased boundary.

W2.3p introduces the shared `toolpipeline.Decide` policy/audit phase. It binds resolved ToolDef, calls the supplied policy and records the decision through the caller's audit envelope; it has no executor or memo access. Direct Run keeps its returned metadata context for execution/hooks. Primitive contracts and six mutations preserve the phase. New actual agent contracts passed on old source: batch policy/invoked admission completes before tool effects, and a later denial wins over memo (count=20). Agent policy routing is still unchanged until the separate repointing.

W2.3q removes the unused `agent.WithPolicyToolDef` setter shim (use `platform/policyctx.WithPolicyToolDef`) and routes agent policy admission through `toolpipeline.Decide`. The loop supplies its scoped observation taint and explicit no-policy allow callback, retains its journal/error envelope, checks memo only after the audited decision and leaves execution context unchanged. Batch effects still wait for all admission. Actual batch/memo/audit-failure and causal-window/provenance contracts pass count=20; seven mutations protect the repointing. Availability/schema/loop guard, memo scheduling, execute/settle/timeout remain caller-owned pending remaining phase convergence.

### 3.3 LoopConfig fields (agent_loop.go)
Required: `Provider`, `Bus`, `Actor` (`validateLoopConfig` fails before `task.received`). Everything else is optional.

- Bounds: `MaxIter` (default `DefaultMaxIter=50`), `MaxAutoContinue` (default `DefaultMaxAutoContinue=5`; negative disables), `AutoContinueWait` (default 2s), `ToolTimeout` (per call), `MaxParallelTools` (default 4; 1 or less means sequential), `MaxIdenticalToolCalls` (default 5; negative disables), `MaxRunCostMicrocents` + `CostFn`, `MaxTokens`.
- Routing and identity hints: `Model`, `TaskType`, `ModelChain`, `Agent`, `AgentDailyCeilingMc`, `JSONMode`, `Params`.
- Provenance (stamped on `task.received`): `WakeSource, WakeReason, ScheduleID, StandingID, StandingName, TriggerSubject, ParentCorrelation`, `Images`.
- Governance: `Policy`, `DirectiveTaintWindow` (default 1), `ToolSelector`, `ObservationDeltas`, `ToolMemo`, `ToolResultHook`.
- Context: `ContextBudget` (chars), `ContextProtectLast` (default 4), `ContextProtectFirst` (default 0), `SummarizeElided`, `ContextRescueMarkers`, `Artifacts`, `ArtifactThreshold` (default 8 KiB).
- Live control and durability: `Steer`, `Checkpoint func(iter, messages)`, `PriorMessages`, `StartIter`.

### 3.4 Run algorithm (agent_run.go, `Run(ctx, cfg, userIntent) (answer, runErr)`)

```
validateLoopConfig ─fail→ return (no events)
normalizeLoopConfig
publish task.received (taskReceivedPayload)
defer: if runErr != nil → publish task.failed {error, reason=failureReason()}
defer: recover() → runErr = ErrPanic wrap                     (panic firewall, M168; registered last so it runs first)
messages = initialMessages()      (fresh user turn, or a COPY of PriorMessages on resume)
tools    = buildToolDefs()        (LintToolSchema on every tool; a malformed schema fails the run)
st = newRunState(); summarizeElided = newElisionSummarizer()
for iter := StartIter; ; iter++:
   if iter >= segmentEnd:                                     (auto-continue, M833)
       if continues used up → break → ErrMaxIter
       publish task.continued {attempt, of, iters_so_far}; wait AutoContinueWait (ctx-aware)
       append user turn autoContinuePrompt; segmentEnd += MaxIter
   ctx.Err() → return
   cfg.Checkpoint(iter, messages)                             (resume snapshot, M1002)
   cfg.Steer.Wait(ctx); for d in Steer.Drain(): append user turn (prefix
       "[operator steering] " or "[operator note — FYI; ...] "), publish run.steered {iter, directive, mode}
   if ContextBudget>0: compactMessagesDetailed → publish context.compacted if anything was elided
   offered = st.offeredTools(tools)  (drops tools denied >= maxToolDenials=2 times, or hard-denied once)
   if ToolSelector: offered = normalizeSelectedTools(offered, selector(...))
   publish llm.request {iter, messages, model, tools, context_chars, context_by_role[, tool_discovery, tools_before_discovery]}
   resp = callProvider(completionRequestFor(...))   (streaming → ephemeral llm.token / llm.reasoning)
   publish llm.response {iter, stop_reason, usage, text_chars, reasoning_chars, tool_calls}
   cost cap: spent += CostFn(billedModel, in, out); if spent >= cap → ErrRunBudgetExceeded
   append resp.Message
   if StopReason != tool_use || no ToolCalls:
       publish task.completed {iters, chars, stopped, answer(truncated to 8192 runes)}; return answer
   jobs = st.gateToolCalls()      (phase 1, sequential, in call order)
   executeToolJobs()              (phase 2, parallel up to MaxParallelTools; per-goroutine recover)
   messages = st.finalizeToolJobs()  (phase 3, original order: classify, tool.result, append tool turns)
return ErrMaxIter
```

`failureReason` tags: `panic`, `max_iters`, `cost_budget`, `canceled`, `timeout`, `error`.

### 3.5 Tool turn: gate / execute / finalize (run_tools.go, run_tools_gate.go)

W2.3v adds the pure toolphaseapi.Phases port and moves the unchanged resolution/decision/execution types behind platform aliases. The separate contract avoids the existing llm→toolapi dependency cycle; publisher strings bridge registered kinds without importing event. The per-kernel platform/app service implements all phases, and its direct Invoke routes through that port. Explicit RunWithOptions phase selection works through legacy forwarding; nil retains the existing path. Eleven mutation guards and app/trace contracts cover phase routing, early stops, typed errors and lookup ownership. W2.3w binds this port per kernel into the shared root/delegation loop builder. All five loop phases consume the injected service; a nil standalone LoopConfig.ToolPhases gets one canonical service per run. Legacy runtime exposes phases, while old one-shot-only factories retain historical loop behavior through one canonical host-bound compatibility service per kernel. Actual app-bound kernel root/tenant/delegated traces and compatibility tests pass count=20; ten mutations cover the binding. Caller batch/memo/taint/output/error/hook behavior stays unchanged. W2.3 exit requirements still need final audit.

W2.3r shares lookup and schema admission through `toolpipeline.Resolve`. It performs one caller-owned lookup and one definition read, retaining the complete metadata even on schema rejection. No policy/audit/tool effects occur in this phase. Both callers retain their existing unavailable/schema error formatting and lookup scope. Old-source gate and actual mixed-batch contracts pass count=20; eight mutations cover name, ordering, metadata, schema and quota boundaries.

**Gate** (`(*runState).gateToolCalls`), for each call in order:
1. Tool not in `cfg.Tools`: the synthetic error result is `tool "x" is not available`.
2. Shared `toolpipeline.Resolve` validates the resolved definition/input using `platform/schema`: on failure the result is `tool call rejected by schema: …`. Steps 1–2 precede guard quota, policy and memo.
3. Loop guard (M116): key `name\x00input`. Above `MaxIdenticalToolCalls`, the model gets a "loop guard: …" error result.
4. Policy: `toolpipeline.Decide` binds trusted metadata and audits the callback; the loop supplies `WithUntrustedObservationTaint(ctx, scopedTaint)`. `scopedTaint.DirectiveLike` is true only while `iter - directiveObsIter <= directiveWindow`. If no Policy is configured, the verdict is `Allow` with reason `"no policy configured"`. **`policy.decision` is always published**, with the 23-field `policyDecisionPayload`.
5. Deny: increment `toolDenials` (hard-deny sets it to the max at once). The result is `tool call denied by policy: <reason>`. No `tool.invoked`.
6. Memo: if `ToolMemo != nil` and the call is read-only (verdict or def effect class), a cache hit or an in-turn duplicate becomes `memoHit`. **Policy runs before the memo lookup**, so memoization never grants permission.
7. Shared `toolpipeline.Announce` publishes `tool.invoked {tool, call_id, input}` through the loop envelope; `job.tool = tool` only after successful audit. Any invocation-audit failure aborts the whole admission phase before effects.

W2.3s shares invocation announcement with direct calls. The phase owns event kind and the unchanged three-field payload, accepts no executor/context, and returns the mandatory publisher error unchanged. Denial, memo hits and coalesced duplicates skip announcement. Caller subject/actor/correlation and error formatting remain separate; announcing loop calls does not execute them. Old-source gate/direct and actual batch/memo contracts pass count=20; nine mutations protect event identity, input, audit cause and admission/effect ordering. Execution/timeout/settlement remain caller-owned.

The HITL approval wait happens inside step 4 (`policyHook` blocks in `approvals.Submit`). Approvals for a multi-call turn are therefore requested **sequentially**, before any call of that turn executes.

**Execute** (`executeToolJobs` → `invokeToolJob` → `toolpipeline.Execute`): the loop assembles WithCorrelation(ctx, corr), then the shared phase owns optional positive ToolTimeout, safe toolinvoke.Invoke, panic mapping, deadline capture and context cleanup. Panic mapping runs before cleanup so context-sensitive formatter text and ErrPanic identity remain unchanged. Direct calls pass zero timeout/no mapper and retain their caller budget/error text. Raw backend result/error and panic/timeout facts return to callers for settlement. Old-source loop/direct and actual panic/cancel/timeout contracts pass count=20; twelve mutations guard the phase and wiring. One job, or MaxParallelTools <= 1, runs inline; otherwise the existing semaphore-bounded fan-out and worker recover remain.

**Finalize** (`finalizeToolJobs` → `toolpipeline.Settle`), in original order:

W2.3u shares terminal kind/core fields and mandatory publication. Caller observation/artifact/delta/memo/skipped fields are copied without normalizing nil/empty values; call/result identity wins over extra fields. The loop sends an audit preview copy and keeps full model/hook output. Direct deny/error/panic/success use the same phase through their existing output decorator and envelope. Classification, error joins, remaining batch writes, cancellation checks and hook/model ownership stay outside the phase. Old-source and actual terminal/delta/memo contracts pass count=20; twelve mutations protect the boundaries. Per-kernel app/loop invocation routing remains open.
- A panicked job returns `ErrPanic` and terminates the run.
- A run-ctx error returns `ctx.Err()` (run-level terminal).
- A tool timeout gives the error result `tool "x" exceeded its <d> timeout` and the run continues.
- Other invoke errors become `Result{Output: err.Error(), IsError: true}`.
- Memo store on success. Observation deltas (CH-04): `DiffObservation(prev, curr)` for a repeated (tool, input).
- **Observation boundary**: `ObservationBoundaryForTool`. Untrusted by default for `browser.read`, `http`, `fetch`, `web_search`, `mcp_*`, `forge_*`, or whenever the tool set `Result.ObservationTrust`. For untrusted output: merge taint (sources, matches), record `directiveObsIter` if directive-like (`directiveInjectionNeedles`), and wrap the content in the `UNTRUSTED OBSERVATION … END UNTRUSTED OBSERVATION` envelope the model sees.
- Artifact offload: above `ArtifactThreshold`, the event carries a 512-byte preview plus `raw_ref` and `output_bytes`. **The model still receives the full output.**
- Publish `tool.result`, call `ToolResultHook`, append a `RoleTool` message.

### 3.6 Context compaction (agent_context.go)
- `compactMessagesDetailed` elides the oldest `RoleTool` message contents in `[ContextProtectFirst, len-ContextProtectLast)` until total chars fit the budget. Skipped: already-elided stubs (prefix `[tool output elided to fit context budget`), and outputs containing a rescue marker (`DefaultContextRescueMarker = "_agezt_context_rescue"`). The stub embeds either an abstractive summary (≤160 chars, from `SummarizeElided`, cached per distinct output) or an 80-char head snippet. A stub is only used if it is shorter than the original.
- Only tool outputs are ever elided. User, assistant and system turns are never touched.
- `AutoContextBudgetChars(ctxTokens) = tokens * 4 * 0.5`.
- `contextSize` is a char count by role, including the system prompt and tool-call argument JSON, excluding images.

### 3.7 Agent-loop invariants (verified in code)
1. `task.received` is published before anything else. Validation failures publish nothing.
2. Exactly one terminal event per started run: `task.completed` on the success return, otherwise `task.failed` from the defer. A panic is converted to `ErrPanic` first.
3. `policy.decision` precedes `tool.invoked` for every executed call. A denied, schema-rejected, unknown or loop-guarded call never reaches `Tool.Invoke`.
4. Every requested call yields exactly one `tool.result` and one `RoleTool` message with the matching `ToolCallID`, in the model's call order, regardless of parallelism. Denied calls also get a `tool.result` (with `error:true`). Memo hits get `memo_hit:true` and no `tool.invoked`.
5. Checkpoint, steering, compaction and auto-continue run only at the top of an iteration. At that point `messages` ends on a settled assistant→tool set, so no tool_call is ever dangling.
6. Taint is set by the tool-result boundary, never by the model. Provenance is sticky for the whole run. Only the gating flag decays after `DirectiveTaintWindow` iterations.
7. The cost cap is post-call: a run can overshoot by at most one call.
8. A paused run is still killable, because `Steer.Wait` returns `ctx.Err()`.
9. `CompletionRequest` construction lives in one place (`completionRequestFor`), so a new routing field cannot be forgotten on the wire.

### 3.8 File table — kernel/agent

| File | What it does |
|---|---|
| `doc.go` | Package doc: canonical loop, Provider/Tool interfaces, panic containment. |
| `agent.go` | Type aliases only (W1.1). The provider contract (Role, Message, ToolCall, CompletionRequest/Response, StopReason, Usage, Params, Provider, StreamingProvider, Chunk) lives in `kernel/contract/llm`; the tool contract (Tool, ToolDef, ToolCapability + `For`, EffectClass, ToolEffect, Result, ObservationTrust) in `kernel/contract/toolapi`. New code imports those directly. |
| `agent_loop.go` | `LoopConfig` (every knob documented) and the start of the `Steerer` doc. |
| `agent_steer.go` | `Steerer`, `Directive`, compaction constants (`DefaultContextProtectLast/First`, `ContextCharsPerToken=4`, `DefaultCompressFraction=0.5`). |
| `agent_run.go` | `Run`, the main loop driver, plus `failureReason`. |
| `run_setup.go` | Prologue helpers: `normalizeLoopConfig`, `validateLoopConfig`, `taskReceivedPayload`, `initialMessages`, `buildToolDefs`, `newElisionSummarizer`, `resolveDirectiveWindow`. |
| `run_provider.go` | `callProvider` (streaming vs non-streaming; ephemeral `llm.token` / `llm.reasoning`; nil-response guard) and `completionRequestFor`. |
| `run_tools.go` | `toolJob`, `runState` (callCounts, observations, toolDenials, taint window), `newRunState`, `directiveActive`, `offeredTools`, `maxToolDenials=2`. |
| `run_tools_gate.go` | `gateToolCalls`, `policyDecisionPayload`, `executeToolJobs`, `invokeToolJob` (shared platform primitive, cleanup on panic), `finalizeToolJobs` (settles terminal batches before failure, joins audit/terminal causes). Skipped sequential calls are marked `not_executed`; terminal batches suppress result hooks. |
| `agent_context.go` | Budget and compaction (`AutoContextBudgetChars`, `compactMessagesDetailed`, `rescuedToolOutput`, `contextSize`, `truncateForJournal`), loop defaults, sentinels (`ErrMaxIter`, `ErrPanic`, `ErrRunBudgetExceeded`, `ErrUnknownTool`), steering prefixes, `autoContinuePrompt`. |
| `agent_context_offload.go` | Compatibility `ArtifactPutter`/default aliases and `offloadToolOutput` forwarding to `platform/tooloutput.Offload` (W2.3f move only). Threshold, preview and best-effort fallback are unchanged. |
| `agent_context_policy.go` | Exact `PolicyVerdict`/`Policy` compatibility aliases to `contract/policyapi` (W2.3h move only). |
| `observation.go` | Trust boundary (the `ObservationTrust` type itself is in `contract/toolapi`): `ObservationBoundary`, `ObservationBoundaryForTool`, `RenderObservationForModel`, `MergeUntrustedObservationTaint`, directive-needle matching, `DiffObservation`, `DefaultDirectiveTaintWindow=1`. |
| `toolctx.go` | Policy ToolDef/taint compatibility helpers forward to `platform/policyctx`; `UntrustedObservationTaint` aliases `contract/policyapi`. Invocation identity/workdir helpers live in `contract/toolapi`. |
| `toolselect.go` | `LexicalToolSelector`, `DeferredLexicalToolSelector` (pinned `tool_search`, no fallback-all), `normalizeSelectedTools`, scoring. `max <= 0` returns a nil selector. |
| `schema.go` | Compatibility `ValidateToolInput`, `ValidateJSON`, `LintToolSchema` forwarding to unchanged `platform/schema` validation (W2.3j). |
| `memo.go` | `ToolMemo` (`NewToolMemo`, `Get`, `Set`, LRU order), `memoKey` (SHA-256). |
| `middleware.go` | `Middleware`, `Wrap`, synthesized streams, `ExtractReasoningMiddleware`, `SimulateStreamingMiddleware`, `DefaultParamsMiddleware`. |
| `generate.go` | `GenerateObject` (JSONMode plus schema instruction, `extractJSON`/`balancedSpan`, validate, repair loop), `ObjectError`. |

Tests (20 files): `agent_test.go` (loop end to end with mock provider), `panic_test.go` (firewall), `parallel_test.go` (M880 ordering), `runcost_test.go`, `steer_test.go`, `resume_seed_test.go`, `compact_internal_test.go`/`context_*_test.go` (compaction), `directive_test.go`/`observation_test.go` (injection window), `offload*_test.go`, `memo_test.go`, `middleware_test.go`, `generate_test.go`, `toolselect_test.go`, `vision_test.go`, `run_tools_internal_test.go`.

---

## 4. kernel/runtime — the Kernel

### 4.1 Kernel struct and concurrency (runtime_kernel.go)
`Kernel` holds `cfg Config`, the stores (journal, state, bus, edict, warden, approvals, scheduler, memory/world/forge/skill stores, standing, resume, roster, toolForge, mcpStore, workflows, workboard, okr, taste, seat, artifacts + artIndex, lake, reflect, schedules, schedEngine, agentGW, configCenter), `tools` (the effective in-process map), `conductorExec`, catalog and catalogStore, and three sub-managers (`lifecycle`, `accessors`, `runexec`).

Mutexes, with a mandatory acquisition order (documented on the struct and in runexec/doc.go):
```
configMu < runsMu < fanoutMu < treeMu < steersMu < spawnsMu < mcpMu
```
| Mutex | Guards |
|---|---|
| `configMu` | live `system`, `model`, `cfg`, `catalog`, `schedEngine` |
| `runsMu` | `halted`, `runs map[corr]CancelFunc` |
| `fanoutMu` | `fanout map[spawningCorr]int` (M46) |
| `treeMu` | `tree map[rootCorr]int` (M629) |
| `steersMu` | `steers map[corr]*runControl` |
| `spawnsMu` | `spawns map[childCorr]*spawnHandle` (async delegation) |
| `mcpMu` | `mcpConns map[server]mcp.Conn` |

Also: `suspending atomic.Bool` (latched by `Suspend`, never cleared); `runWG sync.WaitGroup` (root runs plus async spawns, drained by `Close`/`DrainAndHalt`); `toolCaps` (read-only after Open); `startTime`.

Goroutines started by the runtime: the agentgw listener (`Open`), one per async sub-agent (`runSubAgentAsync`), the drain waiters in `Close`/`DrainAndHalt`, and the tool fan-out workers inside `agent.Run`. Everything else runs on the caller's goroutine.

### 4.2 Composition: `Open(cfg Config) (*Kernel, error)` (compose.go)
Order (each store registers a closer; `fail()` unwinds them in reverse):

1. `journal.Open(<BaseDir>/journal)` and `state.Open(<BaseDir>/state)`.
2. Edict (`cfg.Edict` or `edict.New`), `bus.New(journal)`, Warden (`warden.New(bus)` default), Approvals (`approval.New{Bus, Timeout: ApprovalTimeout}`), `scheduler.New{Bus, Monitor: ContextInvariantMonitor}`.
3. `memory.Open(memory/)` + `NewManager` (+ embedder); `worldmodel.Open(worldmodel/)` + `NewGraph`; `skill.Open(skills/)` + `NewForge` + `OpenBundles` (best effort).
4. `cadence.OpenStore(cadence/)`, `artifact.Open(artifacts/)` + `OpenIndex`, `datalake.Open(BaseDir)` + `SeedBuiltins("system")`, `standing.Open(standing/)`.
5. `resume.Open(resume/, ResumeSnapshotMaxBytes)`, only when `ResumeEnabled`.
6. `roster/`, `toolforge/`, `mcp/`, `workflows/`, `workboard/`, `okr/`, `taste/`, `seats/` stores; `reflect.New(journal, world, bus)`.
7. Effective tools: `cfg.Tools` plus `memory` (MemoryTool), `world` (WorldTool), `delegate` and `delegate_await` (SubAgentTool), `market` (MarketTool), `voice`, `image_generate`, `rerank` (when their implementations are injected).
8. Catalog: `cfg.Catalog` or `catalog.NewStore(catDir).Load()`, then `governor.SetCatalog(cat)` (global).
9. Build `*Kernel`, then late-bind the closures: `subTool.run=k.runSubAgent`, `subTool.spawn=k.runSubAgentAsync`, `awaitTool.await=k.awaitSubAgent`, `mktTool.Manager=k.Market`, and `SaveArtifact=k.artifacts.Put` for image and voice.
10. `configcenter.Open`, then `agentgw.NewGateway` (token secret via `agentgw.ResolveTokenSecret`; `AGEZT_AGENTGW_SOCKET` override read directly with `os.Getenv`). Then `go agentGW.Listen(context.Background())`.
11. `lifecycle.New(k)`, `accessors.New(k)`, `runexec.New(k)`.

`Close()` (compose_close.go): `Suspend("close")`, `Halt()`, a bounded wait on `runWG` (`DefaultShutdownDrainTimeout=5s`; on timeout it publishes `anomaly.detected{anomaly:shutdown_drain_timeout}`), `closeMCPConns`, `bus.Close`, then `closeAll(state, memoryDir, worldDir, skillDir, journal, agentGW)` joined with `errors.Join`.

**Persistence under AGEZT_HOME (BaseDir) opened by the runtime:** `journal/`, `state/`, `memory/`, `worldmodel/`, `skills/`, `cadence/`, `artifacts/`, datalake dirs, `standing/`, `resume/` (+ `resume/quarantine/`), `roster/`, `toolforge/scripttools.json`, `mcp/`, `workflows/`, `workboard/`, `okr/`, `taste/`, `seats/`, `catalog/` (or `CatalogDir`), plus the configcenter and agentgw files. Formats belong to the owning packages (see 06/07). This area owns only `resume/<corr>.json` and `toolforge/scripttools.json`.

### 4.3 Run entry points and callers

| Entry | Body | Callers |
|---|---|---|
| `(*Kernel).Run(ctx, intent)` | mints corr, then RunWith | tests, simple callers |
| `(*Kernel).RunWith(ctx, corr, intent)` | rejects empty corr, then `runexec.Runner.RunWith` | controlplane `handleRun` (socket/console; REST `POST /api/v1/runs` reaches it via `kernelAPIEngine.RunModel` in `cmd/agezt/api_engine.go`), channel inbound (`makeChannelHandler`), cadence (`buildCadence`), standing orders (`buildStandingRunner`), workboard dispatch, roster escalation, selfrepair wake, overseer tool, `LoopRunner()` (scheduler DAG `loop` nodes), boot resumer |
| `RunAssured(ctx, corr, intent, n)` | `Runner.RunAssured` → `assure.Until(run=RunWith/RunWithRetry, verify=VerifyCompletion)` | controlplane (`assure` arg), cadence, standing, resumer |
| `RunWithRetry(ctx, corr, intent, roster.RetryPolicy)` | `Runner.RunWithRetry`: up to `min(MaxAttempts,10)` RunWith attempts, `agent.retry` events, backoff via `retryDelay` | the same callers when a roster profile has a `RetryPolicy` |
| `RunPlan(ctx, plan, planID)` | registers `plan-<ulid>` in `k.runs`, then the scheduler | controlplane `plan` |
| `RunTool(ctx, corr, callID, name, args)` | `toolexec.Run`; stamps runtime actor/corr for approvals, resolves merged forge/MCP tools, carries the trusted ToolDef to policy | controlplane direct tool calls, research, workflow tool/HTTP/pipeline and canvas-node calls, Council web grounding |

Callers stamp the run context before calling, using exported helpers in `runctx_*.go`: `WithAgentProfile`, `WithAgentIdent`, `WithWakeContext`, `WithModel`, `WithModelChain`, `WithSystem`, `WithTools` (allowlist), `WithImages`, `WithJSONMode`, `WithRunTimeout`, `WithMaxCost`, `WithTrustCeiling`, `WithAutoApproveCapabilities`, `WithTrustedObservations`, `WithResumeSeed`, `WithResumeOwned`.

### 4.4 End-to-end lifecycle of ONE run

```mermaid
sequenceDiagram
    autonumber
    participant C as Caller (controlplane.handleRun / channel handler / cadence / standing / REST engine)
    participant K as runtime.Kernel
    participant R as runexec.Runner
    participant A as agent.Run
    participant G as governor (cfg.Provider)
    participant P as Kernel.policyHook
    participant E as edict.Engine
    participant AP as approval.Registry
    participant T as agent.Tool
    participant B as bus → journal

    C->>K: NewCorrelation() → "run-<ulid>"; ctx = With*(…)
    C->>K: RunWith(ctx, corr, intent) [or RunAssured / RunWithRetry]
    K->>R: Runner.RunWith
    R->>K: SetupRunState(corr) (runsMu: halted? dup corr? → WithTenant, auto-approve, WithTimeout/WithCancel, runs[corr], runWG.Add, steers[corr]=newRunControl)
    R->>R: WithActorCorrelation("agent-"+corr), memory/world/skill/warden.WithCorrelation
    R->>R: skill.ParseActivationDirective; intent.Interpret → WithFrame
    R->>B: intent.interpreted
    alt deterministicHeuristicBypass (time/date)
        R->>B: task.received, info{bypass}, task.completed{stopped:heuristic_bypass}
        R-->>C: canned answer
    end
    R->>K: BuildRunPrompt (profile, taste, memory, world, skills → taste.injected / context.selection / skill.activated / memory.retrieved / worldmodel.retrieved)
    R->>K: ResolveRunModel, ModelChainFromCtx, BuildLoopConfig (tools: base+forge_+mcp_ → agent tool policy → noise policy → allowlist → tool_search; selector; ctx budget; CostFn; Artifacts; ToolMemo)
    R->>K: InjectHostEnvironment; ClaimResumeTicket (resume/<corr>.json)
    R->>A: agent.Run(runCtx, lc, intent)
    A->>B: task.received
    loop each iteration
        A->>K: Checkpoint(iter, messages) → resume.Store.Snapshot
        A->>K: Steer.Wait / Drain → run.steered
        A->>B: context.compacted (if elided)
        A->>B: llm.request
        A->>G: CompleteStream/Complete(CompletionRequest{TaskType:"chat", ModelChain, Agent, CorrelationID…})
        G-->>B: budget.consumed / provider.fallback (governor-owned)
        G-->>A: CompletionResponse (llm.token/llm.reasoning ephemeral)
        A->>B: llm.response
        alt final answer
            A->>B: task.completed
        else tool_use
            loop each ToolCall (sequential gate)
                A->>A: lookup, ValidateToolInput, loop guard
                A->>P: Policy(ctx+ToolDef+scoped taint, tc)
                P->>E: Decide / DecideWithCeiling(capabilityFor(tc,def), input)
                P->>P: approvalDecisionBundle, epistemicGate, agentToolPolicyDenial, noise denial, intent regret, injection guard
                opt RequiresApproval and not auto-approved
                    P->>AP: Submit(SubmitSpec) — blocks (approval.requested → granted/denied/timeout)
                end
                P-->>A: PolicyVerdict
                A->>B: policy.decision
                A->>B: tool.invoked (if allowed and not memo)
            end
            par up to MaxParallelTools
                A->>T: Invoke(WithCorrelation(ctx), input) [ToolTimeout]
            end
            A->>B: tool.result (original order; trust/taint/raw_ref)
        end
    end
    A-->>R: answer / err (task.failed via defer on error)
    R->>K: FinalizeResumeTicket, DeregisterRunSteer, Forge.RecordOutcome
    alt error
        R->>B: context.selection{phase:failure_analysis}
    else success
        R->>K: MaybeDistill / MaybeForge / MaybeShadowEval (threshold-gated, best-effort)
        R->>K: CompleteAgentLifecycle → roster.updated{lifecycle_cycle_completed} / retire
    end
    R->>K: (defer) CleanupRunState: delete runs/fanout/tree/steers, cancel orphan async spawns, runWG.Done
    opt RunAssured
        K->>K: VerifyCompletion (completeAux taskType "verify") → assure.verdict; retry with gap
    end
    K-->>C: answer, err
```

Numbered narrative with the real function names:
1. **Intent submission.** The caller mints `corr = k.NewCorrelation()` (`lifecycle.Manager.NewCorrelation`, `"run-"+ulid`). It may subscribe to `k.SubjectForRun(corr)` = `agent.agent-<corr>.>` first, then calls `RunWith`/`RunAssured`/`RunWithRetry`.
2. **Registration** (`setupRunState`). Under `runsMu` it refuses with `ErrHalted` or on a duplicate corr (M480). It stamps the tenant (`tenantctx.WithTenant`) and the merged auto-approve caps, then derives `runCtx` with `MaxDuration` or the per-run `WithRunTimeout` (DeadlineExceeded) or a plain cancel. It records `k.runs[corr]=cancel`, calls `runWG.Add(1)`, and registers `k.steers[corr]` (taking `steersMu` while still holding `runsMu`, which follows the lock order).
3. **Context stamping** (`Runner.RunWith`). Actor is `"agent-"+corr`. Correlation is threaded into memory/world/skill/warden contexts. An explicit `/skill` directive is stripped. The `intent.Frame` is computed (`intent.Interpret`) unless the caller supplied one, and `intent.interpreted` is journaled.
4. **Heuristic bypass.** Exact-match time/date questions return without any LLM call. Disable with `DisableHeuristicBypass` or the agent override `AGEZT_DISABLE_HEURISTIC_BYPASS`.
5. **System prompt** (`buildRunPrompt`). Base is `k.System()` (live) or `WithSystem` (the agent profile soul). Then the operator profile (`ProfileInject`), taste exemplars, memory recall (`RecallScoped`), world entities (`Resolve`), and skills (`ActivateFor`/`ActivateExplicitFor`). Each injection publishes a `context.selection` manifest (chosen vs rejected candidates). System agents (`roster.Profile.System`) skip all injection.
6. **Model.** `resolveRunModel` uses `WithModel` (explicit) or else the live `k.model`. An explicit pick with no chain becomes `ModelChain=[model]` (M931) so it wins over the task chain.
7. **LoopConfig** (`buildLoopConfig`, shared with sub-agents). `effectiveConfig` = cfg + live model/system + per-agent `ConfigOverrides`. Tools = `mergeMCPTools(mergeScriptTools(k.tools))`, then `applyAgentToolPolicy`, `applyAgentNoisePolicyToPromptTools`, the `WithTools` allowlist, and `withToolSearch` when over `ToolDiscoveryMax`. Policy is `k.policyHook`. `CostFn = governor.CostMicrocents`. `Artifacts = k.artifacts`. A new `ToolMemo` per run. The context budget is explicit or auto (catalog `Limit.Context`). The elision summarizer is optional (`makeElidedSummarizer`, 64 tokens, or 1024 for reasoning models).
8. **Host environment** (`injectHostEnvironment`/`injectEnvironment`). OS, shell hint, workspace, date and tool briefing, prepended last.
9. **Resume ticket** (`claimResumeTicket`), only for root runs. Then the root identity fields: `TaskType="chat"`, Agent, daily ceiling, Wake*, images, JSON mode, `MaxRunCostMicrocents` (`WithMaxCost`), `Steer`, `Checkpoint`, `PriorMessages`/`StartIter`.
10. **`agent.Run`** iterates as in section 3.4. Model calls go to `cfg.Provider`. In the daemon this is the governor router, which owns routing, fallbacks, budgets and `budget.consumed`; see 05.
11. **Tool call → Edict gate.** `policyHook`, section 4.5. Allow runs the tool. Deny returns an error result to the model. **Ask** blocks in `approval.Registry.Submit` until grant, deny, timeout (`ApprovalTimeout`, default `approval.DefaultTimeout` 5m) or run cancel.
12. **Completion.** `FinalizeResumeTicket` (keeps the ticket if the run was interrupted by shutdown), `DeregisterRunSteer` immediately, skill outcome attribution, then on success distill/forge/shadow-eval (thresholds `MemoryDistillMinTools`/`SkillForgeMinTools`, default 4 tool results counted by `FoldRunTools`) and `CompleteAgentLifecycle` (idempotent per corr via `Lifecycle.LastCompletedRun`). Cleanup always runs in the defer.
13. **Assure / proof** (optional). `RunAssured` loops verify→retry (≤10 attempts, default 3) under the same corr. `ProveTask` (workboard) judges acceptance criteria and gates the task's `done` state (section 10).

### 4.5 policyHook — the Edict gate in detail (policy.go, policy_helpers.go, epistemic*.go, intent.go)
1. `capabilityFor(tc, def)`: (a) `def.Capability.For(input)` if it names a capability Edict knows; (b) else `k.toolCaps[name]` (plugin manifest overlay M900, validated at Open by `validatedToolCaps`); (c) else `edict.CapabilityForToolCall` (name switch: `forge_*`→`code.exec`, `mcp_*`→`mcp.call`, …). An unknown capability name falls through to `Capability(toolName)`, which Edict default-denies unless `AGEZT_ALLOW_ALL` (`unknownAllow`).
2. `edict.DecideWithCeiling(cap, input, ceiling)` when the ctx carries a trust ceiling (agent profile `TrustCeiling`, pulse initiative); otherwise `Decide`.
3. `approvalDecisionBundle`: effect class from the ToolDef, else `defaultEffectClass(cap)`. Also predicted effects, affected resources (from input), rollback notes, confidence.
4. `epistemicGate`: signals `dynamic_tool_surface`, `temporal_sensitive`, `novel_tool_conditions`, `matched_failure_conditions:N`, `low_effect_confidence:X`, `permissive_schema_effectful_tool`. History comes from `matchHistoricalToolOutcomes`, a **full `journal.Range`** over `policy.decision`/`tool.result`, keeping the last 4096 and weighting failures by time decay. The result is advisory unless `EpistemicEscalation` is set.
5. Hard denials that bypass approval: `agentToolPolicyDenial` (profile `ToolAllow`/`ToolDeny`) and `agentNoisePolicyDenial` (e.g. memory writes disabled, notify throttling).
6. Fail-closed guards that each force HITL with their own reason (`guardRaised`): epistemic escalation (opt-in), intent/regret gating (`IntentRegretGating`, publishes `intent.confirmation_required`), and the prompt-injection guard (`PromptInjectionOn` and not `WithTrustedObservations`; in `Warn` mode it publishes `prompt_injection.warned` and allows). The injection guard only fires for non-read-only effect classes.
7. Session auto-approve (`autoApproveCap`) satisfies **only** the Edict Ask axis, never a guard. It publishes `policy.auto_approved`.
8. Live HITL: `k.approvals.Submit(ctx, approval.SubmitSpec{Capability, ToolName, Input, Reason, Actor, CorrelationID, EffectClass, PredictedEffects, AffectedResources, RollbackNotes, Confidence, CanonicalIntent, HarmfulInterpretation, AmbiguityScore, RegretAxes, ConfirmationPrompt})`. Grant gives `Allow=true`, `Reason="approval granted by <who>"`. Anything else gives `approval <decision>: <reason>`.

### 4.6 Halt / cancel / drain (runtime/lifecycle)
- `HaltWith(reason)` sets `halted`, snapshots and empties `k.runs`, calls every cancel (Canceled → `task.failed{reason:canceled}`), and publishes `halt` (`kernel.halt`, `{cancelled_runs, reason}`). New `RunWith`/`RunPlan`/async spawns return `ErrHalted` until `ResumeWith` (`resume` event, `kernel.resume`).
- `CancelRun(corr)` cancels one registered run: a root run, a plan, or an **async** sub-agent. Sync sub-agents are not in `k.runs`; they die with their parent's ctx.
- `DrainAndHalt(timeout)`: `Suspend("drain")` → `Halt()` → bounded `runWG.Wait`. Used by self-update.
- `ActiveRuns`, `ActiveRunIDs` (sorted), `SubjectForRun`.
- Halt during an approval wait: `Submit` sees ctx cancel and resolves to cancel, so the verdict is denied and the loop's next `ctx.Err()` check ends the run.

### 4.7 Live steering, queue and BTW (steer.go, steer_ops.go, kernel/intervention)
- `runControl` has: `mu`, `paused`, `pauseUntil` (lease), `stepOnce`, `directives []agent.Directive`, a `wake` channel (closed and replaced to broadcast), and idempotency `results`.
- Operator surface on `*Kernel`: `PauseRun` (`run.paused`), `ResumeRun` (`run.resumed`), `StepRun` (`run.stepped`, one iteration then re-block), `SteerRun(corr, text, note)` (queued; the loop publishes `run.steered{mode:steer|note}` when folded in), `RunControlState`, and `InterveneRun(intervention.Request)`.
- `kernel/intervention` is the wire grammar only. Primitives: `halt` (lease-bounded pause, default `DefaultLease=5m`), `abort` (→ `CancelRun`), `redirect` (forceful steer), `adjust` (note/BTW), `query`. Idempotency key replay returns the prior `Result`. Publishes `run.intervention`.
- Directives are applied **only at the next iteration boundary**. A run blocked in a long tool call sees them after the tool returns. Steering exists for root runs (`setupRunState`) and each sub-agent (`prepareSubAgent` registers `steers[childCorr]`, M631).
- `Suspend` injects a shutdown note into every live run as a `Note` directive.

### 4.8 Checkpoint and resume across daemon restart (runtime/resume.go + kernel/resume + cmd/agezt resumer)
1. **Claim** (`claimResumeTicket`): only if `k.resume != nil && ResumeEnabled && WakeContext.ParentCorrelation == ""` (root runs) and no outer frame owns it (`WithResumeOwned`). `buildResumeTicket` flattens the resolved context: `AgentSlug`, `MaxCostMc`, `TrustCeiling *int`, `RunTimeoutMs`, Wake*, `Kind` (`run`/`assured`/`retry`), `AssureBudget`. `Resumable=false` for explicit `WithSystem`, `WithModel` or `WithTools` overrides, which the ticket does not record. Profile-sourced model/system values carry their source slug in `runStringSetting`; values from the same named profile can be reconstructed on boot and do not block resume (W2.2b). Unnamed or retargeted profile values fail closed. The control plane preserves this provenance rather than re-copying profile defaults through explicit setters. Existing non-resumable tickets stay quarantined; their missing provenance cannot be guessed.
2. **Checkpoint**: at the top of every iteration, `resumeCheckpointFn` copies `messages` and calls `Store.Snapshot(corr, msgs, iter)`. That is an atomic tmp+fsync+rename via `internal/atomicfile`, 0600, pretty JSON. Above `DefaultSnapshotMaxBytes = 2 MiB`, the messages are dropped and `SnapshotDropped=true`, so resume falls back to intent replay.
3. **Shutdown**: `Close`/`DrainAndHalt` call `Suspend(reason)` **before** cancelling. It CASes `suspending`, injects the notice, `MarkSuspendedAll()`, and publishes `info` on subject `run.suspending`.
4. **Finalize**: `finalizeResumeTicket` keeps the ticket iff `suspending && err is Canceled/DeadlineExceeded`. Otherwise it deletes it (clean, failed, operator-cancelled, timed out).
5. **Boot** (`buildResumer`, cmd/agezt/main_standing_resume.go): for each ticket, quarantine (`resume/quarantine/`, `anomaly.detected` on `run.resume.quarantined`) if it is non-resumable, has `Attempts >= AGEZT_RESUME_MAX_ATTEMPTS` (default 3), or its agent is gone/retired/disabled. Otherwise `IncrementAttempt` (fsynced **before** dispatch), rebuild the ctx (`WithResumeOwned`, `WithAgentProfile`, `WithWakeContext{Reason:"resumed"}`, `WithTrustCeiling`, `WithMaxCost`, `WithRunTimeout`, and `WithResumeSeed` for `KindRun` with messages), publish `info` `run.resumed`, and dispatch on a goroutine through the matching entry point. `ResumeFinalize` runs on return. The resumed loop starts at `StartIter=t.Iter` with a fresh `MaxIter` segment.
6. Anomalies (`run.resume.anomaly`, `anomaly.detected`): `ticket_write_failed`, `snapshot_failed`, `ticket_delete_failed`, `mark_suspended_failed`. Resume is best-effort and never fails a run.

### 4.9 Delegation / sub-agent model (subagent_*.go, kernel/delegation)
- Tools: `delegate` (input `task`, `model`, `task_type`, `agent`, `async`) and `delegate_await` (`spawn_id`). Both declare `Capability{Name: edict.CapDelegate}`. `delegate` effect is compensable; `delegate_await` is reversible. They are registered at Open when `cfg.SubAgentTool` is set (daemon: `AGEZT_SUBAGENT != off`).
- `prepareSubAgent` (identical for sync and async):
  1. Resolve a named agent (`roster.Get`). Reject unknown, retired, paused, or `!AllowsDelegationFrom(caller)`. Managed agents need a live, enabled manager (`validateDelegationManager`).
  2. Model: explicit > profile `Model` > profile `AGEZT_MODEL` override > live `k.Model()`. Task type: explicit > profile > `"delegate"`. The chain is primary + profile `Fallbacks`, filtered by `delegation.KeyedModelChain(…, cfg.ModelAvailable, default)` (M838: drop unkeyed models).
  3. **Rails, enforced before spawn:**
     - depth: `delegation.DepthFromCtx(ctx) >= SubAgentMaxDepth` (kernel default 1 if unset; **daemon default `AGEZT_SUBAGENT_DEPTH=3`**) is refused.
     - fan-out: `k.fanout[parentCorr] >= SubAgentMaxFanout` (`AGEZT_SUBAGENT_FANOUT`, 0 = unbounded).
     - tree total: `k.tree[rootCorr] >= SubAgentMaxTotal` (`AGEZT_SUBAGENT_MAX_TOTAL`; daemon derives **48** when unset and depth > 1, M843).
     - spend: `subAgentSpendMicrocents(parentCorr) >= SubAgentMaxSpendMicrocents` (`AGEZT_SUBAGENT_SPEND_CAP` USD×1e9), computed by scanning the journal for `budget.consumed` under the children's correlations.
     
     A violated rail becomes a tool error the lead model sees ("delegation failed: …").
  4. `childCorr = NewCorrelation()`, actor `subagent-<childCorr>`. Register `steers[childCorr]`. Publish **`subagent.spawned` under the parent correlation** with payload `{task, child_correlation, depth, parent, model, task_type[, agent, autonomy_runbook, wake_source:"delegated", delegated_by, parent_correlation_id][, async]}`.
  5. Child ctx: `delegation.WithDepth(depth+1)`, ctxKeyActor, ctxKeyCorrelation, ctxKeyRoot (the tree root propagates), `WithAgentProfile` for named agents. System = `delegation.SystemPrompt` + (profile soul or live `k.System()`).
- `executeSubAgent`: the same `buildLoopConfig` as root runs (LD-1: cost, compaction, memo, tool policy). Then `subAgentInjectedSystem` (memory/world/skill injection for the child), `TaskType`, `ModelChain`, `WakeSource="subagent"`, `WakeReason="delegation"`, `ParentCorrelation`, `MaxRunCostMicrocents=profile.MaxCostMc`, `Steer=rc`, then `agent.Run`. Profile `RetryPolicy` retries inline (`agent.retry` with `subagent:true`). On exit it releases `fanout[childCorr]` and `steers[childCorr]`. `CompleteAgentLifecycle` runs on success. **No resume ticket and no `k.runs` entry for sync children.**
- Async (`runSubAgentAsync`, M881): `context.WithoutCancel(childCtx)` plus a kernel-owned `WithCancel`. Under `runsMu`→`spawnsMu` it records `spawns[childCorr]` and `runs[childCorr]`, plus `runWG.Add`. A goroutine runs `executeSubAgent` and publishes **`subagent.completed`** under the parent correlation (`{child_correlation, ok, async, error|chars}`) **before** closing `done`. `awaitSubAgent` refuses foreign callers (`caller != parentCorr`). A ctx timeout returns "still running", and the handle remains collectable. A result is collected exactly once. When the parent run ends, `cleanupRunState` cancels every un-awaited spawn whose `rootCorr` or `parentCorr` is the ending run.
- Sub-agents inherit the parent ctx values: trust ceiling, auto-approve caps, tenant, intent frame, memory scope (unless overridden by profile). Daemon-wide `AutoApproveCapabilities` are documented as inherited.
- Cross-node delegation (`remote_run` / mesh) is a different mechanism: `kernel/meshctx` hop counter (section 11).

### 4.10 The Tool interface: registration, description to models, execution
1. **Contract**: `agent.Tool{Definition() ToolDef; Invoke(ctx, json.RawMessage) (Result, error)}`. Declare `ToolDef.Capability` (a missing or unknown capability means DEFAULT-DENY via the Edict fallback). Declare `ToolDef.Effect` for HITL bundles, memo eligibility (read-only) and the injection guard (non-read-only).
2. **Boot registration** (`kernel/toolreg`): each first-party tool package calls `toolreg.Register(Spec{Name, Build, PreOpen, Configure, Late, Netguard, YieldOnConflict})` (from `plugins/builtintools`). The daemon calls `BuildAll(BuildDeps)`, which aborts on a name collision unless the later spec is `YieldOnConflict` (the plugin host loses to in-process). Then `Set.Tools()` → `runtime.Config.Tools`, `Set.ApplyPreOpen(&cfg)`, `Set.ToolCapabilities()` → `Config.ToolCapabilities`, `Set.PluginManifest()` → `Config.Plugins`; after `Open`, `Set.Configure(KernelDeps)` (wires `NetguardAware.SetOnBlock`) and `Set.ConfigureLate(LateDeps)` (channels, board).
3. **Kernel-registered tools** at `Open`: `memory`, `world`, `delegate`, `delegate_await`, `market`, `voice`, `image_generate`, `rerank`.
4. **Per-run dynamic tools** (`buildLoopConfig`): `mergeScriptTools` adds every ACTIVE toolforge script as `forge_<name>` (`forgedTool`, executed through `cfg.ScriptRunner`, capability `code.exec`). `mergeMCPTools` adds live MCP attachments as `mcp_<server>_<tool>` (`bridgedMCPTool`, name sanitized and capped at 64 chars), or one `mcp_<server>` `lazyMCPDispatch` when the server is `Lazy`, honouring per-server `ToolAllow`. Then the agent tool policy, noise policy and allowlist filters apply, then `tool_search` injection.
5. **Description to model**: `buildToolDefs` lints the schemas. Each iteration offers `offeredTools` (minus repeatedly-denied ones), optionally narrowed by the `ToolSelector` (lexical; deferred mode pins `tool_search`). `ToolDef` travels in `CompletionRequest.Tools`, and each provider adapter translates it (`plugins/providers/internal/toolname` sanitizes names). The host-environment preamble also lists the tools in prose (`capabilityBriefing`, `forgeBias`).
6. **Execution**: inside the loop (section 3.5), or shared direct/workflow `RunTool` → `toolexec.Run`. Workflow tool/HTTP/pipeline/canvas calls reach it through `invokeWorkflowTool(ctx, corr, ...)`; parsed outputs and workflow error ports are preserved. Each physical workflow tool attempt appends a ULID to its node/step hint so retries/nested runs cannot reuse an audit identity. Workflow code nodes still call `ScriptRunner` after a separate policy check and await convergence.

### 4.11 Per-agent overrides (agentconfig*.go)
`agentOverrides` maps profile `ConfigOverrides` keys to Config fields: `AGEZT_MODEL`, `AGEZT_MAX_ITER`, `AGEZT_MAX_AUTO_CONTINUE`, `AGEZT_AUTO_CONTINUE_WAIT`, `AGEZT_PARALLEL_TOOLS`, `AGEZT_TOOL_DISCOVERY_MAX`, `AGEZT_CONTEXT_BUDGET`, `AGEZT_OBSERVATION_DELTAS`, `AGEZT_DISABLE_HEURISTIC_BYPASS`. Malformed values are ignored at run time and reported by `agentRuntimeConfigIssues` (used by the reaper's misconfigured-agent scan).

### 4.12 Aux LLM features hosted by the runtime
All of these use `completeAux(ctx, corr, taskType, req)`, the single funnel that fills `CorrelationID` and `TaskType` so governor routing and spend attribution work (LD-5). The direct `cfg.Provider.Complete` call bypasses the agent loop, so there is no `llm.request`/`llm.response`, only governor events.

| Feature | Entry | Task type | Events |
|---|---|---|---|
| Completion verify | `Runner.VerifyCompletion` | `verify` | `assure.verdict` |
| Criteria proof | `ProveTask`/`verifyCriteria` | `verify` | `assure.verdict`, `workboard.task.proved/unproven` |
| Council of Elders | `Council` (`councilRound`, `councilSynthesize`, `councilGrounding` → shared `RunTool(web_search)` with unique search IDs). Refusal/failure yields date-only grounding | per seat model | `council.started/brief/convened/opinion/consensus`; correlated policy/tool search audit and approvals |
| Conductor | `Conduct` (thinker/worker/verifier; verifier may run code via `CodeExecutor.RunScript`) | `conductor` | `conductor.started/step/done` |
| Deep research | `Research` (plan → `RunTool(web_search)` → `RunTool(browser.read)` → synth → `verifyResearchClaims`) | `research` | tool events via toolexec, plus its own |
| Workflows | `RunWorkflow`/`runWorkflowGraph`/`execWorkflowNode`/`TestWorkflowNode`; `DraftWorkflow`/`RefineWorkflow` | `workflow` | `workflow.started/node/completed/failed/drafted/saved/...` |
| Image admission / vision sidecar | `Kernel.AdmitImages` shared by control-plane, REST/OpenAI and channels → `Runner.DescribeImages` (`VisionModel()`; `ErrNoVisionModel`). Confirm primary vision or caption; empty/failed caption rejects; cancelled context stays cancelled | vision | `capability.rerouted`; correlated `capability.rejected` |
| Elision summary | `makeElidedSummarizer` | — | — |
| Memory distill / forge / shadow eval | `MaybeDistill`, `MaybeForge`, `MaybeShadowEval`, `DistillBrain`, `DistillProfile` | (memory/skill pkgs) | `memory.*`, `skill.*` |

---

## 5. Run events emitted (this area)

Subjects for loop events are `agent.<actor>.<suffix>`, with actor `agent-<corr>` (root) or `subagent-<childCorr>`. Every event carries `CorrelationID`. Ephemeral events use `bus.PublishStreaming` (not journaled).

| Kind | Emitter | Subject suffix | Payload fields |
|---|---|---|---|
| `task.received` | `agent.Run`, heuristic bypass | `.task` | `intent`, `images`, `agent`, `wake_source`, `wake_reason`, `schedule_id`, `standing_id`, `standing_name`, `trigger_subject`, `parent_correlation` (absent when empty) |
| `task.continued` | `agent.Run` | `.task` | `attempt`, `of`, `iters_so_far` |
| `task.completed` | `agent.Run`, bypass | `.task` | `iters`, `chars`, `stopped` (stop reason or `heuristic_bypass`), `answer` (≤8192 runes) |
| `task.failed` | `agent.Run` defer | `.task` | `error`, `reason` (`panic`/`max_iters`/`cost_budget`/`canceled`/`timeout`/`error`) |
| `run.steered` | `agent.Run` | `.steer` | `iter`, `directive`, `mode` (`steer`/`note`) |
| `context.compacted` | `agent.Run` | `.context` | `elided`, `reclaimed_chars`, `context_chars_before`, `context_chars_after`, `budget`, `skill_rescued_count`, `skill_rescued_chars` |
| `llm.request` | `agent.Run` | `.llm` | `iter`, `messages`, `model`, `tools`, `context_chars`, `context_by_role`, `tool_discovery`, `tools_before_discovery` |
| `llm.token` / `llm.reasoning` (ephemeral) | `callProvider` | `.llm` | `iter`, `text` |
| `llm.response` | `agent.Run` | `.llm` | `iter`, `stop_reason`, `usage`, `text_chars`, `reasoning_chars`, `tool_calls` |
| `policy.decision` | `gateToolCalls` (`.policy`); `toolexec.Run` (subject `policy`, 8 fields) | `.policy` | `tool, call_id, capability, allow, reason, would_ask, hard_denied, effect_class, affected_resources, epistemic_action, epistemic_reason, epistemic_signals, epistemic_confidence, failure_matches, weighted_failures, schema_hash, input_shape, temporal_sensitive, novel_tool, untrusted_observation, observation_sources, directive_like, directive_matches` |
| `tool.invoked` | gate / toolexec | `.tool` | `tool`, `call_id`, `input` |
| `tool.result` | finalize / toolexec | `.tool` | `tool, call_id, output, error, observation_trust, observation_source, directive_like, directive_matches`, plus `raw_ref, output_bytes` (offload), `observation_delta, model_output_bytes, raw_output_bytes`, `memo_hit` |
| `intent.interpreted` | `publishIntentInterpreted` | — | intent `Frame` (hash, canonical intent, ambiguity, …) |
| `intent.confirmation_required` | policyHook | — | frame, regret axes, prompt |
| `policy.auto_approved` | `publishAutoApprove` | `policy.auto_approved` | capability, tool |
| `prompt_injection.warned` | `publishPromptInjectionWarned` | `prompt_injection.warned` | tool, capability, sources, trusted_run |
| `approval.requested/granted/denied/timeout` | `approval.Registry` (see 05) | — | — |
| `subagent.spawned` | `prepareSubAgent` (parent corr) | `agent.subagent-<c>.subagent` | see 4.9 |
| `subagent.completed` | async goroutine (parent corr) | same | `child_correlation`, `ok`, `async`, `error`/`chars` |
| `agent.retry` | `RunWithRetry`, `executeSubAgent` | `agent[.<slug>].retry` | `agent, attempt, next_attempt, max_attempts, reason, error, delay_ms, backoff, base_delay_sec, max_delay_sec, retry_on` / `subagent` |
| `assure.verdict` | `VerifyCompletion`, `verifyCriteria` | `agent.agent-<corr>.assure` | `complete`, `gap`, `criteria` |
| `run.paused/resumed/stepped` | steer ops | `kernel.steer` | `correlation_id` (+ `primitive`, `lease_expires_unix`) |
| `run.intervention` | `InterveneRun` | `kernel.intervention` | `primitive, correlation_id, accepted, applied, state, paused, pending, scope, idempotency_key, reason, lease_expires_unix, directive` |
| `halt` / `resume` | lifecycle | `kernel.halt` / `kernel.resume` | `cancelled_runs`, `reason` |
| `info` | Suspend; resumer | `run.suspending` / `run.resumed` | `suspended_runs`, `reason` / `corr, agent, kind, iter, attempt, seeded` |
| `anomaly.detected` | resume, Close | `run.resume.anomaly`, `run.resume.quarantined`, `kernel.shutdown` | `anomaly`, `severity`, `correlation_id`, `error` |
| `context.selection` | `publishContextSelection` | `context.selection` | `contextselect.Manifest{phase, query, budget_chars, chosen, rejected, summary}`. Phases: `memory`, `world`, `skill`, `failure_analysis` |
| `taste.injected` | buildRunPrompt | `agent.agent-<corr>.taste` | `count`, `ids`, `scope` |
| `roster.updated` | `CompleteAgentLifecycle` | `roster.<slug>` | `slug, action:"lifecycle_cycle_completed", completed_cycles, max_cycles, run` |
| `scripttool.*`, `mcp.*`, `workflow.*`, `workboard.task.*`, `okr.*`, `standing.*`, `roster.*`, `council.*`, `conductor.*`, `reflection.completed` | the respective kernel methods | — | see the owning docs |

---

## 6. runtime sub-packages

| Package | Files | Status / role |
|---|---|---|
| `runtime/runexec` | `doc.go`, `api.go` (`KernelAPI`, ~85 methods), `runner.go` (`Runner`, `New`, `Run`, **`RunWith` — the real run body**), `runner_lifecycle.go` (`CompleteAgentLifecycle`, `RunAssured`, `MaybeDistill/Forge/ShadowEval`), `runner_retry.go` (`RunWithRetry`, `Why/Causes/ParentOf/Verify`, `VerifyCompletion`, `PublishHeuristicBypass`, `DescribeImages`, `ErrNoVisionModel` duplicate), `helpers.go` (`ErrHalted` duplicate, `shadowEvalLimit=2`, `assureVerifyMaxTokens=400`, `visionDescribeMaxTokens=1024`, `buildTranscript`, `deterministicHeuristicBypass`, `shouldRetireAgentAfterComplete`, `resetCompletedCycleTasks`) | Live. Never imports kernel/runtime; reaches the kernel through public wrapper methods (`SetupRunState`, `BuildRunPrompt`, `ClaimResumeTicket`, …). Test: `helpers_test.go`. |
| `runtime/lifecycle` | `doc.go`, `api.go` (`KernelAPI`: `Halted/SetHalted/Runs/RunsMu/RunWG/Suspend/PublishBus`), `manager.go` | Live (section 4.6). Test: `manager_test.go`. |
| `runtime/accessors` | `doc.go`, `api.go`, `accessors.go`, `accessors_mutate.go` (48 funcs: getters, standing/roster CRUD with events) | **Constructed in Open (`k.accessors`) but never called.** `*Kernel`'s own `accessors_*.go` methods re-implement the same logic. Duplicate code, drift risk. Test: `accessors_test.go`. |
| `runtime/compose` | `doc.go`, `api.go` (`OpenAPI` interface), `stores.go` (intentionally empty) | **Dead**: no importers. Day-20b extraction rolled back; `Open` lives in `runtime/compose.go`. |
| `runtime/types` | `doc.go`, `types.go` (`SubAgentLimits`, `PluginInfo`, `CouncilMember`) | Shared value types aliased by runtime (`PluginInfo`, `CouncilMember`, `SubAgentLimits`). |

---

## 7. kernel/runtime file-by-file

Root package, grouped by concern; every non-test file is listed.

**Composition and core**

| File | What it does |
|---|---|
| `doc.go` | Package doc: composition root, one Kernel per process, sub-package split. |
| `lifecycle.go` | Stub (package clause only). |
| `runtime.go` | `Config` (every runtime knob: tools, subagent rails, edict/warden/approvals, memory/world/skill/taste/profile injection, context budget, guards, resume, closures VisionModel/ModelAvailable/CouncilMembers, OnReload) and the `PluginInfo` alias. |
| `runtime_kernel.go` | `Kernel` struct, mutex inventory and lock order. |
| `compose.go` | `Open` (store opening, tool registration, catalog, agentgw), `DefaultShutdownDrainTimeout`. |
| `compose_close.go` | `Close` (Suspend, Halt, bounded drain, MCP detach, store close), `closeAll`. |
| `runtime_accessors.go` | Simple getters (Journal, Bus, State, Edict, Warden, Approvals, Scheduler, Provider, Memory, AgentGateway, Schedules, Tools), `ErrHalted`. |
| `runtime_lifecycle.go` | KernelAPI wrappers for lifecycle/runexec (`Halted`, `RunsMu`, `RunWG`, `SetupRunState`, `CleanupRunState`, `DeregisterRunSteer`, `PublishBus`, …) and the public lifecycle API (`Halt`, `HaltWith`, `DrainAndHalt`, `CancelRun`, `Resume`, `ActiveRuns`, `NewCorrelation`, `SubjectForRun`). |
| `accessors.go`, `accessors_ctx.go`, `accessors_misc.go`, `accessors_roster.go`, `accessors_standing.go` | Read/mutate surface: schedule engine, world/forge/market, ctx-derived getters for runexec (`WakeContext*`, `ImagesFromCtx`, `ResumeSeedFromCtx`, …), `Model/SetModel`, `System/SetSystem` (live, configMu), `Catalog/ReloadCatalog/Reload` (calls `OnReload`), `LoopRunner`, `RunPlan`, `SubAgentLimits`, roster CRUD (`AddProfile`, `UpdateProfile`, `SetProfileRetired`, `AgentImpact`), standing CRUD (`AddStanding`, …) with `standing.*`/`roster.*` events. |
| `toolseams.go` | Aliases `Reranker`, `ImageGen`, `Voice` to the seam interfaces in reranktool/imagetool/voicetool. |
| `taste.go` | `Taste()` and `Seats()` getters. |

**Run engine and context**

| File | What it does |
|---|---|
| `runexec.go` | `Run`, `RunAssured`, `RunWithRetry` wrappers; `ClaimResumeTicket`, `FinalizeResumeTicket`, `RetryReason`, `AgentRetryable`, `RetryDelay` KernelAPI wrappers. |
| `runexec_lifecycle.go` | `RunWith` (corr check, delegates to the Runner), `CompleteAgentLifecycle`, `setupRunState`, `cleanupRunState` (5-mutex release, orphan spawn cancels, `runWG.Done`), `deregisterRunSteer`, `ErrNoVisionModel`. |
| `runexec_helpers.go` | `retryReason`, `agentRetryable` (default RetryOn `error`,`timeout`), `retryDelay`, the elided-summary constants, `makeElidedSummarizer`. |
| `runexec_delegates.go` | `FoldRunTools` (journal scan for tool.result), `Why`, `Causes`, `ParentOf`, `Verify`. |
| `loopconfig.go` | `BuildLoopConfig`/`buildLoopConfig`: the shared LoopConfig base for root and sub-agent runs. |
| `agentconfig.go` | `agentOverrides` table, `applyAgentOverrides`, `agentRuntimeConfigIssues`, `effectiveConfig`, `resolveRunModel`. |
| `agentconfig_helpers.go` | `overrideString/Int/Bool/Duration` setter adapters. |
| `agentconfig_parsers.go` | Raw override lookup and typed parsers. |
| `completeaux.go` | `completeAux`/`CompleteAux`: the aux-completion funnel. |
| `runctx.go` | `ctxKey` enum (24 keys), `agentToolPolicy`, `agentNoisePolicy`/state, `WakeContext`. |
| `runctx_agent.go` | `WithAgentProfile` (system agent, soul→WithSystem, lifecycle, retry policy, tool policy, noise, overrides, trust ceiling, model+chain, memory scope, workdir, ident), `WithAgentIdent`, noise-policy resolution. |
| `runctx_basic.go` | `WithTrustCeiling`, `WithImages`, `WithJSONMode`, `WithModel`, `WithSystem`, `WithRunTimeout`, `WithMaxCost`, `rootFromCtx`, and their getters. |
| `runctx_security.go` | `WithAutoApproveCapabilities`/merge/`autoApproveCap`, `PromptInjectionMode` + `ParsePromptInjectionMode`, `WithTrustedObservations`. |
| `runctx_wake.go` | `WithWakeContext`, `systemAgentFromCtx`, tool/retry policy getters, `AgentConfigOverrides`, `WithModelChain`, `WithTools`, `actorFromCtx`, `correlationFromCtx`. |
| `image_admission.go` | Shared image admission and `ImageAdmission{Intent, Images, Caption}`; caption text replaces raw refs before the primary run, while channel artifacts retain the caption. |
| `resume.go` | Resume integration (section 4.8): `ResumeStore`, `WithResumeSeed`, `WithResumeOwned`, `claimResumeTicket`, `buildResumeTicket`, `resumeCheckpointFn`, `finalizeResumeTicket`, `ResumeFinalize`, `Suspend`, `publishResumeAnomaly`. |
| `steer.go` | `runControl` (Wait/Drain/pause/resume/step/inject/snapshot/idempotency), `controlFor`. |
| `steer_ops.go` | `PauseRun`, `ResumeRun`, `StepRun`, `SteerRun`, `RunControlState`, `InterveneRun`, `publishSteer`, `publishIntervention`. |

**Prompt assembly and context selection**

| File | What it does |
|---|---|
| `prompt.go` | `agentProfileSystem` (soul plus task list rendering), `profileTasksByScope`, `writeProfileTasks`. |
| `prompt_context.go` | `injectMemory`, `injectUserProfile`, `injectTaste`, `injectWorld`, `injectSkills` (text blocks prepended to the system prompt). |
| `prompt_run.go` | `buildRunPrompt` (layered injection with `context.selection` manifests) and the `injectHostEnvironment` doc. |
| `prompt_env.go` | `(*Kernel).injectHostEnvironment` (gated on `EnvironmentInject`). |
| `prompt_environment.go` | `injectEnvironment` (OS/arch, shell hint via `shellHinter`, workspace, date, tool briefing), `capabilityBriefing`, `forgeBias`, `shellGuidance`. |
| `context_selection.go` | `publishContextSelection`, `publishContextFailureAnalysis` (journal scan for the last manifest, then suspects), candidate adapters. |

**Policy and safety**

| File | What it does |
|---|---|
| `policy.go` | `validatedToolCaps`, `capabilityFor`, `policyHook` (section 4.5). |
| `policy_helpers.go` | Noise policy denial/notify (`agentNoisePolicyDenial`, `completeAgentNoiseNotify`, state in `state` ns `agent_noise`), `approvalBundle`/`approvalDecisionBundle`, `defaultEffectClass`, `affectedResourcesFromInput`, rollback/confidence defaults. |
| `epistemic.go` | `epistemicDecision`, `epistemicGate`, `epistemicHistoryLimit=4096`. |
| `epistemic_helpers.go` | `shouldEscalateEpistemic`, `confidenceFloor`, `hashSchema`, `inputShape`, `schemaPermissive`, `temporalSensitive`, `matchHistoricalToolOutcomes` (journal scan), decay helpers. |
| `intent.go` | Publishers: `intent.interpreted`, `intent.confirmation_required`, `policy.auto_approved`, `prompt_injection.warned`; `regretAxesPayload`. |
| `toolpolicy.go` | `applyAgentNoisePolicyToPromptTools`, `filterTools`, `applyAgentToolPolicy`, `agentToolPolicyDenial`. |

**Tools**

| File | What it does |
|---|---|
| `toolrun.go` | `LookupTool`, `CheckPolicy`, `PublishEvent`, `NotifyNoise` (toolexec interfaces), `RunTool`, private `runToolWithLookup` for runner adapters. |
| `code_execution.go` | Invocation-local `code_exec` adapter around the existing `CodeExecutor`/`ScriptRunner`; trusted code.exec capability/effects, shared invoker and unique IDs, actual executor-entry marker for Conductor `Ran`. Does not register/replace a tool or change sandbox/network settings. |
| `toolsearch.go` | `tool_search` deferred-catalog tool (`withToolSearch`, lexical scoring; default 8, max 20 results). |
| `scripttool.go` | Toolforge lifecycle with `scripttool.*` events (`DraftScriptTool`, `UpdateScriptTool`, `TestScriptTool`, `PromoteScriptTool`, `RequestToolPromotion` via approval capability `toolforge.promote`, `QuarantineScriptTool`, `RemoveScriptTool`), `mergeScriptTools`, `forgedTool`. |
| `mcptool.go` | MCP lifecycle: `AddMCPServer`, `SetMCPServerEnabled`, `AttachMCPServer` (stdio `mcp.Dial` or HTTP `mcp.DialHTTP`), `DetachMCPServer`, `RemoveMCPServer`, `AttachEnabledMCPServers`, `MCPAttached`, `closeMCPConns`. |
| `mcptool_bridge.go` | `mcpToolName`, `mergeMCPTools`, `bridgedMCPTool`, `lazyMCPDispatch`. |
| `subagent.go` | `subAgentTool` struct (run/spawn closures). |
| `subagent_tool.go` | `delegate`/`delegate_await` Definition and Invoke, `spawnHandle`, `subAgentPrep`. |
| `subagent_prep.go` | `prepareSubAgent` (identity, model chain, all rails, spawn event, child ctx), `validateDelegationManager`. |
| `subagent_exec.go` | `executeSubAgent` (child loop plus retry). |
| `subagent_run.go` | `runSubAgent`, `runSubAgentAsync`, `awaitSubAgent`. |
| `subagent_inject.go` | `subAgentInjectedSystem` (memory/world/skill injection for children), `subAgentSpendMicrocents` (journal sum of `budget.consumed` over the parent's children via `delegation.SpawnLink`/`BudgetCostMicrocents`). |

**Domain features hosted on the Kernel**

| File | What it does |
|---|---|
| `workboard.go` | Workboard state transitions with `workboard.task.*` events (create/claim/heartbeat/comment/block/fail/unblock/retry policy/complete/review/archive/link/dependency/reclaim/sweep), `assureCriteriaMaxTokens=800`. |
| `workboard_proof.go` | `ProveTask`, `verifyCriteria`, `parseCriteriaVerdict`, `gatherProofEvidence` (artifacts plus journal seq range under corr). |
| `okr.go` | OKR CRUD and rollup from workboard done-status, `okr.*` events. |
| `council.go`, `council_helpers.go` | Council of Elders: multi-seat, multi-round deliberation with chair synthesis and web grounding. |
| `conductor.go`, `conductor_helpers.go`, `conductor_steps.go` | Conductor: thinker plan, worker, verifier (code execution via `CodeExecutor` or LLM critique), with rounds. |
| `research.go`, `research_helpers.go`, `research_prompts.go`, `research_run.go`, `research_verify.go` | Deep research: sub-question plan, search/fetch through `RunTool`, cited synthesis, adversarial claim verification, confidence. |
| `workflowrun.go`, `workflowrun_graph.go`, `workflowrun_node.go`, `workflowrun_helpers.go`, `workflowtestnode.go` | Workflow execution: `RunWorkflow` (step cap 256), graph walk, per-node retry/timeout (`execNodeWithReliability`), node kinds (llm, tool, code_exec, pipeline, subflow depth ≤3, conditions/items), `TestWorkflowNode`. |
| `workflowdraft.go` | Workflow copilot `DraftWorkflow`/`RefineWorkflow` (one repair round; returned unsaved), `autoLayoutWorkflow`. |
| `braindistill.go` | `DistillBrain`, `DistillProfile` wrappers over kernel/memory. |
| `reaper.go`, `reaper_scan.go`, `reaper_health.go`, `reaper_routing.go`, `reaper_probation.go`, `reaper_helpers.go` | Read-only staleness and health scans (`ReaperScan`): idle/degraded/misconfigured agents, routing/retry pressure, forced probation, stale artifacts. Env knobs `AGEZT_ROUTING_PRESSURE_THRESHOLD/WINDOW`, `AGEZT_RETRY_PRESSURE_THRESHOLD/WINDOW`, `AGEZT_ROUTING_FORCE_PROBATION`, `AGEZT_ROUTING_UNSTABLE_THRESHOLD/WINDOW`. |

Tests (59 files), grouped:
- Loop integration and approvals: `runtime_test.go`, `approval_test.go`, `autoapprove_test.go`.
- Guards and safety: `promptinjection*_test.go`, `capability_guard_test.go`, `policy_capability_internal_test.go`, `toolcaps_test.go`.
- Delegation: `subagent*_test.go` (async, cycle, keyed chain, roster, runcost).
- Lifecycle and resume: `drain_test.go`, `closeall_internal_test.go`, `resume_lifecycle_test.go`, `runtime_steer_test.go`, `steer_internal_test.go`.
- Context: `context_budget_test.go`, `context_selection_test.go`, `environment*_test.go`.
- Workflows: `workflow*_test.go`.
- Features: `council`, `conductor`, `research`, `reaper`, `epistemic`, `intent`, `tenant`, `world`, `memory`, `skill`, `scripttool`, `mcptool`, `toolsearch`.

---

## 8. kernel/toolexec, kernel/toolreg, kernel/toolforge

### kernel/toolexec
- W2.3f adds `platform/tooloutput.Offload`, moving the agent's existing audit representation unchanged: 8 KiB default, byte threshold, full artifact bytes plus preview/ref, inline fallback on nil store/Put failure/empty ref. Three mutations and original source tests preserve the contract. **Fixed W2.3g:** runtime paths use `RunWithOptions` with the runtime store/effective threshold. Every terminal outcome shares the audit representation while caller/hook output stays complete; legacy `Run` retains its signature and inline behavior. Tests: `toolexec/offload_test.go`, runtime `tool_offload_audit_test.go` (four actual paths, configured threshold); seven mutations cover the boundaries.
- W2.3d extracts the existing invocation-only panic firewall to `kernel/platform/toolinvoke.Invoke`. The forwarding helper retains direct-call behavior; the primitive returns the recovered value so agent-loop orchestration can retain its own typed panic classification. Lookup, schema, policy, timeout and journal remain caller responsibilities. Contract tests preserve one call, result/error identity, correlation context and panic text; three mutations fail. This is a foundation, not the complete app pipeline.
- `toolrun.go`: interfaces `ToolLookup`, `PolicyChecker`, `EventPublisher`, `NoiseNotifier`. `Run(ctx, corr, callID, toolName, args, …)`: runtime lookup includes registered + active forge + live MCP tools (registered names win); schema validation; trusted resolved `ToolDef` and tool correlation in the policy context; `CheckPolicy`; mandatory `policy.decision` (8-field payload); allow → mandatory `tool.invoked` → panic-contained invocation → `tool.result`; deny → failed `tool.result` with no invocation/no noise callback. Audit failures before invocation prevent execution; terminal audit failures are reported alongside an invocation error or refusal. Existing unknown-tool/schema rejections remain before policy. Used by direct/workflow tool/HTTP/pipeline/canvas and Council grounding through `(*Kernel).RunTool`; Conductor and workflow code runners use invocation-local adapters through the same `runToolWithLookup` machinery (W2.3c). Tests: `toolrun_test.go`, runtime `workflow_tool_audit_test.go`, control-plane `tool_audit_identity_test.go` (log/stats join within the run and omit latency samples on denial). Agent-loop convergence, output offload and full policy-payload convergence remain W2.3 work.

### kernel/toolreg
- `toolreg.go`: package doc (4 lifecycle phases), `BuildDeps` (BaseDir, WorkspaceRoot, Warden, Stderr, Get, AllowAll, NotifyTargets), `KernelDeps` (K, Bus, Artifacts, Lake, Journal, BaseDir, Stdout, NetguardPublish), `LateDeps` (ChannelSend, ChannelSendMedia, Board, BoardNotify), `Built` (Tool, Extra, Desc, Caps, Infos), `Spec`, `NetguardAware`, the global registry (`Register`, `Names`, `snapshot`; `regMu` RWMutex), `Set`.
- `toolreg_set.go`: `BuildAll`, `add` (collision and YieldOnConflict), `Tools`, `ApplyPreOpen`, `Configure`, `ConfigureLate`, `Descs`, `PluginManifest`, `ToolCapabilities` (skips dropped names), `NetguardGaps` (a registry-driven guard test asserts this is empty).
- `Names()` exists only for the `plugins/builtintools` ratchet test (deadcodecheck allowlist). Test: `toolreg_test.go`.

### kernel/toolforge
- `doc.go`: lifecycle draft → test → (operator) promote → active `forge_<name>`; quarantine is the kill switch; any code edit demotes to draft and clears the test record.
- `toolforge.go`: `ScriptTool{ID, Name, Description, Language, Code, InputSchema, Status, TestedOK, TestedMS, CreatedMS, UpdatedMS}`; `Status` draft/active/quarantined; `Runner.RunScript(ctx, language, code, inputJSON)`; `Validate` (name `^[a-z][a-z0-9_]{0,39}$`, code ≤128 KiB, desc ≤2 KiB, schema ≤16 KiB); `ErrNotFound`, `ErrUntested`; `safeCall`.
- `toolforge_store.go`: `Store` (single JSON `toolforge/scripttools.json` via `filestore`, mutex) with `Add`, `Update`, `RecordTest`, `Promote` (requires `TestedOK`), `Quarantine`, `Remove`, `Get`, `List`, `Active`, `Count`.
- Tests: `toolforge_test.go`, `r7_proof_test.go`, `coverage_supp_test.go`.

---

## 9. kernel/contextselect, convo, delegation, intent, intervention, planner, reflect

| Package / file | Content |
|---|---|
| `contextselect/doc.go` | Package doc. |
| `contextselect/context.go` | `CandidateLimit=12`, `rejectedLimit=5`, `Candidate` (source, id, score, tokens, hard/soft/risk cost, freshness, confidence, chosen, reason, signals), `Manifest`, `SplitCandidates`, `Summary`, `ChosenIDSet`, `CandidateIDs`, `FailureAnalysisSuspects`. |
| `contextselect/context_adapters.go` | `MemoryCandidates`, `WorldCandidates`, `SkillCandidates`. |
| `contextselect/context_scoring.go` | `TokenCost` (~4 chars/token), `Freshness`, `Risk`, skill confidence. |
| `convo/convo.go` | `Turn{Role, Text}`, `TranscriptIntent(turns)`: a lone user turn passes through verbatim; otherwise system/developer guidance goes first, then a `User:`/`Assistant:` transcript. Used by controlplane, openaiapi and webui to fold multi-turn chat into one intent (the loop is single-intent). |
| `delegation/doc.go`, `delegation.go` | `DepthKey`, `SystemPrompt` (used), and `SpawnHandle`, `Prep`, `DefaultSubAgentMaxDepth=8`, `SubAgentTool`, `SubAgentAwaitTool` (**unused duplicates** of runtime's live implementations; different schema field `agent_ref` vs `agent`). |
| `delegation/delegation_context.go` | `DepthFromCtx`, `WithDepth`. |
| `delegation/delegation_helpers.go` | `SpawnLink`, `BudgetCostMicrocents`, `KeyedModelChain`, `AppendUniqueStrings`/`String`, `ValidateSpawnTask`, `FormatDuration`. |
| `intent/intent.go` | `Frame{UserUtteranceHash, CanonicalIntent, Assumptions, ExplicitExclusions, CandidatePlans, HarmfulReading, AmbiguityScore, Underdetermined}`, `Action`, `RegretAxes{Physical, Informational, Social, Identity}` with `Max`/`Sum`. `Interpret` is deterministic keyword heuristics (EN+TR): mutating + broad scope → 0.85, mutating → 0.6, read-only → 0.15, else 0.3; underdetermined at ≥0.6. `RegretForAction` (effect class + keyword haystack). `RequiresConfirmation` (underdetermined && (max≥0.75 or sum≥1.0)). `ConfirmationPrompt`. |
| `intent/context.go` | `WithFrame`, `FrameFromContext`. |
| `intervention/intervention.go` | `Primitive` (halt/abort/redirect/adjust/query), `Request`, `Result`, `Normalize`, `DefaultLease=5m`. Wire contract only. |
| `planner/doc.go` | Single-call DAG planner, node kinds `loop`/`gate`, validation rules. |
| `planner/planner.go` | `TaskType="plan"`, `SystemPrompt`, `Config{Provider, Model, MaxTokens, SystemOverride}`, `DefaultMaxTokens=2048`, `Generate`, `GenerateFromIntent` (sends the intent Frame JSON, not the raw utterance), `Plan`, `Node`, `extractJSONBlock`. |
| `planner/planner_validate.go` | `ValidateJSON`, `parseAndValidate`, `validateDAG` (Kahn topological walk), `validateIntentBoundary` (an underdetermined intent requires a `gate` ancestor before any high-regret `loop` node). |
| `planner/refine.go` | `RefineSystemPrompt`, `Refine` (operator feedback). |
| `planner/cost.go` | `CostEstimate`, `EstimateCost` (3000 in / 1000 out tokens per node), `CostEstimator`, `FormatUSD`. |
| `reflect/reflect.go` | `Engine` (`New(journal, world, bus, Config)`), `Reflect(ctx, corr)` (full-journal fold into `Observations`, `world.Decay`, advisory `Proposals`, never raises autonomy), `Latest`, publishes `reflection.completed`. No own state. |

The planner is used by controlplane/`agt plan` and executes through the scheduler (`RunPlan`, `LoopRunner`). It is not part of the per-run loop.

---

## 10. kernel/assure and kernel/proof

- `assure/assure.go`: `DefaultMaxAttempts=3`, `MaxMaxAttempts=10`, `Verdict{Complete, Gap}`, `RunFn`, `VerifyFn`, `Attempt`, `Result{Answer, Complete, Attempts, Gap, History}`. `Until`: run, then verify against the **original** task; if incomplete, retry with `retryInstruction(task, gap)`. A run or verify error aborts. `ParseVerdict` tolerates fences and prose; the kernel treats unparseable output as incomplete.
- `proof/proof.go`: `Criterion{Text, Met, Note}`, `Evidence{Corr, Artifacts, JournalFrom, JournalTo}`, `Proof{Verdict, Criteria, Evidence, Attempts, Judge, ProvedMS}`, `Satisfied()` (verdict complete AND all criteria met), `UnmetCount`, `Clone`. Pure data.
- Flow: `RunAssured` → `assure.Until` → per attempt `RunWith`/`RunWithRetry` (same corr; `CompleteAgentLifecycle` is idempotent per corr) → `VerifyCompletion` (`completeAux` "verify", 400 max tokens) → `assure.verdict`. Workboard: `ProveTask(ctx, corr, id, answer)` → `gatherProofEvidence` → `verifyCriteria` (falls back to `VerifyCompletion` with no criteria) → `workboard.Prove` → `workboard.task.proved|unproven` → `recomputeOKRForTask`. Tasks without criteria are never gated (default-allow posture).
- Tests: `assure_test.go`, `parse_verdict_test.go`, `proof_test.go`.

---

## 11. kernel/resume and kernel/meshctx

| File | Content |
|---|---|
| `resume/doc.go` | Ticket lifecycle; imports only kernel/agent (+atomicfile) to avoid a cycle with runtime. |
| `resume/resume.go` | `KindRun/KindAssured/KindRetry`, `StatusActive/StatusSuspended`, `DefaultSnapshotMaxBytes=2 MiB`, `Ticket` (see 4.8), `Store{dir, quarDir, maxBytes, mu}`, `Open` (mkdir `quarantine`, 0700). |
| `resume/resume_helpers.go` | `Dir`, `safeName` (strips `/ \ .. :`), `path` (`<dir>/<corr>.json`), `writeAtomic` (atomicfile 0600). |
| `resume/resume_ops.go` | `Put`/`putLocked` (stamps times; drops messages above cap), `Snapshot` (no-op if the ticket is gone, so a deleted ticket is never resurrected), `Get`, `List`, `Delete`, `MarkSuspendedAll`, `IncrementAttempt`, `Quarantine`. All under `Store.mu`. |
| `meshctx/meshctx.go` | Cross-node delegation hop guard: `MaxHops=8`, `EnvMaxHops="AGEZT_MESH_MAX_HOPS"` (valid 1..64), `HopHeader="X-Agezt-Mesh-Hop"`, `MaxHopsFromEnv`, `MaxHopsConfig`, and ctx helpers. The REST handler reads the header and refuses above the limit; `remote_run` forwards hop+1. |

---

## 12. Config / env vars touching this area
Read **directly** in scope: `AGEZT_AGENTGW_SOCKET` (`runtime.Open`), `AGEZT_MESH_MAX_HOPS` (meshctx), the reaper knobs (4.12/7), and the per-agent override keys (4.11, read from profile `ConfigOverrides`, not the process env).

Everything else reaches `runtime.Config` through `cmd/agezt/internal/daemonconfig` (see 01): `AGEZT_MAX_ITER`, `AGEZT_MAX_AUTO_CONTINUE`, `AGEZT_AUTO_CONTINUE_WAIT`, `AGEZT_PARALLEL_TOOLS`, `AGEZT_TOOL_DISCOVERY_MAX`, `AGEZT_CONTEXT_BUDGET` (`auto`), `AGEZT_ARTIFACT_THRESHOLD`, `AGEZT_OBSERVATION_DELTAS`, `AGEZT_SUBAGENT`, `AGEZT_SUBAGENT_DEPTH` (3), `AGEZT_SUBAGENT_FANOUT`, `AGEZT_SUBAGENT_SPEND_CAP`, `AGEZT_SUBAGENT_MAX_TOTAL` (48 derived), `AGEZT_RESUME` (default on), `AGEZT_RESUME_MAX_ATTEMPTS` (boot resumer), `AGEZT_USER_PROFILE`, `AGEZT_TASTE_INJECT`, `AGEZT_COUNCIL_WEBSEARCH`, `AGEZT_ALLOW_ALL` (Edict unknown-allow), and the prompt-injection mode.

---

## 13. Extension points

- **Add a tool**: implement `agent.Tool`; set `ToolDef.Capability` (multi-axis via `Field`/`ByValue`, with a safe fallback `Name`) and `ToolDef.Effect`; set `Result.ObservationTrust=untrusted` for external-world content. Register a `toolreg.Spec` in `plugins/builtintools` (and update its ratchet test). Add `NetguardAware` if it makes outbound connections. If the capability is new, add it to Edict (see 05); an undeclared or unknown capability is default-denied.
- **Add a provider**: implement `agent.Provider` (optionally `StreamingProvider`; must return the same response). Honour `Params`, `JSONMode`, `ProviderOptions[family]`. Never read the Governor-only hints. See 08.
- **Add a per-run knob**: add a field to `runtime.Config`, thread it in `buildLoopConfig` (shared by root and sub-agent; never only in `RunWith`), and if it is per-agent tunable, add an `agentOverrides` row. New `AGEZT_*` read in cmd/agezt must be listed in controlplane `configEnvVars`.
- **Add a run-context value**: add a `ctxKey` constant plus `WithX`/`xFromCtx` in `runctx_*.go`. If resume must reconstruct it, add it to `resume.Ticket` and `buildResumeTicket`, otherwise mark the ticket `Resumable=false`, and handle it in the boot resumer.
- **Add a policy guard**: implement it in `policyHook` after the Edict decision, set `guardRaised` so auto-approve cannot satisfy it, and add fields to `contract/policyapi.PolicyVerdict` and `platform/toolaudit.PolicyDecisionPayload` if they must be audited (both loop and direct paths use that renderer).
- **Add an aux LLM feature**: call `k.completeAux(ctx, corr, "<taskType>", req)`, never `cfg.Provider.Complete` directly, so spend is attributed.
- **Steering primitive**: extend `kernel/intervention.Primitive` and the switch in `InterveneRun`. Effects must apply via `runControl` at the iteration boundary.
- **Context injection layer**: add an `injectX` in `prompt_context.go`, call it in `buildRunPrompt` (and in `subAgentInjectedSystem` for children), and publish a `contextselect.Manifest`.

---

## 14. Gotchas / invariants / surprises

1. **Lock order** `configMu < runsMu < fanoutMu < treeMu < steersMu < spawnsMu < mcpMu` is load-bearing; `-race` tests guard it. `setupRunState` takes `steersMu` inside `runsMu`. `runSubAgentAsync` takes `runsMu` then `spawnsMu`.
2. **One corr, one live run** (M480). Re-running under the same corr is only legal sequentially (RunAssured / RunWithRetry / resumer).
3. **Stale comments in runexec.** `runexec/doc.go`, `runner.go`'s `Runner` doc, `runexec.go`'s file comment and `compose.go` say "the 260-line RunWith body stays on *Kernel". It does not: the full body is `runexec.Runner.RunWith`. `spawnHandle`'s comment says "Guarded by k.mu"; the actual lock is `spawnsMu`.
4. **Dead or duplicate code:**
   - `kernel/runtime/compose` has no importers.
   - `kernel/runtime/accessors` is constructed but never called (`k.accessors` has zero uses); `*Kernel` re-implements the same standing/roster mutators.
   - `kernel/delegation`'s `SubAgentTool`, `SubAgentAwaitTool`, `SpawnHandle`, `Prep`, `DefaultSubAgentMaxDepth=8` are unused duplicates of runtime's live versions, with a divergent schema (`agent_ref`).
   - `ErrHalted` and `ErrNoVisionModel` are each defined twice with different identities (runtime and runexec); the wrapper translates via `errors.Is` for `ErrNoVisionModel` only.
5. **Probable bug: `tool_search` capability.** `toolSearchTool.Definition()` declares no `Capability`, and `edict.CapabilityForToolCall` has no `tool_search` case. It resolves to `Capability("tool_search")`, which Edict default-denies unless `AGEZT_ALLOW_ALL`. So with `AGEZT_TOOL_DISCOVERY_MAX` set, the pinned discovery tool is likely refused (and dropped after 2 denials). This is the exact failure class described in the `ToolDef.Capability` comment. Verify with a test.
6. **Resume vs overrides (fixed W2.2b).** Profile model/system settings carry their source slug and are reconstructed via `WithAgentProfile` on boot. Explicit `WithSystem`/`WithModel` replace both value and source, so even an explicit value identical to the profile remains non-resumable until tickets record overrides. `WithTools` (including an empty list) is also non-resumable. A change of identity cannot reuse another profile's provenance. Trust/cost/time ceilings are still captured and restored; the durable attempt increment precedes dispatch. Approvals are re-asked, not persisted.
7. **Policy audit gaps on side paths.**
   - ✅ **Fixed W2.3a:** `invokeWorkflowTool` uses shared `RunTool` for tool/HTTP/pipeline/canvas calls, with correlated policy/invoked/result audit on allow, deny, invocation error and reported error. Approval identity and per-call capability axes come from the resolved context/definition. Workflow **code** nodes joined through their runner adapter in W2.3c below.
   - ✅ **Fixed W2.3b:** Council `councilSearch` invokes `web_search` through shared `RunTool`, respecting explicit capability/profile/trust restrictions and approval before search or panel execution. One search grounds the panel; failure, refusal, malformed output or panic leaves only the date and never injects a failed brief. Disabled/missing tools remain no-ops. Search audit IDs are unique even under a reused run correlation.
   - ✅ **Fixed W2.3c:** Conductor verifier and workflow code nodes adapt their existing runners into the same invoker. Code.exec/profile/ceiling restrictions, correlated approval and mandatory preflight audit apply; denial reports `Ran=false` and a false live execution flag, failing verification without LLM fallback. Errors, reported failures and panics produce terminal audit. Canvas/retries/error ports use the same route; unique IDs prevent attempt collisions. Regression: `code_execution_audit_test.go`, including unavailable audit and unchanged input/output semantics.
   - ✅ **Fixed W2.3a:** direct denials emit failed terminal results, direct lookup includes active forge/MCP, policy sees resolved tool metadata, and audit failures cannot silently permit execution. The invoker catches tool panics and reports terminal audit-write errors; its call primitive moved to `platform/toolinvoke` in W2.3d.
   - ✅ **Fixed W2.3e:** actual agent-loop panic/cancel turns settle every admitted terminal result before task failure; per-call contexts are released and the shared platform primitive retains typed panic classification. Sequential panic prevents later effects, with `not_executed` results; parallel dispatch/order stay intact. Terminal hook/model work is suppressed. Audit errors retain terminal causes and later writes are attempted. Tool-log/stats expose skipped results without execution latency. Regressions: `tool_terminal_audit_test.go`, `tool_terminal_audit_internal_test.go`, `tool_audit_identity_test.go`; twelve mutations guard the boundaries. Admission/policy/memo and result formatting still belong to the loop pending app convergence.
8. **Performance hot spots.**
   - `epistemicGate` → `matchHistoricalToolOutcomes` does a full `journal.Range` on every gated tool call, even with `EpistemicEscalation` off (the signals are always computed and journaled).
   - `FoldRunTools`, `publishContextFailureAnalysis`, `subAgentSpendMicrocents` and `reflect.observe` also scan the whole journal.
9. **HITL is serialized per turn.** Approvals happen in the sequential gate phase, so a parallel fan-out waits for every approval before any call executes.
10. **The cost cap is post-call** (overshoot ≤ 1 call). Sub-agent spend is checked only at spawn time, from the journal.
11. **Sync sub-agents are not in `k.runs`.** `CancelRun(childCorr)` returns false for them; they die with the parent ctx. Async children are registered and cancellable, and are cancelled when the root run's cleanup runs.
12. **Steering is boundary-only.** A run wedged in a long tool call (or an approval wait) does not see pause or steer until that returns. Halt and cancel do interrupt via ctx.
13. **Model output vs journal.** The model always gets the full tool output. Only the journaled `tool.result` is slimmed (artifact `raw_ref`) and the journaled answer capped at 8192 runes. Untrusted outputs are wrapped for the model but journaled raw.
14. **`Close` does not close `configCenter`** (it is in Open's failure-unwind `closers`, but not in `closeAll`).
15. **Heuristic bypass** emits `task.received` without the wake/agent provenance fields that `agent.Run` adds, and skips policy, memory and lifecycle except `CompleteAgentLifecycle`.
16. **`governor.SetCatalog` is process-global**: two Kernels in one process (multi-tenant) share it.
17. **The agentgw listener goroutine** uses `context.Background()` and is not tied to `Close` cancellation; it relies on `agentGW.Close()`.
18. **Guard tests to know:**
    - The `toolreg` NetguardGaps test and the builtintools ratchet (`Names`).
    - `-race` lock-order tests (`drain_test`, `runtime_steer_test`).
    - Panic-firewall tests (`agent/panic_test.go`, `subagent_*`).
    - `prompt-injection*_test` (window decay, auto-approve not satisfying guards).
    - `resume_lifecycle_test` (Suspend before cancel keeps the ticket), `resume_profile_internal_test` (profile provenance vs explicit overrides), `cmd/agezt/resume_profile_test` (actual interrupted run → reopened stores → boot resumer, durable attempts, saved ceilings, quarantine rails), and `controlplane/run_resume_profile_test` (direct-agent defaults stay reconstructible).
