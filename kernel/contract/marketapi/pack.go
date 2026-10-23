// SPDX-License-Identifier: MIT

package marketapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
)

// Counts returns how many skills, MCP servers and CLI-tool requirements the
// pack carries — the at-a-glance numbers a gallery card shows.
func (p Pack) Counts() (skills, mcps, tools int) {
	return len(p.Skills), len(p.MCPServers), len(p.ToolRequirements)
}

// Validate reports whether the pack may be installed: its name is a safe
// identifier, its version is semver, it carries something at all, every skill
// it carries is a parseable SKILL.md with safe resource paths, and every MCP
// server it carries passes the contract's own check.
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
		if _, err := skillapi.ParseSkillMD([]byte(ps.SkillMD)); err != nil {
			return fmt.Errorf("market: pack %q skill #%d: %w", p.Name, i, err)
		}
		for rel := range ps.Resources {
			if err := SafeRelPath(rel); err != nil {
				return fmt.Errorf("market: pack %q skill #%d resource %q: %w", p.Name, i, rel, err)
			}
		}
	}
	for i := range p.MCPServers {
		if err := mcpapi.Validate(p.MCPServers[i]); err != nil {
			return fmt.Errorf("market: pack %q mcp #%d: %w", p.Name, i, err)
		}
	}
	return nil
}

// CanonicalBytes returns the pack's canonical bytes — the bytes a signature
// is over and the content hash is computed from. The signature is stripped:
// it signs the content, so it cannot be part of it.
func (p Pack) CanonicalBytes() ([]byte, error) {
	c := p
	c.Signature = nil
	// json.Marshal sorts map keys; slices keep author order. Stable enough for a
	// content hash because packs are built deterministically.
	return json.Marshal(c)
}

// ContentHash returns the pack's content hash — the hex SHA-256 of its
// canonical bytes.
func (p Pack) ContentHash() (string, error) {
	b, err := p.CanonicalBytes()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Entry returns the pack's catalogue row — the metadata a marketplace index
// lists it under, plus where to fetch the full pack from.
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
