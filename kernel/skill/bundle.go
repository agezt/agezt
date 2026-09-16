// SPDX-License-Identifier: MIT
//
// kernel/skill BundleStore type + OpenBundles constructor.
// Extracted from bundle.go during Day 211 god-file refactor (#92).
// Public API unchanged.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
)

// BundleStore holds the on-disk resources that travel with a skill — the
// agentskills.io / ClawHub bundle shape (SPEC-13). A skill is no longer only an
// inline body: it can ship REFERENCE files (extra docs the body points at) and
// SCRIPTS (helpers the agent runs — `scripts/setup.sh` to install a CLI, a
// Python tool, etc.). The SKILL.md body stays small and is injected into
// context; the heavier resources live here and are disclosed progressively —
// the agent lists them (op=files), reads one when relevant (op=read), and runs
// scripts with its existing shell/code_exec tools against the bundle Dir.
//
// Layout: <skillsDir>/bundles/<slug>/<relative-path...>. Bundles are keyed by
// the skill's slug (normalized name), not its content-id, because resources are
// stable across body edits (a new version of "pdf-fill" keeps the same scripts).
//
// A BundleStore is safe for concurrent use: each operation is a self-contained
// filesystem call and writes go through a temp-dir swap (see Write).
type BundleStore struct {
	root string // <skillsDir>/bundles
}

// OpenBundles opens (or creates) the bundle store rooted at <dir>/bundles.
func OpenBundles(dir string) (*BundleStore, error) {
	root := filepath.Join(dir, "bundles")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("skill: mkdir bundles %s: %w", root, err)
	}
	return &BundleStore{root: root}, nil
}

// MaxBundleFile caps a single resource file; MaxBundleTotal caps a whole bundle.
// Bundles are text procedures and helper scripts, not data blobs — generous
// limits that still stop an accidental gigabyte from landing in the skill store.
const (
	MaxBundleFile  = 1 << 20       // 1 MiB per file
	MaxBundleTotal = 8 * (1 << 20) // 8 MiB per bundle
)
