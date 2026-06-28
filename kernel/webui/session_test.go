// SPDX-License-Identifier: MIT

package webui

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// cookieRe pulls the session cookie value out of a Set-Cookie header.
var cookieRe = regexp.MustCompile(sessionCookie + `=([0-9a-f]+)`)

// TestPasswordGate_TokenAloneIsNotEnough — in STRICT mode (M817 compose,
// AGEZT_WEB_PASSWORD_STRICT=on) a valid token is necessary but NOT sufficient
// for a data route; a session cookie from a successful login is also required.
// (The M933 default treats the password as an alternative door instead — see
// TestPasswordGate_AlternativeDoor.)
func TestPasswordGate_TokenAloneIsNotEnough(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	s.SetPasswordFn(func() string { return "hunter2" })
	s.SetPasswordStrict(true)

	// A data route with a good token but NO session → 401 (password required).
	req := httptest.NewRequest(http.MethodGet, "/api/status?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("token-only data request: code=%d, want 401 (session required)", rec.Code)
	}

	// authmeta reports the gate (token-gated, session-independent).
	req = httptest.NewRequest(http.MethodGet, "/api/authmeta?token=secret", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"password_required":true`) {
		t.Fatalf("authmeta: code=%d body=%s, want password_required:true", rec.Code, rec.Body.String())
	}

	// Wrong password → 401, no cookie.
	rec = postLogin(t, s, `{"password":"nope"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad password: code=%d, want 401", rec.Code)
	}

	// Correct password → 200 + a session cookie.
	rec = postLogin(t, s, `{"password":"hunter2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("good password: code=%d, want 200", rec.Code)
	}
	m := cookieRe.FindStringSubmatch(rec.Header().Get("Set-Cookie"))
	if m == nil {
		t.Fatalf("no session cookie set; Set-Cookie=%q", rec.Header().Get("Set-Cookie"))
	}
	sid := m[1]

	// Now token + session cookie → the data route is allowed.
	req = httptest.NewRequest(http.MethodGet, "/api/status?token=secret", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sid})
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("token+session data request: code=%d, want 200", rec.Code)
	}

	// A session cookie WITHOUT the token is still rejected (token is factor one).
	req = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sid})
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("session-only (no token) data request: code=%d, want 401", rec.Code)
	}
}

// TestPasswordGate_AlternativeDoor — the M933 default: with a password set, a
// token-less browser can reach the shell, probe authmeta, log in, and then use
// the data routes on the session cookie ALONE; the token also keeps working
// alone. This is the "open localhost:8787, type the password" flow.
func TestPasswordGate_AlternativeDoor(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	s.SetPasswordFn(func() string { return "hunter2" })

	// Token alone still opens data routes (it is a door, not a half-key).
	req := httptest.NewRequest(http.MethodGet, "/api/status?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("token-only data request: code=%d, want 200 (alternative-door default)", rec.Code)
	}

	// Token-less: the SPA shell must be served so the login screen can render.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("token-less shell: code=%d, want 200 when a password is configured", rec.Code)
	}

	// Token-less authmeta reports the gate and not-authed.
	req = httptest.NewRequest(http.MethodGet, "/api/authmeta", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"password_required":true`) ||
		!strings.Contains(rec.Body.String(), `"authed":false`) {
		t.Fatalf("token-less authmeta: code=%d body=%s", rec.Code, rec.Body.String())
	}

	// Token-less data route without a session → still 401.
	req = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("token-less, session-less data request: code=%d, want 401", rec.Code)
	}

	// Token-less login with the correct password mints a session…
	rec = postLogin(t, s, `{"password":"hunter2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("token-less login: code=%d, want 200", rec.Code)
	}
	m := cookieRe.FindStringSubmatch(rec.Header().Get("Set-Cookie"))
	if m == nil {
		t.Fatalf("no session cookie; Set-Cookie=%q", rec.Header().Get("Set-Cookie"))
	}

	// …and the session ALONE opens data routes.
	req = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: m[1]})
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session-only data request: code=%d, want 200 (password is a door)", rec.Code)
	}
}

// TestNoPassword_TokenlessShellStays401 — with NO password configured the token
// is the only secret, so a token-less "/" must stay unauthorized (with a hint).
func TestNoPassword_TokenlessShellStays401(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("token-less shell without a password: code=%d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("401 body should hint at password setup, got: %s", rec.Body.String())
	}
}

// TestNoPassword_TokenSuffices — the gate is transparent when no password is set.
func TestNoPassword_TokenSuffices(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret") // no SetPassword

	req := httptest.NewRequest(http.MethodGet, "/api/status?token=secret", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("token-only with no password configured: code=%d, want 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/authmeta?token=secret", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"password_required":false`) {
		t.Fatalf("authmeta with no password: body=%s, want password_required:false", rec.Body.String())
	}
}

// TestLogin_LockoutAfterRepeatedFailures — the brute-force bound trips after
// maxLoginFails wrong attempts and refuses further tries with 429.
func TestLogin_LockoutAfterRepeatedFailures(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	s.SetPasswordFn(func() string { return "hunter2" })

	for i := 0; i < maxLoginFails; i++ {
		if rec := postLogin(t, s, `{"password":"x"}`); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: code=%d, want 401", i, rec.Code)
		}
	}
	// Next attempt — even with the CORRECT password — is locked out.
	if rec := postLogin(t, s, `{"password":"hunter2"}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("post-lockout: code=%d, want 429", rec.Code)
	}
}

// TestLogout_RevokesSession — after logout the session no longer authorizes.
func TestLogout_RevokesSession(t *testing.T) {
	fc := &fakeCaller{result: map[string]any{"ok": true}}
	s, _ := newServer(t, fc, "secret")
	s.SetPasswordFn(func() string { return "hunter2" })

	rec := postLogin(t, s, `{"password":"hunter2"}`)
	sid := cookieRe.FindStringSubmatch(rec.Header().Get("Set-Cookie"))[1]
	if !s.sessions.valid(sid) {
		t.Fatal("session not valid after login")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/logout?token=secret", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sid})
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout: code=%d, want 200", rec.Code)
	}
	if s.sessions.valid(sid) {
		t.Fatal("session still valid after logout")
	}
}

func postLogin(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/login?token=secret", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}
