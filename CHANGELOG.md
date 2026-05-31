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
- **`agt doctor` model-readiness check** (SPEC-08, M26) — the capability work
  (M23–M25) now lands in the operator's go-to diagnostic. `agt doctor` gains a
  `model readiness` line: OK when the running model advertises tool-use, WARN
  (with the advisory + a remediation hint) when it doesn't — so someone debugging
  "why won't my agent call tools?" sees the cause in the first command they run.
  Conservative like the rest of the triad: an offline/mock model, an unsynced
  catalog, or a model the catalog doesn't list is an informational OK, never a
  false FAIL. `agt status` now also reports the configured `model`. Proven live:
  doctor WARNs on a `tool_call=false` model and is OK on a tool-capable one. See
  `.project/PHASE-M26-DOCTOR-MODEL-READINESS-REPORT.md`.
- **Strict model-capability enforcement** (SPEC-15, M25) — the enforcement step
  after the M23/M24 advisories. `AGEZT_MODEL_STRICT=on` makes the Governor reject
  a tools-bearing request whose target model the catalog *knows* lacks tool-use,
  pre-flight — turning a confusing deep upstream failure into a clear
  `governor: model does not support tool-use` error before any provider is
  called, journaled as a `capability.rejected` event. Conservative by design:
  off by default (advisory-only), only blocks models the catalog actually knows
  (an unknown/local model is never blocked — a catalog-data gap must not break a
  working setup), and non-tool requests always pass. Per-tenant governors inherit
  it (the Config is copied by `WithLimits`). Proven live both ways: with strict
  on, a 7-tool run is rejected pre-flight and journaled; with strict off
  (default) the same run flows through the chain. See
  `.project/PHASE-M25-STRICT-CAPABILITIES-REPORT.md`.
- **Boot-time model advisory** (SPEC-15, M24) — the daemon now surfaces the M23
  agent-readiness check at startup: when the auto-selected primary model is in
  the catalog and doesn't advertise tool-use (or has a tiny context window), the
  banner prints a `model advisory : ⚠ …` line, using the same
  `catalog.Model.AgentWarnings` as `agt provider check --caps`. An operator who
  points the tool-driven loop at a model that can't call tools learns it the
  moment they boot, not deep in a failing run. Conservative by design: a model
  the catalog doesn't know (the offline mock, a bare local model) yields no line,
  not a false alarm. Proven live: booting on a `tool_call=false` model prints the
  advisory; a tool-capable model boots clean. See
  `.project/PHASE-M24-BOOT-ADVISORY-REPORT.md`.
- **Model capability inspection** (SPEC-15, M23) — the catalog tracked per-model
  capability flags (tool-use, reasoning, modalities, context window) but nothing
  surfaced or checked them, so pointing the tool-driven agent loop at a model
  that can't call tools failed deep in a run with a cryptic upstream error. `agt
  provider check --caps [<id>]` now reports a model's capabilities — tool-use,
  reasoning, vision, attachments, input/output modalities, context/output limits,
  knowledge cutoff — straight from the catalog with **no network call and no
  credentials**, and flags agent-readiness gaps under a ⚠ marker (headline: a
  model that doesn't advertise tool-use). Exit 3 when warnings exist so CI can
  gate "is this model agent-ready?"; `--caps --json` emits a stable record. New
  pure `catalog.Model` helpers (`SupportsModality`, `SupportsVision`,
  `AgentWarnings`) back it. Proven: a tool-less model warns + exits 3, a
  tool-capable model reports agent-ready + exits 0. See
  `.project/PHASE-M23-MODEL-CAPABILITIES-REPORT.md`.
- **Per-tenant policy management** (ROADMAP P6-MULTI, M22) — the runtime policy
  surface (deny rules · trust levels · approval mode, M18–M21) was primary-kernel
  only; tenants (M14) had isolated engines but no way to manage them. Every `agt
  edict` subcommand now takes `--tenant <id>`: `agt edict deny add --tenant acme
  "shell:kubectl delete"`, `agt edict level --tenant acme http.post L0`, `agt
  edict mode --tenant acme deny`, and the read commands (`show`/`test`/`deny
  list`) too. Server-side every handler routes through `kernelFor(tenant)` —
  empty targets the primary, else the tenant's isolated engine — and journals to
  that kernel's own bus, so a tenant's policy changes land in the tenant's own
  hash-chained journal. Isolation is total: a rule added to one tenant is
  invisible to other tenants and to the primary. Per-tenant durability comes for
  free: with `AGEZT_EDICT_DURABLE=on` each tenant kernel replays its OWN
  policy.changed history on open (M20), so tenant policy survives a restart.
  Proven live: a deny rule + level change set on tenant `alpha` deny only for
  `alpha` (beta + primary unaffected), survive a full daemon restart restored
  from alpha's own journal, and the primary journal holds zero tenant policy
  events. See `.project/PHASE-M22-PER-TENANT-POLICY-REPORT.md`.
- **Runtime approval-mode changes** (DECISIONS F3, M21) — the third and final
  runtime policy knob, alongside deny rules (M18) and trust levels (M19). `agt
  edict mode <allow|deny|prompt>` changes how Ask-class levels (L1..L3) are
  folded on a running daemon — `deny` for strict (only L4 runs), `prompt` for
  live HITL, `allow` to fold-and-journal — no restart. The hard-deny floor is
  unaffected (it fires before AskPolicy), so no mode can relax a hard-deny.
  Journaled as a `policy.changed` event (`action=mode.set`, `from`/`to`) and —
  because it flows through the same event — **durable for free** under M20:
  `AGEZT_EDICT_DURABLE=on` replays it, the banner shows `mode=deny` restored.
  Proven live: `mode deny` makes ask-class shell deny; after a full restart the
  mode is restored without re-setting; an unknown mode is rejected. This
  completes the runtime-policy surface (deny · level · mode), all three
  runtime-manageable and durable. See `.project/PHASE-M21-EDICT-MODE-REPORT.md`.
- **Durable runtime policy** (DECISIONS F3/F4, M20) — runtime deny rules (M18)
  and trust-level changes (M19) lived only in the running engine and reverted on
  restart. They were already journaled as `policy.changed` events; with
  `AGEZT_EDICT_DURABLE=on` the daemon now replays those events at boot and
  reconstructs the net overlay onto the freshly-built engine — the journal is
  the source of truth, the engine state a projection of it. Pure projection
  (`edict.ProjectPolicyChanges`): level changes are last-wins, deny rules are
  tracked by journaled name so an add-then-remove leaves no trace, malformed
  historical events are skipped rather than wedging the boot. Opt-in by design —
  a level *loosening* that silently persisted across a restart would be a
  footgun, so the operator asks for it; the banner reports what was restored
  (`durable=on (restored N level(s), M deny rule(s))`). Proven live: a deny rule
  + an `http.post` level change added in one session both fire after a full
  daemon restart (without re-adding), a non-durable boot restores neither, and
  the hard-deny floor is intact throughout. See
  `.project/PHASE-M20-EDICT-DURABLE-REPORT.md`.
- **Runtime trust-level changes** (DECISIONS F3, M19) — the other half of the
  policy engine, the trust ladder (L0 deny .. L4 allow), was boot-only config
  (env vars). `agt edict level <capability> <level>` now changes a capability's
  level on a running daemon — `agt edict level shell L0` locks shell down mid-
  incident, `agt edict level http.post allow` opens one up — no restart.
  Loosening is safe by construction: the hard-deny floor fires regardless of
  level, so even `shell=L4` still blocks `rm -rf /` (proven live). Levels accept
  `L0..L4` or word aliases (`deny`/`ask`/`askfirst`/`askscoped`/`allow`); an
  unknown capability or unparseable level is an error, never a silent default-
  deny phantom. Each change journals a `policy.changed` event
  (`action=level.set`, with `from`/`to`) so the trust ladder's history is as
  auditable as the deny floor's. See `.project/PHASE-M19-EDICT-LEVEL-REPORT.md`.
- **Runtime-managed policy deny rules** (DECISIONS F4, M18) — the hard-deny
  floor could only be changed by restarting the daemon (M17's `AGEZT_EDICT_DENY`
  is boot config). `agt edict deny list|add|rm` now manages it live over the
  control plane: `add "shell:kubectl delete"` (same syntax as the env var)
  appends a rule with no restart; `list` shows every rule tagged `floor` or
  `runtime`; `rm runtime[N]` removes one. The load-bearing invariant — runtime
  `rm` only touches runtime-added rules; built-in and `operator[N]` floor rules
  are refused with an error, never silently dropped — so the floor can be
  *tightened* at runtime but never *loosened*. Every add/rm is journaled as a
  `policy.changed` event (actor `operator`, with the rule + new count) in the
  same hash-chained journal as the decisions it governs, so a policy change is
  as auditable as a policy decision. Proven live: `add` → the rule fires via
  `agt edict test`; removing `rm-rf-root` or `operator[1]` is refused; `rm
  runtime[1]` clears it; both mutations land in the journal. See
  `.project/PHASE-M18-EDICT-RUNTIME-REPORT.md`.
- **Operator-extensible policy deny rules** (DECISIONS F4, M17) — Edict's
  hard-deny layer (the non-overridable floor that fires regardless of trust
  level) was a fixed built-in list. `AGEZT_EDICT_DENY` now appends site-specific
  rules: a `;`-separated spec where each entry is `substring` (denied for every
  capability) or `<capability>:substring` (scoped, when the prefix is a known
  capability — e.g. `shell:rm -rf /etc`, `http.post:169.254`). A `https://…`
  prefix isn't a capability, so URLs are taken verbatim; a blank substring is a
  hard error (it would deny everything). Rules are named `operator[N]` so a
  denial's journaled reason names the rule that fired. Proven live: booting with
  `AGEZT_EDICT_DENY="git push;shell:/etc/shadow"`, `agt edict test` denies both
  and allows ordinary commands. See `.project/PHASE-M17-EDICT-DENY-REPORT.md`.
- **Network egress guard against SSRF / metadata theft** (SPEC-06, M16) — an
  autonomous (or prompt-injected) agent making outbound HTTP must not reach the
  host's internal network: the cloud metadata endpoint (`169.254.169.254`) hands
  out IAM credentials, `127.0.0.1` reaches co-located admin services, RFC1918 is
  the private LAN. A hostname allowlist did not stop this — an allowed host can
  DNS-rebind to an internal IP, and `http.Client` follows redirects, so an allowed
  first hop can `Location:` you to the metadata endpoint. A new `kernel/netguard`
  validates the **resolved IP** at the dialer (`net.Dialer.Control`), which fires
  on every connection — initial dial **and each redirect hop** — so it sees past
  the hostname and refuses loopback / private (RFC1918+ULA) / link-local (incl.
  metadata) / unspecified addresses at connect time, defeating both rebinding and
  redirect SSRF. Both agent-driven URL fetchers — the **http tool** and
  **`browser.read`** — are guarded by default (even `AGEZT_HTTP_ALLOW_ALL` /
  `AGEZT_BROWSER_ALLOW_ALL` can no longer reach internal addresses);
  `AGEZT_{HTTP,BROWSER}_ALLOW_LOOPBACK` / `_ALLOW_PRIVATE` relax one range each for
  local use, and neither unblocks the metadata endpoint. The remaining outbound
  paths (peer, MCP bridge, webhook sinks) and per-call Edict egress are named
  follow-ups. See `.project/PHASE-M16-NETGUARD-REPORT.md`.
- **Secret redaction at the journal boundary** (ROADMAP/SPEC-06, M15) — the
  journal is append-only and hash-chained, so any secret that reaches an event
  payload (a key echoed in tool stdout, a token in a prompt, an `Authorization`
  header in a debug dump) would be recorded permanently. A new `kernel/redact`
  `Redactor` scrubs secrets on two signals — exact **literal** values from the
  creds vault and high-confidence **patterns** (OpenAI/Anthropic `sk-…`, AWS
  `AKIA…`, GitHub `ghp_…`, Slack `xox…`, Google `AIza…`, `Bearer …`, PEM private
  keys) — replacing each with `[REDACTED]`. The bus applies it to every durably-
  published event's payload and tags **before** hashing/writing, so the secret
  never enters the chain (which still verifies over the redacted bytes), and the
  redaction is deterministic so replay is unaffected. On by default in the daemon
  (seeded from the vault, refreshed on rotation, installed on the primary and
  every tenant bus; `AGEZT_REDACT=off` disables). Because the state/memory/world
  stores are event-sourced projections fed by the bus, scrubbing at this one
  chokepoint keeps the raw secret out of *every* on-disk store at once (proven
  live). Operators can add site-specific secrets the vault doesn't hold and the
  patterns can't recognise (internal tokens, DB passwords) via
  `AGEZT_REDACT_EXTRA` (`;`-separated literals). Streaming display tokens and
  custom regex rules are named follow-ups. See
  `.project/PHASE-M15-REDACTION-REPORT.md`.
- **Multi-tenant isolation foundation** (ROADMAP P6-MULTI, Phase 1) — a
  `kernel/tenant` `Registry` that lets one process host many fully-isolated
  tenants, each with its own base dir (and therefore its own journal, state,
  vault, memory, world model, skills, and schedules) and its own lazily-opened
  kernel. Tenant ids are validated as a single safe path segment
  (`[a-z0-9_-]`, 1–64 chars), so an id can neither traverse out of the root nor
  collide with a sibling — isolation by construction. The registry is decoupled
  from `kernel/runtime` via an injected opener (`OpenFunc`), with lazy
  `Acquire` (idempotent), `Release` (close, keep state), `Remove` (destructive),
  `List`, and `CloseAll`. Proven end-to-end: two tenants each run an intent
  through their own governed loop and each journal contains only its own run (no
  cross-tenant bleed). The daemon mounts the registry opt-in via
  `AGEZT_MULTITENANT=on` (rooted at `<base>/tenants`, each tenant opened with the
  primary's provider/tools but a fresh per-tenant Warden/Edict), and operators
  manage tenants with `agt tenant create|list|release|rm` over the control plane
  — proven live: isolated base dirs created, `release` keeps state while `rm`
  deletes only that tenant's tree, traversal ids rejected. Runs can be routed to
  a tenant with `agt run "<intent>" --tenant <id>` — the run executes under that
  tenant's governance and lands in its journal (proven isolated from the primary
  journal; an unknown tenant id is auto-created on demand). The native **REST
  API** routes per tenant too: a `POST /api/v1/runs` (or `GET
  /api/v1/runs/{corr}`) carrying an `X-Agezt-Tenant: <id>` header runs on — and
  streams from — that tenant's kernel and bus, isolated from the primary (proven
  live; header-less requests stay on the primary). The **OpenAI-compatible** API
  honours the same header: `/v1/chat/completions`, `/v1/responses`, and
  `/v1/models` route per tenant (both SSE streaming forms subscribe to the
  tenant's own bus), so any OpenAI SDK can target a tenant with one extra header.
  An **ACP** editor session can be bound to a tenant too: `agt acp --tenant <id>`
  forwards the id on every prompt so an IDE drives an isolated tenant kernel.
  With this, every run entry point — `agt run`, REST, OpenAI, ACP — routes per
  tenant through one seam. Each tenant also gets its **own budget ledger**: a
  per-tenant governor with an independent daily-spend counter and ceiling, so one
  tenant exhausting its cap can never starve another (or the primary), while the
  provider pool and credentials stay shared. The ceiling defaults to the
  primary's; `AGEZT_TENANT_DAILY_CEILING=<usd>` overrides it for every tenant.
  Each tenant also has its **own auth token**, minted on create and stored at
  `<base>/tenants/<id>/.tenant-token`: `agt tenant create` prints it and `agt
  tenant token <id>` reveals it, and the REST + OpenAI surfaces enforce it — a
  request targeting a tenant may authorize with the daemon admin token (any
  tenant) OR that tenant's own token (that tenant ONLY); a tenant token used for
  another tenant, or with no `X-Agezt-Tenant` header, is `401`. So you can hand
  one tenant's operator a credential that can't touch the others. Each tenant
  also has a per-minute **call-rate cap** (the frequency companion to the $/day
  ceiling): the governor admits up to `AGEZT_TENANT_RATE_PER_MIN` calls per
  clock-minute and returns a `rate.limited` event + error beyond that, per tenant
  and independent — so one tenant can't burst-flood the shared provider pool even
  while under its daily budget. Together these make the per-tenant quota +
  isolation story complete (see `.project/PHASE-M14-MULTITENANT-REPORT.md`).
- **Scheduled intents** — a `cadence` daemon resident (autonomy): fires intents
  on a recurring timer through the same governed loop (Edict + journal + budget),
  so the system acts on its own ("every morning, summarise new commits and brief
  me") — the timer companion to Pulse's event-driven proactivity. Schedules live
  in a **persistent store** (survive restarts, reversible) and are managed with
  `agt schedule add|list|rm|run|pause|resume` over the control plane; `AGEZT_SCHEDULE`
  (`;`-separated `interval=intent` jobs) seeds env-sourced entries at startup and
  is synced into the same store, and any entry can be **edited in place** (`agt
  schedule edit <id>`) — change its intent, model, or cadence while preserving its
  id (a field-only edit leaves the next-run time undisturbed). Four cadences: **interval** (`--every 1h`),
  **daily wall-clock** (`--at 09:30`, local time, e.g. a morning brief),
  optionally restricted to **specific weekdays** (`--days mon-fri`, `--days
  weekends`, or a list/range like `mon,wed,fri`) so a daily schedule fires only on
  the days you want (DST-correct, advancing by calendar date), and **one-shot**
  reminders (`--in 30m` relative, or `--once --at 18:00`) that fire exactly once
  and then remove themselves from the store, plus **windowed intervals** (`--every
  15m --between 09:00-17:00 [--days mon-fri]`) that fire on a sub-daily cadence
  but only inside a daily time window on permitted weekdays, jumping to the next
  window-open when one closes. Wall-clock cadences (daily and windowed) accept a
  **per-schedule IANA timezone** (`--tz America/New_York`) so "09:00" means 09:00
  *there* regardless of where the daemon runs (DST handled by the zone); `agt
  schedule pause`/`resume` disable and re-enable an entry without deleting it (a
  paused entry is skipped by the ticker but kept in the store). A single ticker
  fires every due entry; a still-running entry is skipped (no overlap). Each firing journals a
  `schedule.fired` event carrying the run's correlation, so `agt why` / `agt
  journal grep schedule` show what the system did autonomously. The store always
  works (`agt schedule` is always available); env-only setups need no CLI.
- **Mesh delegation** — the `remote_run` tool (ROADMAP P6-MULTI / M8): a lead
  agent on one Agezt node can hand a self-contained task to a *peer* Agezt node
  and get the answer back, by driving the peer's native REST surface
  (`POST /api/v1/runs`). The peer runs the task through its own governed loop
  (its tools, its policy, its journal), so delegation does not bypass the peer's
  governance, and the returned correlation id makes the remote run auditable on
  that node — cooperating nodes, each under its own authority. Peers are
  operator-configured via `AGEZT_PEERS` (`name=url|token,…`); a malformed spec is
  a hard startup error. Gated Ask-first by a new Edict `remote_run` capability
  (it ships a task to an external node). Off unless `AGEZT_PEERS` is set.
  `agt peers [--json]` lists the configured peers and checks each one's REST
  `/api/v1/health` (reporting OK + version, or unreachable/401), so an operator
  can verify the mesh wiring; it exits non-zero if any peer is unreachable.
- **Native REST API** (ROADMAP P7-API-02) — a first-party `/api/v1` HTTP surface
  with Agezt-native semantics (where `/v1` mimics OpenAI). `POST /api/v1/runs`
  submits an intent and returns a `correlation_id` + answer (sync JSON), or an
  SSE event stream (`start` → `token`* → `done`/`error`) with `"stream":true` or
  an `Accept: text/event-stream` header; `GET /api/v1/runs/{correlation_id}`
  returns that run's full journaled event arc (correlation-first inspection the
  OpenAI surface can't do); plus `GET /api/v1/health` and `GET /api/v1/models`.
  Every run goes through the same governed kernel loop (Edict + journal + budget);
  per-request `model` is honoured. Off unless `AGEZT_REST_ADDR` is set;
  loopback-bound + Bearer-token authed, same lifecycle as the OpenAI resident.
- **Outbound webhooks** (ROADMAP P7-API-02) — a daemon resident that POSTs
  journal events to operator-configured HTTP endpoints as they happen, so
  external systems react to Agezt in real time (a run completed, an approval is
  pending, the system halted). Configured via `AGEZT_WEBHOOKS`, a comma-list of
  `url|subject|secret` sinks; `subject` is a bus pattern (`agent.>`, `edict.>`,
  `>`) so matching reuses the bus verbatim. When a `secret` is set each POST is
  HMAC-SHA256-signed (`X-Agezt-Signature: sha256=…`) for receiver verification;
  headers also carry `X-Agezt-Event`/`X-Agezt-Subject`/`X-Agezt-Delivery`.
  Deliveries retry with backoff and each outcome is journaled
  (`webhook.delivered` / `webhook.failed`) — and the dispatcher never
  re-delivers its own `webhook.*` events, so there is no feedback loop. Runs on
  the daemon ctx (halt/shutdown stop it); off unless `AGEZT_WEBHOOKS` is set.
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
