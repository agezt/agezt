# Access-Control Verification — Findings A & B

Read-only adversarial verification against the actual code in `D:/Codebox/PROJECTS/AGEZT`.

---

## FINDING A — Cross-tenant IDOR + sender spoofing on the REST mailbox

**Verdict: CONFIRMED (structural), but severity is MEDIUM, not HIGH, due to hard preconditions.**

### What the code actually does

**REST auth has no per-command / per-route restriction.**
`kernel/restapi/restapi.go:272-289` (`authorized`) is the entire authorization
logic for every `/api/v1/*` route. It accepts a request if EITHER:
- the presented bearer equals the daemon admin token (any tenant), OR
- `s.tenantAuth(id, presented)` returns true for the `X-Agezt-Tenant` header
  (`kernel/restapi/restapi.go:283-287`), wired to `reg.Authorize`
  (`cmd/agezt/main.go:4877`).

There is **no command/route allowlist** after that point. The same
`s.auth(...)` wrapper guards `/api/v1/runs`, `/api/v1/mailbox/*`, etc.
(`restapi.go:180-193`). A valid tenant token therefore reaches every mailbox
handler.

**The mailbox is daemon-global and is NOT tenant-partitioned.**
`kernel/restapi/mailbox.go:25-26` states it outright: "The mailbox is
daemon-global (one board per daemon), so the X-Agezt-Tenant header does not
partition it." The handlers confirm it — they call `s.mailbox(w)` →
`s.board` (the one injected store, `restapi.go:107`, wired
`cmd/agezt/main.go:4775-4777`) and never consult the tenant. `handleMailboxInbox`
reads `?name=` directly (`mailbox.go:215-221`); `handleMailboxWatch` subscribes
the **primary** bus regardless of any tenant header (`mailbox.go:328-347`).

**`From` / `To` / `by` are caller-supplied and bound to nothing.**
The send body `mailboxSendRequest` (`mailbox.go:108-116`) carries `From`,
`To`, `Topic`, `ReplyTo`. Every store call passes `req.From` verbatim
(`mailbox.go:161-180`) with no comparison to the authenticated tenant.
`handleMailboxMessageSub` ack takes `req.By` verbatim (`mailbox.go:260-281`).
So a tenant-A token holder can:
- read any agent's inbox: `GET /api/v1/mailbox/inbox?name=<victim>`
- enumerate threads/replies by id: `.../messages/{id}/replies`, `messages?topic=`
- spoof `from`: `POST {"to":"researcher","from":"ceo-agent","text":...}`
- ack/clear another reader's inbox: `POST .../{id}/ack {"by":"<victim>"}`
- tail the cross-tenant firehose: `GET /api/v1/mailbox/watch` (no name/topic)
- wake standing orders as a forged sender — the send fires `boardNotify`
  (`mailbox.go:190-192`), the same `board.posted` an agent's send fires.

### The control-plane asymmetry the finding cites is real

`kernel/controlplane/server.go:485-504` is the contrast. On the control-plane
socket a non-primary (tenant) token is (a) checked against `tenants.Authorize`,
(b) **restricted to `tenantTokenAllows(req.Cmd)`** (`server.go:493`), and (c)
pinned to its own tenant arg. `tenantTokenAllows`
(`kernel/controlplane/tenant.go:68-89`) is a deny-by-default allowlist that
**does NOT contain any `board_*` command** (`CmdBoardRead/Send/Inbox/Ack/
Replies/Get`, defined `kernel/controlplane/protocol.go:1454-1512`). So over the
control plane a tenant token genuinely cannot touch the board. The REST surface
exposes the same board with none of that gating.

**Missing-check location: `kernel/restapi/restapi.go:272-289`** — `authorized()`
returns a bare bool with no per-route/per-command scoping; there is no REST
equivalent of `tenantTokenAllows`, and no binding of `From`/`name`/`by` to the
authenticated principal in `kernel/restapi/mailbox.go`.

### Why severity is MEDIUM, not HIGH — preconditions

All three must hold simultaneously, and none is on by default:

1. **REST surface OFF by default.** `buildRESTAPI` returns early unless
   `AGEZT_REST_ADDR` is set (`cmd/agezt/main.go:4755-4758`). Operator opt-in,
   loopback-recommended (`docs/THREAT-MODEL.md:411`).
2. **Multi-tenancy OFF by default.** `SetTenantAuthorizer`/`SetTenantResolver`
   are only wired when `reg != nil` (`main.go:4863-4878`), i.e. multi-tenant
   mode. With no registry, the only credential is the admin token — and an admin
   token holder is already daemon-global, so there is no privilege boundary to
   cross (no IDOR). The bug only bites when BOTH REST is enabled AND tenants are
   configured.
3. **Attacker must already hold a valid tenant token.** Tenant tokens are minted
   by the primary operator (`handleTenantCreate`/`handleTenantToken`,
   `tenant.go:91-132`), so the threat actor is a *provisioned but untrusted
   tenant* — a real model in a multi-tenant SaaS deployment, but not an anonymous
   one.

Given the preconditions are a deliberate, non-default, operator-enabled
multi-tenant deployment, this is a genuine cross-tenant isolation break worth
**MEDIUM/High-Medium** — the isolation the control plane carefully enforces is
silently absent on the REST twin. Where the deployment IS multi-tenant + REST-on,
the in-deployment impact is high (full cross-tenant mailbox read/write/spoof +
standing-order wake). The same gap exists on the OpenAI-compat surface
(`kernel/openaiapi`, same `authorized` shape) but that surface exposes no
mailbox, so the cross-tenant data is limited to runs there.

**Note for the OpenAI-compat twin:** `kernel/openaiapi/openaiapi.go:143-150`
mirrors the same tenant-authorizer wiring; its `authorized` is the same
unscoped bool, but it has no mailbox/board routes, so Finding A's
read-another-inbox / spoof-sender impact is specific to `restapi`.

---

## FINDING B — overseer `op=edit/create` rewriting a System guardian's ToolAllow / ConfigOverrides / Soul

**Verdict: CONFIRMED that the protection does NOT block behavioral edits of a
System guardian — but this is NOT a privilege ESCALATION over the admin-token
boundary, because the admin path is *equally* unprotected. PARTIAL on the
"bypasses the admin-token boundary" framing.**

### What can actually happen

1. **An agent (no admin token) can invoke `overseer`.** The tool maps to
   `CapOversee` (`kernel/edict/toolmap.go:122-123`), which ships at
   **`LevelAllow`** — `DefaultLevels()` sets every capability to `LevelAllow`
   (`kernel/edict/edict.go:606-609`; confirmed in `.project/PHASE-M850-...:67`).
   An agent with no `ToolAllow` set receives the FULL registered tool set
   (the per-agent tool policy is only built `if len(p.ToolAllow) > 0 ||
   len(p.ToolDeny) > 0`, `kernel/runtime/runtime.go:2127-2132`; `filterTools`
   is only applied when an allowlist exists). So a default agent has `overseer`
   and is allowed to call it with no HITL gate.

2. **`overseer op=edit` rewrites a guardian's Soul / ToolAllow / ToolDeny /
   ConfigOverrides / TrustCeiling wholesale, with no System check.**
   `plugins/tools/overseertool/tool.go:178-191` → `Source.EditAgent` →
   `kernelsource.go:61-94`. The mutator sets `dst.Soul`, `dst.ToolAllow`,
   `dst.ToolDeny`, `dst.ConfigOverrides`, `dst.TrustCeiling`, etc. directly from
   caller input. It calls `k.UpdateProfile` (`runtime.go:1196-1209`), which does
   **NOT** check `System` (contrast `RemoveProfile`, `runtime.go:1213-1218`,
   which explicitly refuses System). The store-level `roster.Update`
   (`kernel/roster/roster.go:840-865`) restores `ID/Slug/CreatedMS/Enabled/
   Retired` from the snapshot but **does not restore `System`** and applies no
   field allowlist beyond identity/lifecycle. `applySystemGuardianDefaults`
   (`roster.go:492-535`) enforces FLOORS only on MaxCost/MaxDaily/TrustCeiling/
   MemoryScope/NoisePolicy — it does **not** protect `Soul`, `ToolAllow`,
   `ToolDeny`, or `ConfigOverrides`.

   Concrete escalations confirmed reachable by a non-admin agent:
   - (a) **grant capabilities** — `op=edit` another agent (or, if it has no
     `ToolAllow`, itself) with a `profile.tool_allow` that adds `shell`,
     `code_exec`, etc. (TrustCeiling is also caller-set in the same mutator).
   - (b) **edit a System guardian's behavior** — rewrite a guardian's `soul`
     (neutralize its prompt), `tool_allow`/`tool_deny`, or `config_overrides`.
     The System flag stays true (it is never assigned by the mutator), so the
     guardian is still "protected" against *removal*, but it is now behaviorally
     defanged. This defeats the intent of `[[system-guardians]]` ("remove
     refused, reaper skips, no spoof").
   - (c) **create a new agent** — `op=create` (`tool.go:193-205` →
     `kernelsource.go:98-101`); `System` is forced false so it is not a
     *protected* agent, but it can be created with any `tool_allow` /
     `config_overrides` the caller wants (privileged-but-unprotected).
   - (d) **neutralize a guardian** — combination of (b): set the guardian's
     `soul` to a no-op and/or `tool_deny` its overseer/notify tools.

3. **The `op=repair` path** (`kernelsource.go:103-169`) runs a governed pass AS
   the target and auto-applies a closing profile proposal via the same
   `UpdateProfile`, with the same absence of System-field protection. It does
   gate on `AllowsDirectCall()` (managed sub-agents refused, `kernelsource.go:114`)
   but not on System.

### Why this is PARTIAL on "bypasses the admin-token boundary"

The finding's premise was that the control-plane `agents/edit` path
"field-allowlists edits and preserves System and refuses removing System," and
that overseer bypasses that. In reality the admin path is the SAME mechanism:
`handleAgentEdit` (`kernel/controlplane/roster.go:1165-1210`) calls the identical
`s.k.UpdateProfile(ref, applyAgentMutableProfileFields)`, and
`applyAgentMutableProfileFields` (`roster.go:1212-1237`) sets `Soul`,
`ToolAllow`, `ToolDeny`, `ConfigOverrides`, `TrustCeiling` wholesale with **no
System-guardian field protection** either. So:

- The admin path does NOT refuse editing a guardian's Soul/ToolAllow/
  ConfigOverrides; it only forces `System=false` on **create**
  (`roster.go:1149`) and protects identity/lifecycle at the store. There is no
  stronger "admin boundary" for the overseer tool to bypass — both paths are
  equally permissive on behavioral fields.
- Therefore the real finding is **a missing System-guardian field-protection in
  the shared edit path** (`UpdateProfile` / `roster.Update`), exposed to agents
  (not just operators) because `CapOversee` is allow-by-default and the
  `overseer` tool is in every default agent's tool set. The genuine privilege
  concern is **agent → fleet-admin** (an ordinary agent gaining
  create/edit/grant/neutralize powers with no approval), NOT
  **tenant-token → admin-token**.

The unit test confirms the gap is by-design-but-narrow: `overseer_test.go:77,85,
293-294` asserts only that the **System flag itself** cannot be set via
edit/create — it never asserts a System guardian's Soul/ToolAllow/ConfigOverrides
are protected, because they are not.

**Severity:** MEDIUM-to-HIGH depending on threat model. If any agent in the
roster can be prompt-injected or is itself untrusted, it can self-grant
capabilities and silently neutralize the self-healing guardian fleet without
tripping an approval — a meaningful autonomy/containment break. It is in tension
with `[[default-allow posture]]` (the owner deliberately ships every capability
at LevelAllow), so it may be a known/accepted posture; flag it for an owner
decision rather than assuming it is an unintended bug.

**Primary missing-check locations:**
- `kernel/runtime/runtime.go:1196-1209` (`UpdateProfile`) — no System-aware
  field protection; contrast `RemoveProfile` at `:1213-1218`.
- `kernel/roster/roster.go:840-865` (`Store.Update`) — restores identity/
  lifecycle only; no field allowlist for System guardians.
- `plugins/tools/overseertool/kernelsource.go:61-94` (`EditAgent`) — applies
  Soul/ToolAllow/ToolDeny/ConfigOverrides/TrustCeiling from caller input with no
  System check and no admin gate.
- Capability posture: `kernel/edict/edict.go:606-609` (CapOversee = LevelAllow)
  + `kernel/runtime/runtime.go:2127-2132` (empty ToolAllow ⇒ full toolset).

---

## Summary

| Finding | Verdict | Real severity | Key file:line |
|---|---|---|---|
| A — REST mailbox cross-tenant IDOR + spoof | CONFIRMED | MEDIUM (HIGH in-deployment; gated by REST-on + multi-tenant + tenant token) | `kernel/restapi/restapi.go:272-289` (no per-command scope); `kernel/restapi/mailbox.go:25-26,161-180,215-221` (global board, unbound From/name/by) |
| B — overseer edits a System guardian | CONFIRMED (no protection) / PARTIAL (not an admin-token bypass) | MEDIUM–HIGH (agent→fleet-admin; admin path equally unprotected) | `kernel/runtime/runtime.go:1196-1209`; `kernel/roster/roster.go:840-865`; `plugins/tools/overseertool/kernelsource.go:61-94`; `kernel/edict/edict.go:606-609` |

Report written to: `D:/Codebox/PROJECTS/AGEZT/security-report/verify-authz.md`
