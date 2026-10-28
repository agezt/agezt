// SPDX-License-Identifier: MIT

package tools

import (
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
)

// The tools application's ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the tool registry,
// the script-tool forge and the MCP servers need, and how each of them
// answers. The host hands over the handle and nothing else — it does not
// decide what the tools are read from, and it cannot change the tools wiring
// without editing the application.
//
// The same shape already exists in app/council, app/roster, app/standing,
// app/schedule and app/workflow: the application takes the kernel and builds
// its own ports.
//
// This family owns the tool REGISTRY, and the registry is read by other
// families too: the roster's permission picture, the config's tool count,
// the schedule's tool targets and the nodes' remote-run check all ask the
// same question. The application answers it once, here.

// RegistryReader reads the tool registry: the registered tools, keyed by
// their registration name.
//
// The method is the APPLICATION's own name. The kernel's verb for the same
// thing belongs to this layer, and the adapter must not name it.
type RegistryReader interface {
	Registered() map[string]toolapi.Tool
}

// Ports are the tools application's capabilities over the tool registry, the
// script-tool forge and the MCP servers.
type Ports struct {
	// Registry reads the registered tools.
	Registry RegistryReader

	// ScriptForge reads the script-tool forge.
	//
	// It is nil when the daemon holds no forge: the application answers
	// "the script-tool forge is not available on this daemon" rather than
	// reading into nothing.
	//
	// The field is the application's own name, for the reason Attachment has
	// it: the kernel's verb for the skill library is called "Forge", and a
	// port field that answers to a kernel verb's name is a verb the adapter
	// may name — and the adapter holds this port.
	ScriptForge ForgeReader

	// ForgeWriter drives the script-tool lifecycle: draft, edit, test,
	// promote, quarantine and remove.
	ForgeWriter ForgeWriter

	// MCP reads the MCP registrations.
	MCP MCPRegistrationReader

	// Attachment reads the MCP attachment status.
	//
	// The field is the application's own name: the kernel's verb for the
	// same thing is a tools verb, and the adapter must not name it — not
	// even as a field of the port it holds.
	Attachment MCPAttachmentReader

	// MCPLifecycle drives the MCP server lifecycle: add, attach, detach,
	// enable and remove.
	MCPLifecycle MCPLifecycleWriter
}

// PortsFromKernel fills the tools application's ports from the kernel handle.
func PortsFromKernel(k *runtime.Kernel) Ports {
	return Ports{
		Registry:     kernelRegistry{k: k},
		ScriptForge:  k.ToolForge(),
		ForgeWriter:  k,
		MCP:          k.MCPReader(),
		Attachment:   k,
		MCPLifecycle: k,
	}
}

// kernelRegistry is the kernel's tool registry seen through the
// application's own port.
type kernelRegistry struct{ k *runtime.Kernel }

func (r kernelRegistry) Registered() map[string]toolapi.Tool { return r.k.Tools() }

// Registered returns the registered tools.
//
// It is nil-safe: a daemon with no registry sees no tools rather than a call
// into nothing.
func (p Ports) Registered() map[string]toolapi.Tool {
	if p.Registry == nil {
		return nil
	}
	return p.Registry.Registered()
}

// Count reports how many tools are registered.
func (p Ports) Count() int { return len(p.Registered()) }

// Has reports whether a tool is registered under that name.
func (p Ports) Has(name string) bool {
	_, ok := p.Registered()[name]
	return ok
}

// RemoteRun reports the remote_run tool: whether the registry holds it and
// whether the entry is a tool at all.
//
// The name is the tools family's: only the application knows which tool the
// node family means when it asks for a remote run.
func (p Ports) RemoteRun() (present, registered bool) {
	tool, ok := p.Registered()["remote_run"]
	return ok, tool != nil
}
