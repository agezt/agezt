# Changelog

All notable changes to the Agezt kernel (`agezt` daemon + `agt` CLI) are
recorded here. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versioning is [semantic](https://semver.org/spec/v2.0.0.html). Pre-1.0 the
minor version tracks the product milestone (ROADMAP.md).

This is the human, per-component changelog (SPEC-08 §4.1). The machine,
tamper-evident timeline of what actually happened to a running system lives in
the hash-chained journal — `agt journal tail` / `agt why` (SPEC-08 §4.2).

## [Unreleased]

### Added
- **OpenAI Responses API** — `POST /v1/responses` (ROADMAP P7-API-02), alongside
  the existing `/v1/chat/completions`, so clients on OpenAI's newer Responses
  surface drive Agezt too. Accepts a string or message-array `input` plus
  top-level `instructions`, which collapse into one Agezt intent through the same
  governed kernel loop (Edict + journal + budget). Non-streaming returns a
  `response` object (`output[].content[].output_text` + `output_text` +
  `agezt_correlation_id`); streaming emits the Responses SSE event sequence
  (`response.created` → `response.output_text.delta*` →
  `response.output_text.done` → `response.completed`). Same resident, auth, and
  loopback binding as the chat endpoint.
- **ACP-agent bridge** — the `acp_agent` tool (SPEC-15 §3, the inverse of the
  `agt acp` server): delegates a task to an *external* agent that speaks the
  Agent Client Protocol (Claude Code, Codex, Gemini CLI, or any command via
  `AGEZT_ACP_AGENT_CMD`). It spawns the agent as a subprocess and drives it over
  JSON-RPC 2.0 on stdio — `initialize` → `session/new` → `session/prompt` —
  relaying the agent's streamed `agent_message_chunk` updates back as the tool
  result. The new `kernel/acp` `Client` is transport-agnostic (round-trip tested
  against the real `Server` over pipes); the bridge's spawn path is proven by a
  live test that drives a genuine ACP subprocess end to end. Gated by a new Edict
  `acp_agent` capability (Ask-first — the external agent acts in its own
  sandbox). Off unless `AGEZT_ACP_AGENT_CMD` is set.
- **Coding-agent bridge** — the `coding` tool (ROADMAP P6-CODE, SPEC-04 §4):
  delegates a coding task to an external coding agent (Claude Code, Codex, Aider,
  or any command via `AGEZT_CODING_CMD`) running in an **isolated git worktree**
  off the current HEAD, captures the resulting diff, and returns it for review.
  It never commits to, merges, or force-pushes the working branch — applying the
  diff is a separate operator-approved step (§4.3 escalation). The task is passed
  in `$AGEZT_CODING_TASK` (no shell-quoting of model output); the worktree is
  removed afterward. Gated by a new Edict `coding` capability (Ask-first). Off
  unless `AGEZT_CODING_CMD` is set. Proven live against real git: a stub agent's
  new file is captured as a diff while the working repo stays untouched.
- **Cross-provider model routing** (SPEC-15 §1) — the daemon now registers
  *every* credentialed + supported catalog provider (not just the primary), each
  carrying the model ids it serves; the Governor routes a request naming a model
  to the provider that serves it (`ProviderInfo.Models` + `applyModelRoute`, a
  pure reorder that preserves the fallback chain). Combined with the OpenAI API's
  per-request model override, `{"model":"gpt-4o"}` routes to OpenAI and
  `{"model":"claude-…"}` to Anthropic on the same daemon — "drive Agezt with any
  provider/model" end to end. The banner reports `model-routable_alternates=N`.
- **ACP server** — `agt acp` (SPEC-15 §3): an Agent Client Protocol server
  speaking JSON-RPC 2.0 over stdio, so an IDE (Zed and other ACP clients) can
  drive Agezt as an agent backend. Implements `initialize` / `session/new` /
  `session/prompt` with streamed `session/update` (agent_message_chunk)
  notifications. Each prompt is forwarded to the daemon as a normal governed
  `run`, so it passes through the same tool-loop + Edict + journal — the editor
  does not bypass governance (§3.3). The protocol core is transport- and
  kernel-agnostic (a `Runner` interface), tested with a fake; the `agt acp`
  bridge backs it with the control-plane streaming client.
- **Multi-agent delegation** (ROADMAP P6-MULTI-01) — a `delegate` in-process
  tool lets a lead agent spawn a bounded sub-agent (its own tool-loop) for a
  focused subtask and get back a concise result; issuing several `delegate`
  calls in one turn fans out concurrently. Each spawn is journaled as
  `subagent.spawned` under the **parent** correlation (carrying the child
  correlation), so `agt why <parent>` shows the delegation and the child
  correlation is the drill-down into the sub-agent's own run. Nesting is bounded
  by `AGEZT_SUBAGENT_DEPTH` (default 1); the sub-agent's actual tool calls are
  each gated through Edict (new `delegate` capability, allow-by-default — the
  delegation itself has no external side effect). On by default;
  `AGEZT_SUBAGENT=off` disables it.
- **OpenAI-compatible API server** (ROADMAP P7-API-01) — a daemon resident
  exposing `POST /v1/chat/completions` (streaming + non-streaming) and
  `GET /v1/models`, so any OpenAI client, SDK, or IDE can drive Agezt as if it
  were OpenAI. Each request runs through the same kernel tool-loop as `agt run`
  — Edict, journal, budget all apply; it is not a governance backdoor. The
  OpenAI `messages[]` collapse into one Agezt intent (single-turn → verbatim;
  multi-turn → labelled transcript; array content flattened); streaming maps the
  kernel's `llm.token` events to `chat.completion.chunk` SSE frames; the
  response carries an `agezt_correlation_id` so any call is `agt why`-able.
  Off unless `AGEZT_API_ADDR` is set; loopback-bound + Bearer-token authed.
  The request's `model` is honoured per-request (threaded through the run via
  `runtime.WithModel` into the provider's `CompletionRequest.Model`), so callers
  pick the model per call instead of being pinned to the daemon's default.
- `agt provider import` — credential auto-discovery (SPEC-15 §1.3): scans the
  process environment, a local `.env`, an explicit `--from <file>`, and
  well-known agent-CLI credential files (Codex, Gemini) for API keys, matches
  them against the synced catalog (or a `*_API_KEY`/`*_TOKEN` heuristic with
  `--all`), and stores the recognised ones in the vault. Values are always
  masked; nothing is written without per-key confirmation unless `--yes`.
  `--dry-run` previews; `--json` for automation. "Works with every provider you
  already have a key for" with one command.
- `agt world forget <id>` — tombstone a world-model entity (soft delete;
  reversible, journaled), completing the symmetry with `memory forget`.
- **Web UI world graph** — the World panel now renders a node-link diagram
  (entities as nodes, relations as directed arrows) above the entity list, an
  inline SVG laid out client-side with no dependency. `GET /api/world` now
  returns the relation `edges` (from/verb/to/weight) alongside the existing
  `relation_count` to feed it.
- **Web UI operator actions** — the dashboard is no longer read-only: a HALT /
  Resume control bar, an Approvals panel (approve/deny pending HITL requests),
  and per-item actions in the Memory (forget), World (forget), and Skills
  (promote / quarantine / revert) panels. Mutating actions are a fixed
  allowlist, POST-only, token-authed, and pass only allowlisted args
  (GET/no-token are refused); reads stay GET.

- `agt quickstart` — interactive first-run wizard: syncs the catalog
  (offline), shows configured providers, prompts to add a key for the one you
  pick, and prints the exact daemon start command + next steps. Thin glue over
  `catalog sync --local` + `provider setup`.
- `make install` (binaries onto PATH) and `make run` (build + run the daemon)
  targets; the README quick start now documents the real onboarding —
  `catalog sync --local` → `provider setup` → start with a provider → `doctor`
  → `run`, plus the Web UI.
- `agt help` now leads with a "New here? Run `agt quickstart`" pointer, so a
  first-time operator is steered to onboarding instead of the flat command wall
  (`run` errors with no catalog/key yet).

### Fixed
- Web UI Memory panel read the wrong result key (`memories` vs the actual
  `records`), so it never listed stored facts; now renders them.
- Onboarding now surfaces `AGEZT_WORKSPACE="$PWD"` in the quickstart/README
  start command so the file tool can read the project you launch from — the
  common "my first `agt run` can't see my files" gap. The safe sandboxed
  default (`~/.agezt/workspace`) is unchanged; this is a visible opt-in.

## [0.1.0] — 2026-05-30

The **MVP** (ROADMAP §2.2): a usable, single-deployment Jarvis. Everything the
system does is journaled, content-addressed, and reversible; you can see why it
did anything (`agt why`) and stop it instantly (`agt halt`).

### Kernel & foundation
- **Event-sourced journal** — append-only JSONL with a BLAKE3 hash chain;
  `agt journal verify` proves integrity, `agt why <id>` reconstructs causation
  by correlation. Mutable state store + in-process bus alongside the log.
- **First-party agent loop** — LLM ↔ tool tool-calling core; DAG scheduler +
  planner (`agt plan generate|run|validate|visualize|cost`) over it.
- **Control plane** — token-authed localhost TCP; `agt` is a thin client.
  `agt halt`/`resume`/`shutdown`/`status`, ULID identity everywhere.
- **Single-instance guard** — the daemon refuses to start when a live daemon
  already serves the same base dir (overriding `AGEZT_FORCE_START=1`), so
  clients never silently split across two kernels.
- **`agt doctor`** — zero-config preflight: base dir, daemon, version skew,
  journal integrity, tools, halt state → OK/WARN/FAIL with hints; exit 1 on
  failure for CI.

### Providers & cost
- **models.dev catalog** — `agt catalog sync` (now also **offline/client-side**
  without the daemon, `--local`), `agt catalog list`, Ollama auto-discover.
- **Every catalog family wired** via one compat layer — Anthropic, OpenAI &
  OpenAI-compatible, Google Gemini, Mistral, Cohere, Azure OpenAI, AWS Bedrock,
  Google Vertex. Real providers proven end-to-end (incl. third-party
  Anthropic-shaped endpoints like MiniMax coding-plan).
- **Guided key setup** — `agt provider setup [id]` lists providers needing a key
  and prompts (stdin, never argv) to add the missing ones; `agt provider
  creds set|list|rm`, encrypted vault (`agt vault encrypt`).
- **Governor v1** — USD-microcent budgeting + daily ceiling, fallback chains,
  per-task-type routing/model/budget overrides; `agt provider check` live
  roundtrip (latency/cost), `agt budget`.

### Tools & safety
- **4 sandboxed tools** — shell, file, http, browser (Warden namespace /
  container profiles).
- **Edict policy v1** — trust ladder, hard-deny rules, HITL approvals
  (`agt approvals`/`approve`/`deny`), secret redaction, `agt halt`, anomaly
  auto-halt. `agt edict show|test`.

### Channels & proactivity
- **Telegram channel** (duplex) — command in, proactive brief out; inbound
  treated as untrusted data behind an allowlist.
- **Pulse v1** — heartbeat + observers (repo/CI, system health) + salience
  (rules + optional cheap LLM) + Quiet/Balanced/Chatty dial + Initiative;
  briefs to Telegram. `agt pulse` (live tail), `agt pulse status|pause|resume`.

### Memory & self-improvement (Phase 2)
- **Memory** — content-addressed facts the agent reads as context; ranked
  retrieval, soft delete. `agt memory add|list|search|get|forget`.
- **World model** — entity/relation graph; reference resolution feeds Pulse
  salience. `agt world add|relate|resolve|neighbors|list|show`.
- **Forge** — skill lifecycle (draft→shadow→active→quarantined→archived),
  operator-gated promotion, lineage + revert. `agt skill list|show|history|
  promote|quarantine|revert`.
- **Reflection** — folds the journal into observations, auto-decays stale
  world-model entities (safe bound), surfaces advisory proposals. `agt reflect
  run|show`, optional `AGEZT_REFLECT_EVERY` timer.

### Web UI (Phase 5, v1)
- **SSE Live Monitor + read panels** — stdlib `net/http` + `embed`, no build
  chain; streams the bus and proxies the same control-plane reads the CLI uses.
  Localhost-bound + token-authed + read-only. `AGEZT_WEB_ADDR=127.0.0.1:8787`.

### Operability
- **Unified inbox** (`agt inbox`), **runs** (`agt runs list|show|last`),
  **state** (`agt state list|get`), **config** (`agt config show`),
  resolved-config + env-presence views.

### Engineering
- **stdlib-first** — the only external dependencies are BLAKE3 (+ its CPU-id
  helper); every addition is justified and CI-gated (POLICY).
- Multi-arch `CGO_ENABLED=0` builds; `go test ./...`, `go vet`, and a
  `GOOS=linux` cross-build are green.

[Unreleased]: https://example.invalid/agezt/compare/v0.1.0...HEAD
[0.1.0]: https://example.invalid/agezt/releases/tag/v0.1.0
