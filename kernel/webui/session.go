// SPDX-License-Identifier: MIT

package webui

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Console password (M817 second factor → M933 alternative door). When
// AGEZT_WEB_PASSWORD is set, the password is by default an ALTERNATIVE first
// factor: visiting the console WITHOUT the URL token shows a login screen, and
// a correct password mints a session cookie that opens the data routes — so an
// operator can just browse to localhost:8787 and log in, no banner URL needed.
// The token keeps working on its own (banner link, Bearer API callers).
//
// AGEZT_WEB_PASSWORD_STRICT=on restores the original M817 compose semantics:
// the token gets you the page but every data route ALSO requires the password
// session ("token alone isn't enough") — for operators who exposed the console
// beyond loopback (tunnel) and want two factors, not two doors.
//
// Unset password = token-only, the pre-M817 behaviour, consistent with the
// allow-by-default posture. A failed-attempt lockout bounds online guessing on
// the (now token-free) login route.

const (
	sessionCookie = "agezt_web_session"
	sessionTTL    = 12 * time.Hour

	// Online-guess lockout: after this many consecutive bad passwords, refuse
	// further attempts for the cooldown. Reset on a correct password.
	maxLoginFails  = 8
	loginLockout   = 5 * time.Minute
	loginBodyLimit = 4 * 1024
)

// sessionStore holds minted browser sessions in memory (id → expiry). Sessions
// die with the daemon — there is no persistence; a restart simply asks the
// operator to log in again, which is the safe default for a credentialed
// surface. Access is mutex-guarded; expiry is sliding (each valid check extends
// the window) so an active session isn't logged out mid-use.
type sessionStore struct {
	mu sync.Mutex
	m  map[string]time.Time

	// Brute-force bound (shared across sessions — it's a per-daemon gate, not
	// per-session). fails counts consecutive failures; lockedUntil holds the
	// cooldown deadline.
	fails       int
	lockedUntil time.Time
}

func newSessionStore() *sessionStore { return &sessionStore{m: map[string]time.Time{}} }

// create mints a fresh random session id and records its expiry.
func (s *sessionStore) create() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	s.mu.Lock()
	s.m[id] = time.Now().Add(sessionTTL)
	s.mu.Unlock()
	return id, nil
}

// valid reports whether id names a live session, extending its window (sliding
// expiry) when so. Expired ids are reaped on access.
func (s *sessionStore) valid(id string) bool {
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.m[id]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.m, id)
		return false
	}
	s.m[id] = time.Now().Add(sessionTTL)
	return true
}

// revoke drops a session (logout).
func (s *sessionStore) revoke(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	delete(s.m, id)
	s.mu.Unlock()
}

// lockedOut reports whether login is currently in cooldown after too many bad
// attempts.
func (s *sessionStore) lockedOut() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.lockedUntil)
}

// noteFail records a failed attempt, arming the lockout at the threshold.
func (s *sessionStore) noteFail() {
	s.mu.Lock()
	s.fails++
	if s.fails >= maxLoginFails {
		s.lockedUntil = time.Now().Add(loginLockout)
		s.fails = 0
	}
	s.mu.Unlock()
}

// noteSuccess clears the failure counter on a correct password.
func (s *sessionStore) noteSuccess() {
	s.mu.Lock()
	s.fails = 0
	s.lockedUntil = time.Time{}
	s.mu.Unlock()
}

// SetPasswordFn wires a LIVE password source (M933): evaluated on each gate
// decision, so a password set from the Setup wizard / Config Center (which
// updates the process env) takes effect without a daemon restart.
func (s *Server) SetPasswordFn(fn func() string) { s.passwordFn = fn }

// SetPasswordStrict restores the M817 compose semantics (token AND password
// session on every data route) instead of the M933 alternative-door default.
func (s *Server) SetPasswordStrict(on bool) {
	s.hostPolicyMu.Lock()
	s.passwordStrict = on
	s.hostPolicyMu.Unlock()
}

func (s *Server) passwordStrictOn() bool {
	s.hostPolicyMu.RLock()
	defer s.hostPolicyMu.RUnlock()
	return s.passwordStrict
}

// consolePassword returns the live console password. Empty = no password gate.
func (s *Server) consolePassword() string {
	if s.passwordFn != nil {
		return strings.TrimSpace(s.passwordFn())
	}
	return ""
}

// sessionValid reports whether the request carries a live session cookie.
func (s *Server) sessionValid(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return s.sessions.valid(c.Value)
}

// handleSSEToken returns the ephemeral SSE-only token (VULN query-string-
// token fix): the SPA wraps it as ?st=<token> in the EventSource URL for
// /events, so the main console bearer token never appears in a URL query
// string. This handler is itself token-gated (auth), so only an already-
// authenticated caller learns the SSE token.
func (s *Server) handleSSEToken(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": s.sseToken})
}

// handleAuthMeta tells the SPA whether a password gate exists and whether this
// request can already reach the data routes — so it knows to render the login
// screen. Token-FREE (M933): a token-less browser must be able to probe before
// logging in. Leaks only the existence of a password gate, never data.
func (s *Server) handleAuthMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"password_required": s.consolePassword() != "",
		"authed":            s.authorized(r),
	})
}

// handleLogin verifies the password (constant-time) and mints a session cookie.
// Token-FREE since M933 (it is the token-less door); the failed-attempt lockout
// bounds online guessing.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	password := s.consolePassword()
	if password == "" {
		// No gate configured — nothing to authenticate against.
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "password_required": false})
		return
	}
	if s.sessions.lockedOut() {
		http.Error(w, "too many attempts — try again later", http.StatusTooManyRequests)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, loginBodyLimit))
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(password)) != 1 {
		s.sessions.noteFail()
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}
	s.sessions.noteSuccess()
	id, err := s.sessions.create()
	if err != nil {
		http.Error(w, "session mint failed", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   cookieSecure(r),
		MaxAge:   int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// cookieSecure decides whether the session cookie carries the Secure attribute.
// Direct TLS (r.TLS) is the obvious case. But the console-password feature is
// aimed at deployments behind a TLS-terminating reverse proxy (nginx/Caddy/
// Cloudflare), where the app speaks plaintext on loopback and r.TLS is nil — so
// `Secure: r.TLS != nil` would ship the session id over a cookie that the
// browser is free to resend on a plaintext hop (CWE-614/CWE-311). We therefore
// also honor the proxy's X-Forwarded-Proto / X-Forwarded-Ssl hint.
//
// Trusting the forwarded header here is safe without a proxy allowlist: it can
// only ADD the Secure attribute. A client that spoofs X-Forwarded-Proto: https
// over plaintext just makes its own browser refuse to resend the cookie over
// http — a self-inflicted failure, not a way to leak another user's session.
// Plain http://localhost (no TLS, no proxy header) keeps Secure off so local
// development over HTTP still works.
func cookieSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	// X-Forwarded-Proto may be a comma-separated list (proxies append); the
	// left-most value is the original client-facing scheme.
	proto := r.Header.Get("X-Forwarded-Proto")
	if i := strings.IndexByte(proto, ','); i >= 0 {
		proto = proto[:i]
	}
	if strings.EqualFold(strings.TrimSpace(proto), "https") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Ssl")), "on")
}

// handleLogout revokes the session and clears the cookie. POST-only: a
// state-changing endpoint must not act on a GET (defense-in-depth atop the
// SameSite=Strict cookie), matching handleLogin.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   cookieSecure(r),
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
