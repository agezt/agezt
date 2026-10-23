// SPDX-License-Identifier: MIT

//go:build !windows

package plugin

// Unix-only: makeChild must put the child in its own process group so
// teardown can kill the whole tree (M184). Validated on Linux CI (the
// daemon's first-class target); not run on the Windows dev box, where
// setProcessGroup is intentionally a no-op.

import "testing"

func TestMakeChild_SetsProcessGroup(t *testing.T) {
	cmd := makeChild("some-plugin-binary", nil)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Errorf("makeChild did not set Setpgid; SysProcAttr=%+v", cmd.SysProcAttr)
	}
}
