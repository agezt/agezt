// SPDX-License-Identifier: MIT

// Internal-package test for the attention-arg parser. Lives in `package
// controlplane` (not `controlplane_test`) so it can exercise the unexported
// `attentionArgs` helper directly — going through c.Call would only prove the
// public path works, not the per-input fallback semantics the status-read
// contract relies on.

package controlplane

import (
	"testing"
	"time"
)

// TestAttentionArgs_Direct exercises the helper that does the actual arg
// parsing. It is a unit-level guard for the fallback behavior the
// status-read contract relies on: bad input must fall back to defaults
// (24h, 8) rather than 400ing, and limits must be capped at 50.
func TestAttentionArgs_Direct(t *testing.T) {
	// Defaults.
	w, l := attentionArgs(map[string]any{})
	if w != 24*time.Hour || l != 8 {
		t.Errorf("default args = (%v, %d), want (24h, 8)", w, l)
	}

	// Happy path.
	w, l = attentionArgs(map[string]any{"window": "1h", "limit": "3"})
	if w != time.Hour || l != 3 {
		t.Errorf("happy args = (%v, %d), want (1h, 3)", w, l)
	}

	// Bad window → default.
	w, _ = attentionArgs(map[string]any{"window": "not-a-duration"})
	if w != 24*time.Hour {
		t.Errorf("bad window = %v, want 24h fallback", w)
	}

	// Bad limit → default.
	_, l = attentionArgs(map[string]any{"limit": "abc"})
	if l != 8 {
		t.Errorf("bad limit = %d, want 8 fallback", l)
	}

	// Limit capped at 50.
	_, l = attentionArgs(map[string]any{"limit": "999"})
	if l != 50 {
		t.Errorf("limit cap = %d, want 50", l)
	}

	// Numeric inputs (JSON path) work too.
	w, l = attentionArgs(map[string]any{"window": float64(3600), "limit": float64(5)})
	if w != time.Hour || l != 5 {
		t.Errorf("numeric args = (%v, %d), want (1h, 5)", w, l)
	}
}
