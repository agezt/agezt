// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime MCP lifecycle: the READS over the registry port + the
//             MODULE'S FACE.
//
// The lifecycle is the EXTENSIONS MODULE's (W3.71): the module keeps the
// registry's operations, the live attachments and the bridge. Since W3.76w the
// daemon holds no verb per operation either — it used to hold SEVEN
// forwarders (`AddMCPServer`, `SetMCPServerEnabled`, `AttachMCPServer`,
// `DetachMCPServer`, `RemoveMCPServer`, `AttachEnabledMCPServers` and
// `MCPAttached`) plus two readers of its own (`MCPServers` and `MCPServer`),
// and all nine are gone.
//
// What the daemon holds now:
//
//   - the registry's FILE — `MCPStore`, the contract's store port (W3.58: the
//     composition root opens the file and hands the port over);
//   - the READ GATES — `MCPReader` (the nil-safe reader the catalog layer
//     holds) and `MCPRegistrationCount`. A read over the host's own store is
//     the host's, exactly as the roster's and the workflow library's reads
//     stayed;
//   - the module's FACE — `MCPRegistryService`, the port the application layer
//     and the agent-facing tool bind.
//
// The vocabulary is the CONTRACT's (`mcpapi.Runner` — add, attach, detach,
// remove, list, get, attachment status), so the module answers in those names
// and the daemon spells none of them.

import (
	"github.com/agezt/agezt/kernel/contract/mcpapi"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
)

// MCPStore returns the durable MCP-server registry (M796) — the CONTRACT's
// port (W3.58). A kernel given no registry holds a nil port; the nil-safe
// readers below are what the layers above should call.
func (k *Kernel) MCPStore() mcpapi.Registry { return k.mcpStore }

// mcpRegistrations returns every registered MCP server — the read side, and
// the nil-safe answer for a kernel with no registry ("no servers").
func (k *Kernel) mcpRegistrations() []mcpapi.Server {
	if k.mcpStore == nil {
		return nil
	}
	return k.mcpStore.List()
}

// MCPRegistrationCount returns how many MCP servers are registered — the
// nil-safe count for a kernel with no registry.
func (k *Kernel) MCPRegistrationCount() int {
	if k.mcpStore == nil {
		return 0
	}
	return k.mcpStore.Count()
}

// MCPReader returns the read-only half of the registry — the CONTRACT's
// reader port (W3.58), wrapped so a kernel with no registry answers "no
// servers" instead of handing a nil port to the catalog layer.
//
// The raw port stays reachable through MCPStore for the callers that drive
// the lifecycle; the READERS go through here, so there is exactly one
// nil-port surface for the reads.
func (k *Kernel) MCPReader() mcpapi.RegistrationReader { return mcpReader{k} }

// mcpReader is the nil-safe reader over the MCP registry: every answer comes
// from the kernel's own nil-safe readers, so a kernel with no registry is a
// kernel with no servers — not a crash.
type mcpReader struct{ k *Kernel }

func (r mcpReader) List() []mcpapi.Server { return r.k.mcpRegistrations() }

// mcpService returns the injected registry service, or nil.
//
// The service is the EXTENSIONS MODULE's: the module keeps the lifecycle, the
// live attachments and the bridge. The composition root hands the BUILDER over
// (see runtime.Config.MCPBuilder) and the kernel calls it while opening itself
// — so the daemon never needs a setter for it (W3.76k: the setter's only
// caller was the daemon's own test).
func (k *Kernel) mcpService() extapi.MCPRegistryService {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.mcp
}

// MCPRegistryService returns the MCP registry's service — the EXTENSIONS
// MODULE's FACE.
//
// It is the port the tools application and the agent-facing MCP tool bind
// (W3.76w), and it is nil when the daemon holds no module: the callers refuse
// then, rather than calling into nothing.
//
// The daemon does not speak FOR the registry (W3.76w): it used to hold seven
// forwarders over the module's own operations — a second spelling of every
// verb the contract's runner port already names — plus two readers of its own.
// The module answers in the contract's names now.
func (k *Kernel) MCPRegistryService() extapi.MCPRegistryService { return k.mcpService() }

// closeMCPConns detaches every live MCP attachment — the daemon's shutdown
// path.
//
// The attachments are the MODULE's, so the daemon asks the module to close
// them: the daemon holds no connection of its own to close.
func (k *Kernel) closeMCPConns() {
	svc := k.mcpService()
	if svc == nil {
		return
	}
	svc.Close()
}
