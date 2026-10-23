// SPDX-License-Identifier: MIT

//go:build !windows

package plugin

import (
	osexec "os/exec"
	"syscall"
)

// setProcessGroup puts the child in its OWN process group (M184), so the
// whole group — the plugin and any grandchildren it forks (a shell
// wrapper, a Python subprocess, …) — can be torn down together. Without
// it, killing only the direct child leaves orphaned grandchildren
// running: a resource leak and a persistence/escape flavour for an
// untrusted plugin.
func setProcessGroup(cmd *osexec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessTree force-kills the child's entire process group (M184).
// Because setProcessGroup made the child a group leader, its pgid equals
// its pid, so signalling the negative pid hits the whole group. Falls
// back to killing just the child if the group signal fails (e.g. the
// child already reaped its group).
func killProcessTree(cmd *osexec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
