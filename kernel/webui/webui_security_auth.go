// SPDX-License-Identifier: MIT

// WebUI security auth: secure + shellAuth + setSecurityHeaders + hostAllowed + sameOriginMutation + hostName + canonicalHostPort + tokenPresented + dataTokenPresented + tokenPresentedFrom + authorized + tokenMatch + sseTokenMatch.
// Code extracted from webui_security.go during the Day-75 god-file split. Public API unchanged.
package webui


import (
	kernelauth "github.com/agezt/agezt/kernel/auth"
	"net"
	"net/http"
	"net/url"
	"strings"
)


// (the bundle + favicon) that the browser must be able to load without a token.
func (s *Server) secure(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		if !s.hostAllowed(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if !sameOriginMutation(r) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// shellAuth gates the SPA shell: the token always opens it, and when a console
// password is configured (M933) the shell is served credential-free too — it
// has to be, or a token-less browser could never reach the login screen. The
// shell is compiled UI code with no data; route policy still guards every data
// route. Host/Origin checks and security headers are applied outside the router.
// With no password configured a token-less visit gets a hint page instead of a
// bare "unauthorized", pointing at the banner URL / password setup.
func (s *Server) shellAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.tokenPresented(r) && s.consolePassword() == "" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized — open the console via the tokened URL from the daemon banner,\n" +
				"or set a console password (Config Center → Interfaces → Web UI password, or AGEZT_WEB_PASSWORD)\n" +
				"to enable password login at this address.\n"))
			return
		}
		next(w, r)
	}
}

// setSecurityHeaders applies defensive response headers to every web UI route
// (set before the auth check so even 401s carry them). This is a control surface:
//   - X-Frame-Options DENY — the dashboard has state-mutating controls
//     (approve/halt/resume/decide), so framing is denied to block clickjacking.
//   - Referrer-Policy no-referrer — the page URL carries the auth token in
//     `?token=`, so the referrer is suppressed to keep it out of any Referer header.
//   - X-Content-Type-Options nosniff — stop content-type sniffing/confusion.
//   - Content-Security-Policy (static): the SPA loads only external, same-origin
//     hashed JS/CSS, so `script-src 'self'` admits the genuine bundle and refuses
//     any inline/injected script — STRICTER than the old per-nonce scheme (which
//     existed to allow one inline block). `style-src 'self' 'unsafe-inline'` is
//     required because React Flow / Radix inject runtime inline styles (measured
//     transforms) that can't be hashed at build time; 'unsafe-inline' on
//     style-src enables no code execution. `connect-src 'self'` confines fetch +
//     EventSource to the daemon; the rest closes framing/exfil/pivot avenues.
func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy",
		"default-src 'none'; "+
			"script-src 'self'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"connect-src 'self'; "+
			"img-src 'self' data:; "+
			"font-src 'self' data:; "+
			"base-uri 'none'; "+
			"form-action 'none'; "+
			"frame-ancestors 'none'")
}

func (s *Server) hostAllowed(hostport string) bool {
	host := hostName(hostport)
	if host == "" {
		return false
	}
	lower := strings.ToLower(host)
	if lower == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsUnspecified()
	}
	s.hostPolicyMu.RLock()
	defer s.hostPolicyMu.RUnlock()
	return s.allowedHosts[lower]
}

func sameOriginMutation(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(canonicalHostPort(u.Host), canonicalHostPort(r.Host))
}

func hostName(hostport string) string {
	h := strings.TrimSpace(hostport)
	if h == "" {
		return ""
	}
	if strings.HasPrefix(h, "[") {
		if end := strings.Index(h, "]"); end > 0 {
			return strings.TrimSuffix(h[1:end], ".")
		}
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		return strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	}
	if strings.Count(h, ":") == 1 {
		if host, _, ok := strings.Cut(h, ":"); ok {
			return strings.TrimSuffix(host, ".")
		}
	}
	return strings.TrimSuffix(strings.Trim(h, "[]"), ".")
}

func canonicalHostPort(hostport string) string {
	h := strings.TrimSpace(strings.ToLower(hostport))
	if h == "" {
		return ""
	}
	if host, port, err := net.SplitHostPort(h); err == nil {
		return strings.TrimSuffix(strings.Trim(host, "[]"), ".") + ":" + port
	}
	return strings.TrimSuffix(strings.Trim(h, "[]"), ".")
}

// tokenPresented reports whether the request carries the valid console token.
// Shell/deep-link requests still accept ?token= because the daemon banner URL
// is how an operator first opens the console.
func (s *Server) tokenPresented(r *http.Request) bool {
	return s.tokenPresentedFrom(r, true)
}

func (s *Server) dataTokenPresented(r *http.Request) bool {
	// /events uses the ephemeral SSE token (minted once at startup) in the
	// query string instead of the main console token, because the browser's
	// EventSource API cannot set custom headers (VULN query-string-token).
	// Other data routes default to Bearer header / session cookie; the
	// allowQueryTokensForData legacy flag preserves ?token= for test support.
	if r.URL.Path == "/events" {
		if s.sseTokenMatch(r.URL.Query().Get("st")) || s.tokenPresentedFrom(r, false) {
			return true
		}
		// Fallback for programmatic / non-browser SSE clients that pass the
		// main token in query (before the SPA was updated). Accept the main
		// token in query ONLY for /events and ONLY as a transition aid.
		return s.tokenMatch(r.URL.Query().Get("token"))
	}
	allowQuery := s.allowQueryTokensForData
	return s.tokenPresentedFrom(r, allowQuery)
}

func (s *Server) tokenPresentedFrom(r *http.Request, allowQuery bool) bool {
	if s.token == "" {
		return false // never serve without a configured token
	}
	if allowQuery {
		if s.tokenMatch(r.URL.Query().Get("token")) {
			return true
		}
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return s.tokenMatch(strings.TrimPrefix(h, "Bearer "))
	}
	return false
}

// authorized gates a DATA route. Default (M933): the token OR a password
// session opens it — the password is an alternative door, so a token-less
// browser that logged in with the console password works. Strict mode
// (AGEZT_WEB_PASSWORD_STRICT=on) restores M817 compose: token AND session,
// for operators who exposed the console beyond loopback and want two factors.
// With no password configured, the token alone suffices (pre-M817 behaviour).
func (s *Server) authorized(r *http.Request) bool {
	pw := s.consolePassword()
	if pw == "" {
		return s.dataTokenPresented(r)
	}
	if s.passwordStrictOn() {
		return s.dataTokenPresented(r) && s.sessionValid(r)
	}
	return s.dataTokenPresented(r) || s.sessionValid(r)
}

// tokenMatch compares a presented token against the configured one in CONSTANT
// TIME, so an attacker who can reach the web UI can't recover the token
// byte-by-byte by timing the auth check. Mirrors the control-plane's
// shared auth verifier. Caller guarantees s.token != "".
func (s *Server) tokenMatch(presented string) bool {
	return s.tokenAuth != nil && s.tokenAuth.Authorize(presented, kernelauth.TierAdmin)
}

// sseTokenMatch compares a presented token against the ephemeral SSE token
// (minted once at startup). Used for the /events ?st= parameter — the browser's
// EventSource API cannot set custom headers, so the SSE token travels in the
// query string through a loopback-bound surface with no third party exposure.
func (s *Server) sseTokenMatch(presented string) bool {
	return s.sseAuth != nil && s.sseAuth.Authorize(presented, kernelauth.TierAdmin)
}

// handleSPA serves the embedded React single-page app. The hashed JS/CSS live
// under /assets/ (see handleAssets); this serves index.html at "/" and for any
// other non-API, non-asset path (client-side deep links like /runs), so a
// refresh on a sub-view re-loads the app rather than 404-ing. index.html is
// served no-cache so a daemon upgrade (new asset hashes) is picked up
// immediately rather than showing a stale shell that points at gone assets.
