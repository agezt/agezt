// SPDX-License-Identifier: MIT

package provopts

import (
	"encoding/json"
	"testing"
)

// TestMerge_InvalidBody covers the body-unmarshal error branch.
func TestMerge_InvalidBody(t *testing.T) {
	_, err := Merge([]byte(`not json`), json.RawMessage(`{"a":1}`))
	if err == nil {
		t.Fatal("expected error for invalid body JSON")
	}
}

// TestMerge_InvalidExtra covers the extra-unmarshal error branch.
func TestMerge_InvalidExtra(t *testing.T) {
	_, err := Merge([]byte(`{"a":1}`), json.RawMessage(`{not json`))
	if err == nil {
		t.Fatal("expected error for invalid extra JSON")
	}
}

// TestMerge_NullBody covers the `base == nil` branch: a JSON `null` body
// unmarshals to a nil map, which Merge must allocate before copying the overlay.
func TestMerge_NullBody(t *testing.T) {
	got, err := Merge([]byte(`null`), json.RawMessage(`{"temperature":0.5}`))
	if err != nil {
		t.Fatalf("Merge(null,...) err: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("result not an object: %s", got)
	}
	if m["temperature"] != 0.5 {
		t.Fatalf("overlay lost onto null body: %v", m)
	}
}

// TestThinkingBudget_AllEffortLevels covers each switch case plus the floor and
// the max_tokens cap boundaries.
func TestThinkingBudget_AllEffortLevels(t *testing.T) {
	cases := []struct {
		effort string
		maxTok int
		wantB  int
		wantOK bool
	}{
		{"minimal", 0, 1024, true},
		{"low", 0, 2048, true},
		{"medium", 0, 8192, true},
		{"high", 0, 16384, true},
		{"HIGH", 0, 16384, true}, // case-insensitive
		{"  low  ", 0, 2048, true},
		{"bogus", 0, 0, false},
		// maxTokens larger than budget → budget kept.
		{"low", 100000, 2048, true},
		// maxTokens just above budget → budget kept (b < maxTokens).
		{"medium", 8193, 8192, true},
		// maxTokens == budget → capped to maxTokens-1.
		{"medium", 8192, 8191, true},
		// maxTokens below budget → capped, still >= 1024.
		{"high", 2048, 2047, true},
		// maxTokens so small the capped budget < 1024 → disabled.
		{"high", 1024, 0, false},
		{"high", 500, 0, false},
	}
	for _, c := range cases {
		b, ok := ThinkingBudget(c.effort, c.maxTok)
		if b != c.wantB || ok != c.wantOK {
			t.Errorf("ThinkingBudget(%q,%d) = (%d,%v), want (%d,%v)",
				c.effort, c.maxTok, b, ok, c.wantB, c.wantOK)
		}
	}
}

// TestNormalizeEffort_Invalid covers the default (empty) return.
func TestNormalizeEffort_Invalid(t *testing.T) {
	for _, in := range []string{"", "bogus", "extreme", "  "} {
		if got := NormalizeEffort(in); got != "" {
			t.Fatalf("NormalizeEffort(%q) = %q, want empty", in, got)
		}
	}
}
