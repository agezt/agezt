// SPDX-License-Identifier: MIT

package builtintools

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/modules/skills/skill"
	"github.com/agezt/agezt/kernel/runtime"
)

// withSkill opens the skill library the composition root would open (W3.55)
// and hands the kernel the contract's store port and the BUILDER.
//
// The library's file is the DAEMON's: the kernel holds the contract's ports,
// and the daemon opens the file and names the module. The skill tool is a
// PLUGIN and may not name the module — it asks for the contract's ports, and
// the port must carry the live implementation, not a copy of it.
func withSkill(t *testing.T, cfg runtime.Config) runtime.Config {
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
