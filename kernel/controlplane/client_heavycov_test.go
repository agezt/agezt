// SPDX-License-Identifier: MIT

package controlplane

import "testing"

// TestToString covers the safe string-from-any extractor: a real string passes
// through, everything else yields "".
func TestToString(t *testing.T) {
	if got := toString("hello"); got != "hello" {
		t.Errorf("toString(string) = %q, want hello", got)
	}
	for _, v := range []any{nil, 42, true, 3.14, []string{"x"}, map[string]any{}} {
		if got := toString(v); got != "" {
			t.Errorf("toString(%#v) = %q, want empty", v, got)
		}
	}
}

// TestToBool covers the safe bool-from-any extractor.
func TestToBool(t *testing.T) {
	if !toBool(true) {
		t.Error("toBool(true) = false, want true")
	}
	for _, v := range []any{false, nil, "true", 1, 0, 3.14} {
		if toBool(v) {
			t.Errorf("toBool(%#v) = true, want false", v)
		}
	}
}

// TestClientClose confirms Close is a no-op returning nil.
func TestClientClose(t *testing.T) {
	c := &Client{}
	if err := c.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}
