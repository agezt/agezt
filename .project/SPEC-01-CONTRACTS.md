# Agezt — Plugin Contracts & Event Schema (SPEC-01)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Active · Domain: github.com/agezt/agezt · License: MIT · Language: English
> This is the **backbone document**. Every other component binds to the contracts defined here. If this is wrong, everything downstream is wrong.

---

## 0. Scope

This document defines:

1. The **wire contract** between the Agezt kernel and out-of-process plugins (gRPC over Unix domain socket / stdio).
2. The **seven plugin interfaces** (`Channel`, `Provider`, `Tool`, `CodingAgent`, `Memory`, `Storage`, `Tunnel`).
3. The **canonical event schema** — the single source of truth that flows through the journal and the internal event bus.
4. The **plugin lifecycle handshake** (registration, health, capability advertisement, shutdown).

Non-goals here: kernel internals (see KERNEL-SPEC), Pulse logic (see PULSE-SPEC), UI surfaces (see UI-SPEC).

---

## 1. Design principles for the contract layer

- **Process isolation first.** A plugin is a separate OS process. A crashing plugin must never take down the kernel. The kernel supervises plugin processes and restarts them per policy.
- **Language-neutral.** Contracts are defined in Protocol Buffers v3. SDKs (Go, TS, Python, Rust) are generated from the same `.proto` files. No hand-written contract drift.
- **Capability-based, not type-based, at runtime.** A plugin advertises *capabilities* on registration. The kernel routes by capability, so one plugin process can satisfy multiple interfaces (e.g. a plugin that is both a Tool and a Memory backend).
- **Everything is an event.** Plugins do not mutate shared state directly. They emit events; the kernel folds events into state. This makes every action auditable and reversible.
- **Streaming-native.** Long-running work (LLM token streams, browser sessions, file transfers) uses server-streaming RPCs, not polling.
- **Backward-compatible evolution.** Proto fields are append-only. Reserved field numbers are never reused. A v0.2 kernel must still talk to a v0.1 plugin for the same major version.

---

## 2. Transport & handshake

### 2.1 Transport
- **Local (default):** gRPC over a Unix domain socket at `${AGEZT_RUNTIME_DIR}/plugins/<plugin-id>.sock`.
- **Remote (mesh, future):** gRPC over TCP with mTLS. Contracts are identical; only the dialer changes. (Mesh-ready from day one, implemented later.)
- **Bootstrap:** the kernel launches the plugin binary with environment variables:
  - `AGEZT_PLUGIN_SOCKET` — path the plugin must listen on
  - `AGEZT_KERNEL_SOCKET` — path the plugin uses to call back into the kernel (emit events, request LLM, read config)
  - `AGEZT_PLUGIN_TOKEN` — a one-time capability token; the kernel rejects any connection without it
  - `AGEZT_PROTOCOL_VERSION` — e.g. `1`

### 2.2 Handshake sequence
```
kernel                                   plugin process
  │  spawn(binary, env)                        │
  │───────────────────────────────────────────▶│  listen(AGEZT_PLUGIN_SOCKET)
  │                                             │
  │  Register()  ◀──────────────────────────────│  (plugin dials kernel, presents token)
  │  → RegisterResponse{accepted, kernel_caps}  │
  │                                             │
  │  Handshake stream opens (bidirectional)     │
  │  ◀── HealthPing / HealthPong ──────────────▶│  (every N seconds)
  │                                             │
  │  ...interface RPCs flow...                  │
  │                                             │
  │  Shutdown(reason)  ─────────────────────────▶│  graceful drain, then exit
```

If a `HealthPong` is missed `max_missed` times, the kernel marks the plugin unhealthy and applies the supervision policy (restart / quarantine / disable).

---

## 3. Core proto: registration & capabilities

```proto
syntax = "proto3";
package agezt.v1;

// ── Plugin registration ───────────────────────────────────────────
message RegisterRequest {
  string plugin_id        = 1;   // stable id, e.g. "telegram", "anthropic"
  string display_name     = 2;
  string version          = 3;   // semver of the plugin
  uint32 protocol_version = 4;   // must match kernel major
  string token            = 5;   // one-time bootstrap token
  repeated Capability capabilities = 6;
  map<string, string> metadata = 7; // author, homepage(TBD), license, etc.
}

message Capability {
  CapabilityKind kind = 1;
  // Free-form capability descriptors the kernel routes on.
  // e.g. for Provider: {"models": "claude-*,gpt-*"}; for Channel: {"direction":"duplex"}
  map<string, string> attributes = 2;
}

enum CapabilityKind {
  CAP_UNSPECIFIED   = 0;
  CAP_CHANNEL       = 1;
  CAP_PROVIDER      = 2;
  CAP_TOOL          = 3;
  CAP_CODING_AGENT  = 4;
  CAP_MEMORY        = 5;
  CAP_STORAGE       = 6;
  CAP_TUNNEL        = 7;
}

message RegisterResponse {
  bool accepted            = 1;
  string reason            = 2; // populated if rejected
  uint32 protocol_version  = 3;
  repeated string kernel_capabilities = 4; // what the kernel offers back (e.g. "llm.request","event.emit")
}

// ── Health ────────────────────────────────────────────────────────
message HealthPing { uint64 seq = 1; int64 sent_unix_ms = 2; }
message HealthPong { uint64 seq = 1; PluginHealth health = 2; }

enum PluginHealth { HEALTH_OK = 0; HEALTH_DEGRADED = 1; HEALTH_FAILING = 2; }

// ── Shutdown ──────────────────────────────────────────────────────
message ShutdownRequest { string reason = 1; uint32 drain_seconds = 2; }
message ShutdownResponse { bool ok = 1; }
```

---

## 4. Kernel-facing services (what plugins can call back into)

Every plugin may call these on the kernel. This is the "SDK surface" most plugin authors actually use.

```proto
service Kernel {
  // Emit an event into the journal + internal bus. The kernel assigns
  // id, sequence, hash-chain link, and timestamp. Plugins never set those.
  rpc EmitEvent(EventEmit) returns (EventAck);

  // Request an LLM completion through the Conduit/Governor. The plugin
  // does NOT pick the model or hold API keys; the Governor decides
  // routing, budget, and fallback. Streaming variant for tokens.
  rpc RequestCompletion(CompletionRequest) returns (stream CompletionChunk);

  // Read effective config for this plugin (merged: defaults < file < env < runtime).
  rpc GetConfig(ConfigRequest) returns (ConfigValue);

  // Read/write this plugin's own scoped key-value state (namespaced, sandboxed).
  rpc StateGet(StateKey) returns (StateValue);
  rpc StateSet(StateEntry) returns (StateAck);

  // Subscribe to events on the internal bus by subject pattern.
  rpc Subscribe(SubscribeRequest) returns (stream Event);
}
```

Key rule: **plugins never receive raw provider API keys and never set journal metadata.** Secrets live in the kernel's Conduit; identity/sequence/hash live in the Journal. This keeps the trust boundary clean and the audit trail unforgeable.

---

## 5. The seven plugin interfaces

Each interface is a gRPC `service` the plugin *implements* (kernel calls plugin). All share the registration/health/shutdown machinery above.

### 5.1 Channel (Telegram, Discord, Slack, WhatsApp, Email, SMS, …)
```proto
service ChannelPlugin {
  // Kernel asks the plugin to start listening; inbound messages are
  // delivered by the plugin via Kernel.EmitEvent with EVT_CHANNEL_INBOUND.
  rpc Start(ChannelStartRequest) returns (ChannelStartResponse);

  // Kernel asks the plugin to deliver an outbound message.
  rpc Send(OutboundMessage) returns (SendReceipt);

  // Optional: presence/typing/read-receipts where the platform supports it.
  rpc Signal(ChannelSignal) returns (SignalAck);

  rpc Stop(ChannelStopRequest) returns (ChannelStopResponse);
}

message OutboundMessage {
  string channel_id   = 1;   // platform conversation/thread id
  string text         = 2;
  repeated Attachment attachments = 3;
  MessagePriority priority = 4; // drives Briefing: URGENT→push now, INFO→batch
  string in_reply_to  = 5;
  string correlation_id = 6;   // ties to the originating task/agent
}
enum MessagePriority { PRIO_INFO=0; PRIO_NORMAL=1; PRIO_IMPORTANT=2; PRIO_URGENT=3; }
```
**Unification rule:** every channel normalizes its native message into a `UnifiedMessage` event (§7) so the Inbox and agents see one shape regardless of platform.

### 5.2 Provider (Anthropic, OpenAI, OpenRouter, Ollama, vLLM, …)
```proto
service ProviderPlugin {
  rpc Complete(ProviderCompletionRequest) returns (stream ProviderChunk);
  rpc Embed(EmbedRequest) returns (EmbedResponse);          // optional
  rpc ListModels(ListModelsRequest) returns (ModelList);
  rpc ReportLimits(LimitsRequest) returns (LimitsResponse); // feeds Governor
}
```
The **Governor** (kernel side) sits in front of all providers: it picks which provider/model serves a request based on subscription priority, budget, rate limits, and fallback chain. The provider plugin just executes against one backend.

### 5.3 Tool (shell, file, http, browser, image, audio STT/TTS, video, …)
```proto
service ToolPlugin {
  rpc Describe(Empty) returns (ToolSchema);          // JSON-schema of inputs/outputs
  rpc Invoke(ToolInvocation) returns (stream ToolEvent); // streams progress + result
  rpc Cancel(CancelRequest) returns (CancelAck);
}
message ToolInvocation {
  string tool_name     = 1;
  bytes  input_json    = 2;        // validated against Describe() schema
  IsolationProfile isolation = 3;  // none|namespace|container|microvm
  string correlation_id = 4;
  uint32 timeout_ms    = 5;
}
```
Native first-party tools MAY additionally be compiled as **WASM** and run in-process for latency; the contract is identical so callers don't care where it runs.

### 5.4 CodingAgent (Claude Code, Codex, Aider, Cursor bridges)
```proto
service CodingAgentPlugin {
  rpc StartSession(CodingSessionRequest) returns (CodingSessionHandle);
  rpc Send(CodingTurn) returns (stream CodingEvent); // diffs, tool calls, output
  rpc Approve(ApprovalDecision) returns (ApprovalAck); // gate dangerous ops
  rpc EndSession(EndSessionRequest) returns (Empty);
}
```
This makes external coding agents **first-class DAG nodes**. A `loop-node` can delegate "implement feature X" to Claude Code and stream its diffs back into the journal. (Neither competitor treats coding agents as a native node type.)

### 5.5 Memory (embedded default, Flint Vector, Redis, …)
```proto
service MemoryPlugin {
  rpc Write(MemoryRecord) returns (MemoryAck);     // facts, summaries, user-model deltas
  rpc Query(MemoryQuery) returns (MemoryResults);  // semantic + keyword + graph
  rpc Forget(ForgetRequest) returns (ForgetAck);   // tombstone, never hard-delete
  rpc Snapshot(SnapshotRequest) returns (SnapshotHandle); // for time-travel
}
```
All memory mutations are **also** emitted as journal events, so memory is reconstructable and revertible. `Forget` is a tombstone event, not a destructive delete — supports audit and undo.

### 5.6 Storage (journal/state/memory persistence drivers)
```proto
service StoragePlugin {
  rpc AppendJournal(JournalBatch) returns (JournalAck); // append-only, returns hash-chain head
  rpc ReadJournal(JournalRange) returns (stream Event);
  rpc KVGet(StateKey) returns (StateValue);
  rpc KVSet(StateEntry) returns (StateAck);
  rpc Capabilities(Empty) returns (StorageCapabilities); // which layers this driver serves
}
```
A driver may serve one layer (just journal) or all three. Default driver = embedded (CobaltDB + JSONL + SQLite). Pluggable = Postgres, etc.

### 5.7 Tunnel (Cloudflare Tunnel, Tailscale, WireRift, mesh)
```proto
service TunnelPlugin {
  rpc Up(TunnelUpRequest) returns (stream TunnelStatus); // exposes a local service
  rpc Down(TunnelDownRequest) returns (Empty);
  rpc Status(Empty) returns (TunnelStatus);
}
```
Used to expose the Web UI / gateway / a specific agent endpoint to the outside world without manual networking.

---

## 6. The DAG node ↔ plugin mapping

The kernel's DAG Scheduler executes a graph; each node resolves to plugin calls:

| Node type | Resolves to | Notes |
|---|---|---|
| `tool-node` | `ToolPlugin.Invoke` | deterministic; isolation profile enforced by Edict |
| `llm-node` | `Kernel.RequestCompletion` → Governor → `ProviderPlugin.Complete` | single reasoning step |
| `loop-node` | bounded iteration of `llm-node` + `tool-node` | the "agentic" part; max-iterations enforced |
| `gate-node` | Edict policy + optional human approval via Channel | trust-ladder checkpoint |
| `agent-node` | `Lifecycle.Spawn` (sub-agent) | parallel workstream, returns a summary event |
| `coding-node` | `CodingAgentPlugin` session | delegate to external coding agent |

A node never calls a plugin directly; it goes through the kernel so every call is journaled and policy-checked.

---

## 7. Canonical event schema (the single source of truth)

Everything that happens is an `Event`. The journal is an append-only log of these; state is a fold over them.

```proto
message Event {
  // ── Identity & ordering (kernel-assigned; plugins MUST NOT set these) ──
  string  id          = 1;   // ULID
  uint64  seq         = 2;   // monotonic per-journal sequence
  int64   ts_unix_ms  = 3;
  string  prev_hash   = 4;   // BLAKE3 of previous event (hash chain)
  string  hash        = 5;   // BLAKE3 of this event's canonical bytes

  // ── Routing ──
  string  subject     = 6;   // e.g. "agent.42.task.completed", "channel.telegram.inbound"
  string  actor       = 7;   // who emitted: kernel | plugin:<id> | agent:<id> | user
  string  correlation_id = 8; // ties an event to a task/agent/session lineage
  string  causation_id   = 9; // the event that caused this one (provenance graph)

  // ── Payload ──
  EventKind kind      = 10;
  bytes   payload     = 11;  // type depends on kind; see EventKind table
  map<string,string> tags = 12; // searchable labels (project, repo, severity, …)
}

enum EventKind {
  EVT_UNSPECIFIED        = 0;

  // Lifecycle
  EVT_AGENT_SPAWNED      = 10;
  EVT_AGENT_SUSPENDED    = 11;
  EVT_AGENT_RESUMED      = 12;
  EVT_AGENT_DIED         = 13;
  EVT_AGENT_CRASHED      = 14;

  // Task / DAG
  EVT_TASK_RECEIVED      = 20; // an intent arrived
  EVT_PLAN_PROPOSED      = 21; // Planner produced a DAG
  EVT_NODE_STARTED       = 22;
  EVT_NODE_COMPLETED     = 23;
  EVT_NODE_FAILED        = 24;
  EVT_TASK_COMPLETED     = 25;

  // Tool / LLM
  EVT_TOOL_INVOKED       = 30;
  EVT_TOOL_RESULT        = 31;
  EVT_LLM_REQUEST        = 32;
  EVT_LLM_TOKEN          = 33; // streamed
  EVT_LLM_RESPONSE       = 34;

  // Channels (unified)
  EVT_CHANNEL_INBOUND    = 40; // payload = UnifiedMessage
  EVT_CHANNEL_OUTBOUND   = 41;

  // Memory / Forge (self-improvement)
  EVT_MEMORY_WRITE       = 50;
  EVT_MEMORY_FORGET      = 51; // tombstone
  EVT_SKILL_CREATED      = 52;
  EVT_SKILL_PATCHED      = 53;
  EVT_SKILL_PROMOTED     = 54;
  EVT_SKILL_QUARANTINED  = 55;
  EVT_SKILL_REVERTED     = 56;

  // Pulse (proactive heart)
  EVT_PULSE_TICK         = 60;
  EVT_OBSERVER_DELTA     = 61; // an observer noticed a meaningful change
  EVT_SALIENCE_SCORED    = 62; // delta scored for importance
  EVT_INITIATIVE_TAKEN   = 63; // system decided to act on its own
  EVT_BRIEFING_SENT      = 64;

  // Governance / control
  EVT_POLICY_DECISION    = 70; // Edict allowed/denied/escalated
  EVT_BUDGET_CONSUMED     = 71; // Governor recorded cost
  EVT_APPROVAL_REQUESTED = 72;
  EVT_APPROVAL_GRANTED   = 73;
  EVT_APPROVAL_DENIED    = 74;
  EVT_HALT               = 75; // dead-man's switch engaged
}

// Unified inbound/outbound message — every channel normalizes to this.
message UnifiedMessage {
  string channel_kind = 1;   // "telegram","email",...
  string channel_id   = 2;   // conversation/thread id
  string sender       = 3;   // platform user id
  string text         = 4;
  repeated Attachment attachments = 5;
  int64  platform_ts_ms = 6;
  map<string,string> platform_meta = 7;
}
```

### 7.1 Why this schema wins
- **`causation_id` provenance graph** → `agt why <event>` walks the chain backwards: which observer → which salience score → which LLM call → which policy decision. Explainability for free.
- **Hash chain** → the audit log is tamper-evident. You can prove no event was altered or removed.
- **Tombstone forgets** → memory is revertible; "undo what the agent learned yesterday" is just replaying up to a sequence.
- **Single shape for channels** → Inbox, agents, and Pulse all consume `UnifiedMessage`; adding a new platform doesn't ripple through the system.

---

## 8. Subject naming convention (internal bus routing)

Hierarchical, dot-separated, wildcard-subscribable (NATS-style):

```
agent.<agent_id>.<lifecycle|task|tool>.<verb>
channel.<kind>.<inbound|outbound>
task.<task_id>.<node_id>.<started|completed|failed>
pulse.<observer|salience|initiative|briefing>.<verb>
memory.<write|forget>
skill.<created|patched|promoted|quarantined|reverted>
policy.<decision>
budget.<provider>.<consumed|exceeded>
system.<halt|resume|anomaly>
```

Examples a plugin or the UI might subscribe to:
- `pulse.>` — everything the proactive heart does (Live Monitor)
- `channel.telegram.inbound` — drive a reactive agent
- `task.*.*.failed` — surface failures anywhere
- `skill.>` — Memory Explorer's version feed
- `budget.>` — cost dashboard

---

## 9. Versioning & compatibility rules

- `protocol_version` is a single integer = the **major** contract version. Kernel rejects mismatched majors at `Register`.
- Within a major: proto fields are **append-only**; removed fields are `reserved`; enum values are never renumbered.
- SDKs are generated and published per major version. A plugin built against major `1` works with any kernel major `1`.
- Event `kind` values are stable forever once shipped; deprecated kinds are documented, never reused.

---

## 10. Open questions (to resolve before freezing this spec)

1. **Token budget unit for Governor** — normalize to USD-cost, to provider-native tokens, or to an abstract "credit"? (Affects `EVT_BUDGET_CONSUMED` payload.)
2. **Attachment storage** — inline bytes for small, content-addressed blob store for large? Threshold?
3. **Sub-agent result contract** — does `agent-node` return a free-form summary, or a typed result schema declared at spawn?
4. **WASM tool ABI** — adopt the WASI preview-2 component model, or a thinner custom ABI for first-party tools?
5. **Cross-major migration** — do we ship a contract-translation shim, or require plugin rebuilds on major bumps?

These don't block KERNEL-SPEC; they affect payload details we can finalize during F0/F1.

---

*Next: KERNEL-SPEC (the 6 kernel responsibilities in depth), then PULSE-SPEC (the proactive heart). This contract document is the dependency root for both.*
