# API / Client-Side Security Audit — AGEZT

Scope: API security (REST / OpenAI-compat / control-plane), rate limiting & DoS,
CSRF, CORS, clickjacking, WebSocket. Read-only review of `kernel/webui`,
`kernel/controlplane`, `kernel/openaiapi`, `kernel/restapi`, `kernel/agentgw`,
`cmd/agezt`.

## Surface map

| Surface | Bind | Auth | Transport |
|---|---|---|---|
| Web console (`kernel/webui`) | `127.0.0.1:8787` (default-ON, `AGEZT_WEB_ADDR`), TCP | token `?token=`/Bearer **OR** password session cookie | HTTP + SSE |
| OpenAI-compat (`kernel/openaiapi`) | operator-chosen TCP, off by default | Bearer/`?token=` every route | HTTP + SSE |
| REST (`kernel/restapi`) | operator-chosen TCP, off by default | Bearer/`?token=` every route | HTTP + SSE |
| Control plane (`kernel/controlplane`) | `127.0.0.1` random port, runtime file | constant-time token | line-delimited (not HTTP) |
| Agent gateway (`kernel/agentgw`) | **unix socket** (TCP only as fallback) | per-run JWT-like claims + capability check | HTTP |

Overall the API layer is in good shape: bearer/constant-time auth on every data
route, 16 MiB body caps with `MaxBytesReader` everywhere, slow-loris timeouts,
no inbound WebSocket server, and **no CORS headers anywhere** (a deliberate,
correct choice). The one real residual is the console's CSRF posture in
password-session mode combined with the absence of a Host/Origin check (DNS
rebinding). Details below.

---

## Findings

### FINDING 1 — No CSRF protection on console mutations; cookie-only auth in default password mode (DNS-rebinding amplifies it)  — Severity: MEDIUM (HIGH if console exposed beyond loopback via tunnel without STRICT)

**Files:** `kernel/webui/webui.go:1064-1073` (`authorized`), `kernel/webui/session.go:211-219` (cookie), `kernel/webui/webui.go:562-617` (route table — no Origin/Host gate).

**Misconfig.** When a console password is configured, the default
(non-STRICT) gate is:
```go
// webui.go:1072
return s.tokenPresented(r) || s.sessionValid(r)
```
So a **session cookie alone** authorizes every mutating `POST /api/*`
(writeProxy/jsonProxy/run/market-install/toolbox-install/etc.). There is **no
CSRF token** on any route and **no Origin/Host validation** anywhere in the
handler chain (`secure` only sets response headers; `auth` only checks the
token/session — neither inspects `Origin`, `Referer`, or `Host`).

The cookie is `HttpOnly` + `SameSite=Strict` + `Secure`-when-TLS
(`session.go:215-217`), and mutations are POST-only — so the practical CSRF risk
is bounded by SameSite=Strict, which modern browsers honor for cross-site POSTs.
That is the *only* thing standing between a malicious website and a forged,
authenticated console mutation when the operator is logged in.

**Exploit (DNS rebinding — the angle that defeats SameSite).** Because there is
no `Host` header allowlist on the console, a remote attacker page can:
1. Serve `evil.com` resolving to attacker IP, then rebind its A record to
   `127.0.0.1`.
2. The victim's browser, now treating `evil.com` as same-origin with the
   attacker's *own* page (not cross-site), issues `fetch('http://evil.com:8787/api/agent/edit', {method:'POST', credentials:'include', ...})`. SameSite=Strict does **not** block this — the request is same-site from the browser's view (origin is `evil.com`), but it lands on `127.0.0.1:8787`.
3. With a live session cookie the request is authorized and the mutation
   executes (edit an agent's model/soul, install a market pack, kick off a run,
   etc.). Token mode is unaffected (attacker can't read the `?token=`), but the
   **password-session door is reachable** because the cookie auto-attaches.

SameSite=Strict also does *not* defend the token-in-URL door if the token ever
leaks, but the cookie path is the concrete gap here.

**Why STRICT only half-fixes it.** `AGEZT_WEB_PASSWORD_STRICT=on` requires
`tokenPresented(r) && sessionValid(r)` (webui.go:1070) — the attacker would also
need the `?token=`, which a cross-origin page cannot read, so STRICT closes the
rebinding-CSRF hole. But STRICT is **opt-in**, and the M933 default is the
single-door OR mode precisely for the tunnel-exposure scenario where this matters
most.

**Fix (defense in depth, cheap):**
1. Add a Host allowlist to the console handler chain: reject any request whose
   `Host` header is not `127.0.0.1[:port]`, `localhost[:port]`, or an operator-
   configured public hostname. This single check kills DNS rebinding for all
   routes and is the standard mitigation for localhost daemons. Apply it inside
   `secure()` so even 401s are covered.
2. For data routes that mutate, also enforce an `Origin`/`Sec-Fetch-Site` check
   (reject `Origin` not matching the bound host; accept missing Origin for
   same-origin GETs and native clients). `Sec-Fetch-Site: cross-site` → 403.
3. Consider making STRICT (or at least the Host check) the default when a
   password is set, since that mode targets exposed consoles.

---

### FINDING 2 — `?token=` query-param auth on every HTTP surface (token leaks via logs / proxy / history / Referer)  — Severity: LOW–MEDIUM

**Files:** `kernel/restapi/restapi.go:293-298` (`bearerToken`), `kernel/openaiapi/openaiapi.go:259-264`, `kernel/webui/webui.go:1045-1056` (`tokenPresented`).

**Misconfig.** All three HTTP surfaces accept the admin token in the URL query
string (`?token=`) as a fallback to the `Authorization: Bearer` header. The
token is the daemon's most privileged credential. In the URL it lands in:
reverse-proxy/access logs, browser history, and — absent the existing
`Referrer-Policy: no-referrer` (which only the *webui* sets, not restapi /
openaiapi) — the `Referer` header of any subresource the page loads.

**Exploit.** Anyone with read access to a proxy/access log or the operator's
browser history recovers the full-privilege token and drives the API. On the
REST/OpenAI surfaces there is no `Referrer-Policy`, so a token-in-URL page that
loads any off-origin resource leaks it in `Referer`.

**Mitigating context.** `?token=` exists because `EventSource` (SSE) can't set
headers — a genuine browser constraint. The webui already sets
`Referrer-Policy: no-referrer`.

**Fix.** Prefer Bearer header for non-SSE routes; restrict `?token=` acceptance
to the SSE/`EventSource` routes that actually need it (`/events`,
`/v1/chat/completions` stream, REST stream). Add `Referrer-Policy: no-referrer`
to restapi/openaiapi responses. Document that exposing these surfaces should use
header auth.

---

### FINDING 3 — SSE event streams have no per-connection / per-IP cap  — Severity: LOW

**Files:** `kernel/webui/webui.go:1161-` (`handleEvents`), `kernel/agentgw/handlers.go:15-79` (`handleEventbusSubscribe`), `kernel/openaiapi` & `kernel/restapi` stream paths.

**Misconfig.** SSE subscriptions are token/claims-gated but unbounded in count.
Each open stream holds a bus subscription (256–1024-buffered channel) and a
goroutine for the connection's lifetime.

**Exploit.** An authenticated-but-hostile or compromised client (or, on the
gateway, a buggy agent subprocess) opens thousands of concurrent `/events` /
`eventbus.subscribe` streams to exhaust goroutines/fan-out memory. Auth bounds
this to credential holders, so impact is limited.

**Fix.** Cap concurrent SSE streams per token/run (a counting semaphore on
subscribe, 503 over the cap). Low priority given auth gating.

---

### FINDING 4 — `handleMemorySearch` limit parsed with `fmt.Sscanf`, no upper bound  — Severity: LOW

**File:** `kernel/agentgw/handlers.go:200-210`.

```go
limit := 20
if l := r.URL.Query().Get("limit"); l != "" {
    fmt.Sscanf(l, "%d", &limit)   // no clamp; negative/huge accepted
}
results, err := g.mem.Recall(claims.RunID, query, limit)
```
A negative or enormous `limit` is passed straight to `Recall`. Depending on the
memory backend this is either a no-op or an unbounded result materialization.
On a unix socket reachable only by capability-checked agent subprocesses, so
exposure is small.

**Fix.** Clamp `limit` to `[1, 200]` and ignore parse errors explicitly.

---

## Cleared / non-issues (verified, documented for completeness)

- **CORS — clean.** `grep` for `Access-Control` returns **zero** runtime
  settings. The agent-gateway SSE handler explicitly *omits* a wildcard ACAO and
  documents why (`agentgw/handlers.go:51-53`). No surface reflects `Origin` and
  none combines `*` with credentials. A malicious website therefore **cannot
  read** console/API responses cross-origin (the SOP blocks it; no CORS opt-out
  exists). Good.

- **Clickjacking — covered.** Every webui response sets `X-Frame-Options: DENY`
  and CSP `frame-ancestors 'none'` (`webui.go:1028,1039`), applied before the
  auth check so even 401s carry them. Console cannot be framed.

- **CSP — strong.** `default-src 'none'; script-src 'self'; connect-src 'self';
  base-uri 'none'; form-action 'none'; frame-ancestors 'none'` (webui.go:1030-1039).
  No inline script; tight exfil/pivot closure.

- **WebSocket — no inbound surface.** `coder/websocket` is used **only** as an
  outbound client to Nostr relays (`plugins/channels/nostr/nostr.go:35`). There
  is no WS upgrade endpoint, so no upgrade-origin / WS-auth / WS-message-size
  attack surface exists in the daemon. (If a WS server is ever added, it must do
  an explicit origin check — `coder/websocket`'s `Accept` defaults to
  same-origin, but that relies on the `Host`/`Origin` pair being trustworthy,
  which ties back to Finding 1.)

- **Login brute-force — lockout confirmed.** `session.go:36-40,113-121,188-191`:
  8 consecutive bad passwords → 5-minute cooldown (429), reset on success;
  constant-time compare (`subtle.ConstantTimeCompare`, session.go:200); 4 KiB
  login body cap. As recon noted.

- **Request body limits — comprehensive.** `http.MaxBytesReader` on openaiapi
  (16 MiB, openaiapi.go:45-51), audio upload (25 MiB, :200), restapi
  (`restapi.go:388`, `mailbox.go:134/263`, `update_handlers.go:88` 64 KiB),
  agentgw (`maxBodyBytes` on every decode, handlers.go:105/146/295), webui
  jsonProxy (`jsonBodyMax`, webui.go:1293), webhook (`webhookBodyCap` via
  `LimitReader`, webui.go:692-693), login (4 KiB). Control plane bounds its
  line-protocol at 16 MiB **pre-auth** (`server.go:343-368`,
  `readBoundedLine`). No unbounded `json.Decode` / `io.ReadAll` on a network
  body was found.

- **Slow-loris — guarded.** `newGuardedHTTPServer` sets `ReadHeaderTimeout` +
  `IdleTimeout` on every HTTP surface; `WriteTimeout` intentionally unset for SSE
  (`cmd/agezt/main.go:4207-4213`).

- **OpenAI-compat key confusion — none.** The OpenAI surface does **not** proxy
  with the caller's keys. The caller's `model` field is echoed but routing is the
  Governor's job using the **server's** configured providers/keys
  (`openaiapi.go:11-16,514-517`); every request runs the full kernel tool-loop
  (Edict/journal/budget), not a raw provider passthrough. No key confusion.

- **Webhook / OAuth-callback token-free routes — gated.** `/hooks/<name>` is
  POST-only, body-capped, and secret-gated with a constant-time compare
  (`workflow.go:232`), with uniform 403s that don't leak why
  (webui.go:678-731). `/oauth/callback` requires a `state` value validated by
  the control plane (webui.go:623-643).

- **Control-plane & gateway not browser-reachable.** Control plane is a
  loopback line-protocol on a random port (not HTTP, no CORS/CSRF surface);
  agentgw is a **unix socket** (TCP only as an explicit fallback). Neither is a
  cross-origin browser target under default config.

---

## Priority

1. **Finding 1 (MEDIUM/HIGH)** — add a `Host` allowlist (and ideally
   `Origin`/`Sec-Fetch-Site` check) to the console to close DNS-rebinding CSRF in
   password-session mode; consider defaulting to STRICT when a password is set.
   This is the only finding with a concrete cross-origin write path.
2. **Finding 2 (LOW–MED)** — narrow `?token=` to SSE routes; add
   `Referrer-Policy` to REST/OpenAI surfaces.
3. **Findings 3 & 4 (LOW)** — SSE per-credential cap; clamp memory-search limit.

Report written to: `D:/Codebox/PROJECTS/AGEZT/security-report/api-client-results.md`
