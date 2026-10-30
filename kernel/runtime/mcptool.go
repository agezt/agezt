// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime MCP lifecycle: the READS over the registry port +
//             the forwarders to the extensions module's registry service.
//
// The lifecycle is the EXTENSIONS MODULE's (W3.71): the module keeps the
// registry's operations, the live attachments and the bridge, and the host
// holds the module's face (extapi.MCPRegistryService) and forwards to it.
//
// The host's public names did not move — AddMCPServer and its six siblings
// are still the daemon's verbs, and every caller outside the runtime is
// untouched. What moved is WHERE the answer comes from: the daemon used to
// hold the store AND spell out the lifecycle over it, which left the module
// keeping the DATA and the daemon keeping the OPERATIONS.
//
// The READS stay on this side of the line on purpose: they are the nil-safe
// view over the CONTRACT's port (W3.58) — the daemon holds the port, and a
// host with no registry answers "no servers" instead of calling into nothing.

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
)

// ErrNoMCPRegistry is answered when the daemon handed the kernel no MCP
// registry (W3.58). The registry's FILE is the daemon's business, so a kernel
// without one is a daemon that has no MCP servers — and the writes must say
// so rather than call into nothing.
//
// The sentinel is the CONTRACT's row (mcpapi.ErrNoRegistry) since W3.71: the
// module raises it when the daemon handed over no registry port, and the host
// raises it when the module was never wired — and a caller compares with
// errors.Is, so both sides must raise the SAME error.
var ErrNoMCPRegistry = mcpapi.ErrNoRegistry

// MCPStore returns the durable MCP-server registry (M796) — the CONTRACT's
// port (W3.58). A kernel given no registry holds a nil port; the nil-safe
// readers below are what the layers above should call.
func (k *Kernel) MCPStore() mcpapi.Registry { return k.mcpStore }

// MCPRegistrations returns every registered MCP server — the read side, and
// the nil-safe answer for a kernel with no registry ("no servers").
func (k *Kernel) MCPRegistrations() []mcpapi.Server {
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

// MCPRegistration returns one registered MCP server by id or name — the
// nil-safe read for a kernel with no registry.
func (k *Kernel) MCPRegistration(ref string) (mcpapi.Server, bool) {
	if k.mcpStore == nil {
		return mcpapi.Server{}, false
	}
	return k.mcpStore.Get(ref)
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

func (r mcpReader) List() []mcpapi.Server { return r.k.MCPRegistrations() }

// MCPServers returns every registered MCP server — the ROW surface for the
// agent-facing tool. The tool is a plugin, and a plugin may reach the
// contract and the platform layers and nothing else, so it may not name the
// registry: it asks for the rows instead.
func (k *Kernel) MCPServers() []mcpapi.Server { return k.MCPRegistrations() }

// MCPServer returns one registered MCP server by id or name.
func (k *Kernel) MCPServer(ref string) (mcpapi.Server, bool) {
	return k.MCPRegistration(ref)
}

// SetMCPRegistry injects the MCP registry service the daemon drives.
//
// The registry is the EXTENSIONS MODULE's service: the module keeps the
// lifecycle, the daemon keeps the names. The composition root builds it over
// the store it opened and hands it over here.
func (k *Kernel) SetMCPRegistry(s extapi.MCPRegistryService) {
	k.configMu.Lock()
	k.mcp = s
	k.configMu.Unlock()
}

// mcpService returns the injected registry service, or nil.
func (k *Kernel) mcpService() extapi.MCPRegistryService {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.mcp
}

// mcpOrRefusal returns the registry service the daemon drives, or the
// refusal.
//
// The refusal is the contract's row: a daemon with no registry answers no
// registry, whether the store was never opened or the module was never
// wired.
func (k *Kernel) mcpOrRefusal() (extapi.MCPRegistryService, error) {
	svc := k.mcpService()
	if svc == nil {
		return nil, mcpapi.ErrNoRegistry
	}
	return svc, nil
}

// AddMCPServer validates and persists a new MCP server registration,
// journaling mcp.added. Registration alone spawns nothing — attach does.
func (k *Kernel) AddMCPServer(corr string, srv mcpapi.Server) (mcpapi.Server, error) {
	svc, err := k.mcpOrRefusal()
	if err != nil {
		return mcpapi.Server{}, err
	}
	return svc.Add(corr, srv)
}

// SetMCPServerEnabled flips a server's auto-attach-at-start flag, journaling
// mcp.updated. It does not touch a live attachment.
func (k *Kernel) SetMCPServerEnabled(corr, ref string, enabled bool) (mcpapi.Server, error) {
	svc, err := k.mcpOrRefusal()
	if err != nil {
		return mcpapi.Server{}, err
	}
	return svc.SetEnabled(corr, ref, enabled)
}

// AttachMCPServer spawns a registered server, completes the MCP handshake,
// discovers its tools, and journals mcp.attached with the discovered tool
// names — from the next run on, every agent is offered them as
// mcp_<name>_<tool>. Attaching an already-attached server is an error
// (detach first).
func (k *Kernel) AttachMCPServer(ctx context.Context, corr, ref string) (mcpapi.Server, []string, error) {
	svc, err := k.mcpOrRefusal()
	if err != nil {
		return mcpapi.Server{}, nil, err
	}
	return svc.Attach(ctx, corr, ref)
}

// DetachMCPServer closes a live attachment — the kill switch: its tools
// vanish from the next run on. Journals mcp.detached.
func (k *Kernel) DetachMCPServer(corr, ref string) error {
	svc, err := k.mcpOrRefusal()
	if err != nil {
		return err
	}
	return svc.Detach(corr, ref)
}

// RemoveMCPServer deletes a registration (detaching it first when live),
// journaling mcp.removed. Returns whether it existed.
func (k *Kernel) RemoveMCPServer(corr, ref string) (bool, error) {
	svc, err := k.mcpOrRefusal()
	if err != nil {
		return false, err
	}
	return svc.Remove(corr, ref)
}

// AttachEnabledMCPServers attaches every enabled registration — the daemon's
// boot path. Failures are per-server and reported, never fatal: one broken
// server must not take the others (or the daemon) down.
func (k *Kernel) AttachEnabledMCPServers(ctx context.Context) (attached []string, failures map[string]error) {
	failures = map[string]error{}
	svc := k.mcpService()
	if svc == nil {
		return nil, failures
	}
	return svc.AttachEnabled(ctx)
}

// MCPAttached returns the names of currently-attached servers (sorted) with
// their bridged tool counts — the status surface for `agt mcp list` and the
// console.
func (k *Kernel) MCPAttached() map[string]int {
	svc := k.mcpService()
	if svc == nil {
		return map[string]int{}
	}
	return svc.Attached()
}

// closeMCPConns detaches everything — called from Kernel.Close.
func (k *Kernel) closeMCPConns() {
	svc := k.mcpService()
	if svc == nil {
		return
	}
	svc.Close()
}
