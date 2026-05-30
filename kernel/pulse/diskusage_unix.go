// SPDX-License-Identifier: MIT

//go:build !windows

package pulse

import "syscall"

// DiskUsage returns the available (to an unprivileged user) and total bytes
// for the filesystem containing path. Stdlib-only (syscall.Statfs); no
// external dependency.
func DiskUsage(path string) (free, total uint64, err error) {
	var st syscall.Statfs_t
	if err = syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bsize := uint64(st.Bsize)
	return st.Bavail * bsize, st.Blocks * bsize, nil
}
