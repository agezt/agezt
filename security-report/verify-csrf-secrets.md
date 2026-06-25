# Verification report: Findings C, D, E

Read-only adversarial verification against actual code in `D:/Codebox/PROJECTS/AGEZT`.

---

## FINDING C — CSRF via DNS rebinding on the web console

**Claimed: MEDIUM (HIGH if tunnel-exposed). Verdict: PARTIAL / mostly REFUTED as stated. Real severity: LOW (defense-in-depth gap), not a practically reachable forged-mutation bug.**

### What the code actually does

`kernel/webui/webui.go:1064` `authorized()`:
```go
func (s *Server) authorized(r *http.Request) bool {
    pw := s.consolePassword()
    if pw == "" {
        return s.tokenPresented(r)          // token-only (default install)
    }
    if s.passwordStrict {
        return s.tokenPresented(r) && s.sessionValid(r)
    }
    return s.tokenPresented(r) || s.sessionValid(r)   // session cookie alone authorizes
}
```

The claim that, in default (non-strict) password mode, a **session cookie alone** authorizes a mutating `POST /api/*` is **CONFIRMED** (line 1072 `tokenPresented(r) || sessionValid(r)`).

The claim that there is **no CSRF token and no Host/Origin/Sec-Fetch check** anywhere in the handler chain is **CONFIRMED**. I searched the whole `kernel/webui` package for `Origin`, `Sec-Fetch`, `X-Forwarded-Host`, `r.Host`, `csrf`, `Host` allowlist — there is none. (Every "allowlist" hit in the package is an *argument-name* allowlist in `writeProxy`/`jsonProxy`, unrelated to host/origin validation.) So the only cross-site defense is the cookie's `SameSite=Strict` (`session.go:216`).

### Why the DNS-rebinding chain does **not** actually work

This is the crux, and the finding's threat model is wrong about the cookie mechanics:

- The session cookie is set with **no `Domain` attribute** (`session.go:211-219`), so it is a *host-only* cookie scoped to exactly the host the browser used when it was minted — `127.0.0.1` (or `localhost`, or the tunnel hostname). Host-only cookies are sent **only** when the request URL's host string-matches the cookie's host.
- DNS rebinding works at the **IP layer**: the attacker keeps the *document origin* as `attacker.com` but rebinds `attacker.com`'s DNS to resolve to `127.0.0.1`. The browser still treats the request as going to **`attacker.com`** for all cookie and same-site purposes — the URL host is `attacker.com`, not `127.0.0.1`. The `127.0.0.1` session cookie therefore **does not attach** (host mismatch), and even if it did, the request's site is `attacker.com` so `SameSite=Strict` would withhold it anyway.
- For the cookie to attach, the attacker would have to get a victim to mint a session against `http://attacker.com` *and* have the daemon accept that Host — but the daemon serves the same content on any Host (no vhost), and the cookie minted under `attacker.com` is useless against the `127.0.0.1`-served daemon because... it is the same server, but the victim never logged in via `attacker.com`. DNS rebinding gives the attacker's *script* network reach to `127.0.0.1`, not the victim's `127.0.0.1` cookies.

So `SameSite=Strict` is **not** "defeated by DNS rebinding" as claimed — the cookie-host scoping already breaks the chain one step earlier. DNS rebinding bypasses *network* origin (lets attacker JS reach a loopback port) but **not** cookie scoping. A rebinding attacker reaching the daemon over loopback has **no session cookie to ride** and would need the token (which they don't have). The genuine residual exposure is the classic CSRF case: a victim with a **live console session in the same browser** visiting an attacker page — and there `SameSite=Strict` does hold (the request to the daemon is cross-site, cookie withheld). Strict same-site is exactly the right and sufficient control for that.

### Preconditions and missing check

- **Precondition: a console password must be set** (`AGEZT_WEB_PASSWORD` or via Setup/Config Center). A fresh default install is **token-only** — `consolePassword() == ""` → `authorized()` returns `tokenPresented(r)` only, and there is **no cookie path at all**. So default installs have zero CSRF surface here.
- The control surface binds **loopback only** (`kernel/controlplane/server.go:256` `net.Listen("tcp", "127.0.0.1:0")`; webui likewise loopback), and the code base has explicit non-loopback-bind warnings (`status.go:159`). Tunnel exposure is an operator opt-out of the default posture.
- "Missing check": no Origin/Sec-Fetch-Site validation on mutating routes — `kernel/webui/webui.go` (`writeProxy` ~`:1254`, `jsonProxy` ~`:1307`, and the `authorized()` gate at `:1064`). Adding a `Sec-Fetch-Site: same-origin/none` check or an Origin allowlist would be belt-and-suspenders.

**Bottom line:** The missing Origin/CSRF-token check is real and worth adding as defense-in-depth, but the specific "DNS rebinding defeats SameSite=Strict" exploitation chain is **not sound** — host-only cookie scoping plus Strict same-site already block it. Real severity **LOW**, gated behind (a) operator setting a password and (b) non-strict mode; classic same-browser CSRF is already covered by `SameSite=Strict`.

---

## FINDING D — Config Center returns RatingSecret values in cleartext over the console API

**Claimed: MEDIUM. Verdict: CONFIRMED. Real severity: LOW–MEDIUM (gated behind primary-token/console auth; loopback).**

### Evidence

`kernel/controlplane/configcenter_handler.go:387-411` `entryToMap`:
```go
func entryToMap(e *configcenter.ConfigEntry) map[string]any {
    m := map[string]any{
        "key":   e.Key,
        "value": e.Value,        // <-- raw, unmasked, for EVERY rating incl. RatingSecret
        "rating": string(e.Rating),
        ...
```

- `handleConfigCenterList` (`:108-152`) calls `entryToMap` on every entry, including when `rating=secret` is explicitly requested (`:121-122`, `ListByRating(RatingSecret)`), and `handleConfigCenterGet`/`Set` use it too (`:77, :104, :144, :262`).
- The webui exposes these as `/api/configcenter/list` and `/api/configcenter/get` (`kernel/webui/webui.go:219-220`), proxied to the control plane under the webui's primary token; the SPA renders the returned `value`.

**This contradicts every other code path, which deliberately redacts secrets:**
- `kernel/configcenter/audit.go:59` — audit log writes `"REDACTED"` for `RatingSecret`.
- `kernel/configcenter/types.go:310` `ListSecrets` exists separately, and `Search` (`types.go:309-312`) **skips** `RatingSecret` entirely.
- `kernel/configcenter/config.go:50` policy default for `RatingSecret` is `PolicyDeny` for *agent* access — but that gate is the agent access path, **not** the operator console list/get path, which bypasses it via `entryToMap`.

### What actually gets `RatingSecret`

The auto-classifier (`kernel/configcenter/classifier.go:50-92`) tags as `RatingSecret`: anything whose key matches `api_key|apikey|secret_key|private_key`, `password|passwd|credentials`, `token|jwt|bearer|*_token`, `aws_access|aws_secret`, `github_token|ghp_`, `slack_token`, `stripe_key|sk_live`, db passwords, encryption keys; plus values matching JWT/`AKIA…`/`ghp_…`/`sk_…`/SendGrid shapes. So **provider API keys and any secret stored as a config entry are cleartext-exposed** by list/get. (Note: primary provider keys also live in the dedicated keyring/vault per the keyring design, but any secret-shaped config entry set through Config Center is stored in the Center and leaked here.)

### Auth posture

Behind console auth only (primary token, or password session) and loopback bind — so it is an **authenticated operator-only, local** disclosure, not an anonymous one. That caps real severity at **LOW–MEDIUM**: the operator already holds the token, but (a) the SPA paints secrets in cleartext in the browser/DOM (shoulder-surf, screenshot, browser history/devtools, any XSS), and (b) it is inconsistent with the redaction everywhere else.

### Fix

In `entryToMap` (`configcenter_handler.go:387`), mask `e.Value` when `e.Rating == configcenter.RatingSecret` (e.g. emit `"masked": true` + a `previewValue`-style prefix, mirroring `audit.go:80 previewValue`), and add an explicit, separately-authorized "reveal" command if operators genuinely need the plaintext. Apply to list and get.

---

## FINDING E — `coding` and `acp_agent` spawn subprocesses with the full unscrubbed daemon environment

**Claimed: MEDIUM x2. Verdict: CONFIRMED (env leak is real). Real severity: LOW–MEDIUM, gated behind operator configuration.**

### Evidence — the leak is real

- `plugins/tools/coding/coding.go:141`:
  ```go
  agentEnv := append(os.Environ(), "AGEZT_CODING_TASK="+task)
  ...t.run(ctx, wt, agentEnv, shell, shellArg, t.Cmd)
  ```
  The spawned external coding agent inherits the **entire daemon environment** — all `AGEZT_*`, provider keys, vault token, AWS creds.
- `plugins/tools/acpagent/acpagent.go:238-244` `spawnAgent`:
  ```go
  c := exec.Command(shell, arg, cmdStr)
  if cwd != "" { c.Dir = cwd }
  c.Stderr = os.Stderr
  ```
  `c.Env` is **never set** → Go defaults to `os.Environ()`, so the ACP agent process likewise inherits the full daemon environment.

### Evidence — the correct (scrubbed) pattern that these two bypass

- `plugins/tools/shell/env.go:25-72` `scrubEnv` + `isSecretName` — allowlists PATH/system/locale, drops everything matching `KEY|TOKEN|SECRET|PASSWORD|PASSWD|CRED|AWS_|AGEZT_`. Used at `shell/shell.go:172`.
- `plugins/tools/codeexec/runtimes.go:114-152` — identical `scrubEnv`; used at `codeexec/codeexec.go:276`, `packages.go:60`. Comment: *"load-bearing safety property of the whole tool."*
- MCP/warden default a nil `Spec.Env` to an empty environment for the same reason.

So `coding` and `acp_agent` are the **two tools that diverge** from an otherwise-consistent secret-scrub posture.

### Reachability / preconditions (why not full MEDIUM)

- Both tools are **inert until the operator configures an external agent command**: `coding` returns "coding agent not configured (set AGEZT_CODING_CMD)" when `t.Cmd == ""` (`coding.go:106-108`); `acp_agent`'s `cmdStr` comes only from `AGEZT_ACP_AGENT_CMD` or a trusted `acpcatalog` slug (`acpagent.go:232-237`). They are not enabled on a default install.
- The spawned command itself is **operator-trusted** (not model-chosen) — `acpagent.go` documents that the `agent` selector is resolved slug-only via `acpcatalog.ResolveCommand` to prevent CWE-78 injection, and `coding` runs the operator's `AGEZT_CODING_CMD`. So the model cannot inject an arbitrary command.
- **However**, the *agent (LLM) IS the caller* of these tools by default-allow posture, and the `task` parameter is fully model-/prompt-injection-controlled free text ("The complete, self-contained coding instruction. The agent sees only this and the repository contents.", `coding.go:77`). A prompt-injected agent that calls `coding`/`acp_agent` hands the **operator-configured external agent a process pre-loaded with every daemon secret in its environment** — that external agent (e.g. an off-the-shelf coding CLI with network access) can read and exfiltrate them. The leak is to the *spawned external agent's* process, which is exactly the untrusted-execution surface the other tools scrub against.

### Fix

Set `c.Env` to a scrubbed allowlist in both spots, mirroring `shell/env.go`:
- `coding.go:141`: replace `append(os.Environ(), …)` with `append(scrubEnv(wt), "AGEZT_CODING_TASK="+task)` (and keep `AGEZT_CODING_TASK`, which is non-secret task text — though note the existing `isSecretName` drops `AGEZT_*`, so pass it explicitly after scrubbing).
- `acpagent.go:238` `spawnAgent`: set `c.Env = scrubEnv(cwd)` instead of inheriting.
Factor the shared `scrubEnv`/`isSecretName` into a common helper so all four tools (shell, codeexec, coding, acpagent) use one implementation.

**Real severity: LOW–MEDIUM.** The env leak is genuine and the inconsistency with the documented secret-scrub posture is a real defect, but it is gated behind the operator explicitly wiring an external agent command, and the spawned command is operator-trusted (so no command injection). The exposure is "a prompt-injected agent run can hand all daemon secrets to the operator's configured external coding/ACP agent process."

---

## Summary

| Finding | Claimed | Verdict | Real severity | Key precondition |
|---|---|---|---|---|
| C — CSRF/DNS-rebind | MED (HIGH tunnel) | PARTIAL (rebind chain REFUTED; missing Origin check real) | LOW | password set + non-strict; rebind doesn't ride host-only cookie |
| D — secret cleartext in Config Center | MED | CONFIRMED | LOW–MED | authenticated console (token/password), loopback |
| E — unscrubbed env in coding/acp_agent | MED x2 | CONFIRMED (env leak real) | LOW–MED | operator-configured external agent cmd; model-callable + injectable `task` |

Key file:line anchors:
- C: `kernel/webui/webui.go:1064` (`authorized`), `:1072` (session-alone), `kernel/webui/session.go:211-219` (host-only cookie, SameSite=Strict); no Origin/Host check anywhere in `kernel/webui`.
- D: `kernel/controlplane/configcenter_handler.go:390` (`"value": e.Value` unmasked), list `:142-144`, vs redaction `kernel/configcenter/audit.go:59`, `types.go:310`.
- E: `plugins/tools/coding/coding.go:141`, `plugins/tools/acpagent/acpagent.go:238-244`; correct scrub at `plugins/tools/shell/env.go:25`, `plugins/tools/codeexec/runtimes.go:120`.
