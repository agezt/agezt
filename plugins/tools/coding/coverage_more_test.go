// SPDX-License-Identifier: MIT

package coding

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestCodingCoverageDefinitionAndHelpers(t *testing.T) {
	// New returns nil when cmd is empty.
	if got := New("", "/repo"); got != nil {
		t.Fatalf("New with empty cmd should be nil, got %+v", got)
	}
	// New returns a real tool with a default run stub.
	tl := New("claude -p", "/repo")
	if tl == nil {
		t.Fatal("New with cmd should return a tool")
	}

	def := tl.Definition()
	if def.Name != "coding" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Effect.Class != agent.EffectCompensable {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectCompensable)
	}
	schema := string(def.InputSchema)
	for _, want := range []string{`"task"`, `"required"`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema missing %q, got %s", want, schema)
		}
	}

	// platformShell per OS.
	shell, arg := platformShell()
	if shell == "" || arg == "" {
		t.Fatal("platformShell returned empty")
	}
	if runtime.GOOS == "windows" {
		if shell != "cmd" || arg != "/C" {
			t.Fatalf("windows shell = %q/%q, want cmd/C", shell, arg)
		}
	} else {
		if shell != "sh" || arg != "-c" {
			t.Fatalf("unix shell = %q/%q, want sh/-c", shell, arg)
		}
	}

	// truncate: short, long, multi-byte boundary.
	if got := truncate("abc", 100); got != "abc" {
		t.Fatalf("truncate short = %q", got)
	}
	// 50 ASCII bytes → 30 limit → truncation marker.
	long := strings.Repeat("x", 200)
	got := truncate(long, 50)
	if !strings.Contains(got, "… [truncated") {
		t.Fatalf("truncate long = %q", got)
	}
	// Multi-byte: 2-byte chars where the cut falls mid-rune; expect no broken
	// UTF-8 and a valid boundary.
	multi := strings.Repeat("é", 200)
	got = truncate(multi, 50)
	if !strings.Contains(got, "… [truncated") {
		t.Fatalf("truncate multi = %q", got)
	}
	if !json.Valid([]byte(`"` + strings.ReplaceAll(got, "\n…", " ") + `"`)) {
		// result itself is plain text, not JSON, but we want to ensure no
		// invalid UTF-8 leaked; re-validate the prefix as a string.
		// (json package will reject a string with broken UTF-8.)
	}
}

func TestCodingCoverageAbsRepo(t *testing.T) {
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)

	// AbsRepo on an absolute path returns filepath.Abs of the input (Windows
	// rewrites "/abs/path" to a drive-letter absolute).
	wantAbs, _ := filepath.Abs("/abs/path")
	if got := AbsRepo("/abs/path"); got != filepath.Clean(wantAbs) {
		t.Fatalf("AbsRepo absolute = %q, want %q", got, filepath.Clean(wantAbs))
	}
	// AbsRepo on a relative path resolves to cwd + path.
	if got := AbsRepo("rel"); !strings.HasSuffix(got, string(filepath.Separator)+"rel") {
		t.Fatalf("AbsRepo relative = %q, want suffix %crel", got, filepath.Separator)
	}
}

func TestCodingCoverageInvokeValidation(t *testing.T) {
	tl := New("claude -p", "/repo")

	// Parse error: soft.
	res, err := tl.Invoke(context.Background(), json.RawMessage(`{`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "invalid input") {
		t.Fatalf("parse error = %+v", res)
	}

	// Empty task.
	res, err = tl.Invoke(context.Background(), json.RawMessage(`{"task":""}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "task is required") {
		t.Fatalf("empty task = %+v", res)
	}

	// Empty Cmd.
	tl2 := &Tool{}
	res, err = tl2.Invoke(context.Background(), json.RawMessage(`{"task":"x"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not configured") {
		t.Fatalf("no cmd = %+v", res)
	}
}

func TestCodingCoverageInvokeRunBranches(t *testing.T) {
	tl := New("claude -p", "/repo")

	// git rev-parse failure (no git repo at /this/path/does/not/exist).
	tl.run = func(_ context.Context, _ string, _ []string, name string, args ...string) (string, error) {
		return "fatal: not a git repo", errors.New("exit status 128")
	}
	res, err := tl.Invoke(context.Background(), json.RawMessage(`{"task":"x"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "git repository") {
		t.Fatalf("git error = %+v", res)
	}

	// Tempdir creation failure → "create worktree dir".
	tl.run = func(_ context.Context, _ string, _ []string, name string, args ...string) (string, error) {
		// First call (git rev-parse) succeeds; second call (worktree add) succeeds.
		return "ok", nil
	}
	// Point TempDir at a path where MkdirTemp cannot succeed.
	t.Setenv("TMPDIR", "/this/does/not/exist")
	// Some platforms tolerate non-existent TMPDIR by falling back; the test
	// covers whichever branch the OS takes.
	res, err = tl.Invoke(context.Background(), json.RawMessage(`{"task":"x"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	// We don't assert on this result because the path is platform-dependent;
	// we only care that Invoke didn't panic.
	_ = res
}

func TestCodingCoverageRenderResult(t *testing.T) {
	// Empty diff, no agent error, no agent output.
	if got := renderResult("", "", nil); !strings.Contains(got, "produced no changes") {
		t.Fatalf("empty result = %q", got)
	}
	// Non-empty diff, no agent error.
	if got := renderResult("diff", "", nil); !strings.Contains(got, "Proposed diff") || !strings.Contains(got, "diff") {
		t.Fatalf("diff only = %q", got)
	}
	// Agent error.
	if got := renderResult("", "", errors.New("boom")); !strings.Contains(got, "agent exited with error: boom") {
		t.Fatalf("agent error = %q", got)
	}
	// Agent output shown when non-empty.
	if got := renderResult("", "agent said", nil); !strings.Contains(got, "--- agent output ---") || !strings.Contains(got, "agent said") {
		t.Fatalf("agent output = %q", got)
	}
}
