// SPDX-License-Identifier: MIT

// Package mcptool is the in-process `mcp` tool (M796): governed self-install.
// The agent extends its OWN reach at runtime — register an MCP server
// (op=add), ATTACH it (op=attach: the daemon spawns the process, handshakes,
// and from the next run on its tools are callable as mcp_<server>_<tool>),
// detach it (the kill switch), list, or remove. Install ops are gated by the
// `mcp.install` Edict capability (Ask by default — attaching runs an
// arbitrary external process); every bridged call later exercises
// `mcp.call`. The child gets a scrubbed environment and every lifecycle
// transition is journaled (mcp.*).
package mcptool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/mcp"
)

// Kernel is the slice of the runtime kernel this tool drives — satisfied by
// *runtime.Kernel and easy to fake in tests.
type Kernel interface {
	AddMCPServer(corr string, srv mcp.Server) (mcp.Server, error)
	AttachMCPServer(ctx context.Context, corr, ref string) (mcp.Server, []string, error)
	DetachMCPServer(corr, ref string) error
	RemoveMCPServer(corr, ref string) (bool, error)
	MCPStore() *mcp.Store
	MCPAttached() map[string]int
}

// Tool implements agent.Tool. Construct with New, then Bind the live kernel.
type Tool struct {
	mu sync.RWMutex
	k  Kernel
}

// New returns an unbound Tool — Invoke reports the surface unavailable until
// Bind is called.
func New() *Tool { return &Tool{} }

// Bind wires the live kernel. Called once after the kernel opens.
func (t *Tool) Bind(k Kernel) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.k = k
}

func (t *Tool) current() Kernel {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.k
}

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "mcp",
		Description: "Extend your own toolbox by installing MCP (Model Context Protocol) servers at runtime. " +
			"op=add registers a server (a stdio command, e.g. command \"npx\" args [\"-y\",\"@modelcontextprotocol/server-everything\"]); " +
			"op=attach spawns it and discovers its tools — from your NEXT run they are callable as mcp_<name>_<tool>; " +
			"op=detach stops it (its tools vanish); op=list shows registrations and what is live; op=remove deletes a registration. " +
			"Use this when a task needs a capability an existing MCP server provides. " +
			"Installs are operator-governed (approval) and journaled.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":          {"type":"string", "enum":["add","attach","detach","list","remove"]},
    "name":        {"type":"string", "description":"For op=add: the server's handle (lowercase letters/digits, e.g. \"everything\"); its tools appear as mcp_<name>_<tool>."},
    "command":     {"type":"string", "description":"For op=add: the executable to spawn (stdio MCP server), e.g. \"npx\"."},
    "args":        {"type":"array", "items":{"type":"string"}, "description":"For op=add: the command's arguments, e.g. [\"-y\",\"@modelcontextprotocol/server-everything\"]."},
    "description": {"type":"string", "description":"For op=add (optional): what this server provides."},
    "ref":         {"type":"string", "description":"For op=attach/detach/remove: the server's name or id."}
  }
}`),
	}
}

type input struct {
	Op          string   `json:"op"`
	Name        string   `json:"name"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	Description string   `json:"description"`
	Ref         string   `json:"ref"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("mcp: parse input: %w", err)
	}
	k := t.current()
	if k == nil {
		return errResult("mcp self-install is not available on this daemon"), nil
	}
	corr := agent.CorrelationFromContext(ctx)

	switch in.Op {
	case "add":
		srv, err := k.AddMCPServer(corr, mcp.Server{
			Name:        in.Name,
			Command:     in.Command,
			Args:        in.Args,
			Description: in.Description,
		})
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{
			"name": srv.Name, "command": srv.Command, "args": srv.Args,
			"message": "registered — attach it (op=attach) to make its tools callable",
		}), nil

	case "attach":
		if strings.TrimSpace(in.Ref) == "" {
			return errResult(`op=attach needs a "ref" (the server's name or id)`), nil
		}
		srv, tools, err := k.AttachMCPServer(ctx, corr, in.Ref)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{
			"name": srv.Name, "tools": tools,
			"message": fmt.Sprintf("attached — %d tool(s) callable from your next run", len(tools)),
		}), nil

	case "detach":
		if strings.TrimSpace(in.Ref) == "" {
			return errResult(`op=detach needs a "ref" (the server's name or id)`), nil
		}
		if err := k.DetachMCPServer(corr, in.Ref); err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{"message": "detached — its tools are no longer offered"}), nil

	case "list":
		attached := k.MCPAttached()
		servers := k.MCPStore().List()
		views := make([]map[string]any, 0, len(servers))
		for _, srv := range servers {
			v := map[string]any{
				"name": srv.Name, "command": srv.Command, "args": srv.Args,
				"auto_attach": srv.Enabled, "attached": false,
			}
			if n, live := attached[srv.Name]; live {
				v["attached"] = true
				v["tool_count"] = n
			}
			if srv.Description != "" {
				v["description"] = srv.Description
			}
			views = append(views, v)
		}
		// A live connection whose registry row was removed still matters.
		for name, n := range attached {
			if _, found := k.MCPStore().Get(name); !found {
				views = append(views, map[string]any{"name": name, "attached": true, "tool_count": n, "unregistered": true})
			}
		}
		sort.Slice(views, func(i, j int) bool {
			return views[i]["name"].(string) < views[j]["name"].(string)
		})
		return okJSON(map[string]any{"count": len(views), "servers": views}), nil

	case "remove":
		if strings.TrimSpace(in.Ref) == "" {
			return errResult(`op=remove needs a "ref" (the server's name or id)`), nil
		}
		ok, err := k.RemoveMCPServer(corr, in.Ref)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if !ok {
			return errResult("no mcp server " + in.Ref), nil
		}
		return okJSON(map[string]any{"message": "removed (detached first if it was live)"}), nil

	case "":
		return errResult("op required (add|attach|detach|list|remove)"), nil
	default:
		return errResult("unknown op " + in.Op + " (add|attach|detach|list|remove)"), nil
	}
}

func okJSON(v any) agent.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "mcp: " + msg, IsError: true}
}
