// SPDX-License-Identifier: MIT

// workboard_store_internals.go holds the *private* Store
// methods (mutate / find / saveLocked / dependsOnLocked) and
// the dependencySatisfied helper used by the public CRUD
// surface in workboard_store_more.go. Lives in its own file
// so the public API reads linearly. Carved out during the
// Day-211 god-file split (#114).
package workboard

import (
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/jsonstore"
)


func (s *Store) mutate(id string, fn func(*Task, int64) error, now time.Time) (Task, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Task{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil {
		return Task{}, ErrNotFound
	}
	prev := cloneTask(*t)
	ts := now.UnixMilli()
	if err := fn(t, ts); err != nil {
		*t = prev
		return Task{}, err
	}
	t.UpdatedMS = ts
	if err := s.saveLocked(); err != nil {
		*t = prev
		return Task{}, err
	}
	return cloneTask(*t), nil
}

func (s *Store) find(id string) *Task {
	for _, t := range s.tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (s *Store) saveLocked() error {
	return jsonstore.Save(s.path, diskState{Version: storeVersion, Tasks: s.tasks})
}

func (s *Store) dependsOnLocked(startID, targetID string, seen map[string]bool) bool {
	if startID == targetID {
		return true
	}
	if seen[startID] {
		return false
	}
	seen[startID] = true
	t := s.find(startID)
	if t == nil {
		return false
	}
	for _, d := range t.Dependencies {
		if s.dependsOnLocked(d.ID, targetID, seen) {
			return true
		}
	}
	return false
}

func dependencySatisfied(t Task) bool {
	return t.Status == StatusDone || t.CompletedMS > 0
}
