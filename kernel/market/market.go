// SPDX-License-Identifier: MIT

// Package market is AGEZT's capability marketplace: it packages skills, MCP
// servers, and CLI-tool requirements into installable "packs", catalogues them
// in "marketplaces" (a built-in Official one plus, later, synced remotes), and
// installs a pack by materializing its parts into the systems that already run
// them — skills into the Forge, MCP servers into the MCP registry, tool needs
// reported to the Toolbox. It deliberately reuses those subsystems rather than
// reimplementing capability execution; the marketplace is only discovery +
// packaging + install/sync on top.
//
// Manifest shapes mirror the Claude Code plugin.json / marketplace.json open
// standard (and agentskills.io bundles) so packs are portable across tools.
package market

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/skill"
)

// FormatVersion is the on-disk/wire manifest version. Bump on breaking changes.
const FormatVersion = 1

// MarketplaceOfficial is the reserved name of the built-in, offline catalogue.
// Remote sources may not use it, so a remote can never shadow the seed.
const MarketplaceOfficial = "official"

// nameRe validates pack and marketplace names: kebab-case, lowercase, so they're
// safe as path segments and stable identifiers.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// semverRe is a pragmatic semver check (major.minor.patch with optional
// -prerelease). Versions order installs and gate updates.
var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

// PackSkill is one skill carried by a pack: the agentskills.io shape — a SKILL.md
// body plus optional resource files (reference docs, scripts) keyed by relative
// path. Materialized via skill.Forge.Create, so the runtime treats it exactly
// like any other skill (retrieval, injection, lifecycle).
type PackSkill struct {
	SkillMD   string            `json:"skill_md"`
	Resources map[string][]byte `json:"resources,omitempty"`
}

// Signature is an optional Ed25519 attestation over the pack's canonical bytes
// (UPD-001 primitive). Unsigned packs are allowed but flagged on install
// (default-allow posture); verification is opt-OUT, not a hard wall.
type Signature struct {
	SHA256   string `json:"sha256"`
	Sig      string `json:"sig"`       // hex Ed25519 signature over the canonical pack bytes
	PubKey   string `json:"pubkey"`    // hex Ed25519 public key
	SignedAt int64  `json:"signed_at"` // unix ms
}

// Pack is one installable artifact: skills + MCP servers + CLI-tool needs.
type Pack struct {
	Name             string       `json:"name"`
	Version          string       `json:"version"`
	Description      string       `json:"description,omitempty"`
	Author           string       `json:"author,omitempty"`
	Category         string       `json:"category,omitempty"`
	Tags             []string     `json:"tags,omitempty"`
	Keywords         []string     `json:"keywords,omitempty"`
	Skills           []PackSkill  `json:"skills,omitempty"`
	MCPServers       []mcp.Server `json:"mcp_servers,omitempty"`
	ToolRequirements []string     `json:"tool_requirements,omitempty"`
	Signature        *Signature   `json:"signature,omitempty"`
}

// MarketplaceEntry is one pack's catalogue row in a marketplace index — metadata
// for browse/search, plus where to fetch the full pack from (a relative path
// within the marketplace, or empty for a built-in pack carried in-process).
type MarketplaceEntry struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Source      string   `json:"source,omitempty"` // relative pack path within the marketplace
	SHA256      string   `json:"sha256,omitempty"`
	Signed      bool     `json:"signed,omitempty"`
	// Featured marks curated/editor's-pick packs a marketplace wants surfaced
	// first (the built-in catalogue stars its flagship combos; remotes may set it
	// in their index). Downloads is a registry-supplied popularity signal —
	// zero/absent when a registry doesn't track installs.
	Featured  bool  `json:"featured,omitempty"`
	Downloads int64 `json:"downloads,omitempty"`
	// Content counts for at-a-glance gallery cards (no per-pack fetch needed).
	SkillCount int `json:"skill_count"`
	MCPCount   int `json:"mcp_count"`
	ToolCount  int `json:"tool_count"`
}

// Marketplace is a catalogue of packs. The built-in Official marketplace is
// always present and offline; remote ones are fetched + cached by the Syncer
// (Phase 2). Builtin marks the in-binary seed (which can't be removed/updated).
type Marketplace struct {
	Name            string             `json:"name"`
	Owner           string             `json:"owner,omitempty"`
	FormatVersion   int                `json:"format_version"`
	GeneratedUnixMS int64              `json:"generated_unix_ms,omitempty"`
	Source          string             `json:"source,omitempty"` // remote URL (empty for built-in)
	Builtin         bool               `json:"builtin,omitempty"`
	Packs           []MarketplaceEntry `json:"packs"`
}

// InstalledPack records what an install materialized, so uninstall can reverse
// exactly its own footprint (and never touch shared/other-owned resources).
type InstalledPack struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Marketplace string   `json:"marketplace"`
	InstalledMS int64    `json:"installed_ms"`
	SkillIDs    []string `json:"skill_ids,omitempty"`
	MCPServers  []string `json:"mcp_servers,omitempty"` // server names this pack added
	ToolReqs    []string `json:"tool_reqs,omitempty"`
	Unsigned    bool     `json:"unsigned,omitempty"`
	// VetVerdict records the security review's verdict at install time
	// (clean|caution|danger) — provenance for "what did I let in, knowing what?".
	VetVerdict string `json:"vet_verdict,omitempty"`
}

// Counts summarizes a pack's contents for at-a-glance UI ("3 skills · 1 MCP · 2 tools").
func (p Pack) Counts() (skills, mcps, tools int) {
	return len(p.Skills), len(p.MCPServers), len(p.ToolRequirements)
}

// SkillSummary parses a pack skill's SKILL.md and returns a one-line
// "name — description" for display (UI / the agent-facing market tool). Errors
// if the SKILL.md is malformed.
func SkillSummary(ps PackSkill) (string, error) {
	md, err := skill.ParseSkillMD([]byte(ps.SkillMD))
	if err != nil {
		return "", err
	}
	if md.Description == "" {
		return md.Name, nil
	}
	return md.Name + " — " + md.Description, nil
}

// Validate checks a pack's user-supplied fields before install/publish. It
// reuses mcp.Validate for each server and parses every SKILL.md so a malformed
// pack fails loudly rather than half-installing.
func (p Pack) Validate() error {
	if !nameRe.MatchString(p.Name) {
		return fmt.Errorf("market: pack name must match %s", nameRe)
	}
	if !semverRe.MatchString(p.Version) {
		return fmt.Errorf("market: pack %q version %q must be semver (major.minor.patch)", p.Name, p.Version)
	}
	if len(p.Skills) == 0 && len(p.MCPServers) == 0 && len(p.ToolRequirements) == 0 {
		return fmt.Errorf("market: pack %q is empty (needs at least one skill, MCP server, or tool)", p.Name)
	}
	for i, ps := range p.Skills {
		if strings.TrimSpace(ps.SkillMD) == "" {
			return fmt.Errorf("market: pack %q skill #%d has empty SKILL.md", p.Name, i)
		}
		if _, err := skill.ParseSkillMD([]byte(ps.SkillMD)); err != nil {
			return fmt.Errorf("market: pack %q skill #%d: %w", p.Name, i, err)
		}
		for rel := range ps.Resources {
			if err := safeRelPath(rel); err != nil {
				return fmt.Errorf("market: pack %q skill #%d resource %q: %w", p.Name, i, rel, err)
			}
		}
	}
	for i := range p.MCPServers {
		if err := mcp.Validate(p.MCPServers[i]); err != nil {
			return fmt.Errorf("market: pack %q mcp #%d: %w", p.Name, i, err)
		}
	}
	return nil
}

// safeRelPath rejects path traversal and absolute paths in bundled resource
// keys — the same guard the remote skill registry applies to fetched files.
func safeRelPath(rel string) error {
	if rel == "" {
		return fmt.Errorf("empty path")
	}
	if strings.ContainsAny(rel, "\\") || strings.HasPrefix(rel, "/") {
		return fmt.Errorf("must be a forward-slash relative path")
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return fmt.Errorf("path traversal not allowed")
		}
	}
	return nil
}

// CanonicalBytes returns the deterministic JSON encoding of a pack used for
// content-hashing and signing. The Signature field is excluded so the hash is
// over the payload the signature attests to.
func (p Pack) CanonicalBytes() ([]byte, error) {
	c := p
	c.Signature = nil
	// json.Marshal sorts map keys; slices keep author order. Stable enough for a
	// content hash because packs are built deterministically.
	return json.Marshal(c)
}

// ContentHash is the hex SHA-256 of the canonical pack bytes.
func (p Pack) ContentHash() (string, error) {
	b, err := p.CanonicalBytes()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Entry builds the catalogue row for this pack (metadata + content hash).
func (p Pack) Entry(source string) MarketplaceEntry {
	hash, _ := p.ContentHash()
	skills, mcps, tools := p.Counts()
	return MarketplaceEntry{
		Name:        p.Name,
		Version:     p.Version,
		Description: p.Description,
		Category:    p.Category,
		Tags:        append([]string(nil), p.Tags...),
		Source:      source,
		SHA256:      hash,
		Signed:      p.Signature != nil,
		SkillCount:  skills,
		MCPCount:    mcps,
		ToolCount:   tools,
	}
}

// matchesQuery reports whether a free-text query hits a pack's searchable fields.
func matchesQuery(e MarketplaceEntry, q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	hay := strings.ToLower(e.Name + " " + e.Description + " " + e.Category + " " + strings.Join(e.Tags, " "))
	for _, tok := range strings.Fields(q) {
		if !strings.Contains(hay, tok) {
			return false
		}
	}
	return true
}

// sortEntries orders catalogue rows deterministically: category, then name.
func sortEntries(entries []MarketplaceEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		return entries[i].Name < entries[j].Name
	})
}
