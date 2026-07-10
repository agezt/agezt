// SPDX-License-Identifier: MIT

// Package atomicfile writes files crash- and concurrency-safely. Data goes to
// a UNIQUE temp file in the target's directory, is fsynced, chmodded, then
// renamed over the target — atomic on POSIX and on Windows (MoveFileEx with
// replace-existing). A crash mid-write leaves the previous file intact, an
// unsynced temp can never be renamed into place, and two concurrent writers
// to the same target never race on a shared "<path>.tmp" name (M478).
//
// This is the one canonical implementation; the per-package atomicWrite
// helpers in kernel/* and plugins/* are thin wrappers around it.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile writes data to path atomically with the given mode. The parent
// directory must already exist. On any error the temp file is removed and the
// target is left untouched.
func WriteFile(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("atomicfile: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename has moved it
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomicfile: write temp: %w", err)
	}
	// Flush to disk before rename: without it a crash right after the rename
	// can leave a zero-length/stale file on some filesystems.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomicfile: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomicfile: close temp: %w", err)
	}
	// CreateTemp opens 0600; widen/narrow to the caller's mode before the
	// rename so the target never exists with the wrong permissions.
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("atomicfile: chmod temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("atomicfile: rename: %w", err)
	}
	return nil
}
