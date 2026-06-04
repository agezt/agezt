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
- **`agt cache`.** The CLI counterpart of the Web UI Cache panel — prints the
  prompt-cache savings (tokens served from / written to cache, and the dollars
  saved versus the full input rate), `--since` windowable, `--tenant`-scoped,
  `--json`. Reuses the `cache_stats` command, so CLI and Web UI agree.
- **Prompt-cache savings aggregate + Web UI Cache panel.** A new `cache_stats`
  control-plane command folds `budget.consumed` events into how many prompt tokens
  were served from / written to the provider cache and how many microcents that
  saved — the no-cache baseline (every input token at the full input rate) minus
  the recorded cache-aware cost, summed per call. A Web UI Cache panel surfaces it
  ($ saved, cache-read tokens, cache-write tokens, priced calls), the visible
  payoff of the cache-aware cost accounting. Tenant-scoped, `--since`-windowable.
- **Web UI: a Budget panel.** Shows the governor's daily spend snapshot — date,
  spent, ceiling (or "unlimited"), utilization %, strict-pricing flag, and any
  per-task caps with their spend — by proxying `budget` (the Web UI counterpart of
  `agt budget`). It refreshes live off `budget.*` events. The spend reflects the
  cache-aware cost accounting (cached prompt tokens billed at the cache-read rate).
- **Cache-write premium billing.** Prompt-cache *creation* tokens are now billed
  at the model's cache-write rate (a premium over input — Anthropic's 1.25×)
  rather than folded into the input rate. New `agent.Usage.CacheWriteInputTokens`
  + `catalog.Cost.cache_write` → `modelPrice.CacheWriteMicrocentsPerMTok`; the
  governor bills `fresh·input + read·cache_read + write·cache_write + output` and
  records `cache_write_input_tokens` on `budget.consumed`. The Anthropic providers
  map `cache_creation_input_tokens` into the new field; the fallback Claude prices
  carry the 1.25× cache-write list. A model with no cache-write price bills those
  tokens at the input rate (conservative). Completes the cache cost model
  (fresh / cache-read / cache-write / output).
- **Anthropic cache-token accounting.** The Anthropic providers (direct +
  Anthropic-on-Vertex, streaming and non-streaming) now parse
  `cache_read_input_tokens` and `cache_creation_input_tokens`. Anthropic reports
  `input_tokens` *excluding* cached prompt tokens, so the canonical usage now sums
  all three — fixing an under-count where cached prompt tokens were billed at zero
  — and marks cache reads as cached so they bill at the cheaper cache-read rate
  (see cache-aware cost accounting below). Cache-creation tokens fold into the
  input total (their write premium isn't modelled yet).
- **Cache-aware cost accounting.** Prompt-cached input tokens are now billed at
  the model's cache-read rate instead of the full input rate. The openai (and
  compatible) provider parses `usage.prompt_tokens_details.cached_tokens` into a
  new `agent.Usage.CachedInputTokens`; the governor bills
  `(input−cached)·input_rate + cached·cache_read_rate + output·output_rate`
  (cache-read sourced from the catalog's `cost.cache_read`, with the fallback
  Claude prices carrying Anthropic's 0.1× cache-read list), and records
  `cached_input_tokens` on `budget.consumed`. A model with no separate cache
  price bills cached tokens at the full input rate (conservative — never
  under-bills). Previously every cached token was charged at full input rate, so
  cost was over-estimated for prompt-caching reasoning models.
- **Web UI: the Tools panel drills into the per-call invocation log.** Clicking
  the panel opens a modal listing recent tool calls — tool name, ok/✗, latency,
  the input the agent passed, and the resulting output — via the read-only
  `tool_log` route. So the aggregate "what tools are running, and are they
  erroring" view now has a "what did the agent actually run, with what input"
  companion (mirroring the Providers panel → routing-log pairing).
- **Web UI: a Tools panel showing tool-execution health.** A new panel proxies
  `tool_stats` to show how many tool calls ran, the error rate, and a per-tool
  breakdown (calls, errors in red, average latency) — the execution analogue of
  the Stats/Providers aggregates, and the Web UI counterpart of `agt tool stats`.
  It refreshes live off `tool.*` events.
- **Web UI: the Providers panel drills into the per-call routing timeline.**
  Clicking the panel opens a modal listing recent routing decisions (which
  provider was chosen + the fallback chain + task type) and provider fallbacks
  (failed → next + reason), newest-first — via the read-only `provider_log` route.
  So the aggregate "who served my traffic" view (added below) now has a "what
  happened, call by call" companion.
- **Web UI: a Providers panel showing the routing picture.** A new panel proxies
  `provider_stats` to show how many calls each provider actually served
  (`by_primary`), the total routed/fallback counts, the fallback rate, and a
  fallbacks-by-provider breakdown — so "which provider is handling my traffic, and
  is any of it silently falling back?" is answerable at a glance. It refreshes
  live off `routing.*` / `provider.*` events. Extends the fallback-observability
  arc (status badge → fallback detail modal → this aggregate routing view).
- **Web UI: the live event feed can be filtered by kind.** A `filter kind…` input
  in the feed header hides every row whose event kind doesn't contain the typed
  substring (e.g. `task.`, `tool.`, `provider.`), so an operator can focus a busy
  firehose on one family. Purely client-side row toggling — switching or clearing
  the filter is instant and never reconnects the stream — and new incoming events
  respect the active filter as they arrive.
- **Web UI: the provider-fallback badge is now clickable.** Clicking the header
  `⚠ N fallbacks` chip opens a modal listing the recent `provider.fallback`
  events — which provider failed, which backup took over, when, and why — so a
  glance at the badge can drill straight into the underlying errors. It reuses the
  run-detail modal shell and the read-only `/api/journal` route (filtered by
  `kind=provider.fallback`); no new endpoint.
- **Web UI: a header warning badge for provider fallbacks.** When the status
  reports a non-zero provider-fallback count, the dashboard shows a prominent red
  `⚠ N fallbacks` chip in the header (with the last reason on hover), so a
  silently-degraded provider is obvious at a glance rather than buried in the
  Status panel. It clears itself when the count returns to zero.
- **Silent provider fallbacks are now visible.** When the governor falls back
  from a primary provider to a backup (because the primary errored), it was only
  recorded as a `provider.fallback` journal event — so a provider that fails on
  every request (masked by the always-on mock fallback) was invisible without a
  journal dig. `agt status` now shows a `fallbacks: N` line with the most recent
  reason (quiet at zero), `agt doctor` raises a `provider-fallbacks` WARN, and
  the Web UI Status panel carries the count — all folded from the same journal
  events. This is exactly the signal that would have surfaced the dotted
  tool-name 400 immediately.
- **Web UI: a run-stats panel with an outcome bar.** Beside the per-run list, a
  Stats panel shows the aggregate (`agt runs stats`): run count, success rate,
  total spend, and a proportional horizontal bar of outcomes (completed green,
  running accent, failed red) — an at-a-glance read of fleet health. It refreshes
  live on `task.*` events alongside the Runs panel.
- **Web UI: click a run for its event arc.** Each row in the Runs panel is now
  clickable and opens a detail modal showing that run's full journaled arc —
  every step in order (task.received, llm.request/response, policy.decision,
  `tool.invoked` with its input, `tool.result` with ✓/✗ and output, per-call
  budget, and the final answer or failure reason). It fetches the run's events
  by `correlation_id` through a new read-only, arg-allowlisted `/api/journal`
  route (GET, forwarding only `correlation_id`/`kind`/`limit`). Esc or a click
  outside closes it.
- **Web UI: a live Schedules panel.** The dashboard now shows the configured
  autonomous schedules (`agt schedule list` over the control plane) beside Runs —
  each with its cadence (e.g. `every 5s`), the intent, a paused marker, and a
  colour-coded chip for its last firing's outcome. It refreshes on every streamed
  `schedule.*` event, so a schedule firing on its own appears live alongside the
  run it produced in the Runs panel.
- **Web UI: a live Runs panel.** The dashboard now shows the recent runs
  (`agt runs list` over the control plane) beside Status — each with a
  colour-coded status chip (completed green, failed/abandoned red, running
  accent), the intent, duration, iteration count, and spend. It refreshes on
  every streamed `task.*` event (debounced), so a run appears the moment it
  finishes, and sub-agent runs are marked with a `↳`. The same event-driven
  refresh now also updates the Skills/Memory/World/Approvals panels live.
- **`agt skill registry <url>` — browse and install from a remote registry.**
  The registry command now accepts an http(s) URL: it fetches the `index.json`
  manifest a publisher wrote with `agt skill export --all`, lists the available
  skills, and with `--install <name>` fetches the named bundle and installs it
  through the same content-address verification as a local import. Fetches are
  bounded (20s timeout, 8 MiB cap) and an index entry's file name is validated to
  be a plain filename (no traversal) before download. A static file host is all a
  registry needs.
- **`agt skill export --all` now writes an `index.json` registry manifest.**
  Alongside the per-skill bundle files, it writes an `index.json` listing every
  published skill (name, version, id, description, file) — the manifest a static
  HTTP host serves so a remote consumer can discover the registry without a
  directory listing. The directory scan continues to ignore it (it is not a
  `*.skill.json`), so a local `agt skill registry` view is unaffected.
- **`agt skill export --all [--dir <dir>]` — publish your whole skill library.**
  Exports every skill to its own verifiable bundle file in a directory (one file
  per skill, filenames slugified from the skill name plus a short id so versions
  never collide). The publisher side of the skill registry: a node exports its
  skill library as a directory another node browses with `agt skill registry` and
  installs from with `--install`.
- **`agt skill registry <dir> --install <name>` — install a bundle by name.**
  Resolves a skill name within a directory registry to exactly one verified
  bundle and installs it (delegating to the same verify-then-import path as
  `agt skill import`). Refuses an ambiguous name (several bundles share it —
  e.g. different versions, listing each so the operator imports the one they mean
  by path) and ignores tampered/malformed candidates. Completes the local
  marketplace loop: export → share a directory → discover → install by name.
- **`agt skill registry <dir>` — discover skill bundles in a directory.** The
  discovery layer of the skill marketplace: lists every `*.skill.json` bundle in
  a directory with its name, version, id, and description, verifying each one's
  content address offline. A tampered bundle is flagged `TAMPERED` and a
  malformed file is flagged with its parse error (the command exits non-zero if
  any bundle is bad), and each good entry prints the exact `agt skill import`
  command to install it. `--json` for scripting. Pure offline file read; no
  daemon needed.
- **`agt skill import <bundle>` — install a skill from a portable bundle.** The
  read-back half of `agt skill export`: verify the bundle's content address
  *offline* (a tampered bundle is rejected before the daemon is contacted), then
  install the skill via the Forge as a fresh **draft** — content-addressed,
  deduped against an identical existing skill, and journaled like any authored
  skill. An imported skill is never auto-active; the operator promotes it
  (`agt skill promote`) to put it into the retrieval pool. New control-plane
  command `skill_import`.
- **`agt skill export <id>` — write a portable, verifiable skill bundle.** The
  first piece of skill portability (toward a skill marketplace): fetch a skill
  from the daemon and emit it as a self-contained JSON bundle (default stdout,
  or a file with `--out`). The bundle carries only the skill's content fields —
  name, description, triggers, body, required tools, version, lineage — never
  instance-local state (status, metrics, timestamps, the producing event), so an
  imported skill arrives fresh rather than inheriting the source's lifecycle.
  Because a skill's id is content-addressed over (name, body), the bundle is
  self-verifying: export refuses to emit a skill that does not match its own
  address, and an importer can detect tampering before trusting it.
- **`agt backup inspect <file>` — read a backup bundle without restoring it.**
  An offline inspection of a `agt backup` archive: shows the manifest (tool,
  format, creation time, recorded journal head, included subtrees) and lists the
  contained files with sizes, without unpacking. Flags any entry outside the
  known include subtrees (a sign of a tampered or foreign archive) and exits
  non-zero so a bad bundle is caught before a restore. `--json` for scripting.
  The whole-home counterpart to `agt journal verify --bundle`.
- **`agt vault status` surfaces the vault's key-derivation policy.** An
  encrypted vault's status now reports its KDF and iteration count and whether it
  is up to date — read from the envelope without the passphrase — so an operator
  sees whether `agt vault migrate` is worth running before running it. A stale
  vault gets a "migration: recommended" pointer; a plaintext vault shows no KDF
  line.
- **`agt vault migrate` — upgrade an old encrypted vault to the current KDF.**
  The operator-facing wiring for the credential-vault migration: inspects the
  on-disk vault and, if it is encrypted with the legacy key-derivation or below
  the current iteration policy, re-encrypts it in place at the current PBKDF2
  policy. It is a no-op (with a clear notice) for a plaintext vault or one
  already at the current policy, and requires `AGEZT_VAULT_PASSPHRASE` only when
  an actual re-encryption is needed. Prints the before/after KDF and iteration
  count and points to `agt provider reload`.
- **Credential-vault migration (`creds.InspectVault` / `Store.MigrateEncryption`).**
  The first piece of the migrate tooling: detect an encrypted vault written with
  the legacy key-derivation (pre-PBKDF2) or below the current iteration policy,
  and upgrade it in place by re-encrypting with the current KDF — passphrase and
  secrets unchanged. `InspectVault` reports a vault's KDF/iterations and whether
  it is up to date without needing the passphrase, so an operator can check
  migration status at a glance. (CLI command wires this up next.)
- **Public Go SDK (`github.com/agezt/agezt/sdk`).** A stable, ergonomic client
  for embedding Agezt in Go programs: `sdk.Dial("")` connects to the local
  daemon, and `Client.Run(ctx, intent, opts...)` / `Client.RunStream(...)` run an
  intent through the same governed kernel loop as `agt run` — Edict, the journal,
  cost governance — returning a typed `Result` (answer, correlation id, model,
  iterations, cost in USD). Functional options cover model, tenant, system
  prompt, timeout, tool allow-list, image attachments, and a per-run cost cap.
  Callers no longer need to speak the control-plane wire protocol or import
  kernel internals. First milestone of the SDK; further surfaces (events helper,
  approvals, runs inspection) build on it.
- **SDK run inspection (`Client.Runs`).** The SDK can now list recent agent runs
  (newest first) as typed `RunInfo` values — correlation id, intent, status and
  failure reason, parent (for sub-agents), start time and duration as
  `time.Time` / `time.Duration`, iterations, cost in USD, and model — reading the
  journal on the daemon without starting a run.
- **SDK human-in-the-loop approvals.** `Client.PendingApprovals` lists requests
  awaiting a decision as typed `Approval` values (id, capability, tool, reason,
  actor, input, timeout); `Client.Approve` / `Client.Deny` resolve one by id with
  a journaled reason. An embedding app can now build its own approval UI on top
  of the same HITL gate `agt approve` / `agt deny` use.
- **SDK streamed-event helpers.** `sdk.TokenText`, `sdk.ToolCall`, and
  `sdk.IsTerminal` decode the common cases from a `RunStream` callback's events
  (the answer's text deltas, which tool the agent invoked, and the run's terminal
  event) so a consumer renders live progress without hand-parsing event payloads
  or importing kernel internals.
- **SDK example program + godoc examples.** A runnable
  `examples/agezt-run` connects to the daemon, streams a run live (printing the
  answer as it generates and noting tool calls), then lists recent runs — a
  copy-paste starting point for embedding Agezt. Godoc `Example` functions on the
  SDK package document `Run`, `RunStream`, `Runs`, and approvals.

### Fixed
- **The OpenAI-compatible API now reports real provider token usage.** The
  `usage` block on `/v1/chat/completions` and `/v1/responses` was a rough
  whitespace word-count estimate, so a cost-tracking client reading it got wildly
  wrong numbers (e.g. `prompt_tokens: 8` for a run that actually consumed 1406).
  The server now reports the real tokens the provider billed — summed across the
  run's LLM calls, folded from the journal's `budget.consumed` events — falling
  back to the estimate only when no usage was recorded (a free/local/mock model).
  New optional `UsageReporter` engine capability; verified end-to-end against a
  live gpt-5.5 gateway (`1406/11` vs the old `8/1`).
- **OpenAI-compatible providers no longer reject every tool-bearing request.**
  Agezt exposes a dotted tool name (`browser.read`), but OpenAI and strict
  openai-compatible gateways require tool names to match `^[a-zA-Z0-9_-]+$` and
  return a 400 ("does not match pattern") for the whole request. With the
  always-on mock fallback catching that error, **every run against a real
  OpenAI-compatible provider silently fell back to the mock** — invisible unless
  you inspected `provider.fallback` in the journal. The openai adapter now
  sanitises tool names on the wire (`browser.read` → `browser_read`, in both the
  streaming and non-streaming request, and in assistant tool-call history) and
  maps the name back on the response so the tool call still routes to the real
  tool. Verified end-to-end against a live gateway (gpt-5.5): a multi-turn
  tool-using run completed on the real provider with real token spend and **no**
  fallback.
- **`agt skill import` of a skill with no triggers/tools no longer errors.** The
  CLI sent the optional `triggers` / `tools_required` args as an explicit JSON
  `null` when the skill had none, which the daemon's strict array decoder
  rejected ("must be an array"). Those args are now omitted when empty, so a
  minimal skill (name + body only) imports cleanly. Surfaced while building
  `agt skill registry --install`.
- **Corrected stale references to now-shipped features.** `agt provider check
  --stream` printed "provider family X does not yet support streaming (M1.q only
  wires anthropic; others land in M1.q.x)" when a provider lacked a streaming
  adapter — but every first-party family (anthropic, openai, google, bedrock,
  vertex, cohere, ollama, openai-compatible) now streams, so the message was both
  unreachable for real families and wrong; it now accurately points at re-running
  without `--stream`. A credential-vault doc comment that called `agt vault
  encrypt`/`migrate` "(deferred)" was likewise updated — both commands ship.
- **A single oversized ACP message can no longer balloon memory.** Both the ACP
  server (driven by an IDE) and the ACP client (driving an external agent) read
  with a `json.Decoder`, which buffers a whole JSON value with no size limit — so
  one giant message could exhaust memory. ACP is newline-delimited JSON, so both
  now read with a line scanner capped at 8 MiB per message; an over-cap message
  is rejected instead of buffered. Completes the previous fix, which bounded the
  *accumulation* of streamed chunks but not a single huge one.
- **A runaway ACP agent can no longer balloon the daemon's memory.** The
  `acpagent` tool accumulated every streamed chunk into one buffer and only
  truncated it to 60 KiB at the end — so an external agent that streamed without
  end grew the buffer unbounded (and could OOM the daemon, taking every
  concurrent run with it) before the timeout reaped it. Accumulation now stops at
  the 60 KiB cap; the relayed answer is unchanged.
- **Sending an image to a non-vision model via the API or a channel now fails
  fast with a clear message.** The control plane already pre-checked a run's
  model for vision capability before spending a provider call (M91), but the
  OpenAI-compatible API and the chat channels call the run path directly and
  bypassed that gate — so an image attached to a non-vision model produced a
  cryptic downstream provider error (and a wasted call) instead of an actionable
  one. Both paths now run the same confirmed-or-reject vision gate up front: an
  unknown or known-non-vision model is refused with "model … does not support
  vision (image input)".
- **The `browser` tool's host allowlist is now enforced on redirects too.** Same
  gap as the `http` tool: the allowlist was checked only on the initial URL, so
  an allowlisted page that 302-redirected to an arbitrary external host would be
  fetched anyway (netguard still blocked internal IPs). The fetch client now
  re-checks the allowlist on each redirect hop and caps the chain.
- **The `file` tool no longer lets a new file escape through a symlinked parent
  directory.** Writing a not-yet-existing path (e.g. `linkdir/new.txt` where
  `linkdir` is a symlink to a directory outside root) was checked only
  lexically, so the new file could be created outside the workspace. The
  containment check now symlink-resolves the deepest existing ancestor of a new
  path and confirms it is inside root, while still allowing legitimate writes
  that create parent directories.
- **The `file` tool no longer lets an absolute path bypass its symlink
  containment.** A symlink inside the workspace root pointing outside it was
  correctly refused when reached by its relative path, but the absolute-path
  branch of the containment check skipped symlink resolution — so the same
  symlink could be read/written via its absolute path, escaping the workspace.
  Both branches now resolve symlinks and verify the real location is inside root.
- **The `http` tool's host allowlist is now enforced on redirects, not just the
  first URL.** netguard already blocks internal/metadata IPs on every hop, but
  the host allowlist was checked only on the initial URL — so an allowlisted host
  that returned a 302 to an arbitrary external host would send the follow-up
  request (carrying any headers the agent set, including `Authorization`) to a
  host the operator never allowed. The tool now re-checks the allowlist on each
  redirect hop and caps the chain, closing an allowlist-bypass / header-leak via
  open redirects.
- **The Responses API (`/v1/responses`) now accepts image input too.** Chat
  Completions already forwarded `image_url` parts (the prior change); the
  Responses surface ignored its `input_image` parts (where `image_url` is a bare
  string, a different shape). It now extracts them — tolerating both the string
  and `{url}` object forms — and forwards them to the run, so vision input works
  on both OpenAI-compatible endpoints. An image-only Responses input runs with a
  default instruction.
- **An image attached to a Discord slash command now reaches a vision model —
  inbound vision is complete across all three channels.** When a slash command
  carries an `ATTACHMENT` image option, the channel resolves it via
  `data.resolved.attachments`, downloads the CDN file after the fast interaction
  ACK (so the 3-second deadline is never at risk), and forwards it as a `data:`
  URL; an image-only command (no prompt text) is no longer rejected as "nothing
  to do". Non-image attachments are ignored.
- **An image shared in Slack now reaches a vision model.** Like the Telegram
  fix, the Slack channel ignored inbound file attachments. It now downloads each
  shared *image* file (`url_private`, authenticated with the bot token) as a
  `data:` URL and forwards it to the run; non-image files and files from
  non-allowlisted channels are skipped. (Discord slash-command attachments are
  the remaining inbound surface.)
- **A photo sent to the Telegram bot now reaches a vision model.** Inbound
  channel messages only carried text, so a user sending a picture (with or
  without a caption) got a text-only run and the image was lost. The Telegram
  channel now fetches the largest photo size (getFile → download, in the channel
  where the bot token lives), forwards it as a `data:` URL on the unified
  message, and the run threads it to the model via the same path the CLI and API
  use. A photo's caption becomes the message text; an uncaptioned photo runs with
  a default "describe the image" instruction. Photos from non-allowlisted senders
  are never fetched. (Discord/Slack inbound attachments are follow-ups.)
- **Agezt's OpenAI-compatible endpoint now accepts image input from clients.**
  The `/v1/chat/completions` server flattened multimodal content to text and
  silently dropped `image_url` parts, so a client sending a vision request to
  Agezt-as-a-gateway got a text-only run — the mirror of the provider-side gap
  just closed. It now parses `image_url` parts from user messages and forwards
  the URLs to the run (which the providers turn into the model's native image
  input), completing the round trip. An image-only message (no text) runs with a
  default "describe the image" instruction instead of being rejected as empty;
  a message with neither text nor image is still rejected. (Responses-API
  `input_image` parts are a separate shape, still a follow-up.)
- **Vision now also works on Vertex AI — every first-party provider is now
  covered.** Both Vertex encoders dropped image attachments: Anthropic-on-Vertex
  now emits a `type=image` base64 block, and Gemini-on-Vertex now emits an
  `inlineData` part, each before the text. With this, `agt run --image` reaches
  the model on every built-in provider — Anthropic, OpenAI, Gemini, Bedrock, and
  Vertex — plus the OpenAI-compatible vendors that wrap the OpenAI encoder.
- **Vision now also works for Claude-on-Bedrock.** The Anthropic-on-Bedrock
  encoder (the largest Bedrock use case) has its own copy of the Messages-API
  content-block builder, which also dropped image attachments. It now emits a
  `type=image` base64 block before the text block, matching the direct Anthropic
  provider. Covers both Bedrock request paths (streaming and non-streaming share
  the encoder).
- **Vision now also works on the Gemini provider — completing the mainstream
  set.** The Google `generateContent` encoder (`canonicalToGemini`) now emits a
  user message's image attachments as `inlineData` parts (base64 + mimeType)
  before the text part, instead of dropping them. With this, all three first-
  party providers — Anthropic, OpenAI, Gemini — deliver `agt run --image` to the
  model, and the OpenAI-compatible `compat` vendors (Groq, xAI, DeepSeek, …)
  inherit it through the OpenAI encoder. Covers both Gemini request paths
  (streaming and non-streaming share the encoder).
- **Vision now also works on the OpenAI provider.** Following the Anthropic fix,
  the OpenAI provider's `canonicalToOA` now emits a user message's image
  attachments as OpenAI's multimodal content-parts array (a `text` part followed
  by one `image_url` part per attachment, carrying the `data:` URL OpenAI accepts
  natively) instead of a text-only string. The message `content` field became
  polymorphic (string or parts array) without disturbing the text path — a
  tool-call-only assistant message still omits `content`, and a non-URL
  attachment is skipped rather than sent as an invalid `image_url`. Covers both
  the streaming and non-streaming request encoders.
- **`agt run --image` now actually sends the image to the model (Anthropic).**
  The flag stat-checked the file, gated the run against the model's vision
  capability, and journaled an attachment count — but only the *basename*
  travelled to the daemon, which no provider could resolve, so the picture never
  reached the model: vision was silently a no-op. The CLI now reads the bytes
  (the file is on the operator's machine, not the daemon's), forwards a
  self-describing `data:` URL, and the Anthropic provider emits it as a base64
  `image` content block on both the streaming and non-streaming paths. Supported
  types: png, jpeg, gif, webp; oversize files are refused client-side with a
  clear message against the 16 MiB control-plane request cap. (OpenAI/Gemini
  emission lands in follow-up milestones.)
- **A crashed daemon now gives an actionable CLI error, not "connection
  refused".** When the daemon left a stale runtime address (it crashed but its
  addr file remained), every `agt` command failed with a cryptic transport error,
  unlike the clear "start the daemon" hint shown when it was never started. The
  client now does a liveness probe and reports both cases the same way —
  "daemon recorded but not responding … (re)start the daemon". A server-side
  rejection (e.g. a bad token) is distinguished and not misreported as a crash.
- **The ACP server reports the real product version to IDEs.** Its
  `agentInfo.version` was a hardcoded `"0.1.0"`, so an editor connecting to a
  v1.0.0 daemon over the Agent Client Protocol displayed "agezt 0.1.0".
  `agentInfo` now sources its name and version from `internal/brand`, so it
  tracks the actual release (and won't drift on the next bump). The ACP
  `protocolVersion` is unchanged — it's a separate, correctly-constant field.
- **An empty or whitespace-only outbound message is now a no-op, not a failed
  send.** Every channel's send path (Telegram, Discord incl. slash-command
  follow-ups, Slack) returns early on blank text instead of POSTing it — the
  platforms reject an empty message (Telegram 400 "message text is empty", Slack
  "no_text"). This covers the proactive `Send` path (Pulse, `agt send`) — which
  had no guard at all — and whitespace-only agent answers that the inbound reply
  paths' exact-`""` check missed.
- **Long messages to Telegram and Discord are no longer dropped.** A reply over
  the platform's per-message limit (Telegram 4096 UTF-16 code units, Discord 2000
  characters) was sent as a single oversize request, which the API rejects — so
  the agent's answer never arrived. Outbound text is now split into sequential
  in-limit messages (breaking at newline/space boundaries where possible, with a
  hard cut for an unbroken run). A shared `channel.SplitText` does the splitting
  losslessly, counting UTF-16 code units so it's safe for both Telegram (counts
  those) and platforms that count runes/code points. Discord's slash-command
  follow-up path (a long answer to a `/command`) is chunked the same way, and
  Slack (40000-char limit) too — all three channels now split rather than drop.
- **Moonshot AI (Kimi) now works**, and an unrecognised provider package fails
  with an actionable error. Moonshot's official package (`@ai-sdk/moonshotai`)
  hit the same dead end DeepSeek did — classified as an unknown family and
  refused. It's now wired as OpenAI-compatible with its base URL
  (`https://api.moonshot.ai/v1`). And the error for a genuinely-unknown package
  no longer claims (falsely) that the case is "unreachable for any catalog entry";
  it now tells the operator to set the provider's npm to `openai-compatible` in
  `custom.json` if it speaks the OpenAI API — turning a dead end into a one-line fix.
- **DeepSeek now works.** Its official package (`@ai-sdk/deepseek`) classified as
  an unknown family, so `compat.Build` refused it outright with "provider family
  not yet supported" — a vendor named in the README that couldn't actually be
  used. It's now classified as OpenAI-compatible (its wire dialect) with its base
  URL carried, so it works with just a `DEEPSEEK_API_KEY`.

### Added
- **OpenAI-compatible vendors work with just an API key — no `custom.json` URL.**
  Groq, xAI, Cerebras, Together, DeepInfra, Perplexity, Fireworks, and OpenRouter
  are vendors agezt already classifies (`catalog.FamilyFromNPM`), but their base
  URL had to be supplied by hand or the build was refused (to avoid silently
  routing to `api.openai.com`). compat now carries each one's stable
  OpenAI-compatible base URL, so configuring one of them needs only its key. An
  explicit catalog `api` still wins, and an *unrecognised* compat vendor is still
  refused with the `custom.json` hint.

### Security
- **Redaction extended to the Perplexity (`pplx-…`) and Fireworks (`fw_…`) key
  formats** — the two OpenAI-compatible vendors made first-class in this release
  whose keys the earlier rule set didn't catch. (Cerebras `csk-…` is already
  covered by the `sk-…` rule matching its substring.)
- **Plugin stderr is now redacted before it reaches the daemon log.** A
  third-party plugin's stderr is captured and written to the operator's log via
  the plugin logger — a direct path the bus redactor (journaled events only)
  never covered. A plugin that printed a secret (its own API key, etc.) leaked it
  in the clear. Each line now passes through pattern-based redaction first; the
  `[plugin:<name>]` prefix is preserved.
- **Secret redaction now covers the formats agezt's own integrations handle.**
  Added high-confidence patterns for Telegram bot tokens (`<id>:<35-char>`, the
  Telegram channel), Slack app-level tokens (`xapp-…`, complementing the existing
  `xox…`), and Groq (`gsk_…`) and xAI (`xai-…`) API keys — both first-class compat
  providers whose keys the broad `sk-…` rule did not match. Without these, such a
  secret appearing in a log line, tool output, or journal payload would have gone
  out in the clear. False-positive-guarded against ordinary text.

### Added
- **OpenAI streaming honours `stream_options.include_usage`.** When a chat
  completions client sets it, the stream now ends with a usage-only chunk
  (`choices: []` + a `usage` object) before `[DONE]`, matching OpenAI — so
  cost-tracking clients and the OpenAI SDK get token counts from streamed runs.
  Without the option, no usage chunk is emitted (OpenAI's default). Unknown
  request fields are still ignored, so this is additive.
- **`agt doctor` validates per-tenant peer overrides.** A new `tenant-peers`
  check (shown only when `AGEZT_TENANT_PEERS` is set) validates the per-tenant
  mesh peer sets (M219): a malformed spec — which the daemon hard-fails on — is
  caught as a FAIL before a restart, a valid one is confirmed with a per-tenant
  peer-count summary (no URLs or tokens printed), and a tenant whose peer set is
  empty (silently dropped by the parser, so its override does nothing and it
  falls back to the global set) is surfaced as a WARN that names it — a
  misconfiguration nothing previously reported.
- **`agt doctor` surfaces refused mesh delegation loops.** The M209 loop guard
  rejects an incoming cross-node run whose hop count exceeds the limit (508 Loop
  Detected) and journals a `mesh.loop_refused` event — but that signal was only
  visible by digging through the journal. A new `mesh-loops` check folds the
  journal's per-kind counts and WARNs when any loop has been refused (naming the
  count, hinting at a federation-topology cycle). It stays silent when none have
  occurred, so healthy and single-node output is unchanged. No new kernel state.
- **`agt doctor` now pre-flights the plugin env-specs.** A new `plugins` check
  validates `AGEZT_PLUGINS` (and the optional `AGEZT_PLUGIN_PINS` /
  `AGEZT_PLUGIN_TOOLS`) using the same parsers the daemon runs at startup, so a
  malformed spec — which makes the daemon *refuse to start* — is caught as a
  FAIL before the operator restarts, rather than discovered by a daemon that
  won't come back up. A pin or tool-allowlist entry whose prefix matches no
  configured plugin is a WARN (stale config); a clean spec reports the plugin
  count with pinned / allow-listed annotations. Reads only the environment; no
  spawn.
- **`AGEZT_PLUGINS` paths may now be quoted to contain spaces.** A plugin path
  or argument wrapped in single or double quotes keeps its spaces — necessary
  for the common Windows case of a plugin installed under `C:/Program Files/...`
  (`tool="C:/Program Files/agezt-tool.exe" --verbose`). Unquoted input still
  splits on whitespace exactly as before, so the change is purely additive; an
  unterminated quote is a startup error. (A path containing a comma still can't
  be expressed — the comma is the entry separator.)

### Fixed
- **`AGEZT_PLUGINS` duplicate prefix is now a hard startup error.** Parsing of
  the plugin spec moved into a testable `plugin.ParsePluginSpec`. Previously two
  entries sharing a prefix (`search=/a,search=/b`) both spawned, and the second
  plugin's tools lost a name conflict to the first, emitting a misleading
  "conflicts with in-process version" warning while a second process ran
  unused. A repeated prefix is a config typo, not a request to run two plugins
  under one namespace, so it is rejected at startup — matching the
  already-strict `AGEZT_PLUGIN_PINS` / `AGEZT_PLUGIN_TOOLS` parsers. Malformed
  entries (missing `=`, empty prefix, empty path) are likewise hard errors now
  rather than silent warn-and-skip, so a typo can't leave the daemon quietly
  running with fewer tools than configured.

### Added
- **`agt plugin new <name>`** — a plugin scaffolder (the ROADMAP's
  `create-agezt-plugin`). It generates a complete, buildable Go tool plugin on
  top of the SDK: a gofmt-clean `main.go` with one example tool (the output is
  run through `go/format`, so it is always valid, formatted Go), a `go.mod`
  requiring the agezt SDK with a local-dev `replace` hint, a README with build
  and `AGEZT_PLUGINS` wiring instructions, and a `.gitignore`. Refuses to write
  into a non-empty directory. Flags: `--dir`, `--module`. Turns the SDK from
  "copy the example by hand" into "one command to a working plugin" — verified
  end-to-end by building a scaffolded plugin against the real SDK and driving
  its protocol.
- **Go plugin SDK** (`plugins/sdk`) — the official authoring kit for tool
  plugins. `sdk.Serve(sdk.Tool{...})` implements the entire line-delimited JSON
  protocol on the author's behalf: initialize/invoke/shutdown dispatch, frame
  demux, goroutine-safe write serialisation, progress streaming (`Emit`),
  host callbacks (`CallHost`), and panic containment (one bad call returns a
  tool error instead of crashing the plugin). A plugin shrinks from the ~260
  lines of hand-rolled protocol in `testdata/echoplugin` to just its tool
  logic. The package is stdlib-only and imports no kernel package, preserving
  the rule that plugins never compile against the daemon (DECISIONS B0). A
  complete runnable example lives at `plugins/sdk/example/greet`; an
  integration test compiles it and drives it through the real kernel plugin
  host (initialize, invoke success/error, progress, host callback). First
  post-1.0 step toward the polyglot SDK story (ROADMAP §5).

## [1.0.0] — 2026-06-03

**Scale release (ROADMAP M8): "One Agezt across many nodes."** v1.0 fuses the v0.1.0 MVP
with a **federated mesh** and **multi-tenant isolation** — the two halves the ROADMAP
defines as v1.0. The mesh gained peer discovery (`agt peers models`), capability-aware
auto-routing of `remote_run` by model with transport-fault failover, a bounded-TTL
discovery cache, a delegation **loop guard** (hop-limited, tunable, audited, tenant-scoped),
and `agt doctor` / `agt status` mesh observability; the env-spec parsers were hardened
against silent misconfiguration; and finally **per-tenant peer sets** (M219) partition the
mesh by tenant — leak-safe via kernel-stamped tenant identity. All work below was previously
under `[Unreleased]`; it ships as v1.0.0.

### Added
- **Per-tenant mesh peer sets — the federated-mesh × multi-tenant capability** (M219) — the
  `remote_run` mesh tool now routes a tenant's delegations against that tenant's **own** peer
  set, configured via `AGEZT_TENANT_PEERS` (a JSON map `{"alpha":"nodeA=url|tok,…"}`, each value
  an `AGEZT_PEERS`-style spec validated per tenant). This is the ROADMAP's v1.0 (M8)
  intersection of "federated mesh" and "multi-tenant": tenant alpha and tenant beta can
  federate to entirely different node sets. Implemented leak-safe by construction: tenant
  identity is stamped onto every run's context by the **kernel** (new `runtime.Config.TenantID`,
  injected via the new `kernel/tenantctx` package in `RunWith`) — not the HTTP layer — so it
  covers *all* trigger paths uniformly (REST, OpenAI API, schedules, channels), and the tool's
  `peersFor(ctx)` falls back to the **global** set for the primary or any tenant without an
  override, **never** another tenant's. The discovery cache is keyed by peer URL so per-tenant
  name collisions can't cross sets. Single-tenant deployments are unaffected (empty `TenantID`
  ⇒ global peers, exactly as before). Cross-tenant isolation is covered by explicit tests
  (a tenant's run never reaches another tenant's peers; an unknown tenant degrades to global).
  See `.project/PHASE-M219-PER-TENANT-PEERS-REPORT.md`.
- **`agt doctor` flags token-less mesh peers** (M214) — a peer configured as `name=url` with
  no `|token` means `remote_run` delegates tasks to that node **unauthenticated** — at odds
  with Agezt's "loopback + token only" posture, and easy to do by accident. `agt doctor` now
  reports the mesh auth posture (when peers are configured): all-tokened is OK, and any
  token-less peer is a WARN naming it with a hint to add a token. WARN, not FAIL — a peer on a
  trusted private network may legitimately need none — so it only fails `--strict`. Tokens
  themselves are never printed. See `.project/PHASE-M214-DOCTOR-MESH-AUTH-REPORT.md`.
- **`agt doctor` flags a misconfigured mesh hop limit** (M213) — M211 made the mesh hop limit
  tunable via `AGEZT_MESH_MAX_HOPS`, but an invalid value (a typo, a non-integer, out of the
  `[1,64]` range) silently falls back to the default 8 with no signal — a quiet failure for a
  safety-relevant setting. `agt doctor` now surfaces it: when the env is set, a valid override
  is reported OK with its effective value (`delegation hop limit = N`), and an invalid one is a
  WARN naming the bad value and the accepted range. Only shown when the env is set, so
  single-node operators see no noise. New `meshctx.MaxHopsConfig()` reports the effective
  limit, the raw value, and whether a set value was a valid override (the single source the
  daemon and doctor share). See `.project/PHASE-M213-DOCTOR-HOP-CONFIG-REPORT.md`.
- **A loop-refused tenant delegation is audited to that tenant's bus** (M212) — the M210
  `mesh.loop_refused` event always went to the *primary* bus, so when a delegated run that
  loops targeted a specific tenant (`X-Agezt-Tenant`), that tenant never saw its own mesh
  refusal on its pulse/journal — and it landed on the primary's instead, blurring the
  isolation. The REST handler now resolves the target tenant's bus and publishes the audit
  event there (falling back to the primary bus for a header-less or unknown-tenant request).
  The `508` outcome is unchanged. This aligns the mesh loop-guard's observability with the
  per-tenant isolation (M14/M38/M39) — the federated-mesh × multi-tenant intersection. See
  `.project/PHASE-M212-MESH-LOOP-TENANT-BUS-REPORT.md`.
- **The mesh hop limit is operator-tunable** (M211) — the M209 cross-node delegation bound
  was a fixed 8. It is now configurable per node via `AGEZT_MESH_MAX_HOPS`: a deployment with
  legitimately deeper delegation chains can raise it, and a tighter one can lower it. The
  override is validated — only an integer in `[1, 64]` is honored; anything unset, zero,
  negative, over the cap, or unparseable falls back to the default 8, so a typo can't silently
  defeat the guard. The receiving node is authoritative (it enforces its own limit on inbound
  hops), the env is registered in `agt config show`, and the refusal event (M210) reports the
  effective `max_hops`. See `.project/PHASE-M211-MESH-MAXHOPS-CONFIG-REPORT.md`.
- **A refused mesh delegation loop is now audited** (M210) — when the M209 hop guard refuses
  an over-limit cross-node delegation, the REST handler now publishes a `mesh.loop_refused`
  event (`{hop, max_hops}`) to the bus/journal, so a stopped federation loop is visible via
  `agt pulse --kind mesh.loop_refused` / `agt journal` instead of being known only to the
  rejected caller. Honors the kernel's "everything is an event" principle for a
  safety-relevant action; best-effort (a nil bus is simply skipped). See
  `.project/PHASE-M210-MESH-LOOP-EVENT-REPORT.md`.
- **Mesh delegation loop guard — bounded cross-node hops** (M209) — a federated mesh could
  recurse forever: node A's `remote_run` delegates to B, B delegates back to A, and so on,
  each hop a real governed run that costs money and never terminates. Delegations now carry a
  hop count: `remote_run` forwards the current run's hop +1 in an `X-Agezt-Mesh-Hop` header,
  and the receiving node's `POST /api/v1/runs` refuses a run past `meshctx.MaxHops` (8) with
  `508 Loop Detected`, threading the hop into the run context so that node's own `remote_run`
  forwards hop+1 in turn. The delegating tool also refuses locally once at the limit (no
  doomed round-trip). A normal, non-delegated run (local `agt run`, schedule, channel) has no
  header and starts the chain at 0, so nothing changes for single-node use. New
  `kernel/meshctx` package carries the hop through the run context (which already threads from
  the REST handler down to tool invocation). See `.project/PHASE-M209-MESH-LOOP-GUARD-REPORT.md`.
- **`agt status` shows the configured peer mesh** (M208) — the live status overview now
  includes a `mesh` line listing the configured peers (`AGEZT_PEERS`) with their URLs, and a
  `mesh` array (name + URL) in `agt status --json`. It is a cheap client-side config snapshot
  — **no** health probe (that stays the job of `agt doctor` (M207) and `agt peers`), so
  `status` remains fast even when a peer is down — and tokens are always redacted. Quiet when
  single-node, so most operators see no extra noise. Pairs the mesh's *configuration* view
  (status) with its *health* view (doctor/peers). See
  `.project/PHASE-M208-STATUS-MESH-REPORT.md`.
- **`agt doctor` gains a mesh-health check** (M207) — the operator's go-to pre-flight now
  reports the configured peer mesh (`AGEZT_PEERS`): it probes each peer's REST
  `/api/v1/health` (reusing the `agt peers` check) and reports all-reachable as OK, an
  unreachable peer as WARN (the local node is fine, the mesh is degraded) naming the down
  peers with a remediation hint, a malformed `AGEZT_PEERS` as WARN, and no-peers as an
  informational OK (single-node). So a broken mesh surfaces in the one diagnostic operators
  already run, instead of only at `remote_run` time. The check is independent of the local
  daemon (each peer is reached over its own surface) and never prints tokens. Completes the
  mesh thread's operability story alongside M204's `agt peers route`. See
  `.project/PHASE-M207-DOCTOR-MESH-CHECK-REPORT.md`.
- **`remote_run` auto-routing fails over to the next serving peer (fault tolerance)** (M206)
  — when auto-routing by model, the M203 router picked a single peer; if that node was down
  the whole delegation failed even though another peer could serve the model. The router now
  considers *all* peers that serve the model (in name order) and, on a **transport** failure
  (no HTTP response — the task provably never ran), falls back to the next one, surfacing an
  error only when every serving peer is unreachable. Crucially, a peer that *responds* — even
  with an error status — is **never** retried elsewhere, because it may already have executed
  side effects; that failure is surfaced as-is. Named-peer and single-peer dispatch are
  unchanged (no failover, original error message preserved). This gives the mesh genuine node
  fault tolerance without risking double execution. See
  `.project/PHASE-M206-AUTOROUTE-FAILOVER-REPORT.md`.
- **`remote_run` auto-routing caches model discovery (bounded TTL)** (M205) — the M203
  auto-router probed every candidate peer's `GET /api/v1/models` on *every* invocation; in a
  busy agent loop that meant a fan-out of discovery requests per delegated task. The tool now
  caches each peer's model list for a bounded TTL (`DefaultCacheTTL` = 60s), so repeated
  auto-routes reuse the recent result instead of re-probing the mesh — model inventories
  change rarely, and the TTL bounds staleness. Discovery errors are *not* cached (a transient
  failure won't suppress a later retry), the cache is mutex-guarded with the network call made
  outside the lock (no serialization of concurrent discoveries), and a named-peer dispatch
  still skips discovery entirely. Behaviour is otherwise unchanged. See
  `.project/PHASE-M205-DISCOVERY-CACHE-REPORT.md`.
- **`agt peers route <model>` — inspect mesh routing decisions** (M204) — a new verb that
  shows which peer `remote_run` would auto-route a task for `<model>` to, and the fallback
  order, *without* dispatching anything. It mirrors the tool's selection exactly (M203):
  peers are queried in name order and the first reachable one that serves the model is marked
  `chosen`, the other servers `fallback`, non-servers `does not serve`, and unreachable peers
  are surfaced — so an operator can answer "why did my remote_run land on peer Y?" and verify
  mesh wiring before running anything. Exits non-zero when no reachable peer serves the model;
  `--json` for scripting. Completes the mesh routing trio: M201 discover → M203 auto-route →
  M204 inspect. See `.project/PHASE-M204-PEERS-ROUTE-INSPECT-REPORT.md`.
- **`remote_run` auto-routes to a peer that serves the requested model — cross-node routing**
  (M203) — when a `model` is given but no `peer` is named (and more than one peer is
  configured), the mesh tool now discovers each peer's routable models (`GET /api/v1/models`)
  and dispatches the task to a peer that serves that model, instead of erroring "a peer name
  is required". Peers are queried in name order so the pick is deterministic; an unreachable
  peer is skipped (not fatal); if no peer serves the model the error names what was checked
  and what was unreachable. This is the automation layer over M201 (discovery) + M202 (model
  pinning): the agent can now say "run this on `opus`" without knowing which node has it.
  Naming a peer still bypasses discovery entirely (the explicit-dispatch path and single-peer
  behaviour are unchanged), and discovery responses are bounded-read (1 MiB). See
  `.project/PHASE-M203-REMOTE-RUN-AUTOROUTE-REPORT.md`.
- **`remote_run` can pin the peer's model — capability-aware delegation** (M202) — the mesh
  delegation tool now accepts an optional `model` argument and forwards it to the peer's
  `POST /api/v1/runs`, so the delegating node can ask a peer to route the handed-off task to
  a *specific* model (one the peer can serve) instead of always falling back to the peer's
  default. This is the dispatch half that pairs with M201's discovery (`agt peers models`
  tells you which node serves a model; `remote_run {model}` sends the task there). The model
  is forwarded only when pinned — an absent/blank model leaves the request body byte-for-byte
  unchanged, so existing delegations behave exactly as before. The peer echoes the model it
  actually routed to, which is now recorded in the result footer (`[peer=… model=…
  correlation=…]`) for an auditable cross-node trail. See
  `.project/PHASE-M202-REMOTE-RUN-MODEL-REPORT.md`.
- **`agt peers models [<name>]` — mesh model discovery** (M201) — a new verb on the M8
  mesh command that fetches each configured peer's routable model set (and its default)
  from the peer's `GET /api/v1/models`, so an operator can see *which* node can serve a
  given model before dispatching a `remote_run` — previously `agt peers` only reported a
  health summary with a model *count*, not the actual ids. Queries all peers (sorted) or a
  single named one; `--json` for scripting; exits non-zero if any queried peer is
  unreachable. The response is bounded-read (1 MiB `io.LimitReader`, shared with the M200
  health cap via the renamed `maxPeerResponseBytes`) and tokens are never printed. See
  `.project/PHASE-M201-PEER-MODELS-DISCOVERY-REPORT.md`.
- **`agt run --dry-run` warns when strict pricing would refuse the run** (M195) — with
  `AGEZT_PRICING_STRICT=on`, a run on a model with no known price is refused before any
  provider call; the dry-run now surfaces that ("…would be REFUSED before any provider
  call; `agt catalog sync`…") so an operator learns it up front instead of from a
  surprising submit-time failure — the same preventive-advisory pattern as the cost-cap
  warnings (M167/M169). Correctly distinguishes a known-FREE model (priced 0, allowed)
  from a genuinely unpriced one via the new exported `governor.ModelIsPriced`. See
  `.project/PHASE-M195-DRYRUN-STRICT-PRICING-REPORT.md`.
- **`AGEZT_PRICING_STRICT` env + `agt budget` spend-protection line** (M194) — makes the
  M193 strict-pricing gate operator-configurable (`AGEZT_PRICING_STRICT=on`, off by
  default, registered in `agt config show`) and surfaces the posture in `agt budget`:
  a `pricing  strict: …` / `pricing  lax: unpriced models are charged $0 (set …)` line
  alongside the spend total, plus `strict_pricing` in `agt budget --json`. So an operator
  can both turn the protection on and see at a glance whether unpriced models are refused
  or silently free. See `.project/PHASE-M194-BUDGET-STRICT-PRICING-SURFACE-REPORT.md`.
- **`StrictPricing` governor mode — refuse unpriced models instead of charging $0
  (governor review HIGH)** (M193) — by default a model with no known price (missing from
  the catalog AND the fallback table) is charged $0, so it silently bypasses the daily and
  per-task budgets (fail-open). The new opt-in `Config.StrictPricing` refuses such a
  request with `ErrUnpricedModel` BEFORE any provider call (journaling a `budget.unpriced`
  event), so an operator can guarantee every billed call is accounted for. Known-FREE
  models (local/mock, priced 0 in the table) and an empty `req.Model` still pass — only
  genuinely unknown models are refused. `priceFor` now distinguishes "found, price 0" from
  "not found" via `priceForOk`/`modelIsPriced`. See
  `.project/PHASE-M193-GOVERNOR-STRICT-PRICING-REPORT.md`.
- **Cost cap inert-on-unpriced detection, authoritative + run-time** (M169) — a
  per-run `--max-cost` can only bind if the run accrues *priced* spend; on a model
  with no known pricing (unknown to the catalog AND absent from the fallback table,
  or a free/local model) the cap silently never trips. Detection is now authoritative
  via `governor.CostMicrocents` (catalog → fallback), replacing the catalog-only
  `m.Cost != nil` check that mis-classified fallback-priced or catalog-unknown models.
  At run submission, a cap on an unpriced model journals a `budget.cap_inert` advisory
  tied to the run's correlation (so `agt why <run>` shows the guardrail was inert);
  `agt run --dry-run` reports it ahead of time. See
  `.project/PHASE-M169-COST-CAP-INERT-REPORT.md`.
- **`agt run --dry-run` shows the cost cap** (M167) — the dry-run plan now carries a
  `cost_cap` line (`$0.50 (per-run)` / `none`), completing the per-run override
  preview (model/system/timeout/tools/**cost**). It also advises when a `--max-cost`
  is set against an **unpriced** model (unknown to the catalog, or free/local with no
  cost): the cap can never trip there, so the dry-run warns "the cap will not bind"
  rather than letting an operator believe a run is money-bounded when it isn't. See
  `.project/PHASE-M167-DRY-RUN-COST-CAP-REPORT.md`.
- **`agt run --max-cost <usd>` — per-run cost cap** (M166) — bound a single run's
  cumulative provider spend (`agt run --max-cost 0.50 "…"`) without a daemon-wide
  ceiling — the money analogue of `--timeout` (M154). The agent loop accumulates each
  call's spend locally and, once the running total reaches the cap, terminates the
  run with `task.failed(reason=cost_budget)` — bounded overshoot of at most one call,
  exactly like the daily ceiling. Implemented as a local stack accumulator in the
  loop fed by an injected `CostFn` (`governor.CostMicrocents`), so it adds **zero**
  shared state or concurrency surface (no per-run map, no lifecycle). New
  `runtime.WithMaxCost` ctx override + a `max_cost` run arg (microcents); the CLI
  parses dollars (`$0.50` / `0.50`) and rejects a non-positive/garbage amount
  client-side. The Governor's daily ceiling still applies on top. See
  `.project/PHASE-M166-RUN-MAX-COST-REPORT.md`.
- **`agt doctor --strict`** (M165) — exit non-zero on **warnings** too, not just
  failures. By default warnings are advisories (exit 0); `--strict` makes any WARN
  exit 1, so monitoring/CI can alert on the advisory-level security signals the
  recent checks surface (a failing schedule, an egress block, throttling) instead of
  only hard failures. Text mode prints `strict: warnings treated as failures (exit
  1)`; `--json` gains `strict` and an `ok` field (the strict-aware exit verdict),
  while `healthy` still tracks FAILs only. See
  `.project/PHASE-M165-DOCTOR-STRICT-REPORT.md`.
- **Rate-limit health check in `agt doctor`** (M164) — `agt doctor` now WARNs when
  callers have been throttled in the last 24h: a `rate.limited` event means a tenant
  exceeded its per-minute request cap (M14 quotas) and was refused. Persistent
  throttling means a caller is undersized for its workload (or something is hammering
  the daemon), and otherwise only manifests as mysterious failed runs. The detail
  carries the count, the cap, and the peak observed rate. Reuses `CmdRateLimitStats`
  (M106) over a self-clearing 24h window (shared with the netguard check). See
  `.project/PHASE-M164-DOCTOR-RATELIMIT-REPORT.md`.
- **Egress-guard health check in `agt doctor`** (M163) — `agt doctor` now WARNs when
  the netguard egress guard has refused connections in the last 24h: a
  `netguard.blocked` event means a tool (http/browser) tried to reach an
  internal/metadata address (e.g. `169.254.169.254`) and was stopped — a strong
  SSRF / prompt-injection / exfiltration signal, or a legitimate host that needs
  allowlisting. The hint names the most recent target (`tool→ip`). It's a WARN, not
  a FAIL — the guard did its job — and self-clears after 24h of clean operation, so
  a reviewed historical block doesn't alarm forever. Reuses `CmdNetguardLog` (M109);
  no control-plane change. See `.project/PHASE-M163-DOCTOR-NETGUARD-REPORT.md`.
- **Scheduled-run health check in `agt doctor`** (M162) — `agt doctor` now folds
  the autonomy axis into the single-pane diagnostic: it WARNs when an **enabled**
  schedule's most recent firing `failed` or was `abandoned`, naming the schedule in
  the hint (`agt schedule fires --id <id>`). Scheduled runs fire unattended, so a
  failing one otherwise sits silently in the journal until someone thinks to run
  `agt schedule list`. A disabled schedule's past failure is ignored, and a
  never-fired schedule is healthy-by-default. See
  `.project/PHASE-M162-DOCTOR-SCHEDULES-REPORT.md`.
- **`agt run --dry-run` advisories** (M160) — the dry-run plan now carries a
  `warnings` list of preventive advisories a run would otherwise hit only at
  execution time: an effective model that isn't in the catalog (capabilities
  unverified); a `tool_call=false` model with tools enabled (calls may be ignored,
  and under `AGEZT_MODEL_STRICT=on` the run would be rejected pre-flight) — surfaced
  only when tools are actually enabled, so `--no-tools` stays quiet; and a small
  context window (<8192 tokens). Human output gains a `warnings:` section; `--json`
  gains a `warnings` array (omitted when empty). See
  `.project/PHASE-M160-DRY-RUN-ADVISORIES-REPORT.md`.
- **`agt run --dry-run` — resolve the run plan without executing** (M159) — print
  exactly what a run WOULD do (effective model + its catalog capabilities, the
  system-prompt source, the effective wall-clock timeout, and the precise tool set
  the agent loop would see after the `--tools`/`--no-tools` filter) and stop — no run
  started, no tokens spent. Composes with every per-run override
  (`--model`/`--system`/`--timeout`/`--tools`) and passes the same vision gate, so an
  operator can confirm resolution (and spot a model that isn't in the catalog, or a
  requested tool that isn't registered) before committing budget. `--json` emits the
  raw plan object. Reuses `CmdRun` with a `dry_run` arg (no new protocol command).
  See `.project/PHASE-M159-RUN-DRY-RUN-REPORT.md`.
- **`agt run --tools <csv>` / `--no-tools` — per-run tool restriction** (M158) — scope
  a single run to a named subset of tools (`agt run --tools shell,file "..."`) or
  disable tools entirely (`agt run --no-tools "what is 2+2"`) for a safe,
  pure-reasoning query — without changing the daemon's tool set. A new
  `runtime.WithTools` ctx override carries the allow-list; `RunWith` applies
  `filterTools` to the kernel's tools just for that run. An empty allow-list (the
  `--no-tools` case) is distinct from omitting the flag (full toolset): a model that
  still calls a filtered-out tool gets `tool "X" is not available` fed back, exactly
  like an unknown tool. Completes the per-run override family (`--model` / `--system`
  / `--timeout` / `--tools`). See `.project/PHASE-M158-RUN-TOOLS-REPORT.md`.
- **`agt run --quiet` / `-q`** (M156) — print ONLY the final answer (no per-event
  lines, no live token stream, no correlation/usage footer), so scripts can
  `agt run -q --file spec.md > answer.txt` and get clean output. `--json` still
  takes precedence for machine consumption. See
  `.project/PHASE-M156-RUN-QUIET-REPORT.md`.
- **`agt run --timeout <dur>` — per-run wall-clock timeout** (M154) — bound a single
  run (`agt run --timeout 30s "..."`) without setting the daemon-wide
  `AGEZT_RUN_TIMEOUT`. Completes the per-run override family (`--model` / `--system`
  / `--timeout`): a new `runtime.WithRunTimeout` ctx override that takes precedence
  over `Config.MaxDuration`; an overrun cancels with `DeadlineExceeded` →
  `task.failed(reason=timeout)`, exactly like the daemon-wide cap. The CLI validates
  the duration client-side and keeps the connection open at least as long as the run
  may take. See `.project/PHASE-M154-RUN-TIMEOUT-REPORT.md`.
- **Budget-headroom check in `agt doctor`** (M153) — `agt doctor` now reports the
  day's spend against the daily ceiling and WARNs as it nears (≥90%) or reaches the
  cap. Once the ceiling is hit, runs fail terminally (no fallback), so an operator
  wants the heads-up before a confusing mid-run "all providers failed", not after.
  All-clear shows `$X / $Y today (Z%)`; no ceiling configured is an OK; a failed
  budget call is an informational OK (never a false alarm). See
  `.project/PHASE-M153-DOCTOR-BUDGET-REPORT.md`.
- **Scheduled runs can deliver their answer to a channel** (M152) — with
  `AGEZT_SCHEDULE_NOTIFY=on`, each scheduled intent's answer is pushed to the
  operator's configured channels (Telegram/Slack/Discord allowlists), prefixed with
  the schedule id — so a "every morning, summarise new commits" job actually reaches
  you instead of sitting silently in the journal. Closes the Jarvis proactive loop
  (schedule → run → deliver) without the intent having to call `notify` itself.
  Off by default; only successful, non-empty answers are sent; reuses the channel
  sender + allowlists. See `.project/PHASE-M152-SCHEDULE-NOTIFY-REPORT.md`.
- **`agt run` reads intent from stdin or a file** (M151) — long or multi-line
  prompts no longer have to be quoted on the command line: `agt run -` reads the
  intent from stdin (`cat prompt.txt | agt run -`, heredocs, pipelines) and
  `agt run --file <path>` reads it from a file. Precedence: `--file` → `-` (stdin) →
  the joined positional text; all trimmed. A missing `--file` is a clear error. See
  `.project/PHASE-M151-RUN-STDIN-FILE-REPORT.md`.
- **Channel-health check in `agt doctor`** (M150) — `agt doctor` now WARNs when a
  messaging channel is **half-configured**: it has a listen addr but inbound is
  disabled (a Slack/Discord webhook channel set up with a token + addr but no
  signing secret / public key), so the endpoint is up yet silently rejects every
  event. The boot banner shows this once and `agt status` renders it as
  "outbound-only" (M141), but neither nags — now it's a persistent check in the
  go-to diagnostic, naming the channel and the fix (the M137 status→doctor pairing).
  All-healthy / no-channels is an OK; an addr-less outbound-only channel is a
  deliberate choice, not flagged. See `.project/PHASE-M150-DOCTOR-CHANNELS-REPORT.md`.
- **`agt run --system <prompt>` — per-run system-prompt override** (M149) — set a
  one-off persona/instruction for a single run (`agt run --system "You are a terse
  reviewer." "..."`) without changing `AGEZT_SYSTEM_PROMPT` or restarting. The
  sibling of `--model` (M148): a new `runtime.WithSystem` ctx override that REPLACES
  the configured base system prompt for that run, while memory / world / skill
  injection still layer on top. Empty = the kernel default. See
  `.project/PHASE-M149-RUN-SYSTEM-OVERRIDE-REPORT.md`.
- **`agt run --model <id>` — per-run model override** (M148) — route a single run to
  a specific model (`agt run --model claude-opus-4-8 "hard one"` /
  `--model haiku "quick one"`) without restarting the daemon or changing
  `AGEZT_MODEL`. Reuses the same per-request routing the OpenAI-compatible API uses
  (`runtime.WithModel` → the loop's `modelFromCtx`); empty = the kernel default. The
  vision capability gate now judges the *effective* model (the override, not the
  daemon default), so attaching an image to a vision-capable override model is no
  longer wrongly rejected. See `.project/PHASE-M148-RUN-MODEL-OVERRIDE-REPORT.md`.
- **`agt run` reports what the run cost** (M146) — a finished run now prints a
  `usage:` line with the model, iteration count, and USD cost
  (`usage: claude-sonnet-4-6 · 4 iteration(s) · $0.0123`), so an operator sees the
  price of a run without a follow-up `agt runs show`. The fields (`model`, `iters`,
  `spent_mc`) are folded from the journal via the same `collectRuns` path `agt runs`
  uses (so the numbers agree) and added to the run result, so `agt run --json`
  carries them too. Unpriced runs (e.g. the offline mock) omit the cost. See
  `.project/PHASE-M146-RUN-USAGE-REPORT.md`.
- **Multi-turn conversation context for channels** (M144, SPEC-04 §1.4) — an inbound
  chat message used to start a fresh, memory-less run, so "what's the capital of
  France?" → "and Germany?" lost all thread. Now every channel (Telegram/Slack/
  Discord) prepends a compact transcript of the recent conversation for that chat
  (folded read-only from the journal, the same source as `agt inbox`) as the run
  intent, so the agent answers follow-ups in context. Bounded by
  `AGEZT_CHANNEL_HISTORY` messages (default 10; `0` disables; each message clipped),
  and the first turn of a conversation runs the raw text unchanged. No new state, no
  new event. See `.project/PHASE-M144-CHANNEL-CONTEXT-REPORT.md`.
- **`notify` tool — proactive agent messaging** (M143) — a running agent can now
  send a short message to the operator over a configured channel MID-task ("I've
  started the long task, I'll report back"; a progress note; an alert) instead of
  staying silent until the final reply — the Jarvis "keep me posted" capability.
  Security (SPEC-04 §1.7): destinations are PINNED to the operator's own configured
  allowlist — the agent supplies only the text (and optionally which channel kind),
  never the recipient — so a prompt-injected agent can only ever message the
  operator's own chats, not exfiltrate to arbitrary ids. Gated by Edict `CapNotify`
  (allowed by default; operator can raise/deny like any capability); the send is
  journaled as `channel.outbound` (visible in `agt inbox` / `agt why`). Registered
  into the live tool map at boot when at least one channel has an allowlist; the mock
  driver exercises it via `AGEZT_DEMO_NOTIFY=1`. See
  `.project/PHASE-M143-NOTIFY-TOOL-REPORT.md`.
- **`agt send` — operator-initiated channel egress** (M142) — push a one-off message
  out a configured channel: `agt send --channel discord --to D9 "deploy finished"`.
  The manual complement to Pulse briefs and agent replies, so a script / CI / cron job
  can notify a chat without driving the agent ("build green → ping Slack"). Routed
  through the control plane (authenticated by the primary token, so no per-channel
  allowlist gate — the caller already holds daemon authority) to the live channel's
  `Send`, which journals `channel.outbound` — so the message shows up in
  `agt inbox` / `agt why` like any other. An unconfigured channel kind is a clear
  error. Wired via `Server.SetChannelSender` (a primitive func, so `kernel/controlplane`
  still never imports the channel plugins). See
  `.project/PHASE-M142-AGT-SEND-REPORT.md`.
- **Configured channels in `agt status`** (M141) — the daemon now reports its
  messaging channels (Telegram, Slack, Discord) in `agt status`:
  `channels : telegram (inbound, allow 2), slack (inbound @127.0.0.1:8840, allow 1),
  discord (outbound-only, allow 3)`. Each channel shows whether it can receive
  commands (`inbound` vs `outbound-only`), its listen addr (webhook channels), and
  its allowlist size — so an operator confirms what's listening without scrolling
  back to the boot banner. Crucially, a half-configured webhook channel (a listen
  addr but no signing secret / public key) shows as `outbound-only`, surfacing a
  silent misconfiguration. Injected via `Server.SetChannels` (the M137
  `SetHTTPBindings` decoupling pattern); also in `agt status --json` under
  `channels`. See `.project/PHASE-M141-CHANNELS-IN-STATUS-REPORT.md`.
- **`agt inbox --channel KIND` filter** (M140, SPEC-07 §4) — with three duplex
  channels now live (Telegram, Slack, Discord), the Unified Inbox mixes every
  platform's threads together. `agt inbox --channel discord` (also `--channel=slack`,
  case-insensitive) scopes the view to one channel kind, applied server-side over the
  journal fold before the limit so `inbox 5 --channel slack` means "the last 5 Slack
  threads", not "Slack threads among the last 5". The control-plane `inbox` command
  gains an optional `channel` arg and echoes the applied filter back; an unmatched
  filter returns an empty inbox with a kind-specific message. See
  `.project/PHASE-M140-INBOX-CHANNEL-FILTER-REPORT.md`.
- **Discord channel** (M139, SPEC-04 §1) — a third first-class duplex channel,
  stdlib-only (`net/http` + `crypto/ed25519`, no SDK, no Gateway WebSocket). Free-form
  Discord messages need the Gateway (a persistent WebSocket → a dependency); instead
  the channel drives the agent through Discord's HTTP **Interactions** endpoint (a
  slash command like `/agezt prompt:<text>`). It SERVES `POST /discord/interactions`,
  verifies Discord's **Ed25519** request signature over `(timestamp‖body)` with a
  5-minute freshness window (an empty/invalid public key fails closed), replies to the
  PING handshake, then for a command ACKs with a DEFERRED response in <3s ("Agezt is
  thinking…"), runs the agent asynchronously, and delivers the answer via a follow-up
  webhook (`webhooks/{app}/{token}`). A channel-id Allowlist gates who may drive the
  agent — a non-allowlisted command gets an immediate ephemeral "not authorized" and
  never runs. Outbound briefs (Pulse) post via the bot token to `channels/{id}/messages`.
  Inbound/outbound are journaled (`channel.inbound.discord` / `channel.outbound.discord`)
  for `agt why`. This proves the channel abstraction generalizes across signature
  schemes: Telegram long-polls, Slack signs with HMAC-SHA256, Discord with Ed25519 —
  the kernel sees only `UnifiedMessage`. Config via `AGEZT_DISCORD_TOKEN` /
  `_PUBLIC_KEY` / `_APP_ID` / `_ADDR` / `_CHANNELS` / `_API_BASE`. See
  `.project/PHASE-M139-DISCORD-CHANNEL-REPORT.md`.
- **Slack channel** (M138, SPEC-04 §1) — a second first-class duplex channel beyond
  Telegram, stdlib-only (`net/http` + `crypto/hmac`, no SDK). Unlike Telegram's
  long-poll, Slack pushes events, so the channel SERVES an Events API endpoint
  (`POST /slack/events`): inbound is verified with Slack's HMAC-SHA256 request
  signature + a 5-minute timestamp freshness window (replay protection), ACKed in
  <3s, then the agent runs asynchronously and posts its reply via `chat.postMessage`.
  An empty signing secret fails closed; a channel-id Allowlist gates who may drive
  the agent; bot/self/subtype messages are ignored so the agent never loops on its
  own posts; retries (`X-Slack-Retry-Num`) are ACKed but not reprocessed. Inbound and
  outbound are journaled (`channel.inbound.slack` / `channel.outbound.slack`) so
  `agt why` can reconstruct the exchange. Config via `AGEZT_SLACK_TOKEN` /
  `_SIGNING_SECRET` / `_ADDR` / `_CHANNELS` / `_API_BASE`; the channel feeds Pulse
  briefs through the shared sink. See `.project/PHASE-M138-SLACK-CHANNEL-REPORT.md`.
- **Network-exposure check in `agt doctor` + `agt status`** (M137) — the web UI /
  REST / OpenAI HTTP servers drive the full agent loop (shell/file/http tools) gated
  only by a token, so a non-loopback bind puts the agent on the network. The daemon
  warned once at boot; now `agt status` reports each HTTP server's bind + loopback
  state, and `agt doctor` WARNs persistently when any is reachable beyond localhost,
  naming it with the remediation. See `.project/PHASE-M137-EXPOSURE-CHECK-REPORT.md`.
- **Graceful shutdown drain** (M136, SPEC-08 §3.1) — on `agt shutdown` / SIGTERM the
  daemon now flips `/readyz` to not-ready (`"draining"`) FIRST, so a load balancer /
  k8s readiness probe stops routing new traffic, then waits (bounded by
  `AGEZT_DRAIN_TIMEOUT`, default 15s) for in-flight runs to finish before halting
  them — a rolling restart no longer kills work mid-flight. Set the timeout to 0 for
  the old immediate-halt behavior. See `.project/PHASE-M136-GRACEFUL-DRAIN-REPORT.md`.
- **Prometheus `/metrics` endpoint** (M135, SPEC-14 §9) — the REST API exposes the
  daemon's operational gauges in Prometheus text format (up, halted, uptime,
  active_runs, journal_head_seq/bytes, memory/world/skill counts, schedules,
  pending_approvals, spend_today + budget ceiling, disk_free_bytes/ratio) so it can
  be wired into Grafana/alerting. Token-authed (it exposes spend/activity; scrape
  with a bearer_token), stdlib-only, all reads cheap (no per-scrape journal fold).
  Pairs with the M134 health probes. See `.project/PHASE-M135-METRICS-REPORT.md`.
- **Unauthenticated `/healthz` + `/readyz` probes** (M134, SPEC-14 §9) — the REST
  API now serves deployment-grade health endpoints with no token, so systemd
  watchdogs, container/k8s liveness+readiness probes, load balancers, and uptime
  monitors can check the daemon. `/healthz` is liveness (200 while the process
  serves). `/readyz` is readiness — 200 when serving, **503 while halted** (a load
  balancer pulls it from rotation without the process dying). They expose only
  liveness/readiness; version/model stay behind the authed `/api/v1/health`. See
  `.project/PHASE-M134-HEALTH-PROBES-REPORT.md`.
- **`agt changelog` — the system timeline** (M133, SPEC-08 §4.2) — a curated,
  tamper-evident fold of the journal showing only MATERIAL changes to the system
  (halt/resume, policy changes, skill lifecycle, reflection, catalog/provider sync,
  pulse pause/resume), newest-first, each carrying its event id so `agt why <id>`
  can prove and explain it. Distinct from `journal tail` (raw, every kind): the
  human-meaningful "what changed about my system, and when". See
  `.project/PHASE-M133-SYSTEM-CHANGELOG-REPORT.md`.
- **`agt journal stats`** (M132) — the journal's size and shape: total events,
  segment count, on-disk bytes, the time span it covers, and a per-event-kind
  breakdown so an operator sees WHAT is filling it (neither `agt disk` nor `status`
  showed this). The journal is append-only / full-retention (projections rebuild
  from it on boot, so it isn't pruned in place); the disk-pressure remedy across
  `agt disk` / `doctor` now correctly points at `agt backup` + a larger disk
  instead of an unsafe in-place prune. See `.project/PHASE-M132-JOURNAL-STATS-REPORT.md`.

- **Disk-space observability: `agt disk` + a doctor check** (M131) — the journal is
  append-only and never shrinks, so on a small host (the $5-VPS deploy target) a
  full disk is the classic silent outage: writes start failing and the daemon
  stops recording. `agt disk` reports the journal's on-disk size and free space on
  its filesystem; `agt doctor` now WARNs under 10% free and FAILs under 3% (the
  journal will soon fail to write). The daemon reports its own disk via an injected
  cross-platform probe (`pulse.DiskUsage`), keeping controlplane free of the pulse
  import. See `.project/PHASE-M131-DISK-SPACE-REPORT.md`.
- **`agt status` shows autonomy + actionable signals** (M130) — the at-a-glance
  dashboard now reports armed scheduled intents (`schedules : N (M enabled)`),
  pending HITL approvals (`approvals : K PENDING — answer with agt approvals`), and
  the tenant count when multi-tenancy is on. Scheduled autonomy and a blocking
  approval queue were previously invisible until something tripped. Cheap in-memory
  reads; quiet when there's nothing to show. See
  `.project/PHASE-M130-STATUS-SIGNALS-REPORT.md`.
- **`--tenant` on the observability CLIs** (M129) — `agt memory log`, `world log`,
  `approvals log|stats`, `plan history|stats`, `provider log|stats|rejections`,
  `schedule fires|stats`, and `warden log|stats` now accept `--tenant <id>`, so an
  operator can inspect any tenant's own isolated subsystems and a tenant can read
  its own via the CLI. The client half of the M128 daemon grant. See
  `.project/PHASE-M129-OBSERVABILITY-TENANT-FLAG-REPORT.md`.

### Fixed
- **A webhook HMAC secret containing `|` is no longer truncated** (M218) — `ParseSinks` split
  each `url|subject|secret` entry on `|` with an unbounded `strings.Split`, so a secret that
  itself contained a pipe (`…|subject|se|cr|et`) kept only the text up to the third `|` and
  silently dropped the rest — corrupting the signing key so *every* delivery's HMAC signature
  mismatched at the receiver, a silent delivery failure with no error to diagnose. The split
  is now `SplitN(entry, "|", 3)`, so the secret field captures everything after the second
  pipe intact. Found alongside the M217 subject-filter validation. See
  `.project/PHASE-M218-WEBHOOK-SECRET-PIPE-REPORT.md`.
- **A malformed webhook subject filter is now rejected at parse time** (M217) — a sink in
  `AGEZT_WEBHOOKS` (`url|subject|secret`) whose subject filter was not a well-formed NATS-style
  pattern (an empty token like `agent..tool`, or `>` not last like `>.agent`) was silently
  accepted — and then matched *nothing*, so the sink delivered no events with no error,
  presenting as a baffling "my webhook never fires". `ParseSinks` now validates the filter via
  the newly-exported `bus.ValidatePattern` and rejects a malformed one at startup with a clear
  message. An empty filter still defaults to `>` (all events). See
  `.project/PHASE-M217-WEBHOOK-SUBJECT-VALIDATION-REPORT.md`.
- **A duplicate prefix in the plugin pin / tool-allowlist specs is now rejected** (M216) — both
  `ParsePinSpec` (`AGEZT_PLUGIN_PINS`, binary-hash integrity) and `ParseToolAllowlistSpec`
  (`AGEZT_PLUGIN_TOOLS`, restricting what a plugin may advertise) keyed by prefix and silently
  overwrote on a collision, so a typo'd or copy-pasted duplicate (`search=hashA,…,search=hashB`)
  silently shadowed the operator's intended value — for *security* controls, where a lost pin
  could admit the wrong binary or a lost allowlist could widen what a plugin exposes. A
  duplicate prefix is now a hard error (`… for "search" is defined more than once`), caught at
  startup like the other malformed-spec cases — the same class of fix as M215's duplicate peer
  name. See `.project/PHASE-M216-PLUGIN-SPEC-DUP-PREFIX-REPORT.md`.
- **A duplicate peer name in `AGEZT_PEERS` is now rejected** (M215) — `ParsePeers` keyed peers
  by name and silently overwrote on a collision, so `AGEZT_PEERS="a=…,b=…,a=…"` parsed to TWO
  peers, not three: a mesh node was silently lost, and a `remote_run` to the shadowed name hit
  the wrong URL. A duplicate name is now a hard error (`peer "a" is defined more than once`),
  caught at daemon startup like other malformed specs — so the misconfiguration surfaces
  immediately instead of becoming a silent routing bug. Distinct names sharing a URL remain
  valid. See `.project/PHASE-M215-PEER-DUP-NAME-REPORT.md`.
- **`agt peers` bounds a peer's health response** (M200) — the mesh health check
  (`checkPeer`) decoded a peer's `GET /api/v1/health` body with `json.NewDecoder(resp.Body)`
  and no size cap, so a hostile or misconfigured peer could stream an unbounded body and
  exhaust the operator's CLI — even though the sibling `remote_run` tool already bounds its
  own peer responses to 1 MiB. The decode now reads through `io.LimitReader(resp.Body,
  maxPeerHealthBytes)` (1 MiB, matching `remote_run`); an over-limit body is cut off and the
  peer is reported unreachable with a decode error instead of being ingested. Completes the
  bounded-read guarantee (plugin host M177, mcpbridge M185, control plane M188, HTTP APIs
  M198) on the federation/mesh client surface. See
  `.project/PHASE-M200-PEER-HEALTH-BOUND-REPORT.md`.
- **One-shot schedules are now crash-safe (at-least-once)** (M199) — a `once` schedule
  (`agt schedule once …`) was removed from the store the instant `Store.Due` reported it as
  due, *before* its run launched. A daemon crash in the run window therefore dropped the
  one-shot silently: it never ran and was already gone from the store, so a restart could
  not recover it. `Due` now leaves a one-shot in place (enabled and due) for the whole
  duration of its run; the engine removes it via the new `Store.CompleteFiring` only after
  the run completes (success or error, so a permanently-failing one-shot cannot retry-storm).
  A crash mid-run thus re-fires the one-shot on restart instead of dropping it, and the
  engine's in-flight guard still prevents a duplicate fire across ticks. Recurring
  (interval/daily/window) schedules keep their deliberate at-most-once advance — a slot
  missed across a crash self-corrects at the next slot, which is less disruptive than
  re-running a stale recurring slot on restart. See
  `.project/PHASE-M199-CRASH-SAFE-ONESHOT-REPORT.md`.
- **HTTP request bodies are now bounded on the network-exposed API surfaces** (M198) — the
  REST control surface (`POST /api/v1/runs`) and the OpenAI-compatible surface
  (`/v1/chat/completions`, `/v1/responses`) decoded `r.Body` with `json.NewDecoder` and no
  size cap, so an authenticated client could stream an unbounded body and force unbounded
  memory growth — the HTTP analogue of the framed-read caps already applied to the plugin
  host (M177), mcpbridge (M185), and control plane (M188). Each body is now wrapped in
  `http.MaxBytesReader(w, r.Body, 16 MiB)`; an over-limit body is rejected with `413
  Request Entity Too Large` (`request_too_large` / `invalid_request_error`) instead of
  being read into memory, and normal requests are unaffected. Both surfaces are
  post-authentication, so this is defense-in-depth against an authenticated DoS, not a
  pre-auth hole. See `.project/PHASE-M198-HTTP-BODY-BOUND-REPORT.md`.
- **Daily scheduling hardened against DST fall-back double-fire + regression coverage**
  (M197) — `nextDaily` now makes the "fire once per day" property EXPLICIT: on a fall-back
  day the wall-clock `at` time occurs twice, so a same-day (`i==0`) candidate at or before
  `now`'s wall clock is skipped, advancing to the next permitted day. In practice Go
  resolves an ambiguous wall time to the *earlier* offset, so the daily schedule does not
  actually double-fire today — but that's an implicit runtime detail; the guard makes the
  single-fire guarantee independent of how the platform/tzdata resolves the fold, and adds
  the missing fall-back regression test (America/New_York 2026-11-01, daily at 01:30 → next
  fire is 2026-11-02 01:30, ~25h later, never ~1h). See
  `.project/PHASE-M197-CADENCE-DST-FALLBACK-REPORT.md`.
- **Scheduler can't busy-loop on a corrupt interval (cadence review HIGH)** (M196) — the
  cadence engine's `advance` computed the next run as `now + IntervalSec` with no floor,
  and `OpenStore` loaded `schedules.json` without validation. `Add` rejects a
  sub-minimum interval, but a hand-edited or corrupt file with `interval_sec: 0` (or
  negative) would make the next run land on `now`/the past — so every ticker wake (10s)
  finds it due and fires a run, forever. `advance` now floors the interval to
  `MinInterval` (interval and window modes), and `OpenStore` repairs sub-minimum entries
  on load (durable + visible in `agt schedule list`). A bad value degrades to the slowest
  safe rate instead of hammering the daemon. See
  `.project/PHASE-M196-CADENCE-INTERVAL-FLOOR-REPORT.md`.
- **Deterministic longest-prefix price match (governor review HIGH)** (M192) — the
  fallback price-table prefix match returned the *first* key Go's randomized map
  iteration happened to hit, so a model id overlapping more than one key got a
  **nondeterministic** price (different across boots) and could bind to a less-specific,
  cheaper entry. The lookup now picks the **longest** matching key — deterministic (no
  two distinct equal-length keys can both prefix the same string) and always the most
  specific price. The live catalog path was already exact-match; this only affects the
  bootstrap fallback table. See
  `.project/PHASE-M192-GOVERNOR-PRICE-MATCH-REPORT.md`.
- **Budget cost math is overflow/negative-safe — a hostile usage report can't disable
  the spend ceiling (governor review CRITICAL)** (M191) — `costMicrocents` computed
  `tokens × price/MTok` with raw int64 arithmetic on provider-reported token counts.
  Those counts are untrusted (a `compat`/`ollama` endpoint can be operator-configured to
  an arbitrary URL, and a buggy/hostile provider can report any value). Two ways this
  broke the budget: a **negative** token count charged a negative cost — *crediting* the
  ledger; and an **absurd** count (e.g. 2e9 output tokens × 7.5e9/MTok) overflowed int64
  and **wrapped to a negative cost**. Either drove `spentToday` negative, after which the
  daily-ceiling check (`spent >= ceiling`) stayed false for the rest of the day — the
  budget gate silently disabled, agent free to overspend. Token counts are now clamped to
  ≥0 (ledger and audit event) and the cost math saturates to `MaxInt64` on overflow
  (fail-closed — an absurd report trips the ceiling immediately). See
  `.project/PHASE-M191-GOVERNOR-COST-OVERFLOW-REPORT.md`.
- **Provider response bound rolled out to all remaining provider families** (M190) —
  applied the M189 `httpread.All` cap to the non-streaming success reads and
  streaming-error reads of the anthropic, bedrock, cohere, google, ollama, and vertex
  providers (incl. vertex's OAuth token-exchange read), so no provider can OOM the
  daemon with an oversized response body. Each provider's existing tests confirm no
  regression on normal-size responses; a cross-dialect live test (anthropic) proves the
  bound fires there too. Every provider's response read is now bounded. See
  `.project/PHASE-M190-PROVIDER-RESPONSE-BOUND-ROLLOUT-REPORT.md`.
- **Provider HTTP response bodies are bounded (openai family) — no OOM from a hostile
  endpoint** (M189) — providers read the non-streaming response body with an unbounded
  `io.ReadAll(httpResp.Body)`. A provider endpoint can be operator-configured to an
  arbitrary URL (the openai-compat / custom-base-URL path that `compat` routes through
  the openai impl), or be buggy/MITM'd, so a multi-gigabyte or never-ending body OOMs
  the daemon. (The SSE streaming scanners were already bounded at 1 MiB/line.) Added a
  shared `httpread.All` helper (cap 64 MiB → `ErrResponseTooLarge`) and wired it into the
  openai provider's success and error read paths. The other six provider families roll
  out the same helper as follow-ups. See
  `.project/PHASE-M189-PROVIDER-RESPONSE-BOUND-REPORT.md`.
- **Control-plane request read is bounded — no pre-auth memory-exhaustion DoS** (M188) —
  `handleConn` read the request line with an unbounded `bufio.ReadBytes('\n')`, and that
  read happens BEFORE authentication (the token is inside the request). So any local
  client that can reach the loopback control port could stream bytes without a newline
  and drive the daemon to OOM — a pre-auth DoS, the same unbounded-read class as the
  plugin host (M177) and mcpbridge (M185), on the control socket. The request is now
  read through a bounded `readBoundedLine` capped at 16 MiB; an over-cap request gets a
  `request too large` error and the connection is dropped. See
  `.project/PHASE-M188-CONTROLPLANE-REQUEST-BOUND-REPORT.md`.
- **Control-plane primary-token check is now constant-time (security review)** (M187) —
  the primary (admin) token — the daemon's most privileged credential, which authorizes
  every command on every tenant — was compared with a plain `req.Token != s.Token()` in
  the auth gate (and `==` in `whoami`). Go's string comparison returns at the first
  differing byte, leaking the token byte-by-byte to anyone who can time the response.
  The *tenant* path was already hardened with `subtle.ConstantTimeCompare`; the
  more-privileged primary path was not. Both sites now route through a `tokenIsPrimary`
  helper using a constant-time comparison, with a blank-token guard (a blank presented
  or unset server token never authorizes — `ConstantTimeCompare("","")` returns 1).
  See `.project/PHASE-M187-CONTROLPLANE-CONSTANT-TIME-TOKEN-REPORT.md`.
- **Warden `nil` env no longer leaks the daemon environment into child processes
  (security review HIGH)** (M186) — `Spec.Env` documents "Nil = empty environment (most
  restrictive)", but the engine set `cmd.Env = spec.Env` directly, and Go's `os/exec`
  treats `cmd.Env == nil` as *inherit the parent's environment*. So a warden run with a
  nil Env actually ran the (untrusted) child with the **entire daemon environment** —
  API keys, tokens, `AWS_*`, etc. — the exact opposite of the documented default. This
  was live: pulse's probe runner (`observers.go`) passes `Env: nil` for
  operator-configured probe commands. `Run` now translates a nil Env to an explicit
  empty slice, so the documented default is also the safe one; a caller wanting
  inheritance must pass `os.Environ()` explicitly. See
  `.project/PHASE-M186-WARDEN-ENV-LEAK-REPORT.md`.
- **mcpbridge bounds reads from untrusted MCP servers — no OOM via a flooding server**
  (M185) — both MCP transports read newline-delimited frames from a peer the bridge
  doesn't control (a spawned MCP server's stdout; a remote SSE event-stream) with a
  plain `bufio` `ReadBytes`/`ReadString`, which grows without limit until a newline or
  EOF. A hostile or buggy server that writes bytes but never emits `\n` (or one huge
  line) OOM-kills the bridge process — the same class as the plugin-host C1 (M177), one
  trust boundary out. Reads now go through a bounded `readBoundedLine` capped at 16 MiB;
  the SSE transport additionally bounds the accumulated multi-line event `data` so a
  server can't grow it without a dispatching blank line. Over-cap frames tear the
  transport down (`onTransportDead`) instead of exhausting memory. See
  `.project/PHASE-M185-MCPBRIDGE-FRAME-BOUND-REPORT.md`.
- **Plugin teardown kills the whole process group — no orphaned grandchildren
  (security review MEDIUM)** (M184) — `Close` killed only the direct child, so any
  process a plugin forked (a shell wrapper, a Python subprocess) survived as an orphan
  after teardown — a resource leak and a persistence/escape flavour for an untrusted
  plugin (plugin-host review M4). The child is now placed in its own process group at
  spawn (`makeChild`), and the force-kill path signals the whole group. Platform-split:
  a real process group via `Setpgid` on Unix (the daemon's first-class target, killed
  with a negative-pid `SIGKILL`); on Windows it remains a direct-child kill (reliable
  whole-tree teardown there needs a Job Object, tracked as a follow-up). See
  `.project/PHASE-M184-PLUGIN-PROCESS-GROUP-REPORT.md`.
- **Plugin `Close` is now nil-safe on a half-started plugin (security review MEDIUM)**
  (M183) — `Close` called `p.cmd.Wait()`/`p.cmd.Process.Kill()` and wrote a shutdown
  request to `p.stdin` with no nil guard (plugin-host review M3). On a `Plugin` whose
  child never finished starting (no `cmd`/`stdin`), that nil-panics. The path is not
  currently reachable in production (`Spawn` returns before constructing the `Plugin`
  on a start failure), but it's a latent footgun for future refactors. `Close` now
  guards `stdin`, `cmd`, and `cmd.Process`, so it always marks the plugin dead and
  drains pending waiters without panicking. See
  `.project/PHASE-M183-PLUGIN-CLOSE-NIL-GUARD-REPORT.md`.
- **Plugin advertised-tool count capped — a malicious initialize can't blow up the
  registry (security review MEDIUM)** (M182) — the initialize result's `Tools` list was
  taken verbatim with no count limit (plugin-host review M2). The M177 frame bound caps
  the raw bytes, but ~1M tiny tool defs still fit in 16 MiB and each becomes a registry
  map entry + `remoteTool` wrapper at registration — a memory blow-up at spawn. `Spawn`
  and `Reload` now reject a plugin advertising more than `Config.MaxAdvertisedTools`
  (default 256) with `ErrTooManyTools`, before any registration. See
  `.project/PHASE-M182-PLUGIN-TOOL-CAP-REPORT.md`.
- **Plugin host-callback fan-out bounded — a callback flood can't exhaust the daemon
  (security review MEDIUM)** (M181) — the read loop spawned `go handleCallback(f)` for
  every plugin-initiated `host/invoke` frame with no concurrency limit (plugin-host
  review M1). A hostile plugin streaming callbacks as fast as the host reads them spawns
  an unbounded number of goroutines, each running a curated host tool with up to
  `InvokeTimeout` — goroutine/memory exhaustion plus amplification of whatever those
  tools touch. Dispatch now acquires a slot from a per-plugin counting semaphore
  (`Config.MaxConcurrentCallbacks`, default 16) non-blockingly: at the cap, the
  callback is rejected inline with `ErrTooManyCallbacks` rather than queued or spawned,
  so the read loop stays responsive and goroutines stay bounded under a flood. The
  semaphore is created once and persists across reloads. See
  `.project/PHASE-M181-PLUGIN-CALLBACK-LIMIT-REPORT.md`.
- **Plugin correlation ids stay monotonic across reload — no response confusion
  (security review HIGH)** (M180) — `respawn` reset the per-plugin id counter with
  `p.nextID.Store(0)` on every `Reload`, so post-reload requests reused the same
  `q-1`, `q-2`, … ids as pre-reload ones (plugin-host review H4). A late or crafted
  response carrying a reused id could then satisfy the wrong (new) request —
  response/result confusion. The counter is now left untouched across a reload so it
  climbs monotonically for the plugin's whole lifetime and an id is never reused. Also
  corrected the `Reload` doc comment, which wrongly claimed it "holds `p.mu` for the
  duration" (it relies on `Close` marking the old child dead before fresh state is
  installed). See `.project/PHASE-M180-PLUGIN-RELOAD-IDS-REPORT.md`.
- **Plugin response delivery is now race-safe — no send-on-closed-channel daemon crash
  (security review HIGH)** (M179) — the plugin host's read loop delivered each terminal
  response by sending on the caller's pending channel *outside* `p.mu`, while
  `markDead`/`Close` close those channels *under* `p.mu` (plugin-host review H3). A
  plugin that floods responses for in-flight ids while a `Reload`/`Close` runs could
  interleave a `close(ch)` between the read loop's unlocked lookup and its send — a
  send-on-closed-channel **panic** in the unrecovered read-loop goroutine, crashing the
  daemon. Delivery (lookup + send) now happens together under `p.mu` and the send is
  non-blocking, so it's mutually exclusive with teardown's close+delete and also drops a
  malicious duplicate terminal frame instead of blocking. A defensive `recover` in the
  read loop now turns any other unforeseen panic on the untrusted-input path into a
  plugin teardown rather than a daemon crash. See
  `.project/PHASE-M179-PLUGIN-DELIVER-RACE-REPORT.md`.
- **Plugin death-cause field made atomic — fixes a data race on plugin teardown
  (security review HIGH)** (M178) — `Plugin.deathErr` (the recorded cause of a plugin's
  death) was a plain `error` field written by the read-loop goroutine (`markDead`) and
  `Close` while being read by callers in `callWithProgress` and `remoteTool.Invoke`
  (plugin-host review H2). The `dead` flag is an `atomic.Bool`, but that does not
  publish a *separate* plain field under Go's memory model, so a plugin that crashes
  exactly as a caller enters `Invoke` is a genuine data race (a `go test -race`
  failure, and on some architectures a torn interface read). It's now an
  `atomic.Pointer[error]` accessed via `deathError()`/`setDeathErr()`. No behavior
  change; closes the race. See `.project/PHASE-M178-PLUGIN-DEATHERR-RACE-REPORT.md`.
- **Plugin stdout frame size bounded — a flooding plugin can't OOM the daemon
  (security review CRITICAL)** (M177) — the plugin host read one newline-delimited
  frame per loop off an untrusted child's stdout with `bufio.Reader.ReadBytes('\n')`,
  which grows its buffer without limit until a newline or EOF. A buggy or hostile
  plugin that writes bytes but never emits `\n` (or emits one pathologically large
  line) drove the host to allocate unbounded and OOM-killed the whole daemon — one
  plugin taking down every other plugin and the kernel, defeating the "kernel keeps
  running" guarantee (plugin-host review C1). Reads now go through a bounded
  `readFrame` (stdlib `ReadSlice` chunk-accumulation) capped at `Config.MaxFrameBytes`
  (default 16 MiB); a frame past the cap tears the plugin down (`markDead`, in-flight
  callers fail fast) instead of the daemon. The stderr path was already bounded (1 MiB
  per line); this closes the matching stdout hole. See
  `.project/PHASE-M177-PLUGIN-FRAME-BOUND-REPORT.md`.
- **Durable policy snapshot bound to the tamper-evident journal (security review
  HIGH)** (M176) — the durable-policy compaction snapshot (`edict_overlay_snapshot.json`,
  M95) was loaded as authoritative at boot with no integrity check, so an attacker who
  could write that file could downgrade trust levels or drop deny rules at the next
  restart — the snapshot, not the hash-chained journal, won (M173 review HIGH-1). Compact
  now records the snapshot's SHA-256 content hash in a new `policy.compacted` journal
  event, and boot trusts the snapshot ONLY when its hash matches the latest journaled
  value; otherwise it ignores the snapshot and folds the journal (the source of truth) in
  full. A tampered, corrupt, or pre-binding snapshot therefore cannot loosen policy —
  it can only ever be a no-op fallback to the full replay (which stays equivalent, M95).
  See `.project/PHASE-M176-EDICT-SNAPSHOT-INTEGRITY-REPORT.md`.
- **Wider hard-deny floor: raw block-device writes, wipefs, poweroff (security
  review follow-up)** (M175) — the default hard-deny floor missed several
  catastrophic shell operations even in plain form (M173 MEDIUM-2). Added rules for
  writing `dd` output to a raw block device (`of=/dev/sd|nvme|vd|xvd|mmcblk` — so a
  `dd of=/dev/sdb` with no `if=` is now caught), `wipefs`, and `poweroff`. The
  device rules deliberately exclude the safe pseudo-devices, so benign
  `dd of=/dev/null` / `echo … > /dev/null` stay allowed — important because the
  floor has no override. Composes with M173 (decoded/normalized matching). See
  `.project/PHASE-M175-EDICT-FLOOR-COVERAGE-REPORT.md`.
- **Strict `runtime[<digits>]` validation for removable policy rules (security
  review follow-up)** (M174) — `IsRuntimeRule` — the load-bearing
  "tighten-but-never-loosen" invariant that decides which hard-deny rules
  `agt edict deny rm` may remove — was a bare prefix check, so a crafted name like
  `runtime[evil` or `runtime[]` passed as "removable." It now validates the full
  canonical shape `runtime[<digits>]` that `AddHardDeny` actually mints, so a
  malformed/spoofed name can never masquerade as a runtime rule (defense-in-depth
  from the M173 Edict review). See `.project/PHASE-M174-EDICT-RUNTIME-RULE-REPORT.md`.
- **Policy hard-deny floor now matches the decoded action, not raw JSON (security
  review)** (M173) — an adversarial review of the Edict policy engine confirmed its
  core is sound (deny-before-ladder ordering, unknown-capability default-deny,
  fail-closed `AskPrompt`, conservative tool→capability classification, concurrency)
  but found a **Critical** bypass: the non-overridable hard-deny floor matched
  substrings against the *raw JSON tool argument*, so the model (the actor the floor
  exists to constrain) could evade `rm -rf /` etc. by padding whitespace
  (`{"command":"rm  -rf /"}`) or JSON-escaping a byte (`/` for `/`, `m`
  for `m`) — the decoded command was still destructive but the floor never fired.
  `Decide` now matches each rule against the raw input PLUS every JSON-decoded string
  value with whitespace collapsed, so escaped/padded variants normalize back to the
  banned form and are denied. (Substring matching remains best-effort against
  semantic rewrites like flag reordering; deeper findings — snapshot integrity, a
  stricter `IsRuntimeRule`, a fuller floor list — are tracked for follow-ups.) See
  `.project/PHASE-M173-EDICT-DENY-NORMALIZE-REPORT.md`.
- **Vault KDF hardened to genuine PBKDF2-SHA256 (crypto review)** (M172) — a crypto
  review of the at-rest credential vault confirmed the disaster properties are
  correctly prevented (no GCM key+nonce reuse — fresh salt→key AND fresh nonce per
  save; auth tag verified; no plaintext staged to disk; `crypto/rand` throughout;
  no algorithm-downgrade path) and found the custom KDF was a *sound* keyed
  HMAC-SHA256 chain but **not** the PBKDF2-equivalent the header claimed — it lacked
  PBKDF2's per-round XOR accumulation, a slight offline-cracking weakness. The KDF
  is now genuine PBKDF2-HMAC-SHA256 (stdlib-only; verified against published RFC
  test vectors incl. the 4096-iteration vector) under a new versioned id
  `pbkdf2-hmac-sha256`; vaults written with the legacy `hmac-sha256-iter` KDF still
  decrypt (dispatch on the envelope's `kdf`). Also raised the accepted
  iteration-count floor from 1000 → 100000 (200× below the 200k policy was too
  lax). See `.project/PHASE-M172-VAULT-PBKDF2-REPORT.md`.
- **Egress-guard SSRF range hardening — closed a NAT64 metadata-credential bypass
  (security review)** (M171) — an adversarial review of the netguard egress guard
  (which stops the prompt-injectable http/browser tools from reaching cloud
  metadata) confirmed its architecture is sound (it checks the *actually-dialed* IP
  via `Dialer.Control`, re-checks every redirect hop, and fails closed on parse
  errors) but found range-completeness gaps in `Allowed()`. **Critical:** a
  NAT64-wrapped metadata address `64:ff9b::a9fe:a9fe` (= `169.254.169.254`) and the
  IPv4-compatible `::a9fe:a9fe` fell through to *allowed*, reaching the metadata
  service on IPv6-only/NAT64 cloud hosts → IAM credential theft. Also missing:
  CGNAT `100.64.0.0/10`, the full `0.0.0.0/8` block (only exact `0.0.0.0` was
  caught), and multicast/broadcast. `Allowed` now collapses IPv6-embedded-IPv4
  forms (NAT64 + IPv4-compatible) to their v4 and classifies that, and blocks the
  added ranges. IPv4-mapped `::ffff:`, all RFC1918, link-local, loopback, and the
  opt-in flag scoping were confirmed already correct. See
  `.project/PHASE-M171-NETGUARD-SSRF-REPORT.md`.
- **Secret-redaction hardening — closed permanent-journal leak paths (security
  review)** (M170) — a security review of the redactor (the chokepoint before the
  append-only, hash-chained journal, where a miss is permanent and unscrubbable)
  found and fixed real leaks: (1) **HTML-escaping bypass (Critical)** — the bus
  marshaled payloads with `json.Marshal`, which escapes `&`/`<`/`>` to `&`
  etc., so the literal scrubber (searching for the *raw* value) missed any
  configured secret containing those characters (common in generated passwords /
  connection strings) and journaled it forever; the bus now marshals with
  `SetEscapeHTML(false)`. (2) **Base64/OAuth token char-class gap (High)** — the
  `sk-` and `bearer` patterns excluded `+` `/` `=`, so a standard-base64 token
  (e.g. a Google `ya29.…` access token) broke the match below its length floor and
  leaked *entirely*; the classes now include them. (3) **Missing patterns (High)** —
  added JWT (`eyJ….eyJ….…`) and GitHub fine-grained PAT (`github_pat_…`) detectors;
  widened the Google-key quantifier so a longer key isn't left with an unredacted
  tail. The review confirmed the redactor is concurrency-safe, mutation-safe, has no
  journal-write bypass, and no ReDoS. (A separate `[]byte`-payload base64 gap is
  tracked for a structural-redaction follow-up.) See
  `.project/PHASE-M170-REDACTION-HARDENING-REPORT.md`.
- **Agent loop panic firewall — a misbehaving provider/tool can no longer crash the
  daemon (code review)** (M168) — an independent review of the agent tool-loop (the
  hottest path, just touched by M166) found that a provider returning `(nil, nil)` —
  a contract violation an out-of-process, third-party plugin can easily commit on an
  unexpected empty upstream body — dereferenced nil and **panicked the run goroutine,
  which has no `recover()` → the whole daemon crashed, killing every concurrent
  run**. Now `Run` guards a nil response (clean error) and wraps the loop in a panic
  firewall: any panic from a provider or tool is recovered into `ErrPanic`, fails
  just that one run, and is journaled as `task.failed(reason=panic)` (the message
  preserved) — blast radius is one run, not the process. The review also confirmed
  the M166 cost accounting, the once-only `task.failed` emitter, per-tool-timeout
  context handling, and streaming/non-streaming `resp` consistency are all correct.
  See `.project/PHASE-M168-AGENT-PANIC-FIREWALL-REPORT.md`.
- **Per-run override args are type-validated, not silently mis-handled (code review)**
  (M161) — an independent review of the accreted run-submission path
  (`handleRun`, M148–M160) found that every per-run override
  (`model`/`system`/`timeout`/`tools`/`images`/`dry_run`) used a comma-ok type
  assertion that collapsed "absent" and "present-but-wrong-type" into the same
  zero value — turning a client-side typo into silent wrong behavior. The two
  dangerous cases: a `dry_run` sent as the string `"true"` failed the `bool`
  assertion and **executed the run for real** (spending tokens) the operator meant
  only to preview; a `tools` sent as a bare string (not an array) silently scoped
  the run to **zero tools**. New typed accessors (`argString`/`argBool`/
  `argStringList`) distinguish absent from wrong-typed and return a usage error for
  the latter; the override block now parses each arg **once** (reused by both the
  real run and the dry-run plan, so the plan can't drift), and the `--system`
  override is stored trimmed. See `.project/PHASE-M161-RUN-ARG-VALIDATION-REPORT.md`.
- **Journal hardening: torn-line tolerance + rotation resilience (code review)**
  (M157) — a review of the event-sourcing foundation found two real bugs, now fixed:
  - **Torn final-line read (Critical)**: `Range` / `Tail` / `Verify` / recovery used
    `bufio.Scanner`'s default split, which yields an unterminated trailing line as a
    token — so an in-flight append (a concurrent reader, since the journal is on the
    hot path) or a crash mid-write made them fail to JSON-decode a partial record and
    error out a *healthy* journal. Every committed line ends in `\n`, so the scanner
    now discards a trailing line that lacks one (only ever an in-flight/torn write,
    never a committed record); a corrupt middle line still surfaces. This also makes
    crash recovery boot cleanly past a half-written final event instead of refusing
    to start.
  - **Rotation wedge (High)**: a failed segment-open during rotation closed the old
    segment first, stranding `curFile` on a closed handle and wedging all further
    appends, while the just-written event was already durable (in-memory/on-disk
    divergence). Rotation now opens the next segment BEFORE swapping (atomic; a
    failure leaves the current segment live and usable), and a rotation failure no
    longer fails an already-committed append. See
    `.project/PHASE-M157-JOURNAL-HARDENING-REPORT.md`.
- **Governor concurrency hardening (code review)** (M155) — a focused review of the
  Governor (routing + spend + fallback) found two real concurrency bugs, now fixed:
  - **Data race (Critical)**: `routeChain` and `Providers` read the `primary`/
    `fallback` routing slices with no lock, while `Replace` (the credential-rotation
    / hot-reload path) rebuilds them under the lock — a concurrent reload during an
    in-flight `Complete` was an unsynchronized slice read/write that could mis-route
    or nil-deref-panic the daemon. The readers now snapshot under the mutex, and
    `Replace` builds fresh slices instead of truncating the live backing array.
  - **Bus pointer tear (High)**: `SetBus` wrote `cfg.Bus` while `publish` read it
    lock-free on the hot path; a `WithLimits` sibling re-pointing its bus mid-serve
    could tear the pointer. The bus is now latched in an `atomic.Pointer`.
  The review also confirmed the most cost-sensitive logic is correct (fallback
  classification treats cancel/timeout/budget-exhaustion as terminal; daily/per-task
  spend counters and the UTC rollover are consistently locked). The daily ceiling is
  documented as the soft cap it is (concurrent in-flight calls can overshoot by a
  bounded amount). A concurrency stress test guards it. See
  `.project/PHASE-M155-GOVERNOR-HARDENING-REPORT.md`.
- **`agt journal tail` no longer scans the whole journal** (M147) — the tail handler
  forward-walked every segment of the (append-only, full-retention) journal just to
  keep the last N events by seq — O(total), growing with the journal forever. A new
  `Journal.Tail(n)` reads segments newest→oldest and stops as soon as it has N
  events, so the cost is ≈ the last segment (where N is small), not the entire
  history. Output is identical (last N in seq order); concurrency matches `Range`
  (no lock, reads durable lines alongside `Append`). See
  `.project/PHASE-M147-JOURNAL-TAIL-REPORT.md`.
- **Channel-arc hardening (code review)** (M145) — a focused quality pass over the
  M138–M144 channel arc fixed several real issues found in review:
  - **Boot-time data race**: the `notify` tool was written into the kernel's live
    tool map AFTER the HTTP servers / channels began listening — a request in that
    window could trigger a concurrent map read+write (fatal panic). The tool is now
    registered before the kernel starts and Bind-wired (mutex-guarded) once channels
    exist; the map is never written while the agent loop reads it.
  - **Cross-user context bleed (privacy)**: `ConversationHistory` folded by
    `(kind, channel_id)` only, so in a SHARED Slack/Discord channel one user's
    messages leaked into another user's prompt. It now isolates per sender (a
    sender's own inbound + the agent replies that share their run correlation).
  - **Slack replay**: a captured signed event could be replayed (without the retry
    header) within the 5-minute signature window and reprocessed. Added a bounded
    seen-set keyed on the immutable channel+ts for exactly-once processing.
  - **Slack send false-success**: a malformed/`ok:false` HTTP-200 body was treated as
    delivered (and journaled `channel.outbound`). Now decode failure / `ok:false` is
    a real error and is not journaled as sent.
  - **`notify` partial failure**: a multi-recipient send that partially failed
    returned success; it now flags `IsError` and names the failed recipients.
  - **UTF-8-safe transcript clipping**: `clip` truncated on a byte boundary, which
    could split a multibyte rune (emoji/CJK); now rune-aware.
  - **Discord prompt selection**: the slash-command prompt is now taken from the
    option explicitly named `prompt` (and only STRING options), not "first string
    wins", so a reordered/extra option can't feed the agent the wrong field.
  - **Slowloris**: the Slack/Discord webhook servers now set `ReadHeaderTimeout`.
  See `.project/PHASE-M145-CHANNEL-ARC-HARDENING-REPORT.md`.
- **Tenant self-observability authorization** (M128) — a tenant token was wrongly
  denied read-only access to its OWN isolated subsystems. Many tenant-routed
  observability handlers (memory / world / approvals / plan / provider-routing /
  schedule-firing / warden logs+stats) fold the tenant's own journal via
  `kernelFor(tenantOf(req))` but had been left out of the `tenantTokenAllows`
  allowlist — an inconsistency vs. runs/tool/edict/webhook, which were allowed.
  Audited each handler to confirm it reads only the tenant's kernel (no `s.k`
  leak), then granted the 13 read-only commands. Cross-tenant `tenant_stats` and
  durable-policy compaction stay primary-only; tests lock both directions. See
  `.project/PHASE-M128-TENANT-OBSERVABILITY-AUTH-REPORT.md`.

### Added
- **Complete the `agt config show` env inventory** (M127) — the config view's env
  presence map had silently rotted: 55 of the ~78 `AGEZT_*` vars the daemon reads
  (webhooks, multitenancy, schedules, telegram, sub-agents, peers, pulse, redaction,
  timeouts, …) were missing, so an operator pasting `agt config show` into a bug
  report saw an incomplete picture. Restored the full inventory and added a
  self-enforcing guard test that scans `cmd/agezt` and fails if any read var is
  absent, so it can't rot again. See `.project/PHASE-M127-CONFIG-ENV-INVENTORY-REPORT.md`.
- **`agt tenant stats`** (M126) — a cross-tenant usage view: per-tenant run count /
  completed / failed / active / spend / last activity, plus grand totals, so the
  primary operator can see which tenant is busy, spending, or failing (multitenancy
  could create/list/remove tenants but offered no usage view). Folds each tenant's
  own journal with the same `collectRuns` as `agt runs`; a tenant that was closed is
  re-released afterward, so the read-only query leaves residency unchanged. Primary
  token only. See `.project/PHASE-M126-TENANT-STATS-REPORT.md`.
- **Cost-band filter for `agt runs list`** (M125) — `--min-cost <usd>` /
  `--max-cost <usd>` keep only runs whose spend falls in the band, so an operator
  can find "which runs blew the budget?" / "the expensive runs to optimize" after
  seeing the M124 by-model breakdown. New `usdToMicrocents` parser (the inverse of
  `fmtUSD`). See `.project/PHASE-M125-RUNS-COST-FILTER-REPORT.md`.
- **Per-model cost attribution in `agt runs stats`** (M124) — a `by model` block
  (and `by_model` in `--json`) breaks down run count + spend per model, sorted by
  spend, so "where is my money going across the provider mix?" is answered in the
  fleet-level view. Builds on the M123 per-run model fold; no new event or command.
  See `.project/PHASE-M124-RUNS-STATS-BY-MODEL-REPORT.md`.
- **Model-aware `agt runs`** (M123) — each run now folds and surfaces the model it
  was routed to (first-wins, from its `budget.consumed` events), shown inline in
  `agt runs list` and as a `model` field in `--json`, plus a `--model <substr>`
  filter (case-insensitive). Answers "which runs used claude-opus?" / "did routing
  go as expected?" in a multi-provider deployment — the natural companion to the
  M99 provider-fallback stats. See `.project/PHASE-M123-RUNS-MODEL-REPORT.md`.
- **`agt webhook test`** (M122) — a daemon-free probe that POSTs one synthetic
  `webhook.test` event to a sink using the byte-identical body, headers, and HMAC
  signature a real delivery sends, so an operator can confirm a sink is reachable
  and accepts the format before relying on it — no more waiting for a real event
  to fire. With no `<url>` it probes every sink in `AGEZT_WEBHOOKS`; exit 0 = all
  2xx, 3 = at least one failed. The active companion to the M121 doctor check and
  M112 `webhook stats`. New `webhook.Probe`. See
  `.project/PHASE-M122-WEBHOOK-TEST-REPORT.md`.
- **`agt doctor` webhook-health check** (M121) — the go-to diagnostic now WARNs when
  outbound webhook deliveries are failing, naming the worst sink and pointing at
  `agt webhook log --failed`. A notification sink that silently 5xx's or times out
  is the classic "I never got paged" outage; this folds the M112 webhook-delivery
  data into the first-look diagnostic so broken notifications surface proactively.
  See `.project/PHASE-M121-DOCTOR-WEBHOOKS-REPORT.md`.
- **`agt schedule test`** (M120) — a dry-run that previews a schedule's next N fire
  times, so an operator can validate a complex daily/windowed/timezone/weekday
  cadence before relying on it (parity with `agt edict test`). New
  `cadence.Entry.Forecast`. See `.project/PHASE-M120-SCHEDULE-TEST-REPORT.md`.
- **File tool glob** (M119) — the `file` tool gains a `glob` op that finds files by
  name pattern (`*.go`, `*_test.go`) across the workspace tree, complementing `list`
  (one dir) and `search` (content). See `.project/PHASE-M119-FILE-GLOB-REPORT.md`.
- **`agt skill diff`** (M118) — diff a skill's body against its lineage parent (or
  another skill), with a line-level LCS diff, so an operator can see how a skill
  evolved instead of eyeballing two bodies. See
  `.project/PHASE-M118-SKILL-DIFF-REPORT.md`.
- **File tool line-range read** (M117) — `read` accepts `start_line`/`end_line` to
  page a region of a file (under a `[lines X-Y]` header), so an agent can reach
  content past the 256 KiB truncation point and read around a `search` hit. See
  `.project/PHASE-M117-FILE-LINE-RANGE-REPORT.md`.
- **Agent loop guard** (M116) — the agent loop now refuses to re-execute the SAME
  `(tool, input)` call more than `MaxIdenticalToolCalls` times (default 5) in one
  run, feeding the model a nudge instead of repeating a stuck/failing/expensive
  call up to MaxIter. Distinct inputs are never capped. See
  `.project/PHASE-M116-LOOP-GUARD-REPORT.md`.
- **File tool regex search** (M115) — the `file` tool's `search` op gains an opt-in
  `regex: true` mode (RE2), so an agent can grep for code patterns, not just literal
  substrings. Default literal behaviour is unchanged. See
  `.project/PHASE-M115-FILE-REGEX-SEARCH-REPORT.md`.
- **File tool partial edit** (M114) — the `file` tool gains a `replace` op
  (`find`/`replacement`, unique-match by default or `all=true`), so an agent can
  edit a file surgically instead of rewriting the whole thing — cheaper in context
  and safer. Governed as a write (CapFileWrite). See
  `.project/PHASE-M114-FILE-REPLACE-REPORT.md`.
- **`agt backup` / `agt restore`** (SPEC-09 §8, M113) — one-command, secret-free
  node migration: a portable `.tar.gz` of the home (journal + catalog) that
  restores on a fresh host and boots. Secrets are excluded by construction (only
  journal/ + catalog/ are captured) and restore is path-traversal-safe. See
  `.project/PHASE-M113-BACKUP-RESTORE-REPORT.md`.
- **Webhook delivery observability** (SPEC-08 / P7-API-02, M112) — `agt webhook log`
  (with `--failed`) and `agt webhook stats` surface outbound webhook deliveries and
  failures (per URL), so a silently-failing notification sink is visible instead of
  a "I never got paged" outage. See
  `.project/PHASE-M112-WEBHOOK-OBSERVABILITY-REPORT.md`.
- **`agt provider cost`** (SPEC-08, M111) — standalone model-price lookup: a model's
  per-Mtok input/output price and an optional cost estimate for a given token count
  (`--input-tokens` / `--output-tokens`), reusing the catalog. See
  `.project/PHASE-M111-PROVIDER-COST-REPORT.md`.
- **Catalog-freshness check in `agt doctor`** (SPEC-08, M110) — the diagnostic now
  WARNs when the API model catalog hasn't been synced in over 21 days, since stale
  pricing silently skews cost estimates and budget enforcement. See
  `.project/PHASE-M110-CATALOG-FRESHNESS-REPORT.md`.
- **Egress-block audit** (SPEC-06 / M16, M109) — the egress guard now journals a
  `netguard.blocked` event whenever the http/browser tools are refused a dial to an
  internal/metadata address, and `agt netguard log` surfaces the audit trail — so
  an attempted SSRF / metadata read (a prompt-injection / exfiltration signal) is
  recorded, not lost. See `.project/PHASE-M109-NETGUARD-AUDIT-REPORT.md`.
- **Effective routing in `agt config show`** (SPEC-08, M108) — the config snapshot
  now surfaces the PARSED routing tables (`AGEZT_TASK_ROUTES` / `_ROUTE_REQUIRES` /
  `_MODEL_OVERRIDES`), so an operator can confirm a rule loaded instead of reading
  the boot log. New read-only governor introspection views. See
  `.project/PHASE-M108-CONFIG-ROUTING-REPORT.md`.
- **`agt budget check`** (SPEC-08, M107) — pre-flight remaining daily-spend
  headroom before submitting a run (global + optional `--task-type` cap, whichever
  binds), with exit 3 when exhausted for CI gating. Reuses the existing budget
  snapshot. See `.project/PHASE-M107-BUDGET-CHECK-REPORT.md`.
- **Rate-limit observability + primary cap** (SPEC-08, M106) — `AGEZT_RATE_PER_MIN`
  rate-limits the primary governor (previously only tenants could be capped), and
  `agt ratelimit log` / `agt ratelimit stats` surface throttle events (per tenant),
  turning silent throttling into a first-class SRE signal. See
  `.project/PHASE-M106-RATELIMIT-OBSERVABILITY-REPORT.md`.
- **`agt netguard test`** (SPEC-06 / M16, M105) — preview the egress guard:
  resolve a host and report which resolved IPs the http/browser tools may reach,
  catching SSRF / DNS-rebinding traps (a public name pointing at 169.254.169.254
  or a private address) before any tool dials. Exit 3 when blocked. See
  `.project/PHASE-M105-NETGUARD-TEST-REPORT.md`.
- **`agt redact test`** (SPEC-06, M104) — verify the live secret redactor would
  scrub a candidate string before it could reach the journal (built-in pattern
  categories + configured literals), without leaking which literal matched. Exit
  3 when it would NOT redact, for scriptable secret-hygiene checks. See
  `.project/PHASE-M104-REDACT-TEST-REPORT.md`.
- **Anti-truncation for journal bundles** (SPEC-09 §8, M103) — `agt journal verify
  --bundle` and `agt journal import` now confirm a bundle REACHES the chain head
  its manifest attests (`last.Hash == head_hash`), closing a tail-truncation /
  omission gap: a dropped tail previously still chain-verified as a valid prefix.
  See `.project/PHASE-M103-BUNDLE-COMPLETENESS-REPORT.md`.
- **Journal import / restore** (SPEC-09 §8, M102) — `agt journal import <bundle>
  [--home <dir>]` seeds a fresh host from an `agt journal export` bundle for
  disaster-recovery / migration: it verifies the bundle, refuses to clobber a
  non-empty journal, writes events verbatim (so the restored chain still
  verifies), and re-opens the journal to confirm it boots. New strict
  `journal.Restore` primitive. See `.project/PHASE-M102-JOURNAL-IMPORT-REPORT.md`.
- **Verifiable journal export** (SPEC-09 §8, M101) — `agt journal export [--since
  <dur>] [--out <file>]` writes a hash-chained bundle bound to the chain head, and
  `agt journal verify --bundle <file>` re-verifies it OFFLINE (recompute every
  BLAKE3 hash + check prev-hash continuity), making archives tamper-evident. Adds
  the byte-preserving `Client.CallRaw` so exported events survive the wire
  verifiably. See `.project/PHASE-M101-JOURNAL-EXPORT-REPORT.md`.
- **Tunable HITL timeout + doctor surfaces unanswered approvals** (SPEC-08, M100) —
  `AGEZT_APPROVAL_TIMEOUT` right-sizes how long a prompt-mode approval waits before
  auto-deny (was hardcoded 5m), and `agt doctor` now WARNs when approvals have been
  timing out (operator not answering / window too short → runs silently stall).
  Third doctor single-pane check after M98/M99. See
  `.project/PHASE-M100-HITL-TIMEOUT-REPORT.md`.
- **`agt doctor` provider-health check** (SPEC-08, M99) — the diagnostic now WARNs
  when the daemon has been silently falling back from its primary model provider
  to a secondary, and the hint names the worst-offending provider so the operator
  knows which key/outage to chase. Mirrors the M98 sandbox check. See
  `.project/PHASE-M99-DOCTOR-PROVIDER-REPORT.md`.
- **`agt doctor` sandbox check** (SPEC-08, M98) — the go-to diagnostic now WARNs
  when the OS warden has been silently downgrading isolation (or hitting resource
  limits), turning the M97 sandbox stats into an actionable health signal. See
  `.project/PHASE-M98-DOCTOR-SANDBOX-REPORT.md`.
- **`agt warden stats`** (SPEC-08, M97) — sandbox posture aggregate: executions,
  downgrade count + rate, timeouts, limit breaches, by-effective-profile.
  Completes the warden `log`/`stats` pair and the security-triad stats
  (edict/approvals/warden). See `.project/PHASE-M97-WARDEN-STATS-REPORT.md`.
- **`agt warden log`** (SPEC-08, M96) — the OS-sandbox execution audit: folds
  `warden.executed` / `profile_downgraded` / `limit_exceeded` into one timeline
  (what ran, under which profile, downgrades, limit breaches; `--issues` isolates
  the latter). Completes the security-observability triad with `agt edict log`
  (policy) and `agt approvals log` (HITL). See `.project/PHASE-M96-WARDEN-LOG-REPORT.md`.
- **Durable-policy compaction** (SPEC-08, M95) — `agt edict compact` snapshots
  the net policy overlay (minimal change list + journal seq) so boot
  (`AGEZT_EDICT_DURABLE=on`) replays `{snapshot + only later changes}` instead of
  the whole `policy.changed` history. Fallback-safe (absent/corrupt → full fold);
  the journal stays the immutable source of truth. See
  `.project/PHASE-M95-EDICT-COMPACT-REPORT.md`.
- **`agt edict overlay`** (SPEC-08, M94) — surfaces the NET durable policy
  overlay: every runtime `policy.changed` folded via the same
  `ProjectPolicyChanges` the daemon replays at boot, so an operator sees what
  runtime policy is actually in effect (show = base config, log = raw events,
  overlay = net result). See `.project/PHASE-M94-EDICT-OVERLAY-REPORT.md`.
- **Vision image input** (SPEC-14, M93) — a vision-capable model now actually
  *receives* attachments: `agent.Message` carries `Images`, the agent loop puts
  them on the initial user message (stamping the count on `task.received` for
  provenance), threaded from the control plane via `runtime.WithImages` after the
  M91 gate passes. Demoable offline via `AGEZT_DEMO_VISION=1` (a vision-capable
  mock that echoes the received count). See `.project/PHASE-M93-VISION-INPUT-REPORT.md`.
- **`agt provider rejections`** (SPEC-14, M92) — a capability-gating audit:
  folds `capability.rejected` (M25 tool_call / M91 vision) + `capability.rerouted`
  (M40 down-route) into one timeline of what the capability gates did. Completes
  the M23/M25/M40/M91 enforcement story. See
  `.project/PHASE-M92-PROVIDER-REJECTIONS-REPORT.md`.
- **Vision capability gate** (SPEC-14, M91) — `agt run --image <path>` attaches
  images to a run; the daemon rejects them pre-flight (before any provider call)
  unless the active model is confirmed vision-capable, journaling
  `capability.rejected{capability:vision}`. Confirmed-or-reject (stricter than the
  M25 tool gate, since an image on a non-vision model is a guaranteed failure).
  Enforced at the submission boundary — no agent/message-type change. See
  `.project/PHASE-M91-VISION-GATE-REPORT.md`.
- **`agt schedule fires --intent <substr>`** (SPEC-08, M80) — the last list
  surface gains the intent substring filter, completing symmetry with
  `runs list --intent` (M77). Composes with `--id`/`--status`/`--since`. See
  `.project/PHASE-M80-SCHEDULE-FIRES-INTENT-REPORT.md`.
- **`agt runs stats --intent <substr>`** (SPEC-08, M78) — scopes the run-health
  aggregate to runs whose intent matches, so an operator can ask "the success
  rate / p95 / spend of my deploy runs?". Same matcher as `runs list --intent`
  (M77); composes with `--since`. See `.project/PHASE-M78-RUNS-STATS-INTENT-REPORT.md`.
- **`agt runs list --intent <substr>`** (SPEC-08, M77) — a case-insensitive
  substring filter on a run's intent, applied server-side before the limit, so an
  operator can find "that deploy run" without grepping. Composes with
  `--status`/`--failed`. See `.project/PHASE-M77-RUNS-INTENT-FILTER-REPORT.md`.
- **`agt edict stats` tool & capability scope** (Edict observability, M76) —
  `--tool <name>` / `--capability <cap>` (alias `--cap`) scope the aggregate, so
  the denial rate + counts reflect one tool/capability. Completes the symmetry
  with `edict log`'s M74 filters. See
  `.project/PHASE-M76-EDICT-STATS-SCOPE-REPORT.md`.
- **`agt approvals stats`** (SPEC-08, M88) — the HITL approval aggregate:
  granted / denied / timeout / pending, a grant rate over resolved requests, and
  a denied-by-capability breakdown. Completes the approvals `log`/`stats` pair;
  the human analogue of `agt edict stats`. See
  `.project/PHASE-M88-APPROVALS-STATS-REPORT.md`.
- **`agt provider stats`** (SPEC-08, M90) — the provider-reliability aggregate:
  routed calls, fallback count + rate, calls-by-primary, and
  fallbacks-by-failed-provider. Completes the provider `log`/`stats` pair. See
  `.project/PHASE-M90-PROVIDER-STATS-REPORT.md`.
- **`agt provider log`** (SPEC-08, M89) — a provider-routing activity timeline:
  folds `routing.decision` + `provider.fallback` to show which provider handled
  calls and when the primary fell back (`--fallbacks` isolates failures). The
  provider-layer analogue of `agt tool log`. See
  `.project/PHASE-M89-PROVIDER-LOG-REPORT.md`.
- **`agt approvals log`** (SPEC-08, M87) — the HITL approval audit: folds
  `approval.requested` joined with the terminal granted/denied/timeout into a
  timeline of what was asked, how it resolved, and who decided. The human
  analogue of `agt edict log`; `--denied` + `--since`. See
  `.project/PHASE-M87-APPROVALS-LOG-REPORT.md`.
- **`agt world log`** (SPEC-08, M86) — a timeline of world-model operations
  (entity/relation upserts and forgets): what the agent observed, reinforced, and
  forgot. The world-model analogue of `agt memory log`; `--kind` filter +
  `--since` window. See `.project/PHASE-M86-WORLD-LOG-REPORT.md`.
- **`agt memory log`** (SPEC-08, M85) — a timeline of memory operations
  (`memory.written`/`forgotten`/`superseded`): what the agent learned, forgot,
  and replaced, newest-first, with an `--op` filter and `--since` window.
  `memory list` shows current state; this shows its provenance. See
  `.project/PHASE-M85-MEMORY-LOG-REPORT.md`.
- **`agt plan stats`** (SPEC-02/SPEC-08, M84) — the plan analogue of
  `runs stats`: aggregates plan executions into total / completed / failed /
  running, a success rate, and a duration distribution. Completes the plan
  `history`/`stats` pair. See `.project/PHASE-M84-PLAN-STATS-REPORT.md`.
- **`agt plan history`** (SPEC-02/SPEC-08, M83) — the plan analogue of
  `runs list`: folds `plan.started` joined with `plan.completed`/`plan.failed`
  into a newest-first list of plan executions (name, status, nodes, duration),
  with a `--status`/`--failed` filter. Drill in with `agt runs show <corr>`
  (M82). See `.project/PHASE-M83-PLAN-HISTORY-REPORT.md`.
- **Plan-execution runs in `agt runs show`** (SPEC-02/SPEC-08, M82) — a plan run
  (`agt plan <file>`) is now reachable and legible: `runs show <plan-corr>`
  synthesises a header from the plan lifecycle and renders `plan: <name>` +
  `node <id> [<kind>] started|completed|FAILED` instead of erroring "no run with
  correlation". See `.project/PHASE-M82-PLAN-ARC-REPORT.md`.
- **Task-arc summary footer** (SPEC-08, M81) — `agt runs show` ends with a
  one-line `summary: N round(s), M tool call(s) [(K error(s))], $<spend>,
  <duration>`, so a long arc reads at a glance without scrolling back to tally.
  See `.project/PHASE-M81-ARC-FOOTER-REPORT.md`.
- **Error-message breakdown in `agt tool stats`** (SPEC-08, M79) — an
  `errors by message` block (most-frequent first) buckets failed tool calls by
  their message, so an operator sees WHAT is failing (denied / not-available /
  timeout), not just how many. The tool analogue of `runs stats`'
  `failed_by_reason`. See `.project/PHASE-M79-TOOL-ERROR-BREAKDOWN-REPORT.md`.
- **Per-tool latency in `agt tool stats`** (SPEC-08, M75) — the by-tool
  breakdown now carries a per-tool mean latency (`shell 3 call(s), 0 error(s),
  avg 14ms`), so an operator can see WHICH tool is slow, not just that calls are
  slow overall. See `.project/PHASE-M75-PERTOOL-LATENCY-REPORT.md`.
- **`agt edict log` tool & capability filters** (Edict observability, M74) —
  `--tool <name>` and `--capability <cap>` (alias `--cap`) scope the policy-
  decision log, the drill-down from `agt edict stats`' denied-by-capability
  breakdown. Compose with `--denied` / `--since`. See
  `.project/PHASE-M74-EDICT-LOG-FILTERS-REPORT.md`.
- **`agt tool log --slow <dur>` latency filter** (SPEC-08, M73) — the
  performance-hunting counterpart to `--errors`: keeps only tool calls whose
  invoked→result latency is at/above the floor, applied server-side before the
  limit. Completes the tool-log filter family (errors / slow / tool / since). See
  `.project/PHASE-M73-TOOL-SLOW-REPORT.md`.
- **Per-tool latency inline in the task arc** (SPEC-08, M72) — `agt runs show`
  now renders each `tool.result` with its invoked→result wall-clock
  (`tool.result : ok (18ms) …`), joined by `call_id` from the arc's own event
  timestamps — the same span `agt tool log` reports, on the run-debugging
  surface. See `.project/PHASE-M72-ARC-LATENCY-REPORT.md`.
- **Tool-call latency in `agt tool log` & `tool stats`** (SPEC-08, M71) — each
  log row gains a latency column and `tool stats` gains an avg/min/p50/p95/max
  `latency` block, computed from the journal's `tool.invoked`→`tool.result`
  timestamp span (joined by `call_id`) — a pure read-side fold, no agent or
  event-schema change. See `.project/PHASE-M71-TOOL-LATENCY-REPORT.md`.
- **Failure reason in the task arc** (SPEC-08, M70) — `agt runs show` now renders
  a failed run's header as `status: failed (<reason>) after <duration>` and marks
  the `task.failed` event inline, instead of a bare `status: failed` that dropped
  the why. The reason comes from the same fold `agt runs list --failed` uses. See
  `.project/PHASE-M70-ARC-FAILURE-REPORT.md`.
- **Per-round budget in the task arc** (SPEC-08, M69) — `agt runs show` renders
  `budget.consumed` as `budget: <model> $<cost> (in=N, out=M tokens)` instead of
  a generic event line, so the arc shows WHERE a run's spend accrued round by
  round (complementing the header's M50 total). See
  `.project/PHASE-M69-ARC-BUDGET-REPORT.md`.
- **`agt tool stats` — tool-invocation aggregate** (SPEC-08, M67) — folds the
  journal's `tool.result` events into total / errored / error-rate plus a
  per-tool calls+errors breakdown. The execution-dashboard analogue of
  `agt edict stats`; completes the tool `list`/`log`/`stats` triad. `--tool`,
  `--since`, `--json`, tenant-scoped. See `.project/PHASE-M67-TOOL-STATS-REPORT.md`.
- **`agt tool log` — tool-invocation audit** (SPEC-08, M66) — a read-only view of
  the journal's `tool.invoked` + `tool.result` events: what the agent actually
  ran and how each call turned out (`<time> ok|ERROR <tool>  <output-preview>`).
  The execution analogue of `agt edict log` (which audits the *gating* of those
  same calls). Filters: `--errors`, `--tool <name>`, `--since <dur>`; `--json`;
  tenant-scoped. See `.project/PHASE-M66-TOOL-LOG-REPORT.md`.
- **`--since` windowing for `agt edict log` & `agt schedule fires`** (SPEC-08, M65) —
  both per-event logs gain `--since <dur>` (the time filter their `stats`
  counterparts already had), applied server-side during the journal walk via a
  shared `sinceCutoff` helper. See `.project/PHASE-M65-WINDOWED-LOGS-REPORT.md`.
- **`agt edict stats` — policy-decision aggregate** (Edict observability, M64) — the
  security-dashboard analogue of `agt runs stats`: total / allowed / denied /
  hard-denied, denial rate, and a denied-by-capability breakdown, over the journal's
  `policy.decision` events (`--since` windowed, tenant-scoped). Completes the
  show(rules)/log(decisions)/stats(aggregate) triad. See
  `.project/PHASE-M64-EDICT-STATS-REPORT.md`.
- **`agt edict log` — policy-decision audit** (Edict observability, M63) — a
  read-only view of the journal's `policy.decision` events (every tool-call gating):
  `<time> allow|DENY|DENY(hard) <capability> <tool> (reason)`. `agt edict show` lists
  the RULES; `edict log [N] [--denied]` lists the DECISIONS they produced.
  `handleEdictLog` folds the events newest-first (tenant-scoped, allowlisted). See
  `.project/PHASE-M63-EDICT-LOG-REPORT.md`.
- **`agt whoami`** (SPEC-14 multi-tenancy, M62) — reports the authenticated
  principal: `primary (admin token …)` or `tenant "acme" (own token …)`. M38/M39
  added tenant tokens but a client couldn't confirm which identity it authenticates
  as; `handleWhoami` derives it from `req.Token` vs the primary token (no new auth
  state). `CmdWhoami` is tenant-allowlisted. See `.project/PHASE-M62-WHOAMI-REPORT.md`.
- **Status filter on `agt runs list` & `agt schedule fires`** (SPEC-08, M61) — both
  gain `--status <s>` and `--failed` (shorthand) to filter by run/firing outcome
  (completed|failed|running|abandoned), applied server-side BEFORE the limit so
  `list 5 --failed` returns 5 failed runs. A shared `runEntryStatus` helper keeps
  list/fires/filter in agreement. See `.project/PHASE-M61-STATUS-FILTER-REPORT.md`.
- **`agt runs stats` spend percentiles** (SPEC-12 multi-agent, M60) — the spend
  aggregate now includes a per-run cost distribution (`spend dist`: avg/min/p50/p95/max
  over priced runs), mirroring the duration block and reusing the same nearest-rank
  helper. So an operator sees not just total spend (M47) but how it's distributed.
  See `.project/PHASE-M60-RUNS-STATS-SPEND-PERCENTILES-REPORT.md`.
- **`agt runs list` answer preview** (SPEC-08 × SPEC-12, M59) — `agt runs list` now
  shows each run's one-line answer preview beneath its intent (`answer : "…"`),
  rendering the `answer_preview` M52 already put on every row. Pure render, quiet
  when absent. See `.project/PHASE-M59-RUNS-LIST-ANSWER-PREVIEW-REPORT.md`.
- **Boot-banner the delegation caps** (SPEC-12 multi-agent, M58) — the daemon boot
  banner now shows the active delegation ceilings: `delegation : depth≤1, fan-out
  ≤3, spend $0.5000` (or `off` / `unbounded`), from the same `k.SubAgentLimits()`
  source as `agt status` (M49). Visible at startup, not only on demand. See
  `.project/PHASE-M58-DELEGATION-BOOT-BANNER-REPORT.md`.
- **`agt schedule stats` — autonomy aggregate** (SPEC-08 × cadence, M57) — the
  autonomy analogue of `agt runs stats`: `handleScheduleStats` folds `schedule.fired`
  events, joins each with its run outcome (`collectRuns`), and reports total firings,
  counts by outcome, success rate, total spend, and distinct schedules fired.
  `agt schedule stats [--id <sched>] [--since <dur>] [--json]`, reusing the
  `agt runs` renderers. See `.project/PHASE-M57-SCHEDULE-STATS-REPORT.md`.
- **Per-schedule last outcome in `agt schedule list`** (SPEC-08 × cadence, M56) — each
  schedule row now shows how it last went: `… last: completed 06-01 12:16` (or
  `failed (timeout) …`). `latestFiringBySchedule` folds the journal into a
  schedule_id → newest-firing map (joined with the run outcome via the shared
  `collectRuns` fold, M54/M55); `handleScheduleList` annotates each row with
  `last_status`/`last_reason`/`last_fired_unix_ms`. Pure derivation, no new event.
  See `.project/PHASE-M56-SCHEDULE-LAST-OUTCOME-REPORT.md`.
- **Link firings to their schedule** (SPEC-08 journal × cadence, M55) — the
  `schedule.fired` event now carries `schedule_id`, threaded from the cadence
  Engine's `RunFunc` (widened to `func(ctx, id, intent, model)`) through the
  daemon's firing closure. `agt schedule fires` exposes `schedule_id` per row and
  gains `--id <sched>` to filter the history to one schedule. Pre-M55 firings list
  with an empty id (backward-compatible). The M54 follow-on: a firing now knows
  which schedule produced it. (Also re-aligned the daemon's `kernelruntime.Config`
  literal — a stale gofmt alignment left by M48's long key; whitespace only.) See
  `.project/PHASE-M55-SCHEDULE-FIRING-LINK-REPORT.md`.
- **`agt schedule fires` — autonomy firing history** (SPEC-08 journal × cadence, M54)
  — the first operator view of what scheduled work has *done*, not just what's
  scheduled. `agt schedule list` shows the schedules; `agt schedule fires [N]` (alias
  `history`) shows each firing and its outcome: `<time>  completed (22ms, $X)
  <correlation>  "<intent>"`. The new `handleScheduleFires` walks the journal for
  `schedule.fired` events and joins each with its run outcome from the shared
  `collectRuns` fold (status/duration/spend M47/answer-preview M52) — so a firing
  never disagrees with `agt runs show <correlation>`. The autonomy analogue of
  `agt runs list` (newest-first, `[N]` limit, `--json`, tenant-scoped); manual runs
  are excluded. See `.project/PHASE-M54-SCHEDULE-FIRES-REPORT.md`.
- **Tenant-scoped `agt why`** (SPEC-08 journal × SPEC-14 multi-tenancy, M53) — the
  event-chain tracer is now routed per-tenant via `kernelFor(tenantOf(req))` (the
  M39 seam): `agt why <id> --tenant <id>` traces a tenant's own journal, and the
  primary scope no longer reads across the isolation boundary. `CmdWhy` joins the
  tenant-token allowlist so a tenant can trace its own events with its own token.
  Closes the last non-tenant-aware control surface — isolation is now complete
  across execution (M14), control (M38), and observability (M39 runs + M53 why).
  Proven live: a primary event resolves under the primary scope but "not found"
  under `--tenant acme`. See `.project/PHASE-M53-TENANT-SCOPED-WHY-REPORT.md`.
- **Sub-agent answer preview on the delegation arc** (SPEC-12 multi-agent, M52) —
  `agt runs show <lead>` now appends a one-line excerpt of each sub-agent's answer
  to its `↳` outcome line: `↳ completed (1 iters, 42ms, $0.0021): "kernel/ holds
  event, journal…"`. `collectRuns` folds the M51 `task.completed` answer into
  `runEntry.AnswerPreview` (whitespace collapsed to one line, truncated to 80 runes);
  `handleRunsList` exposes `answer_preview` per row; `renderTaskArc` shows it when
  present. Pure derivation over M51 — no new event or round-trip. Completes the
  delegation story (link → task → outcome → cost → result): an operator sees what a
  delegation said without drilling into the child. See
  `.project/PHASE-M52-DELEGATION-ANSWER-PREVIEW-REPORT.md`.
- **Journal the run answer** (SPEC-08 journal × SPEC-12, M51) — the agent loop now
  records the final assistant text on `task.completed` (`answer`, alongside
  `iters`/`chars`/`stopped`), so `agt runs show`'s "final answer:" section displays
  what a run actually produced — it was empty since written, because the body was
  never journaled (the renderers read a `llm.response.message.content` the daemon
  doesn't populate). The bus's M15 redactor scrubs the answer for free; the stored
  copy is rune-capped (8192) with a `…[truncated]` marker so the hash-chained,
  replayed journal can't be bloated by a pathological output (the full answer is
  still returned to the caller; `chars` records the true length). The renderer
  prefers the journaled answer and falls back to the old path for pre-M51 runs. See
  `.project/PHASE-M51-JOURNAL-RUN-ANSWER-REPORT.md`.
- **Per-run spend in `agt runs list` / `show`** (SPEC-12 multi-agent, M50) — the
  per-run views now show cost, completing the spend story M47 started in aggregate:
  `agt runs list` appends `spend: $0.0021` to a run's row, `agt runs show` adds a
  `spend      : $0.0084` header line (the lead's own spend), and each delegation's
  `↳` outcome line gains its cost — `↳ completed (1 iters, 42ms, $0.0021)`. Pure
  rendering over the M47 `runEntry.SpentMicrocents` fold — one new `spent_mc` JSON
  field server-side, the rest client formatting (reusing the `agt budget` `fmtUSD`).
  Every surface stays quiet at $0 (free/local model or offline mock). See
  `.project/PHASE-M50-PER-RUN-SPEND-REPORT.md`.
- **Delegation ceilings in `agt status`** (SPEC-12 multi-agent, M49) — the status
  round-trip now reports the active delegation governance: `delegation: depth≤1,
  fan-out ≤3, spend ≤$0.5000` (or `unbounded` for an unset cap, `off` when the
  delegate tool is disabled). The M46–M48 caps were silent until a delegation
  tripped one; this makes them legible at a glance. `Kernel.SubAgentLimits()`
  reports the *effective* ceilings (depth defaults to 1 when enabled and unset,
  matching enforcement); `handleStatus` adds a `delegation` object (jq-friendly
  scalars) and `cmdStatus` renders the line, reusing the `agt budget` `fmtUSD`
  formatter. Read-only. See
  `.project/PHASE-M49-DELEGATION-CEILINGS-STATUS-REPORT.md`.
- **Per-delegation spend cap** (SPEC-12 multi-agent, M48) — `AGEZT_SUBAGENT_SPEND_CAP=<usd>`
  caps the total spend a single run's sub-agents may collectively consume; once a
  lead's delegations have spent past it, the next `delegate` is refused with
  `max sub-agent spend $X.XXXX reached` (a tool error the lead adapts to, mirroring
  the M46 fan-out guard). The tally is a **stateless** transitive-descendant sum
  over the journal's M47 `budget.consumed` events — durable by the time each child
  returns, so it's race-free with no in-memory accounting; scanned only when the
  cap is enabled. Closes the count→cost→cap governance loop (M46 count, M47 cost,
  M48 cap). `0`/absent = unbounded. Proven live: 3 attempts under a $0.003 cap → 2
  ran ($0.0042 sub-agent spend), 3rd refused. See
  `.project/PHASE-M48-DELEGATION-SPEND-CAP-REPORT.md`.
- **Per-delegation spend attribution** (SPEC-12 multi-agent, M47) — the governor's
  `budget.consumed` event now carries the spending run's correlation (envelope +
  payload), threaded in via a new Governor-only `CompletionRequest.CorrelationID`
  hint (opaque to providers, mirroring `TaskType`). `collectRuns` folds each run's
  `cost_microcents` into `runEntry.SpentMicrocents` (existing entries only — an
  orphan spend event can't conjure a phantom run), and `agt runs stats` renders
  `spend: $0.0126 (delegated: $0.0042)` — the window's total spend and the share
  attributable to sub-agent runs. Pure journal fold over data the governor already
  emits; no new endpoint or projection. The mock gains `WithUsage` so the offline
  demo exercises the spend path. Pairs M46's delegation-count cap with cost
  *visibility* (a cost cap is the next step). Proven live with
  `AGEZT_DEMO_DELEGATE=3` + `AGEZT_SUBAGENT_FANOUT=2`. See
  `.project/PHASE-M47-DELEGATION-SPEND-REPORT.md`.
- **Sub-agent fan-out bound** (SPEC-12 multi-agent, M46) — `AGEZT_SUBAGENT_FANOUT=<n>`
  caps how many sub-agents a single run may spawn at its level (depth caps nesting;
  this caps breadth, which was previously unbounded — only `SubAgentMaxDepth`
  existed). The Nth+1 `delegate` call is refused with `max sub-agent fan-out N
  reached`, surfaced as a tool error the lead adapts to (mirroring the depth guard)
  and journaled via the existing `tool.result`; the M45 metric correctly excludes
  the refusal. Tallied in-memory per spawning correlation (O(1), no journal scan),
  released when the spawning run ends. `0`/absent = unbounded (default preserved).
  The first *governance* lever on the multi-agent axis, atop M41–M45's
  observability. Proven live: 3 attempts under a cap of 2 → 2 spawned, 3rd refused.
  See `.project/PHASE-M46-FANOUT-BOUND-REPORT.md`.
- **Delegation metrics in `agt runs stats`** (SPEC-12 multi-agent, M45) — the
  stats aggregate now surfaces the *scale* of multi-agent fan-out the other lines
  can't: `delegations: 3 (from 2 run(s), max fan-out 2)` — total sub-agent runs,
  the number of distinct leads that delegated, and the widest single fan-out.
  Folded server-side over the same windowed run set (so `--since` applies) by
  counting runs that carry a `parent_correlation` (M41); the line is omitted when
  no delegation occurred, so single-agent operators see no noise. A sub-agent run
  was previously indistinguishable from a top-level one in the totals — this makes
  it countable without a new endpoint. Proven live with `AGEZT_DEMO_DELEGATE=1`.
  See `.project/PHASE-M45-DELEGATION-METRICS-REPORT.md`.
- **Per-delegation outcome on the lead's arc** (SPEC-12 multi-agent, M44) — in
  `agt runs show <lead>`, each `delegated → <child>` line is now followed by the
  sub-agent's terminal outcome inline: `↳ completed (1 iters, 1ms)` (or
  `failed (timeout)` etc.), so the lead's arc answers "did the delegation
  succeed?" without a second `agt runs show <child>`. `cmdRunsShow` already
  fetches the full runs list, so it builds a correlation→summary map for free and
  passes the outcomes to `renderTaskArc` — no extra round-trips, no server change.
  (The sub-agent's answer *text* is not journaled — the schema records
  `text_chars`/`usage`, not the message body — so the outcome is status/iters/
  duration; the child's events remain one `runs show <child>` away.) Proven live: a
  lead's arc showed its sub-agent's `↳ completed` outcome. See
  `.project/PHASE-M44-DELEGATION-OUTCOME-REPORT.md`.
- **`agt runs list --tree`** (SPEC-12 multi-agent, M43) — renders the delegation
  hierarchy: each lead run with its sub-agent runs nested beneath it (two spaces of
  indent per level, depth-first), instead of the flat newest-first list. Pure
  client-side rendering over the `parent_correlation` field M41 already added — no
  server change. A sub-agent whose lead isn't in the fetched window renders as a
  root so nothing is hidden; the flat default and `--json` are unchanged. Completes
  the delegation-observability trio (M41 link, M42 backlink, M43 tree). Proven
  live: a lead's `--tree` nested its sub-agent under it. See
  `.project/PHASE-M43-RUNS-TREE-REPORT.md`.
- **`agt why` sub-agent → parent backlink** (SPEC-12 multi-agent, M42) — closes the
  child→parent discovery gap M41 left open. A `subagent.spawned` event lives under
  the *parent's* correlation, so parent→child was walkable but from a sub-agent's
  own chain there was no way back to its lead. New `Kernel.ParentOf(childCorr)`
  scans the journal for the spawn that names a correlation as its child and returns
  the lead; `handleWhy` includes `correlation` + `parent_correlation` in its
  result; `agt why <event>` prints `spawned by <lead>  (try: agt runs show <lead>)`
  for a sub-agent chain (and `--json` carries both fields). The delegation tree is
  now walkable in BOTH directions (M41 parent→child, M42 child→parent). Proven
  live: `agt why` on a sub-agent event reported its lead. See
  `.project/PHASE-M42-WHY-PARENT-BACKLINK-REPORT.md`.
- **Sub-agent delegation links in `agt runs`** (SPEC-12 multi-agent, M41) — opens
  the multi-agent orchestration axis. A lead agent's `delegate` tool spawns a
  sub-agent that runs under its own correlation, so parent and child already
  appeared as separate `agt runs` rows — but *unlinked*, with no way to see the
  delegation without reading the journal. Now `collectRuns` also folds the
  `subagent.spawned` event (which carries `child_correlation` + `parent`) to set a
  `parent_correlation` on the child's run entry. `agt runs list` marks a sub-agent
  row `↳ sub-agent of <lead>`; `agt runs show <lead>` renders the delegation as
  `delegated → <child> (task: …)` instead of a generic event line; the link is in
  the `--json` output too. A small `AGEZT_DEMO_DELEGATE=1` escape hatch (mirroring
  `AGEZT_DEMO_FAIL_PRIMARY`) scripts the offline mock to delegate once, so the
  whole path is network-free-demoable. Proven live: a lead delegated to a
  sub-agent and both the list link and the show callout rendered. See
  `.project/PHASE-M41-SUBAGENT-RUN-LINKS-REPORT.md`.
- **Cross-provider down-routing** (`AGEZT_MODEL_DOWNROUTE_CROSS=on`, SPEC-15, M40)
  — extends M37: when a tools-bearing request hits a tool-incapable model whose own
  provider has **no** tool-capable sibling, the substitute search widens to a
  tool-capable model on a *different* registered + credentialed provider (instead
  of falling through to a reject). Same-provider is still preferred (the remap stays
  on the already-serving provider when it can); only when there's no in-provider
  option does it cross. Crucially the eligible set is the providers the governor
  *actually registered* (tracked during registration), so a remap target is always
  one the router can reach via `applyModelRoute`/`Serves` — never a phantom. The
  largest-context capable model wins, tie-broken by id for determinism. Implies
  down-routing; boot banner shows `tool-downrouting(cross)`. New
  `catalog.ToolCapableAlternativeAmong(model, providerEligible)`; `ToolCapable
  Alternative` refactored to delegate (same-provider = eligible-self). Proven live:
  a request to a provider with only an incapable model was rerouted across to a
  capable model on another provider. See
  `.project/PHASE-M40-CROSS-PROVIDER-DOWNROUTE-REPORT.md`.
- **Tenant-scoped run observability** (`agt runs list/stats --tenant <id>`,
  SPEC-14, M39) — a natural M38 follow-on. `runs list` and `runs stats` now walk
  the *target tenant's* journal (via `kernelFor`) instead of always the primary's,
  so a tenant — authenticating with its own token — can observe its **own** run
  health (counts, success rate, durations, failure-reason breakdown, windowed),
  fully isolated from the primary and other tenants. The shared `collectRuns` fold
  is parameterized by kernel; both commands gain `--tenant <id>`; both are added to
  the M38 tenant-token allowlist (they read the tenant's own journal now, not the
  primary's). The primary/empty-tenant path is byte-for-byte unchanged. Proven
  live: a tenant saw only its own run via its token while the primary saw only its
  own, and a tenant token with no tenant arg was denied. See
  `.project/PHASE-M39-TENANT-RUN-OBSERVABILITY-REPORT.md`.
- **Per-tenant authenticated control-plane access** (SPEC-14, M38) — completes the
  M14 tenant-isolation story on the control side. Tenant tokens already existed
  (minted by `agt tenant create`) but the control plane only accepted the *primary*
  token, so they were useless for auth. Now a request that presents a **tenant's
  own token** (plus that tenant's id) authenticates *as* that tenant — a tenant can
  manage its own runs and Edict policy without the primary token. A tenant
  principal is strictly confined: a deny-by-default allowlist of tenant-routed
  commands (`run`, `runs cancel`, all `edict` subcommands), with the tenant arg
  pinned to the authorized tenant, so it cannot touch another tenant, the
  tenant registry, or daemon-global state (halt/shutdown/pulse) — those stay
  primary-only. The primary token retains full access, unchanged. Token presented
  via `AGEZT_TOKEN=<tok>` (overrides the on-disk primary token); authorization uses
  the registry's existing constant-time `Authorize`. Proven live: a tenant token
  managed its own edict, was denied another tenant, primary-only commands, and the
  registry, while the primary token kept full reach. See
  `.project/PHASE-M38-PER-TENANT-AUTH-REPORT.md`.
- **Capability down-routing** (`AGEZT_MODEL_DOWNROUTE=on`, SPEC-15, M37) — completes
  the M23–M27 capability arc: instead of merely *rejecting* a tools-bearing request
  to a tool-incapable model (M25 strict gate), the Governor now **remaps** it to a
  tool-capable sibling in the same provider and proceeds. The substitute is the
  same-provider model with the largest context window (tie-broken by id, so it's
  deterministic) — staying in-provider keeps the remap on an already-credentialed
  provider. Runs pre-flight, before the strict gate, and journals a
  `capability.rerouted` event (`{from_model, to_model}`) so `agt why` shows why the
  served model differs from the requested one. Composes with strict mode
  (reroute-if-possible, else reject) but works independently. Off by default; new
  `catalog.ToolCapableAlternative` + governor `DownRouteToolModels` /
  `ToolCapableAlternative`. Proven live: a tools request to a tool-incapable model
  was rerouted to its capable sibling instead of rejected. See
  `.project/PHASE-M37-CAPABILITY-DOWNROUTE-REPORT.md`.
- **Failure-reason breakdown in `agt runs stats`** (SPEC-08, M36) — the `failed`
  count is now split by *why* runs fail: `failed : 3 (timeout=2, canceled=1)`. The
  M30 reason tag (`error` / `max_iters` / `canceled` / `timeout`, plus `unknown`
  for a failure with no recorded reason) is aggregated into a `failed_by_reason`
  map on `CmdRunsStats` and rendered inline, stably ordered. Turns "10% of runs
  fail" into "…and they're all timeouts" — the actionable form. Purely additive;
  the map is empty (jq-safe) when there are no failures. Proven live: two timed-out
  runs and one cancelled run rendered as `failed : 3 (timeout=2, canceled=1)`. See
  `.project/PHASE-M36-FAILED-BY-REASON-REPORT.md`.
- **Cancel-on-disconnect** (`AGEZT_CANCEL_ON_DISCONNECT=on`, SPEC-08, M35) — when
  enabled, a streaming `agt run` whose client connection drops (Ctrl-C or a killed
  client) cancels its run server-side instead of letting it churn on headless. The
  run handler watches the otherwise-idle client connection in a goroutine; a read
  unblocks only when the connection closes, at which point the run is cancelled via
  the same `Kernel.CancelRun` path as `agt runs cancel` (M32) — so it terminates as
  `failed (canceled)`. Off by default, so a backgrounded `agt run &` (whose client
  stays alive) is unaffected — only a genuinely-gone client triggers it. When the
  run finishes normally the watcher's read returns and the cancel is a harmless
  no-op. Boot banner shows `cancel-on-disc. : on/disabled`. Proven live: killing a
  hung run's client terminated it as `failed (canceled)`. See
  `.project/PHASE-M35-CANCEL-ON-DISCONNECT-REPORT.md`.
- **Per-tool-call timeout** (`AGEZT_TOOL_TIMEOUT=<dur>`, SPEC-08, M34) — bound
  each individual tool invocation's wall-clock without bounding the whole run.
  Where the per-run cap (M31) *fails the run* on overrun, a per-tool overrun fails
  only that one call: the loop hands the model an `IsError` result ("tool X
  exceeded its … timeout") and the run continues so the model can adapt or try
  another approach. A genuine run-level cancel/timeout (operator halt, M32 cancel,
  or the M31 per-run deadline) still propagates and fails the run — the loop keys
  off the *parent* run context to tell the two apart, and off the tool call
  context's own deadline state (not the returned error string) so a tool that
  wraps its error opaquely is still classified cleanly. Plumbed through
  `LoopConfig.ToolTimeout` → `runtime.Config.ToolTimeout` (applies to sub-agents
  too); off by default; boot banner shows `tool timeout : …`. Proven live: with a
  tiny budget a tool call timed out and the run still completed. See
  `.project/PHASE-M34-TOOL-TIMEOUT-REPORT.md`.
- **Windowed run stats** (`agt runs stats --since <dur>`, SPEC-08, M33) —
  restrict the run-health aggregation to runs that *started* within the last
  `<dur>` (e.g. `--since 1h`, `--since 30m`), instead of all-time. Answers "how
  have runs done in the last hour" — a view made meaningful by the
  failed/timeout/canceled terminal terms (M30–M32) that now populate the success
  rate. The server computes the cutoff against its own clock (the same clock that
  stamps event timestamps) and filters on each run's start time; runs with no
  recorded start are excluded from a window. `CmdRunsStats` gains an optional
  `since_ms` arg and echoes `window_ms` (0 = all-time); the header reads `run
  stats (over N run(s), last 1h)`. Both `--since 1h` and `--since=1h` forms work;
  a malformed/zero duration is a usage error. Proven live: three runs counted
  all-time and under `--since 1h`, then aged out under `--since 2s`. See
  `.project/PHASE-M33-RUNS-STATS-SINCE-REPORT.md`.
- **Targeted run cancellation** (`agt runs cancel <correlation>`, SPEC-08, M32) —
  cancel a single in-flight run without halting the whole daemon. Until now the
  only way to stop a stuck run was `agt halt`, which cancels **every** run and
  blocks new ones until `agt resume` — far too blunt for a multi-run daemon.
  `Kernel.CancelRun(corr)` looks up the run's own `CancelFunc` in the live-run
  registry and cancels just it; the agent loop returns `context.Canceled`, which
  the M30 terminal emitter records as `task.failed(reason=canceled)` — so the run
  shows as `failed (canceled)` in `agt runs` while the kernel stays un-halted and
  every other run keeps going. New `CmdCancelRun` control-plane verb (tenant-
  routable) + `agt runs cancel` (exit 0 when a live run matched, 1 when none did,
  for scripting). Proven live: a hung run was cancelled individually, terminated
  as `failed (canceled)`, and the daemon kept serving. See
  `.project/PHASE-M32-RUN-CANCEL-REPORT.md`.
- **Per-run wall-clock timeout** (SPEC-08, M31) — `AGEZT_RUN_TIMEOUT=<duration>`
  (e.g. `90s`, `5m`) arms an optional per-run deadline so a slow provider or a
  blocking tool can't hang a run forever *within* a live session (M28 only
  covers across-restart). Off by default — only `MaxIter` and an explicit halt
  bound a run. When armed, `RunWith` wraps the run context with the deadline; an
  overrun cancels with `context.DeadlineExceeded`, which the M30 terminal emitter
  classifies as `task.failed(reason=timeout)` — so `agt runs` shows
  `failed (timeout)` and `agt runs stats` counts it against the success rate.
  Crucially distinguished from an operator halt: the deadline cancels with
  `DeadlineExceeded` while `Halt()` cancels with `Canceled` (→ `reason=canceled`),
  so the two never blur. A malformed duration is a hard startup error; the boot
  banner shows `run timeout : <d> per run …` / `disabled`. Proven live: a run
  pointed at a black-hole endpoint was cut off at exactly its 2s budget and
  rendered as `failed (timeout)` end-to-end. See
  `.project/PHASE-M31-RUN-TIMEOUT-REPORT.md`.
- **`task.failed` terminal event** (SPEC-08, M30) — a run that started
  (`task.received`) but errored out instead of completing used to emit no
  terminal event, so `agt runs` couldn't tell a real failure apart from a true
  orphan (M28) — both showed as `running` until the next boot abandoned them.
  The agent loop now emits a `task.failed` event on any error return after
  `task.received` (best-effort, via a deferred terminal emitter), carrying
  `{error, reason}` where `reason ∈ {error, max_iters, canceled, timeout}`.
  `agt runs` renders `status="failed (reason)"` with a real duration; `agt runs
  stats` (M29) counts `failed` as a first-class non-success terminal and folds
  it into the success rate (`completed / (completed + failed + abandoned)`); and
  the M28 boot reconciliation treats `task.failed` as terminal, so a failed run
  is never double-marked `abandoned`. Status precedence is
  `completed > failed > abandoned > running`. Proven live with the strict
  capability gate (a tools request to a tool-incapable model is rejected
  terminally → `task.failed(reason=error)`), end-to-end through `agt runs
  list`/`stats`. See `.project/PHASE-M30-TASK-FAILED-REPORT.md`.
- **`agt runs stats`** (SPEC-08, M29) — a pure, read-only aggregation over the
  whole journal that answers "how are my agent runs doing overall?" in one
  command. Folds every `task.received` / `task.completed` / `task.abandoned`
  event (sharing the exact `collectRuns` fold with `agt runs list`, so the two
  can never disagree about a run's status) into: total / completed / running /
  abandoned counts, a success rate (`completed / (completed + abandoned)` — runs
  still in-flight don't count against it), mean iterations, and a duration
  distribution over **completed runs only** (avg / min / p50 / p95 / max).
  Percentiles use the nearest-rank method so every reported value is a real
  observed duration, not an interpolated phantom. `--json` for pipelines; an
  empty journal renders cleanly (`total=0`, zero-valued duration block) rather
  than crashing. New `CmdRunsStats` control-plane verb + `handleRunsStats` +
  `cmdRunsStats` renderer. Proven live with the mock provider. See
  `.project/PHASE-M29-RUNS-STATS-REPORT.md`.
- **Orphaned-run recovery on boot** (SPEC-08, M28) — a run that was in-flight
  when a prior daemon exited (a crash, or a run cancelled/errored without a
  completion event) used to sit in `agt runs` as `running` forever. The daemon
  now reconciles them at startup: it scans the journal for runs with a
  `task.received` but no `task.completed`, and emits a `task.abandoned` event for
  each — so `agt runs` shows `abandoned` instead of a phantom `running`, and the
  recovery is itself auditable. Idempotent (a run already carrying
  `task.abandoned` is skipped, so repeated restarts don't re-abandon), runs
  before any new run is dispatched, and reports the count on the boot banner
  (`recovery : N run(s) abandoned …` / `clean`). Proven live across three boots:
  a hung run is left incomplete, the next boot abandons it (banner + journaled
  event + `agt runs` status), and a third boot is clean. See
  `.project/PHASE-M28-ORPHAN-RUN-RECOVERY-REPORT.md`.
- **Capability matrix** (`agt provider check --caps --all`, SPEC-15, M27) —
  completes the M23 capability view: a one-row-per-provider table comparing every
  supported catalog provider's selected model by tool-use, vision, reasoning, and
  context window, each marked ✓ (agent-ready) or ⚠ (a capability gap), with a
  trailing "N providers, M agent-ready" summary. Network-free and credential-free
  like single `--caps`; `--json` emits the array. Lets an operator pick a model
  by capability at a glance instead of probing one at a time. Proven live: a
  three-provider catalog renders the matrix with the right ✓/⚠ marks and skips
  unsupported families. See `.project/PHASE-M27-CAPABILITY-MATRIX-REPORT.md`.
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
- **Task-arc rendering told the truth** (SPEC-08, M68) — `agt runs show` read two
  journal fields the agent loop never writes: `tool.result` checked `is_error`
  (journaled as `error`), so every tool call showed `ok` even on failure; and
  `policy.decision` checked a non-existent `decision` string, leaving every
  policy line's verdict blank. Both now read the real fields (`error`; `allow` /
  `hard_denied` / `reason`), and the arc additionally shows compact tool
  input/output excerpts. See `.project/PHASE-M68-ARC-HONESTY-REPORT.md`.
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
