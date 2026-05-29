# Agezt — Master Index & Document Map (INDEX.md)

> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> The navigation root for the full Agezt design. Read this first. It maps every document, consolidates the event schema additions, and presents the unified phase plan.

---

## 0. What Agezt is (one paragraph)

Agezt is an **agentic operating system**: a stdlib-first Go core (multiple static, multi-arch binaries) that turns intent into auditable, reversible action via a deterministic DAG with bounded LLM-loop nodes; runs autonomous agents under a trust ladder and policy engine; proactively watches your world (Pulse) and tells you what matters (salience) without spam; extends infinitely through out-of-process, polyglot plugins (channels, providers, tools, coding-agents, memory, storage, tunnels, widgets); remembers via tiered memory + a world model; improves itself via a governed, reversible skill pipeline (Forge); and is visually programmable (React Flow). Everything is an event in a tamper-evident journal — so every action is explainable (`agt why`), reproducible, and revertible. **Autonomous, under your authority.**

---

## 1. Document map

### Build entry (read these first, in this order)
- **BUILD-GUIDE.md** — **START HERE.** The single entry point for implementing Agezt: what to read, what's binding, what to ignore, what to produce, in what order.
- **DECISIONS.md** — the supreme authority; all frozen technical decisions (esp. B0–B0d). Wins over any spec conflict.
- **POLICY.md** — dependency, packaging/binary, versioning, license policy.
- **agezt-contract.jsonc** — the contract source of truth (JSON-RPC 2.0 + JSON Schema).
- **STRUCTURE.md** — the exact repository layout to produce.
- **ROADMAP.md** — build order: M0.5 (minimal core) → MVP → growth, each with a success test.
- **LICENSE** — MIT.

> **Ignore `_ARCHIVE/`** — superseded files (old gRPC `agezt.proto`, deprecated proto, old vision). Kept for history only; never build from them.

### Policy & vision
- **POLICY.md** — dependency, packaging/binary, versioning, license policy. *Authoritative; other docs defer here.*
- **AGEZT-VISION-MASTER.md** — the full vision, two hearts, competitive analysis, MVP cut.

### Specifications (the contract of the system)
- **SPEC-01-CONTRACTS** — plugin gRPC contracts + the canonical event schema (the dependency root).
- **SPEC-02-KERNEL** — the six kernel responsibilities, agent runtime, scheduler, Governor, control plane.
- **SPEC-03-PULSE** — the proactive heart (Observers → Salience → Initiative → Briefing).
- **SPEC-04-PLUGINS** — the seven plugin interfaces in depth + Chronos + MCP bridge.
- **SPEC-05-MEMORY** — memory tiers, world model, skills, Forge, reflection.
- **SPEC-06-SECURITY** — threat model, Warden sandbox, Edict, secrets, autonomous-op safety.
- **SPEC-07-UI** — surfaces: Flow Studio, Unified Inbox, Live Monitor, Memory Explorer, gateway/API, ambient, **+conversation surface (chat history, tool-call debug, context inspector)**.
- **SPEC-08-OPERABILITY** — updates, migrations, plugin contributions, changelog/version tracking, GHCR distribution.
- **SPEC-09-IDENTITY** — ULID/content-address identity, export/import bundles, granular export, backup, point-in-time restore.
- **SPEC-10-LLM-CONTEXT** — model routing, modern-capability leverage, intelligent context management, FinOps, self-improvement leverage.
- **SPEC-11-DEPLOYMENT** — Docker (substrate + sandbox), GHCR/CI-CD, multi-arch, runtime profiles, OTel.
- **SPEC-12-WIDGETS** — in-conversation interactive widgets + widget SDK (sandboxed, data-not-code).
- **SPEC-13-CAPABILITY-ARMY** — ecosystem interop (MCP/agentskills/migration), first-party catalog, self-growth.
- **SPEC-14-RESILIENCE-OPS** — resilience/saga, human-in-the-loop flow, agent eval, RBAC, onboarding, notifications/escalation, secrets lifecycle, i18n/locale, operator observability, project realities.
- **SPEC-15-PROVIDER-ECOSYSTEM** — provider/model catalog sync (models.dev-class), credential import, tool-calling normalization across OpenAI/OpenAI-compat/Anthropic/Gemini/others, Agent Client Protocol (ACP) server + client.
- **SPEC-16-DETAILS** — concrete implementation-level specs: API surface (OpenAI-compat + native REST/gRPC + webhooks), test strategy, full config.yaml reference, standing-order DSL, onboarding flow.

### Decisions, contract & license
- **DECISIONS.md** — all open questions closed and frozen for v1. *Wins over any spec's open-question conflict.*
- **agezt.proto** — the complete, compiling contract (messages + Kernel/PluginBase/7-plugin gRPC services + consolidated event kinds). The dependency root in code form.
- **LICENSE** — MIT.
- **ROADMAP.md** — path from docs to product: foundation → MVP (v0.1) → growth (M2–M8).

### Build documents
- **IMPLEMENTATION.md** — Go architecture, repo layout, tech choices, phase-by-phase build.
- **TASKS.md** — granular, ID'd task breakdown with demo gates.
- **BRANDING.md** — name, component naming, voice, visual direction.
- **README.md** — public-facing project overview.
- **PROMPT.md** — single-shot build prompt for a coding agent.
- **INDEX.md** — this document.

> **Integration status:** The canonical event enum now lives in `agezt.proto` (compiles; includes all kinds through SPEC-15). Remaining housekeeping before "frozen v1.0": (a) fold the prose of SPEC-08..16's additions back into the original SPEC-01/06/07 narrative sections; (b) extend TASKS.md with ID'd tasks + demo gates for SPEC-08..16 (currently only placed in the phase table §3). Neither blocks starting the MVP build — the contract and DECISIONS are authoritative.

---

## 2. Consolidated event-kind additions

Beyond SPEC-01 §7's original kinds, the later specs introduce these (to be merged into the canonical enum, preserving stable numbering):

- **Operability (SPEC-08):** `EVT_PLUGIN_INSTALLED`, `EVT_PLUGIN_UPDATED`, `EVT_PLUGIN_REMOVED`, `EVT_PLUGIN_ENABLED`, `EVT_PLUGIN_DISABLED`, `EVT_MIGRATION_APPLIED`, `EVT_MIGRATION_REVERTED`, `EVT_CORE_UPDATED`, `EVT_CONTRIBUTION_MOUNTED`, `EVT_CONTRIBUTION_UNMOUNTED`.
- **Identity/backup (SPEC-09):** `EVT_EXPORTED`, `EVT_IMPORTED`, `EVT_BACKUP_CREATED`, `EVT_RESTORED`, `EVT_RESTORE_POINT_CREATED`.
- **Widgets (SPEC-12):** `EVT_WIDGET_ACTION`.
- **HITL (SPEC-14):** `EVT_CLARIFY_REQUESTED`, `EVT_CLARIFY_ANSWERED`, `EVT_TASK_SUSPENDED_FOR_INPUT`, `EVT_TASK_STEERED`.
- **Resilience (SPEC-14):** `EVT_CHECKPOINT_CREATED`, `EVT_COMPENSATION_RUN`, `EVT_DEGRADED_MODE_ENTERED`, `EVT_DEGRADED_MODE_EXITED`.

All carry standard identity/routing/provenance fields (SPEC-01 §7); all are subject to the hash chain and `agt why`.

---

## 3. Unified phase plan (full project)

Each phase ends in a demoable slice. Contracts (P0) freeze first. New concerns from SPEC-08..14 are folded into their natural phases below.

| Phase | Theme | Adds from new specs |
|---|---|---|
| **0 Kernel core** | journal, bus, supervisor, plugin host, control plane, `.proto` freeze, go-SDK | ULID/content-address identity (SPEC-09); `agt halt/why/attach` |
| **1 Reasoning & tools** | Governor+providers, scheduler/planner, shell/file/http/browser, Edict v1, Warden namespace, CLI | model routing v1 + context budgeting (SPEC-10); migration runner + plugin contributions (SPEC-08); static multi-arch + scratch image + GHCR CI (SPEC-11); retry/checkpoint + graceful degradation + `agt doctor` (SPEC-14) |
| **2 Memory & Forge** | tiers, world model, retrieval, skill lifecycle, shadow-test | context compression (SPEC-10); export/import + backup (SPEC-09); changelog/version tracking (SPEC-08) |
| **3 Pulse** | heartbeat, observers, salience, initiative, briefing, Chronos, standing orders | timezone/locale correctness (SPEC-14); point-in-time restore (SPEC-09); clarify + suspend/resume (SPEC-14); starter standing order (SPEC-14) |
| **4 Channels & Inbox** | Telegram-first then all channels, Unified Inbox, Pulse→Telegram | notifications/preferences (SPEC-14); agentskills/ClawHub adapter start (SPEC-13) |
| **5 Web UI** | Flow Studio, Live Monitor, Memory Explorer, gateway, sdk-ts | conversation surface + tool-call debug + context inspector (SPEC-07/10); first-party widgets sandboxed (SPEC-12); FinOps + eval harness + guided onboarding + interruption/steering (SPEC-10/14) |
| **6 Hardening & coding agents** | container/microvm, multi-agent, coding-nodes, simulation | Docker sandbox modes + sandbox image family (SPEC-11/06); saga/compensation (SPEC-14); single-instance RBAC (SPEC-14); coding-authored plugins (SPEC-13) |
| **7 Tunnels, SDKs, ambient, API** | tunnels, OpenAI-compat API, ts/py/rust SDKs, scaffolder, MCP bridge, voice/tray/mobile/email | widget SDK (SPEC-12); subscription proxy (SPEC-13); secrets rotation/refresh (SPEC-14); k8s/OTel (SPEC-11); core/plugin update mechanism (SPEC-08) |
| **8 Reflection & marketplace** | reflection loop, marketplace, installers, skins, i18n | eval→reflection integration (SPEC-14); marketplace widgets/verticals (SPEC-12/13); escalation chains + vault + UI i18n (SPEC-14); license/community/docs (SPEC-14 §10) |
| **9 Mesh & migration** | gossip/SWIM mesh, agent migration, multi-tenant, `agt migrate openclaw\|hermes` | mesh rides on mesh-ready contracts (SPEC-02); migration via import pipeline (SPEC-09/13) |

---

## 4. The competitive thesis, restated

Agezt does everything OpenClaw and Hermes do — multi-channel gateway, multi-provider, skills/self-improvement, memory, cron, MCP, coding delegation, browser use — and can literally run their capabilities (MCP servers, agentskills.io skills) via interop. It then beats them on the axes they can't easily match:

1. **Auditable autonomy** — deterministic visible plans + event-sourced, hash-chained, reversible everything.
2. **Proactive with judgment** — Pulse + salience (notices what matters, not everything).
3. **Under your authority** — trust ladder + policy + sandbox + one-command halt.
4. **Operationally mature** — migrations, updates, export/import, point-in-time restore, RBAC, eval, FinOps.
5. **Multiple lean binaries, infinite reach** — stdlib-first core, out-of-process polyglot plugins, any LLM/subscription, any channel, runs on a $5 VPS to a cluster.
6. **Visually programmable & richly interactive** — Flow Studio + widget-decorated conversations.

---

## 5. The one honest caveat

This is a complete, coherent **design** — not yet a running system. Its success depends less on the breadth of this plan and more on shipping a working **core** and growing outward phase by phase, honoring the demo gates and resisting scope creep. The plan's job is to make that build unambiguous; the build's job is to make the plan real. Begin at Phase 0, task `P0-PROTO-01`.

---

*End of index. The suite — POLICY + SPEC-01..14 + IMPLEMENTATION + TASKS + BRANDING + README + PROMPT + this INDEX — constitutes the full project plan.*
