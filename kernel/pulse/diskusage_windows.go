// SPDX-License-Identifier: MIT

//go:build windows

package pulse

import (
	"syscall"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// DiskUsage returns the available (to the calling user) and total bytes for
// the volume containing path, via GetDiskFreeSpaceExW. Stdlib-only (syscall +
// unsafe); no external dependency.
func DiskUsage(path string) (free, total uint64, err error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	r, _, callErr := procGetDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r == 0 {
		return 0, 0, callErr
	}
	return freeBytesAvailable, totalBytes, nil
}
