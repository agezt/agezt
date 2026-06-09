// SPDX-License-Identifier: MIT

package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigSchemaRoute(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"sections": []any{}}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/config/schema?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(fc.calls) != 1 || fc.calls[0] != "config_schema" {
		t.Fatalf("schema route: code=%d calls=%v", rec.Code, fc.calls)
	}
}

func TestConfigValuesRoute(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"fields": []any{}}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/config/values?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(fc.calls) != 1 || fc.calls[0] != "config_values" {
		t.Fatalf("values route: code=%d calls=%v", rec.Code, fc.calls)
	}
}

func TestConfigSetRoute_PostForwardsNameValue(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"saved": true, "applied": "restart"}}
	s, _ := newServer(t, fc, "secret")
	body := `{"name":"AGEZT_EMAIL_FROM","value":"jarvis@example.com","evil":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/config/set?token=secret", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(fc.calls) != 1 || fc.calls[0] != "config_set" {
		t.Fatalf("set route: code=%d calls=%v body=%s", rec.Code, fc.calls, rec.Body.String())
	}
	if fc.lastArgs["name"] != "AGEZT_EMAIL_FROM" || fc.lastArgs["value"] != "jarvis@example.com" {
		t.Errorf("name/value not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Error("non-allowlisted body key leaked into config_set")
	}
}

func TestConfigSetRoute_RejectsGET(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/config/set?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET config/set = %d want 405", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("GET must not issue a write, got %v", fc.calls)
	}
}

func TestConfigSchemaRegisterRoute_ForwardsSection(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"id": "weather", "registered": true}}
	s, _ := newServer(t, fc, "secret")
	body := `{"section":{"id":"weather","name":"Weather","fields":[{"env":"AGEZT_X_W","type":"text"}]},"evil":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/config/schema/register?token=secret", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(fc.calls) != 1 || fc.calls[0] != "config_schema_register" {
		t.Fatalf("register route: code=%d calls=%v body=%s", rec.Code, fc.calls, rec.Body.String())
	}
	sec, ok := fc.lastArgs["section"].(map[string]any)
	if !ok || sec["id"] != "weather" {
		t.Errorf("section not forwarded as object: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Error("non-allowlisted body key leaked into config_schema_register")
	}
}

func TestConfigSchemaUnregisterRoute_ForwardsID(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"id": "weather", "removed": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/config/schema/unregister?token=secret", strings.NewReader(`{"id":"weather"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(fc.calls) != 1 || fc.calls[0] != "config_schema_unregister" {
		t.Fatalf("unregister route: code=%d calls=%v", rec.Code, fc.calls)
	}
	if fc.lastArgs["id"] != "weather" {
		t.Errorf("id not forwarded: %v", fc.lastArgs)
	}
}

func TestConfigSchemaRegisterRoute_RejectsGET(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/config/schema/register?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET config/schema/register = %d want 405", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("GET must not issue a write, got %v", fc.calls)
	}
}
