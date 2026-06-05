// SPDX-License-Identifier: MIT

package coding

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate_RuneSafeAtByteBoundary(t *testing.T) {
	// Pad so that byte index `max` lands in the MIDDLE of a 2-byte rune: (max-1)
	// ASCII bytes, then "ş" (U+015F, bytes C5 9F) — byte `max` is the 0x9F
	// continuation byte. A raw s[:max] slice would leave a lone C5 (invalid UTF-8).
	const max = 16
	in := strings.Repeat("a", max-1) + "ş" + strings.Repeat("b", 10)
	got := truncate(in, max)
	if !utf8.ValidString(got) {
		t.Fatalf("truncate produced invalid UTF-8 at a rune boundary: %q", got)
	}
	// The straddling rune is dropped whole; the kept prefix is the (max-1) a's.
	if !strings.HasPrefix(got, strings.Repeat("a", max-1)+"\n… [truncated") {
		t.Errorf("unexpected truncation result: %q", got)
	}
	if strings.ContainsRune(got, '�') {
		t.Errorf("truncated output contains the replacement char (split rune): %q", got)
	}
}

type call struct {
	name string
	args []string
	env  []string
}

// fakeTool builds a Tool whose run records calls and returns scripted outputs
// keyed by the first arg (or the command name for the shell).
func fakeTool(t *testing.T, diff string, agentErr error) (*Tool, *[]call) {
	t.Helper()
	var calls []call
	tool := &Tool{Cmd: `echo hi`, Repo: "/repo"}
	tool.run = func(_ context.Context, _ string, env []string, name string, args ...string) (string, error) {
		calls = append(calls, call{name: name, args: args, env: env})
		switch {
		case name == "git" && len(args) > 0 && args[0] == "rev-parse":
			return ".git", nil
		case name == "git" && len(args) > 0 && args[0] == "worktree" && args[1] == "add":
			return "", nil
		case name == "git" && len(args) > 0 && args[0] == "add":
			return "", nil
		case name == "git" && len(args) > 0 && args[0] == "diff":
			return diff, nil
		case name == "git" && len(args) > 0 && args[0] == "worktree" && args[1] == "remove":
			return "", nil
		default: // the agent command (shell)
			return "agent output here", agentErr
		}
	}
	return tool, &calls
}

func invoke(t *testing.T, tool *Tool, task string) string {
	t.Helper()
	in, _ := json.Marshal(map[string]string{"task": task})
	res, err := tool.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	return res.Output
}

func TestCoding_HappyPath_ReturnsDiff(t *testing.T) {
	tool, calls := fakeTool(t, "diff --git a/new.txt b/new.txt\n+hello\n", nil)
	out := invoke(t, tool, "add a greeting file")

	if !strings.Contains(out, "diff --git a/new.txt") {
		t.Errorf("output missing diff:\n%s", out)
	}
	if !strings.Contains(out, "NOT applied") {
		t.Error("output should make clear the diff is not applied")
	}
	// The task reached the agent via AGEZT_CODING_TASK.
	var sawTaskEnv bool
	for _, c := range *calls {
		for _, e := range c.env {
			if e == "AGEZT_CODING_TASK=add a greeting file" {
				sawTaskEnv = true
			}
		}
	}
	if !sawTaskEnv {
		t.Error("task should be passed to the agent in AGEZT_CODING_TASK")
	}
	// A worktree was created and removed (isolation + cleanup).
	var added, removed bool
	for _, c := range *calls {
		if c.name == "git" && len(c.args) >= 2 && c.args[0] == "worktree" && c.args[1] == "add" {
			added = true
		}
		if c.name == "git" && len(c.args) >= 2 && c.args[0] == "worktree" && c.args[1] == "remove" {
			removed = true
		}
	}
	if !added || !removed {
		t.Errorf("worktree lifecycle: added=%v removed=%v (both want true)", added, removed)
	}
	// It must NEVER commit/merge/push.
	for _, c := range *calls {
		if c.name == "git" && len(c.args) > 0 {
			switch c.args[0] {
			case "commit", "merge", "push":
				t.Errorf("coding tool must not run git %s", c.args[0])
			}
		}
	}
}

// TestCoding_PostAgentGitUsesFreshContext pins M469: when the agent run exhausts
// the request deadline, the post-agent `git add`/`git diff` must still run (on a
// fresh context) so the agent's partial work is captured, not discarded with a
// "context deadline exceeded" error.
func TestCoding_PostAgentGitUsesFreshContext(t *testing.T) {
	var sawAdd, sawDiff bool
	var addCtxErr, diffCtxErr error
	tool := &Tool{Cmd: "echo hi", Repo: "/repo"}
	tool.run = func(ctx context.Context, _ string, _ []string, name string, args ...string) (string, error) {
		switch {
		case name == "git" && args[0] == "rev-parse":
			return ".git", nil
		case name == "git" && args[0] == "worktree" && args[1] == "add":
			return "", nil
		case name == "git" && args[0] == "add":
			sawAdd, addCtxErr = true, ctx.Err()
			return "", nil
		case name == "git" && args[0] == "diff":
			sawDiff, diffCtxErr = true, ctx.Err()
			return "diff --git a/x b/x\n", nil
		case name == "git" && args[0] == "worktree" && args[1] == "remove":
			return "", nil
		default:
			return "agent output", nil
		}
	}

	// An already-cancelled request context simulates an agent that ran out the
	// deadline.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in, _ := json.Marshal(map[string]string{"task": "x"})
	res, err := tool.Invoke(ctx, in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !sawAdd || !sawDiff {
		t.Fatalf("git add/diff not reached: add=%v diff=%v (output=%q)", sawAdd, sawDiff, res.Output)
	}
	if addCtxErr != nil {
		t.Errorf("git add ran on an expired context (%v): a timed-out agent's partial work is discarded", addCtxErr)
	}
	if diffCtxErr != nil {
		t.Errorf("git diff ran on an expired context (%v)", diffCtxErr)
	}
}

func TestCoding_NoChanges(t *testing.T) {
	tool, _ := fakeTool(t, "", nil) // empty diff
	out := invoke(t, tool, "do nothing")
	if !strings.Contains(out, "no changes") {
		t.Errorf("empty diff should report no changes, got:\n%s", out)
	}
}

func TestCoding_AgentErrorStillReturnsDiff(t *testing.T) {
	tool, _ := fakeTool(t, "diff --git a/x b/x\n", errors.New("exit status 1"))
	out := invoke(t, tool, "try something")
	if !strings.Contains(out, "diff --git") {
		t.Error("a partial diff should still be returned when the agent errors")
	}
	if !strings.Contains(out, "agent exited with error") {
		t.Error("the agent error should be surfaced")
	}
}

func TestCoding_RequiresGitRepo(t *testing.T) {
	tool := &Tool{Cmd: "echo hi", Repo: "/notarepo"}
	tool.run = func(_ context.Context, _ string, _ []string, name string, args ...string) (string, error) {
		if name == "git" && len(args) > 0 && args[0] == "rev-parse" {
			return "", errors.New("not a git repository")
		}
		return "", nil
	}
	out := invoke(t, tool, "task")
	if !strings.Contains(out, "requires a git repository") {
		t.Errorf("non-repo should error clearly, got:\n%s", out)
	}
}

func TestCoding_EmptyTask(t *testing.T) {
	tool, _ := fakeTool(t, "x", nil)
	out := invoke(t, tool, "  ")
	if !strings.Contains(out, "task is required") {
		t.Errorf("empty task should error, got:\n%s", out)
	}
}

func TestNew_DisabledWhenNoCmd(t *testing.T) {
	if New("", "/repo") != nil {
		t.Error("New with empty cmd should return nil (tool disabled)")
	}
	if New("  ", "/repo") != nil {
		t.Error("New with blank cmd should return nil")
	}
	if New("claude -p x", "/repo") == nil {
		t.Error("New with a cmd should return a tool")
	}
}
