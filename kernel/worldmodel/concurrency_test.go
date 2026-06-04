// SPDX-License-Identifier: MIT

package worldmodel

import (
	"fmt"
	"sync"
	"testing"
)

// TestFileStore_ConcurrentAccessGuarded locks in the load-bearing sync.RWMutex
// that guards FileStore.entities and FileStore.relations. The world model is a
// long-lived singleton: the agent loop upserts entities/relations while
// control-plane handlers (`agt world`, resolve/neighbors) read it on other
// goroutines. A refactor dropping the mutex would reintroduce a
// `fatal error: concurrent map writes` daemon crash (and a torn disk snapshot,
// since every Put snapshots under the lock). This test drives all accessors —
// both the entity and relation maps — from many goroutines so the invariant
// fails loudly if the lock is ever removed.
//
// NB: -race is unavailable on the dev host (no cgo); this is a
// functional-consistency stress, not a race-detector run. A concurrent map
// write still fatals deterministically enough under 16×200 contention to catch
// a dropped lock; true race detection is delegated to a cgo-enabled CI.
func TestFileStore_ConcurrentAccessGuarded(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	const workers = 16
	const iters = 120

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				// Bounded id space so writers both insert and overwrite
				// under contention on each map.
				eid := fmt.Sprintf("ent-%d-%d", id, i%8)
				if err := s.PutEntity(Entity{
					ID:     eid,
					Kind:   "project",
					Name:   fmt.Sprintf("entity-%d-%d", id, i%8),
					Weight: 1.0,
				}); err != nil {
					t.Errorf("PutEntity: %v", err)
					return
				}
				rid := fmt.Sprintf("rel-%d-%d", id, i%8)
				if err := s.PutRelation(Relation{
					ID:     rid,
					From:   eid,
					Verb:   "depends_on",
					To:     fmt.Sprintf("ent-%d-%d", id, (i+1)%8),
					Weight: 1.0,
				}); err != nil {
					t.Errorf("PutRelation: %v", err)
					return
				}
				if _, _, err := s.GetEntity(eid); err != nil {
					t.Errorf("GetEntity: %v", err)
					return
				}
				if _, err := s.AllEntities(); err != nil {
					t.Errorf("AllEntities: %v", err)
					return
				}
				if _, err := s.AllRelations(); err != nil {
					t.Errorf("AllRelations: %v", err)
					return
				}
				_ = s.Count()
			}
		}(w)
	}
	wg.Wait()

	// Post-storm consistency: every entity AllEntities reports must be
	// individually Gettable (no half-written entry).
	ents, err := s.AllEntities()
	if err != nil {
		t.Fatalf("AllEntities after storm: %v", err)
	}
	if len(ents) == 0 {
		t.Fatal("expected entities after the write storm")
	}
	if got := s.Count(); got != len(ents) {
		t.Errorf("Count()=%d but AllEntities() returned %d", got, len(ents))
	}
	for _, e := range ents {
		got, ok, err := s.GetEntity(e.ID)
		if err != nil || !ok {
			t.Errorf("entity %q reported by AllEntities but GetEntity ok=%v err=%v", e.ID, ok, err)
			continue
		}
		if got.Name == "" {
			t.Errorf("entity %q has empty name after storm (torn write?)", e.ID)
		}
	}
	rels, err := s.AllRelations()
	if err != nil {
		t.Fatalf("AllRelations after storm: %v", err)
	}
	if len(rels) == 0 {
		t.Fatal("expected relations after the write storm")
	}
}
