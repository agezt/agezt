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
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/chatgptauth"
	"github.com/agezt/agezt/kernel/platform/browsercallback"
	"github.com/agezt/agezt/kernel/runtime"
)

const providerLoginTTL = 5 * time.Minute

// providerLogin is the single in-flight provider OAuth login.
type providerLogin struct {
	provider   string
	state      string
	verifier   string
	status     string // pending | done | error
	errMsg     string
	srv        *browsercallback.Listener
	expiryStop chan struct{}
	expiryDone chan struct{}
	expiryOnce sync.Once
}

type OAuth struct {
	k           *runtime.Kernel
	baseDir     string
	chatgptOnce sync.Once
	chatgpt     *chatgptauth.Manager
	provLoginMu sync.Mutex
	provLogin   *providerLogin
	chatgptSync func() ([]string, string)
	fetchTokens func(context.Context, string, string) (chatgptauth.Tokens, error)
}

func NewOAuth(k *runtime.Kernel, baseDir string, syncModels func() ([]string, string)) *OAuth {
	s := &OAuth{k: k, baseDir: baseDir, chatgptSync: syncModels}
	s.fetchTokens = func(ctx context.Context, code, verifier string) (chatgptauth.Tokens, error) {
		return s.chatgptMgr().ExchangeTokens(ctx, code, verifier)
	}
	return s
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
	login := &providerLogin{provider: provider, state: state, verifier: verifier, status: "pending", expiryStop: make(chan struct{}), expiryDone: make(chan struct{})}
	listener, err := browsercallback.Prepare(chatgptauth.CallbackAddr, s.providerCompletion(login), func() { s.deferredClose(login) })
	if err != nil {
		return OAuthStartOutput{}, fmt.Errorf("cannot bind %s for the sign-in redirect (is it in use?): %w", chatgptauth.CallbackAddr, err)
	}
	login.srv = listener

	s.provLoginMu.Lock()
	s.provLogin = login
	s.provLoginMu.Unlock()

	go func() { _ = login.srv.Serve() }()
	// Auto-expire only while this login owns its listener.
	go s.expireProviderLogin(login, providerLoginTTL)

	return OAuthStartOutput{AuthorizeURL: chatgptauth.AuthorizeURL(challenge, state), State: state}, nil
}

// providerCompletion binds the captured login to the socket-free callback business.
func (s *OAuth) providerCompletion(login *providerLogin) browsercallback.Complete {
	return func(ctx context.Context, code, state, denial string) (bool, string, bool) {
		result := s.completeProviderLogin(ctx, login, providerCallbackInput{Code: code, State: state, Error: denial}, func(ctx context.Context, code, verifier string) error {
			tokens, err := s.fetchTokens(ctx, code, verifier)
			if err != nil {
				return err
			}
			return s.persistProviderTokens(ctx, login, tokens)
		})
		return result.Success, result.Message, result.Close
	}
}

func (s *OAuth) deferredClose(login *providerLogin) {
	time.Sleep(800 * time.Millisecond)
	s.closeProviderLogin(login)
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
	s.closeProviderLogin(l)
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
	s.provLoginMu.Lock()
	if err := s.chatgptMgr().Logout(); err != nil {
		s.provLoginMu.Unlock()
		return OAuthLogoutOutput{}, err
	}
	login := s.provLogin
	s.provLogin = nil
	s.provLoginMu.Unlock()
	s.closeProviderLogin(login)
	if s.k != nil {
		_, _, _ = s.k.Reload()
	}
	return OAuthLogoutOutput{OK: true, Connected: false}, nil
}
