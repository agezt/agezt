// SPDX-License-Identifier: MIT

// Package toolname conforms agent tool names to the strict pattern that LLM
// provider APIs validate function/tool names against, and reverses the change on
// the response so a tool_call still routes to the real tool.
//
// Agezt exposes names like "browser.read", and dynamic MCP/forge tools may carry
// other characters or exceed the length cap. Providers (Anthropic, OpenAI,
// Bedrock, Cohere, Gemini, …) reject those with a 400 "does not match pattern" /
// invalid_request_error, which kills that provider's arm of a routing/fallback
// chain. The conformance here targets the strict-safe intersection of every
// provider's rule:
//
//   - characters: [a-zA-Z0-9_-]
//   - first character: a letter or '_' (Gemini's rule; harmless elsewhere)
//   - length: ≤ 64
//
// Maps is INJECTIVE: when two distinct names conform to the same string the
// collision is broken with a deterministic numeric suffix, so two tools never go
// out under the same wire name (which would let a tool_call route arguments to
// the WRONG tool).
package toolname

import (
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)

// maxLen is the smallest tool-name length cap across supported providers.
const maxLen = 64

// Sanitize conforms a single name: non-[a-zA-Z0-9_-] runes become '_', a
// leading non-letter/underscore is prefixed with '_', and the result is capped
// at 64. Never returns "".
func Sanitize(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := b.String()
	if s == "" {
		return "_"
	}
	if c := s[0]; !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_') {
		s = "_" + s
	}
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}

// Maps returns fwd (original→wire, for every tool) and rev (wire→original, only
// for names that changed; nil when none did). Collisions get a deterministic
// numeric suffix so the mapping stays injective.
func Maps(tools []agent.ToolDef) (fwd, rev map[string]string) {
	fwd = make(map[string]string, len(tools))
	used := make(map[string]bool, len(tools))
	for _, t := range tools {
		if _, dup := fwd[t.Name]; dup {
			continue // duplicate tool name: one wire mapping is enough
		}
		base := Sanitize(t.Name)
		if len(base) > maxLen-4 {
			base = base[:maxLen-4] // leave room for a collision suffix
		}
		wire := base
		for n := 2; used[wire]; n++ {
			wire = base + "_" + strconv.Itoa(n)
		}
		used[wire] = true
		fwd[t.Name] = wire
		if wire != t.Name {
			if rev == nil {
				rev = make(map[string]string, 2)
			}
			rev[wire] = t.Name
		}
	}
	return fwd, rev
}

// Reverse returns just the wire→original map for tools (nil when nothing changed).
func Reverse(tools []agent.ToolDef) map[string]string {
	_, rev := Maps(tools)
	return rev
}

// Wire returns the wire name for a tool name. In-map names use the injective
// mapping; a name not in the map (e.g. a history tool_use for a tool no longer
// offered) is sanitized directly so it is never sent in a non-conforming form.
func Wire(fwd map[string]string, name string) string {
	if fwd != nil {
		if w, ok := fwd[name]; ok {
			return w
		}
	}
	return Sanitize(name)
}

// RestoreCalls rewrites a response's tool-call names from their wire form back to
// the originals, in place. A no-op when rev is empty.
func RestoreCalls(resp *agent.CompletionResponse, rev map[string]string) {
	if resp == nil || len(rev) == 0 {
		return
	}
	for i, tc := range resp.Message.ToolCalls {
		if orig, ok := rev[tc.Name]; ok {
			resp.Message.ToolCalls[i].Name = orig
		}
	}
}
