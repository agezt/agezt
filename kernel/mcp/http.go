// SPDX-License-Identifier: MIT

package mcp

// A minimal MCP client over the Streamable HTTP transport (MCP 2025-03-26):
// the same JSON-RPC handshake the stdio client speaks, but framed over HTTP
// instead of a child process's pipes. One endpoint URL; each JSON-RPC request
// is POSTed there, and the reply comes back either as a single
// `application/json` body or as a `text/event-stream` carrying one or more
// JSON-RPC messages. This is the transport popular remote servers run today
// (e.g. hosted GitHub/Linear endpoints) — the registry's parity with stdio
// (#39).
//
// Scope mirrors the stdio client deliberately: request/reply only. We do NOT
// open the optional long-lived GET listening stream (server-initiated
// requests/notifications) — like the stdio client, this one makes no use of
// resources/prompts/sampling, so there's nothing to receive out-of-band.
// Calls are serialized (one outstanding request), frames are size-capped, and
// the operator's opt-in auth headers (M904) ride every request.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agezt/agezt/kernel/netguard"
)


// httpProtocolVersion is what we advertise on the HTTP transport. The
// Streamable HTTP transport was introduced in the 2025-03-26 revision, so a
// server that speaks it negotiates from at least this version; it is also sent
// back as the MCP-Protocol-Version header on every post-handshake request, per
// spec.
const httpProtocolVersion = "2025-03-26"

// httpConn is the production Conn over Streamable HTTP. Calls serialize under
// mu (one outstanding id), matching the stdio client's contract.
type httpConn struct {
	client   *http.Client
	endpoint string
	headers  map[string]string // operator opt-in (e.g. Authorization), M904

	mu        sync.Mutex // serializes round-trips; guards sessionID
	sessionID string     // Mcp-Session-Id, echoed back after initialize
	nextID    atomic.Int64

	tools []ToolDef
}

// DialHTTP completes the MCP handshake + tool discovery against a remote
// Streamable HTTP endpoint. headers (M904) are the operator's explicit opt-in
// request headers — typically an Authorization bearer token — applied to every
// request including the handshake. Unlike the stdio dialer there is no process
// to scrub: the only thing reaching the remote is what the operator put in
// headers plus the JSON-RPC body.
func DialHTTP(ctx context.Context, endpoint string, headers map[string]string) (Conn, error) {
	if _, err := url.Parse(endpoint); err != nil {
		return nil, fmt.Errorf("mcp http: bad url %q: %w", endpoint, err)
	}
	c := &httpConn{
		// Guarded client: every dial and redirect hop is screened by netguard.
		// Loopback/private are allowed (local MCP servers are legitimate), but
		// link-local is refused so a malicious or redirecting endpoint cannot
		// reach the cloud-metadata service (169.254.169.254) — SSRF, CWE-918.
		client:   netguard.New(netguard.AllowLoopback(), netguard.AllowPrivate()).HTTPClient(callTimeout),
		endpoint: endpoint,
		headers:  headers,
	}
	hctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	if err := c.handshake(hctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// handshake: initialize (captures the session id) → initialized notification →
// tools/list. Same sequence as the stdio client.
func (c *httpConn) handshake(ctx context.Context) error {
	var initRes json.RawMessage
	err := c.roundTrip(ctx, "initialize", map[string]any{
		"protocolVersion": httpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": clientName, "version": "1"},
	}, &initRes)
	if err != nil {
		return fmt.Errorf("mcp http: initialize: %w", err)
	}
	if err := c.notify(ctx, "notifications/initialized", nil); err != nil {
		return fmt.Errorf("mcp http: initialized notification: %w", err)
	}
	var listRes struct {
		Tools []ToolDef `json:"tools"`
	}
	if err := c.roundTrip(ctx, "tools/list", map[string]any{}, &listRes); err != nil {
		return fmt.Errorf("mcp http: tools/list: %w", err)
	}
	c.tools = listRes.Tools
	return nil
}

// Tools implements Conn.
func (c *httpConn) Tools() []ToolDef {
	out := make([]ToolDef, len(c.tools))
	copy(out, c.tools)
	return out
}

// Call implements Conn: one tools/call, content flattened to text.
func (c *httpConn) Call(ctx context.Context, tool string, args json.RawMessage) (string, bool, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, callTimeout)
		defer cancel()
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var res struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	err := c.roundTrip(ctx, "tools/call", map[string]any{"name": tool, "arguments": json.RawMessage(args)}, &res)
	if err != nil {
		return "", false, err
	}
	var parts []string
	for _, blk := range res.Content {
		if blk.Text != "" {
			parts = append(parts, blk.Text)
		}
	}
	return strings.Join(parts, "\n"), res.IsError, nil
}

// Close implements Conn: best-effort DELETE to terminate the server session
// (spec-optional), then nothing else to reap — there's no child process.
func (c *httpConn) Close() error {
	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()
	if sid == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.endpoint, nil)
	if err != nil {
		return nil
	}
	c.applyHeaders(req, sid)
	if resp, err := c.client.Do(req); err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	return nil
}
