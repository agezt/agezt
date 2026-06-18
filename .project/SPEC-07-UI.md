# Agezt — UI & Surfaces Specification (SPEC-07)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> Depends on: SPEC-01 (events/contracts), SPEC-02 (control plane), SPEC-03 (Pulse)
> Defines every human-facing surface: the Web UI (Flow Studio, Unified Inbox, Live Monitor, Memory Explorer), the CLI/TUI, the gateway/API, and ambient surfaces. The contract: **maximum power inside, maximum simplicity outside.**

---

## 0. Surface philosophy

- **One event truth, many views.** Every surface is a *projection of the journal/bus* (SPEC-02 §7). The UI never holds authoritative state; it subscribes and renders. This guarantees consistency across CLI, Web, and remote SDKs.
- **Progressive disclosure.** A first-time user sees one input box. A power user can open the DAG, the policy log, the trust ladder, the world model. Complexity is available, never imposed.
- **Live by default.** Surfaces stream (`agt attach` / WebSocket subscribe). You watch agents think in real time, not refresh.
- **Read–observe–act parity.** Anything observable is explainable (`why`) and, where permitted, actionable (approve/halt) from the same surface.

---

## 1. Transport for surfaces

- **CLI/TUI:** Unix domain socket → kernel control plane (fastest, local).
- **Web UI / remote SDK:** WebSocket + gRPC-Web to the gateway; gateway authenticates and proxies to the kernel. Live event streams over WS/SSE; commands over gRPC-Web.
- **Auth:** local socket = OS user trust; remote = token/OAuth at the gateway; public exposure only via an explicit Tunnel (escalate, SPEC-06).
- All surfaces consume the **same event subjects** (SPEC-01 §8), so a feature added to the bus appears to every surface uniformly.

---

## 2. Web UI — overview

Stack: **React 19 + Tailwind 4 + shadcn/ui + React Flow**, TypeScript throughout, generated `agezt-sdk-ts` for typed event/command access. Dark-first, distinctive (not generic-AI), built around the four primary surfaces below plus settings/governance panels.

Layout: a persistent left rail (surfaces + active agents), a main canvas (surface-specific), and a right context drawer (details / `why` / approvals). A global command palette (`⌘K`) runs any `agt` command.

---

## 3. Surface 1 — Flow Studio (React Flow) · PRIORITY 1

The signature surface. Visually program and observe agentic workflows.

### 3.1 Modes
- **Design mode:** drag-and-drop a DAG. Nodes = node types (tool/llm/loop/gate/agent/coding) bound to concrete plugins via a picker. Edges = data/control flow. Validates against plugin schemas (`Describe`) live — incompatible wiring is flagged before run.
- **Run mode:** the same graph **lights up live** as it executes. Each node shows state (queued/running/done/failed), streamed output, token/cost, and timing — all driven by `task.*` and `EVT_NODE_*` events. This is the journal made visible.
- **Replay mode:** scrub a past task's journal slice; step through node-by-node with recorded LLM outputs (deterministic replay, SPEC-02 §4.4).

### 3.2 What you can build here
- One-off task graphs.
- Reusable **workflow templates** (saved, versioned, shareable to the marketplace).
- **Standing wake rules** authored visually: cron/event/webhook triggers and constraints bound to an agent, workflow, tool, or system task. The UI must not store agent identity or task instructions inside the schedule rule.

### 3.3 Node inspector (right drawer)
Per node: bound plugin + isolation profile, input/output schema, live stream, policy decisions that applied, cost, and a `why` button (provenance chain). Approvals for `gate-node`s surface inline.

### 3.4 Design intent
React Flow is the centerpiece per the project vision ("decorate everything with React Flow"). The aesthetic: a dark technical canvas, precise typography, restrained accent color for live/active state, animated edge flow during execution. Not a toy — a control room.

---

## 4. Surface 2 — Unified Inbox · PRIORITY 2

All channels as one simplified stream.

### 4.1 Model
Every channel normalizes to `UnifiedMessage` (SPEC-04 §1.3), so the Inbox shows Telegram, Discord, Slack, WhatsApp, Email, SMS, etc. in one timeline, grouped by conversation/`correlation_id`, regardless of origin platform.

### 4.2 Capabilities
- Read inbound across all channels; reply from one place (outbound routed back to the originating channel).
- See which agent handled/produced a message and jump to its task in Flow Studio (shared `correlation_id`).
- Cross-channel handoff visible: a thread that moved Telegram→CLI→Email is one continuous lineage.
- Pending approvals from any channel collect here too (approve/deny inline).
- Briefings (Pulse output) appear as a distinct, dismissible stream; dismiss/snooze/"less of this" feeds the reflection loop (SPEC-05 §6).

### 4.3 Simplification, not flattening
Platform-specific richness is preserved in `platform_meta` and accessible on demand, but the default view is one calm, unified stream — the "simple outside" contract.

---

## 5. Surface 3 — Live Monitor · PRIORITY 3

The operational dashboard for a living, autonomous system.

### 5.1 Panels
- **Agents:** every alive/sleeping/working/repairing/retired agent, its kind (system/user/subagent), mailbox depth, current activity, lineage, owner/parent, health, retry/doctor state, tasklist, permissions, and lifecycle controls (wake/pause/resume/retire/revive/remove, policy-gated).
- **Pulse:** heartbeat cadence, recent observer deltas, salience scores with reasons, initiative decisions (acted/asked/informed), briefings sent. The proactive heart, visible.
- **Cost & limits (Governor):** spend by provider/model, budget burn-down, rate-limit posture, fallback events. Subscription vs api-key usage split.
- **Traces:** per-task timeline (nodes, latency, token flow) — the latency budget made concrete.
- **Health:** plugin health, storage status, anomaly indicators, journal chain-verify status.
- **Halt control:** prominent `HALT` affordance (mirrors `agt halt`); shows if auto-halt anomaly guards tripped.

### 5.2 Driven entirely by events
Subscribes to `pulse.>`, `budget.>`, `agent.*.>`, `task.*.>`, `system.>`. No separate telemetry pipeline — the journal is the telemetry.

---

## 6. Surface 4 — Memory Explorer · PRIORITY 4

Inspect and govern what the system knows and how it improves.

### 6.1 Views
- **Knowledge:** browse/search memory tiers (working/episodic/semantic/world-model). Hybrid search (semantic + keyword + graph).
- **World model:** the context graph as an interactive node-link diagram (entities, relations, weights). See what "the portfolio" resolves to; edit/correct entities and preferences.
- **Skills:** the skill library with status (draft/shadow/active/quarantined/archived), metrics (uses/success/fail), and **version history with a revert button** (SPEC-05 §5).
- **Provenance:** for any belief/skill, `why` shows the source event(s) and when it was learned.
- **Reflection:** latest reflection report and proposed adjustments (approve trust changes here).

### 6.2 Governance affordances
- Forget (tombstone) a fact; revert a skill version; pin a skill; correct a world-model entity. All actions are journaled events (auditable, themselves revertible).

---

## 7. Settings & governance panels (supporting)

- **Trust ladder editor:** per-capability L0–L4 with plain-language explanations; raising autonomy is the only place it can go up (reflection can lower autonomously). Shows the impact ("at L3, shell can run reversible commands without asking").
- **Policy (Edict) editor:** YAML with a live tester ("simulate: browser → bank.com" → shows decision). Hard limits shown as read-only.
- **Providers & Governor:** configure providers, auth mode (subscription/api-key/local), priority, budgets, fallback chain.
- **Channels:** connect/configure each channel; per-channel tool/permission scope; quiet hours; preferred briefing channel.
- **Plugins:** installed plugins, capabilities, isolation, health; add/disable; signature/trust status.
- **Salience dial:** the single Quiet/Balanced/Chatty knob + optional per-category overrides.

---

## 8. CLI & TUI — `agt`

The CLI is first-class, not a fallback. Power users live here.

### 8.1 Core commands
```
agt run "<intent>"              # reactive: plan → (approve) → execute
agt flow edit [name]            # open Flow Studio for a workflow (or TUI graph)
agt agent {list|spawn|kill|attach <id>}
agt pulse {status|pause|resume}
agt why <event_id>              # provenance chain
agt halt | agt resume           # dead-man's switch
agt journal {replay|verify|revert <seq>}
agt memory {search|forget}
agt skill {list|history <id>|revert <id>|pin <id>}
agt reflect show
agt plugin {list|add <ref>|disable <id>}
agt channel {list|status|connect}
agt provider {list|use|limits}
agt policy {test|reload}
agt tunnel {up <kind>|down|status}
agt standing {list|add|pause <id>}
agt migrate {openclaw|hermes}
agt doctor                      # diagnose config/providers/plugins
```

### 8.2 TUI (Bubble Tea)
Interactive mode: a live split view — conversation/intent on one side, a live DAG + journal stream on the other; approvals inline; `⌘`-style keybindings. Themable (skins). Zero-config first run: embedded DB + local-model auto-detect, so `agt` works immediately with nothing configured.

---

## 9. Gateway & API

### 9.1 Gateway
A kernel-fronting process that: terminates remote connections (WS/gRPC-Web/HTTP), authenticates, and proxies to the control plane. Hosts the Web UI static assets. Channels and webhooks ingress here. Exposable externally only via a Tunnel (escalate).

### 9.2 Public API (OpenAI-compatible + native)
- **OpenAI-compatible** `/v1/chat/completions` + `/v1/responses` so existing tools/frontends can drive Agezt as if it were a model endpoint (an intent in, streamed result out). Session continuity via a header (correlation id).
- **Native REST/gRPC** for full control: submit intents, manage agents, query journal/memory, manage typed schedules/standing wake rules, stream events.
- **Webhooks** in (trigger agents) and out (notify external systems).
- All API actions pass through Edict and are journaled — the API is not a backdoor around governance.

---

## 10. Ambient surfaces (Jarvis's "bodies")

Same core, different embodiments — each is a thin client over the same event/command transport:
- **Voice:** wake-word → STT (local Whisper) → intent → agent → TTS reply. Push-to-talk in CLI; voice notes in messaging channels. The most "Jarvis" surface.
- **Tray / menu-bar app:** always-present desktop presence; quick intent, latest briefings, halt button. (Builds on the team's MultiPilot/desktop experience.)
- **Mobile:** push notifications for briefings/approvals; reply/approve from the lock screen; intents on the go.
- **Email-native:** reply to a briefing email in natural language → it becomes an intent.
Handoff across all of them is automatic via `correlation_id` lineage (start by voice, continue in Web).

---

## 11. Accessibility, i18n, theming

- WCAG-minded: keyboard-navigable, focus states, sufficient contrast (dark-first but a light theme ships).
- i18n-ready (English default; the user works in Turkish + English — copy is externalized for localization).
- Theming via design tokens (CSS variables); CLI skins; UI light/dark.

---

## 12. Open questions

1. Flow Studio authoring depth: how much logic (conditionals/loops) is visual vs delegated to a `loop-node`'s prompt?
2. Standing-order authoring: visual-only, DSL-only, or both (visual compiles to DSL)?
3. Web UI offline/local mode vs always-connected to a running kernel.
4. Voice always-listening privacy posture and default (likely opt-in, local-only).
5. How much of the world-model graph editing is safe to expose directly vs propose-and-approve.

---

*SPEC suite complete: 01 Contracts · 02 Kernel · 03 Pulse · 04 Plugins · 05 Memory · 06 Security · 07 UI. Next: the build documents — IMPLEMENTATION.md (Go package architecture, module boundaries, tech choices, phase-by-phase build), TASKS.md (granular task breakdown), BRANDING.md, README.md, PROMPT.md.*
