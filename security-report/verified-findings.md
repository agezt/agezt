# AGEZT — Verified Security Findings

_Phase 3 output. Raw per-skill results in `*-results.md` were deduplicated, reachability-checked, and confidence-scored. Findings rated **Info** are documented for completeness but require no action._

Scan date: 2026-06-23 · Branch: `main` · Scope: full repository (1181 Go files, 323 TS/TSX files)

> **Design context honored during verification:** AGEZT intentionally ships a **default-allow capability posture** — agents and `code_exec`/`shell` are deliberately max-capability (network-on, allow-by-default) by owner decision. That posture is **not** treated as a vulnerability. Findings below concern the boundaries that *do* matter: the admin/control-plane surface, console auth, the agent-gateway token, the vault, the SSRF netguard, capability **confusion/bypass** (a tool doing more than its contract promises with weaker gating), CI/CD, and remote/tunnel exposure.

---

## Confirmed findings

| # | Severity | Title | CWE | Location | Confidence |
|---|----------|-------|-----|----------|------------|
| F1 | **High** | `acp_agent` tool runs arbitrary host commands via shell, outside the warden | CWE-78, CWE-441 | `plugins/tools/acpagent/acpagent.go:131,233`; `kernel/acpcatalog/acpcatalog.go:268` | High (verified by direct read) |
| F2 | **Medium** | Session cookie `Secure` flag derived from `r.TLS` — dropped behind a TLS-terminating proxy | CWE-614, CWE-311 | `kernel/webui/session.go:211` | High |
| F3 | **Medium** | CI: third-party GitHub Actions pinned to mutable tags (not SHA) on persistent self-hosted runners | CWE-1357, CWE-829 | `.github/workflows/ci.yml`, `publish-sdks.yml` | High |
| F4 | **Medium** | CI: build tooling installed via `curl\|tar` + `go install …@latest` without integrity verification, on self-hosted runners | CWE-494 | `.github/workflows/ci.yml` | High |
| F5 | **Low** | Gateway `config.write` has no per-key ownership check | CWE-862 | `kernel/agentgw/config_handler.go:177` | Medium |
| F6 | **Low** | Self-update binary download uses a plain `http.Client` (bypasses netguard) | CWE-918 | `kernel/update/update.go` | Medium (operator + signature gated) |
| F7 | **Low** | Self-hosted CI runners are non-ephemeral with shared `$HOME`/`/dev/shm`, `persist-credentials` not disabled | CWE-1393 | `.github/workflows/*` | Medium |

## Informational (no action required)

| # | Title | Location | Why it's Info, not a finding |
|---|-------|----------|------------------------------|
| I1 | `/api/logout` accepts any HTTP method | `kernel/webui` | Neutralized by `SameSite=Strict`; idempotent. |
| I2 | agentgw JWT omits `iss`/`aud` claims | `kernel/agentgw` | Single-issuer/single-audience, per-install HMAC secret, alg-pinned validation. |
| I3 | NIP-04 Nostr DMs use unauthenticated AES-CBC | nostr channel | Mandated by the NIP-04 protocol spec; random IV per message. |
| I4 | Raw `mailto:` interpolation in Data contacts | `frontend/src/views/Data.tsx` | Scheme-fixed, non-executable; not attacker-controlled. |

---

## Verified-clean areas (re-confirmed, not vulnerable)

These were actively hunted and found sound — recorded so future scans don't re-litigate them:

- **Supply chain:** No `replace` directives, no forked/git/tarball sources, no typosquats. Go leans on stdlib (no 3rd-party crypto/JWT/router/zip libs). Only watch-item: `emersion/go-imap/v2` is a beta that parses untrusted mail. Frontend overrides `undici@^7` to clear a transitive advisory.
- **SSRF:** A dialer-level netguard (DNS-rebind + redirect-hop safe) covers all agent/channel/user-reachable outbound sinks (http/fetch/web_search/browser tools, market sync, catalog discovery, channel OAuth exchange, webhook dispatch, media fetches). Operator-pinned hosts correctly skip it. MCP-remote loopback/private allowance is by design.
- **Path traversal / zip-slip:** `file` tool is symlink/TOCTOU-hardened; codeexec & sandbox confine under root; artifacts are content-addressed; the only archive extraction (backup restore) is zip-slip-safe and operator-run. No static file handler (webui is `go:embed`-ded).
- **Crypto / vault / secrets:** AES-256-GCM with fresh random salt+nonce per save; genuine PBKDF2-HMAC-SHA256 @ 200k iters; KDF-downgrade blocked (100k floor); `0600` perms; all secret/token/password/signature compares constant-time; `crypto/rand` for all security material. **No hardcoded secrets** — the historical agentgw `change-me-in-production` secret is confirmed removed (per-install CSPRNG). Secrets are presence-only in Config Center and excluded from backups.
- **Auth / authz / CSRF:** Tenant-token allowlist is deny-by-default with pinned tenant arg + constant-time compare; all four auth surfaces fail closed on empty token; CSRF defended by `SameSite=Strict` + POST-only mutations; agent-scoped memory has no IDOR; login has lockout + constant-time path. PR #370 agent-gateway hardening (no hardcoded secret, subset-capped child mint, alg-pinned JWT) verified intact.
- **Injection / XSS:** Strict CSP (`default-src 'none'; script-src 'self'`); React AST markdown renderer with `safeHref` blocking `javascript:`; `oauthResultPage` escapes untrusted input via `htmlEscape`; no Go `text/template` for responses; datalake is file-per-JSON (no SQL); mass-assignment blocked (agent `add` forces `System=false`, `edit` uses a mutable-field allowlist excluding System/Slug/Retired/Enabled).
- **Infra:** No `pull_request_target`; all CI jobs fork-gated (`head.repo.full_name == github.repository`); `permissions: contents: read`; no `${{ github.event.* }}` interpolated into `run:` (no script injection); publish gated on trusted events. No Dockerfiles/compose/Terraform/k8s exist.
