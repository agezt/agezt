// SPDX-License-Identifier: MIT

// http_helpers.go: round-trip / notify / postLocked / applyHeaders /
// readResponse / readSSEResponse split off from http.go during the Day 211
// god-file refactor (#133). Public API unchanged.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)


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
