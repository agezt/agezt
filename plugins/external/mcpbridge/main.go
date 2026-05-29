// SPDX-License-Identifier: MIT

// Command mcpbridge bridges an MCP (Model Context Protocol) server
// into the agezt plugin protocol so any MCP server — written for
// Claude Desktop, Cursor, Continue, etc. — becomes a agezt tool
// source.
//
// **Two stdio protocols, glued together.** The agezt daemon
// speaks the line-delimited-JSON plugin protocol on this process's
// stdio (see kernel/plugin). This bridge in turn spawns the
// configured MCP server and speaks JSON-RPC 2.0 with it over its
// stdio. Each `tool/invoke` from agezt translates to one MCP
// `tools/call`; the result translates back. No state held between
// invocations on the bridge side — the MCP server holds whatever
// session it needs.
//
// **Configuration.** Environment variables only — no flags. The
// agezt daemon already routes env via plugin.Config.Env so the
// operator's `AGEZT_PLUGINS=mcp=...` entry can pass through
// `MCPBRIDGE_SERVER_CMD` cleanly:
//
//	AGEZT_PLUGINS="ctx=/usr/local/bin/agezt-mcpbridge" \
//	MCPBRIDGE_SERVER_CMD="npx -y @modelcontextprotocol/server-filesystem /tmp" \
//	agezt
//
// will register every tool the filesystem MCP server exposes under
// the `ctx.` prefix in agezt's tool registry. Replace the command
// with any other MCP server (Postgres, GitHub, Slack, …).
//
// **Why a separate binary rather than in-kernel MCP support.** The
// kernel's plugin protocol is deliberately smaller than MCP (no
// JSON-RPC envelope, no SSE, no transport negotiation). Putting MCP
// in the kernel would mean every operator pays for the MCP code
// surface even when they don't use MCP servers — and the protocol
// is also a fast-moving target. As a separate binary the bridge
// can be replaced/upgraded without touching the daemon, and a
// future kernel never has to carry MCP-specific code.
//
// **What's NOT bridged (v1).** MCP also has `prompts/list`,
// `resources/list`, sampling callbacks, and progress notifications.
// Only `tools/*` is bridged — that's the surface agezt's agent
// loop actually consumes. The other surfaces are deferrable to a
// future v2 of the bridge (notably resources, which would map well
// onto a future agezt `resource/...` extension).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	// envServerCmd is the env var the operator sets to declare which
	// MCP server to run. Space-separated like a shell command line;
	// the first field is the executable, the rest are argv.
	envServerCmd = "MCPBRIDGE_SERVER_CMD"

	// envServerURL is the alternate-transport variant (M1.MCP-SSE):
	// the URL of a remote MCP server's `text/event-stream` GET
	// endpoint. When set, the bridge uses the HTTP+SSE transport
	// instead of spawning a child. Exactly one of envServerCmd or
	// envServerURL must be set; the bridge refuses to start with
	// both or with neither.
	envServerURL = "MCPBRIDGE_SERVER_URL"

	// envClientName lets the operator override the clientInfo.name
	// sent in the MCP initialize handshake. Some MCP servers gate
	// features on the client name (e.g. tooling specific to Claude
	// Desktop). Default identifies us as the bridge.
	envClientName = "MCPBRIDGE_CLIENT_NAME"

	// envProtoVersion overrides the MCP protocol version sent during
	// handshake. The default (mcpProtocolVersion) tracks the version
	// the bridge was tested against; operators talking to a newer
	// server may bump it.
	envProtoVersion = "MCPBRIDGE_PROTOCOL_VERSION"

	// mcpProtocolVersion is the MCP spec date the bridge implements.
	// Servers that require a strictly later version reject our
	// initialize; the operator overrides via envProtoVersion.
	mcpProtocolVersion = "2024-11-05"

	// Bridge-side timeouts. Distinct from agezt host's timeouts —
	// the host wraps `tool/invoke` in its own context (default 2m);
	// our internal MCP call uses these to bound the round-trip on
	// the MCP side. Picking matching defaults keeps "who timed out"
	// predictable: the bridge gives up just before the host does.
	initRoundTripTimeout = 15 * time.Second
	callRoundTripTimeout = 110 * time.Second
)

// ----- agezt plugin protocol wire types --------------------------
//
// Mirrors kernel/plugin/protocol.go. Duplicated here (rather than
// imported) so the bridge stays at zero agezt dependencies, the
// same contract every other plugin language must satisfy.

type ageztRequest struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type ageztResponse struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type ageztToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type ageztInitResult struct {
	Tools []ageztToolDef `json:"tools"`
}

type ageztInvokeParams struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type ageztInvokeResult struct {
	Output  string `json:"output"`
	IsError bool   `json:"is_error,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mcpbridge:", err)
		os.Exit(1)
	}
}

func run() error {
	cmdSpec := strings.TrimSpace(os.Getenv(envServerCmd))
	urlSpec := strings.TrimSpace(os.Getenv(envServerURL))
	switch {
	case cmdSpec != "" && urlSpec != "":
		return fmt.Errorf("set exactly one of %s (stdio) or %s (sse), not both", envServerCmd, envServerURL)
	case cmdSpec == "" && urlSpec == "":
		return fmt.Errorf("neither %s nor %s set (use stdio: \"npx -y @modelcontextprotocol/server-everything\"; or sse: \"https://mcp.example.com/sse\")", envServerCmd, envServerURL)
	}
	clientName := getenvDefault(envClientName, "agezt-mcpbridge")
	protoVersion := getenvDefault(envProtoVersion, mcpProtocolVersion)

	var (
		mcp *mcpClient
		err error
	)
	if urlSpec != "" {
		mcp, err = startSSEMCP(context.Background(), urlSpec, clientName, protoVersion)
		if err != nil {
			return fmt.Errorf("start sse mcp %q: %w", urlSpec, err)
		}
	} else {
		parts := strings.Fields(cmdSpec)
		mcp, err = startMCP(context.Background(), parts[0], parts[1:], clientName, protoVersion)
		if err != nil {
			return fmt.Errorf("start mcp server %q: %w", cmdSpec, err)
		}
	}
	defer mcp.close()

	serve(mcp)
	return nil
}

// serve is the agezt-side I/O loop. One Request line in, one
// Response line out, in lockstep. Concurrent invocations from the
// host are serialised here — the MCP server is also single-stream
// over its stdio, so serialising at the agezt boundary makes
// ownership of in-flight calls trivial. (Throughput is not a
// concern: tool calls are typically seconds-scale, and the agezt
// agent loop dispatches them sequentially anyway.)
func serve(mcp *mcpClient) {
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)

	for {
		line, err := in.ReadBytes('\n')
		if err != nil {
			// EOF: host closed stdin (typical shutdown path) or pipe
			// broke. Either way, exit — Close on the MCP child runs
			// via defer in run().
			return
		}
		var req ageztRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeAgezt(out, ageztResponse{ID: req.ID, Error: "mcpbridge: bad request: " + err.Error()})
			continue
		}
		switch req.Method {
		case "initialize":
			handleInitialize(out, mcp, req.ID)
		case "tool/invoke":
			handleInvoke(out, mcp, req)
		case "shutdown":
			// Drain any final stdout, then exit. The host gives us
			// DefaultShutdownGrace (5s) before SIGKILL.
			out.Flush()
			return
		default:
			writeAgezt(out, ageztResponse{ID: req.ID, Error: "mcpbridge: unknown method: " + req.Method})
		}
	}
}

// readResourceToolName is the synthetic tool the bridge registers
// when the MCP server exposes any resources (M1.ww). Operators see
// it as `<prefix>.read_resource(uri=...)` and the bridge dispatches
// to MCP `resources/read`. Single tool rather than one-per-URI so
// the agezt tool registry doesn't churn when the resource catalog
// changes between server runs.
const readResourceToolName = "read_resource"

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
		var args struct{ URI string `json:"uri"` }
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
func flattenContent(items []mcpContentItem) string {
	if len(items) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, c := range items {
		if i > 0 {
			sb.WriteByte('\n')
		}
		switch c.Type {
		case "text":
			sb.WriteString(c.Text)
		case "":
			// Defensive: some servers omit type for text blocks.
			sb.WriteString(c.Text)
		default:
			fmt.Fprintf(&sb, "[mcp:%s content omitted]", c.Type)
		}
	}
	return sb.String()
}

// flattenResourceContents collapses MCP resource-read output into
// a single string (M1.ww). Mirrors flattenContent's contract — the
// agent loop consumes the Output field as plain text — but the
// shape of the input is different: each block carries URI +
// mimeType + (text or blob), so we annotate per-block to preserve
// provenance.
func flattenResourceContents(items []mcpResourceContent) string {
	if len(items) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, c := range items {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		if c.URI != "" {
			fmt.Fprintf(&sb, "--- %s", c.URI)
			if c.MimeType != "" {
				fmt.Fprintf(&sb, " (%s)", c.MimeType)
			}
			sb.WriteByte('\n')
		}
		switch {
		case c.Text != "":
			sb.WriteString(c.Text)
		case c.Blob != "":
			fmt.Fprintf(&sb, "[mcp:blob omitted, %d base64 chars]", len(c.Blob))
		}
	}
	return sb.String()
}

func writeAgezt(w *bufio.Writer, r ageztResponse) {
	raw, err := json.Marshal(r)
	if err != nil {
		// Should be unreachable: every field is plain JSON. If it
		// happens, drop the response — the host will time out the
		// pending request rather than the bridge crashing.
		fmt.Fprintln(os.Stderr, "mcpbridge: encode response:", err)
		return
	}
	_, _ = w.Write(raw)
	_ = w.WriteByte('\n')
	// Flush per response — the host reads line-by-line and a buffered
	// response would deadlock both sides until the buffer filled.
	_ = w.Flush()
}

func getenvDefault(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
