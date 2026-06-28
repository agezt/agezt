// SPDX-License-Identifier: MIT

//go:build !unix && !windows

package file

import "os"

// oNoFollow is a no-op on non-Unix, non-Windows platforms. See nofollow_unix.go
// and nofollow_windows.go for the real implementations.
const oNoFollow = 0

// openFileNoFollow is the fallback for platforms that aren't unix and aren't
// windows. O_NOFOLLOW is POSIX-only, so we fall back to a plain open.
func openFileNoFollow(path string, flag int, perm os.FileMode, _ string) (*os.File, error) {
	return os.OpenFile(path, flag, perm)
}
