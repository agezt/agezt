# 01 — Daemon boot (`cmd/agezt`), shared `internal/*`, build/dev/CI tooling

**Scope:** `cmd/agezt` (31 non-test `.go` files), `cmd/agezt/internal/daemonconfig`, `internal/atomicfile`,
`internal/brand`, `internal/paths`, `internal/strutil`, `tools/*` (8 commands), `scripts/*`, `ops/wsl-runners`,
`Makefile`, `dev.ps1`/`dev.sh`, `install.sh`/`install.ps1`, `.github/workflows/*` + `.github/actions/setup-go-safe`,
`contract/` (build aspects only), `examples/`.

Sibling docs: [00-README.md](00-README.md) · [02-cli-cmd-agt.md](02-cli-cmd-agt.md) ·
[03-control-plane-and-http.md](03-control-plane-and-http.md) · [04-agent-runtime.md](04-agent-runtime.md) ·
[05-governance-routing-security.md](05-governance-routing-security.md) · [06-data-memory-state.md](06-data-memory-state.md) ·
[07-autonomy-and-extensibility.md](07-autonomy-and-extensibility.md) · [08-providers.md](08-providers.md) ·
[09-channels.md](09-channels.md) · [10-tools.md](10-tools.md) · [11-frontend-console.md](11-frontend-console.md) ·
[12-sdks-and-contract.md](12-sdks-and-contract.md)

---

## Responsibilities at a glance

| Area | What it owns |
|---|---|
| `cmd/agezt` (package `main`) | The daemon binary. Subcommands `daemon` (default), `watchdog`, `update`, `version`, `help`. `runDaemon` is the **composition root**: it is the only place that sees provider plugins, tool plugins, channel plugins, guardians, skill bundles and the market catalogue *and* the kernel, so it wires them together (the kernel never imports `plugins/*`). It also mounts the four HTTP surfaces (Web UI, OpenAI-compatible API, REST API, tunnel), starts every resident loop (Pulse, cadence, standing orders, workflow triggers, tickers, webhooks, anomaly breaker, alerter), and runs the graceful drain/suspend shutdown. |
| `cmd/agezt/internal/daemonconfig` | Pure parser: `Load(get, warn) (Config, error)` turns ~75 post-inject `AGEZT_*` env vars into one typed `Config`; decides fatal-vs-warn per var. |
| `internal/brand` | Frozen identity constants (`Name`, `Binary`, `CLI`, `EnvPrefix="AGEZT_"`, `ConfigDir=".agezt"`, `ProtocolVersion=1`) + ldflag-stamped `Version`/`BuildCommit`/`BuildTime`. |
| `internal/paths` | `BaseDir()` = `$AGEZT_HOME` or `<user-home>/.agezt`. Never creates the dir. |
| `internal/atomicfile` | `WriteFile(path, data, mode)`: unique temp + fsync + chmod + rename (Windows remove-then-rename fallback). The single canonical atomic writer for 14 packages. |
| `internal/strutil` | `Ellipsis` (rune-safe truncation), `FirstNonEmpty`, `FirstNonEmptySlice`. |
| `tools/*` | Repo gates: dependency allowlist, deadcode, SDK parity, generated STRUCTURE docs, audit-doc number claims, changelog layout, contract codegen. |
| `scripts/`, `Makefile`, `dev.*`, `install.*` | Build stamping, isolated dev loop (`.dev-home`), e2e smoke harnesses (daemon + Playwright), production installers (systemd / NSSM). |
| `.github/workflows/ci.yml` | 17 gate jobs + 2 aggregator jobs (`CI`, `ci.yml`) required by branch-protection rulesets; all on GitHub-hosted `ubuntu-latest` since 2026-10-01. |

---

## 1. Package `cmd/agezt` (binary `agezt`)

### 1.1 Purpose and position in the graph

- Imports (from `internal-deps.txt`): `cmd/agezt/internal/daemonconfig`, `internal/{brand,paths,strutil}`, ~45 kernel
  packages (agent, alerter, anomaly, artifact, auth, board, bus, cadence(+systemtasks), catalog, channel, channelwire,
  controlplane, creds(+sigv4), edict, event, governor, httpserver, market, memory, netguard, openaiapi, pulse, redact,
  restapi, resume, roster, runtime, selfrepair, settings, skill, standing, state, stt, tenant, toolreg, tunnel, ulid,
  update, warden, webhook, webui, workflow) and the plugin aggregators `plugins/{builtinchannels, builtinguardians,
  builtinmarket, builtinskills, builtintools, providerboot}` + four sidecar adapters `plugins/providers/{embed,image,rerank,voice}`
  + `plugins/tools/codeexec` (banner type assertion only).
- Nothing imports it (it is `main`).
- **Layering rule it enforces by existing:** kernel packages are plugin-free; every plugin→kernel binding is a closure or
  interface value constructed here (e.g. `ChatGPTSync`, `market.Config.Library = builtinmarket.New()`, `builtinguardians.NewKernelHost(k)`).

### 1.2 Subcommands (`run(args)` in `main.go`)

| Invocation | Function | Behaviour |
|---|---|---|
| `agezt` / `agezt daemon` | `runDaemon` | Foreground daemon (section 2). |
| `agezt watchdog` | `runWatchdog` (`watchdog.go`) | Supervisor: re-execs `os.Executable() daemon`, restarts with exponential backoff (1s → 30s cap; reset after 60s uptime), gives up after >6 starts in 2 min. |
| `agezt update [--apply]` | `runUpdate` (`boot_ops.go`) | Control-plane client: `UpdateCheck`, and with `--apply` `UpdateApply(version, sha256, url, notes)`. Requires a running daemon. |
| `agezt version` / `-v` | — | `agezt <Version> (protocol v1)` |
| `agezt help` / `-h` | `printHelp` | Usage + 5 env hints. Unknown command → exit 2. |

### 1.3 File-by-file

| File | What it does |
|---|---|
| `doc.go` | Package comment (subcommands, "daemon starts unconfigured, Setup screen asks for a provider"). |
| `main.go` | `main`, `run`, `printHelp`, **`runDaemon`** (the ~1,670-line boot sequence, section 2), `drainWait` (100 ms poll until `active()==0` or timeout), `defaultChannelHistory=10`, `channelHistoryLimit()` (`AGEZT_CHANNEL_HISTORY`). Ends with an orphaned doc comment for `visionGate` (the function moved to `main_channels.go`). |
| `main_toolinvoker.go` | `openAppKernel`: primary and tenant composition bind `app/tools.NewInvoker` before runtime.Open; each kernel receives its own policy/audit/lookup/hook ports (W2.3m). Standalone runtime callers retain the legacy default. |
| `boot_providers.go` | Phase 2.4 glue: `providerDeps(cat, credStore, baseDir, stderr) providerboot.Deps` (rebuilds the catalog-scoped vault→env→AWS lookup for reloads); `firstRunSetupNeeded(res)` = `res.Primary == providerboot.UnconfiguredName`. |
| `boot_tools.go` | `buildTools(baseDir, stderr, ward, notifyTargets)`: `builtintools.RegisterAll()` → `toolreg.BuildAll(BuildDeps{BaseDir, WorkspaceRoot, Warden, Stderr, Get: os.Getenv, AllowAll, NotifyTargets})` → warns on `set.NetguardGaps()`; returns tool map, `*toolreg.Set`, plugin manifest, per-tool capability map, banner. Also `councilSeatName(i)` ("Elder Alpha/Beta/Gamma", then "Elder N"). |
| `boot_ops.go` | `runUpdate` (CLI) and `startUpdateChecker(ctx, k, svc, …)`: ticker → `svc.Check` → publish `update.available` → `k.DrainAndHalt(svc.DrainTimeout())` → `svc.Apply` → publish `update.applied` → **`os.Exit(0)`**; on failure publishes `update.failed` (KindAnomalyDetected). |
| `api_engine.go` | `kernelAPIEngine{k}` adapts `*runtime.Kernel` to BOTH `openaiapi.Engine` (+`UsageReporter`) and `restapi.Engine`: `RunModel` (WithModel/WithJSONMode/WithImages + `visionGate`), `UsageFor` (governor in-memory index fast path, journal `budget.consumed` fold fallback), `DefaultModel`, `ModelIDs` (catalog), `EventsForCorrelation` (journal range), `ArtifactEntries`/`ArtifactBytes`. `tenantAPIEngine(reg, id)` resolves a tenant kernel via `reg.Acquire`. Kept here on purpose so `kernel/restapi` stays interface-only. |
| `awschain.go` | `buildAWSCredChain(vaultLookup)` composes `creds.ChainLookup`: vault → `os.Getenv` → SSO (`AGEZT_AWS_SSO_PROFILE`) → STS AssumeRole (`AGEZT_AWS_ASSUME_ROLE_*`, signing creds from a separate sub-chain to avoid cache-mutex recursion) → web identity (`AWS_WEB_IDENTITY_TOKEN_FILE`+`AWS_ROLE_ARN`) → `creds.AWSDefaultChain()` (files + IMDSv2). `catalogScopedVaultLookup` refuses legacy bare vault names that are ambiguous across catalog providers. `parseAssumeRoleDurationSeconds` (≤0 → 0 = AWS default). `shortArn`. |
| `maxprocs.go` | Stdlib automaxprocs: `cgroupCPUQuota` (v2 `cpu.max`, then v1 cfs files), `cgroupMaxProcs` (only lowers; ceil; explicit `GOMAXPROCS` wins), `applyAutoMaxProcs` (Linux only). |
| `watchdog.go` | `watchdogPolicy`/`defaultWatchdogPolicy`, `nextDelay`, `proc`/`execProc` seam, `superviseLoop` (testable spawn→wait→backoff loop, `update.sentinel` handling), `ctxSleep`, `runWatchdog`, `resolveBaseDir`. |
| `httpsurfaces.go` | `webUISurface` + **`buildWebUI`**: default-on `127.0.0.1:8787` (falls back to `127.0.0.1:0` if busy), per-boot 256-bit token, `controlplane.NewClient`, `webui.New(bus, client, token)`, allowed hosts, console password (minted per install), strict mode, STT/TTS wiring, `httpserver.Start`, banner URL with token, auto-open browser. |
| `httpsurfaces_apis.go` | `writeAPIListenToken` (→ `kernelauth.WriteTokenFile`, 0600), `buildOpenAIAPI` (`AGEZT_API_ADDR`, `openai.token`), `buildRESTAPI` (`AGEZT_REST_ADDR`, `rest.token`, mailbox = shared board, update service, `/readyz` via `draining`+`IsHalted`, `/metrics` via `restMetrics`, tenant resolver/authorizer), `buildWebhooks` (`AGEZT_WEBHOOKS`, netguard-guarded HTTP client), `isLoopback`, `restMetrics` (16 Prometheus gauges). |
| `httpsurfaces_browser.go` | `shouldOpenWebUI` (`AGEZT_WEB_OPEN`; never under `*.test`), `openBrowser` (rundll32 / open / xdg-open), `webAllowedHosts` (bind host unless unspecified + `AGEZT_WEB_ALLOWED_HOSTS`). Ends with `buildTunnel`'s orphaned doc comment. |
| `httpsurfaces_pwd.go` | `envDisabled` (off/disabled/none/no/0/false), `bannerColor`/`bannerColorEnabled` (`NO_COLOR`), `consolePasswordFile="web-password"`, `consolePasswordBytes=12` (96-bit), `ensureConsolePassword` (read-or-mint via `kernelauth.ReadTokenFile/WriteTokenFile`), `webPasswordDefaultDisabled`, `effectiveWebPassword` (env > opt-out > builtin-on-loopback-only). |
| `httpsurfaces_tunnel.go` | `buildTunnel` (`AGEZT_TUNNEL` preset or `AGEZT_TUNNEL_CMD`; `tunnel.New(cfg)`; `OnURL` allowlists the public host on the Web UI and prints the auth posture), `tunnelTargetFromEnv`, `sameURLTarget`, `publicURLHost`, `tunnelPublicURL` (adds `?token=` only when no password or strict), `urlWithToken`, `addrToURL`. Ends with `writeAPIListenToken`'s orphaned doc comment. |
| `main_cadence.go` | `buildCadence(ctx, k, stdout, onAnswer)`: syncs `AGEZT_SCHEDULE` jobs into the store (`store.SyncEnv`), builds the per-fire `run` closure (agent resolution + 4 targets: workflow / system_task / tool / intent; assured / retry / plain run), `cadence.NewEngine`, `eng.RunTimeout` (`AGEZT_SCHEDULE_RUN_TIMEOUT`, default 1h), `k.SetScheduleEngine`, `eng.Start`. |
| `main_cadence_helpers.go` | `scheduledRunContext` (profile + per-run cost + model pin as a 1-element chain), `scheduleFiredEventPayload` (executor/uses_llm/system-task metadata), `agentAutonomyRunbookPayload` (→ `roster.AutonomyRunbook`). |
| `main_cadence_runners.go` | `runScheduledTrackedTarget`: publishes `task.received`/`task.failed`/`task.completed` on subject `schedule.task` around non-LLM targets so they appear as runs; `schedulePayloadForEntry`, `controlplaneScheduleAction`, `truncateScheduledAnswer` (4096-byte raw slice). |
| `main_channels.go` | `visionGate`/`gateVisionWith` (reject images unless catalog says model supports vision), `voiceReplyEnabled` (`AGEZT_VOICE_REPLY`), `audioExt`, `extForMime`, `splitNonEmpty`. |
| `main_channels_handler.go` | `makeChannelHandler(k)`: the ONE `channel.InboundHandler` for every channel: prepend `channel.ConversationHistory` transcript; images → vision gate, else vision **sidecar** (`k.DescribeImages`) caption injection; voice notes → `k.Voice().Transcribe`; `k.RunWith`; voice-in → TTS voice-out attachment. `persistInboundAudio`/`persistInboundImages` (artifact index entries keyed to corr), `decodeDataURL`. |
| `main_channels_lifecycle.go` | `collectChannels()` (configured channels from manifests for `agt status`; webhook special-case `AGEZT_WEBHOOK_OUTBOUND_URL`), `combineSinks`, `chanInstance`, `wireInstances`, `startInstances` (`go ch.Start(ctx)` + banner line), `instanceSinks`, `registerInstances`, `instanceMatch` (`kind` fans out to every `kind#label`; exact key hits one), `liveChannelKeys`, `briefSink` (log sink + channels). |
| `main_overlay.go` | `replayPolicyOverlay(k)`: folds journal `policy.changed` events, trusting `runtime/edict_overlay_snapshot.json` only when its content hash equals the last journaled `policy.compacted` hash; `overlaySnapshotPath`. |
| `main_overlay_anomaly.go` | `buildAnomaly` (tool-call-rate breaker → `k.HaltWith`), `buildAlertNotify` (`alerter.Start` to channel sinks), `standingTrustCeiling` (min of `max_trust` and the initiative mode's trust; act-or-ask → L1 AskFirst). |
| `main_overlay_redact.go` | `modelAdvisory` (catalog `AgentWarnings`), `credSecrets` (every vault value + `AGEZT_REDACT_EXTRA`), `extraRedactLiterals` (`;`-separated). |
| `main_overlay_scan.go` | Orphan-run reconciliation: `orphanRun`, `runScan` (received vs completed/failed/abandoned), `reconcileOrphanRuns` publishes `task.abandoned` for each orphan. |
| `main_pulse.go` | Tickers on the daemon ctx: `startReflectTicker` (`AGEZT_REFLECT_EVERY`), `startWorkboardSweepTicker` (`AGEZT_WORKBOARD_SWEEP_EVERY`, `_STALE_AFTER` default 10m, ≤100 claims/sweep), `startBrainDistillTicker` (`AGEZT_BRAIN_DISTILL_EVERY`), `startProfileDistillTicker` (default 24h, `AGEZT_USER_PROFILE_EVERY`); `onOff`. |
| `main_pulse_engine.go` | `bootStep{name, run, fatal}` (the late boot table type), `pulseObserverAdmin` (runtime disk/probe watch registration for the control plane), `buildPulse` (cadence/dial/quiet-hours/probe/disk/self-health observers, LLM toggle, initiative mode), `healthStatFromJournal` (last 2000 events → tool error rate, failed runs). |
| `main_pulse_misc.go` | `netguardPublish` (→ `netguard.blocked` events), `voiceTranscriberShim`, `sttTranscriberFromEnv` (`AGEZT_STT_API_KEY`/`OPENAI_API_KEY`, `AGEZT_STT_API_URL`), `selectAskPolicy` (`AGEZT_APPROVAL_MODE`), `wardenOptionsFromEnv` (`AGEZT_WARDEN_DOCKER*`), `selectAutoApproveCapabilities` (`AGEZT_AUTO_APPROVE_CAPS`); `var _ = event.GenesisHash` (import keeper). |
| `main_pulse_artifacts.go` | `wireArtifactIndexer` (bus `>` subscriber: `tool.result` with `raw_ref` → artifact index entry), `boardSubjectSlug`, **`injectConfig`** (Config Center bridge), `workspaceRoot` (`AGEZT_WORKSPACE` or `<base>/workspace`). |
| `main_standing.go` | `buildStandingRunner(ctx, k, brief)`: `fire` closure (panic-contained; agent resolution incl. managed-sub-agent refusal; `standing.ScopedIntent`/`TriggeredIntent`; `standing.fired` event with autonomy runbook; cost + trust ceiling; assured / retry / plain run; `standing.BriefText` → channel), `fireNow` (manual), `standing.StartRunner` (events) + `standing.StartCron`. |
| `main_standing_delivery.go` | `delegationBanner(k)`, `deliverScheduled` (sorted fan-out of `[scheduled: id]` answers). Holds `buildCadence`'s orphaned doc comment. |
| `main_standing_resume.go` | `buildResumer(ctx, k)`: durable run resume (section 2.6). |

Stray non-Go file: `WIP-BOOT-TOOLS-EXPANSION.md` (dated 2026-07-12) describes a hand-wired `buildTools` restore; it is **stale** — tool wiring is now entirely registry-driven (`toolreg` + `plugins/builtintools`).

Tests (group): `main_test.go` (run/help/version, instance matching, warden options, delivery, collectChannels, channel
handler, banners, `runScan`, `drainWait`, board slug), `boot_providers_test.go` (first-run nudge polarity),
`awschain_test.go`, `consolepassword_test.go`, `httpsurfaces_test.go`, `maxprocs_test.go`, `watchdog_test.go`,
`standing_ceiling_test.go`, `vision_gate_test.go`, `schedule_system_task_test.go`, `dataurl_test.go`,
`coverage_*_test.go` (coverage padding for builders/helpers/media).

---

## 2. The daemon boot sequence (`runDaemon`, exact order)

Legend: **F** = fatal (exit 1), **W** = warn and degrade, **B** = best-effort.

```
 0  applyAutoMaxProcs()                                  cgroup quota → GOMAXPROCS (Linux only)
 1  paths.BaseDir()                                      $AGEZT_HOME | ~/.agezt                         F
 2  controlplane.ProbeExisting(base)  ── live? ──► refuse unless AGEZT_FORCE_START=1 (read PRE-inject)  F
 3  catalog.NewStore(base/catalog).Load()   (validate only)                                             F
 4  creds.NewStore(base).Load()  → EncryptInPlace()  (machine-bound AES-256-GCM upgrade)               F / W
 5  injectConfig(base, vault)   settings.Registry pins + settings.Store(config.json) + vault AGEZT_* → os.Setenv
 6  daemonconfig.Load(nil, stderr)  → dcfg                                                             F (historic fatals)
 7  providerboot.SeedChatGPTCatalog(catStore, base); cat := catStore.Load()
 8  credLookup := buildAWSCredChain(catalogScopedVaultLookup(cat, vault.Lookup))
 9  providerboot.Boot{Catalog, Lookup, BaseDir} → Governor (gov), model, Primary                        F
10  warden.NewWithOptions(nil, wardenOptionsFromEnv())          (bus attached later)
11  edict.New{AskPolicy(APPROVAL_MODE), HardDeny(default + EDICT_DENY), [ALLOW_ALL ⇒ all L4 + UnknownAllow]}
12  builtinchannels.RegisterAll()                     (channel manifest registry populated)
13  notifyTargets: telegram/slack/discord with RequiredEnv set AND non-empty AllowlistEnv
14  buildTools(...) → tools map + toolSet (registry specs built, unbound)                               F
15  sidecars: embed.New / voice.NewSTT / voice.NewTTS / image.New / rerank.New  (each W on misconfig)
16  kernelruntime.Config{...}; toolSet.ApplyPreOpen(&cfg)  (code_exec → cfg.ScriptRunner)
    run-loop caps, redactor (credSecrets), closures: OnReload, VisionModel, ModelAvailable, CouncilMembers
17  k = kernelruntime.Open(cfg)   ── opens journal/state/bus/stores (see §4) ──                         F
    defer k.Close()
18  gov.SetBus(k.Bus()); ward.SetBus(k.Bus())          MUST precede any Run
19  Forge toggles: SetAutoQuarantine(0,0) / SetAutoShadow(true) / SetAutoPromote(0,0) per dcfg
20  toolSet.Configure(toolreg.KernelDeps{K, Bus, Artifacts, Lake, Journal, BaseDir, Stdout, NetguardPublish})  F
21  k.Bus().SetRedactor(redactor)                      before any Run, so nothing is journaled unscrubbed
22  [EDICT_DURABLE=on] replayPolicyOverlay(k) → k.Edict().ApplyOverlay                                 F
23  reconcileOrphanRuns(k)  → task.abandoned for every received-but-unterminated run                  F
24  ctx, cancel := context.WithCancel(...)   — THE daemon context
25  wireArtifactIndexer(ctx, k)
26  board.Open(base/board) + boardNotify        (ONE shared store; failure ⇒ board surfaces nil)       W
27  cancelOnDisconnect, update.Service (endpoint | GitHub), httpBindings (WEB/REST/API addrs), 
    [MULTITENANT=on] tenant.New(base/tenants, opener) ; defer reg.CloseAll()                           F (tenant)
28  srv := controlplane.NewServerWithDeps(k, base, Deps{ConfigEnvPinned, Board, BoardNotify, DiskFree,
        UpdateSvc, Tenants, CancelOnDisconnect, HTTPBindings, CredChain, Channels: collectChannels(),
        ChatGPTSync}) ; srv.Start(ctx) ; defer srv.Stop()                                            F
    ── control plane is now accepting connections (runtime/control.addr + control.token written) ──
29  banner: "Agezt <ver> (commit=…, built=…) — daemon ready (protocol v1)" + ~25 status lines
30  chanHandler := makeChannelHandler(k)
    for m in channel.Manifests(): channelwire.BuildKind(ctx, m.Kind, bus, chanHandler) → go ch.Start(ctx)
    channelSinks := combineSinks(every instance's brief sink)
31  buildPulse(...) → eng.Start(ctx); srv.Bind(LateDeps{Pulse, Observers}); eng.AddObserver(Reaper, 30d)
32  tickers: reflect / workboard sweep / brain distill / profile distill
33  buildWebUI(ctx, k, base)       (default ON, 127.0.0.1:8787)
34  buildTunnel(ctx, webSurface)   (opt-in)
35  buildOpenAIAPI(ctx, k, reg)    (AGEZT_API_ADDR)
36  buildWebhooks(ctx, k)          (AGEZT_WEBHOOKS)
37  buildAnomaly(ctx, k)           (default ON: >120 tool calls / 10 s ⇒ HaltWith)
38  buildAlertNotify(ctx, k, channelSinks)  (AGEZT_ALERT_NOTIFY)
39  buildRESTAPI(ctx, k, reg, &draining, board, boardNotify, updateSvc)  (AGEZT_REST_ADDR)
40  liveChannels map ← every instance key; channel.SetLive / SetLiveInstances;
    channelSend / channelSendMedia closures; srv.Bind(LateDeps{ChannelSend})
41  bootSteps table (only #1 fatal):
      1 late-configure tools: toolSet.ConfigureLate(LateDeps{KernelDeps, ChannelSend, ChannelSendMedia, Board, BoardNotify})  F
      2 schedule tool banner      3 board tool banner/failure
      4 skill tool: builtinskills.SeedAll(forge) + k.SetMarket(market.NewManager{CompositeLibrary(builtinmarket.New(), market.Store), …})
      5 introspect  6 overseer  7 builtinguardians.SeedAll(NewKernelHost(k))  8 code_exec  9 tool_forge
     10 workflow   11 workboard  12 k.AttachEnabledMCPServers(ctx)
42  buildCadence(ctx, k, onScheduledAnswer[SCHEDULE_NOTIFY])
43  [updateSvc.CheckInterval>0] go startUpdateChecker(...)
44  buildStandingRunner(ctx, k, standingBrief) ; srv.Bind(LateDeps{StandingFire})
45  buildResumer(ctx, k)                                  (M1002 resume of interrupted runs)
46  selfrepair.WireAutoRepair(ctx, k, base, board, boardNotify)
47  workflow.StartTriggers(ctx, bus, k.Workflows(), RunnerConfig{}, wfFire[15 min timeout])
48  bus.Subscribe(">") → stdout event log (skips llm.token / llm.reasoning)
49  block on SIGINT/SIGTERM  or  <-srv.Shutdown()  (agt shutdown)
50  shutdown (§2.7)
```

### 2.1 Dependency rules embedded in the order

- **Config before everything env-driven.** `injectConfig` (step 5) must precede `daemonconfig.Load`, `providerboot.Boot`,
  channel construction and `buildTools` because all of them read `os.Getenv`. Real env wins; the Config Center store
  and AGEZT_*-named vault secrets only fill gaps (`os.Getenv(name)=="" ⇒ os.Setenv`). `configPinned` (schema fields set
  in the real env) goes to the control plane so the Config Center renders them read-only. `AGEZT_CONFIG=off` skips
  injection but still computes pins.
- **Channel registry before tools.** `builtinchannels.RegisterAll()` must run before `notifyTargets` (derived from
  manifests) and before `collectChannels()` feeds `controlplane.Deps.Channels`.
- **Tool map is frozen before Open.** `notify`/`send_media` specs gate on `notifyTargets` at build time so the tool map
  is complete before the kernel starts reading it (comment: a later write would be a fatal concurrent-map race). Tools
  are built *unbound* and bound in three registry phases on the same `*toolreg.Set`:
  `ApplyPreOpen(&cfg)` → `Configure(KernelDeps)` (post-Open) → `ConfigureLate(LateDeps)` (after live channels + board).
- **Bus wiring before any run:** `gov.SetBus`, `ward.SetBus`, `toolSet.Configure` (netguard audit), `SetRedactor`,
  durable-policy replay and orphan reconciliation all happen before the control plane accepts connections.
- **Forward-declared kernel:** `var k *kernelruntime.Kernel` is captured by `cfg.OnReload`, `cfg.VisionModel`,
  `cfg.ModelAvailable`, `cfg.CouncilMembers`; each nil-checks `k` and re-derives the keyed provider set from the LIVE
  catalog (`k.Catalog()`) + `providerboot.Eligible`, so newly-keyed providers are used without restart.
- **Control plane is constructed in one shot** (`NewServerWithDeps`, "Phase 2.6 3b-ii") so tenant tokens authorize from
  the first accepted connection; only `Pulse`/`Observers`, `ChannelSend`, `StandingFire` arrive later via `srv.Bind(LateDeps)`.
- **Defer order on exit** (LIFO): `sub.Cancel` → `srv.Stop` → `reg.CloseAll` (tenants) → `cancel` → `k.Close`. The tenant
  `CloseAll` is deliberately registered *before* `srv.Stop` so the server stops accepting before tenant kernels close.

### 2.2 How each subsystem family is registered

| Family | Registration point | Mechanism |
|---|---|---|
| Providers / models | step 9 (`providerboot.Boot`), reload via `cfg.OnReload` → `providerboot.Reload(gov, providerDeps(...))` + `k.SetModel` | Governor holds the registry; unconfigured boot resolves to sentinel primary `providerboot.UnconfiguredName` (banner prints "setup needed"). ChatGPT subscription provider seeded into the catalog by `SeedChatGPTCatalog`, refreshed post-sign-in through `Deps.ChatGPTSync`. See [08-providers.md](08-providers.md). |
| Sidecar model services | step 15 → `runtime.Config.{MemoryEmbedder, Voice, ImageGenerator, Reranker}` | nil ⇒ the runtime does not register `voice` / `image_generate` / `rerank` tools. Typed-nil avoidance for `Voice`. |
| Tools | steps 14, 16, 20, 41.1 | `builtintools.RegisterAll` + `toolreg.BuildAll`; the kernel adds `memory`, `world`, `delegate`, `delegate_await`, `market`, `voice`, `image_generate`, `rerank` itself during `Open`. See [10-tools.md](10-tools.md). |
| Channels | step 12 (manifests), step 30 (instances) | `channelwire.BuildKind` per manifest (multi-account `kind#label` instances). One shared inbound handler. See [09-channels.md](09-channels.md). |
| Skills | step 41.4 | `builtinskills.SeedAll(forge, "")` — idempotent, content-addressed. |
| Market | step 41.4 | `k.SetMarket(market.NewManager{Library: NewCompositeLibrary(builtinmarket.New(), Store), Store: market.NewStore(base), Skills: forge, MCP: k, Verify: market.VerifyPack, Syncer: market.NewSyncer()})`. |
| Guardians | step 41.7 | `builtinguardians.SeedAll(builtinguardians.NewKernelHost(k), "")` — System-marked agents, idempotent by slug, operator removals respected. |
| MCP servers | step 41.12 | `k.AttachEnabledMCPServers(ctx)`; per-server failures printed, never fatal. |
| Standing orders | step 44 | `standing.StartRunner` (bus) + `standing.StartCron`; `fire` = governed run AS roster agent with cost + trust ceilings. |
| Schedules | step 42 | `cadence.NewEngine(store, run, 0, stdout)` with `Bus` (injection tripwire) and `RunTimeout`. |
| Workflows | step 47 | `workflow.StartTriggers` (cron/event/webhook; consults the store live). |
| Pulse | step 31 | `pulse.New(Config{Bus, State, Warden, Provider: k.Provider(), Model, Relevance: k.World(), Observers, Dial, Initiative, Cadence, QuietHours, UseLLM, Sink})`. |

### 2.3 HTTP surfaces mounted by the daemon

| Surface | Env | Default | Auth | Token storage | Notes |
|---|---|---|---|---|---|
| Control plane (TCP) | — | loopback, random port | `control.token` | `<base>/runtime/control.addr`, `control.token` | Owned by `kernel/controlplane` ([03](03-control-plane-and-http.md)). `agt` and the Web UI's proxy use `controlplane.NewClient(base)`. |
| Web UI | `AGEZT_WEB_ADDR` | **ON** at `127.0.0.1:8787`; busy ⇒ `127.0.0.1:0`; `off`/… disables | per-boot 64-hex token in URL **or** console password (strict mode ⇒ token AND password) | token not persisted (printed in banner); built-in password `<base>/web-password` (0600, minted once, printed once) | Strict mode auto-raised for wildcard binds and (inside `SetAllowedHosts`) for non-loopback hosts; `AGEZT_WEB_PASSWORD_STRICT` overrides only when set. STT from `k.Voice()` else `sttTranscriberFromEnv()`; TTS from `k.Voice()`. |
| OpenAI-compatible API | `AGEZT_API_ADDR` | off | Bearer | `<base>/openai.token` (0600; banner shows prefix only) | `/v1/chat/completions`, `/v1/responses`, `/v1/models`, `/v1/audio/transcriptions` (if STT env). Tenant routing via `X-Agezt-Tenant`. |
| Native REST API | `AGEZT_REST_ADDR` | off | Bearer | `<base>/rest.token` | `/api/v1/*`, mailbox, update, `/healthz`, `/readyz` (draining / halted), `/metrics`. |
| Tunnel | `AGEZT_TUNNEL` / `_CMD` / `_TARGET` | off | inherits target | — | Supervised cloudflared/ngrok/tailscale(-funnel)/custom; `OnURL` adds the public host to Web UI allowed hosts. |
| Outbound webhooks | `AGEZT_WEBHOOKS` | off | HMAC per sink | — | `netguard`-guarded client (loopback/private opt-in). |

All HTTP servers use `httpserver.Start(ctx, ln, handler, onErr)` and stop when the daemon ctx is cancelled.

### 2.4 Event-bus traffic originated in `cmd/agezt`

| Kind / subject | Where |
|---|---|
| `board.posted` on `board.<topic>` / `board.dm.<slug>` / `board.help[.<slug>]` / `board.broadcast` | `boardNotify` (shared by board tool, control plane, REST mailbox) |
| `task.abandoned` (subject `task`) | `reconcileOrphanRuns` |
| `schedule.fired`; `task.received/failed/completed` (subject `schedule.task`) | `buildCadence`, `runScheduledTrackedTarget` |
| `standing.fired`, `standing.error` (subject `standing.<id>`) | `buildStandingRunner` |
| `run.resumed` (KindInfo), `run.resume.quarantined` (KindAnomalyDetected) | `buildResumer` |
| `netguard.blocked` (subject `netguard.block`) | `netguardPublish` |
| `update.available` / `update.applied` (KindInfo), `update.failed` (KindAnomalyDetected) | `startUpdateChecker` |

Consumed: `tool.result` (artifact indexer), `>` (stdout echo), journal folds over `policy.changed`/`policy.compacted`,
`task.*`, `budget.consumed`, `tool.invoked/result` (health stat).

### 2.5 Concurrency model

- Main goroutine runs boot then blocks in `select` on signals / `srv.Shutdown()`.
- Goroutines spawned here: stdout event echo; artifact indexer; each channel instance `Start(ctx)`; four optional tickers;
  `tunnel.Start`; update checker; one goroutine per resumed run (panic-contained) and per manual standing fire;
  `cadence`, `pulse`, `standing`, `workflow`, `anomaly`, `alerter`, `webhook` loops run inside their packages on `ctx`.
- Shared state: `draining atomic.Bool` (REST readiness vs shutdown); the board store is a single instance by invariant;
  `liveChannels` is built once and only read afterwards.

### 2.6 Resume vs orphan reconciliation (M1002 / M28)

1. Shutdown calls `k.Suspend("shutdown")` *before* the drain so a run still cancelled at the end keeps its resume ticket.
2. Next boot, step 23 `reconcileOrphanRuns` publishes `task.abandoned` for every received-but-unterminated correlation
   (it does not consult the resume store).
3. Step 45 `buildResumer` lists `k.ResumeStore()` tickets: non-resumable / `Attempts >= AGEZT_RESUME_MAX_ATTEMPTS`
   (default 3) / agent gone-retired-disabled ⇒ `store.Quarantine` + `run.resume.quarantined`. Otherwise
   `IncrementAttempt` (fsynced before dispatch), rebuild ctx (`WithResumeOwned`, profile, `WakeContext{Source:"resume"}`,
   re-applied `TrustCeiling`, `MaxCost`, `RunTimeout`, `WithResumeSeed(messages, iter)` for `KindRun`), publish
   `run.resumed`, dispatch `RunAssured` / `RunWithRetry` / `RunWith` under the **same correlation**, then
   `k.ResumeFinalize(corr, err)`.
   Consequence: a resumed run's journal arc contains `task.abandoned` followed by fresh activity on the same corr.

### 2.7 Shutdown

```
signal / agt shutdown
  → draining.Store(true)                      /readyz = "draining"
  → k.Suspend("shutdown")                     mark in-flight runs resumable
  → drainWait(k.ActiveRuns, AGEZT_DRAIN_TIMEOUT[15s, live env read])
  → cancel()                                  stops every ctx-bound loop + HTTP server
  → k.Halt() loop up to 2 s until k.IsHalted()
  → return 0 → defers: sub.Cancel, srv.Stop (removes control.addr/token), reg.CloseAll, k.Close
```

### 2.8 Watchdog and self-update

- `superviseLoop` respawns `agezt daemon` with inherited env/stdio; policy: base 1s, cap 30s, reset after 60s uptime,
  crash loop = >6 starts within 2 min ⇒ exit 1. On ctx cancel it kills and reaps the child.
- `update.sentinel` (`<base>/update.sentinel`) resets the crash counter when present. **Nothing in the tree writes this
  file** (only `watchdog.go` references it; `kernel/update` mentions it in a comment), so the intentional-restart path is
  dead in practice.
- `resolveBaseDir("")` is always called with an empty argument, so the watchdog ignores `AGEZT_HOME` and uses
  `$HOME/.agezt`; with `$HOME` unset (typical on Windows) `agezt watchdog` fails with "$HOME is empty".
- `startUpdateChecker` ends a successful apply with `os.Exit(0)`, bypassing all deferred cleanup (`srv.Stop` never
  removes `runtime/control.addr`/`control.token`; `k.Close` is skipped). A failed apply after `DrainAndHalt` leaves the
  daemon running but halted.

---

## 3. Environment variables read by `cmd/agezt` and `daemonconfig`

Column "When": **pre** = before `injectConfig`; **Load** = captured once by `daemonconfig.Load` after injection;
**boot** = read inline in `cmd/agezt` at boot after injection; **live** = re-read at call time (Config Center
`os.Setenv` edits apply without restart). Booleans: "≠off" means on unless the value is `off` (case-insensitive).

### 3.1 Process / bootstrap

| Var | When | Default | Meaning |
|---|---|---|---|
| `AGEZT_HOME` | pre | `~/.agezt` | Base dir (`internal/paths`). |
| `AGEZT_FORCE_START` | pre | unset | `1` starts despite a live daemon on the same base dir. |
| `AGEZT_CONFIG` | pre | on | `off` disables Config Center store + vault env injection. |
| `AGEZT_VAULT_AUTOENCRYPT`, `AGEZT_VAULT_PASSPHRASE` | pre (in `kernel/creds`) | auto-encrypt on | Vault at-rest encryption (named in the banner via `creds.AutoEncryptEnvVar`/`PassphraseEnvVar`). |
| `GOMAXPROCS` (non-AGEZT) | pre | — | Explicit value disables cgroup auto-tuning. |
| `NO_COLOR` (non-AGEZT) | boot | — | Disables ANSI banner colors. |

### 3.2 `daemonconfig.Load` (typed `Config`)

| Var | Field | Default | Parse / failure mode |
|---|---|---|---|
| `AGEZT_ALLOW_ALL` | `Policy.AllowAll` | off | `== "1"`; every capability L4 + `UnknownAllow`; loud stderr WARNING. Also re-read raw by `buildTools` for network tools. |
| `AGEZT_EDICT_DENY` | `Policy.EdictDeny` | none | `edict.ParseDenyRules`; malformed ⇒ **fatal**. |
| `AGEZT_EDICT_DURABLE` | `Policy.EdictDurable` | off | `on` ⇒ replay journaled policy at boot. Tenant opener re-reads it **live**. |
| `AGEZT_APPROVAL_TIMEOUT` | `Policy.ApprovalTimeout` | kernel 5m | duration; malformed **fatal**; ≤0 ⇒ default. |
| `AGEZT_MEMORY` | `Knowledge.Memory` | on (≠off) | Memory inject + tool + distill. |
| `AGEZT_MEMORY_DISTILL_MIN_TOOLS` | `…MemoryDistillMinTools` | 6 | bad/≤0 silently default. |
| `AGEZT_USER_PROFILE` | `…UserProfile` | on (requires memory) | Operator-profile injection + daily synthesis ticker. |
| `AGEZT_TASTE_INJECT` | `…TasteInject` | on | |
| `AGEZT_WORLDMODEL` | `…WorldModel` | on | World inject + `world` tool. |
| `AGEZT_SKILLS` / `AGEZT_FORGE` | `…Skills` / `…Forge` | on / on | Skill injection / post-run draft proposal. |
| `AGEZT_SKILL_SHADOWEVAL` | `…SkillShadowEval` | off (`on`) | Extra provider calls per run. |
| `AGEZT_SKILL_AUTOQUARANTINE` | `…SkillAutoQuarantine` | on | off ⇒ `Forge.SetAutoQuarantine(0,0)`. |
| `AGEZT_SKILL_AUTOSHADOW` | `…SkillAutoShadow` | off (`on`) | |
| `AGEZT_SKILL_AUTOPROMOTE` | `…SkillAutoPromote` | on | |
| `AGEZT_RUN_TIMEOUT` | `RunLoop.RunTimeout` | off | duration; malformed **fatal**; ≤0 off. |
| `AGEZT_MAX_ITER` | `RunLoop.MaxIter` | `agent.DefaultMaxIter` | positive int; else **fatal**. |
| `AGEZT_MAX_AUTO_CONTINUE` | `RunLoop.MaxAutoContinue(+Set)` | `agent.DefaultMaxAutoContinue` | any int (negative disables); malformed **fatal**. |
| `AGEZT_AUTO_CONTINUE_WAIT` | `RunLoop.AutoContinueWait` | 0 (kernel default) | non-negative duration; else **fatal**. |
| `AGEZT_PARALLEL_TOOLS` | `RunLoop.MaxParallelTools` | agent default | positive int; else **fatal**. 1 = sequential. |
| `AGEZT_TOOL_DISCOVERY_MAX` | `RunLoop.ToolDiscoveryMax` | 0 (off) | non-negative int; else **fatal**. |
| `AGEZT_TOOL_TIMEOUT` | `RunLoop.ToolTimeout` | off | duration; malformed **fatal**. |
| `AGEZT_SUBAGENT` | `SubAgents.Enabled` | on | `delegate`/`delegate_await` tools. |
| `AGEZT_SUBAGENT_DEPTH` | `…Depth` | 3 | bad ⇒ default. |
| `AGEZT_SUBAGENT_FANOUT` | `…Fanout` | 0 (unbounded) | |
| `AGEZT_SUBAGENT_SPEND_CAP` | `…SpendCapMicrocents` | 0 (unbounded) | USD × 1e9; malformed/negative **fatal**. |
| `AGEZT_SUBAGENT_MAX_TOTAL` | `…MaxTotal` | 0 ⇒ **48 when depth > 1** | M843 tree bound. |
| `AGEZT_ARTIFACT_THRESHOLD` | `Context.ArtifactThreshold` | kernel default | bytes; bad ⇒ warn. |
| `AGEZT_CONTEXT_BUDGET` | `Context.Budget` / `BudgetAuto` | off | chars or `auto` (catalog context window); bad ⇒ warn. |
| `AGEZT_CONTEXT_PROTECT_FIRST` | `Context.ProtectFirst` | 0 | bad ⇒ warn. |
| `AGEZT_CONTEXT_SUMMARIZE` | `Context.Summarize` | off | `== "1"`. |
| `AGEZT_OBSERVATION_DELTAS`, `AGEZT_EPISTEMIC_ESCALATION`, `AGEZT_INTENT_REGRET_GATING`, `AGEZT_DISABLE_HEURISTIC_BYPASS` | `Guards.*` | off | `on` or `1`. |
| `AGEZT_PROMPT_INJECTION_GUARD` | `Guards.PromptInjectionGuard` (raw) | warn mode | `runtime.ParsePromptInjectionMode`: `on`/`block` ⇒ HITL, `off`/`0` ⇒ none. |
| `AGEZT_EMBED_URL/_MODEL/_KEY` | `Sidecars.Embed` | off | URL without model ⇒ warn, disabled. |
| `AGEZT_STT_PROVIDER/_URL/_MODEL/_KEY` | `Sidecars.STT` | off | `elevenlabs`/`deepgram`/`cartesia` need no URL; OpenAI-compatible needs URL+model. |
| `AGEZT_TTS_PROVIDER/_URL/_MODEL/_KEY/_VOICE` | `Sidecars.TTS` | off | as STT. |
| `AGEZT_IMAGE_URL/_MODEL/_KEY` | `Sidecars.Image` | off | `image_generate` tool. |
| `AGEZT_RERANK_URL/_MODEL/_KEY` | `Sidecars.Rerank` | off | `rerank` tool. |
| `AGEZT_SYSTEM_PROMPT` | `Misc.SystemPrompt` | "" | raw, untrimmed. |
| `AGEZT_REDACT` | `Misc.Redact` | on | Journal secret scrubbing. |
| `AGEZT_ENV_INJECT` | `Misc.EnvInject` | on | Host OS/shell/workspace preamble. |
| `AGEZT_COUNCIL_WEBSEARCH` | `Misc.CouncilWebSearch` | on | |
| `AGEZT_SCHEDULE_NOTIFY` | `Misc.ScheduleNotify` | off | exact `on` (case-sensitive, unlike the other switches). |
| `AGEZT_WEBHOOK_CHANNELS` | `Misc.WebhookChannels` | none | Standing-order briefing allowlist for the `webhook` channel. |
| `AGEZT_TOOLFORGE_AUTO_PROMOTE` | `Misc.ToolforgeAutoPromote` | on | |
| `AGEZT_MULTITENANT` | `Tenancy.Multitenant` | off (`on`) | |
| `AGEZT_TENANT_DAILY_CEILING` | `Tenancy.DailyCeiling*` | inherit primary | USD; parsed only when multitenant; bad **fatal**. |
| `AGEZT_TENANT_RATE_PER_MIN` | `Tenancy.RatePerMin*` | unlimited | parsed only when multitenant; bad **fatal**. |
| `AGEZT_RESUME` | `Lifecycle.Resume` | on | Durable resume tickets. |
| `AGEZT_RESUME_SNAPSHOT_MAX_BYTES` | `Lifecycle.ResumeSnapshotMaxBytes` | package default | |
| `AGEZT_CANCEL_ON_DISCONNECT` | `Lifecycle.CancelOnDisconnect` | off (`on`) | |
| `AGEZT_UPDATE_ENDPOINT` | `Lifecycle.UpdateEndpoint` | "" | Enables `update.SourceEndpoint`. |
| `AGEZT_UPDATE_DRAIN_TIMEOUT` | `…UpdateDrainTimeout` | 30s | endpoint source only (GitHub source hardcodes 30s). |
| `AGEZT_UPDATE_CHECK_INTERVAL` | `…UpdateCheckInterval` | 0 (check-only) | >0 arms auto-apply checker. |
| `AGEZT_UPDATE_GITHUB_OWNER` / `_REPO` | `…UpdateGitHub*` | "" / `agezt` | |

### 3.3 Read inline in `cmd/agezt`

| Var | When | Default | Meaning |
|---|---|---|---|
| `AGEZT_APPROVAL_MODE` | boot | `allow` | `deny` / `prompt`(`ask`) / `allow`; unknown ⇒ allow + banner note. |
| `AGEZT_AUTO_APPROVE_CAPS` | boot | `all` (every known capability) | `off`… or comma list (unknown names ignored). |
| `AGEZT_WARDEN_DOCKER` (+`_RUNTIME`, `_IMAGE`, `_NETWORK`) | boot | off (`docker`, `python:3.12-slim`, `none`) | Container isolation backend for the warden. |
| `AGEZT_REDACT_EXTRA` | boot (+ OnReload) | none | `;`-separated literal secrets. |
| `AGEZT_WORKSPACE` | boot | `<base>/workspace` | File/shell tool root. |
| `AGEZT_AWS_SSO_PROFILE` | boot (+ each closure call) | off | SSO layer. |
| `AGEZT_AWS_ASSUME_ROLE_ARN` / `_DURATION_SECONDS` / `_SESSION_NAME` / `_EXTERNAL_ID` | boot | off | STS AssumeRole layer. |
| `AGEZT_WEB_ADDR` | boot | `127.0.0.1:8787` | `off`/`disabled`/`none`/`no`/`0`/`false` disable. Also read for `httpBindings` and tunnel target. |
| `AGEZT_WEB_OPEN` | boot | on | Auto-open browser. |
| `AGEZT_WEB_PASSWORD` | **live** (per auth decision) | built-in (loopback only) | Console password. |
| `AGEZT_WEB_PASSWORD_DEFAULT` | boot + live | on | Opt-out keyword disables the minted built-in password. |
| `AGEZT_WEB_PASSWORD_STRICT` | boot | auto | `on` ⇒ token AND password; only applied when set. |
| `AGEZT_WEB_ALLOWED_HOSTS` | boot | none | Extra Host headers accepted. |
| `AGEZT_API_ADDR` / `AGEZT_REST_ADDR` | boot | off | OpenAI API / REST API bind. |
| `AGEZT_TUNNEL` / `AGEZT_TUNNEL_CMD` / `AGEZT_TUNNEL_TARGET` | boot | off / off / Web UI → REST | |
| `AGEZT_WEBHOOKS` | boot | off | `url|subject-filter|secret` sinks (`webhook.ParseSinks`). |
| `AGEZT_WEBHOOK_ALLOW_LOOPBACK` / `_ALLOW_PRIVATE` | boot | off (`1`) | Egress opt-ins for webhook sinks. |
| `AGEZT_WEBHOOK_OUTBOUND_URL` | boot | — | Makes the `webhook` channel count as configured in `collectChannels`. |
| `AGEZT_ANOMALY_MAX_TOOLCALLS` / `AGEZT_ANOMALY_WINDOW` | boot | 120 / 10s | 0 disables the auto-halt breaker. |
| `AGEZT_ALERT_NOTIFY` (+`_LEVEL`, `_COOLDOWN`, `_MAX`, `_MUTE`, `_MUTE_SOURCES`) | boot | off | Alerts → channel sinks. |
| `AGEZT_PULSE` | boot | on | `off` disables Pulse. |
| `AGEZT_PULSE_CADENCE` / `_DIAL` / `_QUIET_HOURS` | boot | 60s / `pulse.ParseDial("")` / none | |
| `AGEZT_PULSE_PROBE` / `AGEZT_PULSE_DISK` | boot | none | Probe spec / `path:minPct`. |
| `AGEZT_PULSE_HEALTH` | boot | on (threshold 0.30) | `off` or a float error-rate threshold. |
| `AGEZT_PULSE_LLM` | boot | off (`on`) | |
| `AGEZT_PULSE_INITIATIVE` | boot | `act` | `off|ask|act`. |
| `AGEZT_REFLECT_EVERY` / `AGEZT_BRAIN_DISTILL_EVERY` | boot | off | Tickers. |
| `AGEZT_WORKBOARD_SWEEP_EVERY` / `AGEZT_WORKBOARD_STALE_AFTER` | boot | off / 10m | |
| `AGEZT_USER_PROFILE_EVERY` | boot | 24h | |
| `AGEZT_SCHEDULE` | boot | none | Env-sourced schedule jobs (e.g. `1h=summarise new commits`), synced into the store. |
| `AGEZT_SCHEDULE_RUN_TIMEOUT` | boot | 1h | `0`/`off` disables the per-firing backstop. |
| `AGEZT_CHANNEL_HISTORY` | boot (handler build) | 10 | Messages of prior conversation per channel; 0 disables. |
| `AGEZT_VOICE_REPLY` | live (per message) | on | Voice-in → voice-out. |
| `AGEZT_STT_API_KEY` (fallback `OPENAI_API_KEY`), `AGEZT_STT_API_URL`, `AGEZT_STT_MODEL` | boot | off | Standalone STT client for Web UI mic / OpenAI `/v1/audio/transcriptions`. |
| `AGEZT_RESUME_MAX_ATTEMPTS` | boot | 3 | Resume crash-loop cap. |
| `AGEZT_COUNCIL_MEMBERS` | **live** | best keyed model per provider (≤3) | Comma list of council models. |
| `AGEZT_DRAIN_TIMEOUT` | **live** (at shutdown) | 15s | 0 = no drain wait. |

### 3.4 Read during boot by packages `runDaemon` calls (documented elsewhere)

- `plugins/providerboot` ([08](08-providers.md)): `AGEZT_PROVIDER`, `AGEZT_MODEL`, `AGEZT_DEMO_ECHO` (keyless echo mock
  used by every e2e harness), `AGEZT_DEFAULT_CHAIN`, `AGEZT_FALLBACK_CHAINS`, `AGEZT_TASK_ROUTES`, `AGEZT_TASK_MODEL_CHAINS`,
  `AGEZT_TASK_MODEL_OVERRIDES`, `AGEZT_TASK_ROUTE_REQUIRES`, `AGEZT_TASK_BUDGETS`, `AGEZT_GEN_TEMPERATURE`, `_GEN_TOP_P`,
  `_GEN_REASONING_EFFORT`, `AGEZT_LLM_CACHE_TTL`, `AGEZT_RATE_PER_MIN`, `AGEZT_MODEL_DOWNROUTE(_CROSS)`, `AGEZT_MODEL_STRICT`,
  `AGEZT_PRICING_STRICT`.
- `kernel/creds` ([05](05-governance-routing-security.md)): `AGEZT_VAULT_*`, `AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED/_ENV`.
- `toolreg.BuildAll` → `plugins/builtintools` specs with `Get: os.Getenv` ([10](10-tools.md)): `AGEZT_HTTP_*`,
  `AGEZT_BROWSER_*`, `AGEZT_SANDBOX*`, `AGEZT_CODING_CMD`, `AGEZT_ACP_AGENT_CMD`, `AGEZT_HOMEASSISTANT_*`, `AGEZT_PEERS`,
  `AGEZT_PLUGINS*`, ….
- Channel manifests' `RequiredEnv` / `AllowlistEnv` / `InboundEnv` / `AddrEnv` ([09](09-channels.md)).

Guard: `kernel/controlplane/config_inventory_test.go` `TestConfigEnvVars_CoversCmdAgeztReads` scans `kernel/`, `plugins/`,
`internal/`, `cmd/agezt/` for `EnvPrefix+"X"` and any `"AGEZT_X"` literal and fails if a name is missing from
`controlplane.configEnvVars`. A new env var read anywhere in the daemon must be added there.

---

## 4. AGEZT_HOME on-disk layout created at boot

Directories are created by their owning package on first open (paths does not create anything). Order follows boot.

```
<AGEZT_HOME>/                         default ~/.agezt (dev: <repo>/.dev-home; install.sh: /var/lib/agezt)
├── catalog/                          catalog.Store: api.json, custom.json, local.json, meta.json      (step 3)
├── creds.json                        credential vault (AES-256-GCM envelopes, machine-bound)          (step 4)
├── config.json                       Config Center settings store (settings.FileName)                  (step 5)
├── schemas/<section>.json            registered Config Center schema sections (settings.SchemaDir)
├── journal/NNNNN….jsonl              BLAKE3 hash-chained segments                                       (Open)
├── state/<namespace>.json            key/value namespaces                                               (Open)
├── memory/  worldmodel/  skills/  cadence/  artifacts/ (+ artifacts/index/)  datalake/<coll>/{_schema.json,rec/}
├── standing/  resume/ (+ resume/quarantine/)  roster/  toolforge/  mcp/  workflows/  workboard/  okr/  taste/  seats/
├── runtime/control.addr, runtime/control.token      control plane (0600; removed on clean Stop)       (step 28)
├── runtime/edict_overlay_snapshot.json              compacted durable policy (hash-bound to journal)
├── board/board.json                  shared message board                                               (step 26)
├── tenants/<id>/…                    one full AGEZT_HOME tree per tenant (+ tenant token file)          (step 27)
├── web-password                      minted built-in console password (0600)                           (step 33)
├── openai.token / rest.token         API listen tokens (0600), only when the surface is enabled       (35 / 39)
├── market/{installed.json,sources.json,marketplaces/…}                                                 (step 41)
├── workspace/                        default AGEZT_WORKSPACE (file/shell tool root)
├── sandbox/  browser-sessions/       code_exec sandbox, browser.action sessions (tool-created)
├── configcenter/  agentgw/           created on use by kernel/configcenter, kernel/agentgw
├── chat_prompts.json                 control-plane saved chat prompts (on use)
├── bin/, update.lock                 self-update staging (kernel/update, on apply)
└── update.sentinel                   read (never written) by the watchdog
```

Details of each store's file formats: [06-data-memory-state.md](06-data-memory-state.md),
[07-autonomy-and-extensibility.md](07-autonomy-and-extensibility.md), [03-control-plane-and-http.md](03-control-plane-and-http.md).

---

## 5. `cmd/agezt/internal/daemonconfig`

- **Purpose:** Phase 2.5 extraction of ~450 lines of inline env parsing so defaults, warnings and fatal decisions are
  unit-testable.
- **API:** `type Config struct{Policy; Knowledge; RunLoop; SubAgents; Context ContextBudget; Sidecars; Guards; Tenancy;
  Lifecycle; Misc}`; `ModelService{URL, Model, Key}.Enabled()`; `VoiceHalf{Provider, URL, Model, Key, Voice, Attempt}`;
  `func Load(get func(string) string, warn io.Writer) (Config, error)` (`get==nil` ⇒ `os.Getenv`, `warn==nil` ⇒ discard).
- **Invariants (package comment):** must run after `injectConfig`; returns an error only for historically fatal vars, in
  the same source order as the old inline code, with error text starting `AGEZT_X:` so `runDaemon`'s
  `"%s: %v"` reproduces the historical output byte-for-byte. Excluded on purpose: `FORCE_START` (pre-inject),
  `COUNCIL_MEMBERS`/`DRAIN_TIMEOUT`/tenant `EDICT_DURABLE` (live), channel manifest env (dynamic), `WEB/REST/API_ADDR`
  (owned by the surface builders). Many other inline reads (Pulse, anomaly, alerts, tickers, warden docker, approval mode,
  …) were never migrated and still live in `cmd/agezt`.
- **Deps:** `internal/brand`, `kernel/edict` (`ParseDenyRules`, `HardDenyRule`), `plugins/providers/voice` (native provider
  constants only). Used only by `cmd/agezt`. No goroutines, no persistence.

| File | What it does |
|---|---|
| `doc.go` | Package contract (above). |
| `daemonconfig.go` | All config struct types + `ModelService.Enabled`. (Its "Provenance" header wrongly claims it holds the helpers.) |
| `daemonconfig_load.go` | `Load`, `voiceProviderIsNative`, `splitNonEmpty` (duplicate of `cmd/agezt`'s). |

Tests: `daemonconfig_test.go` — `TestLoad_Defaults`, `_KnowledgeSwitches`, `_Policy`, `_SubAgents`,
`_ContextBudget_WarnAndDegrade`, `_Guards`, `_Sidecars`, `_RunLoop`, `_Tenancy`, `_Lifecycle`.

---

## 6. `internal/*`

### 6.1 `internal/brand`
- **Purpose:** DECISIONS A1 — nothing else may hardcode the project name, binary names, env prefix or config dir.
- **Exports:** consts `Name="Agezt"`, `Binary="agezt"`, `CLI="agt"`, `EnvPrefix="AGEZT_"`, `ConfigDir=".agezt"`,
  `ProtocolVersion=1`; vars `Version="1.1.0"`, `BuildCommit`, `BuildTime` (set via `-X` ldflags); `BuildInfo()` (vcs.revision,
  vcs.time, vcs.modified from `debug.ReadBuildInfo`, test seam `debugReadBuildInfo`); `BuildStamp()`.
- **Invariant:** `BuildStamp()` must be referenced from `main` (it is, in the ready banner) or the linker dead-strips
  `BuildCommit/BuildTime` and the `-X` stamp silently no-ops.
- **Used by:** both binaries, `internal/paths`, daemonconfig, acp, cadence/systemtasks, chatgptauth, controlplane, restapi,
  runtime, selfrepair, update, builtinchannels, builtintools, providerboot, introspect/overseer tools.
- Files: `brand.go`. Tests: `brand_test.go`, `buildinfo_test.go`.

### 6.2 `internal/paths`
- `BaseDir() (string, error)`: `$AGEZT_HOME`, else `os.UserHomeDir()/.agezt`; error text tells the operator to set
  `AGEZT_HOME`. Does not create the dir. Used by `cmd/agezt`, `cmd/agt`, `cmd/agt/dial`, `kernel/webui`, `sdk`.
- Files: `paths.go`. Tests: `paths_test.go`, `paths_error_test.go`.

### 6.3 `internal/atomicfile`
- `WriteFile(path, data, mode)`: `os.CreateTemp(dir, "."+base+".*.tmp")` → write → `Sync` → `Close` → `Chmod(mode)` →
  `Rename`. Windows: if rename-over-existing fails (AV/indexer handle), remove target then rename, then as a last resort a
  direct non-atomic `os.WriteFile`. Parent dir must exist; no parent-dir fsync.
- Comment states it is "the one canonical implementation"; per-package `atomicWrite` helpers wrap it.
- Used by: artifact, auth, catalog, creds, datalake, edict, filestore, market, resume, seat, settings, state, webui,
  plugins/tools/file. Files: `atomicfile.go`. Tests: `atomicfile_test.go`.

### 6.4 `internal/strutil`
- `Ellipsis(s, maxBytes, marker)` (cuts on a rune boundary; non-positive max ⇒ marker only), `FirstNonEmpty(items...)`,
  `FirstNonEmptySlice(primary, fallback)`. Used by both binaries, channel, controlplane, creds, planner, selfrepair and five
  plugins. Files: `strutil.go`. Tests: `strutil_test.go`.

---

## 7. Build, dev and install tooling

### 7.1 `Makefile` (exports `CGO_ENABLED=0`)

| Target | Does |
|---|---|
| `build` | `go build -trimpath -ldflags "-s -w -X brand.Version/BuildCommit/BuildTime" ./...` — a compile check; with `./...` no binaries are emitted. `VERSION` = `git describe --tags --always --dirty=-dev`. |
| `install` | `go install` `./cmd/agezt` with the same flags (not `agt`). |
| `test`, `vet` | `go test ./...`, `go vet ./...`. |
| `fmt` | `gofmt -l kernel cmd internal plugins sdk tools contract examples` must be empty. |
| `gen` | `go run ./tools/jsonschemagen -in .project/agezt-contract.jsonc -out contract/gen/types.gen.go -pkg gen`. |
| `structure-md` / `structure-md-check` | Regenerate / verify `.project/STRUCTURE.generated`. |
| `deps-check`, `sdk-parity`, `deadcode-check`, `doc-claims` | The `tools/*` gates (section 8). |
| `frontend-build`, `frontend-test`, `frontend-deadcode` | `npm run build` / `npm test` / `npm run deadcode` in `frontend/`. |
| `e2e`, `webui-e2e`, `webui-e2e-ps` | `scripts/e2e-smoke.sh`, `scripts/webui-e2e.sh`, `scripts/webui-e2e.ps1`. |
| `check` | `gen fmt vet test deps-check sdk-parity deadcode-check structure-md-check doc-claims frontend-deadcode frontend-test`. |
| `test-race`, `clean`, `linux`, `darwin`, `windows` | Written with cmd.exe syntax (`set X=Y && …`, `if exist bin rmdir /s /q`) — under a POSIX make these do not set `GOOS`/`CGO_ENABLED` and `clean` fails. `.PHONY` lists `race` (target is `test-race`) and omits `fmt`. |

### 7.2 Scripts

| File | Purpose |
|---|---|
| `dev.ps1`, `dev.sh` (root) | Thin wrappers → `scripts/dev.ps1` / `scripts/dev.sh`. |
| `scripts/dev.ps1` | Isolated dev loop: `AGEZT_HOME=<repo>/.dev-home` (never the real `~/.agezt`), parses `.env` (repo or main-worktree root) without echoing; passes through only `AGEZT_PROVIDER`, `AGEZT_MODEL`, `AGEZT_ALLOW_ALL`, `AGEZT_VAULT_PASSPHRASE`; warns when the checkout is behind upstream (stale embedded console), `-Pull` fast-forwards; builds `agezt.exe`/`agt.exe` in the repo root; seeds `catalog/` (copy of `~/.agezt/catalog` or `agt catalog sync --local`); seeds every `*_API_KEY` into the dev vault via `agt provider creds set`; runs with `AGEZT_WEB_ADDR` (default `127.0.0.1:8899`). Flags `-Fresh`, `-SkipBuild`, `-Pull`, `-WebAddr`. |
| `scripts/dev.sh` | Same loop for macOS/Linux (`--fresh`, `--skip-build`, `--pull`, `--web-addr`). |
| `scripts/build.sh` | `build|test|race|clean|vet` with the Makefile's ldflags. |
| `scripts/ci-go-retry.sh` | Retry wrapper for every Go step in CI (originally for WSL tmpfs `compile` corruption; clears tmpfs GOCACHE/temp between attempts). |
| `scripts/e2e-smoke.sh` / `.ps1` | Boots the real daemon in a temp `AGEZT_HOME` with `AGEZT_DEMO_ECHO=1` + API/REST addrs; asserts: `agt run` echo, doctor + journal chain verify, OpenAI chat (plain + streaming content delta, M550 guard), REST run, 401/400/413, 10 concurrent runs, halt→refuse→resume, graceful shutdown with 0 panics. `GOMAXPROCS=3`. |
| `scripts/webui-e2e.sh` / `.ps1` | Daemon with `AGEZT_DEMO_ECHO=1 AGEZT_MODEL=mock AGEZT_WEB_ADDR=127.0.0.1:18787`, seeds a run, scrapes the tokenized URL from the log, drives the committed `kernel/webui/dist` SPA with Playwright (zero console errors under CSP). |
| `scripts/webui-smoke.ps1`, `nav-audit.ps1`, `nav-screenshots.ps1` | Focused Playwright specs (API-key pages; 36-view nav audit; PNG per nav view). |
| `scripts/coverage-rank.sh` / `.ps1` | Per-package `go test -cover`, sorted ascending. |
| `scripts/dev/*` | Ad-hoc one-off utilities (import rewriters, `split-god-file.py`, scout-soul helpers). `split-god-file.py` is the tool behind the "Provenance: … extracted during Day-N god-file split" headers. |

### 7.3 Installers

- `install.sh` (Ubuntu, systemd): `install|start|stop|restart|status|update|run|expose {tailscale,cloudflare,ngrok}`.
  Requires a pinned tag (`AGEZT_REF`, default `v1.0.0`) unless `AGEZT_ALLOW_UNPINNED=1`; third-party installers need
  `AGEZT_ALLOW_REMOTE_INSTALL=1`. Builds frontend + both binaries into `/opt/agezt/releases/<ver>-<commit>-<ts>`, atomically
  flips `/opt/agezt/current`, symlinks into `/usr/local/bin`, writes `/etc/agezt/agezt.env` (0640, `AGEZT_HOME=/var/lib/agezt`,
  `AGEZT_REST_ADDR=127.0.0.1:8787`), unit `ExecStart=… agezt daemon`, `Restart=on-failure`, `NoNewPrivileges`,
  `ProtectSystem=full`, `ProtectHome=true`, `ReadWritePaths=$AGEZT_HOME`; refuses `AGEZT_HOME` under `/home`.
- `install.ps1` (Windows): same verbs; NSSM service `AGEZT`; defaults `C:\ProgramData\Agezt\{src,home,config}`,
  `C:\Program Files\Agezt`, REST `127.0.0.1:8787`.
- **Port clash:** both installers default `AGEZT_REST_ADDR` to `127.0.0.1:8787`, which is also the Web UI's default bind
  (on by default). The Web UI binds first (boot step 33 vs 39), so the REST API fails with "listen … address already in
  use" and the installer's `/healthz` probe hits the Web UI listener.

### 7.4 `ops/wsl-runners`
Documentation + keepalive (`wsl-keepalive.service`, `.cmd`, `install-keepalive.{sh,ps1}`) for three self-hosted WSL2
runners on host `WHITE`. README status: **DORMANT** since 2026-10-01 (`actions/runners total_count: 0`); no job uses them.

### 7.5 `contract/` and `examples/` (build aspects)
- `contract/gen/types.gen.go` — generated by `tools/jsonschemagen` from `.project/agezt-contract.jsonc` (24 types);
  `codegen-in-sync` CI job fails on drift. `contract/fixtures/*.json` + `fixtures_test.go` pin event payload shapes.
  Details: [12-sdks-and-contract.md](12-sdks-and-contract.md).
- `examples/agezt-run/main.go` — Go SDK example (stream a run, print cost/corr, list runs). `examples/autonomous/{mailbox-delegation,
  plugin-governance, policy-denial-audit, typed-schedule-system-task}` — `README.md` + `run.sh` + `expected.md` scenario scripts.

---

## 8. CI and verification gates

### 8.1 `tools/*` — what each enforces

| Tool | Invocation | Enforces |
|---|---|---|
| `depscheck` | `go run ./tools/depscheck` | Every non-main module in `go list -m all` is in `tools/depscheck/allowlist.txt` (24 modules: btcec/chainhash, coder/websocket, decred secp256k1/blake256, emersion go-imap/message/sasl, cpuid, goldmark, golang.org/x/*, yaml.v3, lukechampine blake3, testify + deps). Mirrors POLICY §1.1 / DEPENDENCIES.md. |
| `deadcodecheck` | `go run ./tools/deadcodecheck` | Runs `golang.org/x/tools/cmd/deadcode@v0.47.0 ./...` **without `-test`** (reachable = reachable from a binary). Allowlists only `sdk/` findings, `kernel/internal/testfixtures/`, and the single cross-package test seam `kernel/toolreg/toolreg.go|Names`. Resolves `go` via `runtime.GOROOT()` to dodge PATH shims. |
| `sdkparity` | `-check docs/SDK-PARITY.md` / `-out` | Statically extracts route registrations from `kernel/restapi/restapi.go` and checks each SDK tree contains the route prefix; the committed report must match. |
| `structure-md` | `-check -out .project/STRUCTURE.generated` | Regenerates per-package tables from each package's first doc comment (Go `doc.go`, TS header) across kernel/internal/cmd/plugins/sdk/tools/frontend/src; drift fails. Catches god-file-split tooling that overwrote 59 package comments. |
| `docclaimscheck` | `-base origin/main` | Numbers stated in `.project/AUDIT-2026-09-BACKEND-SURFACE.md`, `PR-BODY.md`, `REVIEW-MAP.md` (files changed, doc.go count, changelog entries, CI job count) must equal facts measured from git against the merge base. Needs `fetch-depth: 0`. |
| `changelog-lint` | `go run ./tools/changelog-lint` | Root `CHANGELOG.md` has `[Unreleased]` pointing at `CHANGELOG/unreleased/current.md`; split tree files exist with `# Changelog` heading / `###` sections; unknown markdown rejected; `*.bak-<RFC-stamp>` backups tolerated. |
| `changelog-split` | `--dry-run|--emit|--verify [--force] [--discard-working-set]` | Generates the split tree (per-version files, 100-wide M-number buckets); files carry a trailing generated sentinel and are never overwritten without `--force`. `--verify` is **advisory** in CI (`continue-on-error`) because the committed tree is not yet adopted. |
| `jsonschemagen` | `-in … -out … -pkg gen` | Stdlib-only JSONC + JSON Schema → Go types; unknown shapes → `json.RawMessage` with TODO. |

### 8.2 `.github/workflows/ci.yml` (push to `main`, every PR; `concurrency` cancels in-progress; `permissions: contents: read`)

Every job carries the fork guard `github.event_name == 'push' || head.repo == this repo`, runs on `ubuntu-latest`, and uses
the composite `./.github/actions/setup-go-safe` (latest stable Go; tmpfs GOROOT staging skipped when `RUNNER_ENVIRONMENT=github-hosted`;
toolchain integrity probe).

| Job | Gate |
|---|---|
| `test (linux)` | `go vet ./...`, `go test ./...`, **100% statement-coverage ratchet** on 10 packages (`plugins/providers/voice`, `kernel/stt`, `kernel/board`, `tools/jsonschemagen`, `plugins/tools/runstool`, `plugins/tools/workboardtool`, `plugins/providers/internal/provopts`, `plugins/tools/standingtool`, `kernel/event`, `kernel/tunnel`), `go build ./...`. |
| `race-breadth` | `CGO_ENABLED=1 go test -race ./...`. |
| `race-depth` | `go test -race -count=20` on channel, controlplane, chatgptauth, runtime, pulse, governor, bus, journal. |
| `e2e smoke (linux)` | Build both binaries, `scripts/e2e-smoke.sh`. |
| `codegen-in-sync` | `jsonschemagen`, `git diff --exit-code contract/gen/`. |
| `frontend-dist-in-sync` | `npm ci --ignore-scripts && npm run build`, `git diff --exit-code kernel/webui/dist/`. |
| `frontend-dist-rebuild` | On push to main only: rebuild dist and push a `[skip ci]` bot commit if it drifted (`contents: write`; token-URL push, not persisted). |
| `frontend-test` | `npm run deadcode`, `npm test`, `npm run test:coverage:voice`. |
| `webui-e2e` | Playwright against the committed go:embed bundle (`scripts/webui-e2e.sh`). |
| `python-sdk` / `typescript-sdk` / `rust-sdk` | `unittest` / `npm test` / `cargo fmt --check && cargo test`. |
| `multi-arch` | Cross-build `./cmd/...` for linux/amd64+arm64, darwin/amd64+arm64, windows/amd64, freebsd/amd64. |
| `deps-check` | depscheck, sdkparity, deadcodecheck, structure-md `-check`, docclaimscheck (`fetch-depth: 0`). |
| `lint` | No tracked ignored files; `gofmt -l cmd internal kernel plugins sdk tools`; staticcheck 2026.2.1 (prebuilt, SHA-256 verified, 5 retries); govulncheck v1.4.0 (5 retries). |
| `secrets` | gitleaks v8.30.1 over full history with `.gitleaks-baseline`. |
| `changelog` | `changelog-lint` (blocking), `changelog-split --verify` (advisory). |
| `CI` (aggregator) | `needs:` all 17 jobs, `if: always()`; enumerates job conclusions with `gh api …/actions/runs/$RUN_ID/jobs` and fails on anything not `success`/`skipped`. Required context of ruleset 22206739. |
| `ci.yml` (job id `ci_file`) | `needs: CI`; fails unless `needs.CI.result == 'success'`. Required context of ruleset 22200577. |

`publish-sdks.yml` (on release published / manual): builds + dry-run validates Python (build 1.2.2 / twine 6.1.0), npm,
and crates packages; publishes each only when its token secret is set.

Local equivalent of the full gate: `make check` + `make e2e` + `make webui-e2e` + race runs (cap `GOMAXPROCS` per project convention).

---

## 9. Extension points

| To add… | Do this |
|---|---|
| A boot-time env setting | Add the field + parse to `daemonconfig` (`daemonconfig.go` type, `daemonconfig_load.go` parse; fatal only if it must be) and a `TestLoad_*` case; consume `dcfg.X` in `runDaemon`. If it must apply without restart, read it live at call time instead and say so in the comment. Add the name to `controlplane.configEnvVars` (guard test). |
| A tool | A spec in `plugins/builtintools` registered via `toolreg`; implement `ApplyPreOpen` / `Configure` / `ConfigureLate` hooks as needed — no `cmd/agezt` change except an optional banner `bootStep`. Netguard-guarded specs must implement `NetguardAware` (boot warns via `NetguardGaps`). |
| A channel kind | A manifest + factory in `plugins/builtinchannels`/`channelwire`; the uniform loop at boot step 30 builds and starts it. To make it a notify target, extend the hard-coded `[]string{"telegram","slack","discord"}` list. |
| An HTTP surface | `buildX(ctx, k, …) string` in an `httpsurfaces_*.go` file using `httpserver.Start(ctx, …)`; persist any listen token with `writeAPIListenToken` (never print the full token); add an `httpBindings` entry for exposure reporting; print a banner line. |
| A resident loop / ticker | Start it on the daemon `ctx` (never `context.Background()`), return a banner description, panic-contain any goroutine that runs agent code. |
| A late boot step | Append a `bootStep{name, run, fatal}` to the `bootSteps` table (non-fatal steps report their own failures). |
| A seeded built-in (skills, guardians, market packs) | Extend the corresponding `plugins/builtin*` package; seeding must stay idempotent and best-effort. |
| A repo gate | A `tools/<name>` command + Makefile target + `check` entry + a step in a CI job listed in the `CI` aggregator's `needs`. |

---

## 10. Gotchas and invariants

1. **Single-instance guard** reads `AGEZT_FORCE_START` before `injectConfig`, so it can only be set in the real env.
2. **Real env beats Config Center.** `injectConfig` only fills unset vars; values set in the process env appear read-only
   (pinned) in the console.
3. **One shared board store** (`board.Open` once): tool, control plane and REST mailbox write through the same instance
   because each store saves its whole list; a second instance would clobber writes.
4. **Bus before runs:** `gov.SetBus`, `ward.SetBus`, `toolSet.Configure` and `SetRedactor` must precede the first run;
   the control plane only starts afterwards.
5. **Late-binding window:** the control plane (step 28), channels (step 30) and Web UI (step 33) accept work before
   `ConfigureLate` binds `notify`/`send_media`/`board` (step 41.1), before guardians/skills/market are seeded, and before
   standing orders, schedules, resume and workflow triggers start (steps 42–47). A run accepted in that window sees
   those tools unbound.
6. **Orphan reconcile + resume** both act on interrupted runs; a resumed correlation is first marked `task.abandoned`.
7. **Tenant kernels** copy the primary `runtime.Config` but get their own governor (`gov.WithLimits`), fresh Warden/Edict,
   no `OnReload`, same redactor; `AGEZT_EDICT_DURABLE` is re-read live at lazy open.
8. **Token hygiene:** OpenAI/REST tokens go to 0600 files and only a prefix is printed; the Web UI banner intentionally
   prints the full per-boot token URL, and prints a freshly minted console password once.
9. **Console password security history (comments):** the built-in password used to be the constant `"agezt"`
   (SECRET-002, fixed 2026-08-13); an unconditional `SetPasswordStrict(env)` used to clear auto-raised strict mode
   (AUTH-001, fixed 2026-08-12). `effectiveWebPassword` never applies the built-in to a non-loopback bind.
10. **`AGEZT_ALLOW_ALL=1`** sets every capability to L4 but keeps the catastrophe hard-deny rails; it is read twice
    (daemonconfig and raw in `buildTools`).
11. **Boot resilience:** sidecar misconfig, board open failure, skill/guardian seeding, MCP attach and market wiring all
    degrade with a message rather than exit; only the listed F steps are fatal.
12. **`os.Exit(0)` in the auto-update path** skips all defers (stale `runtime/control.*` files, unclosed kernel).
13. **Watchdog** ignores `AGEZT_HOME`, requires `$HOME`, and its `update.sentinel` path is never produced.
14. **God-file split residue:** several files carry another function's orphaned doc comment (`main.go` → `visionGate`,
    `httpsurfaces_browser.go` → `buildTunnel`, `httpsurfaces_tunnel.go` → `writeAPIListenToken`,
    `main_standing_delivery.go` → `buildCadence`), many exported-style helpers lost their doc comments, and file names
    no longer match contents (`main_pulse_artifacts.go` holds `injectConfig`; `main_pulse_misc.go` holds approval/warden
    selectors; `main_overlay_anomaly.go` holds `standingTrustCeiling`). `WIP-BOOT-TOOLS-EXPANSION.md` is stale.
15. **`splitNonEmpty`** exists twice (`cmd/agezt` and `daemonconfig`); `truncateScheduledAnswer` slices raw bytes although
    `strutil.Ellipsis` exists for rune-safe cuts.
16. **CI aggregator check:** the `CI` job calls `gh api` with no `GH_TOKEN`/`GITHUB_TOKEN` in its env and the workflow
    grants only `contents: read` (no `actions: read`). If `gh` fails, the command substitution in `for job in $(…)`
    yields nothing and the step prints "All 17 jobs succeeded"; if it succeeds, unquoted word-splitting turns
    `"success test (linux)"` into three words, and `test`/`(linux)` match neither `success*` nor `skipped*`. Either way
    the step does not evaluate `needs.*.result`. This should be verified against a real run; a `needs`-based check
    (`contains(needs.*.result, 'failure')`) would avoid both problems.
17. **CI `gofmt` roots** (`cmd internal kernel plugins sdk tools`) differ from `make fmt` roots (also `contract examples`).
18. **Cross-platform Makefile targets** use cmd.exe syntax (see 7.1); `scripts/build.sh` is the portable path.
