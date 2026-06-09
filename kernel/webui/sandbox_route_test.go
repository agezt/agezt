// SPDX-License-Identifier: MIT

package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// /api/sandbox is a parameterless read proxied to sandbox_list.
func TestSandboxRouteProxiesList(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"projects": []any{}, "count": 0}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/sandbox?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "sandbox_list" {
		t.Errorf("expected one sandbox_list call, got %v", fc.calls)
	}
}

// /api/sandbox_file forwards only the allowlisted project + file args.
func TestSandboxFileRouteForwardsProjectAndFile(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"content": "x"}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet,
		"/api/sandbox_file?token=secret&project=calc&file=add.py&evil=rm", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "sandbox_file" {
		t.Fatalf("expected one sandbox_file call, got %v", fc.calls)
	}
	if fc.lastArgs["project"] != "calc" || fc.lastArgs["file"] != "add.py" {
		t.Errorf("project/file not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Error("non-allowlisted arg leaked into the sandbox_file call")
	}
}

// /api/sandbox/delete is a POST-only mutation forwarding the project arg.
func TestSandboxDeleteRouteForwardsProject(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"deleted": "calc"}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/sandbox/delete?token=secret&project=calc&evil=x", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "sandbox_delete" {
		t.Fatalf("expected one sandbox_delete call, got %v", fc.calls)
	}
	if fc.lastArgs["project"] != "calc" {
		t.Errorf("project not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Error("non-allowlisted arg leaked into the delete call")
	}
}

// A GET to the delete route must NOT issue the command (mutations are POST-only).
func TestSandboxDeleteRejectsGET(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/sandbox/delete?token=secret&project=calc", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET delete = %d want 405", rec.Code)
	}
	if len(fc.calls) != 0 {
		t.Errorf("GET must not issue a delete, got %v", fc.calls)
	}
}
