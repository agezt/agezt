// SPDX-License-Identifier: MIT
//
// kernel/market Pack methods (Counts, Validate, CanonicalBytes, ContentHash, Entry).
// Extracted from market.go during Day 211 god-file refactor (#87).
// Public API unchanged.
package market

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/skill"
)

func (p Pack) Counts() (skills, mcps, tools int) {
	return len(p.Skills), len(p.MCPServers), len(p.ToolRequirements)
}
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
func (p Pack) CanonicalBytes() ([]byte, error) {
	c := p
	c.Signature = nil
	// json.Marshal sorts map keys; slices keep author order. Stable enough for a
	// content hash because packs are built deterministically.
	return json.Marshal(c)
}
func (p Pack) ContentHash() (string, error) {
	b, err := p.CanonicalBytes()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
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
