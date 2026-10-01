// SPDX-License-Identifier: MIT

//go:build !windows

package tunnel

import (
	"os/exec"
	"syscall"
	"testing"
)

// TestKillProcessTree_FallsBackWhenGroupMissing covers the one statement that
// the 100% coverage ratchet in ci.yml was failing on: the fallback inside
// killProcessTree that runs when the process-group signal fails. On unix the
// package sat at 99.0% statement coverage — `proc_unix.go:28.72,30.3`, the
// `_ = cmd.Process.Kill()` arm — so `test (linux)` was red on main and nobody
// saw it, because the runner pool those jobs waited on had no runners.
//
// The group signal is failed deterministically rather than racily. killProcessTree
// signals -cmd.Process.Pid, which names a real process group only when the child
// was made a group leader by setProcessGroup. This child is started WITHOUT it,
// so it stays in the test binary's group and no group with pgid == child pid
// exists: syscall.Kill returns ESRCH on every run, with no dependence on pid
// reuse and none on a child that has already been reaped. The fallback then
// kills the direct child, which also cleans it up.
func TestKillProcessTree_FallsBackWhenGroupMissing(t *testing.T) {
	name, args := sleepCommand(30)
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer func() { _ = cmd.Wait() }()

	// Guard the premise rather than trusting it: if the child somehow leads its
	// own group, the group signal would succeed, the fallback would not run, and
	// this test would pass while covering nothing.
	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err != nil {
		t.Fatalf("getpgid(%d): %v", cmd.Process.Pid, err)
	} else if pgid == cmd.Process.Pid {
		t.Fatalf("child leads its own process group (pgid=%d); the group signal "+
			"would succeed and the fallback arm would never be reached", pgid)
	}

	killProcessTree(cmd)
}
