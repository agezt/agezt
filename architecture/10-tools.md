# 10 — First-party tools, guardians, market seed, skill bundles

**Scope:** `plugins/tools/*` (30 tool packages), `plugins/builtintools` (the boot tool registry), `plugins/builtinguardians` (self-healing guardian fleet), `plugins/builtinmarket` (Official marketplace seed), `plugins/builtinskills` (+ its 16 embedded agentskills.io bundle directories).

Related docs: kernel-side contract and registry plumbing (`kernel/agent`, `kernel/toolreg`, `kernel/runtime` tool loop, research/council/conductor orchestration) → [04-agent-runtime.md](04-agent-runtime.md); Edict, warden, netguard, envscrub, executionprofile, settings/creds → [05-governance-routing-security.md](05-governance-routing-security.md); artifact index, board, datalake, workboard → [06-data-memory-state.md](06-data-memory-state.md); cadence, standing, skill Forge, market, mcp, acp/acpcatalog, workflow, plugin host, memory/world/voice/image/rerank tools → [07-autonomy-and-extensibility.md](07-autonomy-and-extensibility.md); daemon boot order (`cmd/agezt/boot_tools.go`, `main.go`) → [01-daemon-boot-cmd-agezt.md](01-daemon-boot-cmd-agezt.md); channels used by notify/send_media → [09-channels.md](09-channels.md); console Tools/Market/Skills views → [11-frontend-console.md](11-frontend-console.md).

---

## Responsibilities at a glance

- **Tool contract.** Every tool implements `agent.Tool` (`kernel/agent/agent.go:205`): `Definition() agent.ToolDef` + `Invoke(ctx, json.RawMessage) (agent.Result, error)`. `ToolDef` carries model-facing `Name/Description/InputSchema` plus two **governance-only** fields that never reach the provider wire: `Effect agent.ToolEffect` (reversibility class + predicted effects, used for HITL decision bundles) and `Capability agent.ToolCapability` (the Edict axis the call is gated on).
- **Registration.** `builtintools.RegisterAll()` registers 32 ordered `toolreg.Spec`s into the package-level `kernel/toolreg` registry; `cmd/agezt/boot_tools.go:buildTools` calls `toolreg.BuildAll` and drives the resulting `*toolreg.Set` through four lifecycle phases (Build → PreOpen → Configure → ConfigureLate). Kernel-owned tools (`memory`, `world`, `delegate`, `delegate_await`, `market`, `voice`, `image_generate`, `rerank`) are registered by `kernel/runtime`, NOT here.
- **Governance.** Each tool call is resolved to an Edict capability (declared `ToolDef.Capability` first → plugin manifest caps → `edict.CapabilityForToolCall` name switch) and decided by Edict. An **unknown capability is default-DENIED**; guard tests make that a build-time failure. Every governed capability is **Allow by default** (owner's default-allow posture).
- **Safety floors that are NOT policy:** netguard SSRF guard on network tools (loopback/RFC1918/metadata refused), host allowlists per redirect hop, file-tool root jail with symlink/TOCTOU defense, secret-scrubbed child environments for shell/code_exec/browser driver/ACP/coding, warden-mediated process execution with output caps and timeouts, notify/send_media recipient pinning, overseer refusal to edit/repair `System` guardians.
- **Guardians.** 7 System-flagged agents seeded idempotently at boot, each with exactly one trigger (event standing order or cadence schedule), clamped budgets/trust ceiling/noise policy on every boot.
- **Market seed.** 16 single-bundle packs (`<bundle>-pack`) + 12 curated combo packs (skills + key-less MCP servers + CLI tool requirements) = the always-present offline "official" marketplace.
- **Skill bundles.** 16 embedded bundles (SKILL.md + reference/ + scripts/) seeded into the skill Forge and promoted to active on every boot (content-addressed dedupe).

---

## 1. The tool contract (kernel side, summarized)

Defined in `kernel/agent` (full detail in 04-agent-runtime.md):

| Type | Fields / methods | Notes |
|---|---|---|
| `agent.Tool` | `Definition() ToolDef`, `Invoke(ctx, json.RawMessage) (Result, error)` | `Invoke` must honour ctx. A returned `error` is a transport failure; tool-level failures should be `Result{IsError:true}` so the model can retry. |
| `agent.ToolDef` | `Name`, `Description`, `InputSchema json.RawMessage`, `Effect ToolEffect` (`json:"-"`), `Capability ToolCapability` (`json:"-"`) | Effect/Capability are governance metadata only. |
| `agent.ToolCapability` | `Name string` (fallback axis), `Field string` (top-level input key that selects the axis, e.g. `op`, `method`, `operation`), `ByValue map[string]string` | `For(input)` (`kernel/contract/toolapi`): unparseable input / missing field / unknown value → `Name`. Lookup lower-cases+trims. `IsZero()` = no Name and no ByValue. |
| `agent.ToolEffect` | `Class EffectClass` (`read_only`/`reversible`/`compensable`/`irreversible`), `PredictedEffects`, `AffectedResources`, `RollbackNotes`, `Confidence` | `plugins/tools/tool_effects_test.go` requires every first-party tool to fill all five. |
| `agent.Result` | `Output`, `IsError`, `ObservationTrust` (`ObservationUntrusted` for external content), `ObservationSource` | Untrusted observations are rendered as data, not instructions (prompt-injection guard). |

Context helpers tools read: `agent.CorrelationFromContext`, `agent.AgentFromContext` (acting agent slug), `agent.WorkdirFromContext` (per-agent workdir, M792), `warden.CorrelationFrom`, `warden.ProfileOverrideFrom`, `executionprofile.{SSH,K8s,Modal,Daytona}OverrideFrom`, `tenantctx.Tenant`.

### Capability resolution order (`kernel/runtime/policy.go:capabilityFor`)

1. `def.Capability.For(tc.Input)` — **only if** `edict.KnownCapability(cap)`; an unknown declared axis is ignored, not honoured (honouring a typo would default-deny the tool).
2. `k.toolCaps[name]` — M900 plugin-manifest declared caps (`Built.Caps` from the plugins spec; a plugin may join an existing axis, never invent one).
3. `edict.CapabilityForToolCall(name, input)` (`kernel/edict/toolmap.go`) — the legacy name switch, still authoritative for dynamic surfaces: `forge_*` → `code.exec`, `mcp_*` → `mcp.call`; unknown names fall through to `Capability(name)` → default-deny.

Then `policyHook` calls `edict.Decide` (or `DecideWithCeiling` under an initiative trust ceiling) and builds an approval bundle from `ToolDef.Effect`.

---

## 2. `plugins/builtintools` — the boot tool registry

**Purpose.** Tool mirror of `builtinchannels`: each first-party tool contributes a `toolreg.Spec` whose `Build` contains the construction logic that used to live inline in `cmd/agezt` (Phase 2.2 of the 2026-08 refactor). Env reads go through `BuildDeps.Get` so tests drive registration with a map-backed env.

**Depends on:** `internal/brand`, `kernel/agent`, `kernel/plugin`, `kernel/redact`, `kernel/runtime`, `kernel/toolreg`, and all 30 `plugins/tools/*` packages. **Used by:** `cmd/agezt` (`boot_tools.go`), `plugins/tools/tool_effects_test.go`.

### `toolreg.Spec` lifecycle (defined in `kernel/toolreg/toolreg.go`)

| Hook | When | Deps struct | Used by |
|---|---|---|---|
| `Build(BuildDeps) (Built, error)` | `toolreg.BuildAll` before the kernel exists | `BaseDir, WorkspaceRoot, Warden, Stderr, Get, AllowAll, NotifyTargets` | every spec. Zero `Built` = declined (env-gated off); error = **hard boot failure**. |
| `PreOpen(tool, *runtime.Config)` | `Set.ApplyPreOpen` before `runtime.Open` | runtime config | `code_exec` (sets `cfg.ScriptRunner` for forged tools) |
| `Configure(tool, KernelDeps)` | `Set.Configure` after `runtime.Open` (`main.go:839`) | `K, Bus, Artifacts, Lake, Journal, BaseDir, Stdout, NetguardPublish` | Set*-injection + kernel-bound tools |
| `Late(tool, LateDeps)` | `Set.ConfigureLate` once channels + shared board exist (`main.go:1427`) | `KernelDeps` + `ChannelSend, ChannelSendMedia, Board, BoardNotify` | `notify`, `send_media`, `board` |
| `Netguard bool` | Configure wires `NetguardPublish(name)` into every built instance implementing `toolreg.NetguardAware` (`SetOnBlock`) | — | `http`, `browser.read`, `browser.action`, `web_search`, `fetch` |
| `YieldOnConflict bool` | BuildAll drops a colliding name with a warning instead of erroring | — | `plugins` only |

`Built{Tool, Extra map[string]agent.Tool, Desc, Caps map[string]string, Infos []runtime.PluginInfo}`. `buildTools` returns `set.Tools()`, the Set, `set.PluginManifest()`, `set.ToolCapabilities()` and the joined banner `Descs()`, and warns (does not fail) if `set.NetguardGaps()` is non-empty.

Every hook closes over the concrete instance its own Build produced (`var ce *codeexec.Tool` captured in the spec closure) — no string-keyed downcasts. Consequence: each `RegisterAll()` generation configures only its own instances.

### Registration order (pinned by `ratchet_test.go:bootSpecNames`)

```
shell, file,                                             # workspace pair
http, browser.read, browser.action, web_search, fetch,   # netguard slice
config, artifacts, db, council, conductor, research, code_exec,   # Set*-injection batch
schedule, runs, standing, skill, introspect, overseer, tool_forge, mcp, workflow, workboard,  # kernel-bound
notify, send_media, board,                               # late-bound
coding, acp_agent, homeassistant, remote_run,            # env-gated externals
plugins                                                  # external plugin host — MUST be last
```

Order drives the daemon `tools :` boot banner and guarantees in-process names beat plugin names.

### File-by-file

| File | What it does |
|---|---|
| `doc.go` | Package doc for the env-gated / workspace-scoped spec slice (shell, file, coding, acp_agent, homeassistant, remote_run); notes malformed operator specs are hard boot errors. |
| `tools.go` | `RegisterAll()` (the ordered registration list), `splitHosts` CSV helper, `specBrowserAction` (Configure injects artifact index into `*browser.ActionTool`), `specFetch` (fetch + `AllowAll`→loopback/private, Configure injects artifact index). |
| `tools_helpers.go` | `buildHTTP`, `buildBrowserRead`, `buildBrowserAction`, `buildWebSearch`: env → allowlist/egress flags (see §4 env table). browser.action is opt-in (`AGEZT_BROWSER_ACTIONS=1` + resolvable `browse.mjs` driver) and contributes the 10 `browser.*` verb tools as `Built.Extra`. |
| `inject.go` | `specConfig` (SetKernel), `specArtifacts` (SetIndex), `specDB` (SetStore lake), `specCouncil`/`specConductor`/`specResearch` (SetRunner kernel), `specCodeExec` (gates on `AGEZT_SANDBOX!=off` and `codeexec.DetectRuntimes()` non-empty; PreOpen sets `cfg.ScriptRunner`; Configure → SetIndex, `Bind(bus)`, `K.SetConductorExec(ce)`). |
| `kernelbound.go` | Zero-arg kernel-bound specs: `schedule` (binds `K.Schedules()` + roster lookup), `runs` (journal), `standing`, `skill` (Forge), `introspect` (`introspecttool.NewKernelSource`), `overseer` (`overseertool.NewKernelSource(K, BaseDir)`), `tool_forge`, `mcp`, `workflow`, `workboard`. No banner descs. |
| `latebound.go` | `notify`/`send_media` (only built when `BuildDeps.NotifyTargets` non-empty; Late binds channel sender + pinned targets; send_media resolves artifact refs lazily via `K.Artifacts().Get`), `board` (always built; Late binds the ONE shared `*board.Store` + `BoardNotify`). |
| `envgated.go` | `specShell` (warden, WorkDir=workspace root, BaseDir), `specFile` (`filetool.NewWithCheckpoint(root, baseDir)`; error = hard boot failure), `specCoding` (`AGEZT_CODING_CMD`), `specACPAgent` (`AGEZT_ACP_AGENT_CMD` OR any installed acpcatalog agent), `specHomeAssistant` (URL+TOKEN+≥1 allowlist axis), `specRemoteRun` (`AGEZT_PEERS` / `AGEZT_TENANT_PEERS`; parse error = hard boot failure). |
| `plugins.go` | `specPlugins`/`buildPlugins`: parses `AGEZT_PLUGIN_PINS`, `AGEZT_PLUGIN_TOOLS`, `AGEZT_PLUGINS` (malformed → hard error), `plugin.Spawn`s each with a 30 s init context, prefixes tools `<prefix>.`, records M900 declared caps + `runtime.PluginInfo`, warns on unused pins/allowlists. Plugin stderr is pattern-redacted via `pluginLogLine` (`redact.New()`). A failed plugin is skipped, never fatal. |

Tests: `ratchet_test.go` (ordered spec list), `capability_guard_test.go` (see §8), `tools_test.go` (netguard wiring coverage from the registry, always-on specs build+configure, code_exec gating, browser.action gated off), `plugins_test.go` (log redaction).

### Boot flow

```
cmd/agezt main
 ├─ derive notifyTargets from channel manifests
 ├─ buildTools(): builtintools.RegisterAll(); toolreg.BuildAll(BuildDeps{Get: os.Getenv, AllowAll: AGEZT_ALLOW_ALL=="1"})
 │     └─ warn on Set.NetguardGaps()
 ├─ Set.ApplyPreOpen(&cfg)            # code_exec → cfg.ScriptRunner
 ├─ runtime.Open(cfg with tools map, ToolCapabilities, PluginManifest)
 ├─ Set.Configure(KernelDeps{K, Bus, Artifacts, Lake, Journal, NetguardPublish})
 ├─ ... channels + shared board.Store built ...
 ├─ Set.ConfigureLate(LateDeps{ChannelSend, ChannelSendMedia, Board, BoardNotify})
 ├─ builtinskills.SeedAll(forge, "")                  # main.go:1490
 ├─ market library = NewCompositeLibrary(builtinmarket.New(), marketStore)   # main.go:1507
 └─ builtinguardians.SeedAll(NewKernelHost(k), "")    # main.go:1535 (best-effort)
```

Tools are registered into the kernel map BEFORE the kernel/channels start, so the map is never mutated while the agent loop reads it; late tools are registered unbound and guard their binding with a mutex (`notify`, `sendmedia`, `boardtool`, `overseertool`, `forgetool`, `mcptool`, `workboardtool`…).

---

## 3. Complete tool catalog

Legend — **Cap**: Edict capability (from the tool's own `ToolDef.Capability`; "switch" = resolved only by `edict/toolmap.go`). **Effect**: `ToolDef.Effect.Class`. Registration: **always** = always registered; **gated** = env/host dependent.

### 3.1 First-party tools registered by `builtintools`

| Model-visible name | Package | Registration | What it does | Cap (by input) | Effect | Side effects | Notable guards |
|---|---|---|---|---|---|---|---|
| `shell` | `tools/shell` | always | Run one command in `cmd /C` (Windows) / `sh -c`; combined stdout+stderr | `shell` | irreversible | arbitrary host command in workspace root (or per-agent subdir) | warden-run; scrubbed env (`scrubEnv`); 30 s default timeout; 64 KiB model-facing budget incl. status prefix; Windows `cmd /S /C "<cmd>"` verbatim (warden `cmdline_windows.go`, M958); secret bucket keyed on EFFECTIVE warden profile (RCE-001); SSH/K8s/Modal/Daytona execution-profile overrides |
| `file` | `tools/file` | always | read (line ranges), write, append, list, search (substring/RE2), stat, delete, replace (unique-match find/replace), glob | `op`: read/stat/search→`file.read`; list/glob→`file.list`; write/append/replace→`file.write`; delete→`file.delete`; **no fallback** (unknown op ⇒ default-deny) | reversible | workspace files; rollback catalog `<home>/rollback/checkpoints.json` | root jail (Abs+EvalSymlinks; existing-ancestor resolution for new files); walk skips symlinks escaping root; O_NOFOLLOW (unix) / `GetFinalPathNameByHandleW` re-check (Windows); atomic replace; caps 256 KiB read, 1000 list, 200 hits, 8 MiB scan; read ops return untrusted observations |
| `http` | `tools/http` | always | GET/POST with headers/body, returns status+headers+body | `method`: POST→`http.post`, else `http.get` | compensable | outbound request | netguard (loopback/private/metadata refused); optional host allowlist re-checked on every redirect (≤10, M251); 256 KiB req/resp caps; untrusted observation |
| `browser.read` | `tools/browser` | always | GET page, strip scripts/styles, decode entities → text JSON | `browser.read` | reversible | outbound GET | netguard client; allowlist per redirect (M254); 4 MiB raw, 64 Ki chars text (rune-safe cut); non-2xx = error; untrusted |
| `browser.action` | `tools/browser` | gated: `AGEZT_BROWSER_ACTIONS=1` + driver found | Playwright driver (`browse.mjs` from the browseruse bundle): ordered actions, extract, snapshot refs, events, cookies, screenshots/downloads → artifacts; profiles isolated/session/user-attached/remote-cdp | **switch only** → `browser.action` | irreversible | headless browser process, remote side effects, local artifacts, session dirs under `<home>/browser-sessions` | URL+each `goto` validated (scheme, allowlist, pre-resolved IPs vs netguard); driver env `envscrub.Scrubbed()`; 30 s/120 s timeouts; 512 KiB driver output cap; user-profile/remote-CDP need explicit env opt-in; session/tab ids validated + root-contained |
| `browser.open`, `browser.snapshot`, `browser.click`, `browser.type`, `browser.wait`, `browser.screenshot`, `browser.downloads`, `browser.cookies`, `browser.tabs`, `browser.close` | `tools/browser` (`ActionVerbTool`) | same gate (Built.Extra) | Thin verbs that convert to a `browser.action` spec (or list/close session tabs) | **switch only** → `browser.action` | per-verb (`actionVerbEffect`) | same as browser.action | same as browser.action |
| `web_search` | `tools/websearch` | always | DuckDuckGo LITE (`https://lite.duckduckgo.com/lite/`) keyword search → {title,url,snippet} (default 6, max 15) | `web.search` | reversible | outbound GET to fixed host | fixed engine host; netguard; 1 MiB parse cap; fail-soft (never errors a run) |
| `fetch` | `tools/fetch` | always | Download URL bytes and save as artifact; returns id/mime/size | `http.get` | reversible | outbound GET; artifact store write | the http tool's egress posture (`AGEZT_HTTP_ALLOWED_HOSTS`, `AGEZT_HTTP_ALLOW_*`; W1.4 — it used to ignore the allowlist); 60 s, 50 MiB cap |
| `config` | `tools/config` | always | Config Center schema/get/set/register/unregister; scope effective/global/agent | `op`: set/register/unregister→`config.write`; else `config.read` | reversible | `settings` file, creds vault (`creds.json`) for secrets, roster `ConfigOverrides` for scope=agent; live provider/model reload | get reports secret presence only; field ReadOnly/Locked; registered fields must be `AGEZT_*` and cannot shadow built-ins; agent scope refuses secrets |
| `artifacts` | `tools/artifacts` | always | list/read/delete saved artifacts | `op`: delete→`file.delete`; else `file.read` | reversible | artifact delete | 256 KiB inline read, list limit 50 |
| `db` | `tools/db` | always | Personal Data Lake: list/create/drop collections, insert/get/update/delete/query records | `memory` (all ops) | compensable | `kernel/datalake` collections | actor stamped `agent:corr` |
| `council` | `tools/council` | always | Convene Council of Elders (multi-model panel, consensus+dissent) | `delegate` | reversible | model spend; council.* events | runner nil ⇒ "unavailable" |
| `conductor` | `tools/conductor` | always | Thinker/Worker/Verifier loop; Verifier executes worker code via code_exec backend | `code.exec` | reversible | model spend; sandbox run | rides code.exec because its in-kernel exec never re-enters Edict |
| `research` | `tools/research` | always | Deep-research harness: plan → web_search → browser.read → cited synthesis → adversarial verification | `research` | reversible | model spend; network | inner web_search/browser.read go through `k.RunTool` (each gated); report is untrusted observation |
| `code_exec` | `tools/codeexec` | gated: ≥1 runtime (python/node/deno) and `AGEZT_SANDBOX!=off` | Write+run Python/Node/Deno in ephemeral or named-project dir; pip packages; artifact export | `code.exec` | irreversible | sandbox dirs under `<home>/sandbox`; code.executed events; artifacts | see §5.1 |
| `schedule` | `tools/schedule` | always | Create in/every/daily/continuous schedules with typed targets (agent/intent/workflow/system_task/tool), list, remove; `assure` retries | `schedule` | reversible | cadence store (source=`agent`) | acting-agent binding validated; managed sub-agents cannot be scheduled directly |
| `runs` | `tools/runstool` | always | recent/stats/search over own past top-level runs from the journal | `runs.read` | reversible | none | read-only |
| `standing` | `tools/standingtool` | always | create_event / create_cron / list / remove standing orders | `standing` | reversible | standing store | — |
| `skill` | `tools/skilltool` | always | learn/list/show/promote/retire/files/read skills through Forge | `skill` | reversible | skill store (journaled, revertible) | — |
| `introspect` | `tools/introspecttool` | always | overview/reaper/schedules/standing snapshot of daemon state | `introspect` | reversible | none | read-only |
| `overseer` | `tools/overseertool` | always | Fleet supervision: status/agents/runs/help, cancel/halt/resume, pause/retire/revive/delete, edit/create/clone/search, bulk_*, wake, repair | `oversee` | compensable | roster, halt switch, run cancellation, settings file (`AGEZT_TASK_MODEL_CHAINS`) | see §5.5 |
| `tool_forge` | `tools/forgetool` | always | draft/test/update/request_promotion/list/show script tools (→ `forge_<name>`) | `op`: test→`code.exec`; else `tool.forge` | compensable | toolforge store; sandbox run | promotion via approval (but see auto-promote gotcha §9) |
| `mcp` | `tools/mcptool` | always | add/attach/detach/list/remove MCP servers at runtime | `op`: list→`introspect`; else `mcp.install` | compensable | mcp store; spawned processes | child scrubbed env; bridged tools later gated `mcp.call` |
| `workflow` | `tools/workflowtool` | always | save/run/enable/list/show workflows | `op`: list/show→`introspect`; else `workflow.manage` | compensable | workflow store | tool nodes inside a run re-gate per call (no laundering) |
| `workboard` | `tools/workboardtool` | always | list/show/create/claim/heartbeat/comment/block/fail/unblock/complete/archive/link/policy/depend/reclaim typed work items | `workboard` | compensable | workboard store (journaled) | actor/corr defaults from ctx |
| `notify` | `tools/notify` | gated: ≥1 notify-capable channel with an allowlist | Send text to operator's own chats | `notify` | compensable | channel.outbound | recipient ids pinned to operator allowlist; model supplies only text + optional kind |
| `send_media` | `tools/sendmedia` | same gate as notify | Send an existing artifact (image/voice/file) + caption to operator chats | `notify` | compensable | channel outbound | pinned recipients; payload by artifact ref only (no raw bytes) |
| `board` | `tools/boardtool` | always (unbound if store failed) | Shared mailbox: post/read/topics/send/inbox/reply/replies/broadcast/ack/help | `board` | reversible | board store; `board.posted` notifications (standing triggers `board.<topic>`, `board.dm.<slug>`) | acting agent cannot post as another `from` |
| `coding` | `tools/coding` | gated: `AGEZT_CODING_CMD` | Run external coding agent in a detached git worktree; return diff (never merges) | `coding` | compensable | temp worktree; external agent process | scrubbed env + only `AGEZT_CODING_TASK`; 5 min timeout; 60 KiB diff cap |
| `acp_agent` | `tools/acpagent` | gated: `AGEZT_ACP_AGENT_CMD` or any installed catalog ACP agent | Drive external ACP agent over JSON-RPC stdio (initialize → session/new → session/prompt) | `acp_agent` | compensable | external agent process | scrubbed env; 5 min timeout; ctx-watcher tears down transport; 60 KiB answer cap |
| `homeassistant` | `tools/homeassistant` | gated: URL+TOKEN+≥1 allowlist | get_states / call_service on Home Assistant | `operation`: call_service→`homeassistant.call`; else `homeassistant.read` | irreversible | physical device actuation | fail-closed entity read allowlist + service allowlist (`domain.*`, `*`); fixed operator-configured destination (`netout.OperatorClient`: metadata refused) |
| `remote_run` | `tools/peer` | gated: `AGEZT_PEERS` / `AGEZT_TENANT_PEERS` | POST task to a peer AGEZT node's `/api/v1/runs`; can route by model | `remote_run` | compensable | remote run on peer | per-tenant peer sets never leak across tenants (M219); 5 min timeout; 60 KiB answer cap; `netout.OperatorClient` (operator-configured peers; metadata refused) |
| `<prefix>.<tool>` | external plugins via `plugins` spec | gated: `AGEZT_PLUGINS` | out-of-process plugin tools | M900 manifest cap if known, else name switch → default-deny | n/a | plugin-defined | hash pins, tool allowlists, YieldOnConflict, stderr redaction |

### 3.2 Dynamic / kernel-owned tools (not in this scope, listed for completeness)

| Name | Registered by | Cap |
|---|---|---|
| `forge_<name>` | kernel/toolforge + runtime (via `cfg.ScriptRunner` = code_exec) | `code.exec` (switch prefix) |
| `mcp_<server>_<tool>` / lazy `mcp_<name>` | kernel/mcp | `mcp.call` (switch prefix) |
| `memory`, `world`, `delegate`, `delegate_await`, `market`, `voice`, `image_generate`, `rerank` | kernel/runtime + kernel/{memory,worldmodel,market,voicetool,imagetool,reranktool} | `memory`, `world`, `delegate`, `delegate`, `market.install`, `provider.call` ×3 — guarded by `kernel/runtime/capability_guard_test.go` |

---

## 4. Environment variables read by this area

| Env var | Read by | Effect |
|---|---|---|
| `AGEZT_ALLOW_ALL=1` | daemon → `BuildDeps.AllowAll` | network tools: AllowAll hosts + loopback + private |
| `AGEZT_HTTP_ALLOWED_HOSTS`, `_HTTP_ALLOW_ALL`, `_HTTP_ALLOW_LOOPBACK`, `_HTTP_ALLOW_PRIVATE` | `buildHTTP` | non-empty allowlist RESTRICTS (default = any public host); egress relaxations |
| `AGEZT_BROWSER_ALLOWED_HOSTS`, `_BROWSER_ALLOW_ALL`, `_BROWSER_ALLOW_LOOPBACK`, `_BROWSER_ALLOW_PRIVATE` | `buildBrowserRead` | same pattern for browser.read |
| `AGEZT_BROWSER_ACTIONS`, `_BROWSER_ACTION_DRIVER`, `_BROWSER_ACTION_NODE`, `_BROWSER_ACTION_ALLOWED_HOSTS`, `_BROWSER_ACTION_ALLOW_ALL`, `_BROWSER_ACTION_ALLOW_LOOPBACK`, `_BROWSER_ACTION_ALLOW_PRIVATE`, `_BROWSER_ACTION_ALLOW_USER_PROFILE` + `_BROWSER_ACTION_USER_DATA_DIR`, `_BROWSER_ACTION_ALLOW_REMOTE_CDP` + `_BROWSER_ACTION_REMOTE_CDP_URL`, `_BROWSER_ACTION_SESSION_DIR` | `buildBrowserAction` | opt-in Playwright tool family |
| `AGEZT_SANDBOX` (`off`), `AGEZT_SANDBOX_NO_NET=1` | `specCodeExec` | disable code_exec / force network off |
| `AGEZT_CODING_CMD` | `specCoding` | enables coding bridge |
| `AGEZT_ACP_AGENT_CMD` | `specACPAgent` | default ACP agent command |
| `AGEZT_HOMEASSISTANT_URL`, `_TOKEN`, `_TOOL_READ`, `_TOOL_SERVICES`, `_TOOL_ALLOW_ALL_SERVICES` | `specHomeAssistant` | HA tool gating |
| `AGEZT_PEERS`, `AGEZT_TENANT_PEERS` (JSON) | `specRemoteRun` | mesh peers |
| `AGEZT_PLUGINS`, `AGEZT_PLUGIN_PINS`, `AGEZT_PLUGIN_TOOLS` | `buildPlugins` | external plugin host |
| `AGEZT_OVERSEER_FLEET_LOCK` | `overseertool.fleetLockEnabled` (direct `os.Getenv`) | opt-in lock on agent-initiated edit/create/clone/delete |
| `AGEZT_TASK_MODEL_CHAINS` | written by overseer repair/routing; read/written by guardian-routing via `config` tool | per-task model chains |
| `AGEZT_CODING_TASK` | set by coding tool for the child | task handoff (no shell quoting) |
| `AGEZT_TOOLFORGE_AUTO_PROMOTE` | daemonconfig (01 doc) | default on — tested forge drafts auto-promote |
| `AGEZT_BROWSER_COOKIES` | listed in controlplane `configEnvVars` only | **no reader** — `browser.Tool.EnableCookies` is never called (dead knob) |

---

## 5. Deep sections

### 5.1 `code_exec` sandbox (`plugins/tools/codeexec`)

Per call (`codeexec_invoke.go:Invoke`):

1. Resolve language in `t.Runtimes` (from `DetectRuntimes`: `python` (Windows prefers `python` over the Store `python3` shim), `node`, `deno`; absolute interpreter paths). Language enum in the schema is exactly the detected set.
2. `workDir(project)`: no project → `os.MkdirTemp(<home>/sandbox, "run-")`, removed after (ephemeral); project → `<home>/sandbox/projects/<slug>` (slug = lowercase kebab alnum, cannot contain separators/`..`), persists. Dirs/files 0700/0600.
3. Write entrypoint (`main.py`/`main.js`/`main.ts`), extra `files` (each via `sanitizeRelFile`: rejects abs, `..`, NUL, any `:`), optional `stdin.txt`.
4. Profile = `t.Profile` (default `ProfileNamespace`) or ctx override; credential bucket = `executionprofile.ProfileIDForWardenProfile(w.EffectiveProfile(profile))` — the profile that will **actually** run (RCE-001 fix: non-Linux downgrades to None, so isolated-tier secrets must not follow).
5. Remote execution overrides (ctx): `invokeSSH` (scp upload, remote mkdir/pip/run, scp artifacts back, rm ephemeral), `invokeK8s` (kubectl cp/exec), `invokeModal` (artifact tar.gz base64 envelope between `__AGEZT_MODAL_ARTIFACTS_BEGIN__/END__` in stdout), `invokeDaytona`. Each uses its own client-env scrubber (`codeexec_remote_env.go`) and `executionprofile.ShellQuote`.
6. Local: env = `scrubEnv(dir)` (allowlist PATH/PATHEXT/COMSPEC/SYSTEMROOT/SYSTEMDRIVE/WINDIR/NUMBER_OF_PROCESSORS/PROCESSOR_ARCHITECTURE/LANG/LC_*; drops any name containing KEY/TOKEN/SECRET/PASSWORD/PASSWD/CRED/AWS_/AGEZT_; HOME/USERPROFILE/TMP* = work dir; `PYTHONDONTWRITEBYTECODE=1`, `PYLAUNCHER_ALLOW_INSTALL=0`) + `AppendEnvPassthrough(profileID)` + `PrepareSecretFileMounts` (profile-scoped vault secret files, cleaned up after).
7. Packages (python only, needs network): `validatePackages` rejects anything starting with `-` or containing whitespace/NUL; `python -m pip install --target <dir>/.deps` through warden (timeout = MaxTimeout, MaxOpenFiles 4096). Failed install short-circuits. `PYTHONPATH=<dir>/.deps` when present.
8. `warden.Run` with `buildArgv`: Deno gets `run --quiet --no-prompt --allow-read=<dir> --allow-write=<dir> --allow-env [--allow-net]` (real fs jail on every OS; deliberately no `--allow-run`); python/node get the warden profile only. Limits: timeout default 120 s / max 600 s, output 256 KiB, CPU 120 s, AS 2 GiB, 512 fds, 256 MiB files (rlimits real only on Linux namespace).
9. `publish` → bus event `KindCodeExecuted` (subject `code.exec`, actor `tool.code_exec`, payload language/code_bytes/exit/timed_out/net/profile_effective/duration/project).
10. `exportArtifactsFromDir`: files under `.agezt-artifacts/` (symlinks skipped; ≤32 files, ≤50 MiB each, ≤100 MiB total; tar extraction also path-sanitized) → `artifact.Index.PutEntry` (Source `code_exec`). Header line reports `isolation=<effective>` and whether it was downgraded — results never overstate containment.

Also exposes `RunScript(ctx, lang, code, inputJSON)` used as `runtime.Config.ScriptRunner` (forge tools) and `kernel.CodeExecutor` (Conductor Verifier). Network defaults ON (owner law: code-exec is deliberately max-capability; secret-scrub/isolation/audit are the non-negotiables).

| File | What it does |
|---|---|
| `doc.go` | Package doc: per-call dirs, scrubbed env, Deno jail, honest isolation reporting. |
| `codeexec.go` | Constants (timeouts, caps, rlimits), `Tool`, `artifactIndexer`, `NewWithWarden`, `Bind(bus)`, `SetIndex`, `Languages`, `Definition` (schema built from detected runtimes), `input`. |
| `codeexec_invoke.go` | `Invoke`: the execution router described above. |
| `codeexec_run.go` | `RunScript`, `workDir`, `resolveTimeout`, `render`/`renderRemote`/`renderRemoteProfile`, `publish`, `errResult`. |
| `codeexec_remote.go` | `invokeSSH`, `runSSHCommand`, `invokeK8s`, `runK8sCommand`, `invokeModal`, `wrapModalArtifactExport`. |
| `codeexec_remote_cmds.go` | `remoteRuntimeCommand`, `remotePipInstallCommand`, `remoteRunCommand`, `quoteCommand`. |
| `codeexec_remote_env.go` | `sshClientEnv`, `kubectlClientEnv`, `modalClientEnv`, `daytonaClientEnv` (allowlisted client envs). |
| `codeexec_remote_paths.go` | `remoteWorkDir`, `modalMountDir`, `k8sWorkDir`. |
| `daytona.go` | `invokeDaytona`, workspace materialization (`writeDaytonaFile`), artifact export, `runDaytonaCommand`, `daytonaWorkDir`. |
| `artifacts.go` | Export limits/consts, SSH/K8s artifact copy-back, `tempArtifactExportDir`, `exportArtifactsFromDir`, `exportTarGzBase64Artifacts`. |
| `artifacts_helpers.go` | base64 cleanup, safe tar.gz extraction, mime/kind detection, `appendArtifactExport`, `splitArtifactEnvelope`. |
| `packages.go` | `pyDepsName`, `validatePackages`, `pipInstall`, `installTail`. |
| `runtimes.go` | Lang consts, `DetectRuntimes`, `pythonCandidates`, `entryName`, `buildArgv`, `scrubEnv`, `isSecretName`, `slug`, `sanitizeRelFile`. |

Tests: `codeexec_test.go` (scrub/slug/sanitize/argv/render), `coverage_more_test.go`, `runscript_test.go`, `live_test.go` (host-runtime dependent).

### 5.2 `shell` (`plugins/tools/shell`)

- `Invoke` (`shell_invoke.go`): parse → timeout (30 s default; `timeout_ms` override is **not capped**) → execution-profile overrides (SSH/K8s/Modal/Daytona each with its own env allowlist: `sshEnv` keeps `SSH_AUTH_SOCK`, `kubectlEnv` keeps `KUBECONFIG`, `cloudCLIEnv` keeps Modal/Daytona config paths) → default path: profile (namespace requested; downgraded+journaled `warden.profile_downgraded` off-Linux), credential bucket on effective profile, per-agent workdir `WorkDir/<agent workdir>` (created lazily), env `scrubEnv(workDir)` + passthrough + secret file mounts → `warden.Run{Argv: [shell, arg, command], Actor: "tool.shell", CorrelationID}`.
- `scrubEnv` exists because warden defaults nil env to EMPTY (anti-leak), which breaks `cmd.exe` (M957); the scrubbed host env keeps PATH/SYSTEMROOT/COMSPEC etc. and drops every secret-shaped name and all `AGEZT_*`; HOME/USERPROFILE/TEMP point at the workdir so `~/.ssh`, `~/.aws` are not the shell's home.
- `renderResult` builds the final string (status prefix "timed out after…" / suffix "[exit code N]") **before** applying the 64 KiB budget; tail-truncates keeping the prefix and a `[truncated to last 64 KiB]` marker — the budget is on `Result.Output`, not per stream (warden caps each stream separately).
- Windows quoting: handled in `kernel/warden/cmdline_windows.go` — `SysProcAttr.CmdLine = cmd /S /C "<command>"` so os/exec MSVC escaping never mangles quoted paths (M958); guarded by `shell_quote_windows_test.go` (quoted path + trailing flag, nested `cmd /C`).

| File | What it does |
|---|---|
| `doc.go` | Package doc + M1.c security note (warden ProfileNone cross-platform; Edict is the gate). |
| `shell.go` | Consts (`DefaultTimeout`, `MaxOutputBytes`), `Tool`, `NewWithWarden`, `Definition`, `shellInput`. |
| `shell_invoke.go` | `Invoke` router (profile overrides, workdir, env, secret mounts, warden run). |
| `shell_render.go` | `renderResult` (budget/markers/exit code), `ShellHint`, `resolveShell`. |
| `env.go` | `scrubEnv`, `sshEnv`, `kubectlEnv`, `cloudCLIEnv`, `isSecretName`. |

Tests: `env_test.go`, `output_budget_test.go`, `render_budget_test.go`, `shell_path_test.go`, `shell_quote_windows_test.go` (windows build tag), `coverage_more_test.go`, `shell_test.go`.

### 5.3 `file` (`plugins/tools/file`)

- **Jail.** `New(root)` = Abs + EvalSymlinks (creates root if missing). `resolve(rel)`: abs or root-joined → if exists, EvalSymlinks and `withinRoot` check (applies to absolute in-root paths too, M252); if not, `resolveNewWithinRoot` walks up to the deepest existing ancestor, resolves it and re-checks (symlinked parent dir, M253). `withinRoot` uses `filepath.Rel` and rejects `..`-prefixed results. Errors wrap `ErrEscape`.
- **Walks.** `search`/`glob` call `entryEscapesRoot` for every symlink entry (WalkDir never follows links but ReadFile would, M427).
- **TOCTOU.** All opens go through `openFileNoFollow`: unix `O_NOFOLLOW`; Windows opens then verifies `GetFinalPathNameByHandleW` is inside root (junctions are creatable by non-admins); other OSes plain open.
- **Writes.** `write`/`append` open with no-follow; `replace` requires a unique match unless `all=true` and writes via local `atomicWriteFile` (temp O_EXCL + fsync + rename; `writeAll` seam for failure tests). Each mutating op first appends a `file.snapshot` rollback checkpoint (base64 content ≤8 MiB, mode, exists flag; refuses symlinks/dirs) to `<AGEZT_HOME>/rollback/checkpoints.json` (versioned JSON catalog, 0600, `internal/atomicfile`), consumed by `agt rollback apply`.
- Per-agent workdir rebasing for relative paths; read-only ops return `ObservationUntrusted` with source `workspace:<path>`.

| File | What it does |
|---|---|
| `doc.go` | Package doc: ops list, containment policy, op-enum↔dispatch lockstep test. |
| `file.go` | Caps, `Tool`, `New`, `NewWithCheckpoint`, `Root`, `Definition` (multi-axis capability), `fileInput`, `Invoke` dispatch. |
| `file_helpers.go` | `doRead`, `doReadRange` (line windows, 200 default lines). |
| `file_write.go` | `doWrite`, `doReplace`, `readUpTo`, `writeAll`, `atomicWriteFile`. |
| `file_explore.go` | `doList`, `doSearch` (literal/RE2, 8 MiB per-file scan cap), `doGlob`, `doStat`. |
| `file_explore_delete.go` | `doDelete` (refuses root/dirs, checkpoints first), `ErrEscape`. |
| `file_explore_paths.go` | `resolve`, `resolveNewWithinRoot`, `entryEscapesRoot`, `withinRoot`, `errResult`, `fileObservation`. |
| `checkpoint.go` | Rollback catalog types + `checkpointFileSnapshot`, load/append/write catalog. |
| `nofollow_unix.go` / `nofollow_windows.go` / `nofollow_other.go` | Platform `openFileNoFollow` implementations (build tags). |

Tests: containment (`..`, abs, symlink both paths, symlinked parent, walk links), `read_toctou_proof_test.go`, `nofollow_*_test.go`, `workdir_test.go`, checkpoint tests, `TestFile_EveryAdvertisedOpIsDispatched`.

### 5.4 Browser family (`plugins/tools/browser`)

- `browser.read` (`Tool`): stdlib only — no JS. `client()` = `netguard.New(opts).HTTPClient(30s)` with `CheckRedirect` re-applying the allowlist each hop (≤10). Per-invoke cookie jar shim (copy of client) if `Cookies` set (PSL-aware `cookiejar`, `cookies.go`) — but nothing in production enables it. `HTMLToText` (`htmltext.go`) is a regex/state-machine extractor (no `x/net/html`, lean-deps policy): strip script/style/noscript blocks (closed and unclosed), comments, tags; block tags → paragraph breaks; entity decoding; whitespace collapse.
- `browser.action` (`ActionTool`): spawns `node browse.mjs` (`runActionDriver`, `exec.CommandContext`, `envscrub.Scrubbed()`, JSON spec on stdin, 512 KiB stdout/stderr `limitedBuffer`). `prepareProfile`: `isolated` (default, strips session/tab/user dir), `session` (`<SessionRoot>/<session_id>` contained; ids `[A-Za-z0-9._-]{1,80}`, no leading dot), `user-attached` and `remote-cdp` (require operator env; CDP URL scheme http/https/ws/wss + egress check). Tab state (`action_tabs.go`) persists final URL + snapshot refs per session/tab as JSON so `tab_id` / `ref` can be reused; it is not a live tab. Output normalized, screenshots/downloads copied into the artifact index (`attachArtifacts`, only browser temp paths accepted), result is untrusted.
- `ResolveActionDriverPath` searches `plugins/builtinskills/browseruse/scripts/browse.mjs` relative to cwd ancestors and the executable — a runtime dependency of a Go tool on a skill bundle's script.

| File | What it does |
|---|---|
| `doc.go` | Package doc (read vs action split, tool naming). |
| `browser.go` | `browser.read` consts, `Tool`, `New`, `SetOnBlock`, netguard `client` with per-hop allowlist, `EnableCookies`, `Definition`, `ErrHostDenied`. |
| `browser_invoke.go` | `browser.read` `Invoke` (fetch→cap→HTMLToText→rune-safe truncate→JSON), `hostAllowed` (bare + `*.x` one-level wildcard). |
| `htmltext.go` | `HTMLToText` and its regexes. |
| `cookies.go` | `newDefaultJar` (PSL-partitioned cookiejar). |
| `action.go` | `ActionTool`, consts (timeouts, profiles), `NewAction`, `SetIndex`, `SetOnBlock`, `Definition` (full action schema). |
| `action_invoke.go` | `Invoke`, `textLimit`, `validateActions`, `validateActionOptions`, `prepareProfile`, `sessionDir`. |
| `action_runtime.go` | `runActionDriver`, `limitedBuffer`, `normalizeActionOutput`, artifact attach/save, truncation, `ResolveActionDriverPath`. |
| `action_tabs.go` | Tab state read/save, `ResolveTabRef`, `CloseSession`, `CloseTab`, `ListTabs`, `validateCDPURL`, `validateURL`, `validateHostEgress`. |
| `action_verbs.go` | `ActionVerb*` names, `ActionVerbTool`, `NewActionVerbTools`, verb `Definition`/`Invoke`. |
| `action_verbs_convert.go` | Verb → `actionInput` conversion, snapshot `ref` resolution, optional wait step. |
| `action_verbs_meta.go` | Per-verb description / effect / schema. |

Tests: `browser_test.go`, `redirect_test.go`, `cookies_test.go`, `action_test.go`, `action_e2e_test.go`, `coverage_*_test.go`.

### 5.5 Research / deep research (`plugins/tools/research` → `kernel/runtime/research_*.go`)

Tool is a thin front: `Runner.Research(ctx, corr, question, runtime.ResearchOptions{MaxSubQuestions, MaxSources, Verify (default true), MaxVerifyClaims})`; output JSON `{question, sub_questions, sources[{id,url,title,rank}], markdown, claims, confidence, cited_sources, verified, notes}` marked untrusted (`research:web`).

Kernel flow (`Kernel.Research`, see 04 doc):
1. **Plan** — aux completion (task type `research`, 512 tok, JSON array) → sub-questions (default 3, max 8; fallback = original question).
2. **Gather** — `k.RunTool(... "web_search" ...)` per sub-question (4 results each), URL-dedupe, cap sources (default 8, max 20).
3. **Fetch** — `k.RunTool(... "browser.read", max_chars 6000)`; fallback to search snippet; each source `S#` with sha256 prefix hash.
4. **Synthesize** — 2048-tok cited answer using only `[S#]` sources.
5. **Citation enforcement** — confidence from distinct cited sources.
6. **Adversarial verify** (if Verify) — refute-first check of each claim (≤6 default, max 12) against its source text; confidence = supported fraction; refuted claims appended as a warning inside the markdown.

Because inner calls go through `RunTool`, each search/fetch is itself Edict-gated and journaled under the same correlation.

### 5.6 Council and Conductor

- **`council`** (`Runner.Council(ctx, corr, question, nil, rounds)`): members default to `cfg.CouncilMembers()` (one seat per keyed provider, deduped; seat names "Elder Alpha/Beta/Gamma…" from `cmd/agezt/boot_tools.go:councilSeatName`). Grounding = today's date + optional web brief (≤6 web_search hits). Round 0 independent openings, then `rounds` (default 1) deliberation rounds seeing peers' latest; chair (first member) synthesizes consensus + dissent. Per-opinion 900 tok, consensus 1200. Events `council.*` with text clipped to 4000 runes. Cap `delegate` (multi-model consultation).
- **`conductor`** (`Runner.Conduct(ctx, corr, ConductorConfig{Task, Thinker, Worker, Verifier, MaxRounds, Plan})`): optional plan call, Thinker brief, Worker solution, Verifier — runs the worker's code via `kernel.CodeExecutor` (`*codeexec.Tool.RunScript`, injected by `specCodeExec.Configure` → `SetConductorExec`) or falls back to LLM critique; failed verify loops (default 2 rounds). Token bounds thinker 900 / worker 1600 / verifier 600 / plan 700. Cap **`code.exec`** — the verifier's sandbox call is in-kernel and never re-enters Edict, so the conductor itself must ride the code.exec grant (denying code.exec must close every code-running path).

Both tools hold only a `runner` pointer (no mutex; set once in Configure) and return `"<tool>: … unavailable"` when unbound.

### 5.7 Overseer (`plugins/tools/overseertool`)

- `Tool{mu, src Source}`; `Bind(NewKernelSource(k, baseDir))`. `Source` is a 25-method interface (read: IsHalted/ActiveRunIDs/Agents/OpenHelp/AgentImpact; intervene: CancelRun/Halt/ResumeAll/SetAgentEnabled/SetAgentRetired/EditAgent/CreateAgent/DeleteAgent/GetAgent/CloneAgent/SearchAgents/Bulk*/WakeAgent/RepairAgent).
- **Ops:** `status, agents, runs, help, cancel, halt, resume, pause, unpause, retire, revive, impact, edit, create, delete, get, clone, search, bulk_pause, bulk_unpause, bulk_retire, bulk_revive, bulk_delete, wake, repair`.
- **Protections:** `EditAgent` refuses `System` agents (agent path only; operator control plane can still edit) and uses PATCH semantics (only keys present in the input JSON apply; id/slug/enabled/retired/System never touched). `CreateAgent`/`CloneAgent` force `System=false`. `DeleteAgent` refuses System. `op=repair` refuses System targets in the tool (PE-006) but not in `kernelSource.RepairAgent` (operator console uses it). `AGEZT_OVERSEER_FLEET_LOCK` (opt-in) refuses agent-initiated edit/create/clone/delete (V-012).
- **WakeAgent** refuses retired/paused/managed-sub-agents, spawns `go k.RunWith(context.Background()+agent profile+MaxCost, …)` and returns the new correlation immediately.
- **RepairAgent** builds a brief from the reaper report (`repair.go:buildRepairBrief`), runs AS the agent, parses a closing JSON proposal (`soul, model, fallbacks, task_type, task_model_chain, config_overrides`) and applies it; routing changes update the governor's task chains live and persist `AGEZT_TASK_MODEL_CHAINS` via `settings.NewStore(baseDir).Save()` (`kernelsource_chains.go`).
- Flattened inputs: models often put profile fields at top level; `parseProfile` accepts either a `profile` object or flat fields, ignoring `overseerControlKeys`.
- `Invoke` ignores its ctx (`_ context.Context`).

| File | What it does |
|---|---|
| `overseer.go` | Package doc, `Source`, `BulkResult`, `SearchFilter`, `RepairResult`, `Tool`, `New`, `Bind`, `current`. |
| `tool_contract.go` | `Definition` (op enum, schema), `input`. |
| `tool.go` | `Invoke` op dispatcher, System guard for repair, `overseerControlKeys`. |
| `tool_helpers.go` | `parseProfile`, flat-field detection, `cancelNote`, `agentView`, `helpView`, `cleanSlugs`, `okJSON`, `errResult`. |
| `kernelsource.go` | `kernelSource`, `NewKernelSource`, fleet lock, read/intervene adapters, PATCH `EditAgent`, `CreateAgent`, `DeleteAgent`, `GetAgent`, `CloneAgent`. |
| `kernelsource_ops.go` | Bulk ops, `WakeAgent`, `SearchAgents`, `RepairAgent`, `RollbackRouting`, `ApplyRoutingChain`, `OpenHelp` (reads board under `<home>/board`). |
| `kernelsource_chains.go` | Task model chain read/set via governor (`TaskModelChainsView`/`SetTaskModelChains`), persist to settings. |
| `repair.go` | Repair brief builder, reaper lookups, proposal parse/clean/apply, task-type selection. |

### 5.8 Other tool packages — file tables

| Package / file | What it does |
|---|---|
| **acpagent** `doc.go` | Package doc (ACP client bridge, SPEC-15 §3). |
| `acpagent.go` | `transport`, `dialFunc`, `Tool{Cmd, Cwd, Timeout, dial}`, `New`, `Definition` (task + optional `agent` slug), `render`, `truncate`, `platformShell`, `AbsCwd`. |
| `acpagent_invoke.go` | `Invoke` (resolve via `acpcatalog.ResolveCommand`, spawn, ctx-watcher close, Initialize/NewSession/Prompt with bounded accumulation), `spawnAgent` (`exec.Command` via shell, scrubbed env, idempotent bounded close). |
| **artifacts** `artifacts.go` | `Index` interface, `Tool`, list (filter/limit)/read (text detection, 256 KiB)/delete. |
| **boardtool** `doc.go` | Package doc. |
| `board.go` | Limits, `boardStore` interface, `PostNotifier`, `Tool{mu, store, now, notify}`, `Bind(dir)` (test/legacy), `BindStore`, `OnPost`. |
| `tool.go` | `Definition` (10 ops; op inferred when omitted), `input`, `clampLimit`, `Invoke` (notifies on post/send/reply/broadcast/help). |
| `tool_helpers.go` | `applyActorIdentity` (no impersonation; defaults from/to), `msgView`, `okJSON`, `errResult`. |
| **coding** `coding.go` | Bridge: git-repo check, detached worktree in temp, run `AGEZT_CODING_CMD` via shell with scrubbed env + task env, `git add -A` + `git diff --cached HEAD` on a fresh 30 s ctx, worktree removal; `renderResult`, `execCommand`, `AbsRepo`. |
| **conductor** `conductor.go` | `Runner`, `Tool`, `SetRunner`, `Definition`, `Invoke`. |
| **config** `doc.go` | Package doc. |
| `config.go` | `Tool{baseDir, kernel}`, `SetKernel`, `Definition`, `Invoke`, `doSchema`, `doGet`, `doSet` (agent scope → roster ConfigOverrides; secrets → vault; else settings + live reload). |
| `config_helpers.go` | `configScope`, `doRegister`, `doUnregister`, `errf`. |
| **council** `council.go` | `Runner`, `Tool`, `SetRunner`, `Definition`, `Invoke` (opinions view). |
| **db** `db.go` | `Store` interface (datalake subset), `Tool`, `Definition`, `Invoke` op switch, `dataLakeActor`, per-op handlers, `jsonResult`. |
| **fetch** `fetch.go` | `Indexer`, `Tool`, `SetIndex`, `SetOnBlock`, netguard `client`, `Definition`, `Invoke` (GET → ≤50 MiB → `PutEntry`), `cleanMime`, `kindForMime`, `nameFromURL`. |
| **forgetool** `tool.go` | `Kernel` interface (Draft/Update/Test/RequestToolPromotion/ToolForge), `Tool`, `Bind`, `Definition`, `Invoke` (draft/test/update/request_promotion/list/show). |
| **homeassistant** `doc.go` | Package doc (separate allowlists from the HA channel; fixed destination). |
| `homeassistant.go` | `Tool{BaseURL, Token, AllowedServices, ReadEntities, HTTP}`, `Definition`, `Invoke`, `getStates` (single entity or filtered bulk), `callService`, `do` (Bearer). |
| `homeassistant_helpers.go` | `matchAllowed` (exact / `domain.*` / `*`), `Capabilities` banner string. |
| **http** `http.go` | Whole tool: `Tool`, `New`, `SetOnBlock`, netguard `client`, `Definition`, `Invoke` (GET/POST only), `hostAllowed`, `flattenHeaders`, `errResult`. |
| **introspecttool** `doc.go` | Package doc. |
| `introspect.go` | View types (`Overview`, `Delegation`, `Reaper*`), `Source`, `Tool`, `Bind`, `Definition`, `Invoke` (overview/reaper/schedules/standing). |
| `introspect_views.go` | `scheduleView`, `next`, `standingView`, `okJSON`, `errResult`. |
| `kernelsource.go` | Kernel adapter: `Overview`, `Schedules`, `Reaper`, `Standing`, provider-fallback health, reaper overview. |
| **mcptool** `tool.go` | `Kernel` interface (Add/Attach/Detach/Remove MCP, store, attached), `Tool`, `Bind`, `Definition`, `Invoke`. |
| **notify** `notify.go` | `Sender`, `Tool{mu, send, targets}`, `Bind` (prunes empty kinds), `Definition`, `Invoke` (send to every allowlisted id of the chosen/all kinds). |
| **peer** `doc.go` | Package doc (mesh delegation). |
| `peer.go` | `Peer`, `Tool` (global + tenant peers, model-list cache 60 s keyed by URL), `NewWithTenants`, `peersFor`, `Definition`. |
| `peer_http.go` | `httpPost`, `httpListModels` (Bearer), `ParsePeers` (`name=url\|token,…`), `ParseTenantPeers` (JSON). Also imported by `cmd/agt` doctor. |
| `peer_route.go` | `Invoke` (by peer name or by model routing), `render`, `routeCandidates`, `serversForModel`, `resolve`, `truncate`. |
| **runstool** `runs.go` | Journal-backed recent/stats/search over top-level runs (sub-agent runs folded out). |
| **schedule** `doc.go` | Package doc. |
| `schedule.go` | `store` interface, `Tool`, `Bind`, `BindAgentLookup`, `Definition`, `applyAssure`, acting-agent binding/validation, `scheduleTarget`. |
| `schedule_helpers.go` | `validateScheduledJob`, `applyTypedTarget`, `finalizeEntry`, `Invoke`, `parseHHMM`, `entryView`. |
| **sendmedia** `sendmedia.go` | `Tool` (sender, targets, resolver), `Bind`, `Definition`, `Invoke` (resolve artifact ref → attachment → pinned targets). |
| **skilltool** `skilltool.go` | Package doc + Forge interface, `Tool`, `Bind`. |
| `tool.go` | `Definition`, `Invoke` (learn/list/show/promote/retire/files/read). |
| **standingtool** `standing.go` | Standing CRUD interface, `Tool`, `Definition`, `Invoke` (create_event/create_cron/list/remove). |
| **websearch** `doc.go` | Package doc (keyless, SSRF-guarded, fail-soft). |
| `websearch.go` | Consts, `engineURL`, `Tool`, `New`, `SetOnBlock`, `client`, `Definition`, `Invoke`. |
| `websearch_helpers.go` | `parseResults`, `cleanURL` (strip `uddg=` redirect), `cleanText`, `softResult`, `errResult`. |
| **workboardtool** `doc.go` | Package doc. |
| `workboard.go` | `Kernel` interface (Create/Claim/Heartbeat/Comment/Block/Fail/Unblock/Complete/Archive/Link/RetryPolicy…), `Tool`, `Bind`, `Definition`. |
| `workboard_invoke.go` | `input`, `Invoke` op dispatch, `applyContextDefaults` (actor/corr). |
| `workboard_ops.go` | `list`, `create`, result helpers, `retryPolicyFromInput`, `taskView`. |
| **workflowtool** `tool.go` | Kernel interface over workflow store/runner, `Tool`, `Bind`, `Definition` (node library in schema), `Invoke` (save/run/enable/list/show). |
| `plugins/tools/tool_effects_test.go` | Package `tools_test`: walks the real registry with every gate switched on and asserts each tool's `Effect` is fully populated (code_exec constructed directly because runtimes are host-dependent). |

Every package has `*_test.go` (unit + `coverage_*_test.go`); peer has 9 test files (mesh routing, tenant isolation).

### Persistence summary (under `AGEZT_HOME`)

| Path | Writer |
|---|---|
| `rollback/checkpoints.json` | file tool (pre-mutation snapshots) |
| `sandbox/run-*` (ephemeral), `sandbox/projects/<slug>/` (+ `.deps/`, `.agezt-artifacts/`) | code_exec |
| `browser-sessions/<session_id>/` (+ tab state JSON) | browser.action (unless `AGEZT_BROWSER_ACTION_SESSION_DIR`) |
| settings file (`AGEZT_TASK_MODEL_CHAINS`, config `set`) | overseer chains, config tool |
| creds vault | config tool (secret fields) |
| artifact store, datalake, board, cadence, standing, roster, skill, toolforge, mcp, workflow, workboard stores | via kernel APIs (see 06/07 docs) |
| temp `agezt-coding-*` worktree (OS temp, removed) | coding |

### Concurrency model

- Tools are shared singletons invoked concurrently by parallel runs. Late-bound tools guard their binding with `sync.RWMutex` (notify, sendmedia, boardtool, overseertool, forgetool, mcptool, workboardtool, workflowtool, skilltool, schedule; peer guards its model-list cache). standingtool, runstool and introspecttool bind without a lock (written once in Configure before runs start). Set-once runner pointers (council/conductor/research/config/db/artifacts/fetch index) are written in Configure before any run.
- browser.read avoids mutating the shared client's Jar (per-invoke shallow copy).
- acpagent: one watcher goroutine per call closes the transport on ctx done; close is `sync.Once` with bounded post-kill wait.
- overseer `WakeAgent` fires an untracked goroutine with `context.Background()`.
- File rollback catalog append is load-modify-write without a package lock (see gotchas).

---

## 6. `plugins/builtinguardians` — self-healing guardian fleet

**Purpose.** Seed System-flagged guardian agents + their trigger into the roster at boot (M961). **Depends on** `kernel/cadence`, `kernel/roster`, `kernel/runtime`, `kernel/standing`. **Used by** `cmd/agezt/main.go:1535` (best-effort; seed failure never blocks startup).

`Host` interface (`Agents, AddAgent, UpdateAgent, StandingOrders, UpdateStanding, AddStanding, SetStandingEnabled, Schedules, Reschedule, AddInterval, AddDaily`), implemented by `kernelHost` over `*runtime.Kernel` (schedules tagged source `system` so env sync never prunes them).

| Slug | Trigger | Task type | Role (soul summary) |
|---|---|---|---|
| `guardian-health` | schedule every 12 h | research | one introspect overview sweep; notify only on real problems; overseer halt/resume only if critical |
| `guardian-doctor` | event `pulse.observer.system:reaper`, cooldown 8 h | research | diagnose reaper candidates; `overseer op=repair` misconfigured/degraded non-system agents; pause/retune; never touch guardians |
| `guardian-stuck` | schedule every 12 h | research | runs + overseer runs; cancel wedged/token-burning runs |
| `guardian-budget` | events `budget.exceeded`, `budget.cap.inert`, cooldown 8 h | research | find burner; pause or lower `max_daily_mc`; never raise ceilings |
| `guardian-routing` | events `provider.fallback`, `rate.limited`, cooldown 8 h | research | read/set `AGEZT_TASK_MODEL_CHAINS` via config tool; demote persistent offenders; never empty a chain |
| `guardian-code` | daily 03:00 local | coding | review forged tools + workspace code; apply safe fixes via file/code_exec |
| `guardian-initiative` | event `pulse.initiative.act`, cooldown 8 h, **seeded DISABLED** | research | M999 Pulse-initiative responder; act within ceiling or report |

Every seeded profile: `MemoryScope system/<slug>`, `MaxCostMc` $5/run, `MaxDailyMc` $10/day (`usd = 1e9` microcents), `TrustCeiling L2`, `NoisePolicy{SilentOnSuccess, DisableMemoryWrites, MinNotifySeverity warning, MinNotifyIntervalSec 8h}`, `ToolDeny [memory]`, `System true`. Standing triggers carry `Initiative{Mode: ActOrAsk, MaxTrust: L2}`.

**SeedAll flow:** for each guardian — present by slug → reconcile (`reconcileExistingGuardian` clamps MaxCostMc, trust ceiling down to L2, restores noise policy floors; `reconcileExistingGuardianStanding` raises cooldown floor; `reconcileExistingGuardianSchedule` restores mode/interval/daily time if drifted) and leave otherwise untouched (an operator's pause/removal of the trigger is respected); absent → `AddAgent` + `seedTrigger` (standing order or schedule; `disabled` flips the order off after `Add`). Returns `[]Seeded{Slug, Created, Trigger}` + first error.

| File | What it does |
|---|---|
| `builtinguardians.go` | Package doc, `Host`, `Seeded`, `guardian` spec, budget consts, the `guardians` fleet, `SeedAll`, reconcile functions, `seedTrigger`. |
| `builtinguardians_helpers.go` | `defaultGuardianNoisePolicy`, `notifySeverityRank`, `trustRank`, `appendUnique`, `sameEventSubjects`. |
| `kernelhost.go` | `kernelHost` adapter + `NewKernelHost`. |

Tests: `builtinguardians_test.go`, `coverage_supp_test.go` (fake Host; idempotence, reconcile clamps, disabled initiative).

---

## 7. `plugins/builtinmarket` and `plugins/builtinskills`

### builtinskills

- `//go:embed` of 16 directories into `bundles embed.FS`; `builtinBundles` (seed order) = browseruse, computeruse, dataanalysis, dockerservices, gitops, webresearch, pdftools, imagetools, sqldb, archivetools, httpapi, emailtools, sshremote, cryptotools, calendartools, officedocs.
- `Forge` interface (`Create, Promote, Get`), `SeedAll(f, corr)` (best-effort per bundle), `Names()`, `Bundle(name) (skillMD, resources, err)`, `seedOne` (parse SKILL.md frontmatter via `skill.ParseSkillMD` → `Create` (content-addressed: unchanged bundle dedupes) → promote up to 3 steps to `StatusActive`).
- Depends only on `kernel/skill`. Used by `cmd/agezt` (seed) and `builtinmarket`; `browser.ResolveActionDriverPath` also finds `browseruse/scripts/browse.mjs` on disk.

| Bundle dir | Skill name | Purpose | `tools:` | Files |
|---|---|---|---|---|
| archivetools | archive-tools | zip/tar pack/unpack/list with traversal guard | code_exec, shell, artifacts | `reference/recipes.md`, `scripts/arc.py` |
| browseruse | browser-use | headless browser automation (Playwright) | code_exec, shell, artifacts | `reference/actions.md`, `scripts/browse.mjs`, `scripts/setup.sh` |
| calendartools | calendar-tools | write/parse .ics events | code_exec, shell, artifacts | `recipes.md`, `scripts/cal.py` |
| computeruse | computer-use | install software, desktop GUI automation | shell, code_exec, artifacts | `reference/patterns.md`, `scripts/desktop.py`, `setup.sh` |
| cryptotools | crypto-tools | hashes, HMAC, base64, random tokens | code_exec, shell | `recipes.md`, `scripts/crypto.py` |
| dataanalysis | data-analysis | pandas analysis + charts | code_exec, shell, artifacts | `recipes.md`, `scripts/analyze.py`, `setup.sh` |
| dockerservices | docker-services | run/inspect/log/teardown detached services | shell, code_exec | `reference/services.md`, `scripts/svc.sh` |
| emailtools | email-tools | SMTP send / IMAP read | code_exec, shell | `recipes.md`, `scripts/mail.py` |
| gitops | git-ops | clone/branch/commit/push/PR via gh | shell, code_exec | `recipes.md`, `scripts/gitflow.sh` |
| httpapi | http-api-client | full-method REST client | code_exec, shell, fetch | `recipes.md`, `scripts/api.py`, `setup.sh` |
| imagetools | image-tools | inspect/resize/convert/crop/stamp images | code_exec, shell, artifacts | `recipes.md`, `scripts/img.py`, `setup.sh` |
| officedocs | office-docs | generate .docx / .xlsx | code_exec, shell, artifacts | `recipes.md`, `scripts/office.py`, `setup.sh` |
| pdftools | pdf-tools | extract text/tables, merge/split | code_exec, shell, artifacts | `recipes.md`, `scripts/pdf.py`, `setup.sh` |
| sqldb | sql-db | SQLite/Postgres/MySQL queries → JSON | code_exec, shell, artifacts | `recipes.md`, `scripts/db.py`, `setup.sh` |
| sshremote | ssh-remote | remote commands + SFTP | code_exec, shell | `recipes.md`, `scripts/ssh.py`, `setup.sh` |
| webresearch | web-research | multi-source cited research | fetch, code_exec, shell, browser | `recipes.md`, `scripts/extract.py`, `setup.sh` |

Note `browser` in webresearch's `tools:` list is not a real tool name (`browser.read`/`browser.action` are).

| File | What it does |
|---|---|
| `builtinskills.go` | Embed directive, `Forge`, `Seeded`, `builtinBundles`, `SeedAll`, `Names`, `Bundle`, `seedOne`. |

Tests: `builtinskills_test.go`, `coverage_supp_test.go` (every bundle parses, seeding promotes to active, idempotence).

### builtinmarket

- `MarketplaceName = "official"`; `Library` implements `market.Library` (`Marketplaces()`, `ResolvePack(_, name, version)`). Built by `New()`: one `<bundle>-pack` per builtin bundle (version 1.0.0, author AGEZT, category from `skillCategory`, tags = bundle name + triggers) + combo packs. Composite with the synced store in `cmd/agezt` (`market.NewCompositeLibrary(builtinmarket.New(), marketStore)`), builtin consulted first.
- `ResolvePack` with an explicit version that differs returns an error (never a silent substitute) so the composite falls through to synced marketplaces.

| Combo pack | Skills | MCP servers (all `Lazy`) | Tool requirements | Featured |
|---|---|---|---|---|
| web-research-pro | webresearch | `fetch` = `uvx mcp-server-fetch` (PyPI; DEP-006) | rg, fd | yes |
| git-workshop | gitops | — | git, rg, fd, delta | |
| github-automation | gitops, httpapi | — | git, gh, jq | yes |
| data-analyst-pro | dataanalysis, sqldb | — | sqlite3, jq | yes |
| second-brain | webresearch | `memory` (`@modelcontextprotocol/server-memory`), `thinking` (`server-sequential-thinking`) | — | yes |
| browser-automation-pro | browseruse | `playwright` (`@playwright/mcp@latest`) | — | yes |
| deep-search | webresearch | `duckduckgo` (`uvx duckduckgo-mcp-server`) | rg | |
| document-suite | officedocs, pdftools | `excel` (`uvx excel-mcp-server stdio`) | pandoc | |
| personal-organizer | emailtools, calendartools | `time` (`uvx mcp-server-time`) | — | |
| container-ops | dockerservices | — | docker, jq | |
| secops-toolkit | cryptotools, sshremote | — | openssl, ssh | |
| media-workshop | imagetools, archivetools | — | ffmpeg | |

| File | What it does |
|---|---|
| `builtinmarket.go` | Categories, `combo` + `combos`, `Library`, `New`, `add`, `categoryFor`, `Marketplaces`, `ResolvePack`. |

Tests: `builtinmarket_test.go`, `resolveversion_test.go`.

---

## 8. Guard tests that pin this area

| Test | Location | Asserts |
|---|---|---|
| `TestRegisterAllMatchesPinnedSpecList` | `builtintools/ratchet_test.go` | exact ordered spec list; plugins last |
| `TestBootTools_ResolveToGovernedCapabilities` | `builtintools/capability_guard_test.go` | every boot tool × every `opProbes` input resolves (via the **name switch**) to `edict.KnownCapability` — unknown = default-deny = dead tool |
| `TestBootTools_DeclaredCapabilityMatchesTheNameSwitch` | same | `ToolDef.Capability.For(probe)` == name-switch result (parity during the Phase 3.4 migration) |
| `TestBootTools_DeclareTheirCapability` | same | every built boot tool declares a non-zero Capability |
| `TestGovernedCapabilities_AllowedByDefault` | same | every `edict.AllCapabilities()` decides Allow with default options (owner posture) |
| `TestRegistryNetguardWiring_CoversEveryNetguardSpec` | `builtintools/tools_test.go` | every Netguard spec's instances implement `NetguardAware` (also checked at boot via `NetguardGaps`) |
| `TestFirstPartyToolDefinitionsDeclareEffects` | `plugins/tools/tool_effects_test.go` | every tool (all gates ON) fills Effect class/effects/resources/rollback/confidence |
| `TestRuntimeTools_*`, `TestPreviouslyUngovernedTools_*` | `kernel/runtime/capability_guard_test.go` | mirror guard for kernel-registered tools |
| `TestCapabilityForToolCall` | `kernel/edict/toolmap_test.go` | name-switch table |
| `TestFile_EveryAdvertisedOpIsDispatched` | `tools/file` | schema op enum ↔ Invoke switch lockstep |
| configEnvVars guard | `kernel/controlplane` (see 03 doc) | every `AGEZT_*` read in `cmd/agezt` is listed in `configEnvVars` |

---

## 9. Extension points — how to add a new first-party tool

1. **Create** `plugins/tools/<pkg>/` implementing `agent.Tool`. In `Definition()` set `Name`, `Description`, `InputSchema`, a full `Effect` (all five fields — otherwise `tool_effects_test` fails), and **`Capability`** joining an existing Edict axis. For multi-axis tools set `Field` + `ByValue` and choose the fallback `Name` deliberately (readers fall back to the read axis; installers to the gated axis; `file` intentionally has none so unknown ops default-deny).
2. **If a genuinely new axis is required**: add the `Cap*` constant in `kernel/edict/edict.go`, include it in `AllCapabilities()` / defaults (`edict_defaults.go`) at **Allow** (owner decision needed otherwise — `TestGovernedCapabilities_AllowedByDefault` will fail), and add the name case in `kernel/edict/toolmap.go` (still the fallback and what the guard's resolution test reads — `TestBootTools_ResolveToGovernedCapabilities` calls the switch directly, so a tool known only via its declaration still fails it).
3. **Register a spec** in `plugins/builtintools` (choose the file by lifecycle: `envgated.go`/`tools_helpers.go` for Build-only, `inject.go`/`kernelbound.go` for Configure, `latebound.go` for Late). Close over the concrete instance; mark `Netguard: true` and implement `SetOnBlock` if it dials out (build the client with `netout.Egress{...}.Client` — it re-checks the host allowlist on every redirect hop; archcheck rejects a hand-built `http.Client`).
4. **Insert the name in `RegisterAll()`** in its banner position (never after `plugins`) and in `ratchet_test.go:bootSpecNames` at the same position.
5. **If the schema has a steering field**, add every enumerated value to `opProbes` in `capability_guard_test.go` (the `file` op=glob bug was a missing branch). If env-gated, switch it on in `tool_effects_test.go`'s permissive env and add it to that test's `want` list.
6. **New `AGEZT_*` env vars** read via `BuildDeps.Get` that are operator-facing must be added (alphabetically) to `kernel/controlplane/config.go` `configEnvVars`; document them in Config Center schema if editable.
7. If the tool runs processes: route through `warden.Engine` (as shell/code_exec do) with an allowlisted env (never `nil` env inheriting the daemon, never raw `os.Environ()`); if it returns external content set `ObservationTrust: agent.ObservationUntrusted`.
8. If the console catalog should show the risky axis of an input-branching tool, add a probe to `kernel/controlplane/tool.go:catalogProbe`.
9. Run the source package's tests plus `go test ./plugins/builtintools/ ./plugins/tools/ ./kernel/edict/ ./kernel/runtime/` (GOMAXPROCS≤4).

Adding a **guardian**: append to `guardians` (slug `guardian-*`, exactly one trigger, terse ACT-or-report soul), update the boot banner string in `main.go` and tests. Adding a **skill bundle**: new dir with `SKILL.md` (+ `reference/`, `scripts/`), add it to the `//go:embed` line and `builtinBundles`, a `skillCategory` entry in builtinmarket, and test expectations. Adding a **combo pack**: append to `combos` (prefer key-less MCP servers; verify the package really exists on npm/PyPI).

---

## 10. Gotchas / invariants

- **Unknown capability ⇒ default-deny ⇒ dead tool.** That is how `conductor` and `market` once shipped unusable; the guards in §8 exist because the failure is silent at run time ("no trust level configured for <tool>").
- **browser.action and its 10 verbs declare no `ToolDef.Capability`** — they resolve only through the edict name switch. `TestBootTools_DeclareTheirCapability` doesn't catch it because `buildRealBootTools` runs with an empty env, so browser.action is gated off in that walk. Same for any env-gated tool (coding, acp_agent, homeassistant, remote_run, notify/send_media, code_exec on a runtime-less CI box) — they happen to declare, but the guard does not prove it.
- **Model-facing descriptions/comments drift from posture:** mcptool/acpagent/peer docs say "Ask by default"/"Ask-first", but all capabilities are Allow by default. `tool_forge`'s description says a draft goes live only when the operator approves, yet `AGEZT_TOOLFORGE_AUTO_PROMOTE` defaults ON (auto-promotes tested tools).
- **`AGEZT_BROWSER_COOKIES` is a dead knob**: listed in controlplane `configEnvVars`, but `browser.Tool.EnableCookies` is never called; `tools_helpers.go` comments it as handled by a "browser.cookies tool" that is actually the Playwright verb.
- **Overseer System guard is tool-path only**: `EditAgent` refuses System agents inside `kernelSource`, but `op=repair`'s refusal lives in `tool.go` only (operator path via `RepairAgent` stays open by design). Fleet lock is opt-in.
- **Overseer persists settings directly** (`settings.NewStore(baseDir).Save()` for `AGEZT_TASK_MODEL_CHAINS`) from a plugin — a second writer of the settings file besides the control plane/config tool; concurrent writes are last-writer-wins.
- **WakeAgent uses `context.Background()`** — a woken run is detached from the caller's cancellation (intended async semantics, but it bypasses any caller ctx deadline).
- **File rollback catalog** (`appendRollbackCheckpoint`) is an unlocked load-modify-write of one JSON file; concurrent mutating file ops in parallel runs can drop a checkpoint. Checkpoints are skipped for files >8 MiB (the write then fails rather than proceeding unchecked).
- **coding tool runs git with the daemon's full environment** (`t.run(..., nil, "git", ...)` = inherit); only the external agent gets the scrubbed env. Repo-local git hooks (e.g. post-checkout on `worktree add`) execute with daemon secrets in env. acpagent/coding/browser driver bypass warden (plain `os/exec`), so no warden audit event or rlimits for them.
- **browser.action egress check is pre-resolution**: URLs and `goto` steps are validated by resolving DNS and checking netguard, but the Chromium process dials independently (DNS-rebinding window; in-page redirects/subresources are not re-checked). browser.read/http/fetch/web_search enforce at dial time via the netguard transport.
- ✅ **Fixed (W1.4):** fetch had no host allowlist (any public host, 50 MiB) even when `AGEZT_HTTP_ALLOWED_HOSTS` restricted `http`; both now take one posture (`builtintools.httpEgress`). The http and browser allowlist matchers also disagreed on `*.x.com` depth — one grammar now (`netout.Egress.HostAllowed`, exactly one level).
- **shell `timeout_ms` is uncapped** (code_exec caps at 600 s, browser.action at 120 s).
- **Windows**: shell quoting correctness lives in `kernel/warden/cmdline_windows.go`, not in the tool; `scrubEnv` must keep SYSTEMROOT/COMSPEC/PATH or `cmd.exe` fails; code_exec prefers `python` over the Store `python3` shim and sets `PYLAUNCHER_ALLOW_INSTALL=0`.
- **Registry order is load-bearing** (banner + plugins-last). `toolreg.Names()` exists only for the ratchet and is allowlisted in `tools/deadcodecheck`.
- **Boot banner drift**: `main.go` prints "seeded N (health, doctor, stuck, budget, routing, code)" — omits `guardian-initiative` (7 guardians ship).
- **Layering**: `cmd/agezt` imports `plugins/tools/codeexec` directly (display-only `Languages()` banner type assertion); `cmd/agt` imports `plugins/tools/peer` for doctor checks. Kernel never imports plugins — Conductor uses the `runtime.CodeExecutor` interface, council/research/conductor tools import `kernel/runtime` (plugin → kernel only).
- **browser.action depends on a skill bundle file** (`plugins/builtinskills/browseruse/scripts/browse.mjs`) located by walking cwd/executable ancestors; packaged installs must set `AGEZT_BROWSER_ACTION_DRIVER`.
