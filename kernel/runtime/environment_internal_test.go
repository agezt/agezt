// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

// envFakeTool is a minimal agent.Tool with a controllable name/description for
// asserting the preamble's tool list.
type envFakeTool struct {
	name, desc string
}

func (f envFakeTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: f.name, Description: f.desc, InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func (envFakeTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	return agent.Result{Output: "ok"}, nil
}

// envShellTool also implements shellHinter so the preamble can report an exact,
// override-aware shell.
type envShellTool struct {
	envFakeTool
	bin, arg string
}

func (s envShellTool) ShellHint() (string, string) { return s.bin, s.arg }

func TestInjectEnvironment_CoreFields(t *testing.T) {
	when := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	tools := map[string]agent.Tool{
		"file":  envFakeTool{name: "file", desc: "Read, write, and search files within the workspace. Long details follow here."},
		"shell": envShellTool{envFakeTool{name: "shell", desc: "Run a command."}, "cmd", "/C"},
	}
	out := injectEnvironment("BASE PERSONA", `C:\ws`, tools, when)

	for _, want := range []string{
		"## Runtime environment",
		"OS / arch: " + runtime.GOOS,
		`C:\ws`,
		"2026-06-08",
		"- shell —",
		"- file —",
		"BASE PERSONA", // base prompt preserved at the end
	} {
		if !strings.Contains(out, want) {
			t.Errorf("preamble missing %q\n---\n%s", want, out)
		}
	}
	// The shell hint must win: cmd /C reported with Windows guidance regardless of host.
	if !strings.Contains(out, "cmd /C") {
		t.Errorf("expected the shell tool's exact hint 'cmd /C':\n%s", out)
	}
	if !strings.Contains(out, "NOT ls/cat/rm") {
		t.Errorf("cmd shell must carry Windows command guidance:\n%s", out)
	}
	// First-sentence trimming: the file tool's long description is clipped.
	if strings.Contains(out, "Long details follow") {
		t.Errorf("tool description should be trimmed to the first sentence:\n%s", out)
	}
}

func TestCapabilityBriefing_TunedToTools(t *testing.T) {
	// A full tool set yields the emphatic, no-limits briefing with the relevant lines.
	full := map[string]agent.Tool{
		"shell":      envFakeTool{name: "shell", desc: "Run a command."},
		"code_exec":  envFakeTool{name: "code_exec", desc: "Run code."},
		"file":       envFakeTool{name: "file", desc: "Files."},
		"tool_forge": envFakeTool{name: "tool_forge", desc: "Forge tools."},
		"skill":      envFakeTool{name: "skill", desc: "Skills."},
	}
	out := capabilityBriefing(full)
	for _, want := range []string{
		"act without artificial limits",
		"Python, Node/JavaScript, Deno", // code_exec line
		"npm / pip",                     // shell install line
		"forge your own durable tool",   // tool_forge line
		"reusable skill",                // skill line
		"Default to action",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("full briefing missing %q\n---\n%s", want, out)
		}
	}

	// Tuned: no code_exec → no code line; no tool_forge → no forge line.
	noCode := map[string]agent.Tool{"shell": full["shell"], "file": full["file"]}
	out2 := capabilityBriefing(noCode)
	if strings.Contains(out2, "Python, Node/JavaScript") {
		t.Errorf("briefing promised code execution without code_exec:\n%s", out2)
	}
	if strings.Contains(out2, "forge your own durable tool") {
		t.Errorf("briefing promised tool_forge when absent:\n%s", out2)
	}

	// No build/run tools at all → empty briefing (nothing to promise).
	if got := capabilityBriefing(map[string]agent.Tool{"notify": full["skill"]}); got != "" {
		t.Errorf("expected empty briefing with no build/run tools, got:\n%s", got)
	}
}

func TestForgeBias_TunedToTools(t *testing.T) {
	full := map[string]agent.Tool{
		"code_exec":  envFakeTool{name: "code_exec", desc: "Run code."},
		"tool_forge": envFakeTool{name: "tool_forge", desc: "Forge tools."},
		"skill":      envFakeTool{name: "skill", desc: "Skills."},
	}
	out := forgeBias(full)
	for _, want := range []string{
		"Prefer deterministic tools",
		"Write a script (code_exec)",                     // code line
		"forge it into a durable tool",                   // tool_forge line
		"capture a working approach as a reusable skill", // skill line
		"self-improvement",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("forge bias missing %q\n---\n%s", want, out)
		}
	}

	// Tuned: no tool_forge → no forge line; no code_exec → no script line.
	noForge := map[string]agent.Tool{"code_exec": full["code_exec"]}
	if strings.Contains(forgeBias(noForge), "durable tool") {
		t.Errorf("forge bias promised tool_forge when absent")
	}
	noCode := map[string]agent.Tool{"skill": full["skill"]}
	if strings.Contains(forgeBias(noCode), "Write a script") {
		t.Errorf("forge bias promised code_exec when absent")
	}

	// None of code_exec/tool_forge/skill → empty (nothing to bias toward).
	if got := forgeBias(map[string]agent.Tool{"notify": full["skill"]}); got != "" {
		t.Errorf("expected empty forge bias with no relevant tools, got:\n%s", got)
	}
}

func TestShellGuidance_ByInterpreter(t *testing.T) {
	cases := map[string]string{
		"cmd":            "Windows",
		`C:\Win\cmd.exe`: "Windows",
		"powershell":     "PowerShell",
		"pwsh":           "PowerShell",
		"sh":             "POSIX",
		"/bin/bash":      "POSIX",
	}
	for bin, want := range cases {
		got := shellGuidance(bin)
		if !strings.Contains(got, want) {
			t.Errorf("shellGuidance(%q) = %q, want it to mention %q", bin, got, want)
		}
	}
}

func TestFirstSentence(t *testing.T) {
	if got := firstSentence("Run a command. More stuff."); got != "Run a command." {
		t.Errorf("got %q", got)
	}
	if got := firstSentence("One line only"); got != "One line only" {
		t.Errorf("got %q", got)
	}
	if got := firstSentence("First line\nsecond line"); got != "First line" {
		t.Errorf("multiline trim got %q", got)
	}
}
