// SPDX-License-Identifier: MIT

// Package marketapi is the contract layer's home for the marketplace's rows:
// one installable pack, the skills and MCP servers it carries, the catalogue
// entry a marketplace lists it under, and the catalogue itself — together
// with the rules that govern them (what makes a pack valid, what its
// canonical bytes and content hash are, how versions are ordered) and the
// port through which a catalogue is read.
//
// The rows live here because the built-in "Official" catalogue is a PLUGIN,
// and a plugin may reach the contract and the platform layers and nothing
// else. The implementation that keeps them — the manager, the store, the
// syncer, the composite library — stays in the module (kernel/market); the
// module keeps the names its own callers use as aliases, so there is one row
// and one spelling of it.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No store, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package marketapi

import (
	"errors"
	"regexp"
	"strings"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
)

// FormatVersion is the on-disk/wire manifest version. Bump on breaking changes.
const FormatVersion = 1

// MarketplaceOfficial is the reserved name of the built-in, offline catalogue.
// Remote sources may not use it, so a remote can never shadow the seed.
const MarketplaceOfficial = "official"

// NamePattern is the pattern a pack or marketplace name must match:
// kebab-case, lowercase, so it is safe as a path segment and a stable
// identifier.
const NamePattern = `^[a-z][a-z0-9-]{0,63}$`

// SemverPattern is a pragmatic semver check (major.minor.patch with optional
// -prerelease). Versions order installs and gate updates.
const SemverPattern = `^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`

// nameRe validates pack and marketplace names.
var nameRe = regexp.MustCompile(NamePattern)

// semverRe validates versions.
var semverRe = regexp.MustCompile(SemverPattern)

// PackSkill is one skill carried by a pack: the agentskills.io shape — a SKILL.md
// body plus optional resource files (reference docs, scripts) keyed by relative
// path. Materialized via the skill Forge, so the runtime treats it exactly
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
	Name             string          `json:"name"`
	Version          string          `json:"version"`
	Description      string          `json:"description,omitempty"`
	Author           string          `json:"author,omitempty"`
	Category         string          `json:"category,omitempty"`
	Tags             []string        `json:"tags,omitempty"`
	Keywords         []string        `json:"keywords,omitempty"`
	Skills           []PackSkill     `json:"skills,omitempty"`
	MCPServers       []mcpapi.Server `json:"mcp_servers,omitempty"`
	ToolRequirements []string        `json:"tool_requirements,omitempty"`
	Signature        *Signature      `json:"signature,omitempty"`
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

// SkillSummary summarizes a pack's skill for an at-a-glance UI ("3 skills · 1 MCP
// · 2 tools"): the skill's name and description, as the SKILL.md carries them.
func SkillSummary(ps PackSkill) (string, error) {
	md, err := skillapi.ParseSkillMD([]byte(ps.SkillMD))
	if err != nil {
		return "", err
	}
	if md.Description == "" {
		return md.Name, nil
	}
	return md.Name + " — " + md.Description, nil
}

// The reasons a resource path is refused. They are named so the error text is
// the same wherever the rule is applied.
var (
	errEmptyPath   = errors.New("empty path")
	errNotRelative = errors.New("must be a forward-slash relative path")
	errTraversal   = errors.New("path traversal not allowed")
)

// SafeRelPath rejects a resource path that would escape the bundle's
// directory: it must be a forward-slash relative path with no traversal.
func SafeRelPath(rel string) error {
	if rel == "" {
		return errEmptyPath
	}
	if strings.ContainsAny(rel, "\\") || strings.HasPrefix(rel, "/") {
		return errNotRelative
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return errTraversal
		}
	}
	return nil
}
