// SPDX-License-Identifier: MIT

// Package dirswap replaces a DIRECTORY with a staged one, and never leaves
// the destination missing.
//
// A tree cannot be renamed over another tree: on POSIX a rename onto a
// non-empty directory fails, and on Windows a rename over an existing path is
// refused outright. The usual workaround — delete the destination, then
// rename the new one into place — has two failure modes, and both of them
// lose data:
//
//   - the delete succeeds and the rename then fails: the destination is GONE
//     and the new tree never arrived. The caller is left with nothing, and
//     the old content is not recoverable — it was deleted, not moved.
//   - the delete fails halfway: a half-deleted tree is left behind, which
//     looks like a valid tree to a reader that only checks for the files it
//     wants.
//
// The policy this package implements, in one place:
//
//  1. The new tree is written into a STAGING sibling. Every file goes through
//     the platform's atomic writer (unique temp + fsync + rename), so a
//     reader never sees a half-written file and the file's own content is
//     durable before the tree is published.
//  2. The old tree is MOVED ASIDE (renamed), never deleted first. It is still
//     there, under another name, for the whole swap.
//  3. The staged tree is renamed into place.
//  4. Only then is the old tree removed — best effort, with the Windows
//     "another handle still holds it" retry.
//
// If step 3 fails, the old tree is moved BACK (the rollback) and the error is
// returned: the destination still holds exactly what it held before, and the
// staged tree is left alone for the caller to inspect.
//
// The leftover of a failed cleanup is bounded: the old tree always sits at
// ONE fixed path next to the destination, and the next swap removes it first,
// so a destination can never accumulate more than one.
package dirswap

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/atomicfile"
)

const (
	// StagingSuffix names the sibling a new tree is written into before it
	// is published.
	StagingSuffix = ".staging"

	// AsideSuffix names the sibling the previous tree is moved to while the
	// new one is renamed into place.
	AsideSuffix = ".old"
)

// IsTransient reports whether a directory name is one of this package's
// working directories (a staging tree or a moved-aside tree) rather than a
// published one.
//
// A reader that lists a directory has to skip these: a leftover from an
// interrupted swap is not a published tree, and publishing it would offer the
// reader a stale copy of something that was replaced.
func IsTransient(name string) bool {
	return strings.HasSuffix(name, StagingSuffix) || strings.HasSuffix(name, AsideSuffix)
}

// StagingPath is the path of the staging sibling of dst.
func StagingPath(dst string) string { return dst + StagingSuffix }

// AsidePath is the path the previous tree is moved to while dst's new tree
// is renamed into place.
func AsidePath(dst string) string { return dst + AsideSuffix }

// Stage prepares the staging sibling of dst: a leftover from an earlier
// attempt is removed and the directory is created.
//
// The staging tree is a SIBLING of the destination on purpose: a rename
// across directories is still a rename, but a sibling keeps it on the same
// filesystem, which is what makes the rename atomic at all.
func Stage(dst string) (string, error) {
	staging := StagingPath(dst)
	if err := os.RemoveAll(staging); err != nil {
		return "", fmt.Errorf("dirswap: clear the staging tree at %q: %w", staging, err)
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return "", fmt.Errorf("dirswap: create the staging tree at %q: %w", staging, err)
	}
	return staging, nil
}

// WriteFile writes one file into a staged tree through the platform's atomic
// writer, and then makes the directory entry durable.
//
// The file is written to a unique temp, fsynced, chmodded and renamed over
// the target: a reader never sees a half-written file, and the content is on
// stable storage before the tree is published. The parent directory is
// fsynced afterwards, so the entry itself survives a power loss — a file
// whose bytes are durable but whose directory entry is not is still a file
// that was never there.
//
// The directory fsync is BEST EFFORT: it legitimately fails on platforms
// whose directory handles cannot be synced (Windows), and a dev machine must
// not fail a publish over a guarantee that only the deploy target needs.
func WriteFile(path string, data []byte, mode os.FileMode) error {
	if err := atomicfile.WriteFile(path, data, mode); err != nil {
		return err
	}
	_ = syncDir(filepath.Dir(path))
	return nil
}

// Swap publishes the staged tree at staged as the tree at dst.
//
// The old tree is moved aside first, the staged tree is renamed into place,
// and only then is the old tree removed. If the rename into place fails, the
// old tree is moved back and the destination is left exactly as it was.
func Swap(staged, dst string) error {
	if staged == dst {
		return fmt.Errorf("dirswap: the staged tree and the destination are the same path %q", dst)
	}
	aside := AsidePath(dst)
	// Clear a leftover from an earlier swap first: the old tree always sits
	// at this ONE path, so the destination can never accumulate more than one.
	if err := os.RemoveAll(aside); err != nil {
		return fmt.Errorf("dirswap: clear the previous tree at %q: %w", aside, err)
	}

	// Move the old tree aside — a RENAME, so it is still there if the swap
	// fails. Deleting it here is what loses the content.
	moved := false
	if _, err := os.Stat(dst); err == nil {
		if err := renameWithRetry(dst, aside); err != nil {
			return fmt.Errorf("dirswap: move the current tree %q aside: %w", dst, err)
		}
		moved = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("dirswap: inspect the destination %q: %w", dst, err)
	}

	if err := renameWithRetry(staged, dst); err != nil {
		if moved {
			// Roll back: the old tree goes back where it was, so the
			// destination is never left missing.
			if rbErr := renameWithRetry(aside, dst); rbErr != nil {
				return fmt.Errorf("dirswap: publish %q into %q failed (%v) and the rollback failed too: the previous tree is at %q: %w",
					staged, dst, err, aside, rbErr)
			}
		}
		return fmt.Errorf("dirswap: publish %q into %q: %w", staged, dst, err)
	}

	// The new tree is in place. The old one is removed now; a failure here
	// leaves a leftover at a fixed path, which the NEXT swap removes.
	_ = os.RemoveAll(aside)
	// Make the rename itself durable: the destination's directory entry must
	// survive a power loss, or the published tree can vanish.
	_ = syncDir(filepath.Dir(dst))
	return nil
}

// renameWithRetry renames a path, retrying on Windows when the rename is
// refused because another process (antivirus, an indexer, a reader without
// FILE_SHARE_DELETE) still holds the target open.
//
// The handle is released in milliseconds, so a short bounded retry turns a
// hard failure into a successful swap. The retry is Windows-only: elsewhere
// a permission error is a real error and retrying it would only hide it.
func renameWithRetry(oldPath, newPath string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 20 * time.Millisecond)
		}
		if err = os.Rename(oldPath, newPath); err == nil {
			return nil
		}
		if runtime.GOOS != "windows" || !os.IsPermission(err) {
			return err
		}
	}
	return err
}

// syncDir fsyncs a directory so a newly created entry inside it is durable,
// not just the file's own content.
//
// Best effort by design: a directory handle cannot be synced on every
// platform (Windows refuses it), and the guarantee this adds is for the
// deploy target, not for the dev machine.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
