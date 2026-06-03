// SPDX-License-Identifier: MIT

package sdk

import (
	"testing"
	"time"
)

func TestParseApprovals(t *testing.T) {
	res := map[string]any{"pending": []any{
		map[string]any{
			"id":           "ap-1",
			"capability":   "shell.exec",
			"tool_name":    "shell",
			"reason":       "policy ask",
			"actor":        "agent-c1",
			"input":        map[string]any{"command": "ls"},
			"timeout_unix": float64(1_700_000_000),
		},
		map[string]any{
			"id":         "ap-2",
			"capability": "http.fetch",
			"input":      "https://example.com",
		},
	}}

	aps := parseApprovals(res)
	if len(aps) != 2 {
		t.Fatalf("want 2 approvals, got %d", len(aps))
	}

	a := aps[0]
	if a.ID != "ap-1" || a.Capability != "shell.exec" || a.Tool != "shell" || a.Reason != "policy ask" || a.Actor != "agent-c1" {
		t.Errorf("approval[0] fields wrong: %+v", a)
	}
	// Structured input is re-encoded as compact JSON.
	if a.Input != `{"command":"ls"}` {
		t.Errorf("input = %q, want compact JSON", a.Input)
	}
	if !a.Timeout.Equal(time.Unix(1_700_000_000, 0)) {
		t.Errorf("timeout = %v", a.Timeout)
	}

	b := aps[1]
	// A string input passes through verbatim; a missing timeout is the zero time.
	if b.Input != "https://example.com" {
		t.Errorf("string input = %q", b.Input)
	}
	if !b.Timeout.IsZero() {
		t.Errorf("missing timeout should be zero, got %v", b.Timeout)
	}
}

func TestParseApprovals_Empty(t *testing.T) {
	if got := parseApprovals(map[string]any{}); len(got) != 0 {
		t.Errorf("no pending key → empty, got %v", got)
	}
}

func TestAnyToString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hi", "hi"},
		{map[string]any{"a": 1}, `{"a":1}`},
		{[]any{"x", "y"}, `["x","y"]`},
		{float64(3), "3"},
	}
	for _, c := range cases {
		if got := anyToString(c.in); got != c.want {
			t.Errorf("anyToString(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
