# AGEZT — Security Architecture Map (Phase 1a Recon)

**Target:** `D:/Codebox/PROJECTS/AGEZT`
**Commit:** `e0041337` (branch `main`)
**Produced by:** `sc-recon`
**Method note:** Every claim below is derived from source with a `file:line` citation. Where a
comment, package doc, or design note asserts a guarantee the code does **not** implement, both are
recorded and the divergence is flagged as **`⚠ DIVERGENCE`**. This codebase has a documented,
repeated history of exactly that failure mode, so those flags are the highest-signal output here.

---

## 1. Technology Stack Detection

### Languages (tracked files, `node_modules` / `frontend/dist` / `.git` excluded)

| Language | LOC | Files | Role |
|---|---:|---:|---|
| Go | 50,250 | 1,572 | Kernel, daemon, CLI, plugins, tools — the entire security-relevant server |
| TSX (React) | 81,310 | 282 | Web console SPA (`frontend/src/views` = 150 views) |
| TypeScript | 27,909 | 168 | SPA libs (`frontend/src/lib`), TS SDK (`sdk/typescript`), e2e |
| Python | 3,601 | 27 | `sdk/python` client SDK + examples |
| Shell | 1,946 | 24 | `scripts/`, `install.sh`, CI helpers |
| Rust | 1,955 | 6 | `sdk/rust` client SDK |
| PowerShell | 1,175 | 9 | `dev.ps1`, `install.ps1`, ops scripts |
| JavaScript | 82 | 117 | Config/shims only (most `.js` are tiny) |

Go is ~65% of security-relevant logic despite the TSX line count (the SPA is view-heavy and
`.tsx` totals include large declarative view files).

### Frameworks / libraries

- **Go HTTP:** stdlib `net/http` + `http.ServeMux` only. No gin/echo/chi/fiber. A thin in-house
  router wraps ServeMux: `kernel/httpserver/router.go:44`.
- **Go dependencies (deliberately minimal)** — `go.mod:5-13`:
  `github.com/btcsuite/btcd/btcec/v2` (secp256k1), `github.com/coder/websocket`,
  `github.com/emersion/go-imap/v2` (inbound email), `golang.org/x/net`,
  `lukechampine.com/blake3`. Indirect: `emersion/go-message`, `go-sasl`, `decred` crypto,
  `klauspost/cpuid`, `golang.org/x/sys`.
- **Frontend** — `frontend/package.json:18-32`: React 19.2, Vite 8, Tailwind 4,
  Radix UI (tabs/tooltip), `@xyflow/react` (Flow Studio canvas), `@monaco-editor/react`
  (in-browser code editor), `lucide-react`. Overrides pin `dompurify ^3.4.11` and
  `undici ^7.28.0` (`frontend/package.json:50-53`) — note `dompurify` is an *override* not a
  direct dependency, i.e. it arrives transitively.
- **Build tooling:** `npm` (not pnpm) for the frontend — both `package-lock.json` and
  `pnpm-lock.yaml` exist in `frontend/`, which is a lockfile-drift hazard.

### Databases / persistence

**There is no SQL database and no ORM anywhere in the tree.** Persistence is bespoke and
file-based:

- `kernel/journal/` — append-only, hash-chained (BLAKE3) event log; the system of record.
- `kernel/jsonstore/`, `kernel/state/` — JSON documents on disk.
- `kernel/datalake/` — the "Personal Data Lake" collections (`/api/data/*`).
- `kernel/creds/` — credential vault.
- Base dir resolution: `internal/paths/paths.go:20-30` — `$AGEZT_HOME`, else
  `<user-home>/.agezt`.

Consequence: **no SQL injection class exists**; the equivalent risk surface is path traversal,
JSON deserialization into `any`, and the command/tool execution layer.

---

## 2. Application Type Classification

AGEZT is a **self-hosted autonomous multi-agent daemon** — a monolith that simultaneously is:

| Type | Evidence |
|---|---|
| Web Application | React SPA go:embed-ed into the binary, `kernel/webui/embed.go`, served at `/` (`kernel/webui/webui.go:775`) |
| REST API | `kernel/restapi/restapi.go:207-239` (`/api/v1/*`) |
| OpenAI-compatible API | `kernel/openaiapi/openaiapi.go:200-212` (`/v1/chat/completions`, `/v1/responses`, `/v1/models`, `/v1/audio/transcriptions`) |
| Webhook receiver | `kernel/webui/webui.go:860` (`POST /hooks/<workflow>`) + ~20 per-channel inbound listeners under `plugins/channels/` |
| CLI tool | `cmd/agt` — 102 non-test Go files, one per command group |
| Daemon / service | `cmd/agezt` — `main.go` is 3,921 lines |
| Agent gateway (IPC/HTTP hybrid) | `kernel/agentgw/gateway.go:135-166` |
| Multi-tenant service | `kernel/tenant/tenant.go:214` — per-tenant token + isolated kernel/bus |
| SDK publisher | `sdk/typescript`, `sdk/python`, `sdk/rust` |

The defining security property: **the daemon's purpose is to execute LLM-directed actions on the
host** — shell, filesystem, HTTP, package installs. Every entry point below is therefore, at some
depth, a path to arbitrary host action mediated only by the Edict policy engine.

---

## 3. Entry Points Mapping

### 3.1 HTTP surfaces — bind, default state, credential gate

| # | Surface | Bind | Default | Credential gate | Registration |
|---|---|---|---|---|---|
| 1 | **Web console** | `AGEZT_WEB_ADDR`, default `127.0.0.1:8787` | **ON** (opt-**out**) | console token *OR* password session (see §5) | `cmd/agezt/httpsurfaces.go:76-113` |
| 2 | **OpenAI-compat API** | `AGEZT_API_ADDR` | **OFF** (unset = disabled) | Bearer, admin token or per-tenant token | `cmd/agezt/httpsurfaces.go:424-427` |
| 3 | **Native REST API** | `AGEZT_REST_ADDR` | **OFF** | Bearer; `/healthz`+`/readyz` public | `cmd/agezt/httpsurfaces.go:484-487` |
| 4 | **Agent gateway** | abstract unix `@agezt/agentgw-<8hex>.sock`; `AGEZT_AGENTGW_SOCKET` overrides | **ON, unconditionally** | own `withAuth` (HS256 token); `GET /health` unauth | started `kernel/runtime/runtime.go:939-945`; routes `kernel/agentgw/gateway.go:135-166` |
| 5 | **Control plane (IPC)** | **loopback TCP `127.0.0.1:0`** (ephemeral, hard-coded) | **ON** with daemon | token in request body, constant-time; token file 0600 | `kernel/controlplane/server.go:278`, `:347-360`, `:431-443` |
| 6 | **Provider OAuth listener** | `127.0.0.1:1455` (transient) | on demand | unguessable `state` | `kernel/controlplane/provider_oauth.go:65`, `:73` |
| 7 | **Channel inbound listeners** (15) | per-channel `AGEZT_<CH>_ADDR` | **OFF** (empty Addr ⇒ outbound-only) | per-channel HMAC/Ed25519/secret | e.g. `plugins/channels/webhook/webhook.go:151,162` |
| 8 | **Tunnel (egress exposure)** | `AGEZT_TUNNEL` / `AGEZT_TUNNEL_CMD` | OFF | n/a — publishes #1 to the internet | `cmd/agezt/httpsurfaces.go:293-343` |

**Surfaces 1, 4 and 5 are on by default. Surface 1 is not read-only; surfaces 4 and 5 have no
operator on/off switch at all.**

#### Control plane (surface 5) detail

- `net.Listen("tcp", "127.0.0.1:0")` — hard-coded, no configurable bind, **no unix-socket or
  named-pipe branch anywhere in the package** (`kernel/controlplane/server.go:278`).
- Handshake dir `<baseDir>/runtime/` at `0755`; `control.addr` + `control.token` at `0600`
  (`server.go:431-443`; names `protocol.go:1643-1644`). Token = 32 CSPRNG bytes hex, regenerated
  every start (`server.go:282-287`). `$AGEZT_TOKEN` overrides the file for clients
  (`kernel/controlplane/client.go:45-52`).
- The **entire request line is read before authentication**, bounded at 16 MiB with a 10-minute
  read deadline and per-connection panic recovery (`server.go:363-390`, `:487-495`, `:598-609`).
- Authorization boundary = the `0600` token file only. Any process running as the same user reads
  it and gets full daemon admin. On Windows `os.WriteFile(..., 0o600)` does not produce a
  restrictive ACL, so "0600" is weaker here than it reads.

#### Agent gateway (surface 4) detail

- Started unconditionally by a bare `go agentGW.Listen(...)` inside `Kernel.Open`
  (`kernel/runtime/runtime.go:939-945`). **There is no enable flag**; a listen failure is only
  `slog.Error`'d and boot continues.
- Default is a **Linux abstract-namespace unix socket** (`kernel/agentgw/gateway.go:73-91`), which
  has **no filesystem permissions at all** — any local process in the same network namespace can
  `connect()`.
- `kernel/agentgw/sockopt_unix.go:11-20` sets **only `SO_REUSEADDR`** (justified in-comment by
  `go test -count=N` ergonomics); `sockopt_other.go:11-13` is a no-op. There is **no `chmod`, no
  `umask`, no `SO_PEERCRED`, no peer-credential check anywhere**. The bearer token is the sole gate.
- **Transport dispatch falls through to TCP** (`gateway.go:186-202`): `@…` → abstract unix,
  `unix://…` or absolute path → unix, **anything else → `lc.Listen(ctx, "tcp", g.sockPath)`**. So
  `AGEZT_AGENTGW_SOCKET=0.0.0.0:9000` yields a **plaintext HTTP gateway on all interfaces with no
  TLS** — and the settings schema advertises exactly this (`kernel/settings/schema.go:467`).
- On **Windows** the default abstract path is treated as a filesystem AF_UNIX path in a
  nonexistent directory → bind fails → gateway silently absent unless `AGEZT_AGENTGW_SOCKET` is
  set, at which point TCP is the only working option.
- Token model: home-rolled HS256 JWT with `alg`/`typ` pinned before signature verification
  (`token.go:97-112`), `hmac.Equal` (`:121`), `iss`/`aud` pinned (`:20-24`, `:136-138`), expiry
  checked (`:141-143`). **No `kid`, no revocation** — a leaked token is valid until `exp`.
  `NewTokenManager` SHA-256-stretches a short secret rather than rejecting it (`token.go:42-48`).
- Secret: `$AGEZT_AGENTGW_TOKEN_SECRET` → `<baseDir>/agentgw.secret` (hex, `0600`, `O_EXCL`) →
  fresh CSPRNG (`secret.go:39-88`). No hardcoded key. But the same key is derivable by the CLI
  (`cmd/agt/token.go:236`), and `cmd/agt/token.go:102-113` mints from raw `--caps` with **no
  parent and no capability ceiling** — so anyone who can read `agentgw.secret` mints an
  arbitrary-capability token for any run id.
- `handleTokenCreate` (`gateway.go:381-460`) is correct for the *child* case: caps are intersected
  with the parent, expiry and rate clamped, `RunID` inherited. Rate limiting is per-`SubprocessID`
  (`gateway.go:290-329`) — but `SubprocessID` is caller-chosen at mint time, so a valid parent can
  reset its own bucket by minting fresh sub-IDs.

### 3.2 Web console route inventory (`kernel/webui/webui.go:748-869`)

Route policy is declarative via `httpserver.RouteOpts`. Tiers actually used:

- `publicRead` / `publicMutation` → `Tier: TierPublic` → **no auth middleware wrapped at all**
  (`kernel/httpserver/router.go:109` only wraps the authenticator when `opts.Tier != TierPublic`).
- `protectedRead` / `protectedMutation` / `jsonProtected` → `TierUser`, but the WebUI overrides
  the decision entirely with `RequestAuthorize` (`kernel/webui/webui.go:750-752`), so **tier
  granularity is collapsed** — every protected WebUI route is gated by the same `s.authorized(r)`.

**Public (no credential) routes on the default-on console:**

| Path | Method | Handler | Note |
|---|---|---|---|
| `/` (and all SPA deep links) | GET | `webui.go:775` | `shellAuth` — served credential-free whenever a password is configured |
| `/api/authmeta` | GET | `webui.go:779` | leaks whether a password gate exists |
| `/api/login` | POST | `webui.go:782` | 4 KiB cap, lockout after 8 fails / 5 min (`session.go:38-39`) |
| `/api/logout` | POST | `webui.go:783` | |
| `/assets/`, `/favicon.ico` | GET | `webui.go:789-790` | static bundle |
| `/hooks/<workflow>` | POST | `webui.go:860` | **console-token-free**; auth = per-workflow secret only |
| `/oauth/callback` | GET | `webui.go:867` | auth = unguessable `state` |

**Protected route families (counts derived from the route maps):**

- `apiRoutes` — 60 parameterless read proxies (`webui.go:192-268`).
- `readArgsRoutes` — 55 argument-taking read proxies (`webui.go:282-415`).
- `writeRoutes` — 89 query-arg **mutating** commands (`webui.go:420-548`).
- `jsonRoutes` — 91 JSON-body **mutating** commands (`webui.go:557-726`).
- Bespoke: `/api/plan/run`, `/api/run`, `/api/toolbox/install`, `/api/market/install`,
  `/api/market/uninstall`, `/api/rollback/apply`, `/api/transcribe`, `/api/tts`,
  `/api/artifact/raw`, `/api/files/{tree,raw,mkdir,rename,delete}`, `/api/sse-token`,
  `/events` (`webui.go:791-852`).

Highest-authority protected routes:

- `POST /api/run` (`webui.go:809`) — free-text intent → full governed agent loop with the
  complete tool set (shell, file, http, code exec). Accepts `auto_approve_caps` from the browser
  body (`webui.go:1079`), which flows to `runtime.WithAutoApproveCapabilities`
  (`kernel/controlplane/server.go:1369-1371`).
- `POST /api/config/set` (`webui.go:562`) — writes daemon settings and the **secret vault**.
- `POST /api/files/delete` / `rename` / `mkdir` (`webui.go:850-852`) — live filesystem writes.
- `POST /api/toolbox/install` (`webui.go:812`) — runs the **host package manager**.
- `POST /api/mcp/add` + `/api/mcp/attach` (`webui.go:682`, `:533`) — registers and spawns an
  arbitrary child process as an MCP server.
- `POST /api/pulse/probe` (`webui.go:448`) — registers a **command** to run every heartbeat.
- `POST /api/edict/set_level`, `set_mode`, `deny_rm` (`webui.go:499-502`) — mutate the policy
  engine itself from the browser.

> **⚠ DIVERGENCE 1 — "read-only" console.**
> `cmd/agezt/httpsurfaces.go:49-50` states the Web UI serves *"the SSE Live Monitor + read panels
> (SPEC-07) — token-authed and read-only (SPEC-06)"*. The code registers **180+ mutating routes**
> (`webui.go:798-803`) plus `/api/run`, `/api/files/*`, and `/api/toolbox/install`. The console is
> a full control plane, not a read-only monitor.

> **⚠ DIVERGENCE 2 — "no generic passthrough".**
> `kernel/webui/webui.go:23-24` states *"The write set is a fixed allowlist (writeRoutes); there is
> no generic passthrough."* In fact `jsonRoutes` (`webui.go:557`), `planRoute` (`webui.go:731`),
> and `runStreamProxy` (`webui.go:1077`) are three additional write paths not covered by that
> sentence, and `/api/run` forwards arbitrary free text into the agent loop — which is a generic
> passthrough by any practical definition.

> **⚠ DIVERGENCE 3 — "token-authed on every request".**
> `kernel/webui/webui.go:20` says the server is *"token-authed on every request"*. Seven route
> patterns are registered `TierPublic` and receive no authenticator wrapper at all
> (`webui.go:775-790`, `:860`, `:867`; enforcement gap at `kernel/httpserver/router.go:109`).

### 3.3 REST API routes (`kernel/restapi/restapi.go:207-239`)

| Path | Method | Tier | Note |
|---|---|---|---|
| `/healthz` | GET,HEAD | **Public** | `restapi.go:207` |
| `/readyz` | GET,HEAD | **Public** | `restapi.go:208` |
| `/metrics` | GET,HEAD | User | `restapi.go:214` — deliberately authed because it exposes spend |
| `/api/v1/health`, `/models` | GET | User | `restapi.go:215-216` |
| `/api/v1/runs` | POST | User | `restapi.go:217` — starts a governed run |
| `/api/v1/runs/`, `/artifacts`, `/artifacts/` | GET | User | `restapi.go:218-220` |
| `/api/v1/mailbox/*` (5) | GET/POST | **Admin** | `restapi.go:226-230` — admin-gated because the board has no tenant partition (V-011) |
| `/api/v1/update`, `/api/v1/update/apply` | GET/POST | **Admin** | `restapi.go:234-239` |

Tenant credentials: a `TierUser` route with a non-empty `X-Agezt-Tenant` header can be opened by
the *tenant's own* token (`kernel/httpserver/auth.go:70-78`), verified constant-time at
`kernel/tenant/tenant.go:214-222`. `TierAdmin` can never be opened this way
(`kernel/httpserver/auth.go:70`) — correctly implemented.

### 3.4 OpenAI-compatible routes (`kernel/openaiapi/openaiapi.go:200-212`)

All five routes are `TierUser`; there are **no public routes** on this surface. Body caps: 16 MiB
JSON (`openaiapi.go:58`), 25 MiB audio (`openaiapi.go:217`).

### 3.5 Agent gateway routes (`kernel/agentgw/gateway.go:135-166`)

⚠ This surface **does not use `kernel/httpserver.Router`** — it registers directly on a raw
`http.ServeMux` with a bespoke `g.withAuth` wrapper. It therefore does not inherit the shared
body caps, tier model, or route-policy introspection.

- Authed: `/v1/eventbus/{subscribe,publish}`, `/v1/memory/{write,delete,search}`,
  `/v1/log/{read,write}`, `/v1/agent/{list,query}`, `/v1/token/create`, `/v1/config*`
  (`gateway.go:135-162`).
- **Unauthenticated:** `GET /health` (`gateway.go:166`).
- `POST /v1/token/create` (`gateway.go:154`) is a token-minting endpoint — historically the
  worst hole in this codebase; verify its authority checks in Phase 2.

### 3.6 CLI (`cmd/agt`)

102 non-test command files. Commands reach the daemon through the control-plane IPC client, so
CLI authority == control-plane authority. Notable privileged commands by filename: `edict.go`,
`edict_overlay.go`, `execution_profile.go`, `backup.go`, `ha.go`, `doctor.go`, `configcenter.go`,
`token.go`, `catalog_sync.go`.

### 3.7 Scheduled / autonomous triggers (no operator in the loop)

Four independent autonomous executors, all wired in `cmd/agezt/main.go`:

1. **Cadence / schedules** (`kernel/cadence/`) — persistent store, 10 s-resolution ticker
   (`cadence.go:35`), `MinInterval` 1 s (`:33`). Started `main.go:3301`, `:3318`; fires
   `RunAssured` / `RunWithRetry` / `RunWith` (`main.go:3283-3289`).
   **⚠ No trust ceiling is applied.** `WithMaxCost` is set (`main.go:3332`) but `WithTrustCeiling`
   never is — it appears only at `main.go:2899` (standing) and `main.go:3029` (resume). An
   unattended scheduled run therefore inherits the global `DefaultLevels()` posture, which is
   all-L4-allow (§5.2). The 1-hour per-firing backstop is disableable via
   `AGEZT_SCHEDULE_RUN_TIMEOUT=off` (`main.go:3309-3316`).
2. **Standing orders** (`kernel/standing/`) — `StartRunner` subscribes to `>` (every event) and
   fires matching enabled orders (`runner.go:48-80`), 15-min per-order cooldown (`runner.go:17`),
   self-trigger suppressed by skipping `standing.*`. Started `main.go:2930-2931`.
   Trust ceiling **is** applied: `standingTrustCeiling` (`main.go:2757-2774`) takes the lower of
   `max_trust` and the mode-implied level, defaulting bare `act_or_ask` to L2 (VULN-003 fix).
3. **Pulse** (`kernel/pulse/`) — **ON by default**, disabled only by `AGEZT_PULSE=off`
   (`cmd/agezt/boot_ops.go:105-107`), 60 s beat (`:109`). Initiative defaults to **`act`**:
   `ParseInitiative` returns `InitiativeAct` for empty *or unknown* input
   (`kernel/pulse/pulse.go:142-150`), read at `main.go:3586`. At `act` an actionable observation
   emits `pulse.initiative.act` (`kernel/pulse/engine.go:354`, `:517`).
4. **Guardians** (`plugins/builtinguardians/`) — seeded unconditionally at boot
   (`main.go:1553`), idempotent by slug.

**Guardians seeded ENABLED by default** (`builtinguardians.go:83-180`, armed by `seedTrigger`
`:350-379`), all at `TrustCeiling: "L2"`, $5/run, $10/day, `ToolDeny: ["memory"]`,
`System: true` (`:73-79`, `:206-218`):

| Slug | Trigger | Authority |
|---|---|---|
| `guardian-health` | 12 h interval | may call `overseer` halt/resume |
| `guardian-stuck` | 12 h interval | may cancel runs |
| **`guardian-code`** | **daily 03:00** (`:150`), `taskType: "coding"` | soul instructs it to **apply fixes** using `file` / `code_exec` and re-forge tools |
| `guardian-doctor` | `pulse.observer.system:reaper`, 8 h cooldown | may repair/pause agents |
| `guardian-budget` | `budget.exceeded`, `budget.cap.inert` | |
| `guardian-routing` | `provider.fallback`, `rate.limited` | may rewrite `AGEZT_TASK_MODEL_CHAINS` via the `config` tool |

Only `guardian-initiative` ships **disabled** (`:170`, flipped off at `:365-369`) — notably that
is the one bound to `pulse.initiative.act`, so the Pulse→autonomous-action bridge is dormant even
though Pulse itself defaults to `act`.

5. **Resume** (`kernel/resume/`) — not a file watcher; durable per-root-run tickets, atomically
   written, re-dispatched at boot. Security-positive: `Ticket.TrustCeiling` is persisted and
   **re-applied** on resume (`resume.go:67-71`, `main.go:3028-3032`), so authority is not silently
   regained across a restart.
6. **Unattended self-update** — when `AGEZT_UPDATE_CHECK_INTERVAL` is set,
   `startUpdateChecker` (`cmd/agezt/boot_ops.go:76-120`) drains, applies the new binary, and
   `os.Exit(0)`s for the watchdog to restart — **no operator confirmation**. Default is 0 = no
   background checker (`main.go:994`, `:1003`).

### 3.8 Streaming / SSE endpoints

- `GET /events` (`webui.go:791`) — the **entire kernel bus firehose** (`Subscribe(">")`,
  `webui.go:1555`). Auth accepts the ephemeral `?st=` token, a Bearer header, **or** the main
  console token in `?token=` as a "transition aid" (`webui.go:1409-1417`).
- SSE responses from `/api/run`, `/api/plan/run`, `/api/toolbox/install`, `/api/market/install`.
- Concurrency bounded by `kernel/streamlimit` — but **`max <= 0` disables the limiter entirely**
  (`kernel/streamlimit/streamlimit.go:30-32`), an opt-out guardrail.

---

## 4. Data Flow Map

### 4.1 Taint sources (all first-class)

1. **HTTP request bodies / query args / headers** — the 180+ mutating console routes, REST
   `/api/v1/runs`, OpenAI `/v1/chat/completions`.
2. **Webhook payloads** — `POST /hooks/<workflow>`; the raw body becomes
   `{{trigger.payload.body}}` in the workflow template engine (`webui.go:1017-1033`).
   Non-JSON bodies ride **verbatim** as a string (`webui.go:1020`).
3. **Inbound channel messages** — ~20 chat platforms (`plugins/channels/`), each converted to a
   `UnifiedMessage` and able to wake an agent.
4. **LLM-generated content and tool-call arguments** — *the dominant source in this
   architecture*. The model chooses tool names and authors every tool argument; those arguments
   land directly in shell commands, file paths, and outbound URLs. There is a prompt-injection
   guard (`AGEZT_PROMPT_INJECTION_GUARD`) but it is a heuristic, not a boundary.
5. **Agent-authored content** — skills, forged script tools (`/api/toolforge/draft`), workflow
   graphs (`/api/workflows/draft`), memory records. Agents can author code that later runs.
6. **Config / env** — 455 distinct `AGEZT_*` variables read across `cmd/`, `kernel/`, `plugins/`,
   `internal/`. Config Center + vault values are injected into the process env at boot
   (`cmd/agezt/main.go:214-218`), so a console `POST /api/config/set` mutates daemon env.
7. **Remote MCP servers and marketplace packs** — content fetched over the network and then
   trusted as tool definitions.

### 4.2 Processing / control points

- `decodeAllowedBody` (`webui.go:1692`) — drops unlisted body keys before forwarding.
- `historyTurns` (`webui.go:1233-1255`) — caps folded chat history at 40 turns.
- `sanitizeRelativePath` + `verifyResolvedWithinRoot`
  (`kernel/webui/files_route.go:109-133`, `:142`) — path-traversal chokepoint for the File
  Manager, including symlink resolution.
- `kernel/redact` — applied to every bus publish (`kernel/bus/bus.go:198`, `:247`) and to
  OpenAI-surface error strings (`kernel/openaiapi/openaiapi.go:47-52`).
- `kernel/edict` — capability policy (see §5).
- `kernel/netguard` — SSRF egress guard.
- `kernel/warden` — sandbox for code execution.

### 4.3 Dangerous sink inventory

#### (a) Process / shell execution

`kernel/warden/warden.go:319` — `exec.CommandContext(...)` is the **single mediated exec sink**;
all agent-facing execution funnels through it. Nil `Env` is coerced to *empty*, not inherited
(`warden.go:323-333`).

**⚠ `kernel/warden/cmdline_windows.go:8`, `:14-24`** sets `SysProcAttr.CmdLine` so
`cmd /S /C "<command>"` reaches `cmd.exe` **verbatim**, deliberately bypassing Go's argument
escaping. The raw LLM-authored command string is handed to the Windows shell unescaped.

LLM/attacker-influenceable command strings:

| Sink | Taint |
|---|---|
| `plugins/tools/shell/shell.go:268` | `Argv: {shellBin, shellArg, in.Command}` — **raw LLM tool-call argument → `cmd /C` / `sh -c`** |
| `plugins/tools/shell/shell.go:148,168,188,209` | same string interpolated into SSH / kubectl / modal / daytona argv |
| `kernel/executionprofile/ssh.go:64-70`, `k8s.go:47-53`, `modal.go:36-41`, `daytona.go:31-46` | `ShellQuote` is applied to the **workdir only**, never to `command` |
| `plugins/tools/codeexec/codeexec.go:~222`, `:322-336` | LLM-authored source written to disk then executed |
| `plugins/tools/codeexec/packages.go:48-62` | `pip install` with **LLM-supplied package names**; `validatePackages` (`:27-35`) blocks leading `-` (so no `--index-url=evil`) but not a malicious package |
| `kernel/mcp/client.go:113` | `exec.Command(command, args...)` where command+args come from the **`mcp` tool's `op=add`** (`plugins/tools/mcptool/tool.go:129-134` → `kernel/runtime/mcptool.go:120`). **No allowlist, no hash pin on the child binary** — the shortest LLM→RCE path in the tree |

Operator/control-plane controlled exec: `kernel/toolbox/toolbox.go:264` (host package managers,
reachable from the console via `kernel/controlplane/toolbox.go:60`), `kernel/tunnel/tunnel.go:235`,
`kernel/creds/aws.go:220` (credential_process — correctly gated by
`AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED`, no shell, no inherited env),
`cmd/agezt/httpsurfaces.go:259-263` (`rundll32 url.dll,...`), `cmd/agezt/watchdog.go:223`,
`cmd/agt/listen.go:123`, `kernel/plugin/pin.go:40`.

Well-guarded exec paths worth citing as the good pattern: `plugins/tools/acpagent/acpagent.go:246`
(slug-only resolution, `kernel/acpcatalog/acpcatalog.go:310-313` refuses to fall through to a raw
command) and `kernel/plugin/host.go:290` (BLAKE3-256 hash pin enforced at spawn and re-verified on
reload, `host.go:1017`). **No `syscall.Exec` / `ForkExec` anywhere.**

#### (b) Filesystem

Guards that exist and are load-bearing:

- `plugins/tools/file/file.go:760-799` (`resolve`), `:805-834` (`resolveNewWithinRoot`),
  `:845-855` (`entryEscapesRoot`), `:857-868` (`withinRoot`) — `filepath.Rel` + `EvalSymlinks`,
  with separate fixes for symlinked-parent writes (M253) and symlink-in-walk arbitrary read (M427).
- `kernel/webui/files_route.go:104-134`, `:142-185`, `:~196+` — lexical containment against
  `rootAbs+PathSeparator` (deliberately not bare `HasPrefix`, `:118`), symlink resolution, and a
  bespoke per-component `os.Readlink` walk (`verifyNoEscapingLinks`) that exists because on
  Windows `EvalSymlinks` returns a **directory junction unchanged** and `os.Lstat` reports
  `ModeIrregular` (`:187-195`). That junction walk is the only thing closing junction escapes on
  the owner's platform — a prime regression-test target.
- `plugins/tools/codeexec/artifacts.go:308-311` (`sanitizeRelFile`),
  `kernel/datalake/datalake.go:162-163` (`validName`), `cmd/agt/backup.go:519-530`.

Dynamic-path sinks: `kernel/webui/files_route.go:415` (`MkdirAll`), `:456` (`Rename`), `:502`
(`RemoveAll`) / `:507` (`Remove`) — all HTTP-parameter driven, all routed through
`resolveFileRoot`. Weaker spots to check in Phase 2: `plugins/tools/shell/shell.go:254-255`
(`filepath.Join(t.WorkDir, ...)` whose comment asserts escape-proofing happens *at the setter*,
not at the join) and `kernel/jsonstore/jsonstore.go:57` (`filepath.Join(dir, name)` with **no
traversal check** in `LoadFrom` — callers are internal today).

#### (c) Outbound HTTP

Netguard (§5.7) is dialer-level and correct where wired. **`plugins/builtintools/tools.go:100-106`
sets a default-ALLOW posture for any public host**; `AGEZT_HTTP_ALLOWED_HOSTS` is an *opt-out*
restriction, and `AGEZT_HTTP_ALLOW_PRIVATE=1` / `AGEZT_ALLOW_ALL=1` remove the SSRF floor
(`tools.go:128-140`). `browser.action` can additionally drive the operator's **persistent
logged-in browser profile** (`action.go:250-258`) or attach to a **remote CDP endpoint**
(`:259-267`).

`kernel/workflow/workflow.go:546-558` (`NodeHTTP`) validates only that the method is GET/POST and
the URL non-empty — and the shipped template at `kernel/workflow/templates.go:89` puts a
**webhook-payload-controlled URL** into an HTTP node.

#### (d) Template rendering / HTML output

- **No `text/template` or `html/template` anywhere in the Go tree.**
- `kernel/webui/webui.go:904-916` (`oauthResultPage`) and its byte-identical twin
  `kernel/controlplane/provider_oauth.go:237-249` are `fmt.Fprintf` into `text/html`, escaped by a
  hand-rolled 5-character replacer (`webui.go:918-921`, `provider_oauth.go:252-255`). Both embed an
  inline `<script>` — which the console CSP (`script-src 'self'`) would block.
- `kernel/webui/artifact_route.go:41-54` — `mime` comes from the query string and the stored mime
  is attacker-influenceable; `safeContentType` (`:65-72`) allowlists, inline-renderable types get
  `Content-Security-Policy: sandbox; default-src 'none'` (`:50`), everything else gets
  `Content-Disposition: attachment` (`:54`). Correct design.
- **React frontend: no `dangerouslySetInnerHTML`, no `eval`, no `new Function`, no
  `document.write`, no `innerHTML` outside two test assertions.** `components/Markdown.tsx:11-12`
  is a hand-rolled AST renderer whose leaves are React text nodes.
- ⚠ `frontend/src/views/Files.tsx:170` — `<iframe src={href}>` for PDF preview with **no
  `sandbox` attribute**, unlike its correct sibling `views/Artifacts.tsx:512-520`
  (`sandbox="" referrerPolicy="no-referrer"`).
- ⚠ `views/Channels.tsx:211`, `views/Models.tsx:362`, `views/Setup.tsx:223` —
  `window.open(r.authorize_url, "_blank", "noopener,noreferrer")` with **no scheme validation** on
  a server-supplied URL.
- ⚠ `frontend/package.json:51` declares `dompurify ^3.4.11` but **nothing in `frontend/src`
  references it** — an unused dependency, or a sanitizer someone expected in the path that isn't.

#### (e) Deserialization / dynamic code

- **No `encoding/gob`. No YAML unmarshal. No `json.Unmarshal` into bare `any` with type
  dispatch** (the only match is a fuzz harness, `kernel/controlplane/fuzz_test.go:38`). All
  decoding targets concrete structs.
- The workflow "expression evaluator" (`kernel/workflow/template.go:21-39`, `:43-67`, `:69-82`) is
  **deliberately not an expression language** — `{{dotted.path}}` lookups against nested
  `map[string]any`/`[]any` only, no pipes, no calls, no arithmetic, misses return `""`. This is the
  safest possible design for that surface.
- The dynamic-code surface is instead: `NodeCode` in a stored workflow
  (`kernel/workflow/workflow.go:559-567`), `tool_forge` (`plugins/tools/forgetool/tool.go:63-105`
  — `op=test` executes now under `CapCodeExec`; going live needs operator approval after a passing
  test), and `mcp` (arbitrary registered stdio command).

#### (f) Archive / extraction

**No `archive/zip` in the tree** — tar+gzip only, so no classic zip-slip surface. All three
extractors are guarded:

- `cmd/agt/backup.go:465-514` — `isAllowedBackupPath` (`:519-530`) + a second lexical
  `HasPrefix(target, cleanDest+sep)` at `:496`, `O_EXCL` at `:502`, non-regular entries (i.e.
  symlinks) skipped at `:483`.
- `plugins/tools/codeexec/artifacts.go:292-349` — the most thorough: `sanitizeRelFile` hard
  refusal (`:308-311`), symlinks dropped (`:317-321`), zip-bomb caps on file count / per-file
  bytes / total (`:323-332`), and `io.CopyN(f, tr, hdr.Size)` (`:342`) rather than trusting the
  stream.

Uploads: `kernel/webui/transcribe.go:34-35` correctly wraps `MaxBytesReader` **before**
`ParseMultipartForm`; the filename is never used as a path.

#### (g) SQL / datastore

**No `database/sql`, no driver, no `sql.Open`, no query strings anywhere — no SQL injection
surface exists.** (`sqlite3` appears only as an installable entry in
`kernel/toolbox/catalog.go:138-139`.) Persistence is `kernel/jsonstore`, `kernel/datalake`,
`kernel/state`, `kernel/artifact`, `kernel/journal`, `kernel/creds`.

### 4.4 Built-in agent tools (LLM tool-call arguments = taint source)

Registered in `plugins/builtintools/tools.go:44-88`. The dangerous capability each grants:

| Tool | Grants |
|---|---|
| `shell` | **Arbitrary OS command execution**; no isolation on Windows/macOS |
| `file` | Read/write/delete/search under the workspace root (traversal + symlink + junction guarded) |
| `http` | Outbound GET/POST to any public host by default |
| `browser.read` / `browser.action` (+10 verbs) | Fetch any URL; drive a real Playwright browser incl. the operator's logged-in profile |
| `web_search` | URL *discovery* — turns "no URL" into "any URL" for the fetch tools |
| `fetch` | Download arbitrary URL bytes and **persist them** into the artifact store |
| `config` | **Read/write daemon `AGEZT_*` settings and the credentials vault.** `op=set` is `CapConfigWrite`-gated and constrained to registry-known fields (`config.go:201-207`) — Phase 2 must verify whether `AGEZT_HTTP_ALLOW_PRIVATE`, `AGEZT_ALLOW_ALL`, `AGEZT_CODING_CMD` are registered fields |
| `artifacts` | list / read / **delete** stored artifacts |
| `db` | Create/drop collections and records in the Data Lake; mapped to `CapMemory` (allow-by-default) |
| `council`, `conductor`, `research` | Multi-model fan-out; `conductor` orchestrates sub-agents ⇒ transitively every other tool |
| `code_exec` | **Executes LLM-authored Python/Node/Deno**; `pip install` of LLM-named packages |
| `schedule` | Creates **persistent recurring triggers** — the agent grants itself future autonomous execution |
| `standing` | **Standing orders** — persistent instructions applied to future runs |
| `skill` | Install/manage skills (new instruction bundles) |
| `introspect` | Reads own config/tools/journal — recon surface |
| `overseer` | Supervisory control over other agents |
| `tool_forge` | Author/test/promote **new persistent script tools** callable by every agent |
| `mcp` | **Register + attach an arbitrary stdio MCP server** — highest-leverage LLM→RCE path |
| `workflow` | Author/run workflows containing `tool` and `code` nodes = stored replayable code execution |
| `workboard`, `board` | Shared task/message board writes |
| `notify` | **Sends messages out to real channels** — exfiltration channel |
| `send_media` | Sends artifact bytes to a channel target — **exfiltration of stored files** |
| `coding` | Spawns an external coding agent (Claude Code / Codex / Aider) on a git worktree |
| `acp_agent` | Drives an external ACP agent over stdio (slug allowlist blocks injection) |
| `homeassistant` | **Actuates physical devices** (locks, lights, switches) |
| `remote_run` | Delegates a run to a **peer node** — lateral movement across the mesh |
| `plugins` | Exposes external plugin binaries' tools (BLAKE3 pin-verified) |

**Three primitives let a single prompt injection outlive the run**: `standing` (standing orders),
`schedule` (recurring triggers), and `tool_forge` + `mcp` (permanent new tools). Treat persistence
as its own node in the threat model.

---

## 5. Trust Boundaries

### 5.1 Authentication model

Three independent credential systems:

**(a) Bearer tokens** — `kernel/auth/token.go`. `StaticVerifier` holds one admin token plus zero
or more user tokens, compared with `crypto/subtle.ConstantTimeCompare`
(`kernel/auth/token.go:68-73`). Fails closed on blank config (`:70-72`) and blank presentation
(`:50-52`). Tiers: `TierPublic < TierUser < TierAdmin` (`kernel/auth/tier.go:12-16`). Tokens are
minted per boot from `crypto/rand` (32 bytes → hex) and written 0600 under the base dir
(`kernel/auth/tokenfile.go:20-38`; callers `cmd/agezt/httpsurfaces.go:428-438`, `:488-498`).
The console token is **never** written to a file — it appears only in the boot banner URL
(`httpsurfaces.go:178`).

**(b) Console password session** — `kernel/webui/session.go`. In-memory session store, 32-byte
random id, 12 h sliding TTL (`session.go:33-34`, `:76-92`), `HttpOnly` + `SameSite=Strict`
cookie (`session.go:235-243`), constant-time password compare (`session.go:224`), lockout after
8 consecutive failures for 5 minutes (`session.go:38-39`, `:113-121`).

> **⚠ DIVERGENCE 4 / HIGH-VALUE FINDING — hardcoded default console password.**
> `cmd/agezt/httpsurfaces.go:230` defines `const defaultLoopbackWebPassword = "agezt"`, and
> `effectiveWebPassword` (`:232-244`) returns it whenever `AGEZT_WEB_PASSWORD` is unset,
> `AGEZT_WEB_PASSWORD_DEFAULT` is not explicitly disabled, and the bind address is loopback.
> Since the console is **on by default at `127.0.0.1:8787`** (`:82`), a stock `agezt` ships with a
> publicly-known password on its full control plane. The comment at `:119-121` frames this as a
> convenience ("so the operator can browse to localhost and change it in Setup"), but nothing in
> the code forces a change. Combine with the tunnel path (§5.6) and with `hostAllowed`'s
> acceptance of any IP literal (§5.4).

**(c) Per-tenant tokens** — `kernel/tenant/tenant.go:214-222`, constant-time, admin-only routes
correctly unreachable (`kernel/httpserver/auth.go:70`).

**(d) Per-workflow webhook secrets** — the only credential on `POST /hooks/` (`webui.go:1008-1011`),
accepted from `X-Agezt-Secret` **or** `?secret=` (i.e. it will land in access logs), verified
constant-time inside the control plane.

**(e) Agent-gateway tokens** — `kernel/agentgw/token.go`, `secret.go`; minting endpoint at
`gateway.go:154`.

No password hashing exists anywhere (the console password is compared in plaintext against the
env value — acceptable for a single-operator shared secret, but it means the value sits in the
process environment and in the config store).

### 5.2 Authorization / capability model — Edict (`kernel/edict/`)

Two orthogonal axes evaluated in fixed order inside `DecideWithCeiling`
(`kernel/edict/edict.go:754`): (1) hard-deny floor (`:765-779`), (2) trust-level lookup (`:782`),
(3) ceiling clamp, tighten-only (`:804-807`), (4) level→decision (`:810-855`). The ladder is
`LevelDeny`(L0) … `LevelAllow`(L4) (`edict.go:227-238`). Ask-class levels (L1–L3) are folded by
`AskPolicy` (`:319-332`): `AskAllow` → Allow, `AskDeny` → Deny, `AskPrompt` → Deny +
`RequiresApproval` (fail-closed for callers that ignore the flag).

36 capabilities are defined (`edict.go:39-221`, enumerated `:671-683`): `shell`, `file.*`,
`http.get/post`, `provider.call`, `delegate`, `coding`, `acp_agent`, `remote_run`, `notify`,
`homeassistant.*`, `browser.*`, `memory`, `world`, `web.search`, `research`, `schedule`,
`runs.read`, `standing`, `board`, `workboard`, `skill`, `introspect`, `oversee`, `code.exec`,
`tool.forge`, `mcp.install`, `mcp.call`, `config.read/write`, `workflow.manage`, `market.install`.

> **⚠ DIVERGENCE 5 — THE LOAD-BEARING ONE. "default-deny" is false for every known capability.**
> `kernel/edict/toolmap.go:15-16` states *"Unknown tool names fall back to Capability(name), which
> is default-denied unless the caller explicitly granted a level."* `kernel/edict/edict.go:506-507`
> says *"secure-default: an unknown capability is refused"*, and `kernel/runtime/policy.go:28`
> reads the same way. Those are true only for **unmapped** capability names. For every **known**
> capability the actual default is `LevelAllow` (L4):
> ```go
> // kernel/edict/edict.go:634-640
> func DefaultLevels() map[Capability]TrustLevel {
>     levels := make(map[Capability]TrustLevel, len(AllCapabilities()))
>     for _, c := range AllCapabilities() { levels[c] = LevelAllow }
>     return levels
> }
> ```
> So `shell`, `code.exec`, `file.delete`, `mcp.install`, `market.install`, `http.post` all default
> to **silent allow, no prompt**. `edict.go:616-633` is honest about this ("MAX-AUTONOMY posture,
> M814") — three other sites were simply not updated. **Out of the box, Edict denies nothing
> except the 16 hard-deny substrings.**

| Case | Default | Line |
|---|---|---|
| Known capability, no operator config | **L4 Allow** | `edict.go:634-640` |
| Unknown / unmapped capability | Deny | `edict.go:790-797` |
| Unknown capability + `UnknownAllow` | L4 Allow | `edict.go:798` |
| `AskPolicy` with `AGEZT_APPROVAL_MODE` unset | `AskAllow` | `cmd/agezt/main.go:3851` |
| `AGEZT_APPROVAL_MODE` typo'd | silently `AskAllow` | `cmd/agezt/main.go:3853-3857` |

`AGEZT_ALLOW_ALL=1` (`main.go:284-299`) additionally sets `UnknownAllow = true`; only the exact
string `"1"` is honoured (`daemonconfig_test.go:148-151`), and a stderr warning is printed
(`main.go:299`).

> **⚠ DIVERGENCE 6 — the hard-deny floor covers one tool, not one behaviour.**
> `edict.go:622-624` claims the defaults do not relax *"the F4 hard-deny strings (fork bombs,
> rm -rf /, raw-device writes)"*. The 16 rules (`edict.go:645-667`) are real — `:(){:|:&};:`,
> `rm -rf /`, `mkfs`, `wipefs`, `dd if=`, `of=/dev/sd…`, `shutdown -`, `poweroff`, `reboot`,
> `format-volume` — but **every one carries `AppliesTo: []Capability{CapShell}`**. The identical
> destructive command routed through `code_exec` (→ `CapCodeExec`, `toolmap.go:136`), `conductor`
> (`toolmap.go:144`), a `forge_*` script (`toolmap.go:25-27`), or an `mcp_*` bridged tool
> (`toolmap.go:30-32`) **matches no rule at all**. Matching is also plain case-insensitive
> substring (`edict.go:373-378`), so `rm -rf  /`, variable expansion, or base64+eval evade it even
> on the shell path.

**Where Edict is actually enforced — only three call sites:**

| Site | Covers |
|---|---|
| `kernel/agent/run_tools.go:189-214` | every model-issued tool call |
| `kernel/toolexec/toolrun.go:60-87` | direct operator/CLI tool runs |
| `kernel/runtime/workflowrun.go:557`, `:745` | workflow tool nodes |

The hook itself is `kernel/runtime/policy.go:84`, wired at `kernel/runtime/loopconfig.go:83`.

**What bypasses Edict:**

1. **⚠ Nil-policy fail-OPEN.** `kernel/agent/run_tools.go:188` initialises
   `verdict := PolicyVerdict{Allow: true, …, Reason: "no policy configured"}` and only overwrites
   it `if s.cfg.Policy != nil`. In-tree the sole `agent.LoopConfig{}` literal
   (`loopconfig.go:73`) always sets `Policy`, so the daemon is safe — but any SDK or embedder
   constructing a `LoopConfig` without `Policy` gets a **silently ungated tool loop**.
2. `kernel/runtime/council.go:298` — `councilSearch` calls `tool.Invoke` on `web_search`
   **directly, with no policy hook**. The outer `council` tool is gated on `CapDelegate`
   (`toolmap.go:170`); the nested web fetch is gated on nothing.
3. `kernel/plugin/host.go:920` — `tool.Invoke` inside the plugin host's own dispatch path.
4. **In-kernel model fan-out** — `toolmap.go:138-144` states this outright: the conductor's
   verifier *"RUNS the worker's code through the injected sandbox backend. That execution is an
   in-kernel call, so it never passes back through this engine for a second decision."*

**Tool→capability resolution** (`kernel/runtime/policy.go:72-82`): (1) the tool's declared
`ToolDef.Capability`, (2) the plugin-manifest overlay `k.toolCaps`, (3) the name switch at
**`kernel/edict/toolmap.go:20-219`** (the mapping table). Prefix rules `forge_*`→`code.exec` and
`mcp_*`→`mcp.call` (`toolmap.go:25-32`) do fire, because forged and bridged tools leave
`ToolDef.Capability` zero (`kernel/runtime/scripttool.go:243-265`,
`kernel/runtime/mcptool.go:319-341`).

Permissive mapping fallbacks: `artifacts`→`file.read` (`toolmap.go:79`), `config`→`config.read`
(`:194`), `homeassistant`→`homeassistant.read` (`:206`), and **`http` → `http.get` for any method
that is not POST** (`:215`) — so PUT/DELETE/PATCH would map to the *read* axis. Bounded today only
because the http tool's schema is `enum:["GET","POST"]` and it rejects others at
`plugins/tools/http/http.go:193`; the mapping itself is unsound.

Layered gates atop Edict (`kernel/runtime/policy.go`): agent tool allow/deny (`:115`), noise policy
(`:122`), epistemic escalation (`:152`), intent/regret gating (`:158`), prompt-injection guard
(`:169-180`), session auto-approve (`:187-192` — flips `Allow=true` without re-consulting Edict;
hard-denies cannot reach that point, per `:186`).

> **⚠ HIGH-VALUE FINDING — HITL approval is auto-granted by default.**
> `selectAutoApproveCapabilities` (`cmd/agezt/main.go:3888-3918`) switches on
> `AGEZT_AUTO_APPROVE_CAPS`. The **empty string** — i.e. the variable unset, the default install —
> falls into `case "", "all", "1", "true", "yes", "on":` (`main.go:3893`) which auto-approves
> **every known capability**. Only an explicit `off|0|false|no|none` disables it (`:3891`). The
> result is wired daemon-wide at `main.go:434` → `main.go:515`
> (`Config.AutoApproveCapabilities`) → `kernel/runtime/runtime.go:1970-1971`. Net effect: the
> Edict "ask" level — the human-in-the-loop gate — **never prompts out of the box**. The banner
> text at `:3898` even describes this as `"on (N known capabilities…)"`, so it is intentional, but
> it means every Phase-2 threat model must assume `ask == allow`.

Per-run escalation: `/api/run` accepts `auto_approve_caps` in the request body
(`webui.go:1079` → `kernel/controlplane/server.go:1369-1371`), and the control plane additionally
accepts `prompt_injection_trust` to downgrade the injection guard to warn-only
(`server.go:1379-1381`).

### 5.3 Rate limiting

| Where | Scope | Citation |
|---|---|---|
| `/hooks/` webhook | 60/min + 30 burst per `workflow\|source-IP`, 4096 bucket cap | `webui.go:930-982`, `:1003` |
| Console login | 8 fails → 5 min lockout, daemon-global | `session.go:38-39` |
| Agent gateway | per-token sliding window, 4096 entries | `kernel/agentgw/gateway.go:287-323`, `types.go:130-175` |
| Config Center | per-agent + per-key per-minute | `kernel/configcenter/access.go:109-122` |
| SSE concurrency | per client key; **disabled when max<=0** | `kernel/streamlimit/streamlimit.go:30-32` |
| Tenants | `AGEZT_TENANT_RATE_PER_MIN` | env-configured |

**Authenticated run endpoints are deliberately unthrottled** — this is a recorded owner decision
(default-allow posture), not an oversight. Budget ceilings (`AGEZT_TENANT_DAILY_CEILING`,
`/api/budget_set`) are the backstop.

### 5.4 CSRF / Host / Origin

All three are applied *outside* the router so they also cover public routes and 401s
(`kernel/webui/webui.go:871-873`, `:1260-1273`):

- **Host allowlist** — `hostAllowed` (`webui.go:1328-1343`). `localhost` always passes; **any IP
  literal that is not unspecified passes unconditionally** (`:1337-1339`); DNS names must be
  explicitly registered. So DNS-rebinding protection exists only for names, and a LAN-reachable
  console accepts `Host: <any-ip>`.
- **Origin check** — `sameOriginMutation` (`webui.go:1345-1362`). Rejects
  `Sec-Fetch-Site: cross-site`; compares `Origin` host:port to `r.Host`. **A missing `Origin`
  header returns `true`** (`:1355-1357`) — safe against browsers (which always send it on
  cross-origin POST) but not a boundary against non-browser clients.
- **Cookie** — `SameSite=Strict`, `HttpOnly`; `Secure` set from `r.TLS` *or* a trusted
  `X-Forwarded-Proto` / `X-Forwarded-Ssl` header (`session.go:261-275`). The reasoning at
  `:255-260` (the header can only *add* Secure, so no allowlist is needed) is sound.
- **No CORS middleware exists anywhere.** `connect-src 'self'` in the CSP is the substitute.

### 5.5 Security response headers (`webui.go:1311-1326`)

`X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, and a
static CSP: `default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline';
connect-src 'self'; img-src 'self' data:; font-src 'self' data:; base-uri 'none';
form-action 'none'; frame-ancestors 'none'`. `'unsafe-inline'` on `style-src` only (React
Flow / Radix runtime styles) — no script execution implication.

**These headers are applied only by `kernel/webui`.** The REST API, OpenAI API, and agent gateway
set none of them.

### 5.6 Exposure escalation paths

1. **Tunnel** (`cmd/agezt/httpsurfaces.go:293-343`) — publishes the loopback console to the
   internet via cloudflared/ngrok/tailscale. `OnURL` auto-allowlists the public host
   (`:308-312`), which trips `passwordStrict` inside `SetAllowedHosts`
   (`kernel/webui/webui.go:156-160`). But `tunnelPublicURL` (`:373-378`) **prints the console
   token into the public URL** when strict mode is on. Combined with the default password
   (`"agezt"`), a tunnelled default install is a public control plane behind a known password
   plus a token printed to the daemon log.
2. **Wildcard bind** — `webAllowedHosts` skips unspecified IPs (`httpsurfaces.go:271`) so binding
   `0.0.0.0` registers **no** allowed host and the strict-mode auto-raise inside
   `SetAllowedHosts` never fires; a separate explicit check at `httpsurfaces.go:129-133` catches
   this and calls `SetPasswordStrict(true)`. The comment at `:124-128` documents the trap
   honestly — this is a case where comment and code now agree.
3. **AUTH-001 regression note** (`httpsurfaces.go:134-146`) — `AGEZT_WEB_PASSWORD_STRICT` is only
   applied when the variable is actually *set* (`os.LookupEnv`), because an unconditional
   assignment previously cleared the auto-raised flag. Correctly implemented today.

> **⚠ DIVERGENCE 7 — `install.sh` exposes the wrong service.**
> `install.sh:40` sets `AGEZT_REST_ADDR=127.0.0.1:8787`, which is *also* the Web UI's default
> port (`cmd/agezt/httpsurfaces.go:82`). On a systemd install the REST API wins the bind and the
> console falls back to a random loopback port (`httpsurfaces.go:101-108`). `install.sh:285` then
> reports `"REST/Web binding: http://$AGEZT_REST_ADDR"` — conflating the two — and every
> `install.sh expose …` recipe tunnels port 8787 (`:349`, `:391`, `:412`). An operator following
> the documented flow publishes the **REST API** (plus its unauthenticated `/healthz` and
> `/readyz`) while believing they exposed the console. `install.sh` never sets `AGEZT_WEB_ADDR`
> or `AGEZT_WEB_PASSWORD`, so the console meanwhile runs with the default password `"agezt"`.

### 5.7 Sandbox — `kernel/warden/`

**What it actually sandboxes: process execution only** (`warden.go:277-409`). Enforced: wall-clock
timeout + `WaitDelay` (`:316`, `:335`), stdout/stderr cap (default 256 KiB, `:89`, `:346-349`),
working directory (`:320`), explicit env (`:330-334`), and a `warden.executed` audit event
(`:431-455`).

- **No filesystem confinement.** No chroot, no bind mount, no path jail — `WorkDir` is just
  `cmd.Dir`. Path-escape guarding lives in the *file tool*, not the warden; the **shell tool
  routes through warden and is unconstrained by it**.
- **No network sandboxing** except in the opt-in container path.

| OS | `ProfileNone` | `ProfileNamespace` | `ProfileContainer` / `MicroVM` |
|---|---|---|---|
| Linux | none | `setpgid` + best-effort `prlimit(2)` on CPU/AS/NOFILE/FSIZE | → downgrade to Namespace |
| **Windows / macOS / all else** | none | **→ `ProfileNone`** | **→ `ProfileNone`** |

`kernel/warden/warden_other.go:20` is literally `func resolveEffectiveProfile(_ Profile) Profile
{ return ProfileNone }`, with `configurePlatformAttrs` and `applyPlatformLimits` as empty no-ops
(`:22-23`). Linux resolution: `warden_linux.go:54-64`.

**⚠ DIVERGENCE 8.** The package doc lists four profiles including "Linux namespaces + cgroups +
seccomp", "OCI container", "lightweight VM" (`warden.go:9-16`) and then **retracts them honestly**
at `:17-38`
(*"There are no namespaces (CLONE_NEWUSER/NEWNS/NEWPID), no seccomp BPF and no cgroup v2"*, *"On
every other platform, including Windows and macOS, ALL profiles resolve to ProfileNone. Nothing is
isolated."*). Confirmed in code — that retraction is accurate and should be quoted in any threat
model.

> **⚠ DIVERGENCE 9 — `EffectiveProfile` is not the honest signal the doc promises.**
> `warden.go:32-36` tells callers to key trust off `EffectiveProfile`. On Linux,
> `EffectiveProfile("namespace")` returns `"namespace"` (`warden_linux.go:58-59`) for a request
> that engages **no namespace at all** — only rlimits and `setpgid`. `warden_linux.go:26-33`
> argues the rlimits *are* the "v1 namespace-equivalent layer"; a caller following the doc
> reasonably concludes namespaces exist. They do not.

Container backend (opt-in, `AGEZT_WARDEN_DOCKER=1`, `container.go:57-88`, defaults at
`cmd/agezt/main.go:3861-3884`: `docker`, `python:3.12-slim`, `--network none`) is **missing
`--read-only`, `--cap-drop=ALL`, `--user`, `--pids-limit`, `--security-opt no-new-privileges`**;
the bind mount is read-write and the container runs as root.

Correctly implemented (RCE-001 fix): `plugins/tools/shell/shell.go:245` and
`plugins/tools/codeexec/codeexec.go:263` key the credential bucket off
`w.EffectiveProfile(profile)`, not the *requested* profile.

### 5.8 SSRF guard — `kernel/netguard/`

`Allowed()` (`netguard.go:69-104`) denies: unspecified, `0.0.0.0/8`, loopback (unless
`AllowLoopback()`), **link-local incl. `169.254.169.254` — with no opt-in at all**, RFC1918 + ULA
+ CGNAT (unless `AllowPrivate()`), multicast and broadcast. NAT64 `64:ff9b::/96` and
IPv4-compatible addresses are collapsed to the embedded v4 and re-classified (`:80-82`,
`:111-132`).

The design is right: the check is a `net.Dialer.Control` hook (`:168-186`) that sees the concrete
resolved `IP:port` on **every** dial including each redirect hop — so DNS rebinding and 30x
redirects are genuinely handled, not merely documented. Unparseable or non-literal dial addresses
fail closed (`:171`, `:177`). `HTTPClient` (`:198-209`) builds a fresh, non-shared transport.
Blocked dials journal `netguard.blocked` via the generic `toolreg` wiring
(`kernel/toolreg/toolreg.go:60-64`, `cmd/agezt/main.go:3707-3720`).

**Wired strictly:** `plugins/tools/http/http.go:102`, `fetch/fetch.go:94`,
`websearch/websearch.go:107`, `kernel/chatgptauth/chatgptauth.go:57`,
`kernel/controlplane/channel_oauth.go:84`, `plugins/channels/onebot/onebot.go:97`.
**Wired loosely (loopback+private allowed):** `kernel/catalog/sync.go:21`,
`kernel/market/sync.go:38`, `kernel/mcp/http.go:75`, `kernel/controlplane/channels.go:403`,
`kernel/update/update.go:182-195` (which does add HTTPS-on-every-hop `CheckRedirect`).

> **⚠ ~45 outbound HTTP call sites bypass netguard entirely.**
> Kernel: `kernel/controlplane/nodes.go:152`, `remote_mirror.go:127` and `:195`
> (`http.DefaultClient.Do`), `kernel/acpcatalog/clients.go:64,115` + `registry.go:96,152`,
> `kernel/stt/stt.go:55`, and `kernel/creds/{aws.go:482, sso.go:181, sts.go:160,
> web_identity.go:116}` (the AWS IMDS one is *necessarily* exempt).
> **Library fail-open:** `kernel/webhook/webhook.go:102` — the dispatcher's default client is
> unguarded and `:92-93` says so; the daemon injects a guarded one
> (`cmd/agezt/httpsurfaces.go:593-594`), but the library default is fail-open, and `webhook.go:361`
> (`Probe`) repeats it.
> **Every channel driver** uses a bare `&http.Client{}`: slack, discord, telegram, whatsapp,
> whatsappgw, teams, matrix, mastodon, signal, sms, line, zalo, dingtalk, feishu, wecom,
> nextcloudtalk, imessage, push, webhook, chatwebhook, homeassistant, onebot (transport half).
> **Every provider driver** likewise: anthropic, bedrock, cohere, google, ollama, openai, image,
> rerank, vertex, plus the shared `plugins/providers/internal/retry/http.go:24,62` which falls back
> to `http.DefaultClient`. Provider base URLs are operator-configurable, so a misconfigured or
> attacker-influenced `base_url` reaches internal space unguarded.

### 5.9 Redaction — `kernel/redact/`, `kernel/envscrub/`

Patterns (`redact.go:55-100`, full-match → `[REDACTED]`): OpenAI/Anthropic `sk-`, AWS `AKIA`,
GitHub `gh[pousr]_` and fine-grained PAT, Slack `xox[baprs]-` / `xapp-`, Telegram bot token, Groq
`gsk_`, xAI `xai-`, Perplexity `pplx-`, Fireworks `fw_`, Google `AIza`, JWT, `Bearer …`, PEM
private-key blocks. Context-preserving templates (`:121-175`) for connection-string passwords,
`aws_secret_access_key=`, and Slack/Discord/Teams webhook URLs. Literals come from the creds store
(min length 8, sorted longest-first so a prefix-secret can't be partially exposed, `:43`,
`:235-240`), seeded at `cmd/agezt/main.go:645-647`.

**The one real chokepoint is the bus**: `kernel/bus/bus.go:198` redacts *before*
`j.Append` (`:199`), and the redacted spec is what subscribers receive (`:204-210`), so journal
**and** live SSE both see scrubbed payloads. `SetEscapeHTML(false)` (`bus.go:105-106`) correctly
stops `&`/`<`/`>` in a literal secret from evading the scrubber. Streaming deltas go through the
same path (`bus.go:247`).

> **⚠ DIVERGENCE 10 — redaction is optional and has a journal bypass.**
> (a) `redact.go:3-9` calls this *"the chokepoint that prevents"* secrets entering the record, with
> no mention that it is optional. `cmd/agezt/main.go:642-651` leaves `redactor` **nil** unless
> `dcfg.Misc.Redact`, and `main.go:873-875` installs it only `if redactor != nil` — so
> `AGEZT_REDACT=off` removes the only systemic chokepoint.
> (b) `kernel/agentgw/audit.go:97` calls `a.j.Append(spec)` **directly, bypassing `bus.Publish`
> entirely**, so `redactSpecLocked` never runs on agent-gateway audit entries.
> (c) Second-tier redactors (`kernel/controlplane/remote_mirror.go:256`,
> `kernel/openaiapi/openaiapi.go:50`, `plugins/builtintools/plugins.go:85`) each construct a bare
> `redact.New()`, which has **no literals** (`redact.go:217`) — they catch only the built-in
> pattern list, so an operator-configured secret with no recognisable prefix passes through all
> three.
> (d) **The model's context window is not redacted.** `kernel/agent/run_tools.go:332` returns
> `job.result` unmodified into the conversation, so a secret in tool output reaches the LLM
> provider in the clear; only the local record is scrubbed.

`envscrub.Scrubbed()` (`envscrub.go:15-43`) is an **allowlist** (PATH, COMSPEC, SYSTEMROOT, HOME,
USERPROFILE, APPDATA, TEMP, LANG, LC_*, …) with `IsSecretName` (`:55-63`) additionally dropping
anything containing KEY/TOKEN/SECRET/PASSWORD/PASSWD/CRED/`AWS_`/`AGEZT_`. Only four callers
(`kernel/creds/aws.go:114`, `plugins/tools/acpagent/acpagent.go:247`,
`plugins/tools/browser/action.go:843`, `plugins/tools/coding/coding.go:145`).
**The shell tool does not use it** — `plugins/tools/shell/env.go` carries four parallel near-duplicate
allowlists (`sshEnv:67`, `kubectlEnv:91`, `cloudCLIEnv:115`, `scrubEnv` used at `shell.go:259`),
each with a local `isSecretName`, and each deliberately re-admits a credential handle the central
list would drop: `SSH_AUTH_SOCK` (`:72`), `KUBECONFIG` (`:96`),
`MODAL_CONFIG_PATH`/`DAYTONA_CONFIG_PATH` (`:120`). Four copies that can drift independently.

Note also that `HOME`/`USERPROFILE`/`APPDATA` are passed through by design (`envscrub.go:21-22`) —
which is precisely where `~/.aws/credentials`, `~/.ssh/id_*` and AGEZT's own vault live. Env
scrubbing does not stop a child from reading secrets off disk.

### 5.10 Credential vault — `kernel/creds/`

`<BaseDir>/creds.json` (`creds.go:49`), written via `atomicfile.WriteFile(..., 0o600)` with the
mode re-applied after rename (`creds.go:210-216`). The comment at `:210` correctly notes **Windows
ignores Unix mode bits** — on the owner's platform the `0600` is cosmetic and protection is
NTFS-ACL inheritance only.

Encryption (`encrypt.go`): AES-256-GCM (`:78`, `:176-193`), fresh 32-byte salt and 12-byte nonce
per save (`:116`, `:119`, `:170-186`), 200 000 iterations (`:114`). Decrypt validates cipher id
(`:221`), KDF id (`:224`), an iteration floor of 100 000 and ceiling of 10 000 000 (`:227-236`)
against the envelope's own attacker-controllable `kdf_iter`, and checks nonce length **before**
`gcm.Open` to avoid Go's panic (`:246-253`). All correct and well-reasoned.

> **⚠ DIVERGENCE 11 — the KDF is not the primitive it is named after.**
> The constant is `KDFPBKDF2 = "pbkdf2-hmac-sha256"` (`encrypt.go:87`) and the function is
> `deriveKeyPBKDF2`, but `encrypt.go:325-341` computes `U_i = HMAC(password, U_{i-1})` — a keyed
> hash chain with PBKDF2's XOR accumulation bolted on. Real PBKDF2 (RFC 2898) re-keys the PRF with
> the password each round. The construction is deterministic, salted, XOR-accumulating and costs
> O(iter) HMAC-SHA256, so it is not obviously weak, and it is self-consistent across
> encrypt/decrypt — but it is **unreviewed custom crypto occupying a slot where a standard is
> claimed**, and the known-answer test therefore locks in AGEZT's own construction rather than RFC
> 2898 vectors. Note the doc header at `encrypt.go:32` is the *honest* one ("iterated HMAC-SHA256
> (200,000 rounds)"); it is the constant name and function name that mislead.

**Key source** — `defaultPassphraseChain` (`machine.go:77-85`, installed at `creds.go:88`):
1. `AGEZT_VAULT_PASSPHRASE` (`creds.go:54`) — operator-managed, wins;
2. **machine-bound key** unless `AGEZT_VAULT_AUTOENCRYPT=off` (`machine.go:43`, `:81-83`);
3. `""` → **plaintext on disk** (`creds.go:195-199`).

The machine key is a **single un-stretched SHA-256** over `"agezt-vault-machine-v1|" + machineID +
"|" + uid|username` (`machine.go:55-72`), where machineID is `HKLM\…\MachineGuid` on Windows,
`/etc/machine-id` on Linux, `IOPlatformUUID` on macOS, `""` elsewhere. The 200 000-round KDF is
then applied to that string, so brute-force cost is fine — but the *input* has no entropy an
attacker on the box lacks. `machine.go:19-23` says so plainly: *"a process running as the same user
on the same machine can derive the key too."* Carry that into the threat model: **the vault is not
protected against the very agent this kernel runs**, which has `shell` at L4 by default.

Three ways the vault ends up plaintext at rest: `AGEZT_VAULT_AUTOENCRYPT=off`; no machine-identity
source (e.g. a container without `/etc/machine-id`, `machine.go:57-59`); or a legacy vault never
migrated — `EncryptInPlace` (`machine.go:92-108`) no-ops when the passphrase is empty, and `Load`
accepts plaintext silently (`creds.go:160-167`).

---

## 6. External Integrations

- **LLM providers** — `plugins/providers/` + `plugins/providerboot/`. Keys via
  `AGEZT_ANTHROPIC_API_KEY`, `AGEZT_OPENAI_*`, Azure, Bedrock (`cmd/agezt/awschain.go`), Vertex,
  DeepSeek, Ollama, and a per-provider keyring (`/api/provider/keys/*`).
- **AWS** — `cmd/agezt/awschain.go` (172 lines): assume-role, SSO profile, and a
  **credential-process** integration gated by `AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED` /
  `_ENV` (an allowlist that exists because the helper was previously handed every secret in the
  environment — see repo history).
- **Model catalog** — `AGEZT_CATALOG_URL` (models.dev) pulled server-side by
  `POST /api/catalog/sync` (`webui.go:577`).
- **Speech** — `AGEZT_STT_*` / `AGEZT_TTS_*`, OpenAI-compatible plus ElevenLabs/Deepgram
  (`cmd/agezt/httpsurfaces.go:157-172`).
- **Embeddings** — `AGEZT_EMBED_URL/MODEL/KEY`.
- **~27 comm channels** — Slack, Discord, Telegram, WhatsApp (Cloud API + WAHA/Evolution
  gateway), Teams, Feishu, DingTalk, WeCom, WeChat, LINE, Zalo, OneBot, Matrix-adjacent, IRC,
  Twitch, Mastodon, Zulip, Gotify, Nextcloud Talk, iMessage, SMS, email (SMTP out, IMAP/POP in),
  generic webhook, chat webhook.
- **Home Assistant** — `AGEZT_HOMEASSISTANT_URL/TOKEN` with a service allowlist
  (`AGEZT_HOMEASSISTANT_TOOL_ALLOW_ALL_SERVICES` is an escape hatch).
- **Remote execution profiles** — `kernel/executionprofile/`: local, warden, Docker,
  **Kubernetes** (`k8s.go`), SSH, Modal, Daytona, and `remote-agezt` peer.
- **MCP servers** — stdio (spawned child processes) and remote Streamable-HTTP.
- **Marketplace** — remote catalogue sync with Ed25519 signatures (`/api/market/source/add`
  takes a `pubkey`).
- **Self-update** — `AGEZT_UPDATE_ENDPOINT` or `AGEZT_UPDATE_GITHUB_OWNER/REPO`, applied via
  `POST /api/v1/update/apply` (admin).
- **No** Sentry/DataDog/APM, **no** S3/GCS/Azure Blob SDK, **no** Redis/Kafka/RabbitMQ.

---

## 7. Authentication Architecture (summary table)

| Pattern | Present | Where |
|---|---|---|
| API key / Bearer | ✅ primary | `kernel/auth/token.go`, all HTTP surfaces |
| Session cookie | ✅ console password only | `kernel/webui/session.go` |
| OAuth 2.0 (as *client*) | ✅ channels + ChatGPT provider | `webui.go:587-594`, `kernel/controlplane/provider_oauth.go` |
| HMAC/signature | ✅ per-channel inbound | `plugins/channels/*` |
| JWT | ❌ | — |
| mTLS | ❌ | — |
| Basic auth | ❌ | — |
| SAML/SSO | ❌ | — |
| MFA | ⚠ partial | `AGEZT_WEB_PASSWORD_STRICT=on` = token **AND** password |
| Account lockout | ✅ | `session.go:38-39` (console login only) |
| Password hashing | ❌ n/a | plaintext constant-time compare, `session.go:224` |

Token storage: `<base>/openai.token`, `<base>/rest.token` at 0600 in a 0700 dir
(`kernel/auth/tokenfile.go:30-35`). Console + SSE tokens are memory-only, regenerated each boot.
Token lifetime = daemon lifetime; there is **no rotation and no revocation** short of restart.

---

## 8. File Structure Analysis — sensitive files & paths

**Runtime base dir** — `$AGEZT_HOME` else `~/.agezt` (`internal/paths/paths.go:20-30`). Contains:
`journal/` (hash-chained event log, holds redacted-but-complete run history), `openai.token`,
`rest.token`, control-plane addr+token files, the credential vault, tenant subdirs, artifacts,
sandbox projects.

**File Manager root** — `AGEZT_FILE_ROOT`, default `~/agezt/workspace`, created 0700
(`kernel/webui/files_route.go:31`, `:43-63`). Operator-settable from the console, which is
exactly the vector the symlink-escape fix at `:121-132` was written for.

**Sensitive HTTP paths on the default-on console:** `/api/config`, `/api/config/values`,
`/api/config/set` (vault writes), `/api/provider/keys` , `/api/files/*`, `/api/journal/export`,
`/events` (full bus firehose), `/api/redact/test`.

**Unauthenticated diagnostic paths:** `/healthz`, `/readyz` (REST), `/health` (agent gateway),
`/api/authmeta` (console). `/metrics` is authed — correctly, since it exposes spend.

**Tracked secrets:** none. Only `.env.example` is committed; `.gitignore` covers `*.exe`,
`*.log`, `node_modules/`, build output. Note `kernel/webui/dist` **is** committed by design (for
`go:embed`).

**Deployment files:**

- ✅ `.github/workflows/ci.yml`, `.github/workflows/publish-sdks.yml`
- ✅ `Makefile`, `install.sh`, `install.ps1`, `dev.ps1`
- ✅ `ops/wsl-runners/wsl-keepalive.service` (systemd unit for CI runners)
- ❌ **No `Dockerfile`, no `docker-compose.yml`, no `k8s/` or Helm chart, no Terraform.**
  (`kernel/executionprofile/k8s.go` and the Docker/warden exec profiles are *runtime execution
  targets*, not infrastructure-as-code for this repo.)

CI notes: `ci.yml` pins `permissions: contents: read` (`:26-27`), uses SHA-pinned actions
(`:37`), and sets `persist-credentials: false` (`:39`). Runs on **self-hosted** WSL runners
(`:31-33`) with `if: github.event_name == 'push' || pull_request.head.repo.full_name ==
github.repository` (`:33`) guarding fork PRs — that guard matters a great deal on self-hosted
runners.

---

## 9. Detected Security Controls

| Control | Status | Citation |
|---|---|---|
| Constant-time credential compare | ✅ everywhere | `auth/token.go:72`, `session.go:224`, `tenant.go:222` |
| Fail-closed auth | ✅ | `httpserver/auth.go:53-79`, `webui.go:1423-1425` |
| CSP + `X-Frame-Options` + `nosniff` + `Referrer-Policy` | ✅ console only | `webui.go:1311-1326` |
| Host allowlist (anti-DNS-rebind) | ⚠ names only | `webui.go:1328-1343` |
| Origin check on mutations | ⚠ absent-Origin passes | `webui.go:1345-1362` |
| CSRF token | ❌ (relies on SameSite=Strict + Origin) | — |
| CORS | ❌ none configured | — |
| Request body caps | ✅ declarative | `httpserver/limits.go:10-20`, per-route `BodyMax` |
| Slow-loris timeouts | ✅ `ReadHeaderTimeout` 10 s, `IdleTimeout` 120 s; `WriteTimeout` intentionally unset for SSE | `httpserver/listener.go:12-35` |
| Secret redaction on journal + stream + webhook egress | ⚠ opt-out + 2 bypasses | `bus/bus.go:198`, `:247`; `main.go:642-651`, `:873-875`; bypass `agentgw/audit.go:97` |
| Error-string redaction on HTTP responses | ✅ OpenAI surface (pattern-only) | `openaiapi.go:47-52` |
| SSRF egress guard | ✅ dialer-level, redirect/rebind-safe — ⚠ ~45 bypassing clients | `netguard/netguard.go:69-104`, `:168-186`; see §5.8 |
| Sandbox for code exec | ⚠ **no isolation on Windows/macOS**; Linux = rlimits only | `warden_other.go:20-23`, `warden_linux.go:54-64` |
| Path-traversal + symlink + Windows-junction guard | ✅ | `webui/files_route.go:104-134`, `:142-185`, `:~196+`; `tools/file/file.go:760-868` |
| Archive-extraction guards (traversal + zip-bomb) | ✅ | `codeexec/artifacts.go:292-349`, `cmd/agt/backup.go:465-530` |
| Rate limiting | ⚠ selective | see §5.3 |
| Audit logging | ✅ hash-chained journal + `/api/journal/verify` | `webui.go:263` |
| Brute-force lockout | ✅ console login only | `session.go:113-121` |
| Vault encryption at rest | ⚠ on by default but same-user-derivable; mislabeled KDF | `creds/machine.go:55-85`, `encrypt.go:325-341` |
| Plugin binary integrity | ✅ BLAKE3-256 hash pin at spawn + reload | `plugin/pin.go:26-34`, `host.go:290`, `:1017` |
| Update signature verification | ⚠ **inert for the default GitHub source** | `update.go:380` (`DefaultPublicKeyHex = ""`), `:390-403`, `:426-445` |
| Dependency pinning | ✅ Go modules + npm lockfile; ⚠ dual npm/pnpm lockfiles in `frontend/`; ⚠ `dompurify` declared but unused | `frontend/package.json:51` |

### Self-update signature posture (detail)

`kernel/update/update.go:380` — `DefaultPublicKeyHex = ""`, intended to be injected via
`-ldflags` (`:368-376`). `resolvePublicKey()` (`:390-403`) therefore returns nil, and
`verifySignature` (`:426-445`) then **accepts any `ProvenanceGitHubRelease` manifest unsigned**,
anchoring only on GitHub TLS + SHA-256. Endpoint- and caller-supplied manifests *are* refused
(`ErrSignatureKeyNotConfigured`, `:357`) — that is the UPD-001 fix, and the `Provenance` zero
value is correctly the untrusted one (`:73-89`). Net: shipped binaries do checksum-only for the
GitHub path; a compromised release asset is not caught.

---

## 10. Consolidated comment-vs-code divergences

| # | The claim | The code | Evidence |
|---|---|---|---|
| 1 | Web UI is "token-authed and **read-only** (SPEC-06)" | 180+ mutating routes incl. `/api/run`, `/api/files/*`, `/api/toolbox/install` | `cmd/agezt/httpsurfaces.go:49-50` vs `webui.go:798-803`, `:809`, `:812`, `:850-852` |
| 2 | "The write set is a fixed allowlist; there is **no generic passthrough**" | `jsonRoutes`, `planRoute`, and `/api/run` (arbitrary free-text intent) | `webui.go:23-24` vs `:557`, `:731`, `:1077` |
| 3 | Server is "**token-authed on every request**" | 7 route patterns are `TierPublic` and get no authenticator wrapper | `webui.go:20` vs `:775-790`, `:860`, `:867`; `httpserver/router.go:109` |
| 4 | (no claim — silent default) | `defaultLoopbackWebPassword = "agezt"` returned whenever loopback + unset | `httpsurfaces.go:230`, `:232-244`, console default-ON at `:82` |
| 5 | "unknown capability is refused / secure-default / default-deny" | **every known capability defaults to L4 silent allow** | `edict.go:634-640` vs `toolmap.go:15-16`, `edict.go:506-507`, `runtime/policy.go:28` |
| 6 | Hard-deny rails cover fork bombs / `rm -rf /` / raw-device writes | all 16 rules are `AppliesTo: [CapShell]`; same command via `code_exec`/`conductor`/`forge_*`/`mcp_*` matches nothing | `edict.go:645-667` vs `:622-624` |
| 7 | `install.sh` "REST/Web binding" + `expose` recipes publish the console | port 8787 is the **REST API** in that config; the console is bounced to a random port | `install.sh:40`, `:285`, `:349` vs `httpsurfaces.go:82`, `:101-108` |
| 8 | Warden offers namespace / container / microVM isolation | **Windows + macOS: all profiles → `ProfileNone`, nothing isolated.** Linux: rlimits + `setpgid`, no namespaces/seccomp/cgroups | `warden.go:9-16` (retracted honestly at `:17-38`); `warden_other.go:20-23`, `warden_linux.go:54-64` |
| 9 | Key trust off `EffectiveProfile` — it downgrades honestly | on Linux it returns `"namespace"` for a request engaging no namespace | `warden.go:32-36` vs `warden_linux.go:26-33`, `:58-59` |
| 10 | redact is "the chokepoint that prevents" secrets entering the record | disable-able (`AGEZT_REDACT=off`); `agentgw` writes to the journal around the bus; second-tier redactors are literal-blind; model context is never scrubbed | `redact.go:3-9` vs `main.go:642-651`, `agentgw/audit.go:97`, `redact.go:217`, `agent/run_tools.go:332` |
| 11 | Vault KDF is `pbkdf2-hmac-sha256` | custom keyed-HMAC chain with XOR accumulation, not RFC 2898 | `encrypt.go:87` + fn name vs `encrypt.go:325-341` (honest header at `:32`) |
| 12 | Policy gate is fail-closed | `PolicyVerdict{Allow: true}` is the **default** when `cfg.Policy == nil` | `agent/run_tools.go:188` |
| 13 | Every tool invocation is gated | `council.go:298` invokes `web_search` with no policy hook; conductor/council in-kernel fan-out never re-enters the engine | `runtime/council.go:298`, `edict/toolmap.go:138-144`, `plugin/host.go:920` |
| 14 | `mcp.install` / `mcp.call` are "Ask by default" | `DefaultLevels()` sets both to `LevelAllow` | `edict.go:181-192`, `mcp/store.go:11-13` vs `edict.go:634-640` |
| 15 | Egress is SSRF-guarded | ~45 raw `http.Client` / `http.DefaultClient` sites bypass netguard, incl. **all** provider and channel drivers and the webhook library default | see §5.8 |
| 16 | `AGEZT_WEB_PASSWORD_STRICT` default is "off" (docs) | code auto-raises it for any non-loopback allowed host and for a wildcard bind | `docs/THREAT-MODEL.md:477` vs `webui.go:156-160`, `httpsurfaces.go:129-133` — *code is safer than the doc here* |
| 17 | `docs/THREAT-MODEL.md:516`: "the safe default is **deny**. Edict is designed to fail closed." | contradicted by the same document at `:470` ("Every capability ships permissive by default") and by `edict.go:634-640` | internal doc contradiction |

Divergence 16 is the only one where the code is *stricter* than its documentation. Every other
row is a guarantee that reads stronger on paper than it behaves in practice.

---

## 11. Prioritized Targets for Phase 2

Ranked by (reachability × authority):

1. **`cmd/agezt/main.go:3888-3918` + `kernel/edict/edict.go:634-640` — the default posture.**
   `AGEZT_AUTO_APPROVE_CAPS` unset auto-approves every capability, and `DefaultLevels()` already
   sets every capability to L4. Confirm that on a stock install *nothing* prompts and *nothing*
   denies outside the 16 shell-only hard-deny substrings. Every other guard's documentation
   quietly assumes otherwise.
2. **`kernel/webui` — the default-on control plane** with `defaultLoopbackWebPassword = "agezt"`
   (`httpsurfaces.go:230`), 180+ mutating routes, and `/api/run` → arbitrary agent execution.
3. **`kernel/agentgw`** — started unconditionally (`runtime.go:939-945`), abstract unix socket
   with no filesystem ACL and **no peer-credential check** (`sockopt_unix.go:11-20`), TCP
   fall-through on any unrecognised `AGEZT_AGENTGW_SOCKET` value (`gateway.go:186-202`),
   unauthenticated `/health`, and `cmd/agt/token.go:102-113` minting uncapped tokens.
4. **`mcp` tool → `exec.Command`** — `plugins/tools/mcptool/tool.go:129-134` →
   `kernel/runtime/mcptool.go:120` → `kernel/mcp/client.go:113`. No binary allowlist, no hash pin
   (unlike `acpagent` and `plugin`, which have both). Plus boot auto-attach at `main.go:1616`
   spawns every enabled registration with **no Edict prompt on that path**.
5. **`plugins/tools/shell` on Windows** — `ProfileNone` (no isolation) plus
   `kernel/warden/cmdline_windows.go:14-24` handing the raw LLM string to `cmd.exe` verbatim,
   against substring-only hard-deny matching (`edict.go:373-378`).
6. **`POST /hooks/` (`webui.go:990`)** — the one token-free mutating path; secret accepted in a
   query string (`:1010`); body flows into `{{trigger.payload.*}}` and, per
   `kernel/workflow/templates.go:89`, into an HTTP node's URL.
7. **Persistence primitives** — `standing`, `schedule`, `tool_forge`, `mcp`: the four ways a
   single prompt injection outlives its run.
8. **`kernel/agent/run_tools.go:188` nil-policy fail-open** and
   **`kernel/runtime/council.go:298`** ungated `web_search` invoke.
9. **Netguard bypass inventory (§5.8)** — especially `kernel/webhook/webhook.go:102`'s fail-open
   library default and the provider drivers' operator-configurable `base_url`.
10. **Cadence-fired unattended runs** (`main.go:3283-3332`) that apply `WithMaxCost` but never
    `WithTrustCeiling`, and **`guardian-code`** seeded enabled to modify code daily at 03:00
    (`builtinguardians.go:149-159`).
11. **`kernel/creds`** — the `deriveKeyPBKDF2` mislabel (`encrypt.go:325-341`) and the
    same-user-derivable machine key (`machine.go:55-72`) against an agent that holds `shell` at L4.
12. **Frontend** — `views/Files.tsx:170` (unsandboxed iframe), the three unvalidated
    `window.open(authorize_url)` call sites, and the unused `dompurify` declaration.

---

## Detected Languages

- **Go** (50,250 LOC, 1,572 files — the entire server, daemon, CLI, plugins) → **activates `sc-lang-go`**
- **TypeScript / TSX** (109,219 LOC combined, 450 files — React 19 console SPA + TS SDK) → **activates `sc-lang-typescript`**
- **Python** (3,601 LOC, 27 files — `sdk/python` client SDK) → **activates `sc-lang-python`**
- **Rust** (1,955 LOC, 6 files — `sdk/rust` client SDK) → **activates `sc-lang-rust`**
- **Shell / PowerShell** (3,121 LOC combined — installers, dev + CI scripts) → no dedicated
  `sc-lang-*`; covered by `sc-ci-cd` and the shell-injection checks inside `sc-lang-go`
- **PHP, Java, C#** — **absent**; do **not** activate `sc-lang-php`, `sc-lang-java`, `sc-lang-csharp`

## Infrastructure files detected

| File | Present | Skill implication |
|---|---|---|
| `.github/workflows/ci.yml` | ✅ | **activates `sc-ci-cd`** |
| `.github/workflows/publish-sdks.yml` | ✅ | **activates `sc-ci-cd`** (publishes to npm/PyPI/crates — check secret handling) |
| `.github/actions/setup-go-safe/` | ✅ | composite action, in scope for `sc-ci-cd` |
| `.github/CODEOWNERS` | ✅ | |
| `Dockerfile` | ❌ | **do not activate `sc-docker`** |
| `docker-compose.yml` | ❌ | **do not activate `sc-docker`** |
| `k8s/` · `helm/` · `*.yaml` manifests | ❌ | **do not activate `sc-iac`** |
| `*.tf` / Terraform | ❌ | **do not activate `sc-iac`** |
| `ops/wsl-runners/wsl-keepalive.service` | ✅ | systemd unit for self-hosted CI runners — review under `sc-ci-cd` |
| `install.sh`, `install.ps1`, `dev.ps1`, `Makefile` | ✅ | installer/build scripts — review for curl-pipe-sh and unsigned download patterns |

> Caveat for whoever runs `sc-docker`/`sc-iac` anyway: `kernel/executionprofile/` contains
> **Docker, Kubernetes, SSH, Modal and Daytona execution drivers**. These are runtime targets the
> agent can execute code on, not deployment descriptors for this repo — but they are a legitimate
> `sc-lang-go` target (command construction, credential propagation, `AGEZT_EXEC_SECRET_*`).
