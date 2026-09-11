// SPDX-License-Identifier: MIT

// Package workflow is the n8n-style workflow engine (M798): durable, named
// graphs of TYPED nodes — trigger, tool, llm, condition, transform, delay —
// wired by edges and carrying data between nodes with {{path}} templates.
// Unlike kernel/planner (intent-in, agent-loop-per-node), a workflow node is
// a precise, deterministic step: THIS tool with THESE args, THIS prompt to
// THIS model, THIS branch on THIS value. The graph is what you see on the
// console canvas; the engine (kernel/runtime) executes it under the same
// governance as everything else — tool nodes pass Edict, llm nodes ride the
// Governor, every step is journaled (workflow.*).
//
// Storage mirrors kernel/standing: one atomic JSON file, journaled CRUD.
package workflow

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/jsonstore"
	"github.com/agezt/agezt/kernel/ulid"
)

// Workflow Store: OpenStore + Save + Restore + SetEnabled + Remove + Get + find + List + Count + save.
// Code extracted from workflow.go during the Day-46 god-file split. Public API unchanged.

func OpenStore(dir string) (*Store, error) {
	s := &Store{now: time.Now}
	path, err := jsonstore.LoadFrom(dir, "workflows.json", &s.items)
	if err != nil {
		return nil, fmt.Errorf("workflow: %w", err)
	}
	s.path = path
	return s, nil
}

// Save upserts a workflow by name: a new name is created (id assigned,
// enabled by default), an existing one is replaced wholesale — the canvas
// always posts the complete graph. Identity/lifecycle fields (ID, CreatedMS,
// Enabled) survive an update. Returns the stored value + whether it was
// created.
func (s *Store) Save(w Workflow) (Workflow, bool, error) {
	w.Name = strings.TrimSpace(w.Name)
	if err := Validate(w); err != nil {
		return Workflow{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UnixMilli()
	for _, ex := range s.items {
		if ex.Name == w.Name {
			snapshot := *ex
			w.ID, w.CreatedMS, w.Enabled = ex.ID, ex.CreatedMS, ex.Enabled
			w.UpdatedMS = now
			*ex = w
			if err := s.save(); err != nil {
				*ex = snapshot
				return Workflow{}, false, err
			}
			return *ex, false, nil
		}
	}
	w.ID = ulid.New()
	w.Enabled = true
	w.CreatedMS = now
	w.UpdatedMS = now
	cp := w
	s.items = append(s.items, &cp)
	if err := s.save(); err != nil {
		s.items = s.items[:len(s.items)-1]
		return Workflow{}, false, err
	}
	return cp, true, nil
}

// Restore upserts a checkpointed workflow while preserving identity and enabled
// state. It is intentionally separate from Save, whose normal editor semantics
// preserve the current lifecycle fields instead of trusting posted ones.
func (s *Store) Restore(w Workflow) (Workflow, bool, error) {
	w.Name = strings.TrimSpace(w.Name)
	if err := Validate(w); err != nil {
		return Workflow{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UnixMilli()
	if w.ID == "" {
		w.ID = ulid.New()
	}
	if w.CreatedMS == 0 {
		w.CreatedMS = now
	}
	w.UpdatedMS = now
	for _, ex := range s.items {
		if ex.ID == w.ID || ex.Name == w.Name {
			snapshot := *ex
			*ex = w
			if err := s.save(); err != nil {
				*ex = snapshot
				return Workflow{}, false, err
			}
			return *ex, false, nil
		}
	}
	cp := w
	s.items = append(s.items, &cp)
	if err := s.save(); err != nil {
		s.items = s.items[:len(s.items)-1]
		return Workflow{}, false, err
	}
	return cp, true, nil
}

// SetEnabled flips trigger-arming for a workflow by id or name.
func (s *Store) SetEnabled(ref string, enabled bool) (Workflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.find(ref)
	if w == nil {
		return Workflow{}, ErrNotFound
	}
	prevEnabled, prevUpdated := w.Enabled, w.UpdatedMS
	w.Enabled = enabled
	w.UpdatedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		w.Enabled, w.UpdatedMS = prevEnabled, prevUpdated
		return Workflow{}, err
	}
	return *w, nil
}

// Remove deletes a workflow by id or name. Returns whether it existed.
func (s *Store) Remove(ref string) (Workflow, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, w := range s.items {
		if w.ID == ref || w.Name == ref {
			removed := s.items
			gone := *w
			s.items = append(append([]*Workflow{}, s.items[:i]...), s.items[i+1:]...)
			if err := s.save(); err != nil {
				s.items = removed
				return Workflow{}, false, err
			}
			return gone, true, nil
		}
	}
	return Workflow{}, false, nil
}

// Get returns one workflow by id or name.
func (s *Store) Get(ref string) (Workflow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w := s.find(ref); w != nil {
		return *w, true
	}
	return Workflow{}, false
}

func (s *Store) find(ref string) *Workflow {
	for _, w := range s.items {
		if w.ID == ref || w.Name == ref {
			return w
		}
	}
	return nil
}

// List returns all workflows, sorted by creation time then id.
func (s *Store) List() []Workflow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Workflow, 0, len(s.items))
	for _, w := range s.items {
		out = append(out, *w)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedMS != out[j].CreatedMS {
			return out[i].CreatedMS < out[j].CreatedMS
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Count returns the number of stored workflows.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

func (s *Store) save() error {
	return jsonstore.Save(s.path, s.items)
}
