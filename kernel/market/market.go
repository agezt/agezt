// SPDX-License-Identifier: MIT
//
// kernel/market public types (PackSkill, Signature, Pack, MarketplaceEntry,
// Marketplace, InstalledPack) + consts/var declarations (FormatVersion,
// MarketplaceOfficial, nameRe, semverRe) + top-level functions (SkillSummary,
// safeRelPath).
// Extracted from market.go during Day 211 god-file refactor (#87).
// Public API unchanged.
package market

import (
	"fmt"
	"regexp"
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
