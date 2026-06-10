// SPDX-License-Identifier: MIT

// Package roster is the durable agent roster (M783): named, persistent agent
// profiles — an identity ("researcher", "ops-watcher") with its own soul
// (system prompt), model (+ ordered fallbacks), default task type, per-run
// spend ceiling, memory scope, and workspace subdirectory. A profile is the
// durable HOME for everything that until now lived per-run: `agt run --agent
// researcher` runs AS that agent, and future arcs attach per-agent messaging,
// budgets, and tool grants to the same identity.
//
// Storage mirrors kernel/standing: a single JSON file rewritten atomically on
// change, safe for concurrent use; every lifecycle mutation is journaled by
// the kernel (roster.created/updated/removed) so `agt why` can explain how an
// agent came to exist.
package roster

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/ulid"
)

// ErrNotFound is returned for an unknown profile id/slug.
var ErrNotFound = errors.New("roster: profile not found")

// Profile is one named agent identity. Slug is the address — unique,
// immutable, what operators and (future) other agents refer to it by.
type Profile struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`           // unique handle, e.g. "researcher"
	Name string `json:"name,omitempty"` // human label; defaults to the slug

	// Soul is the agent's system prompt — who it IS. Applied as the run's
	// system override; memory/world/skill injection still layers on top.
	Soul string `json:"soul,omitempty"`

	// Model is the primary model for this agent's runs (empty = kernel
	// default); Fallbacks is its ordered per-agent fallback chain (reserved
	// for the routing integration arc — stored and validated now so profiles
	// are forward-complete).
	Model     string   `json:"model,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`

	// TaskType is the governor task type this agent's runs default to
	// (e.g. "coding", "research"); empty = unclassified.
	TaskType string `json:"task_type,omitempty"`

	// MaxCostMc is the per-run spend ceiling in USD-microcents (0 = none).
	// Applied as the run's max_cost default; an explicit per-run cap wins.
	MaxCostMc int64 `json:"max_cost_mc,omitempty"`

	// MaxDailyMc is the per-DAY spend ceiling in USD-microcents (0 = none):
	// the Governor meters every completion this agent makes (runs, delegate
	// children, standing firings) against an identity ledger and refuses
	// once today's total reaches the ceiling (M793).
	MaxDailyMc int64 `json:"max_daily_mc,omitempty"`

	// MemoryScope is the agent's private memory scope (M652); empty = the
	// slug, so every named agent gets its own notes by default.
	MemoryScope string `json:"memory_scope,omitempty"`

	// Workdir is a workspace-relative subdirectory this agent works in
	// (reserved for the per-agent sandbox arc). Must be relative and must
	// not escape the workspace.
	Workdir string `json:"workdir,omitempty"`

	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	CreatedMS   int64  `json:"created_ms"`
	UpdatedMS   int64  `json:"updated_ms"`
}

// slugRe: lowercase, digit-or-letter first, then letters/digits/dot/dash/underscore.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

const (
	maxSoulBytes = 64 * 1024 // a soul is a prompt, not a novel
	maxFallbacks = 8
)

// Validate checks a profile's user-supplied fields (identity/lifecycle fields
// are kernel-assigned and not judged here).
func Validate(p Profile) error {
	if !slugRe.MatchString(p.Slug) {
		return fmt.Errorf("roster: slug must match %s", slugRe)
	}
	if len(p.Soul) > maxSoulBytes {
		return fmt.Errorf("roster: soul exceeds %d bytes", maxSoulBytes)
	}
	if len(p.Fallbacks) > maxFallbacks {
		return fmt.Errorf("roster: at most %d fallback models", maxFallbacks)
	}
	for _, f := range p.Fallbacks {
		if strings.TrimSpace(f) == "" {
			return errors.New("roster: empty fallback model id")
		}
	}
	if p.MaxCostMc < 0 {
		return errors.New("roster: max_cost_mc must be >= 0")
	}
	if p.MaxDailyMc < 0 {
		return errors.New("roster: max_daily_mc must be >= 0")
	}
	if p.Workdir != "" {
		w := filepath.ToSlash(p.Workdir)
		if filepath.IsAbs(p.Workdir) || strings.HasPrefix(w, "/") ||
			w == ".." || strings.HasPrefix(w, "../") || strings.Contains(w, "/../") || strings.HasSuffix(w, "/..") {
			return errors.New("roster: workdir must be a relative path inside the workspace")
		}
	}
	return nil
}

// Store is the persistent roster, a single JSON file rewritten atomically on
// change. Safe for concurrent use. Mirrors kernel/standing.Store.
type Store struct {
	path     string
	mu       sync.Mutex
	now      func() time.Time
	profiles []*Profile
}

// Open opens (or creates) the roster store under dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("roster: mkdir %s: %w", dir, err)
	}
	s := &Store{path: filepath.Join(dir, "roster.json"), now: time.Now}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("roster: read %s: %w", s.path, err)
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.profiles); err != nil {
			return nil, fmt.Errorf("roster: parse %s: %w", s.path, err)
		}
	}
	return s, nil
}

// Add validates and persists a new enabled profile, assigning an id +
// timestamps. Caller-supplied ID/Enabled/timestamps are ignored
// (kernel-assigned). The slug must be unique across the roster.
func (s *Store) Add(p Profile) (Profile, error) {
	p.Slug = strings.TrimSpace(p.Slug)
	if err := Validate(p); err != nil {
		return Profile{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ex := range s.profiles {
		if ex.Slug == p.Slug {
			return Profile{}, fmt.Errorf("roster: slug %q already exists", p.Slug)
		}
	}
	now := s.now().UnixMilli()
	p.ID = ulid.New()
	if strings.TrimSpace(p.Name) == "" {
		p.Name = p.Slug
	}
	p.Enabled = true
	p.CreatedMS = now
	p.UpdatedMS = now
	cp := p
	s.profiles = append(s.profiles, &cp)
	if err := s.save(); err != nil {
		s.profiles = s.profiles[:len(s.profiles)-1]
		return Profile{}, err
	}
	return cp, nil
}

// SetEnabled pauses (false) or resumes (true) a profile by id or slug.
func (s *Store) SetEnabled(ref string, enabled bool) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.find(ref)
	if p == nil {
		return Profile{}, ErrNotFound
	}
	// Roll back the in-memory mutation if the durable write fails, so the
	// running view never diverges from disk on a transient save error.
	prevEnabled, prevUpdated := p.Enabled, p.UpdatedMS
	p.Enabled = enabled
	p.UpdatedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		p.Enabled, p.UpdatedMS = prevEnabled, prevUpdated
		return Profile{}, err
	}
	return *p, nil
}

// Update applies edits to a profile's mutable fields via mutate, re-validates,
// and persists. Identity and lifecycle fields — ID, Slug, CreatedMS, Enabled
// (which has its own setter) — are preserved regardless of what mutate does;
// UpdatedMS is bumped. Rolled back in memory on validation/save failure.
func (s *Store) Update(ref string, mutate func(*Profile)) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.find(ref)
	if p == nil {
		return Profile{}, ErrNotFound
	}
	snapshot := *p
	mutate(p)
	// Protect identity + lifecycle fields from the mutator. The slug is the
	// agent's ADDRESS — renaming it would orphan every reference to it.
	p.ID, p.Slug, p.CreatedMS, p.Enabled = snapshot.ID, snapshot.Slug, snapshot.CreatedMS, snapshot.Enabled
	p.UpdatedMS = s.now().UnixMilli()
	if err := Validate(*p); err != nil {
		*p = snapshot
		return Profile{}, err
	}
	if err := s.save(); err != nil {
		*p = snapshot
		return Profile{}, err
	}
	return *p, nil
}

// Remove deletes a profile by id or slug. Returns whether it existed.
func (s *Store) Remove(ref string) (Profile, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.profiles {
		if p.ID == ref || p.Slug == ref {
			removed := s.profiles
			gone := *p
			s.profiles = append(append([]*Profile{}, s.profiles[:i]...), s.profiles[i+1:]...)
			if err := s.save(); err != nil {
				s.profiles = removed // restore: disk write failed, keep the profile
				return Profile{}, false, err
			}
			return gone, true, nil
		}
	}
	return Profile{}, false, nil
}

// Get returns one profile by id or slug.
func (s *Store) Get(ref string) (Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := s.find(ref); p != nil {
		return *p, true
	}
	return Profile{}, false
}

// find returns the live pointer for an id or slug. Caller holds s.mu.
func (s *Store) find(ref string) *Profile {
	for _, p := range s.profiles {
		if p.ID == ref || p.Slug == ref {
			return p
		}
	}
	return nil
}

// List returns all profiles, sorted by creation time then id (deterministic).
func (s *Store) List() []Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Profile, 0, len(s.profiles))
	for _, p := range s.profiles {
		out = append(out, *p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedMS != out[j].CreatedMS {
			return out[i].CreatedMS < out[j].CreatedMS
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Count returns the number of profiles.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.profiles)
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.profiles, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
