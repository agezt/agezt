# Agezt — Task Breakdown (TASKS.md)

> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> Depends on: IMPLEMENTATION.md and SPEC-01..07
> Granular, checklist-style tasks per phase. IDs are stable references (e.g. `P1-CONDUIT-03`). Each task is sized to be independently reviewable. "DoD" = Definition of Done.

Legend: `[ ]` todo · each phase ends with a demoable slice. Contracts (P0) are frozen before downstream work binds to them.

---

## Phase 0 — Contracts & Kernel Core

### Contracts
- [ ] **P0-PROTO-01** Author `contracts/proto/*.proto` for registration, health, shutdown, `Kernel` service, all 7 plugin services, `Event`/`EventKind`, `UnifiedMessage`. DoD: compiles; lint clean; reviewed against SPEC-01.
- [ ] **P0-PROTO-02** Codegen pipeline → `sdk-go` stubs. DoD: `go generate` reproducible; CI check.
- [ ] **P0-PROTO-03** Resolve SPEC-01 §10 open questions that block payloads (budget unit, attachment threshold, sub-agent result shape). DoD: documented decisions in proto comments.

### Journal
- [ ] **P0-JRNL-01** Canonical event encoding (deterministic bytes) + BLAKE3 hashing. DoD: same event → same hash across runs.
- [ ] **P0-JRNL-02** Segmented JSONL writer + sidecar index; rotation. DoD: append throughput benchmark; crash-safe (fsync policy).
- [ ] **P0-JRNL-03** Hash-chain link + `Verify` (find first break). DoD: tamper test detects alteration/removal.
- [ ] **P0-JRNL-04** Projection framework (fold events → in-memory state) + boot replay. DoD: rebuild from genesis matches incremental state.
- [ ] **P0-JRNL-05** Snapshot write/load; replay snapshot→head. DoD: boot time bounded with snapshots.
- [ ] **P0-JRNL-06** Reversal/compensation API (append inverse, never edit). DoD: `revert <seq>` produces consistent projection.

### Bus
- [ ] **P0-BUS-01** Subject router (hierarchical, wildcard). DoD: pattern-match test matrix.
- [ ] **P0-BUS-02** Pub/sub + request/reply + stream; bounded subscriber channels. DoD: backpressure, no goroutine leaks under load.
- [ ] **P0-BUS-03** Durable-before-publish wiring (append → then publish). DoD: subscribers never see unpersisted events (property test).

### Lifecycle / Supervisor
- [ ] **P0-LIFE-01** Agent actor: goroutine + bounded mailbox + envelope protocol. DoD: agent processes messages; backpressure on full.
- [ ] **P0-LIFE-02** State recovery: fold journal by `correlation_id` to reconstruct agent state. DoD: restarted agent resumes mid-task in test.
- [ ] **P0-LIFE-03** Supervisor restart policy (never/on_crash/always, backoff+jitter, quarantine). DoD: crash → restart within policy; exceed window → quarantine + high-salience event.
- [ ] **P0-LIFE-04** Spawn/Suspend/Resume/Kill + lifecycle events. DoD: each emits correct `EVT_AGENT_*`.

### Plugin Host
- [ ] **P0-HOST-01** Manifest discovery + parse (`plugin.yaml`). DoD: scans dir; validates schema.
- [ ] **P0-HOST-02** Subprocess launch with bootstrap env + one-time token. DoD: token rejection test.
- [ ] **P0-HOST-03** Handshake (Register/RegisterResponse) + health ping/pong loop. DoD: missed pongs → unhealthy → policy.
- [ ] **P0-HOST-04** Capability routing table. DoD: resolve by capability+attributes; multiple-match → policy/pin.
- [ ] **P0-HOST-05** Echo test plugin (Go SDK). DoD: registers, echoes, survives kernel restart re-launch.

### Control plane
- [ ] **P0-CTRL-01** Unix socket server + command protocol. DoD: CLI connects locally.
- [ ] **P0-CTRL-02** `agt halt`/`resume` (suspend all, persist, freeze scheduler). DoD: nothing runs after halt; resume restores.
- [ ] **P0-CTRL-03** `agt why <event>` (walk causation chain). DoD: renders provenance for a seeded chain.
- [ ] **P0-CTRL-04** `agt attach` (stream subject pattern). DoD: live events in terminal.
- [ ] **P0-CTRL-05** `agt journal {replay|verify|revert}`. DoD: each works on a seeded journal.

**Phase 0 demo gate:** spawn agent → emit/replay events → verify chain → halt/resume → attach. All green.

---

## Phase 1 — Reasoning & Tools (single task end-to-end)

### Conduit + Governor
- [ ] **P1-CONDUIT-01** Provider registry + capability advertisement consumption. DoD: list models/modalities per provider.
- [ ] **P1-CONDUIT-02** `Governor.Route`: subscription-first → cost → latency; budget + rate-limit check. DoD: routing decision test matrix.
- [ ] **P1-CONDUIT-03** Fallback chain (Anthropic→OpenRouter→Ollama). DoD: limit breach falls through; recorded.
- [ ] **P1-CONDUIT-04** `EVT_BUDGET_CONSUMED` ledger projection + ceilings. DoD: per-task/day spend tracked; breach → stop+surface.

### Providers
- [ ] **P1-PROV-01** `provider-anthropic` (Complete stream, ListModels, ReportLimits, OAuth/subscription + api-key). DoD: streams tokens; reports usage.
- [ ] **P1-PROV-02** `provider-ollama` (local; Complete + Embed). DoD: works fully offline as fallback floor.

### Scheduler + Planner
- [ ] **P1-SCHED-01** DAG model + topological executor + bounded worker pool. DoD: parallel branches run concurrently; path-scoped serialization.
- [ ] **P1-SCHED-02** Node types: tool/llm/loop/gate. DoD: each emits `EVT_NODE_*`; loop enforces max-iter + budget.
- [ ] **P1-SCHED-03** Retry/compensation per node. DoD: transient retry; terminal fail surfaces.
- [ ] **P1-PLAN-01** Planner: intent → DAG using capability inventory. DoD: `EVT_PLAN_PROPOSED`; missing-capability path.
- [ ] **P1-PLAN-02** Plan approval gate (front gate-node for high-trust-cost plans). DoD: escalate → approve → execute.

### Tools
- [ ] **P1-TOOL-01** `tool-shell` (sandboxed, persistent-shell option). DoD: runs in namespace profile; cancel works.
- [ ] **P1-TOOL-02** `tool-file` (read/write/search/patch, scoped paths). DoD: cannot escape workspace.
- [ ] **P1-TOOL-03** `tool-http` (fetch/POST, Edict domain policy). DoD: deny outside allowlist.
- [ ] **P1-TOOL-04** `tool-browser` (Playwright/CDP: navigate/read/act/screenshot/extract). DoD: runs in container; sensitive-domain escalate.

### Edict + Warden v1
- [ ] **P1-EDICT-01** Policy parser + first-match evaluation + `EVT_POLICY_DECISION`. DoD: rule matrix test.
- [ ] **P1-EDICT-02** Trust ladder (per-capability L0–L4) + hard-deny immutable block. DoD: hard-deny never overridable.
- [ ] **P1-EDICT-03** Approval flow (request→route→grant/deny→resume, scoped, timeout=deny). DoD: end-to-end approve/deny.
- [ ] **P1-WARD-01** Namespace+cgroups+seccomp profile. DoD: fs/net/resource limits enforced; downgrade journaled on non-Linux.

### CLI
- [ ] **P1-CLI-01** `agt run "<intent>"` end-to-end. DoD: plan→(approve)→execute→result.
- [ ] **P1-CLI-02** Zero-config first run (embedded DB + local-model autodetect). DoD: works with nothing configured.
- [ ] **P1-TUI-01** Minimal TUI: intent + live DAG/journal split. DoD: watch a run live.

**Phase 1 demo gate:** `agt run "fetch X, summarize, write report.md"` with real policy + budget + sandbox.

---

## Phase 2 — Memory, World Model & Forge

- [ ] **P2-MEM-01** Memory record model + tiers (working/episodic/semantic/world). DoD: write/read each tier.
- [ ] **P2-MEM-02** Hybrid retrieval (semantic+keyword+graph) + rank (relevance×confidence×recency) + tombstone filter. DoD: ranked results with provenance.
- [ ] **P2-MEM-03** Embedded vector index (or delegate to Flint Vector). DoD: ANN recall benchmark.
- [ ] **P2-MEM-04** `memory-flintvector` plugin. DoD: parity with embedded via contract.
- [ ] **P2-WORLD-01** World-model graph (entities/relations/attributes) + journaled mutations. DoD: resolve "the portfolio"; diff across time.
- [ ] **P2-WORLD-02** World-model queries for Planner/Salience/Briefing/Initiative. DoD: each consumer gets needed context.
- [ ] **P2-FORGE-01** Skill model + content-addressed bodies + retrieval/activation. DoD: relevant skills injected into context.
- [ ] **P2-FORGE-02** Skill lifecycle state machine + transition events. DoD: draft→shadow→active→quarantine→revert all journaled.
- [ ] **P2-FORGE-03** Shadow-test harness (compare hypothetical vs actual without side effects). DoD: only-promote-if-helpful gate.
- [ ] **P2-FORGE-04** Consolidation/prune pass (merge overlaps, archive stale, protect pinned). DoD: lineage preserved.
- [ ] **P2-CMP-01** Context compression (protect first/last, summarize middle, sanitize orphan tool pairs). DoD: stays under context budget.

**Phase 2 demo gate:** complex task → skill created → shadow-tested → promoted; `agt skill history/revert`.

---

## Phase 3 — Pulse (proactive heart)

- [ ] **P3-PULSE-01** Heartbeat resident agent (fixed cadence first). DoD: `EVT_PULSE_TICK` emitted.
- [ ] **P3-OBS-01** Observer host + interface; `Delta` model. DoD: observers register; emit deltas not raw data.
- [ ] **P3-OBS-02** Observer: repo/CI (status/issues/advisories). DoD: green→red emits one delta.
- [ ] **P3-OBS-03** Observer: system-health (disk/mem/service/cert). DoD: threshold breach → delta.
- [ ] **P3-SAL-01** Salience scorer (rules + cheap LLM) → score+reason+disposition. DoD: `EVT_SALIENCE_SCORED`.
- [ ] **P3-SAL-02** Novelty suppression (seen-cache; world-model later). DoD: "already told him" suppressed.
- [ ] **P3-SAL-03** Quiet/Balanced/Chatty dial + per-category overrides. DoD: dial changes what surfaces.
- [ ] **P3-INIT-01** Initiative solve-vs-ask (reads trust ladder + budget). DoD: reversible+in-ladder→act; else ask/inform; `EVT_INITIATIVE_TAKEN`.
- [ ] **P3-INIT-02** Safety guards: action rate-limit, no-repeat, reversibility requirement. DoD: thrash/runaway prevented.
- [ ] **P3-BRIEF-01** Briefing composer (disposition→delivery, batching, tone, dedupe, quiet hours). DoD: `EVT_BRIEFING_SENT`; coalesced.
- [ ] **P3-BRIEF-02** Feedback hooks (dismiss/snooze/less-of-this). DoD: feeds reflection later.
- [ ] **P3-CHRON-01** Chronos triggers (time/event/condition/webhook) + persistence (reload from journal). DoD: jobs survive restart.
- [ ] **P3-STAND-01** Standing orders (observers+overrides+initiative scope, Chronos-kept). DoD: a named standing order stays alive.

**Phase 3 demo gate:** unprompted detection of broken CI → briefed (to CLI/log in P3); `agt why` explains; `agt halt` stops.

---

## Phase 4 — Channels & Unified Inbox

- [ ] **P4-CHAN-01** `channel-telegram` duplex (inbound→`UnifiedMessage`, outbound, signals/buttons). DoD: command in → reply out; inline approve.
- [ ] **P4-CHAN-02..11** Channels: email, whatsapp, discord, slack, signal, sms, matrix, teams, homeassistant, webhook. DoD each: normalize to `UnifiedMessage`; crash-isolated.
- [ ] **P4-INBOX-01** Unified Inbox (grouped by correlation; reply routes back; handoff lineage). DoD: multi-channel timeline.
- [ ] **P4-INBOX-02** Approvals + briefings streams in Inbox. DoD: approve/deny + dismiss/snooze.
- [ ] **P4-PULSE-02** Pulse briefs to Telegram (closes the Jarvis loop). DoD: "CI broke" arrives unprompted on Telegram.

**Phase 4 demo gate:** Telegram in→agent→out; unprompted Telegram brief; approve via buttons.

---

## Phase 5 — Web UI: Flow Studio + Live Monitor

- [ ] **P5-GW-01** Gateway (WS/gRPC-Web, auth, static host). DoD: UI connects; events stream.
- [ ] **P5-SDK-01** `agezt-sdk-ts` (typed events/commands). DoD: generated; used by UI.
- [ ] **P5-FLOW-01** Flow Studio design mode (drag/drop, plugin picker, schema-validated wiring). DoD: build a valid DAG.
- [ ] **P5-FLOW-02** Run mode (live node highlight from events, stream output/cost/timing). DoD: watch it light up.
- [ ] **P5-FLOW-03** Replay mode (scrub journal slice, recorded outputs). DoD: deterministic step-through.
- [ ] **P5-MON-01** Live Monitor (agents/pulse/cost/traces/health + HALT). DoD: all panels event-driven.
- [ ] **P5-MEMUI-01** Memory Explorer (knowledge/world-graph/skills+revert/provenance/reflection). DoD: revert a skill from UI.
- [ ] **P5-SET-01** Settings panels (trust ladder, Edict tester, providers/Governor, channels, plugins, salience dial). DoD: policy simulate works.

**Phase 5 demo gate:** build DAG visually → run live → inspect traces/cost → revert skill in UI.

---

## Phase 6 — Warden hardening, multi-agent, simulation

- [ ] **P6-WARD-02** Container profile (OCI, mounted scope, no host net default). DoD: untrusted tool isolated.
- [ ] **P6-WARD-03** microVM profile (optional component). DoD: highest-risk execution isolated.
- [ ] **P6-WARD-04** Egress allow-listing + seccomp hardening. DoD: default-deny egress enforced.
- [ ] **P6-MULTI-01** agent-node spawning sub-agents at scale (parallel workstreams). DoD: N parallel sub-agents return summaries.
- [ ] **P6-CODE-01..03** `coding-claudecode`, `coding-codex`, `coding-aider` (coding-node; merge/force-push escalate; worktree isolation). DoD: delegate task → diff streamed → PR (not merge).
- [ ] **P6-SIM-01** Dry-run/simulation for risky DAGs. DoD: "what would happen" preview + approve.

**Phase 6 demo gate:** delegate "fix CI" to Claude Code in a sandbox → review diff → open PR, one DAG.

---

## Phase 7 — Tunnels, full SDK, ambient, OpenAI-compat API

- [ ] **P7-TUN-01..03** `tunnel-cloudflare`, `tunnel-tailscale`, `tunnel-wirerift` (Up/Down/Status; open=escalate). DoD: expose UI via tunnel.
- [ ] **P7-API-01** OpenAI-compatible `/v1/chat/completions` + `/v1/responses` (+ session header). DoD: external OpenAI client drives Agezt.
- [ ] **P7-API-02** Native REST/gRPC + webhooks in/out (all through Edict + journal). DoD: not a governance backdoor.
- [ ] **P7-SDK-02..04** `sdk-ts`/`sdk-py`/`sdk-rust` + conformance pass. DoD: same contract tests green.
- [ ] **P7-SCAF-01** `create-agezt-plugin` scaffolder. DoD: 20-line plugin works.
- [ ] **P7-MCP-01** `mcp-bridge` (any MCP server → Tool capabilities). DoD: external MCP tools callable.
- [ ] **P7-AMB-01** Voice (wake-word + local Whisper STT + TTS). DoD: talk → act → spoken reply.
- [ ] **P7-AMB-02** Tray/menu-bar app (presence, quick intent, briefings, halt). DoD: desktop presence.
- [ ] **P7-AMB-03** Mobile push (briefings/approvals, reply/approve). DoD: approve from lock screen.
- [ ] **P7-AMB-04** Email-native reply→intent. DoD: reply to brief → new intent.

**Phase 7 demo gate:** tunnel-exposed UI + OpenAI-compat client + voice all driving the same kernel.

---

## Phase 8 — Reflection, marketplace, polish

- [ ] **P8-REF-01** Reflection loop (examine outcomes/feedback/cost; recalibrate salience/initiative within ladder; propose trust changes). DoD: `agt reflect show`; auto-applies only safe tuning.
- [ ] **P8-MKT-01** Marketplace (signed, content-addressed plugins/skills/workflows/standing-orders; `agt plugin add <ref>` resolves local/url/marketplace). DoD: install verifies signature/hash; surfaces required trust levels.
- [ ] **P8-POL-01** Installers, `agt doctor`, skins, i18n (EN default; TR-ready), docs site (TBD domain). DoD: clean install on Linux/macOS/WSL.

---

## Phase 9 — Mesh & migration

- [ ] **P9-MESH-01** Gossip/SWIM node discovery over mesh-ready contracts. DoD: nodes find each other.
- [ ] **P9-MESH-02** Federated bus + agent migration (ship correlation-scoped event slice). DoD: agent migrates node→node.
- [ ] **P9-MT-01** Multi-tenant mode (per-tenant world-model + isolation). DoD: two tenants isolated on one instance.
- [ ] **P9-MIG-01** `agt migrate openclaw|hermes` (import settings/memories/skills). DoD: a real OpenClaw/Hermes config imports.

---

## Cross-cutting (every phase)

- [ ] **X-TEST** Unit + contract-conformance + replay/property + integration + security(injection/sandbox/redaction) + soak/chaos.
- [ ] **X-DOCS** Keep SPEC ↔ code in sync; per-module READMEs.
- [ ] **X-DEPS** `DEPENDENCIES.md` justifies every external dep; CI fails on unjustified additions.
- [ ] **X-SEC** Secure-defaults checklist (SPEC-06 §9) verified each release.

---

*Next: BRANDING.md (name rationale, naming system for components, tone, color/typography direction, taglines), then README.md, then PROMPT.md (the single-shot build prompt for Claude Code).*
