# 03 — Control plane and HTTP surfaces

**Scope:** `kernel/controlplane` (206 non-test files, ~32.0k LOC, 324 protocol ops), `kernel/httpserver`, `kernel/auth`,
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
- **TenantRouted** — handler resolves its kernel per request via `kernelFor`/`edictFor`/`projectJournal`. Invariant
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
(`agentListCache*`, RWMutex, 1.5 s TTL keyed by roster content hash — `roster_list.go`), `standingFire func(id) bool`,
`observers PulseObservers`, `tenants *tenant.Registry`, `configEnvPinned`, `cancelOnDisconnect`, `diskFree DiskFreeFunc`,
`httpBindings []HTTPBinding`, `channels []ChannelInfo`, `channelSend ChannelSender`, `credChain string`, `boardStore *board.Store`
+ `boardNotify` (the ONE shared board instance — M937), `updateSvc *update.Service`, channel-OAuth `oauthPending` (oauthMu),
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
- Mutexes: `mu` (listener/token), `agentListCacheMu`, `oauthMu`, `provLoginMu`. Handlers otherwise rely on the kernel subsystems'
  own locking. `commandRegistry` is init-only and read lock-free.

### 1.6 Persistence (files it writes directly)

| Path under `<baseDir>` | Writer | Format |
|---|---|---|
| `runtime/control.addr`, `runtime/control.token` | `writeRuntimeFiles` | text, 0600 |
| `chat_prompts.json` | `handlePromptsSet` (`prompts.go`) | JSON list of `promptItem`, 0600 |
| `update.sentinel` | `writeUpdateSentinel` (`update_control.go`) | RFC3339 timestamp, tells the watchdog the exit was intentional |
| settings store (`settings.NewStore(baseDir)`) | `persistPulseSetting` (`AGEZT_PULSE_DIAL`, `AGEZT_PULSE_QUIET_HOURS`), `config_set`, routing/chains (`AGEZT_TASK_MODEL_CHAINS`), persona | via `kernel/settings` |
| vault / keyring | `provider_keys.go`, `channel_accounts.go`, `channel_oauth.go`, `provider_oauth.go` | via `kernel/creds` |
| catalog `api.json` + meta, `custom.json` | `catalog_sync`, `provider_connect` | via `kernel/catalog` |

Everything else is delegated to the owning subsystem (roster, memory, cadence store, workflow store, board, datalake, ...) and
journaled by that subsystem. Reads of `sandbox/projects/<name>` are confined by `confineUnder` (`sandbox.go`).

### 1.7 Env vars read directly

`AGEZT_TOKEN` (client), `AGEZT_ACP_AGENT_CMD` (`acp.go`), `AGEZT_PEERS` (`nodes.go`), `AGEZT_REMOTE_EVENT_MIRROR`
(`remote_mirror_helpers.go`), `AGEZT_AUTO_REPAIR_COOLDOWN` (`roster_repair_summary.go`), `AGEZT_WORKSPACE` (`roster_workspace.go`),
`AGEZT_CATALOG_URL` (catalog sync default), provider env names presence (`catalog.go`, `channels.go`, `config_handler.go`,
`settings_handlers.go`); run-time execution-profile errors reference `AGEZT_EXEC_SSH*`, `AGEZT_EXEC_K8S*`, `AGEZT_EXEC_MODAL*`,
`AGEZT_EXEC_DAYTONA*`. `config.go` holds `configEnvVars`, the canonical inventory (~419 entries) of every `AGEZT_*` var the daemon reads,
surfaced by the `config` op as presence-only; guarded by `TestConfigEnvVars_CoversCmdAgeztReads` (M127) — a new daemon env var must be
added here. `config_set` writes the store AND calls `setLiveEnv` (`os.Setenv`) so a change is live; `AGEZT_PROVIDER`/`AGEZT_MODEL`
additionally trigger a kernel reload (`configFieldNeedsKernelReload`).

### 1.8 Events

The control plane is mostly a consumer: it subscribes to the bus for `run` (subject `k.SubjectForRun(corr)`, buffer 1024) and
`pulse_subscribe` (pattern, buffer 4096, optional journal replay first), and folds the journal (`k.Journal().Range`) for every
`*_log` / `*_stats` / `runs_*` / `agent_activity` / `inbox` / `changelog` view through the single engine `projectJournal`
(`projections.go`: limit clamp, `<ms>:<seq>` cursor, `since_ms`, tenant resolution, newest-first). It publishes only a few events
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
StreamLive. **Web UI route(s)**: the `kernel/webui` route that proxies it (blank = CLI/SDK only). 324 ops total, 211 reachable from the
Web UI. Generated from source (registry funcs × `Cmd*` constants × handler definitions).

#### Core lifecycle: run / halt / resume / why / approvals / plan — `registerCoreCommands` (server_handle_run_remote.go), 11 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `file_mkdir` | primary | | `handleFileMutation` → files.go → app/files (file.write) | `/api/files/mkdir` |
| `file_rename` | primary | | `handleFileMutation` → files.go → app/files (file.write) | `/api/files/rename` |
| `file_delete` | primary | | `handleFileMutation` → files.go → app/files (file.delete) | `/api/files/delete` |
| `approvals` | primary |  | `handleApprovals` → server_commands.go | `/api/approvals` |
| `cancel_run` | tenant |  | `handleCancelRun` → server_commands.go | `/api/cancel_run` |
| `decide` | primary |  | `handleDecide` → server_handlers_plan.go | `/api/decide` |
| `halt` | primary |  | `handleHalt` → server_commands.go | `/api/halt` |
| `journal_verify` | primary |  | `handleVerify` → server_commands.go |  |
| `plan` | primary | events | `handlePlan` → server_handlers_plan.go | `/api/plan/run` |
| `resume` | primary |  | `handleResume` → server_commands.go | `/api/resume` |
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
| `runs_list` | tenant |  | `handleRunsList` → runs_handlers.go | `/api/runs` |
| `runs_stats` | tenant |  | `handleRunsStats` → runs_handlers_stats.go |  |
| `sandbox_delete` | primary |  | `handleSandboxDelete` → sandbox.go | `/api/sandbox/delete` |
| `sandbox_file` | primary |  | `handleSandboxFile` → sandbox.go | `/api/sandbox_file` |
| `sandbox_list` | primary |  | `handleSandboxList` → sandbox.go | `/api/sandbox` |
| `shutdown` | primary |  | `handleShutdown` → shutdown.go |  |
| `spend_today` | primary |  | `handleSpendToday` → handle_spend_attention.go | `/api/spend/today` |
| `state_get` | primary |  | `handleStateGet` → state.go |  |
| `state_list` | primary |  | `handleStateList` → state.go |  |
| `status` | primary |  | `handleStatus` → status.go | `/api/status` |
| `storage_stats` | primary |  | `handleStorageStats` → storage.go |  |
| `update_apply` | primary |  | `handleUpdateApply` → update_control.go |  |
| `update_check` | primary |  | `handleUpdateCheck` → update_control.go |  |

#### Journal reads and journal-folded audit logs — `registerJournalLogCommands` (registry.go), 27 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `approvals_log` | tenant |  | `handleApprovalsLog` → approvals_log.go | `/api/approvals_log` |
| `approvals_stats` | tenant |  | `handleApprovalsStats` → approvals_log.go |  |
| `cache_stats` | tenant |  | `handleCacheStats` → cache_stats.go |  |
| `changelog` | primary, tenant-routed |  | `handleChangelog` → changelog.go |  |
| `edict_log` | tenant |  | `handleEdictLog` → policy_log.go | `/api/policy_log` |
| `edict_stats` | tenant |  | `handleEdictStats` → policy_log.go | `/api/policy` |
| `journal_export` | primary |  | `handleJournalExport` → journal_export.go |  |
| `journal_grep` | primary |  | `handleJournalGrep` → journal_grep.go | `/api/journal` |
| `journal_head` | primary |  | `handleJournalHead` → journal.go |  |
| `journal_stats` | primary, tenant-routed |  | `handleJournalStats` → journal_stats.go |  |
| `journal_tail` | primary |  | `handleJournalTail` → journal.go |  |
| `memory_log` | tenant |  | `handleMemoryLog` → memory_log.go | `/api/memory_log` |
| `netguard_log` | tenant |  | `handleNetguardLog` → netguard_log.go | `/api/netguard_log` |
| `provider_log` | tenant |  | `handleProviderLog` → provider_log.go | `/api/provider_log` |
| `provider_rejections` | tenant |  | `handleProviderRejections` → provider_log.go |  |
| `provider_stats` | tenant |  | `handleProviderStats` → provider_log.go |  |
| `ratelimit_log` | tenant |  | `handleRateLimitLog` → ratelimit_log.go | `/api/ratelimit_log` |
| `ratelimit_stats` | tenant |  | `handleRateLimitStats` → ratelimit_log.go |  |
| `schedule_fires` | tenant |  | `handleScheduleFires` → schedule_fires.go | `/api/schedule/fires` |
| `schedule_stats` | tenant |  | `handleScheduleStats` → schedule_fires_stats.go |  |
| `tool_log` | tenant |  | `handleToolLog` → tool_log.go | `/api/tool_log` |
| `tool_stats` | tenant |  | `handleToolStats` → tool_log.go |  |
| `warden_log` | tenant |  | `handleWardenLog` → warden_log.go | `/api/warden_log` |
| `warden_stats` | tenant |  | `handleWardenStats` → warden_log.go |  |
| `webhook_log` | tenant |  | `handleWebhookLog` → webhook_log.go | `/api/webhook_log` |
| `webhook_stats` | tenant |  | `handleWebhookStats` → webhook_log.go |  |
| `world_log` | tenant |  | `handleWorldLog` → world_log.go | `/api/world_log` |

#### Providers, keys, OAuth, routing, chains, budget, config — `registerProviderConfigCommands` (registry.go), 18 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `budget` | primary |  | `handleBudget` → budget.go | `/api/budget` |
| `budget_set` | primary |  | `handleBudgetSet` → budget.go |  |
| `chains_get` | primary |  | `handleChainsGet` → chains.go | `/api/chains` |
| `chains_set` | primary |  | `handleChainsSet` → chains.go | `/api/chains/set` |
| `config` | primary |  | `handleConfig` → config_handler.go | `/api/config` |
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
| `acp_agents` | primary |  | `handleACPAgents` → acp.go | `/api/acp/agents` |
| `channel_account_remove` | primary |  | `handleChannelAccountRemove` → channel_accounts.go | `/api/channel/account/remove` |
| `channel_account_set` | primary |  | `handleChannelAccountSet` → channel_accounts.go | `/api/channel/account/set` |
| `channel_list` | primary |  | `handleChannelList` → channels.go | `/api/channels` |
| `channel_oauth_callback` | primary |  | `handleChannelOAuthCallback` → channel_oauth.go | `/oauth/callback` |
| `channel_oauth_start` | primary |  | `handleChannelOAuthStart` → channel_oauth.go | `/api/channel/oauth/start` |
| `channel_oauth_status` | primary |  | `handleChannelOAuthStatus` → channel_oauth.go | `/api/channel/oauth/status` |
| `inbox` | primary |  | `handleInbox` → inbox.go | `/api/inbox` |
| `provider_probe` | primary |  | `handleProviderProbe` → channels_wa.go | `/api/provider/probe` |
| `send` | primary |  | `handleSend` → send.go | `/api/send` |
| `whatsappgw_qr` | primary |  | `handleWhatsAppGatewayQR` → channels_wa.go | `/api/whatsappgw/qr` |
| `whatsappgw_status` | primary |  | `handleWhatsAppGatewayStatus` → channels_wa.go | `/api/whatsappgw/status` |

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
| `plan_history` | tenant |  | `handlePlanHistory` → plan_history.go | `/api/plan_history` |
| `plan_refine` | primary | LIVE | `handlePlanRefine` → planner.go | `/api/plan/refine` |
| `plan_stats` | tenant |  | `handlePlanStats` → plan_history.go |  |
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
| `edict_compact` | primary, tenant-routed |  | `handleEdictCompact` → edict_overlay.go |  |
| `edict_overlay` | tenant |  | `handleEdictOverlay` → edict_overlay.go |  |
| `plugin_list` | primary |  | `handlePluginList` → plugin.go |  |
| `tool_list` | primary |  | `handleToolList` → tool.go | `/api/tools_catalog` |
| `toolbox_detect` | primary |  | `handleToolboxDetect` → toolbox.go |  |
| `toolbox_install` | primary | events | `handleToolboxInstall` → toolbox.go | `/api/toolbox/install` (SSE) |
| `toolbox_outdated` | primary |  | `handleToolboxOutdated` → toolbox.go |  |

#### Edict policy (tenant-routed) — `registerEdictCommands` (edict.go), 7 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `edict_deny_add` | tenant |  | `handleEdictDenyAdd` → edict_deny.go | `/api/edict/deny_add` |
| `edict_deny_list` | tenant |  | `handleEdictDenyList` → edict_deny.go |  |
| `edict_deny_rm` | tenant |  | `handleEdictDenyRemove` → edict_deny.go | `/api/edict/deny_rm` |
| `edict_set_level` | tenant |  | `handleEdictSetLevel` → edict_set.go | `/api/edict/set_level` |
| `edict_set_mode` | tenant |  | `handleEdictSetMode` → edict_set.go | `/api/edict/set_mode` |
| `edict_show` | tenant |  | `handleEdictShow` → edict.go | `/api/edict_show` |
| `edict_test` | tenant |  | `handleEdictTest` → edict.go | `/api/edict/test` |

#### Live run steering — `registerSteerCommands` (steer.go), 5 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `run_intervene` | tenant |  | `handleRunIntervene` → steer.go |  |
| `run_pause` | tenant |  | `handleRunPause` → steer.go | `/api/run/pause` |
| `run_resume` | tenant |  | `handleRunResume` → steer.go | `/api/run/resume` |
| `run_steer` | tenant |  | `handleRunSteer` → steer.go | `/api/run/steer` |
| `run_step` | tenant |  | `handleRunStep` → steer.go | `/api/run/step` |

#### Multi-tenancy registry — `registerTenantCommands` (tenant.go), 6 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `tenant_create` | primary |  | `handleTenantCreate` → tenant_handlers.go |  |
| `tenant_list` | primary |  | `handleTenantList` → tenant_handlers.go |  |
| `tenant_release` | primary |  | `handleTenantRelease` → tenant_handlers.go |  |
| `tenant_remove` | primary |  | `handleTenantRemove` → tenant_handlers.go |  |
| `tenant_stats` | primary, tenant-routed |  | `handleTenantStats` → tenant_handlers.go |  |
| `tenant_token` | primary |  | `handleTenantToken` → tenant_handlers.go |  |

#### Agent roster (agents) — `registerRosterCommands` (roster.go), 17 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `agent_activity` | primary |  | `handleAgentActivity` → roster_activity.go | `/api/agents/activity` |
| `agent_add` | primary |  | `handleAgentAdd` → roster_crud.go | `/api/agents/add` |
| `agent_edit` | primary |  | `handleAgentEdit` → roster_crud.go | `/api/agents/edit` |
| `agent_escalations` | primary |  | `handleAgentEscalations` → roster_escalation.go | `/api/agents/escalations` |
| `agent_graveyard` | primary |  | `handleAgentGraveyard` → roster_tombstone.go |  |
| `agent_impact` | primary |  | `handleAgentImpact` → roster.go | `/api/agents/impact` |
| `agent_list` | primary |  | `handleAgentList` → roster_list.go | `/api/agents` |
| `agent_remove` | primary |  | `handleAgentRemove` → roster_lifecycle.go | `/api/agents/remove` |
| `agent_repair` | primary |  | `handleAgentRepair` → roster_repair.go | `/api/agents/repair` |
| `agent_repair_status` | primary |  | `handleAgentRepairStatus` → roster_activity.go | `/api/agents/repair_status` |
| `agent_resolve` | primary |  | `handleAgentResolve` → roster_wake.go | `/api/agents/resolve` |
| `agent_retire` | primary |  | `handleAgentRetire` → roster_lifecycle.go | `/api/agents/retire` |
| `agent_revive` | primary |  | `handleAgentRevive` → roster_lifecycle.go | `/api/agents/revive` |
| `agent_set_enabled` | primary |  | `handleAgentSetEnabled` → roster_crud.go | `/api/agents/enable` |
| `agent_task_update` | primary |  | `handleAgentTaskUpdate` → roster_task_update.go | `/api/agents/task` |
| `agent_tombstone` | primary |  | `handleAgentTombstone` → roster_tombstone.go |  |
| `agent_wake` | primary |  | `handleAgentWake` → roster_wake.go | `/api/agents/wake` |

#### Memory — `registerMemoryCommands` (memory_helpers.go), 15 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `memory_add` | primary |  | `handleMemoryAdd` → memory_handlers.go | `/api/memory/add` |
| `memory_audit` | tenant |  | `handleMemoryAudit` → memory_handlers_admin.go | `/api/memory/audit` |
| `memory_bulk_forget` | primary |  | `handleMemoryBulkForget` → memory_handlers_admin.go | `/api/memory/bulk_forget` |
| `memory_clean` | tenant |  | `handleMemoryClean` → memory_handlers_admin.go | `/api/memory/clean` |
| `memory_consolidate` | primary |  | `handleMemoryConsolidate` → memory.go |  |
| `memory_find_related` | primary |  | `handleMemoryFindRelated` → memory_handlers_admin.go |  |
| `memory_forget` | primary |  | `handleMemoryForget` → memory_handlers_tidy.go | `/api/memory/forget` |
| `memory_get` | primary |  | `handleMemoryGet` → memory_handlers.go |  |
| `memory_list` | primary |  | `handleMemoryList` → memory_handlers.go | `/api/memory` |
| `memory_promote` | primary |  | `handleMemoryPromote` → memory_handlers_tidy.go | `/api/memory/promote` |
| `memory_prune` | primary |  | `handleMemoryPrune` → memory_handlers_tidy.go | `/api/memory/prune` |
| `memory_search` | primary |  | `handleMemorySearch` → memory_handlers.go |  |
| `memory_supersede` | primary |  | `handleMemorySupersede` → memory_handlers.go | `/api/memory/supersede` |
| `memory_tidy` | primary |  | `handleMemoryTidy` → memory_handlers_tidy.go | `/api/memory/tidy` |
| `profile_rebuild` | primary |  | `handleProfileRebuild` → memory.go | `/api/profile/rebuild` |

#### World model — `registerWorldCommands` (world.go), 8 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `world_add` | primary |  | `handleWorldAdd` → world_handlers.go | `/api/world/add` |
| `world_edit` | primary |  | `handleWorldEdit` → world_handlers.go | `/api/world/edit` |
| `world_forget` | primary |  | `handleWorldForget` → world_handlers.go | `/api/world/forget` |
| `world_get` | primary |  | `handleWorldGet` → world_handlers.go |  |
| `world_list` | primary |  | `handleWorldList` → world_handlers.go | `/api/world` |
| `world_neighbors` | primary |  | `handleWorldNeighbors` → world_handlers.go |  |
| `world_relate` | primary |  | `handleWorldRelate` → world_handlers.go | `/api/world/relate` |
| `world_resolve` | primary |  | `handleWorldResolve` → world_handlers.go |  |

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

#### Marketplace — `registerMarketCommands` (market.go), 8 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `market_add_source` | primary |  | `handleMarketAddSource` → market.go | `/api/market/source/add` |
| `market_install` | primary | events | `handleMarketInstall` → market.go | `/api/market/install` (SSE) |
| `market_list` | primary |  | `handleMarketList` → market.go | `/api/market` |
| `market_remove_source` | primary |  | `handleMarketRemoveSource` → market.go | `/api/market/source/remove` |
| `market_show` | primary |  | `handleMarketShow` → market.go | `/api/market/show` |
| `market_sources` | primary |  | `handleMarketSources` → market.go | `/api/market/sources` |
| `market_sync` | primary |  | `handleMarketSync` → market.go | `/api/market/sync` |
| `market_uninstall` | primary | events | `handleMarketUninstall` → market.go | `/api/market/uninstall` (SSE) |

#### MCP servers — `registerMCPCommands` (mcp.go), 6 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `mcp_add` | primary |  | `handleMCPAdd` → mcp.go | `/api/mcp/add` |
| `mcp_attach` | primary |  | `handleMCPAttach` → mcp.go | `/api/mcp/attach` |
| `mcp_detach` | primary |  | `handleMCPDetach` → mcp.go | `/api/mcp/detach` |
| `mcp_list` | primary |  | `handleMCPList` → mcp.go | `/api/mcp` |
| `mcp_remove` | primary |  | `handleMCPRemove` → mcp.go | `/api/mcp/remove` |
| `mcp_set_enabled` | primary |  | `handleMCPSetEnabled` → mcp.go | `/api/mcp/enable` |

#### Tool Forge (script tools) — `registerToolforgeCommands` (toolforge.go), 8 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `toolforge_draft` | primary |  | `handleToolforgeDraft` → toolforge.go |  |
| `toolforge_edit` | primary |  | `handleToolforgeEdit` → toolforge.go |  |
| `toolforge_list` | primary |  | `handleToolforgeList` → toolforge.go |  |
| `toolforge_promote` | primary |  | `handleToolforgePromote` → toolforge.go |  |
| `toolforge_quarantine` | primary |  | `handleToolforgeQuarantine` → toolforge.go |  |
| `toolforge_remove` | primary |  | `handleToolforgeRemove` → toolforge.go |  |
| `toolforge_show` | primary |  | `handleToolforgeShow` → toolforge.go |  |
| `toolforge_test` | primary |  | `handleToolforgeTest` → toolforge.go |  |

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

#### Settings / config schema — `registerSettingsCommands` (settings.go), 5 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `config_schema` | primary |  | `handleConfigSchema` → settings_handlers.go | `/api/config/schema` |
| `config_schema_register` | primary |  | `handleConfigSchemaRegister` → settings_handlers.go | `/api/config/schema/register` |
| `config_schema_unregister` | primary |  | `handleConfigSchemaUnregister` → settings_handlers.go | `/api/config/schema/unregister` |
| `config_set` | primary |  | `handleConfigSet` → settings_handlers.go | `/api/config/set`, `/api/rollback/apply` |
| `config_values` | primary |  | `handleConfigValues` → settings_handlers.go | `/api/config/values` |

#### Config Center (rated agent config) — `registerConfigCenterCommands` (configcenter_register.go), 9 ops

| op | auth | stream | handler → file | Web UI route(s) |
|---|---|---|---|---|
| `configcenter.access` | primary |  | `handleConfigCenterSetAccess` → configcenter_handler_access.go | `/api/configcenter/access` |
| `configcenter.access-log` | primary |  | `handleConfigCenterAccessLog` → configcenter_handler_access.go |  |
| `configcenter.audit` | primary |  | `handleConfigCenterAudit` → configcenter_handler_audit.go |  |
| `configcenter.delete` | primary |  | `handleConfigCenterDelete` → configcenter_handler.go | `/api/configcenter/delete` |
| `configcenter.get` | primary |  | `handleConfigCenterGet` → configcenter_handler.go |  |
| `configcenter.health` | primary |  | `handleConfigCenterHealth` → configcenter_handler_audit.go |  |
| `configcenter.list` | primary |  | `handleConfigCenterList` → configcenter_handler.go | `/api/configcenter/list` |
| `configcenter.set` | primary |  | `handleConfigCenterSet` → configcenter_handler.go | `/api/configcenter/set` |
| `configcenter.set-rating` | primary |  | `handleConfigCenterSetRating` → configcenter_handler_access.go | `/api/configcenter/rating` |

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
| `args_compound.go` | `argStrings`, `argStringMap`, `argStringList`. |
| `args_specialized.go` | `argLimit`, `argDryRun`, `argFlag`. |
| `client.go` | `Client`, `NewClient`, `ProbeExisting`, `ErrServerError`, `Call`, `CallRaw`, `Stream`, `StreamUntilCancel`. |
| `client_helpers.go` | `dial` (5 s), `setReadDeadlineFromCtx`, timeout→ctx error mapping, `writeRequest`, `readOneResponse`, `toString`, `toBool`. |
| `client_update.go` | Typed client wrappers `UpdateCheck`/`UpdateApply` and their result types; `Close`. |
| `projections.go` | `projectJournal`: the single journal-fold engine behind every `*_log` handler (limit, cursor, since, tenant, sort, next_cursor). |
| `tenant.go` | `registerTenantCommands`. |
| `tenant_handlers.go` | `tenant_create/token/list/release/remove/stats` handlers over `tenant.Registry`. |
| `tenant_helpers.go` | `tenantOf`, `kernelFor` (Acquire tenant kernel), `SetTenants`. |
| `shutdown.go` | `handleShutdown`: closes `shutdownCh` so the daemon exits via the same path as SIGTERM. |

**Core run lifecycle**

| File | What it does |
|---|---|
| `server_handle_run.go` | `handleRun` (666 lines): resolves tenant/agent/model/vision/system/timeout/tools/cost/execution-profile/assure overrides, dry-run plan, subscribes to run subject, launches the governed run, streams events, enriches result. |
| `server_handle_run_remote.go` | Remote-agezt execution-profile helpers (run events, answer preview, peer metadata) + `registerCoreCommands`. |
| `server_commands.go` | `version`, `halt`, `cancel_run`, `resume`, `why` (correlation walk), `whoami`, `journal_verify`, `approvals` handlers. |
| `server_handlers_plan.go` | `handlePlan` (execute a pre-built DAG `planSpec` via kernel scheduler, streaming) + `handleDecide` (resolve a HITL approval). |
| `dryrun.go` | `buildRunPlan` + `runPlanInput` consumer: renders the dry-run plan (model, context size warnings, tool set, timeout). |
| `dryrun_format.go` | `formatMicrocentsUSD`. |
| `dryrun_pricing.go` | `modelPriced`, `strictPricingPlan`, `runPlanInput` type. |
| `steer.go` | Live run steering (M608): `run_pause/resume/step/steer/intervene` (tenant-routed) + `registerSteerCommands`. |
| `remote_mirror.go` | `mirrorRemoteExecutionProfileEvents`: mirrors a remote peer's run events into the local journal. |
| `remote_mirror_fetch.go` | `fetchRemoteEvents`, `fetchRemoteArtifacts` (HTTP calls to peer REST API). |
| `remote_mirror_helpers.go` | Mirror mode (`AGEZT_REMOTE_EVENT_MIRROR`), peer lookup, payload redaction for mirrored events. |

**Runs listing / status / daemon ops**

| File | What it does |
|---|---|
| `runs.go` | `runEntry`, `runEntryStatus`, `collectRuns` journal walker. |
| `runs_extract.go` | Payload extractors (intent, agent, tool, iters, cost, model, answer preview, spawn link, reason). |
| `runs_handlers.go` | `handleRunsList` (cursor `<ms>:<seq>`, filters status/intent/model/cost). |
| `runs_handlers_stats.go` | `handleRunsStats` + duration percentile stats. |
| `status.go` | `handleStatus`: one-round-trip health overview (version, halted, budget, tools, HTTP bindings, channels, AWS cred chain, fallback counts). |
| `disk.go` | `handleDiskStats`: journal size + free space via injected `DiskFreeFunc` (M131). |
| `storage.go` | `handleStorageStats`: per-top-level-dir usage of the home dir (M927). |
| `journal.go` | `handleJournalTail`, `handleJournalHead`. |
| `journal_export.go` | `handleJournalExport` (streams events + verification material, `MaxJournalExportN`). |
| `journal_grep.go` | `handleJournalGrep`: server-side AND-filtered journal walk (subject/kind/correlation/pattern). |
| `journal_stats.go` | `handleJournalStats`: per-kind counts, segments. |
| `state.go` | `state_list`, `state_get` over the kernel state store. |
| `handle_spend_attention.go` | `spend_today` + `attention` (pending approvals + pulse asks feed) for Mission Control. |
| `update_control.go` | `update_check`, `update_apply` (drain → swap → restart; writes `update.sentinel`). |
| `sandbox.go` | `sandbox_list/file/delete` for code_exec projects under `sandbox/projects`, path-confined. |
| `reaper.go` | `handleReaperScan`: dead-agent/stale-artifact detection (read-only). |
| `redact_test_cmd.go` | `handleRedactTest`: runs the live redactor against a candidate string. |
| `cache_stats.go` | `handleCacheStats`: prompt-cache savings fold over `budget.consumed`. |
| `changelog.go` | `handleChangelog`: human-readable lifecycle timeline from the journal. |

**Audit log folds (journal → paginated lists)**

| File | What it does |
|---|---|
| `approvals_log.go` | `approvals_log`, `approvals_stats` (HITL history). |
| `policy_log.go` | `edict_log`, `edict_stats` over `policy.decision`. |
| `provider_log.go` | `provider_log`, `provider_stats`, `provider_rejections` over `routing.decision`/`provider.fallback`. |
| `ratelimit_log.go` | `ratelimit_log`, `ratelimit_stats` over `rate.limited`. |
| `netguard_log.go` | `netguard_log` over `netguard.blocked`. |
| `warden_log.go` | `warden_log`, `warden_stats` over `warden.*`. |
| `webhook_log.go` | `webhook_log`, `webhook_stats` over `webhook.delivered/failed`. |
| `memory_log.go` | `memory_log` over `memory.written/forgotten/superseded`. |
| `world_log.go` | `world_log` over world-model upserts/forgets. |
| `tool_log.go` | `tool_log`, `tool_stats` over `tool.invoked/result`; input/latency joins use `(correlation_id, call_id)` so reused IDs in another run cannot contaminate a row or give a denied call phantom latency (W2.3a). |
| `tool_decoders.go` | `decodeToolInvoked`, `decodeToolResult`. |
| `tool_helpers.go` | `previewString`. |
| `plan_history.go` | `plan_history`, `plan_stats` over `plan.*`. |
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
| `config_handler.go` | `handleConfig`: which env vars are set (never values). |
| `config_helpers.go` | `stringSliceMapToAny`. |
| `settings.go` | `registerSettingsCommands`. |
| `settings_handlers.go` | `config_schema`, `config_values`, `config_set`, `config_schema_register/unregister` (Config Center schema registry). |
| `settings_helpers.go` | `configFieldNeedsKernelReload`, `setLiveEnv` (`os.Setenv`). |
| `configcenter_register.go` | `registerConfigCenterCommands` (dotted `configcenter.*` names). |
| `configcenter_handler.go` | `configcenter.set/get/list/delete` + entry helpers. |
| `configcenter_handler_access.go` | `configcenter.set-rating`, `configcenter.access`, `configcenter.access-log`. |
| `configcenter_handler_audit.go` | `configcenter.audit`, `configcenter.health`, `entryToMap`, list helpers. |
| `persona.go` | `persona_get/set`: daemon default system prompt (full text). |
| `prompts.go` | `prompts_get/set`: saved chat prompt library in `chat_prompts.json`. |

**Channels, inbox, ACP**

| File | What it does |
|---|---|
| `channels.go` | `handleChannelList`: configured channels + per-account probe/media totals. |
| `channels_probe.go` | `channelAccountProbe`, `channelProbe`, `channelProbeMode`. |
| `channels_totals.go` | Probe/media total accumulators. |
| `channels_wa.go` | `whatsappgw_status`, `whatsappgw_qr` (WAHA/Evolution gateway proxy) and `provider_probe` (GET `<url>/models`). |
| `channel_accounts.go` | `channel_account_set/remove`: multi-account `ENV#label` convention (non-secret → config store, secret → vault). |
| `channel_oauth.go` | Channel OAuth flow (Phase 4): `channel_oauth_start/callback/status`, code exchange, `oauthFlow`/`oauthProvider` table. |
| `channel_oauth_helpers.go` | State minting, pending-flow pruning, instance URL normalization, HTTPS check. |
| `inbox.go` | `handleInbox`: unified inbox folded from `channel.inbound/outbound` journal events. |
| `send.go` | `handleSend`: operator-initiated outbound via injected `ChannelSender`. |
| `acp.go` | `handleACPAgents`: ACP agent inventory (installed/missing/default `AGEZT_ACP_AGENT_CMD`). |

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

**Agent roster** (`agent_*` ops)

| File | What it does |
|---|---|
| `roster.go` | Roster view types, `handleAgentImpact`, `registerRosterCommands`. |
| `roster_list.go` | `agent_list` + 1.5 s TTL cache (`tryServeAgentListCache`, `invalidateAgentListCache`), cursor encode/parse, model chain view. |
| `roster_crud.go` | `agent_add`, `agent_edit`, `agent_set_enabled`. |
| `roster_crud_internal.go` | Profile patch merge, kind normalization, hierarchy validation, managed-subagent guard. |
| `roster_task_update.go` | `agent_task_update` + arg presence helpers. |
| `roster_lifecycle.go` | `agent_retire`, `agent_revive`, `agent_remove` (+ cascade parsing/view). |
| `roster_lifecycle_helpers.go` | Impact/reference helpers (subagents, workflow references, mailbox labels). |
| `roster_cascade.go` | Teardown impact analysis across schedules, memory, skills, config, workspace, workflows, mailbox. |
| `roster_teardown.go` | Retire subagents, remove/pause standing orders and schedules for an agent. |
| `roster_teardown_state.go` | Forget agent memory, archive skills, prune config-center entries/access refs, delete workspace. |
| `roster_tombstone.go` | `agent_tombstone`, `agent_graveyard`. |
| `roster_activity.go` | `agent_activity` (journal timeline), `agent_repair_status`. |
| `roster_activity_views.go` | Repair contract / next-action / escalation owner views. |
| `roster_activity_text.go` | Header-only file left by a split (no code). |
| `roster_activity_text_misc.go` | Retry-policy, paused-trigger, removal-cleanup summaries. |
| `agent_activity_summary.go` | `agentActivitySummary`: 425-line text renderer. |
| `roster_status.go` | `agentStatusAccums` + single-pass journal fold for live status. |
| `roster_status_views.go` | `agentStatusViews`. |
| `roster_status_wake.go` | Wake-state views, schedule/agent matching helpers. |
| `roster_status_escalation.go` | Escalation load + routing-match helpers. |
| `roster_escalation.go` | `agent_escalations`, operator incident lineage, `runAgentWake`, `publishOperatorAction`. |
| `roster_escalation_rows.go` | Journal-derived escalation rows. |
| `roster_wake.go` | `agent_wake`, `agent_resolve` (uses `overseertool.NewKernelSource` → `ApplyRoutingChain`). |
| `roster_wake_helpers.go` | Operator force-generation, delegate validation, exhausted-chain lookup, list utils. |
| `roster_repair.go` | `agent_repair` (journaled auto-repair via `overseertool`). |
| `roster_repair_summary.go` | Repair summaries, `AGEZT_AUTO_REPAIR_COOLDOWN`. |
| `roster_workspace.go` | Workspace root (`AGEZT_WORKSPACE`) and file counts. |
| `roster_helpers.go` | Payload accessors `plString/plInt/...`, `truncate`, `firstNonEmpty`. |
| `tool.go` | `tool_list` (+ catalog probe, rollback mode). |
| `tool_agents.go` | `agent_permissions`, `agent_capabilities` (patch trust ceiling/tool allow-deny/noise/config overrides/memory scope/workdir/cost). |
| `tool_views.go` | `decodeControlplaneArg`, `applyAgentCapabilityPatch`, wake-access view. |
| `tool_views_governance.go` | `agentGovernanceView`, permission rows, noise policy. |

**Memory / world / skills / standing / schedules**

| File | What it does |
|---|---|
| `memory.go` | `memory_consolidate`, `profile_rebuild`. |
| `memory_handlers.go` | `memory_add`, `memory_supersede`, `memory_list`, `memory_get`, `memory_search`. |
| `memory_handlers_admin.go` | `memory_bulk_forget`, `memory_find_related`, `memory_audit`, `memory_clean`. |
| `memory_handlers_tidy.go` | `memory_forget`, `memory_promote`, `memory_prune`, `memory_tidy`. |
| `memory_helpers.go` | `recordView`, `jsonMap`, `registerMemoryCommands`. |
| `world.go` | `entityView`, `registerWorldCommands`. |
| `world_handlers.go` | `world_add/edit/relate/resolve/neighbors/list/get/forget`. |
| `world_helpers.go` | `worldAliasesAttrs`. |
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
| `market.go` | `market_list/show/install/uninstall/sources/add_source/remove_source/sync`, `parseCapList`. |
| `mcp.go` | `mcp_list/add/attach/detach/set_enabled/remove`. |
| `toolforge.go` | `toolforge_list/show/draft/edit/test/promote/quarantine/remove` (promotion is operator-only). |
| `toolbox.go` | `toolbox_detect/outdated/install` (host CLI tools; install streams). |
| `artifact.go` | `artifact_get` (re-verified bytes), `artifact_list`, `artifact_delete`, `artifact_collect`. |
| `plugin.go` | `plugin_list`. |
| `datalake.go` | `data_collections/records/insert/update/delete/create_collection/drop_collection`. |
| `edict.go` | `edictFor`, `edict_show`, `edict_test`, `registerEdictCommands`. |
| `edict_deny.go` | `edict_deny_list/add/rm`. |
| `edict_set.go` | `edict_set_level`, `edict_set_mode`. |
| `edict_overlay.go` | `edict_overlay` (net runtime policy), `edict_compact`. |

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
- **Journal audit view**: implement a `decode func(*event.Event) (map[string]any, bool)` and call `s.projectJournal(conn, req, "rows", decode)`.
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
- **Two kernel entry paths**: Web UI/CLI → control plane; REST/OpenAI/agentgw → kernel directly. Op-level validation in `handleRun`
  (tool allowlists, execution profiles, agent resolution, dry-run) is not exposed by REST/OpenAI (`Engine.RunModel`). Image admission is shared through `runtime.Kernel.AdmitImages` (W2.2a): text-only models use the configured vision sidecar; rejections use the same message and correlated journal event.
- **Workspace path boundary (W2.xa):** console root lookup, path normalization and resolved containment now live in `platform/fileworkspace` with mechanically unchanged bodies. The temporary resolver forwarder preserved HTTP contracts during migration and is removed in W2.xc; source tests retain real link/junction refusal. Target tests and four mutations guard root creation, exact NUL errors, legitimate missing tails and containment. W2.xb also moves mkdir/rename/delete primitives to that platform package while HTTP decoding, path/status/text/result mapping stays in the handlers. Source and target tests (count=20) plus four mutations retain parents, rename direction, recursive opt-in and OS error identity; final symlink refusal remains. These foundations do not journal writes.
- **File Manager governance (W2.xc):** mkdir/rename/delete proxy primary-only mutating ops. `app/files` uses the existing per-kernel invocation port with a local tool adapter; file.write/file.delete policy and tool audit share dispatch's operation correlation. Root creation/resolution occurs after admission. Optional response error_code and ErrServerError.Code preserve status/text mapping; legacy errors remain unchanged. Actual HTTP/socket journal/disk fixtures and eight mutations guard the binding.
- **Remaining unjournaled Web UI mutation**: rollback `file.snapshot` still restores filesystem content directly without an op/policy journal. `rollbackCatalogPath` uses
  `internal/paths.BaseDir()` rather than the daemon's injected base dir.
- **agentgw reachability**: the default socket is a random abstract unix name that nothing publishes (no accessor/env export), so
  subprocesses can only reach it when `AGEZT_AGENTGW_SOCKET` is set; abstract `@` sockets are Linux-specific. `channel.*` and `db.*`
  capabilities are declared but have no routes. Subscribe accepts any bus pattern and publish any subject (kind `info`) — capability
  scoping is per-namespace, not per-subject.
- **Hardcoded env names**: `platform/fileworkspace` and `webui/files_route*.go` read `"AGEZT_FILE_ROOT*"` literally instead of `brand.EnvPrefix`, and
  `runtime/compose.go` reads `"AGEZT_AGENTGW_SOCKET"` literally.
- **Stale doc**: `kernel/restapi/doc.go` lost its package comment head (only the two update routes remain); `controlplane/roster_activity_text.go` is an empty header-only file.
- Several ops are reachable only from the CLI by design (Web UI audit noted un-wired: `why`, `journal_export`, `pulse_asks`, `skill_read_file`, `toolbox_detect/outdated`).
