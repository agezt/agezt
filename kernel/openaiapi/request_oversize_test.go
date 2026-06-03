// SPDX-License-Identifier: MIT

package openaiapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestChat_OversizedBody proves the chat-completions endpoint bounds its request
// body: an authenticated POST whose body exceeds maxRequestBodyBytes is rejected
// with 413 rather than being read unbounded into memory. The body cap is shared
// by every decodeBody caller (chat + responses), so this covers both surfaces.
func TestChat_OversizedBody(t *testing.T) {
	eng := &fakeEngine{}
	s := newAPIServer(t, eng, "secret")

	var b strings.Builder
	b.WriteString(`{"model":"m","messages":[{"role":"user","content":"`)
	b.WriteString(strings.Repeat("x", maxRequestBodyBytes+1024))
	b.WriteString(`"}]}`)

	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(b.String()))
	r.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: got status %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

// TestChat_NormalBodyStillWorks proves the cap is regression-free: an ordinary
// small chat request still decodes and runs.
func TestChat_NormalBodyStillWorks(t *testing.T) {
	eng := &fakeEngine{answer: "ok", model: "m"}
	s := newAPIServer(t, eng, "secret")

	body := `{"model":"m","messages":[{"role":"user","content":"hi"}]}`
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("normal body: got status %d, want 200", rec.Code)
	}
}
