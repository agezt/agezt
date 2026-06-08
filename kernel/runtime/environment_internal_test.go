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
