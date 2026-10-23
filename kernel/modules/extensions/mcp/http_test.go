// SPDX-License-Identifier: MIT

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeStreamableServer is a tiny MCP Streamable HTTP server: it answers
// initialize/tools/list/tools/call over JSON-RPC. It can reply either as a
// single application/json body or as a text/event-stream, toggled per test, to
// exercise both reply framings the client must handle.
type fakeStreamableServer struct {
	sse        bool   // reply with text/event-stream instead of application/json
	sessionID  string // handed out on initialize, expected back afterwards
	gotHeaders http.Header
	gotSession []string // Mcp-Session-Id seen on each non-initialize request
	deleted    bool
}

func (f *fakeStreamableServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			f.deleted = true
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		f.gotHeaders = r.Header.Clone()

		// Notification (no id): 202, no body.
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		if req.Method != "initialize" {
			f.gotSession = append(f.gotSession, r.Header.Get("Mcp-Session-Id"))
		}

		var result any
		switch req.Method {
		case "initialize":
			if f.sessionID != "" {
				w.Header().Set("Mcp-Session-Id", f.sessionID)
			}
			result = map[string]any{"protocolVersion": httpProtocolVersion, "capabilities": map[string]any{}}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{
				{"name": "echo", "description": "echoes input", "inputSchema": map[string]any{"type": "object"}},
			}}
		case "tools/call":
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": "pong"}}, "isError": false}
		default:
			http.Error(w, "unknown method", http.StatusBadRequest)
			return
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}
		raw, _ := json.Marshal(resp)
		if f.sse {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			// A keep-alive comment + the event, terminated by a blank line.
			_, _ = io.WriteString(w, ": keep-alive\n")
			_, _ = io.WriteString(w, "event: message\ndata: "+string(raw)+"\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	}
}

func TestDialHTTP_JSONFraming(t *testing.T) {
	fake := &fakeStreamableServer{sessionID: "sess-123"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	conn, err := DialHTTP(context.Background(), srv.URL, map[string]string{"Authorization": "Bearer tok"})
	if err != nil {
		t.Fatalf("DialHTTP: %v", err)
	}
	defer conn.Close()

	tools := conn.Tools()
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v, want one named echo", tools)
	}
	out, isErr, err := conn.Call(context.Background(), "echo", json.RawMessage(`{"x":1}`))
	if err != nil || isErr || out != "pong" {
		t.Fatalf("Call = %q,%v,%v; want pong,false,nil", out, isErr, err)
	}

	// The opt-in header rode the request, and the session id from initialize
	// was echoed back on the later calls.
	if got := fake.gotHeaders.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("Authorization header = %q, want Bearer tok", got)
	}
	for _, s := range fake.gotSession {
		if s != "sess-123" {
			t.Errorf("Mcp-Session-Id echoed = %q, want sess-123", s)
		}
	}
	if len(fake.gotSession) == 0 {
		t.Error("no non-initialize requests recorded")
	}

	if err := conn.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if !fake.deleted {
		t.Error("Close should have issued a session DELETE")
	}
}

func TestDialHTTP_SSEFraming(t *testing.T) {
	fake := &fakeStreamableServer{sse: true}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	conn, err := DialHTTP(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("DialHTTP (sse): %v", err)
	}
	defer conn.Close()

	if len(conn.Tools()) != 1 {
		t.Fatalf("tools over sse = %+v", conn.Tools())
	}
	out, _, err := conn.Call(context.Background(), "echo", nil)
	if err != nil || out != "pong" {
		t.Fatalf("Call over sse = %q,%v; want pong", out, err)
	}
}

func TestDialHTTP_ServerErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := DialHTTP(context.Background(), srv.URL, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want a 401 handshake error, got %v", err)
	}
}
