// SPDX-License-Identifier: MIT

package skill

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

func TestParseActivationDirective(t *testing.T) {
	got := ParseActivationDirective("  \n/skills diagnose-ci, webresearch\nfix the failing build")
	if !got.Explicit {
		t.Fatal("expected explicit directive")
	}
	if len(got.Refs) != 2 || got.Refs[0] != "diagnose-ci" || got.Refs[1] != "webresearch" {
		t.Fatalf("refs = %v, want [diagnose-ci webresearch]", got.Refs)
	}
	if got.CleanIntent != "fix the failing build" {
		t.Fatalf("clean intent = %q", got.CleanIntent)
	}
}

func TestParseActivationDirective_BareKeepsIntent(t *testing.T) {
	got := ParseActivationDirective("/skill diagnose-ci")
	if !got.Explicit || len(got.Refs) != 1 {
		t.Fatalf("directive parse failed: %+v", got)
	}
	if got.CleanIntent != "/skill diagnose-ci" {
		t.Fatalf("bare directive should keep original intent, got %q", got.CleanIntent)
	}
}

func TestActivateExplicitFor(t *testing.T) {
	f, j := newTestForge(t)
	sk, _, err := f.Create("seed", CreateSpec{Name: "diagnose-ci", Description: "fix CI", Body: "read logs"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Promote("seed", sk.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Promote("seed", sk.ID); err != nil {
		t.Fatal(err)
	}
	draft, _, err := f.Create("seed", CreateSpec{Name: "draft-only", Description: "draft", Body: "do not inject"})
	if err != nil || draft.ID == "" {
		t.Fatalf("draft create: %v", err)
	}

	hits, missing, err := f.ActivateExplicitFor("corr", "", "please help", []string{"diagnose-ci", "draft-only", "missing"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Skill.ID != sk.ID {
		t.Fatalf("hits = %+v, want only active diagnose-ci", hits)
	}
	if len(missing) != 2 || missing[0] != "draft-only" || missing[1] != "missing" {
		t.Fatalf("missing = %v, want [draft-only missing]", missing)
	}

	var payload struct {
		Activation string   `json:"activation"`
		Refs       []string `json:"refs"`
		Missing    []string `json:"missing"`
		IDs        []string `json:"ids"`
	}
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindSkillActivated && e.CorrelationID == "corr" {
			_ = json.Unmarshal(e.Payload, &payload)
		}
		return nil
	})
	if payload.Activation != "explicit" || len(payload.IDs) != 1 || payload.IDs[0] != sk.ID {
		t.Fatalf("activation payload = %+v", payload)
	}
}
