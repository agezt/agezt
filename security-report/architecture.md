# AGEZT — Architecture & Attack-Surface Map (RECON)

Read-only recon for a security audit. Scope: Go daemon `agezt` + `agt` CLI + embedded React/TS web console.
All references are `file:line` against `D:/Codebox/PROJECTS/AGEZT`.

---

## 1. Languages / Frameworks

| Layer | Stack |
|---|---|
| Daemon / CLI | **Go 1.26.4** (`go.mod:3`). Minimal deps: `coder/websocket`, `emersion/go-imap`, `btcec` (secp256k1/Ed25519-adjacent), `blake3`. Std-lib `net/http` for all servers. |
| Frontend | **React 19 + TypeScript 6 + Vite 8**, Tailwind 4, Radix UI, `@xyflow/react` (flow canvas), lucide-react (`frontend/package.json:16-44`). Built and `go:embed`-ded into the daemon binary (`kernel/webui/dist/`). |
| Tests | Go `testing`; frontend Vitest + Playwright. |

Architecture: single Go process (`agezt`) hosts a kernel runtime + multiple optional network listeners; `agt` is a thin CLI that talks to the daemon over a loopback control-plane socket. The web console is a SPA embedded in and served by the daemon.

---

## 2. Network Listeners & Trust Boundaries

All HTTP servers use `newGuardedHTTPServer` (slow-loris read-header/idle timeouts; `cmd/agezt/main.go:4203`). A non-loopback bind prints a `[WARNING: not loopback]` banner but is **not blocked** — the operator is trusted (default-allow posture).

| Listener | Bind / Transport | Default | Auth | Source |
|---|---|---|---|---|
| **Web console** | `127.0.0.1:8787` (TCP loopback); falls back to `127.0.0.1:0` if busy | **ON by default** | per-boot 32-byte hex token (query/header) + optional `AGEZT_WEB_PASSWORD` | `cmd/agezt/main.go:4220-4304` (`net.Listen` @4250) |
| **Control plane** | `127.0.0.1:0` (TCP loopback, ephemeral port) | always on | per-boot token in `runtime/control.token` (0600) | `kernel/controlplane/server.go:256`; addr/token files @409-421 |
| **OpenAI-compat API** | `AGEZT_API_ADDR` (operator-supplied host:port) | **OFF** unless env set | per-boot Bearer token | `cmd/agezt/main.go:4688-4749` (`net.Listen` @4703) |
| **REST API** | `AGEZT_REST_ADDR` (operator-supplied) | **OFF** unless env set | per-boot Bearer token | `cmd/agezt/main.go:4754-...` (`net.Listen` @4766) |
| **Agent gateway** | abstract unix socket `@agezt/agentgw.sock` by default; TCP only if `AGEZT_AGENTGW_SOCKET=host:port` | on (socket) | per-install HMAC-SHA256 JWT | `kernel/agentgw/gateway.go:160-180` |
| **ChatGPT OAuth callback** | `127.0.0.1:1455` (TCP loopback) | only during a sign-in flow; auto-expires | unguessable `state` param | `kernel/controlplane/provider_oauth.go:65`; `kernel/chatgptauth/chatgptauth.go:42-43` |
| **Channel inbound** (slack/discord/etc.) | operator-set `*_ADDR`, e.g. `127.0.0.1:8840` | per-channel opt-in | per-channel allowlist | `cmd/agezt/main.go` channel build funcs |
| **Tunnel** (cloudflared/ngrok) | exposes a local listener to the **public internet** | OFF unless `AGEZT_TUNNEL*` set | inherits target's auth only | `cmd/agezt/main.go:4307-4359` |

**Trust boundaries.** The strong default is loopback-only TCP + local sockets; nothing binds `0.0.0.0` implicitly (`main.go:4217`). The principal trust boundaries are:
- **Browser ⇄ web console** (token/password) — the main human boundary.
- **`agt`/SDK ⇄ control plane** — any local process that can read the 0600 `control.token` is fully authorized (bearer-secret, **not** OS peer-credential).
- **Agent-authored code/commands ⇄ host** — the agent tool surface (§5) is the largest *internal* boundary: a model can run shell/code on the host.
- **Daemon ⇄ external providers / fetched web content** — egress + prompt-injection surface.
- **Tunnel** collapses the loopback boundary to the public internet when enabled — high-risk toggle.

---

## 3. HTTP Route Inventory & Registration

### 3a. Web console — `kernel/webui/webui.go`
Routes registered in `Handler()` (`webui.go:562-617`) via stdlib `ServeMux` and three middleware wrappers:
- `s.secure(...)` — security headers only, **NO auth** (`webui.go:970`).
- `s.auth(...)` — headers **+ token/session auth** (`webui.go:980`).
- `s.shellAuth(...)` — shell-only gate (`webui.go:996`).

**Public (no auth):** `/` SPA shell (shellAuth, serves login if token-less, @568) · `/api/authmeta`, `/api/login`, `/api/logout` (@572-574) · `/assets/`, `/favicon.ico` (@580-581) · `/hooks/<workflow>` — authed by **per-workflow secret** (`X-Agezt-Secret`/`?secret=`), console-token-free (@612, verify @678-731) · `/oauth/callback` — security via unguessable `state` (@616).

**Gated (`s.auth`):** `/events` (SSE firehose, @1164) · large allowlisted route tables — `apiRoutes` read proxies (@104-172), `readArgsRoutes` (@186-272), `writeRoutes` POST mutations (@277-393), `jsonRoutes` POST-JSON mutations (@402-544) · `/api/plan/run`, `/api/run`, `/api/toolbox/install`, `/api/market/install|uninstall`, `/api/transcribe`, `/api/artifact/raw` (@582-607). Every webui data route is a **proxied control-plane Cmd*** call with a fixed arg/key allowlist (no generic passthrough).

### 3b. REST API — `kernel/restapi/restapi.go:172-194`
Public: `/healthz`, `/readyz` only. Gated (`s.auth`): `/metrics`, `/api/v1/health|models|runs|runs/{corr}`, `/api/v1/mailbox/*` (incl. SSE `watch`), `/api/v1/update`, `/api/v1/update/apply`.

### 3c. OpenAI-compat API — `kernel/openaiapi/openaiapi.go:168-178`
**All 5 routes gated** (no public endpoint): `POST /v1/chat/completions`, `POST /v1/responses`, `GET /v1/models`, `GET /v1/models/{id}`, `POST /v1/audio/transcriptions`.

### 3d. Agent gateway — `kernel/agentgw/gateway.go:117-151` (Go 1.22 method-prefixed patterns)
Public: `GET /health` only. Gated (`withAuth` + per-handler capability check): eventbus subscribe/publish, memory write/delete/search, log read/write, agent list/query, **`POST /v1/token/create`**, config get/list/search/set/audit.

---

## 4. AuthN/Z Model

**Control plane** (`kernel/controlplane/server.go`): line-delimited JSON over loopback TCP. Token minted with `crypto/rand`, hex (@260-265), written to `runtime/control.token` (0600) + addr to `control.addr` (0600) (@409-421). Per-request gate `tokenIsPrimary` uses **`subtle.ConstantTimeCompare`** (@325-336, gate @485). Pre-auth request bounded to 16 MiB (`readBoundedLine` @338-368). Fallback **tenant-token** auth: must name `tenant` + present its token, allowlisted commands only (@486-503). Per-connection panic recovery (@1074). No TLS (loopback design).

**Web console** (`kernel/webui/webui.go`, `session.go`):
- Token compared constant-time (`tokenMatch` @1079; empty server token never serves @1045). Accepted via `?token=` (EventSource) or `Authorization: Bearer` (@1049-1054).
- Password: `SetPasswordFn`/`SetPasswordStrict` live-read `AGEZT_WEB_PASSWORD` (`session.go:133-152`). `authorized()` (@1064-1073): no password → token only; password set → **token OR session** (default, M933 alt-door); strict (`AGEZT_WEB_PASSWORD_STRICT=on`) → **token AND session**.
- Login `handleLogin` (`session.go:177-221`): constant-time compare (@200), **brute-force lockout 8 fails → 5-min cooldown** (@36-40,188), 4 KiB body cap.
- Sessions: in-memory, 32-byte `crypto/rand` id, 12h sliding TTL. Cookie `HttpOnly`+`SameSite=Strict`+`Secure`-when-TLS (@211-251).
- **No CSRF token.** Mitigations: SameSite=Strict, POST-only mutations, fixed allowlists, loopback bind, `Referrer-Policy: no-referrer`, strict CSP (`default-src 'none'`, @1025-1040). Cookie-only auth (no token) on a cross-site simple POST is the residual concern to verify.

**APIs** (rest/openai): Bearer header **or `?token=` query** (token-in-URL leakage risk), constant-time compare (`restapi.go:278`, `openaiapi.go:244`). Tenant routing via `X-Agezt-Tenant`: per-tenant token authorizes only its own tenant; admin token authorizes any (`restapi.go:277-288`). **Mailbox is daemon-global, NOT tenant-partitioned** (`mailbox.go:25`) — cross-tenant channel to verify.

**Agent gateway** (`kernel/agentgw/`): HMAC-SHA256 JWT, verified with **`hmac.Equal`** (`token.go:124`); alg/typ/iss/aud pinned, `alg:none` rejected (`token.go:99-145`); expiry checked. Signing key via `ResolveTokenSecret`: env `AGEZT_AGENTGW_TOKEN_SECRET` → `agentgw.secret` file (0600) → fresh CSPRNG (`secret.go:40-130`). **Prior hole (hardcoded secret + unauth mint) is closed**: `/v1/token/create` requires a valid parent token; child caps must be a **subset** of parent (403 on escalation), expiry/rate clamped to parent, RunID inherited (`gateway.go:359-427`). Per-handler capability re-check after `withAuth` (`capabilities.go:47`). Note: abstract-unix-socket default has **no filesystem perm gate** — any local process in the namespace can connect; entire trust = the JWT.

---

## 5. Agent Capability Surface

Capability mapping single-source: `kernel/edict/toolmap.go:20`. **Default-allow is intentional** (owner posture): `DefaultLevels()` sets **every** capability to `LevelAllow` (`kernel/edict/edict.go:606-612`); `AGEZT_ALLOW_ALL` adds `UnknownAllow` for future tools.

**Dangerous tools** (`plugins/tools/*`):
- **`shell`** (CRITICAL) — arbitrary OS command via `cmd /S /C` (Win) / `sh -c` (`shell/shell.go:227`). Env scrubbed (`shell/env.go:25` allowlist + `isSecretName` drop). **No in-tool command allow/denylist** — only Edict gates it.
- **`code_exec`** (CRITICAL) — writes & runs arbitrary Python/Node/Deno, installs pip pkgs (`codeexec/codeexec.go`). Env scrubbed (`runtimes.go:120-156`); workdir-confined.
- **`coding`** (HIGH) — spawns external coding agent (Claude Code/Codex/Aider). **Passes full `os.Environ()` un-scrubbed** (`coding/coding.go:141`) → child inherits daemon API keys (flag for hunt).
- **`mcp` / `acp_agent`** (HIGH) — agent self-installs/spawns arbitrary MCP/ACP subprocesses.
- **`overseer`** (HIGH) — cancel runs, halt/resume daemon, pause/retire agents.
- **`remote_run`, `forge_*`, `schedule`, `standing`, `workflow`, `config`, `homeassistant`** (MEDIUM) — autonomy / config-write / physical actuation.
- **`http`/`fetch`/`web_search`/`browser.read`** (MEDIUM) — netguard-guarded egress.

**Sandbox reality (key finding):** both `shell` and `code_exec` request `ProfileNamespace`, but the **warden downgrades all profiles to `ProfileNone` on every platform** (`kernel/warden/warden.go:15-21,469`) — there is **no container/VM/namespace jail today**. Containment = timeout + output cap + workdir + env-scrub + (Linux-only) best-effort rlimits + `setpgid`. The "sandbox" is essentially env-scrubbing plus Deno's `--allow-read/write/env` fs-flags (`runtimes.go:96-110`).

**Egress / SSRF** (`kernel/netguard/netguard.go`): in-process Go HTTP tools route through netguard, which validates the **resolved IP** at dial + every redirect hop (@176), default-blocking loopback, RFC1918/ULA, link-local incl. **169.254.169.254**, CGNAT, 0.0.0.0/8, and IPv6-embedded-v4 smuggling (@119). Metadata needs `AllowLinkLocal()` which **no tool sets**. **Gap:** `code_exec` Python/Node get **raw host network, NOT behind netguard** — model-written code can reach `127.0.0.1` / RFC1918 / `169.254.169.254` directly.

**Restriction rails** (default-allow ⇒ these are the real controls): **F4 hard-deny floor** (immutable, shell-scoped: fork-bomb, `rm -rf /`, mkfs/wipefs, `dd of=/dev/...`, shutdown/reboot — `edict.go:617-639`, evasion-normalized @360); **netguard** (HTTP tools only); **governor budgets** (separate); **HITL approval gates** layered in the runtime — EpistemicEscalation, IntentRegretGating, **PromptInjectionGuard** (`runtime/runtime.go:1565-1631`, default-deny on 5-min timeout). Hard-deny can never be removed at runtime (`edict.go:513,863`).

---

## 6. Secret / Credential Storage (`kernel/creds/`)

- **Single vault file** `<baseDir>/creds.json` (default `~/.agezt/creds.json`, `creds.go:47,82`). Atomic write → forced **0600** (`atomicWriteVault` @209-235, re-chmod after rename for Windows/rename-widen).
- **Provider API keys live here** as `ENV_VAR_NAME → value` pairs; multi-key keyring adds `NAME#label` slots, mirrors active key to the bare env name (`keyring.go:14-104`). Lookup vault-first-then-env (`ChainLookup` @360). `KeyringList` returns only label/active/last-4, never values.
- **Encryption on by default (machine-bound, M934)** (`encrypt.go`, `machine.go`): AES-256-GCM envelope (random 12-byte nonce + 32-byte salt per save); **PBKDF2-HMAC-SHA256 @ 200,000 iters** (`encrypt.go:100,296-321`); decrypt refuses `kdf_iter < 100000` (downgrade guard @213). Passphrase chain (`machine.go:77-85`): `AGEZT_VAULT_PASSPHRASE` → machine+user-bound key (`SHA-256("...v1|"+machineID+"|"+uid)`, unless `AGEZT_VAULT_AUTOENCRYPT=off`) → `""` plaintext. Plaintext vaults still load (`isEncryptedVault` @137).
- **Honest limit (documented, `machine.go:19-23`):** machine key only protects the file *leaving the machine*; **any same-user local process can re-derive it**. Real secrecy requires `AGEZT_VAULT_PASSPHRASE`.
- Gateway signing secret in `agentgw.secret` (0600); control-plane token in `runtime/control.token` (0600).

---

## 7. Prioritized High-Risk Areas for the Hunt Phase

1. **No real sandbox isolation.** `shell` + `code_exec` run as plain child processes with the daemon's full privileges (warden = `ProfileNone` everywhere). With default-allow, a single prompt-injection that reaches these tools = host RCE. Validate the approval-gate chain is the *only* thing standing between untrusted input and shell/code execution. (`warden.go:15-21,469`; `runtime.go:1565-1631`)
2. **`code_exec` network bypasses netguard.** Python/Node scripts can hit `127.0.0.1`, RFC1918, and cloud metadata `169.254.169.254` directly — full SSRF/credential-exfil from inside the "sandbox." (`codeexec/runtimes.go`, no netguard wiring)
3. **`coding` tool leaks the daemon environment.** `append(os.Environ(), …)` hands API keys / `AGEZT_*` to the spawned external coding agent un-scrubbed, unlike shell/code_exec. (`coding/coding.go:141`)
4. **Web console CSRF posture.** No CSRF token; in token-OR-session (default password) mode a cookie-only-authed cross-site POST is the theoretical gap. Verify every mutating route truly requires a non-cookie secret and SameSite covers all flows. (`webui.go:1064-1073`; `session.go`)
5. **`?token=` query-param auth on REST/OpenAI APIs** — token leaks into access logs, proxies, browser history, Referer. (`restapi.go:293`, `openaiapi.go:259`)
6. **Tunnel + non-loopback bind = silent public exposure.** `AGEZT_TUNNEL*` / non-loopback `*_ADDR` only warns, never blocks; combined with default-allow agent tools this exposes RCE to the internet behind a single bearer token. (`main.go:4307`, `4745`)
7. **Agentgw abstract unix socket has no OS perm gate.** Any local process in the namespace can reach it; all trust = the HMAC JWT. Confirm the per-install secret can't be downgraded to the ephemeral process-random key in a way a co-resident process could exploit, and re-audit the child-token subset/clamp logic. (`gateway.go:160-180,359-427`; `secret.go`)
8. **Vault same-user weakness.** Machine-bound encryption gives no protection against local same-user code; provider keys are recoverable by any process running as the user when `AGEZT_VAULT_PASSPHRASE` is unset. Assess whether the threat model documentation matches operator expectations. (`machine.go:19-23`)
9. **OpenAI `image_url` server-side fetch (SSRF).** Trace attacker-controlled http(s) `image_url` (`images []string`) from `openaiapi.go:417-472` into provider/kernel adapters — confirm the daemon doesn't fetch these URLs server-side without netguard. (`openaiapi.go`, provider adapters)
10. **Mailbox is daemon-global across tenants.** A tenant-scoped token reads/writes the shared board (`restapi/mailbox.go:25`) — verify intended, not a cross-tenant leak.
11. **Webhook (`/hooks/<workflow>`) per-workflow secret path** — console-token-free public route; audit secret generation/comparison and replay/timing. (`webui.go:678-731`)
12. **Prompt-injection guard precision** — per project memory this was reworked (PR #432, possibly unmerged); since it is a primary rail in front of shell/code_exec under default-allow, confirm the deployed guard logic and its causal-window gating actually fire. (`runtime.go:1578`)

---

*Recon complete — read-only, no source modified.*
