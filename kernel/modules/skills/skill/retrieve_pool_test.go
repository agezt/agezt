// SPDX-License-Identifier: MIT

package skill

import (
	"testing"
)

// TestRetrieve_OnlyActiveSkills locks the retrieve.go contract: only ACTIVE
// skills are eligible for injection — draft/shadow/quarantined/archived are never
// retrieved, regardless of keyword match. This is the property the lifecycle
// states rely on ("StatusQuarantined … excluded from the retrieval pool").
func TestRetrieve_OnlyActiveSkills(t *testing.T) {
	const intent = "deploy the app to production"
	mk := func(status Status) Skill {
		return Skill{
			ID: "id-" + string(status), Name: "deploy app", Body: "steps to deploy the app",
			Status: status,
		}
	}
	for _, status := range []Status{StatusDraft, StatusShadow, StatusQuarantined, StatusArchived} {
		got := Retrieve([]Skill{mk(status)}, intent, 5, 0)
		if len(got) != 0 {
			t.Errorf("status %s: retrieved %d, want 0 (non-active must never inject)", status, len(got))
		}
	}
	// The same skill, active, IS retrieved.
	if got := Retrieve([]Skill{mk(StatusActive)}, intent, 5, 0); len(got) != 1 {
		t.Errorf("active matching skill: retrieved %d, want 1", len(got))
	}
}

// TestAutoQuarantine_RemovesFromRetrievalPool ties M387 to retrieval end-to-end:
// an active skill is retrievable; after it auto-quarantines on repeated failure
// it is gone from the pool — the "pulled from production" guarantee.
func TestAutoQuarantine_RemovesFromRetrievalPool(t *testing.T) {
	f, _ := newTestForge(t)
	id := activeSkill(t, f, "deploy app") // body "body of deploy app"

	intent := "deploy app now"
	all, err := f.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := Retrieve(all, intent, 5, 0); len(got) != 1 {
		t.Fatalf("before quarantine: retrieved %d, want 1 (active skill matches)", len(got))
	}

	// Fail it past the threshold → auto-quarantine.
	for i := 0; i < 3; i++ {
		f.RecordOutcome("run", []string{id}, false)
	}
	if statusOf(t, f, id) != StatusQuarantined {
		t.Fatalf("skill should be quarantined after 3 failures")
	}

	all, _ = f.List()
	if got := Retrieve(all, intent, 5, 0); len(got) != 0 {
		t.Errorf("after auto-quarantine: retrieved %d, want 0 (pulled from production)", len(got))
	}
}
