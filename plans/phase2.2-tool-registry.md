# Phase 2.2 — Tool registry (working plan, from live survey 2026-08-06)

Goal: kernel/toolreg Spec/Set registry; retire the 15 string-keyed downcast sites in
cmd/agezt/main.go + the 13 captured-local Bind sites; LD-2 class → compile error.

## Verified facts
- buildTools (cmd/agezt/boot_tools.go:43) + main.go:337-448 build ONE map[string]agent.Tool
  pre-Open; runtime.Open COPIES it (effTools) so post-Open wiring must mutate instances →
  hence the downcasts. 33 boot tools total (21 in buildTools incl. plugin host, 13 zero-arg in main.go).
- 15 downcasts in main.go, three phases: A pre-Open (#1 code_exec→cfg.ScriptRunner @792);
  B post-Open (#2-10 SetKernel/SetIndex/SetStore/SetRunner @1123-1159; #11-14 OnBlock in
  wireNetguardAudit @5560, called @1203); C late (#15 code_exec.Bind+SetConductorExec @1884).
  Plus 13 captured-local .Bind() sites @1762-1915 (notify/send_media/schedule/runs/standing/
  board/skill/introspect/overseer/tool_forge/mcp/workflow/workboard).
- **LIVE BUG (LD-2 residue): plugins/tools/fetch has OnBlock field (fetch.go:54, fed to
  netguard @84) but wireNetguardAudit never wires it and netguard_wire_test has no
  *fetch.Tool case → fetch SSRF refusals unjournaled. homeassistant uses netguard with NO
  OnBlock field at all.** Fix ships with PR 2.
- agent.Tool = {Definition() ToolDef; Invoke(ctx, json.RawMessage) (Result, error)}.
  No Capability on ToolDef (Phase 3.4 — do NOT fold in; keep Built.Caps as M900 passthrough only).
- Per-run assembly: buildLoopConfig (loopconfig.go:21) merges k.tools + forge_<n> +
  mcp_<n> lazy + tool_search; registry must not assume k.tools is the whole set.
- kernel/toolreg is a free name; acyclic above kernel/runtime (plugins/tools already
  import runtime in 8 pkgs). Register calls live in plugins/builtintools (RegisterAll,
  mirror of builtinchannels) — NOT per-tool register.go.

## API (empirical)
BuildDeps{BaseDir, WorkspaceRoot, Warden, Stderr, Get(name), AllowAll, NotifyTargets}
KernelDeps{K, Bus, Artifacts, Lake, Journal, BaseDir, Stdout}
LateDeps{KernelDeps, ChannelSend, ChannelSendMedia, Board, BoardNotify}
Built{Tool (nil=skip), Extra map[string]agent.Tool (browser verb tools/plugin families,
  keyed by Definition().Name, Set asserts no collisions = in-process-wins made explicit),
  Desc, Caps, Info *runtime.PluginInfo}
Spec{Name, Build(BuildDeps)(Built,error) — error=hard boot fail (peer/plugin),
  PreOpen(tool,*runtime.Config), Configure(tool,KernelDeps)error, Late(tool,LateDeps)error,
  Netguard bool}
Set{...}: BuildAll, Tools(), ApplyPreOpen, Configure, ConfigureLate, Descs,
  PluginManifest(), ToolCapabilities().
NetguardAware interface{ SetOnBlock(func(ip, reason string)) } in toolreg; add SetOnBlock
  methods to http/browser/browser.action/websearch/fetch/homeassistant Tools. Configure
  wires publish(name) generically for Netguard specs; Netguard-spec-without-NetguardAware
  = test failure enumerated FROM the registry (no hand-listed type switch).

## PR train
1. kernel/toolreg package + driver + tests (nothing registered).
2. Netguard slice: http, browser.read, browser.action(+10 verbs via Extra), web_search,
   fetch (+homeassistant OnBlock field). Delete wireNetguardAudit. Move/rewrite
   netguard_wire_test registry-driven. SHIPS THE FETCH FIX. Repoint
   TestBuildToolsBrowserActionOptIn → toolreg.BuildAll with map-backed Get.
3. Set*-injection batch: config, fetch(SetIndex), artifacts, db, council, conductor,
   research, code_exec (PreOpen ScriptRunner + Configure SetIndex/Bind/SetConductorExec).
   Kills #1-10, #15.
4. Zero-arg locals: schedule, runs, standing, skill, introspect, overseer, tool_forge,
   mcp, workflow, workboard (Configure with d.K; introspect/overseer need
   NewKernelSource(k[, baseDir])).
5. Late set: notify, send_media, board (LateDeps; only PR touching boot order).
6. Env-gated rest: shell, file, coding, acp_agent, homeassistant, remote_run, plugin host
   (Built.Info/Caps carry manifest+caps out).
7. Delete buildTools; main.go tool section = BuildAll/ApplyPreOpen/Configure/ConfigureLate;
   ratchet test (builtintools, copy factory_ratchet_test shape); replace
   tool_effects_test hand-list with registry walk.

## Test discipline
After each PR: go test ./cmd/agezt/ ./kernel/runtime/ ./plugins/tools/... ./kernel/toolreg/
(runtime's tools_internal/toolcaps/foldruntools/subagent tests read k.tools).
