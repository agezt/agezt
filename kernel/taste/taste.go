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
package taste

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/ulid"
)

const (
	storeVersion = 1
	maxExemplars = 2000
	maxBodyBytes = 8000
)

var ErrNotFound = errors.New("taste: not found")

// Exemplar is one "what good looks like" anchor. Body is the example itself
// (a sample answer, snippet, or artifact); Scope narrows where it applies: an
// empty Scope means every run, otherwise it matches a run's agent slug (or a
// free-form tag the operator dispatches under).
type Exemplar struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Scope     string   `json:"scope,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	CreatedMS int64    `json:"created_ms"`
	UpdatedMS int64    `json:"updated_ms"`
}

type CreateSpec struct {
	Title string
	Body  string
	Scope string
	Tags  []string
}

type Filter struct {
	Scope string
	Tag   string
	Limit int
}

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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("taste: mkdir %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, "taste.json")}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("taste: read %s: %w", s.path, err)
	}
	if len(b) == 0 {
		return s, nil
	}
	var st diskState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("taste: parse %s: %w", s.path, err)
	}
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
func (s *Store) Create(spec CreateSpec, now time.Time) (Exemplar, error) {
	spec.Title = strings.TrimSpace(spec.Title)
	spec.Body = strings.TrimSpace(spec.Body)
	spec.Scope = strings.TrimSpace(spec.Scope)
	if spec.Title == "" || spec.Body == "" {
		return Exemplar{}, errors.New("taste: title and body required")
	}
	if len(spec.Body) > maxBodyBytes {
		return Exemplar{}, fmt.Errorf("taste: body exceeds %d bytes", maxBodyBytes)
	}
	ts := now.UnixMilli()
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
		s.exemplars = append(s.exemplars, nil)
		copy(s.exemplars[idx+1:], s.exemplars[idx:])
		s.exemplars[idx] = removed
		return err
	}
	return nil
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(diskState{Version: storeVersion, Exemplars: s.exemplars}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		if stdruntime.GOOS == "windows" {
			if removeErr := os.Remove(s.path); removeErr == nil || os.IsNotExist(removeErr) {
				if retryErr := os.Rename(tmp, s.path); retryErr == nil {
					return nil
				} else {
					err = retryErr
				}
			}
		}
		_ = os.Remove(tmp)
		return err
	}
	return nil
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
