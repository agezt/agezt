// SPDX-License-Identifier: MIT

package market

// Provenance: the marketplace's ROWS — the pack, the skill it carries, the
//             catalogue entry and the catalogue itself — now live in the
//             contract layer (kernel/contract/marketapi): the built-in
//             "Official" catalogue is a PLUGIN, and a plugin may reach the
//             contract and the platform layers and nothing else. The rules
//             travel with the rows (what makes a pack valid, its canonical
//             bytes, its content hash, how versions are ordered), because a
//             method can only be defined in its type's package.
//
//             The names below stay as aliases and one-line delegations, so
//             every existing caller is unchanged and there is one row and one
//             spelling of it.

import (
	"regexp"

	"github.com/agezt/agezt/kernel/contract/marketapi"
)

// FormatVersion is the on-disk/wire manifest version. Bump on breaking changes.
const FormatVersion = marketapi.FormatVersion

// MarketplaceOfficial is the reserved name of the built-in, offline catalogue.
// Remote sources may not use it, so a remote can never shadow the seed.
const MarketplaceOfficial = marketapi.MarketplaceOfficial

// nameRe validates pack and marketplace names. The PATTERN is the contract's;
// the compiled form is this package's own.
var nameRe = regexp.MustCompile(marketapi.NamePattern)

// semverRe validates versions. The PATTERN is the contract's; the compiled
// form is this package's own.
var semverRe = regexp.MustCompile(marketapi.SemverPattern)

// PackSkill is one skill carried by a pack: the agentskills.io shape.
//
// The row itself lives in the contract layer (kernel/contract/marketapi); the
// name stays here as an alias, so every existing caller is unchanged.
type PackSkill = marketapi.PackSkill

// Signature is an optional Ed25519 attestation over the pack's canonical bytes.
//
// The row itself lives in the contract layer (kernel/contract/marketapi); the
// name stays here as an alias, so every existing caller is unchanged.
type Signature = marketapi.Signature

// Pack is one installable artifact: skills + MCP servers + CLI-tool needs.
//
// The row itself lives in the contract layer (kernel/contract/marketapi) —
// with its methods, because a method can only be defined in its type's
// package. The name stays here as an alias, so every existing caller is
// unchanged.
type Pack = marketapi.Pack

// MarketplaceEntry is one pack's catalogue row in a marketplace index.
//
// The row itself lives in the contract layer (kernel/contract/marketapi); the
// name stays here as an alias, so every existing caller is unchanged.
type MarketplaceEntry = marketapi.MarketplaceEntry

// Marketplace is a catalogue of packs.
//
// The row itself lives in the contract layer (kernel/contract/marketapi); the
// name stays here as an alias, so every existing caller is unchanged.
type Marketplace = marketapi.Marketplace

// InstalledPack records what an install materialized, so uninstall can reverse
// exactly its own footprint (and never touch shared/other-owned resources).
// It stays here: it is the manager's own bookkeeping, persisted in
// installed.json — a plugin never names it.
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

// SkillSummary summarizes a pack's skill for an at-a-glance UI.
//
// The rule lives in the contract layer (kernel/contract/marketapi); this is a
// delegation, so there is one summary.
func SkillSummary(ps PackSkill) (string, error) { return marketapi.SkillSummary(ps) }

// safeRelPath rejects a resource path that would escape the bundle's
// directory.
//
// The rule lives in the contract layer (kernel/contract/marketapi); this is a
// delegation, so there is one rule.
func safeRelPath(rel string) error { return marketapi.SafeRelPath(rel) }
