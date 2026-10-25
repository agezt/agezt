// SPDX-License-Identifier: MIT

package main

// Provenance: the test host helper for the skill library (W3.55). The kernel
//             no longer opens the skill file itself — the composition root
//             does (cmd/agezt) and hands the kernel the contract's ports — so a
//             test that wants a skill library opens one and injects it, the
//             same way the daemon does.

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/modules/skills/skill"
	"github.com/agezt/agezt/kernel/runtime"
)

// withSkill gives the config a skill library: the store (a real file under a
// temp dir) and the builder the daemon injects. The kernel supplies its own
// publisher, so every transition journals through the kernel's bus.
func withSkill(t *testing.T, cfg runtime.Config) runtime.Config {
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
