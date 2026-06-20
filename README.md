# Agezt

> An open-source (MIT) **agentic operating system**: a stdlib-first Go core
> that turns intent into auditable, reversible action; runs autonomous
> agents under a policy/trust system; proactively informs you (Pulse); and
> extends via in-process or out-of-process plugins.
> **Autonomous, under your authority.**

**Status:** active pre-release / Jarvis hardening tree (June 2026). This is a
running Go daemon + CLI + embedded React Web UI, not only a design suite: durable
agents, typed schedules, workflows, memory/world/skills, provider/catalog
plumbing, policy, audit, and recovery surfaces are implemented and being tightened
against the full autonomous-Jarvis acceptance bar. Any **OpenAI client or IDE**
can drive it through the OpenAI-compatible/API surfaces where configured; peer,
channel, and marketplace surfaces remain capability- and environment-dependent.
See [CHANGELOG.md](CHANGELOG.md).
**Recent local gates:** `go test ./...`, frontend `npm test` (121 files / 1052
tests), `npm run build`, and Playwright browser E2E against the embedded SPA and a
live demo daemon.
**Dependencies:** see [DEPENDENCIES.md](DEPENDENCIES.md) for the current `go.mod`-derived dependency inventory and justification status.

## What you get

A single Go daemon (`agezt`) and a CLI (`agt`) that, together, let you:

```
agt run "summarise the latest commits and email the team"      — one-shot intent
agt plan generate "audit my repo for secrets, propose fixes"   — LLM-generated DAG
agt plan run --dry-run "ship the release" --model sonnet       — preview: gen+validate+viz+cost
agt plan refine plan.json --feedback "skip the email step"     — operator-driven re-plan
agt plan validate plan.json                                    — pure client-side check
agt plan visualize plan.json                                   — Mermaid graph TD output
agt pulse --correlation run-01H...                             — live tail of one chain
agt pulse --since 0 --replay-rate 50                           — historical replay
agt status                                                     — daemon health overview
agt budget                                                     — spend vs daily / per-task caps
agt cache                                                      — prompt-cache savings (tokens from cache + $ saved)
agt tool list                                                  — in-process tools the model sees
agt peers [--json]                                             — list peer nodes + check their REST health
agt schedule add "<agent task|label>" --every 1h | --at 09:30  — typed cron jobs: agent/workflow/system-task/tool
agt schedule add --system-task catalog_sync --every 24h        — sync models.dev/api.json without waking an agent
agt tenant create <id> / list / token <id> / rm <id>           — manage isolated tenants + reveal per-tenant token (daemon AGEZT_MULTITENANT=on)
agt run "<intent>" --tenant <id>  ·  HTTP: X-Agezt-Tenant: <id> — route a run to a tenant's kernel (REST/OpenAI APIs + agt acp --tenant; auth with that tenant's token; isolated journal)
agt plugin list                                                — external plugins loaded
agt plugin registry <url> --install <name>                     — install a plugin from a registry (download + BLAKE3-verify; prints the env to enable)
agt skill registry <url> --install <name>                      — install a skill from a remote registry (content-address verified)
agt edict show / edict test shell "rm -rf /"                   — view + preflight policy decisions
agt state list / state get <ns> <key>                          — read kernel state store
agt journal tail 50 --json                                     — snapshot of recent events
agt shutdown                                                   — graceful exit (CI-friendly)
agt provider check --all                                       — verify all credentials
agt provider creds set OPENAI_API_KEY sk-...                   — managed vault
agt vault encrypt / vault rotate                               — at-rest encryption + key rotation
agt why <event_id> --payload                                   — walk the audit chain w/ payloads
agt approvals --json                                           — HITL queue (machine-readable)
```

with **9 provider families** (Anthropic, OpenAI + ~11 compatibles, Google
direct + Vertex, Cohere, Mistral, Ollama, AWS Bedrock with bearer +
SigV4 + STS-AssumeRole + SSO + IRSA/web-identity, Azure OpenAI) with
**per-request model routing**
(a request's `model` selects its provider), **all streaming**, **in-process
tools** (`shell`, `file`, `http`, `browser.read`, `web_search` to DISCOVER
pages via a keyless search engine, `schedule` so the agent can arrange its OWN
future runs, plus `memory`, `world`, `delegate` for sub-agent fan-out, `notify`
to message the operator's channels, `coding` for worktree-isolated external
coding agents, `acp_agent` to drive external ACP agents, `remote_run` to
delegate to peer Agezt nodes, and `homeassistant` to read entity state and
control the smart home), an
**OpenAI-compatible `/v1` API** (chat completions + responses) and an
**ACP server** so any OpenAI client or IDE can drive it, a **native REST
`/api/v1`** (submit + inspect runs) for first-party clients,
**outbound webhooks** (HMAC-signed) so external systems react to its events,
**out-of-process plugins** in any language over a tiny JSON protocol
(with **hot-reload**, **BLAKE3 pin gating**, **tool allowlists**,
**streaming progress**, and **kernel-callbacks**), an **MCP bridge plugin**
(stdio + SSE transports), a **DAG scheduler** with HITL gates and
**operator-driven re-planning**, a **BLAKE3 hash-chain journal** with
`agt why` audit, **subscription-first provider routing** with
**per-task-type budget caps**, **hot reload** of catalog + vault without
restart, a **vault encrypted at rest** with AES-256-GCM and **passphrase
rotation**, and a **Linux warden** with `prlimit64`-enforced CPU/mem/FD
limits and process-group SIGKILL.

It reaches you on **eleven messaging channels** (Telegram, Slack, Discord, Matrix,
SMS, WhatsApp, Signal, email, Microsoft Teams, Home Assistant, and generic
webhooks) — inbound messages drive the agent (allowlisted, fail-closed; the
account's own messages skipped so a reply never loops), outbound carries replies and
Pulse briefs. Each channel can run **multiple accounts at once** (e.g. 10 email
mailboxes, several bots) via guided **Connect pages** with per-channel help, QR /
gateway / **OAuth** ("Connect with Slack/Mastodon") sign-in, and two-way email over
IMAP/POP — see [`docs/CONNECT.md`](docs/CONNECT.md). **Official client SDKs** in
**Python** (sync + asyncio), **TypeScript**,
and **Rust** wrap the REST API; a **plugin/skill marketplace** (`agt plugin
registry` / `agt skill registry`) installs from a remote catalog with **BLAKE3
verification**; a supervised **public tunnel** (cloudflared/ngrok/custom) can
expose the Web UI or REST API to the internet on demand; and you can **talk to it**
— `agt transcribe <file>` and `agt listen` turn audio (a file or the microphone)
into text via any OpenAI-compatible speech-to-text endpoint and feed it to the
agent.

## Quick start

**The one-command path:** after `make build`, run `agt quickstart` — it syncs
the catalog, prompts for a provider key, and prints your exact start command.
The full manual flow:

```bash
# 1. Build (puts agt + agezt in ./bin), or `make install` to put them on PATH:
make build

# 2. Sync the model catalog. Works OFFLINE — no daemon needed:
agt catalog sync --local

# 3. Add credentials for a provider you have a key for. `provider setup`
#    lists who needs a key and prompts on stdin (never argv → no shell history):
agt provider setup                       # what still needs a key?
agt provider setup minimax-coding-plan   # prompt + store MINIMAX_API_KEY
#    Any models.dev provider works (anthropic, openai, …). Ollama needs no
#    key: `agt catalog discover` then use AGEZT_PROVIDER=ollama-local.

# 4. Start the daemon (terminal 1) — pick the provider + model, and
#    optionally expose the Web UI on loopback. AGEZT_WORKSPACE="$PWD" lets the
#    file tool read the directory you launch from (default: a sandboxed
#    ~/.agezt/workspace); omit it to keep the file tool sandboxed.
AGEZT_PROVIDER=minimax-coding-plan AGEZT_MODEL=MiniMax-M2.7 \
  AGEZT_WORKSPACE="$PWD" AGEZT_WEB_ADDR=127.0.0.1:8787 ./bin/agezt

# 5. In another terminal — verify, then use it:
agt doctor                # preflight: daemon, journal integrity, tools, skew
agt provider check        # live roundtrip (latency + cost)
agt run "list the files here and tell me what this project is"
agt why <event_id>        # walk the audit chain for any event
agt halt                  # freeze everything instantly
```

If `AGEZT_WEB_ADDR` is set, the banner prints a tokenized URL — open it for a
**Chat** view (the default: type an intent and watch the governed loop answer
live — streaming text, the tool calls it made with their policy verdict, and the
final answer with its real cost), an
**Activity** live monitor (is anything running right now, and what is it doing? —
in-flight runs with their current step, iteration, elapsed time and spend,
delegated sub-agents nested under the lead run, all folded live off the event
firehose), a
live event monitor (filterable by event kind); a real-time **Mission Control**
(rolling per-second rates incl. delegations) and an **AI Analyst** that reasons
about the running system; an **Alerts** feed of the daemon's own proactive
signals (with a header bell visible from every view); a **Catalog** of the
agent's full capability surface (tool → governing capability → trust level,
editable inline); a multi-agent **Agents** graph where any node opens its steer
cockpit; read panels (status / runs — click one for its full event arc / stats
with an outcome bar / budget / cache savings / providers routing view / tools /
policy / world / skills / memory / inbox); and management cockpits for
**Schedules**, **Standing orders** and **Reflection**, all refreshed live off
the event stream, plus operator controls (HALT, approve/deny, pause/steer a
specific run or sub-agent, promote/forget). A provider-fallback warning badge appears
when a primary provider is erroring — click it to see the underlying fallback
events. Localhost-bound and token-authed. See **[docs/CONSOLE.md](docs/CONSOLE.md)** for
a guided tour of the views and operator controls (steering the proactive heartbeat,
backup & restore, the policy/redaction testers, journal-integrity verify, and more).

**Drive Agezt from any OpenAI client.** Set `AGEZT_API_ADDR=127.0.0.1:8799` and
the daemon serves an OpenAI-compatible API (`POST /v1/chat/completions`,
`POST /v1/responses`, `GET /v1/models`) — point any OpenAI SDK/IDE at it with
the printed Bearer token. Both the Chat Completions and the newer Responses API
shapes are supported (streaming + non-streaming). Every request runs the full
agent loop through Edict + the journal (not a raw passthrough), and the response
carries an `agezt_correlation_id` you can `agt why`:

```bash
curl http://127.0.0.1:8799/v1/chat/completions \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"model":"agezt","messages":[{"role":"user","content":"what is this project?"}]}'
```

**Push events out to your own systems.** Set `AGEZT_WEBHOOKS` to a comma-list of
`url|subject|secret` sinks and the daemon POSTs every matching journal event to
your endpoint as it happens — `task.completed`, `policy.decision`,
`webhook.failed`, anything. The `subject` is a bus pattern (`agent.>`,
`edict.>`, `>` for all); with a `secret` each POST carries an
`X-Agezt-Signature: sha256=…` HMAC so you can verify authenticity. Every
delivery is itself journaled (`webhook.delivered` / `webhook.failed`), retried
on failure, and never loops:

```bash
AGEZT_WEBHOOKS='https://hooks.example.com/agezt|agent.>|my-signing-secret' ./bin/agezt
```

**Or drive it natively over REST.** Set `AGEZT_REST_ADDR=127.0.0.1:8800` for a
first-party `/api/v1` surface with Agezt-native semantics: `POST /api/v1/runs`
submits an intent (sync JSON, or an SSE event stream with `"stream":true`) and
returns a `correlation_id`; `GET /api/v1/runs/{correlation_id}` returns that
run's full journaled event arc; plus `GET /api/v1/health` and
`GET /api/v1/models`. The `/api/v1/mailbox` routes open the shared inter-agent
message board to apps: send a DM to an agent by name (or broadcast with
`"to":"*"`), read an inbox, reply, and acknowledge — a directed message wakes a
standing order watching `board.dm.<name>`, so external mail can trigger an
agent. Same governed loop, loopback-bound + Bearer-token:

```bash
curl -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"intent":"what is this project?"}' http://127.0.0.1:8800/api/v1/runs
```

**Or use an official client SDK.** Dependency-light clients wrap `/api/v1` —
`health` / `models` / `run` / `run_stream` (SSE) / `get_run`, the mailbox
(`mailbox_send` / `mailbox_inbox` / `mailbox_ack` / replies / topics), bearer
auth, multi-tenant aware — in **Python** (`pip install agezt`; sync `Client` + asyncio
`AsyncClient`), **TypeScript** (`@agezt/sdk`, zero runtime deps over `fetch`), and
**Rust** (the `agezt` crate, standard-library only). See [`sdk/`](sdk/).

**Expose it to the internet when you need to.** Set `AGEZT_TUNNEL=cloudflared`
(or `ngrok`, or `AGEZT_TUNNEL_CMD=<command>`) and the daemon supervises the tunnel
binary, prints the public URL, and tears it down on exit — opt-in, so nothing is
public unless you ask.

**Let schedules wake work without becoming prompts.** The daemon treats schedules
as typed cron/event triggers: wake an agent, run a workflow, invoke a system task,
or call an approved tool. Agents keep their own identity, memory, task list,
skills, model/provider settings, retry policy, mailbox, and lifecycle state;
the schedule only says what to wake, when, and under which cadence. Manage
schedules live with `agt schedule` (persisted across restarts, reversible). Every
firing is journaled (`schedule.fired`), so `agt why` links autonomous work back to
the trigger:

```bash
agt schedule add "cycle inbox and brief me" --agent daily-brief --every 2h     # wake an agent; its soul/tasks decide work
agt schedule add --workflow repo-digest --at 09:30           # run a reusable chain
agt schedule add --system-task catalog_sync --every 24h      # sync models.dev/api.json without waking an agent
agt schedule add --tool log_cleanup --every 6h               # invoke an approved internal tool
agt schedule add "standup nudge" --agent standup --at 09:30 --days mon-fri     # weekdays only
agt schedule add "weekend digest" --agent digest --at 11:00 --days weekends
agt schedule add "poll queue" --agent queue-watcher --every 15m --between 09:00-17:00 --days mon-fri  # windowed interval
agt schedule add "ny standup" --agent ny-standup --at 09:00 --days mon-fri --tz America/New_York      # wall-clock in any IANA zone
agt schedule add "release check" --agent release-check --in 30m                # one-shot, then self-removes
agt schedule add --workflow deploy-recap --once --at 18:00   # one-shot at a wall-clock time
agt schedule edit <id> --at 10:00 --days mon-fri             # change cadence in place (id preserved)
agt schedule list            # id, cadence, source, next run
agt schedule run <id>        # fire now (next tick)
agt schedule pause <id>      # disable without deleting (resume re-enables)
agt schedule rm <id>         # reversible
# or seed at startup:
AGEZT_SCHEDULE='24h=security auditor cycle' ./bin/agezt  # legacy interval=agent-task seed
```

For the full operator cheat sheet: `agt help`.  Day-to-day commands:

```
agt run "<intent>"                     one-shot intent (LLM ↔ tools loop)
agt doctor                             health preflight (exit 1 = a check failed)
agt status / agt runs last             daemon health · last run as a task arc
agt provider setup [id] / check        add keys · verify a live roundtrip
agt provider chatgpt login|import      Sign in with ChatGPT (subscription, no key)
agt catalog sync [--local]             refresh models.dev (offline-capable)
agt memory … / agt world … / agt skill …   the cognitive loop (add/list/forget/…)
agt reflect run                        review behaviour, decay stale knowledge
agt approvals / approve / deny         the HITL queue
agt send --channel … / agt ha …        push a message · control Home Assistant
agt transcribe <file> --run            speech-to-text a file → drive the agent
agt listen --seconds 10 --run          record the mic → transcribe → drive the agent
agt why <id> --payload                 walk the audit chain
agt halt / resume / shutdown           stop · resume · graceful exit
```

## Where the design lives

The full spec suite is under [`.project/`](.project/) and remains the
binding source of authority:

- [`.project/BUILD-GUIDE.md`](.project/BUILD-GUIDE.md) — start here
- [`.project/DECISIONS.md`](.project/DECISIONS.md) — supreme authority
- [`.project/STRUCTURE.md`](.project/STRUCTURE.md) — repo layout
- [`.project/SPEC-*.md`](.project/) — 16 component specs
- [`.project/PHASE-*-REPORT.md`](.project/) — every shipped phase, its
  scope and trade-offs (47+ reports from M1.a through M1.zz and beyond)

Operator how-to guides live under [`docs/`](docs/):
- [`docs/CONNECT.md`](docs/CONNECT.md) — connect providers (incl. **Sign in with
  ChatGPT**) and channels (multi-account, guided Connect, OAuth, two-way email)
- [`docs/CONSOLE.md`](docs/CONSOLE.md) — the Web UI console
- [`docs/AGENT-SDK-ARCHITECTURE.md`](docs/AGENT-SDK-ARCHITECTURE.md) — the client SDKs

## What's built

The v1 substrate. Highlights:

**Kernel** (`kernel/`)
- `agent` — single-agent tool-loop with streaming
- `bus` — pattern-subscribed event bus (NATS-style wildcards)
- `journal` — append-only JSONL + BLAKE3 hash chain
- `state` — file-backed mutable store
- `event` — typed events with `IsEphemeral()` discriminator for
  streaming tokens
- `governor` — per-task routing + fallback chain + USD-microcents
  budget cap with **per-task-type daily ceilings**, subscription-first
- `scheduler` — DAG executor with LoopNode + GateNode
- `planner` — LLM → validated `scheduler.Plan` JSON, with
  **operator-driven refinement** (`agt plan refine`)
- `controlplane` — line-delimited JSON over TCP between
  `agt` ↔ `agezt`; includes operator visibility commands
  (`status`, `tool list`, `plugin list`, `budget`, `why --json/--payload`)
- `creds` — credential vault, AES-256-GCM at rest, passphrase rotation,
  pure-stdlib AWS chain (vault → env → SSO → STS-AssumeRole → IRSA/web-identity
  → ~/.aws + IMDS); keyless ambient credentials on EKS (IRSA) and — for Vertex —
  GKE/GCE via the metadata server
- `catalog` — models.dev integration; hot reload
- `approval` — HITL queue (with `--json` for automation)
- `edict` — declarative policy engine
- `warden` — process isolation: Linux `prlimit64` + process-group SIGKILL;
  no-op stubs on macOS/Windows
- `runtime` — wires it all into `Kernel.Open(Config) → *Kernel`
- `plugin` — out-of-process plugin host (stdio JSON protocol) with
  **hot-reload**, **BLAKE3 pin gating**, **tool allowlists**,
  **streaming progress**, and **plugin→host callbacks**

**Providers** (`plugins/providers/`)
- `anthropic`, `openai`, `google`, `vertex` (Gemini + Anthropic on Vertex;
  service-account key **or** GKE/GCE metadata-server creds),
  `bedrock` (bearer + SigV4 + AI21 Jamba + Cohere + Llama + Mistral),
  `cohere`, `ollama` (local, incl. **vision** models like llava/llama3.2-vision),
  `compat` (OpenAI-compatible vendors: Groq, DeepSeek,
  xAI, OpenRouter, Together, ...), Azure OpenAI, Mistral. Every family
  has working streaming; image input on every multimodal-capable family.
- `openairesponses` — **"Sign in with ChatGPT"**: use a ChatGPT Plus/Pro
  **subscription** as a provider (no API key) over the Responses backend. OAuth
  sign-in from the UI or `agt provider chatgpt login`. Unofficial backend — see
  [`docs/CONNECT.md`](docs/CONNECT.md) for the terms/risk caveat.

**Tools** (`plugins/tools/`)
- `shell` — warden-isolated subprocess
- `file` — scoped to `AGEZT_WORKSPACE`
- `http` — GET/POST with host allowlist
- `browser.read` — fetch + HTML→text extraction, opt-in cookie jar
- `web_search` — keyword search against a keyless public engine (DuckDuckGo);
  returns `{title, url, snippet}` so the agent can DISCOVER a URL, then read it
  with `http`/`browser.read`. SSRF-guarded, fail-soft
- `schedule` — the agent arranges typed future wakes in the cadence store
  (once after a delay / recurring / daily / continuous); a schedule later wakes
  an agent, workflow, system task, or approved tool. Tagged `source=agent` for
  operator visibility
- `delegate` — spawn a bounded sub-agent for a focused subtask (multi-agent
  fan-out); depth- AND tree-total-bounded, individually steerable, journaled,
  each sub-action gated through Edict
- `coding` — delegate a coding task to an external agent (Claude Code / Codex /
  Aider / any command) in an isolated git worktree; returns the diff, never
  merges. Off unless `AGEZT_CODING_CMD` is set
- `acp_agent` — delegate a task to an external agent over the Agent Client
  Protocol (Claude Code / Codex / Gemini CLI / any ACP agent), spawned over
  stdio and driven via JSON-RPC; relays its answer. Off unless
  `AGEZT_ACP_AGENT_CMD` is set
- `remote_run` — delegate a task to a peer Agezt node over its native REST
  API (`/api/v1/runs`); the peer runs it through its own governed loop and
  reports back with its correlation id. The mesh primitive — cooperating
  nodes. Off unless `AGEZT_PEERS` (`name=url|token,…`) is set
- `homeassistant` — read smart-home entity state (`get_states`) and call
  Home Assistant services (`call_service`) to control the house (lights,
  climate, locks, …). Fail-closed on two axes: a read-entity allowlist
  (`AGEZT_HOMEASSISTANT_TOOL_READ`) and a service allowlist
  (`AGEZT_HOMEASSISTANT_TOOL_SERVICES`), mapped to distinct Edict capabilities
  (`homeassistant.read` = Allow, `homeassistant.call` = Ask-first). Off unless
  the HA URL/token and at least one allowlist are set

- `mcpbridge` — Model Context Protocol bridge, both stdio and HTTP+SSE
  transports

**Plugin SDK** (`plugins/sdk/`)
- `sdk` — the official Go authoring kit: `sdk.Serve(sdk.Tool{...})` handles the
  whole stdio JSON protocol (frame demux, write serialisation, progress via
  `Emit`, host callbacks via `CallHost`, panic containment) so a plugin is just
  its tool logic. Stdlib-only — imports no kernel package. See
  `plugins/sdk/example/greet` for a complete runnable plugin. Scaffold your own
  with **`agt plugin new <name>`** — it generates a buildable SDK plugin
  (gofmt-clean `main.go`, `go.mod`, README) ready to `go build` and wire

**Binaries**
- `cmd/agezt` — the daemon
- `cmd/agt` — the operator CLI

## Verify

```bash
make test     # 2315 tests, all green
make build    # produces bin/agezt + bin/agt
make gen      # regenerate SDK types from the contract
```

Or without `make`:

```bash
go test ./...
go build ./...
go run ./tools/jsonschemagen -in .project/agezt-contract.jsonc -out contract/gen/types.gen.go -pkg gen
```

## What's deferred (post-v1)

Genuine remaining deferrals — every one is blocked on a non-stdlib
dependency, a CGO requirement, or a substantial design phase:

- **Plugin sandboxing** — out-of-process plugins are isolated only at the
  process boundary today; per-plugin warden profiles (cgroup v2 + seccomp
  BPF + user-namespace) need either non-stdlib bindings or per-OS CGO.
- **Browser tool — JS rendering, screenshots, search** — would require
  `chromedp` (a CGO/non-stdlib dependency).
- **Vault — OS-keychain auto-integration, argon2 KDF** — both need per-OS
  CGO or non-stdlib bindings; PBKDF2-SHA-256 is the stdlib fallback today.
- **Planner v2 — sub-planners, planner-side tool calls** — operator-driven
  refinement shipped (`agt plan refine`); recursive sub-planning is a
  separate design phase.
- **Pulse v2 — TUI** — non-stdlib (Bubble Tea / tview). Programmatic
  observability is otherwise complete: `agt pulse`, `agt status`,
  `agt tool list`, `agt plugin list`, `agt budget`, `agt why --json/--payload`,
  `agt journal tail`, `agt edict show`/`edict test`, `agt state list`/`state get`,
  `agt plan visualize`, `agt plan run --dry-run`, `agt shutdown`.
- **Windows job objects / macOS sandbox-exec** — both need per-OS CGO
  bindings; in M1.d Linux got `prlimit64` (raw syscall, stdlib).

## License

MIT. See [`LICENSE`](LICENSE). Dependency policy: every external
dep requires a written justification in
[`DEPENDENCIES.md`](DEPENDENCIES.md). Current count: 1 + 1 transitive.
