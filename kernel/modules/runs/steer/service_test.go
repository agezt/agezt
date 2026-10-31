// SPDX-License-Identifier: MIT

package steer

import (
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/intervention"
)

// The steering operations moved into the runs module in W3.76b. These tests
// pin the module's half of the move: the module drives the control, and the
// host's ports are the only way it reaches the world.

// harness is a host's four ports, wired to a live control.
type harness struct {
	svc     *Service
	control *Control
	events  []event.Spec
	cancels []string
	now     time.Time
}

func newHarness() *harness {
	h := &harness{control: NewControl(), now: time.Unix(1_700_000_000, 0)}
	h.svc = NewService(Dependencies{
		Lookup: func(corr string) (*Control, bool) {
			if corr == "corr-1" {
				return h.control, true
			}
			return nil, false
		},
		Publish: func(spec event.Spec) { h.events = append(h.events, spec) },
		Cancel: func(corr string) bool {
			h.cancels = append(h.cancels, corr)
			return true
		},
		Now: func() time.Time { return h.now },
	})
	return h
}

// TestTheServiceDrivesTheControl: pause, resume and step reach the control
// and emit the steering events.
func TestTheServiceDrivesTheControl(t *testing.T) {
	h := newHarness()

	if !h.svc.Pause("corr-1") {
		t.Fatal("pause must reach the live run")
	}
	if paused, pending, ok := h.svc.State("corr-1"); !ok || !paused || pending != 0 {
		t.Fatalf("state after pause = (%v,%d,%v) want (true,0,true)", paused, pending, ok)
	}
	if len(h.events) != 1 || h.events[0].Kind != event.KindRunPaused {
		t.Fatalf("events after pause = %+v want one run.paused", h.events)
	}
	if h.events[0].Subject != "kernel.steer" || h.events[0].CorrelationID != "corr-1" {
		t.Fatalf("the steering event = %+v want the kernel.steer subject for corr-1", h.events[0])
	}

	if !h.svc.Resume("corr-1") {
		t.Fatal("resume must reach the live run")
	}
	if paused, _, _ := h.svc.State("corr-1"); paused {
		t.Fatal("the run must be running again after resume")
	}
	if len(h.events) != 2 || h.events[1].Kind != event.KindRunResumed {
		t.Fatalf("events after resume = %+v want one run.resumed", h.events)
	}

	if !h.svc.Step("corr-1") {
		t.Fatal("step must reach the live run")
	}
	if len(h.events) != 3 || h.events[2].Kind != event.KindRunStepped {
		t.Fatalf("events after step = %+v want one run.stepped", h.events)
	}
}

// TestTheSecondPauseEmitsNoDuplicateEvent: the pause is idempotent at the
// event level — the run stays paused, but the firehose does not repeat itself.
func TestTheSecondPauseEmitsNoDuplicateEvent(t *testing.T) {
	h := newHarness()
	if !h.svc.Pause("corr-1") {
		t.Fatal("the first pause must reach the live run")
	}
	if !h.svc.Pause("corr-1") {
		t.Fatal("a second pause still answers true: the run is paused")
	}
	if len(h.events) != 1 {
		t.Fatalf("events = %+v want exactly one run.paused", h.events)
	}
	if paused, _, _ := h.svc.State("corr-1"); !paused {
		t.Fatal("the run must still be paused")
	}
}

// TestTheServiceRefusesAnUnknownRun: a run that is not live is not steered.
func TestTheServiceRefusesAnUnknownRun(t *testing.T) {
	h := newHarness()
	if h.svc.Pause("corr-nope") || h.svc.Resume("corr-nope") || h.svc.Step("corr-nope") {
		t.Fatal("an unknown run must not be steered")
	}
	if _, _, ok := h.svc.State("corr-nope"); ok {
		t.Fatal("an unknown run has no steering state")
	}
	if len(h.events) != 0 {
		t.Fatalf("an unknown run must not emit events, got %+v", h.events)
	}
}

// TestTheServiceRejectsAnEmptyDirective: an empty directive is rejected so the
// UI can validate it.
func TestTheServiceRejectsAnEmptyDirective(t *testing.T) {
	h := newHarness()
	if h.svc.Steer("corr-1", "", false) {
		t.Fatal("an empty directive must be rejected")
	}
	if got := h.control.Drain(); got != nil {
		t.Fatalf("a rejected directive must not be queued, got %v", got)
	}
}

// TestTheServiceQueuesADirective: a steering directive reaches the control.
func TestTheServiceQueuesADirective(t *testing.T) {
	h := newHarness()
	if !h.svc.Steer("corr-1", "do this first", true) {
		t.Fatal("steering a live run must succeed")
	}
	got := h.control.Drain()
	if len(got) != 1 || got[0].Text != "do this first" || !got[0].Note {
		t.Fatalf("queued directives = %v want one note directive", got)
	}
}

// TestTheInterventionsAreApplied: the structured face over pause/cancel/steer/
// query, with the lease and the idempotency key.
func TestTheInterventionsAreApplied(t *testing.T) {
	h := newHarness()

	// HALT parks the run under a lease.
	res, err := h.svc.Intervene(intervention.Request{
		Primitive:     intervention.PrimitiveHalt,
		CorrelationID: "corr-1",
		Lease:         30 * time.Second,
	})
	if err != nil {
		t.Fatalf("intervene: %v", err)
	}
	if !res.Accepted || !res.Applied || res.State != "paused" {
		t.Fatalf("halt result = %+v want an accepted, applied pause", res)
	}
	if !res.LeaseExpires.Equal(h.now.Add(30 * time.Second)) {
		t.Fatalf("lease expiry = %v want %v", res.LeaseExpires, h.now.Add(30*time.Second))
	}

	// REDIRECT queues a forceful directive.
	res, err = h.svc.Intervene(intervention.Request{
		Primitive:     intervention.PrimitiveRedirect,
		CorrelationID: "corr-1",
		Directive:     "stop and report",
	})
	if err != nil {
		t.Fatalf("intervene: %v", err)
	}
	if res.State != "redirect_queued" || !res.Applied {
		t.Fatalf("redirect result = %+v want a queued redirect", res)
	}
	if got := h.control.Drain(); len(got) != 1 || got[0].Note {
		t.Fatalf("a redirect is a forceful directive, got %v", got)
	}

	// ADJUST queues a soft note.
	res, err = h.svc.Intervene(intervention.Request{
		Primitive:     intervention.PrimitiveAdjust,
		CorrelationID: "corr-1",
		Directive:     "by the way",
	})
	if err != nil {
		t.Fatalf("intervene: %v", err)
	}
	if res.State != "adjustment_queued" {
		t.Fatalf("adjust result = %+v want a queued adjustment", res)
	}
	if got := h.control.Drain(); len(got) != 1 || !got[0].Note {
		t.Fatalf("an adjustment is a soft note, got %v", got)
	}

	// QUERY observes without changing anything.
	res, err = h.svc.Intervene(intervention.Request{
		Primitive:     intervention.PrimitiveQuery,
		CorrelationID: "corr-1",
	})
	if err != nil {
		t.Fatalf("intervene: %v", err)
	}
	if res.State != "observed" || res.Applied {
		t.Fatalf("query result = %+v want an observation that applied nothing", res)
	}

	// Every intervention is published on its own subject, with its own kind.
	var interventions int
	for _, ev := range h.events {
		if ev.Subject == "kernel.intervention" {
			interventions++
			if ev.Kind != event.KindRunIntervention {
				t.Fatalf("the intervention event = %+v want kind run.intervention", ev)
			}
		}
	}
	if interventions == 0 {
		t.Fatalf("no intervention was published, events = %+v", h.events)
	}
}

// TestTheAbortCancelsTheRun: an abort cancels through the host's port.
func TestTheAbortCancelsTheRun(t *testing.T) {
	h := newHarness()
	res, err := h.svc.Intervene(intervention.Request{
		Primitive:     intervention.PrimitiveAbort,
		CorrelationID: "corr-1",
	})
	if err != nil {
		t.Fatalf("intervene: %v", err)
	}
	if !res.Accepted || !res.Applied || res.State != "aborted" {
		t.Fatalf("abort result = %+v want an accepted, applied abort", res)
	}
	if len(h.cancels) != 1 || h.cancels[0] != "corr-1" {
		t.Fatalf("cancels = %v want one cancel for corr-1", h.cancels)
	}
}

// TestTheIdempotencyKeyIsHonoured: the same key returns the remembered result
// instead of acting twice.
func TestTheIdempotencyKeyIsHonoured(t *testing.T) {
	h := newHarness()
	req := intervention.Request{
		Primitive:      intervention.PrimitiveRedirect,
		CorrelationID:  "corr-1",
		Directive:      "stop",
		IdempotencyKey: "key-1",
	}
	first, err := h.svc.Intervene(req)
	if err != nil {
		t.Fatalf("intervene: %v", err)
	}
	second, err := h.svc.Intervene(req)
	if err != nil {
		t.Fatalf("intervene: %v", err)
	}
	if second.State != first.State || second.Applied != first.Applied {
		t.Fatalf("the repeated intervention = %+v want the remembered %+v", second, first)
	}
	if got := h.control.Drain(); len(got) != 1 {
		t.Fatalf("the directive must be queued once, got %v", got)
	}
}

// TestAnInterventionForAnUnknownRunIsRefused: an unknown run yields a
// "not active" result, not an error.
func TestAnInterventionForAnUnknownRunIsRefused(t *testing.T) {
	h := newHarness()
	res, err := h.svc.Intervene(intervention.Request{
		Primitive:     intervention.PrimitiveHalt,
		CorrelationID: "corr-nope",
	})
	if err != nil {
		t.Fatalf("intervene: %v", err)
	}
	if res.State != "unknown" || res.Accepted {
		t.Fatalf("result = %+v want a refused, unknown state", res)
	}
}

// TestAMalformedInterventionIsRejected: the grammar refuses it.
func TestAMalformedInterventionIsRejected(t *testing.T) {
	h := newHarness()
	if _, err := h.svc.Intervene(intervention.Request{
		Primitive:     intervention.PrimitiveRedirect,
		CorrelationID: "corr-1",
	}); err == nil {
		t.Fatal("a redirect without a directive must be rejected")
	}
}

// TestAServiceWithoutPortsRefuses: a host that names no ports gets a service
// that refuses every operation — a run is not steered by nobody.
func TestAServiceWithoutPortsRefuses(t *testing.T) {
	svc := NewService(Dependencies{})
	if svc.Pause("corr-1") || svc.Resume("corr-1") || svc.Step("corr-1") {
		t.Fatal("a port-less service must refuse")
	}
	if _, _, ok := svc.State("corr-1"); ok {
		t.Fatal("a port-less service has no state")
	}
	res, err := svc.Intervene(intervention.Request{
		Primitive:     intervention.PrimitiveHalt,
		CorrelationID: "corr-1",
	})
	if err != nil {
		t.Fatalf("intervene: %v", err)
	}
	if res.State != "unknown" || res.Accepted {
		t.Fatalf("result = %+v want a refused, unknown state", res)
	}
}
