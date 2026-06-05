// SPDX-License-Identifier: MIT

//go:build !unix

package file

// oNoFollow is a no-op on non-Unix platforms: O_NOFOLLOW is a POSIX flag, and the
// symlink-swap TOCTOU it guards against is a Unix sandbox concern (Windows symlink
// creation is privileged). See nofollow_unix.go for the real flag.
const oNoFollow = 0
