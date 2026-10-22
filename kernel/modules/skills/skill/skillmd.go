// SPDX-License-Identifier: MIT

package skill

import (
	"errors"
	"strings"
)

// SkillMD is the parsed content of an agentskills.io / ClawHub "SKILL.md" file
// (SPEC-13 §1.2): a YAML-ish frontmatter block plus a Markdown body. Ingesting
// this open-standard format lets the hundreds of existing community skills load
// into Agezt's skill system (where they additionally gain versioning,
// shadow-testing, and reversibility) without rewriting them.
type SkillMD struct {
	Name          string
	Description   string
	Triggers      []string
	ToolsRequired []string
	Version       string
	Body          string
}

// ErrNoFrontmatter is returned when the file does not begin with a '---'
// frontmatter delimiter.
var ErrNoFrontmatter = errors.New("skill: SKILL.md must begin with a '---' frontmatter block")

// ParseSkillMD parses a SKILL.md document. It is a deliberately minimal,
// stdlib-only frontmatter parser (Agezt takes no YAML dependency) covering the
// forms these files actually use:
//   - scalars:        name: diagnose-ci
//   - inline lists:   triggers: [ci, build, failure]
//   - block lists:    tools_required:\n  - shell\n  - http
//
// Keys are case-insensitive; `tools` is accepted as an alias for
// `tools_required`. Unknown keys are ignored (forward-compatible). Quotes around
// scalar values are stripped. A non-empty name and body are required.
func ParseSkillMD(data []byte) (SkillMD, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	// Skip leading blank lines, then require an opening '---'.
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || strings.TrimSpace(lines[i]) != "---" {
		return SkillMD{}, ErrNoFrontmatter
	}
	i++ // past the opening '---'
	fmStart := i
	for i < len(lines) && strings.TrimSpace(lines[i]) != "---" {
		i++
	}
	if i >= len(lines) {
		return SkillMD{}, errors.New("skill: unterminated frontmatter (missing closing '---')")
	}
	fmLines := lines[fmStart:i]
	body := strings.Trim(strings.Join(lines[i+1:], "\n"), "\n")

	md := SkillMD{Body: body}
	for j := 0; j < len(fmLines); j++ {
		key, val, ok := strings.Cut(fmLines[j], ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "name":
			md.Name = unquoteScalar(val)
		case "description":
			md.Description = unquoteScalar(val)
		case "version":
			md.Version = unquoteScalar(val)
		case "triggers":
			md.Triggers = parseFrontmatterList(val, fmLines, &j)
		case "tools_required", "tools":
			md.ToolsRequired = parseFrontmatterList(val, fmLines, &j)
		}
	}
	if md.Name == "" {
		return SkillMD{}, errors.New("skill: SKILL.md frontmatter needs a name")
	}
	if strings.TrimSpace(md.Body) == "" {
		return SkillMD{}, errors.New("skill: SKILL.md has an empty body")
	}
	return md, nil
}

// parseFrontmatterList reads a list value in either inline (`[a, b]`), single-
// scalar (`a`), or block (`- a` lines following the key) form. For the block
// form it advances *j past the consumed item lines.
func parseFrontmatterList(val string, lines []string, j *int) []string {
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		inner := strings.TrimSuffix(strings.TrimPrefix(val, "["), "]")
		var out []string
		for _, p := range strings.Split(inner, ",") {
			if item := unquoteScalar(strings.TrimSpace(p)); item != "" {
				out = append(out, item)
			}
		}
		return out
	}
	if val != "" {
		return []string{unquoteScalar(val)} // `triggers: ci` — a single value
	}
	// Block list: consume following indented `- item` lines.
	var out []string
	for *j+1 < len(lines) {
		next := strings.TrimSpace(lines[*j+1])
		if !strings.HasPrefix(next, "-") {
			break // next key (or blank) ends the list
		}
		if item := unquoteScalar(strings.TrimSpace(strings.TrimPrefix(next, "-"))); item != "" {
			out = append(out, item)
		}
		*j++
	}
	return out
}

// unquoteScalar strips a single pair of surrounding single or double quotes.
func unquoteScalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
