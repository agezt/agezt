// SPDX-License-Identifier: MIT

package skill

import (
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/modules/skills/builtins"
)

// errForge returns errors from Create to exercise SeedBuiltins' firstErr path.
type errForge struct {
	errOnCreate bool
	created     int
}

func (f *errForge) Create(_ string, _ CreateSpec) (Skill, bool, error) {
	f.created++
	if f.errOnCreate {
		return Skill{}, false, errors.New("forge error")
	}
	return Skill{ID: "sk-" + itoa(f.created), Status: StatusDraft}, true, nil
}

func (f *errForge) Promote(_, _ string) (Status, error) {
	return StatusActive, nil
}

func (f *errForge) Get(_ string) (Skill, bool, error) {
	return Skill{}, false, nil
}

func itoa(n int) string {
	if n == 1 {
		return "1"
	}
	return "2" // simplified for tests; real bundles are many
}

func TestBundle_NonexistentName(t *testing.T) {
	_, _, err := builtins.Bundle("nonexistent-bundle")
	if err == nil {
		t.Fatal("Bundle with nonexistent name should error")
	}
}

func TestSeedBuiltins_ErrorPath(t *testing.T) {
	// A forge that errors on Create: SeedBuiltins should collect firstErr but
	// still attempt every bundle.
	f := &errForge{errOnCreate: true}
	seeded, err := SeedBuiltins(f, "")
	if err == nil {
		t.Fatal("SeedBuiltins with error forge should return an error")
	}
	if len(seeded) != 0 {
		t.Fatalf("SeedBuiltins should return empty seeded list on full failure, got %d", len(seeded))
	}
}

func TestSeedBuiltins_NoErrors(t *testing.T) {
	f := &errForge{}
	seeded, err := SeedBuiltins(f, "")
	if err != nil {
		t.Fatalf("SeedBuiltins with happy forge should not error: %v", err)
	}
	// With a forge that always succeeds, all bundles are seeded.
	if len(seeded) != len(builtins.Names()) {
		t.Fatalf("SeedBuiltins returned %d seeded, want %d", len(seeded), len(builtins.Names()))
	}
}
