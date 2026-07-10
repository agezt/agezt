// SPDX-License-Identifier: MIT

package builtinmarket

import "testing"

func TestNew_ReturnsNonNilLibrary(t *testing.T) {
	l := New()
	if l == nil {
		t.Fatal("New() returned nil")
	}
	if l.index == nil {
		t.Error("Library index is nil")
	}
}

func TestNew_ContainsMultiplePacks(t *testing.T) {
	l := New()
	if len(l.packs) == 0 {
		t.Fatal("New() produced zero packs — expected at least the built-in skill bundles")
	}
	if len(l.index) == 0 {
		t.Fatal("Library index is empty")
	}
}

func TestNew_IncludesCombos(t *testing.T) {
	l := New()
	found := map[string]bool{"web-research-pro": false, "git-workshop": false}
	for _, p := range l.packs {
		if _, ok := found[p.Name]; ok {
			found[p.Name] = true
		}
	}
	for name, ok := range found {
		if !ok {
			t.Errorf("combo pack %q not found in library", name)
		}
	}
}

func TestLibrary_Marketplaces_ReturnsOfficialCatalogue(t *testing.T) {
	l := New()
	mps := l.Marketplaces()
	if len(mps) != 1 {
		t.Fatalf("Marketplaces() returned %d catalogues, want 1", len(mps))
	}
	mp := mps[0]
	if mp.Name != MarketplaceName {
		t.Errorf("marketplace name = %q, want %q", mp.Name, MarketplaceName)
	}
	if mp.Owner != "AGEZT" {
		t.Errorf("marketplace owner = %q, want AGEZT", mp.Owner)
	}
	if !mp.Builtin {
		t.Error("marketplace should be marked Builtin")
	}
	if mp.FormatVersion == 0 {
		t.Error("marketplace FormatVersion should not be zero")
	}
	if len(mp.Packs) == 0 {
		t.Fatal("marketplace has zero packs")
	}
}

func TestLibrary_Marketplaces_AllPacksHaveNames(t *testing.T) {
	l := New()
	mps := l.Marketplaces()
	for _, mp := range mps {
		for _, entry := range mp.Packs {
			if entry.Name == "" {
				t.Error("found a pack with empty name in marketplace")
			}
			if entry.Description == "" {
				t.Errorf("pack %q has empty description", entry.Name)
			}
		}
	}
}

func TestLibrary_ResolvePack_Found(t *testing.T) {
	l := New()
	// Resolve the first pack from the list.
	if len(l.packs) == 0 {
		t.Fatal("no packs to resolve")
	}
	name := l.packs[0].Name
	p, err := l.ResolvePack("", name, "")
	if err != nil {
		t.Fatalf("ResolvePack(%q): %v", name, err)
	}
	if p.Name != name {
		t.Errorf("resolved pack name = %q, want %q", p.Name, name)
	}
}

func TestLibrary_ResolvePack_NotFound(t *testing.T) {
	l := New()
	_, err := l.ResolvePack("", "nonexistent-pack", "")
	if err == nil {
		t.Fatal("ResolvePack for nonexistent pack should return error")
	}
}

func TestLibrary_ResolvePack_AllPacksResolvable(t *testing.T) {
	l := New()
	for _, p := range l.packs {
		got, err := l.ResolvePack("", p.Name, "")
		if err != nil {
			t.Fatalf("ResolvePack(%q): %v", p.Name, err)
		}
		if got.Name != p.Name {
			t.Errorf("resolved name = %q, want %q", got.Name, p.Name)
		}
	}
}

func TestCategoryFor_KnownBundles(t *testing.T) {
	tests := []struct {
		bundle string
		want   string
	}{
		{"browseruse", "web"},
		{"webresearch", "web"},
		{"computeruse", "desktop"},
		{"dataanalysis", "data"},
		{"sqldb", "data"},
		{"dockerservices", "dev"},
		{"gitops", "dev"},
		{"sshremote", "dev"},
		{"pdftools", "docs"},
		{"officedocs", "docs"},
		{"imagetools", "media"},
		{"archivetools", "files"},
		{"emailtools", "comms"},
		{"calendartools", "comms"},
		{"cryptotools", "security"},
		{"httpapi", "web"},
	}
	for _, tc := range tests {
		got := categoryFor(tc.bundle)
		if got != tc.want {
			t.Errorf("categoryFor(%q) = %q, want %q", tc.bundle, got, tc.want)
		}
	}
}

func TestCategoryFor_UnknownBundle(t *testing.T) {
	got := categoryFor("nonexistent")
	if got != "skills" {
		t.Errorf("categoryFor(nonexistent) = %q, want skills", got)
	}
}

func TestLibrary_MarketplacePacksHaveValidCategories(t *testing.T) {
	l := New()
	mps := l.Marketplaces()
	validCategories := map[string]bool{
		"web": true, "desktop": true, "data": true, "dev": true,
		"docs": true, "media": true, "files": true, "comms": true,
		"security": true, "skills": true, "knowledge": true,
	}
	for _, mp := range mps {
		for _, entry := range mp.Packs {
			if entry.Category == "" {
				t.Errorf("pack %q has empty category", entry.Name)
				continue
			}
			if !validCategories[entry.Category] {
				t.Errorf("pack %q has unexpected category %q", entry.Name, entry.Category)
			}
		}
	}
}

func TestLibrary_New_AllPacksHaveVersion(t *testing.T) {
	l := New()
	for _, p := range l.packs {
		if p.Version == "" {
			t.Errorf("pack %q has no version", p.Name)
		}
	}
}

func TestLibrary_New_AllPacksHaveAuthor(t *testing.T) {
	l := New()
	for _, p := range l.packs {
		if p.Author == "" {
			t.Errorf("pack %q has no author", p.Name)
		}
	}
}

func TestLibrary_New_AllPacksHaveAtLeastOneSkill(t *testing.T) {
	l := New()
	for _, p := range l.packs {
		if len(p.Skills) == 0 {
			t.Errorf("pack %q has zero skills", p.Name)
		}
	}
}

func TestLibrary_New_ComboPacksHaveToolRequirements(t *testing.T) {
	l := New()
	for _, p := range l.packs {
		if p.Name == "web-research-pro" || p.Name == "git-workshop" {
			if len(p.ToolRequirements) == 0 {
				t.Errorf("combo pack %q should have tool requirements", p.Name)
			}
		}
	}
}

func TestLibrary_New_PacksHaveTags(t *testing.T) {
	l := New()
	for _, p := range l.packs {
		if len(p.Tags) == 0 {
			t.Errorf("pack %q has no tags", p.Name)
		}
	}
}

func TestLibrary_FeaturedCombosSurfaceInEntries(t *testing.T) {
	l := New()
	mp := l.Marketplaces()[0]
	featured := map[string]bool{}
	for _, e := range mp.Packs {
		if e.Featured {
			featured[e.Name] = true
		}
	}
	for _, want := range []string{"web-research-pro", "github-automation", "data-analyst-pro", "second-brain", "browser-automation-pro"} {
		if !featured[want] {
			t.Errorf("expected %q to be featured", want)
		}
	}
	if featured["git-workshop"] {
		t.Error("git-workshop should not be featured")
	}
}

func TestLibrary_MultiSkillCombosCarryAllBundles(t *testing.T) {
	l := New()
	for name, wantSkills := range map[string]int{
		"github-automation": 2, // gitops + httpapi
		"data-analyst-pro":  2, // dataanalysis + sqldb
		"document-suite":    2, // officedocs + pdftools
		"secops-toolkit":    2, // cryptotools + sshremote
	} {
		p, err := l.ResolvePack("", name, "")
		if err != nil {
			t.Fatalf("ResolvePack(%q): %v", name, err)
		}
		if len(p.Skills) != wantSkills {
			t.Errorf("%q has %d skills, want %d", name, len(p.Skills), wantSkills)
		}
		if err := p.Validate(); err != nil {
			t.Errorf("%q does not validate: %v", name, err)
		}
	}
}

func TestNew_AllPacksGetEntry(t *testing.T) {
	l := New()
	mps := l.Marketplaces()
	if len(mps) == 0 {
		t.Fatal("no marketplaces")
	}
	for _, mp := range mps {
		seen := map[string]bool{}
		for _, entry := range mp.Packs {
			if seen[entry.Name] {
				t.Errorf("duplicate pack entry: %q", entry.Name)
			}
			seen[entry.Name] = true
		}
	}
}
