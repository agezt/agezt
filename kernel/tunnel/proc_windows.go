// SPDX-License-Identifier: MIT

//go:build windows

package tunnel

import osexec "os/exec"

// setProcessGroup is a no-op on Windows (reliable whole-tree teardown needs a Job
// Object, which the daemon's first-class Linux target does not require). On
// Windows the supervisor kills the direct child only.
func setProcessGroup(cmd *osexec.Cmd) {}

// killProcessTree kills the direct child on Windows. Any helper the tunnel binary
// forks is not reaped here — a documented limitation mirroring the plugin host.
func killProcessTree(cmd *osexec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
