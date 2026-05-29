# Agezt — Plugin Interfaces Specification (SPEC-04)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> Depends on: SPEC-01 (Contracts), SPEC-02 (Kernel)
> Defines, in depth, all seven plugin interfaces: behavior, lifecycle, error model, first-party implementations, and the SDK author experience for each.

---

## 0. Common plugin model (applies to all seven)

### 0.1 Anatomy of any plugin
- A standalone binary (any language) launched as a subprocess by the Plugin Host.
- Implements one gRPC service (its interface) + the shared `Register/Health/Shutdown` (SPEC-01 §3).
- Calls back into the kernel via the `Kernel` service (emit events, request LLM, read config/state, subscribe).
- Declares capabilities at registration; the kernel routes by capability.
- Sandboxed at the process level by an isolation profile in its manifest.

### 0.2 Error model (uniform)
Every RPC returns a structured `PluginError` rather than relying on gRPC status alone:
```proto
message PluginError {
  ErrorClass class = 1;     // TRANSIENT | INVALID_INPUT | UNAUTHORIZED | UNAVAILABLE | INTERNAL | CANCELLED
  string code      = 2;     // plugin-specific stable code
  string message   = 3;     // human-readable
  bool   retryable = 4;
  uint32 retry_after_ms = 5;
}
```
- `TRANSIENT`/`retryable` → kernel applies node retry policy.
- `UNAVAILABLE` repeatedly → health degrades → supervision policy.
- Errors are journaled (`EVT_NODE_FAILED` / tool error events) so failures are auditable, never silent.

### 0.3 SDK author experience (the "20-line plugin" promise)
For each interface, the SDK provides a base struct that handles registration, health, event plumbing, and config. The author implements only the interface methods.
```go
// Go SDK shape (illustrative, same pattern in TS/Py/Rust)
type MyTool struct{ agezt.ToolBase }
func (m *MyTool) Describe(ctx) (agezt.ToolSchema, error) { ... }
func (m *MyTool) Invoke(ctx, in agezt.ToolInvocation, emit agezt.Emitter) error { ... }
func main() { agezt.ServeTool(&MyTool{}) } // handles everything else
```

### 0.4 Capability negotiation
A plugin advertises fine-grained attributes; the kernel and Planner read them to decide routing and to know what the plugin can do without trial-and-error (e.g. a Provider advertising `modalities: text,vision,embeddings` and `context: 200000`).

---

## 1. Channel plugins

### 1.1 Responsibility
Bridge an external messaging surface (Telegram, Discord, Slack, WhatsApp, Signal, Email, SMS, Matrix, Teams, Home Assistant, …) to Agezt, in both directions, normalized to `UnifiedMessage`.

### 1.2 Lifecycle
- `Start` → plugin connects to the platform, begins listening. Inbound messages are pushed to the kernel via `EmitEvent(EVT_CHANNEL_INBOUND, UnifiedMessage)`.
- `Send` → kernel hands an `OutboundMessage`; plugin delivers; returns `SendReceipt` (platform message id for later edit/dedupe).
- `Signal` → typing, presence, read receipts, button/affordance interactions where supported.
- `Stop` → graceful disconnect.

### 1.3 Normalization contract
Every channel MUST map native concepts to the unified model:
- conversation/thread → `channel_id`
- native message → `UnifiedMessage.text` + `attachments`
- platform-specific data preserved in `platform_meta` (never lost, but not required by core).
This is why adding a 20th channel doesn't ripple: agents, Inbox, and Pulse only ever see `UnifiedMessage`.

### 1.4 Inbound → trigger
An inbound event can spawn a `reactive` agent (Planner turns the message into an intent → DAG) or feed an existing `resident` agent/session. Session continuity uses `correlation_id`, enabling cross-channel handoff (start on Telegram, continue in CLI).

### 1.5 Outbound priority & affordances
`OutboundMessage.priority` (INFO→URGENT) lets the Briefing composer drive delivery (push now vs batch). Where the platform supports interactive elements, approval requests render as inline approve/deny buttons → a `Signal` callback resolves `EVT_APPROVAL_GRANTED|DENIED`.

### 1.6 First-party channels (full project)
Telegram, Discord, Slack, WhatsApp, Signal, Email (IMAP/SMTP), SMS (Twilio), Matrix, Microsoft Teams, Home Assistant, plus a generic Webhook channel. Each is an independent plugin process; a crash in WhatsApp never affects Telegram.

### 1.7 Security notes
- Channels are an **injection surface** (untrusted inbound content). Inbound text is treated as data, never as kernel instructions. Any instruction-like content from a channel that would trigger a privileged action goes through Edict and (for send-on-behalf, purchases, irreversible ops) explicit approval.
- Outbound "send on my behalf" is an escalate-by-default action in Edict.

---

## 2. Provider plugins

### 2.1 Responsibility
Execute LLM/embedding requests against one backend (Anthropic, OpenAI, OpenRouter, Ollama, vLLM, z.ai/GLM, Kimi, MiniMax, Vercel AI Gateway, custom OpenAI-compatible).

### 2.2 Methods
- `Complete` (stream) — chat/completion with tool-calling passthrough; streams `ProviderChunk` (tokens, tool-call deltas, usage).
- `Embed` — vectors for memory/RAG (optional capability).
- `ListModels` — advertise available models + attributes (context window, modalities, pricing hints).
- `ReportLimits` — current rate-limit/quota posture → feeds the Governor.

### 2.3 The Governor relationship (critical)
Providers do **not** decide routing or hold the budget logic. The Governor (kernel, SPEC-02 §6.2) chooses which provider/model serves each request: subscription-first → cost → latency, with budget/limit checks and a fallback chain. A provider plugin is a thin, swappable executor against one backend.

### 2.4 Auth modes
- **subscription** (e.g. a Claude/ChatGPT plan via OAuth) — Governor prefers these (already paid).
- **api-key** — pay-per-use; Governor tracks cost.
- **local** (Ollama/vLLM) — zero marginal cost; default fallback floor so the system never fully stalls when paid quota is exhausted.
Credentials live in the kernel's Conduit; provider plugins receive scoped, short-lived auth at call time, never raw long-lived keys on disk.

### 2.5 Prompt caching & features
Providers advertise feature support (prompt caching, vision, tool use, JSON mode) via attributes; the kernel uses them to optimize (e.g. cache system prompts on supporting providers).

---

## 3. Tool plugins

### 3.1 Responsibility
Perform a concrete, deterministic-ish action: shell, file I/O, HTTP, browser automation, image gen, audio STT/TTS, video gen, document creation (docx/pdf/xlsx), data analysis, etc.

### 3.2 Methods
- `Describe` → JSON-schema of inputs/outputs (the Planner reads this to wire the tool into a DAG; the LLM uses it for tool-calling).
- `Invoke` (stream) → executes within an `IsolationProfile`; streams progress `ToolEvent`s and a final result. Long tools (browser sessions, video render) stream incremental status.
- `Cancel` → cooperative cancellation (also triggered by `agt halt`).

### 3.3 Isolation profiles (enforced by kernel/Warden, see SPEC-06)
`none | namespace | container | microvm`. The caller/Edict selects; the tool runs accordingly. A shell tool defaults to at least `namespace`; an untrusted third-party tool may require `container`.

### 3.4 In-process WASM fast path
First-party tools may also ship as WASM and run in-process for latency. The `ToolPlugin` contract is identical, so a `tool-node` doesn't know or care whether the browser tool is a subprocess or an in-process WASM module — the Plugin Host decides per config/policy.

### 3.5 First-party tools (full project)
- **shell** — command execution (sandboxed, persistent-shell option per SPEC-02 agents).
- **file** — read/write/search/patch within scoped paths.
- **http** — fetch/POST with policy (domain allow/deny via Edict).
- **browser** — Playwright/CDP: navigate, read, act, screenshot, extract (the "browser use" requirement). Treated as a high-privilege tool; sensitive domains escalate.
- **image** — generation + analysis (vision).
- **audio** — STT (local Whisper) + TTS (local + premium providers).
- **video** — generation via pluggable backends.
- **docgen** — docx/pdf/xlsx/pptx artifact production.
- **data** — CSV/parquet analysis, charting.
- **search** — web/x search.

### 3.6 Artifacts
Tool outputs that are files become **artifacts**: content-addressed (BLAKE3), referenced in events (`RawRef`), retrievable via Storage. Large outputs are never inlined in events (SPEC-01 §10.2 threshold). Artifacts are versioned and survive in the journal lineage.

---

## 4. CodingAgent plugins

### 4.1 Responsibility
Bridge an external autonomous coding agent (Claude Code, Codex, Aider, Cursor, OpenCode, or Agezt's own coding loop) so it becomes a **first-class DAG node** (`coding-node`). This is a differentiator: neither competitor treats coding agents as a native node type.

### 4.2 Methods
- `StartSession` → spin up a coding session in a working directory (often a git worktree checkpoint, SPEC-06).
- `Send` (stream) → send a turn ("implement feature X"); stream back `CodingEvent`s: tool calls, diffs, command output, status.
- `Approve` → gate dangerous operations the coding agent wants (e.g. running migrations, force-push) → routed through Edict.
- `EndSession`.

### 4.3 Integration
- A `loop-node` or `agent-node` delegates an implementation task to a coding agent; diffs and outputs stream into the journal, so the work is auditable and the result is an artifact (a branch/PR).
- Edict gates: `merge`, `force-push`, `delete-branch`, running arbitrary infra commands → escalate by default.
- The coding agent runs inside a checkpoint/worktree so changes are isolated and revertible until promoted.

### 4.4 Use with your portfolio
This is exactly the surface to drive your own coding-agent workflows: Agezt's Planner can spawn "fix CI" → delegate to Claude Code → review diff via an `llm-node` → open PR → brief you. The whole chain is one DAG, fully journaled.

---

## 5. Memory plugins

> Full data model and the world model live in SPEC-05. This section is the *interface*.

### 5.1 Responsibility
Persist and retrieve durable knowledge: facts, summaries, user-model deltas, skills, and the world-model graph. Backends: embedded default, Flint Vector (semantic), Redis (cache), or a composite.

### 5.2 Methods
- `Write(MemoryRecord)` → store a fact/summary/relation/skill. Also emits `EVT_MEMORY_WRITE`.
- `Query(MemoryQuery)` → hybrid retrieval: semantic (vector) + keyword (FTS) + graph traversal. Returns ranked records with provenance.
- `Forget(ForgetRequest)` → **tombstone**, never hard-delete. Emits `EVT_MEMORY_FORGET`. Supports audit + undo.
- `Snapshot` → point-in-time handle for time-travel ("what did it believe last Tuesday").

### 5.3 Invariants
- Every memory mutation is **also** a journal event → memory is reconstructable by replay and revertible.
- Records carry `source_event` provenance so `agt why` can explain "you know this because of event X."
- No destructive deletes; forgetting is a tombstone the query layer respects.

---

## 6. Storage plugins

### 6.1 Responsibility
Provide the durable substrate for one or more of: **journal** (append-only event log), **state KV** (projections/scoped plugin state), **memory** (records/vectors). A driver declares which layers it serves.

### 6.2 Methods
- `AppendJournal(batch)` → append events, return new hash-chain head. MUST be atomic and ordered.
- `ReadJournal(range)` (stream) → replay for boot/recovery/time-travel.
- `KVGet/KVSet` → state layer.
- `Capabilities` → which layers + guarantees (durability, transactionality, max value size).

### 6.3 Default vs pluggable
- **Default (zero-dep, single binary):** CobaltDB (embedded B+Tree) for KV + JSONL for journal + an embedded index for memory. Works on a $5 VPS with no external services.
- **Pluggable:** Postgres (journal+state), Redis (cache/state), Flint Vector (memory). The same contract; only the driver changes. An operator scales out by swapping drivers, not rewriting.

### 6.4 Consistency contract
The journal driver MUST guarantee: append atomicity, total order per journal, and durable-before-ack (the kernel publishes to the bus only after a durable append). Drivers that can't meet this can't serve the journal layer (but may serve KV/memory).

---

## 7. Tunnel plugins

### 7.1 Responsibility
Expose a local Agezt surface (Web UI, gateway endpoint, a specific agent's HTTP endpoint) to the outside world without manual networking — or join a private mesh.

### 7.2 Methods
- `Up(request)` (stream status) → establish the tunnel; stream connection state + public URL/hostname (TBD domain handling).
- `Down` → tear down.
- `Status` → current state, addresses, health.

### 7.3 First-party tunnels (full project)
- **Cloudflare Tunnel** — public hostname for the Web UI.
- **Tailscale** — private mesh access from your devices.
- **WireRift** — your own self-hosted tunnel.
- **Karadul mesh** — node-to-node connectivity for the future federated mesh (SPEC-02 §1.4).

### 7.4 Security
- Exposing a surface is an **escalate** action in Edict (it expands audience). Default-deny; explicit user approval to open a public tunnel.
- Tunnel plugins never expose the control-plane socket publicly; only designated surfaces (UI/gateway) with their own auth.

---

## 8. Cross-interface concerns

### 8.1 Composite plugins
One plugin process may implement multiple interfaces (e.g. a service that is both a Tool and a Memory backend) by advertising multiple capabilities. The kernel routes per capability independently.

### 8.2 Versioning
Per SPEC-01 §9: append-only proto evolution, integer major = protocol version, SDKs per major. A plugin built for major 1 runs on any kernel major 1.

### 8.3 Discovery & marketplace (future)
Plugins are content-addressed and signed. The marketplace (SPEC-future) distributes plugins, skills, standing-order templates, and Flow Studio workflows with type-safe, versioned, verifiable artifacts. `agt plugin add <ref>` resolves local path, URL, or marketplace ref.

### 8.4 The Chronos scheduler (where it lives)
Chronos (cron/interval/event/condition/webhook triggers) is a kernel-resident component, not a plugin, because it must integrate with the supervisor to spawn `scheduled` agents and survive restarts (jobs reload from the journal). Triggers:
- **time** — cron/interval.
- **event** — bus subject match.
- **condition** — a predicate over memory/state (e.g. "if budget < X").
- **webhook** — inbound HTTP (via gateway/tunnel).
A Standing Order is typically a Chronos-kept configuration binding observers + initiative scope (SPEC-03 §7).

---

## 9. Open questions

1. Coding-agent result typing — free-form summary vs typed `CodingResult` (PR ref, files changed, test status)? (Pairs with SPEC-01 §10.3.)
2. Tool schema standard — JSON-Schema draft version; alignment with MCP tool schemas for interop.
3. MCP bridge as a special Tool/Provider plugin vs native kernel support — recommend a built-in MCP client plugin that adapts any MCP server into Tool capabilities.
4. Composite-plugin resource accounting — how budget/limits attribute across interfaces in one process.

---

*Next: SPEC-05 (Memory & World Model) — the data model behind Memory plugins, the context graph, skills/Forge, and reflection. Then SPEC-06 (Security, Sandbox & Warden) and SPEC-07 (UI & Surfaces).*
