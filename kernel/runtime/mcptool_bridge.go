// SPDX-License-Identifier: MIT

// Runtime MCP-to-agent bridge: mergeMCPTools + bridgedMCPTool + lazyMCPDispatch.
// Code extracted from mcptool.go during the Day-130 god-file split.
// Public API unchanged.
package runtime


import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/mcp"
)

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
