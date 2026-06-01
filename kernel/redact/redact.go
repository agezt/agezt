// SPDX-License-Identifier: MIT

// Package redact scrubs secrets from text before it is persisted. The kernel's
// journal is append-only and hash-chained: anything written there is permanent.
// A secret that reaches an event payload — an API key echoed in a tool's output,
// a token pasted into a prompt, a credential in an HTTP response — would be
// recorded forever. This package is the chokepoint that prevents that
// (SPEC-06 / ROADMAP "redaction must work before Initiative can act
// autonomously").
//
// It redacts on two signals:
//
//   - Literals: exact secret values the daemon knows (e.g. the configured
//     provider keys from the creds store). Scrubbed wherever they appear, even
//     mid-string and nested.
//   - Patterns: high-confidence secret *formats* (OpenAI/Anthropic `sk-…`, AWS
//     `AKIA…`, GitHub `ghp_…`, Slack `xox…`, Google `AIza…`, bearer tokens, PEM
//     private-key blocks). These catch secrets the daemon was never told about.
//
// Redaction is a pure, deterministic function of (input, literal set): the same
// input always yields the same output, so a redacted payload hashes stably and
// replay is unaffected (the journal already holds the redacted form). Patterns
// are deliberately specific to avoid corrupting legitimate data.
package redact

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Placeholder replaces every redacted span. It contains no JSON-special
// characters, so substituting it inside already-marshaled JSON keeps the JSON
// valid.
const Placeholder = "[REDACTED]"

// minLiteralLen is the shortest literal value we will scrub. Below this a
// "secret" is too likely to be an ordinary substring whose removal would corrupt
// unrelated data; provider keys/tokens are always far longer.
const minLiteralLen = 8

// namedPattern pairs a high-confidence secret detector with a human-readable
// label, used both for redaction and for `agt redact test` diagnostics (M104).
type namedPattern struct {
	name string
	re   *regexp.Regexp
}

// namedPatterns are high-confidence secret formats. Each must match a token
// shape that is implausible as ordinary prose, so a full-match replacement is
// safe. Order is irrelevant: matches are non-overlapping replacements.
var namedPatterns = []namedPattern{
	// OpenAI / Anthropic style keys: sk-…, sk-proj-…, sk-ant-… (the dash class
	// makes the single rule cover the prefixed variants).
	{"openai/anthropic-key", regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`)},
	// AWS access key id.
	{"aws-access-key-id", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	// GitHub tokens: ghp_, gho_, ghu_, ghs_, ghr_.
	{"github-token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`)},
	// Slack tokens: xoxb-, xoxa-, xoxp-, xoxr-, xoxs-.
	{"slack-token", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)},
	// Google API key.
	{"google-api-key", regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)},
	// Bearer tokens (Authorization headers, OAuth dumps).
	{"bearer-token", regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]{20,}`)},
	// PEM private-key blocks (any type), body and delimiters.
	{"pem-private-key", regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)},
}

// patterns is the detector list used by Redact, derived from namedPatterns so
// the labels and the redaction rules can never drift apart.
var patterns = func() []*regexp.Regexp {
	ps := make([]*regexp.Regexp, len(namedPatterns))
	for i, np := range namedPatterns {
		ps[i] = np.re
	}
	return ps
}()

// MatchedCategories returns the labels of the built-in patterns that match s,
// in declaration order. It is a pure, daemon-free helper behind
// `agt redact test`; configured literal secrets are NOT covered here (they are
// only known to a live Redactor). Returns nil when nothing matches.
func MatchedCategories(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, np := range namedPatterns {
		if np.re.MatchString(s) {
			out = append(out, np.name)
		}
	}
	return out
}

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
