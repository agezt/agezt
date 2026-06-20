// SPDX-License-Identifier: MIT

package toolname

import (
	"regexp"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// strict is the intersection pattern: chars [a-zA-Z0-9_-], leading letter/_, ≤64.
var strict = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]{0,63}$`)

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"browser.read":  "browser_read",
		"shell":         "shell",
		"web_search":    "web_search",
		"a:b/c":         "a_b_c",
		"":              "_",
		"9lives":        "_9lives", // leading digit → prefixed
		"-dash":         "_-dash",  // leading dash → prefixed
		"mcp__srv__get": "mcp__srv__get",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	// Over-long names are capped and still valid.
	long := strings.Repeat("x.y", 50)
	if s := Sanitize(long); len(s) > 64 || !strict.MatchString(s) {
		t.Errorf("long sanitize invalid: len=%d %q", len(s), s)
	}
	// Every output matches the strict intersection pattern.
	for in := range cases {
		if s := Sanitize(in); !strict.MatchString(s) {
			t.Errorf("Sanitize(%q)=%q fails strict pattern", in, s)
		}
	}
}

func TestMapsRoundTrip(t *testing.T) {
	tools := []agent.ToolDef{{Name: "browser.read"}, {Name: "shell"}}
	fwd, rev := Maps(tools)
	if fwd["browser.read"] != "browser_read" || fwd["shell"] != "shell" {
		t.Fatalf("fwd = %v", fwd)
	}
	// "shell" unchanged → not in rev; "browser.read" changed → in rev.
	if rev["browser_read"] != "browser.read" {
		t.Fatalf("rev = %v", rev)
	}
	if _, ok := rev["shell"]; ok {
		t.Fatalf("unchanged name should not be in rev: %v", rev)
	}
	if Wire(fwd, "browser.read") != "browser_read" || Wire(fwd, "unknown") != "unknown" {
		t.Fatalf("Wire fallback wrong")
	}

	resp := &agent.CompletionResponse{Message: agent.Message{ToolCalls: []agent.ToolCall{
		{Name: "browser_read"}, {Name: "shell"},
	}}}
	RestoreCalls(resp, Reverse(tools))
	if resp.Message.ToolCalls[0].Name != "browser.read" || resp.Message.ToolCalls[1].Name != "shell" {
		t.Fatalf("restore wrong: %+v", resp.Message.ToolCalls)
	}
}

func TestMapsCollisionInjective(t *testing.T) {
	fwd, _ := Maps([]agent.ToolDef{{Name: "browser.read"}, {Name: "browser_read"}, {Name: "browser:read"}})
	seen := map[string]bool{}
	for _, w := range fwd {
		if seen[w] {
			t.Fatalf("non-injective wire name %q in %v", w, fwd)
		}
		seen[w] = true
		if !strict.MatchString(w) {
			t.Fatalf("wire name %q fails strict pattern", w)
		}
	}
	if len(fwd) != 3 {
		t.Fatalf("want 3 distinct mappings, got %v", fwd)
	}
}
