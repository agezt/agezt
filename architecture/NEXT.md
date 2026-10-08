# NEXT — handoff for the next coding agent

> **Owner update, 2026-10-04:** continue directly on the shared `main`, without
> new task branches. PR #612 consolidates the original W0–W2.1a stack plus W2.2a.
> W2.2a, W2.2b, W2.3 and File Manager operation binding are complete;
> Catalog/provider, memory and world native migrations plus exit evidence are complete. Taste native migration and exit evidence are complete. Skill native migration and exit evidence are complete. Board native migration and exit evidence are complete. Workboard, OKR and storage/artifact native migrations plus exit evidence are implemented. Schedule native migration and exit evidence are complete. Standing seven-command native migration and exit evidence are complete. Workflow13-command native migration and exit evidence are complete. Pulse fourteen-command native migration and exit evidence are complete locally; Autonomy feed native migration and exit evidence are complete locally; Three tool read operations and native exit evidence are complete locally; all eight toolforge native operations and exit evidence are complete locally. All three toolbox native operations and exit evidence are complete locally. All six MCP native operations and exit evidence are complete locally. All eight market native operations and exit evidence are complete locally. Plugin inventory typed native migration and exit evidence are complete locally. Config read typed native migration and exit evidence are complete locally. All five settings native operations and exit evidence are complete locally. Configcenter native exit is complete locally. Channel inventory/account and OAuth service/state/provider/exchange ownership foundations are implemented, with registered-field removal/status race repairs. Gateway status/QR service foundations are implemented. Inbox service foundation is implemented. Send service foundation is implemented. All eleven channel operations have typed shared native bindings and exit evidence. Webhook observability typed native migration and exit evidence are complete locally. Tunnel premise is measured: boot supervision exists, with no native operation to migrate. Update service foundation is implemented. Continue §4.5 with typed update binding and native exit, then order7 and the wider roadmap. Cadence resident run execution remains W2.2/W4 work. Journal raw-ref GC protection remains later module work. W2.10g-i through W2.27m are delivered to main via PR #701 (8416d9c8, all24 exact-head CI jobs passed); W2.27n is delivered via PR #702 (a00b580c, all24 exact-head CI jobs passed); W2.27o is delivered via PR #703 (5703f2d5, all24 exact-head CI jobs passed); W2.27p is delivered via PR #704 (ca25f5c7, all24 exact-head CI jobs passed); W2.27q is delivered via PR #705 (c32e8852, all24 exact-head CI jobs passed); W2.28 is delivered via PR #706 (1f5d5c36, all24 exact-head CI jobs passed); W2.29a awaits its own delivery. Broader adapters and remaining domains remain open.
> Read this handoff, verify the current state, then measure the next item's premise
> before changing code. The original handoff contained a stale claim about channels:
> they already used the vision sidecar; the API and channel rejection audit differed.

---

## 1. What this project is and what we are doing

AGEZT is a Go daemon (`cmd/agezt`) plus CLI (`cmd/agt`) plus a React console (`frontend/`, embedded via
`go:embed` in `kernel/webui/dist`). It runs governed AI agents: providers, tools, a policy engine
(Edict), a hash-chained journal, memory, channels, workflows and schedules.

The owner asked for a **clean-architecture redesign**, done as a **module-by-module strangler** on
`main`. There is no rewrite branch, and `main` stays shippable after every PR.

Authoritative documents (read in this order):

| File | What it is |
|---|---|
| `architecture/20-target-architecture.md` | The target: layers L0–L7, three pipelines (Operation `app.Dispatch`, Run `runs.Start`, Tool `tools.Invoke`), modules |
| `architecture/21-migration-roadmap.md` | Waves W0–W5, the PR-by-PR plan, **§1 working rules**, **§5 owner decisions**. Rows marked ✅ are done; each records what was measured and what changed |
| `architecture/00-README.md` | Codemap index; **§9 is the findings register** (open defects; ✅ = fixed) |
| `architecture/01…12-*.md` | Codemap of the current system, per area. Update the relevant one in the same PR that changes the structure it describes |
| `CHANGELOG/unreleased/current.md` | User-facing changelog. Security fixes go under `### Security` |

Layer map in code: `kernel/contract/*` (L1, stdlib-only contracts), `kernel/platform/*` (L2: `filestore`,
`netout`, `sandbox`), then modules, `kernel/app/tools` (L4 invocation entry), adapters, `plugins/` (L6), `cmd/` (L7).
`tools/archcheck/layers.json` places every package; the allowlists only shrink.

---

## 2. Where things stand (2026-10-03)

### 2.1 Consolidated delivery and original stack

**Delivery:** #612 contains the entire original stack and the W2.2a follow-up.
It was merged with history preserved at `415f6cea`; the superseded PRs were closed
and the shared checkout returned to `main`. W2.2b was implemented directly on `main`.
The only commits unique to earlier branch heads are identical cherry-picks of the
`TestMarkInstanceDead` repeatability fix; the top branch includes that fix too.
The table below is historical, not an instruction to recreate a stack.

| PR | Branch | Content |
|---|---|---|
| #595 | `arch/w0-archcheck` | codemap 00–12, target 20, roadmap 21, `tools/archcheck` (W0.1–W0.2) |
| #596 | `fix/w0-correctness` | W0.3–W0.5 security/correctness P0s, dead code |
| #597 | `arch/w1-contracts` | W1.1 contracts move to `kernel/contract/{llm,toolapi,channelapi}` |
| #598 | `arch/w1-repoint` | W1.2 importers use contracts (archcheck 196 → 145) |
| #599 | `arch/w1-store` | W1.3 `platform/filestore`, vault/settings merge-on-save under a cross-process lock |
| #600 | `arch/w1-netout` | W1.4 `platform/netout` (Egress / OperatorClient / MetadataClient) |
| #606 | `arch/w1-sandbox` | W1.5 `platform/sandbox`, no child inherits daemon secrets |
| #607 | `arch/w1-eventlog` | W1.6a event-kind registry closed (`TestKindRegistryIsClosed`) |
| #608 | `arch/w1-journal-recover` | W1.6c corrupt journal record quarantined, not fatal |
| #609 | `arch/w1-approvals` | W1.7 approvals re-ask after restart (pinned by test), not persisted |
| #610 | `arch/w1-config` | W1.8 configcenter: restart persistence, secrets in vault, lock fix, `config.access` journaled |
| #611 | `arch/w2-ingress` | W2.0 a direct `agt run --agent` is bound by the whole profile (`WithAgentProfile`) |
| #612 | `arch/w2-dispatch` | W2.1a dispatch journals every non-`ReadOnly` op (`op.invoked/completed/failed`) |

Every branch from `fix/w0-correctness` on also carries the same commit
`test(channel): TestMarkInstanceDead survives -count=N` (cherry-picked down the stack, see §5.3).

### 2.2 CI state when this was written

- All Go gates pass locally on `arch/w2-dispatch`.
- `secrets (gitleaks)` failed on #612 because of a synthetic key literal in a test. Fixed by rewriting the
  branch (the key is now joined at run time), and gitleaks is clean locally.
- `race-depth (linux, cgo)` failed on #596–#612 for two reasons:
  1. `TestMarkInstanceDead` was not repeatable under `-count=20`. **Fixed** on every branch.
  2. `kernel/controlplane` hit the 10-minute `go test` timeout under `-race -count=20`. On #595 it took
     487 s of 600 s, and locally the package takes the same 37–52 s with or without these PRs. The likely
     cause is runner contention: about 13 stacked PRs run race-depth at once on shared self-hosted runners.
     **Not a code bug as far as measured.** If it still fails, rerun that one job when the runners are quiet
     (`gh run rerun <run-id> --failed`). If it fails alone too, report it to the owner. Do not raise the
     timeout or cut the stress count without asking.

---

## 3. First thing to do in a new session

1. `git status --short`, `git fetch origin`, and `gh pr view 612` — confirm delivery
   and preserve any concurrent work. If delivery is pending, finish its checks and merge.
2. On the shared checkout, use `main` and fast-forward from `origin/main` when clean.
   **Do not create or switch to another task branch.** The owner's explicit instruction
   supersedes the original branch-per-PR recipe.
3. Read the authoritative target and roadmap, then start the first open item in §4.
4. Use isolated temporary `AGEZT_HOME` directories for runtime verification; never the
   owner's real state or password-protected `.dev-home` console.
5. Read `MEMORY.md` in the Claude memory directory if available
   (`~/.claude/projects/D--Codebox-PROJECTS-AGEZT/memory/`), especially
   `architecture-redesign-2026-10.md` for the standing invariants.

---

## 4. The next work items, in recommended order

The roadmap lists W2.1 (op framework) before W2.2 (runs). I recommend doing the **defect-driven** slices
first, because each one fixes something real and lands safely. The framework work (4.4) has no measured
defect behind it, and it gets easier once 4.1–4.3 have reduced the number of side paths. W2 items are
independent of each other, so this reordering is within the plan. Say so in the PR body.

**Rule for every item: measure the premise before building.** Twice the roadmap's claim was wrong:
approvals were not lost on restart, and the journal index was not urgent. Once the real defect was the
opposite of the claim (W2.0). Write the measurement into the roadmap row.

### 4.1 ✅ W2.2a — shared image admission (completed)

**Measured result:** REST/OpenAI rejected even with a vision sidecar; channels already captioned, contrary to the original claim below. Channels lacked correlated rejection audit and accepted empty captions. `Kernel.AdmitImages` now serves all three adapters, preserves channel artifacts/captions, consumes raw images after captioning, and returns the existing control-plane rejection message. Actual adapter tests failed on old code; four separate mutations prove sidecar use, raw-image removal, rejection correlation and model precedence.

The following is the original task context, retained for the measurement trail.

**Defect (measured in W2.0):** a run with images on a non-vision model behaves differently depending on
where it came from:
- **Control plane** (`kernel/controlplane/server_handle_run.go`, the console and `agt run`): asks a vision
  model to describe the images (`k.DescribeImages`, M821) and continues.
- **REST/OpenAI** (`cmd/agezt/api_engine.go:63`) and **every channel** (`cmd/agezt/main_channels_handler.go:33`):
  hard-reject via `visionGate` → `gateVisionWith` (`cmd/agezt/main_channels.go:18`).

So a photo sent over Telegram/WhatsApp to a daemon whose model lacks vision is refused, while the same
photo in the console works.

**Do:**
- Put one function in `kernel/runtime`, e.g. `(*Kernel).AdmitImages(ctx, corr, model, intent, images)`.
  It returns the new intent, the remaining images and an error, and it performs the confirm-or-caption-or-reject
  logic plus the `governor.capability` / `KindCapabilityRejected` event the control plane emits today.
- Call it from all three sites, and delete `visionGate` / `gateVisionWith` if nothing else uses them.
  Check `cmd/agezt/coverage_media_test.go`, which tests them.

**Tests:** a channel-style and a REST-style run with an image on a non-vision model, plus a configured
vision sidecar, must caption (red on old code). With no sidecar it must still reject with the same message.

**Note:** this is the first slice of the roadmap's `RunRequest`/`runs.Start`, so name it with that in mind.

### 4.2 ✅ W2.2b — agent-profile resume (completed)

**Measured result:** persistent tests interrupted soul-only, model-only and combined profile runs, reopened the stores and invoked the real boot resumer. All three were quarantined on the old code. Model/system values now carry their source profile slug in the run context; profile defaults remain reconstructible, while explicit setters replace the source. The control plane only calls those setters for explicit args. The ticket format and boot resumer are unchanged; older non-resumable tickets are still quarantined. Tests cover same-value explicit overrides, unnamed/retargeted profile values, denied tools, a tighter saved trust ceiling against a looser current profile, saved cost, attempts durable before provider dispatch, ticket cleanup and quarantine rails.

**Evidence:** `resume_profile_internal_test.go`, `controlplane/run_resume_profile_test.go`, `cmd/agezt/resume_profile_test.go`. Independent mutations verify model/system source, explicit-override rejection, control-plane provenance, attempt persistence, trust/cost restoration and the attempt cap. The following is the original task context, retained for the measurement trail.

**Claim** (findings register 9.2; `architecture/04-agent-runtime.md` gotcha 6):
- `buildResumeTicket` (`kernel/runtime/resume.go:96–127`) marks a ticket `Resumable=false` whenever
  `systemFromCtx` or `modelFromCtx` is set.
- `WithAgentProfile` sets both for any agent with a soul or model, so such runs are **quarantined at boot**
  instead of resumed, even though the resumer (`cmd/agezt/main_standing_resume.go`) re-applies
  `WithAgentProfile` and would rebuild them exactly.

**Measure:** write the test first. Start a run AS an agent with a soul, kill it mid-run (the resume tests in
`kernel/runtime` show how), reopen, and assert that it resumes. Expect red.

**Fix direction (target §3.2 step 5):** the ticket stores the *agent slug* plus the *explicit per-run
overrides*, not the resolved context. A system/model that came from the profile is reconstructible and
must not block resume. Only a per-run `--system`/`--model`/`--tools` that the ticket does not record
should block it, or record those too.

**Careful:** the resume path is crash-loop-guarded (attempt counter fsynced before dispatch). Don't weaken
that. Owner decision 5.5′: approvals are *not* persisted, and a resumed run re-asks.

### 4.3 ✅ W2.3 — tool side paths (exit verified)

**W2.3a measured and fixed:** actual registered-tool, HTTP, pipeline and canvas-node calls lacked policy/invoked/result events; direct denials lacked a terminal result. The shared `RunTool`/`toolexec` invoker now serves those paths, keeps forge/MCP lookup, supplies trusted ToolDef metadata (including parameter-dependent capability axes), stamps approval/run identity, catches tool panics and reports audit failures. Pre-invocation audit failure prevents execution. Persistent regressions were red on old code; thirteen independent mutations guard the boundaries.

**W2.3b measured and fixed:** actual Council grounding ignored `web.search=L0`, agent tool deny and trust ceiling, called the search and injected its evidence with no policy/tool audit. It now uses the shared invoker, preserving one search per panel, the 300-rune query/6-hit bounds, unique search IDs and date-only fallback on refusal/failure. Correlated live approval blocks both search and panel calls until resolution. Disabled/missing/nil search tools remain no-ops. Tests: `council_grounding_audit_test.go` (allow, three restriction paths, errors, malformed output, panic, disable/missing, HITL grant/deny and repeated IDs) plus the original Council suite. Seven mutations independently guard the boundaries.

**W2.3c measured and fixed:** the actual Conductor executed worker code and passed verification even with `code.exec=L0`, a profile deny or trust ceiling, with no policy/tool audit. Workflow code nodes enforced policy but left no tool execution audit. Both now adapt their existing sandbox runners into invocation-local `code_exec` tools and use the same governed invoker, with trusted capability/effect metadata and distinct attempt IDs. Denial/preflight audit failure never enters the runner; Conductor reports `Ran=false` in its result and a false live execution flag, failing verification without critique fallback. Backend errors, error results and panics fail with terminal audit. Interpolated inputs, structured output, workflow error ports/retries and Conductor critique-only paths remain covered. Tests: `code_execution_audit_test.go` plus the original Conductor/workflow suites; ten independent mutations guard the boundaries. Sandbox execution/network settings are unchanged.

**W2.3d foundation (move only):** the direct invoker's panic-contained tool call now lives in `platform/toolinvoke.Invoke`. The `toolexec` forwarding helper preserves the same result/error/context and panic error text; policy, admission and audit remain in their callers. This is the move-before-rewrite step for agent convergence, not the full app invoker. Three mutations independently reject missing recovery, lost errors and replaced context.

**W2.3e measured and fixed:** eight actual agent-loop scenarios (panic/cancel, sequential/parallel, fault first/last) lost terminal `tool.result` records; panic also skipped per-call context cancellation. Agent execution now uses the shared platform primitive, releases the call context and settles the whole admitted batch before task failure. A sequential panic still prevents later effects; those calls get failed results marked `not_executed`. Parallel dispatch stays bounded and results retain original order. Terminal batches do not invoke bookkeeping/automation hooks or make another model call; cancellation is rechecked after audit and before return. An audit-write failure is joined with the typed panic/cancellation cause and remaining terminal writes are attempted. Tool-log/stats honor the skipped marker and omit invented execution latency. Actual entry-point and source suites plus twelve independent mutations cover these boundaries; timeout feedback, memo/taint/offload and default allow remain.

**W2.3f foundation (move only):** agent output representation moved to `platform/tooloutput.Offload`. `agent.ArtifactPutter` and `DefaultArtifactThreshold` remain compatibility aliases; its forwarding helper retains the 8 KiB default, byte threshold boundary, preview, full artifact bytes and best-effort inline fallback. Three mutations reject a wrong boundary, lost fallback and storing only a preview. Original agent/source suites remain green.

**W2.3g measured and fixed:** actual direct, workflow, canvas and code paths journaled 20 KB inline without `raw_ref`/`output_bytes`. Runtime now supplies its artifact store and effective threshold to `toolexec.RunWithOptions`, using the same audit-only representation as the loop. Legacy `Run` retains its signature and inline/no-store behavior. Success, reported error, invocation error, panic and denial use preview/ref/byte count; caller and noise hook receive full output and typed error causes remain intact. Nil/unavailable stores, empty refs and inline boundaries retain best-effort fallback. Four actual-path regressions were red; seven independent mutations guard runtime wiring/threshold, metadata, caller/hook bytes and error retention.

**W2.3h foundation (move only):** `PolicyVerdict` and `Policy` moved to the pure `contract/policyapi` package with exact agent aliases. The loop's existing 23-field policy renderer moved to `platform/toolaudit.PolicyDecisionPayload`; a forwarding helper preserves its journal bytes, nil/zero values and event ownership. Full-value and actual-loop regressions plus four mutations cover field retention, call identity, provenance and forwarding. Same-verdict allow/deny tests measured that direct Run/RunWithOptions still omit 15 fields; the behavior repair follows separately.

**W2.3i measured and fixed:** same-verdict allow/deny tests drove actual agent.Run and Run/RunWithOptions: direct records omitted 15 of the loop's 23 fields. Actual direct/workflow/canvas/code allow/deny journal regressions reproduced the loss. Direct invocations now use the shared policy renderer, preserving resource, epistemic and observation/provenance metadata, call/correlation identity and explicit nil/zero values. Eight mutations guard field retention, identity, verdict, event kind and preflight audit error. Decision/admission behavior, default allow, approval and terminal output remain unchanged.

**W2.3j foundation (move only):** measured direct invoker dependence on `agent.ValidateToolInput` and `agent.WithPolicyToolDef`. The unchanged dependency-free schema validator now lives in `platform/schema`; resolved ToolDef/observation context helpers live in `platform/policyctx`, with the taint type in pure `contract/policyapi`. Agent forwarding functions/type alias retain schema errors, nil/empty behavior, parent context and cross-boundary accessor interoperability. Direct importers remain unchanged for this extraction; repointing follows separately. Exact contract/source suites and seven mutations cover JSON admission, nested arrays, registration lint, metadata/taint, empty-taint identity and parent context.

**W2.3k repointing (behavior preserved):** direct Run/RunWithOptions now use `platform/schema` validation and `platform/policyctx` resolved metadata. Production `go list -deps ./kernel/toolexec` no longer reaches `kernel/agent`. Actual invoker tests pass on both old and new source: unknown/invalid/schema-rejected calls reach no policy/backend/events/hooks; allow/deny paths preserve resolved metadata, parent agent/provenance, correlation/call/input and result/audit semantics. Five mutations and 20-repeat entry-point tests guard those boundaries. The loop still owns its admission sequence and remains on the compatible shared helpers.

**W2.3l invocation-port foundation:** `contract/toolapi` owns Invocation/Invoker and lookup/artifact ports. The legacy pipeline implements that port through `toolexec.NewInvoker`; `runtime.Open` constructs one service per kernel via Config.NewToolInvoker (nil factory retains standalone compatibility). RunTool and invocation-local code adapters call the injected port with lookup, effective artifact settings and original identity. Constructors bind each kernel's own policy/audit/hook ports; a nil result fails startup and unwinds stores before the gateway listener starts. Actual direct/workflow/canvas/code offload and two-kernel policy/journal tests cover the boundary; seven mutations guard injection, local lookup, artifact options, identity, host isolation and nil startup. Pipeline bodies/admission/approval remain unchanged. App/daemon factory binding follows separately.

**W2.3m app/daemon binding:** `app/tools.NewInvoker` binds the existing shared pipeline behind the L1 port. Primary and tenant daemon construction use the same `openAppKernel` helper, creating fresh services against each kernel's own dependencies; no global registration or L3 app import. Standalone runtime.Open/legacy Run remain compatible. Actual four-path offload/policy/provenance, two-kernel isolation, command-constructor and live approval identity tests run through app binding. Six mutations guard daemon binding, context, identity, local lookup, effective threshold and per-kernel instances. The pipeline implementation remains in legacy toolexec and the agent loop still owns its separate admission; this app ingress is not full W2.3 completion.

**W2.3n generic-engine move:** the sole schema/policy/audit/execution implementation moved unchanged from legacy toolexec into `platform/toolpipeline`. Host policy remains injected; the engine has no runtime/agent/app import. Public toolexec types remain exact aliases and Run/RunWithOptions retain production-reachable forwarding through the same output decorator. App construction now uses the moved engine, while dependency type consumers remain on compatibility aliases until repointing. Execution/recovery/output/port-adapter bodies were compared to old source; new primitive and original source contracts plus eight mutations guard schema/audit/denial, typed errors, caller bytes, artifact options, context and local lookup.

**W2.3o dependency repointing:** app construction and runtime host/factory declarations now consume `platform/toolpipeline` ports directly. Production app dependency closure excludes legacy toolexec, runtime and agent. Runtime retains one explicit legacy NewInvoker fallback for standalone compatibility; all public legacy Run/RunWithOptions stay reachable through that adapter and execute the same engine. No policy/audit/result/batch behavior changes. Existing primitive, app/compatibility and actual runtime entry-point suites protect the repointing; the engine's eight mutation guards remain.

**W2.3p policy-phase foundation:** `toolpipeline.Decide` separates trusted ToolDef context, policy callback and mandatory decision audit from execution/memo. Direct Run now uses it and retains the returned context for backend/hook metadata. Primitive contracts and six mutations guard resolved metadata, policy/audit ordering, error/call retention and execution context. Actual agent tests on old source fixed the existing contract: whole-batch admission precedes effects in sequential/parallel allow/deny cases, and a memo hit never bypasses a later denial (count=20). Agent production gating is unchanged in this extraction; shared-phase routing follows separately.

**W2.3q shared agent policy admission:** the now-unused agent.WithPolicyToolDef setter shim is removed (use platform/policyctx.WithPolicyToolDef), without a deadcode exception.  agent gating now uses toolpipeline.Decide for resolved metadata, policy callback and mandatory policy audit. The loop scopes directive taint before the phase and retains explicit no-policy default allow, caller event/error envelope, deny counters and policy-before-memo. It consumes only the verdict, preserving its existing execution context. Actual sequential/parallel allow/deny batch ordering, later-denied memo, whole-batch policy-audit failure and causal-window/provenance tests pass count=20. Seven mutations guard memo/effect ordering, audit error, window, metadata, default allow and call envelope. Execution/timeout/terminal settlement and availability/schema/loop guard/memo scheduling remain unchanged.

**W2.3r shared resolution (move only):** direct and agent admission now use `toolpipeline.Resolve` for caller-owned lookup, one trusted definition read and unchanged schema validation. The phase performs no policy/audit/tool execution; callers retain their unavailable/schema error envelopes. Old-source gate and actual mixed-batch tests passed count=20, alongside direct preflight/schema and batch/memo contracts. Registry aliases retain full metadata; missing/invalid calls consume no loop-guard quota and never reach policy/effects. Eight mutations guard lookup identity, availability-first ordering, schema error, metadata, single definition, direct rejection and loop quota/metadata. The enabled loop set and dynamic direct registry remain caller-owned; no shim or allowlist expansion.

**W2.3s shared invocation announcement (move only):** `toolpipeline.Announce` publishes the same tool.invoked kind and three-field call payload through a mandatory caller publisher, returning its error unchanged. Direct and loop callers retain their journal identity/error envelopes and call it only after policy/memo admission. It accepts no executor or context, so announcing a loop call does not begin its effects. Old-source gate/direct contracts plus actual batch/memo-coalescing/later-denial tests passed count=20. Nine mutations guard event kind, name/ID/input, audit cause, single publish, memo ordering, whole-batch audit stop and direct effects after audit. No execution/timeout/settlement behavior change or new shim.

**W2.3t shared execution (move only):** `toolpipeline.Execute` now owns the admitted safe invocation plus the loop’s unchanged optional positive per-call timeout, deadline capture and cleanup. It returns the original result/error, raw panic value and timeout fact. The loop supplies its ErrPanic mapper before context cleanup (including context-sensitive panic formatting); direct calls supply zero timeout and no mapper, retaining caller budget and panic text. Correlation/metadata remain assembled by callers, and batch scheduling/terminal settlement/hooks are unchanged. Old-source loop/direct and actual panic/cancel/timeout contracts pass count=20. Twelve mutations guard context, positive-only budget, result/error/panic facts, deadline-vs-error, cleanup/mapping order, mapper-on-panic and loop/direct wiring. The private direct forwarding helper is removed; shared toolinvoke.Invoke stays production-reachable.

**W2.3u shared terminal publication (move only):** `toolpipeline.Settle` owns the terminal kind and authoritative tool/call/output/error fields, copies caller metadata and returns the mandatory publisher error unchanged. Direct deny/error/panic/success and loop finalization use it. The loop passes an audit-only preview copy, retaining full model/hook bytes, observation provenance/deltas, typed nil-versus-empty matches, artifact/memo/skipped fields and result ordering. Classification, remaining batch write attempts, error joins and terminal hook suppression stay caller-owned. Old-source representation/cause/cancel-hook and actual terminal/delta/memo contracts pass count=20; twelve mutations cover kind/identity/output/error, metadata ownership, cause, one publication, remaining writes, full hooks and direct denial audit.

**W2.3v phase-port foundation:** the pure contract/toolphaseapi package owns the Phases port and unchanged resolution/decision/execution data, with exact platform aliases. It is separate from toolapi because llm already imports toolapi. The constructed platform/app invocation service implements all five shared phases; direct Invoke routes through its own phase port. RunWithOptions can receive an explicit phase service, including through legacy forwarding, while nil retains the original legacy path. Publisher kind strings bridge the pure contract to the registered event kinds without a journal dependency. Trace tests cover every phase, early schema/deny/audit stops, typed causes and per-service/local lookup; eleven mutations guard app exposure, injected selection, each phase, registry ownership, legacy forwarding and kind bridging. Agent/runtime loop routing is unchanged in this foundation.

**W2.3w runtime/loop binding:** an actual app-bound Kernel.RunWith executed its tool while the injected execution phase saw zero calls on old source. Kernel.Open now retains the constructed service’s phase port and BuildLoopConfig supplies it to both root and delegated loops. All five loop phases use that port, retaining enabled lookup, taint/default-policy, guard/memo, batch scheduling, timeout/terminal/error/hook and caller envelopes. Standalone agent.Run gets a per-run canonical default; legacy runtime invokers expose the phase port. Older custom one-shot-only factories retain their previous loop behavior through one host-bound canonical compatibility phase service per kernel. Actual primary/tenant deny, all-phase counters, delegated child correlation and one-shot compatibility pass count=20; ten mutations guard binding, every phase, service retention and compatibility.

**W2.3 exit verified:** side-path and dynamic forge/MCP fixtures now explicitly inject the app constructor and pass count=20, alongside actual root/delegated all-phase traces. Journal identity/stat fixtures pass separately. See [exit evidence](22-w23-exit-evidence.md) for the executable coverage, source boundary and legacy/custom-factory limits. Caller-specific batch/memo/observation/model behavior remains outside the shared mechanism. **Next open slice: §4.4.**

The lower-layer invocation port and per-kernel constructor injection are in place. The app service binds the shared L2 mechanism; L3 runtime/loop must not import app or grow the allowlist. Preserve standalone `runtime.Open`/legacy Run compatibility and the loop's batch admission/memo behavior; the shared preflight helpers no longer require an agent implementation import.

Retry attempts get distinct audit IDs. Tool log/stats join by run plus call ID, so a denied call cannot borrow another run's input/latency. Both guarantees were red before their fixes. Evidence: `kernel/runtime/workflow_tool_audit_test.go`, `kernel/toolexec/toolrun_test.go`, and `kernel/controlplane/tool_audit_identity_test.go`. The original finding list follows; all four original side-path findings are fixed (W2.3a–c).

The findings register (9.1, "Side paths skip governance/audit") lists:
1. ✅ workflow registered-tool/HTTP/pipeline/canvas calls now journal policy/tool events; code nodes joined in W2.3c;
2. ✅ Council grounding calls the shared governed invoker and records policy/tool audit (W2.3b);
3. ✅ Conductor code verification now checks `code.exec` and journals policy/tool events through the shared invoker (W2.3c);
4. ✅ `toolexec` now emits `tool.result` on deny and resolves active forge/MCP tools.

The exit evidence now records actual app-bound tests for these paths and their journal assertions. The original implementation requirement follows: The fix target is one invoker (target §3.3, `app/tools.Invoke`): lookup → policy
decision (Edict, trust ceiling, agent tool policy) → journal `tool.call`/`tool.result` → execute. Do it
in slices, one side path per PR or one invoker PR plus re-pointing PRs.

**Constraints:**
- Default-allow posture: every capability is LevelAllow by default and restriction is opt-out. Don't add new denials.
- Tool capability must be mapped: an unmapped tool name means an unknown capability, which Edict default-denies.

### 4.4 ✅ W2.x — governed File Manager and file snapshot restore

**W2.xa foundation:** an actual authenticated mkdir request returned HTTP 200 and
created the directory, but its durable journal contained zero op events and the
control-plane caller saw zero calls. A separate authenticated file.snapshot
rollback request restored the previous bytes with zero caller requests and zero
op/policy records in its temporary durable journal. Both defects are confirmed. The existing root,
path normalization and resolved containment bodies now live unchanged in
`platform/fileworkspace`; The WebUI resolver method forwards during migration; the unused root wrapper is removed.
Existing HTTP/source tests and target contracts preserve creation paths, exact
errors, home expansion and root containment. Four independent mutations guard
root creation, NUL rejection, missing-tail handling and resolved containment.
The real source junction/symlink fixture remains. This is a move before the
operation rewrite; mkdir/rename/delete and file snapshot restore are still local
and unjournaled. Next: bind their mutations to primary-only control-plane ops
and the canonical policy/audit invoker; remove the temporary resolver forwarding method
when the adapters consume the shared root/resolver directly.

**W2.xb mutation primitive move:** the console's mkdir/rename/delete filesystem
branches now live in `platform/fileworkspace`. HTTP handlers still own decoding,
path resolution, identical status/text mapping and response fields; final symlink
refusal, missing-file OS identity and recursive opt-in are retained. Source and
target tests pass count=20; four mutations guard parents, rename direction,
recursive opt-in and OS error identity. This separate move leaves governance
unchanged. Next bind these primitives through primary-only audited control-plane
operations, then migrate file snapshot restore independently.

**W2.xc File Manager binding complete:** mkdir/rename/delete are now primary-only
control-plane mutations. Dispatch assigns the operation correlation to the
handler; app/files admits an invocation-local adapter through the existing
per-kernel invoker. Mkdir/rename use file.write; delete uses file.delete. Root
creation and path resolution occur only after policy and mandatory audit. Actual
HTTP/control-plane tests inspect the five-event allow arc and four-event deny
arc under one correlation/call identity, verify disk effects, and prove denial
or unavailable audit does not bootstrap a missing root. Source HTTP path/status/
JSON contracts are retained through optional domain error_code transport; the
unused resolver shim is removed. Eight independent mutations guard the port,
capability, correlation, error propagation, policy, audit/tenant metadata and
local lookup. Default-allow remains. **Next: file.snapshot rollback restore.**

**W2.xd rollback-store foundation:** checkpoint/catalog data and the existing
catalog/restore/conversion bodies move mechanically to platform/rollbackstore.
WebUI keeps type aliases and production forwarders until operation binding;
legacy AGEZT_HOME lookup, JSON fields, version defaults, atomic writes, restore
bytes/absence and validation errors are unchanged. Source and target suites pass
count=20; body parity and four mutations guard restore absence, bytes, directory
refusal and zero catalog version. This foundation does not add governance.
Next: a primary-only file snapshot restore op, using the existing invocation
port and trusted catalog data, with no snapshot content in audit payloads.

**W2.xe file snapshot restore binding complete:** the primary-only file_restore
op resolves the checkpoint from the daemon's injected catalog path and passes a
private snapshot adapter through the existing per-kernel invoker. Content restore
uses file.write; absent-state restore uses file.delete. Only checkpoint identity,
path and existence enter audit input; snapshot bytes stay private. The server
marks AppliedMS only after successful governed restore and owns the catalog
write inside the audited operation. Already-applied checkpoints remain no-ops.
Actual HTTP/socket tests assert allow/deny event arcs, correlation/call IDs,
unchanged file/catalog on denial or unavailable audit, repeat behavior and no
snapshot data in any journal payload. A separate fixture proves caller-supplied
Before data and a different AGEZT_HOME cannot redirect the daemon's restoration.
Eight mutations guard the port, privacy, capability, applied order, repeats,
correlation, audit metadata and catalog authority. The file restore forwarding
shim is removed; remaining catalog/type compatibility serves the existing
skill/workflow/config UI until its domain migrates. File mutation findings are
closed. **Next: §4.5 operation framework.** WebUI catalog discovery still uses its
legacy home lookup; file restore mutation authority is daemon-owned.

The following is the original finding context, retained as the measurement trail.
Findings register 9.1: the web console's File Manager and rollback-restore wrote the filesystem directly
from the web UI layer: `kernel/webui/files_route.go` and `kernel/webui/rollback.go`
(+ `rollback_helpers.go`). Confirm with a test that inspects the journal. Now that W2.1a exists, the fix is
to make them control-plane ops: then dispatch journals them automatically (op audit), and they can take
a policy check. The web UI then calls the op instead of touching the disk.

### 4.5 W2.1b — the op framework proper (transport independence)

**W2.1b foundation:** source signatures confirmed status/version results were
coupled to net.Conn. Their status/fallback/version presentation now lives in
app/system with context + explicit Input -> Output/error handlers. The existing
map outputs and host reads are unchanged; the control-plane wrappers keep socket
encoding, auth/tenant/read-only metadata and fresh daemon extras. HTTPBinding and
ChannelInfo aliases retain callers. Actual socket-free handler tests and original
status/version/auth/tenant/op-audit suites pass count=20. Status/fallback body
parity and four mutations retain empty-head clamp, fallback dimensions, optional
tenant binding and build provenance. This is the move before registry/dispatch
rewriting; metadata derivation, typed schema framework and adapter routing are
still open. Do not call the full W2.1 framework complete yet.

**W2.1c typed dispatch pilot:** contract/opapi owns pure operation metadata,
principal/caller and auth/router/audit/emitter ports. app binds Go-typed unary and
emitting handlers, derives supported input/output schemas and owns immutable
registry dispatch: authenticate, authorize, route, decode/validate, mandatory
mutation audit, handler, terminal audit. Audit failure/cancellation prevents
handler entry; panic and typed terminal causes settle through owned spans.
System status/version now register from app specs; control-plane metadata is
derived and old socket handler wrappers are deleted. The adapter keeps native
auth framing, binds fresh service data and encodes output. Explicit unknown-input
compatibility retains the pilot's old args behavior; legacy output maps remain.
Source and pipeline/schema/stream/metadata contracts pass count=20; eight
mutations guard admission/effects/schema ownership and adapter metadata.
Mock-host, no-audit-I/O dispatch benchmark: 3.6–4.5 us/op on local Windows,
GOMAXPROCS=4 (not end-to-end status/journal-fold latency).
At W2.1c, schema derivation rejected nullable pointers, embedding and custom
JSON representations instead of advertising an incorrect contract. W2.1d adds
nullable support; the full framework is still incomplete: output maps remain legacy. Mutating/streaming transport host
adapters and other domains still need migration. Extend these boundaries against
actual types before closing W2.1; generated surfaces remain later work.

**W2.1d nullable/schema refinement:** actual typed registration rejected nullable
pointer fields. FromType now derives null unions for pointer roots/fields, nil
slices/byte slices and maps, retaining required-vs-optional fields and validation
of non-null nested values. Map additionalProperties carries its element schema;
recursive unsupported shapes still fail registration. Actual app.Dispatch tests
retain root/field nulls and reject missing required fields or typed map mismatches
before handlers. Source/target suites, count=20 and four mutations guard pointer,
slice/byte null and typed map contracts. The existing validator already supported
type unions; no tool-validation semantics were rewritten. Embedding/custom
representation, strongly typed outputs and mutation/streaming host adapters
remain open next; full W2.1 is still not complete.

**W2.1e explicit/custom wire schemas:** a time.Time registration with a supplied
schema was red because binding discarded the declaration. Explicit input/output
schemas are now linted and copied at registration; reflection still records the
actual Go types, and absent declarations keep derived schemas. Terminal results
and stream emissions are serialized/validated against the owned output schema
before leaving app dispatch. Custom decoder input restrictions, immutable schema
bytes, invalid-schema registration and bad terminal/stream values have actual
contracts (count=20), with five independent mutation guards. Output validation
raises the mock-host/no-I/O benchmark to 7.0–8.6 us/op, still below 50 us; it does
not measure live I/O. Embedding derivation, typed pilot output models and
mutation/streaming transport hosts remain open; full W2.1 is not complete.

**W2.1f typed pilot outputs:** status/version now return Go output models, including
nested schedule/fallback/delegation metadata. Status derives an actual field
schema; version uses an explicit schema for the brand.Binary custom wire key.
The control-plane adapter serializes typed outputs into its legacy Result map.
Optional metadata omission and an enabled empty tenant registry (`tenants: 0`)
remain compatible; returned metadata slices are copied. Original status JSON
parity, typed wire/schema and source tests pass count=20; five independent
mutations guard zero-tenant presence, snapshot ownership, nested wire names,
output schema specificity and the version brand key. Full Go/build/vet/static
and architecture gates pass without allowlist growth. Embedded derivation and
production mutation/streaming host adapters remain open; full W2.1 is incomplete.

**W2.1g embedded wire fields:** registration tests were red for named/flattened
anonymous fields and silently lost exported fields of private embedded values.
Schema derivation now resolves the encoding/json wire field set before deriving
child types: shallower fields win, tagged fields win at equal depth, unresolved
conflicts disappear, tagged embeddings remain nested and skipped fields stay
absent. Promoted fields beneath nil-capable anonymous pointers are optional.
Private value embeddings retain exported fields; private pointer allocation,
recursive embeddings and custom/quoted representations still require explicit
schemas. Actual MarshalJSON parity, app dispatch admission/terminal output and
source suites pass count=20; six independent mutations guard promotion, depth,
tag priority, conflicts, nil-parent omission and private value promotion. Full
Go/build/vet/static and architecture gates pass with unchanged allowlists.
Mock-host/no-I/O dispatch is 6.4–8.5 us/op (<50 us). Production mutating/streaming
hosts and remaining domains stay open; full W2.1 is not complete.

**W2.1h independent stream contracts:** source signatures confirmed the streaming
builder required the emitted frame and terminal result to share one Go type,
while actual market handlers emit progress events and return a distinct record.
The builder now binds Input/Output/Emission types and independent owned schemas;
frames validate against EmissionSchema and the return value against OutputSchema.
Same-type streams retain their former explicit output-schema fallback. Unary
operations expose no emission metadata and cannot emit even if a transport port
is supplied. Actual dispatch contracts with distinct structs, derived/explicit
schemas, custom time.Time emissions, malformed frames/terminal values, transport
causes and schema-byte ownership pass count=20; six mutations guard independent
binding, frame schema selection, emission metadata/ownership/copies and unary
mode. Full Go/build/vet/static and architecture gates pass without allowlist
growth. No domain or production host has migrated in this slice: mutating audit
and socket streaming host adapters remain next; full W2.1 is incomplete.

**W2.1i control-plane host ports:** real socket fixtures were red because app
mutations had no auditor and streams had no emitter; rejected typed inputs also
entered legacy socket audit. The common adapter now binds mandatory journal
admission/settlement and native event publication. Migrated registrations carry
AppOwned so app dispatch alone owns their audit; legacy handlers retain their
existing path. Routing binds the primary/caller-tenant kernel plus shared actor
and correlation context. Registration checks object terminal schemas and native
kernel event frame types before effects. Actual socket mutation/progress, panic,
invalid input, preflight/terminal persistence failure, tenant-vs-primary journal,
secret argument redaction, transport causes and live-disconnect settlement pass
count=20. Thirteen independent mutations guard the host/ownership/routing/wire
boundaries. Full Go/build/vet/static and architecture gates pass with unchanged
allowlists. Shipped status/version remain read-only; mutation/stream fixtures
exercise the production registration/factory/adapter with test operations.
**Next: verify the complete W2.1 exit against actual supported representations,
then continue the ordered catalog/provider domain migration.** Generated surfaces
and broader adapters remain later work; the architecture goal is not complete.

**W2.1j text codec boundary and framework exit:** exit checks reproduced netip.Addr
being derived as an empty object although encoding/json writes a string. Go text
encoder/decoder method sets now require explicit schemas, including pointer and
nested/collection representations. Actual explicit decode/dispatch/encode tests
pass count=20; separate encoding-only and decoding-only mutations fail. Complete
framework, pilot, native host and source invariants pass count=20; full Go/build/
vet/static and architecture gates pass without allowlist growth. No-I/O dispatch
is 6.9–9.0 us/op (<50 us). [W2.1 exit evidence](23-w21-exit-evidence.md) maps each
framework requirement to source and executable proof and states the supported
schema, principal, native wire and live-verification boundaries. **W2.1 framework
plus status/version pilot is complete; next migrate catalog/provider in roadmap
order.** Other domains, generated surfaces, broader adapters and W3–W5 remain open.

**W2.4a catalog move foundation:** source signatures confirmed sync/list/discover
were coupled to net.Conn. Their existing fetch/persistence/full-reload/event and
wire projection now live in app/catalog behind context + Input -> Output/error
methods. The CP wrappers retain original arg decoding/error text, primary-only
metadata and legacy audit. Output maps, credential lookup, deterministic model
order, optional prices/reload errors and env defaults remain. Actual socket-free
sync/list/discover plus source/catalog/registry/tenant/audit suites pass count=20;
original list socket JSON parity passes count=20. Nine independent mutations
retain full sync/discovery reloads, both optional rebuild errors, scoped/duplicate
credential rules, model order, failed discovery kind and the price wire field.
Full Go/build/vet/static and architecture gates pass: 219 packages, unchanged
145 import/13 call allowlists. This is the move before operation binding; no
policy/audit rewrite or additional provider domain move. **Next: catalog specs,
common app adapter binding and deletion of these compatibility handlers.**

**W2.4b catalog operation binding:** app/catalog owns three typed specs and actual
input/output models, including nested providers/models and optional price values.
Registration derives CP metadata and the per-server dispatcher includes system
plus catalog ops. Old sync/list/discover socket handlers are deleted. Unknown
legacy args remain allowed; known fields validate before fetch or audit. Sync
and discovery use mandatory app-owned audit, list stays read-only, and domain
success/failure events share the host-owned operation correlation. Real socket
fixtures reproduced ignored primary-only tenant args being copied into audit
principal metadata; operator tenant is now set only for caller-tenant ops.
Existing tenant-routed tests retain operator-selected tenant behavior. Native
framing, JSON timestamp/list parity, full reload, optional rebuild errors,
credential projection, known-zero vs unknown price behavior and HTTP proxy
contracts pass count=20. Eight independent mutations retain registry inclusion,
mutation/read-only metadata, ignored args, typed schemas, price wire name, primary
audit scope and domain identity. Full Go/build/vet/static and architecture gates
pass without allowlist growth (219 packages; 145 imports/13 calls). **Catalog
migration is complete; next provider domain, move before binding.** Generated
routes/broader adapters and the overall architecture migration remain open.

**W2.4c provider catalog/keyring move:** connect/reload and four keyring business
handlers now live in app/providers with context + Input -> Output/error methods.
CP wrappers retain primitive arg validation/order, response error text, existing
registration/auth/audit and request-independent service construction. Existing
catalog IDs keep their model entries; new endpoints retain unknown model coverage
instead of seeding a UI hint. Scoped/global key target rules, fingerprint-only
listing, active mirror persistence and reload-on-active-change behavior remain.
Socket-free lifecycle/default/error tests and source/provider/registry/tenant/
audit suites pass count=20; six original socket handler results have JSON parity
count=20. Nine independent mutations retain existing-entry preservation, unknown
models, scoped/namespace validation, active reloads/persistence and rebuild error
contracts. Full Go/build/vet/static and architecture gates pass (220 packages;
unchanged 145 imports/13 calls). **Next: provider catalog/keyring operation binding,
then OAuth lifecycle and tenant-routed log/stats/rejections and primary-only probe.** This partial
provider foundation does not close the complete provider domain or rewrite policy.

**W2.4d provider catalog/keyring binding:** six typed specs/results join the common
app registry and per-server dispatcher. Per-operation inputs validate only fields
used by that operation, retaining ignored legacy args. Old CP catalog/provider/
keyring socket wrappers and their validation-helper copies are deleted. Five
mutations now require app-owned audit before catalog/vault/reload effects; key
listing stays read-only and returns only label/active/last4. Real socket lifecycle,
rejected-input/unavailable-audit effects, scoped privacy and source/auth/tenant/
registry/HTTP contracts pass count=20; six original handler JSON results retain
parity count=20. Eight independent mutations guard inclusion, mutation/read-only
metadata, ignored args, typed active/output fields, redaction and mandatory audit.
Full Go/build/vet/static and architecture gates pass: 220 packages, import debt
145 -> 144 (CP -> catalog removed and officially ratcheted), 13 call sites remain.
**Next: provider OAuth lifecycle, then tenant-routed log/stats/rejections and primary-only probe.**
The entire provider domain and overall architecture goal remain open.

**W2.4e provider OAuth state/business move:** login state, its mutex/listener,
token-manager initialization and start/status/import/logout business now belong
to app/providers.OAuth. CP owns one lazy instance per Server and preserves fresh
model-hook lookup, legacy input coercion, socket framing and auth/audit metadata.
The existing fixed-port callback/expiry/deferred-close/page flow moves with its
state; its HTTP adaptation is deliberately unchanged in this move-only slice.
Socket-free isolated import/status/logout, authoritative hook model surface,
state filtering, denial/concurrent status and HTML escape tests pass count=20,
alongside source OAuth/registry/tenant/audit contracts. Six independent mutations
retain import/token clearing, model hook, state filter, denial state and escaping.
Full Go/build/vet/static and architecture gates pass: CP -> chatgptauth edge is
removed and officially ratcheted (143 imports/13 calls; 220 packages). **Next:
OAuth specs/common operation binding; callback HTTP/lifecycle refinement follows
separately if required by adapter migration.** Tenant-routed observations and the
primary-only provider probe remain open; the whole provider domain is incomplete.

**W2.4f provider OAuth RPC binding:** four typed specs/results now register through
the common app dispatcher; old socket start/status/import/logout wrappers are
removed. The factory resolves the existing per-server OAuth state at handler
entry. Start/import/logout require mandatory app audit before state/listener/
vault/reload effects; status retains the existing read-only metadata and model
hook behavior. Known typed fields validate before admission while unknown legacy
args remain accepted. Actual socket import/status/logout, authoritative empty
models, token privacy, correlation/order and preflight effect barriers pass
count=20 alongside original OAuth/registry/tenant/audit/HTTP and typed schema
contracts. Seven independent mutations retain registry, mutation/read-only flags,
ignored args, rich output schema, null/empty models and mandatory audit. Full
Go/build/vet/static and architecture gates pass: 220 packages, 143 import/13 call
exceptions remain. **Next: tenant-routed provider log/stats/rejections and
primary-only provider probe.** Existing callback HTTP/expiry/lifecycle behavior
is unchanged; later callback adapter/lifecycle refinement and the overall
provider/architecture goal remain open.

**W2.4g shared journal projection move:** newest-first sorting, cursor filtering,
row cutoff, pagination and output stamping now live in platform/journalview.
CP retains limit admission, since_ms conversion, tenant selection and native
result/error framing; every existing log caller still shares the same engine.
Decoder callbacks see every event before cutoff so cross-event inputs outside
the window remain available to results inside it. The moved body matches the
original after reader/error-envelope boundary substitutions. New primitive and
original log/provider suites pass count=20; seven independent mutations reject
lost decoder state, time/sequence ordering, cursor filtering/order, boundary
cursor and journal error causes. Full Go/build/vet/static and architecture gates
pass: 221 packages, unchanged 143 import/13 call exceptions. **Next: move provider
log/stats/rejections business into app/providers, then bind tenant-routed typed
operations; primary-only probe and callback adapter refinement remain open.**

**W2.4h provider observation service move:** app/providers.Observations owns
log/stats/rejections journal folds and wire projections. The host supplies its
selected journal; CP retains tenant selection, legacy limit/window/boolean
admission and socket result/error framing. Log uses the shared platform engine;
rejections retains its existing non-cursor wire shape and stats excludes model-
chain hops from provider fallback rates. Three original/current native handler
outputs match across eight argument sets count=20. Socket-free observation and
source provider/tenant/registry/audit suites, including actual socket primary/own-tenant
data isolation and cross-tenant refusal, pass count=20; nine independent
mutations retain window, provider/model distinction, filters, model identities,
sequence order, chain wire and journal causes. Full Go/build/vet/static and
architecture gates pass: unchanged 221 packages, 143 import/13 call exceptions.
**Next: typed observation specs/results and common tenant-routed binding; then
primary-only probe.** Callback adaptation and the overall provider migration
remain open.

**W2.4i provider observation operation binding:** log/stats/rejections now have
three typed specs/results in the common registry. The operation factory reads
the routed app host's journal; OwnTenant/CallerTenant and read-only metadata
replace the old socket wrappers/registrations. Typed numeric/boolean admission
runs before service entry; unrelated legacy args remain accepted. Numeric limits
retain truncation, absent/null default 20 and clamp 1..1000; permissive opaque
cursors retain malformed/non-string first-page fallback. Relative window
conversion lives at the app boundary, and present-empty optional row fields
remain present through typed pointers. Native JSON parity across eight argument
sets, empty-field/schema/cursor/window contracts and actual socket tenant data
isolation pass count=20 with source registry/audit/HTTP contracts. Ten independent
mutations retain registry, tenant admission/routing, read-only flags, ignored
args, limits/window, field presence and rich row/map schemas. Full Go/build/vet/
static and architecture gates pass: unchanged 221 packages, 143 import/13 call
exceptions. Log retains its existing HTTP hint; stats/rejections remain native
operations without inventing HTTP routes. **Next: primary-only provider probe.**
Callback HTTP/lifecycle refinement and the overall provider migration remain open.

**W2.4j guarded GET move:** the unchanged bounded HTTP helper shared by provider
probe and WhatsApp gateway status/QR now lives in platform/netout.GatewayGET.
CP retains its forwarding helper and all native handlers/admission metadata.
The moved body matches original source after its function rename. Fresh client,
background ten-second timeout, local/LAN posture, redirect limit, header/status/
content-type forwarding, body bounds and best-effort partial reads retain their
existing behavior. Local fixture primitive and actual provider/WhatsApp socket
contracts pass count=20 alongside original netout posture/redirect/client tests;
eight independent mutations retain method, headers, bounds, response fields,
loopback posture, parse identity and native forwarding. Full Go/build/vet/static
and architecture gates pass: unchanged 221 packages and 143 import/13 call
exceptions. **Next: primary-only probe service, then typed operation binding.**
Caller-context/partial-read refinement, callback adaptation and full provider
migration remain open; this mechanical move changes no HTTP posture or effects.

**W2.4k provider probe service move:** app/providers.Probe owns the endpoint
models check behind an injected bounded GET port. Its default uses the unchanged
platform/netout helper; CP retains lenient string admission, native framing and
primary-only/read-only registration. URL/key trimming, models path, one-MiB bound,
reachability versus authorization, 2xx-only model counting and failure-result
shape retain their source behavior. Socket-free six-status fixtures and source
contracts pass count=20; original/current native JSON outputs match across six
statuses and six argument sets count=20. Eight independent mutations retain
missing-URL admission, normalization, path/bound, 401 reachability, model counting
and failure shape. Full Go/build/vet/static and architecture gates pass: unchanged
221 packages and 143 import/13 call exceptions. **Next: typed primary-only probe
spec/output and common operation binding.** Context/callback adaptation and the
overall provider migration remain open; no live provider or paid model was used.

**W2.4l provider probe operation binding:** a typed primary-only/read-only spec
and variant-preserving output now use the common dispatcher. Its HTTP hint
matches the existing POST /api/provider/probe route; the old native wrapper and
registration are deleted. Known URL/key types validate before transport entry;
unrelated legacy args remain accepted. Success preserves explicit false/zero
fields; endpoint failure keeps only ok/error. Actual socket admission, tenant
refusal before requests, read-only audit absence, optional output/schema and
source tenant/registry/audit/HTTP contracts pass count=20. Original/current native
JSON parity spans six statuses and four compatible argument sets count=20;
eight independent mutations retain registry, primary admission/routing,
read-only flags, ignored args, rich schema, field presence and route metadata.
Full Go/build/vet/static and architecture gates pass: unchanged 221 packages and
143 import/13 call exceptions. **Next: separate provider OAuth callback business
from browser HTTP adaptation.** Existing callback lifetime and probe background
transport context remain for subsequent refinement; the provider/architecture
goal is not closed by native RPC binding alone.

**W2.4m OAuth callback business extraction:** callback admission and effects now
live in the socket-free completeProviderLogin helper. The HTTP wrapper supplies
query fields, request context and the actual exchange port, renders the returned
result, and honors its delayed-close signal. Denial/invalid state, code/verifier,
parent context, thirty-second budget/release, failed/canceled exchange, terminal
state before reload, existing ignored reload errors and model hook order retain
source behavior. Socket-free and original OAuth/native contracts pass count=20;
old/new status/headers/HTML/login-state parity spans five denial/invalid requests
count=20. Nine independent mutations retain admission, exchange identity/context/
budget, terminal state, models, close signal and HTTP framing. Full Go/build/vet/
static and architecture gates pass: unchanged 221 packages and 143 import/13 call
exceptions. **Next: extract callback HTTP query/presentation adaptation.** Existing
fixed-port startup, five-minute expiry, delayed close and logout behavior remain
for separate lifecycle work; no live token exchange or paid provider was used.

**W2.4n callback HTTP presentation move:** platform/browsercallback owns query/
request-context projection, the unchanged success/failure HTML renderer/escaping,
and post-render close dispatch. App's callback bridge invokes its business helper
and returns the result; the now-unreachable private page wrapper is removed and
its original source test targets the actual renderer. Render/escape bodies match
original source after renaming. Primitive query/context/close-order/HTML and
source OAuth/native contracts pass count=20; original/current HTTP status/headers/
HTML/login-state parity across five denial/invalid inputs passes count=20. Seven
independent mutations retain query/context, framing, escaping and close decisions.
Full Go/build/vet/static and architecture gates pass: 222 packages, unchanged 143
import/13 call exceptions; the official structure writer records 113 kernel
packages. **Next: callback listener/lifetime ownership, then caller context.**
Fixed-port startup, TTL and delayed-close/logout behavior remain unchanged;
no live exchange, paid provider or browser session was used for this move.

**W2.4o callback listener move:** platform/browsercallback.Prepare owns TCP bind,
callback path, mux and HTTP server/header timeout. App stores the prepared listener,
publishes the login under its mutex, then launches its Serve method; app's bound
completion bridge is socket-free and production app no longer imports net/http
or net. Existing fixed address, status publication, expiry/delayed-close and
server-close semantics remain. Prepared listener path/bind/options/Serve/Close
and source OAuth/native contracts pass count=20; old/new callback HTTP page/state
parity remains count=20. Six independent mutations retain TCP/caller address,
path, header budget, explicit Serve and server close. Full Go/build/vet/static and
architecture gates pass: unchanged 222 packages and 143 import/13 call exceptions.
**Next: prove/fix listener lifetime ownership, including Close before Serve.**
TTL/logout/stale-session refinement and caller context remain open; this move
alone does not claim those lifecycle guarantees or live OAuth exchange.

**W2.4p prepared listener ownership repair:** an owned loopback proof reproduced
that Close before Serve left the bound port unavailable for immediate rebinding.
Listener.Close now closes both HTTP server and prepared socket, normalizes an
already-closed socket, and joins remaining cleanup causes. Permanent regressions
cover Close-before-Serve, repeated close, concurrent Serve/Close, immediate port
reuse and owned close-error identity. Focused/source tests and the full adapter
race suite pass count=20; three independent mutations retain release, idempotent
normalization and error propagation. Full Go/build/vet/static and architecture
gates pass: unchanged 222 packages and 143 import/13 call exceptions. **Next:
provider session/expiry/logout ownership, then caller-context refinement.**
This scoped repair does not claim token/session fencing or TTL/logout semantics;
its sockets are controlled fixtures, with no live provider exchange.

**W2.4q successful logout callback ownership repair:** a controlled prepared
listener proof reproduced successful token logout retaining pending login ownership
and its callback port. Logout now stops the login after successful vault cleanup,
before reload. A failed token cleanup preserves the pending login and propagates
its existing error. Permanent logout/port-reuse and vault-failure regressions,
source OAuth/native tests and callback race tests pass count=20; two independent
mutations retain stop-on-success and failure ordering. Full Go/build/vet/static
and architecture gates pass: unchanged 222 packages and 143 import/13 call
exceptions. **Next: expiry/session ownership and caller-context refinement.**
This fix does not claim cancellation of the five-minute expiry worker or fencing
of a token exchange already in progress; those remain explicit lifecycle work.

**W2.4r expiry worker ownership repair:** an isolated actual Start/Stop goroutine
proof reproduced a retired login retaining its five-minute sleep worker. Login
now owns stop/done channels and idempotent cancellation; stop and delayed callback
close release the timer worker. Normal TTL keeps the existing timeout state;
expiry checks current login identity and cancellation before changing state.
Permanent Start/Stop wiring, stop/callback-close, normal TTL and retired timer
regressions plus source/native and callback race tests pass count=20. Four
independent mutations retain cancellation, current identity, timeout state and
Start wiring. Full Go/build/vet/static and architecture gates pass: unchanged
222 packages and 143 import/13 call exceptions. **Next: in-flight token/session
fencing, then caller-context refinement.** This repair does not claim ownership
of a token exchange already running or cancellation of its persistence effects.

**W2.4s token exchange fetch/persist foundation:** Manager.ExchangeTokens owns
unchanged authorization-code/PKCE HTTP fetching and returns the raw token candidate
without manager/vault effects. Public ExchangeCode forwards fetch then StoreTokens,
retaining legacy persistence/account derivation and error behavior. App's callback
uses the two explicit steps in the same order; no session policy change is claimed
in this foundation. Isolated endpoint form/candidate/no-write/error tests and
original manager/OAuth/native contracts pass count=20. Four independent mutations
retain legacy persistence, candidate fields, verifier form and fetch error cause.
Full Go/build/vet/static and architecture gates pass: unchanged 222 packages and
143 import/13 call exceptions. **Next: admit persistence against the current login
identity after fetch, then caller-context refinement.** In-flight ownership,
logout/replacement fencing and live provider validation remain open.

**W2.4t callback session persistence admission:** a controlled delayed fetch proof
reproduced logout completing before an old callback restored tokens and reported
success. App now admits persistence under the login mutex against current identity,
pending state, open expiry ownership and live context. Logout token cleanup and
retirement use the same mutex, preserving pending ownership on failure. Callback
business checks ownership before exchange and after its await before terminal
state/model effects. Production fetch is an owned constructor-bound port; no live
endpoint is needed for the race proof. Permanent delayed logout/replacement,
context/stopped/terminal/retired admission, current success and post-exchange
retirement tests plus source/native and callback race suites pass count=20.
Five independent mutations retain identity/state/context/stop and post-exchange
admission. Full Go/build/vet/static and architecture gates pass: unchanged 222
packages and 143 import/13 call exceptions. **Next: caller-context refinement and
provider exit evidence.** Effects admitted before retirement retain their normal
ordering; broader generated surfaces/adapters/domains and the architecture goal
remain open.

**W2.4u probe caller-context repair:** controlled typed-dispatch HTTP fixtures
reproduced cancellation leaving the probe on its background timeout, and canceled
response bodies reporting successful probes. Context-aware GatewayGETContext,
Probe.CheckContext and injected port now carry caller cancellation/deadline while
retaining the guarded posture/ten-second ceiling. Caller cancellation after body
read returns its cause; legacy GatewayGET/Check and injected context-free getters
retain their background/best-effort behavior. Typed probe uses the context-aware
entry. Primitive/source/native/context/legacy contracts and focused race tests
pass count=20; four independent mutations retain transport/operation/port context
and canceled-body result. Full Go/build/vet/static and architecture gates pass:
unchanged 222 packages and 143 import/13 call exceptions; no dead-code exception
was added for the live legacy entry points. **Next: provider exit evidence.**
Live provider/browser validation and broader generated transports/domains remain
outside these controlled-fixture and native-host checks.

**W2.4v catalog/provider native exit:** all 17 operations have typed input/output
schemas and common AppOwned native binding. Aggregate registry evidence guards
the complete set, read-only flags and primary/own-tenant routing; old business
handlers are absent. Related source/native/registry/tenant/audit/lifetime/context
suites pass count=20; full Go/build/vet/scoped static and architecture gates pass
with unchanged 222 packages, 143 imports/13 calls and dead-code/dependency ratchets.
No-I/O dispatch is 6.4–6.9 us/op (<50 us). [Exit evidence](24-w24-exit-evidence.md)
maps operation ownership, lifetime repairs and validation boundaries. **Next:
memory, world, taste and skill in roadmap order.** Broader adapters, generated
surfaces, full runs.Start convergence and W3–W5 remain open; controlled fixtures
do not certify live provider/browser behavior.

**W2.5a memory read-service foundation:** current native get/search/find-related
bodies depend on socket response helpers. Their store lookup/search, legacy limit
defaults/cap and seed exclusion now live in app/memory with context + typed input
-> typed output/error; CP retains required/type admission and native framing.
Typed record projection preserves optional expiry (including present zero),
provenance, tombstone/suspension and absent-versus-empty response fields. Existing
source tests caught omitted lifecycle fields during the move; these are retained
and guarded by explicit wire contracts. Three original/current native handlers ×
nine argument sets pass count=20; exact row/field/error parity allows only bounded
wall-clock ranking-score drift. Source/read/registry/tenant/audit suites and the
new package race tests pass count=20; eight independent mutations fail. Full
Go/build/vet/scoped static and all gates pass: 223 packages, unchanged 143
imports/13 calls, official kernel structure 114; no exception expansion.
**Next: remaining memory business (list/log/write/hygiene/distillation), then
typed operation binding and old wrapper deletion.** This foundation retains the
existing native registry/auth/audit behavior; it does not close memory migration.

**W2.5b memory curation-service foundation:** add/supersede/forget/promote/
bulk-forget store business and typed output now live in app/memory. RememberInput
owns field conversion, source-tag merge and explicit operator Actor/Force; caller
tags stay untouched. CP retains required/type/order admission, native framing and
the existing registry/audit behavior. Same-content supersede remains a no-op;
promotion preserves present empty subject, and bulk forget keeps repeated/missing
counts, the 500-ID bound and stop-on-error partial effects. Five original/current
native handlers × ten argument sets match exact JSON count=20. Source/curation/
registry/tenant/audit and new-package race suites count=20 plus ten independent
mutations pass. Full Go/build/vet/scoped static and all gates pass: unchanged 223
packages, 143 imports/13 calls and official kernel structure 114. **Next: memory
list/log/hygiene/distillation, then typed binding and wrapper deletion.** No
correlation, policy or audit rewrite is included in this business-only move.

**W2.5c memory hygiene-service foundation:** prune/tidy/audit/clean business
now lives in app/memory with context + typed input/output/error. Native wrappers
retain nil/tenant selection, lenient day conversion, bool/string dry-run admission
and the existing registry/audit behavior. Prune retains its 30-day default,
pre-mutation stats, separate present-zero prunable/pruned fields and age predicate;
tidy and clean forward the admitted dry-run flag and preserve curated records.
Four original/current native handlers × eight argument sets match count=20 after
normalizing bounded cutoff-clock drift and existing unordered contradiction
groups/members. Source/hygiene/registry/tenant/audit and package race tests pass
count=20; nine independent mutations fail. Full Go/build/vet/scoped static and all
gates pass: unchanged 223 packages, 143 imports/13 calls and structure 114.
**Next: memory list/log/distillation, then typed binding and wrapper deletion.**
This move changes no policy, native registration, audit or correlation behavior.

**W2.5d memory distillation-service foundation:** consolidate/profile-rebuild
orchestration now lives in app/memory.Distillation behind a runtime-supplied
Distiller port. Methods accept context and typed input and return typed reports;
the unchanged orchestration still creates a fresh correlation and owns a bounded
five-minute background context, canceled on success/failure. Native report fields
retain present null arrays/zero counts and active_after naming. Two original/
current handlers × two argument sets × running/halted states match count=20 after
normalizing fresh correlation IDs. Permanent actual-socket no-op/halt and fake-port
report/order/budget/cleanup/cause contracts pass count=20; nine independent
mutations fail. Full Go/build/vet/scoped static and all gates pass: unchanged 223
packages, 143 imports/13 calls and structure 114. **Next: memory list/log move,
typed binding, then measured caller-context/operation-correlation refinement.**
This move preserves background orchestration and legacy audit behavior; it does
not yet promise caller cancellation or shared operation identity for distillation.

**W2.5e memory list-service foundation:** app/memory.PreparedList owns
newest-first record projection and cursor pagination. PrepareList reads active
records before native argument admission, preserving the source order; CP then
admits typed limit/cursor and encodes Page's typed result. Independent pages copy
the prepared slice. Legacy default/cap/fractional limits, timestamp/ID ties, strict
cursor filtering, pre-filter total, terminal cursor omission and empty arrays
retain behavior; the duplicate CP record projection is removed. Original/current
native list matches exact JSON across twelve argument sets count=20. Source/list/
registry/tenant/audit and package race tests pass count=20; eleven independent
mutations fail. Full Go/build/vet/scoped static and all gates pass: unchanged 223
packages, 143 imports/13 calls and structure 114. **Next: memory journal log,
typed binding and measured caller-context/operation-correlation refinement.**
Native registration/auth/audit remain unchanged during this business move.

**W2.5f memory journal-service foundation:** app/memory.LogService owns the
memory event fold, aliases and typed row shaping through platform/journalview;
the native adapter retains lenient limit/window/cursor admission and selects its
tenant journal before the service call. Write/revive aliases, forget/supersede/
promote identity/subject shaping, cutoff/cursor order, present empty/zero row
fields, empty ops and always-present next_cursor retain behavior. Original/current
native log matches exact JSON across thirteen argument sets count=20. Source/log/
registry/tenant/audit and package race suites pass count=20; eight independent
mutations fail. Full Go/build/vet/scoped static and all gates pass: unchanged 223
packages, 143 imports/13 calls and structure 114. The business foundation now
covers the 16 memory/profile native commands. **Next: typed operation binding,
mandatory audit/host metadata, old wrapper deletion and measured caller-context/
operation-correlation refinement.** Native registration/auth/audit remain unchanged
during these moves; the complete memory migration remains open.

**W2.5g memory typed native binding:** a restored legacy-binding proof confirmed
closed-journal requests still added/superseded/forgot/promoted/cleaned records and
reported success. All 16 memory/profile specs now share the common app/native
dispatcher; ten mutating operations require audit admission before service entry.
Six reads stay unaudited; audit/clean/log retain OwnTenant/CallerTenant and every
other operation stays primary-only. Known type/null/required admission precedes
effects; unknown unused args, bool/string dry-run, lenient day/log inputs and
bulk trim/blank filtering retain compatibility. Source wrappers/registrations
and two newly unreachable result helpers are removed without new exceptions.
Actual closed-journal and tenant socket fixtures, typed schema/metadata/effect
contracts, source/registry/tenant/audit and focused race suites pass count=20;
eleven independent mutations fail. Sixteen old/current bindings × three compatible
argument sets pass count=20 with documented cutoff/score clocks, fresh identity
and generic failure-code boundary. Full Go/build/vet/scoped static and all gates
pass: unchanged 223 packages, 143 imports/13 calls and structure 114.
**Next: measured caller-context/operation-correlation refinement, then memory
native exit evidence.** Distillation still owns its legacy background context and
fresh identity, and memory business writes retain their prior domain correlation;
the complete memory/architecture migration remains open.

**W2.5h memory operation/domain identity repair:** an actual typed native add
proof confirmed operation audit had a correlation while memory.written had none;
a controlled distillation port proved the admitted identity was replaced. All
eight store mutations now pass the context operation correlation to their manager.
Both distillation paths retain the admitted identity and mint a fresh one only
when the context has none. Context-free store calls retain their empty domain
correlation, and background distillation retains its legacy fresh identity/budget.
Permanent native invocation -> domain effect -> terminal ordering, eight real
store/bus/journal paths and both controlled distillation paths pass count=20;
ten independent mutations reject each lost identity path. Source/typed/native/
tenant/audit and focused race contracts count=20 plus full Go/build/vet/scoped
static and all gates pass: unchanged 223 packages, 143 imports/13 calls and
structure 114. **Next: measured distillation caller-context refinement, then
memory native exit evidence.** Background model context/lifetime remains for the
next slice; this repair adds no unrelated policy or model invocation behavior.

**W2.5i distillation caller-context repair:** controlled blocked-port proofs
confirmed consolidate/profile rebuild retained model waits after caller cancel,
lost caller deadlines/values and admitted already-canceled direct calls. Both
paths check cancellation before identity/port effects and derive their owned
five-minute ceiling from the caller context. Cancellation, earlier deadline and
model context values now reach the runtime port; deferred cleanup and admitted
operation identity remain. Background callers retain fresh fallback identity and
the five-minute ceiling. Direct/typed blocked cancellation, deadline/value/pre-
canceled, report/cleanup/identity and source/native/tenant/audit contracts plus
focused race suites pass count=20; six independent mutations reject service,
admission and typed binding context loss. Full Go/build/vet/scoped static and all
gates pass: unchanged 223 packages, 143 imports/13 calls and structure 114.
**Next: memory native exit evidence, then world/taste/skill in roadmap order.**
Controlled ports and isolated mock-provider sockets do not certify live model or
provider behavior; broader adapters, generated surfaces and W3–W5 remain open.

**W2.5j memory native exit:** complete common-registry coverage now guards all
16 memory/profile operations, actual schemas/types and native metadata. Both
independent family/operation removal mutations fail; restored coverage and
related source/typed/native/tenant/audit/identity/context suites pass count=20.
Full Go/build/vet/scoped static and all gates pass with unchanged 223 packages,
143 imports/13 calls and structure 114. No-I/O dispatch is 5.7–7.2 us/op (<50 us).
[Exit evidence](25-w25-exit-evidence.md) maps ownership, admission, closed-journal
repair, tenant isolation, domain identity and caller lifetime with explicit
parity/live-provider boundaries. **Next: world, then taste/skill in roadmap order.**
Memory native migration is complete; knowledge/runtime module dissolution,
broader adapters, generated surfaces, runs.Start and W3–W5 remain open.

**W2.6a world graph-service foundation:** add/edit/relate/resolve/neighbors/
list/get/forget business now lives in app/world with context + typed input/output/
error. Native wrappers retain required/type/collection/limit admission, registry,
auth/audit and legacy empty correlation. Entity projection moves with business;
the private duplicate CP view is removed. Content identity, open kind/verb
normalization, alias/attribute replacement, relation endpoint/direction behavior,
quiet resolve, absent/empty/zero and lifecycle/provenance fields retain semantics.
Resolve receives the already-admitted integer limit, preserving fractional input
that truncates to zero. Eight original/current native handlers × eight argument
sets match count=20 with bounded ranking clock drift. Source/graph/registry/tenant/
audit and new-package race tests count=20 plus twelve independent mutations pass.
Full Go/build/vet/scoped static and all gates pass: 224 packages, 142 imports/13
calls. The official writer removes paid CP->worldmodel debt only; official kernel
structure is 115 packages. **Next: world journal fold, then typed operation binding
and old wrapper deletion.** This move changes no policy/audit/correlation behavior;
the complete world migration remains open.

**W2.6b world journal-service foundation:** app/world.LogService owns world
entity/relation/forget event folding, labels and typed rows through journalview.
Native wrappers retain kind type admission, lenient limit/window/cursor behavior
and tenant-selected journal before service entry. Kind annotations, relation
direction labels, forgotten name/verb fallback, upsert default, filtering,
cutoff/cursor boundaries, empty arrays and present zero/empty fields retain source
behavior. Original/current native log across thirteen argument sets matches exact
JSON count=20. Source/log/graph/registry/tenant/audit and package race suites pass
count=20; ten independent mutations fail. Full Go/build/vet/scoped static and all
gates pass: unchanged 224 packages, 142 imports/13 calls and structure 115.
**Next: world typed operation binding, old wrapper deletion and measured domain
identity refinement, then native exit evidence.** Native registry/auth/audit remain
unchanged during this business move; complete world migration remains open.

**W2.6c world typed native binding:** restored legacy binding confirmed four
graph mutations changed state and returned success with a closed journal. All
nine world specs now use the common app/native host; four mutations require audit
before service effects and five reads remain unaudited. World log keeps routed
tenant scope; all graph commands stay primary-only. Known required/type/null
admission precedes effects; unused unknown fields, case-preserving trimmed aliases,
native resolve defaults/cap/fractional zero and lenient log inputs remain compatible.
Old socket wrappers/registrations and the now-unused argStringMap helper are removed
without new exceptions. Actual closed-journal and tenant socket, metadata/schema/
effect/source/registry/audit and focused race suites pass count=20; nine independent
mutations fail. Nine old/current bindings × three compatible inputs match count=20
with bounded score-clock drift and generic failure-code boundary. Final full
Go/build/vet/scoped static and all gates pass: unchanged 224 packages, 142 imports/
13 calls and structure 115. **Next: measured world operation/domain identity
refinement, then native exit evidence.** Graph business retains its prior empty
domain correlation; complete world migration and broader architecture remain open.

**W2.6d world operation/domain identity repair:** actual typed native add
proved operation audit had identity while the graph upsert event had none. Add,
edit, relate and forget now pass the admitted context correlation into the graph;
relation-created endpoint events share it too. Context-free calls retain legacy
empty domain correlation. Permanent native ordered invocation -> graph effect ->
terminal proof and all four real store/bus/journal paths pass count=20; four
independent mutations reject each missing identity path. Source/typed/native/
tenant/audit and focused race suites count=20 plus full Go/build/vet/scoped static
and all gates pass: unchanged 224 packages, 142 imports/13 calls and structure 115.
**Next: world native exit evidence, then taste/skill in roadmap order.** This
repair changes event joins without altering graph normalization, policy or model
behavior; broader domain/adapter/module migration remains open.

**W2.6e world native exit:** complete common-registry coverage guards all nine
world operations, actual schemas/types and native metadata. Independent family/
log removal mutations fail; restored source and related source/graph/log/typed/
native/tenant/audit/identity suites pass count=20. Focused race and final full
Go/build/vet/scoped static plus all gates pass: unchanged 224 packages, 142
imports/13 calls and structure 115. No-I/O dispatch is 10.5–11.7 us/op (<50 us).
[Exit evidence](26-w26-exit-evidence.md) records graph contracts, admission,
closed-journal repair, tenant scope and ordered domain/audit identity with explicit
clock/failure-code/live-provider boundaries. **Next: taste, then skill in roadmap
order.** World native migration is complete; knowledge/runtime module dissolution,
broader adapters, generated surfaces, runs.Start and W3–W5 remain open.

**W2.7a taste-service foundation:** list/create/delete store business now lives
in app/taste with context + typed input/output/error. Native wrappers retain
lenient trimmed string/limit/tag admission, registration/auth/audit and response
framing. Exemplar fields use the actual store type, preserving optional scope/tags,
timestamps and present-empty arrays. Source store filtering/case-sensitive tag
dedupe, creation validation, delete causes and persistence rollback retain behavior.
Three original/current native handlers match count=20 with explicit fresh identity
and bounded create-clock boundaries. Source/service/store/registry/tenant/audit and
package race suites count=20 plus eight independent mutations pass. Full Go/build/
vet/scoped static and all gates pass: 225 packages, 141 imports/13 calls. Official
ratchet removes only paid CP->taste debt; generated kernel structure is 116.
**Next: taste typed operation binding and old wrapper deletion, then exit evidence
and skill.** This move changes no native policy/audit or runtime exemplar selection;
complete taste migration remains open.

**W2.7b taste typed native binding:** a closed-journal fixture confirmed legacy
create/delete returned success and changed exemplars despite unavailable audit.
Three typed app/taste operations now derive native metadata through the common
host. Both mutations require audit before effects; list remains unaudited.
Old wrappers/registrations are removed. Primary-only policy/store selection,
StreamNone, no invented HTTP hints, lenient strings/default-200 limits (including
fractional zero), mixed/CSV tags and unused fields retain native compatibility.
Output schemas derive actual exemplar fields. Three original/current handlers
match count=20 with fresh identity/bounded create-clock boundaries. Source/service/
store/registry/tenant/audit and package race pass count=20; eight independent
mutations fail. Actual closed-journal/tenant sockets and single-owned-audit-arc
regressions pass. Full Go/build/vet/scoped static and all gates pass: unchanged
225 packages, 141 imports/13 calls, structure 116. **Next: taste native exit
 evidence, then skill.** Runtime exemplar selection and broader migration remain.

**W2.7c taste native exit:** exact common-registry coverage guards all three
operations, actual schemas/types, AppOwned binding and primary/read-only metadata.
Independent family/delete removal mutations fail; related source/native/tenant/
audit/exit suites pass count=20. Package race and final full Go/build/vet/scoped
static plus all gates pass: unchanged 225 packages, 141 imports/13 calls and
structure 116. Three benchmark measurements satisfy the <50 us dispatch budget.
[Exit evidence](27-w27-exit-evidence.md) records lenient admission, typed exemplar
fields, closed-journal repair, one owned audit arc, primary store isolation and
fresh-identity/clock/live-provider boundaries. **Next: skill in roadmap order.**
Taste native migration is complete; broader adapters, generated surfaces,
knowledge/runtime dissolution, runs.Start and W3-W5 remain open.

**W2.8a skill read-service foundation:** skill list/get business now lives in
app/skill with context + typed input/output/error and a narrow actual Forge reader
port. Typed records retain the native projection, required empty agent/description
and six zero-valued metric fields, optional body/provenance/arrays, list order,
active count, present-empty arrays and missing found-only shape. Native wrappers
retain ID admission, registrations/auth/audit and response framing. Original/current
native list/get handlers match exactly count=20 across empty/missing/invalid inputs
and draft/shadow/active states. Source/service/store/registry/tenant/audit and package
race suites pass count=20; eight independent mutations fail. Full Go/build/vet/
scoped static and all gates pass: 226 packages, unchanged 141 imports/13 calls,
structure 117. No new exception or early debt removal. **Next: remaining skill
history/lifecycle/file services, then typed binding and exit evidence.** Runtime
skill execution/retrieval and broader migration remain separate work.

**W2.8b skill lifecycle-service foundation:** promote/quarantine/archive/revert/
restore now live in app/skill.Lifecycle with context + typed input/output/error.
Native wrappers retain required/optional string admission, registration/auth/audit
and response framing. Actual Forge transitions, archive idempotence, parent
restoration, restore target validation and cause propagation retain behavior.
Wire outputs preserve present-empty archive/restore reason and restored-parent ID.
Five original/current native handlers x three initial states x six argument sets
match exactly count=20. Source/service/store/native and package race suites pass
count=20; eight independent mutations fail. Final full Go/build/vet/scoped static
and all gates pass: unchanged 226 packages, 141 imports/13 calls, structure 117.
No new exception or early debt removal. **Next: skill ownership/import/history/
file services, then typed binding/domain identity and exit evidence.** This move
retains empty legacy domain correlation; common-host identity binding follows.

**W2.8c skill curation-service foundation:** share/reassign/import now live in
app/skill.Curation with context + typed input/output/error. A narrow actual Forge
port and caller-selected roster lookup retain ownership admission before effects.
Native wrappers keep required/optional/lenient string/array/resource admission,
registration/auth/audit and framing. New imports stay draft/content-addressed;
existing content retains its lifecycle and first owner. Portable bundle contents,
resource manifests, empty target/present name and original causes retain behavior.
Three original/current native handlers match exactly count=20 across admission,
ownership, Unicode resources and dedupe shapes. Source/service/store/native and
package race suites pass count=20; eight independent mutations fail. Final full
Go/build/vet/scoped static plus all gates pass: unchanged 226 packages, 141 imports/
13 calls, structure 117. No new exception or early debt removal. **Next: skill
history/files/hygiene, then typed binding/domain identity and exit evidence.**
Legacy domain correlation and native policy remain until common-host binding.

**W2.8d skill observation-service foundation:** history/files/read_file/hygiene
now live in app/skill.Observations with context + typed input/output/error. Native
wrappers retain ID/path/lenient idle-days admission, registration/auth/audit and
response framing; unused old history/projection helpers are removed. Typed history
rows retain chronology, admitted lifecycle kinds, ID/restored matching and required
empty correlation; nil events remain null. Files prefer nonnil successful disk
listing and retain manifest fallback, directory and Unicode byte count. Hygiene
retains 30-day default, cutoff, projection plus top-level usage and present-empty
idle arrays. Four original/current native handlers match exactly count=20. Source/
service/store/native and package race count=20, eight independent mutations and
final full Go/build/vet/scoped static plus all gates pass: unchanged 226 packages,
141 imports/13 calls and structure 117. This move explicitly retains best-effort
history Range error swallowing and its existing kind set; any behavior repair
requires its own proof. **Next: skill typed binding, measured history/domain
identity repairs and exit evidence.** No new exception or early debt removal.

**W2.8e skill typed native binding:** actual closed-journal fixtures confirmed
all eight legacy mutation commands returned success and changed skill state with
unavailable operation audit. Fourteen typed app specs now derive native metadata
through the common host. Mandatory audit precedes all eight mutations; six reads
remain unaudited. Old wrappers/registrations and resource admission helper are
removed. CLI compare rollback evidence points to the moved lifecycle/spec sources. Primary-only scope/store selection, StreamNone and actual nine HTTP hints
remain; native-only get/history/read_file/restore/reassign gain no invented hints.
Inputs retain byte-preserving IDs/reasons/path, lenient import text, strict nonnull
string arrays, nullable string resource objects, lenient numeric/decimal-string
idle-days and unused fields. Actual output schemas retain native wire projection.
Fourteen old/current bindings x three compatible inputs match count=20 with the
generic typed failure-code envelope explicitly normalized; fields/domain errors
remain exact. Closed-journal/effect/metadata/schema/tenant/source/registry/audit and
package race suites pass count=20; eight independent mutations fail. Final full
Go/build/vet/scoped static and all gates pass: unchanged 226 packages, 141 imports/
13 calls, structure 117. CP->skill debt remains owned by roster teardown sources;
no exception is removed early or expanded. **Next: measured history error/domain
identity repairs, then skill native exit.** History legacy behavior stays open.

**W2.8f skill history read-error repair:** owned corrupt JSONL journals proved
native history returned successful empty/partial results despite real Range decode
failure; a reader fixture also exposed the swallowed original cause. History now
returns the original Range error and discards failed partial output. Valid history
shape/order/kinds and malformed individual payload filtering remain. Permanent
native corrupt-journal empty/partial cases and exact-cause/zero-output service
cases pass count=20; three independent mutations fail. Source/native/tenant/audit
and package race count=20 plus final full Go/build/vet/scoped static and all gates
pass: unchanged 226 packages, 141 imports/13 calls and structure 117. **Next: skill
domain operation identity and measured ownership-history coverage, then exit
evidence.** Fixtures use owned TempDir journals; no owner journal is touched.

**W2.8g skill operation/domain identity and ownership-history repair:** actual
native fixtures proved eight mutations' domain events carried empty correlation
beside admitted audit IDs, and real shared/reassigned events were absent from
history. Lifecycle/curation methods now forward opapi caller identity; context-free
calls retain empty correlation. History admits both ownership kinds without
changing chronology, row projection, original Range failure or malformed-row
filtering. Eight direct caller paths and eight actual native fixtures verify
exactly one trusted ordered invocation/domain/completion arc; client-supplied
correlation input cannot choose the identity. Native ownership-history fixtures
retain event IDs/correlation/sequence and show both changes. Source/native/tenant/
audit/CLI rollback/compare and package race count=20, ten independent mutations
and final full Go/build/vet/scoped static plus all gates pass: unchanged 226
packages, 141 imports/13 calls and structure 117. CLI already renders ownership
kinds; the repaired fold exposes those rows. **Next: skill native exit evidence.**

**W2.8h skill native exit:** exact fourteen-command common-registry coverage
verifies actual schemas/types and native metadata. Independent family/import
removal mutations fail; related source/native/tenant/audit/history/identity/exit
and CLI suites pass count=20. Package race and final full Go/build/vet/scoped
static plus all gates pass: unchanged 226 packages, 141 imports/13 calls,
structure 117. Three dispatch benchmarks satisfy <50 us excluding audit I/O.
[Exit evidence](28-w28-exit-evidence.md) records projection/admission, lifecycle,
ownership/import/bundles, mandatory audit, read errors, tenant isolation and
trusted domain identity. CP->skill debt stays with roster teardown sources.
**Next: board, then workboard/OKR/storage/artifacts in roadmap order 3.** Broader
adapters, generated surfaces, runs.Start and W3-W5 remain open.

**W2.9a board-service foundation:** all seven board business methods now live
in app/board with context + typed input/output/error and the actual store port.
Native wrappers retain store selection/admission/registration/auth/audit/framing:
reads select the shared store or fresh fallback; writes require the shared instance.
Typed message projection retains required topic/text/ts_unix_ms, optional IDs/
addressing/help and owned acknowledgement slices. Read retains full-store cursor
filtering, descending timestamp/ID ties, pre-filter total, topic counts, optional
cursor and admitted zero as unbounded. Send retains reply/help/broadcast/DM/post
precedence, original reply topic/recipient, source failures and one success-only
notifier with the explicit inbound correlation; ack/inbox/replies/help retain
existing case/idempotence/order behavior. Seven old/current native handlers match
count=20 across argument/pagination/routing/unavailable-writer/fresh-reader inputs;
fresh sent IDs/bounded send clocks are documented comparison boundaries. Source/
service/store/native and package race count=20, ten independent mutations and
final full Go/build/vet/scoped static plus all gates pass: 227 packages, unchanged
141 imports/13 calls, structure 118. Old message projection helper is removed;
roster uses still own board limit constants/debt. **Next: board typed binding and
measured audit/notifier behavior, then exit/workboard.** No native policy change.

**W2.9b board typed native binding:** closed-journal fixtures proved send/ack
changed the shared store and returned success; send also notified despite unavailable
operation audit. Seven typed app specs now derive native metadata through the common
host: two mutations require audit before factory/store/notifier effects, five reads
remain unaudited. Host factories retain selected shared/fresh-reader and shared-only
writer behavior plus unavailable-store causes. Lenient strings/limits, fractional
zero as unbounded, strict bool/read strings, cursor/projection/routing and explicit
inbound notifier/result correlation remain. Old native wrappers/registration and
unused limit admission helper are removed; roster-owned constants/debt remain.
Seven old/current native bindings match count=20 with fresh IDs/bounded send clocks
and generic typed failure-code boundary; fields/domain errors remain checked.
Actual closed-journal/fallback/shared-writer/notifier/tenant/schema/metadata/effect
and source/store/package-race count=20, eight mutations plus final full Go/build/
vet/scoped static and all gates pass: unchanged 227 packages, 141 imports/13 calls,
structure 118. Four existing HTTP hints remain; inbox/get/replies stay native-only.
**Next: board native exit evidence, then workboard in roadmap order.** Explicit
inbound correlation is a retained bridge contract; no notifier policy rewrite.

**W2.9c board native exit:** exact seven-command common-registry coverage
verifies actual schemas/types/native metadata. Independent family/ack removal
mutations fail; related source/native/tenant/audit/fallback/notifier/exit suites
pass count=20. Package race and final full Go/build/vet/scoped static plus all
gates pass: unchanged 227 packages, 141 imports/13 calls and structure 118.
Three dispatch benchmarks satisfy <50 us excluding real audit/journal I/O.
[Exit evidence](29-w29-exit-evidence.md) records paging/projection/routing,
shared-writer/fresh-reader ownership, mandatory audit and explicit inbound notifier
correlation as a retained bridge contract. Roster-owned constants/debt remain.
**Next: workboard, then OKR/storage/artifacts in roadmap order 3.** Broader
adapters, generated surfaces, runs.Start and W3-W5 remain open.

**W2.10a workboard read/projection foundation:** list/lanes/show business now
lives in app/workboard with context + typed input/output/error and the actual
read-only store port. Task Record embeds the actual store model plus computed
comment/link/attempt/failed counts, conditional criterion/proof fields and retry
limits; pointer fields preserve present zero/false values. Native task projection
wrapper forwards this view for remaining lifecycle/dispatch outputs. List retains
all selected filters/order; lanes group trimmed assignees with actual per-status
counts, case-insensitive lane sorting and unassigned-last label. Show retains
missing-ID/task errors; empty list/lane arrays remain present. Native wrappers keep
status/lenient string/limit/bool admission, registration/auth/audit/framing.
Three old/current native handlers match count=20 and shared task projection JSON
matches exactly (Go map int/float representations are compared at the wire).
Source/service/store/native parity/CLI and package race count=20, eight independent
mutations plus full Go/build/vet/scoped static and all gates pass: 228 packages,
unchanged 141 imports/13 calls, structure 119. **Next: remaining workboard lifecycle/
relations/dispatch/watch services, then typed binding/audit/exit and OKR.** Existing
native policy and lifecycle/dispatch behavior remain; no early debt removal.

**W2.10b workboard lifecycle-service foundation:** create plus claim/heartbeat/
comment/block/fail/unblock/complete/prove/seat/archive now live in app/workboard.
Lifecycle uses the actual narrow kernel facade plus seat setter; typed inputs/
outputs preserve task projection, creation idempotence and retry-decision fields.
Unused legacy retry projection helper is removed. Native wrappers retain lenient
argument/status/seat admission, explicit/generated
correlation, unknown-task envelope and 90-second prove timeout. Prove receives the
original caller context; seat retains its store clock. Actual kernel transition/
retry/idempotence/correlation fixtures and all eleven facade input/error paths pass.
Eleven old/current native handlers x three inputs match count=20 with generated
root/nested IDs and bounded lifecycle clocks normalized. Source/service/store/CLI
and package race count=20, eight independent mutations plus full Go/build/vet/
scoped static and all gates pass: unchanged 228 packages, 141 imports/13 calls,
structure 119. **Next: relation/maintenance and dispatch/watch services, then
workboard typed binding/audit/exit and OKR.** Kernel transition/journaling mechanisms,
native policy and helper error classification retain their existing behavior.

**W2.10c workboard relation/maintenance foundation:** link/policy/depend/reclaim/
sweep now live in app/workboard.Relations with context + typed input/output/error
over the actual kernel facade. Native wrappers retain explicit/generated correlation,
lenient arguments, clear/max-attempt validation, depends_on/on alias, stale duration
default, sweep actor default/1000 cap and original unknown-task error envelope.
Typed outputs retain task projection, actual reclamation counts, echoed stale
milliseconds and present-empty sweep arrays. Five facade input/cause paths and
real kernel link/policy/dependency-cycle/reclaim/sweep fixtures pass count=20.
Five original/current native handlers x three inputs match count=20 with generated
root/nested IDs and bounded lifecycle clocks normalized. Source/service/store/CLI
and package race count=20, eight mutations plus full Go/build/vet/scoped static and
all gates pass: unchanged 228 packages, 141 imports/13 calls, structure 119.
**Next: workboard dispatch/watch services, then typed binding/audit/exit and OKR.**
Underlying kernel transition/journaling and native policy retain existing behavior.

**W2.10d workboard watch-service foundation:** snapshot/run selection/event fold
and dependency projection now live in app/workboard.Watch with context + typed
input/output/error and actual store/journal reader ports. Native wrappers retain
lenient ID/run/limit admission, 50 default/200 cap, registration/auth/read-only and
single snapshot framing. Typed rows preserve seq/time/kind/subject/empty correlation,
payload absent/null/empty-object distinctions, stable chronological newest-tail
limits, subject-or-run filtering and nil empty events. Run selection retains
explicit -> claim -> latest attempt/link timestamp and tie precedence. Dependency
projection retains required ID/status and positive optional timestamps. Existing
Range and dependency errors remain best-effort for this move; a behavior repair
requires its own proof. Original/current native watch matches exactly count=20;
source/service/store/CLI/package-race count=20 and eight independent mutations plus
full Go/build/vet/scoped static/all gates pass: unchanged 228 packages, 141 imports/
13 calls, structure 119. Old run/fold/dependency view helpers are removed.
**Next: dispatch admission/background execution services, then workboard typed
binding/audit/error repairs/exit and OKR.** Despite historical comments, watch is
currently a unary snapshot; this move introduces no new stream.

**W2.10e workboard dispatch-admission foundation:** admission, dependency/agent
checks, correlation/claim/run link, requested publication and background launch
now live in typed app/workboard.Dispatch over actual store/host ports and a
selected roster projection/callback. The app layer has no runtime/roster import.
Native lenient string admission, error envelope and fresh dispatch correlation
remain. Agent/assignee and reason trimming, retired/paused/managed hints, exact
claim -> link -> publish -> launch order, generated/explicit intent and mutation
causes retain the old contract. Original/current denied native admissions and
intent match exactly count=20 with authentic paused/retired roster states;
accepted launch uses an owned callback fixture. Eight independent mutations,
workboard service/store/CLI/focused native tests and workboard package-race
count=20, complete controlplane race count=1 and full gates pass: unchanged
228 packages/141 imports/13 calls, structure 119. The actual background runner
and execution-profile bridge remain in controlplane for the next extraction.
Unused task-projection/response shims are removed once their last caller moves.
**Next: background execution service, then typed workboard binding/audit,
measured read-error repairs/exit and OKR.**

**W2.10f workboard background-execution foundation:** seat selection/context
axes, degradation comments, run settlement, task retry reclaim/recursion and
proof/review ownership checks now live in typed app/workboard.Execution. Actual
store/seat/kernel ports, selected context bridge and execution/publication
callbacks keep runtime/roster out of the app package. Native bridge still binds
the complete agent profile, wake source/subject/reason, cost ceiling, model/tool
setters, agent retry policy and warden execution-profile policy/backend checks.
Seat-over-agent isolation and model/tool precedence, missing/unavailable-seat
fallbacks, same-correlation task retry, current-claim ownership, proof-error review
fallback, best-effort settlement and Unicode 240/300-byte summaries are retained.
Original/current actual store/mock-provider execution matches count=20 across
review, reader/missing/unavailable seat, failure, task retry, proof and fallback;
only generated record IDs/timestamps are normalized, model/tool requests and
remaining final task fields match. Eight independent execution mutations fail;
workboard/service/store/CLI/focused native/workboard race count=20, complete
controlplane race count=1 and full gates pass: unchanged 228 packages/141 imports/
13 calls, structure 119. Native projection/context bridge remains intentionally;
all-surface runs.Start convergence is still open.
**Next: twenty-one typed workboard native bindings/mandatory mutation audit,
separately proved watch read-error repair and native exit, then OKR.**

**W2.10g workboard typed native binding:** twenty-one typed operations now own
native registration/metadata, replacing the old socket wrappers and registration.
All seventeen mutations require app audit before store, seat, claim, publication
or background effects; four reads remain unaudited unary snapshots. Actual
closed-journal tests cover every mutation independently of production metadata,
and actual tenant requests deny every command without changing primary state.
Request DTOs retain lenient strings/numbers/lists, policy maximum presence/null
and kernel empty-policy normalization, strict booleans, seat validation, limits,
dependency alias and the prove 90-second timeout over the admitted context.
Default lifecycle/domain correlation now matches the host-owned audit operation;
explicit inbound correlation remains the existing native bridge contract.
Dispatch still generates its own fresh run correlation, returned with claim/link;
owned asynchronous execution/review is tested against the actual mock provider.
Twenty-one original/current handlers x three inputs match count=20, normalizing
only generated IDs, bounded clocks and the common generic failure-code boundary.
Native audit/tenant/shape/admission/default and explicit identity/positive dispatch,
service/store/CLI suites pass count=20, workboard race count=20, complete
controlplane race count=1, eight audit-bypass mutations and full Go/build/vet/
scoped static/architecture/dead/deps/doc/changelog/structure/format gates pass.
Unchanged 228 packages/141 imports/13 calls, generated kernel structure 119.
Shared legacy OKR correlation/integer and seat list helpers remain for their
unmigrated callers; CLI comparison evidence points to actual app sources.
Watch Range/dependency errors intentionally remain best-effort until their
separately proved repair. **Next: watch read-error repair, then native exit/OKR.**
**Delivery checkpoint:** W2.10g is local. The current session cannot write Git
metadata and GitHub authentication is unavailable; #701 background-execution CI
was last observed pending before this restriction, so its merge is unverified.

**W2.10h workboard watch read-error repair:** an owned actual JSONL journal with
an invalid complete line returned success with zero/partial events; a failing
dependency port also returned a successful partial snapshot. Both defects were
reproduced before editing. Watch now returns the original Range/dependency cause
and zero output; the native app binding emits one terminal error frame with no
result/event/provider effect. Actual empty/partial journal regressions and the
exact dependency sentinel fail against the original service, pass three verifier
runs count=20, and independently fail when either error gate is suppressed.
Successful snapshot run/filter/order/payload/limit/dependency shapes remain intact;
existing successful-view tests no longer inject an intentionally ignored cause.
Focused native/service/store/CLI suites and workboard race pass count=20, complete
controlplane race count=1 and full Go/build/vet/scoped static/all gates pass.
Unchanged 228 packages/141 imports/13 calls, generated kernel structure 119.
This is an explicit behavior repair after the service move and typed binding.
**Next: native exit evidence, then OKR. Delivery remains local alongside W2.10g.**

**W2.10i workboard native exit:** exact twenty-one-operation aggregate common
registry coverage now guards actual types/schemas, AppOwned metadata, primary/
read-only/unary scope and unknown-input compatibility. Removing the family or
dispatch entry independently fails the exit regression. [Exit evidence](30-w210-exit-evidence.md)
records native compatibility, all seventeen mutation audit gates, four reads,
tenant denial, default/explicit/dispatch identity boundaries, owned execution and
separately proved watch failures. Sixty mutation proofs cover the native wave.
Focused source/service/store/native/audit/identity/tenant/watch/exit/CLI count=20,
workboard race count=20, complete controlplane race count=1 and final full gates
pass. No-audit-I/O framework benchmark: 5932 / 5636 / 6374 ns/op (<50 us).
Unchanged 228 packages/141 imports/13 calls, structure 119; shared OKR/seat helpers
and the selected native runtime context bridge remain for actual callers.
**Next: OKR, then storage/artifacts. Broader migration remains open.**
Native workboard completion is local, with G/H/I code/docs patches preserved as
separate reviewable deliveries; protected main delivery remains unverified.

**W2.11a OKR read/projection foundation:** list/show and live objective progress
projection now live in typed app/okr.Service over actual store reader and kernel
Rollup ports. Durable objective JSON, required progress/percent/achieved/key-result
count, filters, list order/count/empty arrays and show errors retain their native
wire contract. Cached objective status is preserved independently from live task
rollup; actual linked-task completion/unlink fixtures verify changing progress.
Native wrappers keep lenient status/tenant/limit and strict boolean admission,
primary-only registration/read-only policy, existing error envelopes and framing.
Five old writer wrappers use the same typed projection through the remaining
view bridge. All seven original/current native responses x three inputs match
count=20 with generated ID/bounded-clock normalization. Eight independent filter/
projection/empty-array mutations fail; actual service/store/runtime/CLI/native
focused and package-race suites pass count=20, complete controlplane race count=1
and full Go/build/vet/scoped static/all gates pass. Official structure writer
adds the package: 229 placed packages, unchanged 141 imports/13 calls, 120 generated
kernel packages. **Next: five lifecycle services, then OKR typed binding/audit/exit.**
Local code/docs delta is separate from the frozen workboard G/H/I deliveries.

**W2.11b OKR lifecycle foundation:** create/key-result/link/unlink/archive now
live in typed app/okr.Lifecycle over the actual kernel mutation facade and app
projection service. Native wrappers retain required title/ID admission, lenient
strings/integer target, explicit/generated correlation, auth/registration and
unknown-objective error envelopes. Runtime/store transitions, domain journaling,
live workboard rollup and original error causes remain. Link returns the actual
mutation result before cached achievement recomputation; the app projects that
returned snapshot rather than refetching a different durable status. Actual kernel
fixtures verify all six domain events, link/unlink/archiving and this distinction.
Seven original/current native responses x three inputs match count=20; eight
independent input/method/cause/projection mutations fail. Related source/service/
store/runtime/CLI/native and package-race suites pass count=20, complete
controlplane race count=1 and full Go/build/vet/scoped static/all gates pass.
Unchanged 229 packages/141 imports/13 calls, generated kernel structure 120.
Unused objective view/response bridges are removed after their final callers move.
**Next: seven typed native OKR bindings/mandatory mutation audit, then exit.**
Local source/docs delta remains separate from frozen read/projection and workboard
deliveries; Git write/GitHub authentication restrictions still prevent delivery.

**W2.11c OKR typed native binding:** seven app specs replace native wrappers and
registration. Five mutations require app audit before objective/key-result/link/
archive or rollup effects; two reads remain unaudited unary snapshots. Actual
closed-journal tests enumerate every mutation independently of production flags;
actual tenant socket clients reject all seven commands without primary changes.
Request DTOs retain lenient trimmed/nonstring fields and integer target/default
limit admission, unknown status filters and strict booleans. Default domain events
join one host-owned audit span; explicit inbound correlation remains the native
bridge contract. Domain result/error and returned-snapshot semantics retain parity.
Seven original/current handlers x three inputs match count=20 with generated IDs,
bounded clocks, generic failure-code and strict boolean schema-message boundaries.
Eight independent audit/target/identity/error mutations fail. Source/service/store/
runtime/CLI/native/audit/tenant/admission/identity and package-race count=20,
complete controlplane race count=1 and full Go/build/vet/scoped static/all gates
pass. 229 packages/140 imports/13 calls, official structure 120.
The official archcheck updater removes the paid controlplane-to-okr import
allowance (141 -> 140); call debt remains 13. Old OKR wrappers/registration
plus last-caller correlation/integer helpers are
removed; seat's shared string-list helper remains. **Next: OKR native exit.**
C source/docs delta remains a local delivery after frozen A/B and workboard slices.

**W2.11d OKR native exit:** exact seven-operation aggregate common registry now
guards input/output types/schemas, AppOwned metadata, primary/read-only/unary
scope and legacy unknown-input compatibility. Family/archive removal mutations
independently fail the exit regression. [Exit evidence](31-w211-exit-evidence.md)
records live-versus-cached rollup, facade-returned snapshot, five audit gates,
two reads, tenant denial and default/explicit identity/admission boundaries.
Twenty-six independent mutation proofs cover the native wave. Focused source/
service/store/runtime/native/audit/tenant/identity/admission/exit/CLI and package
race count=20, complete controlplane race count=1 and final full gates pass.
No-audit-I/O benchmark: 9757 / 9743 / 12903 ns/op (<50 us); 229 placed packages,
140 import/13 call exceptions and 120 generated kernel packages. Dedicated app
package documentation is generated correctly by the official structure writer.
**Next: storage/artifacts in roadmap order 3; broader migration stays open.**
Native OKR completion is local, with A/B/C/D source/docs preserved separately;
protected main delivery remains blocked by Git write/GitHub access restrictions.

**W2.12a storage inventory foundation:** typed app/storage owns home directory
aggregation, root-file grouping, labels, descending-byte/name-tie order and disk
probe projection. Actual ReadDir/usage ports and the injected selected-base disk
probe keep filesystem access in the native host; context/input/output/error are
transport-independent. Native read-only/primary-only registration, unknown args
and unary framing remain. Existing diagnostic best-effort ReadDir/entry/walk/probe
behavior stays explicit for the move; empty dirs remain null, zero/false fields
stay required and available zero disk values stay present via typed pointers.
Original/current actual owned-home native responses match exactly count=20 across
no/success/zero-free/zero-total/failed disk probes. Eight independent aggregation,
ordering/label/probe mutations fail. Related source/service/CLI/native/package
race count=20, complete controlplane race count=1 and full Go/build/vet/scoped
static/all gates pass. Official structure writer: 230 placed packages, unchanged
140 imports/13 calls, 121 generated kernel packages. **Next: artifact services,
then typed storage/artifact binding/audit and exit.** Local delivery is pending.

**W2.12b artifact service foundation:** typed app/artifacts owns get/list/delete/
collect over actual blob/index ports and injected clock. Native validation,
availability order, day/string and dry-run defaults, auth/audit/framing remain.
Typed records preserve required empty metadata/candidate fields, base64 bytes,
sentinel messages/original causes, empty arrays and dry-run-only candidates.
Actual index fixtures retain strict cutoff equality, unknown-time exclusion and
shared-blob ownership until the final reference is deleted. Four-handler x five-
input native parity passes count=20 with generated IDs/bounded cutoff clocks;
eight ref/size/filter/empty/default/cutoff/bytes/delete mutations fail. Service/
store/source/CLI/native/package race count=20, complete controlplane race count=1
and final full gates pass: 231 packages/139 imports/13 calls, official structure
122. The official updater removes the paid controlplane-to-artifact allowance
(140 -> 139 imports); call debt remains 13. Existing collection policy remains.
**Next: five typed storage/artifact native bindings and mandatory delete/collect
audit, then separately proved cleanup/reference repairs if confirmed and exit.**
Local code/docs delivery remains pending.

**W2.12c storage/artifact native binding:** five app specs replace wrapper/
registration business entry points; three reads remain unaudited unary responses,
while delete and collect require app audit before metadata/blob effects (including
collection dry-run admission). Owned actual closed-journal proofs showed legacy
success and deletion; permanent regressions now preserve index and blob bytes.
Actual tenant clients reject all five commands. Mutation audit is one correlated
invoked/completed span without duplicate native audit. DTOs retain absent/null
strict strings, raw ref/ID (no silent trim), exact filter strings, day number/digit
strings and dry-run default/boolean/exact false-or-0 string behavior. Availability
precedence and native result/error shapes remain. Storage exact five-probe parity
and four artifact handlers x five-input parity pass count=20; eight audit/raw-ref/
day/dry-run/presence mutations fail. Source/service/store/CLI/native/audit/tenant/
admission and package race count=20, complete controlplane race count=1 and final
full gates pass. Unchanged 231 packages/139 imports/13 calls, official structure
122. Actual filesystem/probe/blob/index bridges remain; old wrappers/registration
are gone. **Next: separately measured collection cleanup/reference repairs if
confirmed, then native storage/artifact exit.** Local delivery is pending.

**W2.12d artifact metadata-removal repair:** actual owned nonempty-directory
failure fixtures proved Delete returned nil after forgetting metadata and deleting
the blob; Collect counted that failure as reclaimed, and native delete returned
success. Index.Delete now removes metadata while holding index ownership, returns
the wrapped original non-ENOENT cause before forgetting the entry or touching its
blob, and keeps already-absent metadata idempotent. Collect's existing API counts
only successful deletions and retains failed candidates; it still reports partial
counts rather than a new batch error. Native typed delete returns one error frame
with unchanged entry/blob. Permanent actual core/native regressions are red on
old code, pass three verifiers count=20, and fail independently for ignored removal
and incorrect ENOENT handling. Related service/store/source/CLI/native/package
race count=20, complete controlplane race count=1 and full gates pass: unchanged
231 packages/139 imports/13 calls, official structure 122. This narrow repair does
not certify blob-GC failure handling, every writer race or journal raw_ref leases;
those stay explicit follow-ups under modules/artifacts. **Next: native exit.**
Local source/docs delivery remains pending.

**W2.12e storage/artifact native exit:** exact five-operation common-registry
coverage guards actual schemas/types, AppOwned metadata, primary/read-only/unary
scope and legacy unknown-input admission. Removing storage or artifact collect
independently fails the exit regression. [Exit evidence](32-w212-exit-evidence.md)
records diagnostic best-effort storage, strict raw/presence/default artifact
admission, required empty fields, dry-run and dedup/cutoff semantics, mandatory
audit/tenant isolation and separately proved metadata-removal failure repair.
Twenty-eight independent native-wave mutations pass their failure gates. Focused
service/store/source/CLI/native/audit/tenant/admission/exit and package race pass
count=20, complete controlplane race count=1 and final full gates pass. No-audit-
I/O framework benchmark: 6811 / 8893 / 7451 ns/op (<50 us); unchanged 231 packages/
139 imports/13 calls, official structure 122. **Next: schedule, then standing/
workflow/pulse/autonomy in roadmap order 4.** Core batch error redesign, remaining
writer/blob cleanup races and journal raw_ref leases stay explicit module work.
Native completion is local; A/B/C/D/E code/docs remain separate protected delivery
candidates while Git metadata/GitHub authentication restrictions persist.

**W2.13a schedule read/projection foundation:** typed app/schedule owns list,
system-task catalog, forecast and common entry execution metadata. Actual cadence
reader/clock ports and selected last-firing/runnable/warning callbacks keep native
validation and journal fold integration unchanged. Required empty/zero/false entry
fields, last-firing presence (including empty reason/zero timestamp), blocked/ready
and warning annotations, raw JSON payload and target/authority/identity/LLM
contracts retain their wire shapes. Forecast not-found remains only found:false;
found responses preserve empty mode/false enabled and empty forecast/count fields.
Native ID/numeric count/default/floor/cap admission, auth/read-only registration
and unary framing stay in wrappers. Add/edit retain a projection bridge; dead
metadata shims are removed. Existing best-effort firing annotation errors remain
explicit for the move. Three-handler x seven-input native parity and eight target/
agent projection branches pass exactly count=20; eight annotation/shape/forecast/
metadata mutations fail. Source/service/cadence/CLI/native and package race
count=20, complete controlplane race count=1 and final full gates pass: 232
packages/139 imports/13 calls, official structure 123. **Next: schedule admission/
lifecycle/firing services, then typed native binding/audit/exit.** Local delivery
remains separate from frozen earlier-domain code/docs patches.

**W2.13b schedule remove/run/enable lifecycle:** typed app/schedule now owns
validation-before-effect and success-only operator publication over the actual
cadence store. Run validates a found target before marking it due; enable validates
only when resuming, while pause does not validate. Missing/false results and
original store/validation causes are retained. The selected native bridge publishes
the existing schedule.enable payload with its host correlation only after a
successful update. Required ID and bool/string enabled admission, auth, registration
and unary framing remain native; this slice does not start the cadence engine.
Three-handler x four-target x ten-input response/store/action parity passes count=20;
eight validation/effect/cause/publication/action/ID mutations fail. Actual cadence,
source/service/native/CLI and package race count=20, complete controlplane race
count=1 and full gates pass. Structure remains 123; archcheck 232 packages/139
imports/13 calls. **Next: add/edit admission, target validation and firing services,
then native binding/audit/exit.** Delivery is local; Git metadata is read-only.

**W2.13c schedule runnable/frequency rules:** app/schedule owns live runnable
validation, agent tool policy and frequency warnings. Selected agent/workflow/tool
and managed-agent-message ports preserve native lookup order, untrimmed lookup
references, retired-before-paused and managed checks, tool re-read, normalized
allow/deny with deny precedence/default allow, known system tasks and exact
messages. Warning mode exclusions, task-before-guardian-before-intent precedence
and strict recommended/8-hour/15-minute thresholds are unchanged. Native wrappers
now only construct the selected host or forward; unused errString is removed
after its last caller moved. Add/edit share the moved tool
policy pending their own extraction. Actual selected-runtime parity across nine
agent states, five targets and four bindings (180 validations and 8,640 warning
combinations) passes count=20; ten gate/policy/mode/threshold mutations fail.
Related service/source/native/CLI and package race count=20, complete controlplane
race count=1/full gates pass. Structure 123 and archcheck 232/139/13 remain.
**Next: add/edit admission and firing services, then native binding/audit/exit.**
This slice and its separate source/docs patch remain local.

**W2.13d schedule add target admission:** typed AddTargetInput/AddTarget move
pre-cadence target/agent admission into app/schedule. Selected workflow-name port
retains canonical workflow names; agent references canonicalize to the returned
slug before the second tool-policy lookup. Intent remains the empty target constant;
explicit target/reference override order, conflicting-field errors, default task
text, present-null versus absent workflow/tool payload, ignored intent payload,
retired/paused/managed checks and deny/allow policy retain their original order.
Native argStrings admission remains first, target admission precedes numeric
cadence parsing, and five cadence create/bind/rollback/refetch paths remain native
for the next separate move. Exact response and persisted-entry parity across 36
native inputs passes count=20 (only generated IDs and bounded two-second clock
fields normalized); eleven canonicalization/precedence/presence/lookup/gate/policy/
zero-output mutations fail. Source/service/cadence/native/CLI/package race count=20,
complete controlplane race count=1/full gates pass. Structure 123 and archcheck
232 packages/139 imports/13 calls remain. **Next: cadence creation, edit admission
and firing services, then native binding/audit/exit.** Separate local delivery.

**W2.13e schedule cadence creation/binding:** typed CreationStore/CreateCadence/
CreateInput and injected clock own the five cadence add variants, agent then target
binding, best-effort compensation and final refresh/projection. Native selected-mode
and strict numeric/timezone parsing retain their original order after target
admission; interval/cooldown lower-bound messages move into Create without changing
native results. Fractional-second/integer truncation, continuous core minimum clamp,
operator source, injected creation clock, four binding false/error messages, original
creation/binding causes and zero failure outputs are pinned. Actual-store five-mode
x four-target coverage verifies durable target/payload/model normalization after
refetch; a missing refetch retains the legacy local binding projection. Binding
failure tests cover successful/failed compensation without changing best-effort
semantics (store atomicity remains separate work). Forty-input old native response/
store-entry parity passes count=20, normalizing only generated IDs/bounded clocks;
eleven compensation/cause/refetch/fallback/time/bounds/routing/minimum mutations
fail. Source/service/cadence/native/CLI/package race count=20, complete controlplane
race count=1/full gates pass. Structure 123 and archcheck 232 packages/139 imports/
13 calls remain. **Next: edit admission and firing services, then native binding/
audit/exit.** Separate local delivery; no cadence engine is started by this slice.

**W2.13f schedule edit target admission:** typed EditTargetInput/EditTarget move
current/requested target and agent validation into app/schedule before unchanged
cadence preflight and field/target/reschedule mutations. The last-used tool-policy
forwarding shim is removed. Required-ID and missing-ID
updated:false short circuit remain native before strict target-string decoding;
agent decoding stays lenient with separate presence. Absent/explicit empty intent
target, inherited target/agent, clearing agent, requested-agent canonicalization,
system-task payload/agent rejection, target/reference precedence/conflicts and
agent-before-target-before-tool-policy-before-payload errors retain their order.
Workflow reference remains uncanonicalized during preflight because the later
native second lookup is deliberately retained. Existing inherited-agent tool
checks remain policy-only (no extra paused/retired gate); unrequested workflow/tool
payload remains ignored. Exact response/store-state parity for 42 inputs x four
starting targets passes count=20 (generated IDs/bounded next-run clocks only),
including missing/invalid IDs and retained late-error partial updates. Eleven
presence/inheritance/canonicalization/gate/lookup/policy/payload mutations fail.
Source/service/cadence/native/CLI/package race count=20, complete controlplane race
count=1/full gates pass. Structure 123, archcheck 232 packages/139 imports/13 calls.
**Next: edit cadence/mutation services and firing integration, then native binding/
audit/exit.** Separate local delivery; edit atomicity remains separate work.

**W2.13g schedule edit cadence preflight:** app/schedule ValidateEditCadence
owns timezone/future/minimum/window/day checks over typed CadenceNumber parse
results (value/presence/original cause) and EditCadence. Native numeric decoding
remains a codec; errors are captured without throwing before domain selection.
Timezone validation still runs first with its lenient string cast and trimmed zone,
then once/continuous/window/daily/interval selection retains original parse/error
priority and ignored unselected causes. Float-to-int/duration truncation, future
strictness, interval/cooldown minimum, minute/day bounds and window ordering are
unchanged. Existing numeric-type coverage now calls the app validator through the
native codec; the old business helper is removed. Native missing-ID/target preflight
still precedes cadence input construction, while late strict timezone decoding and
field/target/reschedule writes remain native for the next move. Exact response and
store-state parity for 49 inputs x four starting targets passes count=20 (generated
IDs/bounded next-run clocks only); twelve timezone/variant/cause/minimum/bounds/
presence mutations fail. Source/service/cadence/native/CLI/package race count=20,
complete controlplane race count=1/full gates pass. Structure 123, archcheck 232
packages/139 imports/13 calls. **Next: edit mutation service and firing integration,
then native binding/audit/exit.** Separate local delivery; late-error partial update
and atomicity semantics remain unchanged.

**W2.13h schedule edit mutation service:** typed EditingStore/EditingHost,
Begin/EditState and Apply/EditInput/EditOutput own the initial live read and found-
only clock snapshot, cadence preflight, intent/model/agent setter order, second
workflow/tool lookup and payload serialization, target binding, late strict timezone
cause, five reschedule variants and final projection. Native handler retains required
ID, strict target strings and lenient optional strings/presence decoding; missing ID
still returns only updated:false before any further decode. Original intent/target/
reschedule causes propagate, model/agent errors and false setter results retain
legacy best-effort handling, and late errors retain already-applied fields/targets.
Workflow canonicalization occurs at its second lookup. Final missing refetch still
projects the zero entry with updated:true, while found refetch preserves normalized
payload/model/agent fields. The view/merge forwarding shims are removed after their
last caller moves. Actual-store five-cadence x four-target and phase/false/cause/
missing/partial-update tests pass; exact old response/store-state parity for 49 inputs
x four starting targets passes count=20 (generated IDs/bounded clocks only).
Thirteen clock/preflight/cause/ignored-error/routing/lookup/late-timezone/canonical-
bind/result/refetch/reschedule/payload mutations fail. Source/service/cadence/native/
CLI/package race count=20, complete controlplane race count=1/full gates pass.
Structure 123, archcheck 232 packages/139 imports/13 calls. **Next: firing integration,
then typed native binding/audit/exit.** Separate local delivery; edit atomicity and
best-effort failure repairs remain separate work. No engine is started by editing.

**W2.13i schedule firing journal views:** typed FiringJournal/FiringRun snapshot
ports and Fires/Stats/Latest own history filtering/projection, aggregate counts and
list last-firing annotations. The measured native surface includes ten commands:
eight schedule management/read commands plus tenant-routed read-only fires/stats.
Selected kernel journal and the shared collectRuns fold remain actual host ports;
native strict filter strings, lenient limit/default/cursor/since decoding, pre-route
cutoff anchor and tenant lookup stay in wrappers. Service owns floor-one/cap-1000,
ID/status/case-insensitive intent/cutoff filtering before sort/limit, same-ms sequence
ordering, strict cursor/last-row boundary, outcome precedence/duration/spend/preview,
required empty/zero/false row fields and optional nonempty runbook. Payload/classifier
helpers move unchanged; malformed/legacy payload degrades to zero fields. Stats
retain terminal rate, empty-reason fallback, named schedule cardinality, required
empty map and original window value. Latest retains first same-ms firing and skips
missing schedule IDs; list retains existing best-effort annotation errors. Run-fold
or range errors propagate original causes and zero outputs. Exact three-handler x
27-input owned native response parity passes count=20; socket primary/owner-routed/
tenant-token reads prove selected journal/run joins and foreign denial. Actual owned
journal sequence/tie, required/optional/empty shape, filters/page/cap/status/duration/
error tests and nineteen mutations pass. Source/service/cadence/journal/native/CLI/
package race count=20, complete controlplane race count=1/full gates pass. Structure
123, archcheck 232 packages/139 imports/13 calls. **Next: ten-command native binding/
audit/exit.** Cadence resident run execution belongs to W2.2/W4, not this read-view
wave. Separate local delivery; no engine or external provider is started here.

**W2.13j schedule typed native binding/audit:** ten unary app operation specs
now own native schedule dispatch. Five reads remain unaudited; add/rm/run/enable/
edit require mandatory audit before cadence or provider effects. Eight primary-only
commands and tenant-owned/routed fires/stats derive native metadata from app specs.
Typed raw request fields preserve absence/null, strict strings/numbers, lenient
optional edit strings/enable forms/limit/since, unknown args, selected cadence parse
order and missing-edit early return. Explicit input schema defers per-command
validation; explicit record/list/edit output schemas derive Record fields with raw
JSON payload supported, retaining required payload/updated and optional presence.
Five old native wrapper files and old registration/decode/write helpers are removed;
selected host bridges remain. Numeric source tests are repointed to serialized app
request codecs. Enable operator actions now join the host-owned audit correlation;
raw corr/correlation_id cannot select that identity. Actual closed-journal proofs
block all five mutations, five reads retain unary/no audit, eight-command tenant
denial preserves primary cadence/provider, and existing tenant firing join tests
stay green. Exact old handler versus new dispatch response/store-state parity for
ten commands x thirteen inputs passes count=20, normalizing generated IDs/bounded
clock and generic framework error-code fields only. Twelve audit/scope/decode/form/
correlation/schema/read-only/aggregate mutations fail. Source/service/cadence/journal/
native/CLI/package race count=20, complete controlplane race count=1/full gates pass.
Structure 123, archcheck 232 packages/139 imports/13 calls. **Next: schedule native
exit evidence, then standing/workflow/pulse/autonomy in order 4.** Separate local
delivery; resident run execution and wider trigger/module work remain W2.2/W4/W3.

**W2.13k schedule native exit:** exact ten-spec common registry and metadata,
primary/tenant policies, unary framing, typed schemas and legacy input compatibility
are guarded by TestScheduleNativeMetadataComesFromTenTypedAppSpecs. Removing the
family or tenant firing command independently fails exit coverage. Source/read/
create/edit/firing services and old native wrappers are already moved/deleted;
selected actual host bridges remain. [Exit evidence](33-w213-exit-evidence.md)
records 117 valid wave mutations, native parity/source/owned store/journal/audit/
tenant/schema evidence and preserved failure/partial-update boundaries. Framework
no-I/O dispatch benchmark is 7674/6024/6569 ns/op, 7610/7607/7609 B and 89 allocations,
below 50 us; construction/audit/journal/whole-operation latency is excluded. Final
focused exit/source/package race/CLI reruns count=20 and architecture/static/dead/
dependency/official structure checks pass; unchanged production full Go/build/vet
and complete controlplane race evidence is retained from W2.13j. 232 packages/139
imports/13 calls, 123 generated kernel packages. Resident runs.Start, W3 modules,
W4 triggers and W5 surfaces remain open; no deployment/publication is claimed.
**Next: standing, workflow, pulse and autonomy in order 4.** Local protected delivery
remains pending; source/docs patches stay separate.

**W2.14a standing CRUD/projection foundation:** typed app/standing reader,
actual runtime-writer facade and selected agent/message ports own List/Add/Edit/
SetEnabled/Remove, wire projection, agent validation and frequency warnings. Core
Order embedding keeps required id/name/enabled/triggers/initiative/timestamps and
legacy optional omissions; list adds blocked/ready target and exact cron/event
warnings, counts enabled orders independently of blocked state, and returns [].
Add validates agent before facade persistence without normalizing the stored raw
agent. Edit preserves patch presence, trim-only agent change, initiative/briefing
fields, float-to-int assure/cooldown conversion and updated:false missing result.
Resume validates the found agent; pause skips that gate. Original writer causes
and zero failure outputs are retained. Actual runtime facade lifecycle publications
standing.created/updated/removed remain in order, with core ID/timestamp/enabled
ownership and store rollback behavior untouched. Native ID/order JSON/strict patch
field decoding and registration stay in wrappers. Exact five-handler x fifteen-
input old/new response/store-state parity passes count=20 with generated IDs and
2-second bounded timestamps. Existing non-bool enabled accessor inputs remain
false (the legacy string-form comment was not treated as authoritative behavior).
Twelve list/annotation/count/warning/gate/pause/patch/missing/cause mutations fail;
source/service/core/CLI/native and package race count=20, complete controlplane
race count=1/full gates pass. 233 packages/139 imports/13 calls, official structure
124. **Next: standing why/fire services, then typed binding/audit/exit.** Separate
local delivery; resident execution and broader trigger/module work remain open.

**W2.14b standing why/fire services:** typed Observations/Journal/LifeEvent and
Firing/callback ports own history projection and manual dispatch admission. Why
retains standing.* kind-prefix matching (including future/custom kinds), exact
payload id match, malformed/foreign/nonstring-id skips, journal order, six required
row fields including zero sequence/timestamp/empty identity values, and events:null
when empty. Existing ignored Range errors retain collected partial rows without
an error; this best-effort read policy is explicit and remains separate repair work.
Fire retains callback availability before order lookup, missing order fired:false
with request ID, then agent validation before dispatch; callback result and request
identity are retained. The last validateStandingAgent forwarding helper is removed.
Native required-ID decoding, public SetStandingFire injection and seven-command
registration stay until binding. Actual owned journal sequence/zero timestamp and
recording callback tests pass; exact two-handler x eight-input x three-callback-mode
old/new response/callback parity passes count=20. Nine prefix/id/empty/required-
field/range-policy/availability/validation/identity/result mutations fail. Source/
service/core/journal/native/CLI and package race count=20, complete controlplane
race count=1/full gates pass. 233 packages/139 imports/13 calls, structure 124.
**Next: seven-command typed native binding/audit/exit.** Separate local delivery;
resident runner/run pipeline and broader trigger/module work remain open.

**W2.14c standing typed native binding/audit:** seven explicit primary-only,
unary specs own two unaudited reads (list/why) and five mandatory-audit mutations
(add/edit/set_enabled/remove/fire). Explicit raw-field input schema and typed
RequestInput preserve absence/null/wrong types, required IDs without trimming,
order object re-encoding, strict upfront patch validation order, fractional number
conversion and observed non-bool enabled=false behavior. Derived output schemas
retain embedded order wire/optional omissions, required false/zero fields and
null empty history. Aggregate app/native registry is the only command binding;
old CRUD/why/fire handlers and registration are removed, public SetStandingFire
and selected service bridge remain. Callback is resolved at dispatch time.
Runtime facade lifecycle events retain their original empty correlation; app
invoked/completed/failed share an owned correlation and raw spoof is ignored.
No correlated-domain rewrite or resident execution convergence is claimed.
Actual closed journal blocks all five state/callback effects; actual tenant socket
rejects all seven without state/provider/callback effects. Two reads add no audit;
fire unavailable/declined settle exactly once with joined audit identity.
Exact old/native vs app response/state/callback parity (75 CRUD plus 48 why/fire
combinations) passes count=20, with generated IDs/two-second clock bounds and only
generic error-code boundary normalization. Twelve admission/metadata/decoder/
callback/aggregate mutations fail. Focused/source/native/CLI and package race
count=20, complete controlplane race count=1/full gates pass. Archcheck remains
233 packages/139 imports/13 calls, structure 124. Standing exit evidence follows;
legacy Why partial-error policy and wider runner/trigger/module work remain open.
Separate local delivery; Git metadata unchanged.

**W2.14d standing native exit:** exact seven-operation aggregate and complete
[exit evidence](34-w214-exit-evidence.md) close native management/history/fire
admission. Missing fire or why independently breaks the registry regression.
35 valid wave mutations; source/native/store/journal/runtime-facade/callback/
audit/tenant/parity evidence. Final native/CLI/package-race count=20 and static/
architecture/dead/dependency/official structure pass; unchanged c production full
Go/build/vet/controlplane race evidence retained. No-I/O framework benchmark
6355/6785/6483 ns/op (<50us). 233 packages/139 imports/13 calls, structure 124.
Runtime lifecycle correlation and Why best-effort errors retain existing policy;
resident runner, runs.Start, W3/W4/W5 and other adapters remain open. Separate
local delivery. **Next: workflow, pulse and autonomy in order 4.**

**W2.15a workflow read/projection foundation:** measured native surface is 13
primary-only commands: list/show/runs/templates reads, nine management/copilot/
execution mutations. Typed app/workflow reader/journal/template ports now own
light/full graph DTOs, list/show/template projections and latest-run fold. PrepareList
retains store read before native with_runs admission; journal scan remains opt-in
and once across all names, even an empty list. Native flag/reference/type/error
framing and existing registry remain until operation binding.
Light rows omit nodes/edges; full rows retain raw node Config JSON and edges
omitempty; required identity/false/count/timestamp fields and optional description/
trigger detail/last-run duration survive. Trigger detail precedence stays webhook,
interval, daily, subject, with core daily normalization. List preserves order,
enabled count, [] empty and only matching last-run annotations; template rows keep
all six required fields and built-in gallery, [] empty. Show trims lookup while
retaining raw missing-ref error identity and zero failure output. Latest fold keeps
newest journal started arc, terminal correlation matching, requested subject/nonempty
correlation gates, terminal-without-start skip, finish/start duration guards and
legacy best-effort partial Range error behavior. Original six fold tests move with
production rather than retaining adapter business.
Exact three-handler x fourteen-input native parity plus eight configs x light/full
canonical JSON projection parity passes count=20 without wire normalization.
Actual owned graph store/journal plus projection/opt-in/count/ref/gallery/partial-
error contracts pass; twelve valid mutations fail regression assertions. Service/
core/journal/native/CLI and related package race count=20, complete controlplane
race count=1/full gates pass. 234 packages/139 imports/13 calls, structure 125.
Only shared typed projection/JSON bridge remains for other native methods; save/
restore/enable/remove, run history, copilot, run/node/webhook/detached execution and
13-command binding/audit/exit follow. Wider run/trigger/module/surface work remains
open; separate local protected delivery.

**W2.15b workflow lifecycle foundation:** typed Lifecycle/Writer ports own
Save/Restore/SetEnabled/Remove through the actual runtime facade. Correlation,
posted workflow, reason, raw reference and bool pass unchanged to the four writer
methods; native wrappers still supply empty correlation and preserve JSON/type/
required-ref/reason/enable admission. Workflow bool/string enable transport really
accepts case-insensitive true or exact 1 (without trimming), unlike standing's
observed non-bool false path. Save/Restore return typed full graph and required
created (including false); enable returns light projection; remove keeps required
removed:false for unknown refs. Writer failures return zero output with original
cause. Enable ErrNotFound retains raw-reference unknown-workflow message and
sentinel classification; native custom error framing remains unchanged.
Actual runtime fixture proves save update preserves ID/created/paused enabled,
restore trusts posted checkpoint ID/created/enabled, remove deletes, and five
saved/updated/saved/restored/removed events retain actor and requested correlation.
Core validation remains at the facade/store; lifecycle publication remains best
effort, including a successful durable save after journal close. Mandatory app
operation admission and owned default correlation remain later binding work.
Exact four-handler x eighteen-input native response/store/lifecycle event parity
passes count=20, with only generated IDs and two-second clock bounds normalized.
Port argument/result/required fields/original causes plus actual owned facade/store/
journal regressions pass; twelve valid correlation/reason/ref/bool/presence/result/
classification/cause mutations fail. Service/core/journal/native/CLI and package
race count=20, complete controlplane race count=1/full gates pass. 234 packages/
139 imports/13 calls, structure125. Run-history, copilot and execution services,
then13-command binding/audit/exit follow; native decoder/registry and wider run/
trigger/module/generated-surface work remain open. Separate local delivery.

**W2.15c workflow run-history foundation:** typed History/RunsInput/RunsOutput,
RunRecord and NodeEvent own workflow_runs journal fold. PrepareRuns trims lookup,
keeps raw missing-ref identity and captures canonical workflow subject before
native limit decoding; its snapshot is retained even if the graph later changes.
Native required-ref/custom error framing, lenient numeric admission and registry
remain. Observed present non-number limits (including numeric strings) become
zero through argFloat64 presence, then service default20; no compatibility repair
is silently added. Pure service normalization defaults <=0 to20 and caps at100.
History filters exact subject/nonempty correlation and accepts only start/node/
completed/failed events. It reverses correlation first-seen order, not timestamp
order, and limits after fold; terminal/node-only arcs retain started_ms:0 and
node_events:null. Started metadata and time overwrite, without clearing existing
status/finish/error; success after failure retains previous error. Legacy malformed/
partial payload decoding survives. Node probes/missing IDs are skipped, missing OK
defaults true, explicit OK/handled:false survive, attempts appear only >1, and
nonempty metadata/snippets/executed remain optional. Only positive finish time is
emitted. Empty runs stays []; journal failure returns zero output and journal:
prefix, preserving the underlying cause through the service boundary.
Exact old/current native response parity over27 inputs and125 owned run arcs
passes count=20 without normalization. Owned actual zero-time journal, canonical
snapshot/limits/order/required zero-false-null/metadata/probe/decode/error contracts
pass;18 valid limit/snapshot/filter/presence/decoder/order/cause mutations fail.
Service/core/journal/native/CLI and package race count=20, complete controlplane
race count=1/full gates pass.234 packages/139 imports/13 calls, structure125.
Old native business fold/limits are removed; only required ref/limit framing stays.
Copilot/execution then13-command typed binding/audit/exit follow; wider run/trigger/
module/generated-surface work and separate local protected delivery remain open.

**W2.15d workflow copilot foundation:** typed Copilot/Designer/correlation ports,
DraftInput/RefineInput/Refinement and full CopilotOutput own draft/refine orchestration.
Posted nonnull graph wins without store lookup (including zero graph, whose
validation remains runtime-owned); absent/null resolves trimmed ref before identity
allocation/provider effects. Missing base keeps required-base sentinel and native
failMsg framing; missing ref keeps raw-reference unknown workflow identity.
PrepareRefine captures stored graph before later provider invocation. Native strict
instruction/description/name, posted JSON and optional-ref decode order stay.
Fresh correlation is allocated before each designer call; legacy background context
is bounded at3m and canceled on both success/error. Raw name/description/instruction,
selected base and correlation pass unchanged. Result uses full graph projection;
original causes return zero output, without exposing a failed correlation. No writer
port is present. Actual runtime with mock provider proves draft name override,
refine stored name, two distinct correlations and drafted mode/event joins, while
the stored graphs remain unchanged. Original runtime repair/validation/unsaved contracts
also pass count=20. Context/correlation convergence with operation audit remains
later binding work; this foundation retains the legacy background/fresh policy.
Exact two-handler x22-input native response/provider request/drafted event/unsaved
state parity passes count=20, normalizing only fresh correlation values and checking
the response/request/event joins before normalization.14 valid budget/identity/
cleanup/input/precedence/lookup/preflight/snapshot/cause/zero-output mutations fail.
Service/core/journal/native/CLI/runtime and package race count=20, complete
controlplane race count=1/full gates pass.234 packages/139 imports/13 calls,
structure125. Native copilot timeout/business selection and last unused projection bridge are removed; codec/registry
remain. Run/test-node/webhook/detached execution then13-command typed binding/
audit/exit follow; broader run/trigger/module/generated-surface work and local
protected delivery remain open.

**W2.15e workflow execution foundation:** typed ExecutionHost/Execution/Wake,
RunInput/RunOutput, NodeInput/NodeOutput and WebhookInput/WebhookOutput own manual
sync/async runs, single-node probes, authenticated reply/async webhook dispatch
and detached panic recovery. Selected runtime run/probe methods remain governed;
full seven-field wake conversion, stderr panic diagnostic and existing workflow.panic
publication stay host bridges. No execution logic remains in native wrappers.
Manual sync keeps raw ref/payload, fresh identity, manual wake and background15m;
async allocates identity before trimmed graph lookup, rejects unknown synchronously,
captures canonical name/payload, queues one detached job, returns required accepted/
async/workflow/correlation fields and retains15m detached execution. Observed
present non-bool async values remain false through native argBool presence semantics.
Sync requires executed/outputs even when null; pointer DTOs preserve variant omission.
Missing refs keep raw unknown message; other run failures retain cause and correlation
suffix, with zero service output. Detached failures remain journal-owned; panics
notify once after context cleanup with original wake/correlation/name/value.
Node probe retains posted graph/node/data/payload unchanged, fresh identity,
background3m, original cause/zero failure and required output:null/port:""/attempts:0.
Webhook invalid/blank/empty-secret/unknown/paused/non-webhook/bad-secret all refuse
uniformly before identity/run/detach; constant-time secret comparison remains.
Authenticated reply uses canonical name, webhook trigger provenance and2m budget,
required executed/outputs and prefixed correlation error. Async replies omit run
outputs and queue the same detached15m path. Legacy background/fresh identity policy
is explicit; caller-context/owned operation audit convergence remains later work.
Exact three-handler x28-input old/current response plus lifecycle/node/provenance
parity passes count=20. Only fresh identity and event duration_ms are normalized;
accepted jobs are awaited to terminal event before comparison. Owned transform/probe
runtime and original policy/provenance/node contracts plus actual native panic
containment remain green.24 valid budget/identity/cleanup/ref/payload/field presence/
node inputs/webhook gates/provenance/reply/panic/cause mutations fail. The wrong-kind
fixture was corrected to satisfy the secret gate, making its kind mutation independent.
Service/core/journal/native/CLI/runtime and related package race count=20, complete
controlplane race count=1/full gates pass.234 packages/139 imports/13 calls,
structure125. Native timeouts, manual orchestration, webhook gate and detached
business helper are removed; only codecs/custom error framing/manual registry and
selected host bridges remain. **Next:13-command typed binding/audit, caller context/
correlation refinement and exit.** Wider runs.Start/trigger/module/generated-surface
migration and separate local protected delivery remain open.

**W2.15f workflow typed native binding/audit:**13 primary-only unary specs now
own the full native family: list/show/runs/templates are unaudited reads; save/
restore/remove/set_enabled/run/draft/refine/webhook/test_node require durable audit
before actual graph/provider/probe/webhook/detached effects. Typed raw RequestInput
and explicit input schema retain absent/null/wrong types, required untrimmed refs,
posted JSON re-encoding, strict field/error order and ignored unknown args. Preserve
store read before with_runs flag; graph lookup before history limit; real workflow
enable bool/string forms (case-insensitive true/exact1/no trim), and observed
non-bool async=false plus nonnumber history limit=>default20. Refine posted nonnull
graph wins over ref; null graph uses ref. Probe workflow then node then data type
admission, and webhook malformed inputs retain uniform refusal.
Derived ordinary outputs and explicit full graph/show/save/copilot/template schemas
keep arbitrary raw node Config JSON, full required nodes, created:false, required
identity/count/false/empty fields and optional/variant omissions. Aggregate/native
metadata comes from these specs; five wrapper files, manual registration, common
native write helper, last unused argFlag helper and registry hook are removed. Original three boolean contracts move to the typed request decoder. Selected service/runtime/wake/
logging/publication bridges remain. Four reads add no operation audit; actual
closed journal blocks all nine mutations before graph/provider effects. Real
socket tenant credentials are denied all13 without graph/provider effects.
One owned invoked/completed audit span ignores raw correlation spoof, while runtime
lifecycle still keeps legacy empty correlation. Copilot/execution retain fresh
identity/background budgets. Caller context/domain correlation convergence remains
explicit later work, rather than being claimed by this binding.
All13 native commands pass combined old/native vs typed dispatcher parity count=20:
42 read+72 lifecycle+27 history+44 copilot+84 execution scenarios (269 total), plus
16 canonical graph projections. Store/provider/request/event/async outcome evidence
is retained. Normalize only established generated identity/bounded clock/event
lifetime/generic framework error-code boundaries.18 valid mutation proofs cover
nine mandatory-audit flags, read/primary/unknown-arg policy, bool/limit forms,
raw-config/required nodes/created schemas and aggregate inclusion.
Source/service/core/journal/native/CLI/runtime and related package race count=20,
complete controlplane race count=1/full gates pass.234 packages/139 imports/13
calls, structure125. **Next: caller context/correlation refinement and native exit**,
then pulse/autonomy in order4. Broader runs.Start/trigger/module/generated surfaces
and separate local protected delivery remain open.

**W2.15g workflow caller context/correlation refinement:** before proof on f
reproduced all nine direct mutation paths ignoring a pre-canceled caller, five
blocking designer/run/probe/reply paths ignoring cancellation/budget/values, and
fresh/empty domain identity split from the operation host. Permanent unchanged
proofs now pass count=20: canceled caller blocks writer/designer/run/detach and
fallback identity allocation; in-flight cancellation reaches all five blocking
ports; a10s parent budget/value survives existing15m/3m/2m ceilings.
Lifecycle checks caller state before writes and chooses host-owned correlation
before explicit service ID; direct context-free callers retain explicit/empty IDs.
Copilot/run/probe/authenticated webhook use owned identity when present, fresh
runtime fallback only without a host. Blocking work inherits caller cancellation,
values and earlier deadline; both return paths still cancel child timers.
Accepted manual/webhook jobs use context.WithoutCancel to preserve caller actor/
profile/provenance/identity values, remove caller cancellation/deadline and apply
the original independent15m ceiling. The unused public Detached entry is removed; direct tests cover the private
background path. Uniform webhook gates, codecs/schemas/policy/runtime execution
and response variants remain; caller-canceled service calls return their cause
before effects. Lifecycle writer failures/cancellation retain zero outputs.
Actual nine-command socket fixtures verify one owned invoked/completed audit span
joined to lifecycle/drafted/started/node events and returned correlation, ignoring
raw corr/correlation_id spoof. Actual direct dispatcher cancellation stops an
owned60s delay within the3s verifier bound, produces joined invoked/started/
workflow.failed/op.failed events and no provider call. No new transport-disconnect
or live-provider guarantee is claimed; supplied caller context is what is proved.
16 valid identity precedence/fallback/budget/detached value-cancellation/admission
mutations fail. Original direct fallback/ceilings/presence/gate/panic tests are
retained; legacy tests that expected discarded caller context now assert preserved
values. The f old/native identity parity is historical: these owned identity and
cancellation semantics are explicit approved-architecture changes.
Source/service/core/journal/native/CLI/runtime and related package race count=20,
complete controlplane race count=1/full gates pass.234 packages/139 imports/13
calls, structure125. **Next: workflow native exit**, then pulse/autonomy in order4.
Broader runs.Start/trigger/module/generated surfaces and local protected delivery
remain open.

**W2.15h workflow native exit:** exact13-operation aggregate and complete
[exit evidence](35-w215-exit-evidence.md) close native management/history/copilot/
execution admission. Missing webhook or test_node independently breaks the registry
regression.116 valid wave mutations; source/native/store/journal/designer/run/
probe/panic/audit/tenant/context/correlation evidence. Final native/CLI/package-race
count20 and static/architecture/dead/dependency/official structure pass; unchanged g
production full-check evidence retained after narrow unused-entry cleanup.
No-I/O framework7276/6486/8486 ns/op (<50us).234 packages/139 imports/13 calls,
structure125. **Next: pulse/autonomy in order4**; pulse has13 resident control
commands plus its subscribe/replay stream, so measure the complete surface before
migration. Broader runs.Start/W3/W4/W5, other adapters and local protected delivery
remain open.

**W2.16a pulse resident controls foundation:** complete native surface measured
as14 commands:13 resident controls and pulse_subscribe replay/live stream. Typed
app/pulse Controller/Observers/Controls and setting ports own status/asks, pause/
resume/beat/ask resolve/cadence/dial/quiet/flush/watch/probe/unwatch services.
Public SetPulse/SetPulseObservers boot contracts retain source-compatible aliases;
selected factory and existing best-effort settings load/save stay host bridges.
Native controller/observer availability before argument decode, strict strings,
bool/number/string forms, validation/framing and registration remain until binding.
Status preserves dynamic opaque snapshot and overrides enabled:true on the same
map; disabled status is only enabled:false. Disabled asks stays []; active nil asks
remains null. Pause/resume/resolve retain required false flags, flush/unwatch zero
counts and original error messages/zero service failure output. Resolve preserves
raw issue key and approve; native approval strings are case-sensitive true/exact1.
Cadence keeps duration truncation, live applied milliseconds and persists applied
(not requested) duration string after live change. Dial/quiet persist applied
values; persistence remains best effort. Observer availability is independent of
controller, raw watch path/pct and probe name survive; pct bounds (0,100), strings.Fields
command splitting rather than shell parsing, unchanged false-result errors and
required added/observer outputs remain. Service interfaces do not execute a probe
command or start an engine. Actual owned unstarted core engine proves cadence clamp,
dial normalization, pause/resume/quiet and queued beat without provider calls.
Exact13 handlers x4 availability modes x16 inputs (832 combinations), response/
controller-observer argument/call/actual settings-reload parity passes count=20.
18 valid status/empty/required false-zero/conversion/applied persistence/ref/bool/
observer/split/failure mutations fail. Service/source/native/CLI/journal and package
race count=20, complete controlplane race count=1/full gates pass.235 packages/
139 imports/13 calls, structure126. Subscribe/replay service and14-command binding/
audit/exit follow; resident trigger execution/W3/W4/W5 and other transports remain
open. Separate local protected delivery.

**W2.16b pulse replay foundation:** typed Journal/ReplayInput/ReplayOutput/emitter
port owns historical replay, with no net/socket dependency. Preserve journal order,
all active seq/time/subject/kind/correlation filters as AND, inclusive lower and
exclusive upper boundaries, nil kind map=no filter versus empty map=reject all,
exact correlation and bus wildcard grammar. Events reach emitter as original
pointers/full frames. LastWritten advances only after a successful write, stays
last journal-order emitted seq (not re-sorted/maximized), and remains-1 when none
were sent, including past-head queries. Original range/write cause and partial
checkpoint survive. Positive rate retains duration conversion, first-event immediate,
success-anchored wait, time.Since adjustment and cancellation-aware timer stop;
unlimited replay retains legacy emitter-owned cancellation semantics.
Actual owned journal proves sequence0 is durable and half-open bounds preserve it.
Native old/current replay framing/last seq/error parity across14 modes passes
count=20 without normalization; socket-free filter/order/sentinel/partial-failure/
rate/context and actual-journal tests pass.12 valid cutoff/match/kind/correlation/
sentinel/checkpoint/write/rate/cancellation mutations fail, including a bounded
stalled-wait verifier. Service/source/native/CLI/journal and related package race
count=20, complete controlplane race count=1/full gates pass.235 packages/139
imports/13 calls, structure126. Native replayHistorical is now only selected
journal/emitter framing. Native subscriber still owns argument admission, pre-replay
subscription, live durable dedup/ephemeral pass-through, drop notices, client close
watcher and server-context stop; those move next.14-command binding/audit/exit and
broader trigger/module/generated-surface work/local delivery remain open.

**W2.16c pulse live stream foundation:** typed Stream/Subscription/DropTicker,
selected journal/subscriber/emitter/clientGone ports own subscribe-before-replay,
4096 buffer, bounded replay/live transition, durable checkpoint dedup, kind/
correlation live filters, drop notices and lifetime cleanup. Subscriber is canceled
on every acquired path; bounded replay opens neither watcher nor ticker. Replay
errors retain wrapped cause/pulse replay prefix, subscription errors stay original,
and closed subscription keeps its native message. Clean caller/client close or live
write failure end silently with no terminal result, retaining event-only native wire.
Socket decode/clear-read-deadline and lazy500ms timeout-versus-disconnect watcher
stay adapter mechanics; watcher begins only after successful replay and only live.
Selected bus adapter converts actual channel/drop atomic/cancel ownership. Default
ticker remains1s and stops on return. Durable seq<=last replay checkpoint is skipped;
ephemeral hash-empty events pass even seq0; nil kind map differs from empty, exact
correlation stays. Synthetic per-stream notice is emitted without user filters,
seq0/hash-empty/empty correlation, agezt actor and exact dropped delta/total JSON;
no new notice for unchanged counter, counter progress advances after sample.
Original Replay service is reused; its last native forwarding helper is removed.
Owned actual bus/journal bounded replay and deterministic port tests cover order,
dedup/ephemeral/filter/no-match, errors/writes/cleanup, drop growth/no-growth and
client/server stop.17-mode old/current native full framing parity (bounded/filters/
admission/idle close/replay-to-live) passes count=20 without normalization.16 valid
buffer/cleanup/replay/dedup/filter/drop/framing mutations fail, including a bounded
stalled replay-only verifier. A missing controlled ticker sample in the strengthened
drop fixture was corrected; it is not a product finding. Service/source/native/
CLI/journal and related package race count=20, complete controlplane race count=1/
full gates pass.235 packages/139 imports/13 calls, structure126.
**Next:14-command typed binding/audit with explicit event-only stream adapter
compatibility**, then exit. Wider resident trigger execution/module/generated
surfaces/other transports and separate local protected delivery remain open.

**W2.16d pulse resident typed binding:** thirteen resident controls now derive
native registration, schemas, read policy and primary-only aggregate tenancy from
app.ControlOperations; two reads are unaudited, eleven mutations require audit
before controller, observer or settings effects. Runtime controller/observer ports
are selected on each dispatch; public setters/aliases and best-effort applied-value
settings persistence remain. Raw argument presence, disabled-before-validation,
number/string cadence/min_pct, exact case-sensitive approve, raw strings, Fields
probe splitting, false/zero/null/opaque outputs remain measured native conventions.
Resident handlers, duplicate registry and unused response/availability helpers are
removed; controlplane retains composition/persistence and streaming adapter.
832 legacy/current native response/controller/observer/argument/settings cases pass
count=20. The unavailable-audit before proof fails against all eleven old handlers
(they still acted); unchanged after proof passes count=20 with zero effects. Permanent
actual closed-journal admission, paired invoked/completed/failed correlation, read
policy, output schema, dynamic setter selection, canceled admission, nineteen
argument/persistence cases and thirteen-command real tenant socket denial pass.
Twenty valid audit/presence/availability/argument/registry mutation regressions fail.
The source raw-argument ratchet caught the removed handlers dropping2 casts to0;
its quota is reduced to0 and an additional ratchet mutation fails.
Two temporary proof/harness import/helper dependencies were corrected; no product
finding is inferred from harness compilation failures. Source/service/resident native/
CLI/journal repeats and related race count=20, full controlplane race count=1 and all
Go gates pass.235 packages/139 import/13 call allowances remain; structure126.
**Next: pulse_subscribe typed StreamLive operation with explicit event-only native
adapter compatibility**, then W2.16 exit. Resident trigger execution/module/generated
surfaces/other transports and protected local delivery remain open.

**W2.16e pulse subscription typed binding:** pulse_subscribe is now a typed
primary-only aggregate read operation with canonical StreamLive metadata, independent
Event emission schema and object terminal {}. All fourteen pulse native commands
now derive auth/read/schema policy from app operations: three unaudited reads and
eleven mandatory mutations. SubscribeRequest preserves presence-sensitive strict
string/list/numeric validation order, trimmed pattern/kinds/correlation, blank/empty
kinds as no filter, numeric truncation, negative default bounds and positive-only rate.
Selected SubscribeHost prepares transport only after decoding; bounded replay starts
no watcher, and ClientGone remains lazy after successful replay. Old decoder/business
wrapper and manual stream registry are removed. Shared dispatchAppOperation keeps
canonical auth/routing/schema/cancellation/emission/audit ownership; unary native
framing remains in handleAppOperation. Explicit pulse native projection retains
StreamNone dispatch classification to avoid a second connection reader, the existing
500ms timeout-tolerant watcher, event-only bounded/clean/live-write termination and
cause/error messages. It emits no RespResult; canonical {} is available to future
transports. Native unlimited replay still delegates cancellation to socket writes;
positive rate waits/live lifetime retain caller context. **Explicit refinement:** an
already canceled caller is rejected by shared admission before replay; old bounded
handler still emitted an event. Unchanged before proof fails/after passes count=20;
permanent real native wire regression covers it. No running-replay rewrite is claimed.
28 old/current complete native socket frame modes pass count=20 without normalization.
Permanent default/presence/strict validation order, typed output/emission/admission,
selected port/prepare/cleanup and missing-stream tests; native14 metadata/projection
incompatibility, bounded event+EOF/no audit, single watcher timeout/stray byte/stop,
socket-owned unlimited emitter cancellation and14-command actual tenant denial pass.
22 valid codec/hooks/schema/audit/projection/watcher/emitter mutations fail; source
bytes restored. One obsolete import after extraction was removed before verification.
Source/service/native Pulse+AppHost/CLI/journal repeats and related race count=20,
full controlplane race count=1/all Go gates pass;235 packages139 imports13 calls,
structure126. **Next: W2.16 exit evidence**, then autonomy/order4. Wider resident run
execution/trigger unification/module/generated surfaces/other transports and protected
local main delivery remain open.

**W2.16f pulse native exit:** exact14-operation primary aggregate policy is
pinned: three reads/eleven mandatory mutations, thirteen unary resident commands
and canonical StreamLive subscription with explicit native event-only/single-reader
projection. Independent resident/subscription omission mutations fail; exact e
production bytes restored. Exit/native/service/CLI/race/static/architecture/dependency
gates pass; full native Pulse+AppHost20/whole controlplane race1/all Go gates passed
e. Wave91 valid mutation failures: a18,b12,c16,d21,e22,f2. Generic no-audit-I/O
framework benchmark6796/6995/6662 ns/op (all<50us),7607/7609/7608 B/op,89 allocs;
excludes journal/socket/per-event or resident/provider latency.235 packages139 import/
13 call allowances,28 SDK findings+one test seam,24 dependencies,structure126.
[Full exit evidence](36-w216-exit-evidence.md) maps ownership, native parity/framing,
mandatory audit/no effects, actual tenant denial, replay/drop cleanup and explicit
already-canceled admission behavior. **Next: autonomy in order4.** Wider resident
run/trigger/module/generated surfaces/other transports/protected main delivery stay
open; no resident process/provider deployment is certified.

**W2.17a autonomy feed foundation:** selected Journal.Tail port and explicit
FeedInput/FeedOutput own the recent-history fold, newest-first milestone selection,
clamped1–200 limit and2000-event scan. Default60 remains native codec policy; present
float64 limits truncate, wrong types/null/string retain default. Empty items stay [],
count/seq/time/kind/subject/category/title/correlation remain required even zero/empty.
Doctor/metadata/detail/payload helpers and the complete original private test file
move verbatim to app/autonomy (package/constant homes only), with no other source
callers. Named delegation-only/reactive exclusion, doctor phase/mode titles/fallback,
raw optional enrichment strings, filtered-but-untrimmed chain entries, present zero
numeric metadata and rune-based120 clipping preserve actual behavior. Original Tail
cause returns zero output. Selected actual journal and port tests cover fold/order/
window/limit/empty/error/doctor projection/malformed/Unicode boundaries. Native
adapter retains legacy limit codec, primary ReadOnly registry and socket framing;
row maps deliberately stay the original projection during this move-before-binding.
193 full native response/journal-head/provider parity cases (all doctor phases/modes,
operator actions/milestones/payload shapes/limits/closed journal) pass count=20 with
no normalization; source native and original relocated helper tests pass count=20.
18 valid window/order/limit/count/empty/error/presence/enrichment/selection/Unicode/
detail mutations fail and restore exact source. Initial doctor-default and Unicode
boundary test gaps were strengthened; surviving mutants were not counted as defects.
Source/service/native/CLI/journal repeats and related race20, full controlplane race1/
all Go gates pass;236 packages139 import13 call allowances,structure127. **Next:
typed native binding and explicit row DTO/schema**, then autonomy feed exit/order5.
Wider autonomous execution/run/trigger/module/generated-surface/other-transport work
and protected local main delivery remain open; no resident/provider execution starts.

**W2.17b autonomy feed typed native binding:** one primary-only aggregate
ReadOnly unary app operation derives native registration/input-output types/schema
and unknown-input compatibility. FeedRequest owns the exact default60/numeric-only
limit codec; absent/null/string/bool/object/array stay default, float truncation and
1–200 service clamping remain. FeedItem replaces opaque row maps with23 named fields:
seven required identity/category/title/correlation fields keep zero/empty values;
detail/twelve doctor strings/chain omit only legacy-empty values. Independent *int
metadata preserves absent versus present0, negative/fraction truncation and row/field
independence. Derived schema requires identity/count and retains optional numeric
presence. Original helper/doctor/detail logic stays unchanged. Native wrapper and
manual registry are removed; selected primary journal composition/shared dispatcher
and response framing remain adapters.193 exact original/current full native ingress
response/journal-head/provider parity cases pass count=20 without normalization.
Permanent real native numeric/wrong-type/default/unknown/aggregate read/no-audit/
schema/canceled admission, canonical owned factory/Tail admission/cause/metadata and
actual tenant socket denial tests pass. **Explicit refinement:** already-canceled
operation admission now fails before feed lookup/Tail; the old wrapper returned a
feed result. Unchanged before proof fails/after passes20, with native/canonical
regressions. Direct Feed.List/in-progress non-context Tail behavior is not rewritten.
23 valid required-field/count/numeric presence-independence/raw string/limit/read/
auth-tenancy/unknown/registry/cancel mutation regressions fail; exact source restored.
Source/service/native/CLI/journal repeats and related race20, full controlplane race1/
all Go gates pass;236 packages139 imports13 calls,structure127. **Next: autonomy
feed native exit evidence**, then order5 tool/toolforge/toolbox/MCP/market/plugin.
Wider autonomous execution/trigger/run/module/generated surfaces/other transports
and protected local main delivery remain open.

**W2.17c native numeric precision refinement:** the shared app operation
terminal bridge decoded typed output into map[string]any through float64, so the
native wire rounded int64 sequence/time9007199254740993 to9007199254740992. An actual
native socket proof reproduces it. The bridge now uses Decoder.UseNumber while
retaining the existing object envelope, preserving exact numeric lexemes through
serialization. Unchanged after proof passes20 at2^53+1 and maxint64; its permanent
AppHost regression fails when UseNumber is removed. This explicit correction applies
to all migrated native object replies, not only autonomy rows. Output schemas, domain
folding, auth/read/tenant policy and input/client codecs stay the existing contracts;
generated/client numeric representation remains later work. Source/native AppHost/
autonomy/CLI/journal repeats and related race20, complete controlplane race1/full Go/
static/architecture/dependency/docs/structure/secrets gates pass.236 packages139
imports13 calls,structure127. Original source bytes restored after one valid numeric
mutation; a18/b23/c1=42 valid wave mutations so far. **Next: autonomy feed native
exit**, then order5. Wider autonomous execution/trigger/run/module/generated surfaces/
other transports and protected local main delivery remain open.

**W2.17d autonomy feed native exit:** exact primary aggregate ReadOnly unary
operation/typed23-field schema, native limit codec and unaudited journal fold are
pinned. Operation omission and output-schema weakening independently fail; exact c
production bytes restored.193 final native parity cases20 plus source/AppHost/native/
CLI/service/journal race20/static/architecture/dependency exit checks pass; whole
controlplane race1/full Go gates passed c after shared numeric bridge correction.
Wave44 valid mutations: a18,b23,c1,d2. Generic no-audit-I/O framework benchmark
7098/7304/10239 ns/op (all<50us),7607/7608/7608 B/op,89/89/89 allocs; excludes socket/journal/
feed rendering and row validation latency.236 packages139 imports13 calls,28 SDK
findings+one test seam,24 dependencies,structure127. [Full exit evidence](37-w217-exit-evidence.md) maps ownership, doctor rendering/limits/typed presence,
actual unaudited/tenant admission, pre-canceled refinement and exact-number native
bridge correction. **Next: tool/toolforge/toolbox/MCP/market/plugin in order5.** Wider
autonomous execution/trigger/run/module/generated-client surfaces/other transports
and protected local main delivery remain open.

**W2.18a tool inventory foundation:** app/tools Inventory binds the selected
Tools reader and typed empty input/six-string row/output contract. List reads each
definition once and invokes no tool/provider, preserving empty tools:[]/count0 and
required blank description/effect/rollback notes. Rows use Definition().Name for
presentation/name sorting; registration key selects the original primary capability
with file write/http POST/homeassistant call_service representative probes. Rollback
read-only/reversible/compensable/irreversible/empty/future mappings remain. Duplicate
advertised names retain both entries and the legacy unspecified equal-name tie order;
no deterministic tie or declared-capability-policy rewrite is claimed in this move.
PrimaryCapability is shared with existing agent permission rendering, removing its
controlplane-owned probe dependency. Probe/rollback bodies and all permission logic
remain unchanged except the shared helper call. Native tool_list wrapper/primary
ReadOnly registration/framing stay until typed binding; roster governance stays its
later domain.50 exact original/current native inventory cases plus192 policy-view
comparisons (six effects/eight registration axes/four profile policies) pass count20
without normalization, with actual owned kernels/Edict/definitions and zero tool/
provider/journal-head effects. Permanent name-vs-key, required wire fields/empty,
fresh reader/definition counts, rollback/probe and duplicate retention tests pass20.
20 valid probe/sort/key/count/read-once/empty/rollback/required-field mutations fail;
a mutation harness edit mistakenly discarded its first replacement, was corrected
and excluded as a compile failure. Source/native permission/capability/CLI/app/tools/
Edict repeats and related race20, whole controlplane race1/all Go gates pass.
236 packages139 import13 call allowances,structure127. **Next: tool_list typed binding,
tool log/stat services/binding and native exit**, then toolforge/toolbox/MCP/market/
plugin. Wider run/tool invocation/module/trigger/generated surfaces/other transports
and protected local main delivery remain open.

**W2.18b tool inventory typed native binding:** tool_list now derives native
registration, primary-only aggregate unaudited unary policy and Go input/output/
row schemas from app/tools InventoryOperations. Selected host kernel supplies the
existing definition reader; unknown args retain the native no-argument convention.
Six row strings and tools/count remain required even blank/zero; [] empty output,
definition-name sorting/registration-key representative capabilities/rollback notes,
shared permission rendering and legacy duplicate/tie/classification semantics stay.
The native wrapper and manual registration are removed.50 exact original/current
complete native ingress inventory responses and192 unchanged permission comparisons
pass20 without normalization, with zero tool/provider/journal-head effects. Typed
schema/no-audit/unknown/canceled/tenant canonical admission and actual native empty/
aggregate read/no-audit/schema/canceled/tenant-denial tests pass. **Explicit refinement:**
already-canceled operation admission now rejects before factory/reader/definition
lookup; the old wrapper returned inventory. Unchanged before proof fails/after passes20,
with permanent canonical reader/factory counters and real native wire regression.
Direct Inventory.List/in-progress definition reads retain their previous context
behavior.16 valid row/root required-field/read/auth-tenancy/unknown/registry/schema/
native-cancel/evidence-path mutation regressions fail; exact production bytes restored. The CLI compare gate caught the removed wrapper evidence path; it now points to
app/tools/inventory.go, and reverting the path fails the exact gate. Source/
native inventory/permission/capability/AppHost/CLI/app-tools/Edict repeats and related
race20, full controlplane race1/all Go gates pass;236 packages139 imports13 calls,
structure127. **Next: tool_log/tool_stats services and tenant binding, then tool native
exit** before remaining toolforge/toolbox/MCP/market/plugin order5. Wider invocation/
run/module/trigger/generated surfaces/other transports and protected main delivery
remain open; no actual tool invocation/provider starts in the new fixtures.

**W2.18c tool journal observation foundation:** selected journal Reader and
explicit LogInput/LogOutput/StatsInput/StatsOutput own tool log/stat folds. The
existing platform/journalview engine still owns timestamp-sequence ordering, strict
cursor/limit and decode-before-cutoff mechanics. Invoked inputs/times join by
(correlation_id,call_id), including events before the result window; repeated invokes
keep the actual legacy last-observed entry. Denied/no-invoke/backward-clock spans
stay zero; not_executed calls retain true-only log metadata and no execution latency
samples. Tool/errors/slow filters, required zero/false/null provenance/matches, preview
whitespace/100-rune+ellipsis, malformed-result unknown bucket, error-message buckets/
empty-message fallback and integer averages/nearest-rank percentiles remain.
Decoders/previews move verbatim and their controlplane copies are removed. Shared
pure duration summary/percentile helpers plus original tests move to platform/
journalview; run duration/spend, plan and tool stats now use the single owner with
no shim or algorithm change. Native strict codecs, numeric coercion/default20/cap1000,
clock cutoff, tenant kernel selection/read registry and response framing remain
adapters for the next binding. Row/by-tool/duration maps preserve legacy projection
shape during extraction; typed schemas follow separately.120 exact old/current native
log-stat input/filter/cursor/error/closed-journal responses pass20 without normalization.
Port tests cover scoped pairing/window/cursor/skip/backward clock/provenance/preview/
malformed/empty/cause/statistics/last-observed semantics; owned journal and actual
primary/acme/other socket reads prove tenant selection, cross-tenant denial and no
audit/provider/journal-head changes.26 valid fold/filter/key/latency/cursor/window/
provenance/cause/preview/summary mutations fail, restoring exact source. Source log/
stats/audit/cursor/run/plan/AppHost/CLI/app-tools/journalview/journal repeats and related
race20, whole controlplane race1/full Go gates pass;236 packages139 imports13 calls,
structure127. **Next: typed log/stat tenant binding and explicit output schemas**, then
three-operation tool native exit and remaining order5. Broader invocation/run/module/
trigger/generated surfaces/other transports/protected main delivery stay open.

**W2.18d tool observations typed tenant binding:** tool_log/tool_stats derive
native registration/read/OwnTenant/CallerTenant/unary metadata and input/output schemas
from app/tools operations. Together with primary aggregate tool_list, the exact three
read policies are pinned. ObservationRequest owns strict errors-bool/tool-string
validation order, preserved untrimmed tool filter, numeric-only slow/since coercion,
default20/truncated/clamped1–1000 limit, cursor fallback and a selected/injectable clock.
Stats still ignores errors/slow/limit/cursor args; unknown args remain compatible.
LogItem has14 required zero/false/null metadata/identity fields plus true-only optional
not_executed; ToolSummary requires calls/errors and uses *int64 optional avg_ms so
present0 differs from unmeasurable absence. Stats roots and six latency fields remain
required; duration embedding uses the canonical platform summary type. Row/duration/
by-tool maps are removed from app observations. ProjectValues<T> exposes typed rows
through the same private journalview engine; existing map Project keeps exact stamp-
after-cutoff semantics. Paging/order/decode-before-window/cursor math is single-owned,
with typed/map parity and original projection/source tests. Native codecs/business
wrappers and manual log/stat entries are removed; selected caller journal, shared
dispatcher and terminal framing remain adapters.120 exact original/current complete
native ingress log-stat cases pass20 without normalization; canonical schema/codec/
clock/no-audit/factory-reader admission, typed/map window-cursor/cause and actual
primary/acme/other socket isolation/no-audit/source tests pass. **Explicit refinement:**
both pre-canceled operations now reject before factory/journal/clock entry; old
wrappers returned results. Unchanged before proof fails both/after passes20, with
canonical counters and real native regressions. Direct service/in-progress Range
cancellation semantics are not rewritten.34 valid required-field/optional-average/
not-executed/read/auth/registry/unknown/clock/limit/numeric/tenant-factory/cancel/
projection/schema mutations fail; exact source restored. Staticcheck requested a
direct conversion for identical map-output structs; projection/app tests revalidate
it20 and remaining gates pass. An expired old Temp formatting helper was replaced
by the equivalent current tracked/untracked gofmt gate; setup failure is not a product
finding. Related/source/native/run/
plan/AppHost/CLI/app-tools/journalview/journal repeats/race20, whole controlplane race1/
all Go gates pass;236 packages139 imports13 calls,structure127. **Next: three-operation
tool native exit**, then toolforge/toolbox/MCP/market/plugin order5. Broader invocation/
run/module/trigger/generated surfaces/other transports/protected main delivery remain.

**W2.18e three tool reads native exit:** exact primary aggregate tool_list and
OwnTenant/CallerTenant tool_log/tool_stats unaudited unary specs own native registry,
codecs and typed schemas. Removing each operation independently fails the policy
coverage gate; exact d production bytes restored. Final50 inventory native cases192
permission comparisons and120 log/stat native cases pass20; source/AppHost/CLI and
related race20/static/architecture/dependency/structure/2730-file formatting pass.
99 valid wave mutations; d whole controlplane race1/all Go/build/vet and revalidated
static cleanup remain the full-source boundary. Shared duration summary and typed/map
projection have one owner; legacy codecs/filter/tuple/window/presence/last-observed
semantics stay. Already-canceled admission now blocks lookup for all three (unchanged
before/after proofs); direct/in-progress service reads stay. Generic no-audit-I/O
dispatch6318/6730/8051 ns/op (<50us),236 packages139 imports13 calls,structure127.
[Exit evidence](38-w218-exit-evidence.md) records ownership, compatibility and limits.
**Next: toolforge two reads/six lifecycle mutations**, then toolbox/MCP/market/plugin
order5. Full invocation/run/module/trigger/generated surfaces/other transports and
protected main delivery remain open.

**W2.19a toolforge read foundation:** selected List/Get port and ForgeCatalog
move list/show shaping into app/tools; ForgeToolView moves verbatim apart from name
and is shared by the six retained lifecycle adapters. Store order, [] empty tools,
required counts/fields, active-only callable_as, light code omission/full show code,
optional schema/test timestamp and old float64 view conversion remain unchanged.
Lookup trims ref but unknown errors preserve the raw ref. Native strict ref codec,
primary read registry/terminal framing and lifecycle business adapters remain for
later binding/mutation slices; direct service context behavior stays.152 exact
original/current full native ingress list/show cases pass20 across four owned stores
and19 argument modes, including zero/future/quarantined/active/large-number records;
no store/journal/provider effects. Port tests pin snapshot freshness/order/empty/
required-optional presence/full-light/error/lookup and legacy numeric behavior;
actual primary socket reads/spoofed tenant/tenant-denial/no-audit tests pass.19 valid
mutations fail and source restores exactly; two unused-import harness failures were
repaired and excluded. App/source/native/runtime/CLI repeats and related race20,
whole controlplane race1/all Go/build/vet/static/architecture/dependency/docs/structure/
format/secrets gates pass;236 packages139 imports13 calls,structure127. **Next: two
read typed native bindings and row schemas**, then six lifecycle mutation operations
and toolforge exit before toolbox/MCP/market/plugin. Broader invocation/module/trigger/
surfaces/other transports/protected main delivery remain open.

**W2.19b toolforge typed primary reads:** two primary aggregate unaudited unary
operations derive list/show native registry, request codec and output schemas.
ForgeShowRequest preserves missing/blank required and explicit null/wrong-type string
errors, unknown args, trimmed lookup and raw unknown-ref messages; invalid ref never
looks up the service. Native wrappers and manual list/show entries are removed; six
lifecycle wrappers remain. ForgeItem has11 named fields (eight required zero/false/
empty identity/status/timestamps and three optional input_schema/tested_ms/callable_as);
ForgeDetail adds required code, including empty string. Typed list/detail replace row
maps; one typed projection also serves six remaining lifecycle adapters. Light rows
never include code, optional tested_ms omits exactly0 and callable_as is active-only;
store order, [] empty tools and required count/active_count remain. **Explicit numeric
correction:** old JSON-to-map float64 projection rounded2^53+1/maxint64 timestamps;
new int64 view preserves all three timestamp fields. Actual native unchanged before
fails both reads/after passes20; source/wire regressions and precision mutations guard
it. Shared typed view corrects number rendering for retained lifecycle replies too;
client float decoding is later work. Final152 native cases20:131 byte-exact and21
with only asserted timestamp lexeme corrections; no other normalization. **Explicit
admission refinement:** pre-canceled reads now reject before factory/store; unchanged
before fails both/after passes20 and canonical factory/List/Get counters remain0.
Direct/in-progress non-context store reads remain unchanged. Canonical required-field/
optional/codec/unknown/no-audit/tenant-denial tests and actual primary/tenant socket
fixtures pass;28 valid schema/policy/codec/presence/precision/registry mutations fail,
source restored exactly. One unused-import mutation setup failure was repaired and
excluded. App/store/runtime/native/AppHost/CLI repeats/related race20/whole controlplane
race1/all Go/build/vet/static/full gates pass;236 packages139 imports13 calls,structure127.
**Next: six lifecycle mutation services and bindings**, then toolforge exit and
remaining order5; full invocation/module/trigger/surfaces/other transports/protected
main delivery remain open.

**W2.19c toolforge six lifecycle service foundations:** app/tools ForgeLifecycle
uses a selected six-method kernel writer port for draft/edit/test/promote/quarantine/
remove. Typed mutation/test/remove outputs share ForgeItem; kernel/store still owns
validation, protected identity, test verdict, tested-code promotion, persistence and
scripttool.* publication. The edit callback's tokens move unchanged apart from Tool
input qualification: only nonempty description/language/schema and nonempty code
replace stored fields, whitespace-only code is still forwarded to store validation. Raw refs/sample/
reason, error-first/found precedence, wrapped not-found conversion and backend causes
remain. Service entry rejects canceled contexts before any writer; direct service
host correlation wins explicit fallback and sandbox test inherits its exact context.
Native wrappers retain original strict raw codecs/manual mutation registry/audit,
empty lifecycle correlation and background sandbox context until typed binding;
no native admission/identity change is claimed at this foundation.117 original/current
complete native ingress error/absent-remove cases pass20 byte-exact with unchanged
store rows; successful source and existing native round-trip/test-first suites pass20.
Port tests cover all six methods/typed fields/cause-zero/context/correlation/raw args/
four-field edits/protected fields. Actual owned kernel/store/journal fixture proves
six ordered correlated lifecycle events, untested promotion refusal, runner input
fallback and code never actually executed; real native sample forwarding and tenant
denial preserve state/journal/provider effects.42 valid mutable/protected/identity/
raw/context/cancel/cause/verdict/presence/native-sample mutations fail; exact source
restored. One unused-sample mutation setup failure was repaired and excluded. App/
store/runtime/native/AppHost/CLI repeats/related race20/whole controlplane race1/all
Go/build/vet/static/full gates pass;236 packages139 imports13 calls,structure127.
**Next: six typed native lifecycle bindings**, including canonical audit identity and
caller context, then toolforge exit/remaining order5. Runtime publication semantics,
full invocation/module/trigger/surfaces/other transports/protected delivery remain open.

**W2.19d toolforge six typed native mutations:** all eight toolforge commands now
derive native registration/primary aggregate/unary/read-vs-mandatory-write policy and
schemas from app operations. Six raw typed request codecs preserve absent versus null
tool semantics, exact args.tool JSON error prefix, strict ref-first validation, raw
ref/sample/reason and optional absent versus explicit null/wrong-type strings. Unknown
args remain compatible. Typed mutation/test/remove outputs retain required tool/ok/
output/removed roots and light view fields. The237-line native business/codec/registry
file and manual registration call are removed; shared host/framing/audit remain the
adapter, kernel/store lifecycle ownership remains unchanged.117 exact original/current
native error/absent-remove cases pass20, with one audit pair per ingress and unchanged
store rows; source/native successful lifecycle/sample/tenant suites pass20.
**Explicit admission/identity/context corrections:** closed journal previously allowed
all six real store writes and sandbox test; canceled caller previously allowed all
six writes/audit effects. Native binding now rejects before service factory/writer/
runner entry. Lifecycle events formerly had empty correlation separate from their
operation audit; now exactly one op.invoked/domain/op.completed trio shares a host
identity and ignores caller-poison correlation/tenant args. Test formerly dropped
caller values/deadline; now its runner inherits values/deadline/operation identity.
Unchanged actual native before proof fails19 subcases (six audit failure,six canceled,
six event identity,one runner context); unchanged after passes20, with persistent
store-file/memory/journal/provider/runner assertions. A blocking owned runner confirms
post-admission deadline returns an error without changing the test record and emits
one invoked/failed pair. Real sandbox execution remains runner-owned, not a live test.
Canonical specs/types/schema/root-presence/codec-order/null/unknown/factory-context/
mandatory begin/end/tenant/pre- and post-begin cancellation/settlement cause tests pass.
Settlement errors propagate after a writer has acted; this does not promise rollback.
Official archcheck -update removes the paid controlplane-to-toolforge adapter bypass;
no allowance is added. Full Go/build/vet/static passed before this stale ratchet gate;
archcheck and remaining gates resume on the tightened138-import baseline.
31 valid binding/policy/codec/forwarding/context/root/registry mutations fail; exact
bytes restored. Two unused-variable mutation setup failures were repaired and excluded.
App/store/runtime/native/AppHost/CLI repeats/related race20/whole controlplane race1/
all Go/build/vet/static/full gates pass;236 packages138 imports13 calls,structure127.
120 valid toolforge wave mutations so far. **Next: eight-operation native exit**, then
toolbox/MCP/market/plugin order5. Runtime publication semantics, broader invocation/
module/trigger/surfaces/other transports/protected main delivery remain open.

**W2.19e eight-operation toolforge native exit:** two primary aggregate unaudited
reads and six mandatory audited unary writes own registry/codecs/typed schemas. Eight
independent operation omissions and one full-code schema weakening fail; exact d
production bytes restored. Final152 read cases20:131 byte-exact+21 timestamp-only
corrections; final119 byte-exact native error/absent-remove cases20 add whitespace-code
validation/rollback and retain one audit pair per ingress. Source/AppHost/CLI/related
race20/static/architecture/dependency/structure/2743-file formatting pass; d whole
controlplane race1/all Go/build/vet and official one-edge ratchet reduction remain
full-source evidence.129 valid wave mutations. Native audit/cancel gates, one owned
operation/domain/terminal identity and inherited runner context/deadline are anchored
by19 before failures/unchanged after20 plus blocking deadline/test-record proof.
Settlement errors do not imply rollback. Generic no-audit-I/O framework8587/8775/6950
ns/op (<50us),236 packages138 imports13 calls,structure127.
[Exit evidence](39-w219-exit-evidence.md) records ownership, compatibility and limits.
**Next: toolbox detect/outdated/install**, then MCP/market/plugin order5. Full invocation/
run/module/trigger/generated surfaces/other transports/protected delivery remain open.

**Protected delivery checkpoint after W2.19e:** the authenticated GitHub connection
reads PR #701 as open/mergeable, branch arch/w2-dispatch at the exact local HEAD
#ae063ff145b78991fec15c192e028cb3be713622 (main base e5379d802db6b5ccd67614382146064f6731ab51).
Its old-head CI run37357717652 is successful. A verified55-pair package contains188
live files and35 removals, excluding both unrelated security-report deletions;
normalized UTF-8 payload, file hashes and Git modes are retained in Temp. GitHub
create_tree was denied with “MCP tool call requires approval, but approval policy
is never”. No tree, commit or branch update was created. CLI HTTP401 is separate
from the working authenticated read connection; the write-approval gate and local
read-only Git metadata block this package's delivery. Later write permission must
publish the exact scoped snapshot, rerun final-head CI and use a normal protected
merge; the old-head CI is not evidence for unpublished changes. Local migration
continues with toolbox while this delivery boundary remains open.

**W2.20a toolbox read foundations:** selected ToolboxReader Detect/Outdated ports
and ToolboxReads move inventory pass-through and outdated-key folding into app/tools.
Core toolbox retains catalog, host lookups, bounded version/package-manager probes and
best-effort outcomes; native host adapter binds its existing functions. Inventory
keeps exact fields/optional strings/nil-versus-empty slices/counts/order. Outdated
keeps all map keys (including false values), raw names, [] empty output/required count,
fresh host snapshots and the legacy unspecified map iteration order. Caller context
and canceled direct-service best-effort behavior are preserved; no native admission
change is claimed. Native read registry/policy/no-arg compatibility/framing/map adapter
remain until typed binding. Streaming install and shared structToMap/mustJSONRaw users
in market/ACP remain separate scopes.22 byte-exact original/current complete native
ingress read cases pass20 in owned empty-PATH/working-directory fixtures, with no
journal/provider effects. Port probes cover populated/empty/optional/required/fresh/
raw-key/false-key/context/canceled snapshots; actual tenant socket denial preserves
journal/provider state.10 valid context/snapshot/key/count/empty/presence/cancel
mutations fail and exact source restores; an unused-name mutation setup failure was
repaired and excluded. Source/native/AppHost/CLI/related race20/whole controlplane
race1/all Go/build/vet/static/full gates pass;236 packages138 imports13 calls,structure127.
Controlled no-installed-tool fixtures and mock data do not certify live manager/probe
behavior or host installation. **Next: two typed native read bindings**, then install
service/streaming binding and three-operation toolbox exit before MCP/market/plugin.
Broader invocation/module/trigger/surfaces/other transports and protected delivery remain.

**W2.20b toolbox typed native reads:** detect/outdated derive native registry,
primary aggregate/unaudited/unary policy and input/output schemas from two app specs.
No-argument typed inputs keep unknown args; host inventory retains five required roots,
nine-field tool rows (five required/ four optional), nil-empty slices/counts/raw optional
strings and booleans. Outdated has required array/count, all-key membership including
false, raw names and legacy unspecified map order. Two native read wrappers and manual
entries are removed; selected default host reader and shared framing remain adapters.
Streaming install stays on its existing native path; generic JSON helpers remain for
market/ACP.22 byte-exact complete original/current ingress cases pass20 in owned empty
PATH/CWD fixtures, with unchanged journal/provider head/counters. Canonical specs/types/
schema/root-row required/optional/nil-empty/unknown/no-audit/factory-context/tenant tests
pass; actual tenant socket denial persists. **Explicit admission refinement:** the
old native wrappers returned results for both pre-canceled reads; shared app admission
now rejects before factory/host query. Unchanged actual before fails both/after passes20,
with canonical factory/host counters and permanent native regressions. Direct service/
already-running best-effort probe behavior remains unchanged.24 valid policy/context/
registry/required-optional schema mutations fail; exact source restored. Source/native/
AppHost/CLI/related race20/whole controlplane race1/all Go/build/vet/static/full gates
pass;236 packages138 imports13 calls,structure127.34 toolbox wave mutations so far.
Controlled no-installed-tool native fixtures plus populated mock snapshots do not
certify live manager/probe behavior or installation. **Next: streaming install service
and typed event binding**, then three-op toolbox exit before MCP/market/plugin; full
invocation/module/trigger/surfaces/other transports and protected delivery remain open.

**W2.20c toolbox install service foundation:** selected ToolboxInstaller plus
publisher/emitter callbacks move sequential install orchestration, outcome classes,
progress payload/envelope and typed terminal summary into app/tools. Core retains
catalog/manager resolution, bounded sandbox HelperEnv execution and version probing;
native host adapter binds existing Install. Empty input rejects before publication;
requested event precedes per-item context check, each attempt publishes outcome before
progress, and duplicate/raw requested names remain. OK wins over Skipped; summary uses
requested names while journal/progress keep reported Tool. All seven lifecycle payload
keys, per-tool InstallResult optional fields, zero transient event identity/sequence/
time, and installed/failed/skipped [] arrays remain. New service API stops later effects
on publisher/emitter errors and returns the original cause without partial summary;
an already attempted install is not rolled back. Native compatibility callbacks retain
best-effort publication/write behavior and empty domain identity until typed stream
binding; names codec/manual StreamEvents registration/framing remain. Pre-canceled
native calls still publish requested before failure, and cancellation during an attempt
still reports that outcome/progress before the next-item check.18 byte-exact full native
old/current frame-sequence cases20 use only asserted unknown catalog names in owned
empty PATH/CWD; no host manager install is reached. Port probes cover mixed success/
skip/failure/overlap/raw duplicates/order/context/cancel/failure boundaries, emitted
metadata/payload and typed arrays. Actual native unknown-tool progress/summary/journal
order, raw names codec and tenant-denial regressions pass20.27 valid outcome/order/
payload/context/array/error/native-codec mutations fail; two initially invisible port
fixtures (background context and equal reported/requested fail names) were strengthened,
not counted as product defects. Source/native/AppHost/CLI/related race20/whole
controlplane race1/all Go/build/vet/static/full gates pass;236 packages138 imports13 calls,
structure127.61 toolbox wave mutations so far. **Next: typed StreamEvents install
binding with canonical audit/identity/error handling**, then three-op exit/order5.
No real manager installation/live sandbox claim; wider invocation/module/trigger/
surfaces/other transports/protected delivery remain open.

**W2.20d toolbox typed StreamEvents install:** all three toolbox commands derive
native registry/primary aggregate/read-write/stream metadata and schemas from app.
Install has one mandatory audited StreamEvents spec with raw Names input, three
required terminal arrays and checked event.Event emission. Its names codec preserves
array-only/drop-nonstring-or-empty/raw-whitespace/duplicate semantics and exact empty
error; unknown args remain. Native install wrapper/manual entry/best-effort publisher
and old private codec are removed; original codec regression moves to the new raw
boundary. Host read/install adapters and shared market/ACP JSON helpers remain.
Kernel event.WireSchema now owns the former Pulse envelope literal byte-exactly;
Pulse consumes it. Install specializes that envelope with required payload and typed
InstallResult schema (required tool/ok plus optional fields), keeping transient event
format/zero metadata unchanged.15 normal original/current unknown-catalog native frame
cases pass20 byte-exact. The three former canceled parity cases are superseded by
an explicit no-publication admission correction rather than normalized into parity.
**Four explicit native corrections:** unavailable audit formerly yielded progress/
result; canceled admission formerly wrote requested/audit records; a broken stream
formerly processed all three unknown attempts; lifecycle records formerly had empty
correlation. Shared app now stops before factory/installer/requested publication on
admission failure, propagates emitter errors after the completed attempt and binds
requested/outcome/terminal events to one host operation identity. Four unchanged
actual native before tests fail/after pass20; broken writer gives one attempted
outcome/one failed terminal audit. A journal closed after the first progress now
stops later progress on publication failure and propagates the error; already applied
attempts are not rolled back. Real installation remains untested; native fixtures use
asserted unknown catalog names only. Canonical specs/types/payload/root schemas/codec/
unknown/context/mandatory begin/end/tenant/pre-post-begin cancellation/emitter failure/
settlement cause tests and actual native stream/tenant/source regressions pass20.
18 valid policy/codec/context/required summary-payload/registry/domain correlation/
publisher mutations fail in the final single-writer run; exact source restores and
host matches a reproducible binding snapshot. A missing empty-summary assertion was
strengthened. Two accidentally overlapping mutation rounds were discarded and rerun
serially; their outcomes are not certification.79 toolbox wave mutations so far.
Source/native/AppHost/CLI/related race20/Pulse-event race20/whole controlplane race1/
all Go/build/vet/static/full gates pass;236 packages138 imports13 calls,structure127.
**Next: three-operation toolbox native exit**, then MCP/market/plugin order5. Generic
HTTP/SSE surfaces, live managers, full invocation/module/trigger/surfaces/other transports
and protected main delivery remain open.

**W2.20e toolbox three-operation native exit:** two primary aggregate unaudited
reads and one mandatory audited StreamEvents install own registry/codecs/schemas.
Three independent operation omissions and one required-progress-payload weakening
fail; exact d production restored. Final22 read cases20 and15 normal install frame
cases20 remain byte-exact in owned empty-PATH/CWD/unknown-catalog fixtures. Source/
AppHost/CLI/related race20/Pulse-event race20/static/architecture/dependency/structure/
2757-file formatting pass; d whole controlplane race1/all Go/build/vet remain full-source
evidence.83 valid toolbox wave mutations; overlapped mutation rounds were discarded
and the final serial snapshot-verified run alone counts. Shared event envelope remains
byte-exact for Pulse; install specializes its typed payload. Native audit/cancel/
broken-stream/domain identity corrections have four unchanged before failures/after20;
post-admission publication failure stops later progress without rollback. Generic
no-audit-I/O framework9937/8613/9670 ns/op (<50us),236 packages138 imports13 calls,
structure127. [Exit evidence](40-w220-exit-evidence.md) records ownership and limits.
**Next: MCP one list read/five lifecycle operations**, then market/plugin order5.
Live host installation, HTTP/SSE surfaces, wider invocation/run/module/trigger/
generated surfaces/other transports and protected delivery remain open.

**W2.21a MCP catalog/redacted-view foundation:** selected registration List and
MCPAttached ports move list aggregation/shared registration view into app/tools.
The original view body moves verbatim apart from owner/port names, with no native
view shim; five retained lifecycle adapters use the same app view. Env/header values
never appear in output, only nonempty sorted key-name arrays. Raw optional command/
args/URL/description/lazy/tool_allow fields and required identity/enabled/zero time
fields retain their original JSON projection. Transport uses nonempty raw URL;
attached false is required and tool_count is present for live0/negative counts but
absent for detached rows. Legacy float64 timestamp conversion remains until typed
row binding/explicit precision proof. Store order, [] empty servers/count and initial
attached_count include orphan entries; one initial attachment snapshot plus one fresh
snapshot per row retains the original read cadence rather than introducing a coherent
snapshot rewrite. Direct/canceled best-effort read behavior remains. Native no-arg/
primary read registry/framing and lifecycle codecs/wrappers remain until binding.
54 byte-exact complete original/current native list cases pass20 across six actual
owned kernels/store/mock stdio-http peer states and nine arg shapes. Store/journal/
attachment/provider state stays fixed and reads never Call a remote tool. Port tests
pin redaction/key order/raw fields/required zero/optional/transport/live0-detached/
reader ownership/orphan/fresh-row cadence and legacy number behavior; actual tenant
socket denial preserves store/journal/provider state.16 valid redaction/key/transport/
status/count/fresh-view mutations fail; exact source restored. An unused-attachment
mutation setup failure was repaired and excluded. Source/native/runtime/mock-peer/
AppHost/CLI/related race20/whole controlplane race1/all Go/build/vet/static/full gates
pass;236 packages138 imports13 calls,structure127. **Next: typed native list binding
and redacted row schema**, then five lifecycle services/bindings/native exit before
market/plugin. No live peer/process/network/secret claim; broader invocation/module/
trigger/surfaces/other transports/protected delivery remain open.

**W2.21b MCP typed native list/redacted rows:** one primary aggregate unaudited
unary app spec derives native registry/empty input/output schema. MCPServerView names
16 public fields: seven required id/name/enabled/created_ms/updated_ms/transport/
attached, nine optional command/args/url/description/lazy/tool_allow/env_keys/header_keys/
tool_count. Env/header value fields are absent from the model and declaration. *int
ToolCount keeps live0/negative present versus detached absence. Raw optional strings,
empty collection omission and sorted nonempty key-name arrays remain; Args/ToolAllow/
key-name lists and count pointers are owned per view. List has three required roots,
[] empty servers/store order/initial orphan-inclusive attached count plus fresh per-row
attachment snapshots. Row maps and JSON-to-map projection are removed. All five retained
lifecycle adapters share the same typed view; native list wrapper/manual entry removed.
**Explicit precision correction:** old projection rounded2^53+1/maxint64 timestamps;
new int64 projection preserves native created_ms/updated_ms. Actual before fails/
unchanged after20 and permanent wire/source/ownership tests pin it.63 exact comparison
cases20:54 byte-exact normal store/mock-peer cases plus9 with only asserted timestamp
lexeme changes; no other normalization. **Explicit admission refinement:** old native
list returned results for a canceled caller; app now rejects before factory/registration/
attachment reads. Actual unchanged before fails/after20 plus canonical counters prove
it. Direct service/in-progress non-context attachment reads retain their behavior.
Canonical metadata/types/schema/three roots/seven row fields/optional/null/pointer/
unknown/noaudit/factory-context/tenant tests and actual tenant socket denial pass.
27 valid schema/policy/precision/collection-ownership/live0/registry mutations fail;
source restores exactly.43 MCP wave mutations so far. Source/native/mock peer/runtime/
AppHost/MCP CLI/related race20/whole controlplane race1/all Go/build/vet/static/full gates
pass;236 packages138 imports13 calls,structure127. Shared view corrects lifecycle
number rendering too; client float decoding is later work. **Next: five lifecycle
services/bindings**, then six-op native exit before market/plugin. No live peer/daemon/
process claim; wider invocation/module/trigger/surfaces/other transports/protected
main delivery remain open.

**W2.21c MCP lifecycle service foundation:** five writer methods and selected
attachment reader move add/attach/detach/set_enabled/remove into app/tools, with
explicit inputs and typed public server/attach/detach/remove outputs. Native codecs,
manual lifecycle registrations and legacy empty correlation/background attachment
context remain in this move-only slice. Add forwards the full registration (including
private transport values) only to the writer; public views contain only sorted env/
header key names. Attach forwards context/raw ref/tool names and nil/empty collection
shape; error identity is retained except the original wrapped ErrNotFound translation
for attach/set_enabled. Detach success stays true; absent remove stays false. Runtime
registration validation, durable writes, attach race/dial/close semantics and domain
publication remain unchanged. Port tests pin five writers, explicit correlation,
attach context, raw refs/enabled flags, full input, 2^53+1/maxint64 public timestamps,
live0 and error/no-view behavior.345 native response/state/attachment/close comparison
cases20 use two owned fixtures per case; only positively asserted runtime-assigned
IDs/wall-clock timestamps are normalized. Both stdio/HTTP peers are mocked; no process,
network peer or tool call is made.12 valid mutations detect lost input/correlation/
context/ref/tools/result/enable/error/view behavior; exact source restoration.
The first 5m proof budget expired after passing repetitions; its log is retained.
The unchanged 345-case/count20 proof passes with15m; no production/CI timeout changes.
55 MCP wave mutations so far. Source/native/runtime/MCP CLI/related race20/whole
controlplane race1/all Go/build/vet/static/full gates pass;236 packages138 imports13
calls,structure127. **Next: five typed lifecycle bindings with measured audit/cancel/
correlation/context refinements**, then MCP native exit and market/plugin. This does
not claim lifecycle cancellation/audit admission corrections or protected delivery;
client float decoding and wider invocation/module/trigger/surfaces remain open.

**W2.21d MCP typed native lifecycle bindings:** five primary aggregate mandatory-
audit unary app specs derive native registration and typed public outputs. Raw server/
ref/enabled codecs preserve missing/null/error precedence, raw refs and legacy enable
bool or case-insensitive true/exact1 string semantics (no trimming, other types false).
Add passes private env/header values only to the durable writer, never public views;
unknown arguments remain accepted. Attach nullable/empty/tool order and server/detached/
removed required roots retain their shapes. Five manual registrations/native wrappers
and mcp.go are deleted. **Explicit admission/identity/context refinements:** actual
old native proofs reproduce all five closed-journal effects, all five pre-canceled
effects, five empty domain correlations and background attach context; the same16
permanent cases pass20 after binding. App audit admission blocks writer/dial/close/store
changes; pre-canceled native calls do not begin audit or factory. All five direct service
methods precheck context and prefer operation-owned correlation over explicit fallback;
old-service cancellation/identity proofs fail and after20/mutations pass. Attach keeps
caller value/deadline/identity and a blocking mock dial deadline leaves attachment/store
unchanged with one failed audit pair. Remove's detach+remove events share audit identity.
Canonical metadata/type/schema/root/nullable-tools/50 codec cases/unknown/context/tenant/
20 admission cases/settlement tests pass20. Audit settlement failure after a writer
reports failure without pretending rollback; runtime domain publication behavior is
unchanged.345 native dispatcher response/state/attachment/close comparisons20 use owned
fixtures and mock both peer transports; only positively asserted assigned IDs/times are
normalized.31 valid policy/factory/cancel/correlation/context/codec/root/aggregation
mutations detect regressions; exact source restoration. An initial tools-root mutation
survived a weak assertion; required-root declaration checks were strengthened and the
complete final serial31 mutation run passes.86 MCP wave mutations total. Official
archcheck writer removes exactly controlplane→mcp adapter bypass, with no additions and
byte-identical call ratchet:236 packages137 imports13 calls; structure127. Source/native/
runtime/MCP CLI/related race20/whole controlplane race1/all Go/build/vet/static/full gates
pass. Exact original runtime Attach/Detach/RemoveAndAttachEnabled/Allowlist/Bridged
fixtures and the complete core MCP package also pass20/race20; name-based selectors
cover the source tests that the generic MCP regex missed (all-Go1 already covered them).
**Next: six-operation MCP native exit evidence**, then market/plugin. No real peer,
process, daemon deployment or client float-codec claim; broader transports/invocation/
modules/triggers/surfaces and protected main delivery remain open.

**W2.21e MCP six-operation native exit:** [evidence](41-w221-exit-evidence.md)
closes one primary aggregate unaudited unary list read and five primary mandatory-
audited unary lifecycle writes. Composite native coverage pins exact six input/output
signatures, single AppOwned registration, policies, unary/no-emission/unknown metadata;
each individual operation omission fails. Two private env/header schema additions fail
separately. Actual owned socket tenant denials for all six leave registrations/live
attachments/journal/provider/dial state unchanged.12 exit mutations;98 valid MCP wave
total (a16,b27,c12,d31,e12). Entire production Go path/hash manifest is identical to d;
two new native tests plus one direct CLI protocol test file add exit coverage. Earlier
generic MCP CLI selectors had no MCP tests; Tool/Compare checks were related only.
Exit now has10 CLI wire/alias/rendering cases and16 early invalid-argument cases,
count20/race20/full CLI1 through an owned canned TCP endpoint, not a real daemon/peer.
Four CLI protocol mutations plus six omission/two private-schema mutations give12 exit. App/native20/race20 and whole CP race1/CLI/
static/arch/dead/deps/official structure/2771-file format pass. D all-Go/build/vet/full
and exact runtime Attach/Detach/RemoveAndAttachEnabled/Allowlist/BridgedTool/core MCP20/
race20 remain accepted unchanged; final63 catalog and345 native lifecycle parity20
plus explicit unchanged before/after admission/precision/identity/context proofs
retain their actual d/b records without redundant full writers. Counts236 packages137
imports13 calls,28 SDK+1 test seam,24 deps,structure127. Generic no-I/O dispatch three
100ms runs remain below50us; this excludes audit/socket/lookup/dial/real-peer latency.
Docs/changelog/owned working-source and committed-range gitleaks/diff/index/frozen
patch chain pass. **Next: market/plugin order5**, plus broader runs.Start/invocation/
modules/triggers/surfaces/other transports. No live MCP peer/process/daemon/generated
SDK claim; protected final-head delivery/CI/main merge stays pending.

**W2.22a market read presentation foundation:** list/show/sources move into new
L4 app/market Reads behind one selected core-manager Reader port. Catalogue/installed
provenance/source ownership and core validation/library behavior stay in market.
Native adapters keep trimmed permissive strArg codecs and all eight manual policies;
three reads remain primary aggregate unaudited unary, five writes/streams are untouched.
Service preserves availability before required-name error, raw explicit input query/
marketplace values at its port, name trimming, read-error identity and old ignored
context/canceled-read behavior. List/sources preserve [] empty arrays, manager order,
required counts/rows, every optional core field and original JSON-to-map float64
projection. Show preserves ten roots, int64 installed_at versus projected numeric
fields, full pack manifest/base64 resource representation, raw full SKILL.md and MCP
name/tool order. Valid skill summaries use the first exact em-dash delimiter and keep
empty description; malformed summaries leave only skill_md. Tools retains null versus
[]; installed false/zero remain; informational VetPack, including danger, does not
reject read. Pack-manifest MCP transport values are unchanged in this move; this is
not the separate MCP-management redacted view. Row maps/legacy projection and manual
bindings remain for the next typed step. Shared CP structToMap/mustJSONRaw are still
used by market writes/ACP; strArg/parseCapList remain shared native helpers.
360 byte-exact native cases20 have no normalization: six owned unavailable/empty/
catalogue/installed/corrupt-install/corrupt-source fixtures × three reads × nineteen
normal argument cases, plus18 pre-canceled cases. Stores/journal/attachments/provider
state remain unchanged; no fetch/process/peer/tool call. Permanent actual native
availability/shape/no-effect and three tenant socket denials pass20. Port/read/source
contracts pass20;24 valid query/empty/count/projection/errors/name/summary/content/order/
null/install/vet/availability mutations fail and source restores exactly. An initial
substring compile fixture is excluded; anchored exact error returns plus independent
three-error/count axes pass in the final serial run. Core source tests, new app/native/
related race20/whole controlplane race1/all Go/build/vet/static/full gates pass:
237 packages137 imports13 calls,structure128,2774 formatted Go files. No allowance
added or core/stream behavior rewritten. **Next: typed three read models/operations
and native registration**, then five market writer/stream services/bindings and eight-
operation exit, then plugin. Wider invocation/runs.Start/modules/triggers/surfaces/
other transports and protected final-head/main delivery remain open.

**W2.22b market typed native reads:** three primary aggregate unaudited unary
app specs derive native list/show/sources registration, actual request/result types
and nested output schemas. Permissive raw query/name/marketplace fields retain
missing/null/nonstring→empty, trimming and unknown-field compatibility; unavailable
manager still precedes required name. List has two required roots and17-property/
eight-required embedded Listing rows; sources has two roots/four-property/two-required
Source rows. Show has ten roots,11-property/two-required full pack, typed review and
three-property/one-required skill view: successful summary name/description pointers
keep empty description present, malformed summary omits both but preserves full MD.
Byte resources/null/empty/tools/raw strings/optional fields/order/false/zero/default-
allow informational vet persist. Core Listing/Source/Pack remain full manifest models;
authored pack MCP values are unchanged, not the separate redacted management DTO.
**Explicit precision correction:** before-proof fails native downloads/added_ms/
signed_at/nested MCP created_ms/updated_ms for2^53+1/maxint64; installed_at control
already passed. The unchanged native proof passes20 with typed int64 fields/owned
manifest, preserving the installed_at control. Row maps/JSON-to-map projection are
removed. **Explicit admission refinement:** before returned results for three canceled
read callers; unchanged after20 rejects before factory/reader. Canonical counters
pin no audit/factory/port calls for pre-canceled and tenant callers. Direct service/
in-progress non-context manager reads keep their old behavior. **Ownership repair:**
actual old root tools output mutated reader's collection; typed field-access proof
and mutation now retain its ownership. Former nested projection ownership remains
without float conversion: cloned listing tags/full manifest tags/keywords/tool lists/
skills/resource maps+byte slices/MCP rows+args+allowlists+env+headers/signature; null
versus empty collections are retained.342 normal native cases20:219 byte-exact and123
with only asserted integer lexeme corrections. The18 former canceled parity cases
remain in the a record and are superseded by explicit admission proofs. Stores/
journal/attachments/provider state remain unchanged; no fetch/process/peer/tool call.
Canonical policy/types/schema/roots/conditional summary/byte resources/codec/availability/
context/noaudit/tenant and native before-after/metadata regressions pass20.45 valid
policy/factory/codec/clone/precision/required-root/registry mutations detect regressions;
exact source restoration. Initial proof-loop and unused-import mutation fixtures are
excluded, repaired and the final serial45 run alone counts.69 market wave mutations.
Three native wrappers/manual entries and host read helper are removed; selected
manager binding lives in app host. Five native write/stream handlers and shared
strArg/parseCapList/JSON helpers remain. Source/native/related race20/whole CP race1/
all Go/build/vet/static/full gates pass:237 packages137 imports13 calls,structure128/
2778 Go files. Client float decoding is later work. **Next: five market writer/stream
services and typed bindings**, then eight-op exit/plugin; wider invocation/runs.Start/
modules/triggers/surfaces/other transports/protected delivery remain open.

**W2.22c market writer/stream service foundation:** five selected Writer methods
and optional best-effort publication port move install/uninstall/add_source/
remove_source/sync orchestration into app/market Writes. Core keeps library validation,
signature verification, informational vet/default allow, skill creation/promotion,
MCP registration, provenance, optional reverse APIs, keep-last-good source sync and
network screening. Native codecs/manual five-command metadata and progress framing
remain: two StreamEvents plus three unary writes. Availability precedes required
name/URL. Install/uninstall trim the explicit name; other explicit service fields
keep their raw values while native strArg codecs trim/nonstring→empty. Install/
uninstall pass original caller correlation and void progress callback unchanged;
only Sync has a context-bearing core port and keeps the exact caller context.
Progress precedes terminal publication/result; writer errors can retain earlier
progress but suppress success publication. Install lifecycle payload uses reported
record name/version/marketplace/skill/MCP/tool lists/unsigned flag, not request name.
Uninstall publishes trimmed request name; source-add publishes reported source/URL;
source-remove publishes even absent=false and returns required name/removed roots.
Sync preserves [] empty result arrays, original row/float projection/order/zero fields,
pack sum and source count. Error with no rows fails without publication; nonempty
results plus error succeed with required results/synced/packs and optional partial_error
string plus completion publication. Install/source result projections and numeric
rounding remain in this move. Void progress/publisher failures, pre-canceled direct/
legacy native behavior and publication correlation are not yet refined. No rollback
or live transport/process guarantee is claimed. Shared CP framing/JSON helpers remain.
255 byte-exact native frame/store/materializer/fetch/domain comparisons20 require no
normalization:240 normal+15 canceled across unavailable/available/effect-error modes.
Owned market files reset only within checked fixture roots; skill/MCP materializers
and HTTP RoundTripper are mocked/in-memory, never real host tools or remote peers.
Permanent five actual tenant socket denials preserve store/journal/attachments/provider;
five unavailable failures retain their original audit pairs. Port/input/error/nil
callback/sequence/publication/partial-sync contracts pass20.35 valid writer argument/
callback/required/availability/publication/partial/empty/count/context/projection
mutations fail and exact source restores;104 market wave mutations so far. Source/
native/related race20/whole CP race1/all Go/build/vet/static/full gates pass:
237 packages137 imports13 calls,structure128/2781 Go files. No allowance/dependency
added. **Next: typed five writer/stream bindings with measured audit/cancel/identity/
emitter/publication refinements**, then eight-operation market exit/plugin. Wider
invocation/runs.Start/modules/triggers/surfaces/transports and protected final-head/
main delivery remain open.

**W2.22d market typed writer/stream binding and controlled core:** five primary
aggregate mandatory-audit app specs replace the native writer/frame adapters: install/
uninstall StreamEvents, add/remove_source/sync unary. Raw permissive codecs keep
trim/nonstring→empty/unknown behavior and unavailable-before-required precedence.
Typed InstalledPack/Source/uninstalled/removed+name/sync results retain required/
optional/null/zero/order/partial-error shapes; a pointer partial_error preserves an
empty error string when present. Transient progress keeps original kind/subject/
actor/envelope with typed core.Event payload requiring stage+ok, optional name/detail;
shared event.WireSchema is specialized without adding progress correlation fields.
**Explicit audit/cancel/identity refinements:** actual old17 native cases fail: five
closed-journal admissions, five pre-canceled callers (sync still fetched/audited),
five split audit/domain identities and two first-write failures. The unchanged17
proofs pass20 after binding. Admission stops before factory/manager/materializer/fetch;
all five direct service methods precheck context, install/uninstall choose host-owned
identity over explicit fallback, publication uses the same audit identity, and raw
caller correlation no longer controls materializers. Void callback core APIs retain
legacy behavior through Background/nil-error delegates; new InstallContext/
UninstallContext accept caller context and error-returning progress, checking before
subsequent effects/promotion/provenance and stopping on sink/caller failure. Core still
owns validation/signature/default-allow informational vet/provenance/reverse/sync.
First install vet-write failure prevents all materialization; first uninstall progress
failure retains its first quarantine effect but stops later MCP removal/provenance
removal. No rollback is claimed. Core pre-cancel/progress-cancel/effect-phase/nil sink/
old void behavior tests pass20. Domain+sink causes are joined; an implementation gap
was shown red and repaired before acceptance, with independent join mutations.
Publisher errors propagate after effect and join audit settlement; actual journal
closed after first install progress yields failure after materialization/provenance,
without pretending rollback. Partial sync semantics are retained. **Integer correction:**
typed installed_ms/added_ms/fetched_ms avoid former float projection; arrays/slices are
owned.240 normal frame/store/effect/fetch/domain parity cases20 allow only three
asserted integer lexeme corrections and independently asserted old/new correlation
normalization. Fifteen prior canceled parity cases stay recorded at c and are
superseded by explicit admission proofs; no other normalization. Owned checked-root
state, mocked materializers and memory HTTP only; no real peer/process/fetch/provider.
Canonical five metadata/signature/audit/tenant/context/emitter/schema/payload/root/
null/partial/settlement/ownership/native/core contracts pass20.41 final valid typed
mutations cover those boundaries;145 market wave total. Compile/unused-row fixtures
and a redundant context-guard survivor are excluded; entry-context disconnection
and effect-phase tests strengthen coverage, final serial41 alone counts. Sources
restore exactly. Native market.go, market registration and host helpers are deleted;
parseCapList moves verbatim to args.go; now-unused strArg/mustJSONRaw are removed.
ACP retains structToMap. Staticcheck caught the unused helpers and an unnamed test
context key after full Go/build/vet passed; cleanup uses a named key and scoped
market/ACP/arg20 plus build/vet/static and remaining gates pass without a redundant
full Go writer. Official arch writer removes CP→market only:237 packages136 imports13
calls,structure128/2785 Go files. Source/native/related race20/whole CP race1/all Go/
build/vet/static/full gates pass. Client float decoding/live atomicity remain later.
**Next: eight-operation market native exit evidence**, then plugin; broader
invocation/runs.Start/modules/triggers/surfaces/other transports/protected delivery
remain open.

**W2.22e market eight-operation native exit:** [evidence](42-w222-exit-evidence.md)
closes three primary aggregate unaudited unary reads and five primary mandatory
writers (two StreamEvents/three unary). Exact eight input/output signatures/single
AppOwned registry/read-write/stream/unknown/primary policy are pinned; eight independent
omissions fail. Existing actual owned tenant socket tests cover all eight denials.
**CLI stream repair:** direct wire proof failed because install/uninstall used Call
and aborted on first event. Both use Stream now, draining frames before the unchanged
terminal rendering/JSON.16 normal/alias/request/render cases,16 no-dial invalid inputs
and two terminal failures after progress pass20/race20 through a canned owned TCP
endpoint, not a daemon/store/peer. Reverting either unary call or dropping parsed
version/marketplace fails the test.12 final exit mutations,157 market wave total:
a24,b45,c35,d41,e12; unused-variable compile fixture excluded. Entire production Go
path/hash equals accepted exit state after mutation; relative to d only CLI stream
repair changes production. Two new exit test files. Full CLI/source/app/native20/
related race20/whole CP race1/all Go/build/vet/static (CLI included)/arch/dead/deps/
official structure/2787-file format gates pass. Frozen d17 native before/after proofs,
read342 and writer240 parity20, core phase/cause/legacy/no-rollback and typed schemas
retain their accepted records; no redundant huge parity writer.237 packages136
imports13 calls,28 SDK+1 seam,24 deps,structure128. Generic no-I/O dispatch3 runs
remain below50us, excluding audit/socket/lookup/materialization/sync/real service
latency. Docs/changelog/current owned-source and committed-range gitleaks/diff/index/
frozen patch chain pass. **Next: plugin inventory in order5**; wider invocation/
runs.Start/W3/W4/W5/transports and protected final-head CI/main merge remain open.
No live marketplace/signing authority/host install/MCP process/daemon restart claim.

**W2.23a plugin inventory service foundation:** daemon-supplied six-field
Registration/Reader port and inventory body move into new L4 app/plugins Service.
The selected native adapter translates runtime manifest fields without importing
runtime/agent into app, spawning/inspecting/invoking processes, validating pins or
changing allowlist enforcement. Rows remain maps in this move; six required fields
prefix/path/args/tool_count/hash_pinned/allowed_tools preserve raw strings, zero/
negative counts, false flags and collection order. Args always [] when nil/empty;
allowed_tools nil stays null versus explicit [] stays []; both arrays are copied.
Root plugins is [] empty with required count; original sort.Slice prefix comparator
and duplicate-prefix behavior remain, without sorting/mutating the daemon manifest.
Each call rereads the selected port. Manual primary aggregate unaudited unary native
registration/wrapper and old ignored-context/canceled-read behavior remain for the
next typed step.50 complete byte-exact native cases20 across five owned nil/empty/
zero/rich/duplicate14 manifests × nine ignored/input variants plus five pre-canceled
calls have no normalization; manifest/journal/provider state is unchanged. No plugin
binary starts or runs. Original native manifest tests and permanent actual tenant
socket denial pass20, along with port/field/array/copy/order/freshness tests.12 valid
empty/count/sort/field/pin/array/adapter mutations detect regressions; source restores
exactly. App/native/source/related race20/whole controlplane race1/all Go/build/vet/
static/full gates pass:238 packages136 imports13 calls,structure129/2790 Go files.
No allowance/dependency added. **Next: typed plugin row/output/schema and one app
operation/native registration**, then plugin native exit and remaining domain work.
Broader invocation/runs.Start/modules/triggers/surfaces/transports and protected
final-head CI/main delivery remain open; no live plugin deployment/pin verification
or permission-enforcement claim is made by inventory reads.

**W2.23b typed plugin inventory binding:** Row has six required fields and
ListOutput has two required roots. Args nil/empty still emits []; allowed_tools nil
emits null versus explicit [] emits []; arrays are copied, raw values/zero/negative
counts/false/order and original sort.Slice duplicate-prefix behavior are preserved.
One app/plugins operation owns primary authorization/tenancy, ReadOnly unaudited
unary metadata, input/output signatures and unknown-input compatibility. Native
plugin.go retains only the selected runtime manifest adapter; the socket/business
wrapper and manual registry entry are removed.45 normal complete native byte-exact
cases20 across five manifests and nine input variants pass without normalization.
Five legacy canceled parity cases are superseded by explicit admission evidence:
the unchanged actual native before-proof formerly returned a result; typed dispatch
rejects context.Canceled before the provider/read. Canonical counters pin no read,
factory or audit for pre-canceled/tenant requests. Direct Service.List and in-progress
reader cancellation semantics remain unchanged. Schema tests pin two roots/six row
required fields including zero/false/null; metadata/unknown input/caller context,
copy/order/freshness, native signatures and actual tenant socket denial pass20.
26 valid typed/schema/policy/array/field/adapter/registration mutations fail and exact
sources restore; a nonexistent enum in the initial mutation fixture caused a build
failure and is excluded. Final26 are runnable assertion failures.38 wave mutations
including foundation12. App/native20, related race20, whole controlplane race1,
all Go/build/vet/static/architecture/dead/dependencies/official structure pass:
238 packages136 imports13 calls,structure129/2793 Go files; no allowance/dependency
added. **Next: plugin one-operation native exit evidence**, then order6 config/
settings/configcenter/channels/webhook/tunnel/update. Wider invocation/runs.Start/
modules/triggers/generated surfaces/other transports and protected final-head CI/
main delivery remain open. No live process/pin/enforcement claim follows from reads.

**W2.23c plugin one-operation native exit:** [evidence](43-w223-exit-evidence.md)
closes primary aggregate plugin_list typed/AppOwned unaudited unary binding. Exact
canonical/native signatures/policy/registration and six-required-row/two-root schemas
are pinned. Native wrapper/manual entry are removed; selected manifest adapter remains.
45 normal byte-exact native cases20, actual canceled before-fail/after20, canonical
no-factory/read/audit admission and actual tenant denial preserve manifest/journal/
provider.44 wave valid mutations=foundation12+typed26+CLI6; source restores exactly.
New direct CLI owned TCP fixture covers table/JSON/empty/error and invalid/help8cases20/
race20, status probe then plugin_list nil-args/token, no daemon/process execution.
Initial fixture omitted the normal status probe and was corrected; no product finding.
Production source hashes exactly match b full Go/build/vet/controlplane race acceptance.
Exit entire CLI20/native20/new CLIrace20/scoped static/architecture/dead/dependencies/
official structure2794-file formatting/docs/changelog/secrets/diff gates pass.
238 packages136 imports13 calls,structure129; no allowance/dependency added.
No-I/O benchmark3:21284/20503/28130 ns/op (<50us); socket/audit/process I/O excluded.
**Next: order6 config/settings/configcenter/channels/webhook/tunnel/update**. Wider
invocation/agent-loop/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other
transports and protected final-head CI/main delivery remain open. Inventory evidence
does not verify live deployment, pin checking or permission enforcement.

**W2.24a config read presentation foundation:** order6 begins with one native
config read. New L4 app/config Reader projects selected live base/model/prompt-presence/
tool-count/plugin-count/ask-policy plus optional effective RoutingReader tables.
Environment callback exposes only bool presence; canonical configEnvVars list and
its daemon-wide inventory guard remain native. Service copies configured env names
and builds original map output: seven required roots, six resolved paths, env{} empty
or true-only listed keys (set-empty counts present), no values/system prompt content.
Routing root/submaps appear only for nonempty provider tables; map string values/raw
keys, array nil/empty->[], order and fresh snapshots are copied as before. Native
config_handler.go selects runtime fields and environment presence, then frames output;
manual primary aggregate ReadOnly unaudited unary registration and ignored args/old
context behavior remain for typed binding. Shared native stringSliceMapToAny stays
for chains/routing; app helper is private to this moved presentation, with no layer
bypass/dependency/allowance added.108 complete native byte-exact cases20 cover six
provider routing variants x nine ignored args x normal/pre-canceled calls, no
normalization.54 normal54 legacy canceled; journal/manifest/mock provider unchanged,
fixture env value/private prompt absent from wire. Original config paths/counts,
actual governor routing/absence, env/prompt privacy and daemon inventory tests pass20.
New port tests pin seven roots/six paths/zero-false/raw/count/read-order, listed env
ownership, routing presence/empty arrays/order/copies/freshness and legacy canceled
service behavior. Permanent native actual tenant denial/live model+prompt refresh/
set-empty presence/no-audit/provider contracts pass20.18 valid field/path/presence/
route/array/adapter mutations fail and exact source restores. App/native20/related
race20/whole controlplane race1/all Go/build/vet/static/full gates pass:239 packages
136 imports13 calls,structure130/2797 Go files. **Next: typed config output/schema
and one app operation/native binding**, then config exit and settings/configcenter/
remaining order6. Wider invocation/agent-loop/runs.Start/modules/raw-ref GC/triggers/
generated surfaces/other transports and protected final-head CI/main delivery remain
open. This inventory read does not prove live config reload or secret-store mutation.

**W2.24b typed config read binding:** ShowOutput has eight properties/seven
required roots; Paths has six required fields; optional Routing has three optional
maps. Env is a bool-valued map, created as {} empty with listed true-only presence;
set-empty remains present. Routing root/submaps remain omitted when empty, route/
require slice nil/empty becomes [] and raw keys/model strings/order/zero/false values
stay. Typed slice/maps are copied; reader/env-name ownership/live freshness/read
order remain. One primary aggregate ReadOnly unaudited unary app/config spec owns
input/output/schema, unknown-input compatibility and canonical native metadata.
config_handler.go retains only selected runtime/RoutingReader and bool env-presence
adapters; native wrapper/manual entry and private generic app map helper are removed.
Canonical configEnvVars inventory and shared native chains/routing helper remain.
54 normal complete native byte-exact cases20 across six routing providers/nine args
pass without normalization or journal/manifest/mock provider/content changes.54 old
canceled parity cases are superseded by measured admission refinement: unchanged
real native before-proof returned config; after binding it rejects context.Canceled
before read. Canonical counters pin no provider/reader/env/audit entry on pre-canceled
or tenant requests and same caller context on accepted calls. Direct Service.Show
and in-progress Reader cancellation semantics remain unchanged. Schema golden/field
omission tests pin root/path required and routing optional fields; non-bool env values
reject. Original config/source/governor/privacy/inventory plus typed raw/zero/copy/
array/order/freshness/context/native policy/live refresh/tenant/noaudit contracts pass20.
An initial routing schema fixture assumed [] for zero required fields; the writer
emits null, so fixture correction is excluded from product findings.The initial all-Go run reports three channel packages killed after an anomalous
~24458s elapsed under the unchanged10m timeout. Same-flag focused recheck passes
in ~1s per package; unchanged full all-Go rerun and remaining gates pass. No channel
code/count/timeout change is made or product defect claimed.31 valid required/
optional/policy/context/array/field/ownership/aggregate mutations fail and exact source
restores;49 config wave total including foundation18. App/native20/related race20/
whole controlplane race1/all Go/build/vet/static/full gates pass:239 packages136 imports
13 calls,structure130/2800 Go files. No allowance/dependency added. **Next: config
one-operation native exit evidence**, then five settings operations/nine configcenter
operations and remaining order6. Wider invocation/agent-loop/runs.Start/modules/raw-ref
GC/triggers/generated surfaces/other transports and protected final-head CI/main
delivery remain open; no live reload or secret-store mutation claim follows from reads.

**W2.24c config one-operation native exit:** [evidence](44-w224-exit-evidence.md)
closes typed primary aggregate config ReadOnly unaudited unary binding. Exact
canonical/native registration/signatures/policy, seven-required-root/six-required-
path/optional three-map routing and bool env schema are pinned.54 normal byte-exact
cases20, measured unchanged canceled before-fail/after20 and canonical no factory/
read/env/audit/tenant admission plus actual tenant/live refresh/privacy/source/
governor/env-inventory contracts remain.59 wave valid mutations=foundation18+
typed31+CLI10; exact source restoration. New direct CLI9cases20/race20 uses owned
canned TCP endpoint, status then config nil args/token; successful fixtures validate
canonical output schema. Table/JSON/default/empty/error/path-order/routing/raw/zero/
false/env sorted-key presence and invalid/help no-dial behavior are preserved.
All existing owned production/source hashes match b full Go/build/vet/whole CP race
checkpoint. Initial b channel timeout reports with anomalous elapsed were followed
by unchanged focused/full reruns, no source/count/timeout rewrite. Exit entire CLI20/
native20/new CLIrace20/static/ratchet/structure/2801-file formatting/docs/changelog/
secrets/diff gates pass. Strengthened schema-valid fixture repeats focused20/race20/
static/format/diff.239 packages136 imports13 calls,structure130; no new allowance/
dependency. No-I/O benchmark3:25677/26029/28134 ns/op (<50us), excluding env scans,
socket/audit/live provider work. Lost historical Temp patches are not fabricated:
W39e snapshot hash/base verification plus21 retained pairs and fresh cumulative
patch replay prove294 W44b files; current W44c delivery296 files is stored and verified
under ignored .temp_files/architecture-delivery. Original76-pair chain is incomplete.
**Next: five settings operations, then nine configcenter operations and remaining
order6.** Broader invocation/agent-loop/runs.Start/modules/raw-ref GC/triggers/generated
surfaces/other transports and protected final-head CI/main publication remain open.
No live config reload or secret-store mutation claim follows from inventory reads.

**W2.25a settings read presentation foundation:** two of five settings operations
begin order6 extraction. New L4 app/settings Reads owns original config_schema /
config_values map presentation behind SchemaSections/PrepareValues and ValuesReader
ports. Section aliases the unchanged lower settings declaration; canonical registry
merge/filter/source/locked/Apply normalization and ReloadBoundaries sorting remain
core-owned. Native selected server base directory (distinct from kernel base), pinned
map, fresh store/vault/registry preparation and original ignored read-load errors stay
in adapters. Schema keeps two roots, all required/optional core Section/Field wire
fields and sections nil/null versus [] where supplied; empty boundaries are []. Values
keeps one required fields root [] empty, order and raw names/values; env/secret/
env_pinned/set always present. Secret rows have four fields with bool vault presence
and no value; non-secret rows have five, live nonempty env before stored fallback,
set-empty env falls back and raw whitespace counts set. Reads never call value ports
for secret fields. Manual primary aggregate ReadOnly unaudited unary policy, ignored
args/legacy canceled calls and three original writers/helpers remain for subsequent
steps; no new dependency/layer allowance.144 complete native byte-exact cases20=
4 owned storage states x2 reads x9 input variants x2 contexts, no normalization;
72 normal72 pre-canceled. Server/kernel directories differ; missing/stored/corrupt/
broken-schema fixtures preserve files/journal/mock provider and hide secret marker.
Permanent native selected-root/live-env/store freshness/load-error/pin/privacy/noaudit/
provider and actual two-read tenant denial tests pass20. Port tests pin root/row/
core metadata/null-empty/zero-false/read-order/priority/freshness/canceled behavior;
initial field-count and nil-pin-map fixture errors are corrected and excluded from
product findings.17 valid schema/field/privacy/priority/pin/host-root/load-policy
mutations fail; sources restore exactly. Source app/core settings/native and selected
vault-load20, app/core race20/whole controlplane race1/all Go/build/vet/static/full
gates pass:240 packages136 imports13 calls,structure131/2806 Go files. **Next: three
settings writer/registry service foundations**, then typed five-operation binding/
explicit schemas/cancellation/audit/exit, nine configcenter operations and remaining
order6. Wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/
other transports and protected final-head CI/main delivery remain open. These reads
do not certify live reload, secret-store writes or durable writer audit/refinement.

**W2.25b settings writer service foundation:** app/settings Writes owns config_set
validation/trim/locked/secret-store selection, load/set-or-remove/save sequencing,
pinned admission and live/restart/reload presentation plus typed core-section register/
unregister orchestration. Selected native ports retain server-root registry/config/
vault construction, ignored legacy Set/Remove returns, process env Set/Unset and
kernel reload callback. Core settings validation, schema normalization/reservation/
locked-force/file persistence and vault merge/encryption remain unchanged. Provider/
model reload classification moves verbatim into app; native setLiveEnv I/O helper
remains. Native codecs preserve required/strict name/value/id/force errors, trim order,
section JSON round-trip and error texts; manual primary mutation registration/legacy
audit/background context remain. Set has env/saved/applied roots, true-only env_pinned
and reload_error presence even when message empty; reload failure remains successful
save + applied restart + reported error. Store/vault load/save errors keep exact
prefixes and original causes. Live secrets/lazy nonsecrets do not rebuild providers;
only provider/model live fields reload. Env-pinned edits save then return restart
without env/reload. Clear removes before Save; registry preserves raw section, trimmed
id, force and false removed.280 complete native cases20=4 owned storage modes x35
inputs across3 writers x2 contexts;140 normal140 legacy canceled, response/file/env/
reload effects byte-exact without normalization. Each audit pair is independently
asserted after connection completion (op.invoked + correct terminal, joined nonempty
identity/subject/actor/op, private value absent), not compared by random identity/time.
Kernel/server roots differ. Mock OnReload callback only; no provider/network, fixture
vault uses explicit owned plaintext opt-out for deterministic file comparison.
Port tests pin validation/read-only/locked before effects, exact load/save/cause/
partial/reload/pin/live/clear order and registry shape/cause/force. Permanent native
selected-root store/vault/register/locked-force/clear/pin and actual three-command
tenant denial tests pass20.24 valid validation/trim/clear/load/save/pin/reload/flag/
registry/host mutations fail and exact source restores;41 settings wave total.
Source app/core settings/native/selected vault-load20/app-core race20/whole CP race1/
all Go/build/vet/static/full gates pass:240 packages136 imports13 calls,structure131/
2811 Go files. No allowance/dependency added. **Next: typed five-operation settings
binding, explicit DTO/schema/input/error compatibility and measured cancellation/
mandatory audit refinements**, then settings exit, nine configcenter operations and
remaining order6. Legacy native beginOpAudit still ignores journal failures; this
foundation does not claim mandatory audit admission or caller-cancel refinement.
Wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other
transports and protected final-head CI/main delivery remain open; fake reload/effects
do not certify live daemon/provider rebuild or deployed config propagation.

**W2.25c typed five-operation settings binding:** two primary aggregate ReadOnly
unaudited unary specs (config_schema/config_values) and three mandatory audited unary
specs (config_set/schema_register/schema_unregister) own canonical native registration,
input/output/schema and unknown-input compatibility. Native five wrappers/manual
registrar are removed; selected server-root read/write/env/reload adapters remain.
SchemaOutput has two required roots and aliases unchanged core declarations/boundaries
(Section6props3required,Field10props6required,Boundary2props2required). Typed cloned
sections/fields/options preserve nil/null versus explicit [] and optional core fields.
ValueRow has five properties/four required env/secret/env_pinned/set with *string value:
secret rows omit value, nonsecret empty string stays present. Values root fields []
empty; order/raw/zero/false/env-before-store/pins/load policy are unchanged. Set has
five properties/three required env/saved/applied, true-only optional pin and *string
reload_error retaining present empty message. Register3 and Unregister2 required
roots preserve raw section/trimmed id/force/false removed. Private row/result maps are
removed. Strict RawMessage codecs retain missing/null/nonstring name/value/id/force,
validation order, omitted-value clear, section JSON decode errors/null-to-zero section
and raw metadata. Canonical audit starts before handler codec errors, with one failed
settlement; values remain redacted.8 actual unchanged native before-fail/after20 cases
prove three closed-journal writes and all five pre-canceled calls reject before effects.
Canonical counters pin no factory/read/write/env/audit entry on pre-canceled or tenant
requests, mandatory audit failure before writer factory and same caller context per op.
Direct service/in-progress core cancellation semantics remain unchanged. Three real
post-effect journal-close tests20 prove error terminal with applied effect/no rollback;
Windows journal wording required a fixture correction, not product changes.
72 normal read +140 normal writer native cases20 preserve complete response/state/env/
reload bytes without normalization; previous212 canceled parity cases are superseded.
Audit arcs independently verify identity/actor/subject/op/terminal/private redaction.
Initial schema parity exposed identical JSON values with changed nested member order:
legacy core Section/Field structs retain declared order through a narrow native terminal
codec projection; generic app business/registry remains shared. Permanent byte test and
mutation pin this codec. Source/port/schema required-optional/nil-empty/ownership/codec/
context/audit/selected-root/live/persistence/cause/tenant tests pass20. Context/force
mutation survivors exposed test coverage gaps, then per-operation/raw-id/force assertions
were strengthened; final38 valid mutations all fail and source restores exactly.
79 settings total including foundation17+24. Initial full Go gate exposed stale
compare evidence for removed settings.go; exactly one path was updated to the new
app/settings operation owner, preserving existing compare edits. Focused compare20
and actual full Go rerun/remaining gates pass; initial failed log remains. App/core/native/vault-load20/app-core
race20/whole CP race1/all Go/build/vet/static/full gates pass:240 packages136 imports
13 calls,structure131/2815 Go files. No dependency/allowance added. **Next: settings
five-operation native exit evidence**, then configcenter9 and remaining order6. Wider
invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other transports
and protected final-head CI/main delivery remain open. Mock reload/store fixtures and
inventory contracts do not certify deployed daemon/provider/config propagation.

**W2.25d settings five-operation native exit:** [evidence](45-w225-exit-evidence.md)
closes two primary unaudited reads/three mandatory unary writes. Exact five common
registrations/signatures/policies/no-emission and required/optional DTO/schema/codecs
are pinned. Wrappers/manual registrar are removed; selected server-root ports and
narrow legacy core schema JSON member-order codec remain.72 read+140 writer exact
normal native cases20, eight audit/cancel before-fail/after20, three post-effect journal
failure/no-rollback and source/port/core/tenant/live/pin/privacy/persistence/ownership/
codec contracts remain.91 wave valid mutations=17read+24writer+38typed+12CLI; final
assertion failures/exact restoration. New direct CLI38cases20/race20 cover21 normal/
error/checkpoint plus15 invalid/help+2 malformed files, schema-valid canned output,
status/token/nil-structured args, raw value joins/id/force, field rendering and owned
config.setting checkpoints. Secret checkpoint omits prior value/non-rollbackable;
checkpoint fetch failure prevents mutation, later set failure retains checkpoint.
Generic secret-render assertion was strengthened after a mutation survivor; fixture
prefix/setup corrections are not product findings. Existing source hashes match c
full Go/build/vet/whole CP race acceptance. Final entire CLI20/native20/new CLIrace20/
static/ratchet/structure/2816-file format/docs/changelog/secrets/diff pass.240 packages
136 imports13 calls,structure131; no added allowance/dependency. All five no-I/O
benchmarks3 are12333-23918ns/op (<50us), excluding real store/env/provider/audit/socket
work.317-file durable cumulative delivery replays HEAD exactly; index remains empty,
unrelated changes preserved and protected publication pending. **Next: configcenter
nine operations, then remaining order6**; wider invocation/runs.Start/modules/raw-ref
GC/triggers/generated surfaces/other transports/final-head CI/main delivery remain.
No live daemon/provider/config propagation claim follows from these fixtures.

**W2.26a configcenter read presentation foundation:** five reads (get/list/access-
log/audit/health) move to new L4 app/configcenter Reads behind selected manager Reader.
EntryMap moves verbatim, with a temporary native forwarder shared by three writer
echo paths. Secret-rated values use unchanged creds.MaskValue and true-only masked;
six required key/value/rating/created_at/updated_at/version fields, optional nonempty
description/tags/policy/allowed/excluded shape, raw values/zero timestamps/versions
and ACL-array copies remain. Tags keep original direct-slice semantics for this move.
Core manager still owns entry persistence/classification/vault/agent ACL/policy/HITL,
config.access audit/log filtering and stats. Native codecs validate strings first;
app preserves rating lower-case-without-trim, duration before availability, missing
get error translation, unavailable health2roots versus healthy3roots and checks map.
Get1root/list2roots/access2roots/audit2roots retain [] empty, order and exact raw zero/
empty metadata:8 required access row fields and9 audit row fields; int64 lexemes stay
exact. No new dependency/allowance/upward app import. Manual primary aggregate
ReadOnly unaudited unary registration and ignored args/legacy context remain; four
writer bodies compare byte-for-byte unchanged.550 complete native byte-exact cases20=
5 manager states x5 reads x11 input variants x2 contexts, no normalization;275 normal
275 canceled. Unavailable zero kernel/empty/public/secret/internal owned fixtures
preserve manager/audit files/journal/mock provider and suppress secret middle. Existing
native secret-mask/set-get-list and actual core agent allow/deny/access-log contracts
pass20; permanent actual five-read tenant denial preserves journal/provider. Port
required/optional/raw/mask/ACL copy/availability/error/filter/duration/freshness/canceled/
health/log tests pass20. Initial duplicate legacy method names and unused-variable/
import mutation fixtures are corrected/excluded from product findings; final20 valid
mask/field/copy/rating/filter/duration/log/health mutations fail and source restores.
App/core/native/MaskValue20/app-core race20/whole CP race1/all Go/build/vet/static/full
gates pass:241 packages136 imports13 calls,structure132/2821 Go files. **Next: four
configcenter writer service foundations**, then typed nine-operation binding/explicit
DTO/schema and measured audit/cancellation refinements/native exit before remaining
order6. Wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/
other transports and protected final-head CI/main delivery remain open. These read
fixtures do not certify deployed credentials, live external vault/HITL or config changes.

**W2.26b configcenter four-writer service foundation:** set/delete/set-rating/set-
access business moves to L4 app/configcenter Writes behind the selected core Center
Writer (Set/GetEntry/Delete/GetAutoRating). The latter is the existing exact
Classifier.Classify forwarder; no classifier/ACL/vault/persistence changes. Native
strict string validation and permissive list cleanup/manual primary mandatory audit
remain; list cleanup is pure and occurs at the native input boundary. The temporary
native entryToMap forwarder is removed: all read/write echoes now share app EntryMap.
Set preserves internal default, lower-case without trim, short invalid-rating error
before availability, raw key/value/description/ACL arrays, NewConfigEntry defaults,
Set followed by GetEntry and masked computed echo. SetRating keeps ParseRating's
longer invalid error before availability, missing-key translation before classify,
auto override boolean including false, and lookup mutation before Set. SetAccess
replaces ACLs including nil/empty, preserves other entry fields, and re-reads after
Set; Delete forwards raw keys/errors and returns deleted:true only after success.
Set/persist and post-write-read errors retain original causes and effects; lookup
failures retain legacy key-not-found translation. Direct service context remains
legacy ignored, including pre-canceled contexts; typed admission refinement is next.
368 old/current native handler parity cases20 (4 ops x23 args x4 unavailable/empty/
public/secret states) compare complete responses, sorted core entry snapshots and
actual entry/audit disk bytes after independently asserting created_at/updated_at
are within each operation's measured interval, then normalizing only those clocks.
Initial parity fixtures incorrectly flagged public echo text and unsorted map-backed
lists; corrected fixtures, retained failure log, no product defect attributed.
Permanent port/error/order/reread/mask/raw/ACL/override tests20, real selected-manager
persistence/reload/classifier override/context20, native tenant denial preserving
state/journal/provider, all four native audited success pairs plus failed lookup and
key/value redaction20. Existing core/native agent allow/deny/privacy remain green.
The existing loader resets loaded versions to1 via Store.Set; this extraction keeps
that behavior and the regression fixture explicitly pins it.17 final valid writer
mutations fail, source restores byte-for-byte. Full Go/race/build/vet/scoped static/
arch/dead/deps/gofmt/structure gates pass:241 packages136 imports13 calls,132 kernel
packages/2826 Go files. **Next: typed nine-operation configcenter DTO/schema/codecs,
app binding and measured cancellation/audit refinement**, then native exit/order6.
Wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other
transports and protected final-head CI/main delivery remain open. Owned local test
stores/mock provider do not certify live vault/HITL, daemon deployment or remote CI.

**W2.26c configcenter typed nine-operation native migration:** five read-only
unaudited and four mandatory-audited unary operations are PrimaryOnly/Primary with
unknown-field compatibility, typed per-command RawMessage requests and derived
terminal DTO schemas. Native handlers/helpers/registrar are deleted; selected core
Reader/Writer ports remain. Required/optional/null/non-string/blank validation order
and raw text remain, including short set/list versus long SetRating invalid-rating
errors. ACL codec preserves delimiters without CR splitting, whitespace trimming,
case-insensitive first-casing dedup, non-string element dropping, nil versus empty
and ignored irrelevant fields. EntryRow has12 properties/6 required; secret mask
true-only, optional nonempty fields, borrowed tags and copied ACL arrays retain wire
shape. Entry echoes1root/list2roots/access2roots/audit2roots/delete1/rating1 preserve
zero/false/empty fields, [] empty lists, row order, int64 lexemes and error causes.
Access rows8 required/Audit rows9 required; health3 properties/2 required permits
healthy stats null/empty/object while unavailable omits stats, preserving its2roots.
Canonical nine-op metadata/context/tenant/codec/schema/root/mask/large-int tests20,
actual native wide-integer/zero lexemes, primary signature/aggregate registration and tenant denial contracts
pass20. Unchanged13-case proof fails20 before and passes20 after: four closed-journal
writes and all nine pre-canceled native calls now reject before effects. Direct and
in-progress service cancellation retains legacy semantics; no rollback is introduced.
Four actual writer fixtures close journal after persistence, return terminal audit
failure and retain in-memory plus reloaded disk effects.896 old/current native cases20
(28 input variants x3 real states x9 ops plus unavailable manager x5 reads) compare
byte-exact response/state/disk after independently bounding and replacing only entry
clock fields; writer invocation/terminal audit pairs, actor/subject/correlation and
key/value redaction are independently asserted after connection completion. Reads
remain unaudited, provider zero. A zero-kernel writer's absent audit host is now also
rejected before manager availability; real initialized runtime fixtures retain error
order. Initial mutation survivor missing-get-key fixture is strengthened; final28
valid DTO/presentation/codec/policy mutations fail and source restores byte-for-byte.
Full Go/race/build/vet/scoped static/arch/dead/deps/gofmt/generated gates pass:241
packages136 imports13 calls,132 kernel packages/2829 Go files. **Next: configcenter
native exit measurement and CLI/exit evidence**, then remaining order6 domains.
Wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other
transports and protected final-head CI/main delivery remain open. Owned local/mock
fixtures do not prove deployed credentials, live external vault/HITL or remote CI.

**W2.26d configcenter native exit and CLI corrections:** all nine typed primary
unary commands have native exit evidence in architecture/46-w226-exit-evidence.md.
Five ReadOnly unaudited reads and four mandatory writers use canonical app dispatch;
selected core ports remain, native handlers/helpers/registrar are removed. DTO/schema,
validation/raw text/ACL cleanup/availability/lookup/reread/error/masking contracts,
core ACL/classifier/vault/persistence/access audit and direct/in-progress context
semantics remain. Previous550 read/368 writer foundation and896 typed native parity
cases20,13-case before/after native admission proof20, four post-effect journal-close
persisted-state tests20, exact native signatures/aggregate/tenant/privacy/numeric and
port/schema tests remain green. CLI proof11 cases fails20 before and passes20 after:
Unix-second entry/access/audit clocks no longer display1970, nonempty audit text uses
actual agent_id/decision/policy/reason instead of nonexistent actor/action, valid
access-log/audit filter values are consumed, and missing/adjacent values for those
flags/list/rating reject before dial. Flagged/positional ratings remain supported;
set business/body/default internal rating/raw value/access flags stay byte-unchanged.
68 new CLI cases20/race20 validate schema-conforming owned loopback replies, exact
status-probe/operation/token/args, text/JSON/empty/error branches, secret masking,
list order, override false/true and invalid/help no-dial. No real daemon/provider or
owner home is used.13 final CLI plus10 independent domain-operation/native aggregate
omission mutations fail, exact restoration; together with20 read17 writer28 typed,
88 final valid configcenter wave mutations. Initial encoding/nil-map and unused-value
mutation fixtures are corrected/excluded, retained logs do not establish defects.
Nine-operation full typed Dispatch benchmark with fake Center/noop audit, windows/
amd64/GOMAXPROCS4/200ms x3 ranges10300-33862 ns/op, each below50us excluding I/O.
Full CLI20/new CLI race20/app-core/native-source20/app-core race20/whole CP race1/all
Go/build/vet/static/arch/dead/deps/gofmt/generated/full gates pass:241 packages136
imports13 calls,132 kernel packages/2831 Go files. **Next: remaining order6 channel(s),
then webhook/tunnel/update**, measuring existing behavior before extracting services.
Broader invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other
transports and protected final-head CI/main publication remain open. Mocks/owned
stores prove local contracts, not deployed credentials/live vault/HITL or remote CI.

**W2.27a channel inventory presentation foundation:** channel_list map building
and verbatim account/kind probe/mode/media/probe totals move to new L4 app/channels
Inventory behind selected server-root InventoryReader/InventoryValues. Native snapshot
prepares config store then vault then registry, ignores the same read-load errors,
and exposes only vault presence; environment/store/vault names/sections/pinned and
process-global manifest/liveness reads stay native selected ports. The native wrapper
and manual primary ReadOnly unaudited unary binding remain; account writers/OAuth/
gateway/registry source bytes compare unchanged. No new dependency/allowance/upward
import. Four required result roots channels/count/probe_matrix/media_matrix; rows15
required keys/accounts5/account probe6/kind probe6, fields7 plus required non-secret
value retain raw metadata/zero/false/empty, nil versus [] setup steps and media structs.
Default account precedes sorted deduplicated valid stored/vault #labels; env-only
labels are not discovered. Default-only env_pinned, live env before stored non-secret,
secret env/vault presence with no value member, per-account/live-kind distinction,
configured-required env/vault/store precedence (including stored-only secret required
configuration versus false secret field set), status/mode/notes and total matrices
remain. Empty manifests/sections/fields and no-required-env configuration stay valid;
reads remain fresh, direct/canceled context and ignored request args retain legacy.
160 complete old/current native cases20=8 missing/stored/vault/corrupt/broken/env/live/
mixed states x10 ignored args x2 contexts,80 normal80 canceled, byte-exact without
normalization. Owned server/kernel roots differ; settings/vault/schema files/journal/
provider unchanged and secret middle never emitted. Permanent selected-root/presence,
actual native tenant-denial, lower account/probe/liveness/setting source, port/root/
field/raw/label/filter/priorities/matrix/freshness tests20 pass. Initial unused-import
parity fixture and section-kind coincidence mutation survivor are corrected; final20
valid secret/precedence/label/probe/field/media/total mutations fail, exact restoration.
Full app/channel/settings/native20/app-channel race20/whole CP race1/all Go/build/vet/
static/arch/dead/deps/gofmt/generated gates pass:242 packages136 imports13 calls,133
kernel packages/2836 Go files. **Next: two channel account writer service foundations**,
then remaining OAuth/gateway/inbox/send/ACP communication operations as applicable,
typed binding/DTO/schema and measured audit/cancellation refinement/native exit.
Other order6 webhook/tunnel/update, wider invocation/runs.Start/modules/raw-ref GC/
triggers/generated surfaces/other transports and protected final-head CI/main delivery
remain open. Registry/live flags and mock ports do not certify live channel sessions,
OAuth exchanges, external messaging/vault, daemon restart or remote CI.

**W2.27b channel account writer service foundation:** account set/remove business
and selected section-field lookup move to app/channels Accounts behind AccountWriter
and fresh AccountStore ports. Native strict argStrings order/manual primary mandatory
audit/legacy context remain; inventory/OAuth/gateway/registry bytes compare unchanged.
Set trims lower-case kind/label/name, preserves raw value until settings.Validate,
checks required pair/slug/cross-section/readonly before store effects, trims value
and applies SuffixEnv. Selected Registry.Sections field lookup and global manifest
lookup remain. Secret load/set-remove/save routes to selected vault; public routes
config. Ignored vault Set return and lower remove semantics remain, prefix text
load/save config/vault and legacy unwrapped causes remain. Success5 required roots
kind/label/env/saved/applied:restart; no process environment or kernel reload effect.
Remove requires kind/nonempty label (default cannot be removed), uses global
SectionEnvs rather than selected registered field schema, retains no slug validation,
ignores config then vault load errors, counts each config/vault removal, saves config
before vault and preserves partial effects/first failure. Success4 required roots
kind/label/removed/applied including zero. These existing policy differences are
explicit, not silently repaired in a move-only slice; registered-field removal can
be measured separately before refinement. Direct/pre-canceled service context stays
legacy ignored; typed admission/audit/cancellation refinement remains later work.
576 complete old/current native cases20=6 missing/stored/corrupt config/corrupt vault/
broken vault/broken config states x24 args x2 writers x2 contexts (288 canceled),
byte-exact response/owned config/vault/schema files without normalization. Distinct
kernel/server roots, dynamic/global section behavior, partial persistence/errors,
actor/op/subject/nonempty joined correlation/invocation/terminal kind and value
redaction independently asserted after connection completion; provider zero. Port
validation/field/trim/suffix/routing/clear/ignored loads/count/error-text/cause/save-order/
partial effects and real selected-root/vault-versus-config/process-env/tenant source
contracts20 pass.19 final valid mutations fail and source restores byte-for-byte.
An extra native test accidentally compiled during a deliberate mutation; its failure
is excluded as harness overlap, final source/native acceptance is rerun serially after
restoration. Initial all-Go build failed because Temp Go cache artifacts disappeared;
the same all-Go command and remaining gates are rerun with workspace-isolated cache,
without source/count/timeout changes and with both logs retained. Full app/channel/settings/native20/app-channel race20/whole CP race1/
all Go/build/vet/static/arch/dead/deps/gofmt/generated gates pass:242 packages136
imports13 calls,133 kernel packages/2841 Go files. **Next: remaining communication
operations (OAuth/gateway/inbox/send/ACP as applicable)** and measured account policy
refinement, typed binding/DTO/schema/audit/cancellation and native exit; then other
order6. Wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/
other transports and protected final-head CI/main delivery remain open. Owned stores/
mock providers do not certify deployed credentials/live OAuth/messaging/vault/daemon.

**W2.27c registered channel account removal repair:** the actual native
set->list->remove lifecycle proof fails20 before: remove reports0 while public config
and secret vault #work keys remain and inventory still lists the account. The same
permanent proof passes20 after one selected adapter repair: SectionEnvs now uses the
same merged/filtered server-root Registry.Sections as Set field lookup instead of
global builtin Schema. Registered public/secret keys delete/count correctly while
default/other accounts persist after independent reload. Native manifest override
is restored in test cleanup; no live channel/provider or owner home is used. Accounts
app business bytes remain unchanged, including validation, suffix, restart-only,
count per store, config-before-vault save/partial effects/errors and legacy context.
Permanent selected-root/foreign-root, filtered PATH/reserved builtin field, missing
section/fresh schema and every builtin section/global env-list equivalence tests20
pass. No new dependency/allowance/layer bypass; schema persistence/filtering stays
lower settings.5 final valid global/wrong-section/empty-field/wrong-env/wrong-root
mutations fail and source restores byte-for-byte. Native account/inventory/tenant/
source20, app/channel/settings20/race20/whole CP race1/all Go/build/vet/static/arch/
dead/deps/gofmt/generated/full gates pass:242 packages136 imports13 calls,133 kernel
packages/2843 Go files. Workspace caches remain isolated after prior Temp cache loss.
**Next: remaining communication operations**, starting with measured OAuth/gateway
state/port boundaries then inbox/send/ACP as applicable, typed DTO/schema/native
binding with measured audit/cancellation refinement and native exit; other order6,
wider invocation/runs.Start/modules/raw-ref GC/triggers/generated surfaces/other
transports and protected final-head CI/main delivery remain open. Mocks/owned stores
prove local contracts, not live channel/OAuth/messaging/vault/daemon deployment.

**W2.27d channel OAuth status snapshot repair:** before moving OAuth ownership,
owned in-memory status-writer/polling proof under race mode count3 reproduces data
races and mixed status/error pairs. Status previously retrieved a flow pointer under
mutex then read mutable fields outside it. A narrow repair now captures status/error/
kind/label together under the existing mutex, then releases it before response/socket
write. Known4roots versus unknown1root, raw fields/errors and ID/type wire remain.
The unchanged concurrent proof passes race20 after; permanent blocked-writer test
pins no mutex held across socket backpressure and immutable already-captured snapshot;
known/unknown/raw tests pass20. No OAuth exchange, token, provider, owner home or real
network is used in this proof. Start/callback/set-status/exchange bodies plus helper/
server state bytes compare unchanged. State generation/provider URL/TTL/prune/vault/
legacy context behavior is intentionally left for the separate OAuth move/refinement.
3 final valid unlocked-copy/wrong-error/wrong-kind mutations fail and source restores
byte-for-byte. Native channel/source20/status race20/app-channel/settings20/race20/
whole CP race1/all Go/build/vet/static/arch/dead/deps/gofmt/generated/full gates pass:
242 packages136 imports13 calls,133 kernel packages/2844 Go files. Workspace caches
remain isolated. **Next: channel OAuth start/callback/status service/state foundations**,
then gateway/inbox/send/ACP communication scopes as applicable, typed DTO/schema/
audit/cancellation/native binding/exit, other order6 and wider invocation/runs.Start/
modules/raw-ref GC/triggers/generated surfaces/other transports. Protected final-head
CI/main publication remains open; local fixtures do not prove live OAuth/messaging/
vault, daemon deployment or remote CI.

**W2.27e channel OAuth service/port foundation:** start/callback/status use-case
business moves to app/channels OAuth behind selected provider/nonce/clock/state/
exchange/vault ports. Nonce32-byte RawURLBase64, redirect validation and instance URL
normalization helpers move verbatim; existing source tests now call the app helpers,
with no native forwarding shim. Lenient native string codecs/manual primary unary
read/write policy remain. Server mutex/map, provider table, TTL15min/prune/store and
status-setter plus guarded HTTP exchange (native inner20s/form/headers/1MiB/read-error/
provider-error semantics) remain selected native ownership. Put prunes under existing
mutex, Flow snapshots all scalar fields under it, preserving the previous status race
repair/no lock retention on socket write. Callback carries caller context with outer
20s timeout, exchanges then selected-root vault Load/Set(legacy error ignored)/Save,
updates error/done status in the same order and preserves textual unwrapped errors.
Start validates provider/label/credentials/redirect/instance in original order, trims
same fields, stores pending flow/time and constructs encoded authorize URL/scopes.
Start2 required roots/callback5/status known4 versus unknown1 remain raw/zero/empty;
no token/client secret is emitted. Start/status legacy canceled-context behavior and
callback cancellation/error status persist; state retention/TTL/terminal policy is
left for separate state ownership/refinement, not silently changed in this move.
450 complete old/current native cases20=5 provider-response/vault states x15 input
variants x3 operations x2 contexts. Response bytes match after separately validating
and replacing random32-byte nonce (including authorize query); new flow clocks are
bounded then normalized in state snapshot. State/owned vault/schema bytes/fake HTTP
request counts match; independent writer audit pairs/actor/subject/joined correlation/
private redaction, unaudited status and model provider zero. Fake RoundTripper uses
no external network; real selected-root vault is owned plaintext opt-out fixture.
Port/order/trim/URL/query/flow/nonce/status/raw/error/cause/vault/context/deadline/
cancellation tests20, selected native state prune edge >TTL/copy/fresh status and
original source guarded exchange/cancellation/URL helpers20 pass.17 final valid app/
helper mutations fail/exact restoration; initial duplicate legacy function fixture
is corrected/excluded. Full status/native-source20/race20/app/channel/settings20/
race20/whole CP race1/all Go/build/vet/static/arch/dead/deps/gofmt/generated gates
pass:242 packages136 imports13 calls,133 kernel packages/2849 Go files. **Next: OAuth
state/provider/exchange ownership convergence** with preserved lifecycle, then other
communication gateway/inbox/send/ACP as applicable, typed DTO/schema/audit/cancel/
native binding/exit/order6/7 and wider roadmap. Protected final-head CI/main delivery
remains open. Owned fake HTTP/vault fixtures do not prove live OAuth/messaging/vault,
daemon deployment or remote CI.

**W2.27f channel OAuth state/provider/exchange ownership:** app/channels
OAuthMemory owns the provider table, per-host pending flows/mutex, value snapshots,
status updates and start-only pruning (>15min; equality retained). OAuthMemory.Exchange
owns the previous HTTP form/headers/inner20s timeout/1MiB body bound/provider-error
precedence and body/context cleanup. Native Server selects one memory/service using
sync.Once and binds the existing guarded HTTP client and server-root vault factories.
Native private flow/provider table/status/exchange/prune code and helper file are
removed; three native codec wrappers and app OAuth use-case business are byte-identical
to W2.27e. Scalar snapshots retain status-race protection without socket-held locks.
No implicit lifecycle policy change: read does not prune, missing status does not
create a flow, read errors remain ignored, nonempty tokens retain legacy HTTP-status
handling and callback vault persistence/order/textual errors remain.
450 old/current native cases20=5 response/vault states x15 args x3 operations x2
contexts pass, with independently validated nonce/query and bounded clock normalization,
matching response/state/owned files/fake HTTP counts; independent writer audit privacy,
unaudited status and provider-zero assertions. Owned fake HTTP only. App memory tests
cover exact provider defaults, TTL boundary, lookup retention, copied/fresh state,
owner isolation, concurrent status snapshots, exchange form/headers/deadlines/cleanup,
transport cause/provider error precedence/1MiB/read-error contracts. Native concurrent
32-call initialization/server isolation, paused writer/status snapshots and original
source guarded exchange/cancellation/helper tests pass20.20 valid ownership/exchange
mutations fail with exact source restoration. Full status/native-source20/race20,
app/channel/settings20/race20, whole CP race1, all Go/build/vet/scoped static/arch/dead/
deps/gofmt/official generated gates pass:242 packages136 imports13 calls,133 kernel
packages/2851 Go files. **Next: remaining communication service foundations** starting
with gateway status/QR, then inbox/send/ACP as applicable, typed DTO/schema/audit/cancel/
native binding/exit/order6/7 and wider roadmap. Protected final-head CI/main delivery
remains open; isolated fixtures do not certify live OAuth/messaging/vault/deployment.

**W2.27g channel gateway status/QR service foundation:** app/channels Gateway
owns normalization of URL, backend, session and supplied values. It selects
WAHA/Evolution endpoints and headers, bounded GET arguments and status/QR
presentation behind GatewayGET. Native wrappers
retain lenient typed string accessor and manual primary ReadOnly unaudited binding;
selected factory binds unchanged platform/netout.GatewayGET. Native HTTP forwarding
shim and duplicated use-case business are removed. Platform guarded local/LAN HTTP,
background10s timeout and partial-read semantics remain unchanged; canceled caller
contexts retain legacy behavior in this move. No automatic channel send occurs.
Status preserves early http(s)/host validation; QR leaves that validation to the
selected port/result boundary. Status response precedence status/state/instance,
case-sensitive WORKING versus case-insensitive open, malformed JSON best effort,
success2xx/non2xx http_status error maps remain. QR preserves image content-type
case sensitivity/raw base64, existing data URL/raw JSON base64, missing-image and
HTTP error results. Status1MiB/QR4MiB arguments and raw session path construction
remain. New permanent app contracts pass20;22 valid mutations fail/exact restoration.
480 complete old/current actual native dispatcher cases20=8 owned loopback HTTP
shapes x15 input variants x2 operations x2 contexts. Byte-exact wire with no
normalization and matching request URI/key/owned files; independently checked private
key exclusion, no operation audit/provider calls. Original platform/native source
tests and both primary-only tenant denials pass20. Full native/source20, app/channel/
settings20/race20/status race20/whole CP race1, all Go/build/vet/scoped static/arch/
dead/deps/gofmt/official generated gates pass:242 packages136 imports13 calls,
133 kernel packages/2854 Go files. **Next: inbox/send/ACP service foundations**,
then typed DTO/schema/audit/cancel/native binding/exit/order6/7 and wider roadmap.
Protected final-head CI/main publication remains open; owned loopback/fake ports
do not prove live gateway credentials/WhatsApp/messaging/daemon deployment.

**W2.27h channel inbox service foundation:** app/channels Inbox owns journal
folding, exported message/thread representations, default20/clamp1..1000, channel
normalization/filtering, newest timestamp/correlation descending sort and cursor
pagination/presentation behind InboxJournal.Range. Native typed accessors preserve
float64-only limit/truncation and ignored wrong limit/channel types, raw cursor and
delayed cursor codec error. Factory selects s.k.Journal; manual primary ReadOnly
unaudited binding and legacy background context remain. No store is introduced.
Inbound/outbound only, correlation-or-event-ID grouping, first nonempty channel
metadata, ordered messages and maximum timestamp remain. Malformed payloads retain
best-effort folding; empty threads stays [], sender omission/raw fields retained.
Total counts filtered threads before cursor/page; malformed/overflow/whitespace
cursor falls back, first-colon split retains correlation colons, same-time >=corr
boundary and optional next_cursor/channel remain. Range error preserves its cause
and wins over cursor type error after scan. Large int64 timestamps remain exact.
200 complete actual old/current native dispatcher cases20=5 owned journal states
x20 args x2 contexts: empty, grouped/no-correlation/ignored kind,1005 rows, malformed
payload shapes and owned corrupt segment. Byte-exact wire without normalization;
complete owned file hashes/journal head unchanged/provider zero independently
checked, including corrupt-range versus wrong-cursor precedence. App grouping/
field/order/filter/limit/cursor/large-int64/error/context/source contracts pass20;
selected kernel journal versus unrelated roots and tenant denial pass20.21 final
valid mutations fail/exact restoration; initial unused-variable mutation fixture
corrected and excluded. Full native/source20/status race20/app/channel/settings20/
race20/whole CP race1/all Go/build/vet/scoped static/arch/dead/deps/gofmt/generated
gates pass:242 packages136 imports13 calls,133 kernel packages/2858 Go files.
**Next: send and ACP service foundations**, then typed communication DTO/schema/
audit/cancel/binding/native exit/order6/7 and wider roadmap. Protected final-head
CI/main delivery remains open. Owned journal fixtures do not certify live channel
messages/daemon deployment or journal recovery behavior.

**W2.27i channel outbound send service foundation:** app/channels Outbound
owns channel/to/text trim, channel lowercase, required-field-before-availability
validation, selected Sender invocation, background30s timeout, exact error cause and
three-field result. Native factory resolves the current Server sender per request;
wrapper retains lenient string codecs/manual primary writer audit and socket framing.
Existing stringArg helper, ChannelSender injection/setter and registry bytes remain.
Measured before move: both success/error keep sender context live through a paused
terminal socket write, then cancel after completion (source20). Send uses a terminal
callback to retain that lifecycle and panic cleanup during this foundation; context/
typed operation binding refinements remain separate. Caller values/cancellation stay
ignored as in legacy native send; no live channels/external messages are invoked.
160 old/current actual native dispatcher cases20=4 selected sender states x20 args
x2 contexts: nil, success, error and contained panic. Complete byte-exact wire,
owned effect records/files match; independently bounded sender deadline/cancel
cleanup, correlated invoked/completed-or-failed audit/actor/subject/private argument
redaction/provider zero. Fake senders only. New permanent app validation/normalization/
deadline/error cause/callback lifetime/sender-or-terminal panic cleanup contracts20,
native paused-write lifetime/current sender replacement/nil/server isolation/tenant
no-effect denial and original source20 pass.15 valid mutations fail/exact restoration;
initial unused-import parity fixture corrected/excluded. Full native/source20/status
race20/app/channel/settings20/race20/whole CP race1/all Go/build/vet/scoped static/
arch/dead/deps/gofmt/official generated gates pass:242 packages136 imports13 calls,
133 kernel packages/2863 Go files. **Next: ACP inventory service foundation**, then
typed communication DTO/schema/audit/cancel/native binding/exit/order6/7 and wider
roadmap. Protected final-head CI/main delivery remains open. Owned fake sender/pipe/
file fixtures do not certify live channel delivery, credentials or daemon deployment.

**W2.27j channel ACP inventory service foundation:** app/channels ACPInventory
owns active-command environment selection/trim and ordinary cached discovery through
selected active getter/ACPDiscovery ports, with default catalog owner wiring. List
forwards caller context/cancellation and force=false, returns the complete typed
catalog inventory unchanged, and leaves registry/client failures in-band. Active
environment is read each call; authoritative inventory fields/counts/nil and nested
metadata remain. Native primary ReadOnly unaudited wrapper ignores args and retains
the existing structToMap terminal codec until separate typed binding; no ACP agent
loop/installation is invoked. CP no longer imports acpcatalog. Official archcheck
-update removes that one adapter-bypass exception:135 import exceptions/13 calls.
432 complete old/current actual native dispatcher cases20=6 owned registry/client/
cache/catalog states x4 active commands x9 ignored arg variants x2 contexts. Fresh,
cached, unavailable, malformed registry, stale failure and empty local catalog cases
retain byte-exact wire without normalization; fake source clocks are fixed via the
existing clients' Now ports. Request paths/cancellation/cached refresh policy match;
complete owned file hashes/journal head unchanged/provider zero independently checked.
Fake HTTP and no installed executable/agent launch in parity fixtures. New app full-
value/context/trim/ordinary-refresh/environment freshness/nil/count contracts20 and
original source discovery invoked under owned sources20; primary-only denial checks
no discovery requests/audit/provider.10 valid service/default wiring mutations fail
with exact restoration. Module/native source20/status race20/app/channel/catalog
race20/whole CP race1/all Go/build/vet/scoped static/arch/dead/deps/gofmt/official
generated gates pass:242 packages135 imports13 calls,133 kernel packages/2866 Go
files. **Next: typed communication DTO/schema/operation bindings and native exit**,
preserving source behavior or separating measured validation/cancel refinements,
then remaining order6/7 and wider roadmap. Protected final-head CI/main publication
remains open. Owned fixtures do not certify live catalogs/installed agent versions/
daemon deployment. Native legacy terminal codec precision remains separate work.

**W2.27k channel ACP typed operation/native binding:** ACPInventoryOperations
declares the single acp_agents GET /api/acp/agents spec: primary-only/primary tenancy,
ReadOnly/unaudited/unary, empty typed input with unknown args allowed, full typed
catalog output and generated schemas. CP registers that operation in the shared
dispatcher, removes the native ACP factory/wrapper file/manual row and final
structToMap float round-trip helper. ACP service business and Web UI read route
bytes are unchanged. Constructor rejects a nil service provider; field presence,
nested inventory types/in-band source failures and nil agents remain declared.
Two explicit binding effects: the shared pipeline now rejects an already-canceled
request before provider/discovery (old handler still returned an in-band inventory),
and native typed terminal UseNumber preserves declared large integer counts. The
captured old helper rounded9007199254740993 to9007199254740992 and int64 max to
9223372036854776000; this is a typed wire-contract measurement, not a claim that
real catalogs reach those sizes. Actual ACP native typed frames preserve both values.
432 actual old/current dispatcher cases20=6 owned source/cache/catalog states x4
active commands x9 ignored arg variants x2 contexts.216 normal cases match complete
wire bytes with no normalization/request paths/cache/owned files/head/provider zero;
216 canceled cases explicitly verify old inventory versus new context-canceled
error and zero HTTP requests. Complete owned files/head remain unchanged in both.
Nil-provider/metadata/auth/GET/schema/null/full output/unknown args/malformed roots/
service context/no-audit/non-operator/no-discovery/cancel tests20 pass, with original
source invoked under owned sources/tenant denial20. Captured old precision20 and
current typed ACP actual integer/cancel native tests20;12 valid spec/schema/native
mutations fail/exact restoration. Full module/native source20/status race20/app/
channel/catalog race20/whole CP race1/all Go/build/vet/scoped static/arch/dead/deps/
gofmt/official generated gates pass:242 packages135 imports13 calls,133 kernel
packages/2868 Go files. **Next: remaining channel inventory/account/OAuth/gateway/
inbox/send typed DTO/schema/operation binding and native exit**, with measured
validation/cancel/lifecycle refinements explicit, then order6/7/wider roadmap.
Protected final-head CI/main delivery remains open. Owned source/pipe/model fixtures
do not certify live catalogs/agents/channel messages or daemon deployment.

**W2.27l channel inventory typed DTO/operation/native binding:** ListOutput,
channel/account/field rows, account/kind probes and probe/media matrices are concrete
JSON-tagged models. Secret value pointers stay nil/omitted; public values retain
non-nil pointers even when empty. Empty channels/fields arrays, nil versus empty
setup steps, false fields/raw strings/default account/labels/configuration/liveness/
notes/counts retain their old wire shape. Probe/totals helpers use typed members.
One channel_list GET /api/channels primary-only/primary ReadOnly unaudited spec with
empty typed input/unknown args and fully nested generated output schema now binds
through shared dispatcher. Native wrapper file/manual row removed; selected reader
factory still uses Server root/store-vault-registry order/ignored loads and presence
ports. Web UI route and native reader bytes remain. The native object codec retains
MediaCaps declaration order as it already does core config-schema structs; initial
parity exposed only that member-order difference, corrected without normalization.
Explicit binding effect: already-canceled requests reject before provider/Prepare;
direct service still retains legacy context behavior.160 old/current native cases20
=8 owned store/vault/env/liveness states x10 ignored args x2 contexts.80 normal cases
compare complete raw response bytes with no normalization;80 canceled cases check
old inventory versus new context-canceled error. Selected server/kernel roots remain
distinct; owned files/journal head/provider unchanged and secret contents absent.
Original app map-shaped wire assertions are retained through a test-only JSON view,
with actual typed output mutation/freshness and typed native selected-root assertions.
Full nested schema/field presence/null/arrays/GET/auth/unknown args/no audit/canceled
reader/source/tenant/media-order tests20;27 valid DTO/privacy/logic/schema/native
mutations fail/exact restoration.
CLI compare evidence pointers to the deleted native wrapper are updated to app
inventory; the original all-Go failure is retained, Compare source20 and exact
all-Go command rerun pass after the narrow pointer repair. Full module/native
source20/status race20/app/channel/catalog race20/whole CP race1/all Go/build/vet/
scoped static/arch/dead/deps/
gofmt/official generated gates pass:242 packages135 imports13 calls,133 kernel
packages/2872 Go files. **Next: channel account/OAuth/gateway/inbox/send typed DTO/
schema/operation bindings and native exit**, with validation/cancel/lifecycle changes
measured explicitly, then remaining order6/7/wider roadmap. Protected final-head CI/
main delivery remains open. Owned source/store/pipe fixtures do not certify live
credentials/channels/messages/daemon deployment. Snapshots use full path names.

**W2.27m channel account typed DTO/operation/native binding:** SetAccountOutput
and RemoveAccountOutput are concrete JSON-tagged models; saved/restart, empty label
and zero removed counts remain present. Separate RawMessage request fields preserve
strict kind/label/name/value type admission order and exact native errors, including
null, with unknown input allowed. Two primary-only/primary POST writer specs own
existing account routes; native wrappers/manual rows removed. Selected Server-root
account ports, field/section selection, suffixes, trim/clear, config-vault routing,
ignored removal loads, textual errors, partial persistence and restart behavior
remain unchanged. Shared dispatcher now requires successful audit admission before
writer effects and rejects already-canceled requests before provider/audit; these
binding changes are explicit, while direct service retains legacy context behavior.
576 captured-current/native cases20 =6 store/vault states x24 args x2 operations x2
contexts:288 normal raw response/file bytes exact without normalization,288 canceled
requests assert no file/audit effects. Audit pairs/correlation/actor/privacy retain
normal semantics; selected roots differ and provider remains unused. Original
source/lifecycle/tenant tests plus typed schema/presence/validation/audit/route and
real closed-journal/canceled native tests20 pass.29 valid business/DTO/schema/binding
mutations fail, with exact source restoration; initial fixture actor/error identity
assumptions corrected and excluded. Full Go/source/race/build/vet/static/arch/dead/
deps/format/generated gates pass. **Next: OAuth/gateway/inbox/send typed bindings and
native exit**, then webhook/tunnel/update/order7 and wider W3-W5 roadmap. Protected
final-head CI/main delivery remains open. Owned fixtures do not certify live services.

**W2.27n channel OAuth typed DTO/operation/native binding:** start/callback/status
use concrete JSON-tagged results. Known status always contains error/kind/label,
including empty strings, through non-nil pointers; unknown status contains only
status. Request RawMessage codecs preserve lenient missing/non-string-to-empty
admission and unknown input. Three primary-only/primary specs use shared dispatcher:
POST start/status retain existing routes; internal callback declares no public HTTP
route, because the existing public Web UI GET page forwards with its own credential.
Start/callback writers require successful audit admission; status remains unaudited.
Already-canceled requests reject before provider/state/exchange/vault/audit effects.
Native wrappers/manual rows removed. Selected per-Server OAuthMemory/service/client/
vault factories, token-exchange form/headers/timeouts/bounds/error text, flow lifecycle,
restart outcome and Web UI route/page bytes remain unchanged. Native atomic snapshot
and paused socket/mutex-release assertions now pass through shared output binding.
450 captured-current/native cases20:225 normal complete wire/flow/file/exchange-count
matches and225 canceled/no-effect admissions. Only random32-byte nonce is replaced
after independent decode/query/state validation; new-flow clocks are bounded and
removed from comparisons. Normal audit pair/correlation/actor/privacy retained.
Source/native/schema/presence/auth/lenient input/real closed-journal/context tests20
and29 valid service/helper/DTO/schema/binding mutations pass with exact restoration.
Full Go/source/race/build/vet/static/arch/dead/deps/format/generated gates pass.
**Next: gateway/inbox/send typed bindings and channel native exit**, then remaining
order6/7 and W3-W5. No live credentials/channel messages/deployment certification.

Protected delivery update: PR #701 merged W2.10g-i through W2.27m at8416d9c8
on2026-10-07 after all24 exact407ef3c7-head CI jobs succeeded, including required CI
and ci.yml. Shared local main fast-forwarded to the same merge commit while preserving
local OAuth edits and unrelated security-report deletions. W2.27n is the next local
slice awaiting its own final-head CI/protected merge. Earlier restriction notes below
are historical evidence; current permissions allow normal Git/GitHub delivery.

**W2.27o channel gateway typed DTO/operation/native binding:** separate concrete
status/QR outputs retain optional result member presence through pointers. Successful
status contains connected:false and status:"" when appropriate; transport errors
omit http_status, HTTP errors contain it including zero, and QR/error fields retain
their prior shape. Two primary-only/primary ReadOnly POST specs bind shared dispatcher;
raw-field codecs preserve lenient missing/non-string-to-empty and unknown input.
Native wrappers/manual rows/wgArg removed; selected GatewayGET factory remains a
small native port file, with the same guarded LAN-capable/background-context HTTP
behavior, endpoint/header/bounds/body parsing and Web UI route bytes. Explicit
binding effect: already-canceled requests reject before provider/HTTP calls. In-flight
HTTP cancellation remains a separate refinement; direct service still ignores caller
context as before. Original map-shaped result assertions are retained via a test-only
JSON view; actual typed presence/freshness and full generated schemas are tested.
480 captured-current/native cases20:240 normal complete raw response/HTTP request
bytes exact without normalization,240 canceled/no HTTP calls. Owned loopback states,
files/journal/provider/privacy checked; no external network/credentials. Source/native/
schema/auth/lenient/presence/closed-journal/canceled tests20 and35 valid service/DTO/
schema/binding mutations pass with exact restoration; two compiler-only pointer
fixture attempts retained and excluded. Full Go/race/build/vet/static/arch/dead/deps/
format/generated gates pass. **Next: inbox/send typed bindings and channel native
exit**, then remaining order6/7 and W3-W5; own final-head CI/protected merge remains.

Protected delivery update: OAuth W2.27n merged in PR #702 at a00b580c on2026-10-07
after all24 exact19ca5ce1-head CI jobs succeeded (including CI/ci.yml); shared local
main fast-forwarded to the identical validated merge tree, preserving gateway edits
and unrelated deletions. W2.27o is the next local slice awaiting its own delivery.

**W2.27p channel inbox typed DTO/operation/native binding:** InboxOutput has typed
thread pointers/count/total and optional next_cursor/channel. Existing thread/message
models, empty arrays, sender omission, sorting/filtering/grouping/fallback IDs,
count-before-page and int64 timestamps remain. One primary-only/primary ReadOnly GET
spec binds shared dispatcher; raw codecs retain numeric truncation/ignored wrong
limit types, lenient channel input and delayed strict cursor errors. Journal Range
errors still win over cursor type errors. The native object codec retains nested
thread/message declaration order and exact int64s, like existing inventory/config
struct handling. Wrapper/manual row removed; selected kernel journal factory remains
in small native port file, independent of Server root. Already-canceled requests
reject before provider/Range. Direct service retains legacy context behavior.
200 legacy/current native cases20=5 owned journal states x20 args x2 contexts:
100 normal raw byte-exact responses/no normalization and100 changed canceled
admissions; complete owned files/head/provider unchanged. Permanent schema/nested
presence/freshness/JSON numeric limits/auth/native member-order/int64/range-error/
selected-journal/tenant/source tests20 and32 valid service/DTO/schema/binding mutations
pass with exact restoration. The old private direct-Go-map decoder test is replaced
with real JSON boundary tests: Go int/json.Number arguments are JSON numbers on the
wire, as in the original native Request decoder; this is measured, not a codec
compatibility exception. Full Go/race/build/vet/static/arch/dead/deps/format/generated
gates pass. **Next: send typed binding and channel native exit**, then remaining
order6/7 and W3-W5; own final-head CI/protected main merge remains.

Protected delivery update: gateway W2.27o merged via PR #703 at 5703f2d5 on2026-10-07
after all24 exact3cdf135a-head CI jobs succeeded (including CI/ci.yml). Shared local
main fast-forwarded to the identical validated merge tree, preserving local inbox
edits and unrelated deletions. W2.27p is the next local delivery. Owned fixtures
certify source/wire contracts, not live channel messages or deployed binaries.

**W2.27q send typed binding and channel native exit:** concrete three-field send
result and lenient raw requests use one primary POST writer operation/shared native
dispatcher. Native wrapper/manual row removed; current Server sender factory and
shared stringArg helper remain. stdlib-only opapi TerminalCleanup accepts optional
cleanup ownership, supplied by native terminal scope. Outbound transfers cancellation
only after sender returns; native scope releases after result/error socket write,
including failure/panic, LIFO outside its lock and exactly once. Direct callback
service keeps legacy lifetime and background30s/value isolation; sender panic cancels
immediately before opaque internal-error delivery. Naive return bridge failed the
original paused-write proof3; corrected shared binding passes20. Module/native
lifetime/write-failure/panic/ownership/reentry/concurrent tests20/race20,160 native
cases20=80 normal raw wire/effects/files exact including panic opacity+80 changed
canceled/no sender-audit effects,34 valid mutations/exact restoration pass. Mandatory
audit admission/canceled preflight precede sender factory/effects. Initial panic
assertion gap was strengthened and whole mutation list rerun. Full source/Go/race/
build/vet/static/arch/dead/deps/format/generated gates pass. Eleven channel operations
have unique AppOwned/primary/unary/typed signatures; empty registrar removed and
exit evidence recorded in47-w227-exit-evidence.md. No live channel messages sent.
**Next: webhook, tunnel and update in order6, then order7 and wider W3-W5.** In-flight
sender/gateway context refinement, lifecycle redesign and other transport convergence
remain open; own final-head CI/protected merge remains separate.

Protected delivery update: Inbox W2.27p merged via PR #704 at ca25f5c7 on2026-10-07
after all24 exact32a5975c-head CI jobs succeeded including CI/ci.yml. Shared main
fast-forwarded to identical validated tree preserving send edits/unrelated deletions.
W2.27q merged via PR #705 at c32e8852 on2026-10-08 after all24 exacteaa6f669-head CI jobs succeeded including CI/ci.yml. Local main identical-tree fast-forward preserved webhook edits and unrelated deletions.

**W2.28 webhook observability typed/native exit:** app/webhook owns delivery decode,
log shaping/filtering and stats, using selected routed journal and lower shared
journalview projection. Two ReadOnly/OwnTenant/CallerTenant unary specs replace
native handlers/manual rows. Strict optional failed boolean validation, lenient
numeric limit/since and ignored malformed cursor preserve actual legacy behavior;
count is returned page length. Status/error optional pointers retain zero/empty
presence; nested DTO schemas and exact int64 native output are independently tested.
Pre-canceled native proof red3 on both commands/after20. Actual three-journal tenant
socket isolation and source/schema/codec/race20 pass.240 native cases20=120 normal
complete raw wire/owned files/head exact+120 canceled/no effects,27 valid mutations
including both registration omissions/exact restoration; initial fixture Range-call
count6 corrected to5, excluded from product findings. Full source/Go/race/build/vet/
static/arch/dead/deps/format/generated gates pass; no allowance added. No-I/O dispatch
24.6–27.8us/op x3 (<50us). Evidence48-w228-exit-evidence.md records actual boundaries.
Web UI route bytes unchanged; stats has no invented HTTP route; no external webhook.
Own docs/payload/head CI/protected merge still must close. **Next: measure tunnel's
actual entry (no controlplane command exists), then update/order7/W3-W5.** Inbound
webhook async convergence, dispatcher supervision and trigger unification remain.

**W2.29a update service foundation/tunnel premise:** no native tunnel command exists;
boot target/URL adapter and layer5 supervisor already use platform/sandbox. Boot
helper20/full tunnel race20 prove current boundary; module lifecycle/exposure remains
later work. app/update now owns disabled/check/apply validation/presentation, raw
unverified manifest, background60s/no-deadline contexts and sentinel→callback→100ms
restart ordering. Native selects current Backend/current version/primary drain/
sentinel/delayed shutdown; callback codec and manual primary registrations remain.
Public concrete setter API/nil behavior preserved through canonical interface nil.
Naive return bridge red3 on early context cleanup and restart while socket blocked;
correct callback binding passes20, including failed write versus writer panic,
backend/reply panic and no added release provenance/signature.480 native cases20
preserve raw response/backend effects/manifest/sentinel presence in normal/canceled
contexts; sentinel clock and audit independently bounded/paired.19 valid service
mutations/exact restoration plus caller-context-removal mutation red3; typed-nil
fixture guard repaired and excluded from product findings. Lower HTTP cancellation
fixture10s sleep exhausted the local3m repeat budget; explicit owned-handler release
and context.Canceled assertion now pass20/race20 without reducing count/CI timeout.
Full source/Go/race/build/vet/static/arch/dead/deps/format/generated gates pass.
[Foundation evidence](49-w229-update-foundation-evidence.md). Check/apply results
remain transitional maps; typed operations/shared canceled admission/after-writer-
return ownership/native exit are next. Cleanup release alone cannot encode restart:
returned write error schedules, writer panic does not. No release download/swap/live
restart. W4.5 signing and order7/wider W3-W5 remain. Own protected delivery still open.

Webhook W2.28 delivered via PR #706 at 1f5d5c36 on2026-10-08 after all24 exactb83aef38
head CI jobs succeeded including CI/ci.yml. Shared main identical-tree fast-forward
preserved update edits and unrelated report deletions. W2.29a is next delivery.

**Historical state after W2.1a (before the pilot):**
- The control plane's `commandSpec` (`kernel/controlplane/dispatch.go`) already does authenticate →
  resolve → tenant authz → tenant routing → stream mode → **audit**.
- Every handler, though, writes straight to `net.Conn` (`s.writeResp(conn, …)`), so nothing outside the
  socket protocol can reuse them.
- The web UI proxies to ops through a fixed 198-entry route table, and REST/OpenAI/agentgw are separate
  hand-written surfaces.

**Do (roadmap W2.1, then W2.4…n):**
- Introduce `kernel/app` with `OpSpec{Name, Input type, Authz, Tenancy, Stream, ReadOnly}` and handlers of
  the shape `func(ctx, Input) (Output, error)`.
- Make the control plane an *adapter* that decodes `Request.Args` into `Input` and encodes the result.
- Pilot the domain `status`/`version` (read-only, low risk), then migrate domains in the order of
  roadmap §3, **one domain per PR, deleting the old handler in the same PR**.
- Benchmark dispatch overhead (budget < 50 µs/op excluding audit I/O).

**Invariants to keep:** `dispatch_registry_test.go` (registry ↔ protocol constants 1:1),
`tenant_auth_test.go`, `TestRegistry_TenantAllowedImpliesTenantRouted`, and the op-audit tests
(`dispatch_audit_test.go`). A new read-only op must be marked `ReadOnly` or it gets journaled on every
console poll.

### 4.6 Later (W3–W5)

See roadmap §2 W3–W5: dissolve `runtime.Kernel` into modules in increasing-coupling order, unify
triggers (W4.1, cadence system tasks through policy — owner-approved), channel supervisor and
conversation store, email DKIM/SPF, self-update signing, generated OpenAPI/SDKs, and allowlists → 0.

Cheap open findings you can fold in when you touch that area (register §9):
- seat `"container"` never maps to the container execution profile;
- `shell` `timeout_ms` is uncapped;
- a changed operator-profile facet text creates a second active record;
- artifact GC can delete blobs still referenced by a journal `raw_ref`;
- `reranktool` can panic on a short `scores` slice;
- the boot banner says 6 guardians, but 7 ship.

---

## 5. How to work here (procedures that worked)

### 5.1 Definition of done for one PR

1. **One concern.** Moves and rewrites are separate PRs, and a move uses type aliases so importers don't churn.
2. **A regression test, mutation-verified.** Show it red against the old code. Do that by restoring the
   file from a copy (`cp f $TEMP/f.bak; <break it>; go test …; cp $TEMP/f.bak f`). **Never use
   `git checkout -- f`**: it also wipes your uncommitted fix. Break each guarantee separately and list the
   mutations in the PR body.
3. **Run the source package's tests**, not only the new package's. An extraction once shipped red that way.
4. **Gates, all green** (exact commands in §5.2).
5. **Docs in the same PR:**
   - the codemap file for the area;
   - the roadmap row (✅ plus what was measured and what changed, or ⏸ deferred with the numbers);
   - the findings register row (✅ when fixed);
   - `CHANGELOG/unreleased/current.md` for anything a user or operator would notice.
6. **Commits:** the code commit first, then a separate `docs(arch): record <wave> — …` commit. The message
   explains *why*, names the measured defect, and says how the test was verified.
7. **PR body:** a "Stacked on #N" line if stacked, then *What was measured*, *Change*, *Verification*
   (tests + mutations + gates), *Docs*, and *Not in this PR*.
8. **Owner override (2026-10-03):** merge the validated stack into `main`, then work directly in the shared `main` checkout. Do not create another task branch.

### 5.2 Gate commands (run from repo root; Windows Git Bash works)

```sh
go build ./... && go vet ./...
GOMAXPROCS=4 go test ./... -count=1            # cap CPU: the owner's machine has 32 cores, don't peg them
git ls-files '*.go' | tr -d '\r' | xargs gofmt -l   # must print nothing (tr strips CRLF on Windows)
go run ./tools/archcheck          # ratchet; `-update` only to TIGHTEN after removing edges
go run ./tools/deadcodecheck
go run ./tools/depscheck
go run ./tools/docclaimscheck
go run ./tools/changelog-lint
go run ./tools/structure-md -check -out .project/STRUCTURE.generated   # after adding/renaming a package or doc.go
gitleaks detect --no-banner --redact -s . -b .gitleaks-baseline --log-opts="origin/main..HEAD"   # CI scans ALL commits
cd frontend && npm test && npm run build      # only when frontend/ is touched; npm, not pnpm
```

`make check` runs most of these. On Windows, `go vet ./...` catches copylocks that `go test` doesn't.

### 5.3 Historical: a fix that belonged to an earlier PR in the stack

The owner has retired the branch stack. The recipe below is retained as historical
context; subsequent work uses the shared `main`.

Commit it on the **earliest** branch that contains the bug, push, then cherry-pick the same commit onto
every later branch in order and push each one:

```sh
for b in <later branches in stack order>; do
  git checkout -q $b && git cherry-pick <sha> >/dev/null && git push -q origin $b || { echo "stop at $b"; break; }
done
```

Check that each branch's local head equals `origin/<branch>` before picking (the loop in the last
session silently did nothing the first time). Rewrite history (`reset --soft` + recommit +
`push --force-with-lease`) **only on your own unmerged PR branch**, and only when the bad content must
leave history, as with a gitleaks hit.

### 5.4 Reading CI logs

```sh
gh pr checks <n>
gh api --allow-escape-sequences repos/agezt/agezt/actions/jobs/<job-id>/logs | sed 's/\x1b\[[0-9;]*m//g' > $TEMP/job.log
```

`gh run view --log-failed` refuses while sibling jobs still run; the jobs API works.

### 5.5 Linux-only behaviour from Windows

The Ubuntu WSL distro has no Go toolchain. Cross-compile the test binary and run it there:

```sh
GOOS=linux go test -c -o $TEMP/x.test ./kernel/<pkg>
cd $TEMP && wsl.exe -d Ubuntu -e sh -c './x.test -test.run <Name>'
```

Tests that build helpers with `go` fail there, so filter them out with `-test.run`. Tests that need
`os.Args[0]` must use `os.Executable()`.

### 5.6 Measuring "which handlers do X without Y" (call graph)

A whole-program VTA call graph over `./cmd/agezt` takes about 6 s. The source of the tool that found the
W2.1a audit gap is in the appendix. Put it in a scratch directory outside the repo with its own `go.mod`
(`require golang.org/x/tools v0.47.0`), then `go mod tidy && go run . <repo-path>`.

**Cut traversal at the module boundary**, or `sync.Once` closures make every handler reach every sink.
It inspects handlers only. Since W2.1a, dispatch journals ops above the handler, so the tool still
reports those 54 handlers; that's expected. Treat the output as evidence, not truth: confirm the interesting cases with a runtime probe test, then
delete the probe.

---

## 6. Gotchas that cost time before

- **Nostr tamper fixtures must change the signature:** assigning the first byte
  to 00 can be a no-op (1/256 random signatures). Full validation reproduced the
  false failure; a deterministic scalar-23 fixture has that prefix. Both test
  corruptions now flip one decoded bit. The fixed fixture rejects a no-op mutation,
  the Nostr suite passes count=200 plus race/staticcheck, and full validation passes.
  Production verification is unchanged; retain the deterministic regression.

- **Stale documents:** security reports, `docs/*AUDIT*` and even this roadmap go stale in weeks.
  Re-verify against the source before acting. **Run** CI gates; don't trust a report about them.
- **The editor's LSP diagnostics are routinely stale** after branch switches and big edits. Trust
  `go build` / `go test`.
- **Bash heredocs swallow `\n` inside Python string literals**, which turns Go `"\n"` into a real newline
  and breaks the build. Use the Edit tool for Go strings that contain escapes.
- **CRLF:** strip `\r` from Windows-produced file lists before `xargs`. Git warns "LF will be replaced by
  CRLF" on docs; that's harmless.
- **Synthetic secrets in tests** (anything key-shaped) trip gitleaks and GitHub push protection. Build them
  at run time from parts (`strings.Join([]string{"abc", "def"}, "")`). Never click an "unblock secret" URL.
- **Package-global state in tests** must be fully reset in `t.Cleanup`. race-depth runs `-count=20`.
- **Socket responses precede deferred operation audit.** Linux stress exposed
  audit fixtures reading only `op.invoked`, and an export fixture comparing its
  snapshot head with a journal advanced by the run's later `op.completed`.
  Subscribe before the request and wait for the terminal audit before asserting
  a settled journal (`watchOpAudit` / `awaitOpAudit` in control-plane tests).
  Production response/audit ordering is unchanged.
- **A test that turns red after a security fix often pinned the vulnerable behaviour.** Rewrite it; don't
  revert the fix. Check sibling packages for the same pin.
- **"Test-only" is not "dead":** `deadcodecheck` runs without `-test`, so guards look unreachable. Ask what
  the test asserts before deleting.
- **New `AGEZT_*` env vars** read in `cmd/agezt` must be added to the control plane's `configEnvVars`
  (alphabetical), or a guard test fails.
- **New event kinds** go in `kernel/event/kinds.go` as constants and must be emitted or consumed in
  production code (`TestKindRegistryIsClosed`). Never `event.Kind("literal")`.
- **New control-plane ops:** register in the domain's `register…Commands`, add the protocol constant (the
  1:1 test), set `TenantAllowed` + `TenantRouted` only for tenant-safe ops, and set **`ReadOnly`** for reads.
- **A new way of running AS an agent** must call `runtime.WithAgentProfile` and layer explicit per-run flags
  on top. Never copy profile fields by hand (the W2.0 bug).
- **HTTP clients** come from `kernel/platform/netout`, child processes from `kernel/platform/sandbox`, store
  files from `kernel/platform/filestore`. archcheck's forbidden-call ratchet rejects bare
  `http.Client`/`exec.Command`/`os.WriteFile` elsewhere.
- **Query-string args proxied by the web UI are text:** numeric keys must be listed in `numericQueryArgs`
  (`kernel/webui/webui_proxy.go`).

---

## 7. Owner laws and decisions (don't relitigate)

- **Strategy:** strangler; layers under `kernel/`; storage stays JSON files + journal (no SQLite); dead
  `contract/gen` halves deleted; cadence system tasks go through policy (W4.1).
- **5.5′:** approvals are not persisted; a resumed run re-asks.
- **5.6:** a corrupt mid-journal record is quarantined and the daemon continues.
- **W1.8:** do not merge `settings` and configcenter; that's a product decision for the owner.
- **Default-allow:** capabilities are allowed by default; restriction is opt-out. Hard denies, SSRF guards,
  budgets and explicit HITL stay.
- **Rate limiting:** only the token-free `/hooks` path is throttled. Don't add throttles to authenticated
  run endpoints without asking.
- **`code_exec` is deliberately max-capability** (network on). Don't tighten it without asking; secret
  scrubbing, isolation and audit are non-negotiable.
- **No default provider or model** ships in the daemon. Models come only via routing/chains.
- **Boot resilience:** recoverable config mismatches warn and degrade; they never fail boot.
- **Never live-verify against the owner's real `~/.agezt`.** Use an isolated `AGEZT_HOME` in a temp dir.
- **The `.dev-home` console has a password.** Never fetch or enter it, and don't automate the browser
  against it. Verify UI via vitest, or ask the owner.
- **Ask the owner** (one question, recommended option first) only for real product decisions. Otherwise
  pick the conventional option, say so, and proceed.
- The owner writes in Turkish. Answer in Turkish when they write Turkish; code, commits, PRs and docs are
  English.

---

## Appendix — call-graph audit tool (`main.go`)

```go
package main

// Usage: go run . <path-to-agezt-repo>
// Lists control-plane handlers that reach a persistent write but never a journal publish.
import (
	"fmt"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

const mod = "github.com/agezt/agezt/"

func main() {
	pkgs, err := packages.Load(&packages.Config{Mode: packages.LoadAllSyntax, Dir: os.Args[1]}, "./cmd/agezt")
	if err != nil {
		panic(err)
	}
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	all := ssautil.AllFunctions(prog)
	cg := vta.CallGraph(all, cha.CallGraph(prog))
	cg.DeleteSyntheticNodes()

	pkgOf := func(f *ssa.Function) string {
		if f.Pkg == nil {
			return ""
		}
		return f.Pkg.Pkg.Path()
	}
	isPublish := func(f *ssa.Function) bool { // the "Y" — change these predicates per question
		p := pkgOf(f)
		return (p == mod+"kernel/bus" && strings.HasPrefix(f.Name(), "Publish")) ||
			(p == mod+"kernel/journal" && f.Name() == "Append")
	}
	isWrite := func(f *ssa.Function) bool { // the "X"
		switch p, n := pkgOf(f), f.Name(); p {
		case "os":
			return n == "WriteFile" || n == "Rename" || n == "Remove" || n == "RemoveAll" || n == "Create" || n == "MkdirAll"
		case mod + "kernel/platform/filestore":
			return n == "Save"
		}
		return false
	}

	var rows []string
	for fn := range all {
		if pkgOf(fn) != mod+"kernel/controlplane" || !strings.HasPrefix(fn.Name(), "handle") {
			continue
		}
		recv := fn.Signature.Recv()
		if recv == nil {
			continue
		}
		if pt, ok := recv.Type().(*types.Pointer); !ok || pt.Elem().(*types.Named).Obj().Name() != "Server" {
			continue
		}
		start := cg.Nodes[fn]
		if start == nil {
			continue
		}
		parent := map[*callgraph.Node]*callgraph.Node{start: nil}
		queue := []*callgraph.Node{start}
		var writes, pub bool
		var path string
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			if n != start {
				pub = pub || isPublish(n.Func)
				if !writes && isWrite(n.Func) {
					writes = true
					var p []string
					for c := n; c != nil; c = parent[c] {
						p = append([]string{c.Func.Name()}, p...)
					}
					path = strings.Join(p, " -> ")
				}
				if !strings.HasPrefix(pkgOf(n.Func), mod) {
					continue // cut at the module boundary: stdlib/third-party are sinks only
				}
			}
			for _, e := range n.Out {
				if _, ok := parent[e.Callee]; !ok {
					parent[e.Callee] = n
					queue = append(queue, e.Callee)
				}
			}
		}
		if writes && !pub {
			rows = append(rows, fn.Name()+"\n    "+path)
		}
	}
	sort.Strings(rows)
	fmt.Println(strings.Join(rows, "\n"))
	fmt.Println(len(rows), "handlers write without publishing")
}
```
