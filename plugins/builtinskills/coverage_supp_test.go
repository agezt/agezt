// SPDX-License-Identifier: MIT

package builtinskills

import (
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/skill"
)

// errForge returns errors from Create to exercise SeedAll's firstErr path.
type errForge struct {
	errOnCreate bool
	created     int
}

func (f *errForge) Create(_ string, _ skill.CreateSpec) (skill.Skill, bool, error) {
	f.created++
	if f.errOnCreate {
		return skill.Skill{}, false, errors.New("forge error")
	}
	return skill.Skill{ID: "sk-" + itoa(f.created), Status: skill.StatusDraft}, true, nil
}

func (f *errForge) Promote(_, _ string) (skill.Status, error) {
	return skill.StatusActive, nil
}

func (f *errForge) Get(_ string) (skill.Skill, bool, error) {
	return skill.Skill{}, false, nil
}

func itoa(n int) string {
	if n == 1 {
		return "1"
	}
	return "2" // simplified for tests; real bundles are many
}

func TestBundle_NonexistentName(t *testing.T) {
	_, _, err := Bundle("nonexistent-bundle")
	if err == nil {
		t.Fatal("Bundle with nonexistent name should error")
	}
}

func TestSeedAll_ErrorPath(t *testing.T) {
	// A forge that errors on Create: SeedAll should collect firstErr but still
	// attempt every bundle.
	f := &errForge{errOnCreate: true}
	seeded, err := SeedAll(f, "")
	if err == nil {
		t.Fatal("SeedAll with error forge should return an error")
	}
	if len(seeded) != 0 {
		t.Fatalf("SeedAll should return empty seeded list on full failure, got %d", len(seeded))
	}
}

func TestSeedAll_NoErrors(t *testing.T) {
	f := &errForge{}
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll with happy forge should not error: %v", err)
	}
	// With a forge that always succeeds, all bundles are seeded.
	if len(seeded) != len(builtinBundles) {
		t.Fatalf("SeedAll returned %d seeded, want %d", len(seeded), len(builtinBundles))
	}
}
