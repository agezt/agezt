// SPDX-License-Identifier: MIT

// Prompt construction: the agent-profile + tasks section + firstSentence utility.
// The context-injection helpers live in prompt_context.go; the environment +
// shell section lives in prompt_environment.go.
// Extracted from prompt.go during the Day-203 god-file split.
// Public API unchanged.
package runtime

import (
	"strings"

	"github.com/agezt/agezt/kernel/roster"
)

func agentProfileSystem(p roster.Profile) string {
	var b strings.Builder
	if soul := strings.TrimSpace(p.Soul); soul != "" {
		b.WriteString(soul)
	}
	if len(p.Instructions) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Standing instructions:\n")
		for _, ins := range p.Instructions {
			if ins = strings.TrimSpace(ins); ins != "" {
				b.WriteString("- ")
				b.WriteString(ins)
				b.WriteString("\n")
			}
		}
	}
	if len(p.TaskList) > 0 {
		cycle, total := profileTasksByScope(p.TaskList)
		if len(cycle) > 0 || len(total) > 0 {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			if len(cycle) > 0 {
				b.WriteString("\nCycle tasks:\n")
				writeProfileTasks(&b, cycle)
			}
			if len(total) > 0 {
				b.WriteString("\nTotal tasks:\n")
				writeProfileTasks(&b, total)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func profileTasksByScope(tasks []roster.AgentTask) (cycle, total []roster.AgentTask) {
	for _, t := range tasks {
		status := strings.TrimSpace(t.Status)
		if status == "done" || status == "retired" {
			continue
		}
		if strings.TrimSpace(t.Scope) == "cycle" {
			cycle = append(cycle, t)
		} else {
			total = append(total, t)
		}
	}
	return cycle, total
}

func writeProfileTasks(b *strings.Builder, tasks []roster.AgentTask) {
	for i, t := range tasks {
		if i >= 20 {
			b.WriteString("- ...\n")
			return
		}
		title := strings.TrimSpace(t.Title)
		if title == "" {
			continue
		}
		status := strings.TrimSpace(t.Status)
		if status == "" {
			status = "todo"
		}
		b.WriteString("- [")
		b.WriteString(status)
		b.WriteString("] ")
		b.WriteString(title)
		if desc := strings.TrimSpace(t.Description); desc != "" {
			b.WriteString(" - ")
			b.WriteString(desc)
		}
		b.WriteString("\n")
	}
}
