// SPDX-License-Identifier: MIT

package mcpapi

import "context"

// Runner is the port the MCP tool drives: the daemon's MCP registry — the
// servers the agent may add, attach, detach and remove.
//
// The tool is a PLUGIN, and the registry hands it the port — so the port
// lives in the contract layer, not in the tool's package (W3.39d). The row
// it names is the contract's own: `Server` is the same type the module
// keeps and the operator's list shows.
type Runner interface {
	// AddMCPServer registers a server.
	AddMCPServer(corr string, srv Server) (Server, error)
	// AttachMCPServer connects to a registered server and reports the tools
	// it offers.
	AttachMCPServer(ctx context.Context, corr, ref string) (Server, []string, error)
	// DetachMCPServer disconnects from a server without forgetting it.
	DetachMCPServer(corr, ref string) error
	// RemoveMCPServer forgets a server; the bool says whether it was there.
	RemoveMCPServer(corr, ref string) (bool, error)
	// MCPServers lists every registered server.
	MCPServers() []Server
	// MCPServer returns one registered server; the bool says whether it is
	// there.
	MCPServer(ref string) (Server, bool)
	// MCPAttached reports how many servers are attached, by kind.
	MCPAttached() map[string]int
}
