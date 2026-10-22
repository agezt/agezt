// SPDX-License-Identifier: MIT

package skill

import "testing"

// Auto-quarantine fires when the failure rate REACHES the threshold:
// `if rate < f.aqFailureRate { return }` — i.e. rate >= aqFailureRate quarantines.
// TestRecordOutcome_AutoQuarantinesAfterThreshold drives a 100% rate and
// TestRecordOutcome_NoQuarantineWhenMostlySuccessful a ~23% rate, so neither lands on
// the threshold exactly; mutation testing (M506) showed `<` could weaken to `<=`
// (a skill sitting exactly at the failure-rate threshold would escape quarantine)
// undetected. With the defaults (aqMinFailures=3, aqFailureRate=0.5), 3 failures out of
// 6 is exactly 0.5 — and 0.5 is exactly representable, so the boundary is clean.
func TestRecordOutcome_QuarantinesAtExactFailureRate(t *testing.T) {
	f, _ := newTestForge(t)
	id := activeSkill(t, f, "balanced")

	// 3 successes first, then failures — so the min-failure-count guard (3) keeps the
	// skill active until the 3rd failure, at which point the rate is exactly 0.5.
	for i := 0; i < 3; i++ {
		f.RecordOutcome("s", []string{id}, true)
	}
	f.RecordOutcome("f1", []string{id}, false) // 3S/1F = 25%, 1 failure < min
	f.RecordOutcome("f2", []string{id}, false) // 3S/2F = 40%, 2 failures < min
	if got := statusOf(t, f, id); got != StatusActive {
		t.Fatalf("at 3S/2F (40%%, below the rate threshold and the count) status=%s, want still active", got)
	}

	f.RecordOutcome("f3", []string{id}, false) // 3S/3F = exactly 50% == aqFailureRate
	if got := statusOf(t, f, id); got != StatusQuarantined {
		t.Fatalf("at exactly the 50%% failure-rate threshold (3 failures of 6) the skill must quarantine, got %s", got)
	}
}
