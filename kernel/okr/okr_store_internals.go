// SPDX-License-Identifier: MIT

// okr_store_internals.go holds the *private* Store methods
// (mutate / find / saveLocked) and the package-internal
// helpers (findKR / cloneObjective) used by the public CRUD
// surface in okr_store.go. Lives in its own file so the
// public API reads linearly. Carved out during the Day-211
// god-file split (#115).
package okr

import (
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/jsonstore"
)

func (s *Store) mutate(id string, fn func(*Objective, int64) error, now time.Time) (Objective, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Objective{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.find(id)
	if o == nil {
		return Objective{}, ErrNotFound
	}
	prev := cloneObjective(*o)
	ts := now.UnixMilli()
	if err := fn(o, ts); err != nil {
		*o = prev
		return Objective{}, err
	}
	o.UpdatedMS = ts
	if err := s.saveLocked(); err != nil {
		*o = prev
		return Objective{}, err
	}
	return cloneObjective(*o), nil
}

func (s *Store) find(id string) *Objective {
	for _, o := range s.objectives {
		if o.ID == id {
			return o
		}
	}
	return nil
}

func findKR(o *Objective, krID string) *KeyResult {
	for i := range o.KeyResults {
		if o.KeyResults[i].ID == krID {
			return &o.KeyResults[i]
		}
	}
	return nil
}

func (s *Store) saveLocked() error {
	return jsonstore.Save(s.path, diskState{Version: storeVersion, Objectives: s.objectives})
}

func cloneObjective(o Objective) Objective {
	krs := make([]KeyResult, len(o.KeyResults))
	for i, kr := range o.KeyResults {
		kr.TaskIDs = append([]string(nil), kr.TaskIDs...)
		krs[i] = kr
	}
	o.KeyResults = krs
	return o
}
