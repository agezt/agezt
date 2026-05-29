# Agezt — Provider Ecosystem: Catalog Sync, Tool-Calling Normalization & ACP (SPEC-15)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Draft v0.1 · Language: English · Domain/Repo: TBD · License: MIT
> Depends on: SPEC-04 (Provider/CodingAgent), SPEC-10 (LLM/routing), SPEC-02 (Governor)
> Defines three provider-side concerns that must be first-class: synchronizing the provider/model catalog from external sources (models.dev-class), normalizing tool-calling across every provider dialect, and Agent Client Protocol (ACP) integration in both directions.

---

## 1. Provider & model catalog sync

### 1.1 Problem
There are 100+ providers and thousands of models, changing constantly (new models, prices, context windows, capabilities). Hardcoding them is hopeless. Agezt must **sync** a live catalog and keep the Governor's routing/price tables current.

### 1.2 Catalog source
- A **catalog sync** component pulls from external registries (models.dev-class sources) on a schedule (Chronos) and on demand (`agt provider sync`).
- The catalog records per model: id, provider, modalities (text/vision/audio), context window, tool-use support, JSON-mode, prompt-caching, reasoning support, and **pricing** (input/output/cached per Mtok → converted to USD-microcents, DECISIONS C1).
- Local/self-hosted models (Ollama/vLLM) are **auto-discovered** from the running endpoint and merged into the same catalog (queried at runtime, no external source needed).
- The synced catalog feeds the Governor's price table and the Planner's capability inventory.

### 1.3 API-key / credential sync
- Credentials can be **imported/synced** from existing sources the user already has: environment, Claude Code / Codex credential files (auto-discovery), `.env`, or a user-provided list — then stored in the Conduit (encrypted, SPEC-06).
- Auth modes per provider: subscription (OAuth/PKCE), api-key, or local (none). The Governor prefers subscriptions, respects limits, falls back (DECISIONS C2).
- `agt provider list` shows synced providers, their models, auth status, and limit posture; `agt provider use <model>` pins a default.

### 1.4 Events
`EVT_CATALOG_SYNCED` (added to the schema) records source, counts, and changes (new/removed/repriced models) — visible in the system changelog (SPEC-08).

---

## 2. Tool-calling normalization (provider-agnostic, mandatory)

### 2.1 Problem
Every provider speaks a different tool-calling dialect:
- **OpenAI** — `tools`/`tool_calls`, JSON arguments, `tool` role for results.
- **OpenAI-compatible** (many local/3rd-party) — mostly OpenAI shape, with quirks.
- **Anthropic** — `tool_use`/`tool_result` content blocks.
- **Gemini** — `functionDeclarations` / `functionCall` / `functionResponse`.
- Others (Mistral, Cohere, etc.) — their own variants.

Writing agent logic against each dialect is unmaintainable. Agezt needs **one internal tool-calling representation** that every provider plugin translates to/from.

### 2.2 The internal representation (already in `agezt.proto`)
The kernel and `loop-node`s work only in Agezt's canonical shapes:
- `ToolDef { name, description, schema_json }` — a tool advertised to the model.
- `ToolCall { id, name, input_json }` — the model's request to call a tool.
- tool result → a `ChatMessage` with role `tool` carrying the result JSON + the originating `id`.

The Planner/loop never sees a provider dialect; it sees these.

### 2.3 The provider plugin's job
Each `ProviderPlugin` **translates** between Agezt's canonical shape and its backend's dialect, in both directions:
- **Outbound:** map `ToolDef[]` → the provider's tool/function schema; map prior `ToolCall`/results → the provider's message format.
- **Inbound:** parse the provider's tool-call output (whatever its shape) → canonical `ToolCall` (streamed as `ProviderChunk.tool_call`).
- **Capability fallback:** a provider lacking native tool-calling (or JSON mode) gets a robust prompt-based tool-calling shim in its plugin, and advertises `tool_use=false` so the Governor/Planner know it's emulated. Degradation is recorded (SPEC-10 §2).

This keeps the normalization burden **inside each provider plugin** (where the dialect knowledge belongs), not in the kernel. Adding a new provider = one plugin implementing the translation; the rest of the system is unaffected.

### 2.4 Parallel & streaming tool calls
The canonical representation supports multiple `ToolCall`s per turn (mapped to the scheduler's bounded-parallel execution) and streamed tool-call deltas — provider plugins reconcile their streaming format to the canonical chunk stream.

### 2.5 First-party providers must cover the dialects
v1 ships provider plugins covering: Anthropic (native blocks), OpenAI (tools), OpenAI-compatible (generic, for most local/3rd-party), Gemini (functionDeclarations), and a prompt-shim base for the rest. This guarantees "tool-calling works on essentially any provider."

---

## 3. Agent Client Protocol (ACP)

### 3.1 What ACP gives us
ACP (JSON-RPC 2.0) is the protocol IDEs (Zed, VS Code, JetBrains) use to talk to agent backends, and that some agents use to talk to each other. Two directions:

- **Agezt as an ACP server:** Agezt exposes itself as an agent backend so IDEs connect to it directly — type in your editor, Agezt plans/acts/streams back, with full slash-command support. (Same pattern Hermes ships via `hermes-acp`.)
- **Agezt as an ACP client:** Agezt drives *other* ACP agents as a node type — complementary to the `CodingAgentPlugin` bridges (SPEC-04 §4), but over the standard ACP wire instead of a bespoke bridge where ACP is available.

### 3.2 Placement
- ACP server: a surface in the gateway (SPEC-07 §9), alongside the OpenAI-compatible API — `agezt-acp` entry point.
- ACP client: a `CodingAgentPlugin` variant (or a dedicated `acp-bridge` plugin) that speaks ACP to external agents and maps their turns/diffs/tool-calls into Agezt events.
- This makes Agezt interoperate natively with the editor/agent ecosystem both ways — connect from Zed/VS Code, and orchestrate Codex/Claude-Code/other ACP agents from within a Agezt DAG.

### 3.3 Governance
ACP-driven actions pass through the same Edict/trust-ladder/journal path (SPEC-06) — an external IDE driving Agezt does not bypass governance; an external agent Agezt drives runs sandboxed with merge/force-push escalation (SPEC-04 §4.3).

---

## 4. How it all connects (the provider stack)

```
Planner / loop-node  ── canonical ToolDef/ToolCall (dialect-free) ──┐
                                                                    │
        Governor  ── routes by catalog (synced) + budget + limits ──┤
                                                                    ▼
   ProviderPlugin (per backend)  ── translates dialect ──▶  Anthropic / OpenAI /
        ▲ auto-discovered local (Ollama/vLLM)                OpenAI-compat / Gemini / …
        │
   Catalog sync (models.dev-class)  ── prices, models, capabilities ──▶ Governor table

   ACP server  ◀── IDEs (Zed/VS Code)        ACP client ──▶ external ACP agents
        (both pass through Edict + journal)
```

---

## 5. Phase placement (updates ROADMAP/TASKS)

- Catalog sync + credential import + local auto-discovery: **MVP/Phase 1** (Governor needs the price/capability table; keep it small but real).
- Tool-calling normalization (Anthropic + OpenAI + OpenAI-compatible + Gemini + shim): **Phase 1** (core to any agentic action).
- ACP server + client: **Phase 7** (with SDKs/API and coding agents).

---

## 6. New event kinds (added to SPEC-01 / `agezt.proto`)

`EVT_CATALOG_SYNCED`, `EVT_PROVIDER_CREDENTIAL_ADDED`, `EVT_ACP_SESSION_STARTED`, `EVT_ACP_SESSION_ENDED`. (To be merged into the canonical enum in the next proto revision, preserving numbering.)

---

## 7. Open questions

1. Catalog source trust: cache/verify the external catalog (it influences routing/cost) — signed snapshots? local override file?
2. Tool-call shim quality: how robust can prompt-based tool-calling be for non-native providers before it's not worth offering.
3. ACP version tracking and which IDE clients to certify first.
4. Conflict between synced catalog prices and provider-reported usage — reconcile to provider truth for billing.

---

*This document is folded into the suite as SPEC-15. The provider-side of `agezt.proto` (ProviderPlugin, ToolDef/ToolCall, ModelInfo) already supports this design; catalog sync and ACP add a kernel component and a gateway surface respectively.*
