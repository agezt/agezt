# Agezt — Implementation Specification (IMPLEMENTATION.md)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> Depends on: SPEC-01..07
> Translates the specs into a concrete Go (+ TS frontend) build: repository layout, module boundaries, technology choices, data formats, and a phase-by-phase implementation plan. Philosophy: single static Go binary, near-zero dependencies, stdlib-first (#NOFORKANYMORE).

---

## 1. Technology choices & justification

### 1.1 Kernel — Go
- **Why Go:** single static binary, trivial cross-compilation, goroutines map naturally to the lightweight actor model, strong stdlib (net, crypto, encoding), fast startup (Jarvis must boot instantly on a $5 VPS).
- **Go version:** latest stable; modules; `CGO_ENABLED=0` for a truly static binary (pure-Go deps only).
- **Dependency policy:** stdlib-first. External deps allowed only where stdlib is genuinely insufficient and a pure-Go, well-maintained option exists. Every dep is justified in `DEPENDENCIES.md`. No CGO unless an isolated optional build tag.

### 1.2 Plugin transport — gRPC over Unix socket / stdio
- **Why gRPC:** language-neutral contracts → polyglot SDKs auto-generated; streaming is first-class; mature. Protobuf is the single contract source.
- **go-plugin pattern:** HashiCorp-style subprocess + handshake + health, adapted. (We implement our own thin host to keep deps minimal and the contract ours; the *pattern* is borrowed, not necessarily the library.)

### 1.3 Storage — embedded default, pluggable
- **Default:** CobaltDB (the team's embedded B+Tree KV) for state + skill blobs; JSONL files for the journal; an embedded inverted index for FTS; an embedded vector index for semantic memory. All pure-Go, zero external services.
- **Pluggable:** Postgres (journal+state), Redis (cache), Flint Vector (semantic). Behind the `StoragePlugin`/`MemoryPlugin` contracts.

### 1.4 Crypto
- BLAKE3 (journal hash chain, content-addressing), AES-256-GCM / ChaCha20-Poly1305 (secret-at-rest), pure-Go implementations. (Team's Kronos crypto experience.)

### 1.5 CLI/TUI
- Cobra-style command tree (or a minimal custom router to avoid deps — decision in §9), Bubble Tea for the TUI, Lip Gloss for styling.

### 1.6 Frontend
- React 19 + TypeScript + Vite, Tailwind 4, shadcn/ui, React Flow, generated `agezt-sdk-ts`. State via React Query over the event stream; no localStorage for authoritative state (it streams from the kernel).

### 1.7 SDKs
- Generated from `.proto`: `agezt-sdk-go`, `-ts`, `-py`, `-rust`. Each ships a base/serve helper so a plugin is ~20 lines (SPEC-04 §0.3).

---

## 2. Repository layout (monorepo)

```
agezt/
├── cmd/
│   ├── agezt/            # main binary: kernel + native plugins + CLI entrypoint
│   └── agt/              # thin CLI shim (or alias of agezt cli)
├── kernel/
│   ├── lifecycle/        # supervisor, agent actor runtime (goroutine+mailbox)
│   ├── journal/          # append-only log, hash chain, projections, snapshots
│   ├── bus/              # internal event bus (subjects, pub/sub/req-reply/stream)
│   ├── pluginhost/       # subprocess mgmt, handshake, health, routing
│   ├── scheduler/        # DAG compile + execute (Planner integration)
│   ├── planner/          # intent → DAG meta-agent
│   ├── edict/            # policy engine + trust ladder
│   ├── conduit/          # provider/tool registry + Governor (budget/fallback)
│   ├── chronos/          # cron/interval/event/condition/webhook scheduler
│   ├── pulse/            # heartbeat, observers host, salience, initiative, briefing
│   ├── memory/           # memory tiers, world-model graph, retrieval pipeline
│   ├── forge/            # skill lifecycle state machine, shadow-test
│   ├── reflect/          # reflection loop
│   ├── warden/           # isolation profiles (namespaces/cgroups/seccomp/container)
│   ├── conf/             # config precedence, redaction
│   └── controlplane/     # halt/why/attach, socket server
├── contracts/
│   ├── proto/            # *.proto — the single contract source (SPEC-01)
│   └── gen/              # generated stubs per language (checked-in or built)
├── plugins/             # first-party plugins (each its own module/binary)
│   ├── channel-telegram/  channel-discord/  channel-slack/  channel-whatsapp/
│   ├── channel-signal/    channel-email/    channel-sms/     channel-matrix/
│   ├── channel-teams/     channel-homeassistant/ channel-webhook/
│   ├── provider-anthropic/ provider-openai/ provider-openrouter/
│   ├── provider-ollama/    provider-vllm/    provider-openai-compat/
│   ├── tool-shell/  tool-file/  tool-http/  tool-browser/
│   ├── tool-image/  tool-audio/ tool-video/ tool-docgen/ tool-data/ tool-search/
│   ├── coding-claudecode/ coding-codex/ coding-aider/
│   ├── memory-flintvector/ memory-redis/
│   ├── storage-postgres/
│   ├── tunnel-cloudflare/ tunnel-tailscale/ tunnel-wirerift/
│   └── mcp-bridge/        # adapts any MCP server into Tool capabilities
├── sdk/
│   ├── go/  ts/  py/  rust/
│   └── create-agezt-plugin/   # scaffolder
├── web/                 # React 19 + Vite frontend (Flow Studio, Inbox, Monitor, Memory)
├── gateway/             # remote transport, auth, static asset host, OpenAI-compat API
├── docs/                # the specs + guides
└── DEPENDENCIES.md
```

Native (first-party) plugins can be compiled **into** `cmd/agezt` (embedded, for the single-binary promise) and/or built as standalone binaries; build tags control which. Third-party plugins always run as separate processes.

---

## 3. Module boundaries & key interfaces (Go)

### 3.1 The event/journal core (everything depends on this)
```go
// journal: the single source of truth
type Journal interface {
    Append(ctx, []Event) (head Hash, err error)   // durable-before-ack
    Read(ctx, Range) (iter EventIter, err error)
    Verify(ctx) (firstBreak *uint64, err error)   // hash-chain integrity
    Snapshot(ctx, seq uint64) (SnapshotRef, error)
}

// bus: live fanout; publishes only after durable append
type Bus interface {
    Publish(Event)
    Subscribe(pattern string) (<-chan Event, CancelFunc)
    Request(ctx, subject string, payload []byte) ([]byte, error)
}
```

### 3.2 Agent runtime
```go
type Supervisor interface {
    Spawn(spec AgentSpec) (id string, err error)
    Suspend(id string) error
    Resume(id string) error
    Kill(id, reason string) error
}
// Agent = goroutine consuming a bounded mailbox; state folded from journal by correlation_id.
```

### 3.3 Plugin host
```go
type PluginHost interface {
    Discover(dir string) ([]Manifest, error)
    Launch(Manifest) (PluginHandle, error)   // subprocess + handshake + health
    Resolve(cap CapabilityQuery) (PluginHandle, error) // capability routing
}
```

### 3.4 Scheduler + Planner
```go
type Planner interface { Plan(ctx, Intent, Inventory, WorldCtx) (DAG, error) }
type Scheduler interface { Execute(ctx, DAG) (Result, error) } // topo + bounded parallel
```

### 3.5 Governance & conduit
```go
type Edict interface { Decide(ctx, Action) Decision } // allow|deny|escalate, journaled
type Governor interface { Route(ctx, CompletionReq) (ProviderHandle, Budget, error) }
```

The dependency arrow points inward to journal/bus; nothing in the kernel mutates state except by emitting events.

---

## 4. Data formats

- **Journal on disk (default):** segmented JSONL files (`journal/000001.jsonl` …) with a sidecar index (offset → seq, hash). Rotation by size; snapshots in `snapshots/`. Canonical event encoding (deterministic field order) for hashing.
- **Events on the wire:** protobuf (SPEC-01).
- **Config:** YAML (`~/.agezt/config.yaml`) with precedence defaults < file < env (`AGEZT_*`) < flags.
- **Policy:** YAML (`edict.yaml`).
- **Plugin manifest:** `plugin.yaml`.
- **Skills:** YAML front-matter + body, content-addressed blobs.
- **Workspace/runtime:** `~/.agezt/{config.yaml, journal/, snapshots/, plugins/, secrets.enc, workspace/, runtime/sockets/}`.

---

## 5. Concurrency & performance model

- **Bounded worker pool** for DAG node execution (default 8, configurable) — mirrors Hermes's ThreadPoolExecutor cap but in goroutines.
- **Bounded mailboxes** with backpressure — no unbounded queues, no silent drops.
- **Streaming everywhere** — LLM tokens, tool progress, browser sessions stream via server-stream RPC; the UI renders incrementally.
- **Context compression** (`memory/`): monitor token usage; protect first N + last M turns; summarize the middle via a cheap model call; sanitize orphaned tool-call/result pairs.
- **Snapshot cadence** tuned so boot replay stays sub-second at expected scale.
- **Adaptive Pulse cadence** to avoid hot loops on small hosts.

---

## 6. Reliability

- **Durable-before-ack:** bus publish only after journal append; subscribers never see unpersisted events.
- **Crash recovery = boot:** the same replay path handles cold start and crash restart.
- **Supervised restarts** with exponential backoff + jitter; quarantine after `MaxRestarts/Window`.
- **Plugin crash isolation:** subprocess death never propagates to the kernel.
- **Anomaly auto-halt** wired to spend/rate/error/repetition detectors.

---

## 7. Phase-by-phase build (full project, not MVP)

> Each phase ends with a working, demoable slice. Contracts from SPEC-01 are frozen first; everything binds to them.

### Phase 0 — Contracts & Kernel Core
- Freeze `.proto` (SPEC-01); generate `sdk-go`.
- `journal` (JSONL + BLAKE3 chain + projections + snapshots) + `bus`.
- `lifecycle` (agent actor runtime + supervisor) + `controlplane` (halt/why/attach over socket).
- `pluginhost` (handshake, health, routing) with a trivial echo plugin.
- **Demo:** spawn an agent, emit/replay events, `agt journal verify`, `agt halt`.

### Phase 1 — Reasoning & Tools (single task end-to-end)
- `conduit` + `Governor` v1; `provider-anthropic` + `provider-ollama`.
- `scheduler` + `planner` (intent → DAG); node types tool/llm/loop/gate.
- `tool-shell`, `tool-file`, `tool-http`, `tool-browser`.
- `edict` v1 (policy + trust ladder) + `warden` namespace profile.
- CLI `agt run`, basic TUI.
- **Demo:** `agt run "fetch X, summarize, write report.md"` runs a real DAG with policy + budget.

### Phase 2 — Memory, World Model & Forge
- `memory` tiers + retrieval pipeline + world-model graph; `memory-flintvector` plugin.
- `forge` skill lifecycle (draft→shadow→active→quarantine→revert) + shadow-test.
- Context compression.
- **Demo:** system creates a skill after a complex task, shadow-tests, promotes; `agt skill history/revert`.

### Phase 3 — Pulse (the proactive heart)
- `pulse` heartbeat + observers host; observers: repo/CI + system-health.
- Salience filter (rules + cheap LLM) + the Quiet/Balanced/Chatty dial.
- Initiative (inform/ask first; autonomous act gated by trust ladder) + Briefing composer.
- `chronos` (cron/event/condition/webhook) + standing orders.
- **Demo:** unprompted, it detects a broken CI and briefs via... (Telegram lands in P4; P3 briefs to CLI/log first).

### Phase 4 — Channels & Unified Inbox
- `channel-telegram` (duplex) first, then email/whatsapp/discord/slack/signal/sms/matrix/teams/homeassistant/webhook.
- Unified Inbox in the Web UI; outbound priority/affordances; cross-channel handoff.
- **Demo:** Telegram in → agent → Telegram out; Pulse briefs to Telegram; approve via inline buttons.

### Phase 5 — Web UI: Flow Studio + Live Monitor
- `web/`: Flow Studio (design/run/replay), Live Monitor (agents/pulse/cost/traces/health), Memory Explorer.
- `gateway` (WS/gRPC-Web, auth, static host) + `agezt-sdk-ts`.
- **Demo:** build a DAG visually, run it, watch it light up live; inspect cost/traces; revert a skill in the UI.

### Phase 6 — Warden hardening, multi-agent, simulation
- `warden` container + microvm profiles; egress allow-listing; seccomp.
- True multi-agent parallelism (agent-node spawning sub-agents at scale).
- `coding-claudecode`/`coding-codex`/`coding-aider` (coding-node).
- Dry-run/simulation for risky DAGs.
- **Demo:** delegate "fix CI" to Claude Code inside a sandbox, review diff, open PR — one DAG.

### Phase 7 — Tunnels, full SDK, ambient, OpenAI-compat API
- `tunnel-cloudflare/tailscale/wirerift`; OpenAI-compatible API in `gateway`.
- `sdk-ts/py/rust` + `create-agezt-plugin` published.
- `mcp-bridge` plugin (any MCP server → tools).
- Ambient: voice (local Whisper STT + TTS), tray app, mobile push, email-native.
- **Demo:** expose UI via Cloudflare tunnel; drive Agezt from an OpenAI-compatible client; talk to it by voice.

### Phase 8 — Reflection, marketplace, polish
- `reflect` loop (recalibrate salience/initiative within ladder; propose trust changes).
- Marketplace (signed, content-addressed plugins/skills/workflows/standing-orders).
- Docs, installers, `agt doctor`, skins, i18n.

### Phase 9 — Mesh & migration
- Federated mesh (gossip/SWIM, federated bus, agent migration) over the mesh-ready contracts.
- Multi-tenant mode.
- `agt migrate openclaw|hermes` (import settings/memories/skills).

---

## 8. Testing & quality

- **Unit:** every kernel module; table-driven Go tests.
- **Contract tests:** a conformance suite each plugin SDK must pass (the `.proto` behaviors).
- **Replay/property tests:** fold determinism — replaying a journal yields identical projections; hash-chain invariants.
- **Integration:** end-to-end DAG runs with fake providers/tools; golden traces.
- **Security tests:** injection corpus (channel/web/file/MCP) must not trigger privileged actions; sandbox escape attempts; redaction coverage.
- **Soak/chaos:** kill plugins/agents mid-task; verify recovery and no data loss; anomaly auto-halt fires.
- **Target:** high coverage on the kernel core; CI gates on contract + security suites.

---

## 9. Open implementation decisions

1. CLI framework: Cobra (deps) vs a minimal custom router (zero-dep) — leaning zero-dep to honor the philosophy.
2. Generated stubs: check in `contracts/gen` vs build-time generation (reproducibility vs repo size).
3. Embedded vector index: build minimal pure-Go HNSW vs always delegate to Flint Vector plugin for semantic memory.
4. Native plugins embedded vs always-subprocess: per-plugin build tag policy.
5. microVM backend selection and whether it stays an optional, separately-built component to protect the zero-dep core.
6. Budget unit (USD/tokens/credits) — resolve before Governor lands in P1.

---

*Next: TASKS.md (granular, checklist-style task breakdown per phase/module), then BRANDING.md, README.md, PROMPT.md.*
