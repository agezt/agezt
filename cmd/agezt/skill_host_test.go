// SPDX-License-Identifier: MIT

package main

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/modules/skills/skill"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// withSkill opens the skill library the composition root would open (W3.55)
// and hands the kernel the contract's store port and the BUILDER.
//
// The library's file is the DAEMON's: the kernel holds the contract's ports,
// and the daemon opens the file and names the module. A host with no skill
// library holds a nil port — the skill surfaces report that the library is
// unavailable rather than calling into nothing.
//
// The builder wires the bundle store exactly as the daemon does, so a skill can
// ship reference files and scripts.
func withSkill(t *testing.T, cfg kernelruntime.Config) kernelruntime.Config {
	t.Helper()
	store, err := skill.Open(filepath.Join(cfg.BaseDir, "skills"))
	if err != nil {
		t.Fatalf("open the skill store: %v", err)
	}
	cfg.SkillStore = store
	cfg.SkillCurator = func(s skillapi.Store, pub skillapi.Publisher) skillapi.Curator {
		f := skill.NewForge(s, pub)
		if bundles, berr := skill.OpenBundles(filepath.Join(cfg.BaseDir, "skills")); berr == nil {
			f.SetBundles(bundles)
		}
		return f
	}
	return cfg
}

// withSkillNoBundles gives the config a skill library whose curator has NO
// bundle store — the shape the tool registry must survive: a forge that is
// there but has no bundles behind it.
func withSkillNoBundles(t *testing.T, cfg kernelruntime.Config) kernelruntime.Config {
	t.Helper()
	store, err := skill.Open(filepath.Join(cfg.BaseDir, "skills"))
	if err != nil {
		t.Fatalf("open the skill store: %v", err)
	}
	cfg.SkillStore = store
	cfg.SkillCurator = func(s skillapi.Store, pub skillapi.Publisher) skillapi.Curator {
		return skill.NewForge(s, pub)
	}
	return cfg
}
