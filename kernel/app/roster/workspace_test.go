// SPDX-License-Identifier: MIT

package roster

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	core "github.com/agezt/agezt/kernel/modules/agents/roster"
)

func TestWorkspaceInfo(t *testing.T) {
	root := t.TempDir()
	ports := WorkspacePorts{
		Root:      func() string { return root },
		Stat:      func(dir string) (bool, bool, error) { return true, true, nil },
		CountTree: func(dir string) (int, int64) { return 3, 42 },
	}
	info, ok := WorkspaceInfo(ports, core.Profile{Slug: "ops", Workdir: "ops"})
	if !ok {
		t.Fatal("no workspace info")
	}
	if info != "ops (3 file(s), 42 bytes)" {
		t.Errorf("info = %q, want the label with the file count and the bytes", info)
	}
}

// A profile without a workdir has no workspace.
func TestWorkspaceInfo_NoWorkdir(t *testing.T) {
	ports := WorkspacePorts{
		Root:      func() string { return t.TempDir() },
		Stat:      func(dir string) (bool, bool, error) { return true, true, nil },
		CountTree: func(dir string) (int, int64) { return 0, 0 },
	}
	if _, ok := WorkspaceInfo(ports, core.Profile{Slug: "ops"}); ok {
		t.Error("a profile without a workdir has no workspace")
	}
}

// A path outside the workspace root is refused.
func TestWorkspaceInfo_RefusesOutsidePaths(t *testing.T) {
	root := t.TempDir()
	ports := WorkspacePorts{
		Root:      func() string { return root },
		Stat:      func(dir string) (bool, bool, error) { return true, true, nil },
		CountTree: func(dir string) (int, int64) { return 0, 0 },
	}
	if _, ok := WorkspaceInfo(ports, core.Profile{Workdir: filepath.Join("..", "elsewhere")}); ok {
		t.Error("a path outside the root has no workspace")
	}
}

// The workspace root itself is not an agent's workspace.
func TestWorkspaceInfo_RefusesTheRoot(t *testing.T) {
	root := t.TempDir()
	ports := WorkspacePorts{
		Root:      func() string { return root },
		Stat:      func(dir string) (bool, bool, error) { return true, true, nil },
		CountTree: func(dir string) (int, int64) { return 0, 0 },
	}
	if _, ok := WorkspaceInfo(ports, core.Profile{Workdir: "."}); ok {
		t.Error("the root is not an agent's workspace")
	}
}

// A path that is not a directory has no workspace.
func TestWorkspaceInfo_RefusesANonDirectory(t *testing.T) {
	root := t.TempDir()
	ports := WorkspacePorts{
		Root:      func() string { return root },
		Stat:      func(dir string) (bool, bool, error) { return true, false, nil },
		CountTree: func(dir string) (int, int64) { return 0, 0 },
	}
	if _, ok := WorkspaceInfo(ports, core.Profile{Workdir: "ops"}); ok {
		t.Error("a file is not a workspace")
	}
}

// A path that is not there has no workspace.
func TestWorkspaceInfo_MissingPath(t *testing.T) {
	root := t.TempDir()
	ports := WorkspacePorts{
		Root:      func() string { return root },
		Stat:      func(dir string) (bool, bool, error) { return false, false, nil },
		CountTree: func(dir string) (int, int64) { return 0, 0 },
	}
	if _, ok := WorkspaceInfo(ports, core.Profile{Workdir: "ops"}); ok {
		t.Error("a missing path has no workspace")
	}
}

func TestWorkspaceRoot(t *testing.T) {
	key := brand.EnvPrefix + "WORKSPACE"
	t.Setenv(key, "")
	base := t.TempDir()
	if got := WorkspaceRoot(base); got != filepath.Join(base, "workspace") {
		t.Errorf("root = %q, want the base directory's workspace", got)
	}
	t.Setenv(key, "/custom/root")
	if got := WorkspaceRoot(base); got != "/custom/root" {
		t.Errorf("root = %q, want the environment override", got)
	}
	t.Setenv(key, "   ")
	if got := WorkspaceRoot(base); got != filepath.Join(base, "workspace") {
		t.Errorf("root = %q, want the default for a blank override", got)
	}
}

// The host's file counter is exercised against a real tree, because the label
// it produces is what the impact preview shows.
func TestWorkspaceInfo_RealTree(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "ops")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ports := WorkspacePorts{
		Root: func() string { return root },
		Stat: func(dir string) (bool, bool, error) {
			info, err := os.Stat(dir)
			if err != nil {
				return false, false, err
			}
			return true, info.IsDir(), nil
		},
		CountTree: func(dir string) (int, int64) { return countTreeFiles(dir) },
	}
	info, ok := WorkspaceInfo(ports, core.Profile{Workdir: "ops"})
	if !ok {
		t.Fatal("no workspace info")
	}
	if info != "ops (1 file(s), 5 bytes)" {
		t.Errorf("info = %q, want the file count and the bytes", info)
	}
}

// countTreeFiles is the host's counter; the app calls it through the port.
func countTreeFiles(root string) (int, int64) {
	var files int
	var bytes int64
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, bytes
}
