# Agezt

> An open-source (MIT) **agentic operating system**: a stdlib-first Go core
> that turns intent into auditable, reversible action; runs a fleet of durable
> agents under a policy/trust system; proves its work before calling it done;
> proactively informs you (Pulse); and extends via in-process or out-of-process
> plugins.
> **Autonomous, under your authority.**

**Status:** `v1.1.0` (September 2026), active pre-release. This is a running Go
daemon + CLI + embedded React console, not a design suite: durable agents, a
typed workboard with OKR roll-up, schedules and standing orders, node-graph
workflows, memory/world/skills/taste, provider+catalog plumbing, policy, audit,
budget, and recovery surfaces are implemented and being tightened against the
autonomous-Jarvis acceptance bar. Any **OpenAI client or IDE** can drive it
through the OpenAI-compatible and ACP surfaces; peer, channel, and marketplace
surfaces remain capability- and environment-dependent. See
[CHANGELOG.md](CHANGELOG.md).

- **Docs index:** [docs/index.md](docs/index.md) — security, operations, API stability, SDK parity, runnable demos
- **Positioning:** [docs/COMPARISON.md](docs/COMPARISON.md) — how AGEZT differs from generic agent frameworks, without unverifiable competitor claims
- **Dependencies:** [DEPENDENCIES.md](DEPENDENCIES.md) — the `go.mod`-derived inventory and the written justification each direct dependency needs
- **Latest review artifact:** [docs/SYSTEM-AUDIT-REPORT.md](docs/SYSTEM-AUDIT-REPORT.md)

Local gates for a change are `go vet ./...`, `go test ./...`, frontend
`npm test` + `npm run build`, and the Playwright console E2E against a live
demo daemon (`make e2e`, `make webui-e2e`).

## What it is

One Go daemon (`agezt`) and one operator CLI (`agt`). The daemon runs a
governed agent loop: every model call is routed and budgeted, every tool call is
gated by a policy capability, and every step is appended to a BLAKE3
hash-chained journal you can walk backwards with `agt why`.

```
intent ──▶ governor (route · fallback chain · budget)
             │
             ▼
        agent loop ──▶ Edict policy ──▶ tool  (allow / ask a human / deny)
             │                            │
             ▼                            ▼
        journal (hash-chained, replayable) ──▶ Pulse · console · webhooks · SDKs
```

Nothing runs unattributed: a run, a delegated sub-agent, a scheduled wake, an
inbound Telegram message, and a REST call all land in the same journal under one
correlation id.

## The CLI at a glance

`agt help` is the full cheat sheet; `agt help <command>` (or `agt <command> -h`)
is one command's usage. The command surface, by job:

```
Getting started          quickstart · doctor · status · version · help
Run & control            run · halt · resume · runs · why · conductor · research ·
                         approvals · approve · deny · whoami
Plans & automation       plan · schedule · standing · workflow · workboard · okr ·
                         taste · seats · agent · toolforge · mcp · market
Providers & models       catalog · provider · budget · tool · cache · tenant
Memory & knowledge       memory · world · skill · reflect · state · artifact
Journal & audit          journal · pulse · changelog · edict · warden · exec-profile ·
                         redact · compare · netguard · ratelimit · webhook
Console, config & data   config · web · configcenter · token · vault · backup ·
                         restore · rollback · disk
Channels & integrations  inbox · send · channel · ha · transcribe · listen · peers ·
                         acp · overseer · plugin
Daemon                   shutdown
```

Day to day:

```bash
agt run "summarise the latest commits and brief the team"   # one governed run
agt runs last                       # the last run, replayed as a task arc
agt why <event_id> --payload        # walk that run's audit chain
agt research "what changed in X"    # decompose → gather sources → cited answer
agt conductor "<hard task>"         # Thinker/Worker/Verifier on 3 models
agt approvals --json                # the HITL queue, machine-readable
agt budget                          # today's spend vs daily + per-task caps
agt halt                            # freeze everything, instantly and reversibly
```

## Capabilities

**Providers — 9 adapter families, catalog-driven.** Anthropic, OpenAI (plus the
OpenAI-compatible vendors: Groq, DeepSeek, xAI, Cerebras, Together, DeepInfra,
Perplexity, Fireworks, Moonshot, OpenRouter, …), Google (Gemini API) and Google
Vertex (service-account key **or** GKE/GCE metadata creds), Mistral, Cohere,
Ollama (local, incl. vision models), AWS Bedrock (bearer + SigV4 +
STS-AssumeRole + SSO + IRSA/web-identity), and Azure OpenAI. Provider entries
come from the synced **models.dev catalog** (214 providers at the time of
writing), so a new vendor usually needs a key, not code. Every family streams;
image input works on every multimodal-capable family; extended thinking is
opt-in where the family supports it.

There is **no default provider or model.** `AGEZT_PROVIDER` / `AGEZT_MODEL` set
one, or per-task routing and named fallback chains resolve one per run — a run
that resolves no model fails *that run* with an actionable error rather than
silently picking something.

**"Sign in with ChatGPT."** A ChatGPT Plus/Pro **subscription** can act as a
provider with no API key, over the Responses backend (`agt provider chatgpt
login`, or the console). Unofficial backend — see
[`docs/CONNECT.md`](docs/CONNECT.md) for the terms/risk caveat.

**Tools — 30 registered by default.** Each is gated on a policy capability,
carries a rollback class, and is visible to the operator (`agt tool list`):

| | |
|---|---|
| System | `shell` (warden-isolated), `file` (workspace-scoped read/write/list/search/edit), `code_exec` (deno/node/python sandbox with persistent projects) |
| Web | `http`, `browser.read`, `web_search`, `fetch` (download → artifact), `research` (deep-research harness, every claim cited) |
| Multi-agent | `delegate` + `delegate_await` (bounded, nestable, async fan-out), `conductor` (Thinker/Worker/Verifier), `council` (multi-model panel), `board` (shared mailbox + DMs + help requests), `overseer` (supervise and intervene on the fleet) |
| Durable work | `workboard` (typed task queue), `workflow` (node-graph automation), `schedule` (typed future wakes), `standing` (event/cron wake rules), `runs` (recall its own past runs) |
| Knowledge | `memory` (private by default, `shared:true` opt-in), `world` (entity/relation graph), `skill` (learn → shadow → active, with agentskills.io bundles), `db` (Personal Data Lake collections), `artifacts` |
| Self-extension | `tool_forge` (write a script → test → operator-approved promotion to `forge_<name>`), `mcp` (install/attach MCP servers at runtime), `market` (install capability packs), `config` (a skill configures itself) |
| Reach out | `notify`, `send_media`, `homeassistant`, `introspect` (read this daemon's own live state) |

Env-gated tools: `browser.action` and its verb wrappers
(`AGEZT_BROWSER_ACTIONS=1` — Playwright actions, snapshots, screenshots, cookie
inspection, download capture, session profiles, persistent `tab_id`), `coding`
(`AGEZT_CODING_CMD` — hand a task to Claude Code / Codex / Aider in an isolated
git worktree; returns a diff, never merges), `acp_agent`
(`AGEZT_ACP_AGENT_CMD`), `remote_run` (`AGEZT_PEERS` — delegate to a peer AGEZT
node over its REST API), and `homeassistant` (URL + token + an explicit
read-entity allowlist and a service allowlist, mapped to two distinct
capabilities).

**Channels — 34 registered, most of them two-way.** Telegram, Slack, Discord,
Matrix, Signal, WhatsApp (direct + gateway), SMS (Twilio), email (SMTP out,
IMAP/POP in), Microsoft Teams, Mattermost, Rocket.Chat, Zulip, Google Chat, IRC,
Mastodon, Nostr, LINE, Feishu/Lark, DingTalk, WeCom, WeChat, QQ, Zalo, Twitch,
iMessage, Nextcloud Talk, Synology Chat, Home Assistant, ntfy, Gotify, Pushover,
Pushbullet, and generic webhooks — `agt channel list` shows each one's live
state. Inbound messages drive the agent, allowlisted and fail-closed, with the
account's own messages skipped so a reply never loops; outbound carries replies
and Pulse briefs. Each channel can run **multiple accounts at once** (ten
mailboxes, several bots) through guided **Connect pages** with per-channel help
and QR / gateway / **OAuth** sign-in. See [`docs/CONNECT.md`](docs/CONNECT.md).

**Voice.** `agt transcribe <file>` and `agt listen` turn audio into text through
any OpenAI-compatible STT endpoint and can feed it straight to the agent
(`--run`). The console's hands-free **Voice mode** listens, runs the agent, and
speaks the answer back sentence-by-sentence as it streams, stopping the moment
you talk over it (barge-in), with an optional wake word. It uses the configured
`AGEZT_STT_*` / `AGEZT_TTS_*` backends and falls back to the browser's built-in
speech when they are unset.

## Quick start

**The one-command path:** build, then run `agt quickstart` — it syncs the
catalog, prompts for a provider key, and prints your exact start command.

```bash
# 1. Build. `go build ./...` (what `make build` runs) compiles everything but
#    writes no binaries — ask for them by name:
go build -o agezt ./cmd/agezt
go build -o agt   ./cmd/agt
#    Windows: agezt.exe / agt.exe. `make install` puts the daemon on your GOPATH
#    bin. On Ubuntu, `sudo ./install.sh install` builds it and installs a
#    systemd service instead.

# 2. Sync the model catalog. Works OFFLINE — no daemon needed:
./agt catalog sync --local
./agt catalog list                         # providers + models + pricing

# 3. Add credentials for a provider you have a key for. `provider setup` lists
#    who needs a key and prompts on stdin (never argv → no shell history):
./agt provider setup                       # what still needs a key?
./agt provider setup deepseek              # prompt + store DEEPSEEK_API_KEY
#    Any models.dev provider works. Ollama needs no key:
#    `agt catalog discover`, then AGEZT_PROVIDER=ollama-local.

# 4. Start the daemon (terminal 1), naming a provider and one of ITS models
#    from step 2 (model ids move fast — read them out of the catalog, don't
#    copy them out of a README). AGEZT_WORKSPACE="$PWD" lets the file tool read
#    the directory you launch from (default: a sandboxed ~/.agezt/workspace);
#    omit it to keep the file tool sandboxed.
AGEZT_PROVIDER=<provider-id> AGEZT_MODEL=<model-id> \
  AGEZT_WORKSPACE="$PWD" AGEZT_WEB_ADDR=127.0.0.1:8787 ./agezt

# 5. In another terminal — verify, then use it:
./agt doctor              # preflight: daemon, journal integrity, tools, skew
./agt provider check      # live roundtrip (latency + cost)
./agt run "list the files here and tell me what this project is"
./agt why <event_id>      # walk the audit chain for any event
./agt halt                # freeze everything instantly
```

**Working on AGEZT itself?** `./dev.ps1` (Windows) and `./dev.sh` (macOS/Linux)
are the one-shot dev loop: they build both binaries, seed an **isolated
`.dev-home`** so a dev run can never touch your real `~/.agezt`, load provider
keys from `.env` into that dev vault, warn when the checkout is behind its
upstream (the console is embedded at build time, so a stale checkout ships an
old UI), and start the daemon. Flags: `-Fresh`/`--fresh`,
`-SkipBuild`/`--skip-build`, `-Pull`/`--pull`, `-WebAddr`/`--web-addr`. Frontend
commands run from `frontend/` (`npm test`, `npm run build` — npm, not pnpm).

## The console

With `AGEZT_WEB_ADDR` set, the startup banner prints a tokenized URL. The
console is **64 views, folded into 36 rows across 8 sections**, organized by
operator job rather than by backend package — a section is a job, a row is a
noun, and a tab is a facet of that noun (see
[docs/CONSOLE-IA.md](docs/CONSOLE-IA.md)):

- **Talk** — Jarvis (the presence view) · Chat (the streaming answer, every tool call with its policy verdict, the run's real cost) · Voice · Messages (Inbox / Agent Board)
- **Observe** — Overview (Overview / Mission Control / Live Stream) · Runs (Runs / Activity / Insights / Replay) · Health (Health / Prompt cache / Tool usage / Routing log) · Alerts · Budget
- **Automate** — Wizards · Workflows (Workflows / Flow Studio) · Work (Workboard / Objectives) · Triggers (Schedules / Standing orders) · Autonomy
- **Govern** — Approvals · Policy · Oversight (Overseer / Council / Conductor) · Seats
- **Agents** — Agents · Roster · Skills · Capabilities (Tool registry / Toolbox / Tool Forge / Marketplace / Execution Profiles) · Sandbox
- **Knowledge** — Memory (Memory / Taste) · World · Thinking (Research / Analyst / Reflection) · Search · Data & Files (Data Lake / Artifacts & Files / Storage)
- **Connect** — Providers & Models (Models & Keys) · Routing (Routing / Fallback Chains) · Channels · Integrations (MCP Servers / ACP Agents / Connections)
- **Admin** — Setup · Config Center · Identity (Default Identity / Prompts) · Backup

The Activity tab shows what is running this second — each in-flight run with its
current step, iteration, elapsed time and spend, delegated sub-agents nested
under their lead run — folded live off the event firehose.

Every view refreshes live off the event stream, with operator controls
throughout: HALT, approve/deny, pause or steer a specific run or sub-agent,
promote/forget. Localhost-bound and token-authed; `AGEZT_WEB_PASSWORD` adds a
password door, and `AGEZT_WEB_PASSWORD_STRICT=on` enforces token-and-password on
every data request. See **[docs/CONSOLE.md](docs/CONSOLE.md)** for the guided
tour.

## Integrate

**Any OpenAI client.** Set `AGEZT_API_ADDR=127.0.0.1:8799` and the daemon serves
`POST /v1/chat/completions`, `POST /v1/responses`,
`POST /v1/audio/transcriptions`, and `GET /v1/models` — point any OpenAI SDK or
IDE at it with the printed Bearer token. Both API shapes stream. Every request
runs the full governed loop through Edict and the journal (not a raw
passthrough), and the response carries an `agezt_correlation_id` you can
`agt why`:

```bash
curl http://127.0.0.1:8799/v1/chat/completions \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"model":"agezt","messages":[{"role":"user","content":"what is this project?"}]}'
```

**Native REST.** `AGEZT_REST_ADDR=127.0.0.1:8800` exposes `/api/v1` with
AGEZT-native semantics: `POST /api/v1/runs` submits an intent (sync JSON, or an
SSE stream with `"stream":true`) and returns a `correlation_id`;
`GET /api/v1/runs/{id}` returns that run's full journaled event arc; plus
`/api/v1/health`, `/api/v1/models`, `/api/v1/artifacts`, and `/api/v1/update`.
The `/api/v1/mailbox` routes open the shared inter-agent board to apps — send a
DM to an agent by name (or broadcast with `"to":"*"`), read an inbox, reply,
ack, or `watch` the stream. A directed message wakes a standing order watching
`board.dm.<name>`, so external mail can trigger an agent:

```bash
curl -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"intent":"what is this project?"}' http://127.0.0.1:8800/api/v1/runs
```

**Official client SDKs.** Dependency-light clients wrap `/api/v1` — `health` /
`models` / `run` / `run_stream` (SSE) / `get_run`, the mailbox (send, inbox,
reply, ack, topics, watch), bearer auth, multi-tenant aware — in **Python** (`pip install agezt`; sync
`Client` + asyncio `AsyncClient`), **TypeScript** (`@agezt/sdk`, zero runtime
dependencies over `fetch`), and **Rust** (the `agezt` crate, standard library
only). See [`sdk/`](sdk/) and the CI-enforced
[`docs/SDK-PARITY.md`](docs/SDK-PARITY.md).

**Outbound webhooks.** `AGEZT_WEBHOOKS` takes a comma-list of
`url|subject|secret` sinks; the daemon POSTs every matching journal event as it
happens. The `subject` is a bus pattern (`agent.>`, `edict.>`, `>` for all);
with a `secret` each POST carries an `X-Agezt-Signature: sha256=…` HMAC. Every
delivery is itself journaled (`webhook.delivered` / `webhook.failed`), retried
on failure, and never loops:

```bash
AGEZT_WEBHOOKS='https://hooks.example.com/agezt|agent.>|my-signing-secret' ./agezt
```

**Public exposure, on demand.** `AGEZT_TUNNEL=cloudflare` opens an automatic
`https://*.trycloudflare.com` Quick Tunnel; `ngrok`, `tailscale`,
`tailscale-funnel`, and `AGEZT_TUNNEL_CMD=<command>` also work. The daemon
targets the live console, adds the discovered public host to the allowlist,
prints the public URL, and tears the tunnel down on exit. Set a console password
before exposing anything.

**Multi-tenancy.** With `AGEZT_MULTITENANT=on`, `agt tenant create/list/token/rm`
manages isolated homes; a run routes with `agt run --tenant <id>` or the
`X-Agezt-Tenant` header, authenticates with that tenant's own token, and is
written to that tenant's own journal.

## Autonomy, and the proof that it worked

Work is woken by **typed triggers**, not by re-prompting. A schedule says what to
wake, when, and how often; the agent keeps its own identity, memory, tasks,
skills, model/provider settings, retry policy, mailbox, and lifecycle state.
Every firing is journaled (`schedule.fired`), so `agt why` links autonomous work
back to its trigger.

```bash
agt schedule add "cycle inbox and brief me" --agent daily-brief --every 2h
agt schedule add --workflow repo-digest --at 09:30              # a reusable chain
agt schedule add --system-task catalog_sync --every 24h         # no agent woken
agt schedule add --tool log_cleanup --every 6h                  # an approved tool
agt schedule add "standup nudge" --agent standup --at 09:30 --days mon-fri
agt schedule add "poll queue" --agent q --every 15m --between 09:00-17:00 --days mon-fri
agt schedule add "ny standup" --agent ny --at 09:00 --tz America/New_York
agt schedule add "release check" --agent release --in 30m       # one-shot, self-removing
agt schedule edit <id> --at 10:00 --days mon-fri                # cadence in place
agt schedule list / run <id> / pause <id> / rm <id>
```

Above that sits the durable work spine:

- **Workboard** (`agt workboard`) — a restart-safe typed task queue with status, priority, assignee, tenant, idempotency key, comments, links, claims, dependencies, stale-claim reclaim, and journaled transitions. Not a chat log, and not an agent.
- **Proof > vibes** — a task carries acceptance criteria; the **assure** loop runs it, verifies it, and retries with the verifier's gap fed back, bounded. A task reaches `done` only with a durable, checkable **proof** record attached.
- **OKRs** (`agt okr`) — objectives own key results; key results link workboard tasks and roll their proven completion into a percentage, so fleet activity reads as progress toward goals instead of a flat queue.
- **Seats** (`agt seats`) — task-facing presets for *how* a task runs (isolation surface, model tier, tool tier), layered on top of *who* runs it.
- **Taste** (`agt taste`) — operator-authored "what good looks like" exemplars injected into runs before the model acts.
- **Standing orders** (`agt standing`) — durable event/cron wake rules bound to an agent's governed task plan.
- **Workflows** (`agt workflow`) — node-graph automation (trigger/tool/llm/condition/transform/delay/http/code/map/filter/switch/merge/approval/subworkflow) that users, agents, schedules, and webhooks all run from one saved graph.
- **Pulse** — the proactive heartbeat: it observes the running system, briefs you, and with `AGEZT_PULSE_INITIATIVE` can turn an actionable observation into a governed run under a trust ceiling.
- **Restart resume** — in-flight runs survive shutdown, self-update, and hard kill: a durable ticket per root run carries the accumulated conversation so the work is re-dispatched instead of abandoned.
- **Self-repair** — a seeded guardian fleet claims broken, degraded, or routing-unstable agents, drives a governed repair pass, and escalates through the mailbox when a repair fails.

## Under your authority

- **Edict** — a declarative policy engine over 36 capabilities (`shell`, `file.write`, `http.post`, `code.exec`, `delegate`, `mcp.install`, `oversee`, `config.write`, …). Every tool call resolves to exactly one capability and lands on allow / ask-a-human / deny. An **unknown capability is default-denied**, so a typo kills a tool loudly instead of quietly widening it. `agt edict test shell "rm -rf /"` previews a decision without running anything.
- **Journal** — append-only JSONL with a BLAKE3 hash chain. `agt why <id>` walks every event sharing one correlation; `agt journal` verifies chain integrity; `agt changelog` folds system-level change out of it.
- **Approvals** — a real HITL queue (`agt approvals --json`, `approve`, `deny`) that blocks the calling run until an operator decides, with the effect class and affected resources shown in the prompt.
- **Budgets** — USD-microcents accounting with daily ceilings *and* per-task-type caps; subscription-first routing prefers a flat-rate provider before a metered one. `agt budget`, plus `agt cache` for prompt-cache savings.
- **Anomaly circuit breaker** — a spike in the global tool-call rate auto-engages a halt, so a looping agent cannot burn budget or take repeated action unsupervised.
- **Prompt-injection guard** — a precise causal-window gate over untrusted observations (`AGEZT_PROMPT_INJECTION_GUARD` = on / warn / off) with a "trust web content" operator toggle; a tool call carries the taint of the observation that motivated it.
- **Netguard** — the egress/SSRF guard; `agt netguard` tests a host and lists blocked dials.
- **Warden** — process isolation: on Linux, `prlimit64`-enforced CPU/memory/FD limits and process-group SIGKILL; a documented downgrade to setpgid-only on macOS and Windows, surfaced through `agt warden` and `agt exec-profile`.
- **Vault** — credentials encrypted at rest with AES-256-GCM (PBKDF2-HMAC-SHA-256), machine-bound auto-encryption, passphrase rotation, and a pure-stdlib AWS credential chain (vault → env → SSO → STS-AssumeRole → IRSA/web-identity → `~/.aws` + IMDS).
- **Recovery** — `agt backup` / `restore` (credentials excluded, journal head recorded), `agt rollback` for local mutation checkpoints, `agt disk` for space headroom, and `agt redact` to see exactly what the secret-scrubber would do to a string.

## What's built

**Kernel** (`kernel/`) — around 80 packages. The load-bearing ones:

| | |
|---|---|
| Loop | `agent` (tool-loop + streaming), `runtime` (composition root), `governor` (routing, fallback chains, budgets), `contextselect`, `intent` |
| Substrate | `bus` (NATS-style wildcard subscriptions), `journal`, `state`, `jsonstore`, `event` (with an `IsEphemeral()` discriminator for stream tokens) |
| Governance | `edict`, `approval`, `warden`, `netguard`, `redact`, `envscrub`, `anomaly`, `intervention`, `executionprofile`, `auth`, `tenant` |
| Work | `scheduler` (DAG executor with LoopNode + GateNode), `planner` (LLM → validated plan, with operator-driven refinement), `cadence`, `standing`, `workboard`, `workflow` + `workflowexec`, `assure`, `proof`, `okr`, `seat` |
| Fleet | `roster`, `delegation`, `board`, `selfrepair`, `resume`, `pulse`, `alerter` |
| Knowledge | `memory`, `worldmodel`, `skill`, `taste`, `reflect`, `datalake`, `artifact`, `market`, `toolforge`, `toolbox` |
| Edges | `controlplane` (line-delimited JSON over TCP between `agt` ↔ `agezt`), `httpserver`, `webui`, `restapi`, `openaiapi`, `acp`, `agentgw`, `channel` + `channelwire`, `webhook`, `tunnel`, `mcp`, `plugin`, `stt` / `voicetool`, `update` |
| Identity | `creds` (the vault), `catalog` (models.dev + hot reload), `configcenter`, `settings`, `convo` |

**Providers** (`plugins/providers/`) — `anthropic`, `openai`, `openairesponses`
(the ChatGPT subscription backend), `google`, `vertex`, `bedrock`, `cohere`,
`ollama`, and `compat` (the OpenAI-compatible vendor fan-out), plus `embed`,
`rerank`, `image`, and `voice` adapters.

**Tools** (`plugins/tools/`) — the 30 in the table above, each in its own package
with its capability declared next to the behaviour it describes.

**Channels** (`plugins/channels/`) — 25 packages backing the 34 registered
channel kinds.

**Plugin SDK** (`plugins/sdk/`) — the official Go authoring kit:
`sdk.Serve(sdk.Tool{...})` handles the whole stdio JSON protocol (frame demux,
write serialisation, progress via `Emit`, host callbacks via `CallHost`, panic
containment), so a plugin is just its tool logic. Stdlib-only — it imports no
kernel package. `plugins/sdk/example/greet` is a complete runnable plugin, and
**`agt plugin new <name>`** scaffolds a buildable one (gofmt-clean `main.go`,
`go.mod`, README). Out-of-process plugins get **hot-reload**, **BLAKE3 pin
gating**, **tool allowlists**, **streaming progress**, and **kernel callbacks** —
see [`docs/PLUGIN-SECURITY.md`](docs/PLUGIN-SECURITY.md). An **MCP bridge**
speaks both stdio and Streamable-HTTP/SSE, with a curated catalog of verified
server presets.

**Binaries** — `cmd/agezt` (the daemon), `cmd/agt` (the operator CLI).

## Where the design lives

The full spec suite is under [`.project/`](.project/) and remains the binding
source of authority:

- [`.project/BUILD-GUIDE.md`](.project/BUILD-GUIDE.md) — start here
- [`.project/DECISIONS.md`](.project/DECISIONS.md) — supreme authority
- [`.project/STRUCTURE.md`](.project/STRUCTURE.md) — repo layout
- [`.project/SPEC-*.md`](.project/) — 16 component specs
- [`.project/PHASE-*-REPORT.md`](.project/) — every shipped phase, its scope and trade-offs

Operator and integrator guides live under [`docs/`](docs/):
[`CONNECT.md`](docs/CONNECT.md) (providers, including Sign in with ChatGPT;
channels: multi-account, guided Connect, OAuth, two-way email),
[`CONSOLE.md`](docs/CONSOLE.md) and [`CONSOLE-IA.md`](docs/CONSOLE-IA.md),
[`OPERATIONS.md`](docs/OPERATIONS.md), [`THREAT-MODEL.md`](docs/THREAT-MODEL.md),
[`API-STABILITY.md`](docs/API-STABILITY.md),
[`EVENT-SCHEMA.md`](docs/EVENT-SCHEMA.md), and
[`AGENT-SDK-ARCHITECTURE.md`](docs/AGENT-SDK-ARCHITECTURE.md). Full map:
[`docs/index.md`](docs/index.md).

## Verify

```bash
make check       # gen + vet + go test + deps-check + sdk-parity + dead code + frontend
make test        # go test ./...
make e2e         # boot a real daemon, exercise every core surface, assert 0 panics
make webui-e2e   # Playwright against the embedded console (webui-e2e-ps on Windows)
make gen         # regenerate SDK types from the contract
```

Or without `make`:

```bash
go vet ./...
go test ./...
go run ./tools/jsonschemagen -in .project/agezt-contract.jsonc -out contract/gen/types.gen.go -pkg gen
cd frontend && npm test && npm run build
```

The build is **pure Go with `CGO_ENABLED=0`**, cross-compiled via `make linux` /
`darwin` / `windows`; binaries are stamped with a reproducible version + commit +
build-time triple and `-trimpath`.

## What's deferred (post-v1)

Genuine remaining deferrals — every one is blocked on a non-stdlib dependency, a
CGO requirement, or a substantial design phase:

- **Plugin sandboxing** — out-of-process plugins are isolated only at the process boundary today; per-plugin warden profiles (cgroup v2 + seccomp BPF + user namespaces) need either non-stdlib bindings or per-OS CGO.
- **Browser sessions — live tab lifecycle, DOM-level stale-ref invalidation** — `browser.action` and the verb wrappers already cover opt-in Playwright actions, snapshots, events, screenshots, cookie inspection, download capture, AGEZT-managed `profile=session` state carryover, and persistent `tab_id` URL/snapshot refs through an operator-installed Node driver; the rest is a dedicated design phase.
- **Vault — OS-keychain auto-integration, argon2 KDF** — both need per-OS CGO or non-stdlib bindings; PBKDF2-SHA-256 is the stdlib fallback today.
- **Planner v2 — sub-planners, planner-side tool calls** — operator-driven refinement shipped (`agt plan refine`); recursive sub-planning is a separate design phase.
- **Pulse v2 — TUI** — non-stdlib (Bubble Tea / tview). Programmatic observability is otherwise complete: `agt pulse`, `status`, `tool list`, `plugin list`, `budget`, `why --json/--payload`, `journal tail`, `edict show`/`test`, `state list`/`get`, `plan visualize`, `plan run --dry-run`, `shutdown`.
- **Windows job objects / macOS sandbox-exec** — both need per-OS CGO bindings; Linux got `prlimit64` as a raw stdlib syscall.

## License

MIT. See [`LICENSE`](LICENSE). Dependency policy: every external dependency
requires a written justification in [`DEPENDENCIES.md`](DEPENDENCIES.md), and
`make deps-check` enforces the allowlist. Current count: **5 direct** (BLAKE3, a
WebSocket client, an IMAP client, secp256k1 for Nostr, and `golang.org/x/net`)
plus their transitive graph — see the table for the resolved module list.
