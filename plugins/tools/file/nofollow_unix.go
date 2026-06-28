// SPDX-License-Identifier: MIT

//go:build unix

package file

import (
	"os"
	"syscall"
)

// oNoFollow is the raw flag for os.OpenFile — kept for test compatibility.
// New code should use openFileNoFollow.
const oNoFollow = syscall.O_NOFOLLOW

// openFileNoFollow opens path with O_NOFOLLOW so a trailing symlink in the
// final path component is never followed. On unix this is a kernel-enforced
// guarantee (the open fails with ELOOP). The workspaceRoot parameter is
// unused on unix — the kernel provides the protection directly.
func openFileNoFollow(path string, flag int, perm os.FileMode, _ string) (*os.File, error) {
	return os.OpenFile(path, flag|oNoFollow, perm)
}
