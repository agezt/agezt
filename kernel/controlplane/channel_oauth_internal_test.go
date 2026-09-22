// SPDX-License-Identifier: MIT

package controlplane

// Regression tests for BUG dropped-r.Context (audit fix): the channel
// OAuth callback and the provider OAuth callback both used to drop
// the request context by passing `context.Background()` into the
// outbound HTTP call. A client cancel or daemon shutdown could not
// abort the call before its internal 20-30s timeout.
//
// These tests live in `package controlplane` (not `_test`) so they
// can call the unexported `exchangeOAuthCode` and stand up a Server
// without spinning up the rest of the control plane.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestExchangeOAuthCode_AbortsOnContextCancel is the regression guard
// for BUG dropped-r.Context in the channel OAuth callback: a token
// endpoint that streams forever (simulating a stuck provider) MUST be
// abortable by cancelling the caller's context. Before the fix, the
// caller's ctx was dropped and `context.Background()` was used; only
// the function's internal 20s timeout would have stopped the call.
func TestExchangeOAuthCode_AbortsOnContextCancel(t *testing.T) {
	// Token endpoint that drips the body for ~20s — the test's
	// cancel() must unwind the client-side request even though the
	// server is still streaming. The server handler uses its own
	// r.Context() to bail out on connection close so the httptest
	// server can shut down cleanly.
	gotReq := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case gotReq <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; i < 200; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
			}
			if _, err := w.Write([]byte(".")); err != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
	}))
	defer srv.Close()

	// Swap the package-level client so we can reach the loopback
	// httptest server (the default netguard client blocks loopback).
	prev := oauthClientFor
	oauthClientFor = func(timeout time.Duration) *http.Client {
		return &http.Client{Timeout: timeout}
	}
	t.Cleanup(func() { oauthClientFor = prev })

	s := &Server{}
	flow := &oauthFlow{tokenURL: srv.URL}

	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		token string
		err   error
	}
	resCh := make(chan result, 1)
	go func() {
		token, err := s.exchangeOAuthCode(ctx, flow, "code")
		resCh <- result{token, err}
	}()

	// Wait until the HTTP request actually reaches the server.
	select {
	case <-gotReq:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("token endpoint never received the request")
	}

	cancelStart := time.Now()
	cancel()

	select {
	case r := <-resCh:
		elapsed := time.Since(cancelStart)
		// exchangeOAuthCode wraps its ctx in WithTimeout(20s); the
		// dropped-ctx bug would let the call run for the full 20s.
		// A correct implementation unwinds within ~50ms of cancel.
		if elapsed > 1*time.Second {
			t.Errorf("exchangeOAuthCode honoured %v after ctx cancel — expected <1s (dropped ctx leaked)", elapsed)
		}
		_ = r // err may be nil if cancel raced the success path; the
		//   timing assertion above is what proves ctx was respected.
	case <-time.After(3 * time.Second):
		t.Fatal("exchangeOAuthCode did not return within 3s of ctx cancel (dropped ctx leaked)")
	}
}

// TestIsHTTPSURL_HTTPSOnlyExceptLoopback pins F1: isHTTPSURL is the OAuth
// redirect_uri validator (single caller: channel_oauth.go:108). Before the
// fix, its body returned true for both http:// and https://, accepting any
// http://attacker.example/ as the redirect target forwarded verbatim into
// the provider's authorize URL. After the fix, only https is accepted on
// real hosts; http is allowed only for loopback dev (localhost / 127.0.0.1
// / ::1) so a local dev daemon can still use http://localhost. The name
// now matches the implementation and the downgrade path is closed.
func TestIsHTTPSURL_HTTPSOnlyExceptLoopback(t *testing.T) {
	cases := []struct {
		in   string
		want bool
		note string
	}{
		// Adversarial: http on a non-loopback host must be REJECTED.
		{"http://attacker.example/oauth/callback", false, "attacker-controlled http"},
		{"http://example.com/cb", false, "plain http on a real domain"},
		{"http://192.168.1.10/cb", false, "private-network http"},
		// Legit prod: https must be ACCEPTED.
		{"https://myapp.example.com/oauth/callback", true, "https real host"},
		// Dev loopback carve-out: http on loopback is still accepted.
		{"http://localhost:8080/cb", true, "http localhost with port"},
		{"http://localhost/cb", true, "http localhost no port"},
		{"http://127.0.0.1/cb", true, "http IPv4 loopback"},
		{"http://127.0.0.1:9000/cb", true, "http IPv4 loopback with port"},
		{"http://[::1]/cb", true, "http IPv6 loopback"},
		// Non-http(s) and malformed: must be REJECTED.
		{"ftp://example.com/cb", false, "non-http scheme"},
		{"javascript:alert(1)", false, "javascript scheme"},
		{"file:///etc/passwd", false, "file scheme"},
		{"not-a-url", false, "non-URL"},
		{"", false, "empty"},
		{"https://", false, "https without host"},
	}
	for _, c := range cases {
		if got := isHTTPSURL(c.in); got != c.want {
			t.Errorf("isHTTPSURL(%q) = %v, want %v (%s)", c.in, got, c.want, c.note)
		}
	}
}
