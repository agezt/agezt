// SPDX-License-Identifier: MIT

package providers

// "Sign in with ChatGPT" provider login. Unlike the channel OAuth flow (which
// uses the daemon's public /oauth/callback), this impersonates the Codex CLI
// client, whose redirect URI is fixed to http://localhost:1455/auth/callback —
// so login spins a one-shot listener on 127.0.0.1:1455 that captures the code,
// exchanges it via chatgptauth, and stores the tokens in the vault. The provider
// goes live on the next kernel reload (triggered here).

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/chatgptauth"
	"github.com/agezt/agezt/kernel/runtime"
)

const providerLoginTTL = 5 * time.Minute

// providerLogin is the single in-flight provider OAuth login.
type providerLogin struct {
	provider string
	state    string
	verifier string
	status   string // pending | done | error
	errMsg   string
	srv      *http.Server
}

type OAuth struct {
	k           *runtime.Kernel
	baseDir     string
	chatgptOnce sync.Once
	chatgpt     *chatgptauth.Manager
	provLoginMu sync.Mutex
	provLogin   *providerLogin
	chatgptSync func() ([]string, string)
}

func NewOAuth(k *runtime.Kernel, baseDir string, syncModels func() ([]string, string)) *OAuth {
	return &OAuth{k: k, baseDir: baseDir, chatgptSync: syncModels}
}

type OAuthStartInput struct {
	Provider string `json:"provider,omitempty"`
}
type OAuthStatusInput struct {
	State string `json:"state,omitempty"`
}
type OAuthImportInput struct {
	Path string `json:"path,omitempty"`
}
type OAuthLogoutInput struct{}

// chatgptMgr returns the lazily-built ChatGPT token manager.
func (s *OAuth) chatgptMgr() *chatgptauth.Manager {
	s.chatgptOnce.Do(func() { s.chatgpt = chatgptauth.NewManager(s.baseDir) })
	return s.chatgpt
}

// handleProviderOAuthStart begins "Sign in with ChatGPT": it starts the 1455
// redirect listener and returns the authorize URL. args: provider ("chatgpt").
func (s *OAuth) Start(_ context.Context, in OAuthStartInput) (OAuthStartOutput, error) {
	provider := strings.TrimSpace(strings.ToLower(in.Provider))
	if provider == "" {
		provider = "chatgpt"
	}
	if provider != "chatgpt" {
		return OAuthStartOutput{}, fmt.Errorf("%s does not support OAuth sign-in", provider)
	}
	verifier, challenge, err := chatgptauth.GeneratePKCE()
	if err != nil {
		return OAuthStartOutput{}, fmt.Errorf("pkce: %w", err)
	}
	state, err := chatgptauth.RandomState()
	if err != nil {
		return OAuthStartOutput{}, fmt.Errorf("state: %w", err)
	}

	// Tear down any previous login, then bind the Codex client's fixed redirect.
	s.stopProviderLogin()
	ln, err := net.Listen("tcp", chatgptauth.CallbackAddr)
	if err != nil {
		return OAuthStartOutput{}, fmt.Errorf("cannot bind %s for the sign-in redirect (is it in use?): %w", chatgptauth.CallbackAddr, err)
	}
	login := &providerLogin{provider: provider, state: state, verifier: verifier, status: "pending"}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) { s.providerCallback(w, r, login) })
	login.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	s.provLoginMu.Lock()
	s.provLogin = login
	s.provLoginMu.Unlock()

	go func() { _ = login.srv.Serve(ln) }()
	// Auto-expire so a never-completed login doesn't hold the port forever.
	go func() {
		time.Sleep(providerLoginTTL)
		s.provLoginMu.Lock()
		if login.status == "pending" {
			login.status = "error"
			login.errMsg = "sign-in timed out"
		}
		s.provLoginMu.Unlock()
		_ = login.srv.Close()
	}()

	return OAuthStartOutput{AuthorizeURL: chatgptauth.AuthorizeURL(challenge, state), State: state}, nil
}

// providerCallback handles the browser redirect on 127.0.0.1:1455.
func (s *OAuth) providerCallback(w http.ResponseWriter, r *http.Request, login *providerLogin) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		s.setProviderLoginStatus(login, "error", "authorization denied: "+e)
		providerLoginPage(w, false, "Authorization was denied.")
		go s.deferredClose(login)
		return
	}
	code, state := q.Get("code"), q.Get("state")
	if code == "" || state != login.state {
		providerLoginPage(w, false, "Invalid or expired sign-in. Start again from the console.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.chatgptMgr().ExchangeCode(ctx, code, login.verifier); err != nil {
		s.setProviderLoginStatus(login, "error", err.Error())
		providerLoginPage(w, false, err.Error())
		go s.deferredClose(login)
		return
	}
	s.setProviderLoginStatus(login, "done", "")
	// Bring the provider live without a restart, then refresh the catalog from
	// the backend — the served Codex model ids change over time, and a stale
	// entry leaves the console offering models the backend no longer knows.
	if s.k != nil {
		_, _, _ = s.k.Reload()
	}
	s.syncChatGPTModels()
	providerLoginPage(w, true, "")
	go s.deferredClose(login)
}

func (s *OAuth) deferredClose(login *providerLogin) {
	time.Sleep(800 * time.Millisecond)
	if login.srv != nil {
		_ = login.srv.Close()
	}
}

func (s *OAuth) setProviderLoginStatus(login *providerLogin, status, msg string) {
	s.provLoginMu.Lock()
	login.status = status
	login.errMsg = msg
	s.provLoginMu.Unlock()
}

func (s *OAuth) stopProviderLogin() {
	s.provLoginMu.Lock()
	l := s.provLogin
	s.provLogin = nil
	s.provLoginMu.Unlock()
	if l != nil && l.srv != nil {
		_ = l.srv.Close()
	}
}

// syncChatGPTModels refreshes the catalog entry from the backend and returns the
// served model surface. Safe to call with no hook wired (returns nothing).
func (s *OAuth) syncChatGPTModels() (models []string, defaultModel string) {
	if s.chatgptSync == nil {
		return nil, ""
	}
	return s.chatgptSync()
}

// handleProviderOAuthStatus reports the active login's terminal state plus the
// connected account (best-effort). args: state.
func (s *OAuth) Status(_ context.Context, in OAuthStatusInput) (OAuthStatusOutput, error) {
	state := strings.TrimSpace(in.State)

	// Read the fields INSIDE the critical section, not just the pointer (GO-001).
	// provLoginMu guards providerLogin.status/errMsg — setProviderLoginStatus and
	// the TTL expiry goroutine both write them under it — so copying the pointer
	// out and dereferencing after the unlock protected nothing: an operator
	// polling status while the browser callback lands raced on both fields.
	// (state and verifier are safe unlocked: written once before the callback
	// server goroutine starts, so the `go` statement orders them.)
	status := "unknown"
	errMsg := ""
	s.provLoginMu.Lock()
	if login := s.provLogin; login != nil && (state == "" || state == login.state) {
		status = login.status
		errMsg = login.errMsg
	}
	s.provLoginMu.Unlock()
	email, account := s.chatgptMgr().Account()
	connected := s.chatgptMgr().HasTokens()
	// Report the models the backend actually serves so the console can pin a live
	// default instead of a hardcoded id.
	var models []string
	var defaultModel string
	if connected {
		models, defaultModel = s.syncChatGPTModels()
	}
	return OAuthStatusOutput{Status: status, Error: errMsg, Connected: connected, Email: email, Account: account, Models: models, DefaultModel: defaultModel}, nil
}

// handleProviderOAuthImport pulls tokens from a local Codex CLI auth.json.
func (s *OAuth) Import(_ context.Context, in OAuthImportInput) (OAuthImportOutput, error) {
	path := strings.TrimSpace(in.Path)
	if err := s.chatgptMgr().ImportFromCodexCLI(path); err != nil {
		return OAuthImportOutput{}, err
	}
	if s.k != nil {
		_, _, _ = s.k.Reload()
	}
	models, defaultModel := s.syncChatGPTModels()
	email, account := s.chatgptMgr().Account()
	return OAuthImportOutput{OK: true, Connected: true, Email: email, Account: account, Models: models, DefaultModel: defaultModel}, nil
}

// handleProviderOAuthLogout clears the stored ChatGPT tokens.
func (s *OAuth) Logout(_ context.Context, _ OAuthLogoutInput) (OAuthLogoutOutput, error) {
	if err := s.chatgptMgr().Logout(); err != nil {
		return OAuthLogoutOutput{}, err
	}
	if s.k != nil {
		_, _, _ = s.k.Reload()
	}
	return OAuthLogoutOutput{OK: true, Connected: false}, nil
}

// providerLoginPage renders the minimal browser-facing result page.
func providerLoginPage(w http.ResponseWriter, ok bool, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title, detail := "Signed in ✓", "You can close this window and return to the console."
	if !ok {
		title, detail = "Sign-in failed", htmlEscapeProv(msg)
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>%s</title>`+
		`<body style="font:16px system-ui;display:grid;place-items:center;height:100vh;margin:0;background:#0b1020;color:#e6e8f0">`+
		`<div style="text-align:center;max-width:32rem;padding:2rem"><h1 style="font-size:1.4rem">%s</h1>`+
		`<p style="opacity:.8">%s</p></div><script>setTimeout(function(){window.close()},1500)</script>`,
		title, title, detail)
}

func htmlEscapeProv(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
