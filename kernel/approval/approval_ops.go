// SPDX-License-Identifier: MIT

package approval

// Approval internals: cloneRequest + PendingCount + detach +
// publishRequested + cloneFloatMap + publishResolved + actorOr.
// Carved out of approval.go during the Day 196 god-file split so
// the main file can stay focused on types + New + Submit +
// Resolve + Pending — the lifecycle surface — and the ops file
// can stay focused on the journal helpers.
// Public API unchanged.

import (
	"github.com/agezt/agezt/kernel/event"
)

func cloneRequest(req Request) Request {
	req.PredictedEffects = append([]string(nil), req.PredictedEffects...)
	req.AffectedResources = append([]string(nil), req.AffectedResources...)
	req.RegretAxes = cloneFloatMap(req.RegretAxes)
	return req
}

// PendingCount is a cheap len() over the queue, useful for tests.
func (r *Registry) PendingCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

func (r *Registry) detach(id string) {
	r.mu.Lock()
	delete(r.entries, id)
	r.mu.Unlock()
}

func (r *Registry) publishRequested(req Request) {
	if r.bus == nil {
		return
	}
	_, _ = r.bus.Publish(event.Spec{
		Subject:       "approval.request",
		Kind:          event.KindApprovalRequested,
		Actor:         actorOr(req.Actor, "approval"),
		CorrelationID: req.CorrelationID,
		Payload: map[string]any{
			"approval_id":            req.ID,
			"capability":             req.Capability,
			"tool_name":              req.ToolName,
			"input":                  req.Input,
			"reason":                 req.Reason,
			"timeout_unix":           req.Timeout.Unix(),
			"created_unix":           req.CreatedAt.Unix(),
			"effect_class":           req.EffectClass,
			"predicted_effects":      req.PredictedEffects,
			"affected_resources":     req.AffectedResources,
			"rollback_notes":         req.RollbackNotes,
			"confidence":             req.Confidence,
			"canonical_intent":       req.CanonicalIntent,
			"harmful_interpretation": req.HarmfulInterpretation,
			"ambiguity_score":        req.AmbiguityScore,
			"regret_axes":            req.RegretAxes,
			"confirmation_prompt":    req.ConfirmationPrompt,
		},
	})
}

func cloneFloatMap(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (r *Registry) publishResolved(req Request, out Outcome) {
	if r.bus == nil {
		return
	}
	var kind event.Kind
	switch out.Decision {
	case DecisionGrant:
		kind = event.KindApprovalGranted
	case DecisionDeny:
		kind = event.KindApprovalDenied
	case DecisionTimeout:
		kind = event.KindApprovalTimeout
	case DecisionCancel:
		// Cancellation is a system event; reuse the denied kind with
		// a clear reason so consumers only have to know about the
		// three terminal kinds.
		kind = event.KindApprovalDenied
	}
	_, _ = r.bus.Publish(event.Spec{
		Subject:       "approval.resolve",
		Kind:          kind,
		Actor:         actorOr(req.Actor, "approval"),
		CorrelationID: req.CorrelationID,
		Payload: map[string]any{
			"approval_id": req.ID,
			"decision":    string(out.Decision),
			"reason":      out.Reason,
			"resolved_by": out.ResolvedBy,
		},
	})
}

func actorOr(a, fallback string) string {
	if a == "" {
		return fallback
	}
	return a
}

