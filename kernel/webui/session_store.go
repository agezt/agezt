// SPDX-License-Identifier: MIT

package webui

// session_store.go owns the in-memory sessionStore (minted-id → expiry
// map, sliding TTL, failed-attempt lockout counters). Mutex-guarded;
// dies with the daemon. The Server-side configuration knobs
// (SetPasswordFn, SetPasswordStrict, PasswordStrict, consolePassword,
// sessionValid, handleLogin/Logout/etc.) live in session.go.

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

	// Brute-force bound. Per client (remote address): consecutive failures and
	// that client's cooldown. It used to be one shared counter, so eight wrong
	// guesses from anyone locked everyone out. Plus a global backstop for
	// guesses spread over many addresses (globalFails within the window that
	// started at globalSince).
	clients           map[string]*loginFails
	globalFails       int
	globalSince       time.Time
	globalLockedUntil time.Time
}

// loginFails is one client's failure record.
type loginFails struct {
	n           int
	lockedUntil time.Time
	last        time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{m: map[string]time.Time{}, clients: map[string]*loginFails{}}
}

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

// lockedOut reports whether login from client is in cooldown: that client
// failed too often, or the global backstop tripped.
func (s *sessionStore) lockedOut(client string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if now.Before(s.globalLockedUntil) {
		return true
	}
	c, ok := s.clients[client]
	return ok && now.Before(c.lockedUntil)
}

// noteFail records a failed attempt from client, arming that client's lockout
// at maxLoginFails and the global one at maxGlobalLoginFails per window.
func (s *sessionStore) noteFail(client string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()

	c, ok := s.clients[client]
	if !ok {
		if len(s.clients) >= maxTrackedLoginClients {
			s.pruneClients(now)
		}
		c = &loginFails{}
		s.clients[client] = c
	}
	c.n++
	c.last = now
	if c.n >= maxLoginFails {
		c.lockedUntil = now.Add(loginLockout)
		c.n = 0
	}

	if now.Sub(s.globalSince) > loginLockout {
		s.globalSince, s.globalFails = now, 0
	}
	s.globalFails++
	if s.globalFails >= maxGlobalLoginFails {
		s.globalLockedUntil = now.Add(loginLockout)
		s.globalSince, s.globalFails = now, 0
	}
}

// pruneClients drops records that are neither locked nor recently active,
// and, if that frees nothing, the whole map (the global backstop keeps
// bounding guesses meanwhile). Caller holds s.mu.
func (s *sessionStore) pruneClients(now time.Time) {
	for k, c := range s.clients {
		if now.After(c.lockedUntil) && now.Sub(c.last) > loginLockout {
			delete(s.clients, k)
		}
	}
	if len(s.clients) >= maxTrackedLoginClients {
		s.clients = map[string]*loginFails{}
	}
}

// noteSuccess clears client's failure record on a correct password.
func (s *sessionStore) noteSuccess(client string) {
	s.mu.Lock()
	delete(s.clients, client)
	s.mu.Unlock()
}
