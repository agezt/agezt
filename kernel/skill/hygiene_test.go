// SPDX-License-Identifier: MIT

package skill

import (
	"testing"
	"time"
)

// TestHygiene_FlagsIdleActiveSkills asserts the cleanup report (M858) flags only
// active skills that are never-used or long-unused, gives brand-new skills a
// grace period, and ignores non-active skills.
func TestHygiene_FlagsIdleActiveSkills(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	f := NewForge(s, nil)
	const day = int64(24 * 60 * 60 * 1000)
	now := int64(1_700_000_000_000)
	f.now = func() time.Time { return time.UnixMilli(now) }

	put := func(id string, status Status, uses int, lastUsed, created int64) {
		if err := s.Put(Skill{ID: id, Name: id, Body: "body", Status: status,
			Metrics: Metrics{Uses: uses, LastUsedMS: lastUsed}, CreatedMS: created}); err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
	}
	put("fresh", StatusActive, 5, now-1*day, now-60*day)   // used recently → keep
	put("neveruse", StatusActive, 0, 0, now-60*day)        // never used, old → idle
	put("stale", StatusActive, 3, now-90*day, now-100*day) // used long ago → idle
	put("newbie", StatusActive, 0, 0, now-1*day)           // never used but brand new → grace, keep
	put("shadowy", StatusShadow, 0, 0, now-60*day)         // not active → ignored

	cutoff := now - 30*day
	rep, err := f.Hygiene(cutoff)
	if err != nil {
		t.Fatalf("hygiene: %v", err)
	}
	if rep.Active != 4 {
		t.Errorf("active = %d, want 4", rep.Active)
	}
	idle := map[string]bool{}
	for _, sk := range rep.Idle {
		idle[sk.ID] = true
	}
	for _, id := range []string{"neveruse", "stale"} {
		if !idle[id] {
			t.Errorf("%q should be flagged idle", id)
		}
	}
	for _, id := range []string{"fresh", "newbie", "shadowy"} {
		if idle[id] {
			t.Errorf("%q should NOT be flagged idle", id)
		}
	}
	// Sorted oldest-seen first: never-used (LastUsedMS 0) leads.
	if len(rep.Idle) > 0 && rep.Idle[0].ID != "neveruse" {
		t.Errorf("idle[0] = %q, want neveruse (oldest-seen first)", rep.Idle[0].ID)
	}
}
