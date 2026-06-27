# Access Control — Security Findings

Domain: authentication, authorization/IDOR, privilege escalation, session management, JWT.
Repo root: `D:\Codebox\PROJECTS\AGEZT`. Audited the live tree (not the stale `.worktrees/` copies).

Method: discovery → reachability/exploitability verification. Each finding cites file:line, gives a
concrete attack scenario, remediation, and a confidence score. Controls that are correctly enforced are
recorded as explicit negative results (they matter — several previously-flagged holes are now genuinely closed).

---

## Summary

| ID | Severity | Title | Confidence |
|----|----------|-------|-----------|
| AC-01 | **High** | Trust ceiling is last-write-wins, not min-clamp — delegation to a higher-ceiling profile escapes the parent's initiative cap | 90 |
| AC-02 | **High** | Untrusted Pulse-observation content enters the autonomous run via the *intent*; the prompt-injection guard (taint-based) does not cover the intent path | 75 |
| AC-03 | **Medium** | `act_or_ask`/empty initiative mode + no `max_trust` ⇒ uncapped autonomous run (the most-permissive dial is silently the no-cap dial) | 80 |
| AC-04 | Low | Roster `System` flag protection relies entirely on callers; store `Update` does not re-assert it (defense-in-depth gap, no current bypass) | 60 |

Clean (verified, no vulnerability):
- Control-plane primary/tenant token (constant-time, fail-closed, empty-token rejected).
- Web UI auth/session (constant-time password, lockout, `HttpOnly`+`SameSite=Strict`+proxy-aware `Secure`, sliding TTL, no token-in-URL except `/events`/shell by design).
- REST/OpenAI bearer + `adminAuth` tier; tenant runs are engine-bound (no cross-tenant IDOR).
- agentgw HMAC-JWT: alg pinned, iss/aud pinned, exp checked, `hmac.Equal` constant-time, cap-subset enforced, per-install CSPRNG secret (no hardcoded key, no unauth mint).
- HITL approval gate: no self-approve path, operator-token-gated, timeout fails **closed**.

No Critical findings. The highest-impact issue is the compounding chain **AC-03 + AC-02 + AC-01**: a
loosely-configured *enabled* initiative order can take injected-content-driven autonomous action, and even a
*capped* run holding the default-allow `delegate` tool can launder its work through a higher-ceiling agent.

---

## AC-01 — Trust ceiling is overwritten (not min-clamped) across delegation → privilege escalation

- **Severity:** High
- **CWE:** CWE-269 (Improper Privilege Management)
- **Confidence:** 90
- **Files:**
  - `kernel/runtime/runtime.go:2005-2010` (`WithTrustCeiling` — unconditional overwrite)
  - `kernel/runtime/runtime.go:2228-2232` (`WithAgentProfile` re-applies the profile's ceiling)
  - `kernel/runtime/subagent.go:561,573-575` (child ctx derived from parent, then profile applied)
  - `kernel/runtime/runtime.go:1534-1538` + `kernel/edict/edict.go:726,776-779` (the single ceiling on the child ctx is what the clamp uses)

**What was found.** `WithTrustCeiling` sets the ceiling as a plain context value with no regard for any
ceiling already present:

```go
func WithTrustCeiling(ctx context.Context, ceiling edict.TrustLevel) context.Context {
    if ceiling >= edict.LevelAllow { return ctx }      // L4/empty = no-op
    return context.WithValue(ctx, ctxKeyTrustCeiling, ceiling)   // LAST-WRITE-WINS
}
```

It is last-write-wins, **not** a minimum-clamp. In the delegation path, the child context is derived from
the parent (`subagent.go:561`, inheriting the parent's ceiling), and then — when the delegation targets a
roster profile — `WithAgentProfile(childCtx, *prof)` is applied (`subagent.go:573-575`), which re-applies the
*target profile's* `TrustCeiling` via `WithTrustCeiling` (`runtime.go:2228-2232`). The policy hook then reads
only that single (now-looser) value (`runtime.go:1534`) and `DecideWithCeiling` clamps with
`if ceiling < lvl { lvl = ceiling }` (`edict.go:776`) — using whatever ceiling survived the overwrite.

**Attack scenario.** A run is capped at L0/L1/L2 by a standing-order initiative ceiling
(`cmd/agezt/main.go:5271-5273` → `WithTrustCeiling`). That run holds the default-allow `delegate` tool
(`CapDelegate`). It delegates a task to any **directly-callable** agent (the default for every profile) whose
profile `TrustCeiling` is higher (L3) or empty (→ L4, no cap). Because the apply overwrites instead of
min-merging, the delegated child runs at the **looser** ceiling, so capabilities the parent run was forbidden
to use auto-allowed (or to ask-gate) execute in the child. The non-overridable hard-deny floor still holds,
but the entire L1/L2/L3-vs-L4 ask/approval surface is bypassed for the delegated work.

**Remediation.** In `WithTrustCeiling`, read any existing `ctxKeyTrustCeiling` and store
`min(existing, ceiling)` rather than overwriting; or have the delegation path explicitly intersect the
parent ceiling with the profile ceiling before applying. A ceiling must only ever tighten down the
delegation tree, never loosen.

---

## AC-02 — Untrusted Pulse-observation content reaches the autonomous run via the intent; injection guard does not cover it

- **Severity:** High
- **CWE:** CWE-1427 / CWE-77 (prompt injection → autonomous action)
- **Confidence:** 75
- **Files:**
  - `kernel/standing/runner.go:134-152` (`TriggeredIntent` embeds trigger payload JSON verbatim into the intent)
  - `cmd/agezt/main.go:5229` (the fired order builds its intent through `TriggeredIntent`)
  - `kernel/runtime/runtime.go:1616-1627` (prompt-injection guard fires only on `UntrustedObservationTaint`, attached to *tool observations*)
  - `kernel/pulse/engine.go` (`pulse.initiative.act` emission carries observation summary/reason)

**What was found.** When a standing order fires from a `pulse.initiative.act` event, the trigger payload —
which for web/external Pulse observers can derive from attacker-controlled scraped/ingested content — is
serialized verbatim into the agent's user intent:

```go
b.WriteString("\nTrigger payload:\n```json\n"); b.Write(raw); b.WriteString("\n```")
```

The prompt-injection guard in `policyHook` only engages when the context carries
`UntrustedObservationTaint`, which the agent loop attaches to **tool observations** within a causal window.
Content entering through the **intent string** carries no such taint, so `hasTaint` is false and the guard
never engages for this path. Directive-like text planted in a source a Pulse observer reads can therefore land
in an autonomous run's prompt with the injection guard disengaged.

**Reachability / mitigation.** The seeded `guardian-initiative` responder ships **disabled** and capped at
**L2** (`plugins/builtinguardians/builtinguardians.go:363`, `defaultTrustCeiling`), so this is dormant by
default. It becomes exploitable when an operator (or an agent via the `standing` tool) enables/edits an
`act`-mode order bound to `pulse.initiative.act`. The only backstop on the autonomous run is the trust
ceiling — which AC-03 may remove and AC-01 can escalate away.

**Remediation.** Treat the `pulse.initiative.act` trigger payload as untrusted observation: either run the
fired order's intent through the same injection screening / `UNTRUSTED OBSERVATION` wrapping used for tool
output, or attach `UntrustedObservationTaint` to the run context when the intent is built from an external
trigger payload so the existing guard's approval gate engages on the next effectful action.

---

## AC-03 — `act_or_ask`/empty initiative mode with no `max_trust` runs uncapped

- **Severity:** Medium
- **CWE:** CWE-269 (Improper Privilege Management)
- **Confidence:** 80
- **Files:**
  - `kernel/standing/standing.go:77-86` (`MaxAutonomyTrust()` returns `("", false)` for `act_or_ask`/empty)
  - `cmd/agezt/main.go:5133-5148` (`standingTrustCeiling` returns `(_, false)` when mode imposes no cap and `MaxTrust==""`)
  - `cmd/agezt/main.go:5271-5273` (fire path only calls `WithTrustCeiling` when a ceiling exists)

**What was found.** `standingTrustCeiling` derives the effective ceiling from the more restrictive of the
order's `max_trust` and the level implied by its initiative mode. For `act_or_ask` (and empty) mode,
`MaxAutonomyTrust()` returns no cap; so when the order also has `MaxTrust==""`, `standingTrustCeiling` returns
`(_, false)` and the fire path **never calls `WithTrustCeiling`**. The run then executes at full default-allow
(L4) capability across every cap.

**Attack scenario.** An operator — or an agent holding the `standing` capability (`CapStanding`) — creates or
edits an *enabled* standing order with mode `act_or_ask` and no `max_trust`. Every firing then runs uncapped.
The "initiative mode" reads like the safety dial, but its most-permissive setting silently equals "no trust
clamp." If the order also names no `Agent` (so there is no profile ceiling either), there is zero autonomy
bound on the run.

**Remediation.** Apply a non-`LevelAllow` default ceiling whenever an order has an autonomous mode
(`act`/`act_or_ask`) and no explicit `max_trust`, mirroring the seeded responder's L2 default — i.e. make the
fire path fail safe rather than uncapped. Surface the effective ceiling in `standing_list` so an operator can
see "uncapped" before enabling.

---

## AC-04 — Roster `System` protection depends on callers; store `Update` does not re-assert the flag

- **Severity:** Low (defense-in-depth — no current bypass)
- **CWE:** CWE-269 (Improper Privilege Management)
- **Confidence:** 60
- **Files:**
  - `kernel/controlplane/roster.go:1149` (add forces `p.System = false`)
  - `kernel/controlplane/roster.go:1212-1237` (`applyAgentMutableProfileFields` whitelist excludes `System`)
  - `plugins/tools/overseertool/kernelsource.go:100-102,143` (overseer tool refuses to edit a System guardian; create forces `System=false`)
  - `kernel/roster/roster.go:840-865` (store `Update` preserves identity from the prior profile but does not itself re-assert `System`)

**What was found.** The `System` flag is currently unspoofable from every client/agent path: the control-plane
add forces it false, the edit mutable-field whitelist omits it, and the overseer agent tool both forces
`System=false` on create and refuses outright to edit a System guardian. System guardians also cannot be
hard-deleted to free their slug (`kernel/runtime/runtime.go:1248-1252`). **No working bypass was found.**

The residual risk is structural: the store-level `Update` does not re-assert `System` from the prior
snapshot; protection rests entirely on the two callers never copying `in.System` into the mutated profile. A
future caller that wired `dst.System = in.System` would silently defeat the protection with no store-level
backstop.

**Remediation.** Add a one-line defense-in-depth assertion in the store's identity-preservation block
(`roster.go:851-852`): `p.System = snapshot.System` so the immutable flag is enforced at the store, not only
by convention in each caller.

---

## Verified-clean controls (negative results)

**Control plane (`kernel/controlplane/server.go`).** `tokenIsPrimary` (`:325`) uses
`subtle.ConstantTimeCompare` and rejects empty presented/server tokens (`:332-335`). Non-primary requests must
name a tenant AND present that tenant's token via `tenants.Authorize` (constant-time), and are restricted to
an allowlist (`tenantTokenAllows`) with the tenant arg pinned (`:485-504`). Pre-auth request size bounded
(`maxRequestBytes`, `:343`). Runtime files written 0600 (`:414-419`). `CmdDecide` (approval) is **not** in the
tenant allowlist → primary-token only. No command-dispatch path skips the auth gate.

**Web UI (`kernel/webui/webui.go`, `session.go`).** `auth = secure + authorized`; `secure` runs host
allowlist + same-origin mutation guard on every route (`:1022-1035`). `authorized` (`:1201-1210`): token-only
when no password; password mode = token OR session (or AND in strict). `tokenMatch` constant-time
(`:1216-1218`). Session id = 32 CSPRNG bytes (`session.go:62-67`); cookie `HttpOnly`+`SameSite=Strict`+`Secure`
(proxy-aware via `X-Forwarded-Proto`, which can only *add* Secure); 12h sliding TTL; revoked on logout. Login
constant-time compare + lockout after 8 fails / 5-min cooldown (`session.go:106-129,200-204`). The only
token-free routes (`/`, `/api/authmeta|login|logout`, `/assets/`, `/favicon.ico`, `/hooks/`, `/oauth/callback`)
are deliberate and either data-free or secret-authed.

**REST/OpenAI (`kernel/restapi/restapi.go`).** Bearer token, constant-time, empty fails closed (`:318-324`).
Tenant token authorizes only its own tenant and only when `X-Agezt-Tenant` targets it (`:276-293`).
Mailbox + self-update are `adminAuth` (admin token only) — closes the documented cross-tenant mailbox IDOR
(`:188-197`). Run-by-correlation reads go through the tenant-bound engine (`bind()` resolves the engine from
the same `X-Agezt-Tenant` header, `handleRunByID:558-563`), so a tenant token cannot read another tenant's
run events — no IDOR. `/healthz`/`/readyz` unauth by design (liveness only, no data); `/metrics` token-gated.

**agentgw JWT (`kernel/agentgw/token.go`, `secret.go`, `gateway.go`).** `ValidateToken` pins `alg=HS256` +
`typ=JWT` before trusting the signature (`token.go:113-115`) → no alg-confusion / `alg=none`. Signature
verified with `hmac.Equal` (constant-time, `:124`). `iss`/`aud` pinned to `agezt-agentgw` (`:143-145`).
`exp` enforced (`:148-150`). Signing secret is per-install: `$AGEZT_AGENTGW_TOKEN_SECRET` → persisted 0600
CSPRNG file → process-lifetime CSPRNG fallback (`secret.go:40-86`); the former hardcoded
`"change-me-in-production"` literal is gone (guarded by `security_test.go:146`). Every route except `/health`
is behind `withAuth` (`gateway.go:120-151`); `/v1/token/create` is **behind** `withAuth` and mints only a
*subset* of the parent's caps (`CapsSubset` rejects escalation, `:392-396`), clamped expiry/rate that never
outlive the parent (`:402-419`) — the historical unauth-mint + cap-escalation hole is closed.

**HITL approval (`kernel/approval/approval.go`).** Only the control-plane `handleDecide` (primary-token-gated)
resolves approvals, hardcoding `resolvedBy="operator"`; no agent-facing tool reaches `Resolve`. Request IDs are
server-minted ULIDs. The 5-minute default times out to `DecisionTimeout`, which the policy hook treats as
`Allow=false` — fails **closed**, never auto-approves.
