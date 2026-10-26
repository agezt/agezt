// SPDX-License-Identifier: MIT

package mcp

// Provenance: Package mcp owns the public MCP client types: ToolDef, Conn, Dialer,
//             HTTPDialer, and the JSON-RPC wire shapes (rpcRequest / rpcResponse).
//             The clientConn implementation (type + Dial + newClientConn + every
//             Conn method) lives in client_conn.go. The env helper (appendEnv)
//             lives in client_helpers.go; the base env is sandbox.IsolatedEnv. Day-211
//             god-file split. Public API unchanged.

import (
	"encoding/json"
	"time"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
)

const (
	// maxFrameBytes caps one JSON-RPC line from the server (matches the
	// external bridge's M185 bound) so a hostile server can't OOM the daemon.
	maxFrameBytes = 16 << 20
	// handshakeTimeout bounds initialize + tools/list at attach time.
	handshakeTimeout = 15 * time.Second
	// callTimeout bounds one forwarded tools/call when the run's own context
	// carries no earlier deadline.
	callTimeout = 90 * time.Second

	protocolVersion = "2024-11-05"
	clientName      = "agezt"
)

// ToolDef is one tool an attached server advertises.
//
// The row lives in the contract layer (kernel/contract/mcpapi) (W3.58): the
// layers above the module — the kernel's bridge, the app layer — speak it, and
// they may not name the module. This package keeps the name, so every
// existing caller is unchanged.
type ToolDef = mcpapi.ToolDef

// Conn is a live attachment to one MCP server.
//
// The port lives in the contract layer (kernel/contract/mcpapi) (W3.58) — the
// kernel's bridge holds live attachments and may not name the module. It is an
// interface so the runtime (and tests) never depend on a real child process.
type Conn = mcpapi.Conn

// Dialer spawns + handshakes one server. The runtime takes it as a seam so
// tests can attach fakes; Dial is the production implementation. env is the
// server's opt-in extra environment (M898), injected on top of the scrubbed base.
type Dialer = mcpapi.Dialer

// HTTPDialer handshakes one REMOTE server over Streamable HTTP (M904). The
// runtime takes it as a seam so tests can attach fakes; DialHTTP is the
// production implementation. headers are the operator's opt-in request headers
// (e.g. an Authorization bearer token), applied to every request.
type HTTPDialer = mcpapi.HTTPDialer

// jsonrpc wire shapes (only what this client speaks).
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"` // nil = notification
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     *int64          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
