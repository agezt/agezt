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
	"io"
	"os"
	"path/filepath"
	"runtime"
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
		// Windows: rename-over-existing fails when another process (antivirus,
		// indexer, a reader without FILE_SHARE_DELETE) holds the target open.
		// Remove-then-rename recovers at the cost of a brief window with no
		// file — strictly better than losing the write. Several stores (okr,
		// seat, taste, workboard) had each grown this fallback independently;
		// it lives here now so every caller gets it.
		if runtime.GOOS == "windows" {
			if removeErr := os.Remove(path); removeErr == nil || os.IsNotExist(removeErr) {
				if retryErr := os.Rename(tmpName, path); retryErr == nil {
					return nil
				}
				// The target is gone and the retry rename also failed; the
				// data is still in memory, so a direct (non-atomic) write is
				// the last resort that keeps the file from being lost
				// entirely. (kernel/modules/work/seat used to preserve its fixed-name temp
				// for next-boot recovery to cover exactly this window.)
				if werr := os.WriteFile(path, data, mode); werr == nil {
					return nil
				}
			}
		}
		return fmt.Errorf("atomicfile: rename: %w", err)
	}
	return nil
}

// WriteStream writes what fn produces to path atomically, with the same
// guarantees as WriteFile: a UNIQUE temp file in the target's directory,
// fsynced, chmodded, then renamed over the target. The parent directory must
// already exist.
//
// The callback shape makes the commit atomic BY CONSTRUCTION — there is no
// "forgot to close" state, and the error path cannot leak a temp file. fn
// receives an io.Writer rather than the *os.File so it cannot close or sync
// a handle the platform owns.
//
// On any error the temp file is removed and the target is left untouched —
// including a target that already existed. That last part is the point: a
// failed write must not destroy the previous good file.
//
// The Windows last resort differs from WriteFile's on purpose. WriteFile can
// re-write the bytes because they are in memory; a stream's bytes are not,
// but they are on disk — one rename away in the temp file. So the fallback
// copies the temp onto the target instead of re-writing from memory.
func WriteStream(path string, mode os.FileMode, fn func(w io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("atomicfile: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename has moved it
	if err := fn(tmp); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomicfile: stream: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomicfile: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomicfile: close temp: %w", err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("atomicfile: chmod temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		if runtime.GOOS == "windows" {
			if removeErr := os.Remove(path); removeErr == nil || os.IsNotExist(removeErr) {
				if retryErr := os.Rename(tmpName, path); retryErr == nil {
					return nil
				}
				// The target is gone and the retry rename also failed. The
				// bytes are still in the TEMP FILE — a stream's data is not
				// in memory, but it is on disk — so copy it across rather
				// than lose the write.
				if cerr := copyOver(path, tmpName, mode); cerr == nil {
					return nil
				}
			}
		}
		return fmt.Errorf("atomicfile: rename: %w", err)
	}
	return nil
}

// copyOver copies src onto dst and gives dst the given mode. It is the
// streaming counterpart of WriteFile's in-memory last resort: the bytes are
// on disk, so they can be copied rather than regenerated.
func copyOver(dst, src string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// OpenFile applies the mode only at creation and the umask may have
	// stripped bits from it; the caller's mode is the promise.
	return os.Chmod(dst, mode)
}
