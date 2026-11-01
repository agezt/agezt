// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime tool-call policy hook (capability + verdict). Code extracted
//             from policy.go during the Day-82 god-file split. Public API unchanged.

import (
	"context"
	"fmt"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
	"github.com/agezt/agezt/kernel/platform/policy/admission"
	"github.com/agezt/agezt/kernel/platform/policy/guards"
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
//
// The rule is the platform's (W3.75); the daemon keeps the name.
func validatedToolCaps(declared map[string]string) map[string]edict.Capability {
	return admission.ValidatedToolCaps(declared)
}

// capabilityFor resolves the policy axis one tool call is gated on.
//
// The rule is the platform's (W3.75): the declaration, the manifest overlay
// and the name switch, in that order. The daemon keeps the doors — its
// overlay and its tool map — and asks.
func (k *Kernel) capabilityFor(tc llm.ToolCall, def toolapi.ToolDef) edict.Capability {
	return admission.ResolveCapability(def, k.toolCaps, tc.Name, tc.Input)
}

func (k *Kernel) policyHook(ctx context.Context, tc llm.ToolCall) agent.PolicyVerdict {
	def, _ := agent.PolicyToolDefFromContext(ctx)
	cap := k.capabilityFor(tc, def)
	var out edict.Outcome
	if ceiling, ok := runctx.TrustCeilingFromCtx(ctx); ok {
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
	if reason, denied := agentToolPolicyDenial(runctx.AgentToolPolicyFromCtx(ctx), tc.Name); denied {
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
	requiresConfirmation := false
	if hasIntentFrame {
		confirmationPrompt = intentmodel.ConfirmationPrompt(intentFrame, intentAction, regretAxes)
		requiresConfirmation = intentmodel.RequiresConfirmation(intentFrame, regretAxes)
	}

	// The guards are the PLATFORM's rules (W3.74): the epistemic escalation,
	// the intent/regret gate and the prompt-injection guard, evaluated in that
	// order, and the FIRST one that raises wins — a guard that has already
	// stopped the call cannot be answered by a later one. The daemon keeps
	// the doors: it reads the context, it publishes the events, and it
	// submits the approval.
	outcome := guards.Evaluate(guards.Input{
		Allow:                verdict.Allow,
		AskRequired:          out.RequiresApproval,
		AskReason:            out.Reason,
		EffectClass:          bundle.EffectClass,
		Epistemic:            ep,
		RequiresConfirmation: requiresConfirmation,
		ConfirmationPrompt:   confirmationPrompt,
		DirectiveLike:        hasTaint && taint.DirectiveLike,
		TaintSources:         taint.Sources,
		TrustedObservations:  runctx.TrustedObservations(ctx),
		EpistemicEscalation:  k.cfg.EpistemicEscalation,
		IntentRegretGating:   k.cfg.IntentRegretGating,
		PromptInjection:      k.cfg.PromptInjectionGuard,
		AutoApprove:          runctx.AutoApproveCap(ctx, string(out.Capability)),
	})
	requiresApproval := outcome.RequiresApproval
	approvalReason := outcome.Reason
	if outcome.Guard != "" {
		verdict.Allow = false
		verdict.WouldAsk = true
	}
	if outcome.Guard == guards.GuardIntentRegret {
		k.publishIntentConfirmationRequired(runctx.CorrelationFromCtx(ctx), runctx.ActorFromCtx(ctx), intentFrame, regretAxes, confirmationPrompt)
	}
	if outcome.Warn {
		k.publishPromptInjectionWarned(runctx.CorrelationFromCtx(ctx), runctx.ActorFromCtx(ctx), tc.Name, string(out.Capability), taint.Sources, runctx.TrustedObservations(ctx))
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
	if outcome.AutoApproved {
		verdict.Allow = true
		verdict.WouldAsk = true
		k.publishAutoApprove(runctx.CorrelationFromCtx(ctx), runctx.ActorFromCtx(ctx), string(out.Capability), tc.Name)
		return verdict
	}

	if !requiresApproval {
		return verdict
	}

	// Live HITL: pause the tool-loop, route the request through the
	// approval queue, block until decided.
	actor := runctx.ActorFromCtx(ctx)
	corr := runctx.CorrelationFromCtx(ctx)
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
