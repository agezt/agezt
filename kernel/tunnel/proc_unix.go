// SPDX-License-Identifier: MIT

//go:build !windows

package tunnel

import (
	osexec "os/exec"
	"syscall"
)

// setProcessGroup puts the tunnel binary in its own process group so the whole
// tree (the binary and any helper it spawns) tears down together on shutdown.
func setProcessGroup(cmd *osexec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessTree force-kills the tunnel binary's whole process group (its pgid
// equals its pid because setProcessGroup made it a group leader), falling back to
// the direct child if the group signal fails.
func killProcessTree(cmd *osexec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
