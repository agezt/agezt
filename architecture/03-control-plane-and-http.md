# 03 — Control plane and HTTP surfaces

**Scope:** `kernel/controlplane` (208 non-test files, ~31.9k LOC, 325 protocol ops), `kernel/httpserver`, `kernel/auth`,
`kernel/streamlimit`, `kernel/webui` (Go side), `kernel/restapi`, `kernel/openaiapi`, `kernel/agentgw`, `kernel/webhook`,
`kernel/tunnel`. Wiring of these servers happens in `cmd/agezt` (see [01-daemon-boot-cmd-agezt.md](01-daemon-boot-cmd-agezt.md));
the CLI client side is in [02-cli-cmd-agt.md](02-cli-cmd-agt.md); the browser side in [11-frontend-console.md](11-frontend-console.md).

## Responsibilities at a glance

| Package | Role | Listener / transport | Credential |
|---|---|---|---|
| `controlplane` | THE operator API of the daemon: every `agt` command and every Web UI panel ends up here as a named op dispatched against `*runtime.Kernel`. Also ships the Go `Client`. | TCP `127.0.0.1:0` (ephemeral), newline-delimited JSON, one request per connection | 32-byte hex primary token in `<home>/runtime/control.token` (or `AGEZT_TOKEN` client-side); per-tenant tokens via `tenant.Registry` |
| `httpserver` | Shared HTTP mechanics for every surface: `Router` with per-route tier/method/body policy, `Authenticator`, `BodyLimit`, hardened `Start`, process-wide SSE helper + per-IP stream cap | caller-owned `net.Listener` | delegates to `auth.Verifier` or a surface `RequestAuthorizer` |
| `auth` | Transport-independent `Tier` (`Unset/Public/User/Admin`), `Verifier`, `StaticVerifier` (constant-time), 0600 token-file helpers | — | — |
| `streamlimit` | Per-key concurrent-stream counter (`Limiter.Acquire`) | — | — |
| `webui` | Serves the go:embed-ded React SPA and is a thin, allowlisted HTTP→control-plane proxy (198 proxy routes + ~25 bespoke routes), `/events` SSE firehose from the bus, password login/session, file manager, rollback, STT/TTS, workflow `/hooks/`, OAuth callback | `AGEZT_WEB_ADDR` (default ON at `127.0.0.1:8787`, fallback `127.0.0.1:0`) | per-boot random console token (Bearer / `?token=` on shell), console-password session cookie, ephemeral SSE token `?st=` |
| `restapi` | Versioned REST for SDKs/automation: runs (sync + SSE), health/ready/metrics, models, artifacts, mailbox, self-update. Talks to an `Engine` (kernel) directly, NOT via control plane | `AGEZT_REST_ADDR` (off unless set) | `<home>/rest.token` Bearer (admin) or tenant token + `X-Agezt-Tenant` |
| `openaiapi` | OpenAI-compatible `/v1/chat/completions`, `/v1/responses`, `/v1/models`, `/v1/audio/transcriptions`; every request is a governed kernel run | `AGEZT_API_ADDR` (off unless set) | `<home>/openai.token` Bearer or tenant token + `X-Agezt-Tenant` |
| `agentgw` | HMAC-JWT capability-scoped gateway for agent-subprocess code (bus publish/subscribe, memory, log, agent list, config center) | unix abstract socket `@agezt/agentgw-<rand>.sock` (or `AGEZT_AGENTGW_SOCKET`, may be TCP) | HS256 JWT signed with `<home>/agentgw.secret` / `AGEZT_AGENTGW_TOKEN_SECRET` |
| `webhook` | Outbound: bus subscriber that POSTs matching events (HMAC-signed) to `AGEZT_WEBHOOKS` sinks, journals delivered/failed | outbound HTTP via netguard client | per-sink HMAC secret |
| `tunnel` | Supervises an external tunnel binary (cloudflared/ngrok/tailscale/custom) to expose the Web UI or REST API publicly | child process | — |

```
                 agt CLI ─────────────┐                 SDKs / curl / Prometheus        OpenAI clients/IDEs
                                      │ controlplane.Client         │ Bearer rest.token          │ Bearer openai.token
 Browser (React SPA)                  │ (TCP 127.0.0.1:N, JSON line)│                            │
   │ fetch /api/* (Bearer|cookie)      ▼                             ▼                            ▼
   │ EventSource /events?st=   ┌──────────────────┐        ┌────────────────┐         ┌──────────────────┐
   └──────────────────────────►│ webui.Server     │        │ restapi.Server │         │ openaiapi.Server │
                               │ secure()→Router  │        │ httpserver.Router (Tier/Method/BodyMax)     │
                               │ proxy(cmd) ──────┼─┐      └───────┬────────┘         └────────┬─────────┘
                               │ /events ◄─ bus ">"│ │              │ Engine (cmd/agezt kernelAPIEngine) │
                               └──────────────────┘ │              ▼                                    ▼
                                                    │      runtime.Kernel.RunModel / bus.Subscribe(SubjectForRun)
                                                    ▼
                                      ┌───────────────────────────────┐
                                      │ controlplane.Server.handleConn│ auth (primary|tenant) → commandRegistry
                                      │ → commandSpec.Handler(dc)     │ → kernelFor(tenant) → *runtime.Kernel
                                      └───────────────────────────────┘
 agent subprocess ──JWT──► agentgw (unix socket) ──► bus / memory / roster / configcenter / journal(audit)
 bus ──► webhook.Dispatcher ──HMAC POST──► external sinks          tunnel ──► cloudflared/ngrok → webui|rest
```

Key architectural fact: there are **two paths into the kernel**. The Web UI and the CLI go through the control-plane TCP
protocol (the Web UI is an in-process HTTP server that dials the daemon's own control port with the primary token).
`restapi`, `openaiapi` and `agentgw` call the kernel (or its bus/memory/roster) **directly** through narrow interfaces.

---

## 1. `kernel/controlplane`

### 1.1 Purpose

Local control protocol between the `agezt` daemon and every operator client (`agt`, the Web UI proxy, `sdk/`, `cmd/agt/dial`,
`haltresume`, `keys`, `whoami`). It hosts ~320 named commands covering the whole product surface (runs, roster, memory, world
model, schedules, workflows, marketplace, policy, providers, journal audit folds...). It is the largest kernel package and the
only one that imports almost every other kernel subsystem (50+ internal imports, see import graph).

Imports (from `internal-deps.txt`): `internal/brand internal/strutil kernel/{acpcatalog agent approval artifact board bus cadence catalog channel
chatgptauth configcenter convo creds datalake edict event executionprofile governor intent intervention journal market mcp memory
netguard okr planner redact reflect roster runtime scheduler seat settings skill standing taste tenant toolbox toolforge ulid update
warden workboard workflow worldmodel}` **and `plugins/tools/overseertool`** (layering violation — see Gotchas).
Imported by: `cmd/agezt`, `cmd/agt`, `cmd/agt/{dial,haltresume,keys,whoami}`, `kernel/webui`, `sdk`.

### 1.2 Protocol (deliverable a)

| Aspect | Value (source) |
|---|---|
| Transport | TCP, `net.Listen("tcp", "127.0.0.1:0")` — loopback, ephemeral port (`server_lifecycle.go` `Start`). Not a unix socket / named pipe. |
| Discovery | `<baseDir>/runtime/control.addr` (`"127.0.0.1:NNNNN\n"`) and `<baseDir>/runtime/control.token` (hex), both written 0600 by `writeRuntimeFiles`; dir created 0755. Removed on `initiateShutdown`. |
| Token | 32 random bytes → 64 hex chars, minted per daemon start (`crypto/rand`). Client reads `AGEZT_TOKEN` (`brand.EnvPrefix+"TOKEN"`) first, else the token file (`client.go NewClient`). |
| Framing | One JSON object per line (`\n`). Request line bounded to `maxRequestBytes = 16 MiB` by `readBoundedLine` (pre-auth DoS guard, M188). |
| Request | `Request{ID, Cmd, Token, Args map[string]any}` → `{"id":"q-150405.000","cmd":"run","token":"…","args":{…}}` (`protocol.go`). Client ids are `"q-"+time.Now().Format("150405.000")`. |
| Response | `Response{ID, Type, Event *event.Event, Result map[string]any, Error string}`; `Type` ∈ `event` (0..n), `result` (exactly one terminal) or `error` (alternative terminal). |
| Connection model | One request per connection: dial → write → read until `result`/`error` → close. No multiplexing. |
| Deadlines | Server: 10-min read deadline per connection, 30 s write deadline per response line. Client: 5 s dial timeout, 10 s write deadline, read deadline from ctx. |
| Op naming | snake_case verbs `<domain>_<verb>` (`agent_add`, `workboard_claim`, `memory_bulk_forget`); exception: Config Center uses dotted names `configcenter.get`, `configcenter.access-log`. Constants `Cmd*` live in `protocol_commands*.go`. |
| Auth | `tokenIsPrimary` (constant-time, blank never matches). Otherwise `args.tenant` + `tenants.Authorize(tenant, token)`; failure → `error: "unauthorized"`. |
| Authorization | Tenant principals may only run `TenantAllowed` specs; unknown command and non-allowlisted command both answer `forbidden: a tenant token cannot run "<cmd>" (primary token required)` (no "unknown command" oracle). The handler's `args.tenant` is pinned to the authorized tenant. |
| Streaming | Handler writes `RespEvent` frames then a terminal frame. `StreamMode`: `StreamNone`, `StreamEvents` (bounded progress, deadline kept), `StreamLive` (dispatch wraps ctx with `cancelOnConnClose`: clears read deadline, a goroutine blocks on `conn.Read` and cancels the ctx on disconnect). `pulse_subscribe` never sends a result (client terminates). |
| Errors | `fail`/`failMsg`/`ok` in `respond.go`; panics anywhere in a connection are caught by `recoverConn` → `error: "internal error"` (the daemon survives). |

Client API (`client.go`, `client_helpers.go`, `client_update.go`): `NewClient(baseDir)`, `ProbeExisting(baseDir) (addr, alive)`
(1.5 s `status` probe; used to detect stale runtime files), `(*Client).Call` (single result), `CallRaw` (result as `json.RawMessage`),
`Stream(ctx, cmd, args, onEvent)` (events then result), `StreamUntilCancel` (for `pulse_subscribe`; ctx cancel closes the conn and is not
an error), `UpdateCheck`, `UpdateApply`, `Close` (no-op). Server errors surface as `*ErrServerError{Msg}`; net timeouts are mapped back to
`ctx.Err()` / `context.DeadlineExceeded`.

### 1.3 Registration and dispatch

```
init() → registerAllCommands()            (registry.go: 28 register<Domain>Commands funcs, explicit order)
   register(commandSpec{Cmd, Handler, TenantAllowed, TenantRouted, Streaming, ReadOnly})   (dispatch.go; panics on dup/empty/nil)
       → commandRegistry map[string]commandSpec   (read-only after init)

acceptLoop → go handleConn(ctx, conn)                     (server_handlers.go)
  1. defer recoverConn(conn,&req); SetReadDeadline(+10m)
  2. readBoundedLine(16 MiB) → json.Unmarshal(Request)
  3. primary := tokenIsPrimary(req.Token); else tenant := args.tenant; tenants.Authorize(tenant, token) or "unauthorized"
  4. spec, known := commandRegistry[req.Cmd]; tenant && (!known || !spec.TenantAllowed) → "forbidden…"; pin args.tenant
  5. !known → "unknown command: <cmd>"
  6. dc := &DispatchCtx{Ctx, Conn, Req, S, K: s.k, Tenant, Primary}
     spec.TenantRouted → dc.K = kernelFor(tenantOf(req))   (tenant.Registry.Acquire → *runtime.Kernel)
  7. spec.Streaming == StreamLive → dc.Ctx = cancelOnConnClose(ctx, conn)
  8. !spec.ReadOnly → beginOpAudit: journal op.invoked {op, caller, tenant, redacted args}; dc.Conn = auditedConn
  9. spec.Handler(dc)    // handlers are closures dc.S.handleX(dc.Conn, dc.Req) or (dc.Ctx, dc.Conn, dc.Req)
 10. !spec.ReadOnly → journal op.completed, or op.failed {error} on an error response, a panic, or no response
```

Flags semantics (from `dispatch.go` comments, enforced by `dispatch_registry_test.go` + `tenant_auth_test.go`):
- **TenantAllowed** — deny-by-default allowlist for tenant tokens (M38). 47 ops carry it; 4 more (`changelog`, `journal_stats`, `edict_compact`, `tenant_stats`) are TenantRouted but primary-only.
- **TenantRouted** — handler resolves its kernel per request via `kernelFor` (typed operations get the routed kernel from the app host). Invariant
  `TestRegistry_TenantAllowedImpliesTenantRouted`: every TenantAllowed op must be TenantRouted, sole exception `whoami`.
- **Streaming** — `run`, `plan`, `toolbox_install`, `market_install`, `market_uninstall` are `StreamEvents`; `chat_summarize`,
  `conductor_ask`, `council_ask`, `plan_generate`, `plan_refine`, `research_ask` are `StreamLive`. `run` handles its own
  disconnect watcher (`SetCancelOnDisconnect`, M35) and `pulse_subscribe` its own 500 ms read-deadline watcher — both
  deliberately NOT `StreamLive` to avoid two goroutines reading one conn.
- **ReadOnly** — the op changes no state, so dispatch does not journal it (148 ops). Every other op (173) is journaled by
  dispatch (`dispatch_audit.go`, W2.1): `op.invoked` before the handler, then `op.completed` or `op.failed`, sharing one
  correlation and written to the request's kernel (a tenant's own journal for tenant-routed ops). Arguments are summarized:
  scalars verbatim (strings ≤ 160 runes), names containing key/token/secret/password/cred/auth/cookie/value/code/state/…
  redacted, lists and objects reduced to their shape. The outcome is read from the terminal response `writeResp` writes
  through `auditedConn`. Omitting `ReadOnly` over-audits; it cannot under-audit.

### 1.4 Server state and dependency injection

`Server` (`server.go`) fields: `k *runtime.Kernel`, `baseDir`, listener/token/`done`/`serveCancel`/`wg`/`stopOnce`, `shutdownCh`
(closed by `shutdown` op; daemon main loop selects on `Shutdown()`), `pulse PulseController`, agent-list TTL cache
(`rosterListOnce`/`rosterList` → app/roster ListService, which owns RWMutex/validity/key/1.5s cache), `standingFire func(id) bool`,
`observers PulseObservers`, `tenants *tenant.Registry`, `configEnvPinned`, `cancelOnDisconnect`, `diskFree DiskFreeFunc`,
`httpBindings []HTTPBinding`, `channels []ChannelInfo`, `channelSend ChannelSender`, `credChain string`, `boardStore *board.Store`
+ `boardNotify` (the ONE shared board instance — M937), `updateSvc app/update.Backend` (public concrete setter retained; nil canonicalized), channel-OAuth `channelOAuthState *app/channels.OAuthMemory` and service (sync.Once),
ChatGPT login `chatgpt *chatgptauth.Manager` (chatgptOnce) + `provLogin` (provLoginMu) + `chatgptSync`.

Two-phase DI (`deps.go`): `NewServerWithDeps(k, baseDir, Deps{ConfigEnvPinned, Board, BoardNotify, DiskFree, UpdateSvc, Tenants,
CancelOnDisconnect, HTTPBindings, CredChain, Channels, ChatGPTSync})` before `Start`; `Bind(LateDeps{Pulse, Observers, ChannelSend,
StandingFire})` called incrementally at each boot readiness point (nil fields skipped, preserving per-dependency nil windows). The
individual `Set*` setters remain the unit-test surface. Interfaces exist so the package never imports `kernel/pulse` or channel plugins
(`PulseController`, `PulseObservers`, `ChannelSender`, `DiskFreeFunc`, `ChatGPTSyncFunc`).

### 1.5 Concurrency model

- `acceptLoop` goroutine + one goroutine per connection (tracked by `wg`); `Stop()` = `initiateShutdown` (close `done`, cancel
  `serveCtx` so streaming handlers blocked on ctx return, close listener, delete runtime files) + `wg.Wait()`.
- Extra goroutines: `cancelOnConnClose` reader, `run`'s cancel-on-disconnect reader, `pulse_subscribe` disconnect watcher, provider
  OAuth one-shot listener on `127.0.0.1:1455` (`provider_oauth.go`), workflow detached runs (`runWorkflowDetached`).
- Mutexes: `mu` (listener/token), app-owned roster list cache mutex, `oauthMu`, `provLoginMu`. Handlers otherwise rely on the kernel subsystems'
  own locking. `commandRegistry` is init-only and read lock-free.

### 1.6 Persistence (files it writes directly)

| Path under `<baseDir>` | Writer | Format |
|---|---|---|
| `runtime/control.addr`, `runtime/control.token` | `writeRuntimeFiles` | text, 0600 |
| `chat_prompts.json` | `handlePromptsSet` (`prompts.go`) | JSON list of `promptItem`, 0600 |
| `update.sentinel` | `writeUpdateSentinel` (`update_control.go`) | RFC3339 timestamp, tells the watchdog the exit was intentional |
| settings store (`settings.NewStore(baseDir)`) | `persistPulseSetting` (`AGEZT_PULSE_DIAL`, `AGEZT_PULSE_QUIET_HOURS`), `config_set`, routing/chains (`AGEZT_TASK_MODEL_CHAINS`), persona | via `kernel/settings` |
| vault / keyring | `provider_keys.go`, `channels_account_ports.go`, `channel_oauth_ports.go`, `provider_oauth.go` | via `kernel/creds` |
| catalog `api.json` + meta, `custom.json` | `catalog_sync`, `provider_connect` | via `kernel/catalog` |

Everything else is delegated to the owning subsystem (roster, memory, cadence store, workflow store, board, datalake, ...) and
journaled by that subsystem. Reads of `sandbox/projects/<name>` are confined by `confineUnder` (`sandbox.go`).

### 1.7 Env vars read directly

`AGEZT_TOKEN` (client), `AGEZT_ACP_AGENT_CMD` (`app/channels/acp.go`), `AGEZT_PEERS` (`nodes.go`), `AGEZT_REMOTE_EVENT_MIRROR`
(`remote_mirror_helpers.go`), `AGEZT_AUTO_REPAIR_COOLDOWN` (`roster_repair_summary.go`), `AGEZT_WORKSPACE` (`roster_workspace.go`),
`AGEZT_CATALOG_URL` (catalog sync default), provider env names presence (`catalog.go`, `app/channels/inventory.go`, `config_handler.go`,
`settings_handlers.go`); run-time execution-profile errors reference `AGEZT_EXEC_SSH*`, `AGEZT_EXEC_K8S*`, `AGEZT_EXEC_MODAL*`,
`AGEZT_EXEC_DAYTONA*`. `config.go` holds `configEnvVars`, the canonical inventory (~419 entries) of every `AGEZT_*` var the daemon reads,
surfaced by the `config` op as presence-only; guarded by `TestConfigEnvVars_CoversCmdAgeztReads` (M127) — a new daemon env var must be
added here. `config_set` writes the store AND calls `setLiveEnv` (`os.Setenv`) so a change is live; `AGEZT_PROVIDER`/`AGEZT_MODEL`
additionally trigger a kernel reload (`configFieldNeedsKernelReload`).

### 1.8 Events

The control plane is mostly a consumer: it subscribes to the bus for `run` (subject `k.SubjectForRun(corr)`, buffer 1024) and
`pulse_subscribe` (pattern, buffer 4096, optional journal replay first), and folds the journal (`k.Journal().Range`) for every
`*_log` / `*_stats` / `runs_*` / `agent_activity` / `inbox` / `changelog` view through the shared newest-first engine
`platform/journalview` (limit clamp, `<ms>:<seq>` cursor, `since_ms`, newest-first); typed app reads call `ProjectValues` (the
native `projectJournal` map wrapper is gone). It publishes only a few events
itself (e.g. `publishMarket`, `publishToolbox`, `publishOperatorAction` for operator wakes/resolutions, remote execution-profile mirror
events, `catalog.synced`/`catalog.sync_failed`, run cost-cap advisories); state changes are journaled by the subsystem the handler calls
(`roster.*`, `mcp.*`, `scripttool.*`, `policy.changed`, `standing.*`, `memory.*` ...).

### 1.9 The `run` op lifecycle (most important op)

`handleRun` (`server_handle_run.go`, StreamEvents, TenantAllowed):
1. Validate `args.intent`; resolve kernel via `kernelFor(args.tenant)`.
2. Optional overrides: `model` (M148), `agent` (the whole roster profile via `runtime.WithAgentProfile`, as for every other entry point: system prompt,
   model + fallback chain, tool allow/deny, trust ceiling, memory scope, workspace, ledger identity; plus its per-run cost ceiling and
   execution profile as defaults. Explicit per-run flags win; only explicit model/system args call the override setters, preserving profile provenance for resume (W2.2b). Unknown, retired, paused or managed-subagent agents are rejected), `images` (shared `runtime.Kernel.AdmitImages`: confirmed vision or sidecar caption, otherwise correlated rejection), `system` (M149),
   `timeout` (Go duration, M154), `tools` allowlist (M158; empty list = no tools), `max_cost_mc` (M166), `execution_profile`
   (ssh/k8s/modal/daytona/remote-agezt — each checks backend availability), `remote_peer`, `auto_approve_caps`,
   `prompt_injection_trust`, `assure` (M651).
3. `dry_run=true` → returns `buildRunPlan` (model, priced?, system-prompt source, timeout, effective tool set) without spending.
4. `sub := k.Bus().Subscribe(k.SubjectForRun(corr), 1024)` BEFORE starting (no missed events).
5. Launch in goroutine: `k.RunTool(... "remote_run" ...)` for remote-agezt, `k.RunAssured`, `k.RunWithRetry` (agent retry policy) or `k.RunWith`.
6. Optional cancel-on-disconnect watcher (`cancelOnDisconnect`).
7. Forward each bus event as `RespEvent`; on completion drain, enrich result with cost/iters/model (journal fold) and `agent`, send `RespResult`.

### 1.10 Complete op table (deliverable b)

Legend — **auth**: `tenant` = TenantAllowed (+TenantRouted; tenant tokens may call it), `primary, tenant-routed` = primary token only
but acts on `args.tenant`'s kernel, `primary` = primary token only, primary kernel. **stream**: `events` = StreamEvents, `LIVE` =
StreamLive. **Web UI route(s)**: the `kernel/webui` route that proxies it (blank = CLI/SDK only). 325 ops total, 212 reachable from the
Web UI. Generated from source (registry funcs × `Cmd*` constants × handler definitions).

#### Core lifecycle: run / halt / resume / why / approvals / plan — `registerCoreCommands` (server_handle_run_remote.go), 9 ops plus the file operations

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `file_mkdir` | primary | | `handleFileMutation` → files.go → app/files (file.write) | `/api/files/mkdir` |
| `file_rename` | primary | | `handleFileMutation` → files.go → app/files (file.write) | `/api/files/rename` |
| `file_delete` | primary | | `handleFileMutation` → files.go → app/files (file.delete) | `/api/files/delete` |
| `file_restore` | primary | | `handleFileRestore` → files_restore.go → app/files (file.write/file.delete) | `/api/rollback/apply` (file snapshots) |
| `approvals` | primary |  | `handleAppOperation` → app/approvals.Live.Pending | `/api/approvals` |
| `decide` | primary |  | `handleAppOperation` → app/approvals.Live.Decide | `/api/decide` |
| `halt` | primary |  | `handleAppOperation` → app/system.Lifecycle.Halt | `/api/halt` |
| `journal_verify` | primary |  | `handleAppOperation` → app/system.Lifecycle.Verify |  |
| `plan` | primary | events | `handlePlan` → server_handlers_plan.go | `/api/plan/run` |
| `resume` | primary |  | `handleAppOperation` → app/system.Lifecycle.Resume | `/api/resume` |
| `run` | tenant | events | `handleRun` → server_handle_run.go | `/api/run` (SSE) |
| `version` | primary |  | `handleVersion` → server_commands.go | `/api/version` |
| `whoami` | tenant |  | `handleWhoami` → server_commands.go |  |
| `why` | tenant |  | `handleWhy` → server_commands.go |  |

#### Daemon operations, runs, state, storage, update — `registerDaemonOpsCommands` (registry.go), 19 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `attention` | primary |  | `handleAttention` → handle_spend_attention.go | `/api/attention` |
| `autonomy_feed` | primary |  | `handleAutonomyFeed` → autonomy_feed.go | `/api/autonomy` |
| `disk_stats` | primary |  | `handleDiskStats` → disk.go |  |
| `pulse_subscribe` | primary |  | `handlePulseSubscribe` → pulse.go |  |
| `reaper_scan` | primary |  | `handleReaperScan` → reaper.go | `/api/reaper/scan` |
| `redact_test` | primary |  | `handleRedactTest` → redact_test_cmd.go | `/api/redact/test` |
| `runs_list` | tenant |  | `handleAppOperation` → app/runs.Service.List | `/api/runs` |
| `runs_stats` | tenant |  | `handleAppOperation` → app/runs.Service.Stats |  |
| `sandbox_delete` | primary |  | `handleSandboxDelete` → sandbox.go | `/api/sandbox/delete` |
| `sandbox_file` | primary |  | `handleSandboxFile` → sandbox.go | `/api/sandbox_file` |
| `sandbox_list` | primary |  | `handleSandboxList` → sandbox.go | `/api/sandbox` |
| `shutdown` | primary |  | `handleAppOperation` → app/system.Lifecycle.Shutdown |  |
| `spend_today` | primary |  | `handleSpendToday` → handle_spend_attention.go | `/api/spend/today` |
| `state_get` | primary |  | `handleStateGet` → state.go |  |
| `state_list` | primary |  | `handleStateList` → state.go |  |
| `status` | primary |  | `handleStatus` → status.go | `/api/status` |
| `storage_stats` | primary |  | `handleStorageStats` → storage.go |  |
| `update_apply` | primary |  | `Service.Apply` → app/update/service.go (typed shared binding/after-write ownership) |  |
| `update_check` | primary |  | `Service.Check` → app/update/service.go (typed shared binding/terminal cleanup) |  |

#### Journal reads and journal-folded audit logs — typed app reads plus native folds, 27 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `approvals_log` | tenant |  | `handleAppOperation` → app/approvals.History.Log | `/api/approvals_log` |
| `approvals_stats` | tenant |  | `handleAppOperation` → app/approvals.History.Stats |  |
| `cache_stats` | tenant |  | `handleAppOperation` → app/journal.Service.CacheStats |  |
| `changelog` | primary, tenant-routed |  | `handleAppOperation` → app/journal.Service.Changelog |  |
| `edict_log` | tenant |  | `handleAppOperation` → app/edict.Decisions.Log | `/api/policy_log` |
| `edict_stats` | tenant |  | `handleAppOperation` → app/edict.Decisions.Stats | `/api/policy` |
| `journal_export` | primary |  | `handleAppOperation` → app/journal.Service.Export |  |
| `journal_grep` | primary |  | `handleAppOperation` → app/journal.Service.Grep | `/api/journal` |
| `journal_head` | primary |  | `handleAppOperation` → app/journal.Service.Head |  |
| `journal_stats` | primary, tenant-routed |  | `handleAppOperation` → app/journal.Service.Stats |  |
| `journal_tail` | primary |  | `handleAppOperation` → app/journal.Service.Tail |  |
| `netguard_log` | tenant |  | `handleAppOperation` → app/audit.Service.NetguardLog | `/api/netguard_log` |
| `provider_log` | tenant |  | `handleAppOperation` → app/providers observation operations | `/api/provider_log` |
| `provider_rejections` | tenant |  | `handleAppOperation` → app/providers observation operations |  |
| `provider_stats` | tenant |  | `handleAppOperation` → app/providers observation operations |  |
| `ratelimit_log` | tenant |  | `handleAppOperation` → app/audit.Service.RateLimitLog | `/api/ratelimit_log` |
| `ratelimit_stats` | tenant |  | `handleAppOperation` → app/audit.Service.RateLimitStats |  |
| `schedule_fires` | tenant |  | `handleScheduleFires` → schedule_fires.go | `/api/schedule/fires` |
| `schedule_stats` | tenant |  | `handleScheduleStats` → schedule_fires_stats.go |  |
| `tool_log` | tenant |  | `handleToolLog` → tool_log.go | `/api/tool_log` |
| `tool_stats` | tenant |  | `handleToolStats` → tool_log.go |  |
| `warden_log` | tenant |  | `handleAppOperation` → app/audit.Service.WardenLog | `/api/warden_log` |
| `warden_stats` | tenant |  | `handleAppOperation` → app/audit.Service.WardenStats |  |
| `webhook_log` | tenant |  | `Observability.Log` → app/webhook/observability.go (typed shared binding) | `/api/webhook_log` |
| `webhook_stats` | tenant |  | `Observability.Stats` → app/webhook/observability.go (typed shared binding) |  |

#### Providers, keys, OAuth, routing, chains, budget, config — `registerProviderConfigCommands` (registry.go), 18 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `budget` | primary |  | `handleBudget` → budget.go | `/api/budget` |
| `budget_set` | primary |  | `handleBudgetSet` → budget.go |  |
| `chains_get` | primary |  | `handleChainsGet` → chains.go | `/api/chains` |
| `chains_set` | primary |  | `handleChainsSet` → chains.go | `/api/chains/set` |
| `config` | primary | read | `Service.Show` → app/config/show.go (typed app spec; selected runtime/env adapters) | `/api/config` |
| `execution_profile_check` | tenant |  | `handleExecutionProfileCheck` → execution_profiles.go | `/api/execution_profile_check` |
| `execution_profile_show` | tenant |  | `handleExecutionProfileShow` → execution_profiles.go |  |
| `execution_profiles` | tenant |  | `handleExecutionProfiles` → execution_profiles.go | `/api/execution_profiles` |
| `provider_key_activate` | primary |  | `handleProviderKeyActivate` → provider_keys.go | `/api/provider/keys/activate` |
| `provider_key_add` | primary |  | `handleProviderKeyAdd` → provider_keys.go | `/api/provider/keys/add` |
| `provider_key_list` | primary |  | `handleProviderKeyList` → provider_keys.go | `/api/provider/keys` |
| `provider_key_remove` | primary |  | `handleProviderKeyRemove` → provider_keys.go | `/api/provider/keys/remove` |
| `provider_oauth_import` | primary |  | `handleProviderOAuthImport` → provider_oauth.go | `/api/provider/oauth/import` |
| `provider_oauth_logout` | primary |  | `handleProviderOAuthLogout` → provider_oauth.go | `/api/provider/oauth/logout` |
| `provider_oauth_start` | primary |  | `handleProviderOAuthStart` → provider_oauth.go | `/api/provider/oauth/start` |
| `provider_oauth_status` | primary |  | `handleProviderOAuthStatus` → provider_oauth.go | `/api/provider/oauth/status` |
| `routing_get` | primary |  | `handleRoutingGet` → routing.go | `/api/routing` |
| `routing_set` | primary |  | `handleRoutingSet` → routing.go | `/api/routing/set` |

#### Model catalog + provider connect/reload — `registerCatalogCommands` (catalog.go), 5 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `catalog_discover` | primary |  | `handleCatalogDiscover` → catalog.go |  |
| `catalog_list` | primary |  | `handleCatalogList` → catalog.go | `/api/catalog` |
| `catalog_sync` | primary |  | `handleCatalogSync` → catalog.go | `/api/catalog/sync` |
| `provider_connect` | primary |  | `handleProviderConnect` → catalog_provider.go | `/api/provider/connect` |
| `provider_reload` | primary |  | `handleProviderReload` → catalog_provider.go | `/api/provider/reload` |

#### Channels, inbox, outbound send, ACP — `registerChannelCommands` (registry.go), 12 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `acp_agents` | primary |  | `ACPInventory.List` → app/channels/acp.go (typed shared operation binding) | `/api/acp/agents` |
| `channel_account_remove` | primary |  | `Accounts.Remove` → app/channels/accounts.go (native codec/manual audit remains) | `/api/channel/account/remove` |
| `channel_account_set` | primary |  | `Accounts.Set` → app/channels/accounts.go (native codec/manual audit remains) | `/api/channel/account/set` |
| `channel_list` | primary |  | `Inventory.List` → app/channels/inventory.go (typed shared operation binding) | `/api/channels` |
| `channel_oauth_callback` | primary |  | `OAuth.Callback` → app/channels/oauth.go (typed shared operation binding) | `/oauth/callback` |
| `channel_oauth_start` | primary |  | `OAuth.Start` → app/channels/oauth.go (typed shared operation binding) | `/api/channel/oauth/start` |
| `channel_oauth_status` | primary |  | `OAuth.Status` → app/channels/oauth.go (typed shared operation binding) | `/api/channel/oauth/status` |
| `inbox` | primary |  | `Inbox.List` → app/channels/inbox.go (typed shared operation binding) | `/api/inbox` |
| `provider_probe` | primary |  | `Probe.CheckContext` → app/providers/probe.go (typed app binding) | `/api/provider/probe` |
| `send` | primary |  | `Outbound.Send` → app/channels/send.go (typed shared binding/terminal cleanup ownership) | `/api/send` |
| `whatsappgw_qr` | primary |  | `Gateway.QR` → app/channels/gateway.go (typed shared operation binding) | `/api/whatsappgw/qr` |
| `whatsappgw_status` | primary |  | `Gateway.Status` → app/channels/gateway.go (typed shared operation binding) | `/api/whatsappgw/status` |

#### Cognition: planner, council, conductor, research, persona, prompts, seats, taste — `registerCognitionCommands` (registry.go), 25 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `chat_suggestions` | primary |  | `handleChatSuggestions` → chatsuggestions.go | `/api/suggestions` |
| `chat_summarize` | primary | LIVE | `handleChatSummarize` → chatsummary.go | `/api/chat/summarize` |
| `conductor_ask` | primary | LIVE | `handleConductorAsk` → conductor.go |  |
| `conductor_roles` | primary |  | `handleConductorRoles` → conductor.go |  |
| `council_ask` | primary | LIVE | `handleCouncilAsk` → council.go | `/api/council/ask` |
| `council_members` | primary |  | `handleCouncilMembers` → council.go | `/api/council/members` |
| `council_set` | primary |  | `handleCouncilSet` → council.go | `/api/council/set` |
| `node_registry` | primary |  | `handleNodeRegistry` → nodes.go | `/api/nodes` |
| `persona_get` | primary |  | `handlePersonaGet` → persona.go | `/api/persona` |
| `persona_set` | primary |  | `handlePersonaSet` → persona.go | `/api/persona/set` |
| `plan_generate` | primary | LIVE | `handlePlanGenerate` → planner.go | `/api/plan/generate` |
| `plan_history` | tenant |  | `handleAppOperation` → app/runs.Plans.List | `/api/plan_history` |
| `plan_refine` | primary | LIVE | `handlePlanRefine` → planner.go | `/api/plan/refine` |
| `plan_stats` | tenant |  | `handleAppOperation` → app/runs.Plans.Stats |  |
| `prompts_get` | primary |  | `handlePromptsGet` → prompts.go | `/api/prompts` |
| `prompts_set` | primary |  | `handlePromptsSet` → prompts.go | `/api/prompts/set` |
| `reflect_run` | primary |  | `handleReflectRun` → reflect.go |  |
| `reflect_show` | primary |  | `handleReflectShow` → reflect.go |  |
| `research_ask` | primary | LIVE | `handleResearchAsk` → research.go | `/api/research/ask` |
| `seat_create` | primary |  | `handleSeatCreate` → seat.go |  |
| `seat_delete` | primary |  | `handleSeatDelete` → seat.go |  |
| `seat_list` | primary |  | `handleSeatList` → seat.go |  |
| `taste_create` | primary |  | `handleTasteCreate` → taste.go |  |
| `taste_delete` | primary |  | `handleTasteDelete` → taste.go |  |
| `taste_list` | primary |  | `handleTasteList` → taste.go |  |

#### Artifacts, plugins, tools, toolbox, edict overlay — `registerMiscSmallCommands` (registry.go), 13 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `agent_capabilities` | primary |  | `handleAgentCapabilities` → tool_agents.go | `/api/agents/capabilities` |
| `agent_permissions` | primary |  | `handleAgentPermissions` → tool_agents.go | `/api/agents/permissions` |
| `artifact_collect` | primary |  | `handleArtifactCollect` → artifact.go | `/api/artifact/collect` |
| `artifact_delete` | primary |  | `handleArtifactDelete` → artifact.go | `/api/artifact/delete` |
| `artifact_get` | primary |  | `handleArtifactGet` → artifact.go | `/api/artifact/raw` |
| `artifact_list` | primary |  | `handleArtifactList` → artifact.go | `/api/artifacts` |
| `edict_compact` | primary, tenant-routed |  | `handleAppOperation` → app/edict.Overlay.Compact |  |
| `edict_overlay` | tenant |  | `handleAppOperation` → app/edict.Overlay.Show |  |
| `plugin_list` | primary | read | `Service.List` → app/plugins/inventory.go (typed app spec; selected manifest adapter) |  |
| `tool_list` | primary |  | `handleToolList` → tool.go | `/api/tools_catalog` |
| `toolbox_detect` | primary | read | `ToolboxReads.Detect` → app/tools/toolbox_reads.go (typed app registration) |  |
| `toolbox_install` | primary | events | `ToolboxInstall.Install` → app/tools/toolbox_install.go (typed StreamEvents registration) | `/api/toolbox/install` (SSE adapter remains) |
| `toolbox_outdated` | primary | read | `ToolboxReads.Outdated` → app/tools/toolbox_reads.go (typed app registration) |  |

#### Edict policy (tenant-routed) — reads via `app/edict.Operations`, audited writes via `app/edict.WriteOperations`, 7 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `edict_deny_add` | tenant |  | `handleAppOperation` → app/edict.Writes.DenyAdd | `/api/edict/deny_add` |
| `edict_deny_list` | tenant |  | `handleAppOperation` → app/edict.Service.DenyList |  |
| `edict_deny_rm` | tenant |  | `handleAppOperation` → app/edict.Writes.DenyRemove | `/api/edict/deny_rm` |
| `edict_set_level` | tenant |  | `handleAppOperation` → app/edict.Writes.SetLevel | `/api/edict/set_level` |
| `edict_set_mode` | tenant |  | `handleAppOperation` → app/edict.Writes.SetMode | `/api/edict/set_mode` |
| `edict_show` | tenant |  | `handleAppOperation` → app/edict.Service.Show | `/api/edict_show` |
| `edict_test` | tenant |  | `handleAppOperation` → app/edict.Service.Test | `/api/edict/test` |

#### Live run control — `app/steer.Operations` via the common native adapter, 6 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `cancel_run` | tenant |  | `handleAppOperation` → app/steer.Service.Cancel | `/api/cancel_run` |
| `run_intervene` | tenant |  | `handleAppOperation` → app/steer.Service.Intervene |  |
| `run_pause` | tenant |  | `handleAppOperation` → app/steer.Service.Pause | `/api/run/pause` |
| `run_resume` | tenant |  | `handleAppOperation` → app/steer.Service.Resume | `/api/run/resume` |
| `run_steer` | tenant |  | `handleAppOperation` → app/steer.Service.Steer | `/api/run/steer` |
| `run_step` | tenant |  | `handleAppOperation` → app/steer.Service.Step | `/api/run/step` |

#### Multi-tenancy registry — `app/tenants.Operations` via the common native adapter, 6 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `tenant_create` | primary |  | `handleAppOperation` → app/tenants.Service.Create |  |
| `tenant_list` | primary |  | `handleAppOperation` → app/tenants.Service.List |  |
| `tenant_release` | primary |  | `handleAppOperation` → app/tenants.Service.Release |  |
| `tenant_remove` | primary |  | `handleAppOperation` → app/tenants.Service.Remove |  |
| `tenant_stats` | primary, tenant-routed |  | `handleAppOperation` → app/tenants.Service.Stats |  |
| `tenant_token` | primary |  | `handleAppOperation` → app/tenants.Service.Token |  |

#### Agent roster (agents) — `app/roster` operations via the common native adapter, 17 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `agent_activity` | primary |  | `handleAppOperation` → app/roster.ActivityService.Activity | `/api/agents/activity` |
| `agent_add` | primary |  | `handleAppOperation` → app/roster.ProfileWriteService.Add | `/api/agents/add` |
| `agent_edit` | primary |  | `handleAppOperation` → app/roster.ProfileWriteService.Edit | `/api/agents/edit` |
| `agent_escalations` | primary |  | `handleAppOperation` → app/roster.EscalationService.Escalations | `/api/agents/escalations` |
| `agent_graveyard` | primary |  | `handleAppOperation` → app/roster.GraveyardService.Graveyard |  |
| `agent_impact` | primary |  | `handleAppOperation` → app/roster.ImpactService.Impact | `/api/agents/impact` |
| `agent_list` | primary |  | `handleAppOperation` → app/roster.ListService.List | `/api/agents` |
| `agent_remove` | primary |  | `handleAppOperation` → app/roster.RemoveService.Remove | `/api/agents/remove` |
| `agent_repair` | primary |  | `handleAppOperation` → app/roster.RepairService.Repair | `/api/agents/repair` |
| `agent_repair_status` | primary |  | `handleAppOperation` → app/roster.RepairStatusService.RepairStatus | `/api/agents/repair_status` |
| `agent_resolve` | primary |  | `handleAppOperation` → app/roster.ResolveService.Resolve | `/api/agents/resolve` |
| `agent_retire` | primary |  | `handleAppOperation` → app/roster.SetRetiredService.Retire | `/api/agents/retire` |
| `agent_revive` | primary |  | `handleAppOperation` → app/roster.SetRetiredService.Revive | `/api/agents/revive` |
| `agent_set_enabled` | primary |  | `handleAppOperation` → app/roster.SetEnabledService.SetEnabled | `/api/agents/enable` |
| `agent_task_update` | primary |  | `handleAppOperation` → app/roster.TaskUpdateService.TaskUpdate | `/api/agents/task` |
| `agent_tombstone` | primary |  | `handleAppOperation` → app/roster.ImpactService.Tombstone |  |
| `agent_wake` | primary |  | `handleAppOperation` → app/roster.WakeService.Wake | `/api/agents/wake` |

#### Memory — `app/memory.Operations` via the common native adapter, 16 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `memory_add` | primary |  | `handleAppOperation` → app/memory.Service.Remember | `/api/memory/add` |
| `memory_audit` | tenant |  | `handleAppOperation` → app/memory.Service.Audit | `/api/memory/audit` |
| `memory_bulk_forget` | primary |  | `handleAppOperation` → app/memory.Service.BulkForget | `/api/memory/bulk_forget` |
| `memory_clean` | tenant |  | `handleAppOperation` → app/memory.Service.Clean | `/api/memory/clean` |
| `memory_consolidate` | primary |  | `handleAppOperation` → app/memory.Distillation.Consolidate |  |
| `memory_find_related` | primary |  | `handleAppOperation` → app/memory.Service.FindRelated |  |
| `memory_forget` | primary |  | `handleAppOperation` → app/memory.Service.Forget | `/api/memory/forget` |
| `memory_get` | primary |  | `handleAppOperation` → app/memory.Service.Get |  |
| `memory_list` | primary |  | `handleAppOperation` → app/memory.Service.PrepareList / PreparedList.Page | `/api/memory` |
| `memory_log` | tenant |  | `handleAppOperation` → app/memory.LogService.Log | `/api/memory_log` |
| `memory_promote` | primary |  | `handleAppOperation` → app/memory.Service.Promote | `/api/memory/promote` |
| `memory_prune` | primary |  | `handleAppOperation` → app/memory.Service.Prune | `/api/memory/prune` |
| `memory_search` | primary |  | `handleAppOperation` → app/memory.Service.Search |  |
| `memory_supersede` | primary |  | `handleAppOperation` → app/memory.Service.Supersede | `/api/memory/supersede` |
| `memory_tidy` | primary |  | `handleAppOperation` → app/memory.Service.Tidy | `/api/memory/tidy` |
| `profile_rebuild` | primary |  | `handleAppOperation` → app/memory.Distillation.RebuildProfile | `/api/profile/rebuild` |

#### World model — `app/world.Operations` via the common native adapter, 9 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `world_add` | primary |  | `handleAppOperation` → app/world.Service.Add | `/api/world/add` |
| `world_edit` | primary |  | `handleAppOperation` → app/world.Service.Edit | `/api/world/edit` |
| `world_forget` | primary |  | `handleAppOperation` → app/world.Service.Forget | `/api/world/forget` |
| `world_get` | primary |  | `handleAppOperation` → app/world.Service.Get |  |
| `world_list` | primary |  | `handleAppOperation` → app/world.Service.List | `/api/world` |
| `world_log` | tenant |  | `handleAppOperation` → app/world.LogService.Log | `/api/world_log` |
| `world_neighbors` | primary |  | `handleAppOperation` → app/world.Service.Neighbors |  |
| `world_relate` | primary |  | `handleAppOperation` → app/world.Service.Relate | `/api/world/relate` |
| `world_resolve` | primary |  | `handleAppOperation` → app/world.Service.Resolve |  |

#### Skills — `registerSkillCommands` (skill_view.go), 14 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `skill_archive` | primary |  | `handleSkillArchive` → skill.go | `/api/skill/archive` |
| `skill_files` | primary |  | `handleSkillFiles` → skill_files.go | `/api/skill/files` |
| `skill_get` | primary |  | `handleSkillGet` → skill.go |  |
| `skill_history` | primary |  | `handleSkillHistory` → skill.go |  |
| `skill_hygiene` | primary |  | `handleSkillHygiene` → skill_files.go | `/api/skills/hygiene` |
| `skill_import` | primary |  | `handleSkillImport` → skill.go | `/api/skill/import` |
| `skill_list` | primary |  | `handleSkillList` → skill.go | `/api/skills` |
| `skill_promote` | primary |  | `handleSkillPromote` → skill.go | `/api/skill/promote` |
| `skill_quarantine` | primary |  | `handleSkillQuarantine` → skill.go | `/api/skill/quarantine` |
| `skill_read_file` | primary |  | `handleSkillReadFile` → skill_files.go |  |
| `skill_reassign` | primary |  | `handleSkillReassign` → skill.go |  |
| `skill_restore` | primary |  | `handleSkillRestore` → skill.go | `/api/rollback/apply` |
| `skill_revert` | primary |  | `handleSkillRevert` → skill.go | `/api/skill/revert` |
| `skill_share` | primary |  | `handleSkillShare` → skill.go | `/api/skill/share` |

#### Standing orders — `registerStandingCommands` (standing_fire.go), 7 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `standing_add` | primary |  | `handleStandingAdd` → standing_handlers.go | `/api/standing/add` |
| `standing_edit` | primary |  | `handleStandingEdit` → standing_handlers.go | `/api/standing/edit` |
| `standing_fire` | primary |  | `handleStandingFire` → standing_fire.go | `/api/standing/fire` |
| `standing_list` | primary |  | `handleStandingList` → standing_handlers.go | `/api/standing` |
| `standing_remove` | primary |  | `handleStandingRemove` → standing_handlers.go | `/api/standing/remove` |
| `standing_set_enabled` | primary |  | `handleStandingSetEnabled` → standing_handlers.go | `/api/standing/enable` |
| `standing_why` | primary |  | `handleStandingWhy` → standing_why.go | `/api/standing/why` |

#### Schedules (cadence) — `registerScheduleCommands` (schedule.go), 8 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `schedule_add` | primary |  | `handleScheduleAdd` → schedule_add.go | `/api/schedule/add` |
| `schedule_edit` | primary |  | `handleScheduleEdit` → schedule_edit.go | `/api/schedule/edit` |
| `schedule_enable` | primary |  | `handleScheduleEnable` → schedule.go | `/api/schedule/enable` |
| `schedule_list` | primary |  | `handleScheduleList` → schedule_helpers.go | `/api/schedules` |
| `schedule_rm` | primary |  | `handleScheduleRemove` → schedule_misc.go | `/api/schedule/remove` |
| `schedule_run` | primary |  | `handleScheduleRun` → schedule_misc.go | `/api/schedule/run` |
| `schedule_system_tasks` | primary |  | `handleScheduleSystemTasks` → schedule_misc.go | `/api/schedule/system_tasks` |
| `schedule_test` | primary |  | `handleScheduleTest` → schedule.go | `/api/schedule/test` |

#### Pulse control — `registerPulseControlCommands` (pulse_control.go), 13 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `pulse_ask_resolve` | primary |  | `handlePulseAskResolve` → pulse_control.go |  |
| `pulse_asks` | primary |  | `handlePulseAsks` → pulse_control.go |  |
| `pulse_beat` | primary |  | `handlePulseBeat` → pulse_control.go | `/api/pulse/beat` |
| `pulse_cadence` | primary |  | `handlePulseCadence` → pulse_control.go | `/api/pulse/cadence` |
| `pulse_dial` | primary |  | `handlePulseDial` → pulse_control_helpers.go | `/api/pulse/dial` |
| `pulse_flush` | primary |  | `handlePulseFlush` → pulse_control_helpers.go | `/api/pulse/flush` |
| `pulse_pause` | primary |  | `handlePulsePause` → pulse_control.go | `/api/pulse/pause` |
| `pulse_probe` | primary |  | `handlePulseProbe` → pulse_control_helpers.go | `/api/pulse/probe` |
| `pulse_quiet` | primary |  | `handlePulseQuiet` → pulse_control_helpers.go | `/api/pulse/quiet` |
| `pulse_resume` | primary |  | `handlePulseResume` → pulse_control.go | `/api/pulse/resume` |
| `pulse_status` | primary |  | `handlePulseStatus` → pulse_control.go | `/api/pulse` |
| `pulse_unwatch` | primary |  | `handlePulseUnwatch` → pulse_control_helpers.go | `/api/pulse/unwatch` |
| `pulse_watch` | primary |  | `handlePulseWatch` → pulse_control_helpers.go | `/api/pulse/watch` |

#### Workflows — `registerWorkflowCommands` (workflow_register.go), 13 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `workflow_draft` | primary |  | `handleWorkflowDraft` → workflow_handlers_run.go | `/api/workflows/draft` |
| `workflow_list` | primary |  | `handleWorkflowList` → workflow_handlers.go | `/api/workflows` |
| `workflow_refine` | primary |  | `handleWorkflowRefine` → workflow_handlers_run.go | `/api/workflows/refine` |
| `workflow_remove` | primary |  | `handleWorkflowRemove` → workflow_handlers.go | `/api/workflows/remove` |
| `workflow_restore` | primary |  | `handleWorkflowRestore` → workflow_handlers.go | `/api/rollback/apply` |
| `workflow_run` | primary |  | `handleWorkflowRun` → workflow_handlers_run.go | `/api/workflows/run` |
| `workflow_runs` | primary |  | `handleWorkflowRuns` → workflow_handlers_runs.go | `/api/workflows/runs` |
| `workflow_save` | primary |  | `handleWorkflowSave` → workflow_handlers.go | `/api/workflows/save` |
| `workflow_set_enabled` | primary |  | `handleWorkflowSetEnabled` → workflow_handlers.go | `/api/workflows/enable` |
| `workflow_show` | primary |  | `handleWorkflowShow` → workflow_handlers.go | `/api/workflows/show` |
| `workflow_templates` | primary |  | `handleWorkflowTemplates` → workflow_handlers_run.go | `/api/workflows/templates` |
| `workflow_test_node` | primary |  | `handleWorkflowTestNode` → workflow_handlers_invoke.go | `/api/workflows/test_node` |
| `workflow_webhook` | primary |  | `handleWorkflowWebhook` → workflow_handlers_invoke.go | `/hooks/{name}` |

#### Workboard (tasks, dispatch, proof) — `registerWorkboardCommands` (workboard_dispatch_register.go), 21 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `workboard_archive` | primary |  | `handleWorkboardArchive` → workboard_handlers.go |  |
| `workboard_block` | primary |  | `handleWorkboardBlock` → workboard_handlers.go |  |
| `workboard_claim` | primary |  | `handleWorkboardClaim` → workboard_handlers.go |  |
| `workboard_comment` | primary |  | `handleWorkboardComment` → workboard_handlers.go |  |
| `workboard_complete` | primary |  | `handleWorkboardComplete` → workboard_handlers.go |  |
| `workboard_create` | primary |  | `handleWorkboardCreate` → workboard_handlers.go |  |
| `workboard_depend` | primary |  | `handleWorkboardDepend` → workboard_handlers_link.go |  |
| `workboard_dispatch` | primary |  | `handleWorkboardDispatch` → workboard_handlers_dispatch.go |  |
| `workboard_fail` | primary |  | `handleWorkboardFail` → workboard_handlers.go |  |
| `workboard_heartbeat` | primary |  | `handleWorkboardHeartbeat` → workboard_handlers.go |  |
| `workboard_lanes` | primary |  | `handleWorkboardLanes` → workboard_handlers.go |  |
| `workboard_link` | primary |  | `handleWorkboardLink` → workboard_handlers_link.go |  |
| `workboard_list` | primary |  | `handleWorkboardList` → workboard.go |  |
| `workboard_policy` | primary |  | `handleWorkboardPolicy` → workboard_handlers_link.go |  |
| `workboard_prove` | primary |  | `handleWorkboardProve` → workboard_handlers.go |  |
| `workboard_reclaim` | primary |  | `handleWorkboardReclaim` → workboard_handlers_link.go |  |
| `workboard_seat` | primary |  | `handleWorkboardSeat` → workboard_handlers.go |  |
| `workboard_show` | primary |  | `handleWorkboardShow` → workboard_handlers.go |  |
| `workboard_sweep` | primary |  | `handleWorkboardSweep` → workboard_handlers_link.go |  |
| `workboard_unblock` | primary |  | `handleWorkboardUnblock` → workboard_handlers.go |  |
| `workboard_watch` | primary |  | `handleWorkboardWatch` → workboard_handlers_dispatch.go |  |

#### Message board / mailbox — `registerBoardCommands` (board_handlers.go), 7 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `board_ack` | primary |  | `handleBoardAck` → board_handlers.go | `/api/board/ack` |
| `board_get` | primary |  | `handleBoardGet` → board_handlers.go |  |
| `board_help` | primary |  | `handleBoardHelp` → board_handlers.go | `/api/board/help` |
| `board_inbox` | primary |  | `handleBoardInbox` → board_handlers.go |  |
| `board_read` | primary |  | `handleBoardRead` → board_handlers.go | `/api/board` |
| `board_replies` | primary |  | `handleBoardReplies` → board_handlers.go |  |
| `board_send` | primary |  | `handleBoardSend` → board_handlers.go | `/api/board/send` |

#### Marketplace — 8 typed app operations

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `market_add_source` | primary |  | `Writes.AddSource` → app/market/writes.go (typed app registration) | `/api/market/source/add` |
| `market_install` | primary | events | `Writes.Install` → app/market/writes.go (typed app registration) | `/api/market/install` (SSE) |
| `market_list` | primary | read | `Reads.List` → app/market/reads.go (typed app registration) | `/api/market` |
| `market_remove_source` | primary |  | `Writes.RemoveSource` → app/market/writes.go (typed app registration) | `/api/market/source/remove` |
| `market_show` | primary | read | `Reads.Show` → app/market/reads.go (typed app registration) | `/api/market/show` |
| `market_sources` | primary | read | `Reads.Sources` → app/market/reads.go (typed app registration) | `/api/market/sources` |
| `market_sync` | primary |  | `Writes.Sync` → app/market/writes.go (typed app registration) | `/api/market/sync` |
| `market_uninstall` | primary | events | `Writes.Uninstall` → app/market/writes.go (typed app registration) | `/api/market/uninstall` (SSE) |

#### MCP servers — 6 typed app operations

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `mcp_add` | primary | write | `MCPLifecycle.Add` → app/tools/mcp_lifecycle.go | `/api/mcp/add` |
| `mcp_attach` | primary | write | `MCPLifecycle.Attach` → app/tools/mcp_lifecycle.go | `/api/mcp/attach` |
| `mcp_detach` | primary | write | `MCPLifecycle.Detach` → app/tools/mcp_lifecycle.go | `/api/mcp/detach` |
| `mcp_list` | primary | read | `MCPCatalog.List` → app/tools/mcp_catalog.go (typed app registration) | `/api/mcp` adapter remains |
| `mcp_remove` | primary | write | `MCPLifecycle.Remove` → app/tools/mcp_lifecycle.go | `/api/mcp/remove` |
| `mcp_set_enabled` | primary | write | `MCPLifecycle.SetEnabled` → app/tools/mcp_lifecycle.go | `/api/mcp/enable` |

#### Tool Forge (script tools) — 8 typed app operations

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `toolforge_draft` | primary | write | `ForgeLifecycle.Draft` → app/tools/forge_lifecycle.go (typed app registration) | mandatory audit |
| `toolforge_edit` | primary | write | `ForgeLifecycle.Edit` → app/tools/forge_lifecycle.go (typed app registration) | mandatory audit |
| `toolforge_list` | primary | read | `ForgeCatalog.List` → app/tools/forge_catalog.go (typed app registration) |  |
| `toolforge_promote` | primary | write | `ForgeLifecycle.Promote` → app/tools/forge_lifecycle.go (typed app registration) | mandatory audit |
| `toolforge_quarantine` | primary | write | `ForgeLifecycle.Quarantine` → app/tools/forge_lifecycle.go (typed app registration) | mandatory audit |
| `toolforge_remove` | primary | write | `ForgeLifecycle.Remove` → app/tools/forge_lifecycle.go (typed app registration) | mandatory audit |
| `toolforge_show` | primary | read | `ForgeCatalog.Show` → app/tools/forge_catalog.go (typed app registration) |  |
| `toolforge_test` | primary | write | `ForgeLifecycle.Test` → app/tools/forge_lifecycle.go (typed app registration) | mandatory audit |

#### Personal data lake — `registerDatalakeCommands` (datalake.go), 7 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `data_collections` | primary |  | `handleDataCollections` → datalake.go | `/api/data/collections` |
| `data_create_collection` | primary |  | `handleDataCreateCollection` → datalake.go |  |
| `data_delete` | primary |  | `handleDataDelete` → datalake.go | `/api/data/delete` |
| `data_drop_collection` | primary |  | `handleDataDropCollection` → datalake.go |  |
| `data_insert` | primary |  | `handleDataInsert` → datalake.go | `/api/data/insert` |
| `data_records` | primary |  | `handleDataRecords` → datalake.go | `/api/data/records` |
| `data_update` | primary |  | `handleDataUpdate` → datalake.go | `/api/data/update` |

#### OKRs — `registerOKRCommands` (okr.go), 7 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `okr_archive` | primary |  | `handleOKRArchive` → okr.go |  |
| `okr_create` | primary |  | `handleOKRCreate` → okr.go |  |
| `okr_keyresult` | primary |  | `handleOKRKeyResult` → okr.go |  |
| `okr_link` | primary |  | `handleOKRLink` → okr.go |  |
| `okr_list` | primary |  | `handleOKRList` → okr.go |  |
| `okr_show` | primary |  | `handleOKRShow` → okr.go |  |
| `okr_unlink` | primary |  | `handleOKRUnlink` → okr.go |  |

#### Settings / config schema — typed app/settings operations, 5 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `config_schema` | primary | read | `Reads.Schema` → app/settings/reads.go (typed app spec; selected native read ports) | `/api/config/schema` |
| `config_schema_register` | primary |  | `Writes.Register` → app/settings/writes.go (typed app spec; selected native write ports) | `/api/config/schema/register` |
| `config_schema_unregister` | primary |  | `Writes.Unregister` → app/settings/writes.go (typed app spec; selected native write ports) | `/api/config/schema/unregister` |
| `config_set` | primary |  | `Writes.Set` → app/settings/writes.go (typed app spec; selected native write ports) | `/api/config/set`, `/api/rollback/apply` |
| `config_values` | primary | read | `Reads.Values` → app/settings/reads.go (typed app spec; selected native read ports) | `/api/config/values` |

#### Config Center (rated agent config) — `registerConfigCenterCommands` (configcenter_register.go), 9 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `configcenter.access` | primary |  | `Writes.SetAccess` → app/configcenter/writes.go (canonical app binding) | `/api/configcenter/access` |
| `configcenter.access-log` | primary |  | `Reads.AccessLog` → app/configcenter/reads.go (canonical app binding) |  |
| `configcenter.audit` | primary |  | `Reads.Audit` → app/configcenter/reads.go (canonical app binding) |  |
| `configcenter.delete` | primary |  | `Writes.Delete` → app/configcenter/writes.go (canonical app binding) | `/api/configcenter/delete` |
| `configcenter.get` | primary |  | `Reads.Get` → app/configcenter/reads.go (canonical app binding) |  |
| `configcenter.health` | primary |  | `Reads.Health` → app/configcenter/reads.go (canonical app binding) |  |
| `configcenter.list` | primary |  | `Reads.List` → app/configcenter/reads.go (canonical app binding) | `/api/configcenter/list` |
| `configcenter.set` | primary |  | `Writes.Set` → app/configcenter/writes.go (canonical app binding) | `/api/configcenter/set` |
| `configcenter.set-rating` | primary |  | `Writes.SetRating` → app/configcenter/writes.go (canonical app binding) | `/api/configcenter/rating` |

### 1.11 File-by-file (all 204 non-test files, grouped by concern)

Most files carry a `Provenance:` header from the "god-file split" refactors (Day-NN); they were carved out of a larger file without API change.

**Protocol, server, client core**

| File | What it does |
|---|---|
| `doc.go` | Package doc: transport, discovery files, wire format, one-request-per-connection rule. |
| `protocol.go` | `Request`, `Response`, `RespEvent/RespResult/RespError`, runtime file names. |
| `protocol_commands.go` | `Cmd*` constants: lifecycle, approvals, plan, catalog/provider, keyring, OAuth, pulse, budget, tools, journal, logs (58 consts). |
| `protocol_commands_data.go` | `Cmd*` constants: datalake, research, config/settings, runs, memory, schedule, tenant, disk/storage (63). |
| `protocol_commands_domains.go` | `Cmd*` constants: world, skill, standing, agent roster, toolforge, toolbox, market, workflow, sandbox, channels, reflect, inbox/send, autonomy, board (132). |
| `protocol_commands_edict.go` | `Cmd*` constants: edict/policy, state, artifact (32). |
| `protocol_commands_misc.go` | `Cmd*` constants: workboard, OKR, taste, seat, update (36). |
| `dispatch.go` | `DispatchCtx`, `Handler`, `StreamMode` (`StreamNone/StreamEvents/StreamLive`), `commandSpec`, `commandRegistry`, `register` (panics on dup/empty/nil). |
| `registry.go` | `init()` → `registerAllCommands` (28 domain register funcs); hosts the themed groups for small files (journal logs, provider config, channels, daemon ops, cognition, misc). |
| `server.go` | `Server` struct and early setters (`SetHTTPBindings`, `SetChannels`, `SetChannelSender`, `SetChatGPTSync`, `SetCredChain`, `SetBoard`, `SetUpdateService`, `SetDiskFree`, `SetCancelOnDisconnect`, `SetConfigEnvPinned`) + injected func types. |
| `deps.go` | Two-phase DI: `Deps` + `NewServerWithDeps`, `LateDeps` + `Bind`. |
| `server_lifecycle.go` | `NewServer`, `Start` (listen 127.0.0.1:0, mint token, write runtime files, start accept loop), `Shutdown()`, `Addr`, `Token`, `Stop`. |
| `server_lifecycle_helpers.go` | `tokenIsPrimary` (constant time, blank never matches), `maxRequestBytes` 16 MiB, `readBoundedLine`. |
| `server_lifecycle_internal.go` | `signalShutdown`, `initiateShutdown` (idempotent; removes runtime files), `writeRuntimeFiles`, `acceptLoop`, `cancelOnConnClose`. |
| `server_handlers.go` | `handleConn` (auth → registry → tenant routing → stream wrapping → handler), `writeResp`, `recoverConn` panic firewall. |
| `respond.go` | `fail`, `failMsg`, `ok` envelope helpers. |
| `args.go` | Typed arg accessors `argString`, `argTruthy`, `argBool`, `requiredArgString`, `argFloat64`, `argInt64`. |
| `args_compound.go` | `argStringList`. |
| `args_specialized.go` | `argLimit`, `argDryRun`, `argFlag`. |
| `client.go` | `Client`, `NewClient`, `ProbeExisting`, `ErrServerError`, `Call`, `CallRaw`, `Stream`, `StreamUntilCancel`. |
| `client_helpers.go` | `dial` (5 s), `setReadDeadlineFromCtx`, timeout→ctx error mapping, `writeRequest`, `readOneResponse`, `toString`, `toBool`. |
| `client_update.go` | Typed client wrappers `UpdateCheck`/`UpdateApply` and their result types; `Close`. |
| `tenant.go` | `tenantService`: binds `app/tenants` to the daemon's registry (none when multi-tenancy is disabled) and each tenant's run activity (`kernelFor` + `collectRuns`). |
| `app/tenants/tenants.go` | Typed operator-only `tenant_create`/`token`/`release`/`remove` (audited; strict id, blank required, untrimmed) and read-only `tenant_list`/`tenant_stats` (per-tenant runs, outcomes, spend, last activity; error rows; closed tenants released again). |
| `tenant_helpers.go` | `tenantOf`, `kernelFor` (Acquire tenant kernel), `SetTenants`. |
| `shutdown.go` | `scheduleShutdown`: after the acknowledgement grace delay, closes `shutdownCh` so the daemon exits via the same path as SIGTERM. |
| `app/system/lifecycle.go` | Typed operator-only `halt`/`resume` (strict reason, audited), `journal_verify` (read-only) and `shutdown` (audited; acknowledges, then schedules the exit). |

**Core run lifecycle**

| File | What it does |
|---|---|
| `server_handle_run.go` | `handleRun` (666 lines): resolves tenant/agent/model/vision/system/timeout/tools/cost/execution-profile/assure overrides, dry-run plan, subscribes to run subject, launches the governed run, streams events, enriches result. |
| `server_handle_run_remote.go` | Remote-agezt execution-profile helpers (run events, answer preview, peer metadata) + `registerCoreCommands`. |
| `server_commands.go` | `why` (correlation walk) and `whoami` handlers. |
| `server_handlers_plan.go` | `handlePlan` (execute a pre-built DAG `planSpec` via kernel scheduler, streaming). |
| `dryrun.go` | `buildRunPlan` + `runPlanInput` consumer: renders the dry-run plan (model, context size warnings, tool set, timeout). |
| `dryrun_format.go` | `formatMicrocentsUSD`. |
| `dryrun_pricing.go` | `modelPriced`, `strictPricingPlan`, `runPlanInput` type. |
| `app/steer/steer.go` | Live run control (M32/M608): typed `cancel_run` and `run_pause/resume/step/steer/intervene` specs, strict codecs and the intervention mapping over the routed kernel's `Runs` port (tenant-routed). |
| `remote_mirror.go` | `mirrorRemoteExecutionProfileEvents`: mirrors a remote peer's run events into the local journal. |
| `remote_mirror_fetch.go` | `fetchRemoteEvents`, `fetchRemoteArtifacts` (HTTP calls to peer REST API). |
| `remote_mirror_helpers.go` | Mirror mode (`AGEZT_REMOTE_EVENT_MIRROR`), peer lookup, payload redaction for mirrored events. |

**Runs listing / status / daemon ops**

| File | What it does |
|---|---|
| `runs.go` | `runEntry`, `runEntryStatus`, `collectRuns` journal walker, and `runReads`, the app/runs snapshot port over the routed kernel. |
| `runs_extract.go` | Payload extractors (intent, agent, tool, iters, cost, model, answer preview, spawn link, reason). |
| `app/runs/runs.go` | Typed `runs_list` (strict status/intent/model, lenient limit/cost, newest-first with seq tie-break, `<ms>:<seq>` cursor, live phase/tool) and `runs_stats` (window, intent scope, success rate, failure reasons, delegation, spend and duration distributions). |
| `status.go` | `handleStatus`: one-round-trip health overview (version, halted, budget, tools, HTTP bindings, channels, AWS cred chain, fallback counts). |
| `disk.go` | `handleDiskStats`: journal size + free space via injected `DiskFreeFunc` (M131). |
| `storage.go` | `handleStorageStats`: per-top-level-dir usage of the home dir (M927). |
| `app/journal/journal.go` | Typed `journal_head` (empty-journal clamp), `journal_tail` (lenient `n` clamped 1..10,000, head before read, Event member order kept by the adapter) and `journal_stats` (count, per-kind, time span, segments/bytes) over the routed kernel's journal. |
| `app/journal/search.go` | Typed `journal_grep` (strict filters, exact kind/subject/actor/correlation + case-insensitive pattern incl. payload, lenient limit 1..10,000 stopping the walk) and `journal_export` (daemon-clock `since_ms`, strict correlation scope, hashes + head at export time, `MaxExportN` truncation). |
| `journal_stats.go` | `journalReads` (the app/journal binding with the on-disk size port, daemon clock and `governor.CostMicrocents`), `countSegments`, and the CLI's `MaxJournalExportN`. |
| `state.go` | `state_list`, `state_get` over the kernel state store. |
| `handle_spend_attention.go` | `spend_today` + `attention` (pending approvals + pulse asks feed) for Mission Control. |
| `update_control.go` | Selected backend/current version/drain/sentinel/delayed shutdown only; old wrappers/map callback codec removed. |
| `app/update/operations.go` | Two typed primary unary specs, strict ordered RawMessage codec, disabled priority and opaque panic result. |
| `contract/opapi/terminal_write.go` | stdlib-only post-response-writer ownership; returned error finishes, panic does not. |
| `app_terminal_write.go` | Closed/idempotent/LIFO post-write callbacks outside lock; panic discards while cleanup remains unconditional. |
| `app/update/backend.go` | Existing verified Check/Apply/DrainResult backend port. |
| `app/update/service.go` | Disabled/validation/presentation/unverified manifest/background contexts and sentinel→response→100ms restart ordering. |
| `sandbox.go` | `sandbox_list/file/delete` for code_exec projects under `sandbox/projects`, path-confined. |
| `reaper.go` | `handleReaperScan`: dead-agent/stale-artifact detection (read-only). |
| `redact_test_cmd.go` | `handleRedactTest`: runs the live redactor against a candidate string. |
| `app/journal/changes.go` | Typed `changelog` (material-change kinds with stable labels, payload detail probe, newest first, lenient limit/window) and `cache_stats` (prompt-cache reads/writes and the saving versus the injected full-rate `Cost`). |

**Audit log folds (journal → paginated lists)**

| File | What it does |
|---|---|
| `app/approvals/live.go` | Typed primary-only `approvals` (waiting requests oldest first with intent/effect metadata, unaudited) and audited `decide` (lenient id/decision/reason, empty id checked first, exact grant/deny, resolved as the operator) over the primary kernel's approval registry. |
| `app/approvals/history.go` | Typed `approvals_log` (one row per approval joined across request and resolution, newest request first, `denied` keeps denials and timeouts, cursor paging) and `approvals_stats` (final-status counts by request time, grant rate over resolved, denials by capability) over the routed kernel's journal + clock. |
| `app/edict/decisions.go` | Typed `edict_log` (strict denied/tool/capability, lenient page) and `edict_stats` (windowed fold, denial rate, denials by capability) over the routed kernel's `policy.decision` records; a malformed payload zeroes the whole decision. |
| `app/providers/observation_operations.go` | Typed `provider_log`, `provider_stats`, `provider_rejections` over `routing.decision`/`provider.fallback` (the native `provider_log.go` was removed in an earlier provider slice). |
| `app/audit/audit.go` | Typed guard audit reads: `netguard_log`, `ratelimit_log`/`ratelimit_stats`, `warden_log` (per-kind keys, `issues` filter)/`warden_stats` over `journalview.ProjectValues` and windowed folds. |
| `app/webhook/observability.go` | Delivery projection/stats over selected journal; native business handler file removed. |
| `app/webhook/operations.go` | Two typed tenant read operations with compatible raw codecs and shared admission; existing log GET only. |
| `tool_log.go` | `tool_log`, `tool_stats` over `tool.invoked/result`; input/latency joins use `(correlation_id, call_id)` so reused IDs in another run cannot contaminate a row or give a denied call phantom latency (W2.3a). |
| `tool_decoders.go` | `decodeToolInvoked`, `decodeToolResult`. |
| `tool_helpers.go` | `previewString`. |
| `app/runs/plans.go` | Typed `plan_history` (lenient limit and cursor, strict status, newest start first with a correlation tie-break for start-less plans) and `plan_stats` (status counts, success rate over terminal plans, duration distribution) over the routed kernel's `plan.*` lifecycle. |
| `schedule_fires.go` | `schedule_fires` + `latestFiringBySchedule`. |
| `schedule_fires_classify.go` | Classifiers for a firing (executor, category, effect class, uses-LLM, action). |
| `schedule_fires_payload.go` | `scheduleFiredPayload` + `extractScheduleFired`. |
| `schedule_fires_stats.go` | `schedule_stats`. |
| `autonomy.go` | Autonomy feed constants, `autonomyKinds`, `autonomyMeta`, `autonomyDoctorMeta`. |
| `autonomy_doctor.go` | `autonomyDoctorDetail` renderer for doctor events. |
| `autonomy_feed.go` | `handleAutonomyFeed` + `autonomyDetail`. |
| `autonomy_helpers.go` | Payload accessors `strPayload*`, `intPayload`, `clipDetail`. |

**Providers, catalog, routing, budget, config**

| File | What it does |
|---|---|
| `catalog.go` | `catalog_sync` (models.dev / `AGEZT_CATALOG_URL`), `catalog_list`, `catalog_discover` (Ollama tags) + `registerCatalogCommands`. |
| `catalog_provider.go` | `provider_connect` (writes custom.json entry when new), `provider_reload` (rebuild provider in place), `envOrDefault`. |
| `provider_keys.go` | Keyring (M700): `provider_key_list/add/activate/remove`; values never leave the daemon (label + last-4). |
| `provider_oauth.go` | "Sign in with ChatGPT": one-shot listener on 127.0.0.1:1455, token exchange via `chatgptauth`, vault storage, model sync; `provider_oauth_start/status/import/logout`. |
| `routing.go` | `routing_get`/`routing_set`: governor per-task model chains, persisted as `AGEZT_TASK_MODEL_CHAINS`. |
| `chains.go` | `chains_get`/`chains_set`: named fallback chains (`@name`) + usage map. |
| `budget.go` | `budget` (governor snapshot) and `budget_set`. |
| `execution_profiles.go` | `execution_profiles`, `execution_profile_show`, `execution_profile_check` (tenant-allowed). |
| `config.go` | `configEnvVars` canonical list of daemon env vars (presence-only surfacing). |
| `config_handler.go` | Selected runtime config/RoutingReader and bool-only env-presence adapters → app/config.Service.Show; generic app dispatcher owns native framing. |
| `config_helpers.go` | `stringSliceMapToAny`. |
| `app/settings/operations.go` | Five canonical primary specs and strict compatibility input codecs; native registrar removed. |
| `app/settings/{reads,writes,models}.go` | Five typed services/DTOs; native wrappers removed, generic app dispatcher and selected read/write ports remain. |
| `settings_write_ports.go` | Selected server-root field/store/vault/registry/pinned/process-env/kernel-reload adapters. |
| `settings_read_ports.go` | Selected server-root registry/store/vault/pinned/env read adapters and legacy ignored read-load policy. |
| `settings_helpers.go` | `configFieldNeedsKernelReload`, `setLiveEnv` (`os.Setenv`). |
| `app/configcenter/operations.go` | Nine canonical primary typed specs, legacy string validation order and permissive ACL codec; generic app dispatch owns native framing/admission. |
| `app/configcenter/{reads,writes,models,entry}.go` | Selected services/DTO/schema presentation including secret-masked echoes and large integer fields; native handlers/helpers/registrar removed. |
| `configcenter_write_ports.go` | Selected runtime Center Writer binding, with explicit nil availability; classification uses existing GetAutoRating. |
| `configcenter_read_ports.go` | Selected runtime manager binding to app read port, with explicit nil availability. |
| `persona.go` | `persona_get/set`: daemon default system prompt (full text). |
| `prompts.go` | `prompts_get/set`: saved chat prompt library in `chat_prompts.json`. |

**Channels, inbox, ACP**

| File | What it does |
|---|---|
| `channels_read_ports.go` | Selected server-root config/vault/registry/env/pinned snapshot plus global manifest/live port; ignored read-load policy retained. |
| `app/channels/inventory_models.go` | Concrete root/row/account/field/probe/matrix DTOs; nil secret value omitted/public empty string retained. |
| `app/channels/inventory_operations.go` | One primary ReadOnly GET/unknown-input/full nested schema spec; shared native binding owns canceled admission. |
| `app/channels/inventory.go` | Typed channel/account/field presentation, required configuration and secret presence only. |
| `app/channels/probe.go` | Typed account/kind probes with preserved modes/notes; native helper file removed. |
| `app/channels/totals.go` | Typed probe/media integer accumulators; native helper file removed. |
| `channels_gateway_ports.go` | Selected app Gateway factory retains bounded guarded LAN-capable platform/netout.GatewayGET; shared binding supplies native admission. |
| `app/channels/gateway_operations.go` | Two primary ReadOnly POST gateway specs; lenient raw codecs/typed status and QR schemas/canceled preflight. |
| `app/channels/gateway.go` | Status/QR normalize URL/backend/session/key, choose WAHA/Evolution endpoints/headers/bounds and interpret status/image/JSON results behind GatewayGET. |
| `channels_account_ports.go` | Selected server-root registry/section-env/config/vault plus global manifest adapters; ignored vault Set result. |
| `app/channels/account_operations.go` | Two primary POST account writers; ordered raw-field admission/typed output schemas/shared audit gate. |
| `app/channels/accounts.go` | Section-bound validation, trim/suffix/routing/persist/count/error/partial-effect business. |
| `channel_oauth_ports.go` | Stable per-Server sync.Once OAuth memory/service; guarded HTTP and server-root vault factories. |
| `app/channels/oauth_operations.go` | Three primary specs; POST start/status and internal callback; lenient raw codec/shared audit/cancel admission. |
| `app/channels/oauth.go` | Three use-case validation/URL/query/callback context/vault/status business behind OAuthPort. |
| `app/channels/oauth_memory.go` | Provider table and per-host mutex/pending flow/value snapshots/status/start-only >15min prune owner. |
| `app/channels/oauth_exchange.go` | Moved HTTP exchange form/headers/20s timeout/1MiB bound/error precedence and body cleanup behind OAuthHTTPClient. |
| `app/channels/oauth_helpers.go` | Moved32-byte nonce and redirect/instance URL helpers; native forwarding shim absent. |
| `channels_inbox_ports.go` | Selected kernel journal factory remains independent of Server root; shared binding owns admission. |
| `app/channels/inbox_operations.go` | One primary ReadOnly GET spec; raw JSON limit/channel/cursor decoding preserves delayed errors; fully nested output schema. |
| `app/channels/inbox.go` | Unified journal inbox folding/message/thread models/limits/channel filter/descending sort/cursor pagination/typed result behind InboxJournal.Range. |
| `send.go` | Current Server sender factory and shared stringArg helper only; shared native binding owns admission/framing/terminal cleanup. |
| `app/channels/send_operations.go` | One primary POST writer/raw lenient codec/typed result; transfers optional terminal cleanup and preserves panic opacity. |
| `contract/opapi/terminal.go` | stdlib-only contextual cleanup ownership port; rejection keeps cleanup with caller. |
| `app_terminal.go` | Native LIFO/idempotent terminal release after success/error write, failure and panic; callbacks run outside lock. |
| `app/channels/send.go` | Outbound normalization/validation/Sender/background30s context/error/result; terminal callback preserves context until response delivery completes. |
| `app/channels/acp_operations.go` | One typed GET/primary-only/ReadOnly/ordinary input schema and full inventory output spec; shared native binding owns cancel admission/exact integer terminal. |
| `app/channels/acp.go` | Active-command environment/trim, selected default cached discovery and caller context/full typed inventory ownership. |

**Cognition surfaces**

| File | What it does |
|---|---|
| `planner.go` | `plan_generate`, `plan_refine` (LLM plan JSON; LIVE). |
| `conductor.go` | `conductor_roles`, `conductor_ask` (Thinker→Worker→Verifier). |
| `council.go` | `council_members`, `council_ask`, `council_set` (multi-model panel). |
| `research.go` | `research_ask` (deep-research harness; LIVE). |
| `chatsuggestions.go` | `chat_suggestions`: memory/tool-context derived next-prompt chips (no LLM). |
| `chatsummary.go` | `chat_summarize`: LLM briefing of chat history (LIVE). |
| `reflect.go` | `reflect_run`, `reflect_show`. |
| `nodes.go` | `node_registry`: probes `AGEZT_PEERS` mesh nodes. |
| `seat.go` | `seat_list/create/delete` (workboard execution seats). |
| `taste.go` | `taste_list/create/delete` (taste exemplars). |

**Agent roster** (`agent_*` ops) — all 17 commands are typed `app/roster` operations; the native files below are their ports and status collection.

| File | What it does |
|---|---|
| `app/roster/*.go` | Request codecs, typed outputs, specs and services for every roster operation (list, graveyard, activity + activity summary, repair status, escalations, set enabled, add/edit, task update, wake, repair, resolve, impact/tombstone, retire/revive, remove), plus status presentation and typed snapshot rows. |
| `roster.go` | Native type aliases over the app status rows. |
| `roster_list.go` | List service binding, `profileView`, `agentModelChain`, list cache invalidation. |
| `roster_crud_internal.go` | Hierarchy reference validation and the managed sub-agent direct-call guard. |
| `roster_lifecycle_helpers.go` | Sub-agent tree, workflow references and mailbox impact labels. |
| `roster_cascade.go` | `nativeImpactSource` (per-agent holdings for impact/tombstone/retire/remove) and per-subsystem impact helpers. |
| `roster_teardown.go` | Retire sub-agents; remove, pause and count standing orders and schedules for an agent. |
| `roster_teardown_state.go` | Forget agent memory, archive skills, prune config-center entries/access refs, delete workspace. |
| `roster_status.go` | `agentStatusAccums` + single-pass journal fold for live status. |
| `roster_status_views.go` | Selected reaper/repair/escalation/wake/journal snapshot collection; app StatusService window/render adapter. |
| `roster_status_wake.go` | Wake-state views, schedule/agent matching helpers. |
| `roster_status_escalation.go` | Escalation load + routing-match helpers. |
| `roster_escalation.go` | Autonomy runbook payload, `publishOperatorAction`, the native `runAgentWake` launch. |
| `roster_wake_helpers.go` | Operator force-generation, exhausted-chain lookup and incident lineage matching. |
| `roster_resolve.go` | Resolve ports: operator help delegation and routing-chain application (`overseertool.NewKernelSource` → `ApplyRoutingChain`). |
| `roster_repair.go` | The governed `runAgentRepair` launch with its panic firewall. |
| `roster_repair_summary.go` | Repair summaries/cooldown/history. |
| `roster_workspace.go` | Workspace root (`AGEZT_WORKSPACE`) and file counts. |
| `roster_helpers.go` | Payload accessors `plString/plInt/...`, `truncate`, `firstNonEmpty`. |
| `roster_activity_text.go` | Header-only file left by a split (no code). |
| `tool.go` | `tool_list` (+ catalog probe, rollback mode). |
| `tool_agents.go` | `agent_permissions`, `agent_capabilities` (patch trust ceiling/tool allow-deny/noise/config overrides/memory scope/workdir/cost). |
| `tool_views.go` | `decodeControlplaneArg`, `applyAgentCapabilityPatch`, wake-access view. |
| `tool_views_governance.go` | `agentGovernanceView`, permission rows, noise policy. |

**Memory / world / skills / standing / schedules**

| File | What it does |
|---|---|
| `kernel/app/memory/read.go`, `list.go` | Typed get/search/related/active snapshot/paging and record projection. |
| `kernel/app/memory/write.go`, `hygiene.go` | Typed curation, pruning, tidy, audit and clean business. |
| `kernel/app/memory/distillation.go`, `log.go` | Runtime distillation port/orchestration and tenant-selected journal lifecycle fold. |
| `kernel/app/memory/operations.go` | Sixteen typed specs, legacy-compatible admission and HTTP hints; common CP adapter owns transport/auth/tenant/audit. |
| `kernel/app/world/world.go` | Typed graph curation/query/projection; the former socket business lives here. |
| `kernel/app/world/log.go`, `operations.go` | Tenant-selected history fold, nine typed specs/admission and existing HTTP hints; the common CP adapter owns transport/auth/routing/audit. |
| `skill.go` | `skill_list/get/history/promote/quarantine/archive/revert/restore/share/reassign/import`. |
| `skill_files.go` | `skill_files`, `skill_read_file` (path-confined), `skill_hygiene`. |
| `skill_view.go` | `skillView`, `registerSkillCommands`. |
| `standing.go` | `standingView`, frequency warning. |
| `standing_handlers.go` | `standing_list/add/edit/set_enabled/remove`. |
| `standing_helpers.go` | `validateStandingAgent`. |
| `standing_fire.go` | `SetStandingFire`, `standing_fire`, `registerStandingCommands`. |
| `standing_why.go` | `standing_why` (life story of one order). |
| `schedule.go` | `schedule_enable`, `schedule_test` (next fire forecast), `registerScheduleCommands`. |
| `schedule_add.go` | `schedule_add` (interval/at/days/tz/once/window/continuous; agent/workflow/system-task/tool targets). |
| `schedule_edit.go` | `schedule_edit`. |
| `schedule_helpers.go` | `schedule_list`, entry views, arg/cadence validation, execution metadata. |
| `schedule_misc.go` | `schedule_system_tasks`, `schedule_rm`, `schedule_run`, runnable validation. |

**Pulse**

| File | What it does |
|---|---|
| `pulse.go` | `pulse_subscribe`: live bus subscription (pattern, kinds, correlation, since/until seq/ts replay, replay_rate). |
| `pulse_replay.go` | `replayHistorical`: journal replay before going live. |
| `pulse_control.go` | `SetPulse`, `pulse_status/pause/resume/beat/asks/ask_resolve/cadence`. |
| `pulse_control_helpers.go` | `PulseController`/`PulseObservers` interfaces, `persistPulseSetting`, `pulse_watch/probe/unwatch/dial/quiet/flush`. |

**Workflows, workboard, board, OKR**

| File | What it does |
|---|---|
| `workflow.go` | Workflow view + run-arc summarization helpers. |
| `workflow_register.go` | `registerWorkflowCommands`. |
| `workflow_handlers.go` | `workflow_list/show/save/restore/remove/set_enabled`. |
| `workflow_handlers_run.go` | `workflow_templates`, `workflow_draft`, `workflow_refine` (copilot, never saves), `workflow_run`. |
| `workflow_handlers_runs.go` | `workflow_runs` history. |
| `workflow_handlers_invoke.go` | `workflow_test_node`, `workflow_webhook` (constant-time secret check), `runWorkflowDetached`. |
| `workboard.go` | Task views + `workboard_list`. |
| `workboard_handlers.go` | `workboard_lanes/show/create/claim/heartbeat/comment/block/fail/unblock/complete/prove/seat/archive`. |
| `workboard_handlers_link.go` | `workboard_link/policy/depend/reclaim/sweep`. |
| `workboard_handlers_dispatch.go` | `workboard_dispatch`, `workboard_watch`. |
| `workboard_dispatch.go` | `runWorkboardDispatch`, `applyWardenExecutionProfile`. |
| `workboard_dispatch_args.go` | Retry policy args, correlation id, slice/int args. |
| `workboard_dispatch_helpers.go` | Dispatch intent builder, publish, latest run id, watch events. |
| `workboard_dispatch_register.go` | `registerWorkboardCommands`. |
| `workboard_dispatch_resp.go` | `workboardWriteResp`. |
| `board.go` | Board reader/writer helpers (shared `boardStore`, read-only fallback). |
| `board_handlers.go` | `board_read/help/send/inbox/ack/get/replies` + register. |
| `okr.go` | `okr_list/show/create/keyresult/link/unlink/archive`. |

**Marketplace, MCP, toolforge, toolbox, artifacts, plugins, datalake, edict**

| File | What it does |
|---|---|
| `app/market/write_operations.go` | Five writer specs/codecs/typed progress; native market.go removed. parseCapList is in args.go; unused strArg/mustJSONRaw removed. |
| `app/tools/mcp_lifecycle_operations.go` | Five typed lifecycle specs/codecs; native mcp.go wrappers removed. |
| `app/tools/forge_catalog.go`, `forge_lifecycle.go`, `forge_lifecycle_operations.go` | Eight typed native toolforge operations: two unaudited reads and six mandatory audited lifecycle writes (promotion is operator-only). Kernel/store owns durable lifecycle transitions. |
| `toolbox.go` | Host discovery/installer adapters and shared market/ACP JSON helpers; all three native toolbox operations are app-owned. |
| `artifact.go` | `artifact_get` (re-verified bytes), `artifact_list`, `artifact_delete`, `artifact_collect`. |
| `plugin.go` | Selected runtime-manifest Reader adapter → app/plugins.Service; codec/socket framing uses generic app dispatcher. |
| `datalake.go` | `data_collections/records/insert/update/delete/create_collection/drop_collection`. |
| `app/edict/reads.go` | Typed `edict_show`, `edict_deny_list` (removable runtime rules) and `edict_test` (dry-run decision) over the routed kernel's policy engine; strict tenant, capability checked first for the probe. |
| `app/edict/writes.go` | Typed audited `edict_deny_add` (exactly one rule), `edict_deny_rm` (runtime rules only, journals actual removals), `edict_set_level` (known capability) and `edict_set_mode` over the routed kernel's engine; each change journals `policy.changed` on the routed bus. |
| `app/edict/overlay.go` | Typed audited `edict_overlay` (net runtime policy folded as at boot) and primary-only `edict_compact` (boot snapshot through the head + `policy.compacted` content hash) over the routed kernel's journal, snapshot file and bus. |

Tests (119 files): `dispatch_registry_test.go` (registry ↔ constants 1:1, TenantAllowed⇒TenantRouted), `tenant_auth_test.go`
(tenant sweep), `args_ratchet_test.go` (per-file baseline of raw `req.Args[...].(T)` casts; may only go down),
`config_test.go`/`config_inventory_test.go` (env-var inventory guard), `request_limit_test.go`/`request_oversize_test.go`,
`recover_test.go` (panic firewall), `fuzz_test.go`, `sandbox_escape_test.go`, `vision_gate_test.go`, plus per-domain `*_test.go` and
`*_heavycov_test.go` coverage files.

---

## 2. `kernel/auth`

Purpose: transport-independent credential verification + authority tiers. Imports `internal/atomicfile`; imported by `cmd/agezt`,
`httpserver`, `openaiapi`, `restapi`, `webui`.

| File | What it does |
|---|---|
| `tier.go` | `Tier` = `TierUnset` (zero, invalid) < `TierPublic` < `TierUser` < `TierAdmin`; `Valid()`, `String()`. `TierUnset` exists so a `RouteOpts` that forgets `Tier:` fails the router's validity panic instead of silently becoming public. |
| `token.go` | `Verifier` interface; `StaticVerifier` (one admin token + optional user tokens; constant-time compare; scans all user tokens to avoid position leaks; blank tokens ignored). |
| `tokenfile.go` | `WriteTokenFile` (atomic, 0600, dir 0700, single-segment filename only), `ReadTokenFile` (missing = `("", nil)` for read-or-mint), `TokenPrefix` (`abcd…wxyz`, empty for ≤8 chars). Used for `rest.token`, `openai.token`. |

## 3. `kernel/streamlimit`

`streamlimit.go`: `Limiter{max, mu, active map[string]int}`, `New(max)` (≤0 = unlimited), `Acquire(key) (release func(), ok bool)`
— idempotent release (sync.Once), deletes idle keys, nil limiter always allows. Imported only by `httpserver`.

## 4. `kernel/httpserver`

Purpose: shared HTTP transport mechanics; owns no product routes. Imports `auth`, `streamlimit`; imported by `cmd/agezt`, `agentgw`,
`openaiapi`, `restapi`, `webui`.

| File | What it does |
|---|---|
| `doc.go` | Package scope statement. |
| `auth.go` | `Authenticator{Verifier, TenantAuthorize, TenantHeader (default X-Agezt-Tenant), RequestAuthorize}`; `BearerToken` (exact `Bearer ` prefix, no query tokens); `Authorized` (public → true; `RequestAuthorize` overrides; else Bearer via Verifier, else for `TierUser` only, tenant header + `TenantAuthorize`); `Middleware`. |
| `limits.go` | `BodyLimit(max)` → `http.MaxBytesReader`; panics on non-positive. |
| `listener.go` | `NewStreamingServer` (ReadHeaderTimeout 10 s, IdleTimeout 120 s, **no WriteTimeout** so SSE survives), `Start(ctx, ln, handler, onError)` (graceful 3 s shutdown on ctx cancel). |
| `router.go` | `RouteOpts{Tier, Method, BodyMax, Timeout, Mutation, Unauthorized}`, `Route`, `Router` (`NewRouter`, `Handle`, `Routes`, `ServeHTTP`). `Handle` panics on invalid tier/negative limits/bad method; wraps handler as auth → body limit → method limit (405 + `Allow`). `Timeout` and `Mutation` are metadata only. |
| `sse.go` | Process-wide `sseLimiter` (64 concurrent SSE streams per client IP, V-009/LD-8); `AcquireSSE` (429 + Retry-After 5), `StartSSE` → `SSEStream` with `WriteJSON`, `WriteEvent`, `WriteData`, `Comment`, `Close`. |

Tests: `httpserver_test.go`, `router_method_test.go` (method enforcement), `sse_test.go`.

## 5. `kernel/webui` (Go side)

### 5.1 Purpose and structure

Serves the go:embed-ded Vite bundle (`embed.go`: `//go:embed all:dist`, built by `make frontend-build` from `frontend/` and committed)
and bridges the SPA to the daemon. Stateless apart from in-memory sessions, hook rate buckets and the SSE token. Three data paths:
SPA assets; `/events` SSE firehose straight from `*bus.Bus` (`Subscribe(">", 256)`); every `/api/*` panel proxied through a
`Caller` (`*controlplane.Client`) — so the Web UI and `agt` share the exact same query logic.

Imports: `internal/atomicfile`, `internal/paths`, `auth`, `bus`, `controlplane`, `convo`, `event`, `httpserver`. Imported only by `cmd/agezt`
(`httpsurfaces.go buildWebUI`).

Key types: `Caller` (`Call`, `Stream`), `Transcriber`, `Synthesizer`, `Server{bus, client, token, sseToken, tokenAuth, sseAuth, dist,
transcriber, synthesizer, passwordFn, passwordStrict, sessions, allowedHosts, hostPolicyMu, allowQueryTokensForData, hookRL*}`.
Constructor `New(bus, client, token)` mints the 32-byte SSE token. Setters: `SetTranscriber`, `SetSynthesizer`, `SetAllowedHosts`
(any non-loopback host auto-raises password-strict), `SetPasswordFn` (LIVE password source), `SetPasswordStrict`, `PasswordStrict`.
`Handler()` = `secure(routeRegistry().ServeHTTP)`.

Persistence: File Manager mutations are delegated to primary-only ops using the configured workspace (`AGEZT_FILE_ROOT`, default `~/agezt/workspace`, dirs 0700). The WebUI still reads/rewrites the rollback
catalog `<paths.BaseDir()>/rollback/checkpoints.json` (read; `apply` rewrites it and may restore file snapshots). Env:
`AGEZT_FILE_ROOT`, `AGEZT_FILE_ROOT_MAX_BYTES` (4 MiB), `AGEZT_FILE_ROOT_MAX_ENTRIES` (500); password/strict/addr env are read by
`cmd/agezt` (`AGEZT_WEB_ADDR`, `AGEZT_WEB_PASSWORD`, `AGEZT_WEB_PASSWORD_STRICT`).

| File | What it does |
|---|---|
| `doc.go` | Package doc: embedded SPA, three data paths, security posture. |
| `embed.go` | `distFS` (`//go:embed all:dist`). |
| `webui.go` | `Caller`/`Transcriber`/`Synthesizer`, `Server`, `SetTranscriber`, `SetSynthesizer`, `SetAllowedHosts`, `New`. |
| `webui_routes.go` | `routeRegistry()`: the single route table (see 5.2) built on `httpserver.Router` with a `RequestAuthorize` hook → `s.authorized`; `Handler()`. |
| `webui_read_routes.go` | `apiRoutes` (36 parameterless GET → cmd) and `readArgsRoutes` (41 GET with allowlisted query args); `writeRoute{cmd,args}` type. |
| `webui_write_routes.go` | `writeRoutes` (64 POST, args from query string), `jsonRoutes` (57 POST, args from JSON body allowlist), `planRoute`, `jsonBodyMax`=1 MiB, `planRunTimeout`=30 min. |
| `webui_proxy.go` | `handleEvents` (SSE firehose), `proxy`, `readArgsProxy`, `writeProxy`, `decodeAllowedBody`, `jsonProxy`, `planRunProxy`, `numericQueryArgs` coercion, `writeJSON` (`Cache-Control: no-store`). |
| `webui_security.go` | `allowHook` (fixed-window 60/min + 30 burst per workflow|IP, 4096 buckets), `handleWorkflowHook`, `runStreamProxy` (chat → `run` SSE, folds `history` with `convo.TranscriptIntent`), `toolInstallProxy`, `marketStreamProxy`. |
| `webui_security_auth.go` | `secure` (security headers + Host allowlist + same-origin mutation check), `shellAuth`, `setSecurityHeaders` (CSP etc.), `hostAllowed`, `sameOriginMutation`, `tokenPresented`, `dataTokenPresented`, `authorized`, `tokenMatch`, `sseTokenMatch`. |
| `webui_security_helpers.go` | `toStr`, `stringList`, `historyTurns`. |
| `webui_assets.go` | `handleSPA` (index.html, no-cache), `handleAssets` (immutable cache, explicit content types — avoids Windows registry mime bug), `handleFavicon`, `contentType`. |
| `webui_oauth.go` | `handleOAuthCallback` → `channel_oauth_callback`; self-closing result page; `htmlEscape`; hook constants + `hookBucket`. |
| `session.go` | Console password + session: `SetPasswordFn`, `SetPasswordStrict`, `consolePassword`, `sessionValid`, `handleSSEToken`, `handleAuthMeta`, `handleLogin`, `handleLogout`, `cookieSecure`. Cookie `agezt_web_session`, TTL 12 h sliding, HttpOnly, SameSite=Strict. |
| `session_store.go` | In-memory `sessionStore` (id→expiry, sliding TTL) + GLOBAL failed-login counter: 8 failures → 5 min lockout. |
| `streamcap.go` | `streamClientKey` (RemoteAddr host) for the hook limiter; SSE cap moved to `httpserver`. |
| `artifact_route.go` | `handleArtifactRaw`: `artifact_get` bytes with allowlisted Content-Type, sanitized filename. |
| `files_route.go` | File Manager types and limits; path safety lives in `platform/fileworkspace`. Reads use it directly; mutations proxy primary-only ops through `files_mutation.go`. |
| `files_route_handlers.go` | `handleFileTree`, `handleFileRaw`, `handleFileMkdir`, `handleFileRename`, `handleFileDelete` — direct OS calls. |
| `files_route_helpers.go` | `typeOf`, `readJSONBody`. |
| `rollback.go` | `handleRollbackCheckpoints`, `handleRollbackApply`; catalog/checkpoint types (kinds `skill.status`, `workflow.snapshot`, `file.snapshot`, `config.setting`). |
| `rollback_helpers.go` | Apply a checkpoint (→ `skill_restore` / `workflow_restore` / `config_set` control-plane calls, or direct file restore), catalog load/write. |
| `transcribe.go` | `handleTranscribe` (multipart `file`, 25 MiB) → `Transcriber`; 501 when unconfigured. |
| `tts.go` | `handleTTS` (`{"text"}` → audio stream) → `Synthesizer`. |
| `voice_status.go` | `handleVoiceStatus`: booleans for STT/TTS availability only. |

Tests: `webui_test.go` (77 tests), `*_route_test.go` per route family (forwarding + GET rejection), `files_route_test.go`
(auth + traversal), `hookrate_test.go`, `session_test.go`, `transcribe_test.go`, `tts_test.go`, `voice_status_test.go`.

### 5.2 Web UI route table (deliverable c, part 1)

All routes pass through `secure()` first (headers, Host allowlist, same-origin mutation). "user" = `TierUser` via
`s.authorized(r)` (token OR session by default; token AND session in strict mode; token only when no password). Timeouts are the
proxy's own `context.WithTimeout`.

| Method | Path | Auth | Purpose / op |
|---|---|---|---|
| GET | `/` (and any unmatched path) | public* | SPA shell (`shellAuth`: needs `?token=`/Bearer unless a console password is configured; carries no data) |
| GET | `/assets/…`, `/favicon.ico` | public | Embedded bundle |
| GET | `/api/authmeta` | public | `{password_required, authed}` |
| POST | `/api/login` (4 KiB body), `/api/logout` | public | Password login → session cookie; logout revokes |
| GET | `/events` | user (`?st=` SSE token, Bearer, or legacy `?token=`) | SSE firehose of every bus event (`>`), 20 s ping |
| GET,OPTIONS | `/api/sse-token` | user | Returns the ephemeral SSE token |
| GET | 36 `apiRoutes` (`/api/status`, `/api/config`, `/api/budget`, …) | user | `proxy(cmd)`, no args, 5 s |
| GET | 41 `readArgsRoutes` (`/api/runs`, `/api/agents`, `/api/journal`, …) | user | `readArgsProxy`, allowlisted query args (numeric coercion), 5 s |
| POST | 64 `writeRoutes` (`/api/halt`, `/api/decide`, `/api/edict/set_level`, …) | user | `writeProxy`, args from query string, 5 s |
| POST | 57 `jsonRoutes` (`/api/config/set`, `/api/agents/add`, `/api/schedule/add`, …) | user | `jsonProxy`, JSON body allowlist, 1 MiB, 120 s |
| POST | `/api/plan/run` | user | `plan` via `Client.Stream` (events discarded; browser sees them on `/events`), 30 min |
| POST | `/api/run` | user | Chat: `run` via `Client.Stream`, each event re-emitted as SSE `{kind,subject,payload,correlation_id}`, then `{kind:"done"}`; body keys `intent, model, history, system, agent, execution_profile, auto_approve_caps` |
| POST | `/api/toolbox/install` | user | `toolbox_install` streamed as SSE |
| POST | `/api/market/install`, `/api/market/uninstall` | user | `market_install` / `market_uninstall` streamed as SSE |
| GET | `/api/rollback/checkpoints` | user | Rollback catalog listing |
| POST | `/api/rollback/apply` | user | Apply checkpoint (control-plane restore or direct file write) |
| GET | `/api/voice/status` | user | STT/TTS availability |
| POST | `/api/transcribe` (25 MiB) | user | STT |
| POST | `/api/tts` | user | TTS audio |
| GET | `/api/artifact/raw` | user | `artifact_get` raw bytes, 15 s |
| GET | `/api/files/tree`, `/api/files/raw` | user | File Manager reads under `AGEZT_FILE_ROOT` |
| POST | `/api/files/mkdir`, `/api/files/rename`, `/api/files/delete` | user | Primary-only audited file ops through app/files and the kernel invoker (BodyMax 256 bytes) |
| POST | `/hooks/{workflow}` | public (per-workflow secret `X-Agezt-Secret` or `?secret=`) | Rate-limited (60/min+30 burst per workflow|IP), 256 KiB body, → `workflow_webhook`; 202 async or 200 reply-mode; uniform 403 on auth failure |
| GET | `/oauth/callback` | public (state token) | Channel OAuth redirect → `channel_oauth_callback` |

The per-op mapping of the 198 table-driven routes is the "Web UI route(s)" column of §1.10.

### 5.3 Event streaming path to the browser (deliverable e)

```
kernel (agent loop, governor, tools…) ─ journal.Append ─► bus.Publish(event)   (events are redacted before journaling, see 06-data-memory-state.md)
                                                             │
     webui.handleEvents: bus.Subscribe(">", 256) ◄───────────┘   (subscribe BEFORE StartSSE so failure = plain 500)
         httpserver.StartSSE → per-IP slot (64) → headers text/event-stream, no-cache, keep-alive
         ": connected" comment (fires EventSource.onopen), ": ping" every 20 s
         each event: "data: <json(event.Event)>\n\n" + Flush       (bus drops to slow subscribers; no backpressure to publishers)
                                                             │
 SPA: api.ts readAndScrubToken() strips ?token= from the URL → Bearer header for fetch;
      fetch /api/sse-token (Bearer) → EventSource("/events?st=<sseToken>")   (frontend/src/app/events/events.tsx EventsProvider)
```

Secondary streams: `/api/run` (per-run events relayed from the control-plane `run` stream — this is the chat token stream),
`/api/toolbox/install`, `/api/market/*` (progress events). `/api/plan/run` intentionally does NOT relay (the firehose already carries
`plan.*`/`node.*`). The firehose is the PRIMARY kernel's bus only (no tenant scoping) — the Web UI is a primary-operator console.

---

## 6. `kernel/restapi`

Purpose: stable `/api/v1` REST surface for SDKs and automation + Prometheus `/metrics` + k8s-style probes. Talks to an `Engine`
(`NewCorrelation`, `SubjectForRun`, `RunModel`, `DefaultModel`, `ModelIDs`, `EventsForCorrelation`; optional `ArtifactLister`,
`ArtifactReader`) implemented by `cmd/agezt`'s `kernelAPIEngine` — it does not use the control plane. Imports `internal/brand`, `auth`,
`board`, `bus`, `event`, `httpserver`, `meshctx`, `update`. Imported by `cmd/agezt` (`buildRESTAPI`, enabled by `AGEZT_REST_ADDR`; token
persisted to `<home>/rest.token`). Concurrency: one goroutine per request (net/http); run streams subscribe to the run subject before
starting. Events: publishes `mesh.loop` (`KindMeshLoopRefused`) when `X-Agezt-Hop` exceeds `meshctx.MaxHopsFromEnv()`; mailbox posts
publish `board.posted` via the injected notifier. Env: `AGEZT_REMOTE_ARTIFACT_BYTES=allow` gates artifact byte transfer; mesh hop env via `meshctx`.

| File | What it does |
|---|---|
| `doc.go` | Truncated package doc (only the update routes + security line survive). |
| `restapi.go` | `Engine`, `TenantResolver`, `TenantAuthorizer`, `Server`, `Metric`, `New`, setters (`SetTenantResolver/Authorizer/Readiness/Metrics/UpdateService`), `bind` (X-Agezt-Tenant → tenant engine+bus), `maxRequestBodyBytes` 16 MiB. |
| `restapi_routes.go` | `Handler()` route table + `handleLive`, `handleReady`, `handleMetrics` (Prometheus text), `handleHealth`, `handleModels`. |
| `restapi_routes_runs.go` | `runRequest{intent, model, stream}`, `handleRunsRoot` (mesh hop guard, sync or SSE), `streamRun` (SSE `token`/`done`/`error` events), `handleRunByID` (events for a correlation); `promName` sanitizer. |
| `restapi_prom.go` | `promName`, `promHelp`, `methodNotAllowed`, `writeJSON`, `writeErr`, `tokenText`. |
| `artifacts.go` | `handleArtifacts` (metadata list), `handleArtifactBytes` (fail-closed unless `AGEZT_REMOTE_ARTIFACT_BYTES` allows). |
| `mailbox.go` | `SetMailbox(store, notify)`, limits, view helpers. |
| `mailbox_handlers.go` | `handleMailboxMessages` (GET list / POST send), `handleMailboxInbox`, `handleMailboxMessageSub` (`/{id}/replies`, `/{id}/ack`). |
| `mailbox_watch.go` | `handleMailboxWatch` (SSE `mail` frames from `board.posted`, keep-alive), `handleMailboxTopics`. |
| `update_handlers.go` | `handleUpdateCheck`, `handleUpdateApply` (admin-only). |

Route table (deliverable c, part 2) — `admin` = rest.token only; `user` = rest.token, or a tenant token + `X-Agezt-Tenant`:

| Method | Path | Auth | Does | Kernel path |
|---|---|---|---|---|
| GET,HEAD | `/healthz` | public | liveness `{status:ok}` | — |
| GET,HEAD | `/readyz` | public | 503 when draining or halted | readiness closure |
| GET,HEAD | `/metrics` | user | Prometheus gauges/counters (`agezt_*`) | `restMetrics(k)` (primary kernel) |
| GET | `/api/v1/health` | user | version, default model, model count | Engine (primary, not tenant-bound) |
| GET | `/api/v1/models` | user | model ids | Engine (primary) |
| POST | `/api/v1/runs` | user | run an intent; `stream:true` or `Accept: text/event-stream` → SSE | `Engine.RunModel` on bound tenant |
| GET | `/api/v1/runs/{corr}` | user | events of a run | `Engine.EventsForCorrelation` |
| GET | `/api/v1/artifacts` | user | artifact metadata | `ArtifactLister` |
| GET | `/api/v1/artifacts/{id}` | user | artifact bytes (policy-gated) | `ArtifactReader` |
| GET,POST | `/api/v1/mailbox/messages` | admin | list / send | shared `board.Store` |
| GET,POST | `/api/v1/mailbox/messages/{id}/replies`, `/{id}/ack` | admin | replies / ack | board |
| GET | `/api/v1/mailbox/inbox` | admin | inbox for a name | board |
| GET | `/api/v1/mailbox/watch` | admin | SSE of new mail | bus `board.posted` |
| GET | `/api/v1/mailbox/topics` | admin | topics | board |
| GET | `/api/v1/update` | admin | update check | `update.Service` |
| POST | `/api/v1/update/apply` (64 KiB) | admin | validate + stage update | `update.Service` |

## 7. `kernel/openaiapi`

Purpose: OpenAI-compatible surface; every request is a full governed kernel run (`Engine.RunModel` → Edict, journal, budget) — not a
raw completion proxy. `messages[]` are collapsed into ONE intent (`intentFromMessages`); the `model` field routes per request (falls back to
`DefaultModel`); `response_format` json → jsonMode (M314); images from `image_url` parts. Streaming subscribes to `SubjectForRun(corr)`
(buffer 1024) and maps `llm.token` / `llm.reasoning` events to `chat.completion.chunk` / Responses SSE, terminated by `data: [DONE]`.
Usage from `UsageReporter` (real tokens) or `estimateUsage`. Errors pass through `redact.New()` (`redactErr`). Imports `auth`, `bus`,
`convo`, `event`, `httpserver`, `redact`, `ulid`; imported by `cmd/agezt` (`buildOpenAIAPI`, `AGEZT_API_ADDR`, `<home>/openai.token`).

| File | What it does |
|---|---|
| `doc.go` | Package doc (governance parity, lossy mapping, security). |
| `openaiapi.go` | `Engine`, `UsageReporter`, `TenantResolver/Authorizer`, `Server`, `Transcriber`, `redactErr`, `decodeBody` (413 on MaxBytesError), `chatUsage`, 16 MiB body cap. |
| `openaiapi_server.go` | `New`, setters, `bind`, `Handler()` route table, `audioMaxBytes` 25 MiB. |
| `openaiapi_chat.go` | `chatRequest`, `chatRespFormat`, `streamOptions`, `chatMessage` (string or parts), image extraction. |
| `openaiapi_stream.go` | `handleChat`: non-stream → `runCapturingReasoning`; stream → `streamChat`. |
| `openaiapi_run.go` | `runCapturingReasoning` (captures `llm.reasoning` into `reasoning_content`), `streamChat` (SSE chunks + optional usage chunk + `[DONE]`). |
| `openaiapi_handlers.go` | `handleTranscription`, `handleModels`, `handleModelByID`, `modelRoutable`. |
| `openaiapi_helpers.go` | `intentFromMessages`, `estimateUsage`, `writeJSON`, `writeErr` (OpenAI error envelope), `tokenText`, `reasoningText`. |
| `responses.go` | `responsesRequest`, `handleResponses` (`/v1/responses`). |
| `responses_helpers.go` | Input → intent/messages/images, response object + usage builders, `jsonString`. |
| `responses_stream.go` | `streamResponses` (Responses API SSE event sequence). |

| Method | Path | Auth | Does |
|---|---|---|---|
| POST | `/v1/chat/completions` | user (openai.token or tenant token + X-Agezt-Tenant) | Governed run; JSON or SSE chunks |
| POST | `/v1/responses` | user | Responses API shape over the same run |
| GET | `/v1/models` | user | Default + catalog models |
| GET | `/v1/models/{id}` | user | Single model if routable |
| POST | `/v1/audio/transcriptions` (25 MiB) | user | STT via `AGEZT_STT_*` client |

## 8. `kernel/agentgw`

Purpose: HTTP gateway for agent-subprocess code (Agent SDK) with HS256-JWT capability tokens. Built in `kernel/runtime/compose.go`
(`DefaultGatewayConfig(baseDir)`, `ResolveTokenSecret`, `NewGateway`, `Attach(bus, memory, roster)`, `SetConfigCenter`,
`SetAuditJournal(journal)`, `go Listen(context.Background())`) and exposed via `Kernel.AgentGateway()`. Imports `bus`, `configcenter`,
`edict`, `event`, `httpserver`, `journal`, `memory`, `roster`, `ulid`. Imported by `cmd/agt` (`agt token` mints tokens with the same
secret), `kernel/runtime`, `runtime/accessors`, `runtime/runexec`.

Persistence: `<baseDir>/agentgw.secret` (hex 32 bytes, 0600) unless `AGEZT_AGENTGW_TOKEN_SECRET`; audit entries appended to the journal
as subject `agentgw.audit`, kind `info` (batched; `Flush` on Close). Env: `AGEZT_AGENTGW_TOKEN_SECRET`, `AGEZT_AGENTGW_SOCKET` (read in
`runtime/compose.go`). Concurrency: net/http server; `rlMu` guards per-token `RateLimit` map (4096 entries, idle eviction 5 min);
`srvMu` guards the server.

Token (`token.go`): header `{"alg":"HS256","typ":"JWT"}`, claims `TokenClaims` (`RunID` required, `Caps` ≥1, `ExpiresAt` default 1 h,
`MaxRate` default 60 rpm, `MaxBurst` 10, `Issuer`=`Audience`=`agezt-agentgw`, `TokenID` ULID, `SubprocessID`, `ParentTokenID`);
`CreateSubprocessToken` derives narrower child tokens. `/v1/token/create` refuses capability escalation (`CapsSubset` → 403
`CAP_ESCALATION`). Capabilities (`types.go`): `eventbus.publish|subscribe`, `channel.send|read|list`, `memory.read|write|delete|search|list`,
`log.read|write`, `agent.list|query`, `db.query|read|write`, `config.access|list|search|write`.

| File | What it does |
|---|---|
| `doc.go` | Package doc + capability namespaces. |
| `types.go` | `TokenClaims`, `AgentCapability` consts, `AuditEntry`, `RateLimit` (`NewRateLimit`, `Allow`, `LastSeen`), `HasCap/HasAnyCap`. |
| `token.go` | `TokenManager` (secret < 32 bytes is SHA-256 stretched), `CreateToken`, `ValidateToken` (alg/typ/iss/aud/exp checks, constant-time sig), `CreateSubprocessToken`. |
| `secret.go` | `TokenSecretEnv`, `ResolveTokenSecret` (env → file → mint+persist, retrying read for concurrent first boot), `randomSecret`. |
| `capabilities.go` | `CapabilityChecker` (`Check/CheckAny/CheckAll`), `ParseCapability`, `CapsSubset`, `CapsIntersect`, `NormalizeCaps`. |
| `gateway.go` | `Gateway`, `GatewayConfig`, `DefaultGatewayConfig` (random abstract socket `@agezt/agentgw-<8hex>.sock`, 30 s read/write timeouts), `NewGateway`, `Attach`, `SetConfigCenter`, `SetAuditJournal`, `listenTarget` (`@`/`unix://`/absolute path → unix, else TCP), `Listen` (route table), `Close`. |
| `gateway_auth.go` | `withAuth` (Bearer → validate → per-token rate limit → audit → claims in ctx), `allowRate`, `extractBearerToken`, response helpers, `handleHealth`, `handleTokenCreate`. |
| `handlers.go` | `handleEventbusSubscribe` (SSE, any `pattern`, default `>`), `handleEventbusPublish` (any subject, kind `info`, actor = RunID). |
| `handlers_log.go` | `handleLogWrite`, `handleLogRead`, `handleAgentList`, `handleAgentQuery`. |
| `handlers_memory.go` | `handleMemoryWrite`, `handleMemorySearch`, `handleMemoryDelete`. |
| `config_handler.go` | `ConfigHandler`: config-center get/list/search/set/audit with rating + agent allow/deny. |
| `audit.go` | `AuditLogger` (`Log`, `LogSync`, `Flush`) → journal. |
| `sockopt_unix.go` / `sockopt_other.go` | `setSockOpt` listen control hook (unix-specific options / no-op). |

| Method | Path | Capability |
|---|---|---|
| GET | `/health` | none |
| GET | `/v1/eventbus/subscribe?pattern=` | `eventbus.subscribe` (SSE) |
| POST | `/v1/eventbus/publish` | `eventbus.publish` |
| POST | `/v1/memory/write` · DELETE `/v1/memory/delete` · GET `/v1/memory/search` | `memory.*` |
| GET | `/v1/log/read` · POST `/v1/log/write` | `log.*` |
| GET | `/v1/agent/list` · `/v1/agent/query` | `agent.*` |
| POST | `/v1/token/create` | subset of caller's caps |
| GET | `/v1/config/`, `/v1/config`, `/v1/config/search`, `/v1/config/audit` · POST `/v1/config` | `config.*` (only when a config center is set) |

## 9. `kernel/webhook` (outbound)

`NewDispatcher(bus, sinks, log, opts...)` + `Start(ctx)`: one goroutine per sink with `bus.Subscribe(sink.Subject, 256)`; skips
`webhook.*` kinds (no loops); `deliver` POSTs `json(event)` with headers `X-Agezt-Event`, `X-Agezt-Subject`, `X-Agezt-Delivery`
(= event id, for dedupe), `X-Agezt-Signature: sha256=<hmac>` when a secret is set; up to `DefaultMaxAttempts`=3 with backoff 250 ms·n;
journals `webhook.delivered` / `webhook.failed` (read back by the control plane's `webhook_log`). `ParseSinks("url|subject|secret,…")`
(bounded split so secrets may contain `|`; non-loopback `http://` rejected — HTTPS required). `Probe` sends a test delivery
(`agt webhook test`). `cmd/agezt buildWebhooks` (`AGEZT_WEBHOOKS`) wires a `netguard` client (SSRF guard; `AGEZT_WEBHOOK_ALLOW_LOOPBACK`,
`AGEZT_WEBHOOK_ALLOW_PRIVATE` opt-outs). `DefaultTimeout` 10 s. Imports `bus`, `event`; imported by `cmd/agezt`, `cmd/agt`.

| File | What it does |
|---|---|
| `webhook.go` | Package doc, `Sink`, `Publisher`, `Dispatcher{MaxAttempts, Backoff}`, `Option`, `WithClient`, `NewDispatcher`, `Start`. |
| `webhook_dispatch.go` | `run`, `deliver`, `post`, `newDeliveryRequest`, `journal`, `backoff`. |
| `webhook_helpers.go` | `sign`, `verb`, `loopbackHost`, `ParseSinks`, `ProbeResult`, `Probe`, `Describe`. |

Note: the INBOUND webhook path is the Web UI `/hooks/{workflow}` route (§5.2), not this package.

## 10. `kernel/tunnel`

Supervises an external tunnel binary so the Web UI or REST API is reachable publicly. `Config{Provider ("cloudflare"/"cloudflared",
"ngrok", "tailscale", "tailscale-funnel", "custom"), Command, TargetURL, OnURL}`, `New`, `Start(ctx)` (restart with backoff 1 s→30 s;
30 s uptime resets backoff), `URL()` (scraped from the child's stdout/stderr by regex). Process groups killed on shutdown
(`proc_unix.go` setpgid/kill group, `proc_windows.go` tree kill). No imports of other kernel packages. `cmd/agezt/httpsurfaces_tunnel.go`
reads `AGEZT_TUNNEL` / `AGEZT_TUNNEL_CMD`, auto-allowlists the public host on the Web UI (`SetAllowedHosts` → forces password-strict),
and warns when no console password is set.

| File | What it does |
|---|---|
| `tunnel.go` | Package doc, `Config`, `Tunnel`, `New`, `URL`, `Start` supervisor, `buildCommand` presets, `extractURL`, `execRun`. |
| `proc_unix.go` | `setProcessGroup`, `killProcessTree` (non-Windows). |
| `proc_windows.go` | Windows equivalents. |

---

## 11. Security model at the HTTP edge (deliverable d)

| Control | Where | Detail |
|---|---|---|
| Loopback default | `cmd/agezt` | Control plane always `127.0.0.1:0`; Web UI default `127.0.0.1:8787`; REST/OpenAI off unless an addr is set; banners warn `[WARNING: not loopback]`. |
| Control-plane token | `controlplane` | 256-bit per-boot token, 0600 runtime file; constant-time compare; blank never matches; 16 MiB pre-auth line cap; panic firewall. Possession = full daemon authority (incl. `shutdown`, `config_set`, `update_apply`). |
| Tenant tokens | `controlplane`, `httpserver` | Control plane: `args.tenant` + registry token → TenantAllowed allowlist, tenant arg pinned. HTTP: only `TierUser` routes accept tenant credentials (header `X-Agezt-Tenant`); `TierAdmin` never. |
| Tier type safety | `auth` + `httpserver.Router.Handle` | `TierUnset` zero value panics at registration; methods enforced (405) so GET cannot reach POST-mutations. |
| Web UI credentials | `webui` | Per-boot console token (banner URL `?token=`; SPA scrubs it from the address bar and sends Bearer); data routes do NOT accept `?token=` (except legacy on `/events`); `/events` uses the separate ephemeral SSE token `?st=` because EventSource cannot set headers. |
| Console password | `webui/session.go` + `cmd/agezt` | `AGEZT_WEB_PASSWORD` or a per-install minted password (0600 file, loopback only, shown once — replaced a hardcoded `"agezt"`, SECRET-002). Default: token OR session; strict (`AGEZT_WEB_PASSWORD_STRICT=on`, auto-on for any non-loopback allowed host or wildcard bind, AUTH-001): token AND session. Session cookie `agezt_web_session` HttpOnly, SameSite=Strict, Secure behind TLS/X-Forwarded-Proto, 12 h sliding; 8 bad logins → 5 min global lockout; constant-time compare. |
| Host allowlist (DNS rebinding) | `webui.secure/hostAllowed` | `localhost` + any non-unspecified IP literal + explicitly registered hosts (`SetAllowedHosts`, tunnel host). Else 403 "forbidden host". |
| CSRF / origin | `webui.sameOriginMutation` | Non-GET/HEAD/OPTIONS rejected when `Sec-Fetch-Site: cross-site` or `Origin` host:port ≠ `Host`; absent Origin allowed (non-browser clients). SameSite=Strict cookie; Bearer token not readable cross-site. |
| Security headers | `webui.setSecurityHeaders` | `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, strict CSP (`default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; …; frame-ancestors 'none'`). API JSON `Cache-Control: no-store`. |
| Write allowlist | `webui` route maps | No generic passthrough: every route maps to one fixed op with a fixed arg allowlist (query or body keys); unexpected body keys dropped. |
| Body limits | router `BodyMax` | Web UI JSON 1 MiB, login 4 KiB, hooks 256 KiB, audio 25 MiB, files 256 B; REST/OpenAI 16 MiB, REST update apply 64 KiB; agentgw 1 MiB. |
| Slow-loris / timeouts | `httpserver.NewStreamingServer` | ReadHeaderTimeout 10 s, IdleTimeout 120 s, no write timeout (SSE). |
| SSE caps | `httpserver.sseLimiter` | 64 concurrent streams per client IP across ALL surfaces (webui, restapi, openaiapi, agentgw) → 429. |
| `/hooks/` rate limit | `webui.allowHook` | Only throttled HTTP path (owner decision: token-free path only): 60/min + 30 burst per workflow+IP, keyed pre-auth; uniform 403 for unknown/disabled/bad secret; secret compared constant-time in `workflow_webhook`. |
| Artifact serving | `webui/artifact_route.go`, `restapi/artifacts.go` | Content-Type allowlist (else `application/octet-stream`), sanitized filename; REST byte transfer off unless `AGEZT_REMOTE_ARTIFACT_BYTES=allow`. |
| File Manager | `webui/files_route*.go` | Root confinement + link checks in platform/fileworkspace; writes enter primary-only ops, policy and mandatory tool audit before filesystem effects. |
| SSRF | `netguard` (see [05-governance-routing-security.md](05-governance-routing-security.md)) | Webhook sinks use a netguard HTTP client (blocks loopback/private/metadata unless opted out) + HTTPS required off-loopback; control-plane `netguard_log` audits tool egress blocks. `provider_probe` / `whatsappgw_*` / channel OAuth make operator-directed outbound calls from the daemon. |
| Error redaction | `openaiapi.redactErr` | Upstream error strings scrubbed before returning to API clients. |
| agentgw | `agentgw` | HS256 JWT with fixed iss/aud, expiry, per-token rate limit, capability checks per route, no capability escalation on child tokens, audit to journal, no CORS headers. |
| Public exposure | `tunnel` | Opt-in only; public host auto-allowlisted and strict password mode forced on the Web UI. |

## 12. Extension points

- **New control-plane op**: add `CmdX = "x_y"` to the right `protocol_commands*.go`; implement `func (s *Server) handleX(conn, req)`
  using typed accessors (`argString`, `argLimit`, … — `args_ratchet_test.go` forbids new raw casts) and `ok/fail`; register a
  `commandSpec` in the domain's `register*Commands` (or a themed group in `registry.go`). Set `TenantAllowed` (+`TenantRouted`) only if
  it must be callable by tenant tokens and acts on `dc.K`/`kernelFor`. Choose `Streaming`. `dispatch_registry_test.go` pins the constant↔registry 1:1.
- **Journal audit view**: in the owning `kernel/app/<domain>` package, decode typed rows with `journalview.ProjectValues` (see `app/audit`) and bind a read-only spec.
- **Expose an op in the Web UI**: add one entry to `apiRoutes` / `readArgsRoutes` (GET) or `writeRoutes` / `jsonRoutes` (POST) with an
  explicit arg allowlist; numeric query keys must be in `numericQueryArgs`; add a `*_route_test.go` (forwarding + GET rejection for writes).
  Streaming results need a bespoke handler using `httpserver.StartSSE` + `Client.Stream`.
- **New HTTP surface/route**: use `httpserver.NewRouter` + `RouteOpts{Tier: …}` (mandatory), `httpserver.Start`, `StartSSE` for streams.
- **New daemon env var**: add to `controlplane/config.go configEnvVars` (guard test).
- **New late dependency**: add a field to `LateDeps` + setter; nil-skip in `Bind`.
- **New agentgw capability**: const in `types.go`, route in `Gateway.Listen` behind `withAuth` + `capCheck.Check`, and `agt token` validation list in `cmd/agt/token.go`.

## 13. Gotchas / invariants

- One request per TCP connection; a streaming op's connection IS the run's lifetime. Closing it early cancels `StreamLive` work and (with
  cancel-on-disconnect) runs — which is why `/api/plan/run` drives `Client.Stream` to completion even though it discards events.
- Never add `StreamLive` to an op that reads the conn itself (`run`, `pulse_subscribe`): two readers race and 500 ms deadlines would cancel idle streams.
- Every TenantAllowed op must be TenantRouted (except `whoami`), or a tenant reads the primary kernel.
- Tenant probes get `forbidden` for unknown commands too (no command-existence oracle); authentication precedes lookup.
- Board writes must use the injected shared `boardStore`; a second `board.Store` would clobber the `board` tool's whole-file writes.
- `agent_list` is cached 1.5 s; mutating handlers must call `invalidateAgentListCache()`.
- `config_set` mutates the daemon process env (`os.Setenv`) as well as the store; only `AGEZT_PROVIDER/AGEZT_MODEL` trigger a reload.
- Query-string proxy args are strings; numeric keys absent from `numericQueryArgs` reach the handler as text and fail "must be a number" (502).
- `httpserver` deliberately has no WriteTimeout; `agentgw` builds its own `http.Server` with `WriteTimeout: 30s`, which will cut its own SSE `/v1/eventbus/subscribe` streams after 30 s.
- `/api/files/{mkdir,rename,delete}` have `BodyMax = defaultFileCap = 256` bytes — a rename with two long paths can exceed it (413-class failure).
- `restapi` handlers for `/metrics`, `/api/v1/health`, `/api/v1/models` do not `bind()` the tenant: a tenant credential sees primary-kernel metrics/models.
- The login lockout counter is global, not per IP: 8 wrong passwords from anyone lock everyone out for 5 min (DoS trade-off).
- `/events` streams the whole primary bus to any authorized console session; it is not tenant-scoped or filtered.

## 14. Cross-area surprises

- **Layering violation**: `kernel/controlplane` imports `plugins/tools/overseertool` (`roster_repair.go`, `roster_wake.go` call
  `overseertool.NewKernelSource(s.k, s.baseDir)`), contradicting "kernel never imports plugins".
- **Protected delivery checkpoint after W2.19e:** authenticated GitHub reads verify open PR #701 at local ae063ff, main base e5379d8 and successful old-head CI37357717652.188 scoped files+35 removals are prepared; GitHub create_tree is denied by the required-approval/never-policy gate. No remote object/ref written. CLI HTTP401 and local read-only Git are separate constraints; unpublished snapshot final-head CI and protected merge remain open.
- **Channel account typed binding (W2.27m, local delivery pending):** concrete set/remove results and two primary POST writer specs; ordered codec admission preserves errors/unknown args; native wrappers/manual rows removed. Selected root ports and persistence semantics unchanged. Mandatory audit success/canceled preflight now precede effects.576 native cases20:288 normal raw response/file exact+288 canceled/no effects;29 valid mutations/schema/source/native/tenant/closed-journal20/full gates. OAuth/gateway/inbox/send and protected delivery remain.
- **Channel inventory typed binding (W2.27l, local delivery pending):** concrete DTOs/typed probes and matrices/full nested schema/GET primary read spec/shared dispatcher; native wrapper/manual row removed. Secret value absence/public emptiness/empty arrays and selected reader preserved. Native codec retains MediaCaps order; canceled preflight stops before Prepare.160 native cases20=80 raw byte-exact normal+80 changed cancellation,27 valid mutations/source/schema/root/tenant/media-order20/full Go/race gates;242 packages135 imports13 calls,133 kernel packages/2872 Go files. Remaining channel typed binding/native exit/protected publication remain.
- **Channel ACP typed binding (W2.27k, local delivery pending):** one GET/primary-only/read-only spec/shared dispatcher, native wrapper/manual row/final structToMap removed. Service/Web UI read route bytes retained. Explicit canceled preflight rejects before discovery; native integer terminal exact for declared counts.432 native cases20=216 byte-exact normal+216 changed cancellation,12 valid mutations/schema/auth/source/tenant/old-current integer20/full Go/race gates pass;242 packages135 imports13 calls,133 kernel packages/2868 Go files. Remaining channel typed binding/native exit/protected publication remain.
- **Channel ACP inventory foundation (W2.27j, local delivery pending):** app owns active environment/trim/default cached discovery/caller context/full typed inventory; native args ignored/manual primary read-only/legacy structToMap codec retained.432 byte-exact native cases20/source-cache-context/full files/head/provider checks,10 valid mutations/full model/default source20/tenant no-discovery denial/full Go/race gates;242 packages135 imports13 calls,133 kernel packages/2866 Go files. Official ratchet removes CP catalog import; typed communication binding/native exit/protected publication remain.
- **Channel outbound send foundation (W2.27i, local delivery pending):** app Outbound owns validation/normalization/selected sender/background30s/error/result; terminal callback retains measured context through socket write. Native current sender factory/lenient codecs/manual primary audit retained.160 byte-exact native cases20/effects/files/audit privacy/deadline/provider checks,15 valid mutations/before-after lifetime/current sender/isolation/tenant/source20/full Go/race gates pass;242 packages136 imports13 calls,133 kernel packages/2863 Go files. ACP inventory/typed binding/native exit/protected publication remain.
- **Typed edict reads (W2.33a, local):** the new `kernel/app/edict` owns `edict_show` (`GET /api/edict_show`), `edict_deny_list` and `edict_test` (`GET /api/edict/test`) as tenant-owned, caller-tenant-routed, read-only unaudited operations over the routed kernel's `Engine`; the strict routing tenant is checked first except for the probe, which checks its capability first, as before. The three native handlers and `denyRuleRows` are removed; the four writes stay native for W2.33b. [Evidence](76-w233-typed-edict-reads-evidence.md).
- **Typed guard audit reads (W2.32d, local):** the new `kernel/app/audit` owns `netguard_log`, `ratelimit_log`/`ratelimit_stats` and `warden_log`/`warden_stats` as tenant-owned, caller-tenant-routed, read-only unaudited operations (three GET routes); warden rows carry only their kind's keys via omitempty pointers. `netguard_log.go`, `ratelimit_log.go`, `warden_log.go` and five registrations are removed. [Evidence](75-w232-typed-guard-audit-evidence.md).
- **Typed changelog/cache stats (W2.32c, local):** `changelog` (primary, tenant-routed) and `cache_stats` (tenant-owned and routed) join `app/journal` as read-only unaudited operations; the full-rate pricing is injected as a `Cost` port (`WithCost(governor.CostMicrocents)`), so app/journal does not import the governor. `changelog.go`/`cache_stats.go` and two registrations are removed; the trimFloat unit test moves with its helper. [Evidence](74-w232-typed-changelog-cache-evidence.md).
- **Typed journal grep/export (W2.32b, local):** `journal_grep` (`GET /api/journal`) and `journal_export` join `app/journal` as primary-only, read-only unaudited operations over the primary journal; the adapter keeps the Event member order for both, and the export schema comes from the same wire mirror. `journal_grep.go`/`journal_export.go` and two registrations are removed; `MaxJournalExportN` now names `appjournal.MaxExportN`. [Evidence](73-w232-typed-journal-search-evidence.md).
- **Typed journal head/tail/stats (W2.32a, local):** three primary-only, read-only unaudited app operations over `appjournal.Service`: head and tail read the primary journal, stats follows an operator-named tenant (`TenantRouted` without `TenantAllowed`, derived from PrimaryOnly + CallerTenant). The tail output schema comes from a wire mirror of `event.Event` (raw payload = any JSON), and the adapter writes the journal's Event structs so their member order is unchanged. `journal.go` and three registrations are removed. [Evidence](72-w232-typed-journal-reads-evidence.md).
- **Typed run reads (W2.31c, local):** `runs_list` (`GET /api/runs`) and `runs_stats` are tenant-owned, caller-tenant-routed, read-only unaudited app operations over `appruns.Service`; `collectRuns` stays native as the snapshot port (`runReads`), shared with schedule firing views and roster status. `runs_handlers.go`/`runs_handlers_stats.go` and their registrations are removed. [Evidence](71-w231-typed-run-reads-evidence.md).
- **Typed targeted run cancel (W2.31b, local):** `cancel_run` (`POST /api/cancel_run`) joins `app/steer` as a tenant-owned, caller-tenant-routed write; the routing `tenant` stays a strict optional string checked after the correlation, and the native handler/registration are removed. The common adapter now passes the trimmed tenant as the caller tenant, so an operator-selected padded tenant (`" acme "`) is audited as the `acme` kernel it reached. [Evidence](70-w231-typed-cancel-run-evidence.md).
- **Typed live run steering (W2.31a, local):** `run_pause`/`run_resume`/`run_step`/`run_steer` (`POST /api/run/*`) and the unrouted `run_intervene` are tenant-owned, caller-tenant-routed, non-read-only app operations over `appsteer.Service`, bound to the kernel the dispatcher routed to. Strict codecs, the note/steer mode and the intervention mapping live in `kernel/app/steer`; `steer.go`, `registerSteerCommands` and `argFloat64` are removed. Operator-selected tenant steers are audited with their tenant label. [Evidence](69-w231-typed-steer-evidence.md).
- **Typed agent remove (W2.30r, local) — roster domain complete:** `agent_remove` is a primary-only, non-read-only app operation (`POST /api/agents/remove`) over `approster.RemoveService`, which owns the cascade order and report; the native teardown helpers (`roster_teardown*.go`, `agentRemovalMailboxImpact`, `agentWorkflowImpact`) are its `RemovePorts`. No native roster command group remains (`registerRosterCommands` removed). 84 cloned-kernel steps x20 with response, journal and full-state parity, 36 mutations/full gates. [Evidence](68-w230-typed-agent-remove-evidence.md).
- **Typed agent retire/revive (W2.30q, local):** `agent_retire`/`agent_revive` are primary-only, non-read-only app operations (`POST /api/agents/retire`, `/api/agents/revive`) over `approster.SetRetiredService`; shared dispatch audits them before the write. The retirement previews impact through `ImpactService`, pauses triggers via the native `pauseAgentStanding`/`pauseAgentSchedules`; the revival re-checks hierarchy refs. 102 cloned-kernel steps x20, 29 mutations/full gates. [Evidence](67-w230-typed-agent-retire-revive-evidence.md).
- **Typed agent impact/tombstone (W2.30p, local):** `agent_impact` (`GET /api/agents/impact`) and `agent_tombstone` are primary-only read-only app operations over `approster.ImpactService` and the native `nativeImpactSource` (`roster_cascade.go`: roster get, `agentSubagents`, per-subsystem `Holdings`). `ImpactOutput`/`TombstoneOutput` are typed; `SubagentImpactLabels`/`AggregateSubagentLabels` live in app/roster. Native `agentImpactResult` (retire) and `subagentImpact` (remove) wrap the app. 120 cloned-kernel steps x20 incl. retire, 36 mutations/full gates. [Evidence](66-w230-typed-agent-impact-tombstone-evidence.md).
- **Typed agent resolve (W2.30o, local):** `agent_resolve` is a primary-only, non-read-only app operation (`POST /api/agents/resolve`) over `approster.ResolveService`, which owns the paused/retired/delegated/force_chain decision and payloads; native effects come in through `ResolvePorts` (journal lookups `latestExhaustedRoutingChain`/`latestOperatorForceGeneration`, `postOperatorHelp`, `applyRoutingChain` in `roster_resolve.go`). Shared dispatch audits it before the request is journaled or applied. 222 cloned-kernel steps x20 with journal, board and routing-chain parity, 52 mutations/full gates. [Evidence](65-w230-typed-agent-resolve-evidence.md).
- **Typed agent repair (W2.30n, local):** `agent_repair` is a primary-only, non-read-only app operation (`POST /api/agents/repair`) over `approster.RepairService`; shared dispatch audits it before the repair is journaled or launched. The governed repair stays native in `runAgentRepair` (WF-001 panic firewall, overseer repair source, terminal events unchanged), started on its own goroutine by the launch port. Wake and repair share `directTarget` and `IncidentLineage`. 120 cloned-kernel steps x20 with correlation-grouped journal parity, panic-containment test, 30 mutations/full gates. [Evidence](64-w230-typed-agent-repair-evidence.md).
- **Typed agent wake (W2.30m, local):** `agent_wake` is a primary-only, non-read-only app operation (`POST /api/agents/wake`) over `approster.WakeService`; shared dispatch audits it before the wake is journaled or launched. The run stays native: the launch port starts `runAgentWake` in a goroutine. `BuildOperatorWakeIntent` and `ManagedDirectCallError` live in app/roster (native `managedSubagentDirectCallError` wraps the latter for repair/run/workboard). Check order, lenient reason/lineage, truncated requested payload and the full launched intent preserved. 138 cloned-kernel steps x20 with correlation-grouped journal parity incl. the async mock run, 36 mutations/full gates. [Evidence](63-w230-typed-agent-wake-evidence.md).
- **Typed agent task update (W2.30l, local):** `agent_task_update` is a primary-only, non-read-only app operation (`POST /api/agents/task`) over `approster.TaskUpdateService` and the journaled `UpdateProfile`; shared dispatch audits it before the write. Native handler and helpers removed; the host keeps the legacy `AgentTask` member order in `task`. Validation order, presence rules, errors, and journaling of unchanged updates preserved. 192 cloned-kernel steps x20 with raw and journal parity, 31 mutations/full gates. [Evidence](62-w230-typed-agent-task-update-evidence.md).
- **Typed agent add/edit (W2.30k, local):** `agent_add`/`agent_edit` are primary-only, non-read-only app operations (`POST /api/agents/add`, `/api/agents/edit`) over `approster.ProfileWriteService`; shared dispatch audits them before the write. Patch, kind and hierarchy helpers live in app/roster (native `validateAgentHierarchyRefs` wraps the app one); owner-before-parent order replaces random map order. 114 cloned-kernel steps20 with full journal parity, 27 mutations/full gates. [Evidence](61-w230-typed-agent-add-edit-evidence.md).
- **Typed agent set-enabled (W2.30j, local):** `agent_set_enabled` is a primary-only, non-read-only app operation (`POST /api/agents/enable`) over `approster.SetEnabledService`; shared dispatch audits it before the write. Native handler removed. Lenient enabled codec, error texts, legacy profile view and resume-only paused counts preserved. 102 cloned-kernel steps20 with full journal parity, 16 mutations/full gates. [Evidence](60-w230-typed-agent-set-enabled-evidence.md).
- **agent_resolve audit (W2.30i, local):** the mutating native `agent_resolve` registration was `ReadOnly`, so dispatch skipped its operation audit; the flag is removed and the handler is unchanged. Native regression red3/green20. [Evidence](59-w230-agent-resolve-audit-evidence.md).
- **Typed agent escalations (W2.30h, local):** `agent_escalations` is a primary-only ReadOnly app operation (`GET /api/agents/escalations`) over `approster.EscalationService` (roster get, `EscalationBoard` help/replies view built natively with the 500 cap, journal range). Native handler, fold, row type and `argLimit` removed; `approster.BoardMessageAckedBy` serves the native status/lifecycle callers. Legacy error order (board open before strict cursor), status precedence, origins and tie-break preserved. 756 native cases20,28 mutations/full gates. [Evidence](58-w230-typed-agent-escalations-evidence.md). Repair status delivered via PR715 at 6067c311.
- **Typed agent repair status (W2.30g, local):** `agent_repair_status` is a primary-only ReadOnly app operation (`GET /api/agents/repair_status`) over `approster.RepairStatusService` (roster get, journal range, native cooldown reader, clock). Typed rows/contract/next action; native handler, view helpers and `parseSeqCursor` removed. Fixes the in-place cursor filter that moved `latest`/`next_eligible_ms`/`next_action` with the page. 756 native cases20,29 mutations/full gates. [Evidence](57-w230-typed-agent-repair-status-evidence.md). Activity delivered via PR714 at bb8cc8fa.
- **Typed agent activity (W2.30f, local):** `agent_activity` is a primary-only ReadOnly app operation (`GET /api/agents/activity`) over `approster.ActivityService` (roster get + journal range). Native handler/manual row removed. Legacy ref→unknown-agent→limit order, lenient seq cursor, 50/500 limits, `null` vs `[]` shapes preserved; canceled admission is the intended change.792 native cases20,23 mutations/full gates. [Evidence](56-w230-typed-agent-activity-evidence.md). Activity text delivered via PR713 at fb6a4450.
- **Activity summary move (W2.30e, local):** `approster.ActivitySummary`/`IsMailboxWakeSubject` own the per-event activity text; native `agent_activity` and the status journal collector call them; `agent_activity_summary.go` and `roster_activity_text_misc.go` are gone. 2M-event legacy/moved parity, golden branch test, 21 mutations/full gates. [Evidence](55-w230-activity-summary-move-evidence.md). Graveyard delivered via PR712 at 53c7dd19.
- **Typed agent graveyard (W2.30d, local):** `agent_graveyard` is a primary-only ReadOnly app operation over `approster.GraveyardService` (roster list + injectable clock); no HTTP route. Native handler, manual row and dead `plInt64` removed. Lenient `older_than_days`, boundary/unknown-age rules, truncation, stable oldest-first order and `[]` empty shape preserved; canceled admission is the intended change.468 native cases20,22 mutations/full gates. [Evidence](54-w230-typed-agent-graveyard-evidence.md). Typed list delivered via PR711 at b29dd422.
- **Typed agent list (W2.30c, local):** `agent_list` is a primary-only ReadOnly app operation (`GET /api/agents` metadata) over the app ListService; the native handler/manual row are gone. Concrete ListOutput/ProfileOutput/StatusOutput retain the full lower Profile, legacy float64 profile and cursor projection, exact status integers and explicit-null presence; runbook/mailbox stay dynamic objects. Already-canceled admission is the one intended wire change.528 native cases20,29 mutations/full gates. [Evidence](53-w230-typed-agent-list-evidence.md). Status delivered via PR710 at74727b6b; remaining roster commands/status collection/order7/W3-W5 remain.
- **Roster status foundation (W2.30b, local):** app owns full presentation/typed observation rows/window policy; native current collection remains.3072 raw render cases20+72 actual native collector cases20,21 mutations/full fields/source/race/full gates. [Evidence](52-w230-roster-status-foundation-evidence.md). List/cache delivered via PR709 atd8856048; typed list/remaining roster/own delivery/order7/W3-W5 remain.
- **Roster list foundation (W2.30a, local):** app owns presentation/cache/pagination over current native profiles/status ports. Empty0-key invalidation collision fixed (red3/stale1/1, native after20);240 raw native cases20,15 mutations/source/race/full gates. [Evidence](51-w230-roster-list-foundation-evidence.md). Native codec/manual binding/status derivation retained; Update delivered via PR708 at0d5777c5; own delivery/remaining roster/order7/W3-W5 remain.
- **Typed update/native exit (W2.29b, local):** two primary typed operations share native dispatch; wrappers/manual rows removed. New TerminalWrite preserves sentinel→response→100ms restart and distinguishes returned errors from writer panics.480 native cases20/source/lifetime/tenant/admission/race20,27 mutations/full gates/no-I/O9.0–13.1us. [Evidence](50-w229-update-exit-evidence.md). Foundation delivered via PR707 at0f97e7a3; own delivery/order7/W3-W5 remain.
- **Update foundation/tunnel premise (W2.29a, local):** app owns check/apply business over selected verified backend; native codec/manual bindings retained.480 native cases20/lifecycle20/naive bridge red3/19 mutations+cancel mutation3/full gates; returned write failure still restarts, write panic does not. Nil concrete setter preserved. Tunnel only has boot adapter/layer5 supervision; existing helper20/source race20 pass. [Evidence](49-w229-update-foundation-evidence.md). Webhook delivered via PR706 at1f5d5c36; typed update/native exit/own delivery/order7/W3-W5 remain.
- **Webhook observability/native exit (W2.28, local):** two typed tenant reads use selected appHost journal; delivery/stats use app services/shared projection; native handlers/manual rows removed.240 raw native cases20, canceled red3/after20, real tenant isolation/race/source20,27 mutations/full gates; no-I/O24.6–27.8us. [Evidence](48-w228-exit-evidence.md). Send delivered via PR705 atc32e8852; own delivery/tunnel/update/order7/W3-W5 remain.
- **Channel send/native exit (W2.27q, local):** all eleven channel operations use typed shared native binding. Send terminal scope preserves background30s context through socket delivery and immediate sender-panic cleanup, with mandatory audit/canceled admission; empty registrar removed. Naive bridge proof red3/after20,160 cases20/34 mutations/lifetime/race/full gates and eleven-operation exit tests. [Evidence](47-w227-exit-evidence.md). Inbox delivered via PR704 atca25f5c7; remaining order6/7/W3-W5 and own main publication remain.
- **Channel inbox typed binding (W2.27p, local):** typed root/thread/message schema, one primary ReadOnly GET and legacy input/error precedence. Wrapper/manual row removed; selected journal/Web UI unchanged. Native codec preserves nested member order/int64; canceled preflight stops Range.200 cases20=100 raw wire exact+100 canceled,32 valid mutations/schema/native/source/JSON limit/selected journal/range precedence20/full gates. Gateway delivered via PR703 at5703f2d5; send/exit and own main delivery remain.
- **Channel inbox service foundation (W2.27h, local delivery pending):** app Inbox owns journal fold/models/limit/filter/sort/cursor/presentation; native float-only limit/lenient channel/delayed cursor codec and selected kernel journal/manual primary read-only binding remain.200 byte-exact old/new native cases20/full owned file/head/provider checks,21 final valid mutations/contracts/selected journal/tenant denial/source20/full Go/race gates pass;242 packages136 imports13 calls,133 kernel packages/2858 Go files. Send/ACP, typed binding/native exit/protected publication remain.
- **Channel gateway typed binding (W2.27o, local):** separate result DTOs retain false/empty/optional member presence, two primary ReadOnly POST specs and lenient inputs use shared dispatcher. Wrappers/manual rows removed; selected guarded GET policy/UI routes unchanged; already-canceled requests stop before HTTP.480 cases20=240 raw wire/request exact+240 canceled no effects;35 valid mutations/source/schema/native/closed-journal20/full gates. OAuth delivered through PR702 ata00b580c; inbox/send/channel exit and own protected delivery remain.
- **Channel gateway service foundation (W2.27g, local delivery pending):** app Gateway owns status/QR normalization/endpoint/header/body-bound/presentation behind bounded GET; native manual primary read-only codecs and selected unchanged platform guarded/background-context HTTP remain.480 byte-exact old/new native cases20/no normalization,22 valid mutations/contracts/source/tenant denial20/full Go/race/gates pass;242 packages136 imports13 calls,133 kernel packages/2854 Go files. Inbox/send/ACP and typed binding/native exit/protected publication remain.
- **Channel OAuth typed binding (W2.27n, local):** concrete start/callback/status outputs preserve known-empty versus unknown member presence; three primary specs with lenient input, mandatory writer audit/status read-only and canceled preflight. Wrappers/manual rows removed; native ports and public Web UI page/routes unchanged.450 cases20 (225 normal wire/state/files/exchange +225 canceled no effects),29 valid mutations/schema/native/source/closed-journal/race/full gates. Prior W2.27m is delivered at main8416d9c8 via PR701/all24 successful CI jobs; this slice own delivery and gateway/inbox/send exit remain.
- **Channel OAuth ownership (W2.27f, local delivery pending):** app OAuthMemory owns provider/state/mutex/prune/status and guarded-client exchange business; native sync.Once service/memory binds HTTP/vault factories. Native wrappers/app use-case business unchanged.450 old/new native cases20,20 valid mutations/provider/TTL/concurrent owner/source exchange/race/full gates pass;242 packages136 imports13 calls,133 kernel packages/2851 Go files. Remaining communication/typed binding/native exit and protected publication remain.
- **Channel OAuth service foundation (W2.27e, local delivery pending):** app/channels OAuth owns start/callback/status validation/presentation/sequence and moved nonce/redirect/instance helpers, selected native state/table/guarded exchange/vault ports remain.450 native cases20 with independently validated nonce/clock normalization/state/files/HTTP/audit privacy,17 valid mutations/port/original source20/full Go/race gates;242 packages136 imports13 calls,structure133. State/provider/exchange ownership and typed binding/native exit remain.
- **Channel OAuth status snapshot repair (W2.27d, local delivery pending):** owned before race/mixed-pair proof3, after race20; four fields captured under mutex then released before socket. Blocked writer/immutable capture/known-unknown/raw20 and3 valid mutations/full source/Go/race gates pass;242 packages136 imports13 calls,structure133. Start/callback/exchange/helper/server state bytes retained; OAuth ownership/service foundations follow.
- **Registered account removal repair (W2.27c, local delivery pending):** native SectionEnvs now reads selected server-root merged/filtered registry like account Set. Actual native lifecycle before-red/after-green20 deletes/counts registered config/vault keys and removes listed account while default/other persist; every builtin env-list/selected root/filter/freshness20,5 valid mutations/full source/Go/race gates. Accounts app business bytes unchanged;242 packages136 imports13 calls,structure133. Remaining communication/typed binding/native exit follows.
- **Channel account writer foundation (W2.27b, local delivery pending):** app/channels Accounts owns selected section-field validation and set/remove business through fresh selected config/vault/manifest/global section-env ports; native strict codecs/manual primary audit remain.576 byte-exact native cases20 with independent audit/privacy/partial effects and19 valid mutations, port/native tenant/persistence/source/full Go/race gates.242 packages136 imports13 calls,structure133. Global removal versus selected registered-set schema, ignored load/Set and legacy context remain explicit; remaining communication/typed binding/native exit follows.
- **Channel inventory foundation (W2.27a, local delivery pending):** app/channels Inventory owns channel_list presentation/probe/media totals through selected native server-root snapshot/global manifest/liveness ports; native manual primary ReadOnly binding and other writer/OAuth/gateway bodies stay.160 byte-exact cases20 without normalization,20 valid mutations/selected-root/tenant/privacy/source/full Go/race gates.242 packages136 imports13 calls,structure133. Account writers and remaining communication operations/typed binding/native exit follow.
- **Configcenter native exit (W2.26d, local delivery pending):** all nine typed primary operations/signatures/aggregate/DTOs and selected core ports; four native handler/registrar files removed. CLI seconds/audit fields/filter/missing-value fixes have11 before/after cases20 and68 schema-valid owned cases20/race20;88 wave mutations, nine-op fake-port/noop-audit dispatch10300-33862 ns/op and full Go/race/source gates.241 packages136 imports13 calls,structure132. [Evidence](46-w226-exit-evidence.md). Remaining order6 channels/webhook/tunnel/update follows; wider migration/protected delivery remain.
- **Configcenter typed native migration (W2.26c, local delivery pending):** nine typed PrimaryOnly/Primary unary specs/codecs/DTO/schema now use generic app dispatch, with five unaudited reads/four mandatory writers; selected Center ports remain, four native handler/registrar files removed.896 byte-exact normal cases20 with bounded entry-clock replacement,28 valid mutations/typed/native tenant/privacy/source/full Go/race gates. Unchanged13-case before/after proof20 pins closed-journal and pre-context rejection; four terminal audit-failure fixtures retain actual persisted effects. Native exit/CLI evidence follows.
- **Configcenter writer service foundation (W2.26b, local delivery pending):** selected app/configcenter Writes owns set/delete/rating/access business and computed masked echoes. Core Center/classifier/ACL/vault/persistence and native codecs/manual primary mandatory audit remain; native entryToMap shim removed.368 parity cases20 compare responses/state/disk with independently bounded timestamp normalization;17 valid mutations/port/native tenant/audit/source/full Go/race gates. Nine-op typed DTO/schema/binding/cancellation/audit refinement and exit remain.
- **Configcenter read presentation foundation (W2.26a, local delivery pending):** selected app/configcenter five-read maps and verbatim masked entry projection; native codecs/manual primary read policy/core agent ACL/access audit/four writer bodies remain.550 byte-exact cases20 (275 canceled),20 final mutations and source/core/native privacy/tenant/filter/root tests/full Go/race gates;241 packages136 imports13 calls,structure132. Four writers then typed nine-op binding/exit.
- **Settings native exit (W2.25d, local delivery pending):** exact five primary typed specs/DTO/codecs, mandatory writer admission/cancellation and post-effect error contracts; legacy schema member-order codec remains.72read+140writer parity20, new CLI38cases20/race20/owned rollback checkpoints,91 wave mutations and full gates;240 packages136 imports13 calls,structure131. [Evidence](45-w225-exit-evidence.md). Configcenter9 follows; live deployment/protected publication remain.
- **Typed five-operation settings binding (W2.25c, local delivery pending):** two primary read/three mandatory unary app specs own DTO/schema/strict codecs/native metadata; old wrappers/manual registrar removed.8 actual audit/cancel before-fail/after20,3 post-effect journal failures/no rollback,72read+140writer exact parity20,38 final mutations/79 total/full Go/race gates;240 packages136 imports13 calls,structure131. Narrow native codec preserves legacy core Section/Field JSON member order. Settings exit then configcenter9; live config propagation/protected delivery remain.
- **Settings writer service foundation (W2.25b, local delivery pending):** app/settings Set/Register/Unregister orchestrate validation/load/write/save/pin/live/reload/results behind selected server-root ports; native codecs/manual mutation audit remain.280 byte/state/env/reload exact cases20 with independent audit arcs,24 valid mutations/41 settings total and full Go/race gates;240 packages136 imports13 calls,structure131. Typed five-op binding/explicit DTO/audit/cancellation next; live daemon rebuild/protected delivery remain.
- **Settings read presentation foundation (W2.25a, local delivery pending):** app/settings Schema/Values maps use section/prepared-values ports; native adapter selects server base, pinned flags and legacy store/vault load policy.144 byte-exact cases20 (72 canceled), port/source/core/native tenant/root/privacy/freshness tests and17 mutations/full Go/race gates;240 packages136 imports13 calls,structure131. Three writers/manual five-command policy remain; writer foundation then typed binding/exit.
- **Config inventory native exit (W2.24c, local delivery pending):** one typed primary ReadOnly unaudited unary app spec; exact registry/signatures/required-optional schemas,54 normal parity cases20 and canceled admission/tenant/live/privacy evidence. New schema-valid direct CLI9cases20/race20,59 wave mutations and full gates; existing source hashes match b. [Evidence](44-w224-exit-evidence.md). Settings5/configcenter9 follow; live reload and protected publication remain.
- **Typed config read binding (W2.24b, local delivery pending):** app/config owns seven-required-root output/six-required paths/optional three-map routing/bool env schema and one primary unaudited unary spec. Native wrapper/manual entry removed; selected runtime/env-presence adapters and canonical env list remain.54 byte-exact normal cases20, canceled admission unchanged before-fail/after20,31 valid mutations/49 total and full Go/race gates;239 packages136 imports13 calls,structure130. Config exit then settings/configcenter; live reload/writes/protected delivery remain.
- **Config read presentation foundation (W2.24a, local delivery pending):** app/config selected live Reader/optional RoutingReader and bool-only env-presence port own original map projection. Canonical daemon env inventory and manual primary read policy stay native.108 byte-exact cases20 including54 canceled, actual tenant/live refresh/noaudit/privacy/source/port contracts and18 mutations/full Go/race gates.239 packages136 imports13 calls,structure130. Typed binding/exit then settings/configcenter; writes/reload/protected delivery remain.
- **Plugin inventory native exit (W2.23c, local delivery pending):** one primary ReadOnly unaudited unary typed app spec; exact registry/signatures/schemas,45 normal parity cases20 and canceled admission/tenant evidence. New direct CLI8cases20/race20,44 wave mutations and full gates; production hashes match b. [Evidence](43-w223-exit-evidence.md). Order6 follows; live deployment/pin/enforcement and protected delivery remain.
- **Typed plugin inventory binding (W2.23b, local delivery pending):** one app/plugins primary aggregate unaudited unary spec owns six-required-field rows/two-root output and native metadata. Wrapper/manual entry removed; selected manifest adapter remains.45 byte-exact normal native cases20, explicit canceled admission before-fail/after20,26 final mutations/38 total and full Go/race gates.238 packages136 imports13 calls,structure129. One-op exit then order6; live process/pin/enforcement unverified.
- **Plugin inventory service foundation (W2.23a, local delivery pending):** app/plugins manifest Reader/Registration/Service owns map rows and original prefix sort; native field adapter/manual policy remain.50 byte-exact native cases20, actual tenant denial and12 mutations/full Go/race gates;238 packages136 imports13 calls,structure129. Null/empty array/raw/count/pin/copy semantics stay; typed binding/exit next, no live plugin/pin/permission claim.
- **Market eight-operation native exit (W2.22e, local delivery pending):** exact eight typed signatures/policies/registry and all actual tenant denials; CLI stream repair shown red on old Call then16+16+2 direct cases20/race20.12 final exit mutations/157 wave total; full Go/build/vet/native/core/race/static/2787-file format/doc/secrets gates.237 packages136 imports13 calls,structure128. [Exit evidence](42-w222-exit-evidence.md); plugin inventory/protected delivery/broader work follow.
- **Market typed writer/stream binding (W2.22d, local delivery pending):** five mandatory primary specs, typed terminal/progress and error-aware publication/context core APIs;17 unchanged native old-fail/after20 audit/cancel/identity/first-write cases, core phase/cause/legacy/no-rollback tests.240 parity20 allow only asserted correlation/integer changes;41 final mutations/145 wave total and full source/race/Go gates. Native market.go/registration/helpers removed; common arg helpers remain in args.go. Official writer removes CP→market only:237 packages136 imports13 calls,structure128. Eight-op exit then plugin/protected delivery follow.
- **Market writer/stream service foundation (W2.22c, local delivery pending):** app/market Writes owns five manager calls and terminal/domain/partial presentation via selected writer/best-effort publication. Native codecs/manual metadata/progress framing and core ownership remain;255 byte-exact frame/state/effect/fetch/domain cases20, five tenant denials/unavailable audit pairs and35 mutations/104 wave total; source/race/full gates.237 packages137 imports13 calls,structure128. Typed bindings/audit/cancel/stream refinements then exit/plugin follow.
- **Market typed native reads (W2.22b, local delivery pending):** three primary aggregate unaudited unary app specs, typed full rows/manifest/review/conditional summaries replace wrappers/manual read entries/float projection. Five integer-field/three canceled native before-fail/after20 plus owned collections/root-tools repair;342 cases20=219 exact+123 numeric-only,45 mutations/69 wave total/full Go/race gates.237 packages137 imports13 calls,structure128. Five writers/streams then exit/plugin/protected delivery follow.
- **Market read presentation foundation (W2.22a, local delivery pending):** new app/market Reader/Reads owns list/show/sources; core manager and native codecs/manual policies/five writes stay.360 byte-exact native cases20 including18 canceled legacy reads, native no-effect/availability/three tenant denials and24 mutations; source/race/full Go gates.237 packages137 imports13 calls,structure128. Typed read models/registrations then writes/streams/exit/plugin follow.
- **MCP six-operation native exit (W2.21e, local delivery pending):** exact typed registry/signatures/read-write/unary/primary coverage and actual six tenant socket denials;12 exit mutations/98 wave total. Entire production Go path/hash matches d; native/source20/race20/whole CP race1/static/ratchets/2771-file formatting and full d gates retained.236 packages137 imports13 calls. [Exit evidence](41-w221-exit-evidence.md); market/plugin and protected delivery/broader transports follow.
- **MCP typed native lifecycle bindings (W2.21d, local delivery pending):** five primary mandatory-audit unary specs and raw codecs/typed outputs replace wrappers/manual entries/mcp.go.16 unchanged native before-fail/after20 audit/cancel/correlation/context proofs plus direct-service/blocking-dial/canonical/root/codec/tenant/settlement regressions,345 dispatcher parity cases20 and31 mutations. Official writer removes CP→mcp only:236 packages137 imports13 calls; full Go/race gates. Six-operation native exit then remaining domains/protected delivery follow.
- **MCP lifecycle service foundation (W2.21c, local delivery pending):** app/tools owns five writer calls and typed public lifecycle outputs; native codecs/entries keep original correlation/context behavior.345 owned native response/state/attachment/close comparisons20 (only assigned identity/times normalized), port/error/precision/privacy tests and12 mutations/full Go/race gates. Typed lifecycle audit/cancel/context bindings and MCP exit follow.
- **MCP typed native list/redacted row (W2.21b, local delivery pending):** one primary aggregate unaudited unary app spec and16-field/seven-required DTO exclude private value fields. Owned collections and pointer live0/detached count, raw/sorted keys and snapshot cadence persist.63 native cases20 (54 exact+9 only timestamp corrections), actual before/after20 precision/cancel proofs, canonical schema/context/noaudit/tenant and27 mutations/full Go/race gates;236 packages138 imports13 calls,structure127. Native list wrapper/manual entry/row maps removed; five lifecycle adapters share typed view. Lifecycle/exit then wider work/protected delivery follow.
- **MCP catalog/redacted-view foundation (W2.21a, local delivery pending):** app/tools selected List/MCPAttached owns aggregation and original shared view; five lifecycle adapters reuse it.54 exact native cases20/16 valid mutations/port redaction-key-optional-status-cadence and actual owned/mock-peer/tenant/source/race/full Go gates;236 packages138 imports13 calls,structure127. Native read registry/framing/lifecycle codecs and legacy map numeric projection stay for typed list/row binding, then lifecycle/exit/market-plugin; no live peer claim, protected delivery remains gated.
- **Toolbox native exit (W2.20e, local delivery pending):** all three typed native policies/registry/codecs/schemas pinned.83 valid mutations; final22 read+15 normal install frame cases20 byte-exact/source/AppHost/CLI/related and Pulse-event race/static exit gates, exact d bytes retained; d full Go/controlplane race1 evidence. Generic dispatch9937/8613/9670 ns/op (<50us),236 packages138 imports13 calls,structure127. [Exit evidence](40-w220-exit-evidence.md); MCP one read/five lifecycle operations next; live installs/HTTP surfaces/wider work/protected delivery remain.
- **Toolbox typed StreamEvents install (W2.20d, local delivery pending):** all three app specs derive native registry/policies; install owns raw names/mandatory audit/three arrays/checked event payload.15 exact normal native cases20/four native before failures-after20/18 final serial mutations; audit/cancel gates, stream failure stop, owned lifecycle correlation and post-admission publication error. Shared event.WireSchema owns original Pulse envelope literal; install derives payload schema. Source/Pulse-event/tenant/canonical/race/full Go gates;236 packages138 imports13 calls,structure127. Host adapters/shared market-ACP JSON helpers remain; exit/order5 and protected delivery follow.
- **Toolbox install service foundation (W2.20c, local delivery pending):** selected host installer/app orchestration owns sequential order/outcomes/progress and typed arrays; lower catalog/manager execution remains.18 exact native unknown-name frame cases20/27 valid mutations/mock port/error/context/payload and actual native progress/journal/name-codec/tenant/source/race/full Go gates;236 packages138 imports13 calls,structure127. New service errors stop later effects; native callbacks remain best effort/empty identity with existing StreamEvents registry until binding. No real installation claim; typed stream/exit/order5 and protected delivery follow.
- **Toolbox typed native reads (W2.20b, local delivery pending):** two primary aggregate unaudited unary specs derive native registry/empty input/inventory-outdated schemas. Two wrappers/manual read entries removed; host adapter/shared framing and native install remain.22 exact native empty-PATH/CWD cases20/24 valid mutations/required-optional-nil-empty/noaudit/factory-context/tenant tests. Pre-canceled calls now reject before lookup (unchanged before/after20); direct/in-progress best-effort probes remain. Source/AppHost/CLI/race/full Go gates;236 packages138 imports13 calls,structure127. Install service/stream binding then exit/order5 and protected delivery follow.
- **Toolbox read foundation (W2.20a, local delivery pending):** selected host reader/app views preserve inventory fields/nil-empty/optional/counts and fresh raw outdated-key membership/legacy map order/context.22 exact native empty-PATH/CWD cases20/10 valid mutations/port and actual tenant denial/source/race/full Go gates;236 packages138 imports13 calls,structure127. Native read registry/framing/map projection, streaming install and shared JSON helpers remain for later binding/domain steps; no live host manager/installation claim. Protected delivery gate remains open.
- **Toolforge native exit (W2.19e, local delivery pending):** eight typed primary unary specs own policies/registry/codecs/schemas.129 valid mutations; final152 read cases20 (131 exact+21 timestamp-only changes),119 exact mutation-error/absent-remove cases20/source/AppHost/CLI/race/static exit gates, exact d bytes retained. d full Go/controlplane race1 and official bypass debt reduction;236 packages138 imports13 calls,structure127. Generic dispatch8587/8775/6950 ns/op (<50us). [Exit evidence](39-w219-exit-evidence.md); toolbox then remaining order5/wider work/protected delivery follow.
- **Toolforge typed native mutations (W2.19d, local delivery pending):** all eight specs derive registry/primary aggregate/unary policies and schemas; six mandatory writes own raw codecs/typed roots.237-line wrapper/manual registry file removed.117 exact error/absent-remove cases20/31 valid binding mutations;19 actual before failures/after20 prove audit/cancel gates, single operation-domain correlation and inherited runner context. Deadline leaves test record unchanged. Canonical codec/schema/audit/factory/tenant/cancel/settlement/source-success/race/full Go gates;236 packages138 imports13 calls,structure127,120 wave mutations. Settlement does not roll back an applied writer; eight-op exit then wider work/protected delivery follow.
- **Toolforge lifecycle service foundation (W2.19c, local delivery pending):** selected kernel writer port/app service owns six method orchestration, typed outputs and unchanged mutable edit/error rules. Native codecs/manual mutation registry/audit/empty lifecycle correlation/background sandbox context remain until binding.117 exact native error/absent-remove cases20/42 valid mutations/port and owned store-journal-six-event/context/real native sample-tenant/source-success/race/full Go gates;236 packages139 imports13 calls,structure127. Typed lifecycle bindings then exit/order5 and protected delivery remain.
- **Toolforge typed read operations (W2.19b, local delivery pending):** primary aggregate unaudited list/show specs derive native registration/ref codec and11-field light/12-field detail schemas. Typed projection replaces maps and is shared by six retained lifecycle adapters.152 cases20:131 byte-exact+21 asserted timestamp-only corrections; native before rounded large integers/after exact, canceled admission before returned results/after blocks factory/store.28 valid mutations/canonical schema-codec-noaudit/tenant/socket/repeated/race/full Go gates;236 packages139 imports13 calls,structure127. Six lifecycle services/bindings/exit next; broader work/client numeric decoding/protected delivery remain.
- **Toolforge read foundation (W2.19a, local delivery pending):** app/tools selected List/Get owns list/show, shared view moved verbatim and reused by six lifecycle adapters.152 exact native cases20/19 valid mutations/port and actual primary/tenant-denial/no-audit/repeated/race/full Go gates;236 packages139 imports13 calls,structure127. Legacy row-map numeric conversion retained. Read binding/schema then six lifecycle mutations/exit/order5 and protected delivery remain.
- **Three tool reads native exit (W2.18e, local delivery pending):** primary aggregate inventory and two caller-tenant journal reads derive native registry/codec/typed schema.99 valid mutations;50 inventory+192 permission and120 log/stat final native parity cases20/source/AppHost/CLI/race/static gates; exact d bytes restored, d full Go/build/vet/controlplane race1 and cleanup revalidation retained. Generic dispatch6318/6730/8051 ns/op (<50us),236 packages139 imports13 calls,structure127. [Exit evidence](38-w218-exit-evidence.md); toolforge next, wider work and protected delivery remain.
- **Tool observations typed tenant operations (W2.18d, local delivery pending):** log/stat OwnTenant/CallerTenant unaudited unary app specs derive native registry/codec/clock/schema; typed log/by-tool/latency rows replace maps. Shared ProjectValues/map compatibility path uses one engine with old stamp-after-cutoff semantics. Wrappers/manual entries removed; caller journal/dispatch/framing remain.120 exact native ingress cases20/34 valid mutations/canonical codec-schema-clock/admission/typed-map/actual tenant/source-run-plan/race/full Go gates. Pre-canceled calls now reject before lookup (unchanged before/after); direct/in-progress reads stay.236 packages139 imports13 calls/structure127. Three-read native exit then remaining order5 and broader work/protected delivery remain.
- **Tool journal observation foundation (W2.18c, local delivery pending):** app/tools selected Reader owns log/stat tuple joins/filter/provenance/latency/buckets, existing journalview owns paging/order/window; payload decoders/previews move verbatim. Shared pure summary/percentile code/tests move to journalview and run/plan/tool callers use it without a shim. Native codecs/clock/tenant/read registry/framing and row/by-tool/duration maps remain until typed binding/schema.120 exact native cases20/26 valid mutations, port/owned-journal/actual tenant isolation/no-audit/source/run-plan/race/full Go gates;236 packages139 imports13 calls/structure127. Binding/schema/three-op exit then remaining order5 and wider work/protected delivery remain.
- **Tool inventory typed operation (W2.18b, local delivery pending):** one primary aggregate unaudited unary app spec derives native registration and six-string row/tools-count schema; selected kernel definitions retain existing name/key/probe/rollback/duplicate behavior and unknown args. Wrapper/manual registration removed.50 exact native ingress cases192 permission comparisons20, canonical/native required fields/schema/no-audit/tenant tests16 valid mutations/full source/race/Go gates. Pre-canceled admission now blocks factory/reader lookup (unchanged before/after and real native regression); direct/in-progress inventory behavior stays.236 packages139 imports13 calls/structure127. Tool log/stat services and tenant binding/exit then remaining order5 and wider module/invocation/trigger/surface/transport/protected delivery remain.
- **Tool inventory foundation (W2.18a, local delivery pending):** app/tools selected definition reader/typed six-string rows/name sorting/primary representative probes/rollback modes; existing agent permission view shares PrimaryCapability with unchanged policy logic.50 exact native cases192 permission comparisons20, no actual tool/provider/journal effects, read-once/fresh/required-empty/duplicate tests20 valid mutations/source/race/full Go gates. Native inventory wrapper/primary ReadOnly registration/framing, legacy equal-name tie and primary classification stay until later binding/refinement; roster governance remains its own scope.236 packages139 imports13 calls/structure127. Inventory binding/log-stat/exit then remaining order5 and wider module/trigger/surface/transport/protected delivery work remain.
- **Autonomy feed native exit (W2.17d, local delivery pending):** exact primary aggregate unaudited unary operation/23-field schema, native limits and shared exact-number output bridge;44 valid wave mutations/193 final parity cases20/source/AppHost/race/full Go gates. Generic no-audit-I/O dispatch 7098/7304/10239 ns/op (<50us);236 packages139 imports13 calls/structure127. [Exit evidence](37-w217-exit-evidence.md) documents ownership, preserved doctor/limit/presence behavior and explicit canceled-admission/numeric-wire corrections. Order5 tools next; wider autonomous execution/trigger/module/generated/client/other-transport/protected delivery work remains.
- **Native typed numeric precision (W2.17c, local delivery pending):** shared terminal JSON object decoder now uses UseNumber to preserve integer/decimal lexemes; actual socket2^53+1 rounded before, unchanged after proof and permanent2^53+1/maxint64 regressions pass20, removing UseNumber fails. Domain/auth/schema/input/client codecs stay; full source/AppHost/autonomy/CLI/race/Go gates,236 packages139 imports13 calls/structure127. Native exit then order5 follows; generated/client numeric representation and protected delivery remain later work.
- **Autonomy feed typed operation (W2.17b, local delivery pending):** one primary aggregate unaudited unary app spec derives native registration/limit codec and23-field row/schema. Required identity/count and optional raw doctor strings/chain/independent numeric pointers preserve empty/zero/absent wire semantics; old wrapper/manual registry removed.193 exact native ingress parity cases20, canonical/native/schema/no-audit/actual tenant tests23 valid mutations/full source/race/Go gates. Already-canceled operation now rejects before feed lookup (unchanged before/after proof); direct/in-progress Tail behavior stays.236 packages139 imports13 calls/structure127. Exit then order5 and broader autonomy execution/trigger/module/surface/transport/protected delivery remain.
- **Autonomy feed foundation (W2.17a, local delivery pending):** selected Tail port/app service owns recent2000/newest curated milestone/doctor rendering and1–200 clamping; original helpers/private tests move verbatim. Native default60/raw numeric limit codec/primary ReadOnly registry/framing and row maps stay until typed binding/explicit row schema.193 exact native response/journal-head/provider cases20 and18 valid mutations, actual journal/port/source/race/full Go gates pass;236 packages139 imports13 calls/structure127. Binding/exit then order5 and wider autonomous execution/trigger/module/surface/transport/protected delivery remain.
- **Pulse native exit (W2.16f, local delivery pending):**14 exact primary aggregate app operations (three reads11 mutations), explicit canonical StreamLive/native event-only single-reader projection;91 valid wave mutations, omission tests/source/repeated/race/full Go gates and generic no-audit-I/O dispatch6.66–7.00us (<50us). [Exit evidence](36-w216-exit-evidence.md) documents preserved native/replay/drop/settings behavior, mandatory audit/tenant denial and explicit pre-canceled admission refinement.235 packages139 imports13 calls/structure126. Autonomy in order4 next; broader resident run/trigger/module/surface/transport/protected delivery work remains.
- **Pulse subscription typed operation (W2.16e, local delivery pending):**14 commands now have typed primary-only aggregate app policy;3 unaudited reads11 mandatory mutations. pulse_subscribe canonical StreamLive/Event schema/object terminal has explicit native event-only projection, StreamNone native dispatch/single lazy500ms timeout-tolerant reader, retained socket-owned unlimited replay, no second reader/result. Presence/validation/order native decoder now app-owned; adapter retains selected bus/journal/connection hooks, shared dispatch and framing. Already-canceled admission now rejects before replay (unchanged before/after proof);28 exact native framing modes20, permanent canonical/native/14 tenant tests22 valid mutations/full Go and race gates pass;235 packages139 imports13 calls/structure126. Exit evidence then wider trigger/module/generated surfaces/other transports/local protected delivery remain.
- **Pulse resident typed operations (W2.16d, local delivery pending):**13 control commands use typed primary-only aggregate app specs,2 unaudited reads/11 mandatory mutations. Native codec quirks, raw presence/availability order and applied-value best-effort persistence/setters remain; wrappers/manual registry removed.832 exact native/controller/settings parity cases count=20; failed-audit before/after proof plus actual audit/schema/selection/cancel/tenant tests and20 binding/one ratchet valid mutations; full Go/race gates pass,235 packages139 imports13 calls/structure126. pulse_subscribe StreamLive binding with native event-only compatibility then exit and wider work remain.
- **Pulse live stream foundation (W2.16c, local delivery pending):** typed subscription/replay/emitter/drop/clientGone ports own pre-replay4096 subscribe, bounded/live transition, durable dedup/ephemeral pass-through, live filters, per-stream synthetic drop delta/no-growth and cleanup. Preserve event-only silent clean/live-write end, replay cause/prefix/original subscribe/closed messages,1s ticker; bounded opens no watcher/ticker. Lazy500ms socket disconnect mechanics/admission/framing and bus bridge remain; last replay forward removed.17 exact native parity modes count=20, actual bus/journal and16 valid mutations, repeated/race/full gates pass.235 packages/139 imports/13 calls, structure126.14-command binding/audit/event-only native compatibility and exit/wider work remain.
- **Pulse replay foundation (W2.16b, local delivery pending):** typed selected Journal/filter/checkpoint/emitter service owns journal-order AND-composed inclusive lower/exclusive upper seq/time, subject/kind/correlation and positive-rate context wait. Preserve original frame pointers, nil-empty kind distinction, successful last seq/-1 empty/past-head and original partial write/range causes; unlimited emitter-owned cancellation unchanged.14 exact native parity modes count=20, actual sequence0 journal,12 valid mutations and repeated/race/full gates pass.235 packages/139 imports/13 calls, structure126. Native replay helper only frames; live subscription/dedup/drop/lifetime/admission then14-command binding/audit/exit and wider work remain open.
- **Pulse resident controls foundation (W2.16a, local delivery pending):**14 measured native commands (13 controls+subscribe replay/live); typed controller/observer/settings ports own all13 resident services. Preserve dynamic/disabled/null status/asks, required false/zero results, resolve raw key/approval, live-applied duration/dial/quiet best-effort persistence and independent observer availability/raw path/pct/name/Fields argv/false-result errors.832 exact native call/settings parity combinations count=20, owned unstarted core engine and18 valid mutations, repeated/race/full gates pass.235 packages/139 imports/13 calls, structure126. Public boot aliases/selected factory/native admission/framing/registration remain; stream/binding/audit/exit and wider work follow.
- **Workflow native exit (W2.15h, local delivery pending):** exact13-command app aggregate and [exit evidence](35-w215-exit-evidence.md) close native lifecycle/history/copilot/execution admission; webhook/test_node omission fails exact registry.116 valid wave mutations and full source/native/runtime/policy/panic/audit/tenant/context/identity evidence; final repeated/race/static/architecture gates retain unchanged g full checks. Framework7276/6486/8486 ns/op (<50us);234 packages/139 imports/13 calls, structure125. Pulse resident controls and subscribe/replay stream follow before autonomy; wider run/trigger/module/generated-surface work remains open.
- **Workflow caller context/correlation refinement (W2.15g, local delivery pending):** canceled caller blocks all nine service effects before identity/writer/provider/detach; blocking copilot/run/probe/reply inherit cancellation/value/earlier deadline under existing ceilings. Owned operation identity joins lifecycle/drafted/run/node/output events; direct legacy callers keep explicit/fresh fallback. Detached WithoutCancel preserves caller values and independent15m; unused public entry removed. Before/unchanged after proof, actual nine-command socket joins and canceled60s dispatcher delay within3s,16 valid mutations and repeated/race/full gates pass.234 packages/139 imports/13 calls, structure125. Explicit identity/cancellation semantics replace f legacy behavior; transport disconnect/live-provider guarantees are outside the supplied-context proof. Exit/pulse/autonomy and wider work remain open.
- **Workflow typed native binding/audit (W2.15f, local delivery pending):**13 primary-only unary app specs replace five wrappers/manual registry/write helper; four reads remain unaudited, nine mutations require durable audit before graph/provider/execution effects. Raw input/explicit graph schema keeps type/presence/order/forms/null/raw configs/full nodes/created:false/variants.269 combined old/native vs dispatcher scenarios plus16 graph projections count=20, actual closed-journal/all13 tenant socket/admission/schema/owned app-span coverage,18 valid mutations and repeated/race/full gates pass.234 packages/139 imports/13 calls, structure125. Runtime lifecycle empty correlation and copilot/execution fresh background policy remain for caller context/domain correlation refinement, then exit; selected service/runtime/wake/log+journal bridges and wider work remain.
- **Workflow execution foundation (W2.15e, local delivery pending):** typed manual sync/async, node probe, authenticated reply/async webhook and detached panic services own execution admission/budgets/variants. Retain raw/canonical refs/payload/data, background15m/3m/2m/fresh identities, uniform gate/constant-time check before effects, wake provenance/required null-zero/variant omission/error suffixes and panic cleanup/host log+journal. Native non-bool async=false/decoders/framing/registry and selected governed runtime/wake/panic bridges remain; business helpers/timeouts removed.84 response/event parity scenarios count=20, actual/source runtime and panic contracts,24 valid mutations and repeated/race/full gates pass.234 packages/139 imports/13 calls, structure125.13-command binding/audit/context/correlation refinement/exit and wider work remain open.
- **Workflow copilot foundation (W2.15d, local delivery pending):** typed Draft/Refine/Designer/correlation/base snapshots own orchestration; posted graph wins, absent/null ref resolves before identity/provider. Retain native decoding/framing, fresh correlation and legacy background3m/cancel-on-success/error, raw inputs/full graph/zero original causes/unsaved behavior.44 native response/provider/drafted-event/state parity scenarios count=20 (fresh identity only with joins verified), actual runtime mock and original runtime contracts,14 valid mutations and repeated/race/full gates pass.234 packages/139 imports/13 calls, structure125. Execution/binding/audit/exit and caller context/correlation convergence remain; codec/registry/public runtime boundary retained.
- **Workflow run-history foundation (W2.15c, local delivery pending):** canonical History snapshot plus typed RunRecord/NodeEvent own the selected journal fold; preserve exact subject/correlation, reverse first-seen/20 default/100 cap, zero/null/false required fields, node probe/default/handled/attempts/metadata rules, legacy decode and start/terminal ordering. Journal errors return zero/prefixed cause. Native ref-before-limit and observed nonnumber/string limit=>default policy remain; old business fold/limit constants removed.27-input exact native parity over125 owned arcs count=20, actual journal and18 valid mutations, repeated/race/full gates pass.234 packages/139 imports/13 calls, structure125. Copilot/execution then13-command binding/audit/exit and wider work remain open.
- **Workflow lifecycle foundation (W2.15b, local delivery pending):** typed actual runtime-facade Writer owns Save/Restore/SetEnabled/Remove, preserving posted graph/correlation/reason/raw ref/bool, full/light projections, required created/removed false and original zero failure causes. Enable keeps ErrNotFound/raw-reference message; native JSON/ref/reason/bool-string enable framing and empty correlation remain. Four-handler x eighteen-input response/store/event parity count=20, actual lifecycle identity/checkpoint/correlation/best-effort journal publication coverage, twelve valid mutations and repeated/race/full gates pass.234 packages/139 imports/13 calls, structure125. History/copilot/execution, then typed13-command binding/audit/exit follow; mandatory operation audit and broader run/trigger/module/surface work remain open.
- **Workflow read/projection foundation (W2.15a, local delivery pending):** 13 measured primary-only native commands (four reads/nine mutations); typed graph/journal/gallery ports own list/show/templates and latest-run fold. Preserve store-before-flag admission, opt-in single best-effort journal scan, order/count/annotations, light/full/raw-config/required/optional/empty wire, trigger priority and trimmed lookup/raw error identity. Original six fold tests move with production. Exact three-handler x fourteen-input native plus eight-config x light/full JSON parity count=20, actual owned store/journal, twelve valid mutations and repeated/race/full gates pass. 234 packages/139 imports/13 calls, structure125. Native admission/registry and shared projection bridge remain until lifecycle/history/copilot/execution and typed binding/audit/exit; wider work remains open.
- **Standing native exit (W2.14d, local delivery pending):** exact seven-command aggregate and [exit evidence](34-w214-exit-evidence.md) close native management/history/manual dispatch admission; missing Why/Fire independently fails the registry regression. 35 valid wave mutations, parity/actual runtime facade/store/journal/audit/callback/tenant evidence; final repeated/race/static/architecture gates retain unchanged production full checks from c. Framework dispatch 6355/6785/6483 ns/op (<50us); 233 packages/139 imports/13 calls, structure124. Selected service/public injection bridges remain; resident runner, best-effort read-error repair and wider run/trigger/module/surface work remain open.
- **Standing typed native binding/audit (W2.14c, local delivery pending):** seven primary-only unary app specs replace CRUD/why/fire wrappers and manual registration; list/why remain unaudited, five mutations require actual durable audit before state/callback effects. Typed raw admission keeps absence/null/types/order and observed non-bool enabled=false; derived schema retains embedded order/optional/false/null shapes. Selected runtime facade/callback bridges and public injection remain. Native parity 75 CRUD+48 why/fire count=20, actual closed-journal/tenant socket/latest callback/single audit settlement coverage, twelve mutations and repeated/race/full gates pass. App audit joins owned correlation; runtime lifecycle keeps empty identity. Exit evidence and wider runner/trigger/module/read-error repair remain.
- **Standing why/fire services (W2.14b, local delivery pending):** typed journal/event/callback ports own standing.* prefix/exact payload ID history, required zero fields, journal order and null empty/legacy partial-error results; Fire owns availability before lookup, validation before callback and request ID/false result. Last validator forward removed. Exact two-handler x eight-input x three-callback-mode parity count=20, actual owned journal/recording callback coverage, nine mutations and repeated/race/full gates pass. Required native ID, public callback injection and seven-command registration remain until typed binding/audit/exit. Why Range error repair and wider runner/trigger/module work remain separate.
- **Standing CRUD/projection foundation (W2.14a, local delivery pending):** five typed services over actual core reader/runtime facade and selected agent/message ports preserve core wire/optional omissions, list counts/target annotations/frequency warnings, validation before writes, add raw agent/edit trim/presence/conversions, resume gate/pause bypass, missing results and original causes. Runtime lifecycle journal events and core identity/timestamps/rollback remain. Exact five-handler x fifteen-input parity count=20 (bounded IDs/time), twelve mutations and repeated/race/full gates pass; observed native non-bool enabled inputs stay false. Native order/patch decoding and registration remain until why/fire and typed binding/audit/exit follow. 233 packages/139 imports/13 calls, structure 124.
- **Schedule native exit (W2.13k, local delivery pending):** exact ten-operation common registry, bound types/schemas/primary-tenant/unary/compatibility metadata and no old native business wrappers. Family/tenant-firing removal fails exit coverage. [Evidence](33-w213-exit-evidence.md) records 117 mutations and source/native/store/journal/audit/tenant/parity/failure-boundary proof; final scoped/race/CLI/static/architecture/structure gates pass with unchanged j full-Go/build/vet/controlplane-race evidence. Framework no-I/O dispatch <50 us. 232 packages/139 imports/13 calls, structure 123. Standing/workflow/pulse/autonomy next; resident run/trigger/module/surface work remains open.
- **Schedule typed native operations (W2.13j, local delivery pending):** ten unary app specs derive native metadata: five reads unaudited, five mutations mandatory audited, eight primary-only commands and two tenant-owned/routed firing reads. Typed raw field presence preserves native decode/order/forms/missing-edit short circuit; explicit raw-payload record/list/edit output schemas preserve required/optional shapes. Enable joins host-owned audit correlation. Five old wrapper files and registration/decode/write helpers removed. Ten-command x thirteen-input response/store parity count=20, actual closed-journal/tenant/read/audit/schema proofs, twelve mutations and repeated/race/full gates pass. Native exit evidence follows; resident run/trigger/module work stays open.
- **Schedule firing views (W2.13i, local delivery pending):** Fires/Stats/Latest use typed selected journal/shared-run snapshot ports; native schedule surface totals ten commands with tenant-routed read-only fires/stats. Preserve native codec/cutoff/routing, shared collectRuns data, floor/cap/filter-before-sort/limit/cursor, outcome/duration/spend/preview, payload metadata/required zero fields/optional runbook, stats/window and first same-ms latest tie. Exact three-handler x 27-input parity count=20, real socket tenant join isolation, actual owned journal/presence/error coverage, nineteen mutations and repeated/race/full gates pass. Payload/classifier business files removed; list annotation errors remain best effort. Native binding/audit/exit follow; resident run execution stays W2.2/W4.
- **Schedule edit mutation service (W2.13h, local delivery pending):** typed store/live target/clock Begin/Apply own found-only read/clock state, preflight, field/target/payload/second lookup, late timezone, five reschedules and final projection. Native ID/string/presence decoding retains missing updated:false early return; original causes, ignored model/agent errors/false results, late-error partial changes, workflow canonicalization at second lookup and missing final zero-entry updated:true remain. Actual five-cadence x four-target and phase/cause/false/missing coverage, exact 49-input x four-target native parity count=20, thirteen mutations and repeated/race/full gates pass. View/merge shims removed; atomicity/failure repairs remain separate. Firing then native binding/audit/exit follow.
- **Schedule edit cadence preflight (W2.13g, local delivery pending):** app ValidateEditCadence receives typed parse value/presence/cause through a native codec. Keep lenient/trimmed timezone first, once/continuous/window/daily/interval selection, original selected causes/ignored unselected errors, truncation and strict future/minimum/minute/day/window checks. Existing numeric-type test now traverses codec/app validator; old business helper removed. Exact 49-input x four-target response/store-state parity count=20 (bounded IDs/clock), twelve mutations and repeated/race/full gates pass. Late strict timezone decoding and mutation/partial-update behavior remain native until the next move; firing and binding/audit/exit follow.
- **Schedule edit target admission (W2.13f, local delivery pending):** typed current/requested target, agent and payload presence plan preserves native missing-ID short circuit, strict target strings/lenient agent forms, explicit intent versus absent target, inherited identities/clear/canonical requested-agent binding and original validation order. Workflow preflight retains raw resolved reference for later native lookup; inherited-agent tool checks remain policy-only and unrequested workflow/tool payload stays ignored. Exact 42-input x four-target response/store-state parity count=20 (bounded IDs/clock), eleven mutations and repeated/race/full gates pass. Cadence preflight/mutations and firing then binding/audit/exit remain; existing late-error partial updates/atomicity are unchanged.
- **Schedule creation service (W2.13e, local delivery pending):** five cadence variants over typed actual-store and clock ports own create/agent-target-bind/best-effort compensation/refetch projection. Native selected numeric/timezone parsing follows target admission; lower-bound messages, fractional truncation/core continuous clamp, operator source, exact binding errors, original causes/zero failures and refetch/fallback shapes remain. Forty-input native response/store-entry parity count=20, actual-store five-mode x four-target and failed binding/compensation coverage, eleven mutations and repeated/race/full gates pass. Store atomicity remains separate; edit/firing and binding/audit/exit follow.
- **Schedule add target admission (W2.13d, local delivery pending):** typed target/agent plan over selected workflow-name/agent/tool/message ports. Native argStrings remains first and target validation precedes cadence parsing. Empty intent target, target overrides/conflict order, canonical workflow/agent bindings, payload presence/ignored intent payload and policy/error ordering retain exact 36-input response/store-entry parity count=20 (bounded IDs/clock). Eleven mutations and repeated/race/full gates pass. Five cadence create/bind/rollback/refetch paths stay native until the next move; edit/firing and binding/audit/exit follow.
- **Schedule admission rules (W2.13c, local delivery pending):** runnable validation, agent tool allow/deny and frequency warnings use selected live agent/workflow/tool/message ports in app/schedule. Keep native lookup order, references, exact errors, retired/paused/managed gates, tool re-read, deny precedence/default allow and warning mode/threshold/precedence. Actual-runtime 180 validation/8,640 warning combinations pass legacy parity count=20; ten mutations and repeated/race/full gates pass. Native helpers are bridges/forwards; add/edit admission and firing services then binding/audit/exit remain.
- **Schedule lifecycle foundation (W2.13b, local delivery pending):** remove/run/enable use typed actual cadence, validation and success-only publication ports. Run/resume validate before mutation; pause skips validation. Native required-ID/legacy enabled forms, missing/false outputs, exact causes, operator action payload/correlation and unary framing remain. Three-handler x four-target x ten-input response/store/action parity count=20, eight mutations and repeated/race/full gates pass. Add/edit admission, target validation and firing extraction then native binding/audit/exit remain.
- **Schedule read/projection foundation (W2.13a, local delivery pending):** typed list/catalog/forecast/common metadata with actual cadence reader, selected annotation/validation/warning and clock ports. Exact native/projection parity, repeated/race/full gates and eight mutations pass; preserve required/present fields, forecast shape and best-effort firing annotations. Dead metadata shims removed; add/edit projection bridge remains. 232 packages/139 imports/13 calls, official structure 123. Admission/lifecycle/firing services then binding/audit/exit follow.
- **Storage/artifact native exit (W2.12e, local delivery pending):** exact five-operation aggregate registry and [exit evidence](32-w212-exit-evidence.md) retain diagnostic/admission/error/wire/default/audit/tenant/deletion-failure boundaries. Focused repeated/race/final full gates and two exit mutations pass; twenty-eight native-wave mutations. No-audit-I/O framework dispatch <50 us; unchanged 231 packages/139 imports/13 calls, official structure 122. Schedule follows; broader collection/blob/writer/raw-ref module work remains.
- **Artifact metadata-removal repair (W2.12d, local delivery pending):** real removal failures now preserve entry/blob and return the wrapped cause; native delete emits one error, collection counts only successful removals. Core/native old-code failures, three count=20 verifiers, two mutations and repeated/race/full gates pass. Unchanged 231 packages/139 imports/13 calls, structure 122. ENOENT remains idempotent; batch error redesign/blob cleanup/races/journal raw_ref protection stay outside this narrow fix. Native exit follows.
- **Storage/artifact typed native binding (W2.12c, local delivery pending):** five primary-only app specs replace wrappers/registration; delete/collect require audit before index/blob effects and three reads stay unaudited/unary. Actual legacy closed-journal deletion proof, preservation/tenant/audit/strict presence/default/parity/repeated/race/full gates and eight mutations pass. Unchanged 231 packages/139 imports/13 calls, official structure 122. Host bridges remain; measured collection cleanup/reference repairs then native exit follow.
- **Artifact service foundation (W2.12b, local delivery pending):** typed blob/list/delete/collect over actual store/index/clock ports; native validation/availability/defaults/audit/framing remain. Four-handler x five-input parity, actual cutoff/unknown-time/dedup ownership, repeated/race/full gates and eight mutations pass: 231 packages/139 imports/13 calls, structure 122. Binding/audit then separately proved cleanup/reference repairs/exit follow.
- **Storage inventory foundation (W2.12a, local delivery pending):** typed app/storage owns totals/root/labels/order/probe projection; actual filesystem and selected-base disk probe remain host ports. Exact five-mode native parity/repeated/race/full gates and eight mutations pass; diagnostic best-effort errors and null/zero/presence shapes remain. 230 packages/140 imports/13 calls, official structure 121. Artifact services then binding/audit/exit follow.
- **OKR native exit (W2.11d, local delivery pending):** exact seven-operation aggregate registry and [exit evidence](31-w211-exit-evidence.md) cover live rollup/returned snapshot, audit/tenant/identity/admission and scope. Focused repeated/race/final full gates and two exit mutations pass; twenty-six native-wave mutation proofs. No-audit-I/O framework dispatch <50 us; 229 packages/140 imports/13 calls, official structure 120. Storage/artifacts follow.
- **OKR typed native binding (W2.11c, local delivery pending):** seven primary-only app specs replace wrappers; five mutations require audit before effects and two reads remain unary/unaudited. Actual closed-journal/tenant/identity/admission/parity/repeated/race/full gates and eight mutations pass. Default domain/audit identity joins; explicit bridge remains. Dead OKR/correlation/integer helpers removed; seat list helper retained. 229 packages/140 imports/13 calls, official structure 120. Native exit follows.
- **OKR lifecycle foundation (W2.11b, local delivery pending):** typed Lifecycle owns create/key-result/link/unlink/archive over actual kernel facade and live projection; native admission/correlation/envelope/policy remain. Seven-handler parity/actual journaling/returned-snapshot/repeated/race/full gates and eight mutations pass. Dead view/response bridges removed; unchanged 229 packages/141 imports/13 calls, structure 120. Binding/audit/exit follow.
- **OKR read/projection foundation (W2.11a, local delivery pending):** typed app/okr owns list/show and live Rollup projection over selected store/kernel ports; native admission/registration/error framing and five writers remain. Seven-handler x three-input parity/actual live task rollup/filter/shape/repeated/race/full gates and eight mutations pass. 229 packages, unchanged 141 imports/13 calls, official structure 120. Lifecycle then binding/audit/exit follow.
- **Workboard native exit (W2.10i, local delivery pending):** exact twenty-one-operation aggregate registry and [exit evidence](30-w210-exit-evidence.md) cover types/schemas/scope, audit/tenant/identity/execution and watch error boundaries. Focused repeated/race/final full gates and two exit mutations pass; sixty wave mutation proofs total. No-audit-I/O framework dispatch <50 us; unchanged 228 packages/141 imports/13 calls, structure 119. Actual shared OKR/seat/context bridge callers remain; OKR/storage/artifacts follow.
- **Workboard watch read-error repair (W2.10h, local delivery pending):** actual empty/partial JSONL parse failure and dependency read cause now return zero snapshot and one native error frame, rather than successful incomplete data. Original service fails permanent owned-journal/native/sentinel regressions; three verifiers count=20, two error mutations/repeated/race/full gates pass. Unchanged 228 packages/141 imports/13 calls, structure 119. Native exit then OKR follow.
- **Workboard typed native binding (W2.10g, local delivery pending):** twenty-one primary-only app specs replace wrappers/registration; seventeen mutations require audit before effects and four reads stay unaudited/unary. Actual closed-journal/tenant/shape/admission/identity/async dispatch/repeated/parity/race/full gates pass: unchanged 228 packages/141 imports/13 calls, structure 119. Default domain/audit correlation joins; explicit inbound bridge and fresh dispatch run correlation remain. Shared OKR/seat helpers persist for their callers; watch read-error repair/native exit follow.
- **Workboard background-execution foundation (W2.10f):** typed Execution owns seat precedence/degradation, failure/reclaim/recursion and owned-claim proof/review settlement. The selected native bridge retains actual full profile/wake/cost/model/tool/retry and execution-profile APIs. Eight-case actual store/mock native parity count=20 with generated ID/clock normalization, eight mutations/focused repeated/race/full gates pass: unchanged 228 packages/141 imports/13 calls, structure 119. Twenty-one typed native bindings/audit/read-error repairs/exit follow; runs.Start convergence stays open.
- **Workboard dispatch-admission foundation (W2.10e):** typed Dispatch owns dependency/agent checks, fresh correlation/claim/link, requested publication and callback launch over actual store/host ports. Native lenient admission/envelope and background runner/profile bridge remain. Denied native parity/intent/workboard/CLI/focused native/race count=20, complete controlplane race count=1 and eight mutations/full gates pass: unchanged 228 packages/141 imports/13 calls, structure 119. Background service then binding/audit/exit follow.
- **Workboard watch-service foundation (W2.10d):** typed Watch owns selected run/event/dependency snapshot; native lenient admission/read-only/unary framing remain. Exact native parity/source/CLI/package-race count=20, eight mutations/full gates pass: unchanged 228 packages/141 imports/13 calls, structure 119. Best-effort read errors are retained until a measured repair; dispatch/background services and binding/exit follow.
- **Workboard relation/maintenance foundation (W2.10c):** typed Relations owns link/policy/depend/reclaim/sweep over actual kernel facade; native admission/correlation/defaults/cap/errors remain. Five-handler native parity x three inputs/source/CLI/package-race count=20, eight mutations/full gates pass: unchanged 228 packages/141 imports/13 calls, structure 119. Dispatch/watch and binding/exit follow.
- **Workboard lifecycle-service foundation (W2.10b):** typed app/workboard owns eleven create/lifecycle methods over actual kernel facade/seat setter; native admission/seat lookup/correlation/prove timeout/error envelope remain. Eleven-handler native parity x three inputs/source/CLI/package-race count=20, eight mutations/full gates pass: unchanged 228 packages/141 imports/13 calls, structure 119. Relations/maintenance/dispatch/watch and binding/exit follow.
- **Workboard read/projection foundation (W2.10a):** typed app/workboard owns list/lanes/show and actual-task projection plus counters/conditional proof/retry fields. Native wrappers/forwarder retain admission/policy/framing and remaining lifecycle/dispatch outputs. Three-handler native/shared-projection JSON/source/CLI/package-race count=20, eight mutations/full gates pass: 228 packages, unchanged 141 imports/13 calls, structure 119. Remaining workboard services and binding/exit follow.
- **Board native exit (W2.9c):** exact seven-operation registry guards schemas/types/native metadata; related/source/tenant/audit/fallback/notifier/exit count=20, two mutations/final full gates pass. No-audit-I/O dispatch <50 us; unchanged 227 packages/141 imports/13 calls, structure 118. [Exit evidence](29-w29-exit-evidence.md) records shared-store and explicit notifier correlation boundaries; workboard follows.
- **Board typed native binding (W2.9b):** seven primary-only specs use selected host factories; send/ack require audit before store/notifier effects and five reads remain unaudited. Old wrappers/registration/limit helper removed. Native parity/closed-journal/fallback/tenant/source/race count=20, eight mutations/full gates pass; unchanged 227 packages/141 imports/13 calls, structure 118. Explicit inbound notifier correlation is retained; native exit/workboard follow.
- **Board-service foundation (W2.9a):** typed app/board owns seven use cases and faithful message projection; wrappers retain shared-writer/fresh-reader selection, native admission/auth/audit/framing. Seven-handler parity/source/package-race count=20, ten mutations/full gates pass: 227 packages, unchanged 141 imports/13 calls, structure 118. Cursor/routing/ack/notifier behavior retained; binding/audit/exit and workboard follow.
- **Skill native exit (W2.8h):** exact fourteen-operation common-registry coverage guards schemas/types/native metadata. Related/source/native/tenant/audit/history/identity/exit/CLI count=20, two exit mutations and final full gates pass; no-audit-I/O dispatch <50 us, unchanged 226 packages/141 imports/13 calls, structure 117. [Exit evidence](28-w28-exit-evidence.md) records boundaries; board/workboard/OKR/storage follow.
- **Skill identity/ownership-history repair (W2.8g):** eight mutations forward trusted operation correlation; history includes shared/reassigned events and preserves event provenance/order/read-error handling. Eight direct/native ordered arcs, actual history/source/package-race/CLI count=20, ten mutations/full gates pass; unchanged 226 packages/141 imports/13 calls, structure 117. Context-free callers retain empty correlation; native exit follows.
- **Skill history read-error repair (W2.8f):** real owned corrupt journals reproduced successful empty/partial history. Original Range errors now propagate and failed output is discarded. Native/service/source/race count=20, three mutations and full gates pass; unchanged 226 packages/141 imports/13 calls, structure 117. Identity/ownership-history coverage and exit follow.
- **Skill typed native binding (W2.8e):** fourteen specs derive primary-only metadata; mandatory audit stops eight mutations before effects, six reads remain unaudited. Old wrappers/registrations removed. Actual closed-journal/tenant/schema/effect/native/source/race count=20, 14-binding parity x three inputs count=20, eight mutations/full gates pass; unchanged 226 packages/141 imports/13 calls, structure 117. Roster teardown still owns CP->skill debt; history/identity repairs and exit follow.
- **Skill observation-service foundation (W2.8d):** typed app/skill owns history/files/read_file/hygiene; native admission/auth/audit/framing remain and unused old helpers are removed. Exact four-handler native parity/source/package-race count=20, eight mutations/full gates pass; unchanged 226 packages/141 imports/13 calls, structure 117. Best-effort history Range error/kind behavior is explicitly retained until a measured repair. Binding/identity/exit follow.
- **Skill curation-service foundation (W2.8c):** typed app/skill owns share/reassign/import and caller-selected roster admission. Native admission/auth/audit/framing remain. Exact three-handler native parity/source/package-race count=20, eight mutations/full gates pass; unchanged 226 packages/141 imports/13 calls, structure 117. History/files/hygiene and binding/identity/exit follow.
- **Skill lifecycle-service foundation (W2.8b):** typed app/skill owns promote/quarantine/archive/revert/restore; native admission/auth/audit/framing remain. Five-handler native parity x three states x six inputs/source/package-race count=20, eight mutations/full gates pass; unchanged 226 packages/141 imports/13 calls, structure 117. Ownership/import/history/files and binding/identity/exit follow.
- **Skill read-service foundation (W2.8a):** app/skill owns typed list/get and exact native projection; wrappers retain admission/auth/audit/framing. Two-handler native parity/source/registry/tenant/audit/package-race count=20, eight mutations and full gates pass: 226 packages, unchanged 141 imports/13 calls, structure 117. Remaining skill services/binding/exit follow.
- **Taste native exit (W2.7c):** exact three-operation common registry and actual schema/native metadata regression; related/source/tenant/audit/exit count=20, two exit mutations and full gates pass. No-audit-I/O dispatch <50 us; unchanged 225 packages/141 imports/13 calls, structure 116. [Exit evidence](27-w27-exit-evidence.md) records boundaries. Skill follows; broader migration remains open.
- **Taste typed native binding (W2.7b):** three operations derive primary-only metadata; mandatory audit stops create/delete before effects, while list remains unaudited. Wrappers/registrations removed; lenient admission and actual output schemas retained. Native parity/source/registry/tenant/audit/package-race count=20, eight mutations and full gates pass; unchanged 225 packages/141 imports/13 calls, structure 116. Exit evidence then skill follow.
- **Taste-service foundation (W2.7a):** app/taste owns typed list/create/delete using actual exemplar fields; native lenient admission/registration/auth/audit remain. Native three-handler parity count=20 documents fresh ID/create-clock boundaries. Source/service/store/registry/tenant/audit/package-race suites count=20 plus eight mutations and full gates pass. Paid CP->taste edge officially removed: 225 packages, 141 imports/13 calls, structure 116. Typed binding and exit evidence remain open.
- **World native exit (W2.6e):** complete nine-operation common-registry coverage guards schemas/types/native metadata. Related/source/native/tenant/audit/identity contracts count=20, two exit mutations and final full gates pass; no-I/O dispatch 10.5–11.7 us (<50 us), unchanged 224 packages/142 imports/13 calls. [Exit evidence](26-w26-exit-evidence.md) records boundaries. Taste/skill follow; broader adapters/generated surfaces/W3–W5 remain open.
- **World operation/domain identity repair (W2.6d):** actual native add proved blank graph event correlation beside an admitted audit identity. Four mutations now forward context identity, including relation-created endpoints; context-free calls keep empty legacy correlation. Native ordered arc and four real store/bus/journal paths plus source/tenant/race contracts count=20, four mutations/full gates pass: unchanged 224 packages, 142 imports/13 calls, structure 115. World exit evidence then taste/skill follow.
- **World typed native binding (W2.6c):** controlled legacy binding mutated graph/returned success with a closed journal. Nine app specs/common native adapters require audit for four mutations, leave five reads unaudited and route only world_log to tenant journals. Old wrappers/registrations and unused argStringMap are removed. Actual audit/tenant/schema/effect/source/race contracts count=20, nine mutations, nine-binding × three-input parity and final full gates pass; score-clock/generic-failure-code boundaries recorded. Unchanged 224 packages/142 imports/13 calls, structure 115. Domain identity refinement and exit evidence remain open.
- **World journal-service foundation (W2.6b):** app/world.LogService owns entity/relation/forget folds, labels and typed rows via journalview; CP retains kind/lenient projection admission and tenant-selected reader. Exact thirteen-argument native JSON parity and source/log/graph/registry/tenant/audit/package-race tests count=20 plus ten mutations/full gates pass: unchanged 224 packages, 142 imports/13 calls, structure 115. Typed binding, identity refinement and native exit evidence remain open.
- **World graph-service foundation (W2.6a):** app/world owns eight typed graph business/projection methods; CP retains native admission/framing/registry/auth/audit. Content identity, replacement, direction, quiet resolve, admitted fractional-limit behavior and wire/lifecycle fields are guarded. Eight-handler × eight-argument native parity count=20 allows bounded score clock drift. Source/graph/registry/tenant/audit/package-race tests count=20, twelve mutations/full gates pass. Officially paid CP->worldmodel edge: 224 packages, 142 imports/13 calls, structure 115. Journal fold/typed binding remain open.
- **Memory native exit (W2.5j):** complete 16-operation common-registry coverage guards actual schemas/types and derived native flags. Source/typed/native/tenant/audit/identity/context contracts count=20, two exit mutations and full gates pass; dispatch excluding audit I/O is 5.7–7.2 us (<50 us), unchanged 223 packages/143 imports/13 calls. [Exit evidence](25-w25-exit-evidence.md) records boundaries. World/taste/skill follow; broader adapters, generated surfaces and W3–W5 remain open.
- **Distillation caller-context repair (W2.5i):** controlled ports confirmed retained waits, lost deadline/values and pre-canceled effects. Both paths admit cancellation before identity/effects and derive their owned five-minute ceiling from caller context, retaining cleanup/admitted identity and background fallback behavior. Direct/typed context/report/identity/source/tenant/race contracts count=20 plus six mutations/full gates pass: unchanged 223 packages, 143 imports/13 calls, structure 114. Memory native exit evidence follows; live-provider/broader-adapter/generated/W3–W5 scope remains open.
- **Memory operation/domain identity repair (W2.5h):** actual native add proved missing domain correlation; a controlled port proved distillation replaced admitted identity. Eight store mutation paths and both distillation paths retain the operation correlation, with legacy empty/fresh identity only when none is supplied. Ordered native audit/domain arc, real store/bus/journal and controlled-port/source/tenant/race contracts count=20 plus ten mutations/full gates pass: unchanged 223 packages, 143 imports/13 calls, structure 114. Distillation caller context and exit evidence remain open.
- **Memory typed native binding (W2.5g):** controlled legacy binding reproduced record mutations/success with a closed journal. All 16 memory/profile specs use the common host; ten mutations require audit before services, six reads stay unaudited and audit/clean/log retain routed tenant scope. Old wrappers/registrations and unreachable jsonMap/Server.ok helpers are removed. Native/schema/effect/tenant/source/race contracts count=20, eleven mutations, 16-binding × three-input parity and full gates pass; clock/fresh-identity/generic-failure-code boundaries are recorded. Unchanged 223 packages, 143 imports/13 calls, structure 114. Caller-context/domain-correlation refinement and exit evidence remain open.
- **Memory journal-service foundation (W2.5f):** app/memory.LogService owns event folds/aliases/typed rows through journalview; CP retains lenient projection admission and tenant-selected journal. Lifecycle IDs/subjects, cursor/cutoff, field presence, empty arrays and next_cursor retain behavior. Exact thirteen-argument native JSON parity and source/log/registry/tenant/audit/package-race tests count=20 plus eight mutations/full gates pass: unchanged 223 packages, 143 imports/13 calls, structure 114. Business foundation covers all 16 memory/profile commands; typed binding, mandatory audit, wrapper deletion and context/correlation refinement remain open.
- **Memory list-service foundation (W2.5e):** app/memory prepares active records before native admission and owns typed projection/paging; duplicate CP projection is removed. Defaults/cap/fractional limits, timestamp/ID ties, strict cursor filtering, pre-filter total, terminal cursor omission, empty arrays and independent pages retain behavior. Exact twelve-argument native JSON parity and source/list/registry/tenant/audit/package-race tests count=20 plus eleven mutations/full gates pass: unchanged 223 packages, 143 imports/13 calls, structure 114. Log, typed binding and context/correlation refinement remain open.
- **Memory distillation-service foundation (W2.5d):** app/memory.Distillation owns consolidate/profile-rebuild behind the runtime Distiller port; native wrappers encode typed reports. Fresh identity, five-minute background ceiling, owned cancellation, present null/zero fields and active_after naming retain source behavior. Original/current running/halted parity, actual socket/fake-port tests count=20, nine mutations and full gates pass: unchanged 223 packages, 143 imports/13 calls, structure 114. List/log and typed binding follow; caller cancellation/shared operation identity are separate measured refinement.
- **Memory hygiene-service foundation (W2.5c):** app/memory owns prune/tidy/audit/clean business; native nil/tenant/day/dry-run admission and registration remain. Thirty-day default, age/dry-run safety, pre-prune stats, present-zero counts, curated retention and cause propagation are guarded. Four-handler × eight-argument parity count=20 normalizes bounded cutoff clock and existing unordered contradiction members. Source/hygiene/registry/tenant/audit/package-race tests count=20, nine mutations and full gates pass: unchanged 223 packages, 143 imports/13 calls and structure 114. List/log/distillation and typed binding remain open.
- **Memory curation-service foundation (W2.5b):** app/memory owns five store mutation bodies, operator/source conversion and typed native output; CP retains argument admission/registry/audit. Same-ID revision, present empty promotion subject, repeated/missing batch counts, 500-ID bound and partial stop-on-error effects retain behavior. Exact five-handler × ten-argument native JSON parity and source/curation/tenant/registry/audit/package-race tests count=20 plus ten mutations pass. Full gates: unchanged 223 packages, 143 imports/13 calls, structure 114. Remaining memory business and typed binding stay open.
- **Memory read-service foundation (W2.5a):** app/memory owns get/search/find-related store business and typed wire projection; CP keeps native required/type admission and framing. Original/current three-handler × nine-argument native parity count=20 retains records/fields/errors with bounded ranking clock drift. Source/read/registry/tenant/audit and new-package race suites count=20 plus eight mutations pass. Full gates: 223 packages, unchanged 143 imports/13 calls; official kernel structure 114. Remaining list/log/write/hygiene/distillation and typed binding remain open.
- **Catalog/provider native exit (W2.4v):** all 17 operations use typed app specs/common native binding; aggregate coverage guards readonly/tenant metadata and old business handlers are removed. Source/native/tenant/audit/lifetime/context suites count=20 and full gates pass with unchanged 222 packages, 143 imports/13 calls. Dispatch excluding audit I/O is 6.4–6.9 us (<50 us). [Exit evidence](24-w24-exit-evidence.md) records boundaries; memory/world/taste/skill follow. Broader adapters, live provider/browser checks and W3–W5 remain open.
- **Probe caller-context repair (W2.4u):** controlled typed-dispatch/body fixtures confirmed cancellation left background wait and reported body success. Context-aware transport/service/port preserves caller context, guarded posture and ceiling; canceled body returns its cause. Legacy background/best-effort entries remain live. Context/source/native/focused race tests count=20 and four mutations pass. Full gates: unchanged 222 packages and 143 import/13 call ratchets, without new dead-code exceptions. Provider exit evidence remains next.
- **Current-session persistence admission (W2.4t):** controlled delayed fetch proved old callback restored tokens after logout. Persistence and logout now serialize against current login identity/state/context/stop; callback business checks admission before and after exchange. Constructor-bound fake fetch exercises the production flow. Permanent delayed/current/retired/canceled and source/native/race tests count=20 plus five mutations pass. Full gates: unchanged 222 packages and 143 import/13 call ratchets. Caller-context refinement and provider exit evidence remain open.
- **Token fetch/persist foundation (W2.4s):** Manager.ExchangeTokens returns the raw candidate without manager/vault writes. Legacy ExchangeCode and app callback fetch then persist in the same order. Isolated form/field/no-write/error and original source/native tests count=20 plus four mutations pass. Full gates: unchanged 222 packages and 143 import/13 call ratchets. Current-session persistence admission and caller context remain open.
- **Expiry worker ownership repair (W2.4r):** owned Start/Stop goroutine proof confirmed retired logins retained their TTL sleep worker. Stop/done channels and idempotent cancellation release timers; expiry checks current login and cancellation before terminal state. Permanent wiring/lifecycle/TTL and source/native/race tests count=20 plus four mutations pass. Full gates: unchanged 222 packages and 143 import/13 call ratchets. In-flight token/session persistence fencing and caller context remain open.
- **Successful logout callback ownership repair (W2.4q):** controlled fixture confirmed token logout kept pending login/port. Logout now stops the login after successful vault cleanup and before reload; vault failure retains existing ownership/error behavior. Permanent/source/native and callback race tests count=20 plus two mutations pass. Full gates: unchanged 222 packages and 143 import/13 call ratchets. Expiry worker/session fencing and caller context remain open.
- **Prepared listener ownership repair (W2.4p):** controlled port-reuse proof confirmed Close-before-Serve leaked the bound socket. Close now releases HTTP server and socket, treats already-closed cleanup as success and preserves other causes. Permanent before/concurrent/repeated close, port reuse and cause tests plus adapter race suite pass count=20; three mutations reject missing release/normalization/error propagation. Full gates: unchanged 222 packages and 143 import/13 call ratchets. Session/expiry/logout ownership and caller context remain open.
- **Callback listener move (W2.4o):** platform/browsercallback owns TCP binding, callback mux and HTTP header budget. App holds the prepared listener, publishes login state before Serve and supplies socket-free completion. Production app no longer imports net/http/net; existing TTL/delayed-close/server-close semantics remain. Source/listener/HTTP parity tests count=20 and six mutations pass. Full gates: unchanged 222 packages and 143 import/13 call ratchets. Lifetime ownership proof/fix and caller context remain open.
- **Callback HTTP presentation move (W2.4n):** platform/browsercallback owns query/request-context projection, the existing HTML/escape renderer and requested close dispatch after rendering. App retains its business bridge; the unreachable private page wrapper is removed and its source test targets the actual renderer. Body/HTTP parity and primitive/source suites count=20 plus seven valid mutations pass. Full gates: 222 packages, unchanged 143 import/13 call ratchets; official kernel structure 113. Listener/lifetime ownership and caller-context refinement remain open.
- **OAuth callback business extraction (W2.4m):** a socket-free helper owns denial/state/code admission, bounded exchange and state/reload/model effect order. The existing HTTP wrapper reads fields/context, renders results and schedules delayed close. Socket-free/source contracts, old/new page/state parity count=20 and nine mutations pass. Full gates: unchanged 221 packages and 143 import/13 call ratchets. HTTP presentation extraction and separate startup/expiry/logout lifecycle refinement remain open.
- **Provider probe operation binding (W2.4l):** typed spec/output uses the common dispatcher with primary-only/read-only metadata and the existing POST route. Known URL/key admission precedes requests; unknown legacy args and success/failure field presence are preserved. Native parity, actual socket tenant refusal before requests, typed schema/read-only/source contracts count=20 and eight mutations pass. Full gates: unchanged 221 packages and 143 import/13 call ratchets. Callback business/HTTP and later context/lifecycle refinement remain open.
- **Provider probe service move (W2.4k):** app/providers owns endpoint models checking behind the bounded GET port; CP retains lenient URL/key decoding, native result/error framing and primary-only/read-only metadata. Socket-free/source fixtures and native old/new parity across six statuses × six argument sets pass count=20; eight mutations guard admission, normalization, paths/bounds, status semantics, counting and failure shapes. Full gates: unchanged 221 packages and 143 import/13 call ratchets. Typed probe binding and later context/callback refinement remain open.
- **Shared guarded GET move (W2.4j):** provider probe and WhatsApp status/QR use the unchanged bounded helper in platform/netout through the existing CP forwarder. HTTP posture, fresh client, background timeout, request/response fields, redirect bound and partial reads retain source behavior. Body parity, primitive/native fixture/source suites count=20 and eight mutations pass. Full gates: unchanged 221 packages and 143 import/13 call ratchets. Primary-only probe service/binding and subsequent context/callback refinement remain open.
- **Provider observation operation binding (W2.4i):** log/stats/rejections have typed specs/results and common host binding. OwnTenant/CallerTenant/read-only metadata selects the routed journal; old wrappers/registrations are removed. Numeric limits/window and boolean admission precede service reads; unknown unused args, permissive cursors and present-empty row fields retain their wire behavior. Native parity, socket tenant isolation and source/schema/HTTP contracts count=20 plus ten mutations pass. Full gates: unchanged 221 packages, 143 import/13 call ratchets. Log keeps its existing HTTP hint; stats/rejections retain native-only registration. Primary-only probe and callback adaptation remain open.
- **Provider observation service move (W2.4h):** app/providers owns log/stats/rejections folds and wire shaping against the host-selected journal. CP retains tenant selection, legacy admission and native framing. Model-chain hops remain separate from provider fallback rates; rejection rows retain their non-cursor shape. Three old/new native handlers × eight argument sets match count=20; primitive/source/tenant/audit suites count=20 and nine independent mutations pass. Full gates: unchanged 221 packages and 143 import/13 call ratchets. Typed tenant-routed binding and primary-only probe remain open.
- **Shared journal projection move (W2.4g):** platform/journalview owns the shared fold/cutoff/sort/cursor/page mechanics. CP retains admission, tenant selection and native framing. Decode-before-cutoff preserves cross-event input state; newest-first sequence ties, strict cursor filtering before limit, next-page boundaries and journal error causes retain their source behavior. Body parity, primitive/source suites count=20 and seven independent mutations pass. Full gates: 221 packages, unchanged 143 import/13 call ratchets. Provider observation service/binding and primary-only probe remain open.
- **Provider OAuth RPC binding (W2.4f):** four typed specs/results share the dispatcher and existing per-server state; old socket RPC wrappers are removed. Mandatory start/import/logout admission prevents state/listener/vault/reload effects on unavailable audit; status retains read-only metadata and authoritative hook models, including empty results. Actual socket/schema/source/auth/tenant/HTTP contracts count=20 and seven mutations pass with unchanged 143 import/13 call ratchets. Observations/primary-only probe and later callback adaptation remain open.
- **Provider OAuth state/business move (W2.4e):** app/providers.OAuth owns login state/mutex/listener/token manager and socket-free RPC methods; CP retains per-server instance/fresh model hook and legacy framing/auth/audit. Callback HTTP/expiry/page flow moves unchanged with the state. State/token/model/denial/concurrent-status/escaping/source contracts pass count=20 with six mutations. Full gates remove/ratchet CP→chatgptauth (143 imports/13 calls); OAuth specs/binding and remaining observations/primary-only probe are open.
- **Provider catalog/keyring binding (W2.4d):** six typed specs/results share the app registry/adapter; old socket wrappers and duplicate helpers are removed. Per-op inputs ignore unrelated legacy args, five mutations require audit before effects and list stays read-only/fingerprint-only. Actual socket/schema/rejection/privacy/source/tenant/HTTP contracts count=20, old six-handler JSON parity and eight mutations pass. CP→catalog edge is removed and officially ratcheted (144 imports/13 calls); OAuth and tenant-routed observations/probe remain open.
- **Provider catalog/keyring service move (W2.4c):** app/providers owns connect/reload and key list/add/activate/remove business methods. CP wrappers retain arg validation/order, error framing/auth/audit. Existing catalog model lists and unknown new endpoint coverage remain; keyring scope/privacy/active mirror/reload contracts pass count=20 with original six-handler JSON parity and nine mutations. Full gates retain allowlists (220 packages). Operation binding and remaining OAuth/log/stats/rejections/probe migration remain open.
- **Catalog operation binding (W2.4b):** three typed app/catalog specs derive CP metadata and share the common per-server dispatcher; old socket handlers are deleted. Mutation audit is mandatory/app-owned, list is read-only and domain events share operation identity. Ignored primary-only tenant args no longer label the audit principal as tenant-routed; actual own-tenant routing remains. Typed provider/model/price/timestamp wire, schema admission, source/host/HTTP contracts count=20, original list parity and eight mutations pass. Full gates retain allowlists; provider domain follows.
- **Catalog service move (W2.4a):** app/catalog owns transport-independent sync/list/discover with existing fetch/persistence/full provider reload/event and wire projection. CP wrappers retain argument/error/auth/audit contracts during binding. Socket-free/source tests count=20, original list JSON parity count=20 and nine independent mutations retain credentials, sorted models, optional pricing/rebuild errors and failure kinds. Full gates pass; no allowlist growth. Catalog specs/common app binding and wrapper deletion follow; providers remain a separate next domain.
- **W2.1 framework exit (W2.1j):** text codec types now require explicit schemas, closing the reproduced netip.Addr object-vs-string mismatch. Actual text codec dispatch, complete framework/pilot/native host/source contracts pass count=20 with separate encoder/decoder mutations; full gates retain allowlists. No-I/O dispatch is 6.9–9.0 us (<50 us). [Exit evidence](23-w21-exit-evidence.md) maps requirements and representation/principal/native wire boundaries. Framework plus status/version pilot is complete; ordered catalog/provider and remaining domain migration follows.
- **Control-plane app host ports (W2.1i):** the common adapter binds a mandatory journal auditor and native Event emitter. AppOwned registration transfers audit ownership to app dispatch; legacy handlers keep the old path. Routing injects primary/caller-tenant kernel and shared actor/correlation, and registration rejects incompatible terminal/frame shapes before effects. Real socket mutation/stream/panic/rejection/audit-error/tenant/privacy/cause/live-disconnect fixtures pass count=20 with thirteen mutations. Status/version remain the shipped read-only pilot; full exit verification and ordered domain migration follow.
- **Independent stream contracts (W2.1h):** typed handlers bind emission and terminal result types separately, with owned EmissionSchema/OutputSchema validation. Same-type streams retain explicit output-schema fallback; unary mode has no emission metadata or usable port. Distinct derived/explicit types, custom frame registration, wrong frames/results, transport causes and schema copies pass count=20 with six mutation guards. Existing socket domains are unchanged; production audit/stream host adapters remain open.
- **Embedded operation wire fields (W2.1g):** schema derivation resolves anonymous/named embeddings using shallow/tag dominance and conflict omission before deriving children. Nil anonymous pointer parents make promoted fields optional; private value embeddings retain exported children. Actual JSON parity/app dispatch/source contracts pass count=20 with six mutations; no-I/O dispatch is 6.4–8.5 us. Private pointer allocation, recursive/custom/quoted representations remain explicit-schema boundaries; production mutation/streaming hosts are still open.
- **Typed pilot outputs (W2.1f):** app/system status/version return Go models; nested status field schemas are derived while version uses an explicit schema for brand.Binary custom encoding. The CP adapter serializes into the legacy Result map, preserving optional omission and enabled zero-tenant metadata. Returned host metadata is copied. Legacy JSON parity, typed/schema/source tests count=20 and five independent mutations guard the wire boundary. Embedded schema derivation and production mutating/streaming hosts remain open.
- **Custom operation wire contracts (W2.1e):** explicit input/output schemas are linted/copied at registration, with actual Go type metadata retained and derived defaults. Terminal results and stream emissions validate against the owned output schema before transport publication. Custom time.Time input/ownership/error contracts and five mutations guard the boundary; no-I/O dispatch is 7.0–8.6 us. Generic embedding/typed output models and production host migration remain open.
- **Typed schema refinement (W2.1d):** pointer roots/fields, nil slices/byte slices and maps use JSON null unions without relaxing non-null nested or required-field checks. Map values now carry their Go-derived schemas. Existing validator unions are reused; actual app.Dispatch, source/target and four mutation guards cover the boundaries. Recursive/custom/embedded representations still require subsequent work.
- **Typed operation pilot (W2.1c):** app/system owns status/version specs; control-plane registration derives auth/tenant/stream/read-only fields and a common adapter calls app.Dispatcher. Old socket handlers are deleted; native framing/auth remain, and the app re-authenticates through a host port. Pure opapi contracts and Go-derived supported schemas enforce admission before effects; current pilot output maps/unknown args remain compatible. Mutation audit and streaming emitter contracts are tested independently; production host migration for those modes remains open. Eight mutations and a no-I/O mock-host benchmark (3.6–4.5 us) guard the foundation.
- **System handler boundary (W2.1b):** status/fallback/version presentation is transport-independent in app/system, retaining legacy wire maps. Control-plane wrappers encode the results and bind fresh daemon extras; HTTPBinding/ChannelInfo aliases preserve callers. Actual socket-free/source contracts count=20, body parity and four mutation guards retain the output and optional-data boundaries. The existing registry still owns auth/tenant/read-only/audit metadata until the dispatch rewrite.
- **Two kernel entry paths**: Web UI/CLI → control plane; REST/OpenAI/agentgw → kernel directly. Op-level validation in `handleRun`
  (tool allowlists, execution profiles, agent resolution, dry-run) is not exposed by REST/OpenAI (`Engine.RunModel`). Image admission is shared through `runtime.Kernel.AdmitImages` (W2.2a): text-only models use the configured vision sidecar; rejections use the same message and correlated journal event.
- **Workspace path boundary (W2.xa):** console root lookup, path normalization and resolved containment now live in `platform/fileworkspace` with mechanically unchanged bodies. The temporary resolver forwarder preserved HTTP contracts during migration and is removed in W2.xc; source tests retain real link/junction refusal. Target tests and four mutations guard root creation, exact NUL errors, legitimate missing tails and containment. W2.xb also moves mkdir/rename/delete primitives to that platform package while HTTP decoding, path/status/text/result mapping stays in the handlers. Source and target tests (count=20) plus four mutations retain parents, rename direction, recursive opt-in and OS error identity; final symlink refusal remains. These foundations do not journal writes.
- **File Manager governance (W2.xc):** mkdir/rename/delete proxy primary-only mutating ops. `app/files` uses the existing per-kernel invocation port with a local tool adapter; file.write/file.delete policy and tool audit share dispatch's operation correlation. Root creation/resolution occurs after admission. Optional response error_code and ErrServerError.Code preserve status/text mapping; legacy errors remain unchanged. Actual HTTP/socket journal/disk fixtures and eight mutations guard the binding.
- **Rollback store boundary (W2.xd):** checkpoint/catalog types and unchanged catalog/restore/conversion bodies live in platform/rollbackstore. WebUI aliases and forwarders preserve JSON, legacy home lookup, atomic writes, restore bytes/absence and error text until operation binding. Source/target tests count=20, body parity and four mutations guard the move.
- **File snapshot governance (W2.xe):** `/api/rollback/apply` sends only the file checkpoint ID to primary-only file_restore. The daemon resolves its own catalog, invokes a private snapshot adapter through the kernel's service, checks file.write/file.delete and journals the correlated op/tool arc. Snapshot content never enters audit input; AppliedMS and catalog persistence follow successful restore inside the op. Already-applied IDs remain no-ops; denial/unavailable audit preserves file/catalog. Actual HTTP/socket, daemon authority/privacy tests and eight mutations guard the binding. Legacy skill/workflow/config catalog compatibility remains until domain migration. `rollbackCatalogPath` uses
  `internal/paths.BaseDir()` rather than the daemon's injected base dir.
- **agentgw reachability**: the default socket is a random abstract unix name that nothing publishes (no accessor/env export), so
  subprocesses can only reach it when `AGEZT_AGENTGW_SOCKET` is set; abstract `@` sockets are Linux-specific. `channel.*` and `db.*`
  capabilities are declared but have no routes. Subscribe accepts any bus pattern and publish any subject (kind `info`) — capability
  scoping is per-namespace, not per-subject.
- **Hardcoded env names**: `platform/fileworkspace` and `webui/files_route*.go` read `"AGEZT_FILE_ROOT*"` literally instead of `brand.EnvPrefix`, and
  `runtime/compose.go` reads `"AGEZT_AGENTGW_SOCKET"` literally.
- **Stale doc**: `kernel/restapi/doc.go` lost its package comment head (only the two update routes remain); `controlplane/roster_activity_text.go` is an empty header-only file.
- Several ops are reachable only from the CLI by design (Web UI audit noted un-wired: `why`, `journal_export`, `pulse_asks`, `skill_read_file`, `toolbox_detect/outdated`).
