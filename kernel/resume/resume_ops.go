// SPDX-License-Identifier: MIT

// Package resume: CRUD + state-transition methods (Put + putLocked +
// Snapshot + Get + getLocked + List + Delete + MarkSuspendedAll +
// IncrementAttempt + Quarantine).
// Split from resume.go during Day 211 god-file refactor (#48).
// Public API unchanged.
package resume

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

func (s *Store) Put(t *Ticket) error {
	if t == nil || t.Corr == "" {
		return errors.New("resume: ticket requires a correlation id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.putLocked(t)
}

func (s *Store) putLocked(t *Ticket) error {
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = StatusActive
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("resume: marshal %s: %w", t.Corr, err)
	}
	if len(b) > s.maxBytes && len(t.Messages) > 0 {
		// Too large: drop the conversation snapshot but keep the dispatch
		// metadata so the run can still resume by intent-replay.
		t.Messages = nil
		t.Iter = 0
		t.SnapshotDropped = true
		if b, err = json.MarshalIndent(t, "", "  "); err != nil {
			return fmt.Errorf("resume: marshal %s: %w", t.Corr, err)
		}
	}
	return writeAtomic(s.path(t.Corr), b)
}

// Snapshot refreshes the conversation snapshot of an existing ticket, preserving
// its status and dispatch metadata. No-op (returns nil) if the ticket is gone —
// the run start writes the ticket first, and a race where it was just deleted on
// clean termination must not resurrect it.
func (s *Store) Snapshot(corr string, msgs []agent.Message, iter int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok, err := s.getLocked(corr)
	if err != nil || !ok {
		return err
	}
	t.Messages = msgs
	t.Iter = iter
	t.SnapshotDropped = false
	return s.putLocked(t)
}

// Get loads one ticket. ok is false if no such ticket exists.
func (s *Store) Get(corr string) (*Ticket, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(corr)
}

func (s *Store) getLocked(corr string) (*Ticket, bool, error) {
	b, err := os.ReadFile(s.path(corr))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var t Ticket
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, false, fmt.Errorf("resume: unmarshal %s: %w", corr, err)
	}
	return &t, true, nil
}

// List returns every ticket in the directory (quarantine excluded), sorted by
// CreatedAt so resume is deterministic. A corrupt file is skipped, not fatal.
func (s *Store) List() ([]*Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []*Ticket
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var t Ticket
		if json.Unmarshal(b, &t) != nil || t.Corr == "" {
			continue
		}
		out = append(out, &t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// Delete removes a ticket. Absent is not an error (idempotent — RunWith's defer
// and a resumer cleanup can both fire).
func (s *Store) Delete(corr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path(corr))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// MarkSuspendedAll flips every active ticket to suspended — the shutdown signal
// that these runs are resume candidates, not completed work. Returns the number
// marked. Tickets already suspended (a prior partial shutdown) are left as-is.
func (s *Store) MarkSuspendedAll() (int, error) {
	tickets, err := s.List()
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, t := range tickets {
		if t.Status == StatusSuspended {
			continue
		}
		t.Status = StatusSuspended
		if err := s.putLocked(t); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// IncrementAttempt bumps and durably persists a ticket's attempt counter,
// returning the new count. The resumer MUST call this and observe the fsync
// BEFORE re-dispatching: a resume that hard-crashes the daemon must still have
// recorded the attempt, or the crash-loop guard never trips and the watchdog
// eventually gives up — leaving the daemon down permanently.
func (s *Store) IncrementAttempt(corr string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok, err := s.getLocked(corr)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, os.ErrNotExist
	}
	t.Attempts++
	if err := s.putLocked(t); err != nil {
		return t.Attempts, err
	}
	return t.Attempts, nil
}

// Quarantine moves a poison ticket out of the scan directory so it is never
// re-dispatched but remains on disk for a postmortem. Used when a ticket has
// exceeded its attempt cap or is not resumable.
func (s *Store) Quarantine(corr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.path(corr)
	dst := filepath.Join(s.quarDir, fmt.Sprintf("%s.%d.json", safeName(corr), time.Now().UTC().UnixNano()))
	err := os.Rename(src, dst)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// writeAtomic writes b to path via a unique temp file that is fsynced before
// the rename, so a crash never yields a torn or unsynced ticket (see
// internal/atomicfile).
