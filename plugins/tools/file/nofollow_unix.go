// SPDX-License-Identifier: MIT

//go:build unix

package file

import "syscall"

// oNoFollow makes an O_CREATE open refuse to follow a symlink in the FINAL path
// component. The file tool resolves symlinks and rejects out-of-root targets in
// resolve(), but for a not-yet-existing target there is a narrow TOCTOU window
// between that check and the os.OpenFile: a concurrent process could plant a
// symlink at the path, and a plain O_CREATE would then follow it and write
// outside the workspace. O_NOFOLLOW closes that window — the open fails with
// ELOOP instead. Unix-only; see nofollow_other.go for the no-op fallback.
const oNoFollow = syscall.O_NOFOLLOW
