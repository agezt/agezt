// SPDX-License-Identifier: MIT

package skill

import "testing"

// TestCanTransition_FullMatrix exhaustively pins the SPEC-05 §5.2 skill
// lifecycle state machine — the gate that decides whether a (possibly
// self-authored, Forge-proposed) skill may reach `active` (production). The
// existing TestLegalTransitions only spot-checks 7 of the 25 from×to pairs; a
// regression in an unchecked edge (e.g. draft→active skipping shadow-test, or
// quarantined→active un-quarantining incorrectly) could silently ship a bad
// skill. The expected legal set here is written from the SPEC diagram
// INDEPENDENTLY of the implementation's legalTransitions map, so a divergence
// from the spec is caught, not rubber-stamped.
func TestCanTransition_FullMatrix(t *testing.T) {
	all := []Status{StatusDraft, StatusShadow, StatusActive, StatusQuarantined, StatusArchived}
	// SPEC-05 §5.2: draft→shadow→active; shadow/active→quarantined (regression);
	// quarantined→active (un-quarantine); any non-archived state→archived
	// (operator cleanup); archived is terminal.
	legal := map[Status]map[Status]bool{
		StatusDraft:       {StatusShadow: true, StatusArchived: true},
		StatusShadow:      {StatusActive: true, StatusQuarantined: true, StatusArchived: true},
		StatusActive:      {StatusQuarantined: true, StatusArchived: true},
		StatusQuarantined: {StatusActive: true, StatusArchived: true},
		StatusArchived:    {}, // terminal
	}
	for _, from := range all {
		for _, to := range all {
			want := legal[from][to]
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%s→%s) = %v, want %v", from, to, got, want)
			}
		}
	}

	// Self-transitions are never legal.
	for _, s := range all {
		if CanTransition(s, s) {
			t.Errorf("self-transition %s→%s must be illegal", s, s)
		}
	}
	// Archived is terminal — no outgoing legal edge.
	for _, to := range all {
		if CanTransition(StatusArchived, to) {
			t.Errorf("archived is terminal but →%s is allowed", to)
		}
	}
	// Every non-archived state can be archived (operator cleanup).
	for _, from := range []Status{StatusDraft, StatusShadow, StatusActive, StatusQuarantined} {
		if !CanTransition(from, StatusArchived) {
			t.Errorf("%s→archived must be legal (operator cleanup)", from)
		}
	}
	// draft must NOT skip shadow straight to active (shadow-test gate).
	if CanTransition(StatusDraft, StatusActive) {
		t.Error("draft→active must be illegal — a skill cannot skip the shadow-test gate")
	}
	// An unknown status has no transitions (fail-closed).
	if CanTransition(Status("bogus"), StatusActive) || CanTransition(StatusDraft, Status("bogus")) {
		t.Error("transitions involving an unknown status must be rejected")
	}
}

// TestPromoteTarget_ConsistentWithLegalTransitions locks in the invariant that
// `promote` can never drive a skill into an illegal state: every target
// PromoteTarget returns must itself be a legal CanTransition edge. If the two
// tables ever diverge, promote() would either fail inconsistently or bypass the
// state machine — so they must agree.
func TestPromoteTarget_ConsistentWithLegalTransitions(t *testing.T) {
	all := []Status{StatusDraft, StatusShadow, StatusActive, StatusQuarantined, StatusArchived}
	for _, from := range all {
		target, ok := PromoteTarget(from)
		if !ok {
			continue
		}
		if !CanTransition(from, target) {
			t.Errorf("PromoteTarget(%s)=%s but CanTransition(%s→%s) is false — promote would produce an illegal edge",
				from, target, from, target)
		}
	}
	// The promotion ladder reaches active only via shadow (the production gate).
	if tgt, _ := PromoteTarget(StatusDraft); tgt == StatusActive {
		t.Error("draft must promote to shadow, never straight to active")
	}
}
