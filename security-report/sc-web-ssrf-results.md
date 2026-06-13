# Security Hunt — SSRF / CORS / CSRF / Redirect / WebSocket / Header-Injection / Rate-Limiting

Scanner domain: SSRF, CORS, CSRF, Open Redirect, Clickjacking, WebSocket, HTTP header injection, Webhook/inbound-channel security, Rate-limiting/DoS.
Codebase: AGEZT (Go), D:/Codebox/PROJECTS/AGEZT. Scan date: 2026-06-13.

## Executive summary

The mature core of AGEZT is **well-defended** in this domain: `kernel/netguard` is a dialer-level SSRF guard (validates the *resolved* IP on the initial dial and every redirect hop, collapses NAT64/IPv4-compat IPv6 embeddings, covers IMDS 169.254.169.254, loopback, RFC1918+ULA, CGNAT, 0.0.0.0/8, broadcast/multicast). The `http`, `fetch`, `web_search`, and `browser` tools and the outbound `webhook` dispatcher all route through it. The Web UI (`kernel/webui`) is hardened: constant-time token, strict CSP with `frame-ancestors 'none'`, `X-Frame-Options: DENY`, no CORS headers, POST-only writes with fixed arg allowlists, `Cache-Control: no-store`, SSE-only (no WebSocket). The REST API and OpenAI-compatible API are Bearer/`?token=` authed with no cookies (CSRF-immune) and no CORS. The inbound webhook channel fails closed on HMAC, has a replay window + dedup + body cap + slow-loris timeouts.

The findings concentrate in the **recently-recovered Agent Gateway** (`kernel/agentgw`, landed M939) and in two **outbound-fetch paths that bypass netguard** (MCP remote HTTP, catalog sync/discovery). The Agent Gateway ships a hardcoded HMAC token secret, an unauthenticated token-mint endpoint, a broken rate limiter that stops enforcing after one idle window, an unbounded per-token rate-limit map, and a wildcard CORS header on its SSE stream.

### Counts by severity
- Critical: 2
- High: 3
- Medium: 4
- Low: 2
- Informational / false-positive notes: 3

---

## CRITICAL

### GW-001 — Hardcoded HMAC token-signing secret in the Agent Gateway (production default)
- **Severity:** Critical
- **Confidence:** 95
- **CWE:** CWE-798 (Use of Hard-coded Credentials) / CWE-321 (Hard-coded Cryptographic Key)
- **File:** `kernel/agentgw/token.go:25`, `kernel/agentgw/gateway.go:63`, `kernel/runtime/runtime.go:743`, `cmd/agt/token.go:223-226`
- **Description:** `DefaultTokenSecret = "change-me-in-production"` is the HMAC-SHA256 key used to sign and validate every Agent Gateway capability token. `DefaultGatewayConfig` hardcodes the same string as `TokenSecret`, and the daemon wires the gateway *exclusively* through `DefaultGatewayConfig` (`runtime.go:743`) with no override from the vault, config store, or a random per-boot value — only the **socket path** is overridable (`AGEZT_AGENTGW_SOCKET`), never the secret. The CLI (`cmd/agt/token.go:getTokenSecret`) returns the same constant. `NewTokenManager` SHA-256-stretches short secrets, but the input is a public constant, so the effective key is the publicly-known `sha256("change-me-in-production")`.
- **Exploit path:** Anyone who knows the (open-source) constant can forge a valid token with `caps = AllAgentCaps()` and an arbitrary `run_id`, then call every gateway endpoint: read/write/delete kernel **memory**, publish arbitrary **events** onto the bus (driving downstream automation), read **config**, and enumerate **agents**. Reachable wherever the gateway socket is reachable — and trivially remote if an operator sets `AGEZT_AGENTGW_SOCKET=tcp://0.0.0.0:PORT` (a documented mode, gateway.go:95/141-157). Contrast `kernel/controlplane/server.go:240-249`, which binds loopback and generates a fresh random 32-byte token per start — the gateway should do the same.
- **Remediation:** Generate a random secret at daemon start (or pull from the machine-bound vault), share it with the subprocess out-of-band (it is already spawned by the daemon), and **refuse to start** with the literal `"change-me-in-production"`. Remove the hardcoded default from both kernel and `agt`.

### GW-002 — Unauthenticated token-creation endpoint (privilege escalation / cap minting)
- **Severity:** Critical
- **Confidence:** 90
- **CWE:** CWE-306 (Missing Authentication for Critical Function) / CWE-269 (Improper Privilege Management)
- **File:** `kernel/agentgw/gateway.go:117`, `kernel/agentgw/gateway.go:276-322`
- **Description:** Every gateway route is wrapped in `g.withAuth(...)` **except** `POST /v1/token/create`, which is registered raw (`gateway.go:117`). `handleTokenCreate` accepts a JSON body naming `run_id`, `caps`, `max_rpm`, `max_burst`, `expiry_ms` and returns a freshly minted, fully-valid token. There is no caller authentication and no check that the requested caps are a subset of anything — `NormalizeCaps` only validates that the cap *names* are well-formed.
- **Exploit path:** Any party that can reach the gateway socket POSTs `{"run_id":"x","caps":["memory.write","memory.delete","eventbus.publish","config.access", ...]}` and receives a token granting those capabilities, with no credential required. Combined with GW-001 it is redundant (the secret alone suffices), but it is independently exploitable even if GW-001 were fixed: the mint endpoint hands out maximal-cap tokens to anyone. A subprocess token is meant to be *minted by the daemon and scoped down*; here the scope is fully attacker-chosen.
- **Remediation:** Require authentication on `/v1/token/create` (the daemon's own admin credential), and enforce that requested caps are a subset of the caller's. Subprocess tokens should be derived via `CreateSubprocessToken` from an authenticated parent, never freely requested.

---

## HIGH

### GW-003 — Rate limiter stops enforcing after one idle window (DoS / brute-force bypass)
- **Severity:** High
- **Confidence:** 90
- **CWE:** CWE-799 (Improper Control of Interaction Frequency) / CWE-770
- **File:** `kernel/agentgw/types.go:155-172`
- **Description:** `RateLimit.Allow()` is broken. On window rollover it does:
  ```go
  if now-r.lastTick >= r.windowMs {
      return true   // never resets lastTick or the counter
  }
  ```
  `lastTick` is set once in `NewRateLimit` and **never updated**; the counter (`r.mu`) is **never reset**. So for the first 60 s the counter monotonically climbs to `max+burst` and then blocks — but as soon as 60 s have elapsed since construction, `now-r.lastTick >= windowMs` is permanently true and **every** call returns `true`, disabling the limit forever. The atomic counter is also read-then-incremented non-atomically (a check-then-act race that lets bursts slip past `max+burst` under concurrency).
- **Exploit path:** A token holder waits 60 s, then issues unlimited requests — the per-token throttle the gateway advertises (`MaxRate`/`MaxBurst`) provides no protection against flooding memory writes / event publishes / bus subscriptions after the first window.
- **Remediation:** Implement a real sliding/token-bucket window: on rollover, `atomic.Store(&r.mu, 0)` and update `lastTick` under a lock or with a CAS loop; or replace with `golang.org/x/time/rate.Limiter`. Make the check-and-increment atomic.

### GW-004 — Unbounded rate-limit map keyed by caller-controlled SubprocessID (memory exhaustion)
- **Severity:** High
- **Confidence:** 80
- **CWE:** CWE-770 (Allocation of Resources Without Limits or Throttling)
- **File:** `kernel/agentgw/gateway.go:25,220-229`
- **Description:** `g.rateLimit` is a `map[string]*RateLimit` keyed by `claims.SubprocessID`, populated in `allowRate` and **never evicted or capped**. `SubprocessID` is a free-form field inside the token claims (`types.go:38`), and tokens are mintable with arbitrary claims (GW-002) under a known secret (GW-001). An attacker mints tokens (or a single token and varies `sub_id`) with unique `SubprocessID` values; each first request allocates a new `*RateLimit` entry that lives for the process lifetime.
- **Exploit path:** Loop minting tokens with `sub_id = random()` and firing one request each → the map grows without bound → daemon OOM. Note also that an empty `SubprocessID` (the common case for a top-level run token) makes *all* such callers share one bucket — a correctness side-issue.
- **Remediation:** Bound the map (LRU with a cap), evict entries idle past their window, and tie the rate-limit identity to an authenticated, server-assigned id rather than a client-supplied claim.

### CORS-001 — Wildcard `Access-Control-Allow-Origin: *` on the gateway event-bus SSE stream
- **Severity:** High
- **Confidence:** 75
- **CWE:** CWE-942 (Permissive Cross-domain Policy)
- **File:** `kernel/agentgw/handlers.go:51`
- **Description:** `handleEventbusSubscribe` sets `Access-Control-Allow-Origin: *` on the SSE response — the only CORS header in the entire codebase. The stream carries the **full kernel event firehose** (subject pattern defaults to `>`, handlers.go:29-32), which includes memory writes, run details, tool I/O, and config-change events. Authentication is a Bearer token in the `Authorization` header, so a browser `EventSource` cannot attach it cross-origin (EventSource sends no custom headers, and `*` forbids credentials) — which limits *direct* browser exploitation. But the wildcard signals intent to permit cross-origin reads and would become directly exploitable if any query-param/cookie auth path is ever added to this endpoint, or if a token ever rides the URL. The other SSE surfaces (`webui`, `restapi/mailbox`, `openaiapi`) deliberately set **no** ACAO header; this one is the outlier.
- **Exploit path:** As written, an unauthenticated cross-origin page cannot read the stream (no token reaches the handler). The risk is the permissive policy combined with this being a sensitive firehose; it is a latent cross-origin data-exposure primitive and a clear deviation from the project's own SSE hardening elsewhere.
- **Remediation:** Remove the `Access-Control-Allow-Origin: *` line. The gateway is a same-origin/local agent IPC surface; it needs no cross-origin read grant. If cross-origin is ever required, reflect a validated allowlisted origin, never `*`, and never with credentials.

---

## MEDIUM

### SSRF-001 — MCP remote HTTP transport bypasses netguard (operator/config-supplied URL)
- **Severity:** Medium
- **Confidence:** 80
- **CWE:** CWE-918 (Server-Side Request Forgery)
- **File:** `kernel/mcp/http.go:64-80` (`DialHTTP`), `kernel/runtime/mcptool.go:110-117` (`dialMCP`)
- **Description:** `DialHTTP` builds a plain `&http.Client{Timeout: callTimeout}` with **no netguard dialer**. The endpoint is taken verbatim from a registered MCP server's `URL` field and POSTed to (`postLocked`), with the operator's opt-in headers attached. A server is registrable via the `/api/mcp/add` write route and the MCP catalog. Unlike the `http`/`fetch`/`web_search`/`browser` tools — which all wrap `netguard.New(...).HTTPClient(...)` — this remote dialer connects to whatever IP the endpoint resolves to, including `169.254.169.254`, `127.0.0.1`, and RFC1918 hosts, and follows redirects with Go's default (unguarded) policy.
- **Exploit path:** A registered remote MCP server URL of `http://169.254.169.254/latest/meta-data/...` or `http://127.0.0.1:<admin-port>/` causes the daemon to issue server-side requests to internal/metadata endpoints during attach/handshake/tool-call. Registration is operator/UI-gated (mitigating to Medium under the default-allow posture), but a prompt-injected agent that can drive `/api/mcp/add`, or a malicious catalog entry, turns this into an internal-network/IMDS reach. The handshake JSON-RPC responses also flow back into model context, enabling blind-to-semi-blind SSRF.
- **Remediation:** Route `DialHTTP`'s client through `netguard` (a guarded transport with `CheckRedirect` re-validation), matching the in-process tools. Relax only via the same explicit AllowLoopback/AllowPrivate opt-ins the tools use.

### SSRF-002 — Catalog sync fetch bypasses netguard (config-overridable URL)
- **Severity:** Medium
- **Confidence:** 70
- **CWE:** CWE-918
- **File:** `kernel/catalog/sync.go:35-41,69-83`
- **Description:** `NewSyncer` uses a bare `&http.Client{Timeout: DefaultSyncTimeout}` (no netguard) and GETs `s.URL`. The URL defaults to `https://models.dev/api.json` but is overridable by `AGEZT_CATALOG_URL` and by the `url` argument of the `/api/catalog/sync` write/json route (webui.go:359, forwarded to `CmdCatalogSync`). The body is size-capped (8 MiB) and parsed as JSON, so direct exfiltration is limited, but the daemon will still connect to an arbitrary internal/metadata IP and follow redirects unguarded.
- **Exploit path:** An operator (or UI-driving agent) sets the sync URL to `http://169.254.169.254/...` or an internal host; the daemon issues the request server-side. Lower severity because it is operator-triggered config and the response must parse as a models catalog, but it is still an unguarded server-side fetch to a user-supplied URL.
- **Remediation:** Use a netguard-protected client for the syncer (default-deny internal/metadata), consistent with the rest of the outbound surface.

### SSRF-003 — Ollama discovery uses http.DefaultClient against a config-supplied endpoint
- **Severity:** Medium
- **Confidence:** 65
- **CWE:** CWE-918
- **File:** `kernel/catalog/discovery.go:28-44`
- **Description:** `DiscoverOllama` calls `http.DefaultClient.Do` against `endpoint + "/api/tags"`, where `endpoint` defaults to `http://localhost:11434` but is overridable via `AGEZT_OLLAMA_ENDPOINT`. No netguard. Default is loopback (legitimately — Ollama is a local sidecar), so the loopback reach is intended; the SSRF surface is the *override*, which can point the daemon at an arbitrary internal/metadata host. The parsed response only populates a model catalog (limited reflection).
- **Exploit path:** Operator/config sets `AGEZT_OLLAMA_ENDPOINT=http://169.254.169.254/` (or an internal host); the daemon fetches it server-side at discovery time. Low practical impact (loopback is the expected target and the response is constrained), but it is an unguarded config-driven fetch.
- **Remediation:** Either pin discovery to loopback only, or run it through a netguard client built with `AllowLoopback()` so the intended local reach works while internal/metadata pivots via an overridden endpoint are blocked.

### DOS-001 — Gateway POST handlers decode request bodies with no size limit
- **Severity:** Medium
- **Confidence:** 70
- **CWE:** CWE-770 (Allocation Without Limits)
- **File:** `kernel/agentgw/handlers.go:103,144,290` (`json.NewDecoder(r.Body).Decode(...)`), `kernel/agentgw/gateway.go:290`
- **Description:** The gateway's `eventbus/publish`, `memory/write`, `log/write`, and `token/create` handlers decode `r.Body` directly with `json.NewDecoder(...).Decode(...)` and no `http.MaxBytesReader` wrap. The server sets `MaxHeaderBytes: 1<<20` but **no body cap**. Contrast the Web UI, which wraps every JSON body in `http.MaxBytesReader(w, r.Body, jsonBodyMax)` (webui.go:1015) and the inbound channel webhook's `io.LimitReader(r.Body, maxBody)`. A large body is read fully into memory before/while decoding.
- **Exploit path:** A token holder (or anyone, via the unauthenticated `token/create`, GW-002) POSTs a multi-gigabyte JSON body to exhaust daemon memory. `WriteTimeout`/`ReadTimeout` of 30 s bound duration but not peak allocation.
- **Remediation:** Wrap each handler body in `http.MaxBytesReader` with a sane cap (the UI uses 1 MiB), matching the rest of the codebase.

---

## LOW

### HDR-001 — Operator/agent-supplied request headers passed unfiltered to outbound HTTP (informational)
- **Severity:** Low
- **Confidence:** 50
- **CWE:** CWE-113 (HTTP Response Splitting) — outbound-request variant
- **File:** `plugins/tools/http/http.go:204-206` (`req.Header.Set(k, v)` from model input), `kernel/mcp/http.go:248-250` (operator headers)
- **Description:** The `http` tool copies model-supplied `headers` into the outbound request, and the MCP HTTP client copies operator-supplied headers. Go's `net/http` rejects CR/LF in header values on write, so classic request-splitting is not achievable; this is noted for completeness, not as an exploitable splitting bug. No user input reaches *response* headers anywhere in the scanned surface (response headers are constants or framework-set), so server-side response splitting (CWE-113 inbound) is not present.
- **Remediation:** None required given Go's header sanitization; optionally validate header names against an allowlist for defense in depth.

### DOS-002 — websearch / fetch parse HTML with regexes (ReDoS-resistant but worth noting)
- **Severity:** Low
- **Confidence:** 40
- **CWE:** CWE-1333 (ReDoS)
- **File:** `plugins/tools/websearch/websearch.go:193-198`
- **Description:** The DuckDuckGo result parser uses `(?s)`-mode regexes (`reLink`, `reSnippet`). Go's `regexp` uses RE2 (linear-time, no catastrophic backtracking), so these are **not** ReDoS-vulnerable, and inputs are size-capped (1 MiB). Noted only because regex-over-network-HTML is a common ReDoS locus in other languages; here it is safe by construction.
- **Remediation:** None (RE2 guarantees linear time). Keep the 1 MiB cap.

---

## False positives / verified-safe (explicitly cleared)

1. **Web UI CSRF / clickjacking — SAFE.** `kernel/webui/webui.go:732-762` sets `X-Frame-Options: DENY`, `frame-ancestors 'none'`, `form-action 'none'`, `base-uri 'none'`, strict `script-src 'self'`. All mutating routes are POST-only with constant-time token auth and fixed query/body arg allowlists. No cookies are used for the data routes (token in `?token=`/Bearer), so cross-site form submission cannot forge an authenticated write. The optional console-password session cookie is a same-site session validated server-side; the token is still required in default (non-strict) mode the action surface stays loopback. No reflected-origin, no CORS.
2. **REST API / OpenAI-compatible API CORS+CSRF — SAFE.** `kernel/restapi/restapi.go` and `kernel/openaiapi/openaiapi.go` are Bearer/`?token=` authed, loopback-bound, no cookies, no `Access-Control-*` headers → CSRF-immune and not cross-origin readable. SSE surfaces (mailbox watch, chat stream) sit behind `s.auth`.
3. **Inbound webhook channel — SAFE.** `plugins/channels/webhook/webhook.go` fails closed on empty secret/sig (constant-time `hmac.Equal`), enforces a 5-minute freshness window, dedups replays across two generations, caps body at 1 MiB, and sets slow-loris timeouts. The Web UI workflow hook (`/hooks/`, webui.go:516-587) uses a per-workflow secret verified constant-time in the control plane, caps the body, and returns uniform refusals (no oracle). The **peer** tool (`plugins/tools/peer`) and outbound webhook only contact operator-pre-registered destinations (`AGEZT_PEERS` / `AGEZT_WEBHOOKS`) — not SSRF (no agent-controlled URL); outbound webhook is additionally netguard-wrapped by the daemon (`cmd/agezt/main.go:3708-3709`). **No WebSocket upgrade exists anywhere** (despite the agentgw package doc mentioning it) — all live streams are SSE, so there is no `CheckOrigin`/cross-site-WS-hijacking surface.

---

## Notes on the owner's default-allow posture

Per the documented posture, netguard/SSRF-guards, budgets, and HITL are intentional opt-OUT exceptions, and capabilities default to allow. The SSRF findings above are **not** about the agent being allowed to fetch URLs (that is by design) — they are about specific server-side fetch paths (MCP remote, catalog sync, Ollama discovery) that *bypass the netguard exception the rest of the fetch surface already applies*, so they cannot reach the daemon's own internal network / IMDS the way the in-process tools are stopped from doing. The Agent Gateway findings (GW-001..004, CORS-001, DOS-001) are conventional authentication/rate-limiting/DoS defects independent of the egress posture.
