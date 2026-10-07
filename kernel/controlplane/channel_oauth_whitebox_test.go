// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// withLoopbackOAuthClient lets the token-exchange tests reach an httptest server
// on loopback (the production client deliberately blocks it). Restored on cleanup.
func withLoopbackOAuthClient(t *testing.T) {
	t.Helper()
	prev := oauthClientFor
	oauthClientFor = func(timeout time.Duration) *http.Client { return &http.Client{Timeout: timeout} }
	t.Cleanup(func() { oauthClientFor = prev })
}

func TestExchangeOAuthCode(t *testing.T) {
	withLoopbackOAuthClient(t)
	s := &Server{}
	_ = s.channelOAuth()

	t.Run("slack ok top-level token", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			if r.FormValue("grant_type") != "authorization_code" || r.FormValue("code") != "abc" {
				t.Errorf("bad exchange form: %v", r.Form)
			}
			if r.FormValue("client_id") != "cid" || r.FormValue("client_secret") != "csec" {
				t.Errorf("missing client creds: %v", r.Form)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"access_token":"xoxb-real"}`))
		}))
		defer srv.Close()
		tok, err := s.channelOAuthState.Exchange(context.Background(), appchannels.OAuthFlow{
			ClientID: "cid", ClientSecret: "csec", RedirectURI: "https://x/cb", TokenURL: srv.URL,
		}, "abc")
		if err != nil || tok != "xoxb-real" {
			t.Fatalf("token=%q err=%v", tok, err)
		}
	})

	t.Run("slack ok=false is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_code"}`))
		}))
		defer srv.Close()
		if _, err := s.channelOAuthState.Exchange(context.Background(), appchannels.OAuthFlow{TokenURL: srv.URL}, "x"); err == nil || !strings.Contains(err.Error(), "invalid_code") {
			t.Fatalf("want invalid_code error, got %v", err)
		}
	})

	t.Run("oauth2 error_description surfaces", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"code expired"}`))
		}))
		defer srv.Close()
		if _, err := s.channelOAuthState.Exchange(context.Background(), appchannels.OAuthFlow{TokenURL: srv.URL}, "x"); err == nil || !strings.Contains(err.Error(), "code expired") {
			t.Fatalf("want description error, got %v", err)
		}
	})

	t.Run("missing access_token is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"token_type":"bearer"}`))
		}))
		defer srv.Close()
		if _, err := s.channelOAuthState.Exchange(context.Background(), appchannels.OAuthFlow{TokenURL: srv.URL}, "x"); err == nil {
			t.Fatal("want error on missing access_token")
		}
	})
}

func TestNormalizeInstanceURL(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"https://mastodon.social", "https://mastodon.social", true},
		{"mastodon.social", "https://mastodon.social", true}, // scheme defaulted
		{"https://m.example.com/", "https://m.example.com", true},
		{"https://host:8443/x", "https://host:8443", true},
		{"", "", false},
		{"://bad", "", false},
	}
	for _, c := range cases {
		got, err := appchannels.NormalizeInstanceURL(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("normalize(%q) = %q,%v want %q", c.in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("normalize(%q) should error", c.in)
		}
	}
}

func TestOAuthStatePruneAndUnique(t *testing.T) {
	a, _ := appchannels.NewOAuthState()
	b, _ := appchannels.NewOAuthState()
	if a == "" || a == b {
		t.Fatalf("states not unique: %q %q", a, b)
	}
	p := appchannels.NewOAuthMemory(nil, nil)
	now := time.Now()
	p.Put("old", appchannels.OAuthFlow{Created: now.Add(-appchannels.OAuthFlowTTL - time.Minute)}, now.Add(-appchannels.OAuthFlowTTL-time.Minute))
	p.Put("fresh", appchannels.OAuthFlow{Created: now}, now)
	if _, ok := p.Flow("old"); ok {
		t.Fatal("stale flow should be pruned")
	}
	if _, ok := p.Flow("fresh"); !ok {
		t.Fatal("fresh flow should survive")
	}
}
