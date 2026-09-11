// SPDX-License-Identifier: MIT

package edict

// Pattern-matching helpers for hard-deny rules + input canonicalisation:
// HardDenyRule.matches + denyCandidates + collectJSONStrings +
// collapseWhitespace + stripPunctAdjacentWhitespace + IsRuntimeRule.
// Carved out of edict.go during the Day 35 god file split #1 so the
// main file can focus on Engine + types.

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"unicode"
)

func (r HardDenyRule) matches(cap Capability, input string) bool {
	if len(r.AppliesTo) > 0 && !slices.Contains(r.AppliesTo, cap) {
		return false
	}
	return strings.Contains(strings.ToLower(input), strings.ToLower(r.Substring))
}

// denyCandidates returns the set of strings the hard-deny floor should be matched
// against for a tool input. It always includes the raw input (so prior behavior is
// preserved), plus — when the input is JSON — each decoded string VALUE with its
// whitespace collapsed. Decoding defeats JSON-escape evasion (`/` → `/`,
// `rm` → `rm`) and the whitespace collapse defeats padding (`rm  -rf /` →
// `rm -rf /`). Values are matched individually (not concatenated) so adjacent
// fields can't form a spurious match. Non-JSON input contributes its own
// whitespace-collapsed form (M173).
func denyCandidates(input string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(input)
	// The base strings to derive whitespace-normalised variants from: each
	// decoded string value for JSON input (defeats JSON-escape evasion), else
	// the raw input itself.
	var bases []string
	var v any
	if err := json.Unmarshal([]byte(input), &v); err == nil {
		collectJSONStrings(v, &bases)
	} else {
		bases = []string{input}
	}
	for _, s := range bases {
		// collapsed: padding evasion — `rm  -rf  /` → `rm -rf /` (matches the
		// space-bearing floor rules like `rm -rf /`, `dd if=`).
		add(collapseWhitespace(s))
		// stripped: spacing evasion — `:(){ :|:& };:` → `:(){:|:&};:`. The
		// canonical fork bomb carries spaces that survive collapse but are
		// syntactically optional. Only whitespace ADJACENT TO PUNCTUATION is
		// removed (M426): stripping ALL whitespace collapsed ordinary prose onto an
		// alphabetic floor rule — `re boot the server` → `reboottheserver` matched
		// `reboot`, `mk fs` → `mkfs`, etc. — a permanent, un-overridable false
		// hard-deny. Punctuation-adjacent stripping still normalises the fork bomb
		// (its spaces sit next to `{ | & ;`) without ever merging two words.
		add(stripPunctAdjacentWhitespace(s))
	}
	return out
}

// collectJSONStrings appends every string value reachable in a decoded JSON value
// (objects, arrays, nested) to dst. Keys are ignored — only values carry the
// model-chosen action text.
func collectJSONStrings(v any, dst *[]string) {
	switch t := v.(type) {
	case string:
		*dst = append(*dst, t)
	case []any:
		for _, e := range t {
			collectJSONStrings(e, dst)
		}
	case map[string]any:
		for _, e := range t {
			collectJSONStrings(e, dst)
		}
	}
}

// collapseWhitespace replaces every run of whitespace (spaces, tabs, newlines)
// with a single space and trims the ends, so padded/typeset variants of a command
// normalize to the canonical spacing the floor rules use.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// stripPunctAdjacentWhitespace removes a whitespace character only when it borders
// a punctuation character (a non-space, non-alphanumeric rune) on either side, so a
// spacing-variant fork bomb (`:(){ :|:& };:`, whose spaces sit next to `{ | & ;`)
// normalizes to the no-space floor form WITHOUT ever merging two alphanumeric words.
// Stripping ALL whitespace (the previous behaviour) collapsed ordinary prose like
// `re boot the server` onto the `reboot` rule — an un-overridable false hard-deny
// (M426). A space between two alphanumerics is preserved (kept as a single space).
func stripPunctAdjacentWhitespace(s string) string {
	rs := []rune(s)
	isAlnum := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(rs); i++ {
		if !unicode.IsSpace(rs[i]) {
			b.WriteRune(rs[i])
			continue
		}
		var prev, next rune
		for j := i - 1; j >= 0; j-- {
			if !unicode.IsSpace(rs[j]) {
				prev = rs[j]
				break
			}
		}
		for j := i + 1; j < len(rs); j++ {
			if !unicode.IsSpace(rs[j]) {
				next = rs[j]
				break
			}
		}
		prevPunct := prev != 0 && !isAlnum(prev)
		nextPunct := next != 0 && !isAlnum(next)
		if prevPunct || nextPunct {
			continue // drop whitespace bordering punctuation
		}
		b.WriteRune(' ') // keep a single space between alphanumeric words
	}
	return b.String()
}

// Options seed a new Engine.
type Options struct {
	// Levels overrides per-capability defaults. Caps absent here use the
	// values from Defaults (or LevelDeny if absent entirely).
	Levels map[Capability]TrustLevel
	// HardDeny replaces the default hard-deny rule set (use append with
	// DefaultHardDeny to extend rather than replace).
	HardDeny []HardDenyRule
	// AskPolicy chooses how to fold L1..L3. Default: AskAllow.
	AskPolicy AskPolicy
	// UnknownAllow flips the default for a capability with no configured level
	// from default-DENY to allow (M613). Off by default (secure-default: an
	// unknown capability is refused). The daemon sets it under AGEZT_ALLOW_ALL so
	// "allow everything" truly covers capabilities not in DefaultLevels —
	// including ones a future plugin tool introduces — not just the known set.
	// Hard-deny rules still apply first, so the catastrophe rails hold.
	UnknownAllow bool
}

// Engine is the policy decision engine. Safe for concurrent use.
type Engine struct {
	mu           sync.RWMutex
	levels       map[Capability]TrustLevel
	hardDeny     []HardDenyRule
	askPolicy    AskPolicy
	unknownAllow bool // M613: treat unconfigured capabilities as allow, not deny
	rtSeq        int  // monotonic counter naming runtime-added hard-deny rules
}

// RuntimeRulePrefix names rules added at runtime via AddHardDeny. The
// prefix is the load-bearing security invariant of runtime management:
// RemoveHardDeny only removes rules whose name carries it, so the
// boot-time floor (DefaultHardDeny + AGEZT_EDICT_DENY's operator[N]
// rules) can be *tightened* at runtime but never *loosened*. You can add
// a deny without a restart; you cannot delete a kernel/operator deny.
const RuntimeRulePrefix = "runtime["

// IsRuntimeRule reports whether name belongs to a rule added at runtime
// (and is therefore removable via RemoveHardDeny). Built-in and
// AGEZT_EDICT_DENY rules return false — they are the immutable floor.
//
// The match is STRICT: exactly the shape AddHardDeny mints, `runtime[<digits>]`
// (M174). A bare-prefix check (`runtime[…anything…`) would let a crafted name —
// `runtime[`, `runtime[evil`, `runtime[]` — masquerade as removable and, should
// such a name ever reach a floor rule (a refactor, a forged durable event), strip
// it. Since this prefix is the load-bearing "tighten-but-never-loosen" invariant,
// it is validated to the full canonical shape, not just the opening bracket.
func IsRuntimeRule(name string) bool {
	rest, ok := strings.CutPrefix(name, RuntimeRulePrefix)
	if !ok {
		return false
	}
	digits, ok := strings.CutSuffix(rest, "]")
	if !ok || digits == "" {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Levels returns a snapshot of the per-capability trust levels.
// Returned map is a copy — mutating it doesn't affect the engine.
