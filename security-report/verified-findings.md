# AGEZT — Verified Findings

Date: 2026-06-24 · Scope: `D:/Codebox/PROJECTS/AGEZT` · Mode: full rescan

This file consolidates **two parallel pipeline runs** that executed against this repo:
- **Run A** (tool-backed): ran `gitleaks`, `govulncheck`, `go mod verify`, `gosec`, `npm audit`, `staticcheck` + manual review of code-exec, CSRF, secrets, infra. → V-001…V-010, H-001…H-004.
- **Run B** (deep fan-out): recon + 9 vuln-class hunters + adversarial verification. → added V-011…V-013, H-005…H-006 and reconciled V-006.

Every entry below survived manual/adversarial verification. Refuted scanner noise and accepted-by-design posture is in **Rejected / Accepted** at the end.

> **Remediation status (updated post-scan):**
> - **V-001 / V-002 — FIXED** by a parallel session: new `kernel/envscrub` package; `coding.go` and `acpagent.go` now scrub the child env (tests pass).
> - **V-013 — FIXED:** `entryToMap` now masks `RatingSecret` values via `creds.MaskValue` + `"masked":true` (regression test `configcenter_secret_mask_test.go`).
> - **H-005 — FIXED:** SVG artifacts now served with `Content-Security-Policy: sandbox` (blocks direct-nav script exec, keeps `<img>` rendering).
> - **H-006 — FIXED:** voice + embeddings adapters now use a netguard-protected client (`AllowLoopback`+`AllowPrivate` so local inference servers still work; cloud-metadata stays blocked).
> - **V-006 / V-007 / V-008 / V-009 / V-010 / V-011 / H-001 — FIXED:** Host/Origin checks, `setup-go-safe` fail-fast runner env checks, query-token narrowing, REST/OpenAI Bearer-only auth, REST/OpenAI referrer policy, long-lived SSE connection caps, Discord attachment URL validation, memory-search limit clamping, and admin-only gating for daemon-global REST routes are now in the current tree.
> - **V-004 — GUARDS ADDED:** `internal/ciguard` fails tests if self-hosted `pull_request` jobs lack the same-repo fork guard, if workflow checkouts omit `persist-credentials:false`, if Dependabot stops covering the core ecosystems, or if `.env.example` is missing/ignored/secret-valued. Ephemeral runners and fork-PR approval remain owner/infra action.
> - **V-005 — ABSENT:** `list_vault.py` is not present in the current workspace and is not tracked by Git.
> - **V-012 — OPT-IN GATE ADDED:** the system-guardian defang vector is blocked, and `AGEZT_OVERSEER_FLEET_LOCK=on` disables agent-reachable `overseer` edit/create. Default remains off to preserve the existing default-allow posture.
> - **V-003 — FIXED LOCALLY:** `.playwright-mcp` was removed, secret-shaped `.env` values were blanked, `.env` NTFS ACLs were narrowed to current user + SYSTEM/Administrators, and a sanitized tracked `.env.example` was added. `gitleaks` now reports no leaks. Provider-side rotation/revocation is still recommended for any keys that were live before cleanup.
> - **H-002 / H-003 — ABSENT + IGNORED:** the reported loose root scripts are not present, and `.gitignore` now blocks those local debug/launcher names from being committed.
> - **H-007 — TRACKING ADDED:** `.github/dependabot.yml` now tracks Go modules, frontend npm, TypeScript SDK npm, and GitHub Actions. `go list` confirms `go-imap/v2` still has no stable v2 release beyond `v2.0.0-beta.8`.
> - Remaining open items are operational/posture tasks: provider-side rotation/revocation for any previously live `.env` keys, V-004 ephemeral runners + fork-PR approval, and whether to enable `AGEZT_OVERSEER_FLEET_LOCK`.

**No Critical or High exploitable issue was confirmed.** Highest confirmed class is Medium, and the two highest are conditional (require non-default deployment modes).

## Summary

| ID | Severity | Area | Status |
|---|---:|---|---|
| V-001 | Medium | `coding` tool forwards full daemon env to child | Remediated |
| V-002 | Medium | `acp_agent` inherits full daemon env | Remediated |
| V-003 | Medium | Plaintext secrets in ignored local files (`.env`, playwright snapshot) | Fixed locally; provider rotation recommended |
| V-004 | Medium | Self-hosted CI runners rely on fragile per-job fork guard | Guards added; infra action remains |
| V-005 | Medium | `list_vault.py` vault-probe scratch script committed | Absent in current workspace |
| V-006 | Low–Medium | Console mutating routes lack Origin/Host check (rebinding chain refuted) | Remediated |
| V-007 | Low | `setup-go-safe` `rm -rf` toolcache footgun | Remediated |
| V-008 | Low | `?token=` query-string auth accepted too broadly | Remediated except SSE/bootstrap |
| V-009 | Low | SSE streams have no per-credential connection cap | Remediated for long-lived streams |
| V-010 | Low | agentgw memory-search `limit` parsed without clamp | Remediated |
| **V-011** | **Medium (cond.)** | **REST API tenant token has no per-route restriction → cross-tenant mailbox/board IDOR + sender spoofing** | **Remediated** |
| **V-012** | **Medium** | **`overseer` tool (default-allow) can rewrite/defang agents; agent→fleet-admin escalation, no approval gate** | **Opt-in gate added; owner decides default** |
| **V-013** | **Low–Medium** | **Config Center returns `RatingSecret` values in cleartext over the console API** | **Remediated** |
| H-001 | Low | Discord attachment fetch not netguarded / no URL policy | Remediated |
| H-002 | Low | Loose operator/debug scripts at repo root | Absent + ignored |
| H-003 | Low | Hidden daemon launcher lacks integrity/path checks | Absent + ignored |
| H-004 | Info | `code_exec` not a hard sandbox on every host (by design) | Residual risk |
| **H-005** | **Low** | **Artifact route serves `image/svg+xml` verbatim → stored XSS on direct nav (behind auth)** | **Remediated** |
| **H-006** | **Low** | **voice (STT/TTS) + embeddings adapters use plain `http.Client`, bypass netguard** | **Remediated** |
| H-007 | Info/track | `emersion/go-imap/v2` beta MIME parser on attacker email input | Tracking added; latest remains beta |

---

## Remediation Applied 2026-06-24

- Added `kernel/envscrub` and switched `coding` + `acp_agent` child processes to a scrubbed allowlist environment, while preserving explicit task/peer env passed by the caller. Covered by `kernel/envscrub`, `plugins/tools/coding`, and `plugins/tools/acpagent` tests.
- Added Web UI Host allowlisting and same-origin mutation checks (`Origin` / `Sec-Fetch-Site`) with explicit `AGEZT_WEB_ALLOWED_HOSTS` support for domain/tunnel deployments.
- Narrowed query-string console tokens: production Web UI data routes now require Bearer/session auth; only `/events` keeps `?token=` for EventSource and shell/deep-link bootstrap still accepts the banner URL token. REST/OpenAI-compatible APIs now require Bearer tokens and no longer accept query tokens.
- Added `Referrer-Policy: no-referrer` to REST/OpenAI JSON responses.
- Clamped agent gateway memory-search `limit` to `[1,200]`.
- Added `internal/ciguard` workflow linting for self-hosted `pull_request` jobs and removed unsafe shared-runner fallbacks from `.github/actions/setup-go-safe/action.yml`.
- Added `kernel/streamlimit` and wired it into long-lived Web UI `/events` and REST mailbox `/watch` SSE streams.
- Added Discord attachment URL validation before fetching provider-supplied attachments.
- Added opt-in `AGEZT_OVERSEER_FLEET_LOCK` for the agent-reachable `overseer` edit/create path.
- Removed `.playwright-mcp` local browser-capture artifacts, narrowed `.env` NTFS ACLs, and added `.gitignore` entries for the reported local root debug/launcher scripts.
- Added Dependabot tracking for Go modules, frontend npm, TypeScript SDK npm, and GitHub Actions.
- Expanded `internal/ciguard` tests to enforce checkout `persist-credentials:false` and Dependabot coverage in addition to self-hosted fork guards.
- Added `.env.example` as a tracked, sanitized local-env template; `internal/ciguard` now verifies it remains unignored and secret-value-free.
- Verification:
  - `go test ./kernel/controlplane ./kernel/webui ./kernel/restapi ./kernel/openaiapi ./kernel/agentgw ./kernel/envscrub ./plugins/tools/coding ./plugins/tools/acpagent ./plugins/tools/overseertool ./plugins/providers/embed ./plugins/providers/voice` passed.
  - `go test ./internal/ciguard ./kernel/streamlimit ./kernel/webui ./kernel/restapi ./plugins/channels/discord` passed.

## V-001 — Medium — `coding` tool forwards the full daemon environment
**Location:** `plugins/tools/coding/coding.go:141-143`
Builds the external-agent env with `append(os.Environ(), "AGEZT_CODING_TASK="+task)`, forwarding the daemon's entire environment (provider keys, vault creds, AWS, all `AGEZT_*`) to a prompt-steerable child process.
**Preconditions:** operator enabled the bridge (`AGEZT_CODING_CMD`/catalog); agent allowed to call the tool (default-allow); secrets present in env. The `task` param is fully prompt-injectable.
**Status:** remediated via `kernel/envscrub.Scrubbed()` + explicit `AGEZT_CODING_TASK` append; secret-shaped daemon env names are not inherited.

## V-002 — Medium — `acp_agent` inherits the full daemon environment
**Location:** `plugins/tools/acpagent/acpagent.go:238-255`
`spawnAgent` never sets `Cmd.Env`, so Go defaults to `os.Environ()` — same secret leak as V-001. Command source is operator/catalog-constrained (no shell injection), but the spawned agent can be prompt-steered to disclose inherited secrets.
**Status:** remediated via `c.Env = envscrub.Scrubbed()`; live test now sets only the intended peer env inside the child shell command.

## V-003 — Medium — Secret-shaped values in ignored local files
**Locations:** earlier `.env` generic API-key-shaped values and `.playwright-mcp` browser captures. Detected by gitleaks; not git-tracked.
**Impact:** not committed, but plaintext local secrets leak via backups, support bundles, copied worktrees, or a future force-add. `.env` even contradicts its own "do not store a value" comment by holding the vault passphrase in cleartext.
**Status:** fixed locally: `.playwright-mcp` was removed; `.env` secret-shaped values were blanked (`MINIMAX_API_KEY`, `DEEPSEEK_API_KEY`, and `AGEZT_VAULT_PASSPHRASE`); `.env` ACLs now allow only the current user plus SYSTEM/Administrators; and a sanitized `.env.example` is tracked for future setup. `gitleaks` now reports no leaks. Rotate/revoke any previously live keys at the providers; keep gitleaks in CI/pre-commit incl. ignored-file mode.

## V-004 — Medium — Self-hosted CI runners rely on per-job fork-PR guards
**Location:** `.github/workflows/ci.yml:16-33` + repeated per-job guards.
Triggers on `pull_request` (fires for forks) on persistent self-hosted WSL runners; the only thing blocking fork-PR code exec is a per-job `if: …head.repo.full_name == github.repository`. Correct today, fragile: one unguarded new job re-opens host-level compromise of a long-lived runner.
**Fix:** ephemeral runners + repo fork-PR approval policy + a workflow-lint check failing any self-hosted `pull_request` job missing the same-repo guard.
**Status:** repo-side guards added via `internal/ciguard`: self-hosted `pull_request` jobs must carry the same-repo fork guard, every workflow checkout must set `persist-credentials:false`, `setup-go-safe` must not reintroduce shared runner fallbacks, Dependabot must cover the core ecosystems, and `.env.example` must remain tracked and free of secret-like values. This reduces regression risk for future workflow changes, but does not replace owner/infra hardening: use ephemeral self-hosted runners and enforce fork-PR approval.

## V-005 — Medium — `list_vault.py` committed vault/debug scratch script
**Location:** `list_vault.py:1-11` — invokes `agt vault status --json`, prints output and enumerates secret env-var names. Prints names not values, but a vault-introspection tool should not ship in the repo root.
**Status:** absent in the current workspace and not tracked by Git. If this diagnostic is needed later, add a purpose-built reviewed CLI that never prints secret-adjacent data.

## V-006 — Low–Medium — Console mutating routes lack an Origin/Host check (reconciled)
**Locations:** `kernel/webui/webui.go:1064-1073` (`authorized()`), `kernel/webui/session.go:211-219` (cookie attrs).
In default (non-STRICT) password mode `authorized()` returns `tokenPresented(r) || sessionValid(r)`, so a session cookie alone authorizes mutating POSTs, and there is **no Origin / Host / `Sec-Fetch-Site` / CSRF-token check anywhere** in `kernel/webui`.
**Reconciliation (Run B adversarial verify):** the originally-claimed **DNS-rebinding exploitation is refuted** — the session cookie is **host-only** (`session.go` sets no `Domain`), scoped to `127.0.0.1`. Rebinding changes the resolved IP, not the document origin/host, so the loopback cookie never attaches and `SameSite=Strict` withholds it regardless. Ordinary cross-site CSRF is already covered by `SameSite=Strict`. The real residual is therefore the **missing Origin/Host defense-in-depth check** (matters most if the console is ever exposed via tunnel/reverse-proxy, or to harden against future cookie-scope changes), not a presently-reachable bug. Severity lowered from Medium to **Low–Medium**.
**Status:** remediated with Host validation, explicit allowed-host configuration, and same-origin mutation checks. STRICT-mode default remains an owner posture decision.

## V-007 — Low — `setup-go-safe` deletes broad toolcache paths from env-derived values
**Location:** `.github/actions/setup-go-safe/action.yml:35-38, 79-85` — `rm -rf` against `${RUNNER_TOOL_CACHE:-$HOME/actions-runner-2/…}` and `/dev/shm/goroot-${RUNNER_NAME:-shared}`; can delete the wrong shared-runner cache if env vars are absent. **Fix:** fail-fast when `RUNNER_TOOL_CACHE`/`RUNNER_NAME` unset; drop cross-runner hardcoded fallbacks.
**Status:** remediated. The action now requires `RUNNER_TOOL_CACHE` and `RUNNER_NAME` to be set and no longer falls back to a shared toolcache/`shared` GOROOT path. `internal/ciguard` has a regression test that blocks reintroducing those fallbacks.

## V-008 — Low — Query-string token fallback accepted too broadly
**Locations:** `kernel/webui/webui.go:1042-1055`, `kernel/restapi/restapi.go:291-298`, `kernel/openaiapi/openaiapi.go:257-264`, `frontend/src/lib/api.ts:1-23`.
SSE genuinely needs a `?token=` fallback (EventSource can't set headers), but web/REST/OpenAI accept it more broadly. Leaks token to history/proxy logs/crash reports. Loopback default + `Referrer-Policy: no-referrer` (webui only) keep it Low. **Status:** remediated for production data APIs: Web UI `/api/*` uses Bearer/session, REST/OpenAI require Bearer, and REST/OpenAI JSON responses now include `Referrer-Policy: no-referrer`. `/events` and shell/deep-link bootstrap retain query tokens by design.

## V-009 — Low — SSE streams have no per-credential/per-IP connection cap
**Locations:** `kernel/webui` `/events`, `kernel/restapi/restapi.go:414-461`, `kernel/openaiapi/openaiapi.go:615-677`, `responses.go:206-319`. Body caps + slow-loris timeouts exist, but no stream-count limit → authenticated FD/goroutine exhaustion. **Fix:** small per-token/per-IP active-stream cap; log refusals.
**Status:** remediated for the long-lived streams: `kernel/streamlimit` caps Web UI `/events` and REST mailbox `/watch` by client IP with `429` + `Retry-After` over the cap. Request-bounded proxy/run streams remain timeout-capped and were left unchanged.

## V-010 — Low — agentgw memory-search limit parsed without a clamp
**Location:** `kernel/agentgw/handlers.go:200-210` — `handleMemorySearch` parses `limit` via `fmt.Sscanf` and passes to `Recall` unbounded. **Status:** remediated with explicit parse + clamp `[1,200]`.

## V-011 — Medium (conditional) — REST API tenant token: no per-route restriction → cross-tenant IDOR + sender spoofing
**Location:** `kernel/restapi/restapi.go:272-289` (`authorized()`); contrast `kernel/controlplane/tenant.go:68-89` (`tenantTokenAllows`) + `server.go:485-504`.
The control plane restricts a tenant token via a command allowlist that **excludes every `board_*` command**. The REST API has **no equivalent per-route/per-command restriction** — `authorized()` accepts the admin token OR `tenantAuth(tenant, token)` and then applies the same `auth()` wrapper to *every* route, including the **daemon-global mailbox/board** (`mailbox.go:25-26` never consults the tenant). `From`/`To`/`by` are caller-supplied and bound to nothing (`mailbox.go:108-116,161-180,260-281`).
**Exploit (when enabled):** a tenant-A token can read any agent's inbox (`?name=`), enumerate any thread's replies by id, **spoof `from`** (waking standing orders via `board.posted` as a forged sender), ack/clear another inbox, and tail the cross-tenant board firehose.
**Why Medium not High — all three preconditions are non-default:** REST is OFF unless `AGEZT_REST_ADDR` is set (`main.go:4755-4758`); the tenant authorizer is wired only in multi-tenant mode (`main.go:4863-4878`) — single-tenant has no boundary to cross; attacker must hold an operator-minted tenant token. Where a deployment *is* REST-on + multi-tenant, in-deployment impact is high.
**Status:** remediated for the tenant-token boundary: mailbox/board and host-global update routes are now admin-token-only in the REST mux, with regression coverage proving tenant tokens are rejected there while tenant-scoped routes still work.

## V-012 — Medium — `overseer` tool can rewrite/defang agents (agent→fleet-admin escalation)
**Locations:** capability `CapOversee` ships `LevelAllow` (`kernel/edict/edict.go:606-609`); default agent with empty `ToolAllow` gets the full toolset (`kernel/runtime/runtime.go:2127-2132`); `op=edit` path (`…/overseer tool.go:178-191` → `kernelsource.go:61-94`) calls `UpdateProfile`, which has **no System check** (`runtime.go:1196-1209`; contrast `RemoveProfile:1213-1218` which refuses System). `applySystemGuardianDefaults` only floors cost/trust/noise — not `Soul`/`ToolAllow`/`ToolDeny`/`ConfigOverrides`/`TrustCeiling`.
**Exploit:** a non-admin agent can use `overseer op=edit/create` to self-grant capabilities, create privileged agents, or rewrite a System guardian's `Soul`/`ToolAllow`/`ConfigOverrides` to **behaviorally neutralize it** (the System flag stays `true` so it resists *removal*, but is defanged). The unit test (`overseer_test.go:77,85,293-294`) only enforces "can't set the System flag," not "can't edit a guardian's behavior."
**Important nuance:** the "bypasses the admin-token boundary" framing is **refuted** — the control-plane admin path (`handleAgentEdit`, `roster.go:1165-1237`) uses the *same* `UpdateProfile`/`applyAgentMutableProfileFields` with the same lack of System-field protection. There is no stronger boundary being bypassed. The genuine concern is **agent → fleet-admin privilege escalation with no approval gate**, which is in tension with the owner's deliberate default-allow posture → warrants an **owner decision**, not an assumed bug.
**Status:** the System-guardian defang vector is mitigated: `overseertool` now refuses `EditAgent` against `System` profiles, while the operator admin path remains available. The broader agent→fleet-admin path now has an opt-in gate: set `AGEZT_OVERSEER_FLEET_LOCK=on` to make the agent-reachable `EditAgent`/`CreateAgent` refuse and require operator console/CLI edits. The default remains off to preserve the existing default-allow posture, so enabling it is an owner policy decision.

## V-013 — Low–Medium — Config Center returns `RatingSecret` values in cleartext over the console API
**Location:** `kernel/controlplane/configcenter_handler.go:387-411` (`entryToMap` emits raw `e.Value` for every entry incl. `RatingSecret`); used by `/api/configcenter/list` + `/get` (`webui.go:219-220`) and rendered by the SPA.
This contradicts redaction everywhere else (`audit.go:59` → `"REDACTED"`; `types.go:310` Search skips secrets; provider keyring masks to last-4). The classifier (`classifier.go:50-92`) tags api_key/password/token/aws/github/stripe-shaped entries as secret, so any secret stored in Config Center is exposed in cleartext to the console.
**Why not higher:** behind console auth + loopback; provider API keys live in the encrypted **vault** (correctly masked), not Config Center — exposure is limited to whatever was stored as a Config Center `secret` entry.
**Status:** remediated: `entryToMap` masks via `creds.MaskValue` when `Rating == RatingSecret` and sets `"masked": true`.

## H-001 — Low — Discord attachment fetch lacks URL policy / netguard
**Locations:** `plugins/channels/discord/discord.go:215-219, 251-297, 393-419`. Interactions are signature-verified + channel-allowlisted, content-type filtered, 12 MiB capped — but the URL isn't validated as `https` Discord/CDN, nor routed through netguard. Not a confirmed generic SSRF (URLs are provider-origin), but a defense-in-depth gap. **Fix:** require `https` + allowlist Discord CDN host suffixes, or use the netguard client.
**Status:** remediated. `fetchAttachmentDataURL` validates `att.URL` before dialing and only accepts `https` Discord CDN hosts, rejecting foreign/internal/metadata-style URLs. Covered by `TestValidDiscordAttachmentURL`.

## H-002 — Low — Loose operator/debug scripts at repo root
`fix_scout.py`, `scout_*.ps1`, `update_scout*`, `find_model.py`, `find_302ai.py`, `readlines.js`, `start-daemon.*` — scratch scripts that leak local machine paths (`C:\Users\ersin\…`) and use `-ExecutionPolicy Bypass`. Move to an ignored local dir or a documented `scripts/` namespace.
**Status:** absent in the current workspace. `.gitignore` now blocks these local debug/launcher names from being committed at repo root.

## H-003 — Low — Hidden daemon launcher lacks integrity/path checks
`start-daemon.ps1`/`.bat` launch binaries with no path/integrity check, hidden-window background. Local-operator scope. **Fix:** resolve to expected repo paths, fail if missing, avoid hidden launchers for daemon startup.
**Status:** absent in the current workspace and covered by the root-script `.gitignore` block.

## H-004 — Info — `code_exec` is not a hard sandbox on every host (by design)
`plugins/tools/codeexec/codeexec.go` + `kernel/warden`. Intentionally high-blast-radius, Edict-gated, env-scrubbed, workdir-jailed; hard isolation (Linux namespaces / OS-jailed Deno) only on supported profiles — elsewhere workdir/env/limits only. **Posture:** treat `code.exec` as a trusted capability; for hostile code run the daemon in a VM/container and set `AGEZT_SANDBOX_NO_NET=1`.

## H-005 — Low — Artifact route serves `image/svg+xml` verbatim → stored XSS on direct navigation
**Location:** `kernel/webui/artifact_route.go:55` (`safeContentType` allowlist includes `image/svg+xml`). An agent-generated SVG saved as an artifact, opened by direct navigation, executes embedded `<script>` in the console origin. Behind console auth + `nosniff`, so Low. **Fix:** drop SVG from the allowlist (serve as `application/octet-stream` or `text/plain`), or serve with `Content-Security-Policy: sandbox`.
**Status:** remediated with a sandboxing CSP on SVG artifact responses.

## H-006 — Low — voice + embeddings adapters bypass netguard
**Locations:** `plugins/providers/voice/voice.go:91-96`, `plugins/providers/embed/embed.go:57` use a plain `http.Client`. Destination is operator env config (`AGEZT_STT_URL`/`TTS_URL`/`EMBED_URL`), not agent-tainted, so Low — but every other adapter uses the netguard client. **Fix:** one-line swap to the netguard-protected client for parity.
**Status:** remediated: both adapters use a netguard-protected HTTP client.

## H-007 — Info/track — beta email-parsing stack
`emersion/go-imap/v2 v2.0.0-beta.8` (+ indirect `go-message v0.18.2`, `go-sasl`) parses attacker-influenced email-channel input. No named CVE, but a beta parser on hostile input is the top dependency-freshness item. Track for the stable release.
**Status:** tracking added via `.github/dependabot.yml`. `go list -m -versions github.com/emersion/go-imap/v2` shows `v2.0.0-beta.8` remains the latest published v2 version, so there is no stable v2 target to upgrade to yet.

---

## Rejected / Accepted (not counted as findings)

**Adversarially refuted:**
- **CSRF via DNS rebinding** — host-only loopback cookie + `SameSite=Strict` defeat the cookie-attach; residual is the Origin-check gap captured as V-006.
- **`image_url` SSRF** — URL is forwarded to the upstream LLM provider; AGEZT never fetches it.
- **Matrix `mxc://` host pivot** — attacker server is path-escaped into the operator's own homeserver URL.

**Verified-fixed / structurally safe:**
- agentgw JWT (alg-pinned HS256, `hmac.Equal`, iss/aud/exp pinned, per-install `crypto/rand` secret, no hardcoded fallback, subset/clamp non-escalatable) — the prior hardcoded-secret + unauth-mint holes are closed.
- Vault crypto (AES-256-GCM, fresh per-save salt+nonce from `crypto/rand`, PBKDF2-HMAC-SHA256 200k, machine-bound).
- No SQLi/NoSQLi (bbolt KV, no query language), no SSTI, no XXE (stdlib `encoding/xml`), no LDAP/GraphQL, **no `dangerouslySetInnerHTML`** anywhere; agent text rendered via React-escaped Markdown AST with `safeHref` scheme allowlist.
- netguard validates the **resolved IP** at dial + every redirect hop (blocks loopback/RFC1918/ULA/link-local 169.254.169.254/0.0.0.0/CGNAT + IPv6-embedded-IPv4 tricks); no TOCTOU/rebinding window.
- Strong console security headers (`nosniff`, `X-Frame-Options: DENY`, CSP `frame-ancestors 'none'`/`base-uri 'none'`/`form-action 'none'`, `Referrer-Policy: no-referrer`); login lockout; `MaxBytesReader` body caps; constant-time token compare.

**Accepted-by-design (owner posture):**
- `code_exec`/`shell` network reaches metadata/RFC1918 (in-process netguard doesn't apply to child processes) — documented max-capability posture; real boundary is the Edict gate + secret-scrub, both verified.
- HTML artifact preview runs in an `iframe sandbox="allow-scripts"` **without** `allow-same-origin` — correctly isolated from the console token.

**`gosec`/`staticcheck` noise reviewed & dismissed:** G115 (bit-packing/validated ranges), G404 (`math/rand` jitter, non-security), G101 (env-var names/issuer constants/test fixtures), G703/G704 (local CLI args / operator-configured URLs, not remote server-side), and 4 non-security staticcheck lint items.
