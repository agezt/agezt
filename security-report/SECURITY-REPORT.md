# AGEZT Security Report

**Date:** 2026-06-24 · **Repository:** `D:/Codebox/PROJECTS/AGEZT` · **Mode:** full rescan
**Pipeline:** Recon → Hunt (10 vuln-class agents) → Verify (adversarial) → Report, plus a tool-backed pass (`gitleaks`, `govulncheck`, `gosec`, `npm audit`, `go mod verify`, `staticcheck`).

## Executive Summary

AGEZT is a Go multi-agent daemon (`agezt`) + `agt` CLI + an embedded React 19 / TS 6 / Vite 8 web console. The audit covered every network trust boundary: web console auth, control-plane routing, the REST and OpenAI-compatible APIs, the agent gateway (`agentgw`), agent code-execution/sandbox, outbound HTTP/SSRF protection, secret/vault storage, artifact rendering, channel integrations, dependencies, and CI/CD.

**The overall posture is strong** — among the better-hardened codebases of this size. The classic Go CVE cluster is absent (no web framework, no JWT lib, no `gorilla/*`, small dependency graph; `govulncheck` and `npm audit` clean). Auth uses constant-time comparison, `crypto/rand` tokens, brute-force lockout, and strong CSP/anti-framing headers. The previously-reported `agentgw` holes (hardcoded secret + unauthenticated token mint) are **verified fixed**. netguard robustly blocks SSRF (resolved-IP check at dial + each redirect hop, defeating rebinding/metadata/IPv6 tricks). The vault uses AES-256-GCM + PBKDF2-200k with fresh per-save salt/nonce.

**No Critical or High exploitable issue was confirmed.** The findings are a handful of Medium-severity items — mostly **conditional** (require non-default deployment modes) or **secret-hygiene / capability-posture** decisions rather than memory-unsafe or remotely-reachable bugs.

**Remediation update (2026-06-24):** V-001, V-002, V-003, V-006, V-007, V-008, V-009, V-010, V-011, V-013, H-001, H-002, H-003, H-005, and H-006 were remediated in the working tree. H-007 now has Dependabot tracking, and `go list` confirms `go-imap/v2` still has no stable v2 release beyond `v2.0.0-beta.8`. V-004 has workflow-lint guards for self-hosted fork-PR jobs, checkout credential persistence, Dependabot coverage, and sanitized `.env.example`; the unsafe `setup-go-safe` fallbacks were removed. V-012 has a targeted System-guardian guard plus an opt-in `AGEZT_OVERSEER_FLEET_LOCK`; V-005 is not present in the current workspace. Focused tests passed; see `verified-findings.md` for exact status.

| Severity | Count | Theme |
|---:|---:|---|
| Critical | 0 | — |
| High | 0 | — |
| Medium | 8 | Subprocess env-leak (×2), REST cross-tenant IDOR*, agent→fleet-admin escalation, local plaintext secrets, CI fork-runner posture, committed vault-probe script |
| Low | 9 | Origin-check gap, query-token leakage, Config Center secret echo, SVG XSS, SSE caps, netguard parity, CI/script footguns |
| Info | 2 | code_exec sandbox residual (by design), beta email parser |

*\*conditional on REST API + multi-tenant mode being enabled (both off by default).*

## Top Findings (full detail in `verified-findings.md`)

1. **Medium — `coding` & `acp_agent` tools leak the full daemon environment** (`plugins/tools/coding/coding.go:141`, `plugins/tools/acpagent/acpagent.go:238`). Both spawn an external agent with `os.Environ()` — handing every provider key / vault cred / AWS secret to a prompt-steerable child. Every *other* exec path (`code_exec`, `shell`, `mcp`) correctly scrubs via an allowlist `scrubEnv`; these two don't. **Fix:** reuse `scrubEnv`. *(Highest-value fix — a one-helper change closes both.)*

2. **Medium (conditional) — REST API tenant token had no per-route restriction → cross-tenant mailbox/board IDOR + sender spoofing** (`kernel/restapi/restapi.go:272-289`). The control plane allowlists tenant-token commands (excludes `board_*`); the REST mux previously applied no equivalent restriction, so a tenant token could reach daemon-global mailbox/update surfaces. **Status:** remediated by admin-token-only gating for daemon-global REST routes; tenant-scoped routes remain tenant-token accessible.

3. **Medium — `overseer` tool enables agent → fleet-admin escalation under default-allow posture** (`kernel/runtime/runtime.go:1196` `UpdateProfile` has no System check; `CapOversee` is `LevelAllow`). The specific System-guardian defang vector is now blocked, and `AGEZT_OVERSEER_FLEET_LOCK=on` disables agent-reachable `op=edit/create`. **Remaining posture decision:** enable the lock for stricter deployments, or keep default-allow as an accepted owner policy.

4. **Medium — Plaintext secrets in ignored local files** (`.env:4,7` real-shaped `MINIMAX_API_KEY`/`DEEPSEEK_API_KEY`, a literal `AGEZT_VAULT_PASSPHRASE` value, commented `sk-…` keys at world-readable perms; `.playwright-mcp/…yml:627` JWT+AWS token). Not git-tracked, but live local plaintext leaks via backups/bundles/copied worktrees. **Fix:** rotate if live, delete stale snapshots, move the passphrase to an OS secret store, `chmod 600 .env`.

5. **Medium — CI self-hosted runners rely on a fragile per-job fork guard** (`.github/workflows/ci.yml`). Persistent WSL runners on `pull_request`; one unguarded future job runs untrusted fork code on a long-lived host. **Status:** workflow-lint guards now cover same-repo fork guards and checkout `persist-credentials:false`; `setup-go-safe` no longer has shared-runner fallbacks. Ephemeral runners + fork-PR approval remain owner/infra action.

6. **Medium — `list_vault.py` vault-probe scratch script found during scan** at repo root. **Status:** not present in the current workspace and not tracked by Git.

7. **Low–Medium — Console mutating routes lack an Origin/Host check** (`kernel/webui/webui.go:1064`). In default password mode a session cookie alone authorizes POSTs and there's no Origin/`Sec-Fetch-Site` check. *The originally-feared DNS-rebinding chain was adversarially **refuted*** (host-only loopback cookie + `SameSite=Strict` prevent the cookie from attaching); the real residual is a defense-in-depth gap that matters if the console is tunnel-exposed. **Fix:** Host allowlist + Origin check; default STRICT when a password is set.

8. **Low–Medium — Config Center echoes `RatingSecret` values in cleartext** over `/api/configcenter/list|get` (`configcenter_handler.go:387`), bypassing the masking used everywhere else. Behind console auth; provider keys are *not* here (they're in the vault). **Fix:** mask when `Rating == RatingSecret`.

Plus Lows: broad `?token=` query-string auth (`webui`/`restapi`/`openaiapi`), SVG artifact stored-XSS (`artifact_route.go:55`), missing SSE caps, Discord attachment URL policy, voice/embed netguard parity, agentgw memory-search clamping, root-script hygiene, and dependency tracking are remediated or tracked. See `verified-findings.md`.

## Fixes Applied This Session

| Finding | Status | Change |
|---|---|---|
| V-001 / V-002 — subprocess env-leak | **Fixed** (parallel session) | New `kernel/envscrub` package; `coding`/`acp_agent` scrub the child env |
| V-013 — Config Center secret cleartext | **Fixed** | `entryToMap` masks `RatingSecret` via `creds.MaskValue` + `"masked":true`; regression test added |
| H-005 — SVG artifact stored-XSS | **Fixed** | SVG served with `Content-Security-Policy: sandbox` (direct-nav scripts blocked; `<img>` still renders) |
| H-006 — voice/embed bypass netguard | **Fixed** | Both adapters use a netguard client (`AllowLoopback`+`AllowPrivate`, so local inference still works; metadata blocked) |
| V-006 — Web UI Host/Origin checks | **Fixed** | Host allowlist + same-origin mutation checks (`Origin` / `Sec-Fetch-Site`) |
| V-008 — broad query-token auth | **Fixed except SSE/bootstrap** | Web data routes use Bearer/session; REST/OpenAI require Bearer; `/events` keeps query token by design |
| V-010 — memory-search limit | **Fixed** | `limit` is parsed explicitly and clamped to `[1,200]` |
| V-011 — REST cross-tenant mailbox IDOR | **Fixed** | New `adminAuth` gate: daemon-global mailbox/board + host-global update routes accept the admin token only (per-tenant tokens 401); tenant-scoped runs unaffected. Regression test `TestAdminOnlyRoutes_RejectTenantToken`. No-op in single-tenant default |
| V-012 — `overseer` agent→guardian defang subcase | **Mitigated (targeted guard)** | `overseertool` `EditAgent` now refuses to edit a `System`-protected guardian (mirrors `RemoveProfile`); operator admin path unaffected |
| V-012 (broader) — agent→fleet-admin escalation | **Opt-in gate added** | New `AGEZT_OVERSEER_FLEET_LOCK` (default **off** → default-allow preserved). When set, the agent-reachable `EditAgent`/`CreateAgent` refuse, so an agent can't self-administer the fleet via the `overseer` tool; operator control-plane edits + auto-repair (RepairAgent/routing) are unaffected. Tests `TestFleetLock_RefusesAgentEditAndCreate`, `TestFleetLockEnabled_ParsesEnv`; registered in `configEnvVars` |
| V-003 — plaintext secrets in local files | **Fixed locally** | Removed the gitignored `.playwright-mcp/` console captures, blanked the secret-shaped `.env` values, narrowed `.env` NTFS ACL to current user + SYSTEM/Administrators, and added a tracked sanitized `.env.example` template. `gitleaks` now reports no leaks. If the removed values were live, rotate/revoke them at the providers |
| V-005 — `list_vault.py` committed | **Absent** | File is not present in the current workspace and is not tracked by Git |
| H-002/H-003 — root debug/launcher scripts | **Absent + ignored** | No matching root scripts are present; `.gitignore` now blocks the previously reported local debug/launcher names from being committed |
| H-007 — beta email parser tracking | **Tracked** | Added `.github/dependabot.yml` for Go modules, frontend npm, TypeScript SDK npm, and GitHub Actions. `go list -m -versions github.com/emersion/go-imap/v2` shows `v2.0.0-beta.8` remains latest |
| H-001 — Discord attachment fetch SSRF | **Fixed** | `fetchAttachmentDataURL` now validates `att.URL` is `https` on a Discord-CDN host (`*.discordapp.com`/`.net`) before dialing; rejects foreign/internal/metadata hosts. Regression test `TestValidDiscordAttachmentURL` |
| V-009 — no per-client SSE connection cap | **Fixed** | New reusable `kernel/streamlimit` package (per-key concurrent-stream limiter, idempotent release, fully unit-tested) wired into the long-lived webui `/events` firehose and the REST mailbox `/watch` stream: a client over a generous 64-stream/IP cap gets `429` + `Retry-After`. Request-bounded proxy/run streams left as-is (already timeout-capped). Test `TestSSEGate_OverCapReturns429` |
| V-004 — fragile CI fork-runner guard | **Guards added** (infra still owner) | `internal/ciguard` fails tests if any self-hosted `pull_request` job lacks the same-repo fork guard, if any workflow checkout omits `persist-credentials:false`, if Dependabot stops covering the core ecosystems, or if `.env.example` is missing/secret-valued/ignored. The durable fix (ephemeral runners + fork-PR approval policy) remains an owner/infra action |
| V-007 — `setup-go-safe` toolcache footgun | **Fixed** | Removed hardcoded/shared fallbacks for `RUNNER_TOOL_CACHE` and `RUNNER_NAME`; the action now fails fast if the self-hosted runner environment is malformed. Covered by `internal/ciguard` regression test |

Current remediation smoke test passes (`controlplane`, `webui`, `restapi`, `openaiapi`, `agentgw`, `streamlimit`, `ciguard`, `envscrub`, `coding`, `acpagent`, `overseertool`, `embed`, `voice`, `discord`).

**Still open — owner action only (no remaining code fixes):** flip `AGEZT_OVERSEER_FLEET_LOCK=on` if you want the (now-built) agent→fleet-admin gate active; V-004 move to ephemeral CI runners + fork-PR approval policy (the lint guard is in place). Also rotate/revoke any previously live `.env` keys at their providers.

## Positive Security Posture (verified)

- **AuthN/Z:** constant-time token compare (`subtle`/`hmac.Equal`), `crypto/rand` per-boot token sent in request body (not URL), brute-force lockout, fail-closed mux. agentgw JWT alg-pinned HS256 with per-install secret and non-escalatable subset/clamp — **prior holes fixed**.
- **Listeners:** loopback-only by default (web `127.0.0.1:8787`, control plane `127.0.0.1:0`, agentgw abstract socket); REST/OpenAI off unless an env addr is set; nothing binds `0.0.0.0` implicitly.
- **SSRF:** netguard checks the resolved IP at dial **and every redirect hop** — blocks loopback/RFC1918/ULA/link-local metadata/0.0.0.0/CGNAT and IPv6-embedded-IPv4 encodings; no TOCTOU window. Every agent-tainted outbound fetch routes through it.
- **Secrets:** single `creds.json` vault (0600), AES-256-GCM + PBKDF2-200k, fresh per-save salt+nonce from `crypto/rand`, machine-bound; provider keys masked to last-4 in every API echo; redaction covers the append-only journal. No `InsecureSkipVerify`, no `math/rand` in security paths.
- **Injection:** no SQL/NoSQL query language; no SSTI/XXE/LDAP/GraphQL; **no `dangerouslySetInnerHTML`**; Markdown rendered via React-escaped AST with `safeHref` scheme allowlist; header values sanitized of CR/LF.
- **Client-side:** strong CSP (`frame-ancestors`/`base-uri`/`form-action 'none'`), `X-Frame-Options: DENY`, `nosniff`, `Referrer-Policy: no-referrer`; no CORS headers at all (no cross-origin read); no inbound WebSocket surface; session cookie HttpOnly + SameSite=Strict + Secure.
- **Supply chain:** small curated dependency graph, no `replace`/vendored/git deps, lockfiles consistent, `undici` pinned via overrides; `govulncheck`, `npm audit`, `go mod verify` all clean.

## Verification Performed

Passing: `go mod verify`, `govulncheck ./...`, `go vet ./...`, `npm audit` (frontend + `sdk/typescript`, 0 vulns), `npm run typecheck`, `sdk/typescript` tests (14), Rust SDK dep inventory.
Remediation smoke tests passed:
- `go test ./kernel/controlplane ./kernel/webui ./kernel/restapi ./kernel/openaiapi ./kernel/agentgw ./kernel/envscrub ./plugins/tools/coding ./plugins/tools/acpagent ./plugins/tools/overseertool ./plugins/providers/embed ./plugins/providers/voice`
- `go test ./internal/ciguard ./kernel/streamlimit ./kernel/webui ./kernel/restapi ./plugins/channels/discord`
- `go test ./internal/ciguard`
Reviewed: `gitleaks` (now no leaks found), `gosec` (noisy; no High/Critical after manual triage), `staticcheck` (4 non-security lint items only). Two parallel pipeline runs were reconciled into the findings above. See `scanner-summary.md` + `gitleaks.json`.

## Recommended Remediation Order

1. **Rotate/revoke any previously live `.env` keys at their providers**; the local file is sanitized, but provider-side revocation is the real invalidation step.
2. **Decide the `overseer` privilege-escalation posture** (V-012) — set `AGEZT_OVERSEER_FLEET_LOCK=on` for stricter deployments, or explicitly accept the default-allow posture.
3. **Harden CI operations:** move self-hosted jobs to ephemeral runners and enforce fork-PR approval (V-004); the repo-side guard is already in place.
4. Clean up non-security staticcheck items when convenient.

## Artifact Index

- `architecture.md` — listeners, route inventory, auth model, capability surface, prioritized hunt list
- `dependency-audit.md` — supply-chain review · `scanner-summary.md` — tool command results · `gitleaks.json` — redacted secret scan
- `code-exec-results.md`, `injection-results.md`, `api-client-results.md`, `infra-results.md` — per-class hunter detail
- `verify-authz.md`, `verify-csrf-secrets.md` — adversarial verification of the top findings
- `verified-findings.md` — consolidated, verified findings + rejected/accepted triage *(read this for full detail)*

## Scope Notes

Local static + manual review plus dependency/secret scanning. No live authenticated web pentest, fuzzing, or testing against deployed infrastructure. Severities are CVSS v3.1-style qualitative ratings calibrated to verified reachability under realistic (default) deployment preconditions.
