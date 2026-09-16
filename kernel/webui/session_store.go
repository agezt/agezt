// SPDX-License-Identifier: MIT

// session_store.go owns the in-memory sessionStore
// (minted-id → expiry map, sliding TTL, failed-attempt
// lockout counters). Mutex-guarded; dies with the daemon.
// The Server-side configuration knobs (SetPasswordFn,
// SetPasswordStrict, PasswordStrict, consolePassword,
// sessionValid, handleLogin/Logout/etc.) live in
// session.go.
package webui

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
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
