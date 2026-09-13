// SPDX-License-Identifier: MIT
//
// Prompt context-injection helpers: injectMemory + injectUserProfile +
// injectTaste + injectWorld + injectSkills. Each takes the current system
// prompt and a list of scored hits / exemplars, and returns the augmented
// system prompt.
// Extracted from prompt.go during the Day-203 god-file split.
// Public API unchanged.
package runtime

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/taste"
	"github.com/agezt/agezt/kernel/worldmodel"
)

// injectMemory prepends a compact "Relevant memory" block to the system
// prompt. Records are rendered one per line as "- [TYPE] subject: content".
func injectMemory(system string, hits []memory.Scored) string {
	var b strings.Builder
	b.WriteString("Relevant memory (recalled from prior tasks; use if helpful):\n")
	for _, h := range hits {
		fmt.Fprintf(&b, "- [%s] %s: %s\n", h.Record.Type, h.Record.Subject, h.Record.Content)
	}
	if system != "" {
		b.WriteString("\n")
		b.WriteString(system)
	}
	return b.String()
}

// injectUserProfile prepends the learned operator profile (M1000) to the system
// prompt so the agent always knows who it works for. profileText is the
// pre-formatted facet block from memory.ProfileText (never empty here).
func injectUserProfile(system, profileText string) string {
	var b strings.Builder
	b.WriteString("What you know about the operator you work for (apply naturally; don't recite):\n")
	b.WriteString(profileText)
	b.WriteString("\n")
	if system != "" {
		b.WriteString("\n")
		b.WriteString(system)
	}
	return b.String()
}

// injectTaste prepends a "what good looks like" block of curated exemplars to
// the system prompt so the model anchors its output to concrete examples of good
// work. Each exemplar renders as a titled block; scoped ones are already ordered
// first by the store.
func injectTaste(system string, exemplars []taste.Exemplar) string {
	var b strings.Builder
	b.WriteString("What good looks like (curated exemplars — match this quality and style, don't copy verbatim):\n")
	for _, e := range exemplars {
		b.WriteString("\n### ")
		b.WriteString(e.Title)
		b.WriteString("\n")
		b.WriteString(e.Body)
		b.WriteString("\n")
	}
	if system != "" {
		b.WriteString("\n")
		b.WriteString(system)
	}
	return b.String()
}

// injectWorld prepends a compact "Known entities" block to the system prompt.
// Entities are rendered one per line as "- [kind] name (aliases: ...)" so the
// model can ground references like "the portfolio" to concrete things.
func injectWorld(system string, hits []worldmodel.ScoredEntity) string {
	var b strings.Builder
	b.WriteString("Known entities (from the world model; use to ground references):\n")
	for _, h := range hits {
		e := h.Entity
		if len(e.Aliases) > 0 {
			fmt.Fprintf(&b, "- [%s] %s (aka %s)\n", e.Kind, e.Name, strings.Join(e.Aliases, ", "))
		} else {
			fmt.Fprintf(&b, "- [%s] %s\n", e.Kind, e.Name)
		}
	}
	if system != "" {
		b.WriteString("\n")
		b.WriteString(system)
	}
	return b.String()
}

// injectSkills prepends matching active skills' bodies to the system prompt so
// the model plans with learned procedures. Each is rendered as a titled block.
func injectSkills(system string, hits []skill.Scored) string {
	var b strings.Builder
	b.WriteString("Applicable skills (learned procedures; follow if relevant):\n")
	for _, h := range hits {
		s := h.Skill
		fmt.Fprintf(&b, "## %s — %s\n%s\n", s.Name, s.Description, s.Body)
		// A bundled skill (agentskills.io shape, M847) ships reference files and
		// scripts. List them and tell the agent how to reach them: read a reference
		// with `skill op=read`, run a script with shell/code_exec from the dir that
		// `skill op=files` reports. This is what lets a skill say "run scripts/setup.sh
		// to install the CLI" and have the agent actually do it.
		if len(s.Resources) > 0 {
			fmt.Fprintf(&b, "Bundled resources (use the `skill` tool: op=files for the directory, op=read \"<path>\" to read one; run scripts with shell/code_exec):\n")
			for _, r := range s.Resources {
				fmt.Fprintf(&b, "  - %s\n", r)
			}
		}
	}
	if system != "" {
		b.WriteString("\n")
		b.WriteString(system)
	}
	return b.String()
}

