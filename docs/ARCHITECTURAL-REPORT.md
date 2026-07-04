# Agezt — Comprehensive Architectural Report

> **Generated:** 2026-06-20 · **Branch at generation time:** `main` · **Version:** v1.0.0+ · **Latest phase at generation time:** M781+ (historical; see `docs/SYSTEM-AUDIT-REPORT.md` for current counts)
> **Scope:** Every component, module, and technology across the entire monorepo.
> **Note:** Version and dependency details are sourced from `go.mod`, `frontend/package.json`, and `frontend/.nvmrc`. See `docs/SDK-PARITY.md` for API coverage and `DEPENDENCIES.md` for the dependency inventory.
> **Current-state note:** This generated report is broad but partially stale in its phase/test-count sections. For the latest review and gap plan, see `docs/SYSTEM-AUDIT-REPORT.md`, `docs/MISSING-PARTS-REPORT.md`, and `docs/MISSING-PARTS-PLAN.md`.

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Project Identity & Philosophy](#2-project-identity--philosophy)
3. [Technical Stack](#3-technical-stack)
4. [Repository Structure](#4-repository-structure)
5. [Kernel Architecture (The Core)](#5-kernel-architecture-the-core)
6. [CLI Surface (`agt`)](#6-cli-surface-agt)
7. [Daemon (`agezt`)](#7-daemon-agezt)
8. [Plugin Ecosystem](#8-plugin-ecosystem)
9. [SDK Layer](#9-sdk-layer)
10. [Web UI (Frontend)](#10-web-ui-frontend)
11. [Security & Governance](#11-security--governance)
12. [Data Flow & Persistence](#12-data-flow--persistence)
13. [Interoperability & Protocols](#13-interoperability--protocols)
14. [Build, CI/CD & Distribution](#14-build-cicd--distribution)
15. [Phase History & Implementation Status](#15-phase-history--implementation-status)
16. [Current State & What's Next](#16-current-state--whats-next)

---

## 1. Executive Summary

**Agezt** is an **agentic operating system** — a single, static Go binary that turns
intent into auditable, reversible action via a deterministic DAG with bounded LLM-loop
nodes. It runs autonomous agents under a **trust ladder** and **policy engine**,
proactively watches the user's world (Pulse), extends infinitely through **out-of-process,
polyglot plugins**, remembers through **tiered memory + a world model**, improves itself
via a governed, reversible **skill pipeline (Forge)**, and is **visually programmable**
(React Flow).

Everything is an event in a **tamper-evident, BLAKE3-hash-chained journal** — so every
action is explainable (`agt why`), reproducible, and revertible. The project has shipped
**~781 completed phases** from M0 (repository foundation) through M781 (alert → run
deep-links), merged across **224 pull requests**, currently at **v1.0.0+** with active
ongoing development.

**Core differentiators from OpenClaw and Hermes Agent:**
- Deterministic DAG + bounded LLM-loop (not a black-box stochastic loop)
- Proactive heartbeat (Pulse) that watches and acts unprompted
- Event-sourced journal for full auditability and reversibility
- Single static Go binary with near-zero dependencies
- First-party tool-loop orchestration (no third-party agent SDK dependency)
- Per-capability trust ladder + policy-as-code (Edict)

---

## 2. Project Identity & Philosophy

| Attribute | Value |
|---|---|
| **Name** | Agezt |
| **Daemon binary** | `agezt` |
| **CLI binary** | `agt` |
| **Env prefix** | `AGEZT_` |
| **Config dir** | `~/.agezt/` |
| **License** | MIT (SPDX headers) |
| **Go module** | `github.com/agezt/agezt` |
| **Go version** | 1.26.4 (CGO_ENABLED=0; see `go.mod`) |

### Design Principles

- **Single static binary, near-zero dependencies, stdlib-first.** Runs on a $5 VPS.
- **Everything is an event.** Append-only, hash-chained, reversible.
- **Out-of-process plugins.** Crash isolation; any language; a 20-line plugin via the SDK.
- **Pluggable everything.** Storage, providers, channels, tunnels.
- **Simple outside, powerful inside.** One command to start; every layer available to power users.
- **Security is core, not optional.** Default-deny, trust ladder, sandboxing, redaction, audit.

### Architecture Model — "Two Hearts"

```
        REACTIVE                                   PROACTIVE (Pulse)
   you / cron / event                         the system triggers itself
        │                                              │
   ┌────▼────┐                          ┌──────────────▼──────────────┐
   │ Planner │ intent → DAG             │ Observers → Salience → Initiative │
   └────┬────┘                          └──────────────┬──────────────┘
        └───────────────┬─────────────────────────────┘
                        ▼
   ┌───────────────────────────────────────────────────────────────┐
   │  KERNEL (single static Go binary)                              │
   │  Lifecycle · Journal(event truth+BLAKE3) · Plugin Host ·       │
   │  DAG Scheduler · Edict(policy/trust) · Governor                │
   └───────────────────────────────────────────────────────────────┘
                        │ stdio / JSON-RPC 2.0
   ┌────────────────────▼──────────────────────────────────────────┐
   │ PLUGINS  Channel · Provider · Tool · CodingAgent · Memory ·    │
   │          Storage · Tunnel        (any language, crash-isolated)│
   └───────────────────────────────────────────────────────────────┘
     CLI (agt) · Web UI (React SPA) · SDKs (Go/TS/Py/Rust)
```

---

## 3. Technical Stack

### Kernel (Backend)
| Component | Technology | Version |
|---|---|---|
| **Language** | Go | 1.26.4 |
| **Compilation** | Static binary, `CGO_ENABLED=0` | — |
| **Architectures** | amd64, arm64 | Linux, macOS, Windows |
| **Transport** | JSON-RPC 2.0 over stdio | newline-delimited |
| **Hashing** | BLAKE3 (lukechampine.com/blake3 v1.4.1) | — |
| **IDs** | ULID (custom implementation) | — |
| **Journal** | Append-only JSONL, BLAKE3 hash chain | 64 MiB segments |
| **State Store** | CobaltDB-class embedded KV (pluggable) | — |
| **Event Bus** | In-process, subject-routed, durable-before-publish | — |
| **Secrets** | AES-256-GCM at rest, PBKDF2 key derivation | — |
| **External Dependencies** | See `go.mod` and `DEPENDENCIES.md` for current direct + indirect module inventory | — |

### Frontend (Web UI)
| Component | Technology | Version |
|---|---|---|
| **Framework** | React | 19.2.7 |
| **Language** | TypeScript | 6.0.3 |
| **Build** | Vite | 8.0.16 |
| **Styling** | Tailwind CSS | 4.3.1 |
| **Components** | Radix primitives | dropdown-menu 2.1.17, scroll-area 1.2.11, tabs 1.1.14, tooltip 1.2.9 |
| **Flow/Graph** | @xyflow/react (React Flow) | 12.11.0 |
| **Icons** | Lucide React | 1.18.0 |
| **Testing (unit)** | Vitest | 4.1.8 |
| **Testing (E2E)** | Playwright | 1.60.0 |
| **Runtime** | Node.js 24 for frontend tooling (`frontend/.nvmrc`); built assets embedded via `go:embed` | — |

### SDK Ecosystem
| SDK | Language | Runtime | Dependencies |
|---|---|---|---|
| **Go SDK** | Go | Native | stdlib only (wraps control-plane client) |
| **TypeScript SDK** | TypeScript | Node.js ≥18 (dev tooling uses Node 24 per `frontend/.nvmrc`) | `@types/node` (dev only) |
| **Python SDK** | Python | ≥3.9 | stdlib only (urllib + json) |
| **Rust SDK** | Rust | ≥1.70 | std only (reqwest-free, uses std::net) |

### Key CLI Commands (35+ subcommands)
`run`, `halt`, `resume`, `why`, `whoami`, `journal`, `artifact`, `approvals`, `approve`, `deny`, `plan`, `catalog`, `provider`, `pulse`, `vault`, `plugin`, `budget`, `cache`, `tool`, `status`, `warden`, `redact`, `netguard`, `ratelimit`, `webhook`, `backup`, `restore`, `doctor`, `quickstart`, `acp`, `shutdown`, `edict`, `state`, `runs`, `config`, `disk`, `changelog`, `memory`, `world`, `skill`, `standing`, `reflect`, `inbox`, `send`, `ha`, `transcribe`, `listen`, `peers`, `schedule`, `tenant`

---

## 4. Repository Structure

```
agezt/
├── .project/             # Complete design suite (SPEC-01..16, DECISIONS, POLICY, ROADMAP,
│                         # 580+ phase reports + BRANDING, BUILD-GUIDE, IMPLEMENTATION, TASKS...)
├── .github/workflows/    # CI: multi-OS test, race detection, cross-build, e2e, codegen-in-sync
├── .playwright-mcp/      # Playwright MCP traces for web UI testing
├── .claude/              # Claude Code session files
├── .dfmt/                # Deformity format config/tooling
├── cmd/
│   ├── agezt/            # Daemon binary entry point + subcommand dispatch
│   └── agt/              # CLI client binary (35+ subcommands, ~160 source files)
├── internal/
│   ├── brand/            # Name/paths/version constants (single source of truth)
│   ├── paths/            # Platform-aware path resolution
│   └── strutil/          # String utilities (safe truncation, ellipsis)
├── kernel/               # ~50 packages, the core engine
├── plugins/              # First-party plugins (4 categories)
│   ├── providers/        # LLM provider adapters
│   ├── tools/            # Agent tools
│   ├── channels/         # Messaging channels
│   └── external/         # External bridges (MCP)
├── contract/gen/         # Generated Go SDK types from agezt-contract.jsonc
├── frontend/             # React 19 SPA (~80 components, 40+ views)
├── sdk/                  # Public SDKs (Go/TypeScript/Python/Rust)
├── tools/
│   ├── jsonschemagen/    # JSON Schema → Go type generator
│   └── depscheck/        # Dependency allowlist checker
├── scripts/              # e2e-smoke.sh, webui-e2e.sh
├── examples/             # agezt-run usage example
├── bin/                  # Built binaries (agezt.exe, agt.exe)
└── root config           # go.mod, go.sum, Makefile, CHANGELOG.md, LICENSE, README.md
```

---

## 5. Kernel Architecture (The Core)

The kernel contains ~50 packages organized by responsibility. All point inward toward
`event` + `journal` + `state` + `bus` — the four pillars.

### 5.1 The Four Pillars

#### `kernel/event` — Canonical Event Type
The fundamental data type of the system. Every meaningful action produces an `Event`:
```go
type Event struct {
    ID, Seq, TsUnixMS, PrevHash, Hash, Subject, Actor,
    CorrelationID, CausationID, Kind string
    Payload map[string]any
    Tags    map[string]string
}
```
- **Kinds** grow append-only; current surface includes `agent.spawned/suspended/resumed/died/crashed`,
  `task.received/completed`, `tool.invoked/result`, `llm.request/token/response`,
  `channel.inbound/outbound`, `memory.write/forget`, `skill.created/patched/promoted/quarantined/reverted`,
  `pulse.tick`, `observer.delta`, `salience.scored`, `initiative.taken`, `briefing.sent`,
  `policy.decision`, `budget.consumed`, `approval.requested/granted/denied`, `halt`, `resume`,
  `anomaly.detected`, `plugin.installed/updated/removed/enabled/disabled`,
  `migration.applied/reverted`, `exported/imported`, `backup.created/restored`,
  `catalog.synced`, `acp.session_started/ended`, and many more.

#### `kernel/journal` — Event Log
Append-only JSONL store with BLAKE3 hash chaining. Properties:
- Segmented at 64 MiB boundaries (configurable)
- Each event: `hash = BLAKE3(prev_hash || canonical_json_bytes)`
- `fsync` on batch boundary (durable-before-publish)
- Supports verification, recovery, snapshot generation
- Thread-safe concurrent append via mutex serialization

#### `kernel/state` — Mutable State Store
First-class mutable KV store alongside the journal (DECISIONS B0c). Used for
frequently-read state that would be expensive to recompute by folding the log.
Pluggable backend (embedded B+Tree class default; Postgres optional).

#### `kernel/bus` — Event Bus
In-process pub/sub bus with subject routing. Guarantees durable-before-publish:
events are appended to the journal and fsync'd before they are published to
subscribers. Supports subject-pattern matching for selective consumption.

### 5.2 Orchestration Layer

#### `kernel/agent` — Tool-Loop Core
The first-party, dialect-free single-agent tool-loop (DECISIONS B0d):
- Defines canonical `Message`, `ToolCall`, `ToolDef` types
- Implements bounded iteration loop (configurable MaxIter)
- Every step is journaled via the bus
- Honors context cancellation (how `agt halt` stops a run)
- **No third-party agent SDK** — entirely owned by Agezt

#### `kernel/runtime` — Kernel Wiring
Wires all subsystems into a single `Kernel` struct:
```go
type Kernel struct {
    Journal, State, Bus, Agent, Governor, Edict,
    Warden, Memory, WorldModel, Skill, Scheduler,
    Approval, Artifact, Reflect, Standing, Cadence...
}
```
- One Kernel per Agezt process
- Concurrent `Run` calls supported (each gets its own correlation_id)
- `Halt` cancels all in-flight runs; `Resume` re-enables

#### `kernel/scheduler` — DAG Scheduler
Second layer over the agent loop. Compiles user intent into a DAG and executes
it, with node types: `tool`, `llm`, `loop`, `gate`. 8 concurrent workers by default;
path-scoped serialization for nodes touching the same resource.

#### `kernel/planner` — Intent → DAG
Meta-agent that converts natural language intent into a structured plan DAG.
Supports cost estimation (`plan cost`), validation (`plan validate`),
refinement, and visualization.

### 5.3 Governance & Policy

#### `kernel/governor` — Routing + Budget
Sits between the agent loop and provider plugins:
- **Routing:** subscription-first → quality-floor for task type → cost → latency
- **Fallback chain:** non-primary providers → fallback providers
- **Budget:** USD-microcents (integer) as canonical unit (DECISIONS C1)
- **Ceilings:** per-day cap (default $20/day), per-task cap
- **Strict pricing:** exact model pricing known, zero-tolerance for unknowns
- Tracks all spend and publishes `budget.consumed` events

#### `kernel/edict` — Policy Engine + Trust Ladder
Two-layer security model:
1. **Hard-deny rules** (never overridable): fork-bombs, `rm -rf /`, mkfs,
   shutdown/reboot, audit-disable, secret exfiltration
2. **Trust ladder** (L0–L4 per capability):
   - L0: Always deny
   - L1–L3: Ask (configurable as Allow+journal, Deny, or live human approval)
   - L4: Always allow

Capabilities: `shell`, `file.read`, `file.write`, `file.delete`, `file.list`,
`http.get`, `http.post`, `provider.call`, `delegate`, `coding`, `acp_agent`, and more.

#### `kernel/approval` — Human-in-the-Loop
Runtime approval registry that blocks tool calls until an operator grants or
denies. Integrates with Edict's AskPolicy for live human approval prompts.

### 5.4 Memory & Knowledge

#### `kernel/memory` — Tiered Memory Store
"Memory-lite" implementation (ROADMAP §2.3):
- Journaled, content-addressed knowledge store (BLAKE3 of type\0subject\0content)
- Soft updates (SupersededBy) and soft deletes (Tombstoned)
- Keyword retrieval ranking
- Two-layer split: pure `Store` (testable) + `Manager` (bus-integrated)
- Agent loop reads memory as injected context
- Operator, agent, and auto-distiller can write

#### `kernel/worldmodel` — World Model Graph
Content-addressed graph of the operator's world:
- Entities: projects, repos, people, accounts, channels, topics
- Weighted relations between entities
- Resolves semantic queries ("the portfolio" → set of repos)
- Powers Pulse Salience relevance (what matters *to this operator*)
- Same two-layer split as memory (Store + Graph)

#### `kernel/skill` — Forge (Self-Improvement Pipeline)
Governed skill lifecycle:
- **Auto-quarantine:** rate-bound, failure-based isolation
- **Auto-shadow:** shadow-test new skills against production
- **Shadow eval:** evaluate shadow performance
- **Auto-promote:** graduate proven skills to production
- **Lock-revert:** revert destructive forge operations
- **Retrieval pool:** skill selection and ranking

### 5.5 Proactive Heart (Pulse)

#### `kernel/pulse` — Proactive Engine
The second heartbeat that triggers itself:
```
tick → ① Observers (what changed?) → ② Salience (is it important?) →
③ Initiative (should I act/tell?) → ④ Briefing (deliver to user)
```
- **Observers:** repo/CI, system health, disk usage, anomaly detection
- **Salience:** rules + LLM-based relevance scoring, boosted by world model
- **Initiative:** inform-first, ask-first, autonomous-act (configurable)
- **Briefing:** composed for Telegram, Slack, Discord, Web UI, CLI
- **Quiet hours:** configurable suppression windows
- **Novelty TTL:** prevents repeated notifications
- **Disk threshold edges:** warns before storage exhaustion

### 5.6 Scheduling Infrastructure

#### `kernel/scheduler` — Cron/Cadence Engine
Event-driven cron/interval scheduler:
- Fixed-interval cadences
- Cron expressions
- DST-aware (interval floor, DST fallback)
- Continuous mode (fire-on-startup)
- Crash-safe one-shot timers
- Per-tenant scheduling
- Correlation tracking

#### `kernel/cadence` — Timing Primitives
Low-level timing utilities for the scheduler: interval floor guards,
DST fallback protection, crash-safe once-execution primitives.

#### `kernel/standing` — Standing Orders
Autonomous recurring tasks (SPEC-14):
- Configurable scope, budget, trust level
- Cron-triggered or rule-triggered
- Max-trust ceiling (autonomy gating)
- Briefing delivery after execution
- Web UI panel for management

### 5.7 Communication & Channels

#### `kernel/channel` — Channel Abstraction
Unified message routing:
- `UnifiedMessage` normalization across all channel types
- Channel guard (inbound/outbound auth)
- Message splitting (for platforms with length limits)
- Chat history management
- Empty-message no-op protection
- Media size capping

#### `kernel/webhook` — Webhook Infrastructure
Durable webhook delivery with:
- Status boundary verification (2xx range)
- Deduplication with TTL rotation
- Egress guard (no internal-network webhooks)
- Subject validation
- Secret pipe (secure payload signing)
- Observability logging

### 5.8 Security Infrastructure

#### `kernel/warden` — Sandbox Isolation
Process isolation engine:
- **Profiles:** namespace, container, microvm (optional)
- **Environment:** clean-room environment variables
- **Capability buffer:** Linux capability bounding
- **Classification:** wait-error classification for crash decision
- **Process group isolation** (Unix)
- Per-platform implementations (Linux/Windows/macOS)

#### `kernel/netguard` — Network Egress Control
Default-deny egress with per-capability allowlists:
- Host allowlist with wildcard matching (`*.example.com`)
- SSRF protection (private IP, localhost blocking)
- HTTP/HTTPS-only enforcement

#### `kernel/creds` — Credential Management
Secure secret lifecycle:
- **Vault:** AES-256-GCM encrypted credential store
- **PBKDF2:** key derivation with configurable iterations
- **Migration:** detect stale KDF params, re-encrypt in place
- **AWS chain:** STS, SSO, web identity, process credentials
- **Keyring:** platform-native secure storage integration
- **Rotation:** credential rotation support
- **Encrypt/Decrypt:** vault-level encryption primitives

#### `kernel/redact` — Secret Redaction
Automatic PII/secret detection and redaction:
- Pattern-based: API keys, tokens, connection strings
- Streaming redaction for live output
- Plugin stderr redaction
- Integration-level secret detection in tool results
- Mutation hardening (immutable redacted values)

### 5.9 Control Plane

#### `kernel/controlplane` — Daemon API Server
The full REST API and control plane exposed by the daemon:
- **TCP** localhost server with token auth
- **Constant-time token comparison** (prevents timing attacks)
- **Request size bounds** (prevents OOM)
- **Panic recovery** middleware
- **Multi-tenant auth gates**
- **Read-only proxy routes** for the Web UI
- **Over 50+ route handlers** covering all kernel surfaces

#### `kernel/restapi` — Public REST API
The `/api/v1` surface for external consumers and SDKs:
- Health probes (`/api/v1/health`)
- Metrics endpoint (Prometheus-compatible)
- Mesh hop limit enforcement
- Request oversize protection

### 5.10 Model & Provider Infrastructure

#### `kernel/catalog` — Model Catalog
Provider/model discovery and sync:
- `models.dev`-class catalog sync
- Local auto-discovery
- Cross-provider tiebreaking
- Catalog meta read-modify-write race protection
- Empty sync protection
- Fuzz testing coverage

#### `kernel/openaiapi` — OpenAI API Compatibility
Dual-surface OpenAI compatibility layer:
- **Chat Completions API** (`/v1/chat/completions`)
- **Responses API** (newer endpoint)
- Audio transcription endpoint
- Model retrieval (`/v1/models`)
- Vision input handling
- Usage estimation
- JSON mode support
- Reasoning content extraction
- Fuzz-tested

#### `kernel/governor` — Provider Registry & Routing
- Provider registry with capability introspection
- Model chain routing with task-type awareness
- Model override mechanism
- Capability degradation routing
- Usage index rotation
- Strict pricing enforcement for cost governance

### 5.11 Other Kernel Modules

| Package | Purpose |
|---|---|
| `kernel/ulid` | ULID generation with encode/decode |
| `kernel/convo` | Conversation management (message history) |
| `kernel/assure` | Runtime assertion/verification utilities |
| `kernel/board` | Lightweight planning board (task tracking) |
| `kernel/bus` | Event bus with durable-before-publish |
| `kernel/anomaly` | Anomaly detection + auto-halt monitor |
| `kernel/artifact` | Content-addressed artifact store + CAS |
| `kernel/planner` | Intent-to-DAG compiler with cost estimation |
| `kernel/reflect` | Reflection loop integration |
| `kernel/stt` | Speech-to-text service wrapper |
| `kernel/tunnel` | Tunnel management (cloudflare/tailscale) |
| `kernel/settings` | Configuration registry + schema |
| `kernel/tenant` | Multi-tenant isolation |
| `kernel/tenantctx` | Per-tenant context injection |
| `kernel/meshctx` | Mesh/federation context |
| `kernel/acp` | Agent Client Protocol server + client |
| `kernel/webui` | Web UI embedding + route handling |

---

## 6. CLI Surface (`agt`)

The `agt` CLI is a thin client that connects to the running `agezt` daemon via
the local control plane (TCP localhost + token). It has **35+ subcommands**
organized as a minimal custom command router (zero external deps per POLICY §1):

### Core Operations
| Command | Description |
|---|---|
| `agt run "<intent>"` | Run an intent end-to-end, streams events |
| `agt halt` | Freeze all in-flight runs |
| `agt resume` | Clear the halt flag |
| `agt why <event_id>` | Explain an event's full correlation chain |
| `agt shutdown` | Gracefully stop the daemon |

### Observability
| Command | Description |
|---|---|
| `agt status` | Daemon health + runtime stats + mesh/channel info |
| `agt journal verify` | Verify BLAKE3 hash chain |
| `agt journal tail` | Stream recent journal events |
| `agt journal head` | Show journal head (latest seq + hash) |
| `agt journal grep` | Search journal by regex |
| `agt journal export` | Export journal segment as bundle |
| `agt journal import` | Import/replay a journal bundle |
| `agt journal stats` | Journal statistics |
| `agt runs` | List/inspect past runs |
| `agt runs stats` | Aggregate statistics by model/cost/intent |
| `agt runs tree` | Run dependency tree |
| `agt changelog` | Daemon change history |
| `agt doctor` | Preflight diagnostics (OK/WARN/FAIL) |

### Config & Identity
| Command | Description |
|---|---|
| `agt whoami` | Current operator identity |
| `agt config` | View/export daemon config |
| `agt quickstart` | Zero-config bootstrap (sync catalog, add provider) |
| `agt disk` | Disk usage report |

### Security
| Command | Description |
|---|---|
| `agt edict` | Policy management + test |
| `agt edict overlay` | Runtime policy overlay |
| `agt vault` | Credential vault operations |
| `agt vault migrate` | Upgrade vault encryption |
| `agt vault status` | Vault KDF health |
| `agt warden` | Sandbox config |
| `agt redact` | Redaction test |
| `agt netguard` | Network egress config |

### Providers & Models
| Command | Description |
|---|---|
| `agt catalog sync` | Sync model catalog |
| `agt provider` | Provider management + setup/import/keys |
| `agt provider cost` | Per-provider spend report |
| `agt budget` | Budget status |
| `agt budget check` | Budget feasibility check |
| `agt cache` | Prompt cache usage/stats |

### Channels & Communication
| Command | Description |
|---|---|
| `agt inbox` | Unified channel inbox |
| `agt send` | Send message via channel |
| `agt webhook` | Webhook management |
| `agt listen` | Voice/TTS listen mode |

### Autonomous Features
| Command | Description |
|---|---|
| `agt pulse` | Heartbeat status + control |
| `agt schedule` | Scheduled task management |
| `agt standing` | Standing order management |
| `agt skill` | Skill lifecycle + import/export/diff |
| `agt memory` | Memory store operations |
| `agt world` | World model operations |
| `agt reflect` | Reflection loop status |

### Advanced
| Command | Description |
|---|---|
| `agt acp` | ACP agent management |
| `agt plan` | Plan creation + validation + visualization |
| `agt plugin` | Plugin management + registry |
| `agt peeps` | Mesh peer management |
| `agt tenant` | Multi-tenant management |
| `agt artifact` | Artifact CAS store |
| `agt approvals` | Approval queue |
| `agt approve/deny` | Human-in-the-loop decisions |
| `agt tool` | Tool inspection |
| `agt ratelimit` | Provider rate limit status |
| `agt backup` | Full system backup |
| `agt restore` | Point-in-time restore |
| `agt transcribe` | Speech-to-text |
| `agt ha` | Home Assistant integration |

---

## 7. Daemon (`agezt`)

The `agezt` binary hosts the full kernel runtime:
- **Journal** + **State** + **Bus** (the four pillars)
- All **in-process plugins** (Anthropic, OpenAI, Ollama, Gemini, Vertex, Bedrock, Cohere, Compat providers + shell, file, http, browser, coding, peer, notify, websearch, schedule, board, introspect, skill, standing, runs, homeassistant tools)
- **Control plane** HTTP server (localhost + token)
- **REST API** (`/api/v1/` — health, metrics, mesh)
- **Web UI** (embedded via `go:embed`, served by the daemon)
- **Out-of-process plugin host** (spawns external plugins over stdio/JSON-RPC)
- **Pulse engine** (proactive heartbeat)
- **Scheduler** (cron/cadence engine)
- **Standing orders** runner
- **Anomaly monitor**
- **Credential vault**
- **Memory + World model** managers
- **Skill forge**
- **Reflection loop**
- **Channel sinks** (Telegram, Slack, Discord, Email, SMS, WhatsApp, Signal, Matrix, Teams, HomeAssistant, Webhook)
- **Tunnel manager**
- **STT** service
- **ACP** server

Startup sequence: reads config → opens journal → recovers state → registers providers
→ loads plugins → starts control plane → emits boot advisory → begins Pulse heartbeat.

---

## 8. Plugin Ecosystem

### Provider Plugins (LLM Adapters)

All provider plugins translate between Agezt's canonical dialect-free `Message`/`ToolCall`/`ToolDef`
shapes and each backend's native API format.

| Plugin | Backend | Features |
|---|---|---|
| **anthropic** | Anthropic Messages API | Streaming, extended thinking, vision, prompt caching, system cache, cache-aware cost |
| **openai** | OpenAI Chat Completions | Streaming, vision, JSON mode, reasoning (o1/o3), tool name normalization, limit reporting |
| **ollama** | Local Ollama | Streaming, vision, JSON mode, max-tokens config |
| **google** | Google Gemini | Streaming, vision, JSON mode, thinking, empty-response guards, tool result JSON normalization |
| **vertex** | Vertex AI (Anthropic + Gemini) | Streaming, thinking, vision, metadata credentials, JSON mode, empty-response guards |
| **bedrock** | AWS Bedrock | Multi-model: Anthropic, AI21, Cohere, Llama, Mistral, DeepSeek, Nova; SigV4 auth, streaming, vision, deepseek support |
| **cohere** | Cohere API | Streaming, fuzz-tested |
| **compat** | OpenAI-compatible vendors | DeepSeek, Groq, Together, OpenRouter, xAI, Fireworks, Cerebras, SambaNova, Moonshot, Perplexity; base-URL configuration per vendor |

### Tool Plugins

| Tool | Function | Isolation |
|---|---|---|
| **shell** | Execute shell commands with timeout | namespace (Warden) |
| **file** | Read/write/list/search/stat/delete; workspace-scoped | namespace |
| **http** | HTTP GET/POST with host allowlist | netguard |
| **browser** | Headless browser (Playwright) | container |
| **coding** | Delegate to external coding agent in git worktree | container |
| **acpagent** | Delegate to external agent via ACP | container |
| **peer** | Mesh/routing: remote run, cache, failover, hop discovery | netguard |
| **notify** | Send notifications via channels | — |
| **websearch** | Web search (DuckDuckGo) | netguard |
| **schedule** | Schedule management from within agent | — |
| **boardtool** | Planning board operations | — |
| **introspecttool** | Kernel introspection (tools, config, providers) | — |
| **skilltool** | Skill lifecycle management | — |
| **standingtool** | Standing order operations | — |
| **runstool** | Past run inspection | — |
| **config** | Configuration management | — |
| **codeexec** | Code execution with runtime detection + package management | namespace |
| **homeassistant** | Home Assistant device control | netguard |

### Channel Plugins (11 channels)

All channels normalize to `UnifiedMessage` and support duplex communication:

| Channel | Features |
|---|---|
| **telegram** | Photo dispatch, response cap, inbound images, chunking |
| **slack** | Inbound images, deduplication, chunking, empty-message protection, slowloris guard |
| **discord** | Inbound images, chunking, followup chunking, slowloris guard |
| **email** | Subject CR handling |
| **whatsapp** | Message delivery |
| **sms** | SMS delivery |
| **signal** | Signal messaging |
| **matrix** | Matrix protocol |
| **teams** | Microsoft Teams |
| **homeassistant** | Home Assistant integration |
| **webhook** | Generic webhook receiver + egress guard |

### External Plugins

| Plugin | Purpose |
|---|---|
| **mcpbridge** | MCP (Model Context Protocol) bridge: adapts any MCP server into Tool capabilities. Supports stdio and SSE transports, frame bounding, panic containment. |

---

## 9. SDK Layer

Agezt ships four first-party SDKs, all stdlib-first (zero runtime dependencies):

### Go SDK (`sdk/`)
```go
c, _ := sdk.Dial("")
res, _ := c.Run(ctx, "summarise the repo", sdk.WithModel("claude-sonnet-4-6"))
```
- `Dial` → connect to local daemon
- `Run` → run intent and get typed `Result`
- `RunStream` → observe events in real-time
- `Runs` → list past runs
- `PendingApprovals` / `Approve` / `Deny`
- Event helpers (`TokenText`, `ToolCall`, `IsTerminal`)
- Runnable `examples/agezt-run` + godoc examples

### TypeScript SDK (`sdk/typescript/`)
```typescript
import { Agezt } from "@agezt/sdk";
const client = new Agezt({ baseUrl: "http://127.0.0.1:9779" });
const result = await client.run("check my repos");
```
- Native `fetch` (Node 18+)
- Typed result + event streaming
- Error types
- Node.js test runner compatible

### Python SDK (`sdk/python/`)
```python
from agezt import AgeztClient, AsyncAgeztClient
client = AgeztClient()
result = client.run("summarize my inbox")
```
- Synchronous (`urllib`) + Async (`aiohttp`-style) variants
- Typed errors
- Python ≥3.9, stdlib-only

### Rust SDK (`sdk/rust/`)
```rust
use agezt::Client;
let client = Client::new("http://127.0.0.1:9779")?;
let result = client.run("summarize my inbox")?;
```
- Pure Rust, std-only (no reqwest, uses `std::net`)
- Edition 2021, minimum Rust 1.70
- Typed client with JSON serde
- Error types + HTTP handler

---

## 10. Web UI (Frontend)

A React 19 Single Page Application, built with Vite, styled with Tailwind CSS 4,
using shadcn/ui components and React Flow for visual programming. The build output
is **go:embed-ded** into `kernel/webui/dist` and served directly by the daemon —
**no separate web server needed**.

### Technology Stack
- **React 19** with Server Components mindset (client-side only for the SPA)
- **TypeScript 5.7.2** with strict mode
- **Vite 6** for build (assetsInlineLimit:0 for CSP compatibility)
- **Tailwind CSS 4** with `@tailwindcss/vite` plugin
- **shadcn/ui** component primitives (Radix dropdown-menu, scroll-area, tabs, tooltip)
- **@xyflow/react 12.4.2** (React Flow) for DAG visualization
- **Lucide React** icons
- **Vitest 4.1.8** for unit/component tests
- **Playwright 1.60** for E2E tests

### Component Architecture (~80 files)

#### Views (30+ screens)
| View | File | Purpose |
|---|---|---|
| **Chat** | `Chat.tsx` | Main conversation surface with tool-call debug, context inspector, persona picker, fallback modal, history |
| **Dashboard** | `Dashboard.tsx` | Overview: feeds, status, budget, vitals |
| **Runs** | `Runs.tsx` | Past runs list with filtering by status/cost/model |
| **RunDetail** | `RunDetail.tsx` | Deep-dive into a single run with live cards |
| **Activity** | `Activity.tsx` | Event feed / activity log |
| **Agents** | `Agents.tsx` | Active agent status + management |
| **Config** | `Config.tsx` | Configuration editor |
| **ConfigCenter** | `ConfigCenter.tsx` | Central config hub |
| **Providers** | `Providers.tsx` | Provider management with log modal + reload |
| **Models** | `Models.tsx` | Model catalog browser |
| **Budget** | `Budget.tsx` | Spend tracking + budget panels |
| **Cache** | `Cache.tsx` | Prompt-cache savings visualization |
| **Tools** | `Tools.tsx` | Tool registry with debug modal |
| **Policy** | `Policy.tsx` | Edict policy editor |
| **Schedules** | `Schedules.tsx` | Schedule management with next-fire preview |
| **Standing** | `Standing.tsx` | Standing order management with history |
| **Skills** | `Skills.tsx` | Skill registry + lifecycle |
| **Memory** | `Memory.tsx` | Memory store explorer |
| **World** | `World.tsx` | World model graph visualization |
| **Reflect** | `Reflect.tsx` | Reflection loop status |
| **Pulse/Autonomy** | `Autonomy.tsx` | Proactive heartbeat panel with pause/resume |
| **Alerts** | `Alerts.tsx` | Alert management |
| **Approvals** | `Approvals.tsx` | HITL approval queue |
| **Backup** | `Backup.tsx` | Backup/restore management |
| **Catalog** | `Catalog.tsx` | Provider catalog browser |
| **FlowStudio** | `FlowStudio.tsx` | Visual DAG programming (React Flow) |
| **Health** | `Health.tsx` | Daemon health dashboard |
| **Inbox** | `Inbox.tsx` | Unified channel inbox |
| **Insights** | `Insights.tsx` | Analytics/insights |
| **Mission** | `Mission.tsx` | Mission/standing-order editor |
| **Persona** | `Persona.tsx` | Operator persona management |
| **Prompts** | `Prompts.tsx` | Prompt management |
| **Replay** | `Replay.tsx` | Event replay viewer |
| **Routing** | `Routing.tsx` | Provider routing configuration |
| **Sandbox** | `Sandbox.tsx` | Warden sandbox config |
| **Search** | `Search.tsx` | Global search |
| **Status** | `Status.tsx` | System status dashboard |
| **Board** | `Board.tsx` | Planning board |

#### Shared Components (~20)
| Component | Purpose |
|---|---|
| `AccentPicker` | Theme accent color picker |
| `ActionButton` | Contextual action trigger |
| `AlertBell` | Alert notification bell |
| `AttachPicker` | File/image attachment picker |
| `Charts` | Charting components |
| `CommandPalette` | Cmd+K command palette |
| `ConsoleName` | Console identity display |
| `DataView` | Generic data table viewer |
| `DelegationGraph` | Delegation tree visualization |
| `EventFeed` | Real-time event stream |
| `FlightRecorder` | Run timeline recorder |
| `JsonView` | JSON tree viewer |
| `LogDetail` | Log detail modal |
| `Markdown` | Markdown renderer |
| `MicButton` | Voice input button (STT) |
| `MiniChat` | Compact chat widget |
| `ModelPicker` | Model selection dropdown |
| `Panel` | Reusable panel container |
| `PlanDag` | Plan DAG renderer |
| `ThemeToggle` | Light/dark theme toggle |
| `Vitals` | System vitals display |
| `Widgets` | Widget container system |
| `WorldGraph` | World model graph renderer |

#### Library Modules (~40 files)
| Module | Purpose |
|---|---|
| `accent.ts` | Accent color system |
| `activity.ts` | Activity feed logic |
| `alerts.ts` | Alert management |
| `api.ts` | REST API client |
| `appearance.ts` | Theme/appearance management |
| `attach.ts` | Attachment handling |
| `brand.ts` | Brand constants |
| `catalog.ts` | Catalog queries |
| `chat.ts` | Chat logic |
| `chatStore.tsx` | Chat state (React context) |
| `commands.ts` | Command palette commands |
| `configbackup.ts` | Config backup/restore |
| `conversations.ts` | Conversation management |
| `delegation.ts` | Delegation tracking |
| `eventmeta.ts` | Event metadata rendering |
| `events.tsx` | Event context provider |
| `export.ts` | Data export utilities |
| `format.ts` | Number/date formatting |
| `insights.ts` | Analytics insights |
| `markdown.ts` | Markdown parsing |
| `models.ts` | Model data queries |
| `replay.ts` | Event replay logic |
| `rundetail.ts` | Run detail queries |
| `runfocus.ts` | Run focus management |
| `snapshot.ts` | Snapshot handling |
| `speech.ts` | Speech recognition (Web Speech API) |
| `telemetry.ts` | Telemetry collection |
| `theme.ts` | Theme management |
| `usePanel.ts` | Panel state hook |
| `utils.ts` | General utilities |
| `voice.ts` | Voice/TTS integration |

### Data Flow
- **All state from daemon** over REST API (`/api/v1/` + control-plane routes)
- No localStorage for authoritative state (streams from kernel)
- React context providers for chat, events, and global state
- Real-time event streaming for live updates

### Security
- **Strict CSP:** `script-src 'self'`, no inline scripts, no nonce (enforced by Vite build with `assetsInlineLimit:0`)
- **Constant-time token comparison** in control-plane auth
- **XSS sink guards** in dashboard components
- **Security headers** in daemon-served responses

### Testing
- **Vitest** unit tests (50+ test files co-located with source)
- **Playwright** E2E tests (`frontend/e2e/webui.spec.ts`)
- Component tests for interactive components (DataView, JsonView, ConsoleName, etc.)

---

## 11. Security & Governance

### Trust Ladder (Edict)
```
L0 → Always deny (non-raisable)
L1 → Ask (configurable: allow+log, deny, or live approval)
L2 → Ask
L3 → Ask
L4 → Always allow
```

Default assignments (DECISIONS F3):
- `shell`: L2, `file`: L2, `http`: L1, `browser`: L1
- `channel.send`: L1, `coding.merge`: L1, `purchase`: L0
- Provider spend ceiling: $20/day default
- Reflection may lower autonomy autonomously, **never raise**

### Hard-Deny Rules (Never Overridable)
- Secret exfiltration attempts
- Audit disable attempts
- Destructive delete outside workspace
- Fork-bomb / `rm -rf /` class commands
- mkfs, shutdown/reboot

### Secret Management
- AES-256-GCM at rest in credential vault
- PBKDF2 key derivation with configurable iterations
- Scoped short-lived issuance to plugins
- Redaction on by default (all output streams)
- OAuth via PKCE
- Passwords never typed on user's behalf

### Anomaly Auto-Halt
Default thresholds (configurable):
- >300 tool-calls / 5 minutes
- >$5 spend / 5 minutes
- >50% error rate / 5 minutes
- Same autonomous action repeated >3×

### Sandbox Profiles (Warden)
| Profile | Use Case | Mechanism |
|---|---|---|
| `none` | First-party WASM (read-only) | No isolation |
| `namespace` | Shell, file, http tools | Linux namespaces + cgroups |
| `container` | Browser, coding agents, untrusted plugins | Docker sibling containers |
| `microvm` | Maximum isolation (optional) | Firecracker-class microVM |

---

## 12. Data Flow & Persistence

### Event Lifecycle
```
1. Actor creates event payload
2. Journal appends event (assigns seq, prev_hash, computes hash)
3. Journal fsyncs (durable-before-publish)
4. Bus publishes event to subscribers
5. State store may update projections
6. Subscribers react (Pulse, channels, webhooks, etc.)
```

### Storage Layout
```
~/.agezt/
├── config.yaml              # Daemon configuration
├── journal/                 # Append-only JSONL segments (64 MiB each)
│   ├── 00000001.jsonl
│   ├── 00000002.jsonl
│   └── ...
├── state/                   # Mutable KV state store
├── runtime/
│   └── sockets/             # Control-plane socket / token
├── plugins/                 # Plugin binaries + configs
├── secrets.enc              # Encrypted credential vault
├── workspace/               # Agent workspace root
├── memory/                  # Memory store
├── worldmodel/              # World model graph
├── skills/                  # Skill definitions
├── artifacts/               # Content-addressed artifact store
├── catalog/                 # Provider/model catalog cache
└── snapshots/               # Periodic journal snapshots
```

### Durability Guarantees
- **Durable-before-publish:** Every event is fsync'd to the journal before the bus publishes it
- **Snapshot every 10,000 events or 1 hour** (whichever first)
- **Content-addressing** (BLAKE3) for immutable content
- **ULID** for all time-ordered entities
- **Hash chain verification** (`agt journal verify`) detects any tampering
- **Point-in-time restore** from backup bundles

### Data Integrity
- All mutations are soft (SupersededBy, Tombstoned) — history is never destructively edited
- Memory records, world model nodes/edges, skills are all content-addressed
- Journal hash chain validates the entire event history
- Fuzz-tested: journal, edict, redact, catalog, controlplane, channel signatures, provider streams

---

## 13. Interoperability & Protocols

### Agent Client Protocol (ACP)
- Server + client implementation
- Bidirectional JSON-RPC 2.0 over stdio
- Same wire format as internal plugin transport
- Frame bounding, message accumulation, cancel support
- Per-message bound enforcement
- ACP agent version negotiation

### MCP Bridge
- Adapts any MCP (Model Context Protocol) server into Tool capabilities
- Supports **stdio** and **SSE** transports
- Frame bounding with configurable limits
- Panic containment (isolated process)
- SSE limit enforcement

### OpenAI-Compatible API
- Full `/v1/chat/completions` endpoint
- Responses API endpoint
- Audio transcription endpoint
- Model listing endpoint
- Compatible with any OpenAI SDK client

### REST API (`/api/v1/`)
- Health probes (`/health`, `/ready`)
- Metrics (Prometheus-compatible)
- Mesh/federation endpoints
- All kernel surfaces proxied for Web UI consumption

### Plugin Transport
- JSON-RPC 2.0 over stdio (newline-delimited JSON)
- Bidirectional (kernel ↔ plugin on same channel)
- Streaming via `stream.chunk` / `stream.end` notifications
- Bootstrap env: `AGEZT_PLUGIN_TOKEN`, `AGEZT_PROTOCOL_VERSION`
- In-process plugins implement same interface directly (zero serialization cost)

---

## 14. Build, CI/CD & Distribution

### Build System (Makefile)
```makefile
make gen          # Generate SDK types from JSON Schema contract
make build        # Build all binaries (agezt + agt) into bin/
make install      # Install to GOPATH/bin
make test         # Run all unit tests
make vet          # Run go vet
make lint         # Static checks
make deps-check   # Verify dependency allowlist
make check        # Full CI gate: gen + vet + test + deps-check
make frontend-build  # Build React Web UI → kernel/webui/dist
make frontend-test   # Unit-test Web UI (Vitest)
make webui-e2e    # Drive embedded Web UI in headless browser (Playwright)
make e2e          # Boot real daemon, smoke every core surface
make clean        # Remove build artifacts
```

### CI Pipeline (GitHub Actions)
| Job | Description |
|---|---|
| **test** | Multi-OS matrix (Linux/macOS/Windows): `go vet`, `go test`, `go build` |
| **race** | Race detector on Linux with CGO enabled |
| **e2e** | Runtime E2E gate: boot daemon, smoke all core surfaces |
| **codegen-in-sync** | Regenerate SDK types, fail on diff |
| **multi-arch** | Cross-build: linux/macos/windows × amd64/arm64 |
| **deps-check** | Fail if any `require` not in allowlist |
| **publish-sdks** | Publish SDKs to npm/PyPI/crates.io on tag |

### Distribution
- **Static binaries:** linux/macos/windows × amd64/arm64
- **Docker:** scratch/distroless base images
- **GHCR:** image build + cosign signing + SBOM on tag
- **SDKs:** npm (`@agezt/sdk`), PyPI (`agezt`), crates.io (`agezt`)

### Code Quality Gates
- **gitleaks:** baseline for secret detection
- **staticcheck:** zero warnings enforced
- **govulncheck:** advisory scanning
- **SPDX headers:** on every source file
- **Dependency allowlist:** enforced at CI (currently 1 dep: BLAKE3)

---

## 15. Phase History & Implementation Status

The project has shipped **~781 phases** across the full roadmap from M0 through M781,
merged via **224 pull requests**. Phase numbers map roughly to the nine-milestone roadmap
(ROADMAP §3) plus four major post-roadmap arcs (product layer, multi-agent organism,
customization, console controls), with hardening, observability, and edge-case phases
filling the gaps between major milestones.

### Milestone Summary

| Milestone / Arc | Theme | Status | Phase Range |
|---|---|---|---|
| **M0** | Repository foundation + contracts | ✅ Complete | M0, M0.5 |
| **M1** | MVP: Reasoning + tools + operators | ✅ Complete | ~140 sub-phases (M1.a–M1.gg + batches) |
| **M2–M3** | Operator CLI + Memory-lite | ✅ Complete | 2 phases |
| **M4** | Web UI foundation | ✅ Complete | ~M10–M19 |
| **M5** | Pulse + Channels + Inbox | ✅ Complete | ~M5x–M159 |
| **M6** | Hardening, coding agents, sub-agents | ✅ Complete | ~M160–M299 |
| **M7** | SDKs, tunnels, API, ambient surfaces | ✅ Complete | ~M300–M489 |
| **Hardening arc** | Mutation testing + zero-defect (owner-ratified) | ✅ Complete & closed | M490–M562 |
| **Feature arc** | 11 channels, Flow Studio, React Web UI, 4 SDKs, marketplace, tunnels, voice/STT | ✅ Complete | M563–M587 |
| **Product layer** | Chat (default view), Activity monitor, markdown/widgets/multi-conversation, real-DeepSeek validation (+2 real bug fixes: M597 streaming, M605 denied-tool offers) | ✅ Complete | M588–M605 |
| **Multi-agent organism** | Cockpit dashboard, live run steering, board/standing/skill agent tools, continuous agents, Autonomy view, visual widget kit, humane UI (toasts/skeletons/confirm) | ✅ Complete | M606–M681 |
| **code_exec + voice** | Sandboxed Python/JS/TS execution with package install, Sandbox view, chat voice in/out | ✅ Complete | M682–M692 |
| **Config & routing** | Config Center (schema registry, skill-registered config), Models view + multi-key keyring, per-task model fallback chains | ✅ Complete | M693–M706 |
| **Customization arc** | Live persona editing, prompt library, create/edit schedules/standing orders/skills/world/memory from UI, export/import everything, full snapshot backup/restore | ✅ Complete | M707–M752 |
| **Live console controls** | Heartbeat cadence/proactivity dial, pulse watches, quiet hours, journal hash verify, policy dry-run — all live from the UI | ✅ Complete | M753–M770 |
| **Search & alerts arc** | Search/filter parity (World/Runs/Inbox/Skills), alert history backfill, live nav badge, Cockpit "Needs attention" strip, alert → run deep-links | ✅ Complete | M771–M781 |

### Most Recent Phases (2026-06-10)
- **M781** — Jump from an alert to the run that caused it
- **M780** — "Needs attention" alert strip on the Cockpit
- **M779** — Live unseen-alert badge on the Alerts nav item
- **M778** — Search/filter the skill library
- **M777** — Alert history backfilled from the journal
- **M776** — Inbox conversation search/filter
- **M775** — Runs history filtering
- **M774** — World entity search/filter

### Testing Coverage
- **2,600+ tests** passing across **83 packages** (Go) + 70+ Vitest/component tests +
  Playwright E2E (frontend)
- `go vet` / `gofmt` / `staticcheck` clean
- Cross-builds clean (linux/macos/windows × amd64/arm64)
- `go.mod`/`go.sum` unchanged (single external dep: BLAKE3)

---

## 16. Current State & What's Next

### Current State (2026-06-10)
- **Version:** v1.0.0 (owner chose to keep; tag not cut/pushed yet)
- **Branch:** `main` at M781 (PR #224 merged 2026-06-10 03:29)
- **All 9 planned milestones + 4 post-roadmap arcs shipped** (see §15)
- **Full plugin ecosystem:** 8 providers, 18 tools, 11 channels, MCP bridge
- **All 4 SDKs** (Go, TypeScript, Python, Rust) shipped; publish workflow ready (secret-gated)
- **Web UI** fully functional with ~40 views; Chat is the default, product-grade surface
- **Multi-tenant + mesh federation + marketplace (skills + plugins) + tunnels + voice** complete
- **Security posture:** stdlib-first, keys masked, default-deny, journal hash-chain verifiable from the console

### Outstanding — Owner-Gated (not code work)
1. **GitHub Actions billing is exhausted** — every CI job since ~M585 fails at startup
   ("account payments have failed / spending limit"). All PRs were validated locally
   against the full CI battery and merged under arc authority. Restoring billing and
   re-running CI on `main` is the only step left for the GitHub-side green badge.
2. **SDK publish** — PyPI/npm/crates.io workflow is ready and secret-gated; needs
   `PYPI_API_TOKEN`, `NPM_TOKEN`, `CARGO_REGISTRY_TOKEN` added as repo secrets.
3. **Cut the v1.0.0 release/tag** once CI is green.

### Candidate Future Work (optional)
1. **Alert → channel notifications** — push warning/critical alerts (run failures,
   budget trips, blocked egress, halts) through the existing channel sinks so the
   operator hears about problems without opening the console (natural M782).
2. **Coding-agent delegation** (P6-CODE) — delegate a task to Claude Code/Codex in a
   worktree, stream the diff, open a PR. The largest unbuilt TASKS.md item.
3. **Ambient surfaces** (P7-AMB) — tray app, mobile push. Hardware/platform-bound;
   needs an owner steer.
4. `agt migrate` — deliberately skipped (no real schema migration exists).

### Technical Debt / Known Gaps
- **No `_ARCHIVE/`** directory exists (the spec mentions it but it was not created — the deprecated `agezt.proto` and gRPC files were superseded in-place by `agezt-contract.jsonc`)
- **BUILD-GUIDE.md** references a `docs/` directory that doesn't exist — the spec suite remains in `.project/`
- **ROADMAP.md** mentions `buf`/`protoc` codegen for the gRPC path — this was superseded by the JSON Schema generator (`tools/jsonschemagen/`)
- **STRUCTURE.md** describes a layout from the design phase; the actual layout evolved (e.g., `kernel/` has more packages, plugin directories are organized differently)

---

## Appendix A: Complete Kernel Package Inventory

| Package | Files | Description |
|---|---|---|
| `kernel/event` | 3 | Canonical Event type + Kind constants + test |
| `kernel/journal` | 4 | Append-only JSONL + BLAKE3 chain + fuzz tests |
| `kernel/state` | 2 | Mutable KV state store |
| `kernel/bus` | 2 | In-process event bus (durable-before-publish) |
| `kernel/agent` | 17 | Single-agent tool-loop + streaming + context + offload + vision + panic guard |
| `kernel/runtime` | 26 | Kernel wiring + sub-agents + steer + tools + causation |
| `kernel/governor` | 25 | Routing + budget + pricing (fuzz-tested) + strict pricing + introspection |
| `kernel/edict` | 11 | Policy engine + trust ladder + hard-deny + fuzz tests |
| `kernel/scheduler` | 6 | DAG compile + execute + correlation |
| `kernel/planner` | 8 | Intent → DAG + cost + refine + validate |
| `kernel/approval` | 4 | HITL approval registry + timeout defaults |
| `kernel/anomaly` | 4 | Anomaly detection + auto-halt monitor |
| `kernel/memory` | 8 | Tiered knowledge store + retrieval |
| `kernel/worldmodel` | 10 | World graph + decay + resolve + provenance |
| `kernel/skill` | 16 | Forge lifecycle + shadow-test + auto-quarantine + auto-promote + retrieval pool |
| `kernel/pulse` | 21 | Proactive engine + observers + salience + briefing + disk usage + route matrix |
| `kernel/cadence` | 7 | Cron/cadence engine + DST + crash-safe |
| `kernel/standing` | 7 | Standing orders + runner + cron |
| `kernel/channel` | 10 | Channel abstraction + guard + history + split |
| `kernel/webhook` | 4 | Webhook delivery + deduplication |
| `kernel/catalog` | 11 | Model catalog sync + discovery + fuzz tests |
| `kernel/openaiapi` | 14 | OpenAI API compat + responses + vision + usage + fuzz tests |
| `kernel/restapi` | 7 | REST API + health + metrics + mesh hop limit |
| `kernel/controlplane` | 130+ | Full daemon API (largest single package) |
| `kernel/acp` | 7 | ACP server + client + bounds |
| `kernel/warden` | 10 | Sandbox profiles + capbuf + classification |
| `kernel/creds` | 25 | Credential vault + AWS + SSO + STS + KDF + PBKDF2 |
| `kernel/netguard` | 2 | Network egress control |
| `kernel/redact` | 10 | Secret redaction + fuzz tests |
| `kernel/plugin` | 40+ | Plugin host + spec + proc + pin + frame + reload |
| `kernel/tenant` | 3 | Multi-tenant isolation |
| `kernel/tenantctx` | 2 | Per-tenant context |
| `kernel/meshctx` | 2 | Mesh/federation context |
| `kernel/settings` | 5 | Config registry + schema + store |
| `kernel/reflect` | 2 | Reflection loop |
| `kernel/artifact` | 2 | Artifact CAS store |
| `kernel/stt` | 2 | Speech-to-text |
| `kernel/tunnel` | 4 | Tunnel management |
| `kernel/webui` | 11 | Web UI embedding + route handling |
| `kernel/ulid` | 2 | ULID generation |
| `kernel/convo` | 2 | Conversation management |
| `kernel/assure` | 2 | Runtime assertions |
| `kernel/board` | 2 | Planning board |

## Appendix B: Complete Plugin Inventory

### Provider Plugins (11 + 2 internal)
`anthropic`, `bedrock`, `cohere`, `compat`, `embed`, `google`, `ollama`, `openai`, `openairesponses`, `vertex`, `voice` · internal: `mock`, `internal/*`

### Tool Plugins (27)
`acpagent`, `artifacts`, `boardtool`, `browser`, `codeexec`, `coding`, `config`, `council`, `db`, `fetch`, `file`, `forgetool`, `homeassistant`, `http`, `introspecttool`, `mcptool`, `notify`, `overseertool`, `peer`, `runstool`, `schedule`, `sendmedia`, `shell`, `skilltool`, `standingtool`, `websearch`, `workflowtool`

### Channel Plugins (25)
`chatwebhook`, `dingtalk`, `discord`, `email`, `feishu`, `homeassistant`, `imessage`, `irc`, `line`, `mastodon`, `matrix`, `nextcloudtalk`, `nostr`, `onebot`, `push`, `signal`, `slack`, `sms`, `teams`, `telegram`, `webhook`, `wecom`, `whatsapp`, `whatsappgw`, `zalo`

### External Plugins (1)
`mcpbridge`

## Appendix C: Key Design Documents

| Document | Path | Purpose |
|---|---|---|
| Decisions | `.project/DECISIONS.md` | Supreme authority — all frozen technical decisions |
| Policy | `.project/POLICY.md` | Dependency, packaging, versioning, license rules |
| Contract | `.project/agezt-contract.jsonc` | JSON Schema source of truth |
| Structure | `.project/STRUCTURE.md` | Repository layout specification |
| Roadmap | `.project/ROADMAP.md` | Build order with success tests |
| Build Guide | `.project/BUILD-GUIDE.md` | Single entry point for implementation |
| Implementation | `.project/IMPLEMENTATION.md` | Go architecture + tech choices |
| Vision Master | `.project/AGEZT-VISION-MASTER.md` | Full vision + competitive analysis |
| SPEC-01 through SPEC-16 | `.project/SPEC-*.md` | Complete design specifications |

---

*End of architectural report. This document covers every component, module, and technology
in the Agezt codebase as of 2026-06-20 (branch `main`, ~781+ phases shipped across 224+ PRs, v1.0.0+). For positioning, security, operations, and SDK parity documentation, see `docs/index.md`.*}