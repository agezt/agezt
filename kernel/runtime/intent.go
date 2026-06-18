// SPDX-License-Identifier: MIT

package runtime

import (
	"github.com/agezt/agezt/kernel/event"
	intentmodel "github.com/agezt/agezt/kernel/intent"
)

func (k *Kernel) publishIntentInterpreted(corr, actor string, frame intentmodel.Frame) {
	if k == nil || k.bus == nil {
		return
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "intent.interpreted",
		Kind:          event.KindIntentInterpreted,
		Actor:         actor,
		CorrelationID: corr,
		Payload: map[string]any{
			"user_utterance_hash": frame.UserUtteranceHash,
			"canonical_intent":    frame.CanonicalIntent,
			"assumptions":         frame.Assumptions,
			"explicit_exclusions": frame.ExplicitExclusions,
			"candidate_plans":     frame.CandidatePlans,
			"harmful_reading":     frame.HarmfulReading,
			"ambiguity_score":     frame.AmbiguityScore,
			"underdetermined":     frame.Underdetermined,
		},
	})
}

func (k *Kernel) publishIntentConfirmationRequired(corr, actor string, frame intentmodel.Frame, axes intentmodel.RegretAxes, prompt string) {
	if k == nil || k.bus == nil {
		return
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "intent.confirmation_required",
		Kind:          event.KindIntentConfirmationRequired,
		Actor:         actor,
		CorrelationID: corr,
		Payload: map[string]any{
			"user_utterance_hash":    frame.UserUtteranceHash,
			"canonical_intent":       frame.CanonicalIntent,
			"harmful_interpretation": frame.HarmfulReading,
			"ambiguity_score":        frame.AmbiguityScore,
			"regret_axes":            regretAxesPayload(axes),
			"confirmation_prompt":    prompt,
		},
	})
}

// publishAutoApprove journals that a session-scoped operator grant satisfied an
// approval-class capability without prompting (the chat "auto-approve Tool Forge
// this session" toggle). Auditable: `agt why` / the policy log shows the run
// auto-approved this capability rather than asking.
func (k *Kernel) publishAutoApprove(corr, actor, capability, tool string) {
	if k == nil || k.bus == nil {
		return
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "policy.auto_approved",
		Kind:          event.Kind("policy.auto_approved"),
		Actor:         actor,
		CorrelationID: corr,
		Payload: map[string]any{
			"capability": capability,
			"tool":       tool,
			"reason":     "session-scoped auto-approve grant",
		},
	})
}

func regretAxesPayload(axes intentmodel.RegretAxes) map[string]float64 {
	return map[string]float64{
		"physical":      axes.Physical,
		"informational": axes.Informational,
		"social":        axes.Social,
		"identity":      axes.Identity,
	}
}
