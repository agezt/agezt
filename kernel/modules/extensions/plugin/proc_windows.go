// SPDX-License-Identifier: MIT

//go:build windows

package plugin

import osexec "os/exec"

// setProcessGroup is a no-op on Windows (M184). Reliable whole-tree
// teardown on Windows needs a Job Object, which the host does not yet
// create; the daemon's first-class deployment target is Linux (see the
// GOOS=linux build gate), where setProcessGroup uses a real process
// group. On Windows the host kills the direct child only.
func setProcessGroup(cmd *osexec.Cmd) {}

// killProcessTree kills the direct child on Windows (M184). Grandchildren
// a plugin forks are NOT reaped here — a documented limitation tracked
// for a Windows-specific Job Object follow-up.
func killProcessTree(cmd *osexec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
