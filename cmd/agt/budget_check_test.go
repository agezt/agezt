// SPDX-License-Identifier: MIT

package main

import "testing"

func TestEffectiveHeadroom(t *testing.T) {
	// Only global cap, spent below ceiling → headroom = remainder, not unlimited.
	h, unl := effectiveHeadroom([]budgetDim{{"global", 3_000_000_000, 20_000_000_000}})
	if unl || h != 17_000_000_000 {
		t.Errorf("global only: h=%d unl=%v, want 17e9,false", h, unl)
	}

	// Global + task cap: the task cap binds (smaller remaining).
	h, unl = effectiveHeadroom([]budgetDim{
		{"global", 3_000_000_000, 20_000_000_000}, // 17e9 left
		{"task:code", 400_000_000, 500_000_000},   // 100e6 left ← binds
	})
	if unl || h != 100_000_000 {
		t.Errorf("task binds: h=%d unl=%v, want 1e8,false", h, unl)
	}

	// All uncapped → unlimited.
	h, unl = effectiveHeadroom([]budgetDim{{"global", 5, 0}, {"task:x", 1, 0}})
	if !unl {
		t.Errorf("all uncapped: unl=%v, want true", unl)
	}

	// Exhausted dimension → non-positive headroom.
	h, unl = effectiveHeadroom([]budgetDim{{"global", 20_000_000_000, 20_000_000_000}})
	if unl || h != 0 {
		t.Errorf("exhausted: h=%d unl=%v, want 0,false", h, unl)
	}
	h, _ = effectiveHeadroom([]budgetDim{{"global", 21_000_000_000, 20_000_000_000}})
	if h >= 0 {
		t.Errorf("overspent: h=%d, want negative", h)
	}

	// A capped task alongside an uncapped global → the task binds.
	h, unl = effectiveHeadroom([]budgetDim{{"global", 100, 0}, {"task:y", 0, 500_000_000}})
	if unl || h != 500_000_000 {
		t.Errorf("uncapped global + capped task: h=%d unl=%v, want 5e8,false", h, unl)
	}
}
