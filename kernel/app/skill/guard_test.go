// SPDX-License-Identifier: MIT

package skill

import (
	"context"
	"errors"
	"testing"
)

// TestTheAppLayerRefusesWithoutACurator: the skill library is the DAEMON's
// (W3.55). A host that was given none holds a nil port, and the app layer must
// REFUSE — not panic, and not answer "nothing to do".
func TestTheAppLayerRefusesWithoutACurator(t *testing.T) {
	ctx := context.Background()

	svc := New(nil)
	if _, err := svc.List(ctx, ListInput{}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("list must refuse without a curator, got %v", err)
	}
	if _, err := svc.Get(ctx, GetInput{ID: "s-1"}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("get must refuse without a curator, got %v", err)
	}

	life := NewLifecycle(nil)
	if _, err := life.Promote(ctx, GetInput{ID: "s-1"}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("promote must refuse without a curator, got %v", err)
	}
	if _, err := life.Quarantine(ctx, ReasonInput{ID: "s-1"}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("quarantine must refuse without a curator, got %v", err)
	}
	if _, err := life.Archive(ctx, ReasonInput{ID: "s-1"}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("archive must refuse without a curator, got %v", err)
	}
	if _, err := life.Revert(ctx, GetInput{ID: "s-1"}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("revert must refuse without a curator, got %v", err)
	}
	if _, err := life.Restore(ctx, RestoreInput{ID: "s-1"}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("restore must refuse without a curator, got %v", err)
	}

	cur := NewCuration(nil, func(string) bool { return true })
	if _, err := cur.Share(ctx, GetInput{ID: "s-1"}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("share must refuse without a curator, got %v", err)
	}
	if _, err := cur.Reassign(ctx, ReassignInput{ID: "s-1", Agent: "writer"}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("reassign must refuse without a curator, got %v", err)
	}
	if _, err := cur.Import(ctx, ImportInput{Name: "s", Body: "b"}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("import must refuse without a curator, got %v", err)
	}

	obs := NewObservations(nil, nil)
	if _, err := obs.Files(ctx, GetInput{ID: "s-1"}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("files must refuse without a curator, got %v", err)
	}
	if _, err := obs.ReadFile(ctx, ReadFileInput{ID: "s-1", Path: "ref.md"}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("read-file must refuse without a curator, got %v", err)
	}
	if _, err := obs.Hygiene(ctx, HygieneInput{}); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("hygiene must refuse without a curator, got %v", err)
	}
}

// TestTheRefusalNamesItsReason: the refusal must say WHY — a bare error would
// leave the operator guessing between an unwired daemon, an empty library and
// an unknown id.
func TestTheRefusalNamesItsReason(t *testing.T) {
	if errNoSkillLibrary.Error() != "skill: the skill library is not available on this daemon" {
		t.Fatalf("the refusal says %q", errNoSkillLibrary.Error())
	}
}
