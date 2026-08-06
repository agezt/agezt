// SPDX-License-Identifier: MIT

package controlplane

// Test-only exports for the external controlplane_test package.

// CommandSpecForTest is the dispatch metadata of one registered command,
// exposed so tenant_auth_test.go can sweep the ENTIRE tenant-token surface
// straight from the registry (no hand-maintained command list to go stale).
type CommandSpecForTest struct {
	Cmd           string
	TenantAllowed bool
	Streaming     StreamMode
}

// CommandSpecsForTest snapshots the command registry's dispatch metadata.
func CommandSpecsForTest() []CommandSpecForTest {
	out := make([]CommandSpecForTest, 0, len(commandRegistry))
	for cmd, sp := range commandRegistry {
		out = append(out, CommandSpecForTest{Cmd: cmd, TenantAllowed: sp.TenantAllowed, Streaming: sp.Streaming})
	}
	return out
}
