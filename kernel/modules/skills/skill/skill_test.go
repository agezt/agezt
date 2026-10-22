// SPDX-License-Identifier: MIT

package skill

import "testing"

const day = 24 * 60 * 60 * 1000

func TestContentIDStableAndVersioning(t *testing.T) {
	a := ContentID("diagnose-ci", "step 1\nstep 2")
	b := ContentID("  Diagnose-CI ", "step 1\nstep 2")
	if a != b {
		t.Fatalf("name should be normalized: %s != %s", a, b)
	}
	// A changed body is a new version (different id) — the basis for lineage.
	if ContentID("diagnose-ci", "step 1") == ContentID("diagnose-ci", "step 1\nstep 2") {
		t.Errorf("different bodies must address differently (versioning)")
	}
}

func TestLegalTransitions(t *testing.T) {
	cases := []struct {
		from, to Status
		want     bool
	}{
		{StatusDraft, StatusShadow, true},
		{StatusShadow, StatusActive, true},
		{StatusActive, StatusQuarantined, true},
		{StatusQuarantined, StatusActive, true},
		{StatusDraft, StatusActive, false}, // can't skip shadow
		{StatusArchived, StatusActive, false},
		{StatusActive, StatusShadow, false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.want {
			t.Errorf("CanTransition(%s,%s)=%v want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestPromoteTarget(t *testing.T) {
	if got, ok := PromoteTarget(StatusDraft); !ok || got != StatusShadow {
		t.Errorf("draft promotes to shadow, got %s ok=%v", got, ok)
	}
	if got, ok := PromoteTarget(StatusShadow); !ok || got != StatusActive {
		t.Errorf("shadow promotes to active, got %s ok=%v", got, ok)
	}
	if _, ok := PromoteTarget(StatusActive); ok {
		t.Errorf("active has no further promotion")
	}
}

func TestFileStoreRoundTripAndActiveCount(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	draft := Skill{ID: "a", Name: "x", Body: "do x", Status: StatusDraft, Version: DefaultVersion, CreatedMS: 1, LastSeenMS: 1}
	active := Skill{ID: "b", Name: "y", Body: "do y", Status: StatusActive, Version: DefaultVersion, CreatedMS: 2, LastSeenMS: 2}
	if err := s.Put(draft); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(active); err != nil {
		t.Fatal(err)
	}
	// Count is active-only.
	if s.Count() != 1 {
		t.Errorf("active count = %d want 1", s.Count())
	}
	// Reopen → survives.
	s2, _ := Open(dir)
	got, ok, _ := s2.Get("b")
	if !ok || got.Status != StatusActive {
		t.Fatalf("active skill did not persist: %+v ok=%v", got, ok)
	}
}

func TestPutValidates(t *testing.T) {
	s, _ := Open(t.TempDir())
	if err := s.Put(Skill{ID: "x", Body: "  "}); err != ErrEmptyBody {
		t.Errorf("empty body should be ErrEmptyBody, got %v", err)
	}
	if err := s.Put(Skill{ID: "", Body: "ok"}); err == nil {
		t.Errorf("missing id should error")
	}
}

func sk(id, name, desc string, triggers []string, status Status, lastSeen int64) Skill {
	return Skill{ID: id, Name: name, Description: desc, Triggers: triggers, Body: "b", Status: status, CreatedMS: lastSeen, LastSeenMS: lastSeen}
}

func TestRetrieveActiveOnlyAndRanks(t *testing.T) {
	now := int64(10 * day)
	sks := []Skill{
		sk("ci", "diagnose-ci", "diagnose failing CI builds", []string{"ci", "build"}, StatusActive, now),
		sk("draft", "diagnose-ci-v2", "diagnose failing CI builds better", nil, StatusDraft, now),
		sk("deploy", "deploy-app", "deploy the application", nil, StatusActive, now),
	}
	hits := Retrieve(sks, "my CI build is failing", 5, now)
	if len(hits) != 1 {
		t.Fatalf("only the active CI skill should match, got %d: %+v", len(hits), hits)
	}
	if hits[0].Skill.ID != "ci" {
		t.Errorf("matched %s, want ci", hits[0].Skill.ID)
	}
}

func TestRetrieveEmptyIntent(t *testing.T) {
	sks := []Skill{sk("a", "x", "y", nil, StatusActive, 0)}
	if hits := Retrieve(sks, "   ", 5, 0); len(hits) != 0 {
		t.Errorf("empty intent should match nothing, got %+v", hits)
	}
}
