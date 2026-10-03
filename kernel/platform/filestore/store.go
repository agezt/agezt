// SPDX-License-Identifier: MIT

// Package filestore is the persistence platform for the daemon's single-file JSON
// stores (architecture/20-target-architecture.md §5, layer L2): a tolerant Load,
// an atomic Save, and a cross-process Lock for files that more than one process
// writes (the vault and settings are written by both the daemon and `agt`).
//
// It owns two invariants every store used to decide for itself:
//
//   - Privacy. Store files hold memory, agent profiles, plans and schedules —
//     the same material the journal records — so they get the journal's posture:
//     files 0600, directories 0700, and existing installs are tightened in place.
//   - Atomicity. Save goes through internal/atomicfile (unique temp + fsync +
//     rename, with the Windows remove-retry), so a crash never leaves half a file.
//
// Deliberately NOT a generic Store[T] owning the mutex: every store's value is in
// its domain methods and locking discipline, which stay local. This package owns
// the file IO and the cross-process coordination, nothing else.
package filestore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agezt/agezt/internal/atomicfile"
)

const (
	// FilePerm is the mode every store file is written with.
	FilePerm os.FileMode = 0o600
	// DirPerm is the mode store directories are created with, and tightened to.
	DirPerm os.FileMode = 0o700
)

// Load reads the JSON file at path into out (a pointer, as for
// json.Unmarshal). A missing, empty, or whitespace-only file leaves out
// untouched and returns nil — first boot is not an error. A UTF-8 BOM is
// tolerated (files hand-edited on Windows often carry one). Any other read
// or parse failure is returned wrapped with the path, so boot errors name
// the file at fault.
func Load(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// LoadFrom is Load for the common Open(dir) shape: it ensures dir exists
// (EnsureDir), then loads dir/name into out and returns the joined path for
// the store to keep. The directory is created even when the file is absent so
// the first Save has somewhere to land.
func LoadFrom(dir, name string, out any) (string, error) {
	if err := EnsureDir(dir); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := Load(path, out); err != nil {
		return "", err
	}
	return path, nil
}

// Save writes v as indented JSON to path via atomicfile with FilePerm. The
// rename replaces the file, so an existing 0644 file becomes 0600 on its first
// save. Callers hold their own lock around Save; the store's mutex discipline
// stays in the store.
func Save(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(path, b, FilePerm)
}

// EnsureDir creates dir with DirPerm and tightens an existing one to it.
//
// MkdirAll is a no-op on an existing directory, so without the chmod an
// install created before this package existed would keep its 0755 forever.
// The chmod is best-effort on purpose, exactly as for the journal: refusing to
// open a store — and so failing daemon boot — on a filesystem where chmod
// cannot succeed (some network mounts, some container ownership setups) would
// trade one hardening measure for the whole daemon.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, DirPerm); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	_ = os.Chmod(dir, DirPerm)
	return nil
}
