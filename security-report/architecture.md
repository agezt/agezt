# AGEZT Architecture Map (Security RECON)

Audit target: `D:\Codebox\PROJECTS\AGEZT`
AGEZT is a self-hosted autonomous multi-agent platform: a Go kernel + plugin set, administered over an HTTP control surface and a React/TS web console (built into `kernel/webui/dist`, `go:embed`-ded). Agents run LLM loops that can execute code, run shell, fetch URLs, use MCP servers, and talk over ~27 comm channels.

> **Design context (do NOT re-flag as a vuln):** AGEZT intentionally ships a **default-allow capability posture** — agents, `code_exec`, `shell`, network egress are deliberately max-capability / allow-by-default by owner decision (`memory/default-allow-posture.md`, `codeexec-capability-posture.md`). The *security boundaries that matter* are: the **control-plane admin token**, the **web console auth (token/password/session)**, the **agent-gateway token**, the **credential vault**, the **netguard SSRF guard**, and **any non-loopback / tunnel exposure**. This map points the hunt at those.

---

## 1. Tech stack

- **Language / runtime:** Go **1.26.4** (`go.mod:3`). 505 non-test Go files.
- **HTTP routing:** stdlib `net/http` + `http.ServeMux` everywhere. No third-party router/framework.
  - Web console: `kernel/webui/webui.go:558` (`Handler()` builds the mux).
  - Agent gateway: `kernel/agentgw/gateway.go:117` (uses Go 1.22 method-pattern routes, e.g. `"POST /v1/memory/write"`).
  - REST API: `kernel/restapi/restapi.go:166+`. OpenAI-compatible API: `kernel/openaiapi/openaiapi.go`.
- **Control plane is NOT HTTP:** it is a **custom newline-delimited JSON-over-TCP protocol** on loopback (`kernel/controlplane/server.go:249` binds `127.0.0.1:0`; one giant command switch at `server.go:506-1055`).
- **Crypto:** stdlib only (lean-deps policy excludes `x/crypto`). AES-256-GCM vault, hand-rolled PBKDF2-HMAC-SHA256 (`kernel/creds/encrypt.go`), HMAC-SHA256 JWT-like gateway tokens (`kernel/agentgw/token.go`).
- **Frontend (`frontend/package.json`):** React **19.2**, Vite **8**, TypeScript **6**, Tailwind **4**, Radix UI primitives, `@xyflow/react` (React Flow), lucide-react, CVA + tailwind-merge, self-hosted Inter font. Tests: Vitest + Playwright.

## 2. Application type & components

| Component | Path | Role | Network surface |
|---|---|---|---|
| Control-plane daemon | `cmd/agezt/main.go` (`func main`) | The kernel host: runs agents, schedules, channels, all HTTP residents | loopback TCP control plane + optionally web/REST/OpenAI/webhook/channel HTTP servers |
| CLI | `cmd/agt/main.go` | Operator client; talks the control-plane protocol; mints gateway tokens; `agt vault` | local |
| Agent gateway | `kernel/agentgw/` | HTTP API that **agent subprocess code** calls (eventbus/memory/log/config + token mint) | Unix socket by default (`@agezt/agentgw.sock`), **can be TCP** |
| Web console | `kernel/webui/` | Embedded React SPA + thin proxy to control-plane commands | HTTP, default `127.0.0.1:8787` |
| REST API | `kernel/restapi/` | Native REST (`/api/v1/*`) incl. mailbox | HTTP, opt-in `AGEZT_REST_ADDR` |
| OpenAI-compatible API | `kernel/openaiapi/` | Drop-in `/v1/chat/completions` etc. that runs the governed loop | HTTP, opt-in `AGEZT_API_ADDR` |
| SDKs / plugins | `sdk/`, `plugins/` | Providers, 28 tools, ~27 channels, builtin skills/market | n/a |

## 3. Entry points (where external / untrusted input enters)

### 3a. Web console HTTP (`kernel/webui/webui.go`) — primary admin attack surface
Route registration: `Handler()` at `webui.go:558`. Auth wrappers: `auth()` (`webui.go:976`), `secure()` (no token, `webui.go:966`), `shellAuth()` (`webui.go:992`).

- **Token-gated (`auth`)** — the bulk:
  - `apiRoutes` (read-only proxies) — `webui.go:104-172` (~50 GET routes).
  - `readArgsRoutes` (read with allowlisted query args) — `webui.go:186-268`.
  - `writeRoutes` (POST-only, query-arg mutations) — `webui.go:273-389` (halt/resume/decide, agent lifecycle, edict set_level/deny, provider keys activate/remove, mcp attach/detach, etc.).
  - `jsonRoutes` (POST JSON body mutations) — `webui.go:398-540` (agents add/edit/capabilities, config/set, routing/set, provider/keys/add, channel/account/set, council/conductor ask, workflows save/run, **provider/probe with key in body**, redact/test, etc.).
  - SSE streams: `/events` (`auth`, `webui.go:578`, whole bus firehose), `/api/run` (`runStreamProxy`, `webui.go:592`), `/api/plan/run`, `/api/toolbox/install`, `/api/market/install|uninstall`, `/api/transcribe`, `/api/artifact/raw`.
- **PUBLIC / token-free (`secure`), no data but worth scrutiny:**
  - `/` SPA shell (`shellAuth`, `webui.go:564`) — served credential-free when a console password is configured.
  - `/api/authmeta`, `/api/login`, `/api/logout` (`webui.go:568-570`) — the token-less password door.
  - `/assets/`, `/favicon.ico` — static bundle.
  - **`/hooks/<workflow>`** (`webui.go:608`, `handleWorkflowHook` at `:674`) — the **one deliberately console-token-free write path**; auth is the per-workflow secret (`X-Agezt-Secret` header or `?secret=`), verified constant-time by the control plane. Fires a workflow → runs an agent. POST, 256 KiB body cap.
  - **`/oauth/callback`** (`webui.go:612`, `handleOAuthCallback` at `:619`) — public; provider redirects here with `?code&state`; security rests on the unguessable `state`.

### 3b. Control-plane protocol (`kernel/controlplane/server.go`)
- Loopback TCP, ~250 commands (`server.go:506-1055`). Every webui/REST/CLI mutation funnels here.
- Auth at `handleConn` `server.go:485-504`: primary token (constant-time, `tokenIsPrimary` `:325`) authorizes everything; otherwise a named **tenant** + that tenant's token, restricted to `tenantTokenAllows()` commands and pinned to its tenant. 16 MiB pre-auth request cap (`:343`), per-conn panic recover (`:1072`).

### 3c. Agent gateway (`kernel/agentgw/gateway.go`) — agent-subprocess → kernel
- Endpoints `gateway.go:120-151`: eventbus subscribe/publish, memory write/delete/search, log read/write, agent list/query, **`POST /v1/token/create`** (mint child token), config get/set/audit. `GET /health` is **unauthenticated** (`:151`).
- Auth: `withAuth` (`:216`) requires `Authorization: Bearer` HMAC token; rate-limited per subprocess id; audited.
- Token model: HMAC-SHA256 JWT-like (`token.go`); alg pinned to HS256 (`token.go:99`, closes alg-confusion); child mint intersects caps with parent + clamps expiry/rate (`gateway.go:359-438`).
- **Secret resolution (`secret.go`):** `AGEZT_AGENTGW_TOKEN_SECRET` env → `<baseDir>/agentgw.secret` (0600) → fresh CSPRNG persisted O_EXCL. The former hardcoded `"change-me-in-production"` was removed (see `memory/agentgw-security-hardening.md`).
- **Listener can be Unix socket OR TCP** (`gateway.go:163-180`) — `tcp://host:port` makes the gateway network-reachable. Default is an abstract unix socket.

### 3d. Comm channels (`kernel/channel/` + `plugins/channels/*`) — untrusted inbound messages
- 27+ channels. Inbound handling pattern (telegram `plugins/channels/telegram/telegram.go:238` `handleInbound`):
  - **Allowlist enforced, fail-closed** (`telegram.go:267-289`): non-allowlisted sender is journaled and refused; photos/voice file refs only dereferenced for allowlisted senders.
  - Per-message panic isolation via `channel.Guard` (`kernel/channel/guard.go:21`).
- Webhook-style channels (slack/discord/teams/webhook/whatsappgw/…) expose their own HTTP listeners on `AGEZT_<CHAN>_ADDR`; signature/secret verification is per-channel — **worth auditing each** (HMAC check, replay, allowlist).

### 3e. MCP servers (`kernel/mcp/`)
- `client.go` (stdio: spawns a subprocess), `http.go` (remote Streamable-HTTP), `store.go`.
- **Remote MCP client relaxes netguard** with `AllowLoopback()`+`AllowPrivate()` (`http.go:75`) — an operator/agent-registered MCP endpoint URL can legitimately reach loopback + RFC1918. Registration is gated (admin via `/api/mcp/add`, or agent via mcp self-install tool), but this is an internal-network reach surface to note.

### 3f. CLI (`cmd/agt/`)
- Local operator tool. Notable security-relevant: `cmd/agt/vault.go` (encrypt/migrate vault), `cmd/agt/netguard.go`, channel/tool admin. Mints gateway tokens using the shared secret.

### 3g. Agent tools — untrusted LLM output → dangerous sinks (`plugins/tools/`, 28 tools)
| Tool | Sink | Containment |
|---|---|---|
| `shell` (`shell/shell.go`) | OS command exec via warden | `ProfileNone` on non-Linux = **full daemon privileges**; gated only by Edict trust ladder + hard-deny (`shell.go:13-17`). WorkDir scoped. |
| `code_exec` (`codeexec/codeexec.go`) | Writes+runs Python/Node/Deno | Per-call scratch dir, **scrubbed env (no AGEZT_\* / secrets)**, Deno FS jail, best-effort rlimits real only on Linux+namespace (`codeexec.go:12-26,44-59`). Gated by `code.exec` Edict, journaled. |
| `fetch` / `http` / `websearch` / `browser` | Outbound HTTP | netguard-protected client (default-deny internal/metadata); relaxed per `AllowLoopback`/`AllowPrivate` flags (`fetch.go:73-88`). |
| `file` (`file/file.go`) | FS read/write | **Workspace-confined**: `filepath.Abs`+`Rel` containment, refuses `..`, absolute-outside-root, and symlink escape (`file.go:4-14, 556, 638`); per-agent workdir rebasing. |
| `voice`, `send_media`, `notify`, `db`, `mcptool`, `forgetool`, `overseertool`, `peer`, `schedule`, `standingtool`, `workflowtool`, `config` | various | each routes through Edict + journal. |

## 4. Trust boundaries & authentication model

- **Control plane (loopback TCP):** primary admin **token** (32-byte hex, minted per daemon start, written to `<base>/runtime/token` 0600) — full authority. Optional **tenant tokens** with a restricted command allowlist. Constant-time compares throughout (`server.go:325`).
- **Web console (`kernel/webui/`):** three modes (`authorized()` `webui.go:1060`, `session.go`):
  1. **Token-only** (no password set) — pre-M817 default; `?token=` or `Authorization: Bearer`.
  2. **Password "alternative door"** (default when `AGEZT_WEB_PASSWORD` set) — token **OR** session cookie. Login is **token-free** (`webui.go:569`), constant-time password compare (`session.go:200`), failed-attempt lockout (8 fails → 5 min, `session.go:113-121`), HttpOnly/SameSite=Strict/Secure-if-TLS cookie (`session.go:211`), 12 h sliding TTL, in-memory sessions.
  3. **Strict** (`AGEZT_WEB_PASSWORD_STRICT=on`) — token **AND** session (two factors) for non-loopback exposure.
- **Public web routes (bypass token):** `/` shell, `/api/authmeta|login|logout`, `/assets/`, `/favicon.ico`, **`/hooks/<workflow>`** (per-workflow secret), **`/oauth/callback`** (state nonce). These are the explicit auth-bypass set — primary audit focus.
- **Agent gateway:** Bearer HMAC token, caps enforced, child-mint subset-only. `/health` unauthenticated.
- **REST / OpenAI API:** Bearer token (or `?token=`), constant-time (`restapi.go:278`), empty token fails closed; per-tenant token via `X-Agezt-Tenant`. `/healthz`/`/readyz` unauth; `/metrics` token-authed (exposes spend).
- **Default exposure is loopback.** Non-loopback bind prints a `[WARNING: not loopback]` (`main.go:4299`). `AGEZT_TUNNEL` / `AGEZT_TUNNEL_CMD` can expose any local service to the **public internet** (`main.go:4314`) — combined with token-only auth, a leaked `?token=` URL or password is the whole game.

## 5. Dangerous sinks

- **Command / process execution:** `plugins/tools/shell/shell.go` (host shell), `plugins/tools/codeexec/codeexec.go` (writes+runs code), MCP stdio (`kernel/mcp/client.go` spawns subprocess), tunnel supervisor (`main.go:4314`, runs `cloudflared`/`ngrok`/custom command), toolbox install (host package manager via `CmdToolboxInstall`).
- **File read/write:** `plugins/tools/file/file.go` (workspace-confined), artifact store, sandbox project files (`CmdSandboxFile`, path-confined per comment `webui.go:198`), skill resource reads (`CmdSkillReadFile`, "path-confined").
- **Outbound HTTP / SSRF surface:** `fetch`/`http`/`websearch`/`browser` tools, MCP remote http, provider calls, `provider/probe` + `whatsappgw/status|qr` (operator-supplied `url` POSTed → server-side request), webhook delivery, models.dev catalog sync (`CmdCatalogSync` with optional `url` override). All tool egress *should* go through `kernel/netguard` — verify each path actually uses it.
- **Template / HTML rendering:** `oauthResultPage` (`webui.go:645`) uses `fmt.Fprintf` into HTML with a hand-rolled `htmlEscape` (`:659`) — check the error message path is fully escaped.
- **SQL/DB:** `plugins/tools/db/` + Personal Data Lake (`CmdData*`) — check query construction.
- **Deserialization:** pervasive `json.Unmarshal` of untrusted bodies; gateway token payload (`token.go`); vault envelope (`encrypt.go`); workflow/plan/agent-profile JSON. Go's `encoding/json` is memory-safe, but type-confusion / oversized maps are worth a look (body caps exist: 1 MiB webui, 1 MiB gateway, 16 MiB control plane).

## 6. Secret / credential handling

- **Vault** (`kernel/creds/`): `creds.json` 0600 under base dir; **AES-256-GCM at rest, encrypted by default since M934** (`encrypt.go`). Passphrase from `AGEZT_VAULT_PASSPHRASE`. PBKDF2-HMAC-SHA256 200k iters (`encrypt.go:305`), min-iter floor 100k on decrypt (`:213`), legacy hmac-chain KDF still accepted for old vaults. Derived-key cache keyed by SHA-256(passphrase) (`:283`). Machine-bound auto-encrypt: `kernel/creds/machine*.go`.
- **Provider keyring** (`kernel/creds/keyring.go`): multi-key per provider env var, store-many-pick-active; values never leave daemon (`/api/provider/keys` returns last-4 only, `webui.go:241`).
- **Gateway signing secret** (`kernel/agentgw/secret.go`): per-install, `<base>/agentgw.secret` 0600 or env.
- **Console password:** a vault SECRET, bridged into env at boot, live-read per gate (`main.go:4271`).
- **Env vars:** `AGEZT_*` namespace; **scrubbed from `code_exec` child env** (`codeexec.go:16-17`). New `AGEZT_*` vars must be registered in controlplane `configEnvVars` or a guard test fails (`memory/config-envvars-guard.md`).
- **Secret scrubbing:** a live redactor scrubs secrets from journal/logs; `CmdRedactTest` / `/api/redact/test` dry-runs it (body-only so the probe never lands in a URL/access log, `webui.go:537`).

## 7. Existing security controls (intentional — don't re-flag as bugs)

- **netguard SSRF guard** (`kernel/netguard/netguard.go`): validates the **resolved IP at the dialer** on initial dial AND every redirect hop (defeats DNS rebinding + redirect-to-metadata); blocks loopback, RFC1918+ULA, link-local incl. `169.254.169.254`, CGNAT 100.64/10, NAT64/IPv4-compat IPv6 embeddings (`:119`), 0.0.0.0/8, multicast/broadcast. Opt-in relax per range. On-block journaling.
- **Constant-time token/password compares** everywhere (`subtle.ConstantTimeCompare`): control plane, webui token + password, REST.
- **Pre-auth body caps + panic recovery**: control plane 16 MiB + `recoverConn`; gateway 1 MiB + 1 MB header; webui 1 MiB JSON / 256 KiB webhook / 4 KiB login.
- **Allowlist discipline on the web proxy**: every webui route forwards only named args; no generic passthrough; mutations POST-only (`writeProxy` `:1253`).
- **Security headers** (`setSecurityHeaders` `webui.go:1021`): CSP `default-src 'none'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'`, X-Frame-Options DENY, Referrer-Policy no-referrer (keeps `?token=` out of Referer), nosniff. Set before auth so even 401s carry them.
- **CSRF posture:** writes are POST-only + token in query/Bearer (not auto-sent by a cross-site form); password session cookie is SameSite=Strict. (Note: a cross-site `fetch` cannot read the token, and SameSite=Strict blocks cookie attachment — but verify no state-changing GET exists.)
- **Channel allowlists, fail-closed**; per-message panic isolation (`channel.Guard`).
- **HITL approvals / Edict capability ladder / budgets**: `kernel/approval`, edict deny-lists + trust ceilings, per-run/per-agent cost caps, governor model routing.
- **Tenant isolation** (control plane + REST): separate kernels/journals, restricted command set.
- **JWT alg pinning** (`token.go:99`), child-token subset enforcement, rate-limit map eviction (CWE-770 bound).
- **Reaper / pulse / guardians**: autonomous self-monitoring (not a control boundary but reduces blast radius).

## 8. Top 10 highest-risk areas to hunt (ranked)

1. **Web console auth-bypass routes** — `/hooks/<workflow>` (`handleWorkflowHook` `webui.go:674`) and `/oauth/callback` (`:619`): workflow-secret comparison (constant-time? per-workflow? replay?), name-path handling (`strings.Contains(name,"/")` only), and whether a fired workflow can run arbitrary agent actions; `state` nonce generation/expiry/binding for OAuth callback.
2. **Non-loopback / tunnel exposure with token-only auth** — `AGEZT_WEB_ADDR` beyond loopback + `AGEZT_TUNNEL` (`main.go:4299,4314`). Token travels in `?token=` URL; assess token leakage (logs, history, referer despite no-referrer), and whether strict mode is actually required/enforced when exposed.
3. **Agent gateway when bound as TCP** (`gateway.go:163-180`) — default unix socket is local, but `tcp://` makes the HMAC-token API network-reachable; review token-secret distribution, `/health` unauth info, child-mint cap-escalation edge cases, and whether RunID inheritance can be abused across runs.
4. **SSRF via operator/agent-supplied URLs that may bypass netguard** — `provider/probe`, `whatsappgw/status|qr`, `catalog/sync` (url override), MCP remote http (deliberately AllowLoopback+AllowPrivate, `mcp/http.go:75`), webhook delivery. Confirm each constructs its client through netguard; hunt any `http.Get`/`http.DefaultClient`/raw `http.Client` on a user-controlled URL.
5. **Vault & passphrase handling** — `kernel/creds/encrypt.go`: KDF correctness (hand-rolled PBKDF2), legacy-KDF downgrade acceptance, kdf-cache key collision risk, passphrase sourcing/exposure in env + process listing, machine-bound key derivation (`machine*.go`), and plaintext-vault auto-detection (`isEncryptedVault`).
6. **Per-channel inbound HTTP webhook verification** — slack/discord/teams/webhook/whatsappgw/line/feishu/wecom/dingtalk/zalo listeners on `AGEZT_<CHAN>_ADDR`: HMAC signature check presence/constant-time, replay protection, allowlist enforcement parity with telegram, and unauthenticated-sender → agent-drive paths.
7. **Shell / code_exec containment claims vs reality** — on non-Linux (this is a Windows-primary repo) `shell` runs at `ProfileNone` (full daemon privileges). Verify Edict hard-deny is the only gate and cannot be bypassed via auto-approve caps (`runtime.WithAutoApproveCapabilities`, `server.go:1690`), per-run `tools` override, or agent-self-edit (self-repair `streamRun`).
8. **HTML / message injection in operator-facing render paths** — `oauthResultPage` `htmlEscape` completeness (`webui.go:659`), and any place untrusted text (channel messages, tool output, agent names) reaches the SPA without the CSP catching it (CSP allows `img-src data:` + `style-src 'unsafe-inline'`).
9. **Authorization gaps inside the control-plane command switch** — 250 commands (`server.go:506`); confirm tenant-token allowlist (`tenantTokenAllows`) cannot reach destructive/global commands (agent remove/edit, edict set_level, provider keys, config set, market install, shutdown) and that `tenant` arg pinning (`:498-503`) is honored by every handler that reads it.
10. **Personal Data Lake / DB tool query construction & path-confined file reads** — `plugins/tools/db/`, `CmdData*` handlers, `CmdSandboxFile`/`CmdSkillReadFile` "path-confined" claims (verify the confinement against `..`, absolute paths, symlinks, and Windows path quirks like `\\`, drive letters, alternate streams).

---

### Quick reference — route-registration files
- `kernel/webui/webui.go` (`Handler()` @558; route tables @104/186/273/398)
- `kernel/controlplane/server.go` (command switch @506)
- `kernel/agentgw/gateway.go` (`Listen()` @117)
- `kernel/restapi/restapi.go` (`Handler()` @166)
- `kernel/openaiapi/openaiapi.go`
- `cmd/agezt/main.go` (HTTP residents: `buildWebUI` @4182, `buildOpenAIAPI` @4686, `buildRESTAPI` @4749, `buildTunnel` @4314, channel listeners @2567+)
