# Phase Report — Milestone M12 (Full API surface + Mesh: every protocol in, every node reachable)

> Status: **shipped** · Date: 2026-05-30
> ROADMAP P7-API-02 (complete) + P6-MULTI / M8 (mesh primitive) + SPEC-15 §3
> (bidirectional). Where M11 made Agezt *reachable* and able to *delegate*, M12
> makes its API surface **complete** and turns delegation **outward to peer
> nodes** — the first cooperation between Agezt instances.

## Why this milestone

M11 opened the door (OpenAI chat API, ACP server, sub-agent delegation, credential
import). Three gaps remained between that and the full `.project` vision of a
Jarvis-grade system that interoperates with *everything* and scales past one node:

1. **API completeness.** Only the OpenAI Chat Completions shape and the ACP
   *server* existed. The newer OpenAI **Responses** API, a **first-party REST**
   surface, **outbound** event delivery, and the ACP **client** were all missing.
2. **One-way ACP.** Agezt could be driven as an ACP agent but could not drive
   other ACP agents — SPEC-15 §3 was half-implemented.
3. **Single node.** Delegation stopped at in-process sub-agents; nodes could not
   cooperate.

M12 closes all three, stdlib-only, demo-gated, with `go.mod` unchanged.

## What shipped

### 1. ACP client + `acp_agent` bridge (SPEC-15 §3, now bidirectional)
A transport-agnostic `kernel/acp.Client` speaks the client side of the protocol
(`initialize` → `session/new` → `session/prompt`, consuming `session/update`
notifications), round-trip tested against the real `Server` over pipes. The
`acp_agent` in-process tool spawns an operator-configured external ACP agent
(`AGEZT_ACP_AGENT_CMD` — Claude Code / Codex / Gemini CLI / any) as a subprocess
and relays its streamed answer into the run. **One `kernel/acp` package is now
both** the server an IDE drives **and** the client that drives other agents.
Gated Ask-first (new Edict `acp_agent` capability). **Proven** by the pipe
round-trip, a relay against a real `acp.Server` peer, and a live test driving a
genuine ACP subprocess over real stdio.

### 2. OpenAI Responses API — `POST /v1/responses` (P7-API-02)
The newer OpenAI surface, served beside `/v1/chat/completions` on the same
governed resident. A string or message-array `input` plus top-level
`instructions` collapse into one intent through the *same* `intentFromMessages`
(mapping + tests stay shared); non-streaming returns a `response` object, and
streaming maps `llm.token` events to the Responses event sequence
(`response.created` → `output_text.delta` → `…done` → `completed`). Same auth,
loopback binding, per-request model, and `agezt_correlation_id`.

### 3. Outbound webhooks — `kernel/webhook` (P7-API-02)
The outbound counterpart of the inbound surfaces: a daemon resident subscribes
to the journal bus and POSTs matching events to operator-configured endpoints,
so external systems react to Agezt in real time. Sinks are `url|subject|secret`
triples (`AGEZT_WEBHOOKS`); the subject is a normal bus pattern (matching is the
bus's, not a reimplementation). A secret enables HMAC-SHA256 body signing
(`X-Agezt-Signature`). Deliveries retry with backoff, each outcome is journaled
(`webhook.delivered` / `webhook.failed`, tied to the originating correlation),
and the dispatcher skips its own `webhook.*` events — no feedback loop. **Proven**
live: a mock-provider run's full 9-event arc delivered HMAC-signed to a local
receiver, with 9 matching audit events journaled.

### 4. Native REST API — `kernel/restapi` (P7-API-02, completing it)
A first-party `/api/v1` surface with Agezt-native semantics (where `/v1` mimics
OpenAI). `POST /api/v1/runs` submits an intent → `{correlation_id, answer}`
(sync) or an SSE `start`→`token`*→`done`/`error` stream; **`GET
/api/v1/runs/{corr}` returns the run's full journaled event arc** —
correlation-first inspection the OpenAI surface can't offer; plus `/health` and
`/models`. Same governed loop, resident lifecycle, loopback + Bearer token.
**Proven** live end-to-end: health, a sync run, the 9-event arc via `GET
/runs/{corr}`, the SSE stream (and a provider error surfaced as `error`), 401.

### 5. Mesh delegation — the `remote_run` tool + `agt peers` (P6-MULTI / M8)
The first node-to-node primitive: a lead agent on one node hands a self-contained
task to a *peer* Agezt node and gets the answer back, by driving the peer's
native REST surface (`POST /api/v1/runs`) — composing #4 into cooperation between
nodes, and dogfooding it. The peer runs the task through its **own** governed
loop, so the remote work is auditable on that node via its correlation id and
bypasses no governance on either side. Peers are operator-configured
(`AGEZT_PEERS=name=url|token,…`, hard-validated at startup); gated Ask-first (new
Edict `remote_run` capability). `agt peers [--json]` lists the peers and
health-checks each over `/api/v1/health`. **Proven** by a live round-trip
through the real `kernel/restapi` handler (Bearer auth enforced), a two-daemon
registration check, and a three-peer live `agt peers` (healthy / bad-token /
down, each reported correctly).

## How it connects

```
   OpenAI clients/SDKs ─▶ /v1/chat/completions, /v1/responses ─┐
   first-party clients ─▶ /api/v1/runs (REST, native)         ─┤
   IDEs (Zed, …)       ─▶ agt acp (JSON-RPC server)           ─┤
                                                                ├▶ kernel tool-loop ─▶ Edict ─▶ journal
   agt run / Telegram / Web UI ────────────────────────────────┘        │
   lead agent ──delegate──▶ in-process sub-agent (own loop)              │
   lead agent ──acp_agent──▶ external ACP agent (subprocess)             │
   lead agent ──remote_run─▶ peer node's /api/v1/runs (its own loop)     │
   journal/bus ──webhooks──▶ external HTTP endpoints (HMAC-signed)       │
```

Every inbound path funnels through the one governed loop; every outbound
delegation (sub-agent, ACP agent, peer node) is dispatched *from* that loop
through Edict; and every journal event can fan out to external systems via signed
webhooks — no surface, in or out, is a side-door around Edict or the journal.

## Engineering

- **stdlib-only**, `go.mod` unchanged (still BLAKE3 + its cpuid helper).
- New packages: `kernel/restapi`, `kernel/webhook`, `plugins/tools/acpagent`,
  `plugins/tools/peer`; new `kernel/acp.Client`; new `agt peers` command.
- Three new Edict capabilities (`acp_agent`, `remote_run`, and M11's `coding`),
  two new event kinds (`webhook.delivered/failed`), all registered append-only.
- `go test ./...`, `go vet ./...`, and a `GOOS=linux` cross-build are green;
  every feature has unit tests plus a live end-to-end demo. **1072 tests / 54
  packages.**

## Deferred (named, not forgotten)

- **Multi-tenant isolation, a skill/plugin marketplace, and voice / mobile / tray
  clients** (the large remainder of M8–M9). These need non-stdlib dependencies or
  a dedicated design phase; named in ROADMAP, not started. P7-API-02 is now
  complete (inbound REST + OpenAI chat/responses + outbound webhooks), and the
  mesh *primitive* (node-to-node delegation) shipped here; richer mesh
  (discovery, federation, shared budgets) builds on it.
