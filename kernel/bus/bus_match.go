// SPDX-License-Identifier: MIT
//
// Bus subject-pattern matcher: MatchSubject + matches + ValidatePattern +
// parsePattern (the NATS-style wildcard rules: "*" matches one token, ">"
// matches one or more tokens and must be the final token). Split from bus.go
// during Day 211 god-file refactor (#33). Public API unchanged.
package bus

import (
	"fmt"
	"strings"
)

// MatchSubject reports whether subject matches pattern using the
// bus's NATS-style wildcard rules. Exported for callers (notably
// the controlplane's pulse historical-replay path in M1.aa) that
// need to filter journal events by the same pattern the live
// subscription uses, without going through the full Subscribe
// machinery. Returns false on a malformed pattern rather than
// erroring; pulse's caller validates patterns at subscribe time.
func MatchSubject(pattern, subject string) bool {
	pat, err := parsePattern(pattern)
	if err != nil {
		return false
	}
	return matches(pat, strings.Split(subject, "."))
}

// matches reports whether subject matches pattern. Both are pre-tokenized.
func matches(pattern, subject []string) bool {
	pi, si := 0, 0
	for pi < len(pattern) && si < len(subject) {
		tok := pattern[pi]
		if tok == ">" {
			return true // consumes all remaining subject tokens
		}
		if tok != "*" && tok != subject[si] {
			return false
		}
		pi++
		si++
	}
	if pi == len(pattern) && si == len(subject) {
		return true
	}
	// Pattern still has tokens but subject exhausted: only "> with nothing
	// behind it" doesn't make sense, and "*" requires at least one segment.
	// So only acceptable case is pattern fully consumed already (handled).
	return false
}

// ValidatePattern reports whether p is a well-formed NATS-style subject pattern
// (non-empty, no empty tokens, '>' only as the final token), returning ErrPattern-
// wrapped detail otherwise. Exported so config parsers (e.g. webhook sink subject
// filters) can reject a malformed pattern at startup instead of silently never
// matching it at delivery time.
func ValidatePattern(p string) error {
	_, err := parsePattern(p)
	return err
}

func parsePattern(p string) ([]string, error) {
	if p == "" {
		return nil, fmt.Errorf("%w: empty", ErrPattern)
	}
	tokens := strings.Split(p, ".")
	for i, t := range tokens {
		if t == "" {
			return nil, fmt.Errorf("%w: empty token in %q", ErrPattern, p)
		}
		if t == ">" && i != len(tokens)-1 {
			return nil, fmt.Errorf("%w: %q: '>' must be the last token", ErrPattern, p)
		}
	}
	return tokens, nil
}
