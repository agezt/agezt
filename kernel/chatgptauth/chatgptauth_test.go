// SPDX-License-Identifier: MIT

package chatgptauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeJWT builds an unsigned JWT whose payload carries the given claims (only the
// payload segment is read by the package).
func makeJWT(claims map[string]any) string {
	p, _ := json.Marshal(claims)
	return "h." + base64.RawURLEncoding.EncodeToString(p) + ".s"
}

// withLoopbackClient points token exchanges at an httptest server.
func withLoopbackClient(t *testing.T) {
	t.Helper()
	prev := httpClientFor
	httpClientFor = func(timeout time.Duration) *http.Client { return &http.Client{Timeout: timeout} }
	t.Cleanup(func() { httpClientFor = prev })
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "test-pass") // hermetic vault encryption
	return NewManager(t.TempDir())
}

func TestStoreAndReload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "test-pass")
	m := NewManager(dir)
	id := makeJWT(map[string]any{
		"email":                       "owner@example.com",
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acc-123"},
	})
	if err := m.StoreTokens(Tokens{AccessToken: "at-1", RefreshToken: "rt-1", IDToken: id}); err != nil {
		t.Fatal(err)
	}
	if !m.HasTokens() {
		t.Fatal("HasTokens false after store")
	}
	// A fresh manager over the same dir sees the persisted tokens + derived account.
	m2 := NewManager(dir)
	if !m2.HasTokens() {
		t.Fatal("reload lost tokens")
	}
	email, acc := m2.Account()
	if email != "owner@example.com" || acc != "acc-123" {
		t.Fatalf("account = %q / %q", email, acc)
	}
}

func TestProactiveRefreshOnExpiry(t *testing.T) {
	withLoopbackClient(t)
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.Form
		newID := makeJWT(map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acc-9"}})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-new","refresh_token":"rt-new","id_token":"` + newID + `"}`))
	}))
	defer srv.Close()
	prevEP := tokenEP
	tokenEP = srv.URL
	t.Cleanup(func() { tokenEP = prevEP })

	m := newTestManager(t)
	expired := makeJWT(map[string]any{"exp": float64(time.Now().Add(-time.Hour).Unix())})
	if err := m.StoreTokens(Tokens{AccessToken: expired, RefreshToken: "rt-old"}); err != nil {
		t.Fatal(err)
	}
	access, acc, err := m.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if access != "at-new" || acc != "acc-9" {
		t.Fatalf("after refresh access=%q acc=%q", access, acc)
	}
	if gotForm.Get("grant_type") != "refresh_token" || gotForm.Get("client_id") != ClientID || gotForm.Get("refresh_token") != "rt-old" {
		t.Fatalf("refresh form = %v", gotForm)
	}
}

func TestExchangeCode(t *testing.T) {
	withLoopbackClient(t)
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.Form
		_, _ = w.Write([]byte(`{"access_token":"at-x","refresh_token":"rt-x","id_token":"h.h.h"}`))
	}))
	defer srv.Close()
	prevEP := tokenEP
	tokenEP = srv.URL
	t.Cleanup(func() { tokenEP = prevEP })

	m := newTestManager(t)
	if err := m.ExchangeCode(context.Background(), "the-code", "the-verifier"); err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if gotForm.Get("grant_type") != "authorization_code" || gotForm.Get("code") != "the-code" ||
		gotForm.Get("code_verifier") != "the-verifier" || gotForm.Get("redirect_uri") != RedirectURI {
		t.Fatalf("exchange form = %v", gotForm)
	}
	if !m.HasTokens() {
		t.Fatal("tokens not stored after exchange")
	}
}

func TestImportFromCodexCLI(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	id := makeJWT(map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acc-imp"}})
	blob := `{"OPENAI_API_KEY":null,"tokens":{"access_token":"at-i","refresh_token":"rt-i","id_token":"` + id + `","account_id":"acc-imp"},"last_refresh":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(path, []byte(blob), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.ImportFromCodexCLI(path); err != nil {
		t.Fatalf("import: %v", err)
	}
	access, acc, err := m.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if access != "at-i" || acc != "acc-imp" {
		t.Fatalf("imported access=%q acc=%q", access, acc)
	}
	// Missing-tokens file is rejected.
	bad := filepath.Join(dir, "empty.json")
	_ = os.WriteFile(bad, []byte(`{"tokens":{}}`), 0o600)
	if err := m.ImportFromCodexCLI(bad); err == nil {
		t.Fatal("import of empty tokens should fail")
	}
}

func TestAuthorizeURLAndPKCE(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil || verifier == "" || challenge == "" || verifier == challenge {
		t.Fatalf("pkce = %q/%q err=%v", verifier, challenge, err)
	}
	state, _ := RandomState()
	u, err := url.Parse(AuthorizeURL(challenge, state))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	for k, want := range map[string]string{
		"response_type": "code", "client_id": ClientID, "redirect_uri": RedirectURI,
		"code_challenge_method": "S256", "code_challenge": challenge, "state": state,
		"codex_cli_simplified_flow": "true", "originator": Originator,
	} {
		if q.Get(k) != want {
			t.Errorf("authorize %s = %q want %q", k, q.Get(k), want)
		}
	}
	if !strings.HasPrefix(u.String(), AuthorizeEP) {
		t.Errorf("authorize URL base = %s", u.String())
	}
}

func TestLogout(t *testing.T) {
	m := newTestManager(t)
	_ = m.StoreTokens(Tokens{AccessToken: "a", RefreshToken: "r"})
	if err := m.Logout(); err != nil {
		t.Fatal(err)
	}
	if m.HasTokens() {
		t.Fatal("tokens remain after logout")
	}
}
