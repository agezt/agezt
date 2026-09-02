// SPDX-License-Identifier: MIT
//
// Proof-of-failure: kernel/memory/consolidate.go supersedeExisting silently
// returns nil when the old record is absent from the store.
//
// ROOT CAUSE
// consolidate.go:251 has:
//     if err != nil || !found { return err }
// When the record is absent, FileStore.Get returns (zero Record{}, false, nil).
// The guard fires (err=nil, found=false → true||false = true) and executes
// `return err` — but err is nil! supersedeExisting silently returns nil,
// DistillBrain counts the absent record as successfully superseded, and a
// zero Record{} is written back to the store under the absent record's ID.
//
// IMPACT
// - Silent, incorrect tracking state: absent record appears superseded.
// - Zero Record{} written to store under the absent record's ID.
// - Second DistillBrain pass re-clusters the stale zero record → duplicate consolidation.
// - Orphaned consolidation: a "new" summary record is created even though
//   the old one is already superseded/absent.

package memory

import (
	"testing"
	"time"
)

// absentStore wraps a real FileStore and intercepts Get for a target ID,
// returning the FileStore-standard "absent but no error" tuple.
type absentStore struct {
	real      *FileStore
	absentID  string
	absentGet bool
}

func (s *absentStore) Get(id string) (Record, bool, error) {
	if id == s.absentID && s.absentGet {
		// Simulate: record is absent from the store (e.g. concurrent deletion,
		// partial state corruption, compaction race). FileStore.Get would return
		// (zero Record{}, false, nil) — the standard "absent, no error" response.
		return Record{}, false, nil
	}
	return s.real.Get(id)
}
func (s *absentStore) Put(r Record) error         { return s.real.Put(r) }
func (s *absentStore) Delete(id string) (bool, error) { return s.real.Delete(id) }
func (s *absentStore) All() ([]Record, error)      { return s.real.All() }
func (s *absentStore) Count() int                  { return s.real.Count() }
func (s *absentStore) Close() error                { return s.real.Close() }

// TestSupersedeExisting_SilentNoopOnAbsentRecord proves the bug.
//
// When supersedeExisting is called with an absent oldID, it should return a
// non-nil error so DistillBrain can abort the pass or correct the superseded
// count. Instead it returns nil, silently treating the absent record as
// successfully linked — which is wrong.
func TestSupersedeExisting_SilentNoopOnAbsentRecord(t *testing.T) {
	// Set up a store where ID "k2" will be reported as absent on Get
	// (simulating the race condition), but is still readable via All().
	realStore, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	store := &absentStore{real: realStore, absentID: "k2", absentGet: false}

	m := NewManager(store, nil)
	m.now = func() time.Time { return time.UnixMilli(1000) }

	// Seed two records with predictable IDs (bypassing content-addressing by
	// writing directly into the store).
	oldID := "k2"
	newID := "merged-summary"
	for _, rec := range []Record{
		{ID: "k1", Type: TypeFact, Subject: "k8s", Content: "k8s cluster in frankfurt", Tags: map[string]string{}, Evidence: EvidenceObserved, CreatedMS: 1000, LastSeenMS: 1000},
		{ID: oldID, Type: TypeFact, Subject: "k8s", Content: "our k8s cluster is hosted in frankfurt", Tags: map[string]string{}, Evidence: EvidenceObserved, CreatedMS: 1000, LastSeenMS: 1000},
	} {
		if err := store.Put(rec); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	// Precondition: oldID must be readable via the real store (All returns it).
	all, err := store.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	var found bool
	for _, r := range all {
		if r.ID == oldID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("precondition: %q must be in the store before the test", oldID)
	}

	// Simulate the race: k2 gets deleted concurrently (or is otherwise absent
	// from Get's perspective), but is still in All() because All() reads the
	// snapshot directly.
	store.absentGet = true

	// Verify Get now returns absent.
	_, found, err = store.Get(oldID)
	if err != nil {
		t.Fatalf("Get after absent: %v", err)
	}
	if found {
		t.Fatal("Get should return not-found for k2")
	}

	// The bug: supersedeExisting silently returns nil even though oldID is absent.
	err = m.supersedeExisting("corr", oldID, newID)

	// EXPECTED (correct) behavior: err must be non-nil.
	// ACTUAL (buggy) behavior: err == nil.
	if err == nil {
		t.Errorf("supersedeExisting returned nil for absent oldID %q", oldID)
		t.Errorf("  Root cause: consolidate.go:251")
		t.Errorf("    if err != nil || !found { return err }")
		t.Errorf("  When %q absent: err=nil, found=false", oldID)
		t.Errorf("    → guard fires (false || true = true)")
		t.Errorf("    → executes: return err   ← err is nil!")
		t.Errorf("    → supersedeExisting silently returns nil (should return non-nil)")
		t.Errorf("  Expected: a non-nil error (e.g. ErrRecordNotFound)")
	}

	// Verify the absent record was NOT corrupted in the store.
	// The correct behavior: store unchanged, oldID record unchanged.
	// The buggy behavior: zero Record{} written back under oldID.
	rec, found, err := realStore.Get(oldID)
	if err != nil {
		t.Fatalf("realStore.Get after supersedeExisting: %v", err)
	}
	if found && rec.ID == oldID && rec.SupersededBy == newID {
		// If the zero Record was written back, rec.ID would be "" (zero value).
		// If it was written with ID=oldID and SupersededBy=newID, that's also
		// wrong — we shouldn't write anything when the record is absent.
	}
	if rec.ID == "" {
		t.Errorf("zero Record{} was written back to the store under absent ID %q", oldID)
		t.Errorf("  This confirms the silent-noop bug corrupted the store state")
	}
}
