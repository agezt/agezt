// SPDX-License-Identifier: MIT

package memory

import "testing"

// Memory provenance is first-writer-wins: on a reinforce (re-remembering the same
// fact) Remember copies the existing record's SourceEvent (manager.go: `rec.SourceEvent
// = existing.SourceEvent`), and only sets it from the current event when still empty
// (`if ev != nil && rec.SourceEvent == ""`). The origin event is the meaningful
// provenance for audit/causation. TestRememberCreatesAndJournals only checks a created
// record carries provenance, not that a reinforce PRESERVES it, so mutation testing
// (M505) showed both the copy and the guard could be weakened (overwriting provenance
// with the latest mention) undetected. Pin the preservation. Mirrors worldmodel M503.
func TestRemember_PreservesProvenanceOnReinforce(t *testing.T) {
	m, _ := newTestManager(t)
	const spec = "Agezt is a Go agentic OS"

	rec1, created, err := m.Remember("corr-1", RememberSpec{Type: TypeFact, Subject: "lictor", Content: spec})
	if err != nil || !created {
		t.Fatalf("first remember: created=%v err=%v", created, err)
	}
	if rec1.SourceEvent == "" {
		t.Fatal("first remember must set provenance")
	}

	// Re-remember the identical fact under a different correlation: reinforce, not create.
	rec2, created2, err := m.Remember("corr-2", RememberSpec{Type: TypeFact, Subject: "lictor", Content: spec})
	if err != nil {
		t.Fatalf("second remember: %v", err)
	}
	if created2 {
		t.Fatal("re-remembering the same fact must reinforce, not create a new record")
	}
	if rec2.SourceEvent != rec1.SourceEvent {
		t.Errorf("reinforce overwrote provenance: got %q, want the original %q (first-writer-wins)", rec2.SourceEvent, rec1.SourceEvent)
	}
}

// TestRemember_ActorProvenance pins WHO-attribution (M851): AddedBy is
// first-writer-wins (the original author survives a reinforce by another agent),
// while UpdatedBy tracks the most recent writer.
func TestRemember_ActorProvenance(t *testing.T) {
	m, _ := newTestManager(t)
	const subj, content = "deploys", "prod is eu-west-1"

	rec1, _, err := m.Remember("c1", RememberSpec{Type: TypeFact, Subject: subj, Content: content, Actor: "researcher"})
	if err != nil {
		t.Fatalf("first remember: %v", err)
	}
	if rec1.AddedBy != "researcher" || rec1.UpdatedBy != "researcher" {
		t.Fatalf("create provenance = added:%q updated:%q, want researcher/researcher", rec1.AddedBy, rec1.UpdatedBy)
	}

	// A different agent reinforces the same fact: author preserved, updater changes.
	rec2, created, err := m.Remember("c2", RememberSpec{Type: TypeFact, Subject: subj, Content: content, Actor: "planner"})
	if err != nil || created {
		t.Fatalf("reinforce: created=%v err=%v", created, err)
	}
	if rec2.AddedBy != "researcher" {
		t.Errorf("AddedBy = %q, want researcher (first-writer-wins)", rec2.AddedBy)
	}
	if rec2.UpdatedBy != "planner" {
		t.Errorf("UpdatedBy = %q, want planner (latest writer)", rec2.UpdatedBy)
	}

	// A reinforce with no actor (e.g. an automatic recall-reinforce) must not erase
	// the recorded author or updater.
	rec3, _, err := m.Remember("c3", RememberSpec{Type: TypeFact, Subject: subj, Content: content})
	if err != nil {
		t.Fatalf("third remember: %v", err)
	}
	if rec3.AddedBy != "researcher" || rec3.UpdatedBy != "planner" {
		t.Errorf("actorless reinforce clobbered provenance: added:%q updated:%q", rec3.AddedBy, rec3.UpdatedBy)
	}
}
