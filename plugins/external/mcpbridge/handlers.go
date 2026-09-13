// SPDX-License-Identifier: MIT

package main

// MCP request handlers: handleInitialize + handleInvoke. Carved out
// of main.go during the Day 189 god-file split so the main file can
// stay focused on the lifecycle (types + main + run + serve) and
// the helpers file can stay focused on the content flatteners +
// response writer.
// Public API unchanged.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func handleInitialize(w *bufio.Writer, mcp *mcpClient, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), initRoundTripTimeout)
	defer cancel()
	tools, err := mcp.listTools(ctx)
	if err != nil {
		writeAgezt(w, ageztResponse{ID: id, Error: "mcpbridge: tools/list: " + err.Error()})
		return
	}
	out := make([]ageztToolDef, 0, len(tools)+1)
	for _, t := range tools {
		schema := t.InputSchema
		if len(schema) == 0 {
			// Agezt expects a JSON Schema; default to an empty
			// object schema when the MCP server omitted one (some
			// servers do for zero-arg tools).
			schema = json.RawMessage(`{"type":"object"}`)
		}
		out = append(out, ageztToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}

	// Resources surface (M1.ww). Best-effort: a server that doesn't
	// implement resources returns method-not-found, which listResources
	// translates to nil/nil. We only register read_resource when at
	// least one resource exists so empty-resource servers don't bloat
	// the agezt tool registry.
	if resources, err := mcp.listResources(ctx); err == nil && len(resources) > 0 {
		// Build a description that enumerates the available URIs so
		// the agent's planner can see what's reachable without first
		// having to call a separate list_resources tool.
		var sb strings.Builder
		sb.WriteString("Read an MCP resource by URI. Available URIs:")
		for _, r := range resources {
			fmt.Fprintf(&sb, "\n  - %s", r.URI)
			if r.Name != "" {
				fmt.Fprintf(&sb, " (%s)", r.Name)
			}
			if r.Description != "" {
				fmt.Fprintf(&sb, ": %s", r.Description)
			}
		}
		out = append(out, ageztToolDef{
			Name:        readResourceToolName,
			Description: sb.String(),
			InputSchema: json.RawMessage(`{
  "type":"object",
  "required":["uri"],
  "properties":{
    "uri":{"type":"string","description":"MCP resource URI (e.g. file:///path or scheme://host/key)"}
  }
}`),
		})
	}

	res, err := json.Marshal(ageztInitResult{Tools: out})
	if err != nil {
		writeAgezt(w, ageztResponse{ID: id, Error: "mcpbridge: marshal init result: " + err.Error()})
		return
	}
	writeAgezt(w, ageztResponse{ID: id, Result: res})
}

func handleInvoke(w *bufio.Writer, mcp *mcpClient, req ageztRequest) {
	var p ageztInvokeParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		writeAgezt(w, ageztResponse{ID: req.ID, Error: "mcpbridge: bad invoke params: " + err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), callRoundTripTimeout)
	defer cancel()

	// Synthetic read_resource tool (M1.ww) dispatches to MCP
	// `resources/read` rather than `tools/call`. The dispatch is
	// by name (read_resource is a reserved name in our local
	// surface; an MCP server happening to expose a tool with the
	// same name gets shadowed, which we accept as the cost of
	// keeping the name short).
	if p.Name == readResourceToolName {
		var args struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(p.Input, &args); err != nil || args.URI == "" {
			writeAgezt(w, ageztResponse{ID: req.ID, Error: "mcpbridge: read_resource needs {\"uri\":\"...\"}"})
			return
		}
		contents, err := mcp.readResource(ctx, args.URI)
		if err != nil {
			writeAgezt(w, ageztResponse{ID: req.ID, Error: "mcpbridge: " + err.Error()})
			return
		}
		out := ageztInvokeResult{Output: flattenResourceContents(contents)}
		raw, err := json.Marshal(out)
		if err != nil {
			writeAgezt(w, ageztResponse{ID: req.ID, Error: "mcpbridge: marshal: " + err.Error()})
			return
		}
		writeAgezt(w, ageztResponse{ID: req.ID, Result: raw})
		return
	}

	res, err := mcp.callTool(ctx, p.Name, p.Input)
	if err != nil {
		writeAgezt(w, ageztResponse{ID: req.ID, Error: "mcpbridge: " + err.Error()})
		return
	}
	out := ageztInvokeResult{
		Output:  flattenContent(res.Content),
		IsError: res.IsError,
	}
	raw, err := json.Marshal(out)
	if err != nil {
		writeAgezt(w, ageztResponse{ID: req.ID, Error: "mcpbridge: marshal invoke result: " + err.Error()})
		return
	}
	writeAgezt(w, ageztResponse{ID: req.ID, Result: raw})
}

// flattenContent collapses MCP's content-block array into a single
// string for agezt's flat Output field. Text blocks concatenate
// with newline separators; non-text blocks (image/resource) become
// a placeholder annotation so the agent at least knows something
// non-textual was returned. A future bridge revision could surface
// image bytes via a separate channel, but the agent loop today
// consumes Output as plain text.
