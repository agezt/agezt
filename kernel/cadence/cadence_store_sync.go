// SPDX-License-Identifier: MIT

// Cadence store sync: SyncEnv + Count + save.
// Code extracted from cadence_store.go during the Day-58 god-file split. Public API unchanged.
package cadence


import (
	"github.com/agezt/agezt/kernel/jsonstore"
	"github.com/agezt/agezt/kernel/ulid"
	"strings"
	"time"
)


func (s *Store) SyncEnv(jobs []Job, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.entries[:0:0]
	for _, e := range s.entries {
		if e.Source != SourceEnv {
			kept = append(kept, e)
		}
	}
	for _, j := range jobs {
		iv := j.Interval
		if iv < MinInterval {
			iv = MinInterval
		}
		kept = append(kept, &Entry{
			ID:          "sched-" + ulid.New(),
			Intent:      strings.TrimSpace(j.Intent),
			IntervalSec: int64(iv / time.Second),
			Model:       strings.TrimSpace(j.Model),
			Source:      SourceEnv,
			Enabled:     true,
			CreatedUnix: now.Unix(),
			NextRunUnix: now.Add(iv).Unix(),
		})
	}
	s.entries = kept
	return s.save()
}

// Count returns the number of entries.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// save writes the entries atomically (temp file + rename). Caller holds s.mu.
func (s *Store) save() error {
	return jsonstore.Save(s.path, s.entries)
}

// --- Engine ---

// RunFunc executes one due schedule through the target dispatcher. The engine
// calls it on its own goroutine; a returned error is logged, not fatal. id is
// the firing schedule's entry id (M55) so the caller can attribute the run to
// its schedule (e.g. stamp it on the schedule.fired event). intent is kept for
// the legacy agent-task label and for backward-compatible stores.
