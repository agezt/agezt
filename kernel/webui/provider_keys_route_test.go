// SPDX-License-Identifier: MIT

package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderKeysListRoute_GETForwardsEnv(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"keys": []any{}}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/provider/keys?token=secret&env=OPENAI_API_KEY", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(fc.calls) != 1 || fc.calls[0] != "provider_key_list" {
		t.Fatalf("list route: code=%d calls=%v", rec.Code, fc.calls)
	}
	if fc.lastArgs["env"] != "OPENAI_API_KEY" {
		t.Errorf("env not forwarded: %v", fc.lastArgs)
	}
}

func TestProviderKeysAddRoute_PostBodyForwards(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"added": true, "active_changed": true}}
	s, _ := newServer(t, fc, "secret")
	body := `{"env":"OPENAI_API_KEY","label":"work","value":"sk-secret","active":true,"evil":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/provider/keys/add?token=secret", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(fc.calls) != 1 || fc.calls[0] != "provider_key_add" {
		t.Fatalf("add route: code=%d calls=%v body=%s", rec.Code, fc.calls, rec.Body.String())
	}
	if fc.lastArgs["label"] != "work" || fc.lastArgs["value"] != "sk-secret" || fc.lastArgs["active"] != true {
		t.Errorf("args not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Error("non-allowlisted body key leaked into provider_key_add")
	}
}

func TestProviderKeysActivateRoute_PostForwards(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"active": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/provider/keys/activate?token=secret&env=OPENAI_API_KEY&label=work", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(fc.calls) != 1 || fc.calls[0] != "provider_key_activate" {
		t.Fatalf("activate route: code=%d calls=%v", rec.Code, fc.calls)
	}
	if fc.lastArgs["env"] != "OPENAI_API_KEY" || fc.lastArgs["label"] != "work" {
		t.Errorf("args not forwarded: %v", fc.lastArgs)
	}
}

func TestProviderKeysAddRoute_RejectsGET(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/provider/keys/add?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET keys/add = %d want 405", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("GET must not issue a write, got %v", fc.calls)
	}
}
