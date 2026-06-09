// SPDX-License-Identifier: MIT

package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutingGetRoute(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"chains": map[string]any{}, "task_types": []any{"chat"}}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/routing?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(fc.calls) != 1 || fc.calls[0] != "routing_get" {
		t.Fatalf("routing get: code=%d calls=%v", rec.Code, fc.calls)
	}
}

func TestRoutingSetRoute_ForwardsChains(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"saved": true, "applied": "live"}}
	s, _ := newServer(t, fc, "secret")
	body := `{"chains":{"chat":["claude-opus","gpt-5"]},"evil":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/routing/set?token=secret", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(fc.calls) != 1 || fc.calls[0] != "routing_set" {
		t.Fatalf("routing set: code=%d calls=%v body=%s", rec.Code, fc.calls, rec.Body.String())
	}
	chains, ok := fc.lastArgs["chains"].(map[string]any)
	if !ok || chains["chat"] == nil {
		t.Errorf("chains not forwarded as object: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Error("non-allowlisted body key leaked into routing_set")
	}
}

func TestAPIResponsesAreNoStore(t *testing.T) {
	// API JSON is live daemon state; a stale browser cache must never resurrect an
	// old body after a mutation (the bug that masked saved routing chains on reload).
	fc := &fakeCaller{result: map[string]any{"chains": map[string]any{}, "task_types": []any{"chat"}}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/routing?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestRoutingSetRoute_RejectsGET(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/routing/set?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET routing/set = %d want 405", rec.Code)
	}
}
