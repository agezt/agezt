// SPDX-License-Identifier: MIT

package marketapi

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
)

// wireKeys returns the JSON object keys of the marshalled value, sorted.
func wireKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := make([]string, 0, len(wire))
	for k := range wire {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestThePackWireShape: a pack is signed, hashed and published — the wire is
// the format a registry and a signature are written against, so the key set
// is pinned.
func TestThePackWireShape(t *testing.T) {
	got := wireKeys(t, Pack{})
	want := []string{"name", "version"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the pack wire keys = %v, want %v", got, want)
	}
}

// TestThePackAlwaysFieldsStayOnTheWire: the two fields every consumer reads
// are always present — a missing "name" would make a pack unlistable and a
// missing "version" unorderable.
func TestThePackAlwaysFieldsStayOnTheWire(t *testing.T) {
	raw, err := json.Marshal(Pack{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"name", "version"} {
		if _, ok := wire[k]; !ok {
			t.Errorf("the pack wire is missing the always-field %q: %s", k, raw)
		}
	}
	for _, k := range []string{"description", "author", "category", "tags", "keywords", "skills", "mcp_servers", "tool_requirements", "signature"} {
		if _, ok := wire[k]; ok {
			t.Errorf("an empty %q must stay off the wire: %s", k, raw)
		}
	}
}

// TestTheMarketplaceWireShape: the catalogue is what the UI lists and what a
// remote registry is written against.
func TestTheMarketplaceWireShape(t *testing.T) {
	got := wireKeys(t, Marketplace{})
	want := []string{"format_version", "name", "packs"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the marketplace wire keys = %v, want %v", got, want)
	}
}

// TestTheEntryWireShape: the catalogue entry carries the counts a gallery
// card shows, and they are always on the wire (a card with no count would
// read as "zero").
func TestTheEntryWireShape(t *testing.T) {
	got := wireKeys(t, MarketplaceEntry{})
	want := []string{"mcp_count", "name", "skill_count", "tool_count", "version"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the entry wire keys = %v, want %v", got, want)
	}
}

// TestThePackSkillWireShape: the skill a pack carries.
func TestThePackSkillWireShape(t *testing.T) {
	got := wireKeys(t, PackSkill{})
	want := []string{"skill_md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the pack skill wire keys = %v, want %v", got, want)
	}
}

// TestValidateRejectsWhatItMust: the pack's own rule — the one that decides
// what may be installed — is measured rule by rule.
func TestValidateRejectsWhatItMust(t *testing.T) {
	good := Pack{
		Name:    "web-research",
		Version: "1.0.0",
		Skills:  []PackSkill{{SkillMD: "---\nname: web-research\n---\nbody\n"}},
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("a well-formed pack was refused: %v", err)
	}

	for _, c := range []struct {
		name string
		pack Pack
	}{
		{"a name that is not kebab-case", Pack{Name: "Web Research", Version: "1.0.0", Skills: good.Skills}},
		{"a version that is not semver", Pack{Name: "web-research", Version: "1.0", Skills: good.Skills}},
		{"an empty pack", Pack{Name: "web-research", Version: "1.0.0"}},
		{"a skill with an empty SKILL.md", Pack{Name: "web-research", Version: "1.0.0", Skills: []PackSkill{{SkillMD: "  "}}}},
		{"a skill whose SKILL.md does not parse", Pack{Name: "web-research", Version: "1.0.0", Skills: []PackSkill{{SkillMD: "no frontmatter"}}}},
		{"a resource that escapes the bundle", Pack{Name: "web-research", Version: "1.0.0", Skills: []PackSkill{{
			SkillMD:   "---\nname: web-research\n---\nbody\n",
			Resources: map[string][]byte{"../escape.md": []byte("x")},
		}}}},
		{"an MCP server the contract refuses", Pack{Name: "web-research", Version: "1.0.0", MCPServers: []mcpapi.Server{{Name: "Bad Name"}}}},
	} {
		if err := c.pack.Validate(); err == nil {
			t.Errorf("%s: the pack was accepted", c.name)
		}
	}
}

// TestTheContentHashIgnoresTheSignature: the signature signs the content, so
// it cannot be part of it — and the hash must be stable across a signature
// being added.
func TestTheContentHashIgnoresTheSignature(t *testing.T) {
	p := Pack{Name: "web-research", Version: "1.0.0", Skills: []PackSkill{{SkillMD: "---\nname: web-research\n---\nbody\n"}}}
	before, err := p.ContentHash()
	if err != nil {
		t.Fatalf("ContentHash: %v", err)
	}
	p.Signature = &Signature{SHA256: "abc", Sig: "def", PubKey: "ghi"}
	after, err := p.ContentHash()
	if err != nil {
		t.Fatalf("ContentHash: %v", err)
	}
	if before != after {
		t.Fatalf("the signature changed the content hash: %s vs %s", before, after)
	}
	if !p.Entry("").Signed {
		t.Error("a signed pack must report itself as signed")
	}
}

// TestTheEntryCarriesTheCounts: the catalogue row is the pack's summary — the
// counts and the hash come from the pack itself.
func TestTheEntryCarriesTheCounts(t *testing.T) {
	p := Pack{
		Name: "web-research", Version: "1.0.0",
		Skills:           []PackSkill{{SkillMD: "---\nname: a\n---\nbody\n"}, {SkillMD: "---\nname: b\n---\nbody\n"}},
		MCPServers:       []mcpapi.Server{{Name: "fetch", Command: "uvx"}},
		ToolRequirements: []string{"shell"},
	}
	e := p.Entry("packs/web-research")
	if e.Name != "web-research" || e.Version != "1.0.0" {
		t.Fatalf("the entry lost the pack's identity: %+v", e)
	}
	if e.SkillCount != 2 || e.MCPCount != 1 || e.ToolCount != 1 {
		t.Fatalf("the entry's counts = %d/%d/%d, want 2/1/1", e.SkillCount, e.MCPCount, e.ToolCount)
	}
	if e.Source != "packs/web-research" {
		t.Errorf("the entry's source = %q", e.Source)
	}
	if e.SHA256 == "" {
		t.Error("the entry must carry the content hash")
	}
}

// TestCompareVersionsFollowsSemver: the ONE ordering rule for market versions
// — a second comparator would silently fork it.
func TestCompareVersionsFollowsSemver(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.10.0", "1.9.0", 1},
		{"1.9.0", "1.10.0", -1},
		{"2.0.0", "1.9.9", 1},
		{"2.0.0-rc.1", "2.0.0", -1},
		{"2.0.0", "2.0.0-rc.1", 1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0+b", "1.0.0", 0},
		// A version that does not parse falls back to plain string
		// comparison — "different strings", never "the same". Two
		// malformed versions that are literally equal still compare equal.
		{"not-a-version", "not-a-version", 0},
		{"not-a-version", "also-not", 1},
		{"also-not", "not-a-version", -1},
		{"", "1.0.0", -1},
	} {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestSafeRelPathRejectsEscapes: the rule that keeps a pack's resources
// inside their bundle.
func TestSafeRelPathRejectsEscapes(t *testing.T) {
	for _, ok := range []string{"reference/api.md", "scripts/run.py", "a/b/c.txt"} {
		if err := SafeRelPath(ok); err != nil {
			t.Errorf("SafeRelPath(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "/etc/passwd", `..\escape`, "../escape", "a/../b", "./x", "a//b"} {
		if err := SafeRelPath(bad); err == nil {
			t.Errorf("SafeRelPath(%q) = nil, want an error", bad)
		}
	}
}

// TestSkillSummaryReadsTheSkill: the summary comes from the SKILL.md the
// pack carries.
func TestSkillSummaryReadsTheSkill(t *testing.T) {
	got, err := SkillSummary(PackSkill{SkillMD: "---\nname: web-research\ndescription: research the web\n---\nbody\n"})
	if err != nil {
		t.Fatalf("SkillSummary: %v", err)
	}
	if got != "web-research — research the web" {
		t.Errorf("the summary = %q", got)
	}
	// A skill with no description still carries its name — the summary is
	// what a gallery card shows.
	bare, err := SkillSummary(PackSkill{SkillMD: "---\nname: web-research\n---\nbody\n"})
	if err != nil {
		t.Fatalf("SkillSummary: %v", err)
	}
	if bare != "web-research" {
		t.Errorf("the summary without a description = %q, want the name", bare)
	}
	if _, err := SkillSummary(PackSkill{SkillMD: "no frontmatter"}); err == nil {
		t.Error("an unparseable SKILL.md must fail the summary")
	}
}

// TestTheLibraryPortShape: the port is the whole surface a catalogue is read
// through.
func TestTheLibraryPortShape(t *testing.T) {
	lt := reflect.TypeOf((*Library)(nil)).Elem()
	if lt.Kind() != reflect.Interface {
		t.Fatalf("Library must be an interface, got %v", lt.Kind())
	}
	got := make([]string, 0, lt.NumMethod())
	for i := 0; i < lt.NumMethod(); i++ {
		got = append(got, lt.Method(i).Name)
	}
	sort.Strings(got)
	want := []string{"Marketplaces", "ResolvePack"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the library port methods = %v, want %v", got, want)
	}
}
