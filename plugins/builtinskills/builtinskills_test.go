// SPDX-License-Identifier: MIT

package builtinskills

import (
	"testing"

	"github.com/agezt/agezt/kernel/skill"
)

// newForge builds a real Forge over a temp store + bundle store — the same wiring
// the daemon uses, so the seed test exercises Create + bundle materialization +
// promotion end to end.
func newForge(t *testing.T) *skill.Forge {
	t.Helper()
	dir := t.TempDir()
	store, err := skill.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	f := skill.NewForge(store, nil)
	bundles, err := skill.OpenBundles(dir)
	if err != nil {
		t.Fatalf("open bundles: %v", err)
	}
	f.SetBundles(bundles)
	return f
}

func TestSeedAll_InstallsActiveBrowserUse(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	if len(seeded) != len(builtinBundles) {
		t.Fatalf("seeded %d bundles, want %d", len(seeded), len(builtinBundles))
	}

	var bu *Seeded
	for i := range seeded {
		if seeded[i].Name == "browser-use" {
			bu = &seeded[i]
		}
	}
	if bu == nil {
		t.Fatalf("browser-use not seeded: %+v", seeded)
	}
	if !bu.Created {
		t.Errorf("first seed should create the skill")
	}
	if bu.Status != skill.StatusActive {
		t.Errorf("browser-use status = %q, want active (in the retrieval pool)", bu.Status)
	}

	// The bundle's scripts/reference must be materialized on disk.
	files, err := f.Bundles().List("browser-use")
	if err != nil {
		t.Fatalf("list bundle: %v", err)
	}
	wantFiles := map[string]bool{"scripts/browse.mjs": false, "scripts/setup.sh": false, "reference/actions.md": false}
	for _, rel := range files {
		if _, ok := wantFiles[rel]; ok {
			wantFiles[rel] = true
		}
	}
	for rel, found := range wantFiles {
		if !found {
			t.Errorf("bundle missing %q (got %v)", rel, files)
		}
	}

	// The driver script is real, non-empty.
	driver, err := f.Bundles().Read("browser-use", "scripts/browse.mjs")
	if err != nil || len(driver) == 0 {
		t.Errorf("browse.mjs unreadable/empty: %v", err)
	}
}

func TestSeedAll_Idempotent(t *testing.T) {
	f := newForge(t)
	if _, err := SeedAll(f, ""); err != nil {
		t.Fatalf("first SeedAll: %v", err)
	}
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("second SeedAll: %v", err)
	}
	for _, s := range seeded {
		if s.Created {
			t.Errorf("re-seed created %q again (should dedupe on content address)", s.Name)
		}
		if s.Status != skill.StatusActive {
			t.Errorf("re-seed left %q at %q, want active", s.Name, s.Status)
		}
	}
}
