// SPDX-License-Identifier: MIT

// Package resume: small I/O helpers (Dir + safeName + path + writeAtomic).
// Split from resume.go during Day 211 god-file refactor (#48).
// Public API unchanged.
package resume

import (
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/atomicfile"
)

func (s *Store) Dir() string { return s.dir }

func safeName(corr string) string {
	// Correlation ids are "run-<ulid>" — already filename-safe — but strip any
	// path syntax defensively so a crafted corr can't escape the directory.
	r := strings.NewReplacer("/", "_", "\\", "_", "..", "_", ":", "_")
	return r.Replace(corr)
}

func (s *Store) path(corr string) string {
	return filepath.Join(s.dir, safeName(corr)+".json")
}

// Put writes (or overwrites) a ticket. It stamps UpdatedAt (and CreatedAt on
// first write) and enforces the size cap: an oversized ticket has its message
// snapshot dropped (SnapshotDropped set) so files stay bounded.
func writeAtomic(path string, b []byte) error {
	return atomicfile.WriteFile(path, b, 0o600)
}
