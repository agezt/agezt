// SPDX-License-Identifier: MIT

package configcenter

import (
	"testing"
)

// --- ConfigEntry chaining helpers (AllowAgent / DenyAgent dedup paths) ---

func TestAllowAgent_Dedup(t *testing.T) {
	e := NewConfigEntry("k", "v").AllowAgent("a1").AllowAgent("a2").AllowAgent("a1")
	if len(e.AllowedAgents) != 2 {
		t.Fatalf("AllowAgent should dedup: got %v", e.AllowedAgents)
	}
	if e.AllowedAgents[0] != "a1" || e.AllowedAgents[1] != "a2" {
		t.Fatalf("AllowAgent order wrong: %v", e.AllowedAgents)
	}
}

func TestDenyAgent_Dedup(t *testing.T) {
	e := NewConfigEntry("k", "v").DenyAgent("a1").DenyAgent("a2").DenyAgent("a1")
	if len(e.ExcludedAgents) != 2 {
		t.Fatalf("DenyAgent should dedup: got %v", e.ExcludedAgents)
	}
	if e.ExcludedAgents[0] != "a1" || e.ExcludedAgents[1] != "a2" {
		t.Fatalf("DenyAgent order wrong: %v", e.ExcludedAgents)
	}
}

// --- Store basic operations ---

func TestStore_Delete(t *testing.T) {
	s := NewStore()
	if err := s.Set(NewConfigEntry("k", "v")); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("k"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("k"); err == nil {
		t.Error("Get after Delete should error")
	}
	// Delete non-existent key.
	if err := s.Delete("missing"); err == nil {
		t.Error("Delete missing should error")
	}
}

func TestStore_ListByRating(t *testing.T) {
	s := NewStore()
	pub := NewConfigEntry("pub", "x").SetRating(RatingPublic)
	internal := NewConfigEntry("int", "y").SetRating(RatingInternal)
	secret := NewConfigEntry("sec", "z").SetRating(RatingSecret)
	for _, e := range []*ConfigEntry{pub, internal, secret} {
		if err := s.Set(e); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.ListByRating(RatingPublic); len(got) != 1 || got[0].Key != "pub" {
		t.Fatalf("ListByRating(public) = %d entries, want 1", len(got))
	}
	if got := s.ListByRating(RatingSecret); len(got) != 1 || got[0].Key != "sec" {
		t.Fatalf("ListByRating(secret) = %d entries, want 1", len(got))
	}
	// Empty result for unmatched rating.
	if got := s.ListByRating(RatingRestricted); len(got) != 0 {
		t.Fatalf("ListByRating(confidential) should be empty, got %d", len(got))
	}
}

func TestStore_Search(t *testing.T) {
	s := NewStore()
	for _, e := range []*ConfigEntry{
		NewConfigEntry("api.key", "123").SetRating(RatingInternal).SetTags("api"),
		NewConfigEntry("db.url", "pg://").SetRating(RatingInternal).SetTags("db"),
		NewConfigEntry("secret.token", "ghp_xxx").SetRating(RatingSecret).SetTags("auth"),
	} {
		if err := s.Set(e); err != nil {
			t.Fatal(err)
		}
	}
	// Search by key prefix — secrets are excluded.
	if got := s.Search("api", 0); len(got) != 1 || got[0].Key != "api.key" {
		t.Fatalf("Search('api') = %d entries, want 1", len(got))
	}
	// Search with limit.
	if got := s.Search("", 2); len(got) > 2 {
		t.Fatalf("Search('', limit=2) returned %d entries", len(got))
	}
	// Search by tag.
	if got := s.Search("db", 0); len(got) != 1 || got[0].Key != "db.url" {
		t.Fatalf("Search('db') = %d entries, want 1", len(got))
	}
}

func TestStore_AuditLog(t *testing.T) {
	s := NewStore()
	if got := s.GetAuditLog(0); len(got) != 0 {
		t.Fatalf("audit log on empty store should be empty, got %d", len(got))
	}
	s.AddAuditEntry(&AuditEntry{Key: "k1"})
	s.AddAuditEntry(&AuditEntry{Key: "k2"})
	s.AddAuditEntry(&AuditEntry{Key: "k3"})
	if got := s.GetAuditLog(0); len(got) != 3 {
		t.Fatalf("GetAuditLog(0) = %d, want 3", len(got))
	}
	if got := s.GetAuditLog(2); len(got) != 2 || got[0].Key != "k2" || got[1].Key != "k3" {
		t.Fatalf("GetAuditLog(2) = %#v, want last 2", got)
	}
}
