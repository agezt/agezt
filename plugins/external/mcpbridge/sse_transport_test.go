// SPDX-License-Identifier: MIT

package main

// SSE transport tests (M1.MCP-SSE). Use httptest to stand in for a
// remote MCP server: one handler streams the `text/event-stream`
// GET (with an `endpoint` event then JSON-RPC replies fed by a
// channel from the POST handler), another handler receives the
// POSTs and turns them into queued events.
//
// We deliberately don't run the full MCP handshake through these
// tests — startSSEMCP exercises that path end-to-end and is too
// timing-sensitive for a unit test. These tests target the
// transport layer's correctness: endpoint event resolution,
// request round-trip via the deliver callback, and clean shutdown.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureDeliver records every transport callback so the test can
// assert on them.
type captureDeliver struct {
	mu     sync.Mutex
	resps  []*jsonrpcResp
	notifs [][]byte
	dead   error
	deadCh chan struct{}
}

func newCaptureDeliver() *captureDeliver {
	return &captureDeliver{deadCh: make(chan struct{})}
}

func (c *captureDeliver) onResponse(r *jsonrpcResp) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resps = append(c.resps, r)
}

func (c *captureDeliver) onNotification(raw []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Copy: the transport's read buffer may be reused.
	cp := make([]byte, len(raw))
	copy(cp, raw)
	c.notifs = append(c.notifs, cp)
}

func (c *captureDeliver) onTransportDead(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead == nil {
		c.dead = err
		close(c.deadCh)
	}
}

// mockMCPServer is a tiny SSE server: it accepts a single SSE GET
// (streams events from `events` channel) and POSTs (pushes a
// reply onto `events` keyed by request id).
type mockMCPServer struct {
	srv          *httptest.Server
	events       chan string     // SSE event lines to write
	receivedPOST chan jsonrpcReq // POSTs the test can inspect
	endpointPath string          // POST URL path the server announces
}

func newMockMCPServer(t *testing.T) *mockMCPServer {
	t.Helper()
	m := &mockMCPServer{
		events:       make(chan string, 32),
		receivedPOST: make(chan jsonrpcReq, 32),
		endpointPath: "/messages",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)

		// Announce the POST endpoint (relative URL — exercises the
		// transport's URL-resolution path).
		fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", m.endpointPath)
		if flusher != nil {
			flusher.Flush()
		}
		// Stream events until the client disconnects.
		for {
			select {
			case <-r.Context().Done():
				return
			case ev := <-m.events:
				fmt.Fprint(w, ev)
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req jsonrpcReq
		_ = json.Unmarshal(body, &req)
		m.receivedPOST <- req
		w.WriteHeader(202)
	})
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

// pushReply queues a JSON-RPC response event onto the SSE stream.
func (m *mockMCPServer) pushReply(id int64, result any) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
	m.events <- fmt.Sprintf("event: message\ndata: %s\n\n", body)
}

// pushNotification queues an id-less notification onto the SSE stream.
func (m *mockMCPServer) pushNotification(method string, params any) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	m.events <- fmt.Sprintf("event: message\ndata: %s\n\n", body)
}

func TestSSETransport_EndpointEventResolvesRelativeURL(t *testing.T) {
	srv := newMockMCPServer(t)
	dc := newCaptureDeliver()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := newSSETransport(ctx, srv.srv.URL+"/sse", dc)
	if err != nil {
		t.Fatalf("newSSETransport: %v", err)
	}
	defer tx.close()

	// The transport must have resolved the relative `/messages` path
	// against the SSE URL's origin before send() works.
	id := int64(1)
	go srv.pushReply(id, map[string]string{"hello": "world"})
	if err := tx.send(jsonrpcReq{JSONRPC: "2.0", ID: &id, Method: "ping"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	// Server received our POST.
	select {
	case got := <-srv.receivedPOST:
		if got.Method != "ping" {
			t.Errorf("server saw method=%q want ping", got.Method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server never received the POST")
	}

	// Client received the reply via SSE.
	deadline := time.After(2 * time.Second)
	for {
		dc.mu.Lock()
		got := len(dc.resps)
		dc.mu.Unlock()
		if got >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("response never delivered")
		case <-time.After(20 * time.Millisecond):
		}
	}
	dc.mu.Lock()
	defer dc.mu.Unlock()
	if dc.resps[0].ID == nil || *dc.resps[0].ID != id {
		t.Errorf("delivered resp id mismatch: %+v", dc.resps[0])
	}
}

func TestSSETransport_DispatchesNotifications(t *testing.T) {
	srv := newMockMCPServer(t)
	dc := newCaptureDeliver()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := newSSETransport(ctx, srv.srv.URL+"/sse", dc)
	if err != nil {
		t.Fatalf("newSSETransport: %v", err)
	}
	defer tx.close()

	srv.pushNotification("notifications/message", map[string]any{
		"level":  "info",
		"logger": "test",
		"data":   "hi",
	})

	deadline := time.After(2 * time.Second)
	for {
		dc.mu.Lock()
		got := len(dc.notifs)
		dc.mu.Unlock()
		if got >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("notification never delivered")
		case <-time.After(20 * time.Millisecond):
		}
	}
	dc.mu.Lock()
	defer dc.mu.Unlock()
	if !strings.Contains(string(dc.notifs[0]), "notifications/message") {
		t.Errorf("notification body unexpected: %s", dc.notifs[0])
	}
}

func TestSSETransport_CloseEndsReadLoop(t *testing.T) {
	srv := newMockMCPServer(t)
	dc := newCaptureDeliver()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := newSSETransport(ctx, srv.srv.URL+"/sse", dc)
	if err != nil {
		t.Fatalf("newSSETransport: %v", err)
	}
	tx.close()

	select {
	case <-dc.deadCh:
		// expected — transport reported death after close cancelled
		// the in-flight SSE GET.
	case <-time.After(2 * time.Second):
		t.Fatal("transport never signalled onTransportDead after close")
	}

	// send after close must error rather than block.
	id := int64(99)
	if err := tx.send(jsonrpcReq{JSONRPC: "2.0", ID: &id, Method: "x"}); err == nil {
		t.Error("send after close should error")
	}
}

func TestSSETransport_EndpointTimeoutSurfacesError(t *testing.T) {
	// A server that opens the SSE stream but never sends `endpoint`
	// must cause newSSETransport to error out via the caller's ctx.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		// Keep the connection open without sending the endpoint event.
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := newSSETransport(ctx, srv.URL, newCaptureDeliver())
	if err == nil {
		t.Fatal("expected error when endpoint event never arrives")
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("err should mention endpoint: %v", err)
	}
}
