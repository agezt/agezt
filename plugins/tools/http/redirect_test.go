// SPDX-License-Identifier: MIT

package http

import (
	stdhttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// A redirect to a host outside the allowlist is blocked, even though the
// initial (allowlisted) host was reached. The allowlist applies on every hop,
// not just the first URL (M251).
func TestRedirect_ToNonAllowlistedHostBlocked(t *testing.T) {
	reached := false
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		reached = true
		stdhttp.Redirect(w, r, "http://blocked.example.com/secret", stdhttp.StatusFound)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	tool := New()
	tool.AllowLoopback = true                  // let netguard permit the loopback dial
	tool.AllowedHosts = []string{u.Hostname()} // only the initial host is allowed

	out, isErr := invoke(t, tool, httpInput{Method: "GET", URL: srv.URL})
	if !isErr {
		t.Fatalf("expected the off-allowlist redirect to be blocked; out=%s", out)
	}
	if !strings.Contains(out, "allowlist") {
		t.Errorf("error should mention the allowlist; out=%s", out)
	}
	if !reached {
		t.Error("the initial allowlisted host should have been reached")
	}
}

// A redirect that stays within the allowlist (same host) is followed normally.
func TestRedirect_WithinAllowlistFollowed(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path == "/start" {
			stdhttp.Redirect(w, r, "/dest", stdhttp.StatusFound)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("arrived"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	tool := New()
	tool.AllowLoopback = true
	tool.AllowedHosts = []string{u.Hostname()}

	out, isErr := invoke(t, tool, httpInput{Method: "GET", URL: srv.URL + "/start"})
	if isErr {
		t.Fatalf("a same-host redirect should be followed; out=%s", out)
	}
	if !strings.Contains(out, "arrived") {
		t.Errorf("redirect not followed to destination; out=%s", out)
	}
}
