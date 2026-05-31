// SPDX-License-Identifier: MIT

package browser_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/plugins/tools/browser"
)

// TestBrowser_CookiesPersistAcrossInvokes: a server that sets a
// cookie on the first request and rejects requests without it
// on subsequent calls. With the jar enabled, the second call
// succeeds.
func TestBrowser_CookiesPersistAcrossInvokes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("sid")
		if err != nil || c.Value == "" {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-sid-123", Path: "/"})
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body>set-cookie issued</body></html>"))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>logged in as " + c.Value + "</body></html>"))
	}))
	defer srv.Close()

	tool := browser.New()
	tool.AllowAll = true
	tool.AllowLoopback = true // reach the loopback test server
	if err := tool.EnableCookies(); err != nil {
		t.Fatalf("EnableCookies: %v", err)
	}

	// First call — receives Set-Cookie.
	res1, err := tool.Invoke(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`"}`))
	if err != nil {
		t.Fatalf("first Invoke: %v", err)
	}
	if !strings.Contains(res1.Output, "set-cookie issued") {
		t.Errorf("first Output unexpected: %q", res1.Output)
	}

	// Second call — should send the cookie and see the logged-in page.
	res2, err := tool.Invoke(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`"}`))
	if err != nil {
		t.Fatalf("second Invoke: %v", err)
	}
	if !strings.Contains(res2.Output, "logged in as test-sid-123") {
		t.Errorf("second Output didn't reflect cookie: %q", res2.Output)
	}
}

// TestBrowser_NoCookieJarMeansNoPersistence: when EnableCookies
// wasn't called, the same scenario above should hit the
// "set-cookie issued" branch every time (no persistence).
func TestBrowser_NoCookieJarMeansNoPersistence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("sid"); err != nil {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "x", Path: "/"})
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body>set-cookie issued</body></html>"))
			return
		}
		_, _ = w.Write([]byte("<html><body>logged in</body></html>"))
	}))
	defer srv.Close()

	tool := browser.New()
	tool.AllowAll = true
	tool.AllowLoopback = true // reach the loopback test server
	// NO EnableCookies — jar nil.

	for range 2 {
		res, err := tool.Invoke(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`"}`))
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		if !strings.Contains(res.Output, "set-cookie issued") {
			t.Errorf("got logged-in response without a jar: %q", res.Output)
		}
	}
}
