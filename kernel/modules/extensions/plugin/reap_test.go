// SPDX-License-Identifier: MIT

package plugin

// White-box test for the dedicated process waiter (M422): a child that exits on
// its own must be reaped (cmd.Wait completes) without anyone calling Close — before
// the fix, markDead set the plugin dead without ever calling Wait and Close's
// dead-check skipped the only Wait call site, so a self-exited plugin became a
// zombie. Portable (uses the standard test-binary helper-process idiom), so it runs
// on the Windows dev box and Linux CI alike.

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestStartWaiter_ReapsSelfExitedChild(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess_JustExit")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	done := startWaiter(cmd)

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("startWaiter never reaped the self-exited child (waitDone not closed)")
	}
	// ProcessState is non-nil only once cmd.Wait() has completed — the proof the
	// child was actually reaped, not just abandoned.
	if cmd.ProcessState == nil {
		t.Fatal("child not reaped: ProcessState is nil (Wait never completed → zombie)")
	}
}

// TestHelperProcess_JustExit is not a real test: it is the child process spawned by
// TestStartWaiter_ReapsSelfExitedChild. It exits immediately when invoked as the
// helper, and is a no-op in a normal test run.
func TestHelperProcess_JustExit(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(0)
}
