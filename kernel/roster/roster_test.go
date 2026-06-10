// SPDX-License-Identifier: MIT

package roster

import (
	"strings"
	"testing"
	"time"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

// TestValidate_SlugRules: the slug is the agent's address — lowercase, no
// spaces, bounded; bad shapes are rejected before anything persists.
func TestValidate_SlugRules(t *testing.T) {
	good := []string{"researcher", "ops-watcher", "a", "r2.d2", "x_1"}
	for _, s := range good {
		if err := Validate(Profile{Slug: s}); err != nil {
			t.Fatalf("slug %q rejected: %v", s, err)
		}
	}
	bad := []string{"", "Researcher", "has space", "-leading", ".leading", "_leading",
		strings.Repeat("a", 65), "ünïcode"}
	for _, s := range bad {
		if err := Validate(Profile{Slug: s}); err == nil {
			t.Fatalf("slug %q accepted, want rejection", s)
		}
	}
}

// TestValidate_Bounds: soul size, fallback count/emptiness, negative cost, and
// workdir escape attempts are all rejected.
func TestValidate_Bounds(t *testing.T) {
	if err := Validate(Profile{Slug: "a", Soul: strings.Repeat("x", maxSoulBytes+1)}); err == nil {
		t.Fatal("oversized soul accepted")
	}
	if err := Validate(Profile{Slug: "a", Fallbacks: make([]string, maxFallbacks+1)}); err == nil {
		t.Fatal("too many fallbacks accepted")
	}
	if err := Validate(Profile{Slug: "a", Fallbacks: []string{"m1", " "}}); err == nil {
		t.Fatal("blank fallback accepted")
	}
	if err := Validate(Profile{Slug: "a", MaxCostMc: -1}); err == nil {
		t.Fatal("negative cost ceiling accepted")
	}
	for _, w := range []string{"/abs", "..", "../up", "a/../../b", "a/.."} {
		if err := Validate(Profile{Slug: "a", Workdir: w}); err == nil {
			t.Fatalf("escaping workdir %q accepted", w)
		}
	}
	if err := Validate(Profile{Slug: "a", Workdir: "team/researcher"}); err != nil {
		t.Fatalf("relative workdir rejected: %v", err)
	}
}

// TestAdd_AssignsIdentityAndDefaults: kernel assigns id/enabled/timestamps,
// name defaults to the slug, and the slug must be unique.
func TestAdd_AssignsIdentityAndDefaults(t *testing.T) {
	s := openStore(t)
	p, err := s.Add(Profile{Slug: "researcher", Soul: "You research.", ID: "spoofed", Enabled: false})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if p.ID == "" || p.ID == "spoofed" {
		t.Fatalf("ID not kernel-assigned: %q", p.ID)
	}
	if !p.Enabled || p.CreatedMS == 0 || p.UpdatedMS == 0 {
		t.Fatalf("lifecycle defaults wrong: %+v", p)
	}
	if p.Name != "researcher" {
		t.Fatalf("name should default to slug, got %q", p.Name)
	}
	if _, err := s.Add(Profile{Slug: "researcher"}); err == nil {
		t.Fatal("duplicate slug accepted")
	}
}

// TestGet_ByIdAndSlug: profiles resolve by either handle.
func TestGet_ByIdAndSlug(t *testing.T) {
	s := openStore(t)
	p, _ := s.Add(Profile{Slug: "ops"})
	if got, ok := s.Get(p.ID); !ok || got.Slug != "ops" {
		t.Fatal("lookup by id failed")
	}
	if got, ok := s.Get("ops"); !ok || got.ID != p.ID {
		t.Fatal("lookup by slug failed")
	}
	if _, ok := s.Get("ghost"); ok {
		t.Fatal("unknown ref resolved")
	}
}

// TestUpdate_ProtectsIdentity: mutate can change mutable fields but never the
// id/slug/created/enabled, and a mutation into an invalid state rolls back.
func TestUpdate_ProtectsIdentity(t *testing.T) {
	s := openStore(t)
	p, _ := s.Add(Profile{Slug: "ops", Soul: "v1"})
	got, err := s.Update("ops", func(dst *Profile) {
		dst.Soul = "v2"
		dst.Slug = "hijacked"
		dst.ID = "hijacked"
		dst.Enabled = false
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Soul != "v2" || got.Slug != "ops" || got.ID != p.ID || !got.Enabled {
		t.Fatalf("identity not protected: %+v", got)
	}
	if _, err := s.Update("ops", func(dst *Profile) { dst.MaxCostMc = -5 }); err == nil {
		t.Fatal("invalid mutation accepted")
	}
	if cur, _ := s.Get("ops"); cur.MaxCostMc != 0 || cur.Soul != "v2" {
		t.Fatalf("failed mutation not rolled back: %+v", cur)
	}
	if _, err := s.Update("ghost", func(*Profile) {}); err != ErrNotFound {
		t.Fatalf("unknown ref: err = %v, want ErrNotFound", err)
	}
}

// TestSetEnabled_And_Remove: pause/resume round-trips; remove by slug works
// and reports existence.
func TestSetEnabled_And_Remove(t *testing.T) {
	s := openStore(t)
	_, _ = s.Add(Profile{Slug: "ops"})
	if p, err := s.SetEnabled("ops", false); err != nil || p.Enabled {
		t.Fatalf("pause failed: %+v %v", p, err)
	}
	if p, err := s.SetEnabled("ops", true); err != nil || !p.Enabled {
		t.Fatalf("resume failed: %+v %v", p, err)
	}
	if _, err := s.SetEnabled("ghost", true); err != ErrNotFound {
		t.Fatalf("unknown ref: err = %v, want ErrNotFound", err)
	}
	gone, ok, err := s.Remove("ops")
	if err != nil || !ok || gone.Slug != "ops" {
		t.Fatalf("remove failed: %+v %v %v", gone, ok, err)
	}
	if _, ok, _ := s.Remove("ops"); ok {
		t.Fatal("second remove reported existence")
	}
	if s.Count() != 0 {
		t.Fatalf("count = %d after remove", s.Count())
	}
}

// TestPersistence_SurvivesReopen: the roster is durable — a second Open on the
// same dir sees every profile with fields intact.
func TestPersistence_SurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	s1, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	want, _ := s1.Add(Profile{Slug: "researcher", Soul: "You research deeply.",
		Model: "deepseek-v4-pro", Fallbacks: []string{"m2", "m3"}, MaxCostMc: 500_000,
		MemoryScope: "research", Workdir: "research", Description: "the digger"})
	_, _ = s1.Add(Profile{Slug: "ops"})

	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if s2.Count() != 2 {
		t.Fatalf("count after reopen = %d, want 2", s2.Count())
	}
	got, ok := s2.Get("researcher")
	if !ok {
		t.Fatal("researcher lost on reopen")
	}
	if got.ID != want.ID || got.Soul != want.Soul || got.Model != want.Model ||
		got.MaxCostMc != want.MaxCostMc || got.MemoryScope != want.MemoryScope ||
		got.Workdir != want.Workdir || len(got.Fallbacks) != 2 {
		t.Fatalf("profile fields lost on reopen: %+v", got)
	}
}

// TestList_DeterministicOrder: creation order, stable.
func TestList_DeterministicOrder(t *testing.T) {
	s := openStore(t)
	// Deterministic clock: real adds can share a millisecond, flipping the
	// creation-time sort onto the ID tiebreaker (flaked once in CI-local).
	var tick int64 = 1000
	s.now = func() time.Time { tick++; return time.UnixMilli(tick) }
	for _, slug := range []string{"c", "a", "b"} {
		if _, err := s.Add(Profile{Slug: slug}); err != nil {
			t.Fatalf("Add %s: %v", slug, err)
		}
	}
	got := s.List()
	if len(got) != 3 || got[0].Slug != "c" || got[1].Slug != "a" || got[2].Slug != "b" {
		t.Fatalf("order wrong: %+v", got)
	}
}
