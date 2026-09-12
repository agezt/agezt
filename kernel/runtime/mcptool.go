// SPDX-License-Identifier: MIT

// Runtime MCP lifecycle: MCPStore + Add/SetEnabled/Attach/Detach/Remove/AttachEnabled + dialMCP + MCPAttached + closeMCPConns + mcpToolName.
// Code extracted from mcptool.go during the Day-130 god-file split.
// Public API unchanged.
package runtime


import (
	"context"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/mcp"
)


// MCPStore returns the durable MCP-server registry (M796). Always non-nil
// after Open.
func (k *Kernel) MCPStore() *mcp.Store { return k.mcpStore }

// AddMCPServer validates and persists a new MCP server registration,
// journaling mcp.added. Registration alone spawns nothing — attach does.
func (k *Kernel) AddMCPServer(corr string, srv mcp.Server) (mcp.Server, error) {
	saved, err := k.mcpStore.Add(srv)
	if err != nil {
		return mcp.Server{}, err
	}
	transport := "stdio"
	if saved.URL != "" {
		transport = "http"
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "mcp." + saved.Name, Kind: event.KindMCPAdded, Actor: "mcp",
		CorrelationID: corr,
		Payload:       map[string]any{"id": saved.ID, "name": saved.Name, "transport": transport, "command": saved.Command, "args": saved.Args, "url": saved.URL},
	})
	return saved, nil
}

// SetMCPServerEnabled flips a server's auto-attach-at-start flag, journaling
// mcp.updated. It does not touch a live attachment.
func (k *Kernel) SetMCPServerEnabled(corr, ref string, enabled bool) (mcp.Server, error) {
	srv, err := k.mcpStore.SetEnabled(ref, enabled)
	if err != nil {
		return mcp.Server{}, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "mcp." + srv.Name, Kind: event.KindMCPUpdated, Actor: "mcp",
		CorrelationID: corr,
		Payload:       map[string]any{"id": srv.ID, "name": srv.Name, "enabled": enabled},
	})
	return srv, nil
}

// AttachMCPServer spawns a registered server, completes the MCP handshake,
// discovers its tools, and journals mcp.attached with the discovered tool
// names — from the next run on, every agent is offered them as
// mcp_<name>_<tool>. Attaching an already-attached server is an error
// (detach first).
func (k *Kernel) AttachMCPServer(ctx context.Context, corr, ref string) (mcp.Server, []string, error) {
	srv, found := k.mcpStore.Get(ref)
	if !found {
		return mcp.Server{}, nil, mcp.ErrNotFound
	}
	k.mcpMu.Lock()
	if _, live := k.mcpConns[srv.Name]; live {
		k.mcpMu.Unlock()
		return mcp.Server{}, nil, fmt.Errorf("runtime: mcp server %s is already attached", srv.Name)
	}
	k.mcpMu.Unlock()

	conn, err := k.dialMCP(ctx, srv)
	if err != nil {
		return mcp.Server{}, nil, err
	}

	k.mcpMu.Lock()
	if _, live := k.mcpConns[srv.Name]; live { // raced a concurrent attach
		k.mcpMu.Unlock()
		_ = conn.Close()
		return mcp.Server{}, nil, fmt.Errorf("runtime: mcp server %s is already attached", srv.Name)
	}
	k.mcpConns[srv.Name] = conn
	k.mcpMu.Unlock()

	names := make([]string, 0, len(conn.Tools()))
	for _, t := range conn.Tools() {
		names = append(names, mcpToolName(srv.Name, t.Name))
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "mcp." + srv.Name, Kind: event.KindMCPAttached, Actor: "mcp",
		CorrelationID: corr,
		Payload:       map[string]any{"id": srv.ID, "name": srv.Name, "tools": names},
	})
	return srv, names, nil
}

// dialMCP attaches one registered server over the transport its registration
// implies: a remote endpoint (URL) speaks Streamable HTTP (M904), everything
// else is a spawned stdio process (M796). Both seams default to the production
// dialer and are overridable for tests.
func (k *Kernel) dialMCP(ctx context.Context, srv mcp.Server) (mcp.Conn, error) {
	if strings.TrimSpace(srv.URL) != "" {
		dial := k.cfg.MCPHTTPDialer
		if dial == nil {
			dial = mcp.DialHTTP
		}
		return dial(ctx, srv.URL, srv.Headers)
	}
	dial := k.cfg.MCPDialer
	if dial == nil {
		dial = mcp.Dial
	}
	return dial(ctx, srv.Command, srv.Args, srv.Env)
}

// DetachMCPServer closes a live attachment — the kill switch: its tools
// vanish from the next run on. Journals mcp.detached.
func (k *Kernel) DetachMCPServer(corr, ref string) error {
	// Resolve a registry ref (id or name) to the live-connection key, but
	// also accept the bare name of a connection whose registry row is gone.
	name := ref
	if srv, found := k.mcpStore.Get(ref); found {
		name = srv.Name
	}
	k.mcpMu.Lock()
	conn, live := k.mcpConns[name]
	delete(k.mcpConns, name)
	k.mcpMu.Unlock()
	if !live {
		return fmt.Errorf("runtime: mcp server %s is not attached", name)
	}
	_ = conn.Close()
	_, _ = k.bus.Publish(event.Spec{
		Subject: "mcp." + name, Kind: event.KindMCPDetached, Actor: "mcp",
		CorrelationID: corr,
		Payload:       map[string]any{"name": name},
	})
	return nil
}

// RemoveMCPServer deletes a registration (detaching it first when live),
// journaling mcp.removed. Returns whether it existed.
func (k *Kernel) RemoveMCPServer(corr, ref string) (bool, error) {
	if srv, found := k.mcpStore.Get(ref); found {
		k.mcpMu.Lock()
		_, live := k.mcpConns[srv.Name]
		k.mcpMu.Unlock()
		if live {
			_ = k.DetachMCPServer(corr, srv.Name)
		}
	}
	gone, ok, err := k.mcpStore.Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "mcp." + gone.Name, Kind: event.KindMCPRemoved, Actor: "mcp",
			CorrelationID: corr,
			Payload:       map[string]any{"id": gone.ID, "name": gone.Name},
		})
	}
	return ok, nil
}

// AttachEnabledMCPServers attaches every enabled registration — the daemon's
// boot path. Failures are per-server and reported, never fatal: one broken
// server must not take the others (or the daemon) down.
func (k *Kernel) AttachEnabledMCPServers(ctx context.Context) (attached []string, failures map[string]error) {
	failures = map[string]error{}
	for _, srv := range k.mcpStore.List() {
		if !srv.Enabled {
			continue
		}
		if _, _, err := k.AttachMCPServer(ctx, "", srv.Name); err != nil {
			failures[srv.Name] = err
			continue
		}
		attached = append(attached, srv.Name)
	}
	return attached, failures
}

// MCPAttached returns the names of currently-attached servers (sorted) with
// their bridged tool counts — the status surface for `agt mcp list` and the
// console.
func (k *Kernel) MCPAttached() map[string]int {
	k.mcpMu.Lock()
	defer k.mcpMu.Unlock()
	out := make(map[string]int, len(k.mcpConns))
	for name, conn := range k.mcpConns {
		out[name] = len(conn.Tools())
	}
	return out
}

// closeMCPConns detaches everything — called from Kernel.Close.
func (k *Kernel) closeMCPConns() {
	k.mcpMu.Lock()
	conns := k.mcpConns
	k.mcpConns = map[string]mcp.Conn{}
	k.mcpMu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

// mcpToolName builds the callable name: the mcp_ prefix routes the Edict
// toolmap to the mcp.call capability and namespaces bridged tools away from
// built-ins; the server segment (no underscores, by store validation) keeps
// the origin parseable. The server's own tool name is sanitized to the
// provider-safe alphabet and the whole thing capped at 64 chars.
