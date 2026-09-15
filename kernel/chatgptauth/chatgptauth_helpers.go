// SPDX-License-Identifier: MIT

// Package chatgptauth: OAuth flow + PKCE helpers + Codex CLI importer
// (Manager.ExchangeCode + Manager.ImportFromCodexCLI + DefaultCodexAuthPath
// + GeneratePKCE + RandomState + AuthorizeURL). Extracted from chatgptauth.go
// during the Day-211 god-file split. Public API unchanged.
package chatgptauth


import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ExchangeCode swaps an authorization code (+ PKCE verifier) for tokens and
// stores them.
func (m *Manager) ExchangeCode(ctx context.Context, code, verifier string) error {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", RedirectURI)
	form.Set("client_id", ClientID)
	form.Set("code_verifier", verifier)
	out, err := postToken(ctx, form)
	if err != nil {
		return err
	}
	return m.StoreTokens(Tokens{
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
		IDToken:      out.IDToken,
	})
}

// ImportFromCodexCLI copies tokens from a Codex CLI auth.json into our store.
// An empty path resolves to $CODEX_HOME/auth.json or ~/.codex/auth.json.
func (m *Manager) ImportFromCodexCLI(path string) error {
	if path == "" {
		path = DefaultCodexAuthPath()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("chatgptauth: read codex auth: %w", err)
	}
	var f struct {
		Tokens Tokens `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return fmt.Errorf("chatgptauth: parse codex auth: %w", err)
	}
	if f.Tokens.AccessToken == "" && f.Tokens.RefreshToken == "" {
		return fmt.Errorf("chatgptauth: codex auth has no tokens (run `codex login` first)")
	}
	return m.StoreTokens(f.Tokens)
}

// DefaultCodexAuthPath returns the conventional Codex CLI credential path.
func DefaultCodexAuthPath() string {
	if h := strings.TrimSpace(os.Getenv("CODEX_HOME")); h != "" {
		return filepath.Join(h, "auth.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "auth.json")
}

// --- PKCE + authorize URL ---

// GeneratePKCE returns a fresh (verifier, S256 challenge) pair.
func GeneratePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// RandomState returns an opaque CSRF state token.
func RandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// AuthorizeURL builds the provider authorize URL for a PKCE challenge + state.
func AuthorizeURL(challenge, state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", ClientID)
	q.Set("redirect_uri", RedirectURI)
	q.Set("scope", Scopes)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("originator", Originator)
	q.Set("state", state)
	return AuthorizeEP + "?" + q.Encode()
}
