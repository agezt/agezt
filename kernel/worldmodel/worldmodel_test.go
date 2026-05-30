// SPDX-License-Identifier: MIT

package worldmodel

import (
	"path/filepath"
	"testing"
)

func TestEntityIDStableAndNormalized(t *testing.T) {
	a := EntityID(KindProject, "Lictor")
	b := EntityID(KindProject, "  lictor ")
	if a != b {
		t.Fatalf("EntityID should normalize kind/name: %s != %s", a, b)
	}
	if EntityID(KindProject, "Lictor") == EntityID(KindRepo, "Lictor") {
		t.Errorf("different kinds must not collide")
	}
	// Domain prefix keeps entity and relation id spaces disjoint.
	if EntityID(KindTopic, "x") == RelationID("a", VerbOwns, "b") {
		t.Errorf("entity and relation id spaces overlapped")
	}
}

func TestNormalizeKindVerb(t *testing.T) {
	if NormalizeKind("") != DefaultKind {
		t.Errorf("empty kind should default to %s", DefaultKind)
	}
	if NormalizeKind("PROJECT") != KindProject {
		t.Errorf("kind should fold case")
	}
	if got := NormalizeKind("spaceship"); got != "spaceship" {
		t.Errorf("unknown kind should be kept verbatim, got %q", got)
	}
	if !ValidKind(KindRepo) || ValidKind("spaceship") {
		t.Errorf("ValidKind membership wrong")
	}
	if NormalizeVerb("") != DefaultVerb || NormalizeVerb("OWNS") != VerbOwns {
		t.Errorf("verb normalization wrong")
	}
}

func TestFileStoreRoundTripAndPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	e := Entity{ID: EntityID(KindProject, "Lictor"), Kind: KindProject, Name: "Lictor", CreatedMS: 10, LastSeenMS: 10, Weight: 1}
	if err := s.PutEntity(e); err != nil {
		t.Fatalf("put entity: %v", err)
	}
	r := Relation{ID: RelationID(e.ID, VerbDependsOn, "go"), From: e.ID, Verb: VerbDependsOn, To: "go", CreatedMS: 11, LastSeenMS: 11, Weight: 1}
	if err := s.PutRelation(r); err != nil {
		t.Fatalf("put relation: %v", err)
	}

	// Reopen from disk → data survives.
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok, _ := s2.GetEntity(e.ID)
	if !ok || got.Name != "Lictor" {
		t.Fatalf("entity did not persist: %+v ok=%v", got, ok)
	}
	gr, ok, _ := s2.GetRelation(r.ID)
	if !ok || gr.Verb != VerbDependsOn {
		t.Fatalf("relation did not persist: %+v ok=%v", gr, ok)
	}
	if s2.Count() != 1 {
		t.Errorf("entity count = %d want 1", s2.Count())
	}
	if _, err := Open(filepath.Join(dir, "sub")); err != nil {
		t.Errorf("open fresh subdir should succeed: %v", err)
	}
}

func TestPutEntityValidates(t *testing.T) {
	s, _ := Open(t.TempDir())
	if err := s.PutEntity(Entity{ID: "x", Name: "  "}); err != ErrEmptyName {
		t.Errorf("empty name should be ErrEmptyName, got %v", err)
	}
	if err := s.PutEntity(Entity{ID: "", Name: "ok"}); err == nil {
		t.Errorf("missing id should error")
	}
	if err := s.PutRelation(Relation{ID: "r", From: "", To: "b"}); err == nil {
		t.Errorf("relation without from should error")
	}
}

func TestAllSortedDeterministically(t *testing.T) {
	s, _ := Open(t.TempDir())
	_ = s.PutEntity(Entity{ID: "b", Name: "b", CreatedMS: 2})
	_ = s.PutEntity(Entity{ID: "a", Name: "a", CreatedMS: 1})
	_ = s.PutEntity(Entity{ID: "c", Name: "c", CreatedMS: 1})
	all, _ := s.AllEntities()
	// oldest first; ties (a,c at t=1) broken by id.
	got := []string{all[0].ID, all[1].ID, all[2].ID}
	want := []string{"a", "c", "b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v want %v", got, want)
		}
	}
}
