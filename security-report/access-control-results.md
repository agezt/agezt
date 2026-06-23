# Access Control Security Hunt — Results

Scope: authentication bypass, broken authorization / IDOR, missing-auth routes, CSRF,
session management, control-plane tenant-token allowlist, strict-mode enforcement,
login lockout/timing, agent-gateway child-mint escalation.

Codebase: AGEZT (Go + React), D:\Codebox\PROJECTS\AGEZT

## Summary verdict

The four network auth surfaces (web UI, control plane, agent gateway, REST/OpenAI API)
are, on the whole, well-built: constant-time token compares, deny-by-default tenant
allowlist with pinned tenant arg, JWT alg-pinning + `hmac.Equal`, child-mint subset
enforcement, POST-only mutations, and a strict same-origin CSP. The PR #370 agent-gateway
hardening (no hardcoded secret, authenticated subset-capped mint) is **intact and complete**.

I found no authentication bypass, no missing-auth on a mutating route, no IDOR in the
agent-scoped memory/config reads, and no tenant-token escape. The findings below are a
genuine session-cookie hardening gap (Medium) plus two Low/Info authorization-hygiene
items. None is a critical break of the model.

---

## Finding 1 — Session cookie omits `Secure` behind a TLS-terminating proxy

- **Severity:** Medium
- **CWE:** CWE-614 (Sensitive Cookie Without 'Secure' Attribute) / CWE-319
- **File:** `kernel/webui/session.go:211-219` (login) and `:228-235` (logout)
- **Confidence:** Medium

### What happens
The console session cookie is minted with:
```go
Secure: r.TLS != nil,
```
The `Secure` flag is therefore set **only** when the daemon itself terminates TLS. The
console is HTTP loopback by default, and the documented use case for the password
"alternative door" + `AGEZT_WEB_PASSWORD_STRICT` is explicitly *"operators who exposed the
console beyond loopback (tunnel) and want two factors"* (session.go:23-26, webui.go:1057-1058).

In that exact deployment the operator fronts the plaintext daemon with a TLS-terminating
tunnel/reverse proxy (cloudflared, ngrok, nginx). The proxy speaks HTTPS to the browser but
plain HTTP to the daemon, so `r.TLS == nil` and the `agezt_web_session` cookie is issued
**without `Secure`**. A browser will then attach that session cookie to any plaintext
`http://` request to the same host — e.g. a downgrade attempt, a mixed-content subresource,
or an attacker on the path who forces one cleartext request — leaking the 12-hour session id.

### Impact
Session-token disclosure → full console takeover (halt the fleet, edit agents/policy/budget,
exfiltrate memory) for an operator running precisely the multi-factor remote setup the feature
targets. Bounded by: requires the remote-exposed configuration (not the loopback default) and
a path/downgrade position.

### Remediation
Set `Secure` whenever the request arrived over HTTPS *or* was forwarded as HTTPS by a trusted
proxy: honor `X-Forwarded-Proto: https` (only from a configured/trusted proxy hop), or add an
`AGEZT_WEB_SECURE_COOKIES` opt-in that forces `Secure: true`. At minimum, force `Secure` when
the password/strict feature is enabled, since that mode presumes remote exposure.

---

## Finding 2 — `config.write` has no per-key owner check; a write-capable token can grant itself access to any key

- **Severity:** Low
- **CWE:** CWE-639 (Authorization Bypass Through User-Controlled Key) / CWE-862
- **File:** `kernel/agentgw/config_handler.go:177-237` (`handleConfigSet`)
- **Confidence:** Medium

### What happens
Config **reads** are scoped per-agent: `handleConfigGet`/`List`/`Search` filter by
`claims.SubprocessID` against each entry's rating + allow/deny lists (config_handler.go:56,116,148).
Config **writes** do not. `handleConfigSet` checks only that the caller holds the
`config.write` capability, then accepts an arbitrary `key`, `value`, and — notably —
`AllowedAgents` / `ExcludedAgents` on the entry (config_handler.go:220-225). So any token
granted `config.write` can:
- overwrite a config key owned by / scoped to a different agent, and
- author an entry whose `AllowedAgents` includes itself, i.e. grant itself read access to a
  key it should not see.

There is no check that the writing agent owns the key or is permitted to set those access lists.

### Impact / reachability caveat
This requires already holding `config.write`, which is a privileged capability (`types.go:106`
comments it "privileged"; `edict.go:650` groups it with the high-trust caps). It is **not**
in the `agt token create` CLI's mint allowlist (`cmd/agt/token.go:167-185` omits all config
caps), so it is operator-granted, not freely self-mintable. Real impact is therefore limited
to a scenario where one agent is deliberately given `config.write` and is expected to be
confined to its own keys — that confinement does not exist. Given the platform's documented
default-allow posture for agents, this may be by design; flagging for confirmation.

### Remediation
On write, enforce that the caller may write the key (ownership or an explicit write-ACL), and
forbid a non-operator token from setting `AllowedAgents`/`ExcludedAgents` (or from setting them
to include itself). Mirror the per-agent scoping already applied on the read path.

---

## Finding 3 — `/api/logout` accepts any method (no method/CSRF guard)

- **Severity:** Info
- **CWE:** CWE-352 (CSRF)
- **File:** `kernel/webui/session.go:224-237`, registered token-free at `kernel/webui/webui.go:570`
- **Confidence:** High (behavior) / Low (impact)

### What happens
`handleLogout` is registered via `s.secure` (token-free, by design — it is part of the auth
surface) and performs no method check, so it serves GET as well as POST. It revokes the
session named by the request's `agezt_web_session` cookie.

### Impact
Minimal. The only state change is logging the victim out, and it only fires if the browser
attaches the session cookie — which `SameSite=Strict` (session.go:215, :231) prevents for any
cross-site request. So a cross-site `<img src=/api/logout>` sends no cookie and revokes nothing.
This is a denial-of-convenience at worst and is effectively neutralized by SameSite=Strict.
Noted for completeness, not as an exploitable issue.

### Remediation
Restrict logout to POST for consistency with every other mutating route.

---

## Areas verified clean (no finding)

- **Web UI auth gate** (`webui.go:976-1077`): constant-time token compare; empty token never
  authorizes; default password mode = token OR session, strict = token AND session — both
  correct. All write/json routes are POST-only (`writeProxy`, `decodeAllowedBody`); read-arg
  routes are genuinely read-only. CSRF on cookie-authed mutations is closed by `SameSite=Strict`
  + same-origin CSP (`connect-src 'self'`, `form-action 'none'`) + no state-changing GETs.
- **Login lockout / timing** (`session.go:106-129,177-221`): constant-time password compare,
  8-fail lockout with 5-min cooldown, counter reset on success. Sound.
- **Session management**: fresh 32-byte CSPRNG id minted on login (no fixation — no
  pre-set id is honored), HttpOnly + SameSite=Strict, server-side revoke on logout, sliding
  12h TTL, sessions die with the daemon.
- **Control-plane tenant allowlist** (`controlplane/tenant.go:68-89`, `server.go:478-504`):
  deny-by-default; tenant token restricted to a fixed read/own-work command set; tenant arg
  **pinned** to the authorized tenant after auth (cannot target another tenant or daemon-global
  state). Edict mutations in the allowlist route through `kernelFor(tenantID)` (edict.go:27-35),
  staying tenant-scoped. `tenant.Registry.Authorize` is constant-time (tenant.go:214-223).
- **Agent gateway** (`agentgw/gateway.go`, `token.go`, `secret.go`): `/health` is the only
  unauth route (no data); every other route behind `withAuth`. JWT alg pinned to HS256/JWT
  (rejects `none`/alg-confusion), signature verified with `hmac.Equal`, expiry enforced.
  Child-mint (`handleTokenCreate`) is authenticated, caps subset-checked (rejects escalation),
  expiry clamped to parent, RunID inherited. Token secret is per-install CSPRNG (0600 file or
  env) — the former hardcoded `change-me-in-production` is gone. **PR #370 fixes intact.**
- **REST + OpenAI API** (`restapi/restapi.go:262-298`, `openaiapi/openaiapi.go:228-262`):
  Bearer, constant-time compare, empty fails closed; per-tenant tokens authorize only their own
  tenant (pinned via `X-Agezt-Tenant`) and `bind()` resolves the engine from the same header, so
  a tenant token cannot read another tenant. `/healthz`/`/readyz` unauth liveness only.
- **Agent-scoped memory** (`agentgw/handlers.go`): Remember/Recall/Forget all keyed by
  `claims.RunID` — no IDOR across agents.
- **Workflow webhook** (`webui.go:674-745`): per-workflow secret, constant-time at the control
  plane, uniform refusals (no oracle), POST-only, body-capped. Sound.
