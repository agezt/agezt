// SPDX-License-Identifier: MIT

// MCP wire types (jsonrpcReq/Resp/Error + mcpInitParams/mcpTool/mcpResource/... etc.).
// Code extracted from mcp.go during the Day-101 god-file split.
// Public API unchanged.
package main


import (
	"sync"

	"encoding/json"
	"sync/atomic"
)


// mcpClient is a minimal MCP client over a child process's stdio.
//
// **Wire shape.** MCP is JSON-RPC 2.0 with `\n` framing — request
// per line, response per line. We support the handshake
// (`initialize` + `notifications/initialized`) and `tools/list` /
// `tools/call`. Resources, prompts, and sampling are out of scope
// for v1 of the bridge (see the package doc).
//
// **Concurrency.** The agezt side of the bridge serialises calls,
// so the client also assumes one in-flight call at a time. Even so,
// we use the same pending-map + correlation-id machinery as the
// kernel's plugin host — that way a future bridge that does
// parallelise (e.g. progress callbacks alongside an in-flight call)
// doesn't need rewriting. Notifications (id-less responses from the
// server) are silently dropped here — they're not part of v1's
// surface.
type mcpClient struct {
	tx transport

	mu      sync.Mutex
	pending map[int64]chan *jsonrpcResp

	nextID atomic.Int64
	dead   atomic.Bool
	// done is closed once by markDead to signal the connection died. Callers select
	// on it instead of relying on their pending channel being closed — closing those
	// from markDead raced the read goroutine's send (send-on-closed-channel panic that
	// crashed the bridge); now the read goroutine's send is non-blocking and the
	// pending channels are never closed (M428).
	done chan struct{}

	deathMu  sync.Mutex
	deathErr error
}

// Compile-time check that mcpClient satisfies transportDeliver.
var _ transportDeliver = (*mcpClient)(nil)

// onResponse routes an id-bearing JSON-RPC response to the matching
// pending channel. Called by the active transport's read goroutine.
func (m *mcpClient) onResponse(resp *jsonrpcResp) {
	if resp.ID == nil {
		// Defensive — onResponse should only receive id-bearing
		// frames. Transports route id-less frames to onNotification.
		return
	}
	m.mu.Lock()
	ch, ok := m.pending[*resp.ID]
	m.mu.Unlock()
	if !ok {
		// Stale id (caller timed out and is no longer listening).
		return
	}
	// Non-blocking send (M428): the channel is cap-1 and single-use, so the one
	// legitimate response always fits. A duplicate response carrying the same id (a
	// buggy/hostile server) must NOT block this read goroutine — dropping it keeps the
	// read loop alive instead of wedging every future call. The channel is never
	// closed (markDead signals death via m.done), so this can't panic.
	select {
	case ch <- resp:
	default:
	}
}

// onNotification is the transport callback for id-less frames.
// Delegates to the same handleNotification dispatcher both
// transports share, so MCP progress + log notifications surface
// regardless of how they arrived.
func (m *mcpClient) onNotification(raw []byte) {
	handleNotification(raw)
}

// onTransportDead is the transport callback for "no more frames
// will arrive." Flips dead, unblocks every pending caller, records
// the cause for the deathError() helper.
func (m *mcpClient) onTransportDead(cause error) {
	m.markDead(cause)
}

// ----- JSON-RPC 2.0 envelopes ------------------------------------

type jsonrpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"` // pointer to omit for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// ----- MCP method-specific shapes --------------------------------

type mcpInitParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      mcpClientInfo  `json:"clientInfo"`
}

type mcpClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type mcpToolsListResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpToolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// mcpContentItem is one block of MCP tool-call output. MCP defines
// a tagged union (`type` discriminator) with `text` / `image` /
// `resource` variants. We only round-trip the discriminator + the
// text payload; other fields surface as the placeholder annotation
// in flattenContent.
type mcpContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type mcpToolsCallResult struct {
	Content []mcpContentItem `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

// mcpResource is one entry from MCP `resources/list` (M1.ww).
// Only the operator-useful fields are decoded.
type mcpResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type mcpResourcesListResult struct {
	Resources []mcpResource `json:"resources"`
}

type mcpResourcesReadParams struct {
	URI string `json:"uri"`
}

// mcpResourceContent is one block from `resources/read`. URI is
// echoed back; text is the decoded body for `text/*` MIME types;
// blob is base64 for binary types (we don't decode).
type mcpResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

type mcpResourcesReadResult struct {
	Contents []mcpResourceContent `json:"contents"`
}

// listResources queries MCP `resources/list` for the resource
// catalog (M1.ww). Returns nil + nil error when the server
// doesn't support resources — surfacing the empty list lets
// the bridge skip registering the synthetic read_resource tool
// without erroring on every spawn against tool-only servers.
