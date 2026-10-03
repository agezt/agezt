# 07 — Autonomy & Extensibility

**Scope:** `kernel/roster`, `kernel/cadence` (+ `cadence/systemtasks`), `kernel/scheduler`, `kernel/standing`, `kernel/pulse`,
`kernel/selfrepair`, `kernel/skill`, `kernel/market`, `kernel/plugin`, `kernel/mcp`, `kernel/acp`, `kernel/acpcatalog`,
`kernel/workflow`, `kernel/workflowexec`, `kernel/update`, `kernel/toolbox`, `kernel/channel`, `kernel/channelwire`,
`kernel/stt`, `kernel/voicetool`, `kernel/imagetool`, `kernel/reranktool`, `kernel/internal/testfixtures`.
Also covers the parts of `cmd/agezt` and `kernel/runtime` that turn these packages into running behaviour,
because most "fire" logic lives in the daemon, not in the packages.

Sibling docs: [00-README.md](00-README.md) · [01-daemon-boot-cmd-agezt.md](01-daemon-boot-cmd-agezt.md) ·
[02-cli-cmd-agt.md](02-cli-cmd-agt.md) · [03-control-plane-and-http.md](03-control-plane-and-http.md) ·
[04-agent-runtime.md](04-agent-runtime.md) · [05-governance-routing-security.md](05-governance-routing-security.md) ·
[06-data-memory-state.md](06-data-memory-state.md) · [08-providers.md](08-providers.md) · [09-channels.md](09-channels.md) ·
[10-tools.md](10-tools.md) · [11-frontend-console.md](11-frontend-console.md) · [12-sdks-and-contract.md](12-sdks-and-contract.md)

---

## Responsibilities at a glance

| Concern | Package(s) | One-liner |
|---|---|---|
| Who an agent **is** | `roster` | Durable named identities (`Profile`): soul, model chain, budgets, policies, hierarchy, lifecycle, `System` guardian flag. `<home>/roster/roster.json`. |
| Wake on **time** | `cadence` (+`systemtasks`) | Persistent schedules (interval/daily/once/window/continuous) firing an intent run, workflow, typed daemon system task, or a tool. |
| Wake on **events / cron rules** | `standing` | Standing orders: event-subject globs + 5-field cron → governed run with initiative mode, trust ceiling, budget, assure, briefing. |
| Wake on **observation** | `pulse` | Heartbeat: observers → salience → initiative (`off|ask|act`) → briefing. Emits `pulse.initiative.act` for a standing order to bind. Never acts itself. |
| Fix broken agents | `selfrepair` | Reaper-driven auto-repair coordinator: claim → overseer repair / routing rollback → escalate to parent/owner → apply JSON resolution. |
| Plan DAGs | `scheduler` | NOT a time scheduler: plan DAG executor (LoopNode = one agent run, GateNode = approval) with bounded parallelism. |
| Learned procedures | `skill` | "Forge": content-addressed skills with a journaled lifecycle (draft→shadow→active→quarantined→archived), agentskills.io SKILL.md + bundles, per-agent ownership. |
| Capability packs | `market` | Packs = skills + MCP servers + CLI-tool needs; built-in Official catalogue + synced remotes; Ed25519 signing; heuristic vetting. |
| Out-of-process tools | `plugin` | Child process speaking line-delimited JSON on stdio; BLAKE3 pin, tool allowlist, capability declaration. |
| MCP client | `mcp` | Registry of MCP servers + stdio and Streamable-HTTP clients; tools bridged as `mcp_<server>_<tool>` or one lazy `mcp_<server>` dispatcher. |
| IDE / external coding agents | `acp`, `acpcatalog` | ACP server (IDE → agezt via `agt acp`) + ACP client (agezt → Gemini/Claude Code/Codex) + discovery catalog. |
| Host CLI tools | `toolbox` | 38-tool inventory, per-OS package-manager install (host level, unsandboxed). |
| n8n-style automation | `workflow` (+ `kernel/runtime/workflowrun*.go`) | Typed node graph, `{{path}}` templating, cron/event/webhook/manual triggers, per-node retry/timeout, journal-backed run inspection. |
| Messaging surfaces | `channel`, `channelwire` | `Channel` interface + `UnifiedMessage` vocabulary; factory layer building per-account instances; journal-folded conversation history. |
| Media tools | `stt`, `voicetool`, `imagetool`, `reranktool` | OpenAI-compatible STT client; `voice` / `image_generate` / `rerank` agent tools over injected adapter seams. |
| Self-update | `update` | Release check (GitHub / endpoint), download, SHA-256 + Ed25519 verify, drain, atomic rename. Currently inert by default (see Gotchas). |

## Import graph (from `internal-deps.txt`)

| Package | Imports (internal) | Imported by (non-test) |
|---|---|---|
| `roster` | edict, filestore, ulid | cmd/agezt, agentgw, controlplane, runtime (+accessors, runexec), selfrepair, builtinguardians, tools/{config, overseertool, schedule, standingtool} |
| `cadence` | bus, event, filestore, ulid | cmd/agezt, cmd/agt, cadence/systemtasks, controlplane, runtime (+accessors, runexec), builtinguardians, tools/{introspecttool, schedule} |
| `cadence/systemtasks` | brand, cadence, catalog, event, runtime | cmd/agezt only |
| `scheduler` | agent, approval, bus, event, intent, ulid | controlplane, runtime (+accessors, runexec) |
| `standing` | bus, filestore, ulid | cmd/agezt, controlplane, runtime (+accessors, runexec), builtinguardians, tools/{introspecttool, standingtool} |
| `pulse` | agent, bus, event, state, ulid, warden | cmd/agezt, alerter, channelwire, builtinchannels |
| `selfrepair` | brand, strutil, board, bus, event, roster, runtime, **plugins/tools/overseertool** | cmd/agezt only |
| `skill` | agent, bus, event, filestore | cmd/agezt, cmd/agt, contextselect, controlplane, market, runtime (+accessors, runexec), builtinmarket, builtinskills, tools/skilltool |
| `market` | atomicfile, agent, edict, mcp, netguard, skill | cmd/agezt, cmd/agt, controlplane, runtime (+accessors), builtinmarket |
| `plugin` | agent | cmd/agt, plugins/builtintools |
| `mcp` | filestore, netguard, ulid | controlplane, market, runtime (+compose), builtinmarket, tools/mcptool |
| `acp` | brand | cmd/agt, tools/acpagent |
| `acpcatalog` | — | controlplane, tools/acpagent |
| `workflow` | bus, event, filestore, ulid | cmd/agezt, controlplane, runtime, tools/workflowtool |
| `workflowexec` | — | **nobody** (dead; see §16.5) |
| `update` | brand, netguard | cmd/agezt, controlplane, restapi |
| `toolbox` | — | controlplane |
| `channel` | strutil, bus, event | cmd/agezt, channelwire, controlplane, toolreg, builtinchannels, every plugins/channels/*, tools/sendmedia |
| `channelwire` | bus, channel, pulse, settings | cmd/agezt, builtinchannels |
| `stt` | — | cmd/agezt, cmd/agt |
| `voicetool` / `imagetool` / `reranktool` | agent, edict | runtime (+compose, runexec) |
| `internal/testfixtures` | agent | test files only (agent, controlplane, runtime `mock_helpers_test.go`) |

Layering note: `kernel/selfrepair` (and `kernel/controlplane`) import `plugins/tools/overseertool`. These are the only
kernel→plugins imports outside tests. They break the "kernel never imports plugins" rule. See §7.6.

---

# Part I — Agent identity and the wake model

## 1. `kernel/roster` — agent identity

**Purpose.** The durable store of named agent identities (M783). `agt run --agent X`, schedules, standing orders and channels
can all run "AS" a profile. The package only holds data and validation. It imports only `filestore`, `ulid` and `edict`
(the last for `ParseTrustLevel`). Journaling happens one layer up, in `kernel/runtime/accessors_roster.go`.

### 1.1 `roster.Profile` (every field, `roster.go:38-133`)

| Field (JSON) | Meaning / validation |
|---|---|
| `ID` (`id`) | ULID assigned by `Store.Add`. Caller-supplied value ignored. |
| `Slug` (`slug`) | Unique, immutable address. Regex `^[a-z0-9][a-z0-9._-]{0,63}$`. |
| `Name` (`name`) | Human label. Defaults to the slug. |
| `Soul` (`soul`) | System prompt ("who it IS"), ≤64 KiB. Applied as the run's system override. Memory, world and skill injection layer on top. |
| `Instructions` (`instructions`) | Durable operating rules. ≤64 entries × 4096 B. |
| `Model` / `Fallbacks` (`model`, `fallbacks`) | Primary model plus an ordered per-agent chain (≤8). Empty Model = routing decides (no default provider exists). |
| `TaskType` (`task_type`) | Default governor task class (coding, research, …). |
| `MaxCostMc` (`max_cost_mc`) | Per-run ceiling in USD-microcents. An explicit per-run cap wins. |
| `MaxDailyMc` (`max_daily_mc`) | Per-day ceiling metered by the Governor identity ledger (M793). |
| `MemoryScope` (`memory_scope`) | Private memory scope. Empty = slug (per-agent memory, private by default). |
| `Workdir` (`workdir`) | Workspace-relative subdirectory. No absolute path, no `..` escape. |
| `OwnerAgent` / `ParentAgent` | Hierarchy slugs. A profile cannot own or parent itself. |
| `DirectCallable` (`direct_callable`, `*bool`) | nil/true = operators, schedules and channels may wake it directly. false = managed sub-agent, reachable only via delegation; requires an owner or parent. |
| `RetryPolicy` (`retry_policy`) | `{MaxAttempts 0..10, Backoff fixed|exponential, BaseDelaySec, MaxDelaySec, RetryOn [error,timeout,canceled,halted]}`. Used by `RunWithRetry`. |
| `HealthPolicy` (`health_policy`) | `{StaleAfterSec, FailureWindow, FailureThreshold, DoctorAgent}`. |
| `SelfRepairPolicy` (`self_repair`) | `{Enabled, MaxAttempts 0..10, EscalateTo}`. Opt-in gate for `selfrepair`. |
| `NoisePolicy` (`noise_policy`) | `{SilentOnSuccess, DisableMemoryWrites, MinNotifySeverity, MinNotifyIntervalSec}`. `DisableMemoryWrites` forces `memory` into `ToolDeny` at the store boundary. |
| `ToolAllow` / `ToolDeny` | ≤256 each. Regex `^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`. A name may not appear in both. |
| `TrustCeiling` (`trust_ceiling`) | `L0..L4` (upper-cased), validated by `edict.ParseTrustLevel`. |
| `ExecutionProfile` | `local|warden|container`. Default isolation on the workboard dispatch path. |
| `ConfigOverrides` | Per-agent `AGEZT_*` overrides. Keys `^AGEZT_[A-Z0-9_]+$`, ≤128 entries, values ≤8 KiB. |
| `Lifecycle` (`AgentLifecycle`) | `{Mode persistent|cycle|retire_on_complete, RetireOnComplete, MaxCycles, CompletedCycles, LastCompletedRun}`. `LastCompletedRun` makes cycle counting idempotent per correlation. |
| `TaskList` (`tasklist`) | `[]AgentTask{ID, Title, Description, Scope cycle|total, Status todo|doing|done|blocked|retired, …}`, ≤200. |
| `Description`, `Enabled` | `Enabled=false` = paused. Has its own setter. |
| `Retired`, `RetiredMS`, `RetiredReason` | "Graveyard" (M846). Recoverable via revive. Excluded from delegation. Retiring also pauses. |
| `System` | Shipped guardian flag (M961). Kernel-owned (see 1.3). |
| `CreatedMS`, `UpdatedMS` | Timestamps. |

There are no channel, comms, skill or persona fields. Persona = `Soul` + `Instructions`. Skills carry their own owner
(`skill.Skill.Agent`, §9). The daemon's default identity (`k.System()`) is not a roster profile.

**Helpers:**
- `Kind()`: `"system"` | `"subagent"` (not directly callable) | `"custom"`.
- `AllowsDirectCall()`, `AllowsDelegationFrom(caller)`.
- `AutonomyRunbook(p)` (`autonomy.go`): the canonical "wake contract" payload. It is attached as `autonomy_runbook`
  to `schedule.fired` / `standing.fired` events and to self-repair wakes.

**Store API (`roster_store.go`):**
- `Open(dir)`, `Add`, `Update(ref, mutate)`, `SetEnabled`, `SetRetired(ref, bool, reason...)`, `Remove`, `Get`, `List`, `Count`.
- `Validate(p)` lives in `roster_validate.go`.
- `ErrNotFound`, `ErrRetired`.

### 1.2 System agents vs user agents

- **System agents** are the guardians seeded by `plugins/builtinguardians.SeedAll` (`cmd/agezt/main.go:1535`) with `System: true`.
  There are seven:

  | Guardian | Wakes on | Wired as |
  |---|---|---|
  | `guardian-health` | every 12h | cadence schedule (`source:"system"`) |
  | `guardian-stuck` | every 12h | cadence schedule |
  | `guardian-code` | daily 03:00 | cadence schedule |
  | `guardian-doctor` | event `pulse.observer.system:reaper` (cooldown 8h) | standing order |
  | `guardian-budget` | events `budget.exceeded`, `budget.cap.inert` | standing order |
  | `guardian-routing` | events `provider.fallback`, `rate.limited` | standing order |
  | `guardian-initiative` | event `pulse.initiative.act` | standing order, **seeded DISABLED** |

  Event guardians get a standing order with `Initiative{Mode: act_or_ask, MaxTrust: "L2"}`.
  More detail in [10-tools.md](10-tools.md).
- `applySystemGuardianDefaults` (`roster_normalize_helpers.go:53-99`) runs on every Add/Update and as a migration at `Open`. It sets:
  - `MemoryScope = "system/<slug>"`;
  - cost caps of $0.05 when ≤0;
  - `TrustCeiling` L4 when empty;
  - a noise floor: silent on success, no memory writes, severity ≥ warning, ≥8h notify interval.

  The seeder sets explicit, stricter values: $5/$10 caps, L2, `ToolDeny:["memory"]`. `runtime/runctx_agent.go` also enforces
  the System noise floor at run time.
- **User agents** are `Kind()` `custom` or `subagent`, created from the console, CLI or the overseer tool.

### 1.3 How system agents are protected

1. `Store.Update` restores `System` (and `ID`, `Slug`, `CreatedMS`, `Enabled`, `Retired*`) from the pre-mutation snapshot
   (`roster_store.go:160-162`, MASS-003). Mutators run under `safeCall`, which converts a panic into an error.
2. Clients cannot set it: `controlplane/roster_crud.go:38` forces `System=false` on add. `overseertool/kernelsource.go:193,235`
   does the same on create and clone.
3. No hard delete: `runtime/accessors_roster.go:113-118` and `controlplane/roster_lifecycle.go:110` refuse with
   "protected system guardian — pause or retire it".
   **`roster.Store.Remove` itself does not check `System`.** The guard lives one layer up.
4. Agent-reachable edits and repairs refuse System targets (`overseertool/kernelsource.go:102`, `overseertool/tool.go:279`).
   Operators can still edit, pause and retire guardians.
5. The reaper (`runtime/reaper_scan.go:65`) and auto-repair (`selfrepair_filters.go:28`) skip System agents.

### 1.4 Persistence, concurrency, events

| Aspect | Details |
|---|---|
| File | `<home>/roster/roster.json` (`runtime/compose.go:202`). Indented JSON array of `*Profile` via `filestore.Save` → `atomicfile.WriteFile` (temp + fsync + rename). |
| Load | BOM-tolerant. Missing file = first boot. `Open` saves when the System-defaults migration changed anything. |
| Concurrency | One `sync.Mutex`. No goroutines. Getters return copies. Every mutator rolls memory back if the save fails. |
| Events | Emitted by runtime accessors, not this package: `roster.created`, `roster.updated` (action paused|resumed|retired|revived|edited), `roster.removed`. Subject `roster.<slug>`, actor `roster`. |
| Env | None read. `ConfigOverrides` keys are only syntax-checked. |

**Files:**

| File | Contents |
|---|---|
| `doc.go` | Package doc (M783). Atomic single-file storage mirroring `standing`. |
| `roster.go` | `ErrNotFound`/`ErrRetired`, `safeCall`, the `Profile` struct and policy/lifecycle/task types, `Kind`/`AllowsDirectCall`/`AllowsDelegationFrom`, regexes, limits, guardian default constants. |
| `roster_store.go` | `Store`: `Open` (with System migration), CRUD and lifecycle setters with rollback and identity protection, `save`. |
| `roster_validate.go` | `Validate`: slug, sizes, tool-name regex, allow/deny conflict, trust via edict, config keys, workdir escape, hierarchy refs, per-policy validators. |
| `roster_normalize.go` | `normalizeProfile`: trim, compact, upper-case trust, infer lifecycle mode, fill task IDs, apply system defaults + noise deny. Plus `normalizeProfilePolicies`. |
| `roster_normalize_helpers.go` | `enforceNoiseToolDeny`, `applySystemGuardianDefaults`, `noiseSeverityRank`, `compactStrings`, `compactUniqueStrings`. |
| `roster_normalize_validate.go` | Validators for lifecycle, task list, retry, health, self-repair and noise policies. |
| `autonomy.go` | `AutonomyRunbook(p)`, the single source of the wake-contract payload. |

Tests: `roster_test.go` and `r6_proof_test.go`. They cover: System can't be self-promoted or demoted, guardian defaults,
`Open` migration, noise-policy memory deny, mutate-panic recovery.

---

## 2. The wake model — how a sleeping agent becomes a governed run

Agents are records, not processes. Nothing runs between wakes. Every wake source builds a `context.Context` decorated
with identity, budget, trust and provenance, then calls the same kernel entry points:
`Kernel.RunWith` / `RunWithRetry` / `RunAssured` / `RunWorkflow` / `RunTool`.

```
 TIME                     EVENTS / CRON RULES              OBSERVATION                    MESSAGES
 cadence.Engine           standing.StartRunner (bus ">")   pulse.Engine (ticker)          channel.Start (per instance)
 (10s tick, Store.Due)    standing.StartCron (30s tick)    observers→salience→initiative  allowlist → InboundHandler
        │                         │                                │                              │
        │                         │                pulse.initiative.act / .ask                    │
        │                         │◄───────────────── (bus event, subject glob) ─────────────────── │
        ▼                         ▼                                                               ▼
 cmd/agezt/main_cadence.go  cmd/agezt/main_standing.go fire()                         cmd/agezt/main_channels_handler.go
 run closure:               - resolve roster profile                                  makeChannelHandler:
 - resolve roster profile     (unknown/paused/retired/managed → standing.error)       - ConversationHistory(journal)
   (retired/paused/managed  - ScopedIntent + TriggeredIntent (UNTRUSTED envelope)     - vision sidecar / STT transcript
    → error)                - publish standing.fired (+autonomy_runbook)              - k.RunWith(ctx, corr, intent)
 - scheduledRunContext:     - WithAgentProfile, WithMaxCost                           - optional TTS reply
   WithAgentProfile,        - WithWakeContext{Source:"standing"}
   WithMaxCost, WithModel   - WithTrustCeiling(min(max_trust, mode ceiling))
 - WithWakeContext          - RunAssured | RunWithRetry | RunWith
   {Source:"schedule"}      - BriefText → channel brief
 - publish schedule.fired
 - target switch: intent→RunAssured|RunWithRetry|RunWith,
   workflow→RunWorkflow, tool→RunTool, system_task→systemtasks.Run
        │                         │                                                               │
        └─────────────┬───────────┴───────────────────────────────────────────────────────────────┘
                      ▼
      kernel/runtime  RunWith → runexec.RunWith  (see 04-agent-runtime.md)
        SetupRunState: halted? duplicate corr? register k.runs[corr] (Halt/Cancel), deadline
        intent frame → intent.interpreted; BuildRunPrompt (soul, memory, world, skills, operator profile)
        ResolveRunModel / model chain → governor (routing, budgets, per-run & per-agent daily caps)
        agent.Run loop: every tool call → k.policyHook:
            trust ceiling from ctx → edict.DecideWithCeiling (never loosened by delegation)
            agent ToolAllow/ToolDeny, noise policy, injection guard, intent/regret gates
            Ask → approvals.Submit (blocks for operator)
        every step journaled (BLAKE3 hash chain) under corr; post-run: skill outcome, forge propose,
        memory distill (non-system agents), lifecycle cycle completion
```

Provenance travels in `runtime.WakeContext{Source, Reason, ScheduleID, StandingID, StandingName, TriggerSubject,
ParentCorrelation}`. It is set via `runtime.WithWakeContext`, copied into the loop config (`WakeSource`, `ScheduleID`, …),
and surfaced in `workflow.started` payloads by `workflowRunProvenance`.

Call sites:

| Call site | `Source` |
|---|---|
| `main_cadence.go:77` | `schedule` |
| `main_standing.go:121` | `standing` |
| `main_standing_resume.go:105` | resumed standing |
| `controlplane/workflow_handlers_invoke.go:125` | `webhook` |
| `controlplane/workflow_handlers_run.go:169` | `manual` |
| `controlplane/workboard_dispatch.go:22` | workboard dispatch |
| `controlplane/roster_escalation.go:174` | roster escalation |

Governance applied per wake source:

| Wake source | Identity | Budget | Trust ceiling | Approval / policy |
|---|---|---|---|---|
| cadence `intent` | `ent.Agent` → `WithAgentProfile` | `prof.MaxCostMc` | profile `TrustCeiling` (via profile) | full `policyHook` |
| cadence `workflow` | as above | as above | as above | each tool/http/code node passes `policyHook`; approval node blocks |
| cadence `tool` | as above | — | — | `RunTool` → `toolexec` → `CheckPolicy` |
| cadence `system_task` | none (daemon) | — | — | **none**. Closed enum, operator-created only. |
| standing order | `o.Agent` | `o.Initiative.BudgetPerRunMc`, else `prof.MaxCostMc` | `standingTrustCeiling` = min(`max_trust`, mode). `inform_only`→L0, `ask`→L1, `act_or_ask` with no max → `LevelAskFirst` | full |
| channel inbound | default identity | — | — | full. Inbound text is data; allowlist gates senders. |
| self-repair wake | parent/owner profile | `prof.MaxCostMc` | profile | full |

---

## 3. `kernel/cadence` — typed schedules

**Purpose.** The persistent timer subsystem. An `Engine` ticks every `DefaultResolution` (10 s) and asks the `Store` which
entries are due. It fires each one through a caller-supplied `RunFunc` with no overlap. The actual dispatch to targets is
the closure in `cmd/agezt/main_cadence.go`.

### 3.1 Key API

- **Constants** (`cadence.go`):
  - `MinInterval=1s`, `DefaultResolution=10s`
  - `SourceOperator="operator"`, `SourceEnv="env"`. builtinguardians uses `"system"` without a constant.
  - Targets: `TargetIntent=""` (legacy zero value = agent run), `TargetWorkflow="workflow"`,
    `TargetSystemTask="system_task"`, `TargetTool="tool"`.
- **Modes** (`cadence_entry.go:44-55`):
  - `ModeInterval=""`
  - `ModeDaily="daily"`
  - `ModeOnce="once"`: fires once, then removed.
  - `ModeWindow="window"`: interval only inside a daily window.
  - `ModeContinuous="continuous"`: re-anchors `cooldown` after each completion (M646).
- **`Entry`** fields: `ID` (`sched-<ulid>`), `Intent`, `Mode`, `IntervalSec`, `AtMinutes`, `EndMinutes`,
  `Days` (bitmask, Sun=bit0, `AllDays=0x7F`), `TZ` (IANA), `Model`, `Agent`, `Target`, `Workflow`, `SystemTask`, `Tool`,
  `Payload` (raw JSON), `Source`, `Enabled`, `CreatedUnix`, `LastRunUnix`, `NextRunUnix`, `Fires` (completed-firing
  heartbeat), `Assure` (do-it-for-sure attempt budget, M654).
- **`Entry` methods:** `Validate()` (exactly one consistent target), `Interval()`, `Cadence()` (display),
  `Forecast(from, n)` (backs `agt schedule test`).
- **`Store`:**
  - Adders: `OpenStore`, `Add`, `AddDaily`, `AddOnce`, `AddContinuous`, `AddWindow`.
  - Setters: `SetEnabled`, `SetIntent`, `SetModel`, `SetAgent`, `SetIntentTarget`, `SetWorkflowTarget`,
    `SetSystemTaskTarget`, `SetToolTarget`, `Reschedule`, `SetAssure`.
  - Runtime: `Remove`, `List`, `Get`, `RunNow`, `Due(now)`, `CompleteFiring(id, now)`, `SyncEnv`, `Count`.
- **`Engine`:**
  - `NewEngine(store, run RunFunc, resolution, log)`, `Start(ctx)`, `Wait()`, `RunningCount()`, `WaitIdle(ctx)`.
  - Exported fields `RunTimeout` and `Bus`.
  - `RunFunc func(ctx, id, intent, model string) error`.
- **Legacy env parser:** `ParseJobs("30m=task;1h=other")`. `Describe(entries)` builds the boot banner.
- **System-task catalogue:** `SystemTaskInfo{Name, Label, Description, Category, Executor, UsesLLM, EffectClass, Effect,
  RecommendedIntervalSec}`. Accessors `SystemTasks()`, `SystemTaskInfos()`, `IsSystemTask()`. Seven tasks:
  `catalog_sync`, `artifact_collect`, `memory_clean`, `memory_tidy`, `log_clean`, `graveyard_scan`, `profile_distill`.
  Only `profile_distill` uses the LLM.
- **Injection tripwire:** `SuspiciousIntent(intent) []string` (`injection.go`). Marker labels: `override_instructions`
  (incl. Turkish variants), `persona_hijack`, `prompt_exfil`, `secret_exfil`, `shell_smuggle`, `base64_blob` (≥120-char run).

### 3.2 Invariants

- **Recurring entries advance eagerly (at-most-once).** `Due` moves `NextRunUnix` forward and persists *before* launching,
  so a crash mid-run skips that slot (`cadence_store_run.go:65-100`).
- **Once and continuous entries are not advanced in `Due`.**
  - A one-shot is removed in `CompleteFiring` after the run, success or error. That makes it at-least-once with no retry
    storm (M199).
  - Continuous re-anchors to completion + cooldown and increments `Fires` (M646/M650).
- **No overlap.** `running.LoadOrStore(id)` skips an in-flight entry. `CompleteFiring` runs *before* the deferred
  `running.Delete`, so no tick can re-fire in that gap (`cadence_engine.go:137-204`).
- **Panic containment.** `fireOne` recovers. `RunTimeout` (daemon default 1h) backstops a hung run that would otherwise pin
  its guard forever.
- **Interval floor `MinInterval`** is enforced at every entry point:
  - `OpenStore` repairs persisted values (M196);
  - `safeInterval` floors defensively;
  - `SyncEnv` clamps;
  - `Add` / `AddWindow` / `Reschedule` reject;
  - `AddContinuous` clamps.
- **DST-correct.** `nextDaily` and `nextWindowSlot` walk calendar dates. A fall-back fold guard prevents a double fire
  (`cadence_helpers.go:83-92`, M197).
- **`SetSystemTaskTarget` re-validates the enum**, so no arbitrary task name can be smuggled in.
- **The injection tripwire only scans `TargetIntent`** and is default-allow. It publishes `system.anomaly` and still fires.

### 3.3 Persistence, concurrency, env, events

| Aspect | Details |
|---|---|
| Persistence | `<home>/cadence/schedules.json` (`runtime/compose.go:154`). JSON array of `Entry`, atomic save, whole-file rewrite on every mutation. |
| Concurrency | `Store.mu` guards everything; copies are returned. `Engine.Start` runs one ticker goroutine plus one goroutine per firing. No Stop method: cancel ctx. `Wait()` does not wait for in-flight firings. |
| Env (read in cmd/agezt) | `AGEZT_SCHEDULE` (legacy spec, synced as `source=env`), `AGEZT_SCHEDULE_RUN_TIMEOUT` (default 1h, `off`/`0` disables), `AGEZT_SCHEDULE_NOTIFY` (`on` delivers successful answers to channels). |
| Events from package | `system.anomaly` (`KindAnomalyDetected`) on subject `cadence.injection` with `anomaly:"schedule_intent_injection_suspect"`. |
| Events from the daemon fire path | `schedule.fired` (`KindScheduleFired`, payload from `scheduleFiredEventPayload`, includes `autonomy_runbook` and system-task info), `schedule.task` / `task.*` around typed targets (`runScheduledTrackedTarget`). |

**Wiring.**
- `kernel/runtime/compose.go:154` opens the store. Accessors are `k.Schedules()` and `k.SetScheduleEngine` / `ScheduleEngine`.
- `cmd/agezt/main.go:1633` calls `buildCadence` (`main_cadence.go:25`).
- Control plane: `schedule_*` commands (`schedule_add.go`, `schedule_misc.go`, `schedule_edit.go`, …).
- Agent tool `schedule` (`plugins/tools/schedule`). A schedule an agent creates is bound to the acting agent, except system
  tasks.

**Files:**

| File | Contents |
|---|---|
| `cadence.go` | Package doc, constants, `SystemTaskInfo` catalogue, `Job`, `Store` struct. |
| `cadence_engine.go` | `RunFunc`, `Engine` (ticker, in-flight `sync.Map`, `RunTimeout`, tripwire publish, recover, `CompleteFiring`), `RunningCount`/`WaitIdle`, `ParseJobs`, `Describe`. |
| `cadence_entry.go` | Task accessors, mode constants, `Entry`, `Validate`, interval clamp, `Cadence()`. |
| `cadence_entry_days.go` | `FormatDays` / `ParseDays`: aliases `daily|weekdays|weekends`, wrapping ranges like `fri-mon`. |
| `cadence_forecast.go` | Pure `Forecast` and zone-aware `advance`. |
| `cadence_helpers.go` | `applyZone`, `nextWindowSlot`, `dayAllowed`, `nextDaily` (DST guard). |
| `cadence_store.go` | `OpenStore` (load + sub-minimum repair), `Add*`, `SetEnabled`. |
| `cadence_store_run.go` | `Remove`/`List`/`Get`/`RunNow`, `Due`, `CompleteFiring`, `SetAssure`. |
| `cadence_store_set.go` | `validateWindow`, in-place setters, target switchers that clear conflicting fields, `Reschedule`. |
| `cadence_store_sync.go` | `SyncEnv` (replaces every `source=env` entry), `Count`, `save`. |
| `injection.go` | `SuspiciousIntent` heuristic. |

Tests (clock-controlled): `cadence_test`, `continuous_test`, `dst_test`, `injection_test`, `interval_floor_test`,
`once_crashsafe_test`, `target_validation_test`.

## 4. `kernel/cadence/systemtasks` — daemon maintenance executors

**Purpose.** Executors for the seven typed system tasks. They were extracted from `cmd/agezt` in Phase 2.6. Each runs
directly against `*runtime.Kernel`, never wakes an agent, and publishes a compact summary event.

**Import rule** (`doc.go:9-13`): nothing in `kernel/cadence` or `kernel/runtime` may import `systemtasks`.
The daemon wires them together.

**API:**
- `Info(name) (cadence.SystemTaskInfo, bool)`: decorates `schedule.fired`.
- `Run(ctx, k, corr, scheduleID, task) error`: switch dispatch; an unknown name is an error.

| Task | Executor | Effect / event (`schedule.system_task.<name>`, kind `info`, actor `schedule`) |
|---|---|---|
| `catalog_sync` | `runScheduledCatalogSync` | `catalog.NewSyncer().Sync` → `k.CatalogStore().SaveAPI` → `k.Reload()`. Emits `catalog.synced` / `catalog.sync_failed`. URL from `AGEZT_CATALOG_URL`. |
| `artifact_collect` | `runScheduledArtifactCollect` | `k.ArtifactIndex().Collect(cutoff)`. 30-day cutoff, hard-coded. |
| `memory_clean` | `runScheduledMemoryClean` | `k.Memory().CleanLowValue(corr, false)`. |
| `memory_tidy` | `runScheduledMemoryTidy` | `k.Memory().DedupeDistilled(corr, false)`. |
| `log_clean` | `runScheduledLogClean` | Journal `Range` stats + `Head()`. Payload says `physical_deletion:false`: the hash chain is never pruned. |
| `graveyard_scan` | `runScheduledGraveyardScan` | Notify-only list of retired profiles older than `AGEZT_GRAVEYARD_RETENTION_DAYS` (0 = keep forever). Payload says `action:"report_only"`. |
| `profile_distill` | `runScheduledProfileDistill` | `k.DistillProfile(ctx, corr)`: one LLM pass that writes operator-profile facets. |

Files: `doc.go` (import rule), `systemtasks.go` (`Info`, `Run`), `systemtasks_helpers.go` (the seven executors,
`graveyardRetentionDays`, `envOrDefaultLocal`). Tests: one per executor in `systemtasks_test.go`.

## 5. `kernel/standing` — standing orders

**Purpose.** Durable, named, pausable wake rules (SPEC-16 §4). An order binds event triggers (bus subject globs) and/or
5-field cron triggers to a plan/intent. It optionally runs as a roster agent, under an initiative mode, trust ceiling,
budget, cooldown, assure budget and briefing channel. The package owns the record, the store, the event runner and its own
cron ticker. The daemon supplies the `FireFunc`.

### 5.1 Types

- `TriggerType`: `TriggerCron="cron"`, `TriggerEvent="event"`. `Trigger{Type, Schedule, Subject}`.
- `InitiativeMode`: `inform_only`, `ask`, `act_or_ask`. `Initiative{Mode, MaxTrust "L0..L4", BudgetPerRunMc}`.
- `(InitiativeMode).MaxAutonomyTrust()` (`standing_triggers.go:32`): `inform_only`→`"L0"`, `ask`→`"L1"`, else none (M999).
- **`Order`:**

  | Field (JSON) | Meaning |
  |---|---|
  | `ID`, `Name`, `Enabled` | |
  | `Triggers` | |
  | `Observers`, `ScopeEntities` | |
  | `Initiative` | |
  | `BriefingMin` (`briefing_disposition_min`) | |
  | `BriefingChan` (`briefing_channel`) | |
  | `Plan` | Intent template. Falls back to `Name`. |
  | `Agent` | Run AS this profile (M790). |
  | `Assure` | M655. |
  | `CooldownSec` | |
  | `CreatedMS`, `UpdatedMS` | |
- `Validate(o)`: name required, at least one trigger, cron needs `schedule`, event needs `subject`, known mode,
  non-negative cooldown.
- `Store`: `Open`, `Add`, `SetEnabled`, `Update(id, mutate)`, `Remove`, `Get`, `List`, `Count`.
  `Update` protects `ID`, `CreatedMS` and `Enabled`, re-validates, rolls back, and recovers a panicking mutator.
- Runners:
  - `StartRunner(ctx, bus, store, RunnerConfig{Cooldown, Now}, fire) bool`: `DefaultRunnerCooldown = 15m`.
  - `StartCron(ctx, store, now, fire) bool`: 30 s tick.
  - `FireFunc func(ctx, o Order, triggerSubject string, triggerPayload map[string]any)`.
- Helpers:
  - `ScopedIntent(o, intent)`: grounds the run in what the order watches.
  - `TriggeredIntent(intent, subject, payload)`: wraps the payload in an **UNTRUSTED OBSERVATION**
    `external_data_not_instructions` envelope (VULN-004). `cron:*`, `manual` and `""` pass through unchanged.
  - `BriefText(o, answer)`.

### 5.2 Invariants

- The event runner subscribes to `">"` and skips `standing.*` subjects (self-retrigger guard, `runner.go:80`).
- The cooldown uses the local clock, never the event's `TSUnixMS` (defends against skewed webhook or mesh timestamps).
  `CooldownSec` overrides the 15 m default. It applies to event triggers only.
- Cron fires at most once per order per matching minute (`lastFired[id]==minuteStamp`). Only the first matching trigger
  fires per tick.
  - A malformed cron spec never matches; it does not error.
  - DOM/DOW combine with OR when both are restricted. DOW 0 and 7 both mean Sunday.
  - Evaluated in daemon local time; there is no TZ field.
- `tickCron` returns immediately once ctx is cancelled.
- `safeFire` recovers every `FireFunc`. The daemon's `fire` also recovers and journals `standing.error`.
- `pruneToLive` bounds the dedup and cooldown maps to the live order set.
- **No overlap guard** (unlike cadence). A long cron order can overlap its next minute, and an event order can overlap once
  its cooldown expires.

### 5.3 Persistence, concurrency, events, wiring

| Aspect | Details |
|---|---|
| Persistence | `<home>/standing/standing.json` (`runtime/compose.go:186`). JSON array of `Order`, atomic save. Stored as JSON, not the YAML the SPEC mentions (stdlib-first). |
| Concurrency | `Store.mu`. One bus-subscriber goroutine (`Subscribe(">", 256)`) and one cron goroutine. Each fire runs on `go`. No Stop: cancel ctx. |
| Env | None. The daemon passes `RunnerConfig{}`. |
| Events (emitted by runtime accessors and the daemon) | `standing.created`, `standing.updated` (paused|resumed|edited), `standing.removed`, `standing.fired` (with `autonomy_runbook`), `standing.error` (unknown/paused/retired/managed agent, or panic). Subject `standing.<id>`, actor `standing`. |
| Consumed | Every bus event except `standing.*`. |

**Wiring:**
- `runtime/accessors_standing.go`: `AddStanding`, `SetStandingEnabled`, `UpdateStanding`, `RemoveStanding`, all journaled.
- `cmd/agezt/main.go:1673`: `buildStandingRunner` (`main_standing.go:20`). Its `fireNow` (a "manual" fire) is bound into
  `controlplane.LateDeps.StandingFire`.
- `cmd/agezt/main_overlay_anomaly.go:97`: `standingTrustCeiling`.
- Control-plane `standing_*` commands.
- Agent tool `standing` (`plugins/tools/standingtool`): default mode `ask`; auto-binds the acting agent.

**Files:**

| File | Contents |
|---|---|
| `doc.go` | Package doc. |
| `standing.go` | Trigger, initiative and `Order` types, `ErrNotFound`, `Validate`, `Store` struct. |
| `standing_ops.go` | `Open` + CRUD with rollback and `safeCall`. |
| `standing_triggers.go` | `validMode`, `MaxAutonomyTrust`. |
| `cron.go` | Stdlib 5-field matcher, 30 s `StartCron`, per-minute dedup `tickCron`. |
| `runner.go` | `StartRunner`, `FireFunc`, `ScopedIntent`, `TriggeredIntent`, `BriefText`, `pruneToLive`, `safeFire`, `matchesAnyEventTrigger` (`bus.MatchSubject`, NATS-style `*` / `>`). |

Tests: `coverage_supp_test`, `cron_test`, `r5_proof_test` (mutator-panic recovery), `runner_test`, `standing_test`.

## 6. `kernel/pulse` — the proactive heartbeat

**Purpose.** SPEC-03 "proactive heart". Each beat runs **tick → observers → salience → routing → initiative → briefing**,
and every stage journals its own event so `agt why` can rebuild the chain. The engine owns no permissions: it borrows the
bus, Warden (for probes), the state store and a provider (for LLM salience).

With initiative (M999) it **classifies and emits only**. A standing order bound to `pulse.initiative.act` is what actually
runs, through the governed path.

### 6.1 Types and API

- `Delta{Source, Kind, Summary, Before, After, RawRef, Hints}`:
  - `Severity()` reads `Hints["severity"]` and defaults to medium.
  - `IssueKey()` returns `Hints["issue_key"]`, else `Source/Kind`.
- `Observer{Name(); Poll(ctx) ([]Delta, error)}`.
- Enums:
  - `Disposition`: drop | digest | notify | alert | act.
  - `Dial`: quiet | balanced (default) | chatty.
  - `Delivery`: now | digest | drop.
  - `InitiativeLevel`: `off` | `ask` | `act`. `ParseInitiative` defaults unknown values to **act**.
- `Engine`:
  - `New(Config{Bus, State, Warden, Provider, Model, Relevance, Observers, Dial, Initiative, Cadence, QuietHours, UseLLM,
    NoveltyTTL, Sink, DigestEvery, Now})`. Defaults: cadence 60 s, novelty TTL 30 m, digest every 30 beats.
  - Control: `Start`, `Beat`, `SetCadence` (clamped 5 s–24 h), `SetDial`, `SetInitiative`, `SetQuietHours`, `Pause`,
    `Resume`, `IsPaused`, `Status`, `StatusMap`.
  - Observers: `AddObserver`, `RemoveObserver` (runtime-added only).
  - Digest and asks: `FlushDigest`, `PendingAsks`, `ResolveAsk(issueKey, approve)`.
- Observers:

  | Constructor | `Name()` | Deltas |
  |---|---|---|
  | `NewProbeObserver(name, argv, warden, state)` | `probe:<name>` | `probe_failed` (high) / `probe_recovered`. Runs via `warden.Run`, `ProfileNone`. Baseline persisted. |
  | `NewDiskObserver(path, minPct, DiskFreeFunc)` | `system:disk` | `disk_low` (high; critical below `minPct/2`) / `disk_recovered`. |
  | `NewHealthObserver(HealthStatFunc, degradeAt=0.30, minSample=5)` | `self:health` | `health_degraded` / `health_recovered`. Tool-error and run-failure axes. |
  | `NewReaperObserver(ReaperScanFunc)` | `system:reaper` | `reaper_candidates` when any of 10 counts (dead, degraded, misconfigured, retry, routing, forced-*, unstable, staleArtifacts) grows. |

  `DiskUsage(path)` is platform-specific (unix Statfs, Windows `GetDiskFreeSpaceExW`, other: error).
- `Salience.Score`: novelty drop → severity baseline → optional `agent.GenerateObject` LLM refine (TaskType `salience`) →
  relevance boost +0.15 via `Relevance.IsActiveSubject` (the daemon passes `k.World()`).
- `Route(dial, disp, quietActive)`: pure.
- `QuietHours` / `ParseQuietHours("22-7")`: only alert and act break through quiet hours.
- `Brief`, `BriefSink`, `LogSink`, `SinkFunc`, `MultiSink`: channels supply sinks via `channelwire.Built.Sink`.

### 6.2 Initiative semantics (`engine_process.go:64-108`)

"Actionable" means `Disposition ∈ {alert, act}` or `Hints["actionable"]=="true"`. Salience never actually produces
`DispAct`, and no shipped observer sets the `actionable` hint.

| Level | Behaviour |
|---|---|
| `off` | Branch is always `inform`. No act/ask event. |
| `ask` | Publishes Kind `initiative.act` on subject **`pulse.initiative.ask`** and queues the delta in an in-memory ask queue (≤50). `ResolveAsk(key, true)` re-publishes it on `pulse.initiative.act`. |
| `act` (default) | Publishes Kind `initiative.act` on subject **`pulse.initiative.act`**. |

Every delta also gets `initiative.taken` on `pulse.initiative`, with `branch` = inform|act|ask.

### 6.3 Invariants, persistence, concurrency

- An observer never decides importance. The engine never acts.
- Panic firewall (M423): `safePoll` recovers panics in observers, the salience LLM and the sink, and journals them as
  `observer.delta` with an error.
- Manual beats and ticks share one goroutine, so they never race. A manual `Beat()` fires even while paused.
- Probe and disk observers stay silent on the first poll (baseline).
- **Persistence** in `state.FileStore` under `<home>/state/`:
  - `pulse_probe.json`: probe exit baseline.
  - `pulse_seen.json`: novelty cache.

  Ask queue, digest buffer, pause flag and live overrides are memory-only. The control plane persists
  cadence/dial/quiet-hours to settings. **Initiative is never persisted**, and `controlplane.PulseController` has no
  `SetInitiative`, so changing it requires the env var plus a restart.
- **Concurrency:** one goroutine (ticker + coalescing `beat` / `retune` channels of size 1). `e.mu` guards the state.
  Observers are polled serially, so a slow probe delays the whole beat. No Stop method: cancel ctx.

**Env** (read in `cmd/agezt/main_pulse_engine.go`): `AGEZT_PULSE` (`off`), `AGEZT_PULSE_CADENCE`, `AGEZT_PULSE_DIAL`,
`AGEZT_PULSE_QUIET_HOURS`, `AGEZT_PULSE_PROBE` (`name=..;argv=..`), `AGEZT_PULSE_DISK` (`path:pct`), `AGEZT_PULSE_HEALTH`
(`off` | threshold), `AGEZT_PULSE_LLM` (`on`), `AGEZT_PULSE_INITIATIVE` (`off|ask|act`, default act).

**Events** (actor `pulse`; corr `pulse-<ulid>` per delta; causation = tick id):

| Kind | Subject |
|---|---|
| `pulse.tick` | `pulse.tick` |
| `observer.delta` | `pulse.observer.<source>` |
| `salience.scored` | `pulse.salience` |
| `initiative.taken` | `pulse.initiative` |
| `initiative.act` | `pulse.initiative.act` or `.ask` |
| `briefing.sent` | `pulse.briefing` |
| `pulse.paused` / `pulse.resumed` | `pulse.control` |

**Wiring:**
- `cmd/agezt/main.go:1202` calls `buildPulse`, then `eng.Start(ctx)`, then `srv.Bind(LateDeps{Pulse, Observers})`.
- The reaper observer is added via `AddObserver` at `main.go:1216`, so it is technically removable.
- `kernel/runtime` does not import pulse. The control plane talks to it through interfaces.

**Files:**

| File | Contents |
|---|---|
| `doc.go` | SPEC-03 spine. The "deferred: autonomous act" note is stale. |
| `pulse.go` | Core vocabulary and `ParseDial` / `ParseInitiative`. |
| `engine.go` | `Config`, `Engine`, `New`, `Start` loop, `Beat`, setters, `queueAsk`. |
| `engine_observe.go` | `PendingAsks`, `ResolveAsk`, `tickOnce`, `Add/RemoveObserver`, `safePoll`, `FlushDigest`. |
| `engine_process.go` | `process` pipeline, `flushDigest`, `publish`, `briefPayload`, `Status` type. |
| `engine_status.go` | `Status`, `StatusMap`, `Pause` / `Resume`. |
| `observers.go` | Probe and disk observers, `ParseProbeSpec`. |
| `health.go` | `HealthStat`, `HealthObserver`. |
| `reaper.go` | `ReaperScanFunc`, `ReaperObserver`. |
| `salience.go` | `Relevance`, `Salience`, `Route`, seen cache. |
| `briefing.go` | Brief/sinks, digest composition, `QuietHours`. |
| `diskusage_unix.go` / `_windows.go` / `_other.go` | `DiskUsage`. |

Tests: `engine_test` (`TestInitiativeActEmission`, `TestPendingAsksLifecycle`, `TestTickEmitsFullChain`, …),
`observers_test`, `health_test`, `reaper_test`, `salience_test`, `route_test`, `sink_test`, `relevance_wiring_test`,
`coverage_more_test`, `diskusage_windows_test`.

### 6.4 Flow: observation → initiative → governed run

1. `tickOnce` publishes `pulse.tick`, then `safePoll` on each observer. Each delta goes through `process`.
2. `process`:
   1. `observer.delta`
   2. `Score`, then `salience.scored`
   3. drop / `Route`
   4. initiative branch: `initiative.taken`, then (act|ask) `initiative.act` on `pulse.initiative.<act|ask>`
   5. `MarkSeen`
   6. brief now (sink + `briefing.sent`) or digest
3. `standing.StartRunner` sees the subject and matches enabled orders (e.g. `guardian-initiative`, disabled by default).
   It checks the cooldown, then calls `fire` (§2): profile, budget, trust ceiling (`act_or_ask` + `MaxTrust L2`), and
   finally `RunWith`.
4. With defaults (act mode + disabled responder), Pulse emits the event and nothing runs until the operator enables a
   bound order.

## 7. `kernel/selfrepair` — deterministic auto-repair ("doctor")

**Purpose.** It subscribes to `pulse.observer.system:reaper`. On each `reaper_candidates` delta it:
1. re-runs `k.ReaperScan` (30-day window);
2. claims eligible broken agents;
3. drives a repair (overseer `RepairAgent` = a governed LLM run AS the broken agent proposing profile and routing fixes)
   or a routing rollback;
4. on failure, escalates to the parent or owner via a mailbox help request plus a wake;
5. parses that agent's fenced-JSON resolution and applies it.

Exported surface: `WireAutoRepair(ctx, k, baseDir, mailbox Mailbox, postNotify) string` and the `Mailbox` interface
(`HelpRequest`, `Get`, `Send`; satisfied by `*board.Store`).

### 7.1 Eligibility and modes

- `autoRepairEligible`: `!System && Enabled && !Retired && AllowsDirectCall() && SelfRepairPolicy.Enabled`. Self-repair is
  **opt-in per agent**.
- Claim priority: `misconfigured` > `routing_unstable` > `routing_forced_exhausted` > `routing_forced_failed` > `routing` >
  `retry_pressure` > `degraded`. One claim per slug per tick.
- The three `routing_unstable` / `forced_*` modes never auto-repair. They always escalate.
- Resolutions (whitelist): `handled`, `paused` (`SetProfileEnabled(false)`), `retired`, `delegated` (new help request +
  child-incident wake, recursion with ChainDepth+1), `blocked`, `force_chain` (`ApplyRoutingChain` + generation counter).

### 7.2 Invariants

- **Stable fingerprints (SR-001).** Fingerprints carry only stable identity. Live metrics made cooldown and caps inert
  before this fix.
- **Cooldown.** Same slug + fingerprint within the cooldown (default 30 m) is not re-claimed. In-flight slugs are skipped.
- **Attempt cap.** `MaxAttempts` counts earlier `queued` / `routing_rollback_queued` events **from the journal**, so it
  survives restarts. The cooldown does not survive restarts.
- **Panic firewalls (WF-001)** in `handleTick` and `dispatch`. The recover defer is registered before the release defer, so
  the slug lock is always returned. Panics publish `selfrepair.panic`.
- **Delegation targets.** A delegation can never target the broken agent or the current owner.
- **Escalation target** = `ParentAgent`, then `OwnerAgent`. **`SelfRepairPolicy.EscalateTo` is ignored** by the
  coordinator (it is carried in reaper rows but never read; see Gotchas).

### 7.3 Persistence, concurrency, env, events

| Aspect | Details |
|---|---|
| Persistence | No files of its own. History = journal (`doctor.auto_repair` events). Routing changes persist via overseertool → settings `AGEZT_TASK_MODEL_CHAINS`. Profile changes go to `roster.json`. |
| Concurrency | One coordinator goroutine (bus sub, buffer 64) plus one goroutine per dispatched candidate. `c.mu` guards inflight/last. `claim` holds `c.mu` while scanning the whole journal: O(journal) per tick. |
| Env | `AGEZT_AUTO_REPAIR` (`off`), `AGEZT_AUTO_REPAIR_COOLDOWN` (30 m), `AGEZT_ROUTING_ROLLBACK_PROBATION` (2 h). |
| Events | Subject `doctor.auto_repair`, kind `info`, actor `kernel`. `phase` ∈ queued, attempts_exhausted, routing_*_detected, routing_rollback_{queued,completed,failed}, completed, failed, escalation_{failed,skipped,woke,answered}, resolution_{failed,applied}, delegation_{queued,failed,woke}. Panics: `selfrepair.panic`. |

**Wiring:** `cmd/agezt/main.go:1680` only. Consumers of `doctor.auto_repair`: `controlplane/agent_activity_summary.go`,
`autonomy.go`, `autonomy_doctor.go`, `roster_activity.go`.

**Files:**

| File | Contents |
|---|---|
| `doc.go` | Import posture (imports runtime + overseertool; runtime never imports selfrepair). |
| `selfrepair.go` | Subject/time constants, source/rollbacker/applier interfaces, `Mailbox`, `WireAutoRepair`. |
| `selfrepair_coordinator.go` | Coordinator/candidate structs, env parsers, `run` loop, `handleTick` (scan → claim → queued → `go dispatch`). |
| `selfrepair_claim.go` | Priority-ordered `claim` (rollback-plan injection), `claimOne` gate (in-flight / cooldown / max-attempts). |
| `selfrepair_dispatch.go` | `dispatch` (escalate, `RollbackRouting`, or `RepairAgent`), `autoEscalate`, `release`, `publishAutoRepair`, phase names. |
| `selfrepair_wake.go` | `autoWakeManager`: wake the target, auto-reply in the mailbox, apply the resolution. |
| `selfrepair_wake_agent.go` | `autoRepairWakeAgent`: `RunWith`/`RunWithRetry` under `WithAgentProfile` + `WithMaxCost`. |
| `selfrepair_wake_helpers.go` | Skip reasons, escalation intent with the fenced-JSON contract, mailbox reply. |
| `selfrepair_resolution.go` | Parse, clean, apply resolutions; forced-routing generation logic. |
| `selfrepair_apply.go` | Delegated resolution, incident/root-chain id builders. |
| `selfrepair_routing.go` | Rollback plan, journal readers for the latest rewrite and force generation, live chain via `TaskModelChainsView`. |
| `selfrepair_pressure.go` | Per-mode stable fingerprints and reasons (SR-001 rationale). |
| `selfrepair_fingerprints.go` | Escalation target/from, generic fingerprint, misconfigured reason. |
| `selfrepair_filters.go` | `autoRepairShouldHandle`, `autoRepairEligible`, `autoRepairMaxAttempts`, `previousAutoRepairAttempts`. |
| `selfrepair_helpers.go` | Payload decoders. |

Tests: `selfrepair_test`, `coverage_test`, `wire_test` (eligibility/priority, cooldown dedupe, exhaustion, contained
panics, armed/disabled).

### 7.4 Flow

1. `reaper_candidates` arrives on `pulse.observer.system:reaper`. This also wakes `guardian-doctor` via its standing order.
2. `handleTick`: `ReaperScan` → `claim` → `queued` → `go dispatch`.
3. `dispatch`:
   - exhausted or unstable mode → `autoEscalate` + `autoWakeManager`;
   - otherwise `RollbackRouting` (if a probation plan exists) or `src.RepairAgent` (`overseertool/kernelsource_ops.go:190`);
   - publish `completed`, or `failed` → escalate.
4. `autoWakeManager` → `autoRepairWakeAgent` (governed run as the parent/owner) → `autoReplyEscalation` →
   `applyAutoRepairResolution`.

### 7.5 Extension

- **New repair mode:** add the row to `runtime.ReaperReport` and to the `ReaperScanFunc` arity (pulse + `main.go:1216`),
  then a fingerprint (`selfrepair_pressure.go`), a claim loop in priority order, phase names, and wake-intent text.
- **New resolution value:** update `cleanAutoRepairResolution` (whitelist), `applyAutoRepairResolution`, and the JSON
  contract string.

### 7.6 Layering violation

`selfrepair.go:17` and `selfrepair_dispatch.go:20` import `plugins/tools/overseertool`. This is intentional (`doc.go:9-11`)
so the operator repair button, the overseer `op=repair`, and auto-repair share one repair engine (`NewKernelSource`,
`RepairResult`, `ApplyRoutingChain`). There is no cycle, but it breaks the "kernel never imports plugins" law.
`autoRepairSource` is already an interface, so cmd/agezt could inject it, or the repair engine could move into a kernel
package.

## 8. `kernel/scheduler` — plan DAG executor (not a timer)

**Purpose.** The DAG layer above the single-agent tool loop (SPEC-02 §4). A `Plan` is a DAG of `Node`s. The `Executor`
validates it, schedules ready nodes as their dependencies complete, runs them under a bounded worker pool, and publishes
`plan.*` / `node.*` events. It has no persistence and reads no env.

### 8.1 API

- `DefaultMaxParallel=8`.
- `NodeKind`: `KindLoop="loop"`, `KindGate="gate"` (append-only).
- `Node{ID(); Kind(); DependsOn(); Run(ctx, Inputs) (Result, error)}`.
- `Plan{Name, Nodes, MaxParallel}`, `PlanResult{PlanID, NodeResults, Errors}`.
- `Executor`: `New(Config{Bus, Now, Monitor})`, `Run(ctx, plan, corr)`.
- `InvariantMonitor` with `ContextInvariantMonitor`, checked at `plan_start` and before each `node_start`.
- Errors: `ErrCycle`, `ErrDuplicateNodeID`, `ErrUnknownDependency`, `ErrEmptyPlan`, `ErrPlanInvalidated`.
- `LoopNode{NodeID, Intent, Deps, Runner LoopRunner, IntentFrame, IntentFn}` runs under corr `planCorr+".loop."+id`.
- `GateNode{…, Approvals, Capability (default "plan.gate"), Description, Actor}` submits `approval.Registry.Submit`
  with ToolName `scheduler.gate`. Only a grant passes.

### 8.2 Invariants

- Validation (duplicates, unknown dependencies, Kahn acyclicity, empty plan) happens before anything runs.
- Gate nodes do **not** hold a worker slot (`scheduler_run.go:231`). They block on a human and would otherwise starve the
  frontier.
- A failure is terminal for its branch: downstream nodes never start. The returned error is the lexicographically first
  failed node, so the result is deterministic.

### 8.3 Concurrency, events, wiring

- **Concurrency:** one goroutine per started node, a `sem` channel, a buffered `done` channel (event-driven, no busy wait),
  and `context.WithCancel` per plan.
- **Events** (actor `scheduler`, corr = planID):
  - `plan.started|completed|failed` on `plan.<id>.lifecycle`;
  - `node.started|completed|failed` on `plan.<id>.node.<nodeID>`.
- **Wiring:**
  - `runtime/compose.go:118` constructs it.
  - `k.LoopRunner()` returns `k.RunWith`.
  - `k.RunPlan` (Halt check, registers the plan in `k.runs`) is called from `controlplane/server_handlers_plan.go:42`.

**Files:**

| File | Contents |
|---|---|
| `doc.go` | Package doc. |
| `scheduler.go` | Core types, `Executor`/`New`, errors, `ContextInvariantMonitor`. |
| `nodes.go` | `LoopNode`, `GateNode`, correlation ctx helpers. |
| `scheduler_publish.go` | Event publishers. |
| `scheduler_run.go` | `Run` and `assertAcyclic`. |

Tests: `correlation_test`, `coverage_test`, `scheduler_test`.

---

# Part II — Extensibility: skills, market, plugins, MCP, ACP, toolbox

Capability can be added in six ways, from tightest to loosest coupling:

1. Compiled-in tools: `plugins/tools/*`, registered by `plugins/builtintools` toolreg specs. See [10-tools.md](10-tools.md).
2. Skills: procedures injected as prompt text.
3. Market packs: skills + MCP servers + tool requirements.
4. MCP servers: a stdio child process or a remote HTTP endpoint.
5. Out-of-process `kernel/plugin` children.
6. ACP external coding agents.

Every resulting tool is governed by Edict. An unmapped tool name becomes a one-off capability. An unknown capability is **default-deny** unless `AGEZT_ALLOW_ALL` is set. See [05](05-governance-routing-security.md).

## 9. `kernel/skill` — Forge v1 (learned procedures)

**Purpose.** Agents learn reusable, named procedures from their own runs. Skills are governed by a journaled, reversible state machine instead of being written straight to active markdown (SPEC-05 §4–5).

There are two layers:
- `Store`: a pure, file-backed record store. It has no bus.
- `Forge`: wraps a Store with the kernel bus. Every transition is persisted before it is published, under the run's correlation id.

### 9.1 Model

**Content addressing.**
- `ContentID(name, body)` is the hex BLAKE3-256 of `"skill"\0 lower(trim(name)) \0 body`.
- Editing the body creates a **new record (version)**. Its `Lineage` points at the parent records.
- Proposing an identical skill again dedupes onto the existing record.

**The `Skill` record.**
- Fields: `{ID, Name, Description, Triggers, Body, ToolsRequired, Resources, Agent, Version, Lineage, Status, Metrics, SourceEvent, CreatedMS, LastSeenMS}`.
- `Metrics{Uses, Successes, Failures, LastUsedMS, ShadowEvals, ShadowWins}`.

**Lifecycle (`skill.go`).**

| From | Legal targets |
|---|---|
| draft | shadow, archived |
| shadow | active, quarantined, archived |
| active | quarantined, archived |
| quarantined | active, archived |
| archived | — (terminal) |

- `PromoteTarget` walks draft→shadow→active, and quarantined→active.
- Only **active** skills are retrieved and injected into runs. Shadow skills are only *evaluated*.

**SKILL.md parsing (agentskills.io / ClawHub format).**
- `ParseSkillMD` is a stdlib-only frontmatter parser.
- Recognised keys: `name`, `description`, `version`, `triggers`, `tools_required` (alias `tools`).
- Lists can be written inline, as a scalar, or as a block.
- Name and body are required. Unknown keys are ignored.
- `version` is parsed but never stored.

**Bundles (`BundleStore`).**
- Location: `<home>/skills/bundles/<slug>/`.
- Methods: `Write`, `List`, `Read`, `Dir`, `Remove`.
- Limits: 1 MiB per file, 8 MiB per bundle.
- `Write` stages into `<slug>.tmp`, then swaps it into place.
- `cleanRel` rejects absolute paths and `..`. `Read` re-checks containment after resolving symlinks.
- Bundles are keyed by the **slugified name**, not the content id, so resources survive body edits.

**Activation directives.** Leading lines of the form `/skill a` or `/skills a, b; c` are parsed by `ParseActivationDirective`. They force explicit activation of those skills.

**Ownership (M932 / M942).**
- `Skill.Agent == ""` means the skill is shared.
- `visibleTo` allows a skill if it is shared or owned by the acting agent.
- `Propose` stamps the acting agent, so skills an agent learns are private to it by default.
- `Reassign` publishes `skill.shared` when ownership is cleared, or `skill.reassigned` when it moves to another agent.
- Removing an agent archives its private skills (`controlplane/roster_teardown_state.go`).

### 9.2 Forge API and automation

**Configuration.**
- `NewForge(store, bus)`.
- `SetAutoQuarantine(3, 0.5)` and `SetAutoPromote(3, 0.5)` set the default thresholds.
- `SetAutoShadow(false)` (off by default).
- `SetBundles` wires the bundle store.

**Methods.**
- `Create(corr, CreateSpec{…, Resources, Agent})`, `Get`, `List`, `Count` (counts active skills only), `Hygiene`.
- `ActivateFor(corr, agentSlug, intent, limit)` and `ActivateExplicitFor(…, refs, limit)`.
- `RecordOutcome`: quarantines an **active** skill when failures ≥ minimum **and** failure rate ≥ threshold.
- `ShadowEvaluate`: TaskType `shadow-eval`, no tools; an ambiguous verdict counts as false.
- `RecordShadowOutcome`: promotes a **shadow** skill when wins ≥ minimum **and** win rate ≥ threshold.
- `Propose`: TaskType `forge`; produces **draft** skills only; best-effort.
- Lifecycle: `Promote`, `Quarantine`, `Archive`, `RestoreStatus`, `Revert`. `Revert` appends a reversal and never force-activates a draft parent.

**Retrieval and gating.**
- `Retrieve` scores keyword overlap between the intent and the skill's name, description and triggers (not the body), multiplied by `1/(1+ageDays)`. Ties are broken deterministically.
- `ShadowTest` is the gate for draft→shadow: body ≥ 16 runes, plus a description or triggers.
- `Forge.mu` serialises every read-modify-write (M424), so two concurrent runs cannot bring a quarantined skill back.

### 9.3 Injection path

1. `runexec/runner.go:103` parses the activation directive.
2. `runtime/prompt_run.go:120-175` (`BuildRunPrompt`), only when `SkillInject` is on and the agent is not a system agent:
   - Calls `ActivateExplicitFor` (if a directive named skills) or `ActivateFor`, with topK 3.
   - `injectSkills` prepends "Applicable skills (learned procedures; follow if relevant):" followed by `## name — description`, the body, and hints about bundled resources.
   - Publishes a `contextselect` manifest with phase `skill`.
3. Sub-agents use the same path via `runtime/subagent_inject.go`.
4. After the run:
   - `RecordOutcome` (`runner.go:197`).
   - `MaybeForge` calls `Propose` when the run made at least 4 tool calls (`runner_lifecycle.go:153`).
   - `MaybeShadowEval` evaluates up to 2 shadow skills.
   - All three are skipped for system agents.

**Related entry points.**
- Agent tool `skill` (capability `skill`): ops `learn`, `list`, `show`, `promote`, `retire`, `files`, `read`.
- Built-in skills: 16 agentskills.io bundles, seeded by `builtinskills.SeedAll` (`main.go:1490`) and promoted to active.
- Export lives in the CLI (`cmd/agt/skill_export*.go`). It writes `<slug>-<id12>.skill.json` without status, metrics, agent or resources. Importing it creates a fresh draft.

### 9.4 Persistence, env, events

| Aspect | Details |
|---|---|
| Persistence | `<home>/skills/skills.json` holds `map[id]Skill`, rewritten atomically on every mutation. Bundles live under `skills/bundles/`. Opened in `runtime/compose.go:141-151`. |
| Env | `AGEZT_SKILLS` (default on; off disables injection), `AGEZT_FORGE` (default on), `AGEZT_SKILL_SHADOWEVAL` (default off), `AGEZT_SKILL_AUTOQUARANTINE` (default on), `AGEZT_SKILL_AUTOSHADOW` (default off), `AGEZT_SKILL_AUTOPROMOTE` (default on). Applied in `main.go:795-830`. |
| Events | `skill.created`, `skill.promoted`, `skill.quarantined`, `skill.reverted` (Archive also uses this kind, with `archived:true`), `skill.restored`, `skill.activated` (`activation` = auto or explicit), `skill.shadow_evaluated`, `skill.shared`, `skill.reassigned`. Subject `skill.<suffix>`, actor `forge`. |

**Files**

| File | Contents |
|---|---|
| `doc.go` | Design: Store vs Forge, content addressing. |
| `skill.go` | `Status` and the transition table. |
| `skill_store.go` | `Metrics`, `Skill`, `ContentID`, `Store`, `FileStore`. |
| `skillmd.go` | `ParseSkillMD`. |
| `bundle.go` | `BundleStore`, `OpenBundles`, size limits. |
| `bundle_helpers.go` | `slugify`, `cleanRel`, `resolveSymlinks`. |
| `bundle_io.go` | `Write` (staged swap), `List`, `Read`, `Dir`, `Remove`. |
| `activation.go` | `ParseActivationDirective`. |
| `forge.go` | `Forge` struct, setters, errors, `CreateSpec`, read operations, `publish`, `proposeSystem` prompt. |
| `forge_activate.go` | `visibleTo`, `Activate*`, `RecordOutcome`, auto-quarantine, shadow-judge prompt and parser. |
| `forge_lifecycle.go` | `Create`, auto-shadow, `writeBundle`, `lineageFor`. |
| `forge_shadow.go` | `ShadowEvaluate`, `RecordShadowOutcome`, auto-promote, `Propose`. |
| `forge_transitions.go` | `Promote`, `Reassign`, `Quarantine`, `Archive`, `RestoreStatus`, `Revert`. |
| `retrieve.go` | `Retrieve` / `RetrieveShadow` ranking. |
| `shadowtest.go` | `ShadowTest`. |

Tests: `activation`, `agentscope`, `autoquarantine` (+ rate boundary), `autoshadow`, `bundle`, `bundle_symlink_escape`, `coverage100`, `forge`, `forge_m424` (concurrency), `hygiene`, `reassign`, `retrieve_pool`, `shadoweval`, `skill`, `skillmd`, `transitions_matrix`.

## 10. `kernel/market` — capability marketplace

**Purpose.** A **pack** bundles skills, MCP servers and CLI-tool requirements. Packs are catalogued in **marketplaces**: the built-in offline `official` marketplace, plus remote marketplaces that are synced locally.

Installing a pack hands each part to the subsystem that already runs it:
- skills go to the Forge;
- MCP servers go to the MCP registry;
- tool requirements are only reported, as "install in Toolbox".

The manifest shapes mirror Claude Code's `plugin.json` / `marketplace.json` and agentskills.io.

### 10.1 Types

**Constants.**
- `FormatVersion=1`.
- `MarketplaceOfficial="official"` is reserved: a remote marketplace can never use that name.
- Names must match `^[a-z][a-z0-9-]{0,63}$`. Versions must be semver.

**`Pack`.**
- Fields: `{Name, Version, Description, Author, Category, Tags, Keywords, Skills []PackSkill{SkillMD, Resources}, MCPServers []mcp.Server, ToolRequirements, Signature *Signature{SHA256, Sig, PubKey, SignedAt}}`.
- `Validate` checks: name and semver, the pack is not empty, every SKILL.md parses, resource paths are safe, and every MCP server passes `mcp.Validate`.
- `CanonicalBytes` is the pack serialized with `Signature=nil`. `ContentHash` is its SHA-256.

**Catalogue and provenance.**
- `MarketplaceEntry` carries `Source` (a relative path to the pack), `SHA256`, `Signed`, `Featured`, `Downloads`, and counts.
- `Marketplace{Name, Owner, FormatVersion, GeneratedUnixMS, Source, Builtin, Packs}`.
- `InstalledPack{Name, Version, Marketplace, InstalledMS, SkillIDs, MCPServers, ToolReqs, Unsigned, VetVerdict}` is the provenance record that uninstall relies on.

**`Manager`.**
- Constructor: `NewManager(Config{Library, Store, Skills, MCP, Now, Verify, Syncer})`.
- Methods: `Sources`, `AddSource`, `RemoveSource`, `Sync`, `List`, `Show`, `Install`, `Uninstall`.
- Progress is reported as `Event{Stage}` with stage `vet`, `skill`, `mcp`, `tool` or `done`.

**`NewCompositeLibrary` (pack resolution).**
- `official` is consulted first.
- A named marketplace returns its cached version exactly.
- With no marketplace named, the newest version by `CompareVersions` wins.
- An explicit version request is exact: no substitution.

### 10.2 Ed25519 signing and trust

**Primitives.**
- `SignPack` signs `CanonicalBytes` and embeds the SHA-256 and the signer's public key in the pack.
- `VerifyPack(p, requirePubKeyHex)`:
  - An unsigned pack returns `(false, nil)`. If a key is required, an unsigned pack is an error.
  - A signed pack is checked for: public key length, a match against the pinned key (if one was given), signature length, a matching SHA, and finally `ed25519.Verify`.

**Where trust is anchored.**
- **At sync time,** a `Source.PubKey` pins the signer: every pack from that source must be signed by that key.
- **At install time,** the daemon passes `VerifyPack(p, "")` (`main.go:1512`). This only proves an embedded signature is **self-consistent**. It does not prove who signed it.
- Unsigned packs install, but are marked `Unsigned`.

**Tooling.** `agt market keygen` writes a mode-0600 seed and a `.pub` file. `agt market publish` runs `BuildPackFromDir` then `Publish`, writing `packs/<name>.json` and `marketplace.json`. Resources are capped at 4 MiB each.

### 10.3 Install flow

`Manager.Install` runs these steps in order:

1. `ResolvePack`, then `Validate`.
2. Refuse a **downgrade**.
3. Verify the signature. A signature that is present but invalid aborts the install.
4. `VetPack`, a heuristic scan that is **informational and never blocks**:
   - Danger: `injection-override`, `curl-pipe-shell`, `reverse-shell`, `secret-exfil`, `mcp-remote-exec`.
   - Warn: `cred-path-read`, `encoded-blob`, `env-dump`, `destructive-command`, `mcp-shell-host`.
   - Info: `mcp-nonstd-runner`, `mcp-secret-env`.
   - Also flags risky tool requirements.
5. For each skill: `Forge.Create`, then promote to active. This **bypasses the draft/shadow gate**, and the skill is **shared**.
6. For each MCP server: `Kernel.AddMCPServer`.
7. Record tool requirements.
8. `RecordInstall`.

If a step fails partway, earlier steps are **not** rolled back and no install record is written.

`Uninstall` quarantines the skills and removes the MCP servers listed in the install record. It does not touch host tools.

### 10.4 Remote sync

The `Syncer` uses a netguard-wrapped client: loopback and private addresses are allowed, link-local and cloud metadata addresses are blocked. Timeout 30 s, response cap 8 MiB.

1. Fetch the marketplace index. An index with an empty pack list is refused.
2. Force the local marketplace name and `Builtin=false`.
3. For each entry:
   - `resolveRef` must stay on the **same host** as the index;
   - fetch the pack;
   - the pack name must match the entry;
   - `Validate`;
   - the content hash must match the index's SHA pin;
   - `VerifyPack(p, src.PubKey)`.
4. Write to a staging directory, then rename it into place (keep-last-good).

### 10.5 Persistence, events, wiring

| Aspect | Details |
|---|---|
| Persistence (`<home>/market/`) | `installed.json`, `sources.json` (both atomic), `marketplaces/<name>/index.json`, `marketplaces/<name>/packs/<pack>.json`. |
| Concurrency | `Store` uses an RWMutex. `Manager` takes no lock, so two installs can interleave. |
| Env | None. The `market` tool is always registered. |
| Events | The package itself publishes nothing. `controlplane/market.go` streams `market.install.progress` / `market.uninstall.progress` frames (not journaled). It journals `market.pack.installed`, `market.pack.uninstalled`, `market.source.added`, `market.source.removed`, `market.synced` as ad-hoc kinds with no `kinds.go` constants. **Installs made by an agent through the tool emit no `market.pack.installed`.** |
| Wiring | `main.go:1505-1514` builds `NewManager` over `NewCompositeLibrary(builtinmarket.New(), store)`, with the Forge for skills, the Kernel for MCP, `VerifyPack(p,"")`, and `NewSyncer()`. The tool is registered in `runtime/compose.go:273-276` and bound lazily. |
| Agent tool `market` | Capability `market.install`. Ops: `search` (≤25 results), `show`, `install`. |

**Files**

| File | Contents |
|---|---|
| `doc.go` | Package doc. |
| `market.go` | Types, constants, `SkillSummary`, `safeRelPath`. |
| `market_pack.go` | `Pack` methods. |
| `market_helpers.go` | `matchesQuery`, `sortEntries`. |
| `manager.go` | Interfaces, `Event`, `Manager`, `Config`, sources, `Sync`. |
| `manager_ops.go` | `List`, `Show`, `Install`, `promoteToActive`, `Uninstall`. |
| `library.go` | Composite library. |
| `store.go` | Install records, `atomicWrite`. |
| `sources.go` | `Source`, marketplace cache. |
| `sync.go` | `Syncer`, same-host `resolveRef`, bounded `fetch`. |
| `verify.go` | `VerifyPack`, `SignPack`. |
| `publish.go` | `BuildPackFromDir`, `Publish`, key helpers. |
| `tool.go` | The `market` tool. |
| `version.go` | `CompareVersions`. |
| `vet.go` | `VetPack`. |

Tests: `installguard`, `librarydefensive`, `librarypick`, `libraryversion`, `listupdate`, `market`, `publish`, `sync`, `tool`, `version`, `vet`.

## 11. `kernel/plugin` — out-of-process plugin host

**Purpose.** Lets third parties ship tools without compiling against the agezt binary (DECISIONS B0, M1.y).

- The host spawns an executable and talks to it with **line-delimited JSON** on stdin/stdout.
- This is **not JSON-RPC 2.0**. `doc.go:12-31` explains why gRPC and MCP were rejected.
- Each tool the plugin advertises becomes an `agent.Tool` (`remoteTool`), so the rest of the kernel cannot tell in-process from out-of-process tools.
- "In-process plugins" are the compiled `plugins/tools/*`. The plugin spec is registered last with `YieldOnConflict`, so **an in-process tool wins any name collision**.

### 11.1 Protocol (`ProtocolVersion=1`)

| Direction | Frame |
|---|---|
| host→plugin | `Request{ID, Method, Params}`. Methods: `initialize`, `tool/invoke` (`InvokeParams{Name, Input}`), `shutdown` (id `end`). |
| plugin→host | `Response{ID, Result, Error, Progress}`. Any number of progress frames may come before the terminal frame (M1.ss). |
| plugin→host callback | `host/invoke` (M1.cb). Only tools listed in `Config.HostTools` can be called; if that is empty, the callback fails with `ErrCallbacksDisabled`. |
| initialize reply | `InitializeResult{ProtocolVersion (missing means 1; a major mismatch fails), Tools []ToolDef{Name, Description, InputSchema, Capability}}` |

**Capability manifest (M900).** `ToolDef.Capability` lets a plugin tool join an **existing** Edict axis such as `http.post`. The value flows as follows:

`ToolCapabilities(prefix)` → `toolreg.Built.Caps` → `runtime.Config.ToolCapabilities` → `validatedToolCaps`

`validatedToolCaps` keeps only `edict.KnownCapability` values, so a plugin cannot invent new axes. A tool with no declared capability is default-denied. Each `remoteTool` is marked `EffectCompensable` with confidence 0.45.

### 11.2 Security

**Opt-in controls.**
- **BLAKE3-256 pin** (`AGEZT_PLUGIN_PINS`): checked before `Start` and again on `Reload`. `resolvePluginPath` resolves the binary with `LookPath`, so the file that is hashed is the file that runs (M422).
- **Tool allowlist** (`AGEZT_PLUGIN_TOOLS`): `ErrToolAllowlistMismatch` if the plugin advertises anything else.

**Built-in limits.**

| Limit | Value |
|---|---|
| Frame size | 16 MiB (M177) |
| Advertised tools | ≤ 256 (M182) |
| Concurrent callbacks | ≤ 16, rejected without blocking (M181) |
| stderr line | 1 MiB |

**Process control.**
- Unix: the plugin runs in its own process group, and the whole group is killed (M184).
- Windows: only the direct child is killed.

**No isolation.**
- No warden. ✅ Env fixed (W0.3): the child gets the scrubbed base plus `AGEZT_PLUGIN_ENV` grants (it used to inherit the full daemon environment).
- stderr is pattern-redacted (`redact.New()`) and prefixed `[plugin:<prefix>]`.

### 11.3 Lifecycle

**`Spawn`.**
1. Apply defaults: init timeout 10 s, invoke timeout 2 m.
2. Resolve the path and verify the pin.
3. `Start` the process.
4. Start `startWaiter` (the only caller of `cmd.Wait()`), the stderr reader, and `readLoop`.
5. Send `initialize`.
6. Check the protocol version, the tool cap, and the allowlist.

**Calls and shutdown.**
- `Invoke` / `InvokeWithProgress` go through `callWithProgress`.
  - Request ids `q-N` are never reused (M180).
  - `writeMu` is separate from `mu` to avoid a deadlock under output floods (M460).
- If `readLoop` panics, it is recovered and the plugin is marked dead; its tools then fail fast.
- `Close`: send `shutdown`, wait 5 s, kill the process tree.
- `Reload`: swaps the child in place, so existing tool wrappers keep working (M560).

### 11.4 Boot wiring

`plugins/builtintools/plugins.go` (`buildPlugins`):
1. Parse `AGEZT_PLUGIN_PINS` (`prefix=<blake3hex>`), `AGEZT_PLUGIN_TOOLS` (`prefix=t1+t2`) and `AGEZT_PLUGINS` (`prefix=path args,...`, quote-aware). A malformed value is a **hard boot error**.
2. Spawn every plugin under a shared 30 s context. A plugin that fails is logged as a warning and skipped.
3. Register each plugin's tools as `<prefix>.<tool>`.
4. Record a `runtime.PluginInfo` for the plugin list in the control plane.

The package has no persistence and publishes no events of its own; calls show up as ordinary tool events.

**Files**

| File | Contents |
|---|---|
| `doc.go` | Rationale, lifecycle, crash handling. |
| `protocol.go` | Wire types, methods, version. |
| `host.go` | Defaults, `Config`, `Plugin`, `remoteTool` `Definition`/`Invoke`. |
| `host_spawn.go` | `Spawn`, error sentinels, protocol/tool-cap/allowlist checks, `Tools`, `ToolCapabilities`. |
| `host_loop.go` | `readFrame`, `readLoop`, `deliver`, callbacks, `markDead`. |
| `host_wire.go` | `Invoke*`, `startWaiter`, `Close`, `call*`, `write*`. |
| `host_lifecycle.go` | `IsAlive`, `Reload`, `respawn`, `remoteTool` type. |
| `pin.go` | `resolvePluginPath`, `makeChild`, `VerifyPin`, `HashFile`. |
| `pinspec.go` | `ParsePinSpec`, `ParseToolAllowlistSpec`. |
| `pluginspec.go` | `ParsePluginSpec`, quote-aware `splitFields`. |
| `proc_unix.go` / `proc_windows.go` | Process group handling and kill. |

Tests: 23 files covering allowlist, callbacks and their limit, capability, close, dead tools, death errors, delivery, flood, frames, host, pins, plugin spec, process handling, progress, protocol version, reaping, reload, and tool capabilities. `testdata/` holds `echoplugin` (the reference template), `floodplugin` and `idechoplugin`.

## 12. `kernel/mcp` — MCP client and registry

**Purpose.** Lets the runtime self-install MCP servers under governance (M796). The package provides:
- a durable registry of servers;
- a minimal client over **stdio** or **Streamable HTTP** (M904).

Enabled servers attach at boot. Their tools are merged into every run, either eagerly as `mcp_<server>_<tool>`, or behind one **lazy** dispatcher tool `mcp_<server>` (M906).

### 12.1 Types

**`Server`.**

| Field | Notes |
|---|---|
| `ID` | |
| `Name` | Immutable. Must match `^[a-z][a-z0-9]{0,15}$`: no `_` or `-`, because Edict parses tool names as `mcp_<name>_<tool>`. |
| `Command`, `Args` | stdio transport. |
| `URL` | HTTP transport. Exactly one of `Command` / `URL` must be set. |
| `Enabled`, `Description` | |
| `Env`, `Headers` | Stored in plaintext; redacted on reads. |
| `Lazy` | Expose one dispatcher tool instead of every tool. |
| `ToolAllow` | Restricts which tools are exposed. |
| `CreatedMS`, `UpdatedMS` | |

`Validate` caps args, headers and env at 32 entries each, and `ToolAllow` at 128.

**`Store`.** `OpenStore`, `Add`, `SetEnabled`, `Remove`, `Get`, `List`, `Count`. There is **no Update**: to change a server, remove it and re-add it.

**Client.**
- `Conn{Tools(); Call(ctx, tool, args) (text, isError, err); Close()}`.
- `Dial`: stdio, protocol version `2024-11-05`.
- `DialHTTP`: protocol version `2025-03-26`. Echoes `Mcp-Session-Id`, accepts JSON or SSE replies, sends DELETE on close.
- Handshake: `initialize` → `notifications/initialized` → `tools/list` (the list is cached).
- `tools/call` returns the concatenated `content[].text`.

### 12.2 Invariants and security

- **The child environment is scrubbed.** Only an allowlist passes through (PATH, HOME, TEMP, …). Anything named like KEY, TOKEN, SECRET, PASSWORD, CRED, `AWS_` or `AGEZT_` is dropped. The code calls this "the load-bearing safety property of attach".
- **Limits.** 16 MiB frame cap; 15 s handshake; 90 s per call. Calls on one connection are serialised.
- **HTTP SSRF guard.** Uses `netguard.New(AllowLoopback, AllowPrivate)` for every dial and redirect.
- The control plane echoes only env and header **keys**, never their values.
- A server that fails to attach at boot is skipped; it does not fail boot.

### 12.3 Bridging (`kernel/runtime/mcptool_bridge.go`)

`mergeMCPTools` runs once per run (`loopconfig.go:41`, `workflowrun_helpers.go:22`):
- It snapshots `k.mcpConns` under `mcpMu` and honours `ToolAllow`.
- Tools that are already registered win any name collision.
- **Eager servers:** each tool becomes a `bridgedMCPTool` named `mcp_<server>_<sanitized tool>`, truncated to 64 chars, marked Compensable with confidence 0.45.
- **Lazy servers:** one `lazyMCPDispatch` tool with schema `{tool: enum, arguments}`.

How Edict sees these tools:
- Anything with the `mcp_` prefix requires `mcp.call`.
- The `mcp` management tool: `op=list` requires `introspect`; every other op requires `mcp.install`.

A newly attached tool is visible **from the next run**.

### 12.4 Persistence, concurrency, events, wiring

| Aspect | Details |
|---|---|
| Persistence | `<home>/mcp/servers.json` (atomic). Live connections are held only in `k.mcpConns`. |
| Concurrency | stdio: one `readLoop` goroutine per connection; it gives up after 60 s of frames nobody reads. `Close` sends stdin EOF, waits 2 s, then kills. HTTP: `httpConn.mu` serialises requests. Attach dials outside `k.mcpMu`, then re-checks. |
| Events | `mcp.added`, `mcp.updated`, `mcp.attached`, `mcp.detached`, `mcp.removed`. Subject `mcp.<name>`, actor `mcp`. |
| Wiring | `main.go:1598` calls `AttachEnabledMCPServers`. The `mcp` agent tool supports stdio only for `op=add`. Other entry points: control-plane `mcp_*`, Web UI `/api/mcp*`, `agt mcp add`, and market packs. `builtinmarket` seeds lazy `fetch`, `playwright`, `duckduckgo`, `excel` and `time` servers. The legacy bridge lives in `plugins/external/mcpbridge` ([08](08-providers.md)). |

**Files**

| File | Contents |
|---|---|
| `doc.go` | Governance model. |
| `client.go` | Constants, `ToolDef`, `Conn`, dialer types, JSON-RPC structs. |
| `client_conn.go` | stdio connection (`Dial`, `readLoop`, handshake, `Call`, `Close`). |
| `client_helpers.go` | Environment scrubbing. |
| `http.go` | Streamable HTTP connection, `DialHTTP`. |
| `http_helpers.go` | `roundTrip`, headers, response and SSE parsing. |
| `store.go` | `Server`, `Validate`, `Store`. |

Tests: `client_test`, `http_test`, `store_test`, `coverage_supp_test`.

## 13. `kernel/acp` — Agent Client Protocol (server and client)

**Purpose.** Lets IDEs such as Zed drive agezt over ACP: JSON-RPC 2.0, newline-delimited, on stdio (SPEC-15 §3). The work itself is done by a `Runner`. The package also has a `Client`, used by the `acp_agent` tool to drive external ACP agents.

**Server.**
- `New(runner, in, out)`, `Serve(ctx)`.
- `Runner.Prompt(ctx, cwd, intent, onChunk)` streams chunks:
  - `ChunkMessage` → `agent_message_chunk`;
  - `ChunkThought` → `agent_thought_chunk` (M322).

| Method | Behaviour |
|---|---|
| `initialize` | Replies protocol version 1, `loadSession:false`. |
| `session/new` | Returns `sess-N` and stores the `cwd`. |
| `session/prompt` | Streams `session/update` notifications and returns `stopReason:"end_turn"`. |
| `session/cancel` | Ignored. |
| anything else | `-32601`. |

- Messages are capped at 8 MiB; writes are serialised.
- **Governance:** `cmd/agt/acp.go controlPlaneRunner` streams `CmdRun` to the daemon, mapping `llm.token` to message chunks and `llm.reasoning` to thought chunks. ACP prompts therefore take the same governed path as `agt run`.

**Client.**
- `NewClient`, `Initialize`, `NewSession(ctx, cwd)`, `Prompt(ctx, sid, text, onChunk)`.
- Relays only message chunks.

**Notes.**
- No persistence, no env vars, no events.
- `Serve` is synchronous, so **cancel cannot interrupt a running prompt**.
- `controlPlaneRunner` ignores `cwd`.
- The client never answers requests that the agent sends to the client, such as `session/request_permission` or `fs/*`.
- Some doc comments are attached to the wrong functions (`acp.go:22,205`, `acp_helpers.go:78`).

**Files**

| File | Contents |
|---|---|
| `doc.go` | Package doc. |
| `acp.go` | `New`, wire types, `Serve`/`dispatch`, handlers, `writeMessage`. |
| `acp_helpers.go` | Bounded scanner, `ProtocolVersion`, `ChunkKind`, `Runner`, `Server`, `flattenPrompt`. |
| `client.go` | `Client`. |

Tests: `acp_test`, `bound_test`, `client_test`, `coverage_direct_test`, `coverage_extra_test`.

## 14. `kernel/acpcatalog` — ACP agent discovery

**Purpose.** The discovery layer behind `acp_agent`. It merges three sources:

1. **Built-in `Catalog`** of three agents:
   - `gemini` → `gemini --experimental-acp`
   - `claude-code` → `claude-code-acp`
   - `codex` → `codex acp`
2. **Host detection:** `LookPath` plus a 3 s `--version` probe, with up to 8 probes in parallel.
3. **Official sources:**
   - the ACP registry (CDN JSON; ≤2 MiB, ≤512 agents, 15 min TTL, keep-last-good);
   - the official clients page (MDX, parsed by `ParseClientsMDX`).

**API.**
- `Detect`, `Find`, `Installed`, `AnyInstalled`, `InstalledSlugs`.
- `ResolveCommand(ref, fallback)` accepts **slugs only**. A raw command is accepted only from the operator's `AGEZT_ACP_AGENT_CMD`, and only when `ref==""` (CWE-78).
- `Discover` / `DiscoverWith` merge the sources above.
- `ResolveLaunch` builds program + argv with no shell. It has no caller in this repo and is allowlisted in deadcodecheck for SDK consumers.
- `validateRegistry` checks the registry document.

**Wiring.**
- `controlplane/acp.go` serves the `acp_agents` command and `/api/acp/agents`.
- `plugins/tools/acpagent` (capability `acp_agent`) spawns agents with `sandbox.Command` (`IsolatedEnv`) and a 5 m timeout.
- No persistence and no events. The HTTP client is `netout.OperatorClient` (fixed URLs, 6 s timeout, metadata refused).

**Gotchas.**
- Agents found only in the registry cannot be selected through `acp_agent`, because `ResolveCommand` accepts only the three catalog slugs.
- The probed `InstalledVersion` is never populated.
- Refresh is never forced.

**Files**

| File | Contents |
|---|---|
| `acpcatalog.go` | `Agent`, `Catalog`, inventory types, `Detect`, lookups, `ResolveCommand`. |
| `clients.go` | Clients-page fetcher and `ParseClientsMDX`. |
| `registry.go` | Registry types and `RegistryClient`. |
| `registry_helpers.go` | Platform id, npx/uvx pinning, command rendering. |
| `registry_launch.go` | `LaunchSpec`, `ResolveLaunch`. |
| `registry_validate.go` | `validateRegistry`, `Discover`. |

Tests: `acpcatalog_test`, `clients_test`, `registry_test`, `coverage_supp_test`.

## 15. `kernel/toolbox` — host CLI tool inventory and installer

**Purpose (M956).** Shows which catalog CLI tools are installed, missing or outdated, and installs them through a package manager.

- Detection is read-only.
- **Install runs on the host with no sandbox, by design:** installing software necessarily changes the system. It is reachable only through the authenticated control plane.

**API.**
- `Recipe{Manager, Install, Upgrade}` and `Tool{Name, Category, Description, VersionArgs, BinByOS, Recipes}`.
- `ResolveInstall` is pure: it picks the first recipe whose manager is present.
- `DetectManagers` looks for:
  - Windows: winget, choco, scoop;
  - macOS: brew;
  - Linux: apt, dnf, pacman;
  - any OS: pip, npm, cargo, go.
- `Catalog` lists 38 tools.
- `Detect` probes up to 12 tools in parallel, 3 s each.
- `Install` runs argv only (no shell), for catalog names only, with a 20 m timeout. It reports `Skipped` instead of returning an error.
- `Outdated` substring-matches the package managers' upgrade lists. It is loose: it can match `-y` or `--silent`, so it may flag tools that are up to date.

**Wiring.**
- Control-plane commands `toolbox_detect`, `toolbox_outdated`, `toolbox_install` (streaming).
- Bus events `toolbox.install.requested` and one `toolbox.installed` per tool; `toolbox.progress` frames on the stream.
- No persistence, no env vars.

**Files**

| File | Contents |
|---|---|
| `doc.go` | Package doc. |
| `toolbox.go` | Types, `ResolveInstall`, manager probes. |
| `catalog.go` | Recipe helpers and `Catalog`. |
| `toolbox_detect.go` | `Detect`. |
| `toolbox_install.go` | `Install`. |
| `toolbox_outdated.go` | `Outdated`. |

Tests: `toolbox_test`, `toolbox_cov_test`.

---

# Part III — The workflow engine

## 16. `kernel/workflow` and its executor in `kernel/runtime`

**Purpose.** n8n-style automation (M798). A workflow is a durable, named, **acyclic** graph of typed nodes. Nodes are joined by edges that carry ports, and data passes between them through `{{path}}` templates.

Responsibilities are split across packages:

| Package / file | Owns |
|---|---|
| `kernel/workflow` | Schema, `Validate`, `Store`, templating, built-in gallery, cron/event trigger runner |
| `kernel/runtime/workflowrun*.go` | Execution |
| `kernel/runtime/workflowdraft.go` | Copilot (draft/refine) |
| `kernel/runtime/workflowtestnode.go` | Single-node test |
| `kernel/webui` + `kernel/controlplane` | Webhook trigger |

### 16.1 Schema

- **Types:**
  - `Workflow{ID, Name, Enabled, Description, Nodes, Edges, CreatedMS, UpdatedMS}`
  - `Node{ID, Type, Label, Config json.RawMessage, X, Y, TimeoutSec, Retries, RetryDelaySec}`
  - `Edge{From, To, Port}`
- **Node library** (15 types, `knownTypes`):

| Type | Config | Ports / output |
|---|---|---|
| `trigger` | `TriggerConfig{Kind, IntervalSec, DailyAt, Subject, Secret, Reply}` | Payload becomes `{{trigger.payload}}` |
| `tool` | `ToolConfig{Tool, Args}` | Tool output (parsed JSON when possible) |
| `llm` | `LLMConfig{Prompt, System, Model}` | Trimmed completion text |
| `condition` | `ConditionConfig{Left, Op, Right}` | Ports `true` / `false` |
| `transform` | `TransformConfig{Template}` | Interpolated value |
| `delay` | `DelayConfig{Seconds float64}` | Seconds; capped at 600 |
| `http` | `HTTPConfig{Method GET\|POST, URL, Headers, Body, ContentType}` | Uses the `http` tool |
| `code` | `CodeConfig{Language, Code, Input}` | Sandbox output |
| `map` | `MapConfig{Items, Template}` | Array; `{{item}}` / `{{index}}`; ≤1000 items |
| `filter` | `FilterConfig{Items, Left, Op, Right}` | Filtered array |
| `switch` | `SwitchConfig{Value, Cases[]{Equals, Port}}` (1–16 cases) | Case port or `default` |
| `merge` | `MergeConfig{Mode any\|all}` | Upstream outputs in edge order |
| `approval` | `ApprovalConfig{Description, Capability}` | `"granted by …"` or error |
| `subworkflow` | `SubflowConfig{Workflow, Payload}` | `{executed, outputs}` of the child |
| `pipeline` | `PipelineConfig{Steps[]{ID, Tool, Args, OutputSchema}}` (1–16 steps) | `{last, steps}` |

- **Condition / filter ops:** `equals`, `not_equals`, `contains`, `not_empty`, `empty`, `gt`, `lt`.
- **Failable nodes** (only these may wire an `error` port or set retries): tool, llm, http, code, approval, subworkflow, pipeline.
- **Trigger kinds** (`validateTriggerConfig`):
  - `manual` / `""`
  - `cron`: exactly one of `interval_sec` (≥30) or `daily_at` (HH:MM, padded to canonical form)
  - `event`: subject glob. Refuses `workflow.*`, `>` and `*`, which would create feedback loops.
  - `webhook`: secret of at least 12 chars, plus an optional `reply`
- **Validation limits:**
  - Name: `^[a-z0-9][a-z0-9._-]{0,63}$`
  - ≤100 nodes, ≤300 edges, ≤32 KiB config per node
  - Exactly one trigger, with no incoming edges
  - No self-edges; ports are checked per type
  - Acyclic (Kahn's algorithm)
  - Per node: `timeout_sec` 0..600, `retries` 0..5, `retry_delay_sec` 0..60
  - **Reachability from the trigger is not enforced.**
- **Templating** (`template.go`):
  - `Interpolate` / `Lookup` resolve dotted paths, including numeric indexes. There are no expressions or pipes.
  - Unknown paths render as `""`. Non-strings render as compact JSON.
  - Namespaces: `trigger.payload`, `<node_id>.output`, `item` / `index` (inside map/filter), `steps.<id>.output` (inside a pipeline).
- **Gallery:** `Templates()` provides five templates (`daily-status-check`, `failed-task-triage`, `resilient-fetch`, `team-router`, `list-pipeline`), pinned by `TestTemplatesValidate`. The gallery never writes to the store.

### 16.2 Execution model (`runtime/workflowrun*.go`)

```
RunWorkflow(ctx, corr, ref, payload)                       workflowrun.go:110
  halted? -> ErrHalted ; Get ; Validate (stores may predate rules)
  agent.WithCorrelation ; workflowRunProvenance(wake ctx) -> workflow.started
  runWorkflowGraph                                         workflowrun_graph.go:22
    edges grouped by (from, port) ; indegree ; data = {"trigger":{"payload":p}}
    BFS queue seeded with trigger ; steps <= workflowStepCap (256)
    per node:
      skip if already executed            (a node runs ONCE: first token wins)
      merge "all": wait until tokens == indegree
      nodeInputPreview  -> journal "input" snippet
      execNodeWithReliability                              :136
        attempts = 1+Retries ; per-attempt context.WithTimeout(TimeoutSec)
        error "node timeout after Ns" ; RetryDelaySec sleep ; cancelled RUN never retries
        execWorkflowNode (switch on type)                  workflowrun_node.go:22
      error + wired "error" edge -> output {"error":msg}, port "error" (handled)
      publish workflow.node {workflow,node,type,ok,label,port,attempts,input,output(≤2000 runes),error,handled}
      unhandled error -> return (run fails)
      data[id] = {"output": out} ; enqueue successors on fired port
  -> workflow.completed | workflow.failed (with executed list)
```

**Execution properties:**
- Execution is **sequential**: one goroutine does a BFS, and branches do not run in parallel.
- Delay, approval and retry sleeps all block that goroutine.
- A subworkflow runs inline and recursively under the **same correlation**. Depth comes from ctx `wfDepthKey` and is limited to `maxSubflowDepth=3`.

**Governance per node (no laundering).** Tool, http and pipeline steps call `invokeWorkflowTool` (`workflowrun_helpers.go:21`), which does the following in order:
1. Merges built-in, script and MCP tools.
2. `agent.ValidateToolInput`.
3. `k.policyHook`, so Edict deny and ask-to-approval apply exactly as they do in the agent loop.
4. `tool.Invoke`.

The other node types:

| Node | Governance |
|---|---|
| `http` | Goes through the registered `http` tool, so its egress guard and `http.get` / `http.post` capability apply. |
| `code` | Requires `cfg.ScriptRunner`, then passes a synthetic `code_exec` call through `policyHook`. |
| `llm` | `k.completeAux(ctx, corr, "workflow", …)`, which attributes spend to the correlation. When the node has no model, it resolves the current model through `resolveRunModel`, not the boot model. |
| `approval` | `k.approvals.Submit` with capability `workflow.approve` by default. |

**Data inspection (M808, M811).** There is **no separate run store; the journal is the record.**
- Every executed node emits `workflow.node` carrying input and output snippets.
- `controlplane/workflow_handlers_runs.go:19` folds `workflow.<name>` events by correlation into run history (default 20 runs, max 100).
- `TestWorkflowNode` (`workflowtestnode.go:35`) runs a single node with mock upstream data and journals it with `test:true`. The history fold skips these test events.

**Copilot** (`workflowdraft.go`):
- `DraftWorkflow` / `RefineWorkflow` call `draftLoop`, which makes at most two provider round-trips (the second is a repair attempt that carries the validator error).
- `parseWorkflowDraft` zeroes the ID, timestamps and `Enabled`, auto-lays-out the graph, and validates it.
- The result is **returned unsaved**: the copilot never installs automation. It journals `workflow.drafted`.

### 16.3 Triggers and who calls `RunWorkflow`

| Entry | Path | WakeContext |
|---|---|---|
| cron / event | `workflow.StartTriggers` (`runner.go:48`), called from `main.go:1691`. A bus subscriber on `">"` (drops `workflow.*`, 30 s event cooldown) plus a 15 s ticker (interval anchored at arm time; `daily_at` once per local day), then `safeFire` (journals `workflow.panic`), then `wfFire` (15 min timeout). | **none, so the run is labelled `manual`** (gotcha) |
| webhook | Web UI `handleWorkflowHook`:<br>• POST only<br>• rate-limited before auth<br>• secret from `X-Agezt-Secret` or `?secret=`<br>• 256 KiB body cap<br>Then `CmdWorkflowWebhook` → `handleWorkflowWebhook`:<br>• found + enabled + kind webhook<br>• constant-time secret check<br>• uniform refusal message<br>With `reply`, the run is synchronous (2 min) and returns 200 with outputs. Otherwise it is detached and returns 202 with the correlation id. | `{Source:"webhook", TriggerSubject:"webhook:<name>"}` |
| manual | `controlplane/workflow_handlers_run.go:169`, sync (15 min) or async | `{Source:"manual"}` |
| schedule | cadence `TargetWorkflow` (`main_cadence.go:97`) | `{Source:"schedule", ScheduleID}` |
| agent | `workflow` tool `op=run` (`plugins/tools/workflowtool`) | Inherits the agent's ctx, so `runner:"agent"` |
| subworkflow | `workflowrun_node.go:256` | Parent ctx |

**Agent-saves-disabled invariant.** `workflowtool/tool.go:139-146`: a workflow **created** by an agent is immediately `SetWorkflowEnabled(false)`. Arming it requires an explicit, gated `op=enable`.

Capability mapping for the `workflow` tool:
- Mutating ops: `workflow.manage`
- `list` / `show`: `introspect`

### 16.4 Persistence, concurrency, events

| Aspect | Details |
|---|---|
| Persistence | `<home>/workflows/workflows.json` (`runtime/compose.go:217`): JSON array, saved atomically and rolled back if the save fails. `Store.Save` upserts by name and preserves ID, CreatedMS and Enabled; a **new** console save starts **enabled**. Trigger runner state (last fire times) is **in memory only**. |
| Concurrency | `Store.mu`. The trigger runner has 2 goroutines; each fire is a bare `go safeFire` with no per-workflow concurrency limit beyond the cooldown and interval. Each run is one goroutine. |
| Env | None. Cooldown and tick are code defaults. |
| Events | `workflow.saved`, `workflow.updated`, `workflow.removed`, `workflow.restored`, `workflow.started`, `workflow.node`, `workflow.completed`, `workflow.failed`, `workflow.panic`, `workflow.drafted`. Subject `workflow.<name>`, actor `workflow`. |

**`kernel/workflow` files:**

| File | Contents |
|---|---|
| `doc.go` | Model doc; execution lives in runtime. |
| `workflow.go` | Node-type constants, `knownTypes`, `failable`, Node/Edge/Workflow, limits, `Validate` (Kahn), `TriggerConfig`, `canonicalDailyAt`, `TriggerSpec`. |
| `workflow_node.go` | Per-node config structs, `TriggerNode`, `NodeByID`, `Store` struct. |
| `workflow_validate.go` | Trigger validation, condition ops, `validateEdgePort`, reliability bounds, per-type `validateNodeConfig`. |
| `workflow_store.go` | `OpenStore`, `Save`, `Restore`, `SetEnabled`, `Remove`, `Get`, `List`, `Count`. |
| `runner.go` | `StartTriggers` (event + cron), `safeFire`, `SubjectMatch` (`*` matches one token, trailing `>` matches one or more). |
| `template.go` | `Interpolate`, `Lookup`, `renderValue`. |
| `templates.go` | `Template` and the five-entry gallery. |

Tests:
- In this package: `runner_test`, `templates_test`, `workflow_test`.
- In runtime: `workflowrun_test`, `workflowrun_m800_test`, `workflowreliability_test`, `workflowdraft_test`.

### 16.5 `kernel/workflowexec` — dead code

`workflow.go` holds `StepCap=256`, `SnippetMax=2000`, `MaxSubflowDepth=3`, `DepthKey`, `Result` and `Snippet`. Its package doc claims it holds "the RunWorkflow adapter and per-node dispatch". **It has no importers** outside its own test, and runtime keeps private copies of all of it (`workflowStepCap`, `wfSnippetMax`, `wfSnippet`, `wfDepthKey`, `maxSubflowDepth`, `RunWorkflowResult`). It is an abandoned extraction. `tools/deadcodecheck/main.go:157` allowlists it.

Recommendation: delete it, or finish the extraction. Do not wire new code to it.

---

# Part IV — Channels

## 17. `kernel/channel` — the messaging vocabulary

**Purpose.** Defines the canonical types every messaging surface normalises to. Agents, the Unified Inbox and Pulse only ever see these types (SPEC-04). The package doc states the security rule: *inbound text is data, never kernel instructions*. An allowlist decides who may drive the agent, and every tool call still passes Edict.

**Types:**
- `Channel` interface:
  - `Name() string`
  - `Start(ctx) error`: listens until ctx is cancelled
  - `Send(ctx, Outbound) error`
- `UnifiedMessage{ChannelKind, ChannelID, ThreadID (M885), Sender, Text, Images []string (data: URLs), Audio []string (data: URLs), PlatformTSMS, PlatformMeta}`.
- `Outbound{ChannelID, ThreadID, Text, Priority info|notify|urgent, Attachments}`
- `Attachment{Kind image|audio|file, Data, MIME, Filename}`
- `Reply{Text, Attachments}`
- `InboundHandler func(ctx, UnifiedMessage, corr) (Reply, error)`
- `Allowlist`: `NewAllowlist`, `Allows`, `Empty`. **An empty allowlist denies everyone** (fail-closed), which makes the channel outbound-only.

**Registry** (`registry.go`, process-global, `sync.RWMutex`, VULN-002):
- `Manifest{Kind, Display, Description, Transport, Duplex, ConfigSection, RequiredEnv, DocsURL, Media MediaCaps{ImageIn,VoiceIn,ImageOut,VoiceOut}, SetupSteps, ConnectMethod token|qr|gateway|oauth, AddrEnv, AllowlistEnv, InboundEnv, BannerLabel, DisabledHint}`
- `RegisterManifest`, `Manifests` (sorted by Display), `LookupManifest`
- `SetLive` / `IsLive` (per kind) and `SetLiveInstances` / `IsLiveInstance` (per instance)
- `InstanceKey(kind, label)` returns `kind` or `kind#label`

**Helpers:**
- `Guard(bus, name, fn)` recovers a per-message panic and journals `channel.error` on subject `channel.<name>.error`.
- `ConversationHistory(journal, kind, channelID, threadID, sender, limit)` folds the journal's `channel.inbound` and `channel.outbound` events into a transcript:
  - scoped to the thread;
  - **isolated per sender**: only that sender's inbound messages, plus replies whose correlation matches one of their messages;
  - each message capped at 2000 chars;
  - returns `""` when there is no prior turn.
- `SplitText(text, limit)` chunks by UTF-16 code units, prefers newline or space boundaries, and is lossless.

**Files:**

| File | Contents |
|---|---|
| `channel.go` | Core types, `Channel` interface, `Allowlist`. |
| `guard.go` | `Guard`. |
| `registry.go` | Manifest and live registries. |
| `history.go` | `EventRanger`, `ConversationHistory`. |
| `split.go` | `SplitText`. |

Tests: `channel_test`, `coverage_test`, `guard_test`, `history_test`, `registry_race_test`, `split_test`.

## 18. `kernel/channelwire` — channel factory layer (Phase 2.1)

**Purpose.** Each channel kind registers a `Factory` that builds its configured instances: the default account plus every `#label` account. The daemon then runs one loop over `channel.Manifests()` instead of 27 hand-written builders.

The package sits **above** `kernel/channel` because factories return a `pulse.BriefSink`. Importing pulse into `kernel/channel` would drag `kernel/agent` into every transport plugin (`channelwire.go:14-18`).

**API:**
- `Deps{Ctx, Bus, Handler, Get func(baseEnv) string, Label}`. Factories read configuration **only** through `Get`, which retired the old `os.Setenv` overlay hack.
- `Built{Channels, Sink, Desc}`. `Desc==""` means "not configured, skip it", which replaces the typed-nil reflect probe.
- `NotConfigured`
- `Factory`
- `Register` / `Lookup`, backed by a global map guarded by an RWMutex
- `Instance{Kind, Label, Key, Desc, Channel, Sink}`
- `Labels(kind)`: scans `os.Environ()` for `BASE#label` variants of `settings.SectionEnvs(kind)`. This assumes the settings section id equals the kind.
- `BuildKind(ctx, kind, bus, handler)`:
  - iterates the default account (`""`) and each label;
  - keys extra channels from a multi-channel factory (the push family) by `ch.Name()`;
  - gives the sink to the first instance only.

**Removed helpers.** `BuildAll` and `MissingFactories` were deleted on 2026-08-12 because nothing called them. The manifest→factory drift alarm is `TestEveryManifestHasFactoryOrTODO` in `plugins/builtinchannels`. `RegisterAll` registers 34 manifests and 34 factories (see [09-channels.md](09-channels.md)).

**File:** `channelwire.go` (everything). Tests: `channelwire_test.go`.

### 18.1 Inbound flow: message → run → reply

```
boot: builtinchannels.RegisterAll() (main.go:294)
      chanHandler := makeChannelHandler(k)                        cmd/agezt/main_channels_handler.go:21
      for m in channel.Manifests(): channelwire.BuildKind(ctx, m.Kind, k.Bus(), chanHandler)
      -> registerInstances / go ch.Start(ctx); channel.SetLive / SetLiveInstances
      sinks = combineSinks(instanceSinks(...)) -> buildPulse / buildAlertNotify (outbound briefs)

inbound (e.g. telegram):
  poll loop -> channel.Guard(bus,"telegram", handleInbound)
  build UnifiedMessage ; corr := "chan-"+ulid
  publish channel.inbound {channel_kind, channel_id, thread_id, sender, text, allowed}   (each plugin journals itself)
  allowlist.Allows(chatID)? no -> "not authorized" reply, handler never runs (media not even fetched)
  yes -> handler(ctx, msg, corr):
     intent = ConversationHistory(k.Journal(), kind, id, thread, sender, AGEZT_CHANNEL_HISTORY=10) or msg.Text
     images: visionGate -> WithImages(ctx)  |  vision sidecar k.DescribeImages -> caption appended ; persistInboundImages (artifact index)
     audio : k.Voice().Transcribe (if HasSTT) -> transcript appended ; persistInboundAudio
     text, err := k.RunWith(hctx, corr, intent)            <- the governed run (default identity)
     voice in + TTS + AGEZT_VOICE_REPLY!=off -> k.Voice().Speak -> audio Attachment
  plugin: error -> "sorry — that failed: …" ; Send(Outbound{ChannelID, ThreadID, Text, Attachments})
          SplitText(…, platform limit) ; publish channel.outbound under the same corr
```

**There is no session object.** Each message is a fresh run under a new correlation. Continuity comes **only** from the journal fold, keyed by `(channel_kind, channel_id, thread_id, sender)`.

Outbound sends (`agt send`, notify tools, standing briefs, the scheduled-answer notify) go through the `channelSend` / `channelSendMedia` closures in cmd/agezt, resolving targets with `instanceMatch`:
- an exact `kind#label` sends to one instance;
- a bare `kind` fans out to every instance of that kind.

---

# Part V — Media tools and STT

| Package | Seam (injected by daemon) | Tool | Capability | Registered when |
|---|---|---|---|---|
| `voicetool` (`voice.go`) | `Voice{Transcribe(ctx, audio, filename); Speak(ctx, text) ([]byte, mime, error); HasSTT(); HasTTS()}` | `voice` (op `transcribe` \| `speak`) | `provider.call` | `cfg.Voice != nil` (`runtime/compose.go:280-284`) |
| `imagetool` (`image.go`) | `ImageGen{GenerateImage(ctx, prompt, size, quality, n) ([][]byte, mime, error); HasImage()}` | `image_generate` | `provider.call` | `cfg.ImageGenerator != nil` (`:286-289`) |
| `reranktool` (`rerank.go`) | `Reranker{Rerank(ctx, query, docs, topN) ([]int, []float64, error); HasRerank()}` | `rerank` | `provider.call` | `cfg.Reranker != nil` (`:291-293`) |

- All three names are mapped explicitly in `edict/toolmap.go:147`. Before that mapping they were unmapped and therefore default-denied (see the "tool capability must be governed" lesson).
- `voice` and `image_generate` persist their output through `SaveArtifact` (`k.artifacts.Put`) and return refs, not inline bytes.
- **Voice transcripts are marked `ObservationUntrusted`** with source `voice:transcription`, because they are external data.
- `rerank` returns `[{rank, index, score, text}]` and range-checks indexes. It does not check `len(scores)==len(idx)`, so an adapter that returns a short `scores` slice would cause a panic.
- `runtime/toolseams.go` aliases the seams as `runtime.Voice`, `runtime.ImageGen` and `runtime.Reranker`. The concrete adapters live in `plugins/providers/{voice,image,rerank}` ([08](08-providers.md)). The kernel never imports them.
- Adapter env vars, read in `daemonconfig_load.go:141-175` (a misconfiguration produces a warning and disables the feature; it never fails boot):
  - STT: `AGEZT_STT_PROVIDER`, `AGEZT_STT_URL`, `AGEZT_STT_MODEL`, `AGEZT_STT_KEY`
  - TTS: `AGEZT_TTS_PROVIDER`, `AGEZT_TTS_URL`, `AGEZT_TTS_MODEL`, `AGEZT_TTS_KEY`, `AGEZT_TTS_VOICE`
  - Image: `AGEZT_IMAGE_URL`, `AGEZT_IMAGE_MODEL`, `AGEZT_IMAGE_KEY`
  - Rerank: `AGEZT_RERANK_URL`, `AGEZT_RERANK_MODEL`, `AGEZT_RERANK_KEY`

**`kernel/stt`** (`stt.go`) is **not a tool**. It is a dependency-free client for the OpenAI-compatible `POST <base>/audio/transcriptions` (multipart).
- `New(Config{APIURL, APIKey, Model, HTTPClient})`. Defaults: base `https://api.openai.com/v1`, model `whisper-1`, 120 s timeout. Responses are capped at 4 MiB and the key is scrubbed from errors.
- Used by `agt transcribe` / `agt listen`, as the web mic fallback when `k.Voice().HasSTT()` is false, and by `/v1/audio/transcriptions`.
- **It has its own env family:** `AGEZT_STT_API_KEY` (falls back to `OPENAI_API_KEY`), `AGEZT_STT_API_URL`, `AGEZT_STT_MODEL`. This is separate from the voice-adapter `AGEZT_STT_URL` / `AGEZT_STT_KEY`.

Tests: `stt_test`, `voice_test`, `image_heavycov_test`, `rerank_test`.

---

# Part VI — Self-update

## 19. `kernel/update`

**Purpose.** Check a release source, download to staging, verify the SHA-256 and Ed25519 signature, drain, then atomically rename into place. On error the live binary is untouched, and the daemon never restarts itself automatically (`doc.go`).

### 19.1 API

- Types:
  - `Source`: `SourceGitHub` | `SourceEndpoint`
  - `Provenance`: `ProvenanceUnverified` (zero value, deliberately untrusted), `ProvenanceGitHubRelease` (set only by `checkGitHub`), `ProvenanceEndpoint` (set only by `checkEndpoint`)
  - `UpdateInfo{Version, SHA256, URL, Notes, Provenance, Signature}`
  - `Manifest` (endpoint JSON)
  - `Config{Source, GitHubOwner, GitHubRepo, Endpoint, CurrentSHA256 (unused), BaseDir, DrainTimeout, CheckInterval, HTTPClient}`
- `New(cfg)` builds a netguard client (loopback and private allowed, link-local/CGNAT blocked) whose `CheckRedirect` requires HTTPS on every hop (UPD-002). Timeout 30 s.
- `Check(ctx)`:
  - GitHub: `releases/latest`, choosing the asset by `<os>_<arch>` (darwin→`macos`).
  - Endpoint: `Manifest`.
  - Returns nil if the version equals `CurrentVersion = brand.Version`.
- `Apply(ctx, info, drainFunc)`:
  1. take the `update.lock` O_EXCL lockfile
  2. `downloadBinary` → `bin/agezt[.exe].new` (via `.tmp`, HTTPS re-checked on redirect, empty body rejected)
  3. `validateSHA256`
  4. `verifySignature`
  5. drain
  6. `os.Rename(.new → bin/agezt)`
  7. `chmod 0755`
- Errors: `ErrUpdateInProgress`, `ErrDrainTimeout`, `ErrChecksumMismatch`, `ErrSignatureMissing`, `ErrSignatureKeyNotConfigured`, `ErrSignatureInvalid`.

### 19.2 Signature verification (UPD-001)

- Signed message: `version + "\n" + sha256hex`. The algorithm is `ed25519.Verify`.
- Public key resolution: runtime `updatePubKey` if set, otherwise build-time `DefaultPublicKeyHex`, otherwise nil.
  - **`DefaultPublicKeyHex = ""`** (`update_apply.go:165`), and no build script in the repo injects it with `-ldflags -X`.
  - The `SetPublicKey` named in comments exists **only in `signature_test.go`**.
  - Net effect: no key is configured in production.
- Policy, based on the manifest's own provenance, not on `cfg.Source`:

| Key configured? | Provenance | Result |
|---|---|---|
| No | GitHub release | Passes (SHA only) |
| No | Anything else | `ErrSignatureKeyNotConfigured` |
| Yes | Any | Signature required, must be valid hex, must verify |

### 19.3 Trigger surfaces and persistence

**Trigger surfaces:**
- `agezt update [--apply]` (`boot_ops.go:21`, via the control plane)
- Control-plane commands `update_check` / `update_apply`. A successful apply writes `update.sentinel` (RFC3339) and calls `signalShutdown`; the watchdog respawns the daemon and clears the sentinel.
- REST `GET /api/v1/update` and `POST /api/v1/update/apply` (admin only, no-op drain)
- Background checker `startUpdateChecker` (`boot_ops.go:77`). It runs only for the endpoint source with `CheckInterval>0`.

**Env** (`daemonconfig_load.go:256-272`):
- `AGEZT_UPDATE_ENDPOINT`
- `AGEZT_UPDATE_GITHUB_OWNER`
- `AGEZT_UPDATE_GITHUB_REPO` (defaults to `brand.Binary`)
- `AGEZT_UPDATE_DRAIN_TIMEOUT` (30 s) and `AGEZT_UPDATE_CHECK_INTERVAL` (0 = off) apply to the endpoint source only. GitHub hard-codes 30 s / 0.

**Persistence** (under `<home>`):
- `update.lock`
- `bin/agezt[.exe].new.tmp`
- `bin/agezt[.exe].new`
- `bin/agezt[.exe]`
- `update.sentinel`

**Events** (background checker only):
- `update.available` (kind `info`)
- `update.failed` (kind `system.anomaly`)
- `update.applied` (kind `info`)

**Files:**

| File | Contents |
|---|---|
| `doc.go` | Design principles. |
| `update.go` | Enums with UPD-001 rationale, types, `New`, `Check`, accessors. |
| `update_apply.go` | `Apply`, error types, `DefaultPublicKeyHex`, key store, `resolvePublicKey`, `signedMessage`. |
| `update_verify.go` | `verifySignature`, `checkGitHub`, `checkEndpoint`, `requireHTTPS`, `downloadBinary`, `validateSHA256`, `acquireLock`. |

Tests: `update_test`, `update_cov_test`, `signature_test` (defines the test-only `SetPublicKey`), `https_test`.

### 19.4 The update path is effectively inert today

- **GitHub source:** `checkGitHub` never fills in `SHA256`, so `Apply` always fails "empty SHA256". The tests set the SHA by hand.
- **CLI / control plane / REST apply:** these rebuild `UpdateInfo` from version, sha, url and notes. That drops `Signature` and leaves provenance `Unverified`, so without a key the result is `ErrSignatureKeyNotConfigured`.
- **Background checker:** it calls `k.DrainAndHalt` *before* `Apply` downloads and verifies. With no key, verification fails and **the kernel stays halted with runs suspended**, and nothing resumes it. On success it calls `os.Exit(0)` without writing the sentinel.
- **Wrong binary:** `Apply` swaps `<home>/bin/agezt`, but the watchdog respawns `os.Executable()`. Renaming over a running exe also fails on Windows.
- **No rollback:** the old binary is not kept.
- **Stale lock:** a stale `update.lock` (from a crash) blocks every later update forever, because the PID is never checked for liveness.
- **Panic:** `ErrChecksumMismatch.Error()` slices `Want[:8]`, so a manifest SHA shorter than 8 chars panics.

---

# Part VII — `kernel/internal/testfixtures`

`helpers.go` provides `WithUsage(resp, usage)` and `ToolUse(callID, toolName, input)`. `ToolUse` builds an assistant `agent.CompletionResponse` with one tool call and `StopToolUse`. The package doc says it is "only imported by _test.go files". Its consumers are `kernel/agent`, `kernel/controlplane` and `kernel/runtime` `mock_helpers_test.go`.

It is excluded from the env-var inventory guard (`controlplane/config_inventory_test.go:55`) and allowlisted in deadcodecheck (`tools/deadcodecheck/main.go:153`). It is test-only by design, not dead code (see the "test-only ≠ dead code" lesson).

---

# Cross-cutting reference

## Persistence map (under `AGEZT_HOME`)

| Path | Owner | Format |
|---|---|---|
| `roster/roster.json` | roster | JSON array of `Profile`, atomic |
| `cadence/schedules.json` | cadence | JSON array of `Entry`, atomic |
| `standing/standing.json` | standing | JSON array of `Order`, atomic |
| `state/pulse_probe.json`, `state/pulse_seen.json` | pulse (via `state.FileStore`) | Namespace map, atomic |
| `skills/skills.json` | skill | `map[id]Skill`, atomic |
| `skills/bundles/<slug>/…` | skill | Raw resource files (staged swap) |
| `market/installed.json`, `market/sources.json` | market | JSON, atomic |
| `market/marketplaces/<name>/{index.json, packs/*.json}` | market | Staged dir + rename |
| `mcp/servers.json` | mcp | JSON array of `Server`, atomic |
| `workflows/workflows.json` | workflow | JSON array of `Workflow`, atomic |
| `update.lock`, `update.sentinel`, `bin/agezt*` | update | Lockfile, RFC3339, binary |
| *(journal only)* | selfrepair, workflow run history, channel conversations | `doctor.auto_repair`, `workflow.*`, `channel.inbound/outbound` events |

All JSON stores go through `kernel/platform/filestore` → `internal/atomicfile` (unique temp file, fsync, rename, Windows retry). They rewrite the whole file on each mutation and roll memory back if the save fails.

## Env vars by package (all must be listed in `controlplane/config.go configEnvVars`; a guard test enforces it)

| Area | Variables |
|---|---|
| cadence | `AGEZT_SCHEDULE`, `AGEZT_SCHEDULE_RUN_TIMEOUT`, `AGEZT_SCHEDULE_NOTIFY`, `AGEZT_GRAVEYARD_RETENTION_DAYS`, `AGEZT_CATALOG_URL` |
| pulse | `AGEZT_PULSE`, `_CADENCE`, `_DIAL`, `_QUIET_HOURS`, `_PROBE`, `_DISK`, `_HEALTH`, `_LLM`, `_INITIATIVE` |
| selfrepair | `AGEZT_AUTO_REPAIR`, `AGEZT_AUTO_REPAIR_COOLDOWN`, `AGEZT_ROUTING_ROLLBACK_PROBATION` |
| skill | `AGEZT_SKILLS`, `AGEZT_FORGE`, `AGEZT_SKILL_SHADOWEVAL`, `_AUTOQUARANTINE`, `_AUTOSHADOW`, `_AUTOPROMOTE` |
| plugin | `AGEZT_PLUGINS`, `AGEZT_PLUGIN_PINS`, `AGEZT_PLUGIN_TOOLS` |
| acp | `AGEZT_ACP_AGENT_CMD` |
| channels | `AGEZT_CHANNEL_HISTORY` (10), `AGEZT_VOICE_REPLY`, per-channel env from manifests (with `#label` variants) |
| media | `AGEZT_STT_*`, `AGEZT_TTS_*`, `AGEZT_IMAGE_*`, `AGEZT_RERANK_*`; kernel/stt: `AGEZT_STT_API_KEY`, `AGEZT_STT_API_URL`, `AGEZT_STT_MODEL` |
| update | `AGEZT_UPDATE_ENDPOINT`, `_DRAIN_TIMEOUT`, `_CHECK_INTERVAL`, `_GITHUB_OWNER`, `_GITHUB_REPO` |

Packages that read **no** env themselves: roster, standing, scheduler, market, mcp, acpcatalog, toolbox, workflow, channel, channelwire, the tools.

## Event catalogue for this area

| Prefix | Kinds | Emitted by |
|---|---|---|
| `schedule.*` | `schedule.fired`, `schedule.system_task.<task>` (kind `info`), `schedule.task` | cmd/agezt fire path, systemtasks |
| `standing.*` | `standing.created`, `.updated`, `.removed`, `.fired`, `.error` | runtime accessors, cmd/agezt |
| pulse | `pulse.tick`, `observer.delta`, `salience.scored`, `initiative.taken`, `initiative.act` (subjects `pulse.initiative.act` / `.ask`), `briefing.sent`, `pulse.paused`, `pulse.resumed` | pulse |
| `doctor.auto_repair` | kind `info` with a `phase` field; `selfrepair.panic` | selfrepair |
| `plan.*` / `node.*` | started / completed / failed | scheduler |
| `skill.*` | created, promoted, quarantined, reverted, restored, activated, shadow_evaluated, shared, reassigned | Forge |
| `market.*` | `market.pack.installed`, `.uninstalled`, `market.source.added`, `.removed`, `market.synced` (ad-hoc, no constants) | controlplane |
| `mcp.*` | added, updated, attached, detached, removed | runtime mcptool |
| `workflow.*` | saved, updated, removed, restored, started, node, completed, failed, panic, drafted | runtime, workflow runner |
| `channel.*` | `channel.inbound`, `channel.outbound`, `channel.error` | each channel plugin; `channel.Guard` |
| `toolbox.*` | `toolbox.install.requested`, `toolbox.installed` | controlplane |
| `update.*` | `update.available`, `update.failed`, `update.applied` | background checker |
| `roster.*` | created, updated, removed | runtime accessors |
| `system.anomaly` | `cadence.injection` subject | cadence engine |

---

# Extension points

| To add… | Do this |
|---|---|
| **A system task** | 1. Add a const and a `SystemTaskInfo` entry in `kernel/cadence/cadence.go`.<br>2. Add a `case` in `systemtasks.Run` and a `runScheduledX` that publishes `schedule.system_task.x`.<br>3. Never import systemtasks from runtime/cadence.<br>4. Add any new env var to `configEnvVars`.<br>Validation, control plane, the schedule-tool enum and the fired payload pick it up automatically. |
| **A cadence target kind** | 1. Add a `Target*` const.<br>2. Add a branch to `Entry.Validate`.<br>3. Add a store setter that clears conflicting fields.<br>4. Add a dispatch branch in `main_cadence.go` before the "unknown target" error.<br>5. Update `scheduleFiredEventPayload`, controlplane `schedule_add` / `validateScheduleRunnable` / `scheduleExecutionMetadata`, and the schedule tool. |
| **A cadence mode** | Touch the const, an `Add*`, `advance`, the `Due` eager/deferred decision, `CompleteFiring`, `usesInterval`, `Cadence()`, `Forecast`, `Reschedule`, and controlplane validation. |
| **A standing trigger type** | 1. Add a `TriggerType` + `Validate` case.<br>2. Add a `StartX` driver using `safeFire` + `pruneToLive`, wired in `buildStandingRunner`.<br>3. Pick a subject convention for `TriggeredIntent`.<br>4. Update the controlplane and `standingtool`. |
| **A pulse observer** | 1. Implement `Observer`: transition-based with a silent baseline; set `Hints["severity"]`, optionally `issue_key` / `actionable`.<br>2. Inject kernel data via a func type (pulse never imports runtime).<br>3. Register it in `buildPulse` (permanent) or via `AddObserver` (removable). Its subject is `pulse.observer.<Source>`.<br>4. Add its env var to `configEnvVars`. |
| **A self-repair mode / resolution** | See §7.5. |
| **A roster field** | 1. Add it to `Profile` (`omitempty`).<br>2. Normalize + validate.<br>3. If kernel-owned, restore it from the snapshot in `Store.Update` and strip it in `controlplane/roster_crud.go` + `overseertool/kernelsource.go`.<br>4. Extend `AutonomyRunbook` if it affects waking; extend guardian defaults/reconcile if needed.<br>5. Add it to the hand-written controlplane views. |
| **A plan node kind** | 1. Append a `NodeKind` and implement `Node`.<br>2. Decide whether it holds a worker slot (only `KindGate` doesn't).<br>3. Add a case in `controlplane/server_handlers_plan.go`. |
| **A built-in skill bundle** | Add a dir under `plugins/builtinskills/<name>/` (SKILL.md + resources) plus two list lines and a test ([10](10-tools.md)). The library is considered saturated. |
| **A market pack** | Built-in: `plugins/builtinmarket`. Remote: author a `pack.json` dir, `agt market publish` (optionally signed with `agt market keygen`), host `marketplace.json`; consumers run `agt market add <url> --pubkey <hex>` then `sync`. |
| **An out-of-process plugin** | Any executable speaking §11.1 (template `kernel/plugin/testdata/echoplugin` or `agt plugin new`). Set `AGEZT_PLUGINS`, pin with `agt plugin hash` → `AGEZT_PLUGIN_PINS`, optionally `AGEZT_PLUGIN_TOOLS`. **Declare `capability` per tool or it is default-denied.** |
| **An MCP transport** | 1. Add a dialer type and a `Conn` impl.<br>2. Add a discriminating `Server` field + `Validate` exclusivity.<br>3. Add a branch in `runtime.dialMCP` and a `Config` seam.<br>4. Add a transport label in `AddMCPServer` / `mcpServerView`. |
| **An ACP catalog agent** | Append to `acpcatalog.Catalog` (valid registry-style slug). Registry-only agents need no code to be listed (but cannot be selected; see Gotchas). |
| **A toolbox tool / manager** | Tool: add one `Tool{}` to `Catalog` (`BinByOS` / `VersionArgs` as needed). Manager: add a recipe helper + a `managerProbes` entry (+ optional `Outdated` query). |
| **A workflow node type** | 1. Add the const + `knownTypes` (+ `failable`), a config struct, and `validateNodeConfig` (+ `validateEdgePort`).<br>2. Add `execWorkflowNode` + `nodeInputPreview` cases in runtime.<br>3. Update the `workflowDraftSystem` prompt (it must stay in lockstep), the workflowtool description, and the frontend canvas. |
| **A workflow trigger kind** | 1. Extend `TriggerConfig` + `validateTriggerConfig`.<br>2. Arm it in `runner.go` (via `safeFire`) or add an external gate like the webhook.<br>3. **Set a `WakeContext`.**<br>4. Update the banner counts. |
| **A channel** | 1. Implement `channel.Channel` in `plugins/channels/<x>`: mint `chan-<ulid>`, check the `Allowlist`, wrap per-message work in `channel.Guard`, journal `channel.inbound` / `outbound` with the payload keys `ConversationHistory` reads, chunk with `SplitText`.<br>2. Register a `Manifest` + `channelwire.Register(kind, factory)` in `plugins/builtinchannels`; the factory reads env only via `d.Get` and returns `NotConfigured` when unset.<br>3. Add a settings section whose id equals the kind. |
| **An update source** | 1. Add a `Source` const.<br>2. Add a `checkX` that sets a **new** `Provenance`.<br>3. Add a policy branch in `verifySignature`.<br>4. Keep the zero value untrusted. |

---

# Gotchas, invariants and findings (consolidated)

**Governance-relevant**
1. **System-task schedules bypass the agent loop.** There is no Edict check, trust ceiling or approval. Safety rests on the closed `IsSystemTask` enum and operator-only creation.
2. ✅ **Fixed (W0.3/W1.5):** plugin children inherited the full daemon environment. Plugins now get the scrubbed base + grants; MCP and ACP children get `sandbox.IsolatedEnv` (MCP's private, drifted allowlist copy was deleted).
3. **Market install-time verification checks only that the signature is self-consistent** (`VerifyPack(p, "")`). Publisher pinning exists only at sync time via `Source.PubKey`. Installed pack skills **skip the draft/shadow gate** (promoted straight to active) and are **shared**. Agent-tool installs do not journal `market.pack.installed`.
4. **Standing order trust ceilings:**
   - `act_or_ask` with no `max_trust` → `LevelAskFirst`
   - `inform_only` → L0
   - `ask` → L1

   `WithTrustCeiling` only ever tightens (VULN-001).
5. **Untrusted data stays wrapped.** Standing trigger payloads go in an UNTRUSTED envelope (VULN-004). Channel inbound text is data. Voice transcripts are `ObservationUntrusted`.
6. **Workflows cannot launder tool calls.** Every tool/http/code/pipeline node passes `policyHook`. Agent-created workflows are saved **disabled**. The copilot never saves.
7. **System guardians:** `roster.Store.Remove` does not check `System`; the guard is in runtime/controlplane. Guardian reconcile **resets `MaxCostMc` / `MaxDailyMc` to $5 / $10 on every boot**, overwriting any lower ceilings the operator set.

**Correctness / UX**

8. **Self-update is inert and can strand the daemon** (§19.4). `DefaultPublicKeyHex` is empty and no build injects it. The background checker halts the kernel before verifying and never resumes on failure.
9. **Workflow cron/event fires carry no `WakeContext`** (`main.go:1686`), so they are labelled `source:"manual"` in `workflow.started`.
10. **Standing wakes always set `Reason:"event"`**, even for cron and manual fires. Standing has **no overlap guard** and no timezone. Cron ignores `CooldownSec`.
11. **Pulse:**
    - `SetInitiative` is not reachable from the control plane and is not persisted.
    - Pulse never produces `DispAct`.
    - Routing happens before initiative, so a dropped delta never reaches initiative.
    - `.ask` events share the kind `initiative.act`.
    - `doc.go` still lists "act" as deferred.
12. **Self-repair:**
    - `SelfRepairPolicy.EscalateTo` is ignored (the target is Parent, then Owner).
    - Claim does an O(journal) scan under the coordinator lock.
    - The cooldown resets on restart; the attempt cap does not.
13. **Cadence:**
    - Recurring slots missed during downtime collapse into one fire, and intervals drift by up to 10 s.
    - `RunNow` also re-enables a paused schedule.
    - `AGEZT_SCHEDULE` entries get new IDs on every boot.
14. **ACP:**
    - The server cannot cancel a running prompt and ignores `cwd`.
    - The client never answers agent→client requests.
    - The catalog can list registry agents that `acp_agent` cannot select.
15. **MCP:**
    - No `Update` operation.
    - One in-flight call per server; a slow call blocks the others for up to 90 s.
    - The HTTP client timeout is fixed at 90 s.
    - Operator headers can override protocol headers.
    - Tool names truncated to 64 chars can collide silently.
16. **Channel history performance:** `channel.ConversationHistory` does a full journal `Range` **per inbound message**, i.e. O(journal) per message.
17. **Two STT configuration families** coexist: the voice adapter `AGEZT_STT_URL/_KEY` and `kernel/stt` `AGEZT_STT_API_URL/_API_KEY`.
18. **`reranktool`** can panic on a short `scores` slice.
19. **Toolbox `Outdated`** over-flags.

**Structural**

20. **Layering violation:** `kernel/selfrepair` and `kernel/controlplane` import `plugins/tools/overseertool` (§7.6).
21. **Dead code:** `kernel/workflowexec` is an orphaned duplicate of runtime constants. `acpcatalog.ResolveLaunch` has no in-tree caller (allowlisted). `plugin.Config.HostTools` callbacks, `Reload` and progress callbacks are implemented and tested but have no production caller. The daemon keeps no `*plugin.Plugin` handle, so plugins are never `Close()`d at shutdown.
22. **Misplaced doc comments** remain from the god-file splits across cadence, standing, scheduler, skill and acp. Godoc for those symbols is wrong; read the code.
23. **`kernel/scheduler` is the plan DAG executor**, not a time scheduler. Time-based waking lives in `cadence` and in `standing`'s own cron. Despite a stale comment in `standing/runner.go:46-47`, standing cron does not reuse cadence.
