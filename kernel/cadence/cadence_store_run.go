// SPDX-License-Identifier: MIT

// Cadence store runtime: Remove + List + Get + RunNow + Due + CompleteFiring + SetAssure.
// Code extracted from cadence_store.go during the Day-58 god-file split. Public API unchanged.
package cadence


import (
	"sort"
	"time"
)


func (s *Store) Remove(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.entries {
		if e.ID == id {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return true, s.save()
		}
	}
	return false, nil
}

// List returns a copy of all entries, sorted by creation time.
func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedUnix < out[j].CreatedUnix })
	return out
}

// Get returns the entry with id.
func (s *Store) Get(id string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			return *e, true
		}
	}
	return Entry{}, false
}

// RunNow marks the entry due immediately (the next tick fires it). Returns
// whether the entry exists.
func (s *Store) RunNow(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.ID == id {
			e.NextRunUnix = 0 // epoch → always due
			e.Enabled = true
			return true, s.save()
		}
	}
	return false, nil
}

// Due returns the entries whose next-run time has arrived. Disabled entries are
// never due.
//
// Recurring entries (interval/daily/window) advance eagerly: their NextRunUnix is
// moved to the next slot and persisted here, before the run launches. A crash
// during the run therefore SKIPS that one slot (at-most-once), which self-corrects
// at the next slot — re-running a stale recurring slot after a restart is more
// disruptive than skipping it.
//
// One-shot entries (ModeOnce) are crash-safe (at-least-once): Due returns them but
// does NOT remove or advance them. The entry stays in the store, enabled and due,
// until its run COMPLETES, at which point the engine calls CompleteFiring to remove
// it (M199). So a crash mid-run leaves the one-shot in place to re-fire on restart
// instead of silently vanishing. The engine's in-flight guard (the running map)
// prevents a duplicate fire across ticks while the single run is still going.
func (s *Store) Due(now time.Time) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var due []Entry
	changed := false
	for _, e := range s.entries {
		if !e.Enabled {
			continue
		}
		if now.Unix() < e.NextRunUnix {
			continue
		}
		if e.Mode == ModeOnce || e.Mode == ModeContinuous {
			// Crash-safe one-shot AND completion-anchored continuous: leave
			// NextRunUnix untouched here. The engine's in-flight guard stops a
			// second fire while the run is live, and CompleteFiring re-anchors a
			// continuous entry forward (or removes a one-shot) once it finishes.
			due = append(due, *e)
			continue
		}
		e.LastRunUnix = now.Unix()
		e.NextRunUnix = e.advance(now)
		changed = true
		due = append(due, *e)
	}
	if changed {
		_ = s.save()
	}
	return due
}

// CompleteFiring is called by the engine after a fired entry's run has finished.
// For a one-shot (ModeOnce) it removes the entry from the store and persists — this
// is what makes one-shots crash-safe: the entry survives in the store (enabled and
// due) for the entire duration of its run, so a crash before completion re-fires it
// on restart, and it is removed only once the run has actually run to completion
// (whether it succeeded or errored, so a permanently-failing one-shot cannot
// retry-storm). For recurring entries this is a no-op: Due already advanced them.
// Returns whether an entry was removed.
func (s *Store) CompleteFiring(id string, now time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.entries {
		if e.ID != id {
			continue
		}
		switch e.Mode {
		case ModeOnce:
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return true, s.save()
		case ModeContinuous:
			// Completion-anchored loop: schedule the next cycle `cooldown` after
			// this run finished, so cycles never overlap and the agent runs
			// forever with a steady breather between cycles (M646). Count the cycle
			// just lived — this is the loop's heartbeat (M650).
			s.entries[i].Fires++
			s.entries[i].LastRunUnix = now.Unix()
			s.entries[i].NextRunUnix = now.Add(e.safeInterval()).Unix()
			return false, s.save()
		default:
			// Recurring (interval/daily/window): Due already advanced NextRunUnix
			// before the run; here we only record that a firing completed (M650).
			s.entries[i].Fires++
			return false, s.save()
		}
	}
	return false, nil
}

// SetAssure sets an entry's do-it-for-sure attempt budget (M654): n > 0 makes
// each firing run-verify-retry up to n times; n <= 0 clears it back to a single
// pass. Returns whether an entry with that id existed.
func (s *Store) SetAssure(id string, n int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entries {
		if s.entries[i].ID != id {
			continue
		}
		if n < 0 {
			n = 0
		}
		s.entries[i].Assure = n
		return true, s.save()
	}
	return false, nil
}

// SyncEnv replaces all SourceEnv entries with the given jobs (idempotent across
// restarts): operator-managed entries are untouched, and removing a job from
// AGEZT_SCHEDULE removes its entry on the next start.