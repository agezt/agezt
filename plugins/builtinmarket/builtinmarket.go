// SPDX-License-Identifier: MIT

// Package builtinmarket is the marketplace's built-in "Official" catalogue: it
// wraps the binary's embedded skill bundles (plugins/builtinskills) — and a few
// curated combo packs that add an MCP server + CLI-tool requirements — into
// installable market.Packs. It is always present and works offline; remote
// marketplaces (Phase 2) layer on top. It references the embedded bundle bytes
// via builtinskills.Bundle rather than duplicating them.
package builtinmarket

import (
	"fmt"

	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/plugins/builtinskills"
)

// MarketplaceName is the built-in catalogue's stable name.
const MarketplaceName = "official"

// skillCategory maps a built-in bundle to a marketplace category for browse/filter.
var skillCategory = map[string]string{
	"browseruse":     "web",
	"webresearch":    "web",
	"httpapi":        "web",
	"computeruse":    "desktop",
	"dataanalysis":   "data",
	"sqldb":          "data",
	"dockerservices": "dev",
	"gitops":         "dev",
	"sshremote":      "dev",
	"pdftools":       "docs",
	"officedocs":     "docs",
	"imagetools":     "media",
	"archivetools":   "files",
	"emailtools":     "comms",
	"calendartools":  "comms",
	"cryptotools":    "security",
}

// combo describes a curated pack that bundles a built-in skill together with an
// MCP server and/or CLI-tool requirements — showcasing the "fully-equipped"
// (skill + MCP + tools) pack the owner asked for.
type combo struct {
	name     string
	skill    string // built-in bundle name to include
	category string
	desc     string
	tags     []string
	mcp      []mcp.Server
	tools    []string
}

var combos = []combo{
	{
		name: "web-research-pro", skill: "webresearch", category: "web",
		desc:  "Cited web research with a fetch MCP server and fast local search tools.",
		tags:  []string{"web", "research", "mcp"},
		mcp:   []mcp.Server{{Name: "fetch", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-fetch"}, Description: "HTTP fetch for the agent", Lazy: true}},
		tools: []string{"rg", "fd"},
	},
	{
		name: "git-workshop", skill: "gitops", category: "dev",
		desc:  "Git operations skill plus the CLI tools a coding agent needs.",
		tags:  []string{"git", "dev", "tools"},
		tools: []string{"git", "rg", "fd", "delta"},
	},
}

// Library is the built-in Official marketplace, implementing market.Library.
type Library struct {
	packs []market.Pack
	index map[string]market.Pack
}

// New builds the Official library from the embedded skill bundles + combos.
// Best-effort per bundle: a bundle that fails to read is skipped, not fatal.
func New() *Library {
	l := &Library{index: map[string]market.Pack{}}
	for _, name := range builtinskills.Names() {
		md, res, err := builtinskills.Bundle(name)
		if err != nil {
			continue
		}
		parsed, perr := skill.ParseSkillMD(md)
		if perr != nil {
			continue
		}
		l.add(market.Pack{
			Name:        name + "-pack",
			Version:     "1.0.0",
			Description: parsed.Description,
			Author:      "AGEZT",
			Category:    categoryFor(name),
			Tags:        append([]string{name}, parsed.Triggers...),
			Skills:      []market.PackSkill{{SkillMD: string(md), Resources: res}},
		})
	}
	for _, c := range combos {
		md, res, err := builtinskills.Bundle(c.skill)
		if err != nil {
			continue
		}
		l.add(market.Pack{
			Name:             c.name,
			Version:          "1.0.0",
			Description:      c.desc,
			Author:           "AGEZT",
			Category:         c.category,
			Tags:             c.tags,
			Skills:           []market.PackSkill{{SkillMD: string(md), Resources: res}},
			MCPServers:       c.mcp,
			ToolRequirements: c.tools,
		})
	}
	return l
}

func (l *Library) add(p market.Pack) {
	l.packs = append(l.packs, p)
	l.index[p.Name] = p
}

func categoryFor(bundle string) string {
	if c, ok := skillCategory[bundle]; ok {
		return c
	}
	return "skills"
}

// Marketplaces implements market.Library: a single built-in Official catalogue.
func (l *Library) Marketplaces() []market.Marketplace {
	entries := make([]market.MarketplaceEntry, 0, len(l.packs))
	for _, p := range l.packs {
		entries = append(entries, p.Entry(""))
	}
	return []market.Marketplace{{
		Name:          MarketplaceName,
		Owner:         "AGEZT",
		FormatVersion: market.FormatVersion,
		Builtin:       true,
		Packs:         entries,
	}}
}

// ResolvePack implements market.Library.
func (l *Library) ResolvePack(_, name, _ string) (market.Pack, error) {
	if p, ok := l.index[name]; ok {
		return p, nil
	}
	return market.Pack{}, fmt.Errorf("builtinmarket: pack %q not found in the Official marketplace", name)
}
