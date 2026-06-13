# 🔐 Security Report — AGEZT

**Scan date:** 2026-06-13 · **Pipeline:** security-check (Recon → Hunt → Verify → Report)
**Scope:** full repository (`D:/Codebox/PROJECTS/AGEZT`) · Go daemon + CLI, React/TS console & SDKs
**Method:** 6 parallel hunter agents across 30+ vuln classes + 4 language scanners; findings deduped, multi-agent-consensus weighted, and key claims code-confirmed by the orchestrator.

---

## Executive Summary

AGEZT's **mature core is well-engineered** — the credential vault (AES-256-GCM + a correct stdlib PBKDF2), the web console auth (constant-time, lockout, hardened cookies), inbound webhook HMAC, plugin BLAKE3 pinning, redaction, and the SSRF guard (`netguard`) all hold up under scrutiny. The lean-deps policy is real and CI-enforced; there is no Docker/IaC/WebSocket attack surface.

**The risk is concentrated almost entirely in one component: the recently-recovered Agent Gateway (`kernel/agentgw`, M939).** Its token system is effectively unauthenticated:

> 🚨 **Anyone who can reach the gateway socket can obtain full kernel capabilities — two independent ways.** The HMAC signing key is a public source constant (`"change-me-in-production"`), *and* the token-mint endpoint requires no auth and enforces no capability limits. On Windows the gateway runs over **TCP**, making both remotely reachable.

These two issues (C1, C2) were independently surfaced by **4 of 6** hunters — high confidence, not noise. They should be fixed before the gateway is exposed beyond loopback.

**Risk score: 8.6 / 10 (High)** — driven by two Critical auth-bypasses on a network-reachable control surface, partially mitigated by the default unix-socket binding on Linux/macOS.

### Findings by severity

| Severity | Count | Headline |
|----------|-------|----------|
| 🔴 Critical | 2 | Hardcoded gateway token secret · unauthenticated capability mint |
| 🟠 High | 7 | Token leak in tree · no cap-subset · rate-limit bypass · DoS map · update-channel RCE · CI fork-RCE · alg-confusion |
| 🟡 Medium | 9 | Dead audit trail · config-write authz · 3× SSRF · CORS · body-cap · budget TOCTOU · arg-split · redirect downgrade · FE href |
| 🟢 Low | 6 | symlink confine · CI perms/pinning · update SHA/lockfile · transitive dep · dual lockfiles |
| ℹ️ By-design | — | `code_exec`/`shell` broad caps, default-allow posture (owner policy) |

---

## 🔴 Critical

### C1 · Hardcoded HMAC token-signing secret
**CWE-321/798** · `kernel/agentgw/token.go:25`, `gateway.go:63`, `runtime/runtime.go:743`, `cmd/agt/token.go:225`
The agent-gateway bearer tokens are signed with the public constant `change-me-in-production`, wired through `DefaultGatewayConfig` with **no env/vault override**. Anyone with the (open-source) constant forges a valid token with any `RunID` and every capability — offline, no interaction. The machine-bound vault (M934) is never used to seed it.
**Fix:** seed `TokenSecret` from the encrypted vault or a per-install random secret persisted at first boot; delete the constant; fail closed when unset outside dev.

### C2 · Unauthenticated capability-minting endpoint
**CWE-306/862/269** · `kernel/agentgw/gateway.go:117`
`POST /v1/token/create` is the only data route registered without `withAuth`. It returns a fully-signed token with caller-chosen `caps` and `run_id` — no caller auth, no capability subsetting, no audit. Independent of C1: even a strong secret wouldn't help. Remotely reachable when the gateway is TCP-bound (`AGEZT_AGENTGW_SOCKET=tcp://…`, the documented Windows path).
**Fix:** require an authenticated parent token; enforce minted `caps ⊆ parent.Caps`; rate-limit + audit.

---

## 🟠 High

| ID | Title | Location | CWE | Fix |
|----|-------|----------|-----|-----|
| H1 | Live JWTs + secret-bearing scripts in working tree, not gitignored | `token.txt`, `temp_token.txt`, `decode_jwt.py`, `verify_sig.py`, `test_token.py` | 312/538 | Delete; broaden `.gitignore` (`*.txt`,`*token*`,`temp_*`); rotate after C1 |
| H2 | `CreateSubprocessToken` copies caps with no subset check | `agentgw/token.go:127` | 269 | Intersect with parent caps |
| H3 | Rate limiter self-disables ~60s after first request *(code-confirmed)* | `agentgw/types.go:155-172` | 770 | Reset `lastTick`+counter on window roll-over; add test |
| H4 | Unbounded per-`SubprocessID` rate-limit map → memory DoS | `agentgw/gateway.go:220` | 400/770 | LRU/TTL eviction |
| H5 | Self-update trusts a SHA from the same endpoint; no signature/pinning → RCE | `update/update.go:172-238,343-385` | 494/345 | Sign releases + verify pinned key; HTTPS-only |
| H6 | CI self-hosted WSL runners run untrusted fork-PR code | `.github/workflows/ci.yml` | CI trust | Approve forks; ephemeral runners |
| H7 | JWT `alg`/`typ` header never validated (alg-confusion latent) | `agentgw/token.go:82` | 347 | Pin `HS256`/`JWT` |

---

## 🟡 Medium

| ID | Title | Location | CWE |
|----|-------|----------|-----|
| M1 | Gateway audit logger is dead code (nil journal, never called) → no audit trail *(code-confirmed)* | `agentgw/audit.go`, `gateway.go:73` | 778 |
| M2 | Config **write** gated only by the **read** cap `config.access` | `agentgw/config_handler.go:185` | 862 |
| M3 | SSRF: MCP-remote / catalog-sync / Ollama-discovery fetches bypass `netguard` | `mcp/http.go`, `catalog/sync.go`,`discovery.go` | 918 |
| M4 | Wildcard CORS on agentgw SSE | `kernel/agentgw` (SSE) | 942 |
| M5 | Gateway JSON decoders have no body-size cap | `agentgw/gateway.go` handlers | 770 |
| M6 | Governor agent daily-budget TOCTOU over-spend | `governor/governor.go:540-561` | 367 |
| M7 | `credential_process` arg-splitter mishandles escapes (footgun) | `creds/aws.go splitCommandLine` | 88 |
| M8 | Self-update follows redirect without HTTPS enforcement (downgrade) | `update/update.go` | 319 |
| M9 | Frontend renders untrusted `bookmarkUrl` href without `safeHref` | `frontend/` | 79 |

## 🟢 Low
Sandbox `confineUnder` lacks `EvalSymlinks` (CWE-59) · `ci.yml` no top-level `permissions:` · `dtolnay/rust-toolchain@stable` unpinned · GitHub update source never sets SHA256 · stale update lockfile DoS · old transitive `klauspost/cpuid` · dual npm+pnpm lockfiles.

## ✅ Verified-correct (cleared)
PBKDF2 reimpl (RFC-validated) · vault AES-256-GCM · web console auth (constant-time + lockout + hardened cookies) · webhook HMAC (constant-time + replay window) · redaction chokepoint · plugin BLAKE3 pinning (fail-closed) · lean-deps (2 modules, CI-gated) · REST/OpenAI Bearer auth · no Docker/IaC/WebSocket surface. `code_exec`/`shell` broad capability is **by owner policy**, not a vuln — no sandbox-escape or secret-leak found.

---

## 🛠️ Remediation Roadmap

**Phase 1 — Stop the bleeding (before any non-loopback gateway exposure)**
1. C2: add `withAuth` to `/v1/token/create` + enforce `caps ⊆ parent`. *(smallest diff, kills the worst path)*
2. C1: replace the hardcoded secret with a vault-seeded / per-install random secret; fail closed.
3. H1: delete `token.txt`/`temp_token.txt`/debug scripts; extend `.gitignore`; rotate.

**Phase 2 — Harden the gateway (this sprint)**
4. H2 cap-subset · H3 rate-limit reset (+test) · H4 map eviction · H7 alg pinning · M1 wire audit · M2 `config.write` cap · M5 body caps.

**Phase 3 — Supply chain & update integrity**
5. H5/M8/UPD-003 signed+HTTPS-only updates · H6 CI fork-PR isolation · CICD-002/003 perms + SHA-pin actions.

**Phase 4 — Defense-in-depth**
6. M3 route all server-side fetches through `netguard` · M4 scope CORS · M6 budget check-and-reserve · M7 shlex · M9 `safeHref` · Low items.

---
*Artifacts:* `architecture.md` (recon) · `verified-findings.md` (full deduped detail + confidence scores) · per-hunter raw results `sc-*-results.md`.
*Caveat:* dynamic/runtime exploitation was not performed; findings are from static review with multi-agent cross-checking. The two Criticals and three code-confirmed items (H3, M1, plus C1/C2) are high-confidence; ⭐⭐ items merit a quick maintainer confirm.
