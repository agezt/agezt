// SPDX-License-Identifier: MIT

//go:build linux || darwin || freebsd

package pulse

import "syscall"

// DiskUsage returns the available (to an unprivileged user) and total bytes
// for the filesystem containing path. Stdlib-only (syscall.Statfs); no
// external dependency.
//
// Scoped to the platforms whose `syscall.Statfs_t` exposes the classic
// Bavail/Blocks/Bsize fields. NetBSD has no `syscall.Statfs` (it needs Statvfs1
// via golang.org/x/sys, which we deliberately don't depend on) and OpenBSD names
// the fields F_bavail/F_bsize, so both fall through to diskusage_other.go, which
// returns a "not supported" error that every caller already tolerates.
func DiskUsage(path string) (free, total uint64, err error) {
	var st syscall.Statfs_t
	if err = syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	// Convert each field explicitly: the syscall.Statfs_t field types differ
	// across the platforms this !windows file covers — Bsize is int64 on Linux,
	// uint32 on Darwin, uint64 on FreeBSD; Bavail is uint64 on Linux/Darwin but
	// int64 on FreeBSD. Multiplying mixed types fails to compile (it did on
	// freebsd before this), so widen every operand to uint64 first. Block counts
	// are inherently non-negative, so the int64→uint64 conversions are safe.
	bsize := uint64(st.Bsize)
	return uint64(st.Bavail) * bsize, uint64(st.Blocks) * bsize, nil
}
