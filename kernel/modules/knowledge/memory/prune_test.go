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
// is at-or-after LastSeenMS — i.e. when the record is not yet "old enough" relative
// to the cutoff. The original test used cutoff=0, which accidentally only worked
// because of the bug fixed by F1 (Prune's missing `<= 0` short-circuit). With the
// fix, "<= 0" means "match all soft-deleted regardless of age", so this assertion
// now uses an explicit positive cutoff equal to the pin-clock LastSeenMS.
func TestPrune_RespectsAgeCutoff(t *testing.T) {
	m, _ := newTestManager(t)
	r, _, _ := m.Remember("c", RememberSpec{Type: TypeFact, Subject: "x", Content: "keep me a while"})
	m.Forget("c", r.ID)

	// cuttoff == r.LastSeenMS (lastSeen pinned via fixedNow): "LastSeenMS < cutoff"
	// is false, so the soft-deleted record is NOT prunable.
	atLastSeen := fixedNow.UnixMilli()
	if n, _ := m.Prune("c", atLastSeen, true); n != 0 {
		t.Errorf("recent soft-delete counted as prunable with cutoff equal to LastSeenMS: %d", n)
	}
}

// TestPrune_NonPositiveCutoffMatchesHygiene pins the F1 regression: the sibling
// methods Hygiene and Prune must agree on the same olderThanMs value. Per the
// Hygiene docstring ("olderThanMs <= 0 counts all soft-deleted records as
// prunable"), Prune(<= 0) should also return ALL soft-deleted records, on the
// `tombstoned OR superseded` predicate, regardless of age. Before the fix,
// Prune(0) silently returned 0 because r.LastSeenMS < 0 is unsatisfiable.
func TestPrune_NonPositiveCutoffMatchesHygiene(t *testing.T) {
	m, _ := newTestManager(t)
	gone, _, _ := m.Remember("c", RememberSpec{Type: TypeFact, Subject: "a", Content: "alpha"})
	if ok, err := m.Forget("c", gone.ID); err != nil || !ok {
		t.Fatalf("forget: ok=%v err=%v", ok, err)
	}
	old, _, _ := m.Remember("c", RememberSpec{Type: TypeFact, Subject: "b", Content: "beta v1"})
	if _, err := m.Supersede("c", old.ID, RememberSpec{Type: TypeFact, Subject: "b", Content: "beta v2"}); err != nil {
		t.Fatalf("supersede: %v", err)
	}

	for _, cutoff := range []int64{0, -1, math.MinInt64} {
		hyg, err := m.Hygiene(cutoff)
		if err != nil {
			t.Fatalf("hygiene(%d): %v", cutoff, err)
		}
		if hyg.Prunable != 2 {
			t.Errorf("Hygiene(%d).Prunable = %d, want 2", cutoff, hyg.Prunable)
		}
		n, err := m.Prune("c", cutoff, true)
		if err != nil {
			t.Fatalf("Prune(%d, dry=true): %v", cutoff, err)
		}
		if n != hyg.Prunable {
			t.Errorf("Prune(%d, dry=true) = %d, want %d (must match Hygiene.Prunable)",
				cutoff, n, hyg.Prunable)
		}
	}

	// Real prune with a non-positive cutoff reclaims the dead weight.
	pruned, err := m.Prune("c", 0, false)
	if err != nil {
		t.Fatalf("Prune(0, dry=false): %v", err)
	}
	if pruned != 2 {
		t.Errorf("Prune(0, dry=false) reclaims %d, want 2", pruned)
	}
	if all, _ := m.All(); len(all) != 1 { // only the supersession's successor survives
		t.Errorf("Prune(0) left %d records, want 1 (the active successor)", len(all))
	}
}
