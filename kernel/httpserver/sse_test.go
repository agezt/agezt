// SPDX-License-Identifier: MIT

package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSSEClientKey_WithPort(t *testing.T) {
	r := &http.Request{RemoteAddr: "192.168.1.1:45678"}
	if key := sseClientKey(r); key != "192.168.1.1" {
		t.Errorf("sseClientKey = %q, want 192.168.1.1", key)
	}
}

func TestSSEClientKey_NoPort(t *testing.T) {
	r := &http.Request{RemoteAddr: "unix-socket-path"}
	if key := sseClientKey(r); key != "unix-socket-path" {
		t.Errorf("sseClientKey(no-port) = %q, want raw addr", key)
	}
}

// TestStartSSE_WritesHeadersAndFrames — the shared writer emits the SSE header
// triple and the three frame shapes every surface uses.
func TestStartSSE_WritesHeadersAndFrames(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	sse, ok := StartSSE(rec, req)
	if !ok {
		t.Fatal("StartSSE refused a fresh client")
	}
	defer sse.Close()

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q", cc)
	}
	_ = sse.Comment("connected")
	_ = sse.WriteJSON(map[string]any{"kind": "open"})
	_ = sse.WriteEvent("token", map[string]any{"text": "hi"})
	_ = sse.WriteData("[DONE]")

	want := ": connected\n\n" +
		"data: {\"kind\":\"open\"}\n\n" +
		"event: token\ndata: {\"text\":\"hi\"}\n\n" +
		"data: [DONE]\n\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// TestStartSSE_CapsPerClient — the process-wide limiter refuses stream N+1 from
// one client with 429 and frees the slot on Close (LD-8: the cap now guards
// every SSE endpoint, not just the webui firehose).
func TestStartSSE_CapsPerClient(t *testing.T) {
	const addr = "10.9.9.9:555"
	var streams []*SSEStream
	defer func() {
		for _, s := range streams {
			s.Close()
		}
	}()
	for i := 0; i < maxSSEPerClient; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/stream", nil)
		req.RemoteAddr = addr
		s, ok := StartSSE(rec, req)
		if !ok {
			t.Fatalf("stream %d refused below the cap", i)
		}
		streams = append(streams, s)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	req.RemoteAddr = addr
	if _, ok := StartSSE(rec, req); ok {
		t.Fatal("stream over the cap must be refused")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("over-cap status = %d, want 429", rec.Code)
	}
	// Releasing one slot readmits the client.
	streams[0].Close()
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/stream", nil)
	req2.RemoteAddr = addr
	s, ok := StartSSE(rec2, req2)
	if !ok {
		t.Fatal("client must be readmitted after a slot frees")
	}
	streams = append(streams, s)
}
