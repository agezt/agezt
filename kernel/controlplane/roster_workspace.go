// SPDX-License-Identifier: MIT

package controlplane

// The agent workspace helpers' host bindings: the filesystem and the base
// directory behind the application's workspace decisions. The DECISIONS —
// which path is safe to look at, what the label says — live in the
// application layer (`kernel/app/roster/workspace.go`); what stays here is
// the host's data access.

import (
	"os"
	"path/filepath"

	approster "github.com/agezt/agezt/kernel/app/roster"
	core "github.com/agezt/agezt/kernel/roster"
)

// workspacePorts binds the host's filesystem to the application's workspace
// helpers.
func (s *Server) workspacePorts() approster.WorkspacePorts {
	return approster.WorkspacePorts{
		Root: s.agentWorkspaceRoot,
		Stat: func(dir string) (bool, bool, error) {
			info, err := os.Stat(dir)
			if err != nil {
				return false, false, err
			}
			return true, info.IsDir(), nil
		},
		CountTree: countTreeFiles,
	}
}

// agentWorkspaceInfo describes the agent's workspace — the path and what is
// in it — when it has one.
func (s *Server) agentWorkspaceInfo(p core.Profile) (string, bool) {
	return approster.WorkspaceInfo(s.workspacePorts(), p)
}

// agentWorkspaceRoot is the root every agent workspace lives under.
func (s *Server) agentWorkspaceRoot() string {
	return approster.WorkspaceRoot(s.k.BaseDir())
}

// countTreeFiles counts the files and the bytes under a directory.
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
