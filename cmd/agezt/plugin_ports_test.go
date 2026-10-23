// SPDX-License-Identifier: MIT

package main

import (
	"io"
	"testing"
)

// TestToolBuildDepsSuppliesThePluginSpawnPort: the built-in tools plugin
// hosts out-of-process plugins, and the host side of that lives in a module
// the plugin may not name — so the daemon hands the CONTRACT's spawn port
// over here, at the composition root.
//
// A daemon that dropped it would leave the plugin unable to spawn anything
// on every live box, while every test that builds the tool by hand stayed
// green. This test is the only one that sees that wiring, so it asserts it
// directly.
func TestToolBuildDepsSuppliesThePluginSpawnPort(t *testing.T) {
	deps := toolBuildDeps(t.TempDir(), io.Discard, nil, nil)
	if deps.Plugins == nil {
		t.Fatal("the daemon must supply the plugin spawn port")
	}
}
