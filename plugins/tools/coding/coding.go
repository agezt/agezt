// SPDX-License-Identifier: MIT

// Package coding is the in-process coding-agent bridge tool (ROADMAP P6-CODE,
// SPEC-04 §4): it delegates a coding task to an external coding agent
// (Claude Code, Codex, Aider, or any command) running in an **isolated git
// worktree**, captures the resulting diff, and returns it for review. It never
// merges, commits to, or force-pushes the working branch — applying the diff is
// a separate, operator-gated step (merge/force-push escalation, §4.3). The
// worktree is created off the current HEAD and removed afterward; only the diff
// survives.
//
// The agent command is configured by the operator (AGEZT_CODING_CMD); the task
// is passed in the AGEZT_CODING_TASK environment variable so no shell-quoting of
// model output is needed. Unset command → the tool is not registered.
package coding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agezt/agezt/kernel/agent"
)

// DefaultTimeout caps one delegated coding run.
const DefaultTimeout = 5 * time.Minute

// MaxDiffBytes truncates the returned diff so a huge change set doesn't blow the
// context budget.
const MaxDiffBytes = 60 * 1024

// Tool implements agent.Tool. It is constructed only when an agent command is
// configured; see New.
type Tool struct {
	// Cmd is the shell command that runs the external coding agent. It runs
	// with the worktree as cwd and the task in $AGEZT_CODING_TASK, e.g.
	// `claude -p "$AGEZT_CODING_TASK"` or `aider --yes --message "$AGEZT_CODING_TASK"`.
	Cmd string
	// Repo is the git repository the worktree is branched from (the workspace).
	Repo string
	// Timeout overrides DefaultTimeout when > 0.
	Timeout time.Duration
	// run executes a command; overridable in tests. Defaults to execCommand.
	run func(ctx context.Context, dir string, env []string, name string, args ...string) (string, error)
}

// New builds a coding Tool. cmd is the external agent command; repo is the git
// workspace it operates on. Returns nil when cmd is empty (tool disabled).
func New(cmd, repo string) *Tool {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}
	return &Tool{Cmd: cmd, Repo: repo, run: execCommand}
}

func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "coding",
		Description: "Delegate a focused coding task to an external coding agent running in an " +
			"ISOLATED git worktree, and return the resulting diff for review. It never commits to " +
			"or merges the working branch — you get the proposed patch back; applying it is a " +
			"separate, operator-approved step. Use for self-contained code changes (implement X, " +
			"fix the failing test in Y, refactor Z).",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "task": {
      "type": "string",
      "description": "The complete, self-contained coding instruction. The agent sees only this and the repository contents."
    }
  },
  "required": ["task"]
}`),
		Effect: agent.ToolEffect{
			Class: agent.EffectCompensable,
			PredictedEffects: []string{
				"Spawn an operator-configured external coding agent in an isolated git worktree.",
				"Allow that agent to read repository contents, run commands, and produce a proposed diff.",
			},
			AffectedResources: []string{"temporary git worktree", "external coding-agent process", "repository contents visible to the delegated agent"},
			RollbackNotes:     "The temporary worktree is removed and no patch is applied automatically; compensate leaked or external side effects according to the configured agent's sandbox.",
			Confidence:        0.6,
		},
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
		return agent.Result{Output: "coding agent not configured (set AGEZT_CODING_CMD)", IsError: true}, nil
	}

	to := t.Timeout
	if to <= 0 {
		to = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	// Must be a git repository — that's how we isolate (worktree) and capture
	// (diff) the change without touching the working branch.
	if _, err := t.run(ctx, t.Repo, nil, "git", "rev-parse", "--git-dir"); err != nil {
		return agent.Result{Output: "coding requires a git repository at the workspace (" + t.Repo + "): " + err.Error(), IsError: true}, nil
	}

	// Isolated worktree off the current HEAD (detached, so the working branch
	// is untouched).
	wt, err := os.MkdirTemp("", "agezt-coding-")
	if err != nil {
		return agent.Result{Output: "create worktree dir: " + err.Error(), IsError: true}, nil
	}
	defer os.RemoveAll(wt)
	if out, err := t.run(ctx, t.Repo, nil, "git", "worktree", "add", "--detach", wt, "HEAD"); err != nil {
		return agent.Result{Output: "git worktree add failed: " + err.Error() + "\n" + out, IsError: true}, nil
	}
	// Always detach the worktree from git's metadata, even on error.
	defer func() {
		rmCtx, c := context.WithTimeout(context.Background(), 20*time.Second)
		defer c()
		_, _ = t.run(rmCtx, t.Repo, nil, "git", "worktree", "remove", "--force", wt)
	}()

	// Run the external agent in the worktree with the task in the environment.
	agentEnv := append(os.Environ(), "AGEZT_CODING_TASK="+task)
	shell, shellArg := platformShell()
	agentOut, agentErr := t.run(ctx, wt, agentEnv, shell, shellArg, t.Cmd)

	// Stage everything and diff against HEAD — captures new, modified, and
	// deleted files regardless of whether the agent staged anything. The agent run
	// above may have exhausted ctx's deadline; staging+diffing on the same expired
	// ctx would make exec.CommandContext fail with DeadlineExceeded WITHOUT running
	// git, discarding the partial work a timed-out agent produced. Use a fresh
	// bounded context (like the worktree-cleanup defer) so that work is still
	// captured; the agent's timeout is surfaced via agentErr in renderResult.
	gitCtx, cancelGit := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelGit()
	if out, err := t.run(gitCtx, wt, nil, "git", "add", "-A"); err != nil {
		return agent.Result{Output: "git add failed: " + err.Error() + "\n" + out, IsError: true}, nil
	}
	diff, derr := t.run(gitCtx, wt, nil, "git", "diff", "--cached", "HEAD")
	if derr != nil {
		return agent.Result{Output: "git diff failed: " + derr.Error(), IsError: true}, nil
	}

	return agent.Result{Output: renderResult(diff, agentOut, agentErr)}, nil
}

// renderResult formats the tool output: a short header, the agent's own output
// (truncated), and the captured diff (truncated). A no-op run is reported
// plainly so the model knows nothing changed.
func renderResult(diff, agentOut string, agentErr error) string {
	var b strings.Builder
	if strings.TrimSpace(diff) == "" {
		b.WriteString("The coding agent produced no changes.\n")
	} else {
		b.WriteString("Proposed diff (NOT applied — review and apply separately):\n\n")
		b.WriteString(truncate(diff, MaxDiffBytes))
		b.WriteString("\n")
	}
	if agentErr != nil {
		fmt.Fprintf(&b, "\n[agent exited with error: %v]\n", agentErr)
	}
	if s := strings.TrimSpace(agentOut); s != "" {
		b.WriteString("\n--- agent output ---\n")
		b.WriteString(truncate(s, 8*1024))
	}
	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Back up to a UTF-8 rune boundary so a multi-byte rune straddling the cut
	// (common in non-ASCII diffs) is never split into invalid UTF-8.
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf("\n… [truncated %d bytes]", len(s)-cut)
}

func platformShell() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}

// execCommand runs name+args in dir with the given env (nil = inherit), and
// returns combined stdout+stderr. A non-zero exit is returned as an error with
// the output still captured by the caller via the returned string.
func execCommand(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// AbsRepo returns the absolute, cleaned repo path (helps git resolve worktrees
// consistently). Exposed for the daemon's tool wiring.
func AbsRepo(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
