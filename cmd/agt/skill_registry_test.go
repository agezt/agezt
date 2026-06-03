// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/skill"
)

func writeBundleFile(t *testing.T, dir, file, name, body string, id string) {
	t.Helper()
	b := skillBundle{
		Tool: "agt", FormatVersion: 1,
		Skill: skillBundleBody{ID: id, Name: name, Body: body, Version: "0.1.0", Description: "d"},
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// scanSkillRegistry classifies each *.skill.json: a valid bundle verifies, a
// body that no longer matches its id is flagged not-verified, and a non-bundle
// file carries an error — sorted by name (M270).
func TestScanSkillRegistry_Classifies(t *testing.T) {
	dir := t.TempDir()
	writeBundleFile(t, dir, "good.skill.json", "diagnose-ci", "do it", skill.ContentID("diagnose-ci", "do it"))
	// Tampered: id claims one body, the body field is another.
	writeBundleFile(t, dir, "bad.skill.json", "deploy", "tampered body", skill.ContentID("deploy", "original body"))
	if err := os.WriteFile(filepath.Join(dir, "junk.skill.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, err := scanSkillRegistry(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
	// Sorted by name: "deploy" < "diagnose-ci" < "" (junk has no name → empty,
	// sorts first). So order is junk(""), deploy, diagnose-ci.
	byName := map[string]registryEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if g := byName["diagnose-ci"]; !g.Verified || g.Err != "" {
		t.Errorf("good bundle = %+v, want verified, no error", g)
	}
	if g := byName["deploy"]; g.Verified || g.Err != "" {
		t.Errorf("tampered bundle = %+v, want not verified, no parse error", g)
	}
	if g := byName[""]; g.Err == "" {
		t.Errorf("junk file = %+v, want an error", g)
	}
}

// The index.json manifest `export --all` writes must not be mistaken for a
// bundle by the directory scan (it is not a *.skill.json) — M273.
func TestScanSkillRegistry_IgnoresIndex(t *testing.T) {
	dir := t.TempDir()
	writeBundleFile(t, dir, "good.skill.json", "diagnose-ci", "do it", skill.ContentID("diagnose-ci", "do it"))
	idx := registryIndex{Tool: "agt", FormatVersion: 1, Skills: []indexSkill{{Name: "diagnose-ci", File: "good.skill.json"}}}
	data, _ := json.MarshalIndent(idx, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, registryIndexName), data, 0o600); err != nil {
		t.Fatal(err)
	}

	entries, err := scanSkillRegistry(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "diagnose-ci" {
		t.Errorf("entries = %+v, want only the bundle (index.json ignored)", entries)
	}
}

// The registry index round-trips so the (future) remote consumer can rely on
// its shape.
func TestRegistryIndex_RoundTrips(t *testing.T) {
	in := registryIndex{
		Tool: "agt", FormatVersion: 1, GeneratedUnixMS: 123,
		Skills: []indexSkill{{Name: "n", Version: "0.1.0", ID: "abc", Description: "d", File: "n-abc.skill.json"}},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out registryIndex
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Skills) != 1 || out.Skills[0].File != "n-abc.skill.json" || out.Skills[0].ID != "abc" {
		t.Errorf("round-trip = %+v, want the entry preserved", out)
	}
}

func TestScanSkillRegistry_EmptyDir(t *testing.T) {
	entries, err := scanSkillRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0 for an empty dir", len(entries))
	}
}

// installFromRegistry resolves a name without dialing for the failure cases:
// an absent name, an only-tampered name, and an ambiguous name (several verified
// bundles) all error rather than installing the wrong thing (M271).
func TestInstallFromRegistry_ResolutionErrors(t *testing.T) {
	entries := []registryEntry{
		{Name: "alpha", Version: "0.1.0", ID: "a1", Path: "a1.skill.json", Verified: true},
		{Name: "alpha", Version: "0.2.0", ID: "a2", Path: "a2.skill.json", Verified: true},
		{Name: "beta", Path: "beta.skill.json", Err: "not a skill bundle"},
		{Name: "gamma", ID: "g1", Path: "gamma.skill.json", Verified: false}, // tampered
	}

	cases := []struct {
		name string
		want string // substring expected on stderr
	}{
		{"alpha", "ambiguous"},
		{"gamma", "malformed/tampered"},
		{"delta", "no verified bundle"},
		{"beta", "malformed/tampered"}, // present but only as a parse error
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		if code := installFromRegistry(entries, c.name, &out, &errb); code != 1 {
			t.Errorf("%s: exit = %d, want 1", c.name, code)
		}
		if !strings.Contains(errb.String(), c.want) {
			t.Errorf("%s: stderr = %q, want %q", c.name, errb.String(), c.want)
		}
	}
}

// The command exits non-zero when the registry holds a tampered bundle, and is a
// usage error with no directory.
func TestCmdSkillRegistry_FlagsTamperedAndUsage(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cmdSkillRegistry(nil, &out, &errb); code != 2 {
		t.Errorf("no dir: exit = %d, want 2", code)
	}

	dir := t.TempDir()
	writeBundleFile(t, dir, "bad.skill.json", "deploy", "tampered", skill.ContentID("deploy", "real"))
	out.Reset()
	errb.Reset()
	if code := cmdSkillRegistry([]string{dir}, &out, &errb); code != 1 {
		t.Errorf("tampered registry: exit = %d, want 1", code)
	}
}
