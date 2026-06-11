// SPDX-License-Identifier: MIT

package webui

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestArtifactRaw_ServesBytesWithSanitizedMime(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"data": base64.StdEncoding.EncodeToString([]byte("PNGDATA"))}}
	s, _ := newServer(t, fc, "secret")

	// Token required.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/artifact/raw?ref=abc", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: code=%d, want 401", rec.Code)
	}

	// With token + a known mime → bytes + that Content-Type.
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/artifact/raw?token=secret&ref=abc&mime=image/png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d, want 200", rec.Code)
	}
	if rec.Body.String() != "PNGDATA" {
		t.Fatalf("body=%q, want PNGDATA", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type=%q, want image/png", ct)
	}

	// A hostile/unknown mime is downgraded to octet-stream (nosniff blocks the rest).
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/artifact/raw?token=secret&ref=abc&mime=text/html", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("hostile mime content-type=%q, want application/octet-stream", ct)
	}

	// Missing ref → 400.
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/artifact/raw?token=secret", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no ref: code=%d, want 400", rec.Code)
	}
}
