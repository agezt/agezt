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

// combo describes a curated pack that bundles one or more built-in skills
// together with MCP servers and/or CLI-tool requirements — the "fully-equipped"
// suite packs. featured marks the catalogue's editor's picks (surfaced first in
// the Market UI). MCP servers are chosen key-less where possible so an install
// works immediately; credentialed servers stay in the MCP catalogue where the
// Connect flow can collect their keys.
type combo struct {
	name     string
	skills   []string // built-in bundle names to include
	category string
	desc     string
	tags     []string
	mcp      []mcp.Server
	tools    []string
	featured bool
}

var combos = []combo{
	{
		name: "web-research-pro", skills: []string{"webresearch"}, category: "web", featured: true,
		desc:  "Cited web research with a fetch MCP server and fast local search tools.",
		tags:  []string{"web", "research", "mcp"},
		mcp:   []mcp.Server{{Name: "fetch", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-fetch"}, Description: "HTTP fetch for the agent", Lazy: true}},
		tools: []string{"rg", "fd"},
	},
	{
		name: "git-workshop", skills: []string{"gitops"}, category: "dev",
		desc:  "Git operations skill plus the CLI tools a coding agent needs.",
		tags:  []string{"git", "dev", "tools"},
		tools: []string{"git", "rg", "fd", "delta"},
	},
	{
		name: "github-automation", skills: []string{"gitops", "httpapi"}, category: "dev", featured: true,
		desc:  "GitHub end-to-end: repos, issues, PRs and CI via the gh CLI, plus Git and HTTP API skills.",
		tags:  []string{"github", "git", "ci", "dev"},
		tools: []string{"git", "gh", "jq"},
	},
	{
		name: "data-analyst-pro", skills: []string{"dataanalysis", "sqldb"}, category: "data", featured: true,
		desc:  "Data analysis and SQL together: explore files, query databases, produce charts and findings.",
		tags:  []string{"data", "sql", "analysis"},
		tools: []string{"sqlite3", "jq"},
	},
	{
		name: "second-brain", skills: []string{"webresearch"}, category: "knowledge", featured: true,
		desc:  "Research that remembers: persistent knowledge-graph memory plus a structured reasoning scratchpad.",
		tags:  []string{"memory", "knowledge", "reasoning", "mcp"},
		mcp: []mcp.Server{
			{Name: "memory", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-memory"}, Description: "Persistent knowledge-graph memory", Lazy: true},
			{Name: "thinking", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-sequential-thinking"}, Description: "Step-by-step reasoning scratchpad", Lazy: true},
		},
	},
	{
		name: "browser-automation-pro", skills: []string{"browseruse"}, category: "web", featured: true,
		desc:  "Drive a real browser: navigate, click, fill forms and screenshot via the official Playwright MCP.",
		tags:  []string{"browser", "automation", "playwright", "mcp"},
		mcp:   []mcp.Server{{Name: "playwright", Command: "npx", Args: []string{"-y", "@playwright/mcp@latest"}, Description: "Browser automation via Playwright", Lazy: true}},
	},
	{
		name: "deep-search", skills: []string{"webresearch"}, category: "web",
		desc:  "Key-less web search via DuckDuckGo MCP with fast local text search for sifting results.",
		tags:  []string{"search", "web", "mcp"},
		mcp:   []mcp.Server{{Name: "duckduckgo", Command: "uvx", Args: []string{"duckduckgo-mcp-server"}, Description: "Web search via DuckDuckGo — no API key", Lazy: true}},
		tools: []string{"rg"},
	},
	{
		name: "document-suite", skills: []string{"officedocs", "pdftools"}, category: "docs",
		desc:  "Office documents and PDFs end-to-end, with an Excel MCP for real spreadsheets.",
		tags:  []string{"docs", "office", "pdf", "excel"},
		mcp:   []mcp.Server{{Name: "excel", Command: "uvx", Args: []string{"excel-mcp-server", "stdio"}, Description: "Create and edit Excel workbooks", Lazy: true}},
		tools: []string{"pandoc"},
	},
	{
		name: "personal-organizer", skills: []string{"emailtools", "calendartools"}, category: "comms",
		desc:  "Inbox and calendar in one pack, with timezone-aware scheduling via the time MCP.",
		tags:  []string{"email", "calendar", "assistant"},
		mcp:   []mcp.Server{{Name: "time", Command: "uvx", Args: []string{"mcp-server-time"}, Description: "Current time and timezone conversions", Lazy: true}},
	},
	{
		name: "container-ops", skills: []string{"dockerservices"}, category: "dev",
		desc:  "Run and manage containerized services with the Docker CLI at hand.",
		tags:  []string{"docker", "containers", "dev"},
		tools: []string{"docker", "jq"},
	},
	{
		name: "secops-toolkit", skills: []string{"cryptotools", "sshremote"}, category: "security",
		desc:  "Crypto operations and remote administration over SSH, with OpenSSL for the heavy lifting.",
		tags:  []string{"security", "ssh", "crypto"},
		tools: []string{"openssl", "ssh"},
	},
	{
		name: "media-workshop", skills: []string{"imagetools", "archivetools"}, category: "media",
		desc:  "Convert, resize and package media: image tooling plus archives, powered by ffmpeg.",
		tags:  []string{"media", "images", "ffmpeg"},
		tools: []string{"ffmpeg"},
	},
}

// Library is the built-in Official marketplace, implementing market.Library.
type Library struct {
	packs    []market.Pack
	index    map[string]market.Pack
	featured map[string]bool
}

// New builds the Official library from the embedded skill bundles + combos.
// Best-effort per bundle: a bundle that fails to read is skipped, not fatal.
func New() *Library {
	l := &Library{index: map[string]market.Pack{}, featured: map[string]bool{}}
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
		var packSkills []market.PackSkill
		for _, name := range c.skills {
			md, res, err := builtinskills.Bundle(name)
			if err != nil {
				continue
			}
			packSkills = append(packSkills, market.PackSkill{SkillMD: string(md), Resources: res})
		}
		if len(packSkills) == 0 {
			continue
		}
		l.add(market.Pack{
			Name:             c.name,
			Version:          "1.0.0",
			Description:      c.desc,
			Author:           "AGEZT",
			Category:         c.category,
			Tags:             c.tags,
			Skills:           packSkills,
			MCPServers:       c.mcp,
			ToolRequirements: c.tools,
		})
		if c.featured {
			l.featured[c.name] = true
		}
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
		e := p.Entry("")
		e.Featured = l.featured[p.Name]
		entries = append(entries, e)
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
