// SPDX-License-Identifier: MIT

package market

import (
	"testing"

	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/skill"
)

const sampleSkillMD = `---
name: web-research
description: research a topic on the web and cite sources
triggers: [research, web, cite]
tools_required: [fetch, http]
---
You are a web researcher. Dig deep and cite sources.
`

func samplePack() Pack {
	return Pack{
		Name:             "web-research-pack",
		Version:          "1.0.0",
		Description:      "research the web",
		Category:         "research",
		Tags:             []string{"web", "research"},
		Skills:           []PackSkill{{SkillMD: sampleSkillMD, Resources: map[string][]byte{"reference/tips.md": []byte("cite everything")}}},
		MCPServers:       []mcp.Server{{Name: "fetch", Command: "npx", Args: []string{"-y", "fetch-mcp"}}},
		ToolRequirements: []string{"rg", "fd"},
	}
}

// fakeLib is a one-pack Library.
type fakeLib struct{ p Pack }

func (l fakeLib) Marketplaces() []Marketplace {
	return []Marketplace{{Name: "official", Builtin: true, FormatVersion: FormatVersion, Packs: []MarketplaceEntry{l.p.Entry("")}}}
}
func (l fakeLib) ResolvePack(_, name, _ string) (Pack, error) {
	if name == l.p.Name {
		return l.p, nil
	}
	return Pack{}, errNotFound
}

var errNotFound = &notFound{}

type notFound struct{}

func (*notFound) Error() string { return "not found" }

// fakeForge records skill creation/promotion and implements the quarantiner.
type fakeForge struct {
	created    []string
	promoted   []string
	quarantine []string
}

func (f *fakeForge) Create(_ string, spec skill.CreateSpec) (skill.Skill, bool, error) {
	f.created = append(f.created, spec.Name)
	return skill.Skill{ID: "id-" + spec.Name, Name: spec.Name, Status: skill.StatusDraft}, true, nil
}
func (f *fakeForge) Promote(_, id string) (skill.Status, error) {
	f.promoted = append(f.promoted, id)
	return skill.StatusActive, nil
}
func (f *fakeForge) Quarantine(_, id, _ string) error {
	f.quarantine = append(f.quarantine, id)
	return nil
}

// fakeMCP records server adds/removes.
type fakeMCP struct {
	added   []string
	removed []string
}

func (m *fakeMCP) AddMCPServer(_ string, srv mcp.Server) (mcp.Server, error) {
	m.added = append(m.added, srv.Name)
	return srv, nil
}
func (m *fakeMCP) RemoveMCPServer(_, ref string) (bool, error) {
	m.removed = append(m.removed, ref)
	return true, nil
}

func newTestManager(t *testing.T) (*Manager, *fakeForge, *fakeMCP) {
	t.Helper()
	forge := &fakeForge{}
	mc := &fakeMCP{}
	clock := int64(1000)
	return NewManager(Config{
		Library: fakeLib{p: samplePack()},
		Store:   NewStore(t.TempDir()),
		Skills:  forge,
		MCP:     mc,
		Now:     func() int64 { return clock },
	}), forge, mc
}

func TestInstall_MaterializesSkillsMCPAndTools(t *testing.T) {
	m, forge, mc := newTestManager(t)
	var events []Event
	rec, err := m.Install("corr-1", "official", "web-research-pack", "", func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	// Skill created + promoted to active.
	if len(forge.created) != 1 || forge.created[0] != "web-research" {
		t.Fatalf("created = %v, want [web-research]", forge.created)
	}
	if len(forge.promoted) != 1 {
		t.Fatalf("promoted = %v, want one promote", forge.promoted)
	}
	// MCP server registered.
	if len(mc.added) != 1 || mc.added[0] != "fetch" {
		t.Fatalf("mcp added = %v, want [fetch]", mc.added)
	}
	// Provenance recorded.
	if len(rec.SkillIDs) != 1 || len(rec.MCPServers) != 1 || len(rec.ToolReqs) != 2 {
		t.Fatalf("record = %+v", rec)
	}
	if !rec.Unsigned {
		t.Errorf("pack had no signature and no verifier → should be flagged unsigned")
	}
	// A done event was emitted.
	if len(events) == 0 || events[len(events)-1].Stage != "done" {
		t.Fatalf("events = %+v, want a final done", events)
	}
}

func TestList_ReflectsInstallState(t *testing.T) {
	m, _, _ := newTestManager(t)
	before, _ := m.List("")
	if len(before) != 1 || before[0].Installed {
		t.Fatalf("before = %+v, want one not-installed listing", before)
	}
	if _, err := m.Install("c", "official", "web-research-pack", "", nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	after, _ := m.List("")
	if len(after) != 1 || !after[0].Installed {
		t.Fatalf("after = %+v, want installed", after)
	}
	// Search filters.
	if got, _ := m.List("research"); len(got) != 1 {
		t.Fatalf("search research = %d, want 1", len(got))
	}
	if got, _ := m.List("zzz-nomatch"); len(got) != 0 {
		t.Fatalf("search nomatch = %d, want 0", len(got))
	}
}

func TestUninstall_ReversesFootprint(t *testing.T) {
	m, forge, mc := newTestManager(t)
	if _, err := m.Install("c", "official", "web-research-pack", "", nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := m.Uninstall("c", "web-research-pack", nil); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(forge.quarantine) != 1 {
		t.Errorf("quarantine = %v, want the installed skill", forge.quarantine)
	}
	if len(mc.removed) != 1 || mc.removed[0] != "fetch" {
		t.Errorf("removed = %v, want [fetch]", mc.removed)
	}
	if _, ok, _ := m.store.InstalledByName("web-research-pack"); ok {
		t.Error("install record should be gone after uninstall")
	}
}

func TestValidate_RejectsBadPacks(t *testing.T) {
	bad := []Pack{
		{Name: "Bad Name", Version: "1.0.0", Skills: []PackSkill{{SkillMD: sampleSkillMD}}},
		{Name: "ok", Version: "v1", Skills: []PackSkill{{SkillMD: sampleSkillMD}}},
		{Name: "empty", Version: "1.0.0"},
		{Name: "traversal", Version: "1.0.0", Skills: []PackSkill{{SkillMD: sampleSkillMD, Resources: map[string][]byte{"../escape": {}}}}},
	}
	for i, p := range bad {
		if err := p.Validate(); err == nil {
			t.Errorf("bad pack #%d (%s) passed validation", i, p.Name)
		}
	}
	if err := samplePack().Validate(); err != nil {
		t.Errorf("good pack rejected: %v", err)
	}
}

func TestVerify_FailedSignatureRefusesInstall(t *testing.T) {
	m, _, _ := newTestManager(t)
	m.verify = func(Pack) (bool, error) { return false, errNotFound } // simulate a present-but-bad sig
	if _, err := m.Install("c", "official", "web-research-pack", "", nil); err == nil {
		t.Fatal("install should refuse a pack whose signature fails verification")
	}
}

// --- Pure function tests ---

func TestSkillSummary(t *testing.T) {
	// Valid SKILL.md with description.
	summary, err := SkillSummary(PackSkill{SkillMD: sampleSkillMD})
	if err != nil {
		t.Fatalf("SkillSummary: %v", err)
	}
	if summary != "web-research — research a topic on the web and cite sources" {
		t.Errorf("SkillSummary = %q", summary)
	}
	// Only name (no description), must have a body.
	noDesc := `---
name: bare
---
Do the thing.
`
	summary, err = SkillSummary(PackSkill{SkillMD: noDesc})
	if err != nil {
		t.Fatalf("SkillSummary(no desc): %v", err)
	}
	if summary != "bare" {
		t.Errorf("SkillSummary(no desc) = %q, want %q", summary, "bare")
	}
	// Malformed SKILL.md.
	_, err = SkillSummary(PackSkill{SkillMD: "no front matter"})
	if err == nil {
		t.Error("SkillSummary(bad md) should error")
	}
}

func TestSafeRelPath(t *testing.T) {
	// Valid paths.
	if err := safeRelPath("a/b/c.txt"); err != nil {
		t.Errorf("safeRelPath valid: %v", err)
	}
	// Empty.
	if err := safeRelPath(""); err == nil {
		t.Error("safeRelPath('') should error")
	}
	// Absolute.
	if err := safeRelPath("/etc/passwd"); err == nil {
		t.Error("safeRelPath absolute should error")
	}
	// Backslash.
	if err := safeRelPath("a\\b"); err == nil {
		t.Error("safeRelPath backslash should error")
	}
	// Traversal.
	if err := safeRelPath("../escape"); err == nil {
		t.Error("safeRelPath traversal should error")
	}
	if err := safeRelPath("a/./b"); err == nil {
		t.Error("safeRelPath dot segment should error")
	}
	if err := safeRelPath("a//b"); err == nil {
		t.Error("safeRelPath double-slash should error")
	}
}

func TestSortEntries(t *testing.T) {
	entries := []MarketplaceEntry{
		{Name: "zeta", Category: "tools"},
		{Name: "alpha", Category: "research"},
		{Name: "beta", Category: "research"},
	}
	sortEntries(entries)
	// Category order: research then tools.
	if entries[0].Category != "research" || entries[1].Category != "research" || entries[2].Category != "tools" {
		t.Errorf("sort order by category wrong: %+v", entries)
	}
	// Within category: alpha before beta.
	if entries[0].Name != "alpha" || entries[1].Name != "beta" {
		t.Errorf("sort order within research wrong: %+v", entries[0:2])
	}
}

func TestPackCounts(t *testing.T) {
	p := samplePack()
	skills, mcps, tools := p.Counts()
	if skills != 1 || mcps != 1 || tools != 2 {
		t.Errorf("Counts = %d/%d/%d, want 1/1/2", skills, mcps, tools)
	}
}

func TestMatchesQuery(t *testing.T) {
	e := MarketplaceEntry{Name: "web-research", Description: "research the web", Category: "research", Tags: []string{"web"}}
	if !matchesQuery(e, "") {
		t.Error("matchesQuery('') should return true")
	}
	if !matchesQuery(e, "web") {
		t.Error("matchesQuery('web') should match name")
	}
	if !matchesQuery(e, "research") {
		t.Error("matchesQuery('research') should match category")
	}
	if matchesQuery(e, "nope") {
		t.Error("matchesQuery('nope') should not match")
	}
	// Multiple tokens: all must match.
	if !matchesQuery(e, "web research") {
		t.Error("matchesQuery('web research') should match both tokens")
	}
	if matchesQuery(e, "web nope") {
		t.Error("matchesQuery('web nope') should not match because nope doesn't")
	}
}

func TestPackEntry(t *testing.T) {
	p := samplePack()
	e := p.Entry("official")
	if e.Name != "web-research-pack" || e.Source != "official" {
		t.Errorf("Entry = %+v", e)
	}
	if e.SHA256 == "" {
		t.Error("Entry should have a content hash")
	}
	if e.Signed {
		t.Error("Entry should report Signed=false for unsigned pack")
	}
}
