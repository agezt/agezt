# Phase Report — Milestone M11 (Interop & Autonomy: every client in, every model reachable, agents that delegate)

> Status: **shipped** · Date: 2026-05-30
> ROADMAP P6-MULTI-01, P7-API-01 + SPEC-15 §1.3 / §3 — the layer that makes
> Agezt reachable from any client and able to orchestrate itself. The MVP
> (v0.1.0) proved the system *works*; M11 makes it *interoperate* and *delegate*.

## Why this milestone

The standing direction was: the full `.project` vision — a Jarvis-grade
autonomous OS that works with **every kind of provider and model** and is
excellent in every way. Two gaps stood between the shipped MVP and that:

1. **Reach.** Agezt could be driven only by its own `agt run`. The wider world
   speaks OpenAI's API and the editor world speaks ACP — neither could drive it.
2. **Autonomy depth.** A single agent loop, however good, is not "Jarvis." Real
   autonomy needs an agent that can *decompose* a task and *delegate* parts.

Plus onboarding friction: bringing "every provider" online was one-key-at-a-time.

M11 closes all three, stdlib-only, demo-gated, with `go.mod` unchanged.

## What shipped

### 1. Credential auto-discovery — `agt provider import` (SPEC-15 §1.3)
Discovers API keys the operator already has and vaults the recognised ones in
one pass. Sources, in priority order: the **process environment**, a project
**`.env`**, an explicit **`--from <file>`**, and well-known agent-CLI credential
files (**Codex** `~/.codex/auth.json`, **Gemini** `~/.gemini/settings.json`).
Recognition is against the synced catalog's provider `Env` names, or a
`*_API_KEY` / `*_TOKEN` / `*_SECRET` heuristic with `--all` (also the automatic
fallback on a fresh machine with no catalog). Values are **always masked** in
output; nothing is written without per-key `y/N` confirmation unless `--yes`;
`--dry-run` previews, `--json` for automation. Offline by design — writes the
vault directly like `provider creds set`, then prints the `provider reload` hint.

The discovery core (`discoverCredentials` / `parseDotEnvFile` /
`parseJSONCredFile` / `looksLikeCredName`) is pure and table-driven so sources
are injectable in tests. **Proven:** a 4-line `.env` imported, recognised,
masked, grouped by provider in the vault.

### 2. OpenAI-compatible API server — `kernel/openaiapi` (ROADMAP P7-API-01)
A daemon resident (gated by `AGEZT_API_ADDR`, loopback-bound, Bearer-token
authed — mirrors the Web UI resident's lifecycle) exposing **`POST
/v1/chat/completions`** (streaming + non-streaming) and **`GET /v1/models`**, so
any OpenAI client, SDK, or IDE drives Agezt as if it were OpenAI.

It is an **agent surface, not a raw passthrough**: every request runs the same
kernel tool-loop as `agt run`, so Edict, the journal, and the budget all apply —
not a governance backdoor (P7-API-02 DoD). OpenAI `messages[]` collapse into one
Agezt intent (single user turn → verbatim; multi-turn → labelled transcript;
array content flattened); streaming maps the kernel's `llm.token` events to
`chat.completion.chunk` SSE frames; the response carries an
`agezt_correlation_id` so any call is `agt why`-able. The server depends on a
small `Engine` interface (not the concrete kernel), so the SSE path is tested
for real with a fake engine publishing token events on an in-memory bus.
**Proven live:** `/v1/models` lists the catalog, no-token → 401, a non-streaming
chat returns a journaled answer + correlation id, and the streaming path emits
the role chunk → content deltas → stop → `[DONE]` envelope.

### 3. Multi-agent delegation — the `delegate` tool (ROADMAP P6-MULTI-01)
A lead agent can now spawn sub-agents. The in-process `delegate` tool takes a
self-contained task, runs it as a nested `agent.Run` under a fresh **child
correlation** with its own tool-loop, and returns the result; several `delegate`
calls in one turn fan out concurrently.

Auditable by construction: each spawn is journaled as a new **`subagent.spawned`**
event under the **parent** correlation (carrying the child correlation + task +
depth), so `agt why <parent>` reveals the delegation and the child correlation is
the drill-down into the sub-agent's own run. Bounded and governed: nesting depth
is capped (`SubAgentMaxDepth`, default 1, `AGEZT_SUBAGENT_DEPTH`) via a
context-threaded counter; the delegation is gated by a new Edict `delegate`
capability (allow-by-default — the spawn has no external side effect of its own),
while the sub-agent's **actual** tool calls are each gated through the same
engine, so safety is enforced where the action happens. On by default;
`AGEZT_SUBAGENT=off` disables. **Proven:** delegation flow (answer + spawn event +
child task arc), depth guard (second-level delegate refused → exactly one spawn),
disabled-by-default — all through the real agent loop + bus + Edict; `delegate`
appears live in `agt tool list`.

### 4. ACP server — `agt acp` (SPEC-15 §3)
Agezt as an Agent Client Protocol backend: `agt acp` speaks JSON-RPC 2.0 over
stdio so an IDE (Zed, other ACP clients) connects and drives it —
`initialize` → `session/new` → `session/prompt`, with streamed `session/update`
(`agent_message_chunk`) notifications. Each prompt is forwarded to the daemon as
a normal `run` over the control plane, so it executes through the same tool-loop
+ Edict + journal — an editor driving Agezt does not bypass governance (§3.3).
The protocol core (`kernel/acp`) is transport- and kernel-agnostic (a `Runner`
interface), fully unit-tested with a fake; the `agt acp` command backs the Runner
with the control-plane streaming client. **Proven live:** a piped
initialize/new/prompt session against the daemon returns capabilities, a session
id, a streamed chunk from a real journaled run, and `stopReason: end_turn`.

## How it connects

```
   OpenAI clients/SDKs ─▶  /v1/chat/completions, /v1/responses ─┐
   first-party clients ─▶  /api/v1/runs (REST, native)         ─┤
   IDEs (Zed, …)       ─▶  agt acp (JSON-RPC)                  ─┤
                                                                ├▶ kernel tool-loop ─▶ Edict ─▶ journal
   agt run / Telegram / Web UI ────────────────────────────────┘        │
                                                 lead agent ──delegate──▶ sub-agent (own loop)
                                                          │ ──acp_agent──▶ external ACP agent (subprocess)
                                                          │ ──coding─────▶ external coding agent (worktree)
   provider import ─▶ vault ─▶ Governor routes ──▶ every provider family

   kernel tool-loop ─▶ journal/bus ──webhooks──▶ external HTTP endpoints (HMAC)
```

Every inbound path funnels through the one governed loop; every outbound
delegation (sub-agent, ACP agent, coding agent) is dispatched *from* that loop
through Edict; and every journal event can fan out to external systems via
signed webhooks — no surface is a side-door around Edict or the journal.

## Engineering

- **stdlib-only**, `go.mod` unchanged (still BLAKE3 + its cpuid helper).
- New kernel packages: `kernel/openaiapi`, `kernel/acp`; new runtime file
  `kernel/runtime/subagent.go`; new CLI commands `agt provider import`,
  `agt acp`; one new event kind (`subagent.spawned`) and one new Edict
  capability (`delegate`), both registered append-only.
- `go test ./...`, `go vet ./...`, and a `GOOS=linux` cross-build are green;
  every feature has unit tests plus a live end-to-end demo against the daemon.

## Deferred (named, not forgotten)

- **M8–M9** — mesh / multi-tenant / marketplace, and voice / mobile / tray.
  These need either non-stdlib dependencies or a large design phase; named in
  ROADMAP, not started. (P7-API-02 — both the inbound REST surface and the
  outbound webhooks — is now complete; see below.)

## Follow-up shipped (same milestone)

- **Per-request + cross-provider model routing** (SPEC-15 §1) — the OpenAI API's
  `model` is now honoured per call (`runtime.WithModel` → the agent loop's
  `CompletionRequest.Model`), and the daemon registers **every** credentialed +
  supported provider with the model ids it serves so the Governor routes a named
  model to its provider (`ProviderInfo.Models` + `applyModelRoute`, a pure
  reorder preserving the fallback chain). `{"model":"gpt-4o"}` → OpenAI,
  `{"model":"claude-…"}` → Anthropic, on one daemon. Proven by Governor unit
  tests (model hoists the serving provider; unknown model keeps default order)
  and live (`model-routable_alternates=N` in the banner with two providers
  credentialed).

- **Coding-agent bridge** (P6-CODE) — the `coding` tool delegates a coding task
  to an external agent (Claude Code / Codex / Aider / any `AGEZT_CODING_CMD`)
  inside an **isolated git worktree** off HEAD, captures the diff, and returns it
  marked "NOT applied" — never commit/merge/push (§4.3 escalation kept separate).
  The task rides `$AGEZT_CODING_TASK`; the worktree is removed afterward; a new
  Edict `coding` capability gates it Ask-first. The command runner is injectable,
  so the orchestration is unit-tested deterministically, and a git-guarded live
  test proves real worktree isolation + diff capture against actual git. The
  external coding CLIs bind by config; the bridge mechanism is what shipped.

- **ACP client / `acp_agent` bridge** (SPEC-15 §3, the inverse of the server) —
  Agezt can now *drive* external ACP agents, not just be driven. A new
  transport-agnostic `kernel/acp.Client` speaks the client side of the protocol
  (`initialize` → `session/new` → `session/prompt`, consuming `session/update`
  notifications); the `acp_agent` in-process tool spawns an operator-configured
  external ACP agent (`AGEZT_ACP_AGENT_CMD` — Claude Code / Codex / Gemini CLI /
  any) as a subprocess and relays its streamed answer back into the run. SPEC-15
  §3 is now bidirectional on one substrate: the same `kernel/acp` package is both
  the server an IDE drives and the client that drives other agents. Gated
  Ask-first by a new Edict `acp_agent` capability (the external agent acts in its
  own sandbox). **Proven:** the `Client` round-trips against the real `Server`
  over pipes (both wire directions), the bridge relays a streamed answer from a
  real `acp.Server` peer, and a live test drives a genuine ACP **subprocess** end
  to end (`initialize`/`new`/`prompt` over real stdio).

- **OpenAI Responses API** (`POST /v1/responses`, P7-API-02) — the newer OpenAI
  surface, served beside `/v1/chat/completions` on the same resident. A string or
  message-array `input` plus top-level `instructions` collapse into one intent
  through the *same* governed loop (it reuses `intentFromMessages`, so the
  mapping and tests stay shared); non-streaming returns a `response` object, and
  streaming emits the Responses event sequence (`response.created` →
  `response.output_text.delta` → `…done` → `response.completed`) mapped from the
  kernel's `llm.token` events. Same auth, loopback binding, and
  `agezt_correlation_id`. **Proven:** non-streaming output/usage shape, the
  `instructions` + typed-array `input` flattening, the full streaming event
  sequence, and the non-streaming-provider single-delta fallback — all against
  the fake engine on a real bus.

- **Outbound webhooks** (`kernel/webhook`, P7-API-02) — the outbound counterpart
  of the inbound API surfaces: a daemon resident that subscribes to the journal
  bus and POSTs matching events to operator-configured endpoints, so external
  systems react to Agezt in real time. Each sink is a `url|subject|secret`
  triple (`AGEZT_WEBHOOKS`); the subject is a normal bus pattern, so matching is
  the bus's, not a reimplementation. A secret turns on HMAC-SHA256 body signing
  (`X-Agezt-Signature`) for receiver verification. Deliveries retry with backoff,
  every outcome is journaled (`webhook.delivered` / `webhook.failed`, tied to the
  originating run's correlation), and the dispatcher skips its own `webhook.*`
  events so there is no feedback loop. **Proven:** unit-tested against a real
  `httptest` receiver + real bus/journal (signed delivery + signature
  verification, subject filtering, retry-then-succeed, fail-after-max-attempts,
  the no-loop guard, spec parsing), and live end-to-end — a mock-provider run's
  full arc (`task.received` → … → `task.completed`, 9 events) delivered HMAC-
  signed to a local receiver with 9 matching `webhook.delivered` audit events
  journaled.

- **Native REST surface** (`kernel/restapi`, P7-API-02) — the first-party,
  non-OpenAI inbound API, completing P7-API-02 alongside the webhooks above.
  Where `kernel/openaiapi` mimics OpenAI wire shapes for drop-in clients, this
  speaks Agezt-native semantics: `POST /api/v1/runs` submits an intent and
  returns a `correlation_id` (sync JSON or an SSE `start`→`token`*→`done`/`error`
  stream), and — uniquely — `GET /api/v1/runs/{corr}` returns the run's full
  journaled event arc, so a client can submit *and* audit through one surface.
  `GET /api/v1/health` and `/models` round it out. Same governed loop, same
  resident lifecycle (loopback + minted Bearer token), per-request model. The
  server depends on a small `Engine` interface (the same kernel adapter as the
  OpenAI surface, plus `EventsForCorrelation`), so it is fully unit-tested with a
  fake on a real bus. **Proven live:** health/models, a sync `POST /runs` that
  returned a journaled answer + correlation, `GET /runs/{corr}` returning the
  9-event arc (`task.received` → … → `task.completed`), the SSE stream
  (`start`→`done`, and a provider error correctly surfaced as `error`), and
  no-token → 401.

These are the next reachable steps toward the full vision; the substrate they
build on shipped here.
