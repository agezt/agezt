// SPDX-License-Identifier: MIT

// Prompt inject helpers: agentProfileSystem + profileTasksByScope + writeProfileTasks + injectMemory/UserProfile/Taste/World/Skills/Environment + capabilityBriefing + forgeBias + defaultShellHint + shellGuidance + firstSentence.
// Code extracted from prompt.go during the Day-73 god-file split. Public API unchanged.
package runtime


import (
	"fmt"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/taste"
	"github.com/agezt/agezt/kernel/worldmodel"
	"path/filepath"
	"slices"
	stdruntime "runtime"
	"strings"
	"time"
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

// shellHinter is the optional interface a shell-like tool implements to tell the
// environment preamble the EXACT interpreter it runs (binary + flag), so the
// guidance reflects an operator's shell override rather than a GOOS guess.
type shellHinter interface{ ShellHint() (string, string) }

// injectEnvironment prepends a concise host-environment preamble to the system
// prompt (M609): OS/arch, the shell the shell tool uses (with command-style
// guidance so the model doesn't try `ls` on Windows), the shared workspace dir,
// the date, and the run's available tools. This is the single highest-leverage
// fix for blind trial-and-error tool use on non-Unix hosts. `now` is passed in
// for deterministic tests.
func injectEnvironment(system, workspaceRoot string, tools map[string]agent.Tool, now time.Time) string {
	var b strings.Builder
	b.WriteString("## Runtime environment\n")
	b.WriteString("You run on a real host — act for THIS environment, do not assume Unix.\n")
	fmt.Fprintf(&b, "- OS / arch: %s / %s\n", stdruntime.GOOS, stdruntime.GOARCH)

	// Shell line: prefer the shell tool's own hint (honours overrides); fall back
	// to the GOOS default the shell tool would pick.
	shellBin, shellArg := defaultShellHint()
	if t, ok := tools["shell"]; ok {
		if h, ok := t.(shellHinter); ok {
			shellBin, shellArg = h.ShellHint()
		}
	}
	fmt.Fprintf(&b, "- Shell tool runs commands via `%s %s`. %s\n", shellBin, shellArg, shellGuidance(shellBin))

	if workspaceRoot != "" {
		fmt.Fprintf(&b, "- Working directory (shell + file tools both operate here): %s\n", workspaceRoot)
	}
	fmt.Fprintf(&b, "- Today: %s\n", now.Format("2006-01-02"))

	if len(tools) > 0 {
		names := make([]string, 0, len(tools))
		for name := range tools {
			names = append(names, name)
		}
		slices.Sort(names)
		b.WriteString("- Tools available this run:\n")
		for _, name := range names {
			fmt.Fprintf(&b, "  - %s — %s\n", name, firstSentence(tools[name].Definition().Description))
		}
	}
	if brief := capabilityBriefing(tools); brief != "" {
		b.WriteString(brief)
	}
	if bias := forgeBias(tools); bias != "" {
		b.WriteString(bias)
	}
	b.WriteString("Some capabilities require operator approval and may be denied; if a call is denied, adapt your approach rather than repeating it.\n")

	if system != "" {
		b.WriteString("\n")
		b.WriteString(system)
	}
	return b.String()
}

// capabilityBriefing tells the agent, plainly, how much it is allowed to do —
// so it knows its own reach instead of timidly assuming limits (M848). AGEZT is
// default-allow (owner law): every capability is on unless explicitly opted out,
// so an agent should marshal whatever it takes to finish the task — write and run
// code, install and run CLIs and packages, build whole apps and services. The
// briefing is tuned to the tools ACTUALLY present this run (no point promising
// code execution if code_exec was opted out), and stays honest about the few real
// rails: explicit operator denials, budgets, and the SSRF/secret guards. Returns
// "" when none of the relevant tools are available.
func capabilityBriefing(tools map[string]agent.Tool) string {
	_, hasShell := tools["shell"]
	_, hasCode := tools["code_exec"]
	_, hasFile := tools["file"]
	_, hasForge := tools["tool_forge"]
	_, hasSkill := tools["skill"]
	if !hasShell && !hasCode && !hasFile {
		return "" // nothing to brief — this run can't build or run anything
	}
	var b strings.Builder
	b.WriteString("## What you can do — act without artificial limits\n")
	b.WriteString("You are a full agent on a real machine, not a sandboxed chat. To accomplish the task you may do whatever it takes — there are no artificial capability limits. Be bold: build it, install it, run it.\n")
	if hasCode {
		b.WriteString("- Write and run real code — Python, Node/JavaScript, Deno — via code_exec. Build scripts, prototypes, data pipelines, whole programs.\n")
	}
	if hasShell {
		b.WriteString("- Install and run anything the host supports via the shell: CLI tools, npm / pip / cargo / go packages, build systems, even long-running background services. If a command is missing, install it, then use it.\n")
	}
	if hasFile {
		b.WriteString("- Create and edit as many files, projects, and applications as the task needs in your working directory.\n")
	}
	if hasForge {
		b.WriteString("- When a one-off script isn't enough, forge your own durable tool (tool_forge) so the capability persists.\n")
	}
	if hasSkill {
		b.WriteString("- Capture what works as a reusable skill — including bundled reference files and scripts — so future runs reuse it (skill op=learn / op=files / op=read).\n")
	}
	b.WriteString("Default to action: prefer doing the work over asking whether you're allowed. The only real limits are explicit — a denied approval, a spend budget, and the network/secret guards (no SSRF, secrets stay redacted). Everything else is yours to use.\n")
	return b.String()
}

// forgeBias nudges the agent toward DETERMINISM and SELF-IMPROVEMENT (M902): for
// work that must be exact, is repeatable, or you'll likely do again, prefer a
// tool over ad-hoc reasoning — write a script so the result is deterministic and
// re-runnable, forge a recurring script into a durable tool, and capture what
// works as a skill so it compounds across runs. Tuned to the tools present;
// returns "" when none of code_exec / tool_forge / skill is available (nothing
// to bias toward). Complements capabilityBriefing (which says what you CAN do)
// with how to work well.
func forgeBias(tools map[string]agent.Tool) string {
	_, hasCode := tools["code_exec"]
	_, hasForge := tools["tool_forge"]
	_, hasSkill := tools["skill"]
	if !hasCode && !hasForge && !hasSkill {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Prefer deterministic tools — and improve your own\n")
	b.WriteString("For work that must be exact, is repeatable, or you'll likely do again, reach for a tool instead of reasoning it out by hand each time:\n")
	if hasCode {
		b.WriteString("- Write a script (code_exec) so the result is deterministic, checkable, and re-runnable — not re-derived (and error-prone) each turn. Computation, parsing, transforms, and anything with exact rules belong in code.\n")
	}
	if hasForge {
		b.WriteString("- When a one-off script recurs, forge it into a durable tool (tool_forge) so the capability persists and the next run just calls it.\n")
	}
	if hasSkill {
		b.WriteString("- Check existing skills/tools before re-deriving one, and capture a working approach as a reusable skill (skill op=learn) so it's there next time.\n")
	}
	b.WriteString("Treat each run as self-improvement: when you hit a capability gap, build the tool that closes it — you finish faster and more reliably every time after.\n")
	return b.String()
}

// defaultShellHint mirrors the shell tool's platform default for callers that
// can't reach the live tool (e.g. tools map without a shell). cmd on Windows,
// sh elsewhere — kept in sync with plugins/tools/shell.resolveShell.
func defaultShellHint() (string, string) {
	if stdruntime.GOOS == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}

// shellGuidance returns one line of command-style advice keyed off the shell
// binary, so the model uses native commands (the #1 source of wasted iterations
// on Windows was the model reflexively trying `ls`/`cat`/`rm`).
func shellGuidance(shellBin string) string {
	// Normalize Windows separators first: on Linux filepath.Base treats
	// `C:\Win\cmd.exe` as one element and the interpreter would misroute
	// to the POSIX advice.
	switch strings.ToLower(filepath.Base(strings.ReplaceAll(shellBin, `\`, "/"))) {
	case "cmd", "cmd.exe":
		return "Use native Windows commands (dir, type, copy, del, move, findstr) — NOT ls/cat/rm/cp/mv/grep. Chain with `&&`."
	case "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return "Use PowerShell cmdlets (Get-ChildItem, Get-Content, Copy-Item, Remove-Item) or their aliases."
	default:
		return "Use standard POSIX commands (ls, cat, grep, rm). Chain with `&&`."
	}
}

// firstSentence trims a tool description to its first sentence (or first line),
// keeping the environment preamble compact when a tool has a long description.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	return strings.TrimSpace(s)
}

// Memory injection: recall relevant records and prepend them to the
// system prompt so the model starts the task already knowing what
// Agezt remembers. The recall is journaled (memory.retrieved) under
// corr, so `agt why` shows exactly what knowledge was surfaced.
// Per-run system-prompt override (WithSystem): a one-off identity/instruction
// set for this run only; falls back to the kernel's configured System. Memory /