// SPDX-License-Identifier: MIT

package fetch

import (
	"net/http"
	"testing"
)

func TestFetchCoverageClientInjected(t *testing.T) {
	// When HTTP is injected, client() returns it unchanged. The OnBlock
	// callback isn't wired into the netguard client in this path, so it
	// should never fire here.
	custom := &http.Client{}
	tool := New()
	tool.HTTP = custom
	called := false
	tool.OnBlock = func(_, _ string) { called = true }
	got := tool.client()
	if got != custom {
		t.Fatalf("client() = %p, want %p", got, custom)
	}
	if called {
		t.Fatal("OnBlock should not be called when HTTP is injected")
	}
}

func TestFetchCoverageKindAndCleanMime(t *testing.T) {
	// kindForMime helper: only images and downloads are bucketed.
	cases := map[string]string{
		"image/png":     "image",
		"image/jpeg":    "image",
		"image/svg+xml": "image",
		"audio/ogg":     "download",
		"video/mp4":     "download",
		"text/plain":    "download",
		"application/x": "download",
	}
	for mime, want := range cases {
		if got := kindForMime(mime); got != want {
			t.Fatalf("kindForMime(%q) = %q, want %q", mime, got, want)
		}
	}

	// cleanMime strips parameters and lowercases.
	cases2 := map[string]string{
		"text/html":                  "text/html",
		"text/html; charset=utf-8":   "text/html",
		"APPLICATION/PDF":            "application/pdf",
		"  image/png;charset=utf-8 ": "image/png",
	}
	for in, want := range cases2 {
		if got := cleanMime(in); got != want {
			t.Fatalf("cleanMime(%q) = %q, want %q", in, got, want)
		}
	}
}
