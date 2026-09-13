// SPDX-License-Identifier: MIT
//
// toolforge Store API: the Store struct + Open + save (persistence glue)
// plus the lifecycle transitions (Add + Update + RecordTest + Promote +
// Quarantine + Remove) and the readers (Get + List + Active + Count).
// Extracted from toolforge.go during the Day-203 god-file split.
// Public API unchanged.
package toolforge

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/jsonstore"
	"github.com/agezt/agezt/kernel/ulid"
)

// Store is the persistent script-tool registry, a single JSON file rewritten
// atomically on change. Safe for concurrent use. Mirrors kernel/roster.Store.
type Store struct {
	path  string
	mu    sync.Mutex
	now   func() time.Time
	tools []*ScriptTool
}

// Open opens (or creates) the script-tool store under dir.
func Open(dir string) (*Store, error) {
	s := &Store{now: time.Now}
	path, err := jsonstore.LoadFrom(dir, "scripttools.json", &s.tools)
	if err != nil {
		return nil, fmt.Errorf("toolforge: %w", err)
	}
	s.path = path
	return s, nil
}

// Add validates and persists a new DRAFT script tool, assigning an id +
// timestamps. Caller-supplied ID/Status/Tested*/timestamps are ignored
// (kernel-assigned). The name must be unique across the forge.
func (s *Store) Add(st ScriptTool) (ScriptTool, error) {
	st.Name = strings.TrimSpace(st.Name)
	st.Language = strings.TrimSpace(st.Language)
	if err := Validate(st); err != nil {
		return ScriptTool{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ex := range s.tools {
		if ex.Name == st.Name {
			return ScriptTool{}, fmt.Errorf("toolforge: name %q already exists", st.Name)
		}
	}
	now := s.now().UnixMilli()
	st.ID = ulid.New()
	st.Status = StatusDraft
	st.TestedOK = false
	st.TestedMS = 0
	st.CreatedMS = now
	st.UpdatedMS = now
	cp := st
	s.tools = append(s.tools, &cp)
	if err := s.save(); err != nil {
		s.tools = s.tools[:len(s.tools)-1]
		return ScriptTool{}, err
	}
	return cp, nil
}

// Update applies edits to a script tool's mutable fields via mutate,
// re-validates, and persists. Identity and lifecycle fields — ID, Name,
// CreatedMS, Status, TestedOK/TestedMS — are preserved regardless of what
// mutate does, EXCEPT that changing Code or Language demotes the tool to
// draft and clears its test record: only tested code is ever live. Rolled
// back in memory on validation/save failure.
func (s *Store) Update(ref string, mutate func(*ScriptTool)) (ScriptTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.find(ref)
	if st == nil {
		return ScriptTool{}, ErrNotFound
	}
	snapshot := *st
	if err := safeCall(func() error { mutate(st); return nil }); err != nil {
		return ScriptTool{}, fmt.Errorf("toolforge: mutate panicked: %w", err)
	}
	// Protect identity + lifecycle fields from the mutator: the name is the
	// tool's ADDRESS, and status/test records move only through their own
	// governed transitions.
	st.ID, st.Name, st.CreatedMS = snapshot.ID, snapshot.Name, snapshot.CreatedMS
	st.Status, st.TestedOK, st.TestedMS = snapshot.Status, snapshot.TestedOK, snapshot.TestedMS
	st.Language = strings.TrimSpace(st.Language)
	if st.Code != snapshot.Code || st.Language != snapshot.Language {
		st.Status = StatusDraft
		st.TestedOK = false
		st.TestedMS = 0
	}
	st.UpdatedMS = s.now().UnixMilli()
	if err := Validate(*st); err != nil {
		*st = snapshot
		return ScriptTool{}, err
	}
	if err := s.save(); err != nil {
		*st = snapshot
		return ScriptTool{}, err
	}
	return *st, nil
}

// RecordTest stamps the outcome of a sandbox test of the CURRENT code.
func (s *Store) RecordTest(ref string, ok bool) (ScriptTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.find(ref)
	if st == nil {
		return ScriptTool{}, ErrNotFound
	}
	prevOK, prevMS := st.TestedOK, st.TestedMS
	st.TestedOK = ok
	st.TestedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		st.TestedOK, st.TestedMS = prevOK, prevMS
		return ScriptTool{}, err
	}
	return *st, nil
}

// Promote moves a draft or quarantined tool to ACTIVE — from then on every
// run is offered it as forge_<name>. Refused with ErrUntested unless the
// current code has a passing test on record.
func (s *Store) Promote(ref string) (ScriptTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.find(ref)
	if st == nil {
		return ScriptTool{}, ErrNotFound
	}
	if st.Status == StatusActive {
		return ScriptTool{}, fmt.Errorf("toolforge: %s is already active", st.Name)
	}
	if !st.TestedOK {
		return ScriptTool{}, ErrUntested
	}
	prevStatus, prevUpdated := st.Status, st.UpdatedMS
	st.Status = StatusActive
	st.UpdatedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		st.Status, st.UpdatedMS = prevStatus, prevUpdated
		return ScriptTool{}, err
	}
	return *st, nil
}

// Quarantine pulls an ACTIVE tool from production — the kill switch. The
// test record survives, so an un-edited tool can be re-promoted directly.
func (s *Store) Quarantine(ref string) (ScriptTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.find(ref)
	if st == nil {
		return ScriptTool{}, ErrNotFound
	}
	if st.Status != StatusActive {
		return ScriptTool{}, fmt.Errorf("toolforge: %s is %s, not active", st.Name, st.Status)
	}
	prevStatus, prevUpdated := st.Status, st.UpdatedMS
	st.Status = StatusQuarantined
	st.UpdatedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		st.Status, st.UpdatedMS = prevStatus, prevUpdated
		return ScriptTool{}, err
	}
	return *st, nil
}

// Remove deletes a script tool by id or name. Returns whether it existed.
func (s *Store) Remove(ref string) (ScriptTool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, st := range s.tools {
		if st.ID == ref || st.Name == ref {
			removed := s.tools
			gone := *st
			s.tools = append(append([]*ScriptTool{}, s.tools[:i]...), s.tools[i+1:]...)
			if err := s.save(); err != nil {
				s.tools = removed // restore: disk write failed, keep the tool
				return ScriptTool{}, false, err
			}
			return gone, true, nil
		}
	}
	return ScriptTool{}, false, nil
}

// Get returns one script tool by id or name.
func (s *Store) Get(ref string) (ScriptTool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.find(ref); st != nil {
		return *st, true
	}
	return ScriptTool{}, false
}

// find returns the live pointer for an id or name. Caller holds s.mu.
func (s *Store) find(ref string) *ScriptTool {
	for _, st := range s.tools {
		if st.ID == ref || st.Name == ref {
			return st
		}
	}
	return nil
}

// List returns all script tools, sorted by creation time then id.
func (s *Store) List() []ScriptTool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ScriptTool, 0, len(s.tools))
	for _, st := range s.tools {
		out = append(out, *st)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedMS != out[j].CreatedMS {
			return out[i].CreatedMS < out[j].CreatedMS
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Active returns only the tools currently offered to runs, sorted like List.
func (s *Store) Active() []ScriptTool {
	all := s.List()
	out := all[:0]
	for _, st := range all {
		if st.Status == StatusActive {
			out = append(out, st)
		}
	}
	return out
}

// Count returns the number of script tools.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tools)
}

func (s *Store) save() error {
	return jsonstore.Save(s.path, s.tools)
}
