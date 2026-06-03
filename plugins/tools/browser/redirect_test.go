// SPDX-License-Identifier: MIT

package browser_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agezt/agezt/plugins/tools/browser"
)

// A page that 302-redirects to a host outside the allowlist must not be
// fetched, even though the initial (allowlisted) host was reached. The host
// allowlist applies on every redirect hop (M254).
func TestInvoke_RedirectToNonAllowlistedHostBlocked(t *testing.T) {
	reached := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		http.Redirect(w, r, "http://blocked.example.com/secret", http.StatusFound)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	tool := browser.New()
	tool.AllowLoopback = true                  // let netguard permit the loopback dial
	tool.AllowedHosts = []string{u.Hostname()} // only the initial host is allowed

	input := mustJSON(t, map[string]any{"url": srv.URL})
	if _, err := tool.Invoke(context.Background(), input); err == nil {
		t.Fatal("expected the off-allowlist redirect to be blocked")
	} else if !strings.Contains(err.Error(), "allowlist") {
		t.Errorf("err = %v, want an allowlist denial", err)
	}
	if !reached {
		t.Error("the initial allowlisted host should have been reached")
	}
}

// A same-host redirect stays within the allowlist and is followed.
func TestInvoke_RedirectWithinAllowlistFollowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/dest", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html><body><p>arrived</p></body></html>")
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	tool := browser.New()
	tool.AllowLoopback = true
	tool.AllowedHosts = []string{u.Hostname()}

	input := mustJSON(t, map[string]any{"url": srv.URL + "/start"})
	res, err := tool.Invoke(context.Background(), input)
	if err != nil {
		t.Fatalf("a same-host redirect should be followed: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Output), &out); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if text, _ := out["text"].(string); !strings.Contains(text, "arrived") {
		t.Errorf("redirect not followed to destination; text=%q", text)
	}
}
