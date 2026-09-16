// SPDX-License-Identifier: MIT

// approval_methods.go owns the Registry's external surface:
// Submit (block-until-resolved), Resolve (operator verdict),
// and Pending (snapshot of in-flight requests). The Registry
// constructor + every data type live in approval.go. Carved
// out during the Day-211 god-file split. Public API unchanged.
package approval

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/agezt/agezt/kernel/ulid"
)

// Submit registers a pending request and blocks until Resolve is
// called for it, the per-request timeout fires (DecisionTimeout), or
// ctx is cancelled (DecisionCancel). The returned Outcome is also
// journaled as approval.{granted,denied,timeout} so the trace is
// auditable.
func (r *Registry) Submit(ctx context.Context, spec SubmitSpec) Outcome {
	now := r.now()
	req := Request{
		ID:                    "appr-" + ulid.New(),
		Capability:            spec.Capability,
		ToolName:              spec.ToolName,
		Input:                 spec.Input,
		Reason:                spec.Reason,
		Actor:                 spec.Actor,
		CorrelationID:         spec.CorrelationID,
		CreatedAt:             now,
		Timeout:               now.Add(r.timeout),
		EffectClass:           spec.EffectClass,
		PredictedEffects:      append([]string(nil), spec.PredictedEffects...),
		AffectedResources:     append([]string(nil), spec.AffectedResources...),
		RollbackNotes:         spec.RollbackNotes,
		Confidence:            spec.Confidence,
		CanonicalIntent:       spec.CanonicalIntent,
		HarmfulInterpretation: spec.HarmfulInterpretation,
		AmbiguityScore:        spec.AmbiguityScore,
		RegretAxes:            cloneFloatMap(spec.RegretAxes),
		ConfirmationPrompt:    spec.ConfirmationPrompt,
	}
	entry := &pending{req: req, done: make(chan Outcome, 1)}

	r.mu.Lock()
	r.entries[req.ID] = entry
	r.mu.Unlock()

	r.publishRequested(req)

	// Detach from the entry on every exit so a Resolve after exit is
	// a clean no-op rather than blocking on the buffered channel.
	defer r.detach(req.ID)

	timer := time.NewTimer(r.timeout)
	defer timer.Stop()

	select {
	case out := <-entry.done:
		r.publishResolved(req, out)
		return out
	case <-timer.C:
		out := Outcome{Decision: DecisionTimeout, Reason: "no response within timeout", ResolvedBy: "system"}
		r.publishResolved(req, out)
		return out
	case <-ctx.Done():
		out := Outcome{Decision: DecisionCancel, Reason: ctx.Err().Error(), ResolvedBy: "system"}
		r.publishResolved(req, out)
		return out
	}
}

// Resolve records a decision for the named ID. Returns
// ErrUnknownApproval if no pending entry exists. Idempotent for the
// caller — a second Resolve for the same ID returns ErrUnknownApproval
// because the first one already detached the entry.
func (r *Registry) Resolve(id string, decision Decision, reason, resolvedBy string) error {
	if decision != DecisionGrant && decision != DecisionDeny {
		return fmt.Errorf("approval: Resolve accepts only grant/deny, got %q", decision)
	}
	r.mu.Lock()
	entry, ok := r.entries[id]
	if ok {
		delete(r.entries, id)
	}
	r.mu.Unlock()
	if !ok {
		return ErrUnknownApproval
	}
	if resolvedBy == "" {
		resolvedBy = "operator"
	}
	// Non-blocking send — buffered channel of size 1 means this never
	// blocks, but in the unlikely race where the waiter already exited
	// (ctx-cancel / timeout) we just drop the outcome.
	select {
	case entry.done <- Outcome{Decision: decision, Reason: reason, ResolvedBy: resolvedBy}:
	default:
	}
	return nil
}

// Pending returns a snapshot of currently-pending requests, sorted by
// CreatedAt ascending. Safe to read concurrently with Submit/Resolve.
func (r *Registry) Pending() []Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Request, 0, len(r.entries))
	for _, p := range r.entries {
		out = append(out, cloneRequest(p.req))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}
