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
	k.mcpMu.Lock()
	type liveConn struct {
		name string
		conn mcp.Conn
	}
	live := make([]liveConn, 0, len(k.mcpConns))
	for name, conn := range k.mcpConns {
		live = append(live, liveConn{name, conn})
	}
	k.mcpMu.Unlock()
	if len(live) == 0 {
		return tools
	}
	sort.Slice(live, func(i, j int) bool { return live[i].name < live[j].name })

	out := make(map[string]agent.Tool, len(tools)+8)
	for name, t := range tools {
		out[name] = t
	}
	for _, lc := range live {
		// Per-server config: ToolAllow (M899) trims which tools are exposed; Lazy
		// (M906) collapses them into a single dispatcher. An empty/absent
		// allowlist means "expose all"; lazy defaults off (eager injection).
		var filter map[string]bool
		lazy := false
		if srv, ok := k.mcpStore.Get(lc.name); ok {
			if len(srv.ToolAllow) > 0 {
				filter = make(map[string]bool, len(srv.ToolAllow))
				for _, t := range srv.ToolAllow {
					filter[t] = true
				}
			}
			lazy = srv.Lazy
		}

		// The exposed subset (after the allowlist).
		exposed := make([]mcp.ToolDef, 0, len(lc.conn.Tools()))
		for _, def := range lc.conn.Tools() {
			if filter != nil && !filter[def.Name] {
				continue
			}
			exposed = append(exposed, def)
		}
		if len(exposed) == 0 {
			continue
		}

		if lazy {
			// One dispatcher tool for the whole server — N schemas → 1.
			name := "mcp_" + lc.name
			if _, exists := out[name]; !exists {
				out[name] = lazyMCPDispatch{name: name, server: lc.name, conn: lc.conn, tools: exposed}
			}
			continue
		}
		for _, def := range exposed {
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
		Effect: agent.ToolEffect{
			Class: agent.EffectCompensable,
			PredictedEffects: []string{
				"Forward this tool call to an attached MCP server.",
			},
			AffectedResources: []string{"MCP server " + t.server, "remote resources reachable by MCP tool " + t.def.Name},
			RollbackNotes:     "Compensation depends on the MCP server and specific remote tool; detach the server to stop future calls.",
			Confidence:        0.45,
		},
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

// lazyMCPDispatch collapses one server's exposed tools into a single dispatcher
// tool (mcp_<server>) for context efficiency (M906): instead of injecting each
// tool's full input schema into every run, the run is offered ONE tool whose
// `tool` argument is an enum of the server's tool names and whose `arguments`
// is a freeform object the remote server validates. The tool descriptions are
// listed in the dispatcher's own description so the model can still choose.
type lazyMCPDispatch struct {
	name   string // mcp_<server>
	server string
	conn   mcp.Conn
	tools  []mcp.ToolDef // the exposed (allowlisted) subset
}

func (t lazyMCPDispatch) Definition() agent.ToolDef {
	names := make([]string, 0, len(t.tools))
	var b strings.Builder
	fmt.Fprintf(&b, "Call a tool on the attached MCP server %q. Set \"tool\" to one of its tools and \"arguments\" to that tool's input object (the server validates it). Available tools:\n", t.server)
	for _, d := range t.tools {
		names = append(names, d.Name)
		if desc := strings.TrimSpace(d.Description); desc != "" {
			fmt.Fprintf(&b, "- %s: %s\n", d.Name, desc)
		} else {
			fmt.Fprintf(&b, "- %s\n", d.Name)
		}
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tool":      map[string]any{"type": "string", "enum": names, "description": "which of the server's tools to call"},
			"arguments": map[string]any{"type": "object", "description": "the input object for that tool"},
		},
		"required":             []string{"tool"},
		"additionalProperties": false,
	}
	raw, _ := json.Marshal(schema)
	return agent.ToolDef{
		Name:        t.name,
		Description: strings.TrimRight(b.String(), "\n"),
		InputSchema: raw,
		Effect: agent.ToolEffect{
			Class: agent.EffectCompensable,
			PredictedEffects: []string{
				"Dispatch a selected tool call through an attached MCP server.",
			},
			AffectedResources: []string{"MCP server " + t.server, "remote resources reachable by the selected MCP tool"},
			RollbackNotes:     "Compensation depends on the selected MCP tool; detach the server to stop future calls.",
			Confidence:        0.4,
		},
	}
}

func (t lazyMCPDispatch) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in struct {
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{Output: t.name + ": invalid input: " + err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(in.Tool) == "" {
		return agent.Result{Output: t.name + `: "tool" is required (one of the server's tools)`, IsError: true}, nil
	}
	exposed := false
	for _, d := range t.tools {
		if d.Name == in.Tool {
			exposed = true
			break
		}
	}
	if !exposed {
		return agent.Result{Output: fmt.Sprintf("%s: tool %q is not exposed by server %q", t.name, in.Tool, t.server), IsError: true}, nil
	}
	text, isErr, err := t.conn.Call(ctx, in.Tool, in.Arguments)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return agent.Result{}, err
		}
		return agent.Result{Output: t.name + ": " + err.Error(), IsError: true}, nil
	}
	if text == "" {
		text = "(no output)"
	}
	return agent.Result{Output: text, IsError: isErr}, nil
}
