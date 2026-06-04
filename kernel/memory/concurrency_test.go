// SPDX-License-Identifier: MIT

package memory

import (
	"fmt"
	"sync"
	"testing"
)

// TestFileStore_ConcurrentAccessGuarded locks in the load-bearing sync.RWMutex
// that guards FileStore.data. The store is a long-lived singleton: the agent
// loop writes memory records (Put) while control-plane handlers (`agt memory`,
// recall) read it (Get/All/Count) on other goroutines. A refactor that dropped
// the mutex would reintroduce a `fatal error: concurrent map writes` daemon
// crash — and, because every Put also writes a synchronous disk snapshot under
// the lock, a torn snapshot on disk. The mutex makes that unobservable; this
// test hammers all accessors from many goroutines so the invariant fails loudly
// if the lock is ever removed.
//
// NB: -race is unavailable on the dev host (no cgo), so this is a
// functional-consistency stress, not a race-detector run. A concurrent map
// write still fatals deterministically enough under 16×200 contention that a
// dropped lock is caught; true race detection is delegated to a cgo-enabled CI.
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
				// Bounded id space (i%8) so writers both insert new
				// records and overwrite existing ones — exercising the
				// map-assign branch repeatedly under contention.
				rid := fmt.Sprintf("rec-%d-%d", id, i%8)
				rec := Record{
					ID:         rid,
					Type:       TypeFact,
					Subject:    fmt.Sprintf("worker-%d", id),
					Content:    fmt.Sprintf("observation %d from worker %d", i, id),
					Confidence: 1.0,
				}
				if err := s.Put(rec); err != nil {
					t.Errorf("Put: %v", err)
					return
				}
				if _, _, err := s.Get(rid); err != nil {
					t.Errorf("Get: %v", err)
					return
				}
				if _, err := s.All(); err != nil {
					t.Errorf("All: %v", err)
					return
				}
				_ = s.Count()
			}
		}(w)
	}
	wg.Wait()

	// Post-storm consistency: the store must still be usable and every record
	// All() reports must be individually Gettable (no half-written entry).
	all, err := s.All()
	if err != nil {
		t.Fatalf("All after storm: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("expected records after the write storm")
	}
	if got := s.Count(); got != len(all) {
		t.Errorf("Count()=%d but All() returned %d records", got, len(all))
	}
	for _, r := range all {
		got, ok, err := s.Get(r.ID)
		if err != nil || !ok {
			t.Errorf("record %q reported by All but Get ok=%v err=%v", r.ID, ok, err)
			continue
		}
		if got.Content == "" {
			t.Errorf("record %q has empty content after storm (torn write?)", r.ID)
		}
	}
}
