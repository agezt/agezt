// SPDX-License-Identifier: MIT

package skill

import (
	"fmt"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/modules/skills/builtins"
)

// failingForge refuses to create the named bundles and accepts the rest — the
// seam the seeding contract is written against (best-effort per bundle).
type failingForge struct {
	failOn  map[string]bool
	created int
}

func (f *failingForge) Create(_ string, spec CreateSpec) (Skill, bool, error) {
	f.created++
	if f.failOn[spec.Name] {
		return Skill{}, false, fmt.Errorf("forge refused %q", spec.Name)
	}
	return Skill{ID: fmt.Sprintf("sk-%d", f.created), Status: StatusDraft}, true, nil
}

func (f *failingForge) Promote(_, _ string) (Status, error) { return StatusActive, nil }

func (f *failingForge) Get(_ string) (Skill, bool, error) { return Skill{}, false, nil }

// builtinSkillNames returns the skill name each embedded bundle installs under
// (the bundle directory name and the skill name differ: "browseruse" installs
// "browser-use").
func builtinSkillNames(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, dir := range builtins.Names() {
		md, _, err := builtins.Bundle(dir)
		if err != nil {
			t.Fatalf("Bundle(%q): %v", dir, err)
		}
		parsed, err := ParseSkillMD(md)
		if err != nil {
			t.Fatalf("ParseSkillMD(%q): %v", dir, err)
		}
		out = append(out, parsed.Name)
	}
	return out
}

// TestSeedBuiltins_OneFailingBundleDoesNotStopTheOthers: the seeding is
// best-effort per bundle. A bundle that fails to install (a malformed
// SKILL.md, a store that refuses the write) must not take the rest of the
// built-ins down with it — the next boot would then be missing capabilities
// that were perfectly fine.
func TestSeedBuiltins_OneFailingBundleDoesNotStopTheOthers(t *testing.T) {
	names := builtinSkillNames(t)
	if len(names) < 2 {
		t.Fatalf("need at least two built-ins, got %d", len(names))
	}
	dirs := builtins.Names()
	f := &failingForge{failOn: map[string]bool{names[0]: true}}

	seeded, err := SeedBuiltins(f, "")
	if err == nil {
		t.Fatal("SeedBuiltins should report the failing bundle")
	}
	if !strings.Contains(err.Error(), dirs[0]) {
		t.Errorf("the error does not name the failing bundle %q: %v", dirs[0], err)
	}
	if len(seeded) != len(dirs)-1 {
		t.Fatalf("seeded %d bundles, want %d (only the failing one is dropped)", len(seeded), len(dirs)-1)
	}
	var sawSecond bool
	for _, s := range seeded {
		if s.Name == names[1] {
			sawSecond = true
		}
	}
	if !sawSecond {
		t.Errorf("the bundle after the failing one (%q) was never seeded: %+v", names[1], seeded)
	}
}

// TestSeedBuiltins_ReportsTheFirstFailure: when several bundles fail, the
// returned error names the FIRST failure — the one the operator has to look
// at first. Overwriting it with the last failure would hide the root cause
// behind a later, downstream symptom.
func TestSeedBuiltins_ReportsTheFirstFailure(t *testing.T) {
	names := builtinSkillNames(t)
	if len(names) < 2 {
		t.Fatalf("need at least two built-ins, got %d", len(names))
	}
	dirs := builtins.Names()
	f := &failingForge{failOn: map[string]bool{names[0]: true, names[1]: true}}

	_, err := SeedBuiltins(f, "")
	if err == nil {
		t.Fatal("SeedBuiltins should report a failure")
	}
	if !strings.Contains(err.Error(), dirs[0]) {
		t.Errorf("the error names %v, want the first failure %q", err, dirs[0])
	}
	if strings.Contains(err.Error(), dirs[1]) {
		t.Errorf("the error names the LAST failure %q, want the first one", dirs[1])
	}
}
