// SPDX-License-Identifier: MIT

// Package mcp owns the public MCP client types: ToolDef,
// Conn, Dialer, HTTPDialer, and the JSON-RPC wire shapes
// (rpcRequest / rpcResponse). The clientConn implementation
// (type + Dial + newClientConn + every Conn method) lives
// in client_conn.go. The env helpers (appendEnv /
// scrubbedEnv / isSecretName) live in client_helpers.go.
// Day-211 god-file split. Public API unchanged.
package mcp

import (
	"context"
	"encoding/json"
	"time"
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
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// Conn is a live attachment to one MCP server. Implemented by the stdio
// client here; an interface so the runtime (and tests) never depend on a
// real child process.
type Conn interface {
	// Tools returns the tool list discovered at attach time.
	Tools() []ToolDef
	// Call forwards one tools/call and returns the flattened text content
	// plus the server's own isError verdict.
	Call(ctx context.Context, tool string, args json.RawMessage) (string, bool, error)
	// Close detaches: asks the child to exit, then kills it. Idempotent.
	Close() error
}

// Dialer spawns + handshakes one server. The runtime takes it as a seam so
// tests can attach fakes; Dial is the production implementation. env is the
// server's opt-in extra environment (M898), injected on top of the scrubbed base.
type Dialer func(ctx context.Context, command string, args []string, env map[string]string) (Conn, error)

// HTTPDialer handshakes one REMOTE server over Streamable HTTP (M904). The
// runtime takes it as a seam so tests can attach fakes; DialHTTP is the
// production implementation. headers are the operator's opt-in request headers
// (e.g. an Authorization bearer token), applied to every request.
type HTTPDialer func(ctx context.Context, url string, headers map[string]string) (Conn, error)

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
