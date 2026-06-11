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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
		client:   &http.Client{Timeout: callTimeout},
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

// roundTrip POSTs one JSON-RPC request and returns ITS response, handling both
// a single application/json body and a text/event-stream reply. Serialized
// under mu so at most one id is outstanding.
func (c *httpConn) roundTrip(ctx context.Context, method string, params any, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID.Add(1)
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params})
	if err != nil {
		return err
	}
	resp, err := c.postLocked(ctx, body)
	if err != nil {
		return fmt.Errorf("mcp http: %s: %w", method, err)
	}
	defer resp.Body.Close()
	// The initialize response carries the session id we must echo on every
	// later request; capture it whenever present.
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.sessionID = sid
	}
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("mcp http: %s: server returned %s: %s", method, resp.Status, strings.TrimSpace(string(snippet)))
	}
	rr, err := c.readResponse(resp, id)
	if err != nil {
		return fmt.Errorf("mcp http: %s: %w", method, err)
	}
	if rr.Error != nil {
		return fmt.Errorf("mcp http: %s: server error %d: %s", method, rr.Error.Code, rr.Error.Message)
	}
	if out != nil {
		if err := json.Unmarshal(rr.Result, out); err != nil {
			return fmt.Errorf("mcp http: %s: parse result: %w", method, err)
		}
	}
	return nil
}

// notify POSTs a JSON-RPC notification (no id, no reply expected). The server
// answers 202 Accepted (or 200) with no JSON-RPC body.
func (c *httpConn) notify(ctx context.Context, method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return err
	}
	resp, err := c.postLocked(ctx, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return nil
}

// postLocked issues one POST. Caller holds mu (so sessionID is read race-free).
func (c *httpConn) postLocked(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	c.applyHeaders(req, c.sessionID)
	return c.client.Do(req)
}

// applyHeaders sets the protocol-version + session headers and overlays the
// operator's opt-in headers last (so they can't be silently dropped, and an
// explicit Authorization always wins).
func (c *httpConn) applyHeaders(req *http.Request, sid string) {
	req.Header.Set("MCP-Protocol-Version", httpProtocolVersion)
	if sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
}

// readResponse decodes the reply for request id, dispatching on content type.
// A JSON body is one response; an SSE body is scanned for the message whose id
// matches (skipping notifications and unrelated ids), like the stdio reader.
func (c *httpConn) readResponse(resp *http.Response, id int64) (*rpcResponse, error) {
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.HasPrefix(ct, "text/event-stream") {
		return readSSEResponse(resp.Body, id)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxFrameBytes))
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("empty response body")
	}
	var rr rpcResponse
	if err := json.Unmarshal(raw, &rr); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &rr, nil
}

// readSSEResponse scans an event stream and returns the first JSON-RPC message
// matching id. Notifications and unrelated ids are skipped. The total bytes
// read are bounded — a hostile server must not stream forever.
func readSSEResponse(body io.Reader, id int64) (*rpcResponse, error) {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64*1024), maxFrameBytes)
	var data strings.Builder
	var total int
	flush := func() (*rpcResponse, bool) {
		if data.Len() == 0 {
			return nil, false
		}
		payload := data.String()
		data.Reset()
		var rr rpcResponse
		if err := json.Unmarshal([]byte(payload), &rr); err != nil || rr.ID == nil || *rr.ID != id {
			return nil, false // notification / unrelated id / junk
		}
		return &rr, true
	}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		total += len(line) + 1
		if total > maxFrameBytes {
			return nil, errors.New("sse response exceeded frame cap")
		}
		if line == "" { // blank line dispatches the accumulated event
			if rr, ok := flush(); ok {
				return rr, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") { // comment / keep-alive
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok || field != "data" {
			continue // we only care about data: lines
		}
		value = strings.TrimPrefix(value, " ")
		if data.Len() > 0 {
			data.WriteByte('\n')
		}
		data.WriteString(value)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if rr, ok := flush(); ok { // stream ended without a trailing blank line
		return rr, nil
	}
	return nil, errors.New("sse stream ended before a matching response")
}
