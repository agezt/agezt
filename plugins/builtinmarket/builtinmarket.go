// SPDX-License-Identifier: MIT

// Package builtinmarket is the marketplace's built-in "Official" catalogue: it
// wraps the binary's embedded skill bundles (the skills module's built-in bundles) — and a few
// curated combo packs that add an MCP server + CLI-tool requirements — into
// installable packs. It is always present and works offline; remote
// marketplaces (Phase 2) layer on top. It references the embedded bundle bytes
// via the contract's built-ins port rather than duplicating them.
package builtinmarket

import (
	"fmt"

	"github.com/agezt/agezt/kernel/contract/marketapi"
	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
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
	mcp      []mcpapi.Server
	tools    []string
	featured bool
}

var combos = []combo{
	{
		name: "web-research-pro", skills: []string{"webresearch"}, category: "web", featured: true,
		desc: "Cited web research with a fetch MCP server and fast local search tools.",
		tags: []string{"web", "research", "mcp"},
		// The fetch server ships on PyPI, not npm — @modelcontextprotocol/server-fetch
		// has never been published (DEP-006). The UI catalog (Mcp.tsx) already has this right.
		mcp:   []mcpapi.Server{{Name: "fetch", Command: "uvx", Args: []string{"mcp-server-fetch"}, Description: "HTTP fetch for the agent", Lazy: true}},
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
		desc: "Research that remembers: persistent knowledge-graph memory plus a structured reasoning scratchpad.",
		tags: []string{"memory", "knowledge", "reasoning", "mcp"},
		mcp: []mcpapi.Server{
			{Name: "memory", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-memory"}, Description: "Persistent knowledge-graph memory", Lazy: true},
			{Name: "thinking", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-sequential-thinking"}, Description: "Step-by-step reasoning scratchpad", Lazy: true},
		},
	},
	{
		name: "browser-automation-pro", skills: []string{"browseruse"}, category: "web", featured: true,
		desc: "Drive a real browser: navigate, click, fill forms and screenshot via the official Playwright MCP.",
		tags: []string{"browser", "automation", "playwright", "mcp"},
		mcp:  []mcpapi.Server{{Name: "playwright", Command: "npx", Args: []string{"-y", "@playwright/mcp@latest"}, Description: "Browser automation via Playwright", Lazy: true}},
	},
	{
		name: "deep-search", skills: []string{"webresearch"}, category: "web",
		desc:  "Key-less web search via DuckDuckGo MCP with fast local text search for sifting results.",
		tags:  []string{"search", "web", "mcp"},
		mcp:   []mcpapi.Server{{Name: "duckduckgo", Command: "uvx", Args: []string{"duckduckgo-mcp-server"}, Description: "Web search via DuckDuckGo — no API key", Lazy: true}},
		tools: []string{"rg"},
	},
	{
		name: "document-suite", skills: []string{"officedocs", "pdftools"}, category: "docs",
		desc:  "Office documents and PDFs end-to-end, with an Excel MCP for real spreadsheets.",
		tags:  []string{"docs", "office", "pdf", "excel"},
		mcp:   []mcpapi.Server{{Name: "excel", Command: "uvx", Args: []string{"excel-mcp-server", "stdio"}, Description: "Create and edit Excel workbooks", Lazy: true}},
		tools: []string{"pandoc"},
	},
	{
		name: "personal-organizer", skills: []string{"emailtools", "calendartools"}, category: "comms",
		desc: "Inbox and calendar in one pack, with timezone-aware scheduling via the time MCP.",
		tags: []string{"email", "calendar", "assistant"},
		mcp:  []mcpapi.Server{{Name: "time", Command: "uvx", Args: []string{"mcp-server-time"}, Description: "Current time and timezone conversions", Lazy: true}},
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

// Library is the built-in Official marketplace, implementing marketapi.Library.
type Library struct {
	packs    []marketapi.Pack
	index    map[string]marketapi.Pack
	featured map[string]bool
}

// New builds the Official library from the embedded skill bundles + combos.
// Best-effort per bundle: a bundle that fails to read is skipped, not fatal.
func New(builtins skillapi.Builtins) *Library {
	l := &Library{index: map[string]marketapi.Pack{}, featured: map[string]bool{}}
	for _, name := range builtins.Names() {
		md, res, err := builtins.Bundle(name)
		if err != nil {
			continue
		}
		parsed, perr := skillapi.ParseSkillMD(md)
		if perr != nil {
			continue
		}
		l.add(marketapi.Pack{
			Name:        name + "-pack",
			Version:     "1.0.0",
			Description: parsed.Description,
			Author:      "AGEZT",
			Category:    categoryFor(name),
			Tags:        append([]string{name}, parsed.Triggers...),
			Skills:      []marketapi.PackSkill{{SkillMD: string(md), Resources: res}},
		})
	}
	for _, c := range combos {
		var packSkills []marketapi.PackSkill
		for _, name := range c.skills {
			md, res, err := builtins.Bundle(name)
			if err != nil {
				continue
			}
			packSkills = append(packSkills, marketapi.PackSkill{SkillMD: string(md), Resources: res})
		}
		if len(packSkills) == 0 {
			continue
		}
		l.add(marketapi.Pack{
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

func (l *Library) add(p marketapi.Pack) {
	l.packs = append(l.packs, p)
	l.index[p.Name] = p
}

func categoryFor(bundle string) string {
	if c, ok := skillCategory[bundle]; ok {
		return c
	}
	return "skills"
}

// Marketplaces implements the contract's marketapi.Library: a single built-in Official catalogue.
func (l *Library) Marketplaces() []marketapi.Marketplace {
	entries := make([]marketapi.MarketplaceEntry, 0, len(l.packs))
	for _, p := range l.packs {
		e := p.Entry("")
		e.Featured = l.featured[p.Name]
		entries = append(entries, e)
	}
	return []marketapi.Marketplace{{
		Name:          MarketplaceName,
		Owner:         "AGEZT",
		FormatVersion: marketapi.FormatVersion,
		Builtin:       true,
		Packs:         entries,
	}}
}

// ResolvePack implements the contract's marketapi.Library. An explicit version is an exact
// request, honored only when the catalogue carries it — anything else is an
// error naming what Official actually has, never a silent substitute. That
// matters beyond this package: the composite library consults the builtin
// FIRST, so a version-mismatch error here falls through to the synced
// marketplaces (which may genuinely carry the requested version), whereas a
// silent hit here would shadow them with a version nobody asked for.
func (l *Library) ResolvePack(_, name, version string) (marketapi.Pack, error) {
	p, ok := l.index[name]
	if !ok {
		return marketapi.Pack{}, fmt.Errorf("builtinmarket: pack %q not found in the Official marketplace", name)
	}
	if version != "" && marketapi.CompareVersions(p.Version, version) != 0 {
		return marketapi.Pack{}, fmt.Errorf("builtinmarket: Official carries %q at %s, not %s", name, p.Version, version)
	}
	return p, nil
}
