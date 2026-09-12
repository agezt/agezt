// SPDX-License-Identifier: MIT

// MCP client: mcpClient + lifecycle + handshake + list/call/notify + death/lifecycle handlers.
// Code extracted from mcp.go during the Day-101 god-file split.
// Public API unchanged.
package main


import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"encoding/json"
)

func (m *mcpClient) listResources(ctx context.Context) ([]mcpResource, error) {
	raw, err := m.call(ctx, "resources/list", json.RawMessage(`{}`))
	if err != nil {
		// MCP method-not-found surfaces as a JSON-RPC error with
		// code -32601; the bridge maps that to nil/nil so callers
		// can detect "server has no resources surface" without
		// special-casing the error string.
		if strings.Contains(err.Error(), "-32601") {
			return nil, nil
		}
		return nil, err
	}
	var r mcpResourcesListResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("parse resources/list: %w", err)
	}
	return r.Resources, nil
}

// readResource fetches one resource by URI via MCP `resources/read`.
// Returns the contents array verbatim — the bridge handler
// flattens them the same way it flattens tools/call content blocks.
func (m *mcpClient) readResource(ctx context.Context, uri string) ([]mcpResourceContent, error) {
	p, err := json.Marshal(mcpResourcesReadParams{URI: uri})
	if err != nil {
		return nil, fmt.Errorf("marshal resources/read params: %w", err)
	}
	raw, err := m.call(ctx, "resources/read", p)
	if err != nil {
		return nil, err
	}
	var r mcpResourcesReadResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("parse resources/read: %w", err)
	}
	return r.Contents, nil
}

// newMCPClient is the transport-agnostic constructor (M1.MCP-SSE).
// Takes a transportFactory that knows how to open either a stdio
// or SSE connection; runs the MCP handshake on top. Both
// startStdioMCP and startSSEMCP wrap this with their factory.
func newMCPClient(ctx context.Context, factory transportFactory, clientName, protoVersion string) (*mcpClient, error) {
	m := &mcpClient{
		pending: make(map[int64]chan *jsonrpcResp),
		done:    make(chan struct{}),
	}
	tx, err := factory(ctx, m)
	if err != nil {
		return nil, err
	}
	m.tx = tx

	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := m.handshake(initCtx, clientName, protoVersion); err != nil {
		// Best-effort tear-down — the transport's read loop will
		// already have marked us dead if the child / connection
		// died mid-handshake, but on protocol errors we still need
		// to kill the underlying process / close the HTTP request.
		m.close()
		return nil, fmt.Errorf("handshake: %w", err)
	}
	return m, nil
}

// startMCP spawns a stdio MCP server and returns a ready client.
// Thin wrapper around newMCPClient + newStdioTransport so the
// existing call site in main.go is unchanged.
func startMCP(ctx context.Context, path string, args []string, clientName, protoVersion string) (*mcpClient, error) {
	factory := func(_ context.Context, deliver transportDeliver) (transport, error) {
		return newStdioTransport(path, args, deliver)
	}
	c, err := newMCPClient(ctx, factory, clientName, protoVersion)
	if err != nil {
		return nil, fmt.Errorf("stdio mcp %q: %w", path, err)
	}
	return c, nil
}

// startSSEMCP opens an HTTP+SSE MCP session and returns a ready
// client (M1.MCP-SSE). The SSE GET is opened immediately; the
// handshake runs once the server's `endpoint` event has arrived.
func startSSEMCP(ctx context.Context, sseURL, clientName, protoVersion string) (*mcpClient, error) {
	factory := func(fctx context.Context, deliver transportDeliver) (transport, error) {
		return newSSETransport(fctx, sseURL, deliver)
	}
	c, err := newMCPClient(ctx, factory, clientName, protoVersion)
	if err != nil {
		return nil, fmt.Errorf("sse mcp %q: %w", sseURL, err)
	}
	return c, nil
}

// handshake runs the two-message MCP startup: a request/response
// `initialize`, then a notification `notifications/initialized`.
// The MCP spec requires the notification — many servers stay in
// "starting" state until they receive it and reject every
// subsequent call.
func (m *mcpClient) handshake(ctx context.Context, clientName, protoVersion string) error {
	initRaw, err := json.Marshal(mcpInitParams{
		ProtocolVersion: protoVersion,
		// Empty capabilities object (not nil) — MCP spec requires
		// the field even when we advertise nothing. We don't
		// support sampling/roots/etc. on the client side.
		Capabilities: map[string]any{},
		ClientInfo: mcpClientInfo{
			Name:    clientName,
			Version: "1.0",
		},
	})
	if err != nil {
		return fmt.Errorf("marshal init params: %w", err)
	}
	if _, err := m.call(ctx, "initialize", initRaw); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if err := m.notify("notifications/initialized", nil); err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}
	return nil
}

func (m *mcpClient) listTools(ctx context.Context) ([]mcpTool, error) {
	// MCP allows omitting params for tools/list (or sending `{}`).
	// Some servers reject a missing params field; send an explicit
	// empty object for maximum compatibility.
	raw, err := m.call(ctx, "tools/list", json.RawMessage(`{}`))
	if err != nil {
		return nil, err
	}
	var r mcpToolsListResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("parse tools/list result: %w", err)
	}
	return r.Tools, nil
}

func (m *mcpClient) callTool(ctx context.Context, name string, args json.RawMessage) (mcpToolsCallResult, error) {
	if len(args) == 0 {
		// MCP requires `arguments` to be an object even for zero-arg
		// tools. Agezt allows the agent to pass a raw `null` (or
		// nothing) for such tools; normalise here.
		args = json.RawMessage(`{}`)
	}
	p, err := json.Marshal(mcpToolsCallParams{Name: name, Arguments: args})
	if err != nil {
		return mcpToolsCallResult{}, fmt.Errorf("marshal tools/call params: %w", err)
	}
	raw, err := m.call(ctx, "tools/call", p)
	if err != nil {
		return mcpToolsCallResult{}, err
	}
	var r mcpToolsCallResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return mcpToolsCallResult{}, fmt.Errorf("parse tools/call result: %w", err)
	}
	return r, nil
}

// call mints a JSON-RPC id, sends the request, and waits on the
// pending-map channel for the matching response.
//
// Why ctx-cancellable: the agezt host wraps every tool/invoke in
// its own context (default 2-minute timeout, plus client-cancel
// propagation). The bridge's invoke handler creates a child ctx and
// passes it here, so a host-side cancel cuts the wait — though it
// doesn't cancel the MCP server's in-progress work. MCP itself has
// no cancellation message in 2024-11-05; the server will keep
// computing and discard the result.
func (m *mcpClient) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if m.dead.Load() {
		return nil, fmt.Errorf("mcp connection dead: %w", m.deathError())
	}
	id := m.nextID.Add(1)
	ch := make(chan *jsonrpcResp, 1)
	m.mu.Lock()
	m.pending[id] = ch
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.pending, id)
		m.mu.Unlock()
	}()

	req := jsonrpcReq{JSONRPC: "2.0", ID: &id, Method: method, Params: params}
	if err := m.tx.send(req); err != nil {
		return nil, err
	}
	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("mcp connection lost: %w", m.deathError())
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: server error %d: %s", method, resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-m.done:
		// Connection died while we were waiting (markDead, M428). The pending
		// channels are no longer closed, so this is how a waiting caller wakes.
		return nil, fmt.Errorf("mcp connection lost: %w", m.deathError())
	case <-ctx.Done():
		// Best-effort cancellation notification (M1.jj — MCP v2 bridge).
		// MCP 2024-11-05 has no cancellation; 2024-12+ adds
		// `notifications/cancelled` with the request id. We send it
		// unconditionally — servers that don't understand it ignore
		// the notification, those that do can abort the in-flight
		// work. Either way the bridge stops waiting.
		cancelParams, _ := json.Marshal(map[string]any{
			"requestId": id,
			"reason":    ctx.Err().Error(),
		})
		_ = m.notify("notifications/cancelled", cancelParams)
		return nil, fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

// notify sends a JSON-RPC notification (no id; no response
// expected). Used for `notifications/initialized` in handshake
// and for `notifications/cancelled` on ctx cancel.
func (m *mcpClient) notify(method string, params json.RawMessage) error {
	if m.dead.Load() {
		return fmt.Errorf("mcp connection dead: %w", m.deathError())
	}
	req := jsonrpcReq{JSONRPC: "2.0", Method: method, Params: params}
	return m.tx.send(req)
}

// handleNotification surfaces MCP server-to-client notifications
// to operator-visible stderr (M1.jj). The agezt host's plugin
// logger picks up bridge stderr under `[plugin:<prefix>]`, so
// progress / log notifications from the MCP server land in the
// daemon's log without extra wiring.
//
// Only two kinds are special-cased; everything else is dropped:
//
//   - notifications/progress  — tool-call progress updates from
//     long-running operations.
//   - notifications/message   — structured server logs.
func handleNotification(line []byte) {
	var nf struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params,omitempty"`
	}
	if err := json.Unmarshal(line, &nf); err != nil {
		return
	}
	switch nf.Method {
	case "notifications/progress":
		var p struct {
			ProgressToken any     `json:"progressToken"`
			Progress      float64 `json:"progress"`
			Total         float64 `json:"total,omitempty"`
			Message       string  `json:"message,omitempty"`
		}
		_ = json.Unmarshal(nf.Params, &p)
		if p.Total > 0 {
			fmt.Fprintf(os.Stderr, "mcp progress [%v]: %.0f/%.0f %s\n",
				p.ProgressToken, p.Progress, p.Total, p.Message)
		} else {
			fmt.Fprintf(os.Stderr, "mcp progress [%v]: %.0f %s\n",
				p.ProgressToken, p.Progress, p.Message)
		}
	case "notifications/message":
		var p struct {
			Level  string `json:"level"`
			Logger string `json:"logger,omitempty"`
			Data   any    `json:"data,omitempty"`
		}
		_ = json.Unmarshal(nf.Params, &p)
		fmt.Fprintf(os.Stderr, "mcp log [%s] %s: %v\n", p.Level, p.Logger, p.Data)
	}
}

func (m *mcpClient) markDead(cause error) {
	if !m.dead.CompareAndSwap(false, true) {
		return
	}
	m.deathMu.Lock()
	if m.deathErr == nil {
		m.deathErr = cause
	}
	m.deathMu.Unlock()
	// Signal death via the shared channel and clear pending WITHOUT closing the
	// per-call channels — closing them raced the read goroutine's send and panicked
	// the bridge (M428). Waiting callers wake on <-m.done.
	close(m.done)
	m.mu.Lock()
	for id := range m.pending {
		delete(m.pending, id)
	}
	m.mu.Unlock()
}

func (m *mcpClient) deathError() error {
	m.deathMu.Lock()
	defer m.deathMu.Unlock()
	if m.deathErr == nil {
		return errors.New("unknown cause")
	}
	return m.deathErr
}

// close best-effort terminates the MCP session. Delegates to the
// transport for the actual teardown (stdio: stop the child;
// SSE: cancel the in-flight GET + drop the POST URL). Idempotent.
func (m *mcpClient) close() {
	if m.tx != nil {
		m.tx.close()
	}
	m.markDead(errors.New("closed"))
}
