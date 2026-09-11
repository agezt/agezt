// SPDX-License-Identifier: MIT

// Store: type Store, Open, Add, SetEnabled, SetRetired, Update, Remove, Get, find, List, Count, save, SetNowForTest.
// Code extracted from roster.go during the Day-45 god-file split. Public API unchanged.
package roster


import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/jsonstore"
	"github.com/agezt/agezt/kernel/ulid"
)


type Store struct {
	path     string
	mu       sync.Mutex
	now      func() time.Time
	profiles []*Profile
}

// SetNowForTest overrides the store's clock so tests can inject deterministic
// timestamps. Must not be used in production code.
func (s *Store) SetNowForTest(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// Open opens (or creates) the roster store under dir.
func Open(dir string) (*Store, error) {
	s := &Store{now: time.Now}
	path, err := jsonstore.LoadFrom(dir, "roster.json", &s.profiles)
	if err != nil {
		return nil, fmt.Errorf("roster: %w", err)
	}
	s.path = path
	changed := false
	for _, p := range s.profiles {
		if applySystemGuardianDefaults(p) {
			p.UpdatedMS = s.now().UnixMilli()
			changed = true
		}
	}
	if changed {
		if err := s.save(); err != nil {
			return nil, fmt.Errorf("roster: migrate %s: %w", s.path, err)
		}
	}
	return s, nil
}

// Add validates and persists a new enabled profile, assigning an id +
// timestamps. Caller-supplied ID/Enabled/timestamps are ignored
// (kernel-assigned). The slug must be unique across the roster.
func (s *Store) Add(p Profile) (Profile, error) {
	now := s.now().UnixMilli()
	normalizeProfile(&p, now)
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
	if enabled && p.Retired {
		return Profile{}, ErrRetired
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

// SetRetired moves a profile to the graveyard (true) or revives it (false) by id
// or slug (M846). Retiring also pauses the agent (Enabled=false) so it stops
// firing; reviving leaves it paused for the operator to explicitly resume.
func (s *Store) SetRetired(ref string, retired bool, reason ...string) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.find(ref)
	if p == nil {
		return Profile{}, ErrNotFound
	}
	snapshot := *p
	p.Retired = retired
	if retired {
		p.RetiredMS = s.now().UnixMilli()
		if len(reason) > 0 {
			p.RetiredReason = strings.TrimSpace(reason[0])
		}
		p.Enabled = false // a graveyard agent does not run
	} else {
		p.RetiredMS = 0
		p.RetiredReason = ""
	}
	p.UpdatedMS = s.now().UnixMilli()
	if err := s.save(); err != nil {
		*p = snapshot
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
	if err := safeCall(func() error { mutate(p); return nil }); err != nil {
		return Profile{}, fmt.Errorf("roster: mutate panicked: %w", err)
	}
	// Protect identity + lifecycle fields from the mutator. The slug is the
	// agent's ADDRESS — renaming it would orphan every reference to it.
	p.ID, p.Slug, p.CreatedMS, p.Enabled = snapshot.ID, snapshot.Slug, snapshot.CreatedMS, snapshot.Enabled
	p.Retired, p.RetiredMS, p.RetiredReason = snapshot.Retired, snapshot.RetiredMS, snapshot.RetiredReason // graveyard state has its own setter (M846)
	p.System = snapshot.System                                                                             // kernel-owned: System marks a protected guardian. No mutator writes it today, but a future one doing *dst = in would otherwise allow self-promotion (MASS-003).
	now := s.now().UnixMilli()
	normalizeProfile(p, now)
	p.UpdatedMS = now
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
	return jsonstore.Save(s.path, s.profiles)
}
