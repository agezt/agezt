# Phase 2 — Auth / Session / CSRF / CORS / Clickjacking / WebSocket results

Scope: kernel/webui (token + console-password login, cookies, SSE), kernel/auth, kernel/httpserver,
kernel/restapi, kernel/openaiapi, kernel/agentgw, kernel/tunnel, kernel/acp, kernel/chatgptauth +
controlplane provider/channel OAuth, cmd/agezt/httpsurfaces.go (bind defaults, tunnel wiring).
Skills applied: sc-auth, sc-session, sc-jwt, sc-csrf, sc-cors, sc-clickjacking, sc-websocket.
Method: static trace of every candidate to its real code path; false positives dropped (list at end).

## Findings

### F1 — Console login lockout is a check-then-act race; concurrent/slow-body requests bypass the 8-attempt limit
- Severity: Medium (Low when the default minted 96-bit password is in use; Medium with an operator-chosen `AGEZT_WEB_PASSWORD`, e.g. behind a tunnel/LAN in non-strict mode)
- CWE: CWE-307 (Improper Restriction of Excessive Authentication Attempts), CWE-367 (TOCTOU)
- Location: kernel/webui/session.go:212-227 (with kernel/httpserver/listener.go:28-34 — no ReadTimeout)
- Snippet:
  ```go
  if s.sessions.lockedOut() {            // check happens BEFORE the body is read
      http.Error(w, "too many attempts — try again later", http.StatusTooManyRequests)
      return
  }
  ...
  if err := dec.Decode(&body); err != nil { ... }   // body read here, unbounded in time
  if subtle.ConstantTimeCompare(...) != 1 {
      s.sessions.noteFail()                 // counter only bumped after the compare
  ```
- Exploit: The lockout gate and the failure counter are separate critical sections, and the gate runs
  before the request body is consumed. The streaming server sets only ReadHeaderTimeout, so a body can be
  dribbled indefinitely. An attacker opens N connections to `POST /api/login` (valid headers, no
  `Sec-Fetch-Site: cross-site`, no/same Origin — trivially satisfied by a non-browser client), lets each pass
  `lockedOut()` while the store is unlocked, then completes all N bodies with N different guesses. Every
  in-flight request is compared even after the 8th failure arms the lock, so the attacker gets N guesses
  per 5-minute window instead of 8. Reachable token-free by anyone who can reach the console (local users /
  processes on loopback, LAN on a non-loopback bind, the public internet through a tunnel when strict mode
  is off).
- Confidence: 85
- Fix: Make the attempt a single atomic reservation: under `s.mu`, reject if locked, otherwise increment an
  in-flight/attempt counter *before* comparing (decrement/reset on success). Read the (4 KB-capped) body
  with a short deadline (`http.NewResponseController(w).SetReadDeadline`) before touching the lockout
  state, and add a per-source-IP bucket.

### F2 — Global (not per-client) login lockout lets anyone lock the operator out of password login
- Severity: Low
- CWE: CWE-645 (Overly Restrictive Account Lockout Mechanism)
- Location: kernel/webui/session.go:52-57, 106-121, 212-214
- Snippet:
  ```go
  // Brute-force bound (shared across sessions — it's a per-daemon gate, not per-session).
  fails       int
  lockedUntil time.Time
  ...
  if s.sessions.lockedOut() { http.Error(w, "too many attempts ...", 429) }  // also refuses the CORRECT password
  ```
- Exploit: A single unauthenticated client sending 8 wrong passwords every 5 minutes keeps the only
  password door closed for everyone; the owner's correct password gets 429. Token-URL access still works, so
  impact is availability of the password door (relevant when the operator relies on password login, e.g.
  through a tunnel).
- Confidence: 90
- Fix: Key the failure counter/lockout by client (RemoteAddr, or a trusted X-Forwarded-For when behind a
  configured proxy) with a bounded map, plus a much higher global ceiling.

### F3 — Sessions and the SSE token survive logout and password change
- Severity: Low
- CWE: CWE-613 (Insufficient Session Expiration)
- Location: kernel/webui/session.go:95-102, 280-287 (only revocation path); kernel/webui/webui.go:175-184
  (sseToken minted once per process), 1413-1415, 179-185 (session.go handleSSEToken)
- Snippet:
  ```go
  buf := make([]byte, 32); _, _ = rand.Read(buf)
  sseToken := hex.EncodeToString(buf)          // one per daemon lifetime, never rotated
  ...
  if r.URL.Path == "/events" {
      if s.sseTokenMatch(r.URL.Query().Get("st")) || ...   // st alone authorizes /events (non-strict)
  ```
- Exploit: (a) `sessions.revoke` is only called by `/api/logout` for the presenting cookie. Changing the
  console password (Setup / Config Center updates the env; `passwordFn` is read live) does not invalidate
  existing sessions, so a leaked/stolen session keeps full data-route access for up to 12 h sliding — i.e.
  indefinitely while used (no absolute lifetime). (b) Any session holder can fetch `/api/sse-token`; that
  token keeps opening `/events` (the full live bus feed: run output, tool calls, etc.) after logout,
  session expiry, or a password change, until daemon restart. It also travels in a URL query string.
- Confidence: 85
- Fix: Store a password generation/hash with each session and reject sessions minted under an older
  password (or `sessions.m = {}` on password change). Add an absolute session lifetime. Bind the SSE token
  to the session (mint per session, revoke with it) or make it short-lived and re-fetched.

### F4 — `/events` still accepts the main admin console token in the query string
- Severity: Low
- CWE: CWE-598 (Use of GET Request Method With Sensitive Query Strings)
- Location: kernel/webui/webui.go:1417-1420
- Snippet:
  ```go
  // Fallback for programmatic / non-browser SSE clients that pass the
  // main token in query ... ONLY as a transition aid.
  return s.tokenMatch(r.URL.Query().Get("token"))
  ```
- Exploit: The dedicated `st` SSE token exists precisely so the long-lived main token never lands in URLs
  (proxy/tunnel access logs, browser history, crash reports). This "transition" fallback keeps
  `GET /events?token=<main token>` valid, so any client or doc still using it leaks the full-power console
  credential (same token that authorizes every mutating `/api/*` route). Behind a tunnel provider the URL is
  logged by a third party.
- Confidence: 70 (depends on clients actually using it; the SPA uses `st`)
- Fix: Remove the fallback (or gate it behind the existing `allowQueryTokensForData` test flag).

### F5 — REST `/metrics` exposes daemon-global spend/activity to per-tenant tokens
- Severity: Low
- CWE: CWE-200 / CWE-285 (cross-tenant information exposure)
- Location: kernel/restapi/restapi.go:212-214 (route tier), 280-302 (handler ignores tenant)
- Snippet:
  ```go
  metricsRoute := userRoute                 // TierUser => satisfiable by a tenant token + X-Agezt-Tenant
  router.Handle("/metrics", metricsRoute, s.handleMetrics)
  ...
  for _, m := range s.metrics() {           // global metrics, no s.bind(r) tenant scoping
  ```
- Exploit: With multi-tenancy enabled, a tenant credential (`X-Agezt-Tenant: t1` + t1's token) passes the
  TierUser gate and reads daemon-wide spend/activity counters that include other tenants. The code's own
  comment calls these "financially/operationally sensitive"; mailbox/update were already moved to admin for
  the same reason (V-011). `/api/v1/health` similarly reports the primary engine's model config.
- Confidence: 75
- Fix: Register `/metrics` with `adminRoute` (`Method: "GET,HEAD"`), or scope metrics per tenant.

### F6 — Tunnel banner/URL decision uses a stale strict-mode snapshot (and overrides an explicit STRICT=off)
- Severity: Info (fails closed; correctness/usability)
- CWE: CWE-1188-adjacent (misleading security posture)
- Location: cmd/agezt/httpsurfaces.go:177, 236-237, 395-409, 462-466; kernel/webui/webui.go:156-160
- Detail: `webUISurface.passwordStrict` is captured at boot. When the tunnel URL appears, `allowHost` →
  `SetAllowedHosts` auto-raises strict mode, but `tunnelPublicURL`/banner still read the stale `false`, so
  the operator is told "public URL opens password login" and given a token-less URL that (now strict) will
  not open data routes. It also silently overrides an explicit `AGEZT_WEB_PASSWORD_STRICT=off`. Not
  exploitable (stricter than advertised), but the AUTH-001 lesson ("read the effective value") is violated
  in the tunnel path. Fix: have `allowHost` return / re-read `wsrv.PasswordStrict()` inside `OnURL`.
- Confidence: 80

### F7 — ChatGPT sign-in callback `error=` branch skips the state check
- Severity: Info
- CWE: CWE-352 (minor, DoS of an in-flight login)
- Location: kernel/controlplane/provider_oauth.go:102-106
- Detail: `http://localhost:1455/auth/callback?error=x` (navigable from any web page while a login is
  pending) marks the login failed and closes the listener without matching `state`. Impact limited to
  aborting the operator's sign-in. Fix: require `state == login.state` before acting on `error` too.
- Confidence: 80

## Verified-safe / dropped candidates
- Token comparisons: `auth.StaticVerifier` uses `subtle.ConstantTimeCompare` for admin + all user tokens
  (no early return); password compare constant-time; workflow webhook secret constant-time and empty
  secret refused (controlplane/workflow.go:373-386).
- DNS rebinding: `secure()` Host allowlist (localhost, IP literals, explicit hosts) wraps every webui
  route incl. public ones; a rebinding hostname is rejected with 403.
- CSRF: all webui mutations are POST-only (method enforced by router), `sameOriginMutation` rejects
  `Sec-Fetch-Site: cross-site` and mismatched/`null` Origin; session cookie is HttpOnly + SameSite=Strict +
  Secure on TLS/forwarded https; login/logout POST-only. REST/OpenAI APIs are Bearer-only (no ambient
  credential) → CSRF not applicable; no CORS headers emitted anywhere (no ACAO in the tree).
- Clickjacking: `X-Frame-Options: DENY` + CSP `frame-ancestors 'none'` on every response incl. 401s;
  strict CSP (`script-src 'self'`), `Referrer-Policy: no-referrer`, nosniff.
- WebSocket: no server-side WebSocket endpoints (coder/websocket used only as a client in the nostr
  channel); ACP is JSON-RPC over stdio.
- agentgw (re-verified): all routes except `/health` behind `withAuth`; JWT alg/typ pinned to HS256/JWT,
  `hmac.Equal`, iss/aud pinned, expiry enforced; `/v1/token/create` requires a parent token, rejects cap
  escalation, clamps expiry/rate/burst, inherits RunID; per-TokenID rate limit with bounded map.
- Session IDs: 256-bit crypto/rand, fresh on each login (no fixation).
- Bind defaults: web UI defaults to 127.0.0.1:8787 (fallback 127.0.0.1:0); REST/OpenAI off unless set;
  wildcard bind or non-loopback host auto-raises strict (token AND session); built-in password is minted
  per install (96-bit), only on loopback.
- Multi-tenant REST/OpenAI: tenant token only authorizes TierUser with the tenant header, and `bind()`
  routes by the same header → no cross-tenant run/artifact access (except F5 metrics).
- Channel OAuth `/oauth/callback`: state validated against server-minted pending map; error output
  HTML-escaped. ChatGPT OAuth: PKCE S256 + 256-bit state, listener on 127.0.0.1 only.
- `/hooks/`: token-free by design; can only ask the control plane to fire a named, enabled, webhook-kind
  workflow with a matching secret; rate-limited pre-auth per name+source; uniform refusals. `?secret=`
  query fallback is a documented convenience (logged-URL exposure noted, not reported).
