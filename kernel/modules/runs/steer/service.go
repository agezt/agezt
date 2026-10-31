// SPDX-License-Identifier: MIT

package steer

import (
	"errors"
	"maps"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/intervention"
)

// Dependencies are the host's doors. The module owns the steering; the host
// owns the live runs, the bus and the cancel. The service asks through these
// ports, so it never names the daemon.
type Dependencies struct {
	// Lookup returns the live run's steering surface, and whether the run is
	// live at all. A host without a live run answers false.
	Lookup func(corr string) (*Control, bool)

	// Publish emits one event on the host's bus. Best-effort: a bus error
	// never blocks a control operation.
	Publish func(spec event.Spec)

	// Cancel cancels a live run; it answers whether the run was cancelled.
	Cancel func(corr string) bool

	// Now is the clock. The lease arithmetic must be testable, so the host
	// names the time rather than the module reaching for it.
	Now func() time.Time
}

// Service is the runs module's steering face: the operations an operator (or
// a protocolized intervention) drives a live run with.
type Service struct {
	deps Dependencies
}

// NewService returns the steering service over the host's ports. A host that
// names no ports gets a service that refuses every operation — a run is not
// steered by nobody.
func NewService(deps Dependencies) *Service {
	if deps.Lookup == nil {
		deps.Lookup = func(string) (*Control, bool) { return nil, false }
	}
	if deps.Publish == nil {
		deps.Publish = func(event.Spec) {}
	}
	if deps.Cancel == nil {
		deps.Cancel = func(string) bool { return false }
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Service{deps: deps}
}

// Pause parks a running agent at its next iteration boundary (M608). Returns
// true if a matching live run was found, false otherwise. Idempotent at the
// event level: a second pause on an already-paused run still returns true (the
// run is paused) but emits no duplicate event.
func (s *Service) Pause(corr string) bool {
	if s == nil {
		return false
	}
	rc, ok := s.deps.Lookup(corr)
	if !ok {
		return false
	}
	if rc.Pause(time.Time{}) {
		s.publishSteer(corr, event.KindRunPaused, nil)
	}
	return true
}

// Resume lets a paused agent run freely again (M608). Returns true if the
// run exists; emits run.resumed only on an actual state change.
func (s *Service) Resume(corr string) bool {
	if s == nil {
		return false
	}
	rc, ok := s.deps.Lookup(corr)
	if !ok {
		return false
	}
	if rc.Resume() {
		s.publishSteer(corr, event.KindRunResumed, nil)
	}
	return true
}

// Step advances a run by exactly one iteration then re-pauses it (M608),
// pausing first if it was running. Returns true if the run exists.
func (s *Service) Step(corr string) bool {
	if s == nil {
		return false
	}
	rc, ok := s.deps.Lookup(corr)
	if !ok {
		return false
	}
	rc.Step()
	s.publishSteer(corr, event.KindRunStepped, nil)
	return true
}

// Steer injects an operator directive into a running agent (M608); the loop
// folds it into the conversation as a fresh user turn at the next iteration
// boundary (and emits run.steered when it takes effect). Returns true if the
// run exists. An empty directive is rejected (false) so the UI can validate.
// note=true marks a soft "BTW" (read it, finish the current step, stay on task)
// vs a forceful steer that re-prioritises (M962).
func (s *Service) Steer(corr, directive string, note bool) bool {
	if directive == "" {
		return false
	}
	rc, ok := s.deps.Lookup(corr)
	if !ok {
		return false
	}
	rc.Inject(directive, note)
	return true
}

// State reports a run's live steering state for the operator UI:
// paused flag + count of directives queued but not yet folded. ok=false when
// there is no such active run.
func (s *Service) State(corr string) (paused bool, pending int, ok bool) {
	if s == nil {
		return false, 0, false
	}
	rc, ok := s.deps.Lookup(corr)
	if !ok {
		return false, 0, false
	}
	paused, pending, _ = rc.Snapshot()
	return paused, pending, true
}

// Intervene applies one protocolized live intervention. It is the structured
// face over pause/cancel/steer/query and carries lease + idempotency metadata.
func (s *Service) Intervene(req intervention.Request) (intervention.Result, error) {
	if s == nil {
		return intervention.Result{}, errors.New("steer: the steering service is not available")
	}
	req, err := intervention.Normalize(req)
	if err != nil {
		return intervention.Result{}, err
	}
	if req.Primitive == intervention.PrimitiveAbort {
		res := intervention.Result{
			Primitive:      req.Primitive,
			CorrelationID:  req.CorrelationID,
			Accepted:       true,
			Applied:        s.deps.Cancel(req.CorrelationID),
			State:          "aborted",
			IdempotencyKey: req.IdempotencyKey,
		}
		if !res.Applied {
			res.Accepted = false
			res.State = "unknown"
			res.Reason = "run not active"
		}
		s.publishIntervention(res, req)
		return res, nil
	}

	rc, ok := s.deps.Lookup(req.CorrelationID)
	if !ok {
		res := intervention.Result{Primitive: req.Primitive, CorrelationID: req.CorrelationID, State: "unknown", Reason: "run not active", IdempotencyKey: req.IdempotencyKey}
		s.publishIntervention(res, req)
		return res, nil
	}
	if prior, ok := rc.ResultForKey(req.IdempotencyKey); ok {
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
		res.LeaseExpires = s.deps.Now().Add(req.Lease)
		res.Applied = rc.Pause(res.LeaseExpires)
		res.State = "paused"
		res.Paused, res.Pending, _ = rc.Snapshot()
		if res.Applied {
			s.publishSteer(req.CorrelationID, event.KindRunPaused, map[string]any{"primitive": string(req.Primitive), "lease_expires_unix": res.LeaseExpires.Unix()})
		}
	case intervention.PrimitiveRedirect:
		rc.Inject(req.Directive, false)
		res.State = "redirect_queued"
		res.Paused, res.Pending, _ = rc.Snapshot()
	case intervention.PrimitiveAdjust:
		rc.Inject(req.Directive, true)
		res.State = "adjustment_queued"
		res.Paused, res.Pending, _ = rc.Snapshot()
	case intervention.PrimitiveQuery:
		res.State = "observed"
		res.Paused, res.Pending, res.LeaseExpires = rc.Snapshot()
		res.Applied = false
	}
	rc.RememberResult(req.IdempotencyKey, res)
	s.publishIntervention(res, req)
	return res, nil
}

// publishSteer emits a steering control event correlated to the run, so the
// action shows up on the run's timeline and the live firehose. Best-effort —
// a bus error never blocks the control operation.
func (s *Service) publishSteer(corr string, kind event.Kind, extra map[string]any) {
	payload := map[string]any{"correlation_id": corr}
	maps.Copy(payload, extra)
	s.deps.Publish(event.Spec{
		Subject:       "kernel.steer",
		Kind:          kind,
		Actor:         "operator",
		CorrelationID: corr,
		Payload:       payload,
	})
}

func (s *Service) publishIntervention(res intervention.Result, req intervention.Request) {
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
	s.deps.Publish(event.Spec{
		Subject:       "kernel.intervention",
		Kind:          event.KindRunIntervention,
		Actor:         "operator",
		CorrelationID: res.CorrelationID,
		Payload:       payload,
	})
}
