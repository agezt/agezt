// SPDX-License-Identifier: MIT

package restapi

import (
	"net/http"
	"strings"
	"testing"
)

// TestRunsRoot_OversizedBody proves the run-create endpoint bounds its request
// body: an authenticated POST whose body exceeds maxRequestBodyBytes is rejected
// with 413 Request Entity Too Large rather than being read unbounded into memory.
func TestRunsRoot_OversizedBody(t *testing.T) {
	eng := &fakeEngine{}
	s := newServer(t, eng, "secret")

	// A valid-JSON-prefixed body whose single string value runs past the limit.
	// MaxBytesReader trips during the read, before the value is fully decoded.
	var b strings.Builder
	b.WriteString(`{"intent":"`)
	b.WriteString(strings.Repeat("x", maxRequestBodyBytes+1024))
	b.WriteString(`"}`)

	rec := do(t, s, http.MethodPost, "/api/v1/runs", b.String(), "secret")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: got status %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

// TestRunsRoot_NormalBodyStillWorks proves the body cap is a regression-free
// guard: an ordinary small request still decodes and runs as before.
func TestRunsRoot_NormalBodyStillWorks(t *testing.T) {
	eng := &fakeEngine{answer: "ok"}
	s := newServer(t, eng, "secret")

	rec := do(t, s, http.MethodPost, "/api/v1/runs", `{"intent":"hello"}`, "secret")
	if rec.Code != http.StatusOK && rec.Code != http.StatusAccepted {
		t.Fatalf("normal body: got status %d, want 200/202", rec.Code)
	}
}
