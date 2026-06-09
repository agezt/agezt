// SPDX-License-Identifier: MIT

package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogSyncRoute_PostForwards(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"provider_count": 20, "model_count": 200}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/catalog/sync?token=secret", strings.NewReader(`{"url":"https://models.dev/api.json","evil":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(fc.calls) != 1 || fc.calls[0] != "catalog_sync" {
		t.Fatalf("sync route: code=%d calls=%v body=%s", rec.Code, fc.calls, rec.Body.String())
	}
	if fc.lastArgs["url"] != "https://models.dev/api.json" {
		t.Errorf("url not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Error("non-allowlisted body key leaked into catalog_sync")
	}
}

func TestCatalogSyncRoute_RejectsGET(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/catalog/sync?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET catalog/sync = %d want 405", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("GET must not issue a write, got %v", fc.calls)
	}
}
