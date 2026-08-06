# Phase 2.6 — Boot decomposition (working plan, survey 2026-08-06)

runDaemon = main.go:150-1736 (1,586 lines of 4,885). NO test executes runDaemon —
boot order entirely unpinned (freedom + risk). Value order:
1a selfrepair → 2b HTTP file split → 2a systemtasks → 1b/1c → 3b narrowing → 3b Deps → 3a bootStep.

## Doc corrections (decided)
- kernelAPIEngine does NOT move to kernel/restapi: restapi is deliberately interface-only
  (Engine), importing runtime would invert its posture + couple to openaiapi. File split to
  cmd/agezt/api_engine.go instead (155 lines, adapter serves BOTH restapi.Engine and
  openaiapi.Engine+UsageReporter; constructed at 4 sites).
- NO hard validation at NewServer: 11/14 injected fields have legitimate nil states mapping
  to real operator config (PULSE=off, no channels, no tenants, boardErr...). Nil-tolerant
  Deps; validate shape not presence. startPair (391 uses) must keep working with Deps{}.

## Stage 1 (mechanical)
1a. auto_repair.go → kernel/selfrepair: git mv 3 files (auto_repair.go 1894 +
    auto_repair_test 1127 + coverage_autorepair_test 134 — coverage_wire_test's 2
    wireAutoRepair tests move too), export WireAutoRepair, main.go:1619 call site,
    firstNonEmpty dup resolved via strutil. Imports already kernel-clean;
    kernel→overseertool edge has precedent (controlplane/roster.go:36).
    ADD config_inventory root (AUTO_REPAIR, AUTO_REPAIR_COOLDOWN, ROUTING_ROLLBACK_PROBATION).
1b. kernelAPIEngine+restArtifactEntry (2692-2846) → cmd/agezt/api_engine.go.
1c. Hoist SetHTTPBindings/SetCredChain/SetChannels (1336-1347) up to the NewServer setter
    region (~975-1015) — no data dependency below 975.

## Stage 2
2a. System-task executors (main.go 4056-4457 ~400 lines: runScheduledSystemTask dispatch +
    7 executors + graveyardRetentionDays + envOrDefaultLocal) → kernel/cadence/systemtasks
    (catalogue/validation already in cadence; only executors stranded). Cycle constraint:
    systemtasks imports runtime; NOTHING in cadence/runtime may import systemtasks —
    package doc must say so. buildCadence dispatch site calls systemtasks.Run(...).
    schedule_system_task_test splits 6 move / 7 stay. ADD config_inventory root
    (GRAVEYARD_RETENTION_DAYS, CATALOG_URL, log-clean vars).
2b. HTTP surfaces (2363-3333 ~800 lines: webUISurface/buildWebUI/helpers/buildTunnel/
    buildOpenAIAPI/buildRESTAPI/buildWebhooks/writeAPIListenToken; isLoopback 4466) →
    cmd/agezt/httpsurfaces.go FILE SPLIT (same package — no export churn, no env blindness).
    main_test's tunnel/webui/token tests (~496-670) + coverage_helpers_more_test web/tunnel
    tests may move to a matching _test file or stay (same package — either fine).
    Follow-up cleanups INSIDE the split: dedupe tenant-resolver closure (3109/3264 verbatim
    dup); lift buildRESTAPI's 60-line SetMetrics closure into named restMetrics(k).

## Stage 3 (design care; do after 1-2)
3b-i. Interface narrowing: 5 raw func fields → 2-3 interfaces (PulseAdmin, Outbound...) — 
    separate PR, independent of ctor.
3b-ii. NewServer(k, baseDir, Deps{ConfigEnvPinned, Board, DiskFree, UpdateSvc, Tenants,
    CancelOnDisconnect, HTTPBindings, CredChain, Channels}) + srv.Bind(LateDeps{Pulse,
    DiskWatch, ProbeWatch, ChannelSend, StandingFire}) — two-phase mirrors toolreg
    Configure/ConfigureLate. 17 call sites (1 prod, 16 tests); Deps{} stays valid.
    HARD CONSTRAINT: 9 of 14 setters currently run AFTER srv.Start — the 5 late ones
    (pulse/diskWatch/probeWatch/channelSend/standingFire) CANNOT hoist (closures over
    channel-phase artifacts); tenants COULD hoist above Start.
3a. bootStep table for phase S only (1409-1559: configure_late_tools FATAL + 10 best-effort
    seeds/banners). Banner text byte-identical. Phase T/F/H NOT candidates (different
    closures / dense value computation).

## Watch
- config_inventory_test roots: add kernel/selfrepair + kernel/cadence/systemtasks in the
  SAME commits as the moves.
- 7 direct &Server{} literals in tests bypass NewServer (fine, unaffected).
- 30 srv.Set* calls in 13 controlplane test files — setters must SURVIVE (Deps is additive).
