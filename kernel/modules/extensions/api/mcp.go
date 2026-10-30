// SPDX-License-Identifier: MIT

// Package api is the extensions module's public vocabulary: the rows the
// module publishes, the ports it needs from the host and the verbs the host
// drives.
//
// It exists because another module may import a module's api package and
// nothing else (archcheck's principle P5). The runtime keeps its own names as
// forwarders, so the host's public API is unchanged while the implementation
// lives in the module.
//
// The ROWS live in the contract layer (kernel/contract/mcpapi): the tools and
// the marketplace that name them are plugins, and a plugin may reach the
// contract and the platform layers only.
//
// Rules of the road:
//
//   - Types, interfaces and ports only. No logic that needs a store, a bus or
//     a clock — those are the implementation package's business.
//   - One row, one home. A row both the host and the module name is declared
//     exactly once — in the contract package — and every other side aliases
//     it.
package api

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
)

// MCPRegistryService is the MCP registry's public face: the lifecycle the
// host drives over the registered servers, and the bridge that folds an
// attached server's tools into a run's tool map.
//
// The lifecycle and the bridge are ONE service because they answer ONE
// question — which tools may this run call — and they share the live
// attachments that answer it. A second service would be a second owner of the
// same connections.
//
// The verbs are the REGISTRY's own: a host that holds a service can register
// a server, attach it, detach it and remove it, and it can never reach past
// the registry into the daemon's machinery.
type MCPRegistryService interface {
	// Add validates and persists a new MCP server registration, journaling
	// mcp.added. Registration alone spawns nothing — attach does.
	Add(corr string, srv mcpapi.Server) (mcpapi.Server, error)
	// SetEnabled flips a server's auto-attach-at-start flag, journaling
	// mcp.updated. It does not touch a live attachment.
	SetEnabled(corr, ref string, enabled bool) (mcpapi.Server, error)
	// Attach spawns a registered server, completes the MCP handshake,
	// discovers its tools, and journals mcp.attached with the discovered tool
	// names. Attaching an already-attached server is an error (detach first).
	Attach(ctx context.Context, corr, ref string) (mcpapi.Server, []string, error)
	// Detach closes a live attachment — the kill switch: its tools vanish
	// from the next run on. Journals mcp.detached.
	Detach(corr, ref string) error
	// Remove deletes a registration (detaching it first when live),
	// journaling mcp.removed. Returns whether it existed.
	Remove(corr, ref string) (bool, error)
	// AttachEnabled attaches every enabled registration — the daemon's boot
	// path. Failures are per-server and reported, never fatal: one broken
	// server must not take the others (or the daemon) down.
	AttachEnabled(ctx context.Context) (attached []string, failures map[string]error)
	// Attached returns the names of currently-attached servers with their
	// bridged tool counts.
	Attached() map[string]int
	// MergeTools returns the run's tool map extended with every live
	// attachment's tools. Registered tools win a name collision; the input
	// map comes back untouched when nothing is attached.
	MergeTools(tools map[string]toolapi.Tool) map[string]toolapi.Tool
	// Close detaches everything — the host's shutdown path.
	Close()
}

// MCPRegistryDependencies are the host's ports the registry needs to answer a
// verb.
//
// Every port is a way to ASK, never a way to reach: the registry publishes on
// the host's bus and dials through the host's seams, and it holds nothing
// else of the host's.
type MCPRegistryDependencies struct {
	// Registry is the durable MCP-server registry — the contract's store
	// port. The registry's FILE is the daemon's business (W3.58): the
	// composition root opens it and hands the port over.
	//
	// A nil port means the daemon has no registry, and the writes refuse
	// rather than call into nothing.
	Registry mcpapi.Registry

	// Dialer spawns and handshakes one stdio MCP server.
	//
	// A nil dialer means no stdio server can be attached, and the attach
	// refuses with a readable error rather than calling into nothing.
	Dialer mcpapi.Dialer

	// HTTPDialer handshakes one REMOTE MCP server over Streamable HTTP.
	//
	// A nil dialer means no remote server can be attached, and the attach
	// refuses with a readable error rather than calling into nothing.
	HTTPDialer mcpapi.HTTPDialer

	// Publish emits one event on the host's bus.
	//
	// The host owns the bus; the registry only says what happened to a
	// server, and the subject and the kind are the registry's own. A nil
	// port means the host has no bus, and the registry stays quiet rather
	// than publishing into nothing.
	Publish func(spec event.Spec)
}

// MCPRegistryBuilder builds the registry service over the dependencies the
// host collected.
//
// The daemon hands over the BUILDER (it names the module); the host calls it
// with the store, the dialers and its own publisher — so every mutation
// journals through the host's bus without either side naming the other's
// implementation.
type MCPRegistryBuilder func(MCPRegistryDependencies) MCPRegistryService
