// SPDX-License-Identifier: MIT

// Package acpagent is the in-process ACP-client bridge tool (SPEC-15 §3, the
// inverse of kernel/acp's server): it delegates a task to an *external* ACP
// agent (Claude Code, Codex, Gemini CLI, or any agent that speaks the Agent
// Client Protocol) by spawning it as a subprocess and driving it over JSON-RPC
// 2.0 on stdio — initialize → session/new → session/prompt — relaying the
// agent's streamed message back as the tool result. This lets a Agezt run
// orchestrate another agent as a governed step.
//
// The agent command is configured by the operator (AGEZT_ACP_AGENT_CMD); unset
// → the tool is not registered. The external agent runs with the workspace as
// its session cwd. Because the spawn has real side effects (the external agent
// can edit files / run commands in its own sandbox), the tool is gated Ask-first
// by Edict (the acp_agent capability), like the coding bridge.
package acpagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/acp"
	"github.com/agezt/agezt/kernel/agent"
)

// DefaultTimeout caps one delegated ACP session.
const DefaultTimeout = 5 * time.Minute

// MaxOutputBytes truncates the relayed answer so a runaway agent can't blow the
// context budget.
const MaxOutputBytes = 60 * 1024

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

// New builds an ACP-agent bridge Tool. cmd launches the external ACP agent; cwd
// is the session working directory. Returns nil when cmd is empty (disabled).
func New(cmd, cwd string) *Tool {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}
	return &Tool{Cmd: cmd, Cwd: cwd, dial: spawnAgent}
}

func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "acp_agent",
		Description: "Delegate a task to an EXTERNAL agent that speaks the Agent Client Protocol " +
			"(Claude Code, Codex, Gemini CLI, …) and return its answer. The external agent runs in " +
			"its own sandbox with the workspace as its working directory; use it to hand off work to " +
			"a specialised agent. The result is what that agent reports back.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "task": {
      "type": "string",
      "description": "The complete, self-contained instruction for the external agent."
    }
  },
  "required": ["task"]
}`),
	}
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var in struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	task := strings.TrimSpace(in.Task)
	if task == "" {
		return agent.Result{Output: "task is required", IsError: true}, nil
	}
	if strings.TrimSpace(t.Cmd) == "" {
		return agent.Result{Output: "ACP agent not configured (set AGEZT_ACP_AGENT_CMD)", IsError: true}, nil
	}

	to := t.Timeout
	if to <= 0 {
		to = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	tr, err := t.dial(ctx, t.Cmd, t.Cwd)
	if err != nil {
		return agent.Result{Output: "spawn ACP agent failed: " + err.Error(), IsError: true}, nil
	}
	defer func() { _ = tr.close() }()

	client := acp.NewClient(tr.out, tr.in)
	if err := client.Initialize(ctx); err != nil {
		return agent.Result{Output: "ACP initialize failed: " + err.Error(), IsError: true}, nil
	}
	cwd := t.Cwd
	if cwd == "" {
		cwd = "."
	}
	sid, err := client.NewSession(ctx, cwd)
	if err != nil {
		return agent.Result{Output: "ACP session/new failed: " + err.Error(), IsError: true}, nil
	}

	var answer strings.Builder
	stop, err := client.Prompt(ctx, sid, task, func(chunk string) {
		answer.WriteString(chunk)
	})
	if err != nil {
		return agent.Result{Output: "ACP session/prompt failed: " + err.Error() + render(answer.String(), ""), IsError: true}, nil
	}
	return agent.Result{Output: render(answer.String(), stop)}, nil
}

// render formats the relayed answer with a short footer noting the stop reason.
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
	return s[:max] + fmt.Sprintf("\n… [truncated %d bytes]", len(s)-max)
}

func platformShell() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}

// spawnAgent launches the configured ACP agent command via the platform shell,
// wiring its stdin/stdout for JSON-RPC. Its stderr is forwarded to ours so agent
// diagnostics remain visible. close() shuts stdin (signalling the agent to exit)
// then reaps the process.
func spawnAgent(ctx context.Context, cmdStr, cwd string) (*transport, error) {
	shell, arg := platformShell()
	c := exec.Command(shell, arg, cmdStr) // not CommandContext: we manage teardown via close()
	if cwd != "" {
		c.Dir = cwd
	}
	c.Stderr = os.Stderr
	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := c.Start(); err != nil {
		return nil, err
	}
	return &transport{
		out: stdout,
		in:  stdin,
		close: func() error {
			_ = stdin.Close() // EOF → graceful exit for a well-behaved agent
			done := make(chan error, 1)
			go func() { done <- c.Wait() }()
			select {
			case err := <-done:
				return err
			case <-time.After(5 * time.Second):
				_ = c.Process.Kill()
				return <-done
			}
		},
	}, nil
}

// AbsCwd returns the absolute, cleaned workspace path. Exposed for daemon wiring.
func AbsCwd(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
