# NEXT — handoff for the next coding agent

> **Owner update, 2026-10-04:** continue directly on the shared `main`, without
> new task branches. PR #612 consolidates the original W0–W2.1a stack plus W2.2a.
> W2.2a, W2.2b, W2.3 and File Manager operation binding are complete;
> Catalog/provider native RPC binding and OAuth callback business extraction are complete. Continue §4.5 with callback HTTP presentation/adapter extraction, then context/lifecycle refinement; broader adapter/domain migration remains open.
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
