// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestOrDash covers both branches of orDash: empty/whitespace -> dash, else value.
func TestOrDash(t *testing.T) {
	if got := orDash(""); got != "—" {
		t.Fatalf("orDash(\"\") = %q want dash", got)
	}
	if got := orDash("   "); got != "—" {
		t.Fatalf("orDash(spaces) = %q want dash", got)
	}
	if got := orDash("value"); got != "value" {
		t.Fatalf("orDash(value) = %q", got)
	}
}

// TestPadValue covers padValue's empty and non-empty branches.
func TestPadValue(t *testing.T) {
	if got := padValue(""); got != "" {
		t.Fatalf("padValue(\"\") = %q want empty", got)
	}
	if got := padValue("x"); got != " x" {
		t.Fatalf("padValue(x) = %q want leading space", got)
	}
}

// TestEmptyJSONValue exercises every type branch of emptyJSONValue.
func TestEmptyJSONValue(t *testing.T) {
	cases := []struct {
		v    any
		want bool
	}{
		{nil, true},
		{"", true},
		{"  ", true},
		{"x", false},
		{float64(0), true},
		{float64(1), false},
		{int(0), true},
		{int(2), false},
		{true, false},
		{false, false},
		{[]any{}, true},
		{[]any{1}, false},
		{map[string]any{}, false}, // default branch
	}
	for _, c := range cases {
		if got := emptyJSONValue(c.v); got != c.want {
			t.Fatalf("emptyJSONValue(%#v) = %v want %v", c.v, got, c.want)
		}
	}
}

// TestJoinAnyStrings covers filtering of empty values and joining.
func TestJoinAnyStrings(t *testing.T) {
	got := joinAnyStrings([]any{"a", "", "b", nil}, ",")
	if got != "a,b" {
		t.Fatalf("joinAnyStrings = %q want a,b", got)
	}
	if got := joinAnyStrings(nil, ","); got != "" {
		t.Fatalf("joinAnyStrings(nil) = %q want empty", got)
	}
}

// TestPrintAgentPolicy covers both the skip (nil/empty) and populated paths.
func TestPrintAgentPolicy(t *testing.T) {
	var b bytes.Buffer
	// Non-map -> no output.
	printAgentPolicy(&b, "retry", "not-a-map")
	if b.Len() != 0 {
		t.Fatalf("printAgentPolicy(non-map) wrote %q", b.String())
	}
	// Empty map -> no output.
	b.Reset()
	printAgentPolicy(&b, "retry", map[string]any{})
	if b.Len() != 0 {
		t.Fatalf("printAgentPolicy(empty) wrote %q", b.String())
	}
	// Populated map with a mix of empty + present keys -> renders present ones.
	b.Reset()
	printAgentPolicy(&b, "retry", map[string]any{
		"max_attempts": float64(3),
		"backoff":      "exponential",
		"base_delay_sec": float64(0), // empty -> skipped
	})
	out := b.String()
	if !strings.Contains(out, "max_attempts=3") || !strings.Contains(out, "backoff=exponential") {
		t.Fatalf("printAgentPolicy output = %q", out)
	}
	if strings.Contains(out, "base_delay_sec") {
		t.Fatalf("printAgentPolicy included empty key: %q", out)
	}
}

// TestPrintAgentLifecycle covers skip and populated paths.
func TestPrintAgentLifecycle(t *testing.T) {
	var b bytes.Buffer
	printAgentLifecycle(&b, "not-a-map")
	if b.Len() != 0 {
		t.Fatalf("printAgentLifecycle(non-map) wrote %q", b.String())
	}
	b.Reset()
	printAgentLifecycle(&b, map[string]any{})
	if b.Len() != 0 {
		t.Fatalf("printAgentLifecycle(empty) wrote %q", b.String())
	}
	b.Reset()
	printAgentLifecycle(&b, map[string]any{
		"mode":       "cycle",
		"max_cycles": float64(5),
	})
	if out := b.String(); !strings.Contains(out, "mode=cycle") || !strings.Contains(out, "max_cycles=5") {
		t.Fatalf("printAgentLifecycle output = %q", out)
	}
}

// TestRenderAgentAuthority covers the many optional-field branches of the
// authority renderer, including capability capping and the hard-deny floor.
func TestRenderAgentAuthority(t *testing.T) {
	var b bytes.Buffer
	v := agentAuthorityView{
		Slug:         "coverbot",
		TrustCeiling: "L1",
		DirectCall:   true,
		ToolAllow:    []string{"read", "write"},
		ToolDeny:     []string{"exec"},
		MemoryScope:  "project",
		Workdir:      "/tmp/work",
		ConfigCount:  2,
		ApprovalMode: "ask",
		CapLevels: map[string]string{
			"shell.exec": "L4", // exceeds L1 ceiling -> capped note
			"fs.read":    "L1",
		},
		HardDeny: []agentHardDeny{
			{Name: "no-rm", Substring: "rm -rf", Scope: "shell"},
		},
	}
	renderAgentAuthority(&b, v)
	out := b.String()
	for _, want := range []string{
		"agent:", "coverbot", "trust ceiling", "tool allow", "read, write",
		"tool deny", "exec", "memory scope", "project", "workdir", "/tmp/work",
		"config:", "override", "approval mode", "capability levels",
		"shell.exec", "capped to L1", "hard-deny floor", "no-rm",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("renderAgentAuthority output missing %q\n%s", want, out)
		}
	}

	// Minimal view: exercise the orDash fallbacks and skipped optionals.
	b.Reset()
	renderAgentAuthority(&b, agentAuthorityView{Slug: "bare"})
	out = b.String()
	if !strings.Contains(out, "bare") || !strings.Contains(out, "—") {
		t.Fatalf("renderAgentAuthority(minimal) = %q", out)
	}
	if strings.Contains(out, "capability levels") {
		t.Fatalf("renderAgentAuthority(minimal) unexpectedly listed caps: %q", out)
	}
}
