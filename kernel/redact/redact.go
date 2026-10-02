// SPDX-License-Identifier: MIT

package redact

// This file owns the Redactor type + its methods (the public redaction
// surface). The pattern catalogues and the MatchedCategories helper live
// in redact_patterns.go.

import (
	"sort"
	"strings"
	"sync"
)

// Redactor scrubs secrets from strings and bytes. It is safe for concurrent use;
// the literal set can be updated (e.g. after a creds rotation) without rebuilding.
type Redactor struct {
	mu       sync.RWMutex
	literals []string // sorted longest-first so overlapping secrets fully redact
}

// New returns a Redactor with the built-in patterns active and no literals.
func New() *Redactor { return &Redactor{} }

// SetSecrets replaces the literal secret set. Empty and too-short values are
// dropped; duplicates are removed; the rest are sorted longest-first so that a
// secret which is a prefix of another is not left partially exposed.
func (r *Redactor) SetSecrets(values []string) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if len(v) < minLiteralLen {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	r.mu.Lock()
	r.literals = out
	r.mu.Unlock()
}

// Redact returns s with every known literal secret and every pattern match
// replaced by Placeholder. It is a no-op (returns s unchanged) when nothing
// matches.
func (r *Redactor) Redact(s string) string {
	if s == "" {
		return s
	}
	r.mu.RLock()
	lits := r.literals
	r.mu.RUnlock()
	for _, lit := range lits {
		if strings.Contains(s, lit) {
			s = strings.ReplaceAll(s, lit, Placeholder)
		}
	}
	for _, p := range patterns {
		s = p.ReplaceAllString(s, Placeholder)
	}
	// Context-preserving patterns: mask only the secret span, keep the rest.
	for _, tp := range templatedPatterns {
		s = tp.re.ReplaceAllString(s, tp.repl)
	}
	return s
}

// RedactBytes is Redact over a byte slice (e.g. marshaled JSON). It returns the
// original slice unchanged when nothing matched, and never returns nil for a
// non-nil input.
func (r *Redactor) RedactBytes(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	out := r.Redact(string(b))
	if out == string(b) {
		return b
	}
	return []byte(out)
}
