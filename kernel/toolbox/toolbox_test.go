// SPDX-License-Identifier: MIT

package toolbox

import (
	"context"
	"strings"
	"testing"
)

func TestResolveInstall_PicksFirstAvailableManager(t *testing.T) {
	tool := Tool{
		Name: "demo",
		Recipes: map[string][]Recipe{
			"windows": {
				{Manager: "winget", Install: []string{"winget", "install", "Demo"}},
				{Manager: "choco", Install: []string{"choco", "install", "demo"}},
			},
			"linux": {{Manager: "apt", Install: []string{"apt-get", "install", "-y", "demo"}}},
		},
	}

	// winget present → winget wins (first candidate).
	r, ok := ResolveInstall(tool, "windows", map[string]bool{"winget": true, "choco": true})
	if !ok || r.Manager != "winget" {
		t.Fatalf("want winget, got %+v ok=%v", r, ok)
	}
	// only choco present → choco (skips the unavailable winget).
	r, ok = ResolveInstall(tool, "windows", map[string]bool{"choco": true})
	if !ok || r.Manager != "choco" {
		t.Fatalf("want choco, got %+v ok=%v", r, ok)
	}
	// no manager present → not installable.
	if _, ok := ResolveInstall(tool, "windows", map[string]bool{}); ok {
		t.Error("no manager present should be not-installable")
	}
	// no recipe for the OS → not installable even with managers.
	if _, ok := ResolveInstall(tool, "darwin", map[string]bool{"brew": true}); ok {
		t.Error("no darwin recipe should be not-installable")
	}
}

func TestBinByOS(t *testing.T) {
	fd := Tool{Name: "fd", BinByOS: map[string]string{"linux": "fdfind"}}
	if fd.bin("linux") != "fdfind" {
		t.Errorf("linux fd binary should be fdfind, got %q", fd.bin("linux"))
	}
	if fd.bin("windows") != "fd" {
		t.Errorf("windows fd binary should be fd, got %q", fd.bin("windows"))
	}
}

func TestVersionArgsDefault(t *testing.T) {
	if got := (Tool{Name: "x"}).versionArgs(); len(got) != 1 || got[0] != "--version" {
		t.Errorf("default versionArgs should be [--version], got %v", got)
	}
	if got := (Tool{Name: "kubectl", VersionArgs: []string{"version", "--client"}}).versionArgs(); got[0] != "version" {
		t.Errorf("explicit versionArgs not honoured: %v", got)
	}
}

func TestFirstLineAndTailAndClip(t *testing.T) {
	if got := firstLine("\n\n  jq-1.7.1  \nextra\n"); got != "jq-1.7.1" {
		t.Errorf("firstLine: %q", got)
	}
	if got := clip("abcdef", 3); got != "abc…" {
		t.Errorf("clip: %q", got)
	}
	if got := tail("0123456789", 4); got != "…6789" {
		t.Errorf("tail: %q", got)
	}
}

func TestCatalogIntegrity(t *testing.T) {
	if len(Catalog) == 0 {
		t.Fatal("catalog is empty")
	}
	cats := map[string]bool{}
	for _, c := range Categories() {
		cats[c] = true
	}
	seen := map[string]bool{}
	for _, tool := range Catalog {
		if tool.Name == "" {
			t.Error("tool with empty name")
		}
		if seen[tool.Name] {
			t.Errorf("duplicate tool name %q", tool.Name)
		}
		seen[tool.Name] = true
		if !cats[tool.Category] {
			t.Errorf("tool %q has unknown category %q (not in Categories())", tool.Name, tool.Category)
		}
		if len(tool.Recipes) == 0 {
			t.Errorf("tool %q has no recipes", tool.Name)
		}
		for goos, recs := range tool.Recipes {
			for _, r := range recs {
				if r.Manager == "" || len(r.Install) == 0 {
					t.Errorf("tool %q / %s: recipe missing manager or install argv", tool.Name, goos)
				}
			}
		}
	}
}

func TestInstall_UnknownToolSkips(t *testing.T) {
	res := Install(context.Background(), "definitely-not-a-tool-xyz")
	if !res.Skipped || res.OK {
		t.Errorf("unknown tool should be skipped, got %+v", res)
	}
}

func TestDetect_DoesNotPanicAndCountsConsistently(t *testing.T) {
	inv := Detect(context.Background())
	if inv.OS == "" {
		t.Error("inventory OS empty")
	}
	if len(inv.Tools) != len(Catalog) {
		t.Errorf("inventory tools=%d, catalog=%d", len(inv.Tools), len(Catalog))
	}
	if inv.InstalledCount+inv.MissingCount != len(Catalog) {
		t.Errorf("installed(%d)+missing(%d) != catalog(%d)", inv.InstalledCount, inv.MissingCount, len(Catalog))
	}
	// Spot check: an installed tool carries a path; a missing one doesn't.
	for _, st := range inv.Tools {
		if st.Installed && st.Path == "" {
			t.Errorf("%s installed but no path", st.Name)
		}
		if st.Installable && st.Command == "" {
			t.Errorf("%s installable but no command shown", st.Name)
		}
	}
}

func TestManagerListSorted(t *testing.T) {
	got := ManagerList(map[string]bool{"winget": true, "choco": true, "scoop": false})
	if strings.Join(got, ",") != "choco,winget" {
		t.Errorf("ManagerList should be sorted+filtered, got %v", got)
	}
}
