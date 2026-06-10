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

// Password second factor (M817). The console's first factor is the URL token
// (banner link / Bearer header). When AGEZT_WEB_PASSWORD is set, a SECOND factor
// is layered on top: the token gets you the page, but every data route also
// requires a valid session cookie minted by POST /api/login with the password.
// "Token alone isn't enough." Password is OPT-IN — unset means token-only, the
// pre-M817 behaviour, consistent with the allow-by-default posture.
//
// The login route is itself token-gated, so an attacker must ALREADY hold the
// token to even present a password — the two factors compose, they don't merely
// sit side by side. A small failed-attempt lockout bounds online guessing for
// the case where the token has leaked (e.g. a shared screenshot of the banner).

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

// SetPassword wires the optional console password (AGEZT_WEB_PASSWORD). Empty =
// token-only. Called once at startup, before Handler(), like SetTranscriber.
func (s *Server) SetPassword(pw string) { s.password = strings.TrimSpace(pw) }

// sessionValid reports whether the request carries a live session cookie.
func (s *Server) sessionValid(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return s.sessions.valid(c.Value)
}

// handleAuthMeta tells the SPA whether a password gate exists and whether this
// request is already past it — so it knows to render the login screen. Itself
// token-gated (only token-holders probe), session-independent.
func (s *Server) handleAuthMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"password_required": s.password != "",
		"authed":            s.password == "" || s.sessionValid(r),
	})
}

// handleLogin verifies the password (constant-time) and mints a session cookie.
// Token-gated by the router, so the caller already holds the first factor; the
// lockout bounds guessing if that token has leaked.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.password == "" {
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
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.password)) != 1 {
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
		Secure:   r.TLS != nil,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleLogout revokes the session and clears the cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
