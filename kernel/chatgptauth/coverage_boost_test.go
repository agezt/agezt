// SPDX-License-Identifier: MIT

package chatgptauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// TestForceRefresh drives ForceRefresh through the loopback token endpoint
// (success) and through the not-signed-in path (no refresh token) — the latter
// surfacing an error from refreshLocked.
func TestForceRefresh(t *testing.T) {
	withLoopbackClient(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		newID := makeJWT(map[string]any{
			"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acc-forced"},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-forced","refresh_token":"rt-forced","id_token":"` + newID + `"}`))
	}))
	defer srv.Close()
	prevEP := tokenEP
	tokenEP = srv.URL
	t.Cleanup(func() { tokenEP = prevEP })

	m := newTestManager(t)
	if err := m.StoreTokens(Tokens{AccessToken: "at-old", RefreshToken: "rt-old"}); err != nil {
		t.Fatalf("StoreTokens: %v", err)
	}

	access, acc, err := m.ForceRefresh(context.Background())
	if err != nil {
		t.Fatalf("ForceRefresh: %v", err)
	}
	if access != "at-forced" || acc != "acc-forced" {
		t.Fatalf("ForceRefresh access=%q acc=%q, want at-forced/acc-forced", access, acc)
	}
}

// TestForceRefreshNoRefreshToken exercises the ForceRefresh error path when
// there is no refresh token available.
func TestForceRefreshNoRefreshToken(t *testing.T) {
	m := newTestManager(t)
	// No tokens stored at all -> refreshLocked should fail.
	if _, _, err := m.ForceRefresh(context.Background()); err == nil {
		t.Fatalf("ForceRefresh(no tokens) error = nil, want error")
	}
}

// TestDefaultCodexAuthPath exercises both branches of DefaultCodexAuthPath:
// the CODEX_HOME override and the home-directory fallback.
func TestDefaultCodexAuthPath(t *testing.T) {
	// CODEX_HOME override branch.
	t.Setenv("CODEX_HOME", filepath.Join("custom", "codex-home"))
	got := DefaultCodexAuthPath()
	want := filepath.Join("custom", "codex-home", "auth.json")
	if got != want {
		t.Fatalf("DefaultCodexAuthPath(CODEX_HOME) = %q, want %q", got, want)
	}

	// Fallback branch (no CODEX_HOME). We can't fully control UserHomeDir but the
	// result must end in the expected suffix when a home dir is resolvable.
	t.Setenv("CODEX_HOME", "")
	fallback := DefaultCodexAuthPath()
	if fallback != "" && !strings.HasSuffix(fallback, filepath.Join(".codex", "auth.json")) {
		t.Fatalf("DefaultCodexAuthPath(fallback) = %q, want suffix .codex/auth.json", fallback)
	}
}

// TestTokenNotSignedIn exercises the Token() not-signed-in error branch.
func TestTokenNotSignedIn(t *testing.T) {
	m := newTestManager(t)
	if _, _, err := m.Token(context.Background()); err == nil {
		t.Fatalf("Token(no tokens) error = nil, want not-signed-in error")
	}
}
