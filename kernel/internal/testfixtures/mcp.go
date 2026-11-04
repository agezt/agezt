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
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
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

// MCPAttachments answers the attachment status of a registry service.
//
// The attachments are the EXTENSIONS MODULE's (W3.76w): a daemon that holds
// no registry module holds no face to answer with — and a daemon with no
// module has no attachments. The helper says so instead of calling into
// nothing.
func MCPAttachments(svc extapi.MCPRegistryService) map[string]int {
	if svc == nil {
		return nil
	}
	return svc.MCPAttached()
}
