// SPDX-License-Identifier: MIT

package skill

import (
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// TestReassignSharesAndReassigns exercises the M942 ownership valve: a skill's
// owning agent can be set (skill.reassigned) and cleared back to the shared
// pool (skill.shared), it is a no-op when already on the target owner, and an
// unknown id errors.
func TestReassignSharesAndReassigns(t *testing.T) {
	f, j := newTestForge(t)
	sk, created, err := f.Create("c", CreateSpec{Name: "deploy", Body: "steps"})
	if err != nil || !created {
		t.Fatalf("create: created=%v err=%v", created, err)
	}
	if sk.Agent != "" {
		t.Fatalf("new skill should be shared (Agent empty), got %q", sk.Agent)
	}

	// shared → private to "alice"
	got, found, err := f.Reassign("c", sk.ID, "alice")
	if err != nil || !found {
		t.Fatalf("reassign to alice: found=%v err=%v", found, err)
	}
	if got.Agent != "alice" {
		t.Errorf("Agent = %q, want alice", got.Agent)
	}
	if n := countKind(t, j, event.KindSkillReassigned); n != 1 {
		t.Errorf("skill.reassigned = %d, want 1", n)
	}

	// no-op when already owned by the target — no new event, no churn
	if _, found, err = f.Reassign("c", sk.ID, "alice"); err != nil || !found {
		t.Fatalf("reassign no-op: found=%v err=%v", found, err)
	}
	if n := countKind(t, j, event.KindSkillReassigned); n != 1 {
		t.Errorf("no-op should not emit another skill.reassigned (got %d)", n)
	}

	// private → shared clears the owner and emits skill.shared (not reassigned)
	got, found, err = f.Reassign("c", sk.ID, "")
	if err != nil || !found {
		t.Fatalf("share: found=%v err=%v", found, err)
	}
	if got.Agent != "" {
		t.Errorf("Agent = %q, want empty after share", got.Agent)
	}
	if n := countKind(t, j, event.KindSkillShared); n != 1 {
		t.Errorf("skill.shared = %d, want 1", n)
	}
	if n := countKind(t, j, event.KindSkillReassigned); n != 1 {
		t.Errorf("sharing must not emit skill.reassigned (got %d)", n)
	}

	// unknown id errors and emits nothing
	if _, found, err = f.Reassign("c", "does-not-exist", "alice"); err == nil {
		t.Errorf("reassign of unknown id should error, got found=%v", found)
	}
}
