// SPDX-License-Identifier: MIT

package runtime

// The daemon's face over the MCP REGISTRY — the ratchet of W3.76w.
//
// The MCP registry's lifecycle is the EXTENSIONS MODULE's: the module keeps
// the operations, the live attachments and the bridge. The daemon used to hold
// SEVEN forwarders over the module's own operations — `AddMCPServer`,
// `SetMCPServerEnabled`, `AttachMCPServer`, `DetachMCPServer`,
// `RemoveMCPServer`, `AttachEnabledMCPServers` and `MCPAttached` — plus TWO
// readers of its own (`MCPServers` and `MCPServer`), and all nine are gone.
//
// The vocabulary is the CONTRACT's (`mcpapi.Runner` — add, attach, detach,
// remove, list, get, attachment status), so the module answers in those names
// and the daemon spells none of them.
//
// This file pins the move in both directions:
//
//   - the daemon does not spell out the registry's operations any more;
//   - the daemon still holds the registry's FILE (the contract's store port),
//     its nil-safe READ GATES and the module's FACE.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting a forwarder back into the daemon
// has to make this file fail.

import (
	"testing"
)

// registryVerbsInTheDaemon are the daemon's spellings of the MCP registry's
// operations.
var registryVerbsInTheDaemon = []string{
	"AddMCPServer",
	"SetMCPServerEnabled",
	"AttachMCPServer",
	"DetachMCPServer",
	"RemoveMCPServer",
	"AttachEnabledMCPServers",
	"MCPAttached",
	"MCPServers",
	"MCPServer",
}

// TestTheDaemonDoesNotSpellOutTheMCPRegistry pins the move: the registry's
// lifecycle is the module's, and one operation has one name.
func TestTheDaemonDoesNotSpellOutTheMCPRegistry(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range registryVerbsInTheDaemon {
		if _, ok := methods[name]; ok {
			t.Errorf("*Kernel has the %s verb again — the MCP registry's lifecycle is the extensions module's, and one operation has one name (W3.76w)", name)
		}
	}
}

// TestTheDaemonKeepsTheRegistryGatesAndTheFace pins what stayed: the
// registry's FILE (the daemon opens it), the nil-safe READ GATES (a read over
// the host's own store is the host's) and the module's FACE — the one port
// the application and the agent-facing tool bind.
func TestTheDaemonKeepsTheRegistryGatesAndTheFace(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range []string{"MCPStore", "MCPReader", "MCPRegistrationCount", "MCPRegistryService"} {
		if !methods[name] {
			t.Errorf("*Kernel lost the %s verb — the daemon holds the registry's file, its read gates and the module's face", name)
		}
	}
}
