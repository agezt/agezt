# Phase 3.1 — runtime.go split (working plan, survey 2026-08-06)

runtime.go = 4,233 lines. Target: ~1,500 out via same-package splits + 1 cross-package
move + 4 tool moves. SKIP (doc overstated): mcptool/scripttool package moves (11/9
(k *Kernel) methods — kernel lifecycle wearing tool-file names; at most in-place file
splits), streamlimit→governor (66 lines, 1 consumer httpserver/sse.go, HTTP guardrail
not governance — wrong-direction import; correct target would be httpserver/streamlimit).

## Stage A — same-package file splits (zero risk, contiguous ranges)
A1 policy.go: 1561-1961 (validatedToolCaps, policyHook, agentNoisePolicyDenial + noise
   helpers, approvalBundle cluster). Touches k.{edict,approvals,state,toolCaps,tools,cfg}.
A2 runctx.go: 1963-2707 (ctxKey + 24 consts, value types, With*/fromCtx helpers).
   JUDGMENT: agentProfileSystem/profileTasksByScope/writeProfileTasks (2518-2597) go to
   prompt.go in A3 (persona text, not ctx plumbing); filterTools/applyAgentToolPolicy/
   agentToolPolicyDenial/applyAgentNoisePolicyToPromptTools → toolpolicy.go (tool-set
   shaping, not ctx). resume.go/subagent.go use unexported keys — same package, fine.
A3 prompt.go: 3758-4027 (8 pure injectors + shellHinter/defaultShellHint/shellGuidance/
   firstSentence) + the persona helpers from A2's judgment call.

## Stage B — promptBuilder method extraction (only drift-plausible stage)
B1: RunWith 3241-3409 → (k *Kernel) buildRunPrompt(runCtx, corr, actor, intent,
   systemAgent, skillDirective) (system string, activatedSkillIDs []string) in prompt.go.
   3441-3448 → separate (k *Kernel) injectHostEnvironment(system, tools) — do NOT merge
   across the buildLoopConfig boundary at 3437. Careful: intent rebind at 3213,
   activatedSkillIDs consumed at 3515, systemAgent gate on all five injectors.
   Guard rail: promptinjection_test + scoped + context_selection + skill/taste/memory/world
   tests. subagent.go's thinner duplicate injection: LEAVE (unifying = behavior change).

## Stage C — provenance → kernel/journal (cycle-clean)
Why (4114) / Causes (4161) / ParentOf (4205) are pure k.journal.Range scans →
kernel/journal/provenance.go as (j *Journal) methods; runtime keeps 3 one-line
delegators (controlplane untouched; error text not load-bearing — sole caller checks
err != nil). Move causation_internal_test assertions to journal + keep thin runtime
delegation test.

## Stage D — tool moves (one per commit, easiest→hardest, stop if ugly)
1. reranktool.go → kernel/reranktool (zero coupling, no back-patch). Alias in runtime:
   type Reranker = reranktool.Reranker (cmd/agezt names kernelruntime.Reranker).
2. imagetool.go → kernel/imagetool; export SaveArtifact field; back-patch becomes
   imgTool.SaveArtifact = k.artifacts.Put. Alias ImageGen.
3. voicetool.go → NEW kernel/voice; same SaveArtifact seam; alias Voice; k.Voice()
   stays (4 cmd/agezt call sites).
4. markettool.go → kernel/market (imports only mcp/netguard/skill — clean); export
   Manager field; markettool_test.go (package runtime) moves too.
Config.Voice/ImageGenerator/Reranker field TYPES unchanged (aliases only) — toolreg
imports runtime; changing exported surface ripples to toolreg + cmd/agezt.

## Registration/back-patch facts (current lines)
Construction 792-835 (effTools map + market/voice/image/rerank adds); back-patch block
892-908 (subTool.run/spawn, awaitTool.await, mktTool.manager, imgTool/vTool.saveArtifact).

## Test gate after EVERY stage (drift lesson)
go build ./... && go vet ./... && GOMAXPROCS=4 go test ./kernel/runtime/... 
./kernel/journal/... ./kernel/toolreg/ ./cmd/agezt/ -count=1
57 test files in runtime; internal ones (package runtime) are the drift tripwires.
