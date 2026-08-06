// SPDX-License-Identifier: MIT

// Policy enforcement for tool calls: the Edict-backed policy hook, the
// per-agent noise policy denials, and the approval decision bundle that
// enriches HITL prompts. Split from runtime.go (Phase 3.1 A1).

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/edict"
	intentmodel "github.com/agezt/agezt/kernel/intent"
)

// policyHook adapts the kernel's Edict engine to the agent.Policy
// signature the tool-loop expects. It is called once per ToolCall,
// before invocation.
//
// Three paths:
//
//  1. Hard-deny / unknown-cap / L0 / AskDeny → Allow=false, run skipped.
//  2. L4 Allow or AskAllow folded Ask → Allow=true.
//  3. AskPrompt landed on Ask-class → submit to approval.Registry and
//     block on the operator's decision. Grant flips Allow=true; deny /
//     timeout / cancel keep Allow=false with the verdict reason.
//
// The ctx passed in is the per-run context; cancellation (Halt) flows
// through to Submit and surfaces as DecisionCancel.
// validatedToolCaps keeps only declarations naming a capability Edict knows
// (M900) — a plugin may join an existing policy axis, never invent one.
func validatedToolCaps(declared map[string]string) map[string]edict.Capability {
	if len(declared) == 0 {
		return nil
	}
	out := make(map[string]edict.Capability, len(declared))
	for tool, cap := range declared {
		if edict.KnownCapability(cap) {
			out[tool] = edict.Capability(cap)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (k *Kernel) policyHook(ctx context.Context, tc agent.ToolCall) agent.PolicyVerdict {
	// Declared-capability overlay (M900): a plugin tool whose manifest joined
	// a known policy axis is classified there; everything else falls through
	// to the built-in name/input classification.
	cap, declared := k.toolCaps[tc.Name]
	if !declared {
		cap = edict.CapabilityForToolCall(tc.Name, tc.Input)
	}
	var out edict.Outcome
	if ceiling, ok := trustCeilingFromCtx(ctx); ok {
		out = k.edict.DecideWithCeiling(cap, string(tc.Input), ceiling) // SPEC-16 §4 initiative ceiling
	} else {
		out = k.edict.Decide(cap, string(tc.Input))
	}

	verdict := agent.PolicyVerdict{
		Allow:      out.Decision == edict.DecisionAllow,
		Capability: string(out.Capability),
		Reason:     out.Reason,
		WouldAsk:   out.WouldAsk,
		HardDenied: out.HardDenied,
	}
	def, _ := agent.PolicyToolDefFromContext(ctx)
	bundle := k.approvalDecisionBundle(tc.Name, out.Capability, tc.Input, def)
	verdict.EffectClass = bundle.EffectClass
	verdict.AffectedResources = append([]string(nil), bundle.AffectedResources...)
	ep := k.epistemicGate(tc.Name, out.Capability, tc.Input, def, bundle)
	verdict.EpistemicAction = ep.Action
	verdict.EpistemicReason = ep.Reason
	verdict.EpistemicSignals = append([]string(nil), ep.Signals...)
	verdict.EpistemicConfidence = ep.Confidence
	verdict.FailureMatches = ep.FailureMatches
	verdict.WeightedFailures = ep.WeightedFailures
	verdict.SchemaHash = ep.SchemaHash
	verdict.InputShape = ep.InputShape
	verdict.TemporalSensitive = ep.Temporal
	verdict.NovelTool = ep.NovelTool
	if reason, denied := agentToolPolicyDenial(agentToolPolicyFromCtx(ctx), tc.Name); denied {
		verdict.Allow = false
		verdict.Reason = reason
		verdict.WouldAsk = false
		verdict.HardDenied = true
		return verdict
	}
	if reason, denied := k.agentNoisePolicyDenial(ctx, tc); denied {
		verdict.Allow = false
		verdict.Reason = reason
		verdict.WouldAsk = false
		verdict.HardDenied = true
		return verdict
	}
	taint, hasTaint := agent.UntrustedObservationTaintFromContext(ctx)
	if hasTaint {
		verdict.UntrustedObservation = true
		verdict.ObservationSources = append([]string(nil), taint.Sources...)
		verdict.ObservationDirectiveLike = taint.DirectiveLike
		verdict.ObservationDirectiveMatches = append([]string(nil), taint.Matches...)
	}
	intentFrame, hasIntentFrame := intentmodel.FrameFromContext(ctx)
	intentAction := intentmodel.Action{
		ToolName:          tc.Name,
		Capability:        string(out.Capability),
		EffectClass:       bundle.EffectClass,
		Input:             string(tc.Input),
		AffectedResources: append([]string(nil), bundle.AffectedResources...),
	}
	regretAxes := intentmodel.RegretForAction(intentAction)
	confirmationPrompt := ""
	if hasIntentFrame {
		confirmationPrompt = intentmodel.ConfirmationPrompt(intentFrame, intentAction, regretAxes)
	}

	requiresApproval := out.RequiresApproval
	approvalReason := out.Reason
	if k.cfg.EpistemicEscalation && verdict.Allow && ep.escalates() {
		requiresApproval = true
		approvalReason = ep.Reason
		verdict.Allow = false
		verdict.WouldAsk = true
	}
	if k.cfg.IntentRegretGating && verdict.Allow && hasIntentFrame && intentmodel.RequiresConfirmation(intentFrame, regretAxes) {
		requiresApproval = true
		approvalReason = confirmationPrompt
		verdict.Allow = false
		verdict.WouldAsk = true
		k.publishIntentConfirmationRequired(correlationFromCtx(ctx), actorFromCtx(ctx), intentFrame, regretAxes, confirmationPrompt)
	}
	// Prompt-injection guard: an effectful action within the causal window of a
	// directive-like untrusted observation. The agent loop already scoped
	// taint.DirectiveLike to that window, so this no longer fires for the whole
	// run after one suspicious observation.
	if k.cfg.PromptInjectionGuard != PromptInjectionOff && verdict.Allow && hasTaint && taint.DirectiveLike && bundle.EffectClass != string(agent.EffectReadOnly) {
		// Block only in On mode and only when the operator hasn't trusted this
		// run; warn mode and a trusted run audit without interrupting.
		if k.cfg.PromptInjectionGuard == PromptInjectionOn && !trustedObservations(ctx) {
			requiresApproval = true
			approvalReason = "prompt-injection guard: effectful action is downstream of directive-like untrusted observation from " + strings.Join(taint.Sources, ", ")
			verdict.Allow = false
			verdict.WouldAsk = true
		} else {
			k.publishPromptInjectionWarned(correlationFromCtx(ctx), actorFromCtx(ctx), tc.Name, string(out.Capability), taint.Sources, trustedObservations(ctx))
		}
	}

	// Session-scoped operator grant (chat "auto-approve Tool Forge this session"):
	// if the run carries an auto-approve set covering this capability, satisfy the
	// approval without prompting and journal it as an auto-grant (WouldAsk stays
	// true so `agt why` shows it would have asked). Hard-denies never reach here
	// (they resolve to deny, not approval), so this can't override the F4 floor.
	if requiresApproval && autoApproveCap(ctx, string(out.Capability)) {
		verdict.Allow = true
		verdict.WouldAsk = true
		k.publishAutoApprove(correlationFromCtx(ctx), actorFromCtx(ctx), string(out.Capability), tc.Name)
		return verdict
	}

	if !requiresApproval {
		return verdict
	}

	// Live HITL: pause the tool-loop, route the request through the
	// approval queue, block until decided.
	actor := actorFromCtx(ctx)
	corr := correlationFromCtx(ctx)
	res := k.approvals.Submit(ctx, approval.SubmitSpec{
		Capability:            string(out.Capability),
		ToolName:              tc.Name,
		Input:                 string(tc.Input),
		Reason:                approvalReason,
		Actor:                 actor,
		CorrelationID:         corr,
		EffectClass:           bundle.EffectClass,
		PredictedEffects:      bundle.PredictedEffects,
		AffectedResources:     bundle.AffectedResources,
		RollbackNotes:         bundle.RollbackNotes,
		Confidence:            bundle.Confidence,
		CanonicalIntent:       intentFrame.CanonicalIntent,
		HarmfulInterpretation: intentFrame.HarmfulReading,
		AmbiguityScore:        intentFrame.AmbiguityScore,
		RegretAxes:            regretAxesPayload(regretAxes),
		ConfirmationPrompt:    confirmationPrompt,
	})
	switch res.Decision {
	case approval.DecisionGrant:
		verdict.Allow = true
		verdict.Reason = "approval granted by " + res.ResolvedBy
	default:
		verdict.Allow = false
		verdict.Reason = fmt.Sprintf("approval %s: %s", res.Decision, res.Reason)
	}
	return verdict
}

func (k *Kernel) agentNoisePolicyDenial(ctx context.Context, tc agent.ToolCall) (string, bool) {
	policy, ok := agentNoisePolicyFromCtx(ctx)
	if !ok {
		return "", false
	}
	if tc.Name == "memory" && policy.disableMemoryWrites && memoryToolActionWrites(tc.Input) {
		return "agent noise policy: memory writes are disabled", true
	}
	if tc.Name != "notify" {
		return "", false
	}
	if min := notifySeverityRank(policy.minNotifySeverity); min > notifySeverityRank(notifySeverityFromInput(tc.Input)) {
		return fmt.Sprintf("agent noise policy: notify severity must be at least %s", policy.minNotifySeverity), true
	}
	if policy.minNotifyIntervalSec <= 0 {
		return "", false
	}
	slug, _ := agentIdentFromCtx(ctx)
	if strings.TrimSpace(slug) == "" {
		return "", false
	}
	nowMS := time.Now().UnixMilli()
	var st agentNoiseState
	if raw, ok, err := k.state.Get(agentNoiseStateNS, slug); err != nil {
		return "agent noise policy: notify cooldown state unavailable: " + err.Error(), true
	} else if ok {
		_ = json.Unmarshal(raw, &st)
	}
	if st.PendingNotifyMS > 0 && nowMS-st.PendingNotifyMS < int64(agentNoisePendingNotifyTTL/time.Millisecond) {
		return "agent noise policy: notify send already in progress", true
	}
	if st.LastNotifyMS > 0 {
		elapsed := nowMS - st.LastNotifyMS
		minMS := int64(policy.minNotifyIntervalSec) * 1000
		if elapsed < minMS {
			remaining := time.Duration(minMS-elapsed) * time.Millisecond
			return "agent noise policy: notify cooldown active for " + remaining.Round(time.Second).String(), true
		}
	}
	st.PendingNotifyMS = nowMS
	if err := k.state.Set(agentNoiseStateNS, slug, st); err != nil {
		return "agent noise policy: notify cooldown state unavailable: " + err.Error(), true
	}
	return "", false
}

func (k *Kernel) completeAgentNoiseNotify(ctx context.Context, tc agent.ToolCall, res agent.Result) {
	policy, ok := agentNoisePolicyFromCtx(ctx)
	if !ok || policy.minNotifyIntervalSec <= 0 || tc.Name != "notify" {
		return
	}
	slug, _ := agentIdentFromCtx(ctx)
	if strings.TrimSpace(slug) == "" {
		return
	}
	var st agentNoiseState
	if raw, ok, err := k.state.Get(agentNoiseStateNS, slug); err != nil {
		return
	} else if ok {
		_ = json.Unmarshal(raw, &st)
	}
	st.PendingNotifyMS = 0
	if !res.IsError {
		st.LastNotifyMS = time.Now().UnixMilli()
	}
	_ = k.state.Set(agentNoiseStateNS, slug, st)
}

func memoryToolActionWrites(raw json.RawMessage) bool {
	var in struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "remember", "forget", "bulk_forget":
		return true
	default:
		return false
	}
}

func notifySeverityFromInput(raw json.RawMessage) string {
	var in struct {
		Severity string `json:"severity"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return "info"
	}
	severity := strings.ToLower(strings.TrimSpace(in.Severity))
	if severity == "" {
		return "info"
	}
	return severity
}

func notifySeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 3
	case "warning", "warn":
		return 2
	case "info", "":
		return 1
	default:
		return 0
	}
}

type approvalBundle struct {
	EffectClass       string
	PredictedEffects  []string
	AffectedResources []string
	RollbackNotes     string
	Confidence        float64
}

func (k *Kernel) approvalDecisionBundle(toolName string, cap edict.Capability, input json.RawMessage, def agent.ToolDef) approvalBundle {
	effect := def.Effect
	if effect.Class == "" && len(effect.PredictedEffects) == 0 && len(effect.AffectedResources) == 0 {
		if tool, ok := k.tools[toolName]; ok {
			effect = tool.Definition().Effect
		}
	}

	class := normalizeEffectClass(effect.Class)
	if class == "" {
		class = defaultEffectClass(cap)
	}
	resources := append([]string(nil), effect.AffectedResources...)
	if len(resources) == 0 {
		resources = affectedResourcesFromInput(toolName, cap, input)
	}
	predicted := append([]string(nil), effect.PredictedEffects...)
	if len(predicted) == 0 {
		predicted = []string{fmt.Sprintf("invoke %s under %s", toolName, cap)}
	}
	rollback := strings.TrimSpace(effect.RollbackNotes)
	if rollback == "" {
		rollback = defaultRollbackNotes(class)
	}
	confidence := effect.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = defaultEffectConfidence(class)
	}
	return approvalBundle{
		EffectClass:       class,
		PredictedEffects:  predicted,
		AffectedResources: resources,
		RollbackNotes:     rollback,
		Confidence:        confidence,
	}
}

func normalizeEffectClass(class agent.EffectClass) string {
	switch class {
	case agent.EffectReadOnly, agent.EffectReversible, agent.EffectCompensable, agent.EffectIrreversible:
		return string(class)
	default:
		return ""
	}
}

func defaultEffectClass(cap edict.Capability) string {
	switch cap {
	case edict.CapFileRead, edict.CapFileList, edict.CapHTTPGet, edict.CapBrowserRead,
		edict.CapHomeAssistantRead, edict.CapWebSearch, edict.CapRunsRead,
		edict.CapIntrospect, edict.CapConfigRead, edict.CapProviderCall:
		return string(agent.EffectReadOnly)
	case edict.CapFileWrite, edict.CapMemory, edict.CapWorld, edict.CapSchedule,
		edict.CapStanding, edict.CapBoard, edict.CapSkill, edict.CapOversee,
		edict.CapToolForge, edict.CapConfigWrite, edict.CapWorkflow:
		return string(agent.EffectReversible)
	case edict.CapNotify, edict.CapHTTPPost, edict.CapRemoteRun:
		return string(agent.EffectCompensable)
	case edict.CapShell, edict.CapFileDelete, edict.CapCoding, edict.CapACPAgent,
		edict.CapHomeAssistantCall, edict.CapCodeExec, edict.CapMCPInstall, edict.CapMCP:
		return string(agent.EffectIrreversible)
	default:
		return string(agent.EffectIrreversible)
	}
}

func affectedResourcesFromInput(toolName string, cap edict.Capability, input json.RawMessage) []string {
	out := []string{"tool:" + toolName, "capability:" + string(cap)}
	var obj map[string]any
	if err := json.Unmarshal(input, &obj); err != nil {
		return out
	}
	for _, key := range []string{"path", "url", "endpoint", "entity_id", "service", "command", "op", "name", "id"} {
		if v, ok := obj[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, key+":"+s)
			}
		}
	}
	return out
}

func defaultRollbackNotes(class string) string {
	switch class {
	case string(agent.EffectReadOnly):
		return "No rollback required for read-only action."
	case string(agent.EffectReversible):
		return "Use the corresponding revert/delete/restore operation or journaled state to undo if needed."
	case string(agent.EffectCompensable):
		return "No guaranteed rollback; compensate with a follow-up action if the outcome is wrong."
	case string(agent.EffectIrreversible):
		return "No reliable rollback path declared; approve only if the effect is acceptable."
	default:
		return "No rollback information declared."
	}
}

func defaultEffectConfidence(class string) float64 {
	switch class {
	case string(agent.EffectReadOnly):
		return 0.95
	case string(agent.EffectReversible):
		return 0.75
	case string(agent.EffectCompensable):
		return 0.6
	case string(agent.EffectIrreversible):
		return 0.5
	default:
		return 0.4
	}
}
