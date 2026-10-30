// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime MCP-to-agent bridge: the FORWARDER to the extensions
//             module's bridge (W3.71). The adapters themselves (the bridged
//             tool, the lazy dispatcher and the callable-name rule) moved
//             into the module with the live attachments they answer for.

import (
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// mergeMCPTools returns the run's tool map extended with every live
// attachment's tools.
//
// The rule is the module's (W3.71): a registered tool wins a name
// collision, an allowlist trims what a server exposes, and a lazy server
// collapses into one dispatcher tool. A host with no registry service has no
// attachments, so the map comes back untouched.
func (k *Kernel) mergeMCPTools(tools map[string]toolapi.Tool) map[string]toolapi.Tool {
	svc := k.mcpService()
	if svc == nil {
		return tools
	}
	return svc.MergeTools(tools)
}
