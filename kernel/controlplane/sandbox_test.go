// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// seedProject writes a code_exec-style project under <baseDir>/sandbox/projects.
func seedProject(t *testing.T, baseDir, name string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(baseDir, "sandbox", "projects", name)
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

func TestSandboxList_EnumeratesProjectsAndFiles(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	seedProject(t, k.BaseDir(), "calc", map[string]string{
		"main.py": "from add import add\nprint(add(2,3))\n",
		"add.py":  "def add(a,b): return a+b\n",
	})

	res, err := c.Call(context.Background(), controlplane.CmdSandboxList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if cnt := intOf(res["count"]); cnt != 1 {
		t.Fatalf("count = %d, want 1", cnt)
	}
	projects, _ := res["projects"].([]any)
	if len(projects) != 1 {
		t.Fatalf("projects len = %d, want 1", len(projects))
	}
	p, _ := projects[0].(map[string]any)
	if p["name"] != "calc" {
		t.Errorf("project name = %v, want calc", p["name"])
	}
	if fc := intOf(p["file_count"]); fc != 2 {
		t.Errorf("file_count = %d, want 2", fc)
	}
	if tb := intOf(p["total_bytes"]); tb <= 0 {
		t.Errorf("total_bytes = %d, want > 0", tb)
	}
	files, _ := p["files"].([]any)
	if len(files) != 2 {
		t.Errorf("files len = %d, want 2", len(files))
	}
}

func TestSandboxList_EmptyWhenNoProjects(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdSandboxList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if cnt := intOf(res["count"]); cnt != 0 {
		t.Errorf("count = %d, want 0 on a fresh daemon", cnt)
	}
}

func TestSandboxFile_ReadsContent(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	seedProject(t, k.BaseDir(), "calc", map[string]string{"add.py": "def add(a,b): return a+b\n"})

	res, err := c.Call(context.Background(), controlplane.CmdSandboxFile, map[string]any{"project": "calc", "file": "add.py"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := res["content"]; got != "def add(a,b): return a+b\n" {
		t.Errorf("content = %q", got)
	}
}

func TestSandboxFile_RejectsTraversal(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	seedProject(t, k.BaseDir(), "calc", map[string]string{"add.py": "x"})
	// A secret the traversal must NOT be able to reach.
	secret := filepath.Join(k.BaseDir(), "creds.json")
	_ = os.WriteFile(secret, []byte("TOP-SECRET"), 0o600)

	for _, tc := range []struct{ project, file string }{
		{"calc", "../../creds.json"},
		{"calc", "../../../creds.json"},
		{"..", "creds.json"},
		{"calc", "/etc/passwd"},
	} {
		_, err := c.Call(context.Background(), controlplane.CmdSandboxFile, map[string]any{"project": tc.project, "file": tc.file})
		if err == nil {
			t.Errorf("traversal project=%q file=%q should be rejected, got no error", tc.project, tc.file)
		}
	}
}

func TestSandboxFile_RequiresArgs(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	if _, err := c.Call(context.Background(), controlplane.CmdSandboxFile, map[string]any{"project": "calc"}); err == nil {
		t.Error("missing file arg should error")
	}
}
