# Agezt — Concrete Detail Specifications (SPEC-16)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Active · Domain: github.com/agezt/agezt · License: MIT · Language: English
> Depends on: all prior specs + DECISIONS
> Fills the implementation-level gaps that the architectural specs intentionally left abstract: the API surface, the test strategy, the config reference, the agent wake-rule DSL, and the onboarding flow. These are needed during the build (mostly M3–M6) and are collected here.

---

## 1. API surface (gateway)

### 1.1 OpenAI-compatible API
- `POST /v1/chat/completions` — body is OpenAI-shaped; Agezt treats the final user message as an **intent**, runs a DAG, streams the result as OpenAI-shaped chunks. Tools in the request map to Agezt tools. `X-Agezt-Session-Id` header → `correlation_id` for continuity.
- `POST /v1/responses` — Responses-API shape, same mapping.
- `POST /v1/embeddings` — routed to an embedding-capable provider via Governor.
- Auth: bearer token issued by the gateway. All calls pass through Edict + journal (not a governance bypass).

### 1.2 Native REST/gRPC API
- `POST /api/intents` `{text, context?, trust_scope?}` → `{task_id}`; streams events over `GET /api/tasks/{id}/events` (SSE) or gRPC stream.
- `GET /api/agents` · `POST /api/agents/{id}/{suspend|resume|kill}`
- `GET /api/journal?from=&to=` (paginated) · `GET /api/why/{event_id}` · `POST /api/halt` · `POST /api/resume`
- `GET /api/memory?q=` · `POST /api/memory/forget`
- `GET /api/skills` · `POST /api/skills/{id}/{revert|pin}`
- `GET/POST /api/standing-orders` · `GET/POST /api/cron`
- `GET /api/changelog?system=true` · `POST /api/export` · `POST /api/import`
- `GET /healthz` `GET /readyz` (orchestrator probes)
- Plugin-contributed routes mount under `/v1/plugins/<id>/…` (SPEC-08 §1), namespaced + policy-wrapped.

### 1.3 Webhooks
- Inbound: `POST /api/hooks/<token>` → fires a typed schedule target or agent wake rule.
- Outbound: configured webhooks fire on subscribed events (e.g. notify an external system on `EVT_TASK_COMPLETED`).

### 1.4 Streaming & auth summary
- Live streams: SSE (web) + gRPC stream (SDK) + WS (UI). Local CLI uses the Unix socket.
- Exposure only via Tunnel (escalate, SPEC-06). mTLS for remote/mesh.

---

## 2. Test strategy (concrete)

### 2.1 Layers
- **Unit:** every kernel module; table-driven Go tests; race detector on.
- **Contract-conformance suite:** a runnable harness that any plugin SDK must pass — exercises Register/Health/Shutdown + the interface RPCs against the `.proto` semantics. Ships with the SDK so third parties self-verify. CI runs it against all first-party plugins.
- **Replay/property tests:**
  - *Fold determinism:* replaying a journal yields byte-identical projections.
  - *Hash-chain integrity:* any mutation/removal is detected by `Verify`.
  - *Durable-before-publish:* a subscriber never observes an event absent from the journal (fault-injected).
  - *ID uniqueness:* ULIDs never collide across simulated multi-node generation.
- **Integration:** end-to-end DAG runs with **fake providers/tools** (deterministic stubs) producing **golden traces** (recorded event sequences) diffed against expected.
- **Tool-call contract tests:** every first-party tool schema is linted; valid fixtures pass; malformed, missing-required, wrong-type, and unknown-field inputs are rejected before handler invocation.
- **Security suite (CI-gated):**
  - *Injection corpus:* a curated set of malicious inputs across channels/web/files/MCP/widgets; assertion = no privileged action fires without approval, no secret leaks.
  - *Sandbox escape:* attempts to break out of namespace/container profiles fail.
  - *Redaction coverage:* secret patterns never reach the journal or a provider (property test over generated secrets).
- **Chaos/soak:** kill plugins/agents mid-task → recovery with no data loss; budget/rate spike → anomaly auto-halt fires; long-running soak for leaks.

### 2.2 Gates
CI blocks merge on: build (multi-arch), unit, contract-conformance, replay/property, security suite. Chaos/soak run nightly. Coverage target: high on kernel core (journal/bus/scheduler/edict/governor).

### 2.3 Agent behavioral eval (SPEC-14 §3)
Separate from code tests: scenario suites with expected outcomes; success-rate + regression tracking per skill/capability; feeds reflection. Run on capability changes, not every commit.

Behavioral eval has two replay modes:

- **Recorded replay:** replays journaled provider/tool outputs exactly to rebuild state and inspect causality.
- **Stochastic re-run:** re-executes a scenario with model/provider/temperature/seed when supported, context snapshot, mocked tools, and expectation bands. The oracle is not byte equality unless the model call is deterministic; it is a bundle of schema, semantic, cost, latency, and safety assertions.

---

## 3. Configuration reference

`~/.agezt/config.yaml` — precedence: defaults < file < env (`AGEZT_*`) < flags (SPEC-02 §9).

```yaml
core:
  data_dir: ~/.agezt
  log_level: info
  profile: local            # local | vps | cluster

journal:
  driver: embedded          # embedded | postgres
  segment_bytes: 67108864   # 64 MiB (DECISIONS D1)
  snapshot_every_events: 10000
  fsync: batch

bus:
  driver: inproc            # inproc | nats | redis (future)

agents:
  mailbox_capacity: 256
  worker_pool: 8
  restart: { strategy: on_crash, max: 5, window_s: 60, backoff_base_ms: 500, backoff_max_ms: 30000 }

scheduler:
  loop_max_iterations: 25

providers:
  catalog_source: "https://models.dev"   # synced; can be local override file
  sync_schedule: "0 */6 * * *"
  default_model: ""         # empty → Governor decides
  fallback_chain: [anthropic, openrouter, ollama]
  embeddings: local

governor:
  budget_unit: usd_microcent
  spend_ceiling_per_day_usd: 20
  spend_ceiling_per_task_usd: 5

context:
  compress_at_fraction: 0.5
  protect_first_turns: 3
  protect_last_turns: 4

pulse:
  enabled: true
  cadence_base_s: 60
  salience_dial: balanced   # quiet | balanced | chatty
  quiet_hours: { start: "23:00", end: "07:00", tz: "Europe/Istanbul" }
  observers: [repo_ci, system_health]

edict:
  default_isolation: namespace
  egress: deny
  trust: { shell: L2, file: L2, http: L1, browser: L1, channel_send: L1, coding_merge: L1, purchase: L0 }

warden:
  docker_mode: sibling      # sibling | socket | dind
  microvm: false

security:
  redact_secrets: true
  anomaly: { tool_calls_per_5min: 300, spend_per_5min_usd: 5, error_rate_pct: 50, repeat_action: 3 }

channels: {}                # populated per connected channel
plugins:  {}                # per-plugin config
locale:   { language: en, timezone: "Europe/Istanbul" }
```

Plugins read their own scoped config via `Kernel.GetConfig` (namespaced under `plugins.<id>`).

---

## 4. Agent wake-rule DSL

Visual authoring in Flow Studio compiles to this declarative YAML (DECISIONS G4). A standing order is not an LLM prompt stored in the scheduler. It is a durable wake rule that selects an existing agent identity and tells the scheduler when to wake it. The agent's soul, tasklist, memory scope, permissions, provider/model/fallback config, retry policy, doctor policy, and mailbox live on the agent profile.

```yaml
standing_order:
  id: <ulid>                # kernel-assigned
  name: "portfolio watch wake rule"
  enabled: true
  agent: ops-watch
  triggers:                 # any of these activate evaluation
    - type: cron
      schedule: "0 8 * * *" # every morning
    - type: event
      subject: "github.>"
  event_filter:             # optional scheduler-side prefilter; payload stays data
    subjects: ["github.repo.*", "ci.*"]
  wake:
    kind: agent             # agent | workflow | tool | system_task
    ref: ops-watch
    intent: "cycle"         # small wake label only; not identity or instructions
  constraints:
    max_trust: L2           # extra ceiling for this wake rule
    budget_per_run_usd: 1
    cooldown_s: 3600
    quiet_hours_respected: true
```

The runner: on a trigger, evaluate scheduler predicates → wake the typed target → the target executes from its own profile/definition → journal fire/result/why metadata. Schedules can wake agents, run workflows, call tools, or run system tasks (catalog sync, memory tidy, log cleanup). They must not embed identity instructions, tasklists, provider choices, or agent policy.

Agent behavior belongs to the agent:

- **Agent profile:** identity/soul, lifecycle mode, parent/owner, managed-subagent direct-call policy, memory scopes, tasklists, skills, provider/model/fallback chain, tool permissions, retry/self-repair/doctor policy, mailbox bindings, workspace/data-lake authority.
- **Workflow:** reusable n8n-like typed chain that a user, agent, or schedule can run.
- **Schedule/standing order:** typed cron/event/webhook rule that wakes or runs one target and records causality.

---

## 5. Onboarding flow (first run)

Zero-config start works immediately; onboarding is an **optional guided flow** (skippable, resumable) that turns a blank Jarvis into *your* Jarvis.

### 5.1 Steps
1. **Welcome + safety framing:** explain autonomy is earned (trust ladder starts cautious), and `agt halt` always stops everything.
2. **Connect a provider:** detect existing credentials (Claude Code/Codex/env); else add an API key or point at a local Ollama. Sync the catalog.
3. **Connect a channel (recommended: Telegram):** the proactive loop needs an outbound channel; guided bot setup.
4. **Point at your world:** repos/projects/paths → seeds the world model (entities). Optional short interview ("how should I brief you? terse/detailed? quiet hours?").
5. **First agent + wake rule (suggested):** create `repo-watch`, then wake it each morning and on CI-failure events. Starts at a cautious trust level.
6. **Salience dial:** quiet/balanced/chatty.
7. **Done:** show the first `agt run` and where the UI lives.

### 5.2 Principles
- Every step is skippable; sensible defaults if skipped.
- Onboarding writes world-model + config via normal journaled events (auditable, revertible).
- Progressive disclosure: Flow Studio, policy editor, trust-ladder tuning are discoverable later, not forced now.
- Success criterion: within ~10 minutes, the user has had Agezt *do something real* and *proactively tell them something* — escaping the "still feels like a chatbot" trap.

---

## 6. Phase placement

- Config reference: **Phase 0–1** (config loader is foundational).
- Native + OpenAI-compat API: **Phase 5–7**.
- Test strategy: **every phase** (CI from Milestone 0).
- Agent wake-rule DSL: **Phase 3** (with typed schedule targets and standing wake rules).
- Onboarding flow: **Phase 5** (with the UI), CLI-guided version in **Phase 4**.

---

*With SPEC-15 and SPEC-16, the suite covers provider ecosystem and the previously-abstract implementation details. Remaining housekeeping: fold the new event kinds into the canonical proto enum, and extend TASKS.md with IDs for SPEC-08..16 work (tracked in the updated INDEX).*
