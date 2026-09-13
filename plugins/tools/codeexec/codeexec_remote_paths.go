// SPDX-License-Identifier: MIT

package codeexec

// Remote work-dir helpers: remoteWorkDir + modalMountDir + k8sWorkDir.
// Carved out of codeexec_remote.go during the Day 185 god-file split
// so the main file can stay focused on the SSH/K8s/Modal invoke
// methods + their runners + the modal artifact-export wrapper.
// Public API unchanged.

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/executionprofile"
)

func remoteWorkDir(cfg executionprofile.SSHConfig, localDir, projectSlug string) string {
	root := strings.Trim(strings.TrimSpace(cfg.WorkDir), "/")
	if root == "" {
		root = ".agezt/code_exec"
	}
	if strings.HasPrefix(strings.TrimSpace(cfg.WorkDir), "/") {
		root = "/" + root
	}
	if projectSlug != "" {
		return path.Join(root, "projects", projectSlug)
	}
	base := filepath.Base(localDir)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		base = fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return path.Join(root, "runs", base)
}

func modalMountDir(localDir string) string {
	base := filepath.Base(localDir)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		base = fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return path.Join("/mnt", base)
}

func k8sWorkDir(cfg executionprofile.K8sConfig, localDir, projectSlug string) string {
	root := strings.Trim(strings.TrimSpace(cfg.WorkDir), "/")
	if root == "" {
		root = ".agezt/code_exec"
	}
	if strings.HasPrefix(strings.TrimSpace(cfg.WorkDir), "/") {
		root = "/" + root
	}
	if projectSlug != "" {
		return path.Join(root, "projects", projectSlug)
	}
	base := filepath.Base(localDir)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		base = fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return path.Join(root, "runs", base)
}

