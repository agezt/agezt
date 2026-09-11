// SPDX-License-Identifier: MIT

package controlplane

// Agent workspace helpers: file counts + root path lookup used by the
// impact aggregation in roster_cascade.go. Carved out of roster.go during
// the Day 24 god file split #9.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/roster"
)

func (s *Server) agentWorkspaceInfo(p roster.Profile) (string, bool) {
	workdir := strings.TrimSpace(p.Workdir)
	if workdir == "" {
		return "", false
	}
	root := s.agentWorkspaceRoot()
	dir, ok := confineUnder(root, workdir)
	if !ok || filepath.Clean(dir) == filepath.Clean(root) {
		return "", false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", false
	}
	files, bytes := countTreeFiles(dir)
	return filepath.ToSlash(workdir) + fmt.Sprintf(" (%d file(s), %d bytes)", files, bytes), true
}

func (s *Server) agentWorkspaceRoot() string {
	if ws := os.Getenv(brand.EnvPrefix + "WORKSPACE"); strings.TrimSpace(ws) != "" {
		return ws
	}
	return filepath.Join(s.k.BaseDir(), "workspace")
}

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

