# Access Control Results — `sc-auth` + `sc-authz`

**Target:** AGEZT @ `main` `f815f56e`
**Date:** 2026-08-12
**Scope:** `kernel/auth`, `kernel/httpserver`, `kernel/restapi`, `kernel/webui`,
`kernel/controlplane`, `kernel/tenant`, `kernel/tenantctx`, plus the daemon wiring in
`cmd/agezt/httpsurfaces.go` that determines the effective policy of those packages.
**Method:** read-only source review. Nothing in the tree was modified.

Five findings. Two are High/Medium-impact and concrete; three are lower-severity
latent gaps in the centralisation the new `kernel/httpserver` router is supposed to
guarantee. Section 6 lists what was checked and found correct, so a verifier does not
re-tread it.

---

## AUTH-001 — Console password-strict mode is disarmed at boot; a non-loopback console falls back to single-factor

- **Severity:** High
- **Confidence:** 88
- **CWE:** CWE-863 (Incorrect Authorization), via CWE-696 (Incorrect Behavior Order)
- **File:**
  - `D:\Codebox\PROJECTS\AGEZT\cmd\agezt\httpsurfaces.go:114` and `:125`
  - `D:\Codebox\PROJECTS\AGEZT\cmd\agezt\httpsurfaces.go:243` (`webAllowedHosts`)
  - `D:\Codebox\PROJECTS\AGEZT\kernel\webui\webui.go:139-163` (`SetAllowedHosts`)
  - `D:\Codebox\PROJECTS\AGEZT\kernel\webui\session.go:138-142` (`SetPasswordStrict`)
  - `D:\Codebox\PROJECTS\AGEZT\kernel\webui\webui.go:1443-1452` (`authorized`)

### Description

`webui.SetAllowedHosts` documents and implements an automatic second-factor
escalation: registering any non-loopback host means the console is reachable beyond
localhost, so `s.passwordStrict = true` — the bearer token AND the password session are
then both required on every data route. `Server.authorized` honours that:

```go
if s.passwordStrictOn() { return s.dataTokenPresented(r) && s.sessionValid(r) }
return s.dataTokenPresented(r) || s.sessionValid(r)   // alternative-door default
```

The daemon defeats this control in two independent ways.

**(a) Overwrite by initialization order.** `buildWebUI` calls
`SetAllowedHosts(...)` at line 114 and then `SetPasswordStrict(passwordStrict)` at
line 125. `SetPasswordStrict` assigns unconditionally (`s.passwordStrict = on`), and
`passwordStrict` is `AGEZT_WEB_PASSWORD_STRICT == "on"` — false by default. So any
strict flag the auto-escalation just raised is immediately cleared. This is the exact
path taken by an explicit non-loopback bind (`AGEZT_WEB_ADDR=192.168.1.10:8787` →
`webAllowedHosts` returns `["192.168.1.10"]`) and by `AGEZT_WEB_ALLOWED_HOSTS`
(reverse-proxy / domain deployments — the deployment the comment was written for).

**(b) Trigger never evaluated for a wildcard bind.** `webAllowedHosts`
(`httpsurfaces.go:246`) skips unspecified IPs, so an `AGEZT_WEB_ADDR=0.0.0.0:8787`
bind registers *no* allowed host and `SetAllowedHosts` returns early on
`len(hosts) == 0`. The console is nevertheless reachable from the whole LAN, because
`hostAllowed` (`webui.go:1337-1339`) accepts **any** IP-literal `Host` header
(`return !ip.IsUnspecified()`). Strict mode therefore never arms for the most common
"expose it to my network" configuration.

The tunnel path is *not* affected: `buildTunnel`'s `OnURL` callback invokes
`web.allowHost(host)` at runtime, long after line 125, so a tunnelled console does get
strict mode. That the runtime path works and the static path does not is what makes
this look accidental rather than intended.

### Exploit scenario

Operator runs `AGEZT_WEB_PASSWORD='<chosen password>' AGEZT_WEB_ADDR=0.0.0.0:8787 agezt`
(or binds a LAN IP). They read `SetAllowedHosts`' contract — "a guessed password alone
is insufficient — the bearer token is also required" — and believe the console is
two-factor. It is not; `authorized()` runs the `||` branch.

An attacker on the same network browses to `http://192.168.1.10:8787/`, is served the
SPA shell (`/` is `TierPublic`), and posts to the token-free `/api/login`. The lockout
is 8 consecutive failures per 5 minutes, global rather than per-source and reset on any
success — roughly 2,300 guesses/day against a single human-chosen password, with no
alerting requirement. On success the browser holds a 12-hour sliding session cookie that
opens the entire authenticated console with no second factor:

- `POST /api/run` — arbitrary governed agent runs (spends the operator's provider budget)
- `POST /api/files/mkdir|rename|delete`, `GET /api/files/raw` — the workspace filesystem
- `POST /api/config/set` — writes settings and secrets into the vault
- `POST /api/toolbox/install` — invokes the host package manager
- `POST /api/halt`, `/api/agents/*`, `/api/schedule/*`, `/api/edict/set_mode` — full control-plane mutation surface

### Remediation

Make the escalation monotonic. Either have `SetPasswordStrict(false)` refuse to lower a
strict flag that host policy raised (an explicit `ForcePasswordStrict(false)` for the
opt-out), or move the `SetPasswordStrict` call before `SetAllowedHosts` in
`buildWebUI`. Separately, treat a wildcard/unspecified bind as non-loopback for the
strict decision — the listener being `0.0.0.0` is *stronger* evidence of exposure than a
named host, not weaker.

---

## AUTH-002 — Hardcoded default console password `agezt`

- **Severity:** Medium
- **Confidence:** 95 (the literal is unambiguous; only the reachability of the local
  attacker moves the severity)
- **CWE:** CWE-1392 (Use of Default Credentials); CWE-798 (Hardcoded Credentials)
- **File:** `D:\Codebox\PROJECTS\AGEZT\cmd\agezt\httpsurfaces.go:205-219`

### Description

```go
const defaultLoopbackWebPassword = "agezt"

func effectiveWebPassword(addr string) string {
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_PASSWORD")); v != "" { return v }
	switch strings.ToLower(...Getenv(brand.EnvPrefix + "WEB_PASSWORD_DEFAULT")) { case "off", ...: return "" }
	if isLoopback(addr) { return defaultLoopbackWebPassword }
	return ""
}
```

Every default (loopback) daemon therefore ships with a known console password, and
`authorized()` in the non-strict default treats the password session as a **complete
alternative credential** — not a second factor. The password is a fixed literal in a
public repository; it is not per-install, not derived, and there is no forced change on
first login.

### Exploit scenario

Any principal that can open a TCP connection to `127.0.0.1:8787` but does *not* have the
bearer token — a second local user account on a shared workstation or build host, a
sidecar container sharing the host network namespace, any locally-running unprivileged
process — performs:

```
POST http://127.0.0.1:8787/api/login   {"password":"agezt"}
```

`sameOriginMutation` passes because a non-browser client sends no `Origin`, and
`hostAllowed` passes because `127.0.0.1` is an IP literal. The response sets
`agezt_web_session`, which opens every data route listed in AUTH-001. This converts
"can run code as another local user" into "full daemon admin, arbitrary agent runs on
the operator's budget, vault writes".

A secondary chain worth a verifier's attention: the `code_exec` sandbox is documented in
this project as deliberately network-enabled and allow-by-default. If sandboxed tool code
can reach host loopback, then prompt-injected agent content reaches the same login
endpoint, escalating a content-level compromise to console admin. I did not verify the
sandbox's loopback reachability, so this is stated as a hypothesis, not a claim.

### Remediation

Mint a random per-install first-run password (like the console token already is), print
it once on the banner, and force a change on first successful login. If a memorable
first-run password is a hard product requirement, gate every data route behind
`passwordStrict` until the default has been replaced.

---

## AUTHZ-001 — `/metrics` sits one tier below its daemon-global siblings; a per-tenant credential reads primary-kernel spend and activity

- **Severity:** Medium
- **Confidence:** 85
- **CWE:** CWE-863 (Incorrect Authorization); CWE-200 (Exposure of Sensitive Information)
- **File:**
  - `D:\Codebox\PROJECTS\AGEZT\kernel\restapi\restapi.go:212-214` (registration)
  - `D:\Codebox\PROJECTS\AGEZT\kernel\restapi\restapi.go:280-306` (`handleMetrics` — no `s.bind`)
  - `D:\Codebox\PROJECTS\AGEZT\cmd\agezt\httpsurfaces.go:508`, `:597-661` (`restMetrics(k)` closes over the **primary** kernel)
  - `D:\Codebox\PROJECTS\AGEZT\kernel\httpserver\auth.go:70-78` (tenant credential satisfies `TierUser`)

### Description

`/metrics` is registered as `metricsRoute := userRoute` — `TierUser`. In
`Authenticator.Authorized`, `TierUser` is satisfied *either* by the daemon admin token
*or*, when `TenantAuthorize` is wired, by any tenant's own token presented with a
matching `X-Agezt-Tenant` header. `SetTenantAuthorizer` is wired whenever the tenant
registry exists (`httpsurfaces.go:519`).

But `handleMetrics` never calls `s.bind(r)`. It formats whatever `s.metrics()` returns,
and the daemon binds that closure to the **primary** kernel `k`
(`rest.SetMetrics(func() []restapi.Metric { return restMetrics(k) })`). The response is
daemon-global, not tenant-scoped:

`agezt_spend_today_microcents`, `agezt_budget_ceiling_microcents`, `agezt_active_runs`,
`agezt_memory_records`, `agezt_world_entities`, `agezt_active_skills`,
`agezt_pending_approvals`, `agezt_journal_head_seq`, `agezt_journal_bytes`,
`agezt_schedules_total/enabled`, `agezt_disk_free_bytes`.

This is precisely the reasoning the same file applies to its siblings and reaches the
opposite conclusion for them:

- `restapi.go:221-230` — mailbox routes are `adminRoute`/`adminBodyRoute` because "the
  board is a single daemon-global instance with no tenant partition, so it is gated to
  the admin tier only — a per-tenant token must not reach it" (V-011).
- `restapi.go:232-234` — update routes are admin-only because self-update "changes
  host-global daemon state".
- `restapi.go:209-211` — `/metrics` itself was deliberately moved *off* the public tier
  because "it exposes spend and activity volume (financially/operationally sensitive)".

The route is one tier short of the classification its own comment gives it. The same
`s.bind` omission also affects `/api/v1/health` (`restapi.go:310-321`) and
`/api/v1/models` (`restapi.go:325-345`), which report the primary engine's default model
and model count regardless of the tenant header — lower sensitivity, same root cause.

### Exploit scenario

Multi-tenant daemon with `AGEZT_REST_ADDR` set. Tenant `acme` legitimately holds its own
`.tenant-token` (minted by `tenant.Registry.Acquire`) and is by design confined to its
own kernel on every other route.

```
curl -H 'Authorization: Bearer <acme-tenant-token>' \
     -H 'X-Agezt-Tenant: acme' \
     http://daemon:PORT/metrics
```

`Authorized(r, TierUser)` → admin compare fails → `TenantAuthorize("acme", tok)` → true.
`handleMetrics` returns the operator's daemon-wide daily spend, budget ceiling, in-flight
run count, memory/world/skill inventory sizes and journal growth. Polled on a timer this
is a continuous side channel on every *other* tenant's workload and on the operator's
cost position — data `acme` cannot obtain through any other route.

### Remediation

Register `/metrics` with `adminRoute` (matching mailbox and update), or make the metric
source tenant-aware by resolving through `s.bind(r)` and emitting only that tenant's
kernel gauges. Add `s.bind(r)` to `handleHealth` and `handleModels` for consistency.

---

## AUTHZ-002 — The router records `RouteOpts.Method` but never enforces it

- **Severity:** Low (no currently-exploitable instance; latent)
- **Confidence:** 95
- **CWE:** CWE-1220 (Insufficient Granularity of Access Control)
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\httpserver\router.go:78-100`, `:116`

### Description

`Router.Handle` parses, upper-cases, validates and de-duplicates `opts.Method`, panics on
a malformed method, and stores the result in the `Route` snapshot — then registers the
handler with the method stripped:

```go
method := strings.Join(normalizedMethods, ",")   // ...only ever used as metadata
...
rt.mux.HandleFunc(pattern, wrapped)              // no method in the pattern
```

Go 1.22's `ServeMux` accepts `"POST /api/x"` patterns, so the enforcement point exists
and is simply not used. The consequence is that "per-route policy data rather than
middleware ordering" is only *half* centralised: tier and body cap are enforced by the
router, method is not, and each handler must remember to re-check `r.Method` itself.

I verified every registration on all three surfaces: every mutating handler does
currently re-check (`webui.writeProxy`, `decodeAllowedBody`, `handleRollbackApply`,
`handleTranscribe`, `handleTTS`, `handleFileMkdir/Rename/Delete`, `handleWorkflowHook`,
`restapi.handleUpdateApply`, all five mailbox handlers, `openaiapi.handleTranscription`).
So there is no exploit today. The routes that *do* accept any method
(`webui.proxy`, `readArgsProxy`, `handleArtifactRaw`, `handleOAuthCallback`) are
read-only or idempotent.

It is a trap rather than a bug because of a second property: `webui.sameOriginMutation`
(`webui.go:1345-1348`) returns `true` unconditionally for `GET`/`HEAD`/`OPTIONS`. The
day a mutating handler is added without its own method check, it becomes GET-reachable
*and* skips the cross-origin guard in one step — two layers lost to one omission. (It
would still need a credential, which a cross-site GET cannot supply, so the residual risk
is bounded.)

### Remediation

Register method-qualified patterns when `opts.Method != "*"` (one pattern per method), or
add a method check to the wrapper chain alongside the tier and body-limit wrappers, and
delete the now-redundant per-handler checks. Add a `Routes()`-driven guard test asserting
that no route declares `Mutation: true` with a method the router does not enforce.

---

## AUTHZ-003 — The WebUI's `RequestAuthorize` discards the required tier

- **Severity:** Low (no privilege gap today; fails open by construction)
- **Confidence:** 90
- **CWE:** CWE-863 (Incorrect Authorization)
- **File:** `D:\Codebox\PROJECTS\AGEZT\kernel\webui\webui.go:749-755`

### Description

```go
router := httpserver.NewRouter(httpserver.Authenticator{
	RequestAuthorize: func(r *http.Request, _ kernelauth.Tier) bool {
		return s.authorized(r)
	},
}, ...)
```

`Authenticator.Authorized` gives `RequestAuthorize` complete ownership of every protected
route's decision (`auth.go:60-62`) — including, structurally, `TierAdmin`. The WebUI's
closure ignores the tier argument entirely, so the router's tier model is inert on this
surface: all ~200 protected routes collapse to the single `s.authorized(r)` predicate.

Today this is harmless — I confirmed every WebUI route uses only `TierPublic` or
`TierUser`, so there is nothing for a tier to distinguish. The problem is the direction of
the failure. A future `TierAdmin` WebUI route (a plausible home for
`/api/toolbox/install`, `/api/config/set`, or self-update) would be registered with the
stronger declaration, would *appear* stronger in `Routes()` inspection, and would be
authorized at user level. Compare `restapi`, where the same struct's `Verifier` path does
honour the tier.

### Remediation

Either honour the tier in the closure (e.g. require the console bearer token, not merely
a password session, for `TierAdmin`), or make the WebUI's inability to distinguish tiers
explicit by panicking on a `TierAdmin` registration until a real admin credential exists
on that surface.

---

## 6. Verified correct — do not re-investigate

These were examined against the sc-auth / sc-authz checklists and found sound. Listing
them so the verification pass can spend its budget on the findings above.

**Credential comparison.** Every secret comparison in scope is constant-time and
fail-closed on blank inputs: `auth.tokenEqual` (`kernel/auth/token.go:68-73`, explicit
`configured == ""` guard), `StaticVerifier.Authorize` (checks *all* user tokens without
early return so token position is not timed), `webui.handleLogin`
(`session.go:217`), `controlplane.tokenIsPrimary` (`server.go:347-355`, with the
`ConstantTimeCompare("","") == 1` pitfall explicitly handled),
`tenant.Registry.Authorize` (`tenant.go:214-223`).

**Tier model.** `Tier.Valid()` is total over `uint8`; an out-of-range tier fails
`Authorize` and panics at registration. `TierPublic` short-circuits before any verifier
lookup; every other path requires a non-blank credential and a non-nil verifier.

**Control-plane tenant isolation.** `handleConn` (`controlplane/server.go:531-563`) is
correct: the primary token authorizes everything; otherwise the request must name a
tenant *and* present that tenant's token, the command must be `TenantAllowed`, the
allowlist is applied **before** unknown-command distinction (no command-existence oracle),
and `req.Args["tenant"]` is re-pinned to the authorized id. The
`TenantAllowed ⇒ TenantRouted` invariant is enforced by a permanent test
(`dispatch_registry_test.go:89`), and I spot-checked six tenant-allowed handlers
(`handleCacheStats`, `handleMemoryAudit`, `handleEdictStats`, `handleRunsList`,
`projectJournal`, `handleWardenLog`) — all resolve via `s.kernelFor(tenantOf(req))` rather
than touching `s.k`.

**Tenant id containment.** `tenant.baseDir` validates against `^[a-z0-9][a-z0-9_-]{0,63}$`
and then re-checks `filepath.Dir(dir) == root`. Token minting is `O_CREATE|O_EXCL`,
0600, race-safe, with the stale-blank-file reclaim path.

**REST tenant binding.** `restapi.bind` and `Authenticator.Authorized` read the same
header with the same trimming, so a credential can never authorize tenant A and be routed
to tenant B. `SetTenantResolver` and `SetTenantAuthorizer` are wired together under one
`if reg != nil` on both surfaces — a tenant token cannot exist without a resolver and thus
cannot fall through to the primary engine. Mailbox and update are correctly `TierAdmin`.

**Router wrapper order.** Auth is applied outside the body limit, so an unauthenticated
oversize POST is rejected before the body is read. Negative caps/timeouts and nil handlers
panic at startup.

**Webhook surface.** `/hooks/` refusals are uniform (unknown name, bad secret and disabled
all return the same 403), the rate limit is applied pre-auth so probing cannot burn budget,
and the bucket key uses `r.RemoteAddr` (`streamcap.go:19-24`) — not a spoofable
`X-Forwarded-For`. Same for `httpserver.sseClientKey` (`sse.go:43-48`).

**File Manager traversal.** `resolveFileRoot` is a genuine single chokepoint: NUL
rejection, absolute/drive-letter rejection, `..` segment rejection post-`Clean`, and a
separator-anchored prefix check (not bare `HasPrefix`). Reads and deletes use `Lstat`
specifically so a symlink cannot hide behind `Stat`'s resolution.

**Session cookie.** `HttpOnly`, `SameSite=Strict`, 32 bytes of `crypto/rand`, sliding
12-hour expiry, revoked on logout, POST-only login and logout, and `cookieSecure` correctly
reasons that honouring `X-Forwarded-Proto` can only *add* the `Secure` attribute.

**No route escapes the registry.** All three HTTP surfaces in scope route exclusively
through `httpserver.Router`; `webui.Handler` wraps the whole registry in `secure` so
security headers, the `Host` allowlist and the cross-origin mutation check also cover
public routes and 401 responses. `kernel/agentgw` and `controlplane/provider_oauth.go`
still use raw `ServeMux` — out of the stated scope, but they are the only two remaining
un-migrated surfaces and are noted here for completeness (`agentgw` gates every route
except `GET /health` behind `withAuth`).

### Non-security defects noticed in passing

- `kernel/webui/files_route.go:201-215` — `fi, ferr := e.Info()`; when `ferr != nil`, `fi`
  is nil and line 213's `fi.Mode()` dereferences it. Reachable if a file is removed between
  `ReadDir` and `Info`. Panic → 500 on that request only; a reliability bug, not authz.
- `kernel/webui/webui.go:849` — `filesMutation.BodyMax = defaultFileCap`, and
  `defaultFileCap = 256` (`files_route.go:35`). A **256-byte** body cap on
  mkdir/rename/delete looks like a units mix-up against the neighbouring
  `defaultMaxBytes = 4 MiB`; a rename with two long paths will 400. Availability, not
  security.
