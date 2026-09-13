// SPDX-License-Identifier: MIT
//
// creds entry operations: Set + Get + Has + Remove + Names + Lookup +
// ChainLookup + MaskValue + validateName.
// Extracted from creds.go during the Day-203 god-file split.
// Public API unchanged.
package creds

import (
	"errors"
	"sort"
	"strings"
)

// Set assigns a value to an env-var-style name. Empty value removes
// the entry — same convention as `unset` in a shell. Caller is
// responsible for Save.
func (s *Store) Set(name, value string) error {
	if err := validateName(name); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if value == "" {
		delete(s.data, name)
	} else {
		s.data[name] = value
	}
	return nil
}

// Get returns the value for name, or "" if absent. The empty-means-
// absent convention matches both os.Getenv and compat.CredLookup so a
// Store can be used as a CredLookup directly via the Lookup method.
func (s *Store) Get(name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[name]
}

// Has reports whether name has a non-empty value in the vault.
func (s *Store) Has(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[name]
	return ok && v != ""
}

// Remove deletes the entry. Returns true if there was something to
// delete. Caller is responsible for Save.
func (s *Store) Remove(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, existed := s.data[name]
	delete(s.data, name)
	return existed
}

// Names returns the sorted list of all stored env-var names.
func (s *Store) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.data))
	for k := range s.data {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Lookup is the CredLookup-compatible signature. Equivalent to Get
// but expresses intent at call sites.
func (s *Store) Lookup(name string) string { return s.Get(name) }

// ChainLookup composes multiple credential sources into a single
// CredLookup-compatible function. The first source that returns a
// non-empty string wins. Typical use: vault first, env second, so
// an operator can `export FOO=...` to override a vaulted value for a
// shell session without rewriting the vault.
//
// Nil sources are skipped, so passing `(nil, os.Getenv)` works.
func ChainLookup(sources ...func(string) string) func(string) string {
	return func(name string) string {
		for _, src := range sources {
			if src == nil {
				continue
			}
			if v := src(name); v != "" {
				return v
			}
		}
		return ""
	}
}

// MaskValue redacts a credential for display: keeps the first 4 and
// last 4 chars (or fewer for short values), with the middle as dots.
// 8-character values and shorter are fully masked.
func MaskValue(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 8 {
		return strings.Repeat("•", len(v))
	}
	return v[:4] + strings.Repeat("•", 6) + v[len(v)-4:]
}

// validateName rejects empty or whitespace-only names so the vault
// can't accumulate unreachable junk entries.
func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("creds: env var name must be non-empty")
	}
	if name != strings.TrimSpace(name) {
		return errors.New("creds: env var name must not have leading/trailing whitespace")
	}
	return nil
}
