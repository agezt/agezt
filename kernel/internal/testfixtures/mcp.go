// SPDX-License-Identifier: MIT

package testfixtures

// Provenance: the shared test host helper for the MCP REGISTRY (W3.58). The
//             registry's file is the daemon's and the kernel holds the
//             contract's port — so a test that expects a registry opens one
//             and injects it, the same way the daemon does.
//
// The helper returns the contract's port: the daemon hands the kernel the
// port, never the concrete store.

import (
	"path/filepath"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/modules/extensions/mcp"
)

// MCPRegistry opens an MCP-server registry under dir and returns the
// contract's port for it.
func MCPRegistry(dir string) (mcpapi.Registry, error) {
	store, err := mcp.OpenStore(filepath.Join(dir, "mcp"))
	if err != nil {
		return nil, err
	}
	return store, nil
}
