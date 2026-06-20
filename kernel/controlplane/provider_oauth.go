// SPDX-License-Identifier: MIT

package controlplane

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
	"time"

	"github.com/agezt/agezt/kernel/chatgptauth"
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

// chatgptMgr returns the lazily-built ChatGPT token manager.
func (s *Server) chatgptMgr() *chatgptauth.Manager {
	s.chatgptOnce.Do(func() { s.chatgpt = chatgptauth.NewManager(s.baseDir) })
	return s.chatgpt
}

// handleProviderOAuthStart begins "Sign in with ChatGPT": it starts the 1455
// redirect listener and returns the authorize URL. args: provider ("chatgpt").
func (s *Server) handleProviderOAuthStart(conn net.Conn, req Request) {
	provider := strings.TrimSpace(strings.ToLower(stringArg(req.Args, "provider")))
	if provider == "" {
		provider = "chatgpt"
	}
	if provider != "chatgpt" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: provider + " does not support OAuth sign-in"})
		return
	}
	verifier, challenge, err := chatgptauth.GeneratePKCE()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "pkce: " + err.Error()})
		return
	}
	state, err := chatgptauth.RandomState()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "state: " + err.Error()})
		return
	}

	// Tear down any previous login, then bind the Codex client's fixed redirect.
	s.stopProviderLogin()
	ln, err := net.Listen("tcp", chatgptauth.CallbackAddr)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError,
			Error: "cannot bind " + chatgptauth.CallbackAddr + " for the sign-in redirect (is it in use?): " + err.Error()})
		return
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

	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"authorize_url": chatgptauth.AuthorizeURL(challenge, state),
		"state":         state,
	}})
}

// providerCallback handles the browser redirect on 127.0.0.1:1455.
func (s *Server) providerCallback(w http.ResponseWriter, r *http.Request, login *providerLogin) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.chatgptMgr().ExchangeCode(ctx, code, login.verifier); err != nil {
		s.setProviderLoginStatus(login, "error", err.Error())
		providerLoginPage(w, false, err.Error())
		go s.deferredClose(login)
		return
	}
	s.setProviderLoginStatus(login, "done", "")
	// Bring the provider live without a restart.
	if s.k != nil {
		_, _, _ = s.k.Reload()
	}
	providerLoginPage(w, true, "")
	go s.deferredClose(login)
}

func (s *Server) deferredClose(login *providerLogin) {
	time.Sleep(800 * time.Millisecond)
	if login.srv != nil {
		_ = login.srv.Close()
	}
}

func (s *Server) setProviderLoginStatus(login *providerLogin, status, msg string) {
	s.provLoginMu.Lock()
	login.status = status
	login.errMsg = msg
	s.provLoginMu.Unlock()
}

func (s *Server) stopProviderLogin() {
	s.provLoginMu.Lock()
	l := s.provLogin
	s.provLogin = nil
	s.provLoginMu.Unlock()
	if l != nil && l.srv != nil {
		_ = l.srv.Close()
	}
}

// handleProviderOAuthStatus reports the active login's terminal state plus the
// connected account (best-effort). args: state.
func (s *Server) handleProviderOAuthStatus(conn net.Conn, req Request) {
	state := strings.TrimSpace(stringArg(req.Args, "state"))
	s.provLoginMu.Lock()
	login := s.provLogin
	s.provLoginMu.Unlock()

	status := "unknown"
	errMsg := ""
	if login != nil && (state == "" || state == login.state) {
		status = login.status
		errMsg = login.errMsg
	}
	email, account := s.chatgptMgr().Account()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"status":    status,
		"error":     errMsg,
		"connected": s.chatgptMgr().HasTokens(),
		"email":     email,
		"account":   account,
	}})
}

// handleProviderOAuthImport pulls tokens from a local Codex CLI auth.json.
func (s *Server) handleProviderOAuthImport(conn net.Conn, req Request) {
	path := strings.TrimSpace(stringArg(req.Args, "path"))
	if err := s.chatgptMgr().ImportFromCodexCLI(path); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if s.k != nil {
		_, _, _ = s.k.Reload()
	}
	email, account := s.chatgptMgr().Account()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"ok": true, "connected": true, "email": email, "account": account,
	}})
}

// handleProviderOAuthLogout clears the stored ChatGPT tokens.
func (s *Server) handleProviderOAuthLogout(conn net.Conn, req Request) {
	if err := s.chatgptMgr().Logout(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if s.k != nil {
		_, _, _ = s.k.Reload()
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": true, "connected": false}})
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
