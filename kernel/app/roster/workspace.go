// SPDX-License-Identifier: MIT

package roster

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	core "github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/platform/fileworkspace"
)

// WorkspacePorts are the host's capabilities the workspace helpers need. The
// application owns the DECISIONS (which path is safe, what the label says);
// the host owns the DATA (the filesystem).
type WorkspacePorts struct {
	// Root is the root every agent workspace lives under.
	Root func() string
	// Stat reports whether the directory exists and whether it is a directory.
	Stat func(dir string) (exists bool, isDir bool, err error)
	// CountTree counts the files and the bytes under a directory.
	CountTree func(dir string) (files int, bytes int64)
}

// WorkspaceInfo describes the agent's workspace — the path and what is in it
// — when the agent has one and the path is safe to look at. A path outside
// the workspace root, the root itself and a path that is not a directory all
// have no description.
func WorkspaceInfo(ports WorkspacePorts, p core.Profile) (string, bool) {
	workdir := strings.TrimSpace(p.Workdir)
	if workdir == "" {
		return "", false
	}
	root := ports.Root()
	dir, ok := fileworkspace.ConfineUnder(root, workdir)
	if !ok || filepath.Clean(dir) == filepath.Clean(root) {
		return "", false
	}
	exists, isDir, err := ports.Stat(dir)
	if err != nil || !exists || !isDir {
		return "", false
	}
	files, bytes := ports.CountTree(dir)
	return filepath.ToSlash(workdir) + fmt.Sprintf(" (%d file(s), %d bytes)", files, bytes), true
}

// WorkspaceRoot resolves the root every agent workspace lives under: the
// AGEZT_WORKSPACE override when it is set, and the base directory's
// "workspace" folder otherwise.
func WorkspaceRoot(baseDir string) string {
	if ws := os.Getenv(brand.EnvPrefix + "WORKSPACE"); strings.TrimSpace(ws) != "" {
		return ws
	}
	return filepath.Join(baseDir, "workspace")
}
