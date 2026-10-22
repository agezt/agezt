// SPDX-License-Identifier: MIT

package builtins

import (
	"testing"
)

func TestNames_ReturnsCopy(t *testing.T) {
	got := Names()
	if len(got) == 0 {
		t.Fatal("Names() returned empty slice")
	}
	if len(got) != len(builtinBundles) {
		t.Fatalf("Names() = %d, want %d", len(got), len(builtinBundles))
	}
	// Verify it's a copy by modifying the returned slice.
	got[0] = "modified"
	if builtinBundles[0] == "modified" {
		t.Error("Names() should return a copy of builtinBundles")
	}
	// All expected bundles are present.
	expected := map[string]bool{"browseruse": false, "computeruse": false, "dataanalysis": false,
		"dockerservices": false, "gitops": false, "webresearch": false, "pdftools": false,
		"imagetools": false, "sqldb": false, "archivetools": false, "httpapi": false,
		"emailtools": false, "sshremote": false, "cryptotools": false, "calendartools": false,
		"officedocs": false}
	for _, n := range builtinBundles {
		if _, ok := expected[n]; ok {
			expected[n] = true
		} else {
			t.Errorf("unexpected bundle name: %q", n)
		}
	}
}

func TestBundle_NonexistentReturnsError(t *testing.T) {
	md, res, err := Bundle("nonexistent-bundle")
	if err == nil {
		t.Fatal("Bundle with nonexistent name should return error")
	}
	if md != nil {
		t.Error("md should be nil on error")
	}
	if len(res) != 0 {
		t.Error("res should be empty on error")
	}
}

func TestBundle_ExistingBundleReturnsContent(t *testing.T) {
	md, res, err := Bundle("browseruse")
	if err != nil {
		t.Fatalf("Bundle(browseruse): %v", err)
	}
	if len(md) == 0 {
		t.Error("browseruse SKILL.md should not be empty")
	}
	// Should have resource files like scripts/browse.mjs.
	if _, ok := res["scripts/browse.mjs"]; !ok {
		t.Errorf("expected scripts/browse.mjs in resources, got %v", keys(res))
	}
}

func keys(m map[string][]byte) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	return k
}

func TestBundle_AllBundlesHaveSKILLMD(t *testing.T) {
	for _, name := range builtinBundles {
		md, res, err := Bundle(name)
		if err != nil {
			t.Fatalf("Bundle(%q): %v", name, err)
		}
		if len(md) == 0 {
			t.Errorf("Bundle(%q) has empty SKILL.md", name)
		}
		// If the bundle has scripts/ and reference/ dirs, they should be in resources.
		if name == "browseruse" || name == "computeruse" || name == "dataanalysis" {
			if len(res) == 0 {
				t.Errorf("Bundle(%q) expected resources, got none", name)
			}
		}
	}
}
