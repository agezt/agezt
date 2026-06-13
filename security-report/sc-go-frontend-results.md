# Security Scan — Go (language/race/mass-assignment/business-logic) + Frontend (XSS/client-side)

Scanner domain: Go-specific issues, race conditions/TOCTOU, mass assignment, business logic, plus Frontend (TypeScript/React) XSS & client-side. Codebase: AGEZT @ D:/Codebox/PROJECTS/AGEZT.

## Severity summary

| Severity | Count |
|----------|-------|
| Critical | 2 |
| High     | 3 |
| Medium   | 4 |
| Low      | 5 |
| Info / by-design (noted, not counted) | several |

---

## CRITICAL

### GO-001 — Hardcoded default HMAC token secret used in production gateway wiring
- **CWE:** CWE-798 (Use of Hard-coded Credentials) / CWE-321 (Hard-coded Cryptographic Key)
- **File:** `kernel/agentgw/gateway.go:63` (`DefaultGatewayConfig` → `TokenSecret: []byte("change-me-in-production")`), `kernel/agentgw/token.go:25` (`DefaultTokenSecret`), wired at `kernel/runtime/runtime.go:743` (`gwCfg := agentgw.DefaultGatewayConfig(cfg.BaseDir)`), and `cmd/agt/token.go:223-226` (`getTokenSecret()` returns the same constant).
- **Description & impact:** The Agent Gateway signs and validates its capability tokens (HMAC-SHA256) with the constant `"change-me-in-production"`. The kernel boot path in `runtime.go` constructs the gateway from `DefaultGatewayConfig` and **never overrides `TokenSecret`** — there is no code path that derives it from the machine-bound vault, an env var, or a random per-boot value. `cmd/agt`'s `getTokenSecret()` returns the identical constant. Because the secret is public (in source), **any party who can reach the gateway socket can forge a valid token for any RunID with any capability set** (memory read/write/delete, eventbus publish/subscribe, config read, config write) and bypass `withAuth` entirely. The `NewTokenManager` SHA-256-expands the short secret, but the input is still a known constant, so the derived key is fully predictable.
- **Exploitability raised by GO-002 and the TCP bind path** (`runtime.go:745-746` honours `AGEZT_AGENTGW_SOCKET`, and the gateway `Listen` switch binds plain TCP for non-`@`/non-`unix`/non-`/` paths — the in-tree comment calls this "useful for Windows TCP testing"). On Windows the default abstract-unix socket `@agezt/agentgw.sock` does not work, nudging operators to a TCP bind, which makes forged tokens reachable over the network.
- **Remediation:** Generate a random 32-byte secret at first boot and persist it in the encrypted vault (the machine-bound vault already exists per project memory M934); load it into `GatewayConfig.TokenSecret`. Refuse to start (or log a loud warning + bind loopback only) if the secret equals the default constant. Have `cmd/agt` read the same persisted secret rather than the constant.

### GO-002 — Unauthenticated token-mint endpoint allows arbitrary capability escalation
- **CWE:** CWE-306 (Missing Authentication for Critical Function) / CWE-269 (Improper Privilege Management)
- **File:** `kernel/agentgw/gateway.go:117` — `mux.HandleFunc("POST /v1/token/create", g.handleTokenCreate)` (registered WITHOUT `g.withAuth`), handler at `gateway.go:276-322`.
- **Description & impact:** Every other gateway route is wrapped in `withAuth`, but `/v1/token/create` is mounted bare. The handler accepts an arbitrary `run_id`, an arbitrary capability list, and arbitrary rate/burst from the request body and returns a freshly-signed token (`gateway.go:307-321`). There is no caller authentication, no proof-of-parent-token, and no restriction that the requested caps be a subset of anything. Anyone who can POST to the socket gets a token granting any capability the gateway honours — full privilege escalation into kernel memory/config/eventbus. Combined with GO-001 an attacker does not even need this endpoint (they can sign their own), but this endpoint hands out valid tokens even to someone who does not know the secret.
- **Remediation:** Require authentication to mint tokens (e.g. an operator/admin token, or the same control-plane token), and enforce capability subsetting against the caller's grant. If the endpoint exists only to let the kernel mint subprocess tokens, it should not be exposed on the agent-facing mux at all — mint in-process and inject the token into the subprocess environment.

---

## HIGH

### GO-003 — Agent daily-budget check is a check-then-act race outside the lock (unintended over-spend)
- **CWE:** CWE-362 (Race Condition) / CWE-367 (TOCTOU)
- **File:** `kernel/governor/governor.go:540-561` (agent budget pre-check).
- **Description & impact:** The agent ceiling path acquires `g.mu` only to *read* `spentByAgentToday[agent]`, **releases the lock**, and then compares against the ceiling outside any lock; the matching deduction happens later in `recordUsage` under a separate lock acquisition. N concurrent `Complete()` calls for the same agent all read the same pre-spend total, all pass `spent < ceiling`, all run the provider, and together overshoot the agent ceiling by up to (N-1) calls' cost. Unlike the global/per-task budgets — which carry an explicit in-code comment declaring them deliberate *soft* caps — the agent ceiling reads as intended-hard enforcement, so this race is a genuine budget-enforcement defeat (spend governance / cost-abuse business-logic flaw).
- **Remediation:** Either fold the agent check into the same locked critical section that records the spend (reserve estimated cost under `g.mu`), or document it explicitly as a soft cap like the global path. Minimum fix: compute the `exceeded` boolean inside the lock and return on it, then keep the deduction under the same mutex.

### FE-001 — Agent SDK gateway binds TCP with a known-key token (network exposure of kernel internals)
- **CWE:** CWE-319/CWE-668 (Exposure of Resource to Wrong Sphere), compounding GO-001/GO-002
- **File:** `kernel/agentgw/gateway.go:138-158` (`Listen` socket-type switch) + `kernel/runtime/runtime.go:745-746` (`AGEZT_AGENTGW_SOCKET` override).
- **Description & impact:** When `AGEZT_AGENTGW_SOCKET` is a `host:port` (the documented Windows path), the gateway listens on TCP. There is no loopback enforcement in the agentgw `Listen` (unlike `cmd/agezt` which is careful never to bind `0.0.0.0` implicitly for web/api). With the hardcoded secret (GO-001) and the unauthenticated mint endpoint (GO-002), a TCP-bound gateway exposes kernel memory/config/eventbus to anyone who can reach the port.
- **Remediation:** If TCP is supported, force-bind `127.0.0.1` unless the operator explicitly opts into a public host, and require a non-default secret before allowing any non-loopback bind.

### GO-004 — Gateway capability accesses are never audited (dead audit path)
- **CWE:** CWE-778 (Insufficient Logging)
- **File:** `kernel/agentgw/audit.go` (full `AuditLogger`), constructed at `gateway.go:73` as `NewAuditLogger(nil)`; **no handler in `kernel/agentgw/handlers.go` or `config_handler.go` ever calls `auditLog.Log/LogSync/Flush`** (verified by grep — the only `.Flush()` hits are the SSE `http.Flusher`). The logger is also wired with a `nil` journal and `Attach` never sets a real one.
- **Description & impact:** Every privileged operation an agent subprocess performs through the gateway (memory write/delete, config read/write, eventbus publish) is unlogged. There is no forensic trail for capability abuse, and `writeEntries` short-circuits on the `nil` journal even if it were called. For a system whose security model is "capability tokens + audit", the audit half is non-functional.
- **Remediation:** Call `auditLog.LogSync` (or buffered `Log`) from `withAuth` / each handler with the claims, capability, path, and outcome; wire a real journal into the logger in `Attach`.

---

## MEDIUM

### GO-005 — Gateway JSON decoders have no body-size limit (memory-exhaustion DoS)
- **CWE:** CWE-400 (Uncontrolled Resource Consumption) / checklist item #11
- **File:** `kernel/agentgw/handlers.go:103,144,293` and `gateway.go:290` — `json.NewDecoder(r.Body).Decode(...)` with no `http.MaxBytesReader`. `config_handler.go:191` uses `io.ReadAll(r.Body)` with no cap.
- **Description & impact:** `MaxHeaderBytes` is set on the server (`gateway.go:135`) but request *bodies* are unbounded. A token holder (or, given GO-001/GO-002, anyone) can POST a multi-GB body to `/v1/memory/write`, `/v1/eventbus/publish`, `/v1/log/write`, `/v1/token/create`, or `/v1/config` and exhaust daemon memory. The first-party REST surface (`kernel/restapi/restapi.go:50`) correctly caps bodies at 16 MiB — the gateway should do the same.
- **Remediation:** Wrap `r.Body` in `http.MaxBytesReader(w, r.Body, N)` before decoding in every gateway handler.

### GO-006 — RateLimit window reset never clears the counter (rate limiter is effectively bypassable / wrong)
- **CWE:** CWE-840 (Business Logic Error) / CWE-770
- **File:** `kernel/agentgw/types.go:155-172` (`RateLimit.Allow`).
- **Description & impact:** `Allow()` checks `now - lastTick >= windowMs` and, when true, **returns `true` without ever resetting `r.mu` (the count) or advancing `r.lastTick`**. The counter `r.mu` only ever increments (`atomicAddInt64`) and is never reset, and `lastTick` is set once at construction and never updated. Net effect: for the first 60s a token is limited to `max+burst` requests; after 60s every request takes the early `return true` branch and is allowed unconditionally — the rate limit silently disappears. The comment "Can't use atomic swap for int64 easily, so use sync" flags an unfinished implementation. Also note `RateLimit.mu` is an `int64` count misleadingly named `mu` and mutated via `atomic` while `Allow`'s window branch reads `lastTick`/no atomics — inconsistent synchronization.
- **Remediation:** Implement a proper sliding/fixed window: on window rollover, atomically reset the counter and update `lastTick` (use a `sync.Mutex` around the whole check, or a token-bucket with `atomic.CompareAndSwap`). Add a test that asserts limiting still holds after one window elapses.

### GO-007 — Gateway rate-limit map grows unbounded (per-tid leak)
- **CWE:** CWE-401 (Missing Release of Memory) / CWE-770
- **File:** `kernel/agentgw/gateway.go:25-26,220-229` (`rateLimit map[string]*RateLimit`, never pruned). Also `kernel/configcenter/access.go:30-34,250-257` (`RateLimitMap.agents`/`keys` never pruned).
- **Description & impact:** `allowRate` creates a `*RateLimit` per `claims.SubprocessID` and never deletes it. Since subprocess IDs are per-run/per-subprocess and unbounded over the daemon's lifetime, the map grows without limit — a slow memory leak, and an amplification vector given the unauthenticated mint endpoint (each forged token with a fresh `sub_id` adds an entry). The configcenter rate-limit maps have the same unbounded-growth property keyed by agent id and config key.
- **Remediation:** Evict idle entries (LRU or periodic sweep of entries whose window has long elapsed).

### FE-002 — Untrusted URL rendered in href without scheme validation
- **CWE:** CWE-79 (XSS via `javascript:` URL) / CWE-601
- **File:** `frontend/src/views/Data.tsx:552-554` — `const url = str(r.fields?.bookmarkUrl); … <a href={url} target="_blank" rel="noreferrer">`.
- **Description & impact:** Unlike the Markdown renderer (`frontend/src/lib/markdown.ts:38-40` `safeHref`, which correctly allows only `https?:`/`mailto:`), this anchor renders a database/agent-sourced field straight into `href` with no scheme check. If `bookmarkUrl` is attacker/agent-controlled and contains `javascript:…`, clicking the link executes script in the console origin (which holds the API token — see FE-003). It also omits `noopener` (only `noreferrer` is set), so the opened page gets a live `window.opener` reference.
- **Remediation:** Run the value through `safeHref()` before binding to `href`; add `rel="noopener noreferrer nofollow"`. (Strong CSP would also help as defense-in-depth — see Info note.)

---

## LOW

### FE-003 — API token taken from URL query and kept reachable in the SPA origin
- **CWE:** CWE-598 (Information Exposure Through Query Strings)
- **File:** `frontend/src/lib/api.ts:5` — `export const TOKEN = new URLSearchParams(location.search).get("token") || ""`.
- **Description & impact:** The console reads its bearer token from `?token=` and keeps it in memory (intentionally not in localStorage — a good choice). Residual risk: the token sits in the URL, so it lands in browser history and could leak via `Referer` to any third-party resource the page loads. Combined with any XSS in the SPA origin (e.g. FE-002), the token is directly readable. Low because the console is loopback-bound and the team deliberately avoided localStorage.
- **Remediation:** Prefer an httpOnly cookie set by the daemon on first load, or strip the token from the URL (`history.replaceState`) immediately after reading it; ensure a strict CSP and `Referrer-Policy: no-referrer`.

### GO-008 — `fmt.Sscanf` integer parsing of `limit` ignores errors (input robustness)
- **CWE:** CWE-20 (Improper Input Validation)
- **File:** `kernel/agentgw/handlers.go:200` (`fmt.Sscanf(l, "%d", &limit)`), `config_handler.go:258` (`fmt.Sscanf(limit, "%d", &opts.Limit)`), `cmd/agt/token.go:217`.
- **Description & impact:** Error from `Sscanf` is discarded; malformed input leaves the default, but a caller-supplied large/negative value is passed straight through to `Recall`/`GetAuditLog`/allocation sizing with no bounds clamp. Not an overflow per the governor pricing audit (that path saturates), but unbounded `limit` on memory search can be an amplification knob. Low.
- **Remediation:** Use `strconv.Atoi` with explicit error handling and clamp `limit` to a sane `[1, max]` range.

### GO-009 — Per-task and global budgets are deliberate soft caps (documented, residual cost-abuse risk)
- **CWE:** CWE-362 (informational)
- **File:** `kernel/governor/governor.go:494-512` (global), `517-534` (per-task).
- **Description & impact:** Both carry explicit comments declaring the check-then-spend gap an accepted soft cap bounded by per-call cost. Flagged for completeness: with small per-task budgets and high concurrency the overshoot multiplies. Not a bug per project intent; listed so reviewers know the agent ceiling (GO-003) is the one that is *not* documented as soft.
- **Remediation:** None required if soft-cap is intended; consider documenting the per-task one as explicitly as the global one.

### GO-010 — Tool-capability degradation proceeds silently when strict mode is off
- **CWE:** CWE-636 (Failure of Safe Defaults) — aligns with project "default-allow" posture
- **File:** `kernel/governor/governor.go:408-456`.
- **Description & impact:** When `StrictModelCapabilities=false` (default) and down-routing finds no tool-capable alternative, a tools-bearing request runs on a tool-incapable model; it is journaled but not blocked. This is consistent with the owner's documented default-allow law, so it is intended posture, but it means the capability "downgrade" is advisory, not enforced.
- **Remediation:** None if intended; ensure the journal event is surfaced in monitoring so silent degradation is observable.

### FE-004 — HTML artifact iframe uses `sandbox="allow-scripts"` with `srcDoc` (acceptable, noted)
- **CWE:** CWE-79 (mitigated)
- **File:** `frontend/src/views/Artifacts.tsx:365-373`.
- **Description & impact:** Agent-produced HTML artifacts render in an iframe with `srcDoc` + `sandbox="allow-scripts"`. Because `allow-same-origin` is *not* set, the frame has an opaque origin and cannot read the console token or call same-origin APIs — a correct, defensible design. Noted only because `allow-scripts` does let arbitrary agent script run inside the sandbox (can still do outbound navigation/network within the frame). Acceptable given the threat model.
- **Remediation:** Optionally add `sandbox="allow-scripts"` plus a frame CSP; keep `allow-same-origin` off (as it is).

---

## Positives confirmed (not findings)

- **Frontend XSS surface is clean by construction.** No `dangerouslySetInnerHTML`, no `innerHTML`/`document.write` in product code (only two test assertions), no `eval`/`new Function`/string `setTimeout`. Markdown is a custom dependency-free AST parser rendered as escaped React text (`frontend/src/lib/markdown.ts`, `components/Markdown.tsx`); link hrefs are scheme-validated via `safeHref`. No `postMessage`/cross-origin `message` listeners. Agent/LLM output rendering is safe.
- **REST surface auth is correct:** constant-time token compare via `crypto/subtle.ConstantTimeCompare` (`kernel/restapi/restapi.go:278`), empty token fails closed, 16 MiB body cap (`restapi.go:50`), loopback-bind discipline in `cmd/agezt`.
- **Token HMAC verification uses `hmac.Equal`** (constant-time) — `kernel/agentgw/token.go:95`. The weakness is the *key* (GO-001), not the comparison.
- **Governor pricing math is overflow-safe:** `saturatingMul`/`saturatingAdd` via `bits.Mul64`, negative token counts clamped (`kernel/governor/pricing.go`). No integer-overflow budget bug.
- **Governor response cache is fully mutex-guarded** on all read/write paths (`kernel/governor/cache.go`).
- **Approval registry Submit/Resolve** is lock-correct; detach is idempotent; no auto-approval race (`kernel/approval/approval.go`).
- **Creds vault write** uses atomic temp(0600)-write → fsync → rename → re-chmod 0600 with unique temp names to avoid concurrent-write corruption (`kernel/creds/creds.go:209-235`). No file-perm TOCTOU of concern.
- **configcenter access policy** enforces per-agent/per-key rate limits, allow/exclude lists, rating-based deny, and HITL before returning secret values (`kernel/configcenter/access.go`).
