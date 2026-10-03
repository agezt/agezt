// SPDX-License-Identifier: MIT

package filestore

import (
	"fmt"
	"os"
	"path/filepath"
)

// LockSuffix names the sidecar file Lock holds. The data file itself cannot
// carry the lock: Save replaces it by rename, so a lock on the old inode would
// not exclude a writer that opened the new one.
const LockSuffix = ".lock"

// Lock takes an exclusive lock that guards path across processes AND across
// goroutines of this process, and returns the function that releases it.
// It blocks until the lock is free.
//
// It exists for read-modify-write cycles on files more than one process writes:
// take the lock, re-read the file, apply this writer's changes, Save, release.
// Without it the daemon and `agt` each rewrite the whole file from their own
// in-memory copy, and whichever saves last silently deletes the other's change.
//
// The lock is an OS lock (flock on Unix, LockFileEx on Windows) on a sidecar
// file next to path, so the kernel releases it if the holder dies — a crashed
// `agt` can never wedge the daemon. Both primitives conflict between two opens
// of the same file even inside one process, which is what makes a single
// mechanism cover goroutines too.
func Lock(path string) (unlock func(), err error) {
	// Create a missing directory, but never re-permission an existing one: a
	// locked file may sit directly in the base directory (the vault does), and
	// tightening that is not this call's decision.
	if err := os.MkdirAll(filepath.Dir(path), DirPerm); err != nil {
		return nil, fmt.Errorf("filestore: mkdir %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path+LockSuffix, os.O_RDWR|os.O_CREATE, FilePerm)
	if err != nil {
		return nil, fmt.Errorf("filestore: open lock %s: %w", path+LockSuffix, err)
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("filestore: lock %s: %w", path, err)
	}
	return func() {
		_ = unlockFile(f)
		_ = f.Close()
	}, nil
}
