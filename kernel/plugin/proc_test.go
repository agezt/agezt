// SPDX-License-Identifier: MIT

package plugin

// Cross-platform tests for the process-group teardown helpers (M184).
// The behavioural grandchild-reaping guarantee is Unix-runtime (see
// proc_unix_test.go, validated on Linux CI); these cover the parts that
// run on any platform: nil-safety and that makeChild stays usable.

import (
	osexec "os/exec"
	"testing"
)

func TestKillProcessTree_SafeOnNilAndUnstarted(t *testing.T) {
	// Must not panic on a nil cmd or a cmd that never started.
	killProcessTree(nil)
	killProcessTree(&osexec.Cmd{}) // Process == nil
}

func TestMakeChild_ProducesRunnableCommand(t *testing.T) {
	cmd := makeChild("some-plugin-binary", []string{"--flag"})
	if cmd == nil {
		t.Fatal("makeChild returned nil")
	}
	if len(cmd.Args) == 0 || cmd.Args[0] != "some-plugin-binary" {
		t.Errorf("cmd.Args = %v; want first arg to be the binary", cmd.Args)
	}
}
