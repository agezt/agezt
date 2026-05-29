// SPDX-License-Identifier: MIT

package main

// sseTransport carries MCP frames over the HTTP+SSE transport
// (M1.MCP-SSE). Two streams glued together:
//
//   1. **GET <sseURL>** — long-lived `text/event-stream` connection.
//      First event is `event: endpoint\ndata: <postURL>` — that URL
//      is where we POST requests. Subsequent events carry the
//      JSON-RPC responses + server notifications.
//
//   2. **POST <postURL>** — one HTTP POST per JSON-RPC request,
//      Content-Type application/json. The body is the marshalled
//      jsonrpcReq. The HTTP response is typically 202 Accepted;
//      the actual JSON-RPC reply arrives on the SSE stream.
//
// We block in newSSETransport until the endpoint event arrives,
// so callers can immediately start sending — matches the
// stdioTransport's "constructor returns ready-to-use" contract.
//
// **What this is NOT.** Not the newer "Streamable HTTP" transport
// (2025-03 spec) where a single POST may stream multiple responses
// via Transfer-Encoding: chunked OR text/event-stream. That'll be
// a third transport (sse_streamable_transport.go) when operators
// hit a server that requires it; the HTTP+SSE flavor here is what
// most production MCP servers run today.

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
	"os"
	"strings"
	"sync"
	"time"
)

type sseTransport struct {
	httpClient *http.Client
	sseURL     string
	deliver    transportDeliver

	mu      sync.Mutex
	postURL string // set after the endpoint event arrives
	closed  bool

	// readCancel cancels the in-flight SSE GET when close() runs.
	readCancel context.CancelFunc

	// endpointReady fires once the first endpoint event has been
	// processed; lets newSSETransport block until POSTs are routable.
	endpointReady chan struct{}
	endpointErr   error
}

// newSSETransport opens the SSE stream, waits for the endpoint
// event, and starts the read loop. Returns an error if the GET
// fails or the endpoint event doesn't arrive within timeout.
func newSSETransport(ctx context.Context, sseURL string, deliver transportDeliver) (*sseTransport, error) {
	if _, err := url.Parse(sseURL); err != nil {
		return nil, fmt.Errorf("sse mcp: bad URL %q: %w", sseURL, err)
	}
	t := &sseTransport{
		httpClient:    &http.Client{
			// No client-side timeout: the SSE stream is long-lived
			// by design. Per-request POSTs use a fresh client below
			// with their own context.
		},
		sseURL:        sseURL,
		deliver:       deliver,
		endpointReady: make(chan struct{}),
	}

	// Start the SSE read loop. Use a derived ctx so close() can
	// cancel the in-flight GET cleanly.
	readCtx, cancel := context.WithCancel(context.Background())
	t.readCancel = cancel
	go t.readLoop(readCtx)

	// Block until the endpoint event lands or the caller's ctx
	// expires (initialize timeout governs how long we wait).
	select {
	case <-t.endpointReady:
		if t.endpointErr != nil {
			t.close()
			return nil, t.endpointErr
		}
		return t, nil
	case <-ctx.Done():
		t.close()
		return nil, fmt.Errorf("sse mcp: timed out waiting for endpoint event: %w", ctx.Err())
	}
}

func (t *sseTransport) send(req jsonrpcReq) error {
	t.mu.Lock()
	postURL := t.postURL
	closed := t.closed
	t.mu.Unlock()
	if closed {
		return errors.New("sse mcp: transport closed")
	}
	if postURL == "" {
		return errors.New("sse mcp: endpoint not yet announced by server")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("sse mcp: marshal request: %w", err)
	}

	// Per-POST timeout is generous (90s) so a slow server during
	// startup doesn't kill the call; the agezt host's own
	// invoke timeout (2m) still bounds the upper end.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", postURL, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("sse mcp: build POST: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sse mcp: POST: %w", err)
	}
	defer resp.Body.Close()
	// Drain body so connection can be reused. Server typically
	// returns 202 Accepted with empty body; the JSON-RPC reply
	// arrives on the SSE stream, not in this response.
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("sse mcp: POST returned %s", resp.Status)
	}
	return nil
}

func (t *sseTransport) close() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	t.mu.Unlock()
	if t.readCancel != nil {
		t.readCancel()
	}
}

// readLoop opens the SSE GET and dispatches each event. Returns
// when the stream ends or close() cancels the context.
func (t *sseTransport) readLoop(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, "GET", t.sseURL, nil)
	if err != nil {
		t.signalEndpoint(fmt.Errorf("sse mcp: build GET: %w", err))
		t.deliver.onTransportDead(err)
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		t.signalEndpoint(fmt.Errorf("sse mcp: GET: %w", err))
		t.deliver.onTransportDead(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err := fmt.Errorf("sse mcp: GET returned %s", resp.Status)
		t.signalEndpoint(err)
		t.deliver.onTransportDead(err)
		return
	}

	br := bufio.NewReader(resp.Body)
	var eventType, dataBuf string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			// EOF or canceled context — both are normal terminations.
			t.signalEndpoint(fmt.Errorf("sse mcp: stream ended before endpoint event: %w", err))
			t.deliver.onTransportDead(fmt.Errorf("sse stream ended: %w", err))
			return
		}
		line = strings.TrimRight(line, "\r\n")
		// Blank line dispatches the accumulated event.
		if line == "" {
			if dataBuf != "" {
				t.dispatchEvent(eventType, dataBuf)
			}
			eventType, dataBuf = "", ""
			continue
		}
		// Comment lines (start with ":") — SSE keep-alives, ignore.
		if strings.HasPrefix(line, ":") {
			continue
		}
		// "field: value" — parse SSE field name + value.
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			// Per SSE spec, lines without ":" are field names with
			// empty values. We don't care about those.
			continue
		}
		// Spec says a single leading space after the colon is stripped.
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			eventType = value
		case "data":
			if dataBuf == "" {
				dataBuf = value
			} else {
				// Multi-line data: concatenate with newlines.
				dataBuf += "\n" + value
			}
		}
		// id / retry fields: ignored — we don't implement reconnect.
	}
}

// dispatchEvent routes one parsed SSE event. Two event types matter:
//   - "endpoint": one-shot; the data is the POST URL. We resolve
//     it against the SSE URL if relative, store it, and wake the
//     constructor.
//   - "message" (default): the data is one JSON-RPC frame.
func (t *sseTransport) dispatchEvent(eventType, data string) {
	switch eventType {
	case "endpoint":
		postURL := strings.TrimSpace(data)
		if !strings.HasPrefix(postURL, "http://") && !strings.HasPrefix(postURL, "https://") {
			// Relative URL — resolve against the SSE URL's origin.
			base, err := url.Parse(t.sseURL)
			if err != nil {
				t.signalEndpoint(fmt.Errorf("sse mcp: bad sseURL for endpoint resolution: %w", err))
				return
			}
			rel, err := url.Parse(postURL)
			if err != nil {
				t.signalEndpoint(fmt.Errorf("sse mcp: bad endpoint URL %q: %w", postURL, err))
				return
			}
			postURL = base.ResolveReference(rel).String()
		}
		t.mu.Lock()
		t.postURL = postURL
		t.mu.Unlock()
		t.signalEndpoint(nil)

	case "", "message":
		raw := []byte(data)
		var resp jsonrpcResp
		if err := json.Unmarshal(raw, &resp); err != nil {
			fmt.Fprintf(os.Stderr, "mcpbridge: bad sse message: %v\n", err)
			return
		}
		if resp.ID == nil {
			t.deliver.onNotification(raw)
			return
		}
		t.deliver.onResponse(&resp)
	}
}

// signalEndpoint wakes the constructor exactly once. Subsequent
// calls are no-ops — needed because the read loop may hit an
// error after the endpoint already arrived, in which case the
// error path should not re-close a closed channel.
func (t *sseTransport) signalEndpoint(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	select {
	case <-t.endpointReady:
		return // already signaled
	default:
	}
	t.endpointErr = err
	close(t.endpointReady)
}

