// SPDX-License-Identifier: MIT

// Runtime tool-call policy hook (capability + verdict).
// Code extracted from policy.go during the Day-82 god-file split.
// Public API unchanged.
package runtime


import (
	"context"
	"fmt"
	"strings"

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

// capabilityFor resolves the policy axis one tool call is gated on, in order of
// how specific the source is:
//
//  1. The tool's own ToolDef.Capability — the authoritative declaration, made in
//     the tool's package next to the behaviour it describes.
//  2. Config.ToolCapabilities (M900) — the same declaration arriving through a
//     plugin's capability manifest, for a tool whose ToolDef crossed a process
//     boundary and so cannot carry Go fields.
//  3. edict.CapabilityForToolCall — the name switch, now a fallback. It still
//     owns the DYNAMIC surfaces no static declaration can cover: forged
//     `forge_<name>` script tools and bridged `mcp_<server>_<tool>` calls.
//
// A declaration naming a capability Edict does not govern is IGNORED rather than
// honoured, and resolution continues down the list. This matters: an unknown
// capability is default-denied, so honouring a typo would silently kill the tool
// — the exact failure this field was introduced to end. Same rule the plugin
// manifest overlay already applied (a tool may join an existing axis, never
// invent one), now applied to in-tree declarations too.
func (k *Kernel) capabilityFor(tc agent.ToolCall, def agent.ToolDef) edict.Capability {
	if !def.Capability.IsZero() {
		if cap := def.Capability.For(tc.Input); edict.KnownCapability(cap) {
			return edict.Capability(cap)
		}
	}
	if cap, ok := k.toolCaps[tc.Name]; ok {
		return cap
	}
	return edict.CapabilityForToolCall(tc.Name, tc.Input)
}

func (k *Kernel) policyHook(ctx context.Context, tc agent.ToolCall) agent.PolicyVerdict {
	def, _ := agent.PolicyToolDefFromContext(ctx)
	cap := k.capabilityFor(tc, def)
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
	// guardRaised names the fail-closed guard that routed this call to approval,
	// as opposed to the Edict Ask axis (out.RequiresApproval). The session
	// auto-approve grant below satisfies the Ask axis only: a guard the operator
	// armed deliberately must not be answered by a blanket capability grant.
	guardRaised := ""
	if k.cfg.EpistemicEscalation && verdict.Allow && ep.escalates() {
		requiresApproval = true
		approvalReason = ep.Reason
		verdict.Allow = false
		verdict.WouldAsk = true
		guardRaised = "epistemic escalation"
	}
	if k.cfg.IntentRegretGating && verdict.Allow && hasIntentFrame && intentmodel.RequiresConfirmation(intentFrame, regretAxes) {
		requiresApproval = true
		approvalReason = confirmationPrompt
		verdict.Allow = false
		verdict.WouldAsk = true
		guardRaised = "intent/regret gating"
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
			guardRaised = "prompt-injection guard"
		} else {
			k.publishPromptInjectionWarned(correlationFromCtx(ctx), actorFromCtx(ctx), tc.Name, string(out.Capability), taint.Sources, trustedObservations(ctx))
		}
	}

	// Session-scoped operator grant (chat "auto-approve Tool Forge this session"):
	// if the run carries an auto-approve set covering this capability, satisfy the
	// approval without prompting and journal it as an auto-grant (WouldAsk stays
	// true so `agt why` shows it would have asked). Hard-denies never reach here
	// (they resolve to deny, not approval), so this can't override the F4 floor.
	//
	// It does not satisfy a fail-closed guard either. The grant answers the Edict
	// Ask axis; the guards above (epistemic escalation, intent/regret gating, the
	// prompt-injection guard) are each armed by their own opt-in setting, and a
	// blanket capability grant is not an answer to a question they asked for a
	// different reason. Such a call falls through to live HITL carrying the
	// guard's own reason string, so the operator sees which guard stopped it.
	if requiresApproval && guardRaised == "" && autoApproveCap(ctx, string(out.Capability)) {
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
