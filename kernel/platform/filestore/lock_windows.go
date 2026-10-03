// SPDX-License-Identifier: MIT

//go:build windows

package filestore

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockRange is the byte range locked. Any fixed non-empty range works: every
// holder locks the same one, and Windows byte-range locks may extend past EOF.
const lockRange = 1

func lockFile(f *os.File) error {
	var ol windows.Overlapped
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, lockRange, 0, &ol)
}

func unlockFile(f *os.File) error {
	var ol windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, lockRange, 0, &ol)
}
