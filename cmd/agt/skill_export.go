// SPDX-License-Identifier: MIT

// Package main: `agt skill export` — skillBundle + skillBundleBody types +
// buildSkillBundle + verifySkillBundle + safeSkillFilename (the bundle
// surface). The exportAllSkills + cmdSkillExport moved to skill_export_cli.go.
// Day-211 god-file split. Public API unchanged.
package main


import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/skill"
)
// skillBundle is the portable, shareable representation of a single skill — the
// foundation for moving a skill between Agezt instances (M268). It carries only
// the skill's CONTENT fields, never instance-local state (status, metrics,
// timestamps, the producing journal event): an imported skill should arrive as a
// fresh draft on the target, not inherit the source's lifecycle. The skill ID is
// content-addressed over (name, body), so a bundle is self-verifying — see
// verifySkillBundle.
type skillBundle struct {
	Tool           string          `json:"tool"`
	FormatVersion  int             `json:"format_version"`
	ExportedUnixMS int64           `json:"exported_unix_ms"`
	Skill          skillBundleBody `json:"skill"`
}

// skillBundleBody is the shareable subset of skill.Skill (JSON tags match so the
// fields round-trip from the daemon's `--json` shape verbatim).
type skillBundleBody struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Triggers      []string `json:"triggers,omitempty"`
	Body          string   `json:"body"`
	ToolsRequired []string `json:"tools_required,omitempty"`
	Version       string   `json:"version"`
	Lineage       []string `json:"lineage,omitempty"`
}

// buildSkillBundle projects a daemon skill record (the map returned under
// CmdSkillGet's "skill" key) into a portable bundle. The JSON round-trip drops
// every non-shareable field by construction, since skillBundleBody declares only
// the content fields.
func buildSkillBundle(skillMap map[string]any, nowMS int64) (skillBundle, error) {
	raw, err := json.Marshal(skillMap)
	if err != nil {
		return skillBundle{}, fmt.Errorf("re-encode skill: %w", err)
	}
	var body skillBundleBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return skillBundle{}, fmt.Errorf("decode skill: %w", err)
	}
	return skillBundle{
		Tool:           brand.CLI,
		FormatVersion:  1,
		ExportedUnixMS: nowMS,
		Skill:          body,
	}, nil
}

// verifySkillBundle checks a bundle's integrity: its name/body must hash to its
// claimed content-addressed ID (the same address the skill store uses). A
// mismatch means the bundle was tampered with or built by hand incorrectly.
func verifySkillBundle(b skillBundle) error {
	if strings.TrimSpace(b.Skill.Name) == "" {
		return fmt.Errorf("bundle has no skill name")
	}
	if strings.TrimSpace(b.Skill.ID) == "" {
		return fmt.Errorf("bundle has no skill id")
	}
	want := skill.ContentID(b.Skill.Name, b.Skill.Body)
	if want != b.Skill.ID {
		return fmt.Errorf("content-address mismatch: id=%s but name+body hash to %s", b.Skill.ID, want)
	}
	return nil
}

// safeSkillFilename builds a stable, filesystem-safe bundle filename from a
// skill name and id: lowercased name with non-alphanumeric runs collapsed to a
// dash, plus a short id so two versions of the same name never collide.
func safeSkillFilename(name, id string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	safe := strings.Trim(b.String(), "-")
	if safe == "" {
		safe = "skill"
	}
	short := id
	if len(short) > 12 {
		short = short[:12]
	}
	return fmt.Sprintf("%s-%s.skill.json", safe, short)
}

// exportAllSkills writes every skill to its own bundle file in dir (one
// CmdSkillList call supplies the full records, bodies included). It is the
// publisher side of the skill registry: a node exports its whole skill library
// as a directory another node can browse with `agt skill registry`.
// exportAllSkills writes one bundle per skill into dir plus a registry index.
// When agentFilter is non-empty, only skills owned by that roster agent (M932)
// are exported — `skill export --all --agent <slug>` lifts one agent's private
// skill set out as a portable bundle directory (M943).
