// SPDX-License-Identifier: MIT

// Package taste is AGEZT's curated "what good looks like" store: a small set of
// operator-authored exemplars that are injected into runs before the model acts,
// so output quality is shaped by concrete examples of good work rather than left
// to chance.
//
// It is deliberately distinct from memory (facts the agent recalls) and skills
// (procedures it follows): an exemplar is a taste anchor — a sample answer, a
// well-formed artifact, a house-style snippet. The store is pure data; the
// runtime selects the exemplars relevant to a run's scope and prepends them.
//
// The lifecycle ROWS — the exemplar, the create input, the filter — and the
// port through which the library is read now live in the contract layer
// (kernel/contract/tasteapi): the kernel selects exemplars for a run, the app
// layer curates them, and the daemon owns the file — none of them may name
// this module. The names below stay as aliases, so every existing caller is
// unchanged and there is one row and one spelling of it.
package taste

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/contract/tasteapi"
	"github.com/agezt/agezt/kernel/platform/filestore"
	"github.com/agezt/agezt/kernel/ulid"
)

const (
	storeVersion = 1
	maxExemplars = 2000
	maxBodyBytes = 8000
)

// ErrNotFound is returned when an operation names an exemplar that does not
// exist.
//
// The sentinel itself lives in the contract layer (kernel/contract/tasteapi);
// the name stays here as an alias, so every existing caller is unchanged.
var ErrNotFound = tasteapi.ErrNotFound

// Exemplar is one "what good looks like" anchor.
//
// The row itself lives in the contract layer (kernel/contract/tasteapi); the
// name stays here as an alias, so every existing caller is unchanged.
type Exemplar = tasteapi.Exemplar

// CreateSpec is the input to Create.
//
// The row itself lives in the contract layer (kernel/contract/tasteapi); the
// name stays here as an alias, so every existing caller is unchanged.
type CreateSpec = tasteapi.CreateSpec

// Filter narrows a listing.
//
// The row itself lives in the contract layer (kernel/contract/tasteapi); the
// name stays here as an alias, so every existing caller is unchanged.
type Filter = tasteapi.Filter

// Store is the file-backed exemplar library. It satisfies the contract's
// port, so the layers above it never name this module.
var _ tasteapi.Store = (*Store)(nil)

type Store struct {
	path      string
	mu        sync.Mutex
	exemplars []*Exemplar
}

type diskState struct {
	Version   int         `json:"version"`
	Exemplars []*Exemplar `json:"exemplars"`
}

// OpenStore opens or creates the taste store under dir.
func OpenStore(dir string) (*Store, error) {
	s := &Store{}
	var st diskState
	path, err := filestore.LoadFrom(dir, "taste.json", &st)
	if err != nil {
		return nil, fmt.Errorf("taste: %w", err)
	}
	s.path = path
	for _, e := range st.Exemplars {
		if e == nil {
			continue
		}
		cp := cloneExemplar(*e)
		s.exemplars = append(s.exemplars, &cp)
	}
	return s, nil
}

// Create inserts an exemplar.
//
// The clock is the CALLER's: the port takes nowMS so the contract layer never
// names one (W3.52).
func (s *Store) Create(spec CreateSpec, nowMS int64) (Exemplar, error) {
	spec.Title = strings.TrimSpace(spec.Title)
	spec.Body = strings.TrimSpace(spec.Body)
	spec.Scope = strings.TrimSpace(spec.Scope)
	if spec.Title == "" || spec.Body == "" {
		return Exemplar{}, errors.New("taste: title and body required")
	}
	if len(spec.Body) > maxBodyBytes {
		return Exemplar{}, fmt.Errorf("taste: body exceeds %d bytes", maxBodyBytes)
	}
	ts := nowMS
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.exemplars) >= maxExemplars {
		return Exemplar{}, fmt.Errorf("taste: at most %d exemplars", maxExemplars)
	}
	e := Exemplar{
		ID:        ulid.New(),
		Title:     spec.Title,
		Body:      spec.Body,
		Scope:     spec.Scope,
		Tags:      cleanStrings(spec.Tags),
		CreatedMS: ts,
		UpdatedMS: ts,
	}
	s.exemplars = append(s.exemplars, &e)
	if err := s.saveLocked(); err != nil {
		s.exemplars = s.exemplars[:len(s.exemplars)-1]
		return Exemplar{}, err
	}
	return cloneExemplar(e), nil
}

func (s *Store) Get(id string) (Exemplar, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.exemplars {
		if e.ID == id {
			return cloneExemplar(*e), true
		}
	}
	return Exemplar{}, false
}

// List returns exemplars matching the filter, newest first.
func (s *Store) List(f Filter) []Exemplar {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Exemplar, 0, len(s.exemplars))
	for _, e := range s.exemplars {
		if f.Scope != "" && !strings.EqualFold(e.Scope, f.Scope) {
			continue
		}
		if f.Tag != "" && !hasTag(e.Tags, f.Tag) {
			continue
		}
		out = append(out, cloneExemplar(*e))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedMS != out[j].UpdatedMS {
			return out[i].UpdatedMS > out[j].UpdatedMS
		}
		return out[i].ID < out[j].ID
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out
}

// ForScope returns the exemplars that apply to a run with the given scope: every
// global exemplar (empty Scope) plus those whose Scope matches, newest first and
// capped at limit. This is the selection the runtime injects before a run.
func (s *Store) ForScope(scope string, limit int) []Exemplar {
	scope = strings.TrimSpace(scope)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Exemplar, 0, len(s.exemplars))
	for _, e := range s.exemplars {
		if e.Scope == "" || (scope != "" && strings.EqualFold(e.Scope, scope)) {
			out = append(out, cloneExemplar(*e))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		// Scoped exemplars first (more specific), then newest.
		si, sj := out[i].Scope != "", out[j].Scope != ""
		if si != sj {
			return si
		}
		if out[i].UpdatedMS != out[j].UpdatedMS {
			return out[i].UpdatedMS > out[j].UpdatedMS
		}
		return out[i].ID < out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Delete removes an exemplar by id.
func (s *Store) Delete(id string) error {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, e := range s.exemplars {
		if e.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	removed := s.exemplars[idx]
	s.exemplars = append(s.exemplars[:idx], s.exemplars[idx+1:]...)
	if err := s.saveLocked(); err != nil {
		// Rollback: pre-allocate a slice of the exact size and copy both halves.
		// This avoids the buggy append(nil)+copy approach which silently drops
		// the last element when the backing array is shared (cap==len).
		n := len(s.exemplars) + 1 // original size before the failed delete
		restored := make([]*Exemplar, n)
		copy(restored, s.exemplars[:idx])
		restored[idx] = removed
		copy(restored[idx+1:], s.exemplars[idx:])
		s.exemplars = restored
		return err
	}
	return nil
}

func (s *Store) saveLocked() error {
	return filestore.Save(s.path, diskState{Version: storeVersion, Exemplars: s.exemplars})
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

func cleanStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func cloneExemplar(e Exemplar) Exemplar {
	e.Tags = append([]string(nil), e.Tags...)
	return e
}
