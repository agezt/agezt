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
   OpenAI clients/SDKs ─▶  /v1/chat/completions ─┐
   IDEs (Zed, …)       ─▶  agt acp (JSON-RPC)   ─┤
                                                 ├▶ kernel tool-loop ─▶ Edict ─▶ journal
   agt run / Telegram / Web UI ─────────────────┘        │
                                                 lead agent ──delegate──▶ sub-agent (own loop)
   provider import ─▶ vault ─▶ Governor routes ──▶ every provider family
```

Every inbound path funnels through the one governed loop; no surface is a
side-door around Edict or the journal.

## Engineering

- **stdlib-only**, `go.mod` unchanged (still BLAKE3 + its cpuid helper).
- New kernel packages: `kernel/openaiapi`, `kernel/acp`; new runtime file
  `kernel/runtime/subagent.go`; new CLI commands `agt provider import`,
  `agt acp`; one new event kind (`subagent.spawned`) and one new Edict
  capability (`delegate`), both registered append-only.
- `go test ./...`, `go vet ./...`, and a `GOOS=linux` cross-build are green;
  every feature has unit tests plus a live end-to-end demo against the daemon.

## Deferred (named, not forgotten)

- **ACP client** — driving *external* ACP agents from a Agezt node (the inverse
  of the server). Folds into the coding-agent bridge work below.
- **Coding-agent bridges** (P6-CODE: claude-code / codex / aider as nodes, with
  git-worktree isolation and merge/force-push escalation) — needs external CLIs
  and a substantial worktree/diff/escalation design phase.
- **Per-request model routing** in the OpenAI API — today the request `model` is
  echoed but routing is the Governor's configured primary; routing the API's
  `model` field to a specific catalog provider is a Governor enhancement.
- **OpenAI `/v1/responses`** and native REST/webhooks (P7-API-02 remainder).

These are the next reachable steps toward the full vision; the substrate they
build on shipped here.
