// SPDX-License-Identifier: MIT

package skill

import (
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
	skillmodule "github.com/agezt/agezt/kernel/modules/skills/skill"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The skill family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the skills the kernel's library holds
// are the skills the port lists, and the skill the port archives is really
// archived. A port bound to another library would leave the operator reading a
// catalog the daemon does not keep.
func TestTheSkillPortsReachTheKernelLibrary(t *testing.T) {
	store, err := skillmodule.Open(t.TempDir())
	if err != nil {
		t.Fatalf("skill.Open: %v", err)
	}
	var forge skillapi.Curator
	cfg := runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}
	cfg.SkillStore = store
	cfg.SkillCurator = func(s skillapi.Store, pub skillapi.Publisher) skillapi.Curator {
		forge = skillmodule.NewForge(s, pub)
		return forge
	}
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Reader() == nil {
		t.Fatal("the skill port holds no curator — the kernel holds a skill library")
	}

	created, ok, err := forge.Create("corr", skillmodule.CreateSpec{Name: "port-skill", Body: "do the thing"})
	if err != nil || !ok {
		t.Fatalf("Create: %v (created=%v)", err, ok)
	}

	// The port lists the library's skills.
	skills, err := ports.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, s := range skills {
		if s.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("the skill port listed %d skills, none of them the one the kernel's library holds", len(skills))
	}

	// And it archives one of them: the skill the port archives really is
	// archived. (A new skill is a DRAFT, so the archive is measured against
	// the archived status and not against "it is no longer active".)
	if err := ports.Archive(created.ID, "the test is done"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	after, err := ports.List()
	if err != nil {
		t.Fatalf("List after the archive: %v", err)
	}
	archived := false
	for _, s := range after {
		if s.ID != created.ID {
			continue
		}
		if s.Status != skillapi.StatusArchived {
			t.Fatalf("the skill the port archived is %q, want %q", s.Status, skillapi.StatusArchived)
		}
		archived = true
	}
	if !archived {
		t.Fatalf("the skill the port archived is gone from the library")
	}
}

// A host with no skill library must not call into nothing.
//
// The adapter used to spell "there is no skill library" itself, in three
// places across two files — twice as a nil check and once as a refusal it
// wrote out in full. The application spells it once.
func TestTheSkillPortsSurviveAHostWithNoLibrary(t *testing.T) {
	ports := PortsFromKernel(nil)
	skills, err := ports.List()
	if err != nil {
		t.Fatalf("List on a host with no library: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("a host with no library lists %d skills", len(skills))
	}
	if err := ports.Archive("s-1", "because"); !errors.Is(err, errNoSkillLibrary) {
		t.Fatalf("Archive on a host with no library: %v, want the library's refusal", err)
	}
	if ports.Reader() != nil {
		t.Fatal("a host with no library hands out a reader")
	}
}
