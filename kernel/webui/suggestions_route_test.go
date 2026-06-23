// SPDX-License-Identifier: MIT

package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// /api/suggestions forwards only the allowlisted session_id + tools args to the
// chat_suggestions command. tools is a comma-joined list of recent tool names.
func TestSuggestionsRouteForwardsArgs(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"suggestions": []any{}}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet,
		"/api/suggestions?token=secret&session_id=conv1&tools=write,bash&evil=rm", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", rec.Code)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "chat_suggestions" {
		t.Fatalf("expected one chat_suggestions call, got %v", fc.calls)
	}
	if fc.lastArgs["session_id"] != "conv1" || fc.lastArgs["tools"] != "write,bash" {
		t.Errorf("session_id/tools not forwarded: %v", fc.lastArgs)
	}
	if _, leaked := fc.lastArgs["evil"]; leaked {
		t.Error("non-allowlisted arg leaked into the chat_suggestions call")
	}
}
