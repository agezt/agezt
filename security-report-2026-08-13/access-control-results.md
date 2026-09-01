# AGEZT — Access-Control Domain Results (Phase 2)

**Target:** `D:/Codebox/PROJECTS/AGEZT` · **Commit:** `e0041337` (`main`)
**Skills applied:** `sc-auth`, `sc-authz`, `sc-privilege-escalation`, `sc-session`
**Scope:** authentication, authorization/capability enforcement, privilege escalation
(operator↔agent boundary), session lifecycle.

Every `file:line` below was read directly in this pass. Where the Phase-1 recon map made a
claim I could not confirm, the refutation is recorded in **§ Verified safe / refuted**.

---

## Findings by severity

| ID | Title | Sev | Conf |
|---|---|---|---|
| AC-001 | Trust ceilings are operationally inert — L1–L3 folds to Allow under both the default *and* the hardened approval mode | **Critical** | 95 |
| AC-002 | Agent permanently disables the system-guardian fleet via `overseer op=retire`/`op=pause` — bypasses the System guard *and* `AGEZT_OVERSEER_FLEET_LOCK` | **High** | 92 |
| AC-003 | `overseer op=wake` is a confused deputy: dispatches on `context.Background()`, dropping trust ceiling, injection taint and intent frame | **High** | 88 |
| AC-004 | The `config` tool lets an agent rewrite the daemon's own security posture (no field is `ReadOnly`/`Locked`) | **High** | 90 |
| AC-005 | Hardcoded default console password `"agezt"` opens 180+ mutating routes on the default-ON console | **Medium** | 90 |
| AC-006 | No session invalidation on password change; sliding 12 h TTL never expires an active session | **Medium** | 85 |
| AC-007 | `unix://` gateway socket form can never match — silently falls through to the TCP branch | **Medium** | 95 |
| AC-008 | Agent gateway: no peer-credential check, no socket ACL, unauth `/health`, no token revocation | **Low** | 85 |
| AC-009 | Console login lockout resets its own counter, yielding unlimited 8-per-5-min guessing; lockout is daemon-global | **Low** | 90 |
| AC-010 | Console token printed into the public tunnel URL when `AGEZT_WEB_PASSWORD_STRICT=on` | **Low** | 80 |

---

### AC-001 — Trust ceilings are operationally inert

**Severity:** Critical · **Confidence:** 95 · **CWE-863** (Incorrect Authorization), **CWE-1188** (Insecure Default)

**Files:**
`kernel/edict/edict.go:804-807`, `:825-853` · `cmd/agezt/main.go:3845-3859`, `:3888-3918` ·
`kernel/runtime/policy.go:187-192` · `kernel/runtime/runtime.go:1970-1971`

The trust-ceiling mechanism is the codebase's stated defence for every unattended or
lower-trust execution context. It clamps correctly:

```go
// kernel/edict/edict.go:804-807
if ceiling < lvl {
    lvl = ceiling
    ceilNote = fmt.Sprintf(" (clamped to ceiling %s)", ceiling)
}
```

…but the clamped level is then folded by `AskPolicy`, and every ceiling actually in use is
Ask-class (L1–L3):

```go
// kernel/edict/edict.go:846-853
default: // AskAllow
    return Outcome{
        Decision:   DecisionAllow,
        ...
        WouldAsk:   true,
    }
```

`AskAllow` is the default and also the fallback for a typo'd value
(`cmd/agezt/main.go:3851-3858`). **So on a stock install a ceiling of L1, L2 or L3 produces
`DecisionAllow` — identical to no ceiling at all.** Only L0 restricts.

The obvious hardening — `AGEZT_APPROVAL_MODE=prompt` — is *also* defeated. It produces
`RequiresApproval: true`, which reaches:

```go
// kernel/runtime/policy.go:187-192
if requiresApproval && autoApproveCap(ctx, string(out.Capability)) {
    verdict.Allow = true
    ...
}
```

and `autoApproveCap` consults the daemon-wide set injected into **every** run at
`kernel/runtime/runtime.go:1970-1971`, which with `AGEZT_AUTO_APPROVE_CAPS` unset is *all
known capabilities*:

```go
// cmd/agezt/main.go:3893-3898
case "", "all", "1", "true", "yes", "on":
    caps := map[string]bool{}
    for _, c := range edict.AllCapabilities() { caps[string(c)] = true }
```

**Escalation path.** Start: any agent run that carries a trust ceiling — a seeded guardian
(`plugins/builtinguardians/builtinguardians.go:215`, `TrustCeiling: defaultTrustCeiling` = `"L2"`,
applied via `kernel/runtime/runctx.go:382-386`), a standing order under the VULN-003 fail-safe
(`cmd/agezt/main.go:2771-2772`, which the comment insists "must not run uncapped by omission"),
or a resumed ticket (`cmd/agezt/main.go:3029`). End: that run executes `shell`, `code.exec`,
`mcp.install`, `file.delete` with no prompt and no denial — the exact authority the ceiling
was written to withhold. **No single-variable configuration fixes this:** the operator must set
`AGEZT_APPROVAL_MODE=deny` (which also kills legitimate L1–L3 work), or set *both*
`AGEZT_APPROVAL_MODE=prompt` **and** `AGEZT_AUTO_APPROVE_CAPS=off`.

**Why not a false positive.** This is not the documented default-allow posture. Default-allow
explains why an *unrestricted* capability is L4. Here an operator/system has explicitly applied
a restriction (a numeric ceiling below L4) and the engine returns Allow anyway. The
`AGEZT_AUTO_APPROVE_CAPS` semantics are additionally described in
`kernel/runtime/runctx.go:254-261` as "a **session-scoped operator grant** … NOT a daemon-wide
policy change" — but `runtime.go:1970` applies it daemon-wide to every run, and the second-order
effect (satisfying the prompt-injection guard, epistemic escalation and intent/regret gating at
`kernel/runtime/policy.go:152`, `:158`, `:169-180`, all of which set `requiresApproval` and then
fall through to `:187`) is not mentioned anywhere.

**Remediation.**
1. Make an explicitly-applied trust ceiling bind: when `ceiling < lvl` and the resulting level is
   Ask-class, resolve with a ceiling-specific policy (deny, or prompt regardless of `AskPolicy`)
   rather than the global `AskAllow`.
2. Scope `AutoApproveCapabilities` to the Edict Ask axis only. The prompt-injection guard,
   epistemic escalation and intent-regret gating raise `requiresApproval` for reasons unrelated to
   capability trust and must not be satisfiable by it. Move the check at `policy.go:187` above the
   guards at `:152-180` (or gate it on `out.RequiresApproval` specifically).
3. Change the empty-string case at `main.go:3893` to mean "off", or emit a boot warning that the
   HITL gate is inert.

---

### AC-002 — Agent disables the guardian fleet via `op=retire` / `op=pause`

**Severity:** High · **Confidence:** 92 · **CWE-862** (Missing Authorization)

**Files:**
`plugins/tools/overseertool/kernelsource.go:77-86`, `:341-371` ·
`kernel/runtime/runtime.go:1221-1235`, `:1240-1254`, `:1298-1303` ·
`kernel/roster/roster.go:786-806`, `:811-836`, `:853-854` ·
`plugins/builtinguardians/builtinguardians.go:234-262` · `docs/THREAT-MODEL.md:479`

`RemoveProfile` protects System guardians and says so:

```go
// kernel/runtime/runtime.go:1301-1303
if p, ok := k.roster.Get(ref); ok && p.System {
    return false, fmt.Errorf("agent %q is a protected system guardian — pause or retire it instead of removing", p.Slug)
}
```

The two operations that error message *recommends as the substitute* have no such check.
`SetProfileEnabled` (`runtime.go:1221`) and `SetProfileRetired` (`runtime.go:1240`) call straight
into `roster.SetEnabled` / `roster.SetRetired` (`roster.go:786`, `:811`), neither of which inspects
`p.System`. Nor do their `kernelSource` wrappers check `fleetLock`:

```go
// plugins/tools/overseertool/kernelsource.go:77-79
func (s *kernelSource) SetAgentEnabled(ref string, enabled bool) (roster.Profile, error) {
	return s.k.SetProfileEnabled(ref, enabled)
}
```

Contrast `EditAgent` (`:105`), `CreateAgent` (`:200`), `DeleteAgent` (`:211`) and `CloneAgent`
(`:236`), all of which open with `if s.fleetLock { return … }`.

**Escalation path.** Start: any agent holding `overseer` — `CapOversee` is `LevelAllow` by default,
and the seeded guardians all hold it. One tool call:

```
overseer op=bulk_retire agents=[guardian-health, guardian-stuck, guardian-code,
                                guardian-doctor, guardian-budget, guardian-routing]
```

reaches `BulkSetRetired` (`kernelsource.go:354-371`) → `SetProfileRetired` → `roster.SetRetired`,
which sets `Retired = true` **and** `Enabled = false` (`roster.go:825`) and persists to disk.

End state: the entire self-healing fleet is dead. The effect **survives restart** —
`reconcileExistingGuardian` (`builtinguardians.go:234-262`) re-clamps `ToolDeny`, budgets,
`TrustCeiling`, `MemoryScope` and `NoisePolicy` but never touches `Enabled`/`Retired`, and it
could not if it tried: `roster.Store.Update` explicitly restores both from the pre-mutation
snapshot (`roster.go:853-854`). Standing orders bound to those agents then refuse to fire
(`cmd/agezt/main.go:2812-2818`).

`op=pause` (`SetAgentEnabled`) is the same hole with a softer name; `op=bulk_pause` batches it.

**Why not a false positive.** (a) The code states the protection exists and the previous commit
`0cdd3799` closed exactly this class of sibling gap for `op=repair` — this is the next sibling.
(b) `docs/THREAT-MODEL.md:479` tells the operator that `AGEZT_OVERSEER_FLEET_LOCK=on` means the
overseer tool "can no longer edit, create, or delete agents (no agent→fleet-admin
self-administration)" and that "System-guardian edits or deletion are always refused regardless" —
both readings are defeated by retire, which is functionally equivalent to deletion. This is an
opt-in restriction that fails to restrict when actually configured, not a default-allow posture.

**Remediation.** Add the `p.System` refusal to `SetProfileEnabled` and `SetProfileRetired` at the
`kernelSource` layer (matching the `op=repair` placement, so the operator's console/CLI path is
unaffected), and add the `fleetLock` check to `SetAgentEnabled`, `SetAgentRetired`,
`BulkSetEnabled` and `BulkSetRetired`.

---

### AC-003 — `overseer op=wake` laundering: fresh `context.Background()` drops every per-run guard

**Severity:** High · **Confidence:** 88 · **CWE-441** (Confused Deputy), **CWE-269**

**Files:**
`plugins/tools/overseertool/kernelsource.go:389-418` (esp. `:410`) ·
`kernel/runtime/policy.go:129-135`, `:169-180` · `kernel/agent/run_tools.go:86`, `:194` ·
`kernel/runtime/subagent.go:549-562`, `:242`

```go
// plugins/tools/overseertool/kernelsource.go:410-416
ctx := kernelruntime.WithAgentProfile(context.Background(), p)
if p.MaxCostMc > 0 {
    ctx = kernelruntime.WithMaxCost(ctx, p.MaxCostMc)
}
go func() {
    _, _ = s.k.RunWith(ctx, corr, strings.TrimSpace(intent))
}()
```

The caller's context is discarded entirely. Everything the policy hook reads from context is
therefore absent in the woken run:

- **Prompt-injection taint** — `agent.UntrustedObservationTaintFromContext(ctx)`
  (`policy.go:129`) is per-agent-loop state (`run_tools.go:86`, attached to the policy context at
  `:194`). A new loop starts clean, so the guard at `policy.go:169-180` cannot fire.
- **Trust ceiling** — the caller's ceiling is lost; only the *target's* profile ceiling applies,
  and `WithTrustCeiling` is tighten-only, so the laundering can only loosen.
- **Intent frame / regret gating** (`policy.go:136-164`) and any operator-scoped grants.

**Escalation path.** Start: a low-trust agent whose run is under an active untrusted-observation
taint — e.g. it fetched a web page containing injected directives, so the prompt-injection guard
is armed for its causal window. That agent calls `overseer op=wake agent=guardian-code
intent="<attacker text>"`. `WakeAgent` performs no `System` check and no `fleetLock` check.
End: `guardian-code` — whose seeded soul instructs it to *apply fixes* using `file` and
`code_exec` and to re-forge tools (`plugins/builtinguardians/builtinguardians.go:149-159`) — runs
the attacker's text as its **run intent**, i.e. the trusted operator position, in a fresh
taint-free context, under the guardian's identity and memory scope `system/guardian-code`. The
injected instruction has been promoted from untrusted tool output to trusted intent, which is
precisely the transition the guard exists to block.

**Why not a false positive.** The delegation path deliberately does the opposite:
`prepareSubAgent` derives the child context from the caller (`subagent.go:549-562`) and the async
spawn preserves the values explicitly — `context.WithCancel(context.WithoutCancel(p.childCtx))`
(`:242`) with an in-code note that this "keeps the run-stamped values … while dropping the
tool-call deadline/cancel". So context propagation is the established pattern and `WakeAgent`
is the outlier. The `System`/`fleetLock` omissions are independently verified (see AC-002).

**Remediation.** Derive the wake context from the calling run's context via
`context.WithoutCancel(ctx)` (threading `ctx` into `WakeAgent`, which currently ignores it), so
taint, ceiling and intent frame ride along; add the `p.System` and `fleetLock` refusals.

---

### AC-004 — `config` tool rewrites the daemon's security posture

**Severity:** High · **Confidence:** 90 · **CWE-269** (Improper Privilege Management)

**Files:**
`plugins/tools/config/config.go:44-53`, `:192-287` · `cmd/agezt/main.go:209`, `:3789-3830`,
`:219` · `kernel/settings/schema.go:99`, `:456-469`, `:585-599`

`op=set` is gated on `CapConfigWrite` (`config.go:50-52`) — `LevelAllow` by default — and the
only constraint on *which* setting is written is registry membership plus the `ReadOnly` /
`Locked` flags:

```go
// plugins/tools/config/config.go:201-214
field, ok := t.registry().FieldByEnv(name)
if !ok { return errf("unknown setting %q", name), nil }
if field.ReadOnly { return errf("%s is read-only and cannot be changed", name), nil }
```

Across **203** registered `AGEZT_*` fields in `kernel/settings/schema.go`, exactly **one** is
`ReadOnly` (`AGEZT_CHATGPT_OAUTH`, `:99`) and none are `Locked`. The security-relevant fields are
all freely writable:

| Field | Line | Effect of an agent write |
|---|---|---|
| `AGEZT_ALLOW_ALL` | `:589` | "Master permissive switch" — sets `UnknownAllow`, opens network tools |
| `AGEZT_APPROVAL_MODE` | `:585` | Downgrades `deny`/`prompt` back to `allow` |
| `AGEZT_AUTO_APPROVE_CAPS` | `:586` | Re-enables blanket auto-approval if the operator set `off` |
| `AGEZT_PROMPT_INJECTION_GUARD` | `:588` | `off` disables the guard the operator turned on |
| `AGEZT_WEB_PASSWORD` | `:459` | Changes the console password (secret → vault, also env-injected) |
| `AGEZT_WEB_PASSWORD_STRICT` | `:463` | Drops token-AND-password back to token-OR-password |
| `AGEZT_AGENTGW_SOCKET` | `:467` | `0.0.0.0:9000` → plaintext agent gateway on all interfaces |
| `AGEZT_FILE_ROOT` | `:597` | Repoints the File Manager root at an arbitrary directory |
| `AGEZT_TUNNEL` | `:541` | Publishes the console to the public internet on next boot |

Persistence closes the loop: the write lands in `settings.NewStore(baseDir).Save()`
(`config.go:264-277`), and at boot `injectConfig` (`cmd/agezt/main.go:3789-3830`, called at
`:209`) `os.Setenv`s every stored value **before** `daemonconfig.Load` reads them (`:219`).

**Escalation path.** Agent (has `config`, default L4) → `config op=set scope=global
name=AGEZT_PROMPT_INJECTION_GUARD value=off` and `name=AGEZT_ALLOW_ALL value=1` → next daemon
restart the operator's configured guards are gone and every unknown capability is L4. Add
`AGEZT_TUNNEL=cloudflare` plus a chosen `AGEZT_WEB_PASSWORD` and the agent has published a
remote control plane whose password only it knows.

**Mitigating factor (verified, limits severity to High not Critical).** `injectConfig` only fills
gaps — `if val != "" && os.Getenv(name) == ""` (`main.go:3810`). A guard pinned in the *real*
process environment (systemd unit, `.env` sourced by the shell) wins and cannot be overridden.
However, the product's own Config Center / console `POST /api/config/set` writes to that **same
store**, so any setting the operator configured through the shipped UI is agent-overwritable.

**Remediation.** Mark the posture-governing fields `ReadOnly` (or introduce an
`OperatorOnly` flag the `config` tool refuses while the console/CLI honours), covering at minimum
`AGEZT_ALLOW_ALL`, `AGEZT_APPROVAL_MODE`, `AGEZT_AUTO_APPROVE_CAPS`,
`AGEZT_PROMPT_INJECTION_GUARD`, `AGEZT_OVERSEER_FLEET_LOCK`, `AGEZT_WEB_PASSWORD*`,
`AGEZT_AGENTGW_*`, `AGEZT_TUNNEL*`, `AGEZT_FILE_ROOT`, `AGEZT_VAULT_*`.

---

### AC-005 — Hardcoded default console password `"agezt"`

**Severity:** Medium · **Confidence:** 90 · **CWE-798** (Hardcoded Credentials), **CWE-1392** (Default Credentials)

**Files:** `cmd/agezt/httpsurfaces.go:230`, `:232-244`, `:76-84`, `:599-614` ·
`kernel/webui/webui.go:1443-1452`

```go
// cmd/agezt/httpsurfaces.go:230
const defaultLoopbackWebPassword = "agezt"
```

`effectiveWebPassword` (`:232-244`) returns it whenever `AGEZT_WEB_PASSWORD` is unset,
`AGEZT_WEB_PASSWORD_DEFAULT` is not an explicit off-keyword, **and** the bind address is
loopback (`isLoopback`, `:599-614`). The console is ON by default at `127.0.0.1:8787`
(`:78-84`). Nothing forces a change.

Because strict mode is off for a loopback bind, the password is a *complete alternative
credential*, not a second factor:

```go
// kernel/webui/webui.go:1448-1451
if s.passwordStrictOn() {
    return s.dataTokenPresented(r) && s.sessionValid(r)
}
return s.dataTokenPresented(r) || s.sessionValid(r)
```

A session minted with `"agezt"` opens all 180+ mutating routes including `POST /api/run`
(arbitrary governed agent execution), `POST /api/config/set`, `POST /api/files/delete` and
`POST /api/toolbox/install`.

**Scoping (why Medium, not Critical).** Reachability is genuinely local-only:
`effectiveWebPassword` returns `""` for a non-loopback bind, so a LAN-exposed console has no
default password; DNS-rebinding by name is blocked (`webui.go:1340-1342` requires registered
names); and cross-site POSTs are rejected by `sameOriginMutation` (`webui.go:1350-1352`). The
realistic attacker is other local code — a second local user account, a non-AGEZT process, a
compromised local service, or any browser-adjacent local server. Under the single-operator
threat model that is a real but bounded boundary.

**Remediation.** Mint a random per-install first password and print it in the boot banner
instead of a fixed constant, or require a password change before the first mutating request
succeeds.

---

### AC-006 — No session invalidation on password change; sliding TTL never expires

**Severity:** Medium · **Confidence:** 85 · **CWE-613** (Insufficient Session Expiration)

**Files:** `kernel/webui/session.go:43-59`, `:76-92`, `:94-102`, `:131-134`

`sessionStore` exposes only `create` / `valid` / `revoke` / `lockedOut` / `noteFail` /
`noteSuccess` — **there is no bulk-revoke and no revoke-on-credential-change**. The password
source is deliberately live (`SetPasswordFn`, `:131-134`, "so a password set from the Setup
wizard / Config Center … takes effect without a daemon restart"), so an operator who changes the
console password — the standard response to a suspected compromise, and the documented fix for
AC-005 — leaves every existing session cookie valid for the full TTL.

Expiry is sliding (`:90`, `s.m[id] = time.Now().Add(sessionTTL)` on every successful check), so
a session that is polled more often than every 12 h **never expires**. The console SPA polls
continuously.

**Escalation path.** Attacker obtains a session cookie (AC-005, XSS, shared machine) → operator
notices and rotates `AGEZT_WEB_PASSWORD` in Config Center → the attacker's session is unaffected
and, because the SPA keeps it warm, persists indefinitely.

**Remediation.** Add `sessionStore.clear()` and call it whenever the effective password changes
(compare a hash of the live password on each gate decision, or hook the config write). Add an
absolute maximum session lifetime alongside the sliding idle window.

---

### AC-007 — `unix://` socket form is unreachable dead code; falls through to TCP

**Severity:** Medium · **Confidence:** 95 · **CWE-670** (Always-Incorrect Control Flow)

**File:** `kernel/agentgw/gateway.go:192-201`

```go
case len(g.sockPath) >= 7 && g.sockPath[:6] == "unix://":
```

`g.sockPath[:6]` is a 6-byte string; `"unix://"` is 7 bytes. In Go a string comparison across
different lengths is always false, so **this case can never be taken**. A value like
`unix:///run/agezt/gw.sock` also fails the next case (`:196`, requires `sockPath[0] == '/'`) and
lands in:

```go
default:
    // TCP socket
    ln, err = lc.Listen(ctx, "tcp", g.sockPath)
```

The listen errors out, so the outcome is fail-closed — the gateway is simply absent, and
`kernel/runtime/runtime.go:939-945` only `slog.Error`s. The security consequence is directional:
the one documented way to put the gateway on a *permission-checkable* filesystem socket does not
work, while `AGEZT_AGENTGW_SOCKET`'s schema help actively steers operators the other way — "set
a TCP address to reach the gateway across hosts" (`kernel/settings/schema.go:467`) — onto a
plaintext, TLS-free HTTP listener.

**Remediation.** `strings.HasPrefix(g.sockPath, "unix://")` with `g.sockPath[7:]` as the path;
add a regression test for each transport branch. Separately, reject non-loopback TCP addresses
or require TLS on that branch.

---

### AC-008 — Agent gateway: no peer credentials, no socket ACL, unauth `/health`, no revocation

**Severity:** Low · **Confidence:** 85 · **CWE-306** / **CWE-613**

**Files:** `kernel/agentgw/sockopt_unix.go:12-21` · `kernel/agentgw/gateway.go:166`, `:186-202`,
`:238-267` · `kernel/runtime/runtime.go:931-945`

The listener control hook sets **only** `SO_REUSEADDR`; there is no `chmod`, no `umask`, and no
`SO_PEERCRED` / peer-credential check anywhere in the package. The default is a Linux
abstract-namespace socket (`gateway.go:87-91`), which carries no filesystem permissions at all —
any process in the network namespace can `connect()`. The gateway starts unconditionally with no
enable flag (`runtime.go:931-945`).

The bearer token is therefore the sole gate. It is checked correctly (`withAuth`,
`gateway.go:238-267`: missing token → 401, `ValidateToken` → 401, rate limit, audit, claims into
context), but there is **no revocation** — a leaked token is valid until `exp` — and
`GET /health` (`:166`) is unauthenticated.

Severity is Low because the token requirement holds and the token secret lives in a 0600 file, so
an attacker who can read it already has same-user code execution.

**Remediation.** Add `SO_PEERCRED` (Linux) / `LOCAL_PEERCRED` (BSD/macOS) verification that the
connecting peer runs as the daemon's uid; `chmod 0600` filesystem sockets; add a token
revocation list keyed on `RunID`.

---

### AC-009 — Login lockout resets its own counter; global scope

**Severity:** Low · **Confidence:** 90 · **CWE-307** (Improper Restriction of Excessive Auth Attempts)

**File:** `kernel/webui/session.go:112-121`

```go
func (s *sessionStore) noteFail() {
	s.mu.Lock()
	s.fails++
	if s.fails >= maxLoginFails {
		s.lockedUntil = time.Now().Add(loginLockout)
		s.fails = 0            // ← counter reset when the lockout is armed
	}
	s.mu.Unlock()
}
```

Resetting `fails` at arming time means the cooldown never lengthens: after each 5-minute
lockout the attacker gets a fresh 8 attempts, indefinitely — ~2,300 guesses/day against a
credential that is compared in plaintext. Conversely the counter is daemon-global (not
per-source), so any client can lock the *operator* out of their own console for 5 minutes at a
time by sending 8 bad passwords.

**Remediation.** Keep a persistent failure counter with exponential backoff, and key the lockout
on source IP with a separate, higher global ceiling.

---

### AC-010 — Console token printed into the public tunnel URL under strict mode

**Severity:** Low · **Confidence:** 80 · **CWE-532** (Information Exposure Through Log Files)

**File:** `cmd/agezt/httpsurfaces.go:373-378`, `:312-313`

```go
func tunnelPublicURL(raw string, web webUISurface, targetsWebUI bool) string {
	if targetsWebUI && web.token != "" && (!web.passwordOn || web.passwordStrict) {
		return urlWithToken(raw, web.token)
	}
	return raw
}
```

When the operator has explicitly set `AGEZT_WEB_PASSWORD_STRICT=on` (the recommended posture for
a tunnelled console, `docs/THREAT-MODEL.md:477`), the daemon writes the full-authority console
token into the **public** URL it prints to stdout at `:313`. The token is otherwise memory-only
and never written to disk (`:85-91`).

This is Low because the destination is the local daemon log, not the network — but it converts a
memory-only credential into a logged one exactly for the operators who hardened their setup.

**Note (recon claim refined):** the Phase-1 map suggested this fires for a tunnelled default
install. It does not — `web.passwordStrict` is captured at `:150` *before* `buildTunnel` runs, so
the auto-raise triggered later inside `SetAllowedHosts` (`kernel/webui/webui.go:156-158`) is not
reflected in the struct, and the default-password case takes the `return raw` branch.

**Remediation.** Print the public URL without the token; direct the operator to the local banner
URL for the token.

---

## Route auth matrix — Web console (`kernel/webui/webui.go:748-869`)

`TierPublic` receives **no authenticator wrapper** — `kernel/httpserver/router.go:109` wraps
`rt.authenticator.Middleware` only when `opts.Tier != kernelauth.TierPublic`. Every non-public
route resolves through the same `s.authorized(r)` because the WebUI overrides tier granularity
with `RequestAuthorize` (`webui.go:750-752`), so `TierUser` and `TierAdmin` are indistinguishable
on this surface.

**Eight `TierPublic` route patterns** (the recon map said seven):

| Route | Method | Tier | Gate | Mutating? | Assessment |
|---|---|---|---|---|---|
| `/` (+ SPA deep links) | GET | Public | `shellAuth` (`:1282-1294`) — token, **or** nothing when a password is configured | no | OK. Serves compiled UI only; data routes stay gated. Note: with the default password (AC-005) the shell is always credential-free. |
| `/api/authmeta` | GET | Public | none | no | Acceptable. Leaks only `password_required` + `authed` (`session.go:191-196`). Necessary for the login screen. |
| `/api/login` | POST | Public | password, constant-time, 4 KiB cap, lockout | mints session | Constant-time (`session.go:224`); lockout weak (AC-009). No fixation — the id is minted only after success. |
| `/api/logout` | POST | Public | cookie | revokes own session | OK. POST-only, revokes server-side (`session.go:280-298`). |
| `/assets/` | GET | Public | none | no | OK — hashed static bundle. |
| `/favicon.ico` | GET | Public | none | no | OK. |
| `/hooks/<workflow>` | POST | Public | per-workflow secret (header **or** `?secret=`) | **yes** — fires a governed run | Gate verified correct (see Verified safe). Query-string secret will land in proxy access logs. |
| `/oauth/callback` | GET | Public | 32-byte CSPRNG `state` | **yes** (`Mutation: true`, `:865`) — exchanges code, stores token | State is unguessable + TTL'd. Registered GET, so `sameOriginMutation` (`:1346-1349`) exempts it — correct, since providers redirect cross-origin. |

**Protected families** (all `TierUser`, all gated by `s.authorized`): `apiRoutes` (read proxies),
`readArgsRoutes` (arg-taking reads), `writeRoutes` (mutating, `:798-800`), `jsonRoutes` (mutating,
`:801-803`), plus `/api/run`, `/api/plan/run`, `/api/toolbox/install`, `/api/market/{install,
uninstall}`, `/api/rollback/apply`, `/api/files/{tree,raw,mkdir,rename,delete}`, `/api/transcribe`,
`/api/tts`, `/api/artifact/raw`, `/api/sse-token`, `/events`.

**Cross-cutting gates applied *outside* the router** (`webui.go:871-873`, `:1260-1273`), so they
also cover public routes and 401 responses: security headers, `hostAllowed`, `sameOriginMutation`.

### Other surfaces

| Surface | Public routes | Auth model | Verdict |
|---|---|---|---|
| REST API (`kernel/restapi/restapi.go:207-239`) | `/healthz`, `/readyz` | Bearer; `/api/v1/mailbox/*` + `/api/v1/update*` are `TierAdmin` | Correct — `TierAdmin` is unreachable by a tenant credential (`kernel/httpserver/auth.go:70`) |
| OpenAI-compatible (`kernel/openaiapi/openaiapi.go:200-212`) | none | Bearer, all `TierUser` | OK |
| Agent gateway (`kernel/agentgw/gateway.go:135-166`) | `GET /health` | bespoke `withAuth` HS256 bearer | See AC-007, AC-008 |
| Control plane (`kernel/controlplane/server.go:278`) | none | token in body, constant-time, 0600 file | Not re-audited here (Phase-1 scope) |

---

## Verified safe / refuted

Checks that came back clean, or where I could not substantiate a suspected issue:

1. **`agentgw handleTokenCreate` is not an escalation path** (`gateway.go:381-460`). Runs behind
   `withAuth`; requested caps are rejected (not silently dropped) when not a subset of the parent
   (`:414-418`); expiry clamped to the parent's (`:429-431`); rate/burst clamped (`:434-441`);
   `RunID` inherited so a child cannot mint into another run (`:444`). Correct.
2. **`agentgw` config endpoints are correctly separated** (`config_handler.go:187-221`). Writes
   require a distinct `CapConfigWrite` — never the read-only `CapConfigAccess` — and
   `allowed_agents`/`excluded_agents` (the per-key ACL) are explicitly refused on this surface with
   an in-code CWE-862/269 rationale. This is the pattern the rest of the codebase should follow.
3. **Workflow webhook gate is sound** (`kernel/controlplane/workflow.go:257-279`). An empty
   presented secret is refused before any lookup (`:265`), so a workflow stored with a blank secret
   cannot be triggered; the workflow must exist, be enabled, and declare a webhook trigger;
   comparison is `subtle.ConstantTimeCompare` (`:276`); all refusals are uniform so a prober cannot
   distinguish unknown-name from bad-secret from disabled.
4. **Shared authenticator fails closed** (`kernel/httpserver/auth.go:53-79`). Invalid tier → false;
   blank credential → false; `TierAdmin` can never be opened by a tenant credential (`:70`);
   `BearerToken` requires the exact case-sensitive `"Bearer "` prefix.
5. **Session fixation is not possible** (`kernel/webui/session.go:201-245`). No session exists
   before authentication; the id is minted only after a successful constant-time password compare;
   logout revokes server-side and clears the cookie. Cookie carries `HttpOnly`,
   `SameSite=Strict`, and `Secure` derived from `r.TLS` or a forwarded-proto hint whose
   trust-without-allowlist reasoning (`:255-260` — the header can only *add* `Secure`) is sound.
6. **Constant-time comparison is used for every credential**: console password
   (`session.go:224`), bearer tokens (`kernel/auth/token.go:68-73`), tenant tokens
   (`kernel/tenant/tenant.go:214-222`), webhook secrets (`controlplane/workflow.go:276`), gateway
   HMAC (`kernel/agentgw/token.go:121`). **No `==` on a secret was found on any auth path.**
7. **OAuth `state` handling is adequate.** Channel flow: 32 bytes from `crypto/rand`,
   base64url (`controlplane/channel_oauth.go:96-102`), TTL-swept (`:88-93`), unknown state
   rejected (`:195-198`). Provider flow: one-shot listener on `127.0.0.1:1455`, TTL auto-expiry
   (`provider_oauth.go:82-91`), state compared at `:109`. The provider comparison is `!=` rather
   than constant-time, but against a single-use, TTL-bounded 32-byte CSPRNG value with one attempt
   per flow this is not practically exploitable — noted, not filed.
8. **`overseer op=edit` / `op=create` / `op=clone` / `op=delete` are properly guarded.**
   `EditAgent` refuses `System` (`kernelsource.go:112-114`) and honours `fleetLock` (`:105`);
   `CreateAgent` forces `System = false` (`:203`); `CloneAgent` never copies the flag (`:245`);
   `RemoveProfile` refuses `System` (`kernel/runtime/runtime.go:1301`). `op=repair` was closed by
   commit `0cdd3799` and the guard refuses **before** dispatch (`overseertool/tool.go:341-345`),
   which its test asserts. AC-002 is the remaining sibling.
9. **Recon claim refuted — scheduled runs are *not* uncapped when an agent profile is attached.**
   The map stated cadence sets `WithMaxCost` but never `WithTrustCeiling` (`main.go:3327-3341`).
   True literally, but `WithAgentProfile` applies the profile's ceiling itself
   (`kernel/runtime/runctx.go:382-386`), so a scheduled guardian firing *is* clamped to its
   declared L2. That clamp is nevertheless inert for the reason in AC-001 — the defect is the
   fold, not the plumbing.
10. **Recon claim refuted — the tunnel does not leak the console token on a default install.**
    See AC-010's note; the leak requires an explicit `AGEZT_WEB_PASSWORD_STRICT=on`.
11. **`roster.Store.Update` protects identity and lifecycle** (`kernel/roster/roster.go:851-854`):
    `ID`, `Slug`, `CreatedMS`, `Enabled`, `Retired`, `RetiredMS`, `RetiredReason` are restored
    from the snapshot regardless of what the mutator does, so no `UpdateProfile` caller
    (including the `config` tool's agent-scope path) can rename or resurrect an agent.
12. **`/api/run` does not expose the injection-guard downgrade to the browser.** The body
    allowlist is `{intent, model, history, system, agent, execution_profile, auto_approve_caps}`
    (`webui.go:1079`) — `prompt_injection_trust` (`controlplane/server.go:1379-1381`) is
    reachable only from the CLI/control plane. Correct split.
13. **CSRF/Host/Origin are applied outside the router** (`webui.go:871-873`), so they cover public
    routes and 401 responses. `sameOriginMutation` (`:1345-1362`) rejects
    `Sec-Fetch-Site: cross-site` and mismatched `Origin`; a *missing* `Origin` passes, which is
    safe against browsers (they always send it on cross-origin POST) and irrelevant to non-browser
    clients, which still need the token.
14. **No classic IDOR surface.** Single-operator, no per-user resource ownership model; the
    tenant partition that does exist is enforced constant-time at
    `kernel/tenant/tenant.go:214-222` with `TierAdmin` correctly unreachable.
15. **No weak password hashing, no MD5/SHA1 on a credential path, no default admin account seed.**
    The console password is a plaintext shared secret compared in constant time — acceptable for a
    single-operator local secret, and the code says so — but it means the value lives in the
    process environment and in the config store.

---

## Appendix — reproduction notes

- AC-001 is verifiable without running the daemon:
  `kernel/runtime/ceiling_internal_test.go` already asserts the *clamp*; no test asserts the
  clamped level is **denied**. Adding `TestPolicyHook_TrustCeilingL2_UnderDefaultAskPolicy`
  asserting `verdict.Allow == false` will fail on current `main`.
- AC-002 is verifiable with a unit test against `overseertool.kernelSource` mirroring
  `TestRepairAgent_RefusesSystemGuardian` (`overseer_test.go:454`) but calling
  `op=retire` / `op=pause` on a `roster.Profile{System: true}` — both currently succeed.
- AC-007 is verifiable with `go test` on a table of `sockPath` values; the `unix://` row
  currently reaches the TCP branch.
