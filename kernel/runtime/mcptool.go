// SPDX-License-Identifier: MIT

package runtime

// MCP self-install integration (M796): the kernel-side lifecycle of runtime-
// attached Model Context Protocol servers. The mcp store holds the registry;
// this file journals every transition (mcp.*), owns the LIVE attachments
// (server name → mcp.Conn), and merges each attachment's tools into every
// run's tool map as mcp_<server>_<tool> — the same dynamic seam forged
// script tools ride. Detach is the kill switch; Close detaches everything.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
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
	_, _ = k.bus.Publish(event.Spec{
		Subject: "mcp." + saved.Name, Kind: event.KindMCPAdded, Actor: "mcp",
		CorrelationID: corr,
		Payload:       map[string]any{"id": saved.ID, "name": saved.Name, "command": saved.Command, "args": saved.Args},
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
	k.mu.Lock()
	if _, live := k.mcpConns[srv.Name]; live {
		k.mu.Unlock()
		return mcp.Server{}, nil, fmt.Errorf("runtime: mcp server %s is already attached", srv.Name)
	}
	k.mu.Unlock()

	dial := k.cfg.MCPDialer
	if dial == nil {
		dial = mcp.Dial
	}
	conn, err := dial(ctx, srv.Command, srv.Args)
	if err != nil {
		return mcp.Server{}, nil, err
	}

	k.mu.Lock()
	if _, live := k.mcpConns[srv.Name]; live { // raced a concurrent attach
		k.mu.Unlock()
		_ = conn.Close()
		return mcp.Server{}, nil, fmt.Errorf("runtime: mcp server %s is already attached", srv.Name)
	}
	k.mcpConns[srv.Name] = conn
	k.mu.Unlock()

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

// DetachMCPServer closes a live attachment — the kill switch: its tools
// vanish from the next run on. Journals mcp.detached.
func (k *Kernel) DetachMCPServer(corr, ref string) error {
	// Resolve a registry ref (id or name) to the live-connection key, but
	// also accept the bare name of a connection whose registry row is gone.
	name := ref
	if srv, found := k.mcpStore.Get(ref); found {
		name = srv.Name
	}
	k.mu.Lock()
	conn, live := k.mcpConns[name]
	delete(k.mcpConns, name)
	k.mu.Unlock()
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
		k.mu.Lock()
		_, live := k.mcpConns[srv.Name]
		k.mu.Unlock()
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
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make(map[string]int, len(k.mcpConns))
	for name, conn := range k.mcpConns {
		out[name] = len(conn.Tools())
	}
	return out
}

// closeMCPConns detaches everything — called from Kernel.Close.
func (k *Kernel) closeMCPConns() {
	k.mu.Lock()
	conns := k.mcpConns
	k.mcpConns = map[string]mcp.Conn{}
	k.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

// mcpToolName builds the callable name: the mcp_ prefix routes the Edict
// toolmap to the mcp.call capability and namespaces bridged tools away from
// built-ins; the server segment (no underscores, by store validation) keeps
// the origin parseable. The server's own tool name is sanitized to the
// provider-safe alphabet and the whole thing capped at 64 chars.
func mcpToolName(server, tool string) string {
	var b strings.Builder
	for _, r := range tool {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := "mcp_" + server + "_" + b.String()
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

// mergeMCPTools returns the run's tool map extended with every live
// attachment's tools. Registered tools win a name collision; the input map
// comes back untouched when nothing is attached.
func (k *Kernel) mergeMCPTools(tools map[string]agent.Tool) map[string]agent.Tool {
	k.mu.Lock()
	type liveConn struct {
		name string
		conn mcp.Conn
	}
	live := make([]liveConn, 0, len(k.mcpConns))
	for name, conn := range k.mcpConns {
		live = append(live, liveConn{name, conn})
	}
	k.mu.Unlock()
	if len(live) == 0 {
		return tools
	}
	sort.Slice(live, func(i, j int) bool { return live[i].name < live[j].name })

	out := make(map[string]agent.Tool, len(tools)+8)
	for name, t := range tools {
		out[name] = t
	}
	for _, lc := range live {
		for _, def := range lc.conn.Tools() {
			name := mcpToolName(lc.name, def.Name)
			if _, exists := out[name]; exists {
				continue
			}
			out[name] = bridgedMCPTool{name: name, server: lc.name, def: def, conn: lc.conn}
		}
	}
	return out
}

// bridgedMCPTool adapts one discovered MCP tool to agent.Tool: the call's
// raw JSON input forwards as the tools/call arguments; the server's text
// content (and its own isError verdict) come back as the result.
type bridgedMCPTool struct {
	name   string
	server string
	def    mcp.ToolDef
	conn   mcp.Conn
}

func (t bridgedMCPTool) Definition() agent.ToolDef {
	schema := t.def.InputSchema
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type":"object"}`)
	}
	desc := strings.TrimSpace(t.def.Description)
	if desc == "" {
		desc = t.def.Name
	}
	return agent.ToolDef{
		Name:        t.name,
		Description: desc + " (via the attached MCP server \"" + t.server + "\")",
		InputSchema: schema,
	}
}

func (t bridgedMCPTool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	text, isErr, err := t.conn.Call(ctx, t.def.Name, raw)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return agent.Result{}, err // the run was cancelled, not the tool failing
		}
		return agent.Result{Output: t.name + ": " + err.Error(), IsError: true}, nil
	}
	if text == "" {
		text = "(no output)"
	}
	return agent.Result{Output: text, IsError: isErr}, nil
}
