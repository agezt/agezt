// SPDX-License-Identifier: MIT

package skill

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// activeSkill creates a skill and promotes it draft→shadow→active.
func activeSkill(t *testing.T, f *Forge, name string) string {
	t.Helper()
	sk, _, err := f.Create("c", CreateSpec{Name: name, Body: "body of " + name})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.Promote("c", sk.ID); err != nil { // draft → shadow
		t.Fatalf("promote→shadow: %v", err)
	}
	if _, err := f.Promote("c", sk.ID); err != nil { // shadow → active
		t.Fatalf("promote→active: %v", err)
	}
	return sk.ID
}

func statusOf(t *testing.T, f *Forge, id string) Status {
	t.Helper()
	sk, found, err := f.Get(id)
	if err != nil || !found {
		t.Fatalf("Get(%s): found=%v err=%v", id, found, err)
	}
	return sk.Status
}

// lastQuarantineReason returns the reason of the (last) skill.quarantined event,
// or "" if none.
func lastQuarantineReason(j interface {
	Range(func(*event.Event) error) error
}) (corr, reason string) {
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindSkillQuarantined {
			var p struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			reason = p.Reason
			corr = e.CorrelationID
		}
		return nil
	})
	return corr, reason
}

// TestRecordOutcome_AutoQuarantinesAfterThreshold: an active skill that crosses
// the failure count + rate threshold is pulled from production automatically,
// journaling skill.quarantined with an auto reason carrying the run correlation.
func TestRecordOutcome_AutoQuarantinesAfterThreshold(t *testing.T) {
	f, j := newTestForge(t)
	id := activeSkill(t, f, "flaky")

	// Two failures: below the min-count threshold (3) → still active.
	f.RecordOutcome("run-1", []string{id}, false)
	f.RecordOutcome("run-2", []string{id}, false)
	if got := statusOf(t, f, id); got != StatusActive {
		t.Fatalf("after 2 failures status=%s, want still active", got)
	}
	// Third failure crosses 3 failures @ 100% rate → auto-quarantined.
	f.RecordOutcome("run-3", []string{id}, false)
	if got := statusOf(t, f, id); got != StatusQuarantined {
		t.Fatalf("after 3 failures status=%s, want quarantined", got)
	}
	corr, reason := lastQuarantineReason(j)
	if !strings.Contains(reason, "auto-quarantine") {
		t.Errorf("quarantine reason = %q, want an auto-quarantine reason", reason)
	}
	if corr != "run-3" {
		t.Errorf("quarantine correlation = %q, want run-3 (the failing run)", corr)
	}
}

// TestRecordOutcome_NoQuarantineWhenMostlySuccessful: a skill with many successes
// and few failures is below the RATE threshold and stays active.
func TestRecordOutcome_NoQuarantineWhenMostlySuccessful(t *testing.T) {
	f, _ := newTestForge(t)
	id := activeSkill(t, f, "reliable")
	for i := 0; i < 10; i++ {
		f.RecordOutcome("run", []string{id}, true)
	}
	for i := 0; i < 3; i++ { // 3 failures, but 3/13 ≈ 23% < 50%
		f.RecordOutcome("run", []string{id}, false)
	}
	if got := statusOf(t, f, id); got != StatusActive {
		t.Errorf("status=%s, want active (failure rate below threshold)", got)
	}
}

// TestRecordOutcome_OnlyActiveSkillsAreQuarantined: a shadow skill (still under
// evaluation, not in production) is never auto-quarantined by this path.
func TestRecordOutcome_OnlyActiveSkillsAreQuarantined(t *testing.T) {
	f, _ := newTestForge(t)
	sk, _, _ := f.Create("c", CreateSpec{Name: "shadowy", Body: "b"})
	if _, err := f.Promote("c", sk.ID); err != nil { // draft → shadow only
		t.Fatalf("promote→shadow: %v", err)
	}
	for i := 0; i < 5; i++ {
		f.RecordOutcome("run", []string{sk.ID}, false)
	}
	if got := statusOf(t, f, sk.ID); got != StatusShadow {
		t.Errorf("status=%s, want shadow (auto-quarantine is active-only)", got)
	}
}

// TestRecordOutcome_DisabledLeavesSkillActive: SetAutoQuarantine(0,…) disables the
// behaviour entirely.
func TestRecordOutcome_DisabledLeavesSkillActive(t *testing.T) {
	f, _ := newTestForge(t)
	f.SetAutoQuarantine(0, 0)
	id := activeSkill(t, f, "untouchable")
	for i := 0; i < 6; i++ {
		f.RecordOutcome("run", []string{id}, false)
	}
	if got := statusOf(t, f, id); got != StatusActive {
		t.Errorf("status=%s, want active (auto-quarantine disabled)", got)
	}
}
