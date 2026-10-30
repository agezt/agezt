// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
	skillmodule "github.com/agezt/agezt/kernel/modules/skills/skill"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the skill family's ports — the APPLICATION fills
// them from the kernel handle.
//
// The test pins that binding end to end: the daemon's own skill library
// answers for the daemon's kernel, and the request's library follows the
// ROUTED kernel — a tenant's skills are the tenant's skills.
//
// The skill library is the daemon's file (W3.55), so the test opens one per
// kernel: two libraries, two skills, and the ports must not mix them.
func TestTheAdapterBindsTheSkillPortsToItsOwnKernel(t *testing.T) {
	root, rootForge := kernelWithSkillLibrary(t)
	defer root.Close()
	tenant, tenantForge := kernelWithSkillLibrary(t)
	defer tenant.Close()

	rootSkill, _, err := rootForge.Create("corr", skillmodule.CreateSpec{Name: "daemon-skill", Body: "the daemon's"})
	if err != nil {
		t.Fatalf("Create in the daemon's library: %v", err)
	}
	tenantSkill, _, err := tenantForge.Create("corr", skillmodule.CreateSpec{Name: "tenant-skill", Body: "the tenant's"})
	if err != nil {
		t.Fatalf("Create in the tenant's library: %v", err)
	}

	s := NewServer(root, t.TempDir())
	ctx := context.WithValue(context.Background(), appHostKey{}, appHost{scope: scopeForKernel(tenant)})

	// The daemon's own ports answer for its own kernel.
	daemonSkills, err := s.skillPorts().List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !skillsContain(daemonSkills, rootSkill.ID) {
		t.Error("the daemon's skill ports do not answer for its own kernel")
	}
	if skillsContain(daemonSkills, tenantSkill.ID) {
		t.Error("the daemon's skill ports see the tenant's skill")
	}

	// The request's ports follow the routed kernel.
	tenantSkills, err := requestSkillPorts(ctx).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !skillsContain(tenantSkills, tenantSkill.ID) {
		t.Error("the request's skill ports do not answer for the routed kernel")
	}
	if skillsContain(tenantSkills, rootSkill.ID) {
		t.Error("the request's skill ports see the daemon's skill — a tenant's skills are the tenant's skills")
	}
}

// kernelWithSkillLibrary opens a kernel that holds a skill library of its own.
func kernelWithSkillLibrary(t *testing.T) (*runtime.Kernel, skillapi.Curator) {
	t.Helper()
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
	return k, forge
}

// skillsContain reports whether the skills hold one with the id.
func skillsContain(skills []skillapi.Skill, id string) bool {
	for _, s := range skills {
		if s.ID == id {
			return true
		}
	}
	return false
}
