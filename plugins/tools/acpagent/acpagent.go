// SPDX-License-Identifier: MIT
//
// acpagent Tool type + transport + dialFunc + constructor + Definition +
// render/shell helpers (New, Definition, render, truncate, platformShell, AbsCwd).
// Extracted from acpagent.go during Day 211 god-file refactor (#69).
// Public API unchanged.
package acpagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/acpcatalog"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
)

// transport is the spawned agent's stdio: out is its stdout (we read), in is its
// stdin (we write), close tears the process down.
type transport struct {
	out   io.Reader
	in    io.Writer
	close func() error
}

// dialFunc spawns the external ACP agent and returns its transport. Injectable
// for tests (a fake ACP peer over pipes).
type dialFunc func(ctx context.Context, cmd, cwd string) (*transport, error)

// Tool implements agent.Tool. Constructed only when an agent command is
// configured; see New.
type Tool struct {
	// Cmd is the shell command that launches the external ACP agent, e.g.
	// `claude-code-acp` or `codex acp`. It must speak ACP over stdio.
	Cmd string
	// Cwd is the session working directory handed to session/new (the workspace).
	Cwd string
	// Timeout overrides DefaultTimeout when > 0.
	Timeout time.Duration
	// dial spawns the agent; overridable in tests. Defaults to spawnAgent.
	dial dialFunc
}

// New builds an ACP-agent bridge Tool. cmd is the DEFAULT external ACP agent
func New(cmd, cwd string) *Tool {
	if strings.TrimSpace(cmd) == "" && !acpcatalog.AnyInstalled() {
		return nil
	}
	return &Tool{Cmd: cmd, Cwd: cwd, dial: spawnAgent}
}
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:       "acp_agent",
		Capability: agent.ToolCapability{Name: string(edict.CapACPAgent)},
		Description: "Delegate a task to an EXTERNAL agent that speaks the Agent Client Protocol " +
			"(Claude Code, Codex, Gemini CLI, …) and return its answer. The external agent runs in " +
			"its own sandbox with the workspace as its working directory; use it to hand off work to " +
			"a specialised agent. Optionally pick which installed ACP agent to use with `agent` " +
			"(a catalog slug like \"gemini\", \"claude-code\", or \"codex\"); omit it to use the " +
			"configured default. The result is what that agent reports back.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "task": {
      "type": "string",
      "description": "The complete, self-contained instruction for the external agent."
    },
    "agent": {
      "type": "string",
      "description": "Optional: which installed ACP agent to delegate to (catalog slug, e.g. \"gemini\", \"claude-code\", \"codex\"). Omit to use the configured default."
    }
  },
  "required": ["task"]
}`),
		Effect: agent.ToolEffect{
			Class: agent.EffectCompensable,
			PredictedEffects: []string{
				"Spawn an operator-configured external ACP agent process.",
				"Delegate a task to that agent in the workspace and relay its answer back into this run.",
			},
			AffectedResources: []string{"external ACP agent process", "workspace visible to ACP session", "any sandbox or tools owned by the external agent"},
			RollbackNotes:     "The bridge tears down the ACP process, but side effects performed by the external agent require that agent's own rollback or manual cleanup.",
			Confidence:        0.55,
		},
	}
}
func render(answer, stop string) string {
	var b strings.Builder
	a := strings.TrimSpace(answer)
	if a == "" {
		b.WriteString("The external ACP agent returned no message.")
	} else {
		b.WriteString(truncate(a, MaxOutputBytes))
	}
	if stop != "" && stop != "end_turn" {
		fmt.Fprintf(&b, "\n\n[stopReason: %s]", stop)
	}
	return b.String()
}
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	prefix := strutil.Ellipsis(s, max, "")
	return prefix + fmt.Sprintf("\n… [truncated %d bytes]", len(s)-len(prefix))
}
func platformShell() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}
func AbsCwd(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
