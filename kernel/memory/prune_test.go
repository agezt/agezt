// SPDX-License-Identifier: MIT

package memory

import (
	"math"
	"testing"
)

// TestPrune_RemovesOnlySoftDeleted asserts the prune pass (M857) reclaims
// tombstoned and superseded records but never touches active ones, and that the
// dry-run count matches what a real prune removes.
func TestPrune_RemovesOnlySoftDeleted(t *testing.T) {
	m, _ := newTestManager(t)

	// Two active records.
	keep1, _, _ := m.Remember("c", RememberSpec{Type: TypeFact, Subject: "a", Content: "alpha"})
	keep2, _, _ := m.Remember("c", RememberSpec{Type: TypeFact, Subject: "b", Content: "beta"})
	// One tombstoned.
	gone, _, _ := m.Remember("c", RememberSpec{Type: TypeFact, Subject: "c", Content: "gamma"})
	if ok, err := m.Forget("c", gone.ID); err != nil || !ok {
		t.Fatalf("forget: ok=%v err=%v", ok, err)
	}
	// One superseded (the old record is soft-deleted, the new one is active).
	old, _, _ := m.Remember("c", RememberSpec{Type: TypeFact, Subject: "d", Content: "delta v1"})
	if _, err := m.Supersede("c", old.ID, RememberSpec{Type: TypeFact, Subject: "d", Content: "delta v2"}); err != nil {
		t.Fatalf("supersede: %v", err)
	}

	cutoff := int64(math.MaxInt64) // everything is "old enough"

	hyg, err := m.Hygiene(cutoff)
	if err != nil {
		t.Fatalf("hygiene: %v", err)
	}
	if hyg.Tombstoned != 1 || hyg.Superseded != 1 {
		t.Fatalf("hygiene soft-deleted = tomb:%d super:%d, want 1/1", hyg.Tombstoned, hyg.Superseded)
	}
	if hyg.Prunable != 2 {
		t.Fatalf("prunable = %d, want 2", hyg.Prunable)
	}

	// Dry-run reports the candidates without deleting.
	n, err := m.Prune("c", cutoff, true)
	if err != nil || n != 2 {
		t.Fatalf("dry-run prune = %d (err %v), want 2", n, err)
	}
	if all, _ := m.All(); len(all) != 5 { // 2 active + 1 tomb + 1 super-old + 1 super-new
		t.Fatalf("dry-run deleted records: have %d, want 5", len(all))
	}

	// Real prune removes exactly the two soft-deleted, aged records.
	pruned, err := m.Prune("c", cutoff, false)
	if err != nil || pruned != 2 {
		t.Fatalf("prune = %d (err %v), want 2", pruned, err)
	}
	// The active records (incl. the supersession's successor) survive.
	for _, id := range []string{keep1.ID, keep2.ID} {
		if _, ok, _ := m.Get(id); !ok {
			t.Errorf("prune removed an active record %s", id)
		}
	}
	if _, ok, _ := m.Get(gone.ID); ok {
		t.Errorf("tombstoned record survived prune")
	}
	if _, ok, _ := m.Get(old.ID); ok {
		t.Errorf("superseded record survived prune")
	}
}

// TestPrune_RespectsAgeCutoff: a recent soft-delete is NOT pruned when the cutoff
// is in the past (recently forgotten records stay recoverable).
func TestPrune_RespectsAgeCutoff(t *testing.T) {
	m, _ := newTestManager(t)
	r, _, _ := m.Remember("c", RememberSpec{Type: TypeFact, Subject: "x", Content: "keep me a while"})
	m.Forget("c", r.ID)

	// Cutoff far in the past → nothing is old enough.
	if n, _ := m.Prune("c", 0, true); n != 0 {
		t.Errorf("recent soft-delete counted as prunable with past cutoff: %d", n)
	}
}
