// SPDX-License-Identifier: MIT

package skill

// Provenance: the SKILL.md parser moved to the contract layer
//             (kernel/contract/skillapi): it is pure — a stdlib-only
//             frontmatter parser — and the marketplace's built-in catalogue
//             (a PLUGIN) parses the built-in bundles with it. The name stays
//             here as a delegation, so every existing caller is unchanged.

import "github.com/agezt/agezt/kernel/contract/skillapi"

// SkillMD is the parsed content of an agentskills.io / ClawHub "SKILL.md" file
// (SPEC-13 §1.2).
//
// The row itself lives in the contract layer (kernel/contract/skillapi); the
// name stays here as an alias, so every existing caller is unchanged.
type SkillMD = skillapi.SkillMD

// ErrNoFrontmatter is returned when the file does not begin with a '---'
// frontmatter delimiter.
var ErrNoFrontmatter = skillapi.ErrNoFrontmatter

// ParseSkillMD parses a SKILL.md document.
//
// The parser itself lives in the contract layer (kernel/contract/skillapi) —
// it is pure, and the marketplace's built-in catalogue (a plugin) parses the
// built-in bundles with it. This is a delegation, so there is one parser.
func ParseSkillMD(data []byte) (SkillMD, error) { return skillapi.ParseSkillMD(data) }
