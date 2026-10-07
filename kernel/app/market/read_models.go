// SPDX-License-Identifier: MIT
package market

import core "github.com/agezt/agezt/kernel/market"

type ListOutput struct {
	Packs []core.Listing `json:"packs"`
	Count int            `json:"count"`
}
type SourcesOutput struct {
	Sources []core.Source `json:"sources"`
	Count   int           `json:"count"`
}

// Name and Description are present together after a successful summary, even
// for empty descriptions; malformed summaries expose only the original SkillMD.
type SkillView struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	SkillMD     string  `json:"skill_md"`
}
type ShowOutput struct {
	Pack        core.Pack      `json:"pack"`
	SkillCount  int            `json:"skill_count"`
	MCPCount    int            `json:"mcp_count"`
	ToolCount   int            `json:"tool_count"`
	Skills      []SkillView    `json:"skills"`
	MCPServers  []string       `json:"mcp_servers"`
	Tools       []string       `json:"tools"`
	Installed   bool           `json:"installed"`
	InstalledAt int64          `json:"installed_at"`
	Vet         core.VetReport `json:"vet"`
}
