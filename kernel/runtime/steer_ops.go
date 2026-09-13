// SPDX-License-Identifier: MIT

package runtime

// Kernel-level steering operations: controlFor + PauseRun +
// ResumeRun + StepRun + SteerRun + RunControlState +
// InterveneRun + publishSteer + publishIntervention. Carved out
// of steer.go during the Day 200 god-file split so the main file
// can stay focused on the runControl type + its mutex-guarded
// methods + the per-corr lookup.
// Public API unchanged.

import (
	"maps"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/intervention"
)

func (k *Kernel) PauseRun(corr string) bool {
	rc := k.controlFor(corr)
	if rc == nil {
		return false
	}
	if rc.pause(time.Time{}) {
		k.publishSteer(corr, event.KindRunPaused, nil)
	}
	return true
}

// ResumeRun lets a paused agent run freely again (M608). Returns true if the
// run exists; emits run.resumed only on an actual state change.
func (k *Kernel) ResumeRun(corr string) bool {
	rc := k.controlFor(corr)
	if rc == nil {
		return false
	}
	if rc.resume() {
		k.publishSteer(corr, event.KindRunResumed, nil)
	}
	return true
}

// StepRun advances a run by exactly one iteration then re-pauses it (M608),
// pausing first if it was running. Returns true if the run exists.
func (k *Kernel) StepRun(corr string) bool {
	rc := k.controlFor(corr)
	if rc == nil {
		return false
	}
	rc.step()
	k.publishSteer(corr, event.KindRunStepped, nil)
	return true
}

// SteerRun injects an operator directive into a running agent (M608); the loop
// folds it into the conversation as a fresh user turn at the next iteration
// boundary (and emits run.steered when it takes effect). Returns true if the
// run exists. An empty directive is rejected (false) so the UI can validate.
// note=true marks a soft "BTW" (read it, finish the current step, stay on task)
// vs a forceful steer that re-prioritises (M962).
func (k *Kernel) SteerRun(corr, directive string, note bool) bool {
	if directive == "" {
		return false
	}
	rc := k.controlFor(corr)
	if rc == nil {
		return false
	}
	rc.inject(directive, note)
	return true
}

// RunControlState reports a run's live steering state for the operator UI:
// paused flag + count of directives queued but not yet folded. ok=false when
// there is no such active run.
func (k *Kernel) RunControlState(corr string) (paused bool, pending int, ok bool) {
	rc := k.controlFor(corr)
	if rc == nil {
		return false, 0, false
	}
	paused, pending, _ = rc.snapshot()
	return paused, pending, true
}

// InterveneRun applies one protocolized live intervention. It is the structured
// face over pause/cancel/steer/query and carries lease + idempotency metadata.
func (k *Kernel) InterveneRun(req intervention.Request) (intervention.Result, error) {
	req, err := intervention.Normalize(req)
	if err != nil {
		return intervention.Result{}, err
	}
	if req.Primitive == intervention.PrimitiveAbort {
		res := intervention.Result{
			Primitive:      req.Primitive,
			CorrelationID:  req.CorrelationID,
			Accepted:       true,
			Applied:        k.CancelRun(req.CorrelationID),
			State:          "aborted",
			IdempotencyKey: req.IdempotencyKey,
		}
		if !res.Applied {
			res.Accepted = false
			res.State = "unknown"
			res.Reason = "run not active"
		}
		k.publishIntervention(res, req)
		return res, nil
	}

	rc := k.controlFor(req.CorrelationID)
	if rc == nil {
		res := intervention.Result{Primitive: req.Primitive, CorrelationID: req.CorrelationID, State: "unknown", Reason: "run not active", IdempotencyKey: req.IdempotencyKey}
		k.publishIntervention(res, req)
		return res, nil
	}
	if prior, ok := rc.resultForKey(req.IdempotencyKey); ok {
		return prior, nil
	}

	res := intervention.Result{
		Primitive:      req.Primitive,
		CorrelationID:  req.CorrelationID,
		Accepted:       true,
		Applied:        true,
		IdempotencyKey: req.IdempotencyKey,
	}
	switch req.Primitive {
	case intervention.PrimitiveHalt:
		res.LeaseExpires = time.Now().Add(req.Lease)
		res.Applied = rc.pause(res.LeaseExpires)
		res.State = "paused"
		res.Paused, res.Pending, _ = rc.snapshot()
		if res.Applied {
			k.publishSteer(req.CorrelationID, event.KindRunPaused, map[string]any{"primitive": string(req.Primitive), "lease_expires_unix": res.LeaseExpires.Unix()})
		}
	case intervention.PrimitiveRedirect:
		rc.inject(req.Directive, false)
		res.State = "redirect_queued"
		res.Paused, res.Pending, _ = rc.snapshot()
	case intervention.PrimitiveAdjust:
		rc.inject(req.Directive, true)
		res.State = "adjustment_queued"
		res.Paused, res.Pending, _ = rc.snapshot()
	case intervention.PrimitiveQuery:
		res.State = "observed"
		res.Paused, res.Pending, res.LeaseExpires = rc.snapshot()
		res.Applied = false
	}
	rc.rememberResult(req.IdempotencyKey, res)
	k.publishIntervention(res, req)
	return res, nil
}

// publishSteer emits a steering control event correlated to the run, so the
// action shows up on the run's timeline and the live firehose. Best-effort —
// a bus error never blocks the control operation.
func (k *Kernel) publishSteer(corr string, kind event.Kind, extra map[string]any) {
	payload := map[string]any{"correlation_id": corr}
	maps.Copy(payload, extra)
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "kernel.steer",
		Kind:          kind,
		Actor:         "operator",
		CorrelationID: corr,
		Payload:       payload,
	})
}

func (k *Kernel) publishIntervention(res intervention.Result, req intervention.Request) {
	payload := map[string]any{
		"primitive":       string(res.Primitive),
		"correlation_id":  res.CorrelationID,
		"accepted":        res.Accepted,
		"applied":         res.Applied,
		"state":           res.State,
		"paused":          res.Paused,
		"pending":         res.Pending,
		"scope":           req.Scope,
		"idempotency_key": res.IdempotencyKey,
		"reason":          res.Reason,
	}
	if !res.LeaseExpires.IsZero() {
		payload["lease_expires_unix"] = res.LeaseExpires.Unix()
	}
	if req.Directive != "" {
		payload["directive"] = req.Directive
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "kernel.intervention",
		Kind:          event.KindRunIntervention,
		Actor:         "operator",
		CorrelationID: res.CorrelationID,
		Payload:       payload,
	})
}

