package coding

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoding_LiveGitWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = repo
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0o644)
	run("add", "-A")
	run("commit", "-m", "init")

	// Stub "coding agent": append a new file. Works in both sh -c and cmd /C.
	tool := New("echo CODING_MARKER>> created_by_agent.txt", repo)
	in, _ := json.Marshal(map[string]string{"task": "create a file"})
	res, err := tool.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(res.Output, "created_by_agent.txt") {
		t.Fatalf("diff should mention the new file:\n%s", res.Output)
	}
	// The working repo must be unchanged (only the worktree saw the file).
	if _, err := os.Stat(filepath.Join(repo, "created_by_agent.txt")); !os.IsNotExist(err) {
		t.Error("the new file must NOT appear in the working repo (worktree isolation)")
	}
	t.Logf("captured diff:\n%s", res.Output)
}
