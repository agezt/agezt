// SPDX-License-Identifier: MIT

// Package chatgptauth manages the OAuth tokens for the "Sign in with ChatGPT"
// provider — the same subscription auth Codex CLI uses. It owns the PKCE
// constants, the authorize-URL + code/refresh exchanges against auth.openai.com,
// the at-rest token store (one JSON secret in the kernel vault), and extraction
// of the ChatGPT account id from the id_token.
//
// This deliberately reuses the Codex CLI's public OAuth client and the ChatGPT
// backend — an UNOFFICIAL, undocumented path that may break or run afoul of
// OpenAI's terms. It only ever authenticates the operator's own account; the UI
// gates it behind an explicit acknowledgement.
package chatgptauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/netguard"
)

// OAuth + backend constants (the Codex CLI public client).
const (
	ClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	AuthBase     = "https://auth.openai.com"
	AuthorizeEP  = AuthBase + "/oauth/authorize"
	TokenEP      = AuthBase + "/oauth/token"
	RedirectURI  = "http://localhost:1455/auth/callback"
	CallbackAddr = "127.0.0.1:1455"
	Scopes       = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	Originator   = "codex_cli_rs"

	// VaultKey is the single vault secret holding the JSON token blob.
	VaultKey = brand.EnvPrefix + "CHATGPT_OAUTH"
	// refreshSkew refreshes the access token this long before its JWT exp.
	refreshSkew = 2 * time.Minute
)

// httpClientFor builds the HTTP client for token exchanges. Defaults to an
// SSRF-guarded client (auth.openai.com is public); tests override it to reach an
// httptest server on loopback.
var httpClientFor = func(timeout time.Duration) *http.Client {
	return netguard.New().HTTPClient(timeout)
}

// tokenEP is the token endpoint, overridable in tests.
var tokenEP = TokenEP

// Tokens is the persisted token set (also the shape of ~/.codex/auth.json's
// "tokens" object).
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	AccountID    string `json:"account_id"`
	LastRefresh  string `json:"last_refresh,omitempty"`
}

// Manager loads/refreshes/persists the ChatGPT tokens, backed by the kernel
// vault at baseDir.
type Manager struct {
	baseDir string
	mu      sync.Mutex
	toks    Tokens
	loaded  bool
	now     func() time.Time // test seam
}

// NewManager returns a Manager backed by the vault at baseDir.
func NewManager(baseDir string) *Manager {
	return &Manager{baseDir: baseDir, now: time.Now}
}

func (m *Manager) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

// ensureLoaded reads the token blob from the vault once.
func (m *Manager) ensureLoaded() error {
	if m.loaded {
		return nil
	}
	v := creds.NewStore(m.baseDir)
	if err := v.Load(); err != nil {
		return fmt.Errorf("chatgptauth: load vault: %w", err)
	}
	if raw := v.Get(VaultKey); raw != "" {
		_ = json.Unmarshal([]byte(raw), &m.toks)
	}
	m.loaded = true
	return nil
}

// persist writes the current tokens back to the vault.
func (m *Manager) persist() error {
	v := creds.NewStore(m.baseDir)
	if err := v.Load(); err != nil {
		return fmt.Errorf("chatgptauth: load vault: %w", err)
	}
	blob, err := json.Marshal(m.toks)
	if err != nil {
		return err
	}
	if err := v.Set(VaultKey, string(blob)); err != nil {
		return err
	}
	return v.Save()
}

// HasTokens reports whether a usable token set is stored.
func (m *Manager) HasTokens() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ensureLoaded() != nil {
		return false
	}
	return m.toks.AccessToken != "" || m.toks.RefreshToken != ""
}

// Token returns a valid access token + account id, refreshing proactively when
// the access token is within refreshSkew of expiry.
func (m *Manager) Token(ctx context.Context) (access, accountID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureLoaded(); err != nil {
		return "", "", err
	}
	if m.toks.AccessToken == "" && m.toks.RefreshToken == "" {
		return "", "", fmt.Errorf("chatgptauth: not signed in")
	}
	if m.needsRefreshLocked() {
		if err := m.refreshLocked(ctx); err != nil {
			// A stale access token that hasn't hard-expired may still work; only
			// fail when we have nothing usable.
			if m.toks.AccessToken == "" {
				return "", "", err
			}
		}
	}
	return m.toks.AccessToken, m.toks.AccountID, nil
}

// ForceRefresh refreshes regardless of expiry (used on a 401).
func (m *Manager) ForceRefresh(ctx context.Context) (access, accountID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureLoaded(); err != nil {
		return "", "", err
	}
	if err := m.refreshLocked(ctx); err != nil {
		return "", "", err
	}
	return m.toks.AccessToken, m.toks.AccountID, nil
}

func (m *Manager) needsRefreshLocked() bool {
	if m.toks.RefreshToken == "" {
		return false
	}
	if m.toks.AccessToken == "" {
		return true
	}
	exp, ok := jwtExp(m.toks.AccessToken)
	if !ok {
		return false // can't tell; let it ride until a 401 forces a refresh
	}
	return m.clock().Add(refreshSkew).After(exp)
}

// refreshLocked exchanges the refresh token for a new access token. Caller holds mu.
func (m *Manager) refreshLocked(ctx context.Context) error {
	if m.toks.RefreshToken == "" {
		return fmt.Errorf("chatgptauth: no refresh token")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", ClientID)
	form.Set("refresh_token", m.toks.RefreshToken)
	form.Set("scope", Scopes)
	out, err := postToken(ctx, form)
	if err != nil {
		return err
	}
	m.toks.AccessToken = out.AccessToken
	if out.RefreshToken != "" {
		m.toks.RefreshToken = out.RefreshToken
	}
	if out.IDToken != "" {
		m.toks.IDToken = out.IDToken
	}
	if id := accountIDFromIDToken(m.toks.IDToken); id != "" {
		m.toks.AccountID = id
	}
	m.toks.LastRefresh = m.clock().UTC().Format(time.RFC3339)
	return m.persist()
}

// StoreTokens persists a freshly-exchanged token set (from the login callback),
// filling AccountID from the id_token when absent.
func (m *Manager) StoreTokens(t Tokens) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = m.ensureLoaded()
	if t.AccountID == "" {
		t.AccountID = accountIDFromIDToken(t.IDToken)
	}
	if t.LastRefresh == "" {
		t.LastRefresh = m.clock().UTC().Format(time.RFC3339)
	}
	m.toks = t
	m.loaded = true
	return m.persist()
}

// Account returns the connected account's email + id for display (best-effort).
func (m *Manager) Account() (email, accountID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ensureLoaded() != nil {
		return "", ""
	}
	return jwtEmail(m.toks.IDToken), m.toks.AccountID
}

// Logout clears the stored tokens.
func (m *Manager) Logout() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := creds.NewStore(m.baseDir)
	if err := v.Load(); err != nil {
		return err
	}
	v.Remove(VaultKey)
	m.toks = Tokens{}
	m.loaded = true
	return v.Save()
}

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

// --- token endpoint + JWT helpers ---

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func postToken(ctx context.Context, form url.Values) (*tokenResp, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, tokenEP, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClientFor(20 * time.Second).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out tokenResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("chatgptauth: token endpoint returned non-JSON (status %d)", resp.StatusCode)
	}
	if out.AccessToken == "" {
		if out.ErrorDesc != "" {
			return nil, fmt.Errorf("chatgptauth: %s", out.ErrorDesc)
		}
		if out.Error != "" {
			return nil, fmt.Errorf("chatgptauth: %s", out.Error)
		}
		return nil, fmt.Errorf("chatgptauth: no access_token (status %d)", resp.StatusCode)
	}
	return &out, nil
}

// jwtPayload base64url-decodes a JWT's claims (no signature check — it's our own
// token, used only to read exp / account id / email).
func jwtPayload(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil, false
	}
	return m, true
}

func jwtExp(token string) (time.Time, bool) {
	m, ok := jwtPayload(token)
	if !ok {
		return time.Time{}, false
	}
	exp, ok := m["exp"].(float64)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(int64(exp), 0), true
}

func jwtEmail(token string) string {
	m, ok := jwtPayload(token)
	if !ok {
		return ""
	}
	if e, ok := m["email"].(string); ok {
		return e
	}
	return ""
}

// accountIDFromIDToken extracts chatgpt_account_id from the id_token's custom
// claim namespace.
func accountIDFromIDToken(token string) string {
	m, ok := jwtPayload(token)
	if !ok {
		return ""
	}
	auth, ok := m["https://api.openai.com/auth"].(map[string]any)
	if !ok {
		return ""
	}
	if id, ok := auth["chatgpt_account_id"].(string); ok {
		return id
	}
	return ""
}
