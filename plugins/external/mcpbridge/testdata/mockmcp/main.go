// SPDX-License-Identifier: MIT

// Command mockmcp is a minimal MCP (Model Context Protocol) server
// used by the mcpbridge test suite. It implements just enough of
// MCP 2024-11-05 to exercise the bridge end-to-end: initialize,
// initialized notification, tools/list, tools/call.
//
// Two tools are exposed:
//
//   - `greet` — takes {"name": "..."} and returns text "hello, X".
//   - `boom`  — always returns isError=true with text "deliberate".
//
// The server prints one MOCKMCP_STARTED line to stderr on startup
// (so a test using stderr capture can verify it actually launched),
// then loops on stdin reading line-delimited JSON-RPC 2.0. EOF
// exits clean.
//
// This binary is built on demand by mcpbridge's test suite (the
// same one-shot `go build` pattern the kernel/plugin echoplugin
// uses). It has NO imports of the bridge or agezt — its sole
// contract is the MCP wire format, the same contract any real
// third-party MCP server satisfies.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type jReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jErr           `json:"error,omitempty"`
}

type jErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	fmt.Fprintln(os.Stderr, "MOCKMCP_STARTED")
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	initialized := false
	for {
		line, err := in.ReadBytes('\n')
		if err != nil {
			return
		}
		var req jReq
		if err := json.Unmarshal(line, &req); err != nil {
			writeErr(out, nil, -32700, "parse error: "+err.Error())
			continue
		}
		switch req.Method {
		case "initialize":
			// Echo back a minimal init result. Don't check the
			// client's protocol version — the bridge sends a known
			// good one.
			res, _ := json.Marshal(map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "mockmcp", "version": "0.1"},
			})
			writeResp(out, jResp{JSONRPC: "2.0", ID: req.ID, Result: res})

		case "notifications/initialized":
			// No response (notification). Flip the gate so subsequent
			// tool calls are accepted.
			initialized = true

		case "tools/list":
			if !initialized {
				writeErr(out, req.ID, -32002, "server not yet initialized")
				continue
			}
			res, _ := json.Marshal(map[string]any{
				"tools": []map[string]any{
					{
						"name":        "greet",
						"description": "Returns a friendly greeting.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name": map[string]any{"type": "string"},
							},
							"required": []string{"name"},
						},
					},
					{
						"name":        "boom",
						"description": "Always returns isError=true.",
						"inputSchema": map[string]any{
							"type": "object",
						},
					},
				},
			})
			writeResp(out, jResp{JSONRPC: "2.0", ID: req.ID, Result: res})

		case "notifications/cancelled":
			// MCP v2 cancellation: server-side a real implementation
			// would interrupt the in-flight request. Mock just logs
			// to stderr so the test can verify the bridge sent it.
			fmt.Fprintln(os.Stderr, "MOCKMCP_GOT_CANCEL")

		case "resources/list":
			if !initialized {
				writeErr(out, req.ID, -32002, "server not yet initialized")
				continue
			}
			res, _ := json.Marshal(map[string]any{
				"resources": []map[string]any{
					{
						"uri":         "file:///mock/doc.md",
						"name":        "doc",
						"description": "mock markdown doc",
						"mimeType":    "text/markdown",
					},
				},
			})
			writeResp(out, jResp{JSONRPC: "2.0", ID: req.ID, Result: res})

		case "resources/read":
			if !initialized {
				writeErr(out, req.ID, -32002, "server not yet initialized")
				continue
			}
			var p struct {
				URI string `json:"uri"`
			}
			if err := json.Unmarshal(req.Params, &p); err != nil {
				writeErr(out, req.ID, -32602, "bad params: "+err.Error())
				continue
			}
			if p.URI != "file:///mock/doc.md" {
				writeErr(out, req.ID, -32602, "unknown resource: "+p.URI)
				continue
			}
			res, _ := json.Marshal(map[string]any{
				"contents": []map[string]any{
					{
						"uri":      p.URI,
						"mimeType": "text/markdown",
						"text":     "# mock\n\nthis is the mock doc",
					},
				},
			})
			writeResp(out, jResp{JSONRPC: "2.0", ID: req.ID, Result: res})

		case "tools/call":
			if !initialized {
				writeErr(out, req.ID, -32002, "server not yet initialized")
				continue
			}
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &p); err != nil {
				writeErr(out, req.ID, -32602, "bad params: "+err.Error())
				continue
			}
			switch p.Name {
			case "greet":
				var args struct {
					Name string `json:"name"`
				}
				if err := json.Unmarshal(p.Arguments, &args); err != nil {
					writeErr(out, req.ID, -32602, "bad arguments: "+err.Error())
					continue
				}
				if args.Name == "" {
					args.Name = "stranger"
				}
				res, _ := json.Marshal(map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "hello, " + args.Name},
					},
				})
				writeResp(out, jResp{JSONRPC: "2.0", ID: req.ID, Result: res})
			case "boom":
				res, _ := json.Marshal(map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "deliberate"},
					},
					"isError": true,
				})
				writeResp(out, jResp{JSONRPC: "2.0", ID: req.ID, Result: res})
			default:
				writeErr(out, req.ID, -32601, "unknown tool: "+p.Name)
			}

		case "shutdown":
			return

		default:
			writeErr(out, req.ID, -32601, "method not found: "+req.Method)
		}
	}
}

func writeResp(w *bufio.Writer, r jResp) {
	raw, err := json.Marshal(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mockmcp: encode resp:", err)
		return
	}
	_, _ = w.Write(raw)
	_ = w.WriteByte('\n')
	_ = w.Flush()
}

func writeErr(w *bufio.Writer, id *int64, code int, msg string) {
	writeResp(w, jResp{JSONRPC: "2.0", ID: id, Error: &jErr{Code: code, Message: msg}})
}
