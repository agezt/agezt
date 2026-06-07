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
