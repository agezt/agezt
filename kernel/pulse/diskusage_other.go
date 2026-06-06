// SPDX-License-Identifier: MIT

//go:build !windows && !linux && !darwin && !freebsd

package pulse

import (
	"errors"
	"runtime"
)

// DiskUsage is unsupported on platforms outside the {linux, darwin, freebsd}
// syscall.Statfs family and Windows. Those mainstream targets have native
// implementations (diskusage_unix.go / diskusage_windows.go); the rest (the
// remaining BSDs, plan9, wasm, etc.) land here so the package still builds for
// every GOOS. The error is intentional and tolerated by every caller — the disk
// observer and the control-plane disk-free gauge simply report no disk metrics
// on these platforms rather than failing the daemon.
func DiskUsage(path string) (free, total uint64, err error) {
	return 0, 0, errors.New("pulse: disk usage not supported on " + runtime.GOOS)
}
