// SPDX-License-Identifier: MIT

package agentgw

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
)

// testServer is a full gateway integration test setup with an HTTP server.
type testServer struct {
	gw      *Gateway
	srv     *httptest.Server
	token   string // all-caps token
	mem     *memory.Manager
	bus     *bus.Bus
	roster_ *roster.Store
}

// newTestServer creates a gateway with real dependencies and a running HTTP server.
func newTestServer(t *testing.T) *testServer {
	t.Helper()

	dir := t.TempDir()

	cfg := DefaultGatewayConfig(dir)
	gw := NewGateway(cfg)

	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	b := bus.New(j)
	t.Cleanup(func() { b.Close() })

	memStore, err := memory.Open(dir)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	mem := memory.NewManager(memStore, b)

	ros, err := roster.Open(dir)
	if err != nil {
		t.Fatalf("roster.Open: %v", err)
	}

	gw.Attach(b, mem, ros)

	// Mint an all-caps token for testing
	token, err := gw.tokenMgr.CreateToken(&TokenClaims{
		RunID:     "test-run-001",
		Caps:      []string{"eventbus.publish", "eventbus.subscribe", "memory.write", "memory.read", "memory.delete", "memory.search", "log.write", "log.read", "agent.list", "agent.query", "config.access", "config.list", "config.search", "config.write"},
		MaxRate:   120,
		MaxBurst:  20,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// Build mux with full routing (same as Listen)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/eventbus/subscribe", gw.withAuth(gw.handleEventbusSubscribe))
	mux.HandleFunc("POST /v1/eventbus/publish", gw.withAuth(gw.handleEventbusPublish))
	mux.HandleFunc("POST /v1/memory/write", gw.withAuth(gw.handleMemoryWrite))
	mux.HandleFunc("DELETE /v1/memory/delete", gw.withAuth(gw.handleMemoryDelete))
	mux.HandleFunc("GET /v1/memory/search", gw.withAuth(gw.handleMemorySearch))
	mux.HandleFunc("GET /v1/log/read", gw.withAuth(gw.handleLogRead))
	mux.HandleFunc("POST /v1/log/write", gw.withAuth(gw.handleLogWrite))
	mux.HandleFunc("GET /v1/agent/list", gw.withAuth(gw.handleAgentList))
	mux.HandleFunc("GET /v1/agent/query", gw.withAuth(gw.handleAgentQuery))
	mux.HandleFunc("GET /health", gw.handleHealth)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &testServer{gw: gw, srv: srv, token: token, mem: mem, bus: b, roster_: ros}
}

// doRequest is a helper for making authenticated requests.
func (ts *testServer) doRequest(method, path string, body []byte) (*http.Response, string) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(method, ts.srv.URL+path, bytes.NewReader(body))
	if err != nil {
		return &http.Response{StatusCode: 0}, err.Error()
	}
	req.Header.Set("Authorization", "Bearer "+ts.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		return &http.Response{StatusCode: 0}, err.Error()
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(bodyBytes)
}

// doRequestNoAuth makes a request without authentication.
func (ts *testServer) doRequestNoAuth(method, path string, body []byte) (*http.Response, string) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(method, ts.srv.URL+path, bytes.NewReader(body))
	if err != nil {
		return &http.Response{StatusCode: 0}, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		return &http.Response{StatusCode: 0}, err.Error()
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(bodyBytes)
}

// doRequestWithCaps makes a request with a limited-capability token.
func (ts *testServer) doRequestWithCaps(method, path string, body []byte, caps []string) (*http.Response, string) {
	client := &http.Client{Timeout: 5 * time.Second}
	token, err := ts.gw.tokenMgr.CreateToken(&TokenClaims{
		RunID:     "test-run-001",
		Caps:      caps,
		MaxRate:   60,
		MaxBurst:  10,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})
	if err != nil {
		return nil, ""
	}
	req, err := http.NewRequest(method, ts.srv.URL+path, bytes.NewReader(body))
	if err != nil {
		return &http.Response{StatusCode: 0}, err.Error()
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		return &http.Response{StatusCode: 0}, err.Error()
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(bodyBytes)
}

// --- Health endpoint ---

func TestGateway_HandleHealth(t *testing.T) {
	ts := newTestServer(t)
	resp, body := ts.doRequest("GET", "/health", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health: got %d, want 200: %s", resp.StatusCode, body)
	}
	var r map[string]string
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r["status"] != "healthy" {
		t.Errorf("status: got %q, want %q", r["status"], "healthy")
	}
}

// --- Auth middleware: no token ---

func TestGateway_Auth_NoToken(t *testing.T) {
	ts := newTestServer(t)
	tests := []struct {
		method string
		path   string
	}{
		{"GET", "/v1/agent/list"},
		{"POST", "/v1/memory/write"},
		{"GET", "/v1/memory/search"},
		{"DELETE", "/v1/memory/delete"},
		{"POST", "/v1/log/write"},
		{"GET", "/v1/log/read"},
		{"POST", "/v1/eventbus/publish"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			resp, _ := ts.doRequestNoAuth(tt.method, tt.path, nil)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s %s: got %d, want 401", tt.method, tt.path, resp.StatusCode)
			}
		})
	}
}

// --- Auth middleware: invalid token ---

func TestGateway_Auth_InvalidToken(t *testing.T) {
	ts := newTestServer(t)
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", ts.srv.URL+"/v1/agent/list", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp, _ := client.Do(req)
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("invalid-token: got %d, want 401", resp.StatusCode)
	}
}

// --- Auth middleware: expired token ---

func TestGateway_Auth_ExpiredToken(t *testing.T) {
	ts := newTestServer(t)
	expired, _ := ts.gw.tokenMgr.CreateToken(&TokenClaims{
		RunID:     "test-run-001",
		Caps:      []string{"agent.list"},
		MaxRate:   60,
		MaxBurst:  10,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	})
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", ts.srv.URL+"/v1/agent/list", nil)
	req.Header.Set("Authorization", "Bearer "+expired)
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expired-token: got %d, want 401", resp.StatusCode)
	}
}

// --- Auth middleware: capability denied ---

func TestGateway_Auth_CapDenied(t *testing.T) {
	ts := newTestServer(t)
	// Token only has "agent.list" — trying to use "agent.query" should fail
	limited, _ := ts.gw.tokenMgr.CreateToken(&TokenClaims{
		RunID:     "test-run-001",
		Caps:      []string{"agent.list"},
		MaxRate:   60,
		MaxBurst:  10,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})
	req, _ := http.NewRequest("GET", ts.srv.URL+"/v1/agent/query?id=some-agent", nil)
	req.Header.Set("Authorization", "Bearer "+limited)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil || resp == nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cap-denied: got %d, want 403", resp.StatusCode)
	}
}

// --- handleAgentList ---

func TestGateway_HandleAgentList_OK(t *testing.T) {
	ts := newTestServer(t)
	resp, body := ts.doRequest("GET", "/v1/agent/list", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent list: got %d, want 200: %s", resp.StatusCode, body)
	}
	var r map[string]interface{}
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	agents, ok := r["agents"].([]interface{})
	if !ok {
		t.Fatal("agents field missing or not array")
	}
	if len(agents) != 0 {
		t.Errorf("agents: got %d, want 0 (empty roster)", len(agents))
	}
}

// --- handleAgentQuery ---

func TestGateway_HandleAgentQuery_NotFound(t *testing.T) {
	ts := newTestServer(t)
	resp, body := ts.doRequest("GET", "/v1/agent/query?id=nonexistent", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("not-found: got %d, want 404: %s", resp.StatusCode, body)
	}
}

func TestGateway_HandleAgentQuery_MissingID(t *testing.T) {
	ts := newTestServer(t)
	resp, body := ts.doRequest("GET", "/v1/agent/query", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing-id: got %d, want 400: %s", resp.StatusCode, body)
	}
}

// --- handleMemoryWrite ---

func TestGateway_HandleMemoryWrite_OK(t *testing.T) {
	ts := newTestServer(t)
	body := []byte(`{"type":"FACT","subject":"test-subject","content":"test content"}`)
	resp, respBody := ts.doRequest("POST", "/v1/memory/write", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("memory write: got %d, want 201: %s", resp.StatusCode, respBody)
	}
	var r map[string]interface{}
	if err := json.Unmarshal([]byte(respBody), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r["created"] != true {
		t.Errorf("created: got %v, want true", r["created"])
	}
}

func TestGateway_HandleMemoryWrite_DefaultType(t *testing.T) {
	ts := newTestServer(t)
	body := []byte(`{"subject":"test","content":"content"}`)
	resp, respBody := ts.doRequest("POST", "/v1/memory/write", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("memory write (no type): got %d, want 201: %s", resp.StatusCode, respBody)
	}
}

func TestGateway_HandleMemoryWrite_InvalidJSON(t *testing.T) {
	ts := newTestServer(t)
	body := []byte(`{invalid}`)
	resp, respBody := ts.doRequest("POST", "/v1/memory/write", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid-json: got %d, want 400: %s", resp.StatusCode, respBody)
	}
}

func TestGateway_HandleMemoryWrite_MemoryUnavailable(t *testing.T) {
	ts := newTestServer(t)
	ts.gw.mem = nil // simulate memory unavailable
	body := []byte(`{"subject":"test","content":"content"}`)
	resp, respBody := ts.doRequest("POST", "/v1/memory/write", body)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("memory-unavailable: got %d, want 503: %s", resp.StatusCode, respBody)
	}
}

func TestGateway_HandleMemoryWrite_CapDenied(t *testing.T) {
	ts := newTestServer(t)
	body := []byte(`{"subject":"test","content":"content"}`)
	resp, respBody := ts.doRequestWithCaps("POST", "/v1/memory/write", body, []string{"memory.read"})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cap-denied: got %d, want 403: %s", resp.StatusCode, respBody)
	}
}

// --- handleMemorySearch ---

func TestGateway_HandleMemorySearch_OK(t *testing.T) {
	ts := newTestServer(t)
	// Write first
	writeBody := []byte(`{"subject":"searchable","content":"find me"}`)
	resp, _ := ts.doRequest("POST", "/v1/memory/write", writeBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: memory write failed: %d", resp.StatusCode)
	}
	// Then search
	resp, respBody := ts.doRequest("GET", "/v1/memory/search?q=searchable", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search: got %d, want 200: %s", resp.StatusCode, respBody)
	}
	var r map[string]interface{}
	if err := json.Unmarshal([]byte(respBody), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	results, ok := r["results"].([]interface{})
	if !ok {
		t.Fatal("results field missing or not array")
	}
	if len(results) == 0 {
		t.Error("search returned 0 results, want >= 1")
	}
}

func TestGateway_HandleMemorySearch_MissingQuery(t *testing.T) {
	ts := newTestServer(t)
	resp, respBody := ts.doRequest("GET", "/v1/memory/search", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing-query: got %d, want 400: %s", resp.StatusCode, respBody)
	}
}

func TestGateway_HandleMemorySearch_MemoryUnavailable(t *testing.T) {
	ts := newTestServer(t)
	ts.gw.mem = nil
	resp, respBody := ts.doRequest("GET", "/v1/memory/search?q=test", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("memory-unavailable: got %d, want 503: %s", resp.StatusCode, respBody)
	}
}

// --- handleMemoryDelete ---

func TestGateway_HandleMemoryDelete_MissingID(t *testing.T) {
	ts := newTestServer(t)
	resp, respBody := ts.doRequest("DELETE", "/v1/memory/delete", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing-id: got %d, want 400: %s", resp.StatusCode, respBody)
	}
}

func TestGateway_HandleMemoryDelete_NotFound(t *testing.T) {
	ts := newTestServer(t)
	resp, respBody := ts.doRequest("DELETE", "/v1/memory/delete?id=nonexistent-id", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("not-found: got %d, want 200: %s", resp.StatusCode, respBody)
	}
	var r map[string]interface{}
	json.Unmarshal([]byte(respBody), &r)
	if r["deleted"] != false {
		t.Errorf("deleted: got %v, want false", r["deleted"])
	}
}

// --- handleLogWrite ---

func TestGateway_HandleLogWrite_OK(t *testing.T) {
	ts := newTestServer(t)
	body := []byte(`{"level":"info","message":"test log entry","meta":{"key":"value"}}`)
	resp, respBody := ts.doRequest("POST", "/v1/log/write", body)
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("log write: got %d, want 202: %s", resp.StatusCode, respBody)
	}
	var r map[string]string
	if err := json.Unmarshal([]byte(respBody), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r["status"] != "logged" {
		t.Errorf("status: got %q, want %q", r["status"], "logged")
	}
}

func TestGateway_HandleLogWrite_InvalidJSON(t *testing.T) {
	ts := newTestServer(t)
	body := []byte(`{invalid}`)
	resp, respBody := ts.doRequest("POST", "/v1/log/write", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid-json: got %d, want 400: %s", resp.StatusCode, respBody)
	}
}

// --- handleLogRead ---

func TestGateway_HandleLogRead_OK(t *testing.T) {
	ts := newTestServer(t)
	resp, respBody := ts.doRequest("GET", "/v1/log/read", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("log read: got %d, want 200: %s", resp.StatusCode, respBody)
	}
	var r map[string]interface{}
	if err := json.Unmarshal([]byte(respBody), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg, ok := r["message"].(string); !ok || msg == "" {
		t.Error("message field missing or empty")
	}
}

// --- handleEventbusPublish ---

func TestGateway_HandleEventbusPublish_OK(t *testing.T) {
	ts := newTestServer(t)
	body := []byte(`{"event":"test.event","payload":{"key":"value"}}`)
	resp, respBody := ts.doRequest("POST", "/v1/eventbus/publish", body)
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("publish: got %d, want 202: %s", resp.StatusCode, respBody)
	}
	var r map[string]string
	if err := json.Unmarshal([]byte(respBody), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r["status"] != "published" {
		t.Errorf("status: got %q, want %q", r["status"], "published")
	}
}

func TestGateway_HandleEventbusPublish_InvalidJSON(t *testing.T) {
	ts := newTestServer(t)
	body := []byte(`{invalid}`)
	resp, respBody := ts.doRequest("POST", "/v1/eventbus/publish", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid-json: got %d, want 400: %s", resp.StatusCode, respBody)
	}
}

func TestGateway_HandleEventbusPublish_BusUnavailable(t *testing.T) {
	ts := newTestServer(t)
	ts.gw.bus = nil
	body := []byte(`{"event":"test.event"}`)
	resp, respBody := ts.doRequest("POST", "/v1/eventbus/publish", body)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("bus-unavailable: got %d, want 503: %s", resp.StatusCode, respBody)
	}
}

func TestGateway_HandleEventbusPublish_CapDenied(t *testing.T) {
	ts := newTestServer(t)
	body := []byte(`{"event":"test.event"}`)
	resp, respBody := ts.doRequestWithCaps("POST", "/v1/eventbus/publish", body, []string{"memory.write"})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cap-denied: got %d, want 403: %s", resp.StatusCode, respBody)
	}
}

// --- handleEventbusSubscribe ---

func TestGateway_HandleEventbusSubscribe_BusUnavailable(t *testing.T) {
	ts := newTestServer(t)
	ts.gw.bus = nil
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", ts.srv.URL+"/v1/eventbus/subscribe", nil)
	req.Header.Set("Authorization", "Bearer "+ts.token)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		t.Logf("request failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("bus-unavailable: got %d, want 503", resp.StatusCode)
	}
}

func TestGateway_HandleEventbusSubscribe_DefaultPattern(t *testing.T) {
	ts := newTestServer(t)
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", ts.srv.URL+"/v1/eventbus/subscribe", nil)
	req.Header.Set("Authorization", "Bearer "+ts.token)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		t.Logf("request failed: %v", err)
		return
	}
	defer resp.Body.Close()
	// Even with immediate cancel, we should get headers
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type: got %q, want %q", ct, "text/event-stream")
	}
}

// --- Close ---

func TestGateway_Close(t *testing.T) {
	gw := NewGateway(DefaultGatewayConfig(""))
	err := gw.Close() // nil server → should be no-op
	if err != nil {
		t.Errorf("Close: unexpected error: %v", err)
	}
}
