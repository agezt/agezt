# Security HUNTER Results — Auth / AuthZ / Session / JWT

**Domain:** Authentication, Authorization (IDOR/privilege escalation), Session management, JWT/token flaws
**Scope:** `kernel/agentgw/`, `kernel/controlplane/`, `kernel/webui/`, `kernel/restapi/`, `kernel/openaiapi/`, `kernel/tenant/`, `kernel/tenantctx/`
**Codebase:** D:/Codebox/PROJECTS/AGEZT (Go)

## Executive summary

The vulnerabilities are **almost entirely concentrated in `kernel/agentgw/`** (the Agent-SDK gateway, recovered in M939). That package's token layer is broken in several mutually-reinforcing ways: a hardcoded HMAC secret, an unauthenticated token-minting endpoint, no capability-subset enforcement, an alg/typ field that is never validated, and a rate limiter that fails open. By contrast, the other authenticated surfaces — `controlplane`, `webui`, `restapi`, `openaiapi`, `tenant` — are well-engineered: constant-time token comparison, fail-closed empty-token handling, 32-byte CSPRNG credentials, HttpOnly/SameSite/Secure session cookies, login lockout, tenant path-segment isolation, and per-tenant header-pinned authorization. Only minor / defense-in-depth notes apply there.

**Findings: 1 Critical, 4 High, 3 Medium, 2 Low, 1 Info.**

---

## CRITICAL

### AUTH-001 — Unauthenticated token-mint endpoint grants arbitrary capabilities (capability/privilege escalation)
- **Severity:** Critical
- **CWE:** CWE-862 (Missing Authorization) / CWE-269 (Improper Privilege Management)
- **File:** `kernel/agentgw/gateway.go:117` (route), `gateway.go:276-322` (`handleTokenCreate`)
- **Description:** `POST /v1/token/create` is registered **without** the `g.withAuth(...)` wrapper that guards every other route. The handler reads an attacker-controlled `caps []string` from the request body, runs it through `NormalizeCaps` (which only validates each string is a *known* capability, not that the caller is entitled to it), and mints a fully-valid signed token with `ExpiresAt`, `MaxRate`, `MaxBurst` and `RunID` all taken from the request. Any client that can reach the gateway can therefore mint a token bearing **every capability in the system** (`memory.write`, `memory.delete`, `config.access`, `agent.query`, `db.write`, ...) and then use it against the authenticated routes.
- **Exploit path:** `curl -X POST http://<gw>/v1/token/create -d '{"run_id":"x","caps":["memory.delete","config.access","agent.query","eventbus.publish"],"expiry_ms":86400000}'` → returns a valid token → replay as `Authorization: Bearer <token>` against `/v1/memory/delete`, `/v1/config/<secret-key>`, etc. Combined with AUTH-002 (TCP listen on Windows) this is remotely exploitable. There is **no audit record** (see AUTH-005), so the mint+use is invisible.
- **Remediation:** Wrap the route in `g.withAuth` and require an operator/admin capability to mint. Crucially, the minted caps must be a **subset of the calling token's caps** (and expiry/rate must not exceed the parent's). The intra-process token issuance that the SDK actually needs should happen in-process at subprocess spawn (the parent already holds the secret), not over an HTTP endpoint reachable by the same untrusted subprocess code the gateway is meant to sandbox.

---

## HIGH

### AUTH-002 — Hardcoded HMAC signing secret with no env/vault override (token forgery)
- **Severity:** High (Critical when the gateway listens on TCP)
- **CWE:** CWE-798 (Use of Hard-coded Credentials) / CWE-321 (Hard-coded Cryptographic Key)
- **File:** `kernel/agentgw/token.go:25` (`DefaultTokenSecret = "change-me-in-production"`), `gateway.go:63` (`DefaultGatewayConfig`), `kernel/runtime/runtime.go:743` (`gwCfg := agentgw.DefaultGatewayConfig(...)`), `cmd/agt/token.go:223-226` (`getTokenSecret()`)
- **Description:** The HMAC-SHA256 token signing secret is the public, open-source constant `"change-me-in-production"`. `runtime.go:743` wires the gateway via `DefaultGatewayConfig` and **only** overrides `SocketPath` (from `AGEZT_AGENTGW_SOCKET`, runtime.go:745-746) — `TokenSecret` is never overridden from env or the encrypted vault. `cmd/agt/token.go:getTokenSecret()` hardcodes the same constant. Because `NewTokenManager` (token.go:33-40) SHA-256-stretches the short secret, the *effective* key is still fully determined by the public constant. Anyone with the source can compute the signing key and forge tokens with arbitrary `caps`/`RunID`/`exp` offline — no endpoint access required.
- **Exploit path:** Reconstruct key = `sha256("change-me-in-production")`, sign `{"run_id":"*","caps":["config.access",...],"exp":<far future>}` → present to any authed gateway route. Forged tokens are indistinguishable from legitimate ones.
- **Remediation:** Generate a random ≥32-byte secret at first boot, persist it in the existing machine-bound encrypted vault (the same store that holds provider keys, per the M934 vault arc), and load it in `runtime.go` / `cmd/agt`. Refuse to start the gateway (or log a loud warning and bind loopback-only) if the secret is still the default. Have `agt token` read the live secret from the vault rather than the constant.

### AUTH-003 — No capability-subset enforcement on subprocess tokens (privilege escalation)
- **Severity:** High
- **CWE:** CWE-269 (Improper Privilege Management) / CWE-863 (Incorrect Authorization)
- **File:** `kernel/agentgw/token.go:127-143` (`CreateSubprocessToken`), `capabilities.go:128-146` (`NormalizeCaps`)
- **Description:** The package documents subprocess tokens as "inherits capabilities from the parent but can have a subset" (token.go:125-126), but `CreateSubprocessToken` copies the caller-supplied `caps []string` verbatim into the child claims with **no check that they are a subset of `parent.Caps`**. The only validation anywhere (`NormalizeCaps`) checks that each cap string is a recognized capability, never that it was granted to the issuer. So a sandboxed subprocess holding a narrow token (e.g. `memory.read`) can mint a child token with `memory.delete`+`config.access`+`db.write` and escalate. This is the library-level twin of AUTH-001.
- **Exploit path:** Hostile/compromised agent code calls the subprocess-token path (or AUTH-001's HTTP endpoint) with a superset of its own caps; the resulting token passes `withAuth` + `capCheck.Check` on the privileged routes.
- **Remediation:** In `CreateSubprocessToken`, intersect requested caps with `parent.Caps` and reject (or silently drop) anything not held by the parent. Also clamp child `ExpiresAt ≤ parent.ExpiresAt` and `MaxRate ≤ parent.MaxRate`. Enforce the same subset rule wherever a token is minted from a request (AUTH-001).

### AUTH-004 — JWT `alg`/`typ` header never validated; non-JOSE format invites alg-confusion regressions
- **Severity:** High
- **CWE:** CWE-345 (Insufficient Verification of Data Authenticity) / CWE-347 (Improper Verification of Cryptographic Signature)
- **File:** `kernel/agentgw/token.go:82-116` (`ValidateToken`), `token.go:64` (header is hardcoded but ignored on verify)
- **Description:** `ValidateToken` splits the token into 3 parts, recomputes the HMAC over `parts[0]+"."+parts[1]`, and compares. It **never decodes or inspects the header (`parts[0]`)** — the `alg`/`typ` are write-only. Today verification is hardwired to HMAC-SHA256 so `alg:none` is not *directly* exploitable (a `none` token would still need a valid HMAC). However: (a) the implementation is a hand-rolled pseudo-JWT that *advertises* `"alg":"HS256"` while never honoring the field, which is exactly the trap that turns into a full alg-confusion / `alg:none` bypass the moment anyone adds RS256 support or swaps in a real JWT library that trusts the header; (b) any token whose payload base64-decodes to valid claims and whose 3rd segment HMACs correctly is accepted regardless of header content, so the header provides zero binding. This is a latent CWE-347 and a hardening defect.
- **Remediation:** Either (1) decode `parts[0]`, assert `alg=="HS256"` and `typ=="JWT"`, and reject anything else; or (2) drop the JWT cosplay entirely and use an opaque signed blob (the claims are internal-only — there is no third-party JOSE consumer). Never let the header pick the algorithm.

### AUTH-005 — Gateway has no audit trail (audit logger constructed with nil journal, never wired, never called)
- **Severity:** High
- **CWE:** CWE-778 (Insufficient Logging)
- **File:** `kernel/agentgw/gateway.go:73` (`auditLog: NewAuditLogger(nil)`), `audit.go:71` (no-ops when `a.j == nil`); no caller of `.Record()` exists in any handler
- **Description:** The gateway builds its audit logger with a `nil` journal and the comment "Will be set when attached to kernel" — but neither `Attach` (gateway.go:81-85) nor `SetConfigCenter` (gateway.go:88-91) nor `runtime.go` ever assigns a real journal, and **no handler ever calls `auditLog.Record`**. `AuditLogger.flush` silently returns when `a.j == nil` (audit.go:71). Net effect: every capability access through the gateway — token mints (AUTH-001), memory deletes, config reads of secrets — is completely unlogged. This both removes detection of the other findings' exploitation and breaks the security posture the package's own doc-comment claims ("validated by JWT capability tokens" with auditing).
- **Remediation:** Wire the kernel journal into the gateway (e.g. a `SetJournal`/extend `Attach`), and call `auditLog.Record` (with `Success`, `Capability`, `Operation`, `RunID`, `ClientIP`) in `withAuth` and each handler — at minimum on `/v1/token/create` and all mutating/config routes.

---

## MEDIUM

### AUTH-006 — Rate limiter fails open across window boundary (auth-bypass-adjacent DoS / brute-force enabler)
- **Severity:** Medium
- **CWE:** CWE-307 (Improper Restriction of Excessive Authentication Attempts) / CWE-770 (Allocation Without Limits)
- **File:** `kernel/agentgw/types.go:155-172` (`RateLimit.Allow`)
- **Description:** `Allow()` returns `true` unconditionally whenever `now - lastTick >= windowMs` (types.go:159-162) and **never resets the counter or updates `lastTick`** in that branch — the comment even admits "Can't use atomic swap for int64 easily, so use sync" but no sync is done. Once a full 60s window elapses, every subsequent call hits the early `return true` (because `lastTick` is frozen at construction time and never advances), so the limiter is permanently bypassed after the first minute. The per-token `maxRate+maxBurst` cap (intended to bound abuse of a leaked/forged token) is therefore ineffective in steady state.
- **Remediation:** Implement a correct windowed/token-bucket limiter under the mutex (or `golang.org/x/time/rate`): on window roll-over, atomically reset `count=0` and set `lastTick=now`, then count the current request.

### AUTH-007 — Config write gated only by read capability (`config.access`), no write capability (privilege escalation within config)
- **Severity:** Medium
- **CWE:** CWE-863 (Incorrect Authorization)
- **File:** `kernel/agentgw/config_handler.go:177-230` (`handleConfigSet`), lines 184-189
- **Description:** `POST /v1/config` (set a config value, bump rating, change description/tags) is gated by `capCheck.Check(claims, CapConfigAccess)` — the **read** capability — with the in-code admission "This should be restricted to operator/admin capabilities ... in production, add config.write capability" (config_handler.go:184-185). The same applies to `/v1/config/audit` (line 241). So any token granted read access to config can also **write** config and read the audit log. There is no `config.write`/`config.admin` capability in the `AgentCapability` set (types.go:100-104) to gate it. Compounded by AUTH-001/AUTH-002 (a forged/minted token trivially carries `config.access`), an attacker can rewrite config entries (e.g. flip a value's rating to lower its access bar, or poison a key an agent trusts).
- **Remediation:** Add `CapConfigWrite` (and optionally `CapConfigAudit`) capabilities and gate `handleConfigSet`/`handleConfigAudit` on them; do not accept the read cap as authorization for writes.

### AUTH-008 — Gateway can bind a TCP socket, exposing all of the above remotely
- **Severity:** Medium (amplifier for AUTH-001/002/003)
- **CWE:** CWE-668 (Exposure of Resource to Wrong Sphere)
- **File:** `kernel/agentgw/gateway.go:138-158` (listen dispatch), `runtime.go:745-746` (`AGEZT_AGENTGW_SOCKET` override)
- **Description:** `Listen` falls through to `net.Listen("tcp", g.sockPath)` for any `sockPath` that isn't an `@abstract`, `unix://`, or `/absolute` path (gateway.go:155-157). On Windows, abstract unix sockets don't work, so operators are pushed toward `AGEZT_AGENTGW_SOCKET=tcp://host:port` (the override comment at runtime.go:744 literally says "useful for Windows TCP testing"). A TCP-bound gateway with the default `0.0.0.0`-style host turns the unauthenticated mint endpoint (AUTH-001) and forgeable tokens (AUTH-002) into network-reachable, pre-auth-equivalent compromises. There is no loopback enforcement on the TCP path.
- **Remediation:** If TCP is required, force-bind loopback (reject non-`127.0.0.1`/`::1` hosts unless an explicit "I-know-what-I'm-doing" flag is set), and require the hardened secret (AUTH-002) + authed mint (AUTH-001) before any TCP bind is allowed.

---

## LOW

### AUTH-009 — Subprocess token loses subprocess identity in audit fields; `ParentTokenID` is a placeholder
- **Severity:** Low
- **CWE:** CWE-778 (Insufficient Logging) / traceability
- **File:** `kernel/agentgw/token.go:138` (`ParentTokenID: parent.RunID, // TODO: store actual parent tid`)
- **Description:** `CreateSubprocessToken` stores `parent.RunID` in `ParentTokenID` (its own TODO flags this), so child→parent token lineage cannot be reconstructed. Minor on its own, but it undermines any future audit/forensics once AUTH-005 is fixed.
- **Remediation:** Mint and carry a real per-token `tid` (a `ulid.New()` is already generated and discarded at token.go:61) and record the actual parent `tid` in the child.

### AUTH-010 — SSE handler sets `Access-Control-Allow-Origin: *` on a credentialed (Bearer-authed) stream
- **Severity:** Low
- **CWE:** CWE-942 (Permissive Cross-domain Policy)
- **File:** `kernel/agentgw/handlers.go:51` (`w.Header().Set("Access-Control-Allow-Origin", "*")`)
- **Description:** `handleEventbusSubscribe` (a `withAuth`-gated, token-authed event stream) emits `Access-Control-Allow-Origin: *`. Because the auth is a Bearer header (not a cookie) and CORS with `*` cannot be combined with credentials, this is not a direct browser credential-theft vector today, but it is an unnecessary permissive policy on an authenticated control-plane stream and should not be `*`. Note the WebUI surface, by contrast, has a strict CSP and no wildcard CORS (webui.go:747-762) — the gateway is the outlier.
- **Remediation:** Drop the wildcard ACAO header (the SDK is not a browser) or reflect a strict allow-list.

---

## INFO / Well-built (verified, no action — distinguishing real issues from defense-in-depth)

### AUTH-011 — Surrounding authenticated surfaces are sound (positive confirmation)
- **Severity:** Info
- **Verified safe:**
  - **Control plane** (`controlplane/server.go:309-319`): primary-token check uses `subtle.ConstantTimeCompare`, empty presented/server token never authorizes, per-tenant fallback via `tenant.Registry.Authorize`, tenant-token command allow-list (`tenantTokenAllows`) with tenant-arg pinning, and a bounded pre-auth read to stop pre-auth OOM (M188). 32-byte hex admin token minted at start, written `0600`.
  - **WebUI** (`webui/webui.go:786-803`, `webui/session.go`): token compared in constant time; data routes fail closed when token unset; sessions are 32-byte CSPRNG ids, in-memory, sliding-expiry; login is constant-time with an 8-fail / 5-min lockout; session cookie is `HttpOnly` + `SameSite=Strict` + `Secure` (when TLS) + `MaxAge`; logout revokes server-side and clears the cookie; strict CSP + `X-Frame-Options: DENY` + `Referrer-Policy: no-referrer` (token kept out of Referer). No session-fixation (id only minted on successful login, no pre-auth session). These are **not** findings.
  - **REST API / OpenAI API** (`restapi/restapi.go:272-298`, `openaiapi/openaiapi.go:272-289`): constant-time token compare, empty token fails closed, per-tenant token authorizes **only** its own tenant and **only** when the request targets that tenant via `X-Agezt-Tenant` (header-pinned — no cross-tenant IDOR), admin token authorizes any tenant. Unauthenticated routes limited to `/healthz`/`/readyz` (no spend/state); `/metrics` is authed.
  - **Tenant isolation** (`tenant/tenant.go`): tenant ids are single safe path segments (regex + post-join containment check at `baseDir`), so no path traversal / sibling collision; per-tenant 32-byte tokens minted race-safely (`O_CREATE|O_EXCL`), `0600`, compared in constant time via `Authorize`.
- **No IDOR found** in the reviewed surfaces: run/agent/config/mailbox object access is scoped by the authenticated principal's tenant (header-pinned) rather than by a client-supplied owner id with no check. The agentgw `agent.query`/`memory` handlers key off `claims.RunID`/roster lookups, not attacker-chosen owner fields — the agentgw risk is the *token issuance* layer (AUTH-001/003), not per-object ownership.

### False-positive candidates explicitly excluded
- **Default-allow capability posture** (owner law per project memory `default-allow-posture`): the fact that every *kernel* capability defaults to `LevelAllow` is **by design** and is NOT reported. AUTH-001/003/007 are reported anyway because they are about *who can mint/wield a capability token and whether caps are bounded by the issuer's own grant* — an authorization-integrity problem, not a default-allow policy choice.
- **`change-me-in-production` in test files** / worktree copies (`.claude/worktrees/...`) are not counted; AUTH-002 is reported against the live `kernel/` + `cmd/agt/` paths only.
- **`fmt.Sscanf` for limit parsing** (config_handler.go:258, handlers.go:200) — not a security issue (bounded int parse), excluded.
