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
- **Per-agent memory scope — shared brain, private notes.** The `memory` tool now
  takes an optional `scope` (e.g. an agent's role): a remembered note tagged with
  a scope stays private to it, and recall surfaces shared records (no scope) PLUS
  the requested scope's own — never another scope's private notes. Memory is
  shared by default, so agents keep using one common brain; the scope is the
  per-agent layer the vision called for ("each agent has SOME of its own data but
  shares the main memory"). The daemon's automatic pre-run recall uses the empty
  (shared-only) view, so a run can never inherit an unrelated agent's private
  notes. Implemented as a tag (`scope=<name>`) over the existing store — no new
  store, fully backward-compatible — via a new `Manager.RecallScoped`; recalled
  private records are annotated `(scope: …)` and the Memory view already shows the
  tag as a chip. Verified live with the real provider: an agent stored a note
  scoped to `researcher`; recall under `researcher` returned it, recall under
  `writer` returned nothing; the visibility rules are covered by unit tests. (M652)
- **"Do-it-for-sure" — run, verify, retry until actually done (`agt run --assure`).**
  A new reliability loop: instead of a single pass, an assured run executes the
  task, asks a strict verifier whether it was FULLY accomplished (a plan or a
  promise doesn't count), and if not, retries with the verifier's gap fed back —
  up to N attempts (default 3, max 10), stopping the moment it's judged complete.
  This is the "when I write something, definitely do it, and repeat as needed"
  primitive. The loop core is a pure, fully-tested `kernel/assure` package (run +
  verify closures); the kernel supplies a governed `RunWith` and a provider-backed
  completion judge. Every attempt reuses one correlation id (sequential, never
  overlapping) so the whole objective streams and journals as one run, and each
  completion check emits a `assure.verdict` event ({complete, gap}) so `agt why`
  shows exactly why it retried or stopped. Exposed as `agt run --assure[=<n>]`;
  available to any run surface through the `assure` arg on the run command.
  Verified live with the real provider: an assured run completed and journaled a
  `complete:true` verdict; the retry/exhaust/clamp/parse paths are covered by the
  pure package's unit tests. (M651)
- **Continuous-loop heartbeat — see the living organism breathe.** Every cadence
  entry now carries a `Fires` counter, incremented once per completed firing at
  the universal `CompleteFiring` hook (so it never double-counts an in-flight
  cycle). For a continuous loop this is the number of cycles it has lived through;
  for a recurring schedule, how many times it has run. The Schedules cockpit
  surfaces it: continuous loops get an `∞` cadence mark and a pulsing-heart
  `alive — N cycles` badge, and any schedule that has fired shows a quiet run
  count. The agent's own `schedule` `op=list` reports `fires` too. This makes the
  "never-tiring" loop concrete and observable instead of an invisible background
  goroutine. Verified live with the real provider: a 5s-cooldown continuous loop
  climbed to 5 cycles in ~35s, the cockpit rendered the live heartbeat with zero
  console errors, and the loop was removable from the cockpit/CLI. (M650)
- **Agent Board view — watch agents talk to each other.** The Web UI now has a
  Board view that surfaces the shared inter-agent message board (M647): every
  message agents post to coordinate — handoffs, notes left for the next cycle,
  peer chatter — newest-first, with from-role badges, timestamps, and per-topic
  filter chips. A new read-only control-plane command (`board_read`) and
  `/api/board` route back it; it polls live. To make this readable without the
  kernel importing a plugin, the board store was extracted from the `board` tool
  into a `kernel/board` package (mirroring kernel/cadence←schedule and
  kernel/standing←standing): the tool now wraps it, and the control plane Opens it
  fresh per request (atomic writes mean a fresh Open sees the latest state).
  Verified live with the real provider: two runs posted to topic `deploy` as
  `ci-watcher` and `release-bot`; the Board view rendered both with zero console
  errors. (M649)
- **`skill` tool — agents modify themselves.** A new in-process tool lets an agent
  author, inspect, promote, and retire its OWN reusable procedures through Forge —
  the same journaled, reversible skill state machine the `agt skill` CLI drives.
  `op=learn` distills a procedure the agent just worked out into a named,
  content-addressed skill (a draft); `op=list` / `op=show` inspect them; `op=promote`
  advances a skill toward the active retrieval pool (draft→shadow→active) so future
  runs pull it automatically; `op=retire` quarantines one that's gone wrong. This is
  the self-modification primitive — agents get better over time by capturing what
  they learn — kept honest: every transition is a hash-chained event carrying the
  authoring run's correlation, a new skill starts as a draft OUTSIDE the pool, and
  any change is undoable (`agt skill revert`). Gated by a new `skill` capability
  (ask-first — a genuine self-modification grant). Verified live with the real
  provider: a run authored a `summarize-pr` skill and promoted it draft→shadow; the
  journaled `skill.created` event carried that run's `correlation_id`, and the
  operator CLI (`agt skill list`) saw it.
- **Tools can attribute their side effects to the run that caused them.** The agent
  loop now wraps each tool invocation's context with the run's correlation id
  (`agent.WithCorrelation` / `CorrelationFromContext`), so a tool that mutates
  kernel state — the `skill` tool authoring a procedure — journals under the
  originating run without threading the id through its input schema. (M648)
- **`board` tool — agents talk to each other.** A new in-process tool gives every
  agent on the daemon a shared, persistent, topic-addressed message board:
  `op=post` leaves a message on a topic (with an optional `from` role), `op=read`
  returns recent messages newest-first (optionally filtered to one topic), and
  `op=topics` lists the active topics. This is the inter-agent communication
  primitive — one run can hand off findings, leave a note for its next cycle, or
  coordinate with a peer, and any other agent (lead, sub-agent, scheduled,
  standing-order, or continuous loop) reads it back. It is the shared common
  ground that complements memory (durable facts) and world (entities): the board
  carries shared *messages*. One JSON store under `<base>/board.json`, mutex-
  guarded, atomic writes, capped at 1000 messages (oldest dropped). Gated by a new
  `board` capability (Allow by default — a local shared note-store like memory).
  Verified live with the real provider: one run posted an API base URL under topic
  `handoff`, a separate run read it back verbatim. (M647)
- **Continuous agents — a living, never-tiring loop.** A new cadence mode,
  `continuous`, is a completion-anchored loop: the agent runs, and once its run
  COMPLETES it re-anchors to fire again `cooldown` later — so it runs forever,
  one cycle after another, never overlapping (the engine's in-flight guard) and
  carrying its memory + journal across cycles. The system can now keep a worker
  perpetually alive ("watch the world and act"), not just fire on a fixed clock.
  Exposed through the `schedule` tool (`op=continuous`, `cooldown`) so the agent
  can start one itself; it shows in the Schedules cockpit as `continuous · Ns
  cooldown` and is paused/removed like any schedule (the off-switch). The cooldown
  is floored to 1s so it can't busy-loop the daemon; per-cycle cost rides the
  daily budget ceiling. Verified live: the agent started a 15s-cooldown loop that
  fired 3 times over ~50s, each cycle `last: completed` → `next` re-anchored.
  (M646)
- **`standing` tool — agents create their own event-triggered agents.** A new
  in-process tool lets the agent set up its OWN autonomous, trigger-driven agents
  (Chronos standing orders): `op=create_event` makes one that fires its plan
  whenever a matching journal event is published (e.g. trigger on `task.failed`);
  `op=create_cron` on a cron schedule; plus `list` / `remove`. This is the
  agents-create-agents primitive for the EVENT/cron axis, symmetric with the
  `schedule` tool (the time axis) — so the agent can now arrange reactive
  ("when X happens, do Y") *and* recurring unattended behaviour itself, each
  firing through the full governed loop and visible in the Standing cockpit.
  Conservative `ask` initiative by default; governed by a new ask-first
  `standing` capability. Verified live: the agent created a `task.failed` →
  "investigate the failure" order that persisted in the store. Tool count
  16 → 17. (M645)

### Fixed
- **Inbox view never rendered conversations.** The unified Inbox read the wrong
  payload key (`items`) while the control plane returns `threads`, so it always
  showed empty even with channel traffic. Rewrote it as a proper unified-
  conversation view: each channel thread (Telegram/Slack/Discord/email/…) folded
  newest-first, with messages marked inbound (from the operator) vs outbound
  (from the agent) in a chat layout, live-nudged on `channel.*` events. Verified
  live against the real `/api/inbox` payload (`{threads:[],count:0}`): the empty
  state renders cleanly, 0 console errors. (M640)

### Added
- **`runs` tool — the agent recalls its OWN past work.** A new in-process tool
  lets the agent introspect its own run history by folding the daemon's journal:
  `op=recent` lists recent top-level runs (intent, status, cost, when), `op=stats`
  gives aggregate totals (completed/failed/success-rate/spend), `op=search` finds
  past runs whose intent matches a query. The self-knowledge primitive that
  complements `memory` (deliberate facts) and `world` (entities): the agent can
  see what it has actually *done*, not just what it chose to remember — so it can
  check "have I looked into this already?" or report on its activity. Sub-agent
  runs are folded out (operator-facing leads only); governed by a low-risk
  `runs.read` capability (Allow by default). Verified live: asked to report its
  history, the agent called `runs` and returned its real stats (18 completed,
  100% success) and correctly listed its 3 most recent runs by intent. Tool count
  15 → 16. (M644)
- **Global alert bell — proactive signals visible from every view.** The header
  now carries an alert indicator (on every panel) that counts the daemon's
  warning/critical signals as they stream — self-health degradations, run
  failures, halts, blocked egress — using the same classifier as the Alerts view.
  It pulses red on a critical, shows an amber/red count badge, and clicking it
  jumps to Alerts and clears the count (an acknowledge). So a problem is no longer
  invisible unless you happen to be on the Alerts tab. Verified live: a halt
  raised the badge to "1" while on the Overview panel, and clicking it navigated
  to Alerts and reset, 0 console errors. (M642)
- **Grant/restrict a capability from the catalog.** The Capability catalog's
  trust-level badge is now an editable dropdown (L0 deny … L4 allow): change a
  tool's level right where you see what it does and it posts to the same
  `/api/edict/set_level` the Policy view uses — so observing the agent's
  capability surface and governing it happen in one place. Verified live (default
  policy): setting `shell` to L0 persisted (confirmed via `edict_show`), restoring
  to L2 worked, 0 console errors. (M641)
- **Reflection view — the system reasoning about itself.** The Reflection panel,
  previously a raw-JSON dump, is now a proper view of the daemon's self-reflection
  pass: observation tiles (events folded, tasks done/failed, briefs, approvals,
  skills, world entities), the world-model salience decay it applied (the one
  safe auto-adjustment), and the advisory **proposals** it derived — recalibrations
  it suggests but never auto-applies, each with its area, observation and
  suggestion. A "Run now" button triggers a fresh pass (new `/api/reflect/run`
  write route over the existing deterministic, offline reflect engine). Verified
  live: a pass folded 508 events into the tiles (21 done / 1 failed / 1 brief / 3
  entities) and rendered "nothing to recalibrate — balanced", 0 console errors.
  (M639)
- **Standing-orders cockpit — govern what the daemon does unprompted.** The
  Standing view is now a management surface, not a read-only list: each Chronos
  standing order (a persistent goal that fires on a cron schedule or a matching
  journal event and acts at its initiative level) shows its status, autonomy
  mode, plan, and its triggers — visually distinguished as event (⚡ subject) vs
  cron (🕐 schedule) — with pause-resume / remove controls. Backed by two new
  token-gated webui write routes (`/api/standing/{enable,remove}`) over the
  existing control-plane handlers; `handleStandingSetEnabled` now also accepts
  the webui's string transport for the `enabled` flag (mirroring the schedule
  fix). Verified live: an event-triggered (`observer.delta`) and a cron
  (`0 9 * * *`) order rendered with correct trigger badges, Pause flipped the
  status, and Remove deleted each through the UI — 0 console errors. (M638)
- **Capability catalog — what the agent can do, and under what policy.** A new
  view shows the agent's full tool surface: every registered tool with its
  description, the Edict **capability** that governs it (its primary axis —
  `file.write`, `http.post`, `web.search`, …), the **current trust level**
  (L0–L4, colour-coded, editable in Policy), and live usage (calls / errors /
  unused). Until now the operator could only see tools *after* they were called
  (the usage-based Tools view); the catalog makes the whole capability surface
  observable up front. Backed by enriching the existing tool inventory
  (`CmdToolList`) with each tool's capability (via `CapabilityForToolCall` with a
  representative input) and exposing it as a read route (`/api/tools_catalog`),
  joined client-side with the policy levels and usage stats. Verified live: all 9
  tools rendered with correct capabilities, levels and usage, 0 console errors.
  (M637)
- **Alerts view — what the daemon flagged on its own.** A new view surfaces the
  daemon's PROACTIVE signals as a focused, severity-ranked feed — distinct from
  the raw event firehose: pulse observer deltas (the M628 self-health monitor's
  degradations/recoveries, carrying their severity), the briefings pulse decided
  to send, run failures, blocked egress, budget/rate trips, and halts. Each alert
  shows its level (critical/warning/info, colour-coded), origin, message and time,
  with level-filter chips and counts; alerts accumulate in their own rolling list
  (longer-lived than the 300-event stream). This completes the self-monitoring
  loop: M628 made the daemon notice problems unprompted, and now there's a place
  in the UI that shows what it noticed. Verified live: an emergency halt surfaced
  instantly as a critical alert, 0 console errors. (M636)
- **Schedules cockpit — see and manage what fires unattended, incl. the agent's
  own.** The Schedules view is now a management surface, not a read-only list:
  every scheduled intent shows its origin (an **agent**-scheduled run — created
  via the M634 `schedule` tool — is badged distinctly from operator/env ones,
  with an agent count in the header), its cadence, next fire and last outcome,
  plus **run-now / pause-resume / remove** controls. Backed by three new
  token-gated webui write routes (`/api/schedule/{remove,run,enable}`) over the
  existing control-plane handlers; `handleScheduleEnable` now also accepts the
  webui's string transport for the `enabled` flag. Verified live: an
  agent-scheduled (`every 2h`) and an operator (`daily 08:00`) schedule rendered
  with correct origin badges, and Remove deleted each through the UI — 0 console
  errors. (M635)
- **`schedule` tool — the agent schedules its own future work.** A new
  in-process tool lets the agent arrange future runs in the daemon's persistent
  cadence store: one-shot after a delay (`op=in`), recurring interval
  (`op=every`), daily at a wall-clock time (`op=daily`, optional weekday spec),
  plus `list` / `remove`. A scheduled intent fires later as a fresh run through
  the full governed loop — the autonomy primitive that turns "ask me again later"
  into "I'll handle it then." Schedules it creates are tagged `source=agent` so an
  operator sees and can prune them (`agt schedule list`). Governed by a new
  ask-first `schedule` capability; allowed under `AGEZT_ALLOW_ALL`. Verified live
  end-to-end with the real provider: the agent scheduled a one-shot, and ~40s
  later it fired autonomously (`schedule.fired` → a new run). (M634)
- **Live delegation activity in Mission Control.** The real-time telemetry
  terminal now folds `subagent.spawned` into its per-second buckets and shows a
  fifth live metric — `delegations/60s` — with its own sparkline, so multi-agent
  fan-out is visible as it happens, not just in the after-the-fact Agents graph.
  When a deep tree runs, the card climbs with every sub-agent spawned. Verified
  live: a two-level delegation produced `delegations/60s: 2`, 0 console errors.
  (M633)
- **Click any agent in the delegation graph to fly it.** Nodes in the Agents
  delegation graph are now clickable: selecting one (lead or any sub-agent, at any
  depth) opens a side cockpit panel with that agent's full live detail — status,
  model, tokens, cost, its tool calls and answer — and, for a still-running
  agent, the pause / step / steer / resume controls (the same `RunDetailLoader`
  the Runs view uses). Combined with M631 this means an operator can watch a deep
  tree and reach into it to pause or redirect a *specific* worker, all from the
  graph. The selected node is ring-highlighted; the panel closes back to the full
  graph. Verified live with 0 console errors. (M632)
- **Steer an individual sub-agent — the cockpit reaches into the tree.** Live
  steering (pause / single-step / inject directive / resume, M608) was wired only
  for the top-level lead; sub-agents ran un-steerable. Now every sub-agent
  registers its own steering control under its own correlation, so the per-run
  control API (`/api/run/{pause,resume,step,steer}`) — and therefore the existing
  Runs-view cockpit, which lists sub-agent runs and shows `SteerControls` for any
  live one — can pause or redirect a *specific* worker deep in a delegation tree,
  not just the lead. The pause/steer/resume events are journaled under the
  sub-agent's own timeline. Verified with a live two-run test that pauses, steers,
  and resumes a running sub-agent addressed by its own correlation. (M631)
- **Depth-aware delegation graph + deep-tree root fix.** The Agents view's
  delegation graph now tags each sub-agent with its nesting level (`L1`, `L2`, …)
  so a deep tree reads at a glance — a depth-2 worker is visibly distinct from a
  depth-1 one. While verifying this against a real two-level tree, fixed a
  depth>1 bug: `pickDefaultRoot` selected the newest run that *has* children, but
  with nesting an intermediate sub-agent also has children — so it could root the
  graph mid-tree on a node that isn't even in the lead selector, desyncing the
  view and hiding the true lead. It now restricts to true roots (no parent).
  Verified live: the coordinator → sub-coordinator (L1) → leaf (L2) tree renders
  whole (agents 3, depth 2) with the selector and graph in agreement, 0 console
  errors. (M630)
- **Deep multi-agent delegation, made safe — a tree-wide total cap.** Sub-agent
  nesting beyond one level (`AGEZT_SUBAGENT_DEPTH>1`) was already possible, but a
  depth-D, fan-out-F tree can hold up to Fᴰ agents and neither the per-spawner
  fan-out cap nor the per-lead spend cap bounds the *whole tree's size*. New
  `AGEZT_SUBAGENT_MAX_TOTAL` caps the total number of sub-agents across every
  depth of one delegation tree (attributed to the root run, propagated to every
  descendant), refusing the (N+1)th spawn *anywhere* in the tree with a tool
  error the spawning agent adapts to — the rail that makes depth>1 healthy. The
  cap is surfaced in the boot banner, `agt status`, and the control-plane status
  payload (`delegation.max_total`). Verified live: a real two-level tree
  (coordinator → sub-coordinator → leaf) completed correctly with depth-1 and
  depth-2 spawns both journaled. (M629)
- **Proactive self-monitoring — the daemon watches its OWN health.** A new pulse
  observer (`self:health`) samples the daemon's recent reliability from its own
  journal — tool-error rate (`tool.invoked` vs `tool.result` errors) and
  run-failure rate (`task.completed` vs `task.failed`) — and, when that health
  *transitions* between healthy / degraded / critical, emits a Delta that flows
  through the existing pulse pipeline (salience → initiative → briefing) and is
  delivered over whatever channels are wired. This turns the system's
  self-observation (the reactive Analyst, which answers when asked) into proactive
  self-monitoring (it tells you, unprompted, the moment its health changes). Edge-
  triggered, so it never floods: the first poll is a silent baseline, unchanged
  levels stay silent, and a thin sample can't manufacture an alert (min-sample
  guard). On by default; `AGEZT_PULSE_HEALTH=off` disables it, `=<float>` overrides
  the tool-error-rate degrade threshold (default 0.30). Verified live: a burst of
  failing tool calls produced the alert *"daemon health healthy → degraded: tool
  errors 4/12 (33%), run failures 0/11 (0%)"* with no operator prompt. (M628)
- **`web_search` tool — the agent can now DISCOVER, not just fetch.** A new
  in-process tool runs a keyword query against a public engine (DuckDuckGo's
  no-JS HTML endpoint, keyless) and returns the top results as structured
  `{title, url, snippet}` records — closing the gap where the agent could fetch a
  URL it was handed (http / browser.read) but couldn't *find* one. Keyless (no
  operator secret), SSRF-guarded (the same netguard egress guard that refuses
  internal/metadata addresses), and fail-soft: a flaky search or an empty page
  returns an empty result set with a note, never a hard error that fails a run.
  Governed by a new `web.search` capability (L2, ask-first — the engine host is
  fixed, the query is the only operator input); allowed under `AGEZT_ALLOW_ALL`.
  Verified end-to-end against the live daemon: the model called `web_search`,
  the policy gate decided `web.search`, and it returned the correct URL. (M627)
- **Analyst — the daemon reasons about itself.** A new AI observability view:
  ask a natural-language question about the running system and it gathers a live
  snapshot (run stats, per-tool error rates, cache savings, recent runs) and asks
  the daemon's own model to analyse it and answer — citing the real numbers, with
  the model's reasoning streamed and the reply rendered as Markdown (tables,
  recommendations). Reasons purely from the snapshot (no tool calls). Suggested
  questions seed it (health summary, tool failures, spend drivers, anomalies).
  (M626)
- **Mission Control — real-time telemetry terminal.** A new view folds the live
  event firehose into per-second buckets over a rolling 60-second window and
  renders the daemon's pulse as live rates with animated sparklines: a full-width
  activity waveform (now / peak / avg events-per-second) plus LLM calls/s,
  tokens/s, spend/s and tool-calls/s cards, each updating every second as the
  system works. Unlike the snapshot-based Dashboard/Insights, these are rolling
  rates computed continuously client-side from the stream. (M625)
- **Deep-linkable views.** The active view is now reflected in the URL hash
  (`#agents`, `#insights`, …), so every view is bookmarkable, survives a reload,
  and the browser back/forward buttons move between views. (M624)
- **Prompt-cache savings view.** The Cache view now leads with the dollars saved
  by prompt caching, the priced-call count it covers, cache read/write token
  tiles, and a read-vs-write token-split bar — so the cost benefit of caching is
  legible at a glance. (M623)
- **Skills library.** The Skills view is now a card library: each learned
  procedure shows its status (active / shadow / draft / quarantined, colour-coded),
  name, version, description, triggers, required tools, shadow/usage metrics and
  age, with the full procedure body expandable inline and promote / quarantine /
  revert controls per card. (M622)
- **Memory browser.** The Memory view is now a searchable knowledge board: each
  durable fact is a card showing its type, subject, content, confidence, age and
  tags, sorted newest-first, with live free-text search across
  subject/content/type/tags and one-click forget. (M621)
- **Providers & Tools monitors.** The Providers and Tools views (formerly plain
  stat panels) are now live monitoring dashboards. Providers: routed / fallback /
  fallback-rate tiles, a routes-by-provider bar breakdown with each provider's
  fallback share, and a colour-coded routing-activity log. Tools: calls / errored
  / error-rate tiles, a per-tool usage breakdown where each bar splits success vs
  error share (with calls · errors · avg latency), and a colour-coded invocation
  log. Both refresh on a timer and on the relevant live events. (M620)
- **System health dashboard.** The System view (formerly a raw `/api/status`
  JSON dump) is now a proper vitals board: a big Operational / HALTED banner with
  model, uptime and daemon version; live counter tiles (active runs, pending
  approvals, journal head, tools, memory records, world entities, active skills,
  schedules); and detail cards for delegation limits, the HTTP surface, the
  credential chain, and provider routing (with the most recent fallback reason
  surfaced as a warning). Refreshes on a timer and on halt/resume/run events.
  (M619)
- **Journal Search — find any past event across all history.** Where the Live
  Stream shows the present, the new Search view queries the *whole* journal
  server-side (`journal_grep`): filter by free-text pattern plus
  kind / actor / correlation, and every match renders in the same colour-coded,
  payload-expandable rows as the live console. The Web UI's `/api/journal` route
  gained a `journal_search` sibling exposing the full grep filter set
  (pattern/kind/subject/actor/correlation/limit). (M618)
- **⌘K command palette — reach everything instantly.** Press ⌘K / Ctrl+K (or the
  header button) anywhere to fuzzy-search every view and quick action, navigate
  with the keyboard (↑/↓, Enter, Esc), and jump straight there. Results are
  grouped (Go to / Action) and fuzzy-ranked (label-substring beats subsequence).
  Actions include Halt / Resume / Toggle theme. With 24 views, this is the fast
  path to all of them. (M617)
- **Live Stream — a colour-coded, filterable event console.** The Event Feed is
  now the daemon's full nervous system, observable: every journal event is
  categorised (task / llm / tool / policy / budget / steer / provider / context /
  knowledge / system) with a fixed hue, shown in a dense terminal-style stream.
  Toggle categories on/off (each chip shows a live count), free-text search across
  kind/subject/actor/correlation, click a correlation to filter to one run, pause
  to freeze the view while reading, and click any row to expand its full payload
  as a structured key/value table. Failure/denial kinds are highlighted. (M616)
- **Agents — a live multi-agent delegation graph.** A new view visualises a run
  and its sub-agent fan-out as an interactive node graph (React Flow): the lead
  agent at the top, each delegated sub-agent below it, connected by animated
  edges, every node coloured by status and showing its model, iteration count and
  spend. Pick any lead run; whole-tree totals (agent count, depth, tree spend) sit
  up top; it refreshes live as sub-agents spawn and finish. Tidy-tree layout
  derived client-side from `/api/runs` (`parent_correlation` links). Verified
  against a real 1→3 fan-out. (M615)
- **Insights — an analytics cockpit with charts.** A new view turns the runs
  history into a visual dashboard: headline tiles (runs, total spend, success
  rate, avg duration, avg iterations), a cumulative-spend area chart over time, a
  run-outcomes bar (completed/failed/running), and a per-model spend breakdown.
  All derived client-side from `/api/runs` with dependency-free, theme-aware
  inline-SVG/flex charts (no chart library, CSP-clean), refreshed on a timer and
  nudged when a run finishes. (M614)

### Fixed
- **`browser.read`, `memory`, and `world` were silently un-grantable — now
  first-class.** These three tools mapped to capabilities the policy engine never
  registered, so every call hit the unknown-capability default-deny — and worse,
  they couldn't even be granted (the policy control center / `agt edict level`
  reject unknown capabilities), so `AGEZT_ALLOW_ALL` *still* left them denied.
  They're now proper capabilities with sensible defaults (`browser.read` L2 like
  `http.get`; `memory`/`world` L4), listed in the Policy view and grantable at
  runtime. Verified: `browser.read https://example.com` now succeeds (200) where
  it was permanently denied. (M613)
- **`AGEZT_ALLOW_ALL` now truly covers everything, including future tools.** Added
  an `UnknownAllow` engine option (set under the master switch) so a capability
  with no configured level is allowed rather than default-denied — so a plugin
  tool introducing a brand-new capability is covered too. The hard-deny
  catastrophe rails still fire first. (M613)

### Added
- **Flight recorder — scrub and replay any run, step by step.** A new Replay view
  turns a run's journaled event arc into a playable timeline: pick a run, then
  scrub or hit play to move a cursor through every moment — LLM rounds, tool calls
  and their results, policy decisions, operator steering, spend — with the
  *cumulative* state (iteration, tokens in/out, spend, tool-call count) shown for
  the exact point you're parked on. Click any step to jump, choose 1×/2×/4×
  playback, and watch an in-flight run record itself live (newest run selected by
  default, live events folded in) then rewind it. Pure client-side derivation over
  the existing journal — the kernel stays the source of truth. (M612)
- **`AGEZT_ALLOW_ALL=1` — one switch to allow everything.** For a single-operator
  dev box where the safe-by-default gating is just friction, this master switch
  sets *every* governed capability to L4 (allow) and opens the http/browser tools
  to any host (including loopback and the private network) — so no tool call is
  denied or prompts. Off by default (the project stays safe-by-default for
  everyone else); a loud startup banner makes it impossible to enable silently.
  The built-in catastrophe hard-deny rails (fork-bomb, `dd` to a raw device)
  deliberately remain — they guard against self-destruction, not normal tool use,
  and are no-ops on Windows. Restrict later from the Policy view or by unsetting
  the flag. Verified: with it on, every capability reports L4 and a real
  `http GET https://example.com` succeeds (200) where it was previously
  hard-denied. (M611)
- **Grant or restrict tool capabilities from the cockpit — no restart.** The Web
  UI Policy view was read-only (decision stats + log); enabling a default-denied
  capability meant editing env vars and restarting. It is now a control center:
  every governed capability shows its current trust level (L0 deny … L4 allow) in
  a dropdown you can change live, the engine-wide ask mode (allow/prompt/deny) is
  a selector, and runtime hard-deny rules can be removed. Changes post the
  existing `edict_set_level` / `edict_set_mode` / `edict_deny_*` control-plane
  commands, are journaled as `policy.changed`, and persist in the durable policy
  overlay (survive restart). Verified end-to-end: flipping `http.post` L1→L4 from
  the UI took effect on the live daemon (CLI-confirmed) and landed in the overlay.
  (M610)
- **The agent now knows its host — no more blind `ls` on Windows.** Every run's
  system prompt gains a concise *runtime environment* preamble: the host OS/arch,
  the exact shell the shell tool uses (with command-style guidance — native
  Windows `dir`/`type`/`copy` vs POSIX `ls`/`cat`/`cp`), the shared workspace
  directory, the date, and the tools available this run. Before this, on a
  Windows box the model reflexively tried `ls`/`cat`/`more`, burned iterations on
  "not recognized" errors, then adapted; now it uses the right commands from the
  first call (a verified 6+-iteration trial-and-error run dropped to 4 clean
  iterations). On by default; `AGEZT_ENV_INJECT=off` disables it. (M609)
- **Live run steering — fly a running agent from the cockpit.** You can now grab
  the controls of an in-flight agent without cancelling it: **pause** it at the
  next iteration boundary, **single-step** one iteration at a time, **resume**,
  or **inject a directive** that the agent folds into its very next prompt and
  acts on. The agent loop consults a per-run control surface at each iteration
  boundary (so a pause lands at a safe point — the in-flight model call and tool
  results settle first), and a paused run still honours halt/cancel/timeout, so
  steering never makes a run un-killable. Drive it from the **Web UI** (a "Steer
  this run" panel in the Activity drill-in, with Pause/Step/Resume and a directive
  box) or the **CLI** (`agt runs pause|resume|step|steer <correlation> [text]`).
  Every action is journaled (`run.paused` / `run.resumed` / `run.stepped`, and
  `run.steered` when the directive takes effect) so the run's timeline shows
  exactly when and how an operator intervened. Tenant tokens may steer their own
  runs (same posture as cancel). Verified end-to-end against a live reasoning
  model: a paused agent, redirected mid-run, abandoned its original plan and
  carried out the injected instruction.
- **Adjust the daily spend ceiling at runtime — from the CLI or the Web UI.** The
  global budget cap was fixed at daemon start (`AGEZT_DAILY_CEILING`); changing it
  meant a restart. The governor now takes a runtime override
  (`SetDailyCeiling`) that supersedes the configured value for *all* enforcement
  and reporting, exposed as the `budget_set` control-plane command. Operators set
  it with `agt budget set <amount>` (dollars; `0`/`off` = unlimited) or from the
  Web UI's Budget panel — a dollar input, quick presets ($5/$20/$50/$100), and an
  Unlimited button, with the spend gauge updating live against the new ceiling.
  Every change emits a `budget.ceiling_set` audit event. The control plane forbids
  tenant tokens here (the global ceiling is primary-token-only). Lowering the cap
  below today's spend simply blocks further calls until UTC rollover; per-tenant
  sibling governors keep their own separately-settable ceilings.

### Fixed
- **The shell and file tools now agree on "here".** The shell tool ran in the
  daemon's process CWD while the file tool was scoped to the workspace root, so an
  agent's `dir`/`ls` (shell) and `file read x` (file) saw *different* directories
  — every `file read` of a file the agent had just listed via shell failed "cannot
  find the file." The shell tool now runs in the same workspace root as the file
  tool. Verified end-to-end: shell `cd` and the file tool now resolve identical
  paths, and cross-tool file reads succeed. (M609)
- **The agent stops burning iterations on policy-denied tools.** The loop offered
  the model *every* registered tool, even ones the policy would always refuse
  (e.g. a default-denied `browser.read`), so the model could waste several
  iterations — and tokens — requesting calls that get denied. The loop now drops a
  tool from the set offered to the model once the policy has refused it (hard-deny
  once, or any deny twice) in a run. The M116 guard only caught an identical
  (tool,input) repeat; this catches the same tool retried with new inputs.
  Observed with a real reasoning model: the offered-tool count fell 7→6 right after
  two `browser.read` denials, ending the retries.
- **Live token streaming now works with real providers.** The agent loop streams
  token/reasoning deltas only when its provider satisfies `agent.StreamingProvider`,
  but every real run goes through the `Governor` (routing + fallback + budget),
  which implemented only `agent.Provider` — so the streaming branch never engaged
  and the Web UI Chat (and `agt run`) collapsed each answer into one chunk instead
  of streaming it live. The Governor now implements `CompleteStream`, routing the
  streaming call through the exact same pre-flight gates, fallback chain and usage
  accounting as `Complete` (a non-streaming chain entry, e.g. the offline mock
  fallback, degrades gracefully to no deltas). Verified against real DeepSeek: a
  run that previously emitted 0 `llm.token` events now streams 90+ per answer, and
  the Chat renders progressively.

### Added
- **Cockpit dashboard (Overview).** The Overview is now a live cockpit instead of a
  raw status dump: at-a-glance tiles for running / completed / failed / success
  rate, a budget gauge (today's spend vs the daily ceiling), the active model with
  avg iterations and sub-agent delegations, spend-by-model, and a live event
  ticker — refreshed on a timer and nudged by run start/end events. The raw daemon
  self-report moves to a "System" view.
- **Tool results render as widgets.** When a tool call's output is JSON (file.list,
  http, shell, …), the Chat now shows it as a DataView widget — a table for an
  array, a key/value card for an object — instead of a wall of raw braces. Non-JSON
  output stays as plain text.
- **Multiple conversations in Chat (sidebar).** The Chat is now multi-thread,
  ChatGPT-style: a sidebar lists past conversations (auto-titled from the first
  message, newest first), click to switch, "New chat" starts a fresh one, and each
  is deleteable. All persisted to localStorage; the previous single-thread history
  is migrated into the first conversation, so nothing is lost. (On small screens
  the list is hidden and a New-chat button stays in the thread header.)
- **Cleaner math in Chat.** LaTeX math delimiters (`\( … \)`, `\[ … \]`) that a
  model emits are stripped so the expression reads as plain text (e.g. `2b = 0.10`)
  instead of literal backslash-brackets. Applied only to inline text, never to
  code blocks.
- **Structured-data widgets in Chat.** A ` ```json ` or ` ```widget ` fenced block
  whose body is valid JSON is rendered by shape rather than as raw text: an array
  of objects becomes a table (union-of-keys columns), an object a key/value card,
  an array of scalars a list — recursively, depth-bounded, with a raw-JSON
  fallback. So an agent can emit a data array and the UI shows the right view
  automatically. Plain escaped React throughout (CSP-safe).
- **Chat history persists.** The conversation is saved to `localStorage`, so a
  reload, a daemon restart, or closing the tab no longer loses your thread — it's
  restored on next open. A "New chat" button clears it; a turn that was mid-stream
  when the page closed is restored as "interrupted" rather than a spinner that
  never resolves.
- **Markdown tables + blockquotes in Chat.** Agent answers routinely include GFM
  tables; the renderer didn't parse them, so they showed as raw `| … |` pipes.
  The dependency-free markdown parser now handles GFM tables (header + alignment
  separator + rows) — rendered as a real, styled `<table>` (header bg, row
  striping, horizontal scroll) — and blockquotes. Still XSS/CSP-safe (plain
  escaped React elements, no raw HTML).
- **Active model shown in the Chat composer.** The model field now displays the
  daemon's active model (e.g. `deepseek-v4-pro`) as its placeholder, so you always
  know which model you're talking to — and a typo in the override is obvious
  against it.
- **Reasoning (chain-of-thought) in Chat.** A reasoning model (deepseek-reasoner /
  -v4, o-series, Claude thinking) streams its chain of thought as `llm.reasoning`
  deltas separately from the answer; the Chat now folds and shows it — a live
  "Thinking…" block while the model reasons, which collapses to an expandable
  "Reasoning" toggle once the answer begins. Previously these deltas reached the
  browser but were dropped.
- **Copy a whole answer in Chat** — finished agent replies now have a Copy button
  next to their meta line, copying the full answer to the clipboard (complements
  the per-code-block copy).
- **Drill into a run from the Activity monitor** — each run in the live monitor is
  now expandable: click it to see the full detail — status / model / iterations /
  tokens / cost, every tool call it made with its policy verdict and output, and
  the final answer — fetched from the journal and folded live as the agent works.
  The detail renderer (`components/RunDetail.tsx`) is now shared with the Runs
  view, so the historical list and the live monitor render a run identically.
- **Copy button on Chat code blocks** — fenced code in an agent answer now has a
  hover Copy button (and a language label), so the command or snippet it hands you
  is one click to the clipboard. Uses the async Clipboard API; no-ops silently if
  the context can't write.
- **Per-run Cancel in the Activity monitor** — each in-flight run now has a
  **Cancel** button (the targeted alternative to the global Halt): it issues
  `CmdCancelRun` for that one correlation, leaving every other run and the kernel
  untouched. The daemon emits `task.failed(reason=canceled)`, which the live fold
  turns into a failed row. Exposed as a POST-only, correlation-allowlisted
  `/api/cancel_run` write route.
- **Multi-turn Chat continuity** — the Chat view now carries conversation context:
  each message is sent with the prior turns as `history`, which the `/api/run`
  proxy folds (with the new turn) into one transcript intent before running the
  governed loop, so the agent answers with the whole conversation in view. The
  collapse is the new shared `kernel/convo` package — the *same* mapping the
  OpenAI-compatible API uses (refactored to call it), so both surfaces render
  prior turns identically. A lone first message still passes through verbatim;
  history is capped (most-recent 40 turns) and never reaches the control plane as
  anything but the resolved intent.
- **Markdown rendering in Chat** — finished agent answers now render as Markdown
  (fenced code blocks, inline code, bold/italic, bullet & numbered lists,
  headings) instead of flat pre-wrapped text, so code and structure are legible.
  A tiny, dependency-free, unit-tested parser (`lib/markdown.ts`) emits an AST
  that a React component renders as plain escaped elements — no raw-HTML path, so
  it's XSS-safe under the strict CSP. Streaming text stays plain until the answer
  is final (an unclosed code fence can't swallow mid-stream output).
- **Activity live monitor** — a Web UI view answering "is anything running right
  now, and what is it doing?". Seeds the in-flight runs from `/api/runs`, then
  folds the event firehose so each run's current step ("calling shell",
  "thinking · iter 2"), iteration count, elapsed time and spend update in real
  time. Delegated **sub-agents** are nested under the lead run that spawned them
  (from `subagent.spawned`), so the operator can see the background fleet. A
  pure, unit-tested fold (`lib/activity.ts`) drives the whole state machine.
  Second nav item, after Chat.
- **Chat view (the humane front door)** — a conversational Web UI to actually
  *talk to* the agent, now the default view. Type an intent and watch the
  governed loop answer live: streaming text (token deltas from real providers),
  the tool calls it made rendered as inline chips with their policy verdict
  (allowed / denied) and expandable output, the final answer, and the run's real
  cost (model · iterations · spend). It drives the same `CmdRun` as the CLI, so
  what you see is exactly what the daemon did. Backed by a new SSE streaming
  proxy (`POST /api/run`) that forwards each loop event to the browser inline,
  plus a pure, unit-tested fold (`lib/chat.ts`) turning the event stream into the
  message model. Light + dark, responsive, zero console errors under the strict
  CSP.
- **Voice input (speech-to-text)** — drive the agent by talking. `kernel/stt` is a
  minimal client for the OpenAI-compatible `/v1/audio/transcriptions` endpoint
  (spoken by OpenAI, Groq, and a local whisper.cpp server alike), `net/http` only,
  no dependency. Two CLI surfaces turn audio into text and optionally feed it
  straight to the governed loop: **`agt transcribe <file> [--run]`** (a recorded
  file) and **`agt listen [--seconds N] [--run]`** (the microphone). Microphone
  capture has no portable Go path without a CGO audio library, so — like the
  tunnel — Agezt drives an operator-chosen recorder via `AGEZT_VOICE_RECORD_CMD`
  (with `{seconds}` / `{out}` placeholders; e.g. `arecord` on Linux, `ffmpeg` on
  macOS/Windows), keeping the one-dependency promise. Configured with
  `AGEZT_STT_API_URL` (default OpenAI), `AGEZT_STT_API_KEY` (or `OPENAI_API_KEY`),
  and `AGEZT_STT_MODEL` (default `whisper-1`). The daemon's OpenAI-compatible API
  also gains **`POST /v1/audio/transcriptions`** (token-gated) when STT is
  configured, so any OpenAI audio client can upload a clip to Agezt and get a
  transcript — the third voice source, alongside the file and microphone CLIs.
  (M587)
- **Public tunnels** — expose a local Agezt HTTP service (the Web UI, else the REST
  API) to the public internet by supervising a tunnel binary. `kernel/tunnel`
  spawns `cloudflared` or `ngrok` (built-in presets) — or any custom command
  (`AGEZT_TUNNEL_CMD`) — pointed at the local address, scans its output for the
  public URL it advertises, prints that URL to the daemon log, restarts it with
  capped exponential backoff if it drops, and tears the whole process tree down on
  shutdown. Off unless `AGEZT_TUNNEL=cloudflared|ngrok` or `AGEZT_TUNNEL_CMD` is set
  (the operator opts in explicitly, since this makes the service publicly
  reachable); the target defaults to the Web UI addr, else REST, or
  `AGEZT_TUNNEL_TARGET`. Wrapping the providers' battle-tested rendezvous servers
  keeps the daemon's one-dependency promise — `os/exec` + stdlib only, no new
  dependency, with a process-group teardown mirroring the plugin host. (M586)
- **A plugin registry/marketplace** — `agt plugin registry <dir|url> [--install
  <name>]`. Completes the marketplace alongside the existing remote *skill*
  registry (`agt skill registry`): browse a registry's plugins (an `index.json`
  listing per-platform binaries, each pinned by its BLAKE3-256 digest), then
  install one — the installer picks the build for the running OS/arch, downloads
  it (bounded, from a directory or an `http(s)` URL), **verifies its BLAKE3 against
  the index pin before writing anything** (a mismatch is refused and nothing lands
  on disk), stages it under `<base>/plugins` (or `--dir`), and prints the exact
  `AGEZT_PLUGINS` + `AGEZT_PLUGIN_PINS` lines to enable it. It never edits the
  daemon's environment or loads anything: a plugin runs only when the operator
  wires it in, so "install" is "fetch + verify + stage", under the operator's
  authority. Untrusted index filenames are validated (no path traversal). Reuses
  the daemon's BLAKE3 pin code (`plugin.HashBytes` / `LooksLikePin`); `net/http`
  only, no new dependency. (M585)
- **Signal is now a messaging channel** (via signal-cli-rest-api) — the eleventh
  channel. `plugins/channels/signal` is a duplex channel that talks to an
  operator-run [signal-cli-rest-api](https://github.com/bbernhard/signal-cli-rest-api)
  server: it long-polls `GET /v1/receive/{number}` for inbound messages and POSTs
  `/v2/send` for outbound replies + Pulse briefs, mirroring the Matrix channel.
  An allowlisted sender number (`AGEZT_SIGNAL_RECIPIENTS`) drives the agent; the
  account's own number is skipped so a reply never re-enters the loop, and
  non-allowlisted senders are told once, fail-closed. The API URL is
  operator-pinned (`AGEZT_SIGNAL_API_URL`), so there is no SSRF surface; an
  optional bearer token (`AGEZT_SIGNAL_TOKEN`) covers a reverse proxy fronting the
  API (signal-cli-rest-api is itself unauthenticated). A defensive poll-rate floor
  means a server that doesn't honour `?timeout=` is never busy-spun. `net/http`
  only — no Signal SDK, no new dependency. Wired as `buildSignal`
  (`AGEZT_SIGNAL_API_URL` + `AGEZT_SIGNAL_NUMBER` required; `AGEZT_SIGNAL_RECIPIENTS`
  allowlist, `AGEZT_SIGNAL_TOKEN`, `AGEZT_SIGNAL_POLL_SECS` optional), surfaced in
  `agt status` and `agt send --channel signal`. (M584)
- **The Web UI now has a real browser end-to-end test in CI.** A Playwright suite
  (`frontend/e2e/webui.spec.ts`) drives the actual `go:embed`-ded production SPA in
  headless Chromium against a live keyless demo daemon: it asserts the shell + live
  SSE indicator render, navigation between views works, the Overview shows real
  daemon status, a submitted run renders as an expandable run-detail card (proving
  the journal → cards pipeline, M577/M580), the World panel mounts, and there are
  **zero console errors under the strict CSP** (the M566 regression guard, now
  automated). A reusable harness (`scripts/webui-e2e.sh`, `make webui-e2e`) boots
  the daemon, seeds a run, and resolves the tokenized Web UI URL from its log; a new
  CI job **`webui-e2e`** (20th… now 21st check) builds the binaries, installs the
  browser, and runs it. Dev-only: `@playwright/test` is a devDependency, the e2e
  specs are excluded from Tailwind's content scan (`@source not "../e2e/**"`) and
  from Vitest's `include`, so the committed `dist` is byte-for-byte unchanged. (M583)
- **Official Rust SDK** (`sdk/rust`, crate `agezt`). A zero-runtime-dependency
  client for the daemon's REST API (`/api/v1`) — `Client::new(base_url, token)`
  with `health()`, `models()`, `run()` (blocking), `run_stream()` (an
  `Iterator<Item = Result<StreamEvent>>` over Server-Sent Events →
  `start`/`token`/`done`/`error`), and `get_run()`; bearer-token auth, optional
  `.with_tenant()` for multi-tenant daemons and `.with_timeout()`; non-2xx becomes
  `Error::Api`, transport failures `Error::Transport`. **Standard library only** —
  a tiny built-in HTTP/1.1 client and JSON codec stand in for `reqwest`/`serde`
  (plain `http://`; front with a TLS proxy for `https`), so there is nothing to
  fetch and `#![forbid(unsafe_code)]` holds. Tests run a mock daemon on a
  `std::net::TcpListener` (Content-Length JSON + chunked SSE) — no third-party
  HTTP server or test framework — exercising the real transport path; CI runs
  `cargo fmt --check` + `cargo test`. Completes the Rust quarter of decision A4's
  "SDKs in Go/TS/Python/Rust" — all four SDKs are now shipped. (M582)
- **The Web UI test suite now includes component/DOM tests.** Building on the
  Vitest logic suite, `@testing-library/react` + `jsdom` cover the presentational
  components (`Badge`/`statusVariant`, `JsonView`/`KeyValue`/`Muted`/`ErrorText`)
  — they render to a real DOM and assert output (6 tests; 28 total). Component
  tests opt into jsdom per-file (`// @vitest-environment jsdom`) so the pure-logic
  tests stay on the fast node environment. Test files are excluded from Tailwind's
  content scan (`@source not "*.test.{ts,tsx}"`) so class strings used only in
  assertions never leak into the shipped CSS — the committed `dist` is unchanged.
  Dev-only; zero Go modules, zero runtime deps. (M581)
- **The Web UI's Runs view now updates expanded runs live.** An expanded run
  subscribes to the journal SSE stream and folds matching events into its arc, so
  the detail cards (status, tool calls, iterations, tokens) update as the agent
  works — the same live pattern Flow Studio uses for node recolour. The fetched
  snapshot and live events are merged by journal seq (`mergeEvents`, dedup), so an
  event delivered by both paths is never double-counted, and an event arriving
  before the fetch resolves is not lost. The `subscribe` callback is now stable
  (memoized in the events provider) so consumers don't re-subscribe on every
  event. (M580)
- **The Web UI now has a unit-test suite (Vitest) and a CI gate.** The React app
  had zero automated tests — every change relied on manual browser smoke. The
  run-detail derivation logic is extracted to a pure `lib/rundetail.ts`
  (`deriveDetail` folds a journaled event arc into the summary + tool-call
  breakdown) and covered by Vitest alongside the `format`/`utils` helpers (19
  tests). A new `frontend-test` CI job (`npm ci && npm test`) runs them on every
  push, and `make frontend-test` runs them locally. Dev-only tooling — Vitest is
  not bundled into the embedded `dist`, and adds zero Go modules. (M579)
- **`agt ha` — an operator-facing Home Assistant client.** `agt ha states
  [entity_id]` reads entity state (all as `entity_id = state` lines, or one as
  pretty JSON), `agt ha services` lists the service registry as sorted
  `domain.service` names (the introspection an operator uses to populate
  `AGEZT_HOMEASSISTANT_TOOL_SERVICES`), and `agt ha call <domain.service>
  [--entity id] [--data '<json>']` calls a service. It reads
  `AGEZT_HOMEASSISTANT_URL`/`_TOKEN` and talks to HA directly (no daemon needed)
  — the operator complement to the agent-facing `homeassistant` tool (M575):
  full access by the operator's own authority, where the tool stays fail-closed
  behind allowlists. `--json` prints raw responses; `net/http` only. (M578)
- **The Web UI's Runs view now shows structured run-detail cards.** Expanding a
  run no longer dumps a flat event list first — it renders a summary (status,
  model, iterations, token in/out + cached, cost, duration) derived from the
  run's journaled arc, a **Tool calls** breakdown (each call's tool, the Edict
  **capability** it exercised, an allowed / denied / hard-denied verdict, an
  error flag, and an output preview), and the **final answer** (or error). The
  raw event timeline is still one click away under a collapsible "raw events"
  toggle. Pure frontend — the kernel stays the source of truth; the UI only
  derives the view from the same `/api/journal` arc it already had. (M577)
- **The Python SDK now has an asyncio client.** `AsyncClient` (`from agezt import
  AsyncClient`) mirrors the synchronous `Client` method-for-method, but every call
  is awaitable and never blocks the event loop — `await c.run(...)`,
  `async for ev in c.run_stream(...)`, plus `health`/`models`/`get_run` and
  `async with` support. Still standard-library only (no aiohttp): the blocking
  HTTP work runs in a thread executor and the streaming run is bridged to an async
  iterator through an `asyncio.Queue`, reusing the sync client's request building,
  error mapping, and SSE parsing verbatim. 7 new `unittest` tests
  (`IsolatedAsyncioTestCase`) against the same stdlib http.server mock. (M576)
- **Home Assistant is now a control TOOL**, not just an outbound channel — the
  agent can READ smart-home entity state and ACTUATE the house from inside a run.
  `plugins/tools/homeassistant` exposes two operations over the HA REST API:
  `get_states` (GET `/api/states[/{entity_id}]`) and `call_service`
  (POST `/api/services/{domain}/{service}`). It shares the channel's
  `AGEZT_HOMEASSISTANT_URL`/`_TOKEN` (same instance) but is gated by its OWN
  fail-closed allowlists — `AGEZT_HOMEASSISTANT_TOOL_READ` (which entities are
  readable; a bulk read is filtered to it, so a prompt-injected agent can't
  enumerate the house) and `AGEZT_HOMEASSISTANT_TOOL_SERVICES` (which services
  are callable; `domain.*` and `*` wildcards, `…_ALLOW_ALL_SERVICES=1` to
  bypass). The two axes map to distinct Edict capabilities,
  `homeassistant.read` (Allow by default — reads are low-risk) and
  `homeassistant.call` (Ask-first — actuating the physical world warrants
  confirmation). The HA host is operator-pinned config (the agent never supplies
  it), so there is no SSRF surface. The tool registers only when URL+token AND an
  allowlist are set, so the outbound notify channel can be configured without
  auto-exposing an actionable surface. `net/http` only — no dependency. (M575)
- **Microsoft Teams is now an (outbound) channel** — the tenth channel.
  `plugins/channels/teams` delivers Pulse briefs and `agt send` to Teams via
  Incoming Webhooks (POST a `MessageCard` to the per-channel webhook URL). Because
  Teams webhooks are per-channel, the channel holds a NAMED map of webhooks
  (`AGEZT_TEAMS_WEBHOOKS=general=https://…,alerts=https://…`); the message's
  `channel_id` selects which one (`agt send --channel teams --to general …`), and
  an unknown name is refused (fail-closed). `net/http` only — no dependency.
  Outbound-only (inbound needs the Bot Framework, a follow-up). Wired as
  `buildTeams`, surfaced via `agt status`. (M574)
- **Home Assistant is now an (outbound) channel** — the ninth channel, turning the
  agentic OS into a voice in your home. `plugins/channels/homeassistant` delivers
  Pulse briefs and `agt send` to a Home Assistant instance via its REST notify API
  (`POST {base}/api/services/notify/{service}`, long-lived bearer token,
  `{"message": …}`). The message's `channel_id` is the HA notify SERVICE name
  (e.g. `mobile_app_phone`, `persistent_notification`, `tts`), so a brief can land
  as a phone push, a TTS announcement, or a dashboard notification; an Allowlist
  (`AGEZT_HOMEASSISTANT_SERVICES`) is fail-closed. `net/http` only — no dependency.
  Outbound-only (drive Agezt *from* Home Assistant via the generic webhook
  channel). Wired as `buildHomeAssistant` (`AGEZT_HOMEASSISTANT_URL` +
  `AGEZT_HOMEASSISTANT_TOKEN`), surfaced via `agt status`; `agt send --channel
  homeassistant --to <service>` pushes one off. (M573)
- **Official TypeScript SDK** (`sdk/typescript`, `@agezt/sdk`). A
  zero-runtime-dependency client for the daemon's REST API (`/api/v1`) built on the
  platform `fetch` (Node 18+ and browsers): `new Client(baseUrl, token, {timeoutMs,
  tenant})` with `health()`, `models()`, `run()`, `runStream()` (an
  `AsyncGenerator` over Server-Sent Events → `start`/`token`/`done`/`error`), and
  `getRun()`; bearer-token auth; non-2xx throws `APIError`. The only dev dependency
  is TypeScript; tests use the Node built-in runner (`node:test`) against a
  `node:http` mock — no third-party test framework. Completes the TS half of
  decision A4's SDK set. (M572)
- **Official Python SDK** (`sdk/python`, `pip install agezt`). A standard-library-only
  client for the daemon's REST API (`/api/v1`) — `Client(base_url, token)` with
  `health()`, `models()`, `run()` (blocking), `run_stream()` (Server-Sent Events →
  `start`/`token`/`done`/`error`), and `get_run()`; bearer-token auth, optional
  `tenant` for multi-tenant daemons; non-2xx raises `APIError`. No third-party
  dependencies (urllib + json), and the tests are `unittest` against a stdlib
  http.server mock, so CI runs them with no `pip install`. Realizes the Python
  half of decision A4's "SDKs in Go/TS/Python/Rust". (M571)
- **WhatsApp is now a messaging channel** (Meta WhatsApp Cloud API) — the eighth
  channel. `plugins/channels/whatsapp` serves Meta's inbound webhook: the GET
  verification handshake (echoes `hub.challenge` when `hub.verify_token` matches)
  and POST deliveries authenticated with `X-Hub-Signature-256`
  (sha256=HMAC-SHA256 of the raw body under the app secret; empty secret fails
  closed). An allowlisted sender number drives the agent; since WhatsApp has no
  synchronous reply, the answer is sent back as a fresh Graph API message
  (`POST /{PhoneNumberID}/messages`, Bearer access token, `channel.SplitText` for
  long text). A retried delivery is de-duped on the message id. `net/http` +
  stdlib crypto only — no Meta SDK, no new dependency. Wired as `buildWhatsApp`
  (`AGEZT_WHATSAPP_APP_SECRET` + `AGEZT_WHATSAPP_ACCESS_TOKEN`, inbound on
  `AGEZT_WHATSAPP_ADDR`, outbound via `AGEZT_WHATSAPP_PHONE_NUMBER_ID`, allowlist
  `AGEZT_WHATSAPP_NUMBERS`) and surfaced via `agt status`. (M570)
- **SMS is now a messaging channel** (Twilio Programmable Messaging) — the seventh
  channel. `plugins/channels/sms` serves an inbound Twilio webhook and authenticates
  every request with the `X-Twilio-Signature` header (base64 HMAC-SHA1 over the
  request URL + sorted POST params, keyed by the account auth token; empty token
  fails closed, so no unsigned inbound). An allowlisted sender number drives the
  agent and the reply comes back synchronously as TwiML
  (`<Response><Message>…</Message></Response>`); a retried `MessageSid` is de-duped.
  Proactive messages (Pulse briefs, `agt send`) go out via the Twilio Messages REST
  API (Basic auth, `channel.SplitText` for long bodies). `net/http` + stdlib crypto
  only — no Twilio SDK, no new dependency. Wired as `buildSMS`
  (`AGEZT_SMS_ACCOUNT_SID` + `AGEZT_SMS_AUTH_TOKEN`, inbound on `AGEZT_SMS_ADDR`,
  outbound from `AGEZT_SMS_FROM`, allowlist `AGEZT_SMS_NUMBERS`) and surfaced via
  `agt status` + the config snapshot. (M569)

### Fixed
- **Pulse stops promptly on context cancel.** The heartbeat loop's `select` raced
  `ctx.Done()` against the ticker: when both were ready, Go's uniform-random
  choice could keep firing beats after cancel until the draw happened to land on
  `ctx.Done()` — surfacing as a flaky `TestStartStopsOnContextCancel` on loaded
  CI runners ("beats advanced after the engine should have stopped"). The tick
  branch now re-checks `ctx.Err()` and returns, so a cancelled engine stops within
  at most one in-flight tick. (M568)

### Added
- **Web UI: all remaining panels ported to bespoke React views.** Following the
  React rebuild (M566), the panels that shipped as a generic JSON fallback are now
  first-class React views: Config, Cache, Providers, Tools, Policy, Schedules,
  World, Skills, Standing, Memory, Inbox, Reflection, and Approvals. Providers /
  Tools / Policy gain inline "log" drill-downs (lazy-fetched `*_log` routes);
  World renders a React Flow node-link graph of the world model; Skills (promote /
  quarantine / revert), Memory (forget), World (forget), and Approvals (approve /
  deny) wire the existing mutating routes to action buttons. Pure-frontend: the Go
  routes were already in place, so no server change — the bundle is rebuilt and
  recommitted. The phased Web-UI migration (M566) is now complete. (M567)

### Changed
- **The Web UI is now a React 19 + Vite single-page app, built and embedded into
  the daemon** (decision A4; the previous hand-rolled server-rendered dashboard
  was the MVP cut). The bundle is built with Vite (React, Tailwind CSS v4,
  shadcn/Radix primitives, lucide-react, React Flow, dark/light, responsive) into
  `kernel/webui/dist` and `go:embed`-ded — so it ships in the single Go binary
  with **no Node at runtime and no new Go dependency** (`go:embed` is stdlib;
  `go.mod` unchanged). The Go server is now the thin static+proxy layer: it serves
  the embedded bundle (index.html no-cache, hashed `/assets/*` immutable + public
  so the browser can load subresources, with explicit OS-independent MIME types)
  and keeps every data surface — SSE `/events` and all `/api/*` — token-gated. The
  per-request CSP nonce is replaced by a static, stricter policy (`script-src
  'self'` admits only the hashed bundle, no inline script at all). This PR ships
  the SPA shell + live event feed, Status, Runs (+ event-arc detail), Budget, and
  **Flow Studio rebuilt on React Flow** (the plan DAG with live node recolour);
  the remaining read panels render through a generic view and are ported to
  bespoke React views in follow-ups. Reproducible-build CI gate
  (`frontend-dist-in-sync`) keeps the committed bundle in step with the source. (M566)

### Added
- **Flow Studio: author, visualise, and run plans from the Web UI.** The
  dashboard gains a full-width panel that turns the existing plan toolchain
  (`plan generate` / `refine` / `run` / `history`) into a visual surface: type an
  intent and Generate, edit the resulting plan JSON inline, see it drawn as a
  dependency DAG (loop nodes as boxes, gate nodes as hexagons), Refine with a
  natural-language instruction, then Run — watching each node recolour live
  (amber → running, green → done, red → failed) as `node.*`/`plan.*` events
  arrive on the SSE feed. The DAG is laid out top-down by dependency depth and
  rendered as inline SVG with the page's textContent-only discipline, so it
  inherits the strict CSP (no external script, no innerHTML sink) — no build
  chain, no new dependency. Backend: two JSON-body routes (`/api/plan/generate`,
  `/api/plan/refine`) for payloads too large for a query string, a streaming
  `/api/plan/run` that drives `CmdPlan` to completion while the browser watches
  progress live, and read routes `/api/plan_history` + `/api/plan_stats`. Same
  allowlist discipline as the rest of the surface: POST-only mutations, a body
  size cap, and only named keys ever reach the control plane. (M564)
- **Matrix is now a first-class messaging channel** (sixth, after Telegram, Slack,
  Discord, webhook, and email). A new `plugins/channels/matrix` adapter speaks the
  Matrix Client-Server API v3 over `net/http` only — no SDK, no new dependency. It
  long-polls `GET /_matrix/client/v3/sync` (resuming from a `since` cursor, priming
  past backlog on start), dispatches inbound `m.room.message` / `m.text` events
  through the shared `channel.Guard`, and replies via
  `PUT /_matrix/client/v3/rooms/{id}/send/m.room.message/{txn}` with a fresh ulid
  transaction id per chunk. Self-messages are skipped by resolving the bot's own
  MXID through `/account/whoami` (no echo loop). Rooms are gated by a fail-closed
  `channel.Allowlist`; long replies are split with `channel.SplitText`; the bearer
  token is scrubbed from errors. Wired into the daemon as `buildMatrix` — gated on
  `AGEZT_MATRIX_HOMESERVER` + `AGEZT_MATRIX_TOKEN`, allowlist via
  `AGEZT_MATRIX_ROOMS` — and surfaced through `agt status` and the config snapshot.
  Text-only for now; image/file events are a documented follow-up. (M563)

### Security
- **The webhook replay guard no longer forgets every recent id at once.** The
  dedup set of recently-seen message ids was cleared wholesale when it filled, so a
  captured, validly-signed body could be replayed right after a flush and accepted
  as new — within the freshness window when the client sends `ts_ms`, or
  unconditionally when it doesn't (the freshness check only runs for a supplied
  timestamp, so with none the dedup set is the sole replay guard). Replaced the
  wholesale clear with a two-generation rotation (live + previous): a key is
  forgotten only after it ages out of both generations, roughly doubling the
  remembered window with memory still bounded at 2×cap. (M457)

### Security
- **Known advisory: build with Go ≥ 1.26.4.** `govulncheck` flags two standard-library
  CVEs reachable from Agezt under go1.26.3 — GO-2026-5039 (`net/textproto`, via the
  email channel / journal MIME scan / mcpbridge) and GO-2026-5037 (`crypto/x509`, via
  the slack listener). Both are fixed in the go1.26.4 toolchain; the fix is in the
  compiler-shipped stdlib, so there is nothing to change in Agezt's code or
  `go.mod`/`go.sum` — releases and CI should simply build with go ≥ 1.26.4, after
  which `govulncheck ./...` reports zero. (M487)
- **`gitleaks detect` is now clean (0, was 16) and the secret gate is enforceable.**
  The full-history scan reported 16 hits, all deliberate test fixtures (the
  `kernel/redact/*_test.go` redaction tests, `cmd/agezt/plugin_log_test.go`, and the
  placeholder AWS creds in `kernel/creds/aws_test.go`) — no real secret is committed.
  Standing noise made the gate useless (a future real leak would be indistinguishable),
  so added `.gitleaks.toml` that keeps the full default ruleset (`useDefault`) and
  allowlists *only* those three test paths. Any secret introduced in production code,
  or in any other test, still trips the scan. (M486)

### Fixed
- **Plugin `Reload` no longer lets the dying read loop mark the fresh child dead.** `Reload`
  tore down the old child (`Close`) then reused the struct in `respawn`, but never joined the
  old `readLoop` goroutine. That goroutine, on the closed old pipe, called `markDead("read
  stdout: file already closed")`; if it landed after `respawn` reset `dead`/`deathErr`, it
  marked the *new* child dead and `initialize` failed with a phantom `connection lost`
  (intermittent; only the race detector's scheduling exposed it). Added a per-loop `readDone`
  channel and made `Reload` wait for the old loop to exit before respawn — also closing a
  lock-free `p.stdout` read/reassign data race. Found via the CI race job. (M561)
- **OpenAI-compatible `/v1/chat/completions` streaming no longer drops the answer for a
  non-streaming provider.** `streamChat` relays the kernel's `llm.token` events as content
  deltas, but a provider that implements `Complete` without `CompleteStream` (non-streaming)
  emits no token events — so a `stream:true` request served by such a provider returned only
  the `role` + `stop` chunks and silently dropped the answer, while the same provider via
  non-stream chat returned it. `/v1/responses` already guarded this (`full.Len() == 0` →
  emit the assembled answer); chat/completions now does the same. Found by driving the real
  daemon end-to-end with the echo mock (the new runtime/E2E acceptance dimension). Verified
  by unit negative control + e2e (the content delta now appears). (M550)
- **`FormatUSD` no longer drops the minus sign on sub-dollar negative amounts.**
  `planner.FormatUSD` abs-ed the fractional part to handle negatives, but for an amount
  whose magnitude is under $1 the whole-dollar part is `0`, so the sign lived only in the
  fraction — abs-ing it without recording the sign printed `-$0.50` as `"$0.5000"`. Now the
  sign is captured up front and re-applied as a prefix, so `FormatUSD(-500_000_000)` is
  `"$-0.5000"`. Latent today (all callers pass non-negative cost sums) but the exported
  contract is now correct. Found while triaging a surviving mutant on the abs guard during
  the `kernel/planner` mutation pass (score 0.731). (M517)
- **`DiskUsage` no longer breaks the FreeBSD build, and the daemon builds on every
  supportable OS.** `kernel/pulse/diskusage_unix.go` was tagged `//go:build !windows`
  (claiming all non-Windows platforms) but multiplied `syscall.Statfs_t.Bavail` —
  which is `uint64` on Linux/Darwin yet `int64` on FreeBSD — by a `uint64`, a compile
  error, so `GOOS=freebsd go build ./...` failed. Widened every operand to `uint64`
  explicitly, narrowed the constraint to `linux || darwin || freebsd` (the
  `syscall.Statfs` family), and added a `diskusage_other.go` fallback that returns a
  tolerated "not supported" error for the rest (OpenBSD names the fields differently;
  NetBSD has no `syscall.Statfs` and we stay stdlib-only). Cross-compile matrix is now
  green for linux/darwin/windows/freebsd (+ openbsd/netbsd compile). Found by adding a
  build matrix to the verification battery. (M488)

### Code quality
- **Hardening rubric ratified — "harden Agezt to 100%" goal MET.** The project owner ratified
  `.project/HARDENING.md` as-is (2026-06-06) as the binding definition of "100% hardened".
  Against that definition every PASS criterion holds and the MEASURED mutation floor (every
  non-equivalent mutant killed) is met across 47 packages; the sole exception (offline
  govulncheck) is environment-bound and remediated in CI. The full static re-verify battery
  (gofmt/vet/staticcheck/build/cross-compile/gitleaks/tests) was re-run green tree-wide at the
  arc HEAD. Closes the M490–M549 hardening arc. (M549)
- **Pinned the Slack replay-guard dedup eviction boundary.** The Slack channel drops replayed
  events via a bounded recently-seen-keys set that evicts the oldest key once the ring exceeds
  capacity (so an event flood can't grow it unbounded). The integration replay test never
  inserts enough keys to reach the eviction branch, so `len(ring) > cap → >= cap` (shrinks the
  window) and evicting the wrong slot (`ring[0] → ring[1]`, desyncing `ring` and `seen`) both
  survived. Added `TestSlack_DedupEvictsOldestPastCap` (unit, cap 3); negative control kills
  both (dropping `delete(seen,old)` doesn't compile). No code change. (M548)
- **Pinned the inbound media-download size caps on all three media channels.** Telegram,
  Discord, and Slack each download an attachment from an untrusted source and inline it for a
  vision model, bounded by `io.ReadAll(io.LimitReader(body, MaxRaw+1))` then `if len > MaxRaw`.
  The happy-path image tests use tiny bodies, so two mutation points survived per channel: the
  inclusive boundary (`> MaxRaw → >= MaxRaw`, which would reject a legitimate exactly-max
  upload) and — more dangerously — the load-bearing `+1` (`LimitReader(_, MaxRaw+1) → MaxRaw`,
  which would let an oversized body read as exactly MaxRaw and slip through silently truncated,
  defeating the DoS guard). Added a `Test*SizeCapBoundary` per channel (exactly-max accepted,
  max+1 rejected); negative control kills all six. Same read-bounded idiom family as M509/M531/
  M538/M542. No code change. (M547)
- **Mutation-hardened `strutil.Ellipsis` against the non-positive-max panic edge.** The
  daemon-wide rune-safe truncation helper documents "a non-positive maxBytes yields just the
  marker — never a panic", but its test exercised only `0` and `-5` with a non-empty string.
  go-mutesting surfaced that `maxBytes == -1` and the empty-string + negative cap are
  untested: mutating the `cut < 0` clamp or the `cut > 0` rune-backing loop bound leaves
  `cut` negative and panics on `s[:cut]` / `s[0]`. Added `Ellipsis(_, -1, …)` and
  `Ellipsis("", -1/-3, …)` assertions (kills 4 genuine survivors; the other 2 are equivalent
  no-ops at `cut == 0`, confirmed). First `internal/` mutation target. No code change. (M546)
- **Mutation-tested the web UI — security surface verified solid.** `kernel/webui` (the
  only kernel package not yet mutation-assessed) serves the operator dashboard + token-authed
  JSON API/SSE over loopback. `go-mutesting` scored 0.578 (52/90 killed); every one of the 38
  survivors was classified and **none touches the security surface** — the token gate,
  constant-time compare, per-route arg allowlist, and path guard are all killed by existing
  tests. Survivors are equivalent (unasserted tuning constants: read timeout, SSE buffer,
  heartbeat, nonce length) or cosmetic error-path (DetectContentType-equivalent header Sets,
  BadGateway error bodies, SSE-loop teardown). Completes kernel mutation coverage. No code
  change. (M545)
- **Mutation-hardened the notify tool's empty-kind prune.** `Bind` drops any channel kind
  with no allowlisted recipients (`len(ids) > 0`) so an unusable kind is never advertised to
  the model. The test bound an empty kind and asserted only `IsError` — but the correct
  "not configured" outcome and the mutant's wrong "notify failed (zero recipients)" outcome
  are both `IsError`, so `> 0 → >= 0` survived. Strengthened
  `TestNotify_UnboundReportsNotConfigured` to require the precise "not configured" message
  and that no send was attempted; negative control killed. Completes the `plugins/tools/`
  mutation sweep (coding verified covered). No code change. (M544)
- **Mutation-hardened the browser tool's one-level wildcard (SSRF allowlist).**
  `plugins/tools/browser`'s host allowlist (an SSRF boundary, re-checked per redirect hop)
  matches `*.example.com` exactly one label deep via a dot-count guard
  (`Count(host,".") == Count(pattern,".")`) — stricter than the http tool's any-depth
  wildcard. The test covered apex-denied and one-level-allowed but not a multi-level
  subdomain, so removing the dot-count guard left every test green while `a.b.example.com`
  would match `*.example.com` — silently widening a one-level allowlist to arbitrary depth.
  Extended `TestInvoke_WildcardHostMatch` to require `a.b.example.com` be denied; negative
  control (guard → constant true) is killed. No code change. (M543)
- **Mutation-hardened the acpagent output cap (untrusted external-agent relay).**
  `plugins/tools/acpagent` relays a streamed answer from an *untrusted external ACP agent*
  and bounds it twice so a runaway peer can't OOM the daemon (M256): an in-stream
  accumulation guard (`answer.Len() >= MaxOutputBytes`) and the final `truncate`
  (`len(s) <= max`). The existing runaway test allowed "a chunk or two" of slack and no
  test fed `truncate` a string of length exactly `max`, so both inclusive edges survived
  (`>= → >` appends one chunk past the cap; `<= → <` tears a `truncated 0 bytes` footer onto
  output that exactly fits). Added `TestACPAgent_RunawayGuard_StopsExactlyAtCap` (streams
  exactly the cap + one chunk → result is exactly the cap, no footer) and
  `TestTruncate_InclusiveMaxBoundary`; negative control kills both. Same inclusive-max
  DoS-guard idiom as plugin readFrame (M509), control-plane readBoundedLine (M531), and
  mcpbridge (M538). No code change. (M542)
- **Verified the federation loop guard's client side (peer tool).** The mesh delegation
  loop guard (M209) has two sides; M513 pinned the server (restapi refuses an inbound run
  past the hop limit). This verifies the client: the `peer` remote_run tool refuses to
  delegate at `Hop(ctx) >= maxHops` and forwards `hop+1`. Negative control killed the guard
  `>= → >`, the increment `+1 → +0` (a non-incrementing hop = unbounded chain), and `+1 → +2`.
  The two sides are consistent (no off-by-one). The cross-node runaway protection is now
  verified end to end. No code change. (M541)
- **Verified all inbound channel authorization gates.** Every channel (telegram, discord,
  slack, webhook, email) gates "who may drive the agent" on the verified
  `kernel/channel.Allowlist` (M511), fail-closed. Negative control on each gate
  (`if !allowed → if allowed`, which would let a non-allowlisted sender drive the agent and
  refuse the allowlisted one) is killed in all five suites; telegram's unauthorized-photo
  -fetch guard (`allowed &&`) is killed too. Signature verifiers (discord Ed25519, slack
  signing secret, webhook HMAC) are separately fuzzed (M533). Completes the
  authorization-surface verification alongside the control-plane (M529/M530) and REST/OpenAI
  (M513) token gates. No code change. (M540)
- **Verified provider usage/billing token math (cost-accounting sweep).** Completing the
  surface where M517 found a real money bug: the provider-side token→usage extraction that
  feeds every cost calc. anthropic sums three separate fields
  (`input + cache_read + cache_creation`) — negative control killed dropping either cache
  term and the `+ → -` flip (both streaming + non-streaming tests assert distinct per-term
  values); openai is a direct mapping (`prompt_tokens` already includes the cached subset),
  asserted with concrete values. Both solid. The full money path (governor CostMicrocents,
  agent cost cap, openaiapi estimateUsage, planner FormatUSD, provider usage) is now covered.
  No code change. (M539)
- **Mutation-hardened the MCP-bridge frame cap.** `plugins/external/mcpbridge`'s
  `readBoundedLine` (M185) caps a frame from an untrusted MCP server (stdio/SSE); the tests
  covered under-max and over-max floods but no frame exactly on the cap, so `> max → >= max`
  survived — a legitimate max-size payload (e.g. 16 MiB) torn down as "frame too large".
  Added `TestReadBoundedLine_ExactlyMaxAccepted`. This is the third copy of the identical
  bounded-read DoS guard (plugin readFrame M509, control-plane readBoundedLine M531), all
  now pinned. (M538)
- **Mutation-hardened the shell tool's negative-timeout fallback.** `plugins/tools/shell`
  delegates execution to warden (verified M495); its own timeout precedence
  (`in.TimeoutMS > 0`) was unpinned at negatives, so `> 0 → != 0` survived — a malformed
  negative `timeout_ms` would be forwarded as a negative duration to warden, which can read
  as "no deadline" and silently disable the timeout runaway-guard. Added
  `TestShell_NegativeTimeoutMSFallsBackToDefault`. (M537)
- **Mutation-hardened the http tool's request-body cap.** `plugins/tools/http`'s SSRF core
  (host-allowlist exact + `*.subdomain` wildcard, scheme/method limits, netguard egress,
  per-redirect-hop re-check) is verified solid by negative control; the genuine gap was the
  inclusive body cap — `TestBodyTooLarge` used `Max+1`, so `len(body) > Max → >=` survived
  (a body of exactly 256 KiB wrongly rejected). Added `TestBodyExactlyAtMax`. Same class as
  the plugin readFrame (M509) / control-plane readBoundedLine (M531) guards. (M536)
- **Mutation testing reached the `plugins/` tree (file tool).** The plugin tree (tools,
  channels, providers) had been fuzzed but never mutation-tested. First target
  `plugins/tools/file`: its path-containment security core (`withinRoot`/`resolve` — no
  `..`/symlink escape) is verified solid by negative control, and a usability edge was
  pinned — a single-line read range (`start == end`, "read lines 5-5") was wrongly
  rejected because `end < start → <=` survived (no test sat on `start == end`). Added a
  `[3,3]` case to `TestRead_LineRange`. (M535)
- **Full rubric re-verification after the hardening arc.** Re-ran the complete
  offline-verifiable battery tree-wide after 44 commits: gofmt (committed LF blobs) clean,
  `go vet ./...` 0, `staticcheck ./...` clean, `gitleaks` no leaks (602 commits scanned),
  cross-compile green for linux/{amd64,arm64} + darwin/arm64 + windows/amd64 + freebsd/amd64,
  full `go test ./...` 0, and all 16 fuzz targets clean (M533). Every PASS dimension of the
  six-criterion rubric holds with a current measurement; `go.mod`/`go.sum` unchanged across
  the arc. No code change. (M534)
- **Re-verified all 16 fuzz targets clean.** Every untrusted/external/binary parser (7 kernel
  + 9 plugin: provider stream parsers, channel HMAC verifiers, AWS event-stream framing) was
  re-fuzzed (`GOMAXPROCS=3`, 8s/target) after the M509–M532 arc — no crashers, no new corpus
  seeds. Re-validates the M496 baseline with a current measurement. (M533)
- **Mutation-pinned the `runs list` cost-band floor edge.** The cost-band filter (M125) keeps
  runs spending `≥ min and ≤ max`, but `TestRunsList_CostBandFilter` tested the ceiling at its
  exact edge (a 100-spend run kept against `max=100`) while testing the floor only strictly
  below a run's spend — so `SpentMicrocents < minCostMC → <=` survived, silently dropping a
  run that spent exactly its `--min-cost`. Extended the test with an exact-floor case. (M532)
- **Mutation-pinned the control-plane request-size limit boundary.** `readBoundedLine` (the
  M188 pre-auth DoS guard) caps a request line at `len(buf)+len(chunk) > max`, inclusive, but
  the tests covered only under-cap and a flood well over it (and the fuzz invariant only
  checks `len <= max`), so `> → >=` survived — a request exactly filling the cap wrongly
  rejected as too large. Same shape as the plugin readFrame gap (M509). Added
  `TestReadBoundedLine_ExactlyMaxAccepted`. (M531)
- **Verified the control-plane tenant command-allowlist (privilege boundary).** Extended the
  M529 control-plane verification to the second auth primitive, `tenantTokenAllows` (the
  deny-by-default list of commands a scoped tenant token may run). Both directions are killed
  by the existing integration tests: the allow case `true → false` fails
  `TestTenantToken_Authorizes…/AllowsOwn…`, and the dangerous default `false → true` (a tenant
  able to run admin commands) fails `TestTenantToken_ForbidsNonAllowlistedCmd`. The privilege
  boundary is genuinely pinned; no code change. (M530)
- **Mutation-verified the control-plane primary-token auth gate (rigorous).** `controlplane`
  is too large (~10k LOC) for a whole-package `go-mutesting` run, so its security core,
  `tokenIsPrimary` (constant-time admin-token check, M187), was verified by hand-applied
  negative control: `want == "" → !=`, `presented == "" → !=`, and `ConstantTimeCompare(...)
  == 1 → != 1` are all killed by `auth_test.go`; the `|| → &&` guard survivor is equivalent
  (ConstantTimeCompare's length-mismatch and the both-empty case make `&&` behave
  identically). Upgrades the prior informal "verified out-of-band" note to a reproducible
  result. No code change. (M529)
- **Mutation testing pinned the agent per-run cost cap boundary.** `kernel/agent`'s per-run
  spend cap (M166) terminates at `spentMicrocents >= cap`, but `runcost_test.go` only spends
  strictly over the cap (2000 vs 1500), never exactly at it, so `>= → >` survived — a run
  spending exactly its budget would run one more over-budget round before being caught. Added
  `TestRun_PerRunCostCap_ExactlyAtCap`. The loop guard and max-iter were already edge-pinned.
  Thirty-fifth package in the mutation pass. (M528)
- **Mutation testing pinned openaiapi's word-count usage fallback.** `estimateUsage` (used
  when the engine reports no real provider token counts) had its `total_tokens: p + c`
  arithmetic unpinned — the main usage test uses a `UsageReporter` engine, hitting
  `chatUsage`'s `pt + ct` instead, so `+ → *` and `+ → -` survived (a wrong usage/billing
  total for clients relying on the heuristic). Added a direct `TestEstimateUsage_WordCount`.
  The request/parse/auth surface was already well covered (fuzz + 7 test files). Thirty-fourth
  package in the mutation pass. (M527)
- **Mutation testing pinned pulse's salience disposition-band boundaries.** `kernel/pulse`'s
  `dispositionForValue` (LLM score → Alert/Notify/Digest/Drop band) was exercised only
  indirectly, never at its exact thresholds, so `v >= 0.85`, `v >= 0.45`, and `v >= 0.20`
  could each weaken to `>` — a score landing exactly on a band edge would silently drop a
  notch (alert→notify, notify→digest, digest→drop). Added
  `TestDispositionForValue_BandBoundaries` (each edge + just-below). `Route` was already
  exhaustively tested. Thirty-third package in the mutation pass; the salience novelty-TTL edge, DiskObserver thresholds, and QuietHours.Active window edges were pinned in follow-ups. (M523-M526)
- **Mutation testing pinned tenantctx's empty-id no-op as context identity.** `WithTenant`'s
  early `return ctx` for an empty id could be dropped — falling through to
  `WithValue(ctx, key, "")` — and `Tenant` still returns `""`, so the value-only test
  couldn't tell them apart. The contract is "returned unchanged"; the mutant allocates a
  wrapper on every untenanted (primary-kernel) run. Strengthened `TestWithTenant_EmptyIsNoOp`
  to assert identity (`WithTenant(base,"") == base`), taking the package to a full mutation
  kill. Thirty-second package in the mutation pass. (M522)
- **Mutation testing pinned meshctx's MaxHopsConfig diagnostic returns.** Every test went
  through `MaxHopsFromEnv`, which discards the `raw` and `validOverride` returns of
  `MaxHopsConfig`, so those were unpinned — all three `validOverride` results could flip
  undetected. `validOverride=false` is what `agt doctor` uses to flag a typo'd hop-limit
  override that silently fell back; a stuck-true flag would hide the misconfiguration.
  Added `TestMaxHopsConfig_RawAndValidity` (all three returns across unset/valid/over-cap/
  zero/garbage/whitespace). Thirty-first package in the mutation pass. (M521)
- **Mutation testing pinned reflect's proposal-rule thresholds.** `kernel/reflect`'s
  `proposals` gates three advisory rules on inclusive thresholds, but the existing tests
  fire them only well past the edge, so `ApprovalsDenied-ApprovalsGranted >= denyExcess` and
  `TasksFailed*2 >= TasksStarted` (the ≥50%-failure rule) could each weaken `>= → >`
  undetected — a deny-excess or failure batch *exactly* at the trigger point would stop
  being proposed. Added `TestProposals_ExactThresholds` (fires at the threshold, silent one
  below). Thirtieth package in the mutation pass. (M520)
- **Mutation testing verified the artifact content-addressed store solid (no gap).** A
  hand-applied negative control on every meaningful operator in `kernel/artifact` — the
  `validRef` path-traversal guard (length + all four hex range edges + the De Morgan
  reject structure), `Get`'s corrupt-detection compare, `Put`'s dedup skip, and `pathFor`'s
  shard width — confirmed each is killed by the existing tests. The 31 go-mutesting
  survivors are equivalent (error-path cleanup / `fmt.Errorf` wrapping removals). No code
  change; recorded as verified solid (like anomaly/netguard). Twenty-ninth package. (M519)
- **Mutation testing pinned ULID's decode table as the alphabet's inverse.** `kernel/ulid`'s
  `decodeChar` is only exercised on the few characters in the fixed test vectors, so most of
  its return values were unpinned — the `P–T` (+22) and `W–Z` (+28) offsets and the
  `J`/`K`/`M`/`N`/`V` mappings could each be off by one, silently corrupting `Timestamp()`
  for any ULID whose timestamp encodes those chars. Added `TestDecodeChar_InverseOfAlphabet`
  (`decodeChar(alphabet[i]) == i` for all i; Crockford exclusions I/L/O/U rejected, not
  aliased). Twenty-eighth package in the mutation pass. (M518)
- **Mutation testing pinned the state namespace allowlist edges.** `kernel/state`'s
  `validateNamespace` (the only path-traversal guard) was tested for rejections and for
  low-edge valid chars, but no valid namespace used the far range edges, so `c <= 'z'`,
  `c <= 'Z'`, `c >= 'A'`, and `c <= '9'` could each weaken (`<= → <`, `>= → >`) and
  silently reject a valid identifier (`z`/`Z`/`A`/`9`) undetected. Added `"azAZ09"` to the
  accepted-namespace cases. Traversal rejections + the M426 poison guard were already
  solid. Twenty-sixth package in the mutation pass. (M515)
- **Mutation testing pinned the ACP prompt-flattening block selection.** Every `kernel/acp`
  test sent a single `{"type":"text"}` block, so `flattenPrompt`'s newline join and its
  lenient `b.Type == "" && b.Text != ""` branch were unpinned — `== → !=`, `!= → ==`, and
  `&& → ||` all changed which content blocks were folded into the intent undetected (a
  non-text/image block's text could leak in; an omitted-type text block could be dropped).
  Added `TestFlattenPrompt_BlockSelection` (multi-block: text/typeless/image/empty/text →
  `"one\ntwo\nthree"`). The JSON-RPC notification + auth paths are defended-in-depth
  (equivalent survivors). Twenty-fifth package in the mutation pass. (M514)
- **Mutation testing pinned the REST mesh hop-limit loop guard.** The federation loop
  guard in `kernel/restapi` (`hopIn > maxHops` → 508 Loop Detected, M209) had no
  REST-layer test, so `> maxHops → >= maxHops` (refuse a run at exactly the limit) and
  `→ < maxHops` (never refuse — a federated mesh could recurse unbounded) both survived.
  Added `TestSubmitRun_MeshHopLimit` (hop>limit → 508; hop==limit → 200 and threaded into
  the run). The token-auth core was separately verified solid (constant-time compare,
  empty-token fail-closed, per-tenant gate all killed). Twenty-fourth package in the
  mutation pass. (M513)
- **Mutation testing verified the anomaly circuit breaker solid (no gap).** A hand-applied
  negative control on every meaningful operator in `kernel/anomaly` — the trip boundary
  `count > max`, the window sign/prune bound/inclusive `.Before`, the `Enabled` gate, and
  the monitor's tool-kind filter, trip latch, and start gate — confirmed each is killed by
  the existing tests. The 23 go-mutesting survivors are equivalent mutants. No code change;
  recorded as verified solid (like netguard/event). Twenty-third package in the mutation
  pass. (M512)
- **Mutation testing pinned the channel splitter's empty-piece guard.** `go-mutesting`
  on `kernel/channel` showed `SplitText`'s cut trigger (`units+ru > limit && len(cur) > 0`)
  was unpinned at the empty-buffer guard — no test used a limit smaller than a single
  character, so `len(cur) > 0 → >= 0` survived. Under it `SplitText("😀😀", 1)` emits a
  blank leading chunk (a channel would send an empty message). Added
  `TestSplitText_NeverEmptyPiece` (sub-character limits → no empty piece, lossless rejoin).
  The package's security core (fail-closed Allowlist, per-sender history isolation) was
  already solid; remaining survivors are equivalent. Twenty-second package in the
  mutation pass. (M511)
- **Mutation testing pinned the webhook 2xx success-window upper edge.** `go-mutesting`
  on `kernel/webhook` showed the delivery success test (`status >= 200 && status < 300`,
  duplicated in `ProbeResult.OK`) was unpinned at its upper edge — tests covered 200 and
  500 but never 299 vs 300, so `< 300 → <= 300` survived on both copies (a status 300,
  which Go does not auto-follow, wrongly counted as delivered instead of retried/failed).
  Added `status_boundary_test.go`: an OK table over 199–500 and a dispatch test asserting
  a 300 is journaled `webhook.failed`. Twenty-first package in the mutation pass. (M510)
- **Mutation testing pinned the plugin host's frame-size boundary.** `go-mutesting` on
  `kernel/plugin` showed `readFrame`'s OOM-flood guard (`len(buf)+len(chunk) > max`) was
  unpinned — `frame_test.go` covered under-max, over-max, and EOF, but never a frame
  sitting exactly on the inclusive limit, so `> → >=` survived (a maximum-size frame
  wrongly rejected as `errFrameTooLarge`). Added `TestReadFrame_ExactlyMaxAccepted`
  (exactly `max` accepted, `max+1` rejected). Twentieth package in the mutation pass. (M509)
- **Mutation testing pinned catalog's cross-provider down-route tie-break.** `go-mutesting`
  on `kernel/catalog` showed `ToolCapableAlternativeAmong`'s cross-provider selection
  (`ctx > bestCtx || (ctx == bestCtx && id < bestID)`) was unpinned — the cross tests only
  covered largest-context, never equal context across providers, so the tie-break direction
  and the context comparison could flip undetected (non-deterministic / wrong down-route).
  Added `TestToolCapableAlternativeAmong_TieBreaksByIDAcrossProviders` (two arrangements).
  Nineteenth package in the mutation pass. (M508)
- **Mutation testing pinned standing's cron dom/dow OR rule.** `go-mutesting` on
  `kernel/standing` showed `matchesCron`'s classic both-restricted day rule
  (`domMatch || dowMatch`) was unpinned — every existing case left day-of-month as `*`,
  so a `||`→`&&` regression (requiring both DOM and DOW to match instead of either, the
  wrong cron semantics) survived. Extended `TestMatchesCron` with both-restricted cases
  (`0 8 13 * 5` matching on the 13th and on Fridays). Eighteenth package in the mutation
  pass. (M507)
- **Mutation testing pinned skill's auto-quarantine failure-rate threshold.** `go-mutesting`
  on `kernel/skill` showed `maybeAutoQuarantine`'s `if rate < f.aqFailureRate` was unpinned
  at the boundary — the tests drive 100% and ~23% rates, never exactly the threshold, so a
  `<`→`<=` regression (a skill at exactly the failure rate escaping quarantine) survived.
  Added `TestRecordOutcome_QuarantinesAtExactFailureRate` (3 successes then 3 failures →
  exactly 50% → quarantined). Seventeenth package in the mutation pass. (M506)
- **Mutation testing pinned memory's first-writer-wins record provenance.** `go-mutesting`
  on `kernel/memory` showed the reinforce path's provenance preservation (`rec.SourceEvent
  = existing.SourceEvent` + the `ev != nil && rec.SourceEvent == ""` guard) was unpinned —
  the test only checks creation sets provenance, so two mutants (dropping the copy;
  `&&`→`||`) that overwrite a record's origin event with the latest mention survived. Added
  `provenance_test.go` (re-remember preserves the original SourceEvent), mirroring
  worldmodel M503. Sixteenth package in the mutation pass. (M505)
- **Mutation testing pinned approval's default-timeout guard.** `go-mutesting` on
  `kernel/approval` showed `New`'s `if timeout <= 0 { timeout = DefaultTimeout }` was
  unpinned — every test passes an explicit Timeout, so a `<=`→`<` regression (leaving an
  unset/zero Timeout at 0, which auto-denies every approval instantly) survived. Added a
  white-box `TestNew_DefaultsUnsetTimeout`. Fifteenth package in the mutation pass. (M504)
- **Mutation testing pinned worldmodel's first-writer-wins entity provenance.**
  `go-mutesting` on `kernel/worldmodel` showed `Upsert`'s `ev != nil && e.SourceEvent ==
  ""` (set the origin event once) was unpinned for re-observation — a `&&`→`||` regression
  overwrites SourceEvent on every later mention (last-writer), losing the origin used for
  audit/causation. Added `provenance_test.go` (re-observe preserves the original
  SourceEvent). Fourteenth package in the mutation pass. (M503)
- **Mutation testing pinned tenant List's spurious-entry exclusion.** `go-mutesting` on
  `kernel/tenant` showed `List`'s `!e.IsDir() || !ValidID(name)` filter was unpinned (the
  existing test only creates valid tenants) — a `||`→`&&` regression would surface a stray
  file or an invalid-named directory as a "tenant". Added
  `TestRegistry_ListExcludesSpuriousRootEntries`. The `Authorize` auth gate's survivor was
  verified equivalent (the constant-time compare rejects an empty token regardless).
  Thirteenth package in the mutation pass. (M502)
- **Mutation testing pinned runtime's foldRunTools correlation isolation.** `go-mutesting`
  on `kernel/runtime` showed the memory-distillation fold's filter (`e.CorrelationID !=
  corr || e.Kind != KindToolResult`) was unpinned — a `||`→`&&` regression folds other
  runs' tool results into a run's distilled memory transcript (cross-run contamination).
  Added `foldruntools_internal_test.go`. The `WithTrustCeiling` survivor was verified
  equivalent. Twelfth package in the mutation pass. (M501)
- **Mutation testing pinned the cadence due-check firing boundary.** `go-mutesting` on
  `kernel/cadence` (the scheduler) showed `Due`'s `now < NextRunUnix → skip` boundary was
  unpinned: the existing test probes `now < nextRun` and `now = nextRun+1s` but never the
  exact instant, so a `<` → `<=` regression (delaying every entry by one tick) survived.
  Added `TestStore_Due_FiresAtExactScheduledTime` (now == NextRunUnix is due). Eleventh
  package in the mutation pass. (M500)
- **Mutation testing pinned the bus subject-matcher over-delivery edge.** `go-mutesting`
  on `kernel/bus` (the event backbone) showed `matches` only required the *subject* to be
  fully consumed, not the *pattern*: dropping the `pi == len(pattern)` half let a pattern
  with more tokens than the subject wrongly match (`matches("a.b.c","a.b") → true`), so a
  subscriber to a more-specific subject would receive shorter events (over-delivery). The
  existing table had no longer-pattern-than-subject case; added three. Tenth package in
  the mutation pass. (M499)
- **Mutation testing pinned the scheduler's plan correlation-id generation.**
  `go-mutesting` on `kernel/scheduler` (score 0.774, highest assessed) showed the
  auto-generated plan correlation id (`"plan-"+ulid`, used as `PlanResult.PlanID` and
  stamped on every plan/node journal event) could be removed undetected — many tests
  pass an empty id but none asserted the generated one, so an auto-correlated plan run
  would emit events with an empty correlation id, breaking `agt why` / audit
  correlation. Added `correlation_test.go` (generates when empty, preserves when
  provided). Ninth package in the mutation pass. (M498)
- **Mutation testing pinned the governor's spend-enforcement boundary.** `go-mutesting`
  on `kernel/governor` (the per-day/per-task spend ceiling) showed both `spentToday >=
  ceiling` and `spent >= cap` were unpinned at the exact boundary — the existing budget
  tests overshoot (first call blows past the cap), so the `>=` → `>` mutants survived (a
  regression allowing one extra call once spend reaches the ceiling). Added
  `budget_boundary_internal_test.go` pinning spend == ceiling/cap → blocked and
  ceiling-1 → allowed. Eighth package in the mutation pass. (M497)
- **Active fuzz robustness pass — all 16 targets re-run clean.** Rather than rely on
  the historical "clean" claim, every fuzz target (controlplane request parse, redact,
  edict decide, journal open, catalog, openai-compat content, governor pricing, the
  three channel signature verifiers, and six provider stream parsers) was re-executed
  under bounded time: all exit 0, no crashers written. Fuzz/test runs are now capped at
  `GOMAXPROCS=3` to avoid saturating the CPU. (M496)
- **Mutation testing pinned the warden's blank-argv0 rejection.** `go-mutesting` on
  `kernel/warden` (the command sandbox) showed the second half of the empty-Argv guard
  (`spec.Argv[0] == ""`) was unpinned — the existing test only covered nil Argv, so the
  `spec.Argv[0] == "" → false` mutant survived, which would let a blank command reach
  `exec.CommandContext(ctx, "", …)`. Added `TestRun_RejectsBlankArgv0`. capBuffer (the
  output memory bound) was found exemplary; remaining survivors are equivalent/config
  mutants. Seventh package in the mutation pass. (M495)
- **Mutation testing pinned the legacy vault KDF (and strengthened PBKDF2).**
  `go-mutesting` on `kernel/creds` showed `deriveKeyLegacyHMAC` was unpinned — every
  test exercising it round-trips with the same function, so removing `mac.Write(d)`
  from the keyed-hash chain survived. The legacy KDF is frozen (it decrypts pre-M172
  vaults), so an undetected change makes those vaults unreadable. Added
  `kdf_known_answer_internal_test.go`: a golden-digest test for the legacy KDF
  (independent reimplementation) and a cross-check of `deriveKeyPBKDF2` against the
  stdlib `crypto/pbkdf2` (Go 1.24+, authoritative, no module dep added) covering
  empty/unicode cases. Completes the six-package mutation pass (redact/journal/edict/
  netguard/event/creds): genuine gaps closed where present, the rest verified solid. (M494)
- **Defined "hardened to 100%" as a measurable rubric (`.project/HARDENING.md`).** The
  hardening goal lacked any terminal, decidable criterion. Added a six-dimension
  scorecard (build/portability, static analysis, secrets/security, testing depth,
  defect surface, CI enforcement) where each row is verified by a command and carries
  its current state (PASS / MEASURED-with-floor / documented environment exception),
  plus a one-pass re-verify script. Offered for ratification. Also confirmed via a
  mutation pass that the netguard SSRF core is already solid (all security-line
  survivors equivalent/unreachable — no test added). (M493)
- **Mutation testing pinned the edict whitespace-normalizer contract.** `go-mutesting`
  on `kernel/edict` (the policy engine) showed the backward (left-side) scan in
  `stripPunctAdjacentWhitespace` — which strips spacing-evasion from hard-deny floor
  rules — was never exercised: the fork-bomb tests cover it only via `Decide`, and every
  optional space in `:(){ :|:& };:` has punctuation on its right, so the forward scan
  alone normalizes it. A left-only-punctuation variant could evade a floor rule if the
  backward scan regressed. Added `strip_whitespace_test.go` pinning the documented
  either-side contract (left-punct, right-punct, word-preservation, forward bound, fork
  bomb), with a manual negative control for both the backward-scan and forward-bound
  mutants. The toolmap and TrustLevel survivors were verified equivalent (no gap). (M492)
- **Mutation testing pinned two journal integrity gaps (rotation accounting, Tail
  trim).** `go-mutesting` on `kernel/journal` showed the existing rotation tests use
  tiny segment thresholds where one line already rotates, so a `curBytes += `→`=`
  regression (segments never rotating for normal events → unbounded growth) went
  undetected; and the cross-segment Tail test gathers exactly n, so the
  `collected[len-n:]` trim line never ran. Added `mutation_internal_test.go` with a
  self-calibrating accumulation-rotation test and a Tail-trim test. The journal's
  score is dominated by low-value error-message mutants, so the headline number moved
  little, but both real behavioral gaps are now killed. (M491)
- **Mutation testing hardened the redactor's test suite (score 0.575 → 0.725).**
  `go-mutesting` on `kernel/redact` (the secret-scrubbing chokepoint) found 17
  surviving mutants; 6 were genuine test gaps — nothing pinned the exactly-8-char
  literal length floor, that a leading too-short/duplicate value must not abort the
  registration loop, or the longest-first ordering that fully scrubs a secret which is
  a prefix of another. Added four tests (`redact_m490_test.go`); each would fail on a
  one-token regression that silently leaks a secret. The remaining 11 survivors are
  equivalent mutants (identical observable behaviour), so every non-equivalent mutant
  is now killed. (M490)
- **CI now enforces the cleaned gates.** Added a `lint` job (`gofmt -l`,
  `staticcheck ./...`, `govulncheck ./...`) and a `secrets` job (`gitleaks` over full
  history with the `.gitleaks.toml` baseline) to `ci.yml`, and added `freebsd/amd64`
  to the cross-build matrix (buildable as of M488). The static-analysis, secret-scan,
  vulnerability, formatting, and FreeBSD-build cleanliness from M485–M488 is now
  enforced on every push/PR instead of being a point-in-time snapshot. The full
  golangci-lint correctness sweep (bodyclose/nilerr/ineffassign/unconvert/gocritic/
  noctx/unparam/prealloc) surfaced no genuine defect — the nilerr hits are the
  tool-result convention and the deliberate skip-malformed-on-journal-fold idiom.
  (M489)
- **`staticcheck ./...` is now clean (0 findings, was 17).** Removed unnecessary
  comma-ok discards on map reads in the control plane (`req.Args[k]` returns one
  value — S1005 ×13 across edict/server/state + halt_resume test), converted an
  identical-shape struct literal to a type conversion in the SDK (`invokeResult(out)`
  — S1016), dropped a dead write in the budget test where the headroom value was
  immediately overwritten (SA4006), removed an unused mock field (`gotIter` — U1000),
  and merged a split var declaration in the netguard redirect test (S1021). All
  no-op semantically (the SA4006 fix keeps the test asserting exactly what it meant);
  full suite still green. `staticcheck` joins `go vet` as an enforceable gate. (M485)

### Fixed
- **The peer tool truncates long answers on a UTF-8 rune boundary.** A peer answer
  over the size cap was cut at a raw byte offset, splitting a multi-byte rune and
  emitting invalid UTF-8 to the model. It now backs up to a rune boundary, matching
  the browser and coding tools. (M468)
- **SSO credential requests now URL-escape the role name and account id.** The SSO
  `GetRoleCredentials` query was built by raw string concatenation, so a role name
  containing characters that are valid in IAM but special in a URL query (e.g. `+`,
  which decodes to a space) was sent corrupted and the credential fetch failed for
  those operators. The query is now built with `url.Values`. (M466)

### Performance
- **The plan scheduler's driver is event-driven instead of polling.** It busy-waited
  with a 1 ms sleep while any node was in flight — spinning (lock + map scan) for the
  whole duration of the longest node and capping scheduling latency at ~1 ms. It now
  blocks on a buffered completion channel signalled by each finishing node. Same
  behaviour, no busy-wait, no latency floor. (M472)

### Fixed
- **Vertex tool results with control bytes no longer break the request.** Vertex AI
  uses the Gemini wire format and had the identical `strconv.Quote` defect as the
  Google provider (M481) — a control byte such as ANSI `\x1b` produced invalid JSON
  and wedged the agent loop on Vertex. The result is now encoded with
  `encoding/json`. (M483)
- **Gemini tool results with control bytes no longer break the request.** The
  Google provider built the tool-result JSON with `strconv.Quote` (a Go quoter), so a
  control byte — notably the ANSI escape `\x1b` common in tool output — produced
  invalid JSON, failing the whole request encode and wedging the agent loop on
  Gemini. The result is now encoded with `encoding/json`. (M481)
- **The email Subject header strips a bare carriage return.** The subject (first
  line of the outbound text) was cut only at `\n`, so a lone interior `\r` survived
  into the `Subject:` line — a header-injection foothold against lenient mail
  parsers. It is now cut at the first CR or LF. (M479)
- **Inbound Telegram photos and caption-only messages are no longer dropped.** The
  long-poll loop dispatched only messages with non-empty `Text`, but a photo carries
  its text in `Caption` (or none at all), so photo/caption-only messages were skipped
  before reaching the handler — leaving the inbound-image (vision) path dead on the
  live poll path even though the handler fully supported it. The gate now admits
  messages with a caption or photo. (M476)

### Reliability
- **Bedrock-Mistral responses are always tagged with the assistant role.** The
  adapter copied the role from the wire, but OpenAI-shaped backends often omit it,
  leaving the canonical role empty and misclassifying the turn. It now hard-codes the
  assistant role like every sibling adapter. (M484)
- **An event copies a `json.RawMessage` payload instead of aliasing the caller's
  bytes.** The stored payload shared backing storage with the caller's slice, so a
  later mutation of that slice could silently diverge the payload from the hash
  computed over it and fail `VerifyHash`. The payload is now copied. (M482)
- **A duplicate live correlation id is rejected instead of corrupting the run
  registry.** Two concurrent `RunWith` calls sharing one correlation id clobbered
  `k.runs[corr]` — the second overwrote the first's cancel func and the first's
  cleanup deleted the survivor's entry, leaving a run uncancellable by Halt. The
  second call now returns an "already running" error. (M480)
- **Concurrent catalog sync + discover no longer lose each other's metadata.** The
  `meta.json` sidecar was updated with an unsynchronized read-modify-write and a
  shared temp file, so a concurrent `catalog sync` + `catalog discover` could clobber
  one side's timestamps/source. Meta updates are now serialized and each catalog file
  is written via a unique temp. (M478)
- **`Kernel.Close()` closes every store even if one fails.** It returned on the
  first store-close error, leaking the remaining handles — notably the journal's OS
  file descriptor, which on Windows blocks re-opening the directory. It now closes
  all stores and joins the errors. (M477)
- **The auto context-budget path reads the catalog under the lock.** `RunWith` read
  the `catalog` field directly while `ReloadCatalog` swaps it under the lock — a data
  race when a run starts during a `catalog sync`. It now uses the locked `Catalog()`
  accessor. (M477)
- **The warden runner no longer swallows engine failures on a non-zero exit.** Its
  `cmd.Wait` error classification was gated on the exit code being 0, so a genuine
  non-`ExitError` failure (a failed launch, an I/O error, a `WaitDelay` abandonment
  after a kill) that coincided with a non-zero exit was returned as success, hiding
  it from the caller. Classification is now purely type-based. (M475)
- **A blank tenant-token file no longer permanently wedges a tenant.** A crash
  between creating the token file and writing it left a zero-length file, after
  which every `Token()`/`Authorize()` re-read it as empty and the `O_EXCL` re-mint
  failed — so the tenant returned a blank credential forever and could never
  authenticate. A blank token file is now detected (after a brief retry for a live
  concurrent writer) and re-minted. (M474)
- **The credentials vault writes via a unique temp file.** `Save`/`Rotate` used a
  fixed `creds.json.tmp`; two concurrent `Save` calls (both under the read lock)
  could race on it and leave a torn, unloadable vault. Both now write to a unique
  temp (and fsync) before renaming. (M471)
- **The file tool's truncated read fills the preview and reports read errors.** The
  "first N bytes" preview of a large file used a single `Read`, which may return
  fewer bytes than requested, so the model could get a short prefix while the header
  claimed N bytes; a read error was also silently swallowed. It now loops with
  `io.ReadFull` and surfaces genuine errors. (M470)
- **The coding tool captures a timed-out agent's partial work instead of discarding
  it.** The post-agent `git add`/`git diff` reused the request context, so if the
  agent ran out the deadline those commands failed with "context deadline exceeded"
  and the partial diff was lost. They now run on a fresh bounded context; the
  agent's timeout is still reported alongside the diff. (M469)
- **The file tool's `replace` edit is now atomic.** It opened the target with
  `O_TRUNC` (zeroing it) before writing the new content, so a partial write
  (ENOSPC, crash) left the file empty or half-written and the original was lost —
  for the op explicitly meant to be low-clobber. It now writes to a temp file and
  renames it over the target, so the original survives intact until the complete
  new content is in place; the symlink-refusal guard is preserved. (M467)
- **AWS credential fetches (SSO/STS/web-identity) no longer hang daemon startup on
  a stalled endpoint.** These paths used `http.DefaultClient` (no timeout) with a
  background context (no deadline), so a black-holed STS/SSO endpoint could block
  the credential chain — and thus daemon boot — indefinitely, unlike the IMDS path
  which already bounds itself. Each now uses a 10 s-bounded client. (M465)
- **A plugin call no longer stalls until its timeout when the plugin dies mid-
  registration.** `callWithProgress` checked liveness lock-free, then registered its
  response channel under the lock; a `Close`/`markDead` in between drained `pending`
  before the channel was registered, so it was never closed and the caller blocked
  until its ctx deadline. Liveness is now re-checked under the lock, making
  registration and teardown mutually exclusive. (M464)
- **Journal segment creation now fsyncs the parent directory.** Creating/rotating a
  segment fsync'd the file's contents but not the directory entry, so on power loss
  a freshly rotated segment (and its durable-before-publish records) could vanish
  even though the file was synced. The parent directory is now fsync'd (best-effort)
  after a new segment is created in rotate/open/restore. (M463)
- **A failed journal fsync no longer wedges recovery with a duplicate sequence.**
  When a line was written but its fsync failed (EIO/ENOSPC), the in-memory sequence
  wasn't advanced — yet the line stayed in the segment, so the next append reused
  that sequence and wrote a second line for it. On the next open the duplicate
  tripped a chain break and the journal refused to boot (not a torn tail, so
  unrecoverable). The failed line is now truncated back to the last committed size
  (with a seek so emulated-O_APPEND platforms don't zero-fill the gap), and the
  append fails closed. (M462)
- **`controlplane.Server.Stop()` no longer hangs on in-flight streaming
  connections.** `Stop()` closed the listener but never cancelled the context its
  streaming handlers (run/pulse/plan) block on, nor closed accepted connections —
  so when `Stop()` was the shutdown trigger (rather than cancelling the Start ctx),
  `wg.Wait()` blocked until the per-connection deadline. The server now cancels a
  derived serving context on shutdown, releasing handlers promptly on either path.
  (M461)
- **A plugin can no longer deadlock its host slot by flooding stdout without
  draining stdin.** Host→plugin writes held the same mutex the read loop's response
  router needs, *across* the blocking write to the child's stdin. A plugin that sent
  a callback then stopped reading its stdin made the host's response write block
  while holding that mutex; the read loop then blocked routing the next frame,
  stopped draining stdout, the child's stdout pipe filled, and the write never
  completed — a permanent wedge leaking goroutines. Stdin writes now serialize on a
  dedicated mutex and never hold the routing lock across the write, so the read loop
  keeps draining and the cycle can't form. (M460)
- **Plan gate nodes no longer occupy a compute worker slot while awaiting
  approval.** The scheduler's semaphore bounds compute parallelism, but a
  `GateNode` blocking on a human decision consumed a slot for the whole approval
  window — so with `MaxParallel` waiting gates (or one at `MaxParallel: 1`) no other
  ready node could start until a human responded, stalling independent branches.
  Gate nodes now skip the semaphore (they block only on approval), and the slot is
  acquired inside the worker goroutine so launching a node never blocks the driver.
  Compute parallelism is still bounded. (M459)
- **Cron-triggered standing orders no longer launch after shutdown begins.** The
  cron ticker's `select` chooses at random when both cancellation and a tick are
  ready, so during teardown a tick could still be picked and dispatch fresh order
  goroutines — racing real work (a brief sent post-shutdown, a run touching a store
  being closed) against shutdown. `tickCron` now fires nothing once its context is
  cancelled, with a matching re-check in the ticker branch. (M458)
- **The governor's usage index no longer under-reports tokens after a rotation.**
  The in-memory per-correlation usage index (the fast path behind the API `usage`
  reporting field) was dropped wholesale when it hit its cap. A run still in flight
  when that fired lost its partial entry, and the same run's later calls then built
  a fresh zero-based entry — so `UsageFor` returned that PARTIAL sum with `ok=true`,
  a silent token under-count served as authoritative instead of a clean miss that
  falls back to the journal. Replaced the wholesale drop with a two-generation
  rotation (live + previous, memory ≤ 2×cap): a write for a correlation already in
  the previous generation migrates its accumulated sum into the live map, so a hit
  always reflects the complete running sum; a correlation is dropped only when it
  ages out of both generations, then `UsageFor` cleanly misses. Reporting only;
  billing/ceilings were always journal-authoritative and unaffected. (M456)
- **The Anthropic streaming parser tolerates a malformed structural frame.** It
  aborted the whole stream on one bad SSE frame — discarding already-streamed
  tokens — where the other four providers (and this parser's own EOF handling) skip
  and continue. The four structural decoders now skip a frame that fails to parse
  instead of aborting; a real provider `error` event still propagates. One corrupt
  frame from a proxy no longer throws away a whole response. (M451)
- **Async Slack/Discord inbound runs are cancelled on a clean shutdown.** They
  detached the agent run from the HTTP request (correct) to `context.Background()`,
  which is never cancelled — so on shutdown an in-flight run was killed by process
  exit rather than stopped cleanly. They now detach to the daemon-lifetime context:
  unchanged during normal operation and the drain window (the daemon ctx is
  cancelled only after the drain), but a straggler past the drain timeout now gets a
  clean cancellation instead of an abrupt kill. (M450)
- **The provider streaming-response parsers are now fuzz-tested.** `FuzzParseStream`
  in the openai, anthropic, google, cohere, and ollama providers feeds arbitrary
  bytes to each `parseStream`, asserting it never panics or hangs on a malformed,
  truncated, or hostile upstream (a MITM/buggy proxy) — a garbage stream must yield
  a clean error, not crash the agent loop. ~17 M executions across the five found no
  panic and no hang. (M449)
- **The journal reopen path is now fuzz-tested against corrupt segments.**
  `FuzzJournalOpen` writes arbitrary bytes as a journal segment and exercises
  `Open`/`Range`/`Tail`/`Verify` — the custom torn-tail truncation, line-split, and
  hash-chain parsing a crash or bit-rot can feed garbage. It asserts a corrupt
  journal never crashes or hangs the daemon on startup (Open may reject it or
  truncate the torn tail, but always terminates cleanly). A 45 s / ~91 K-execution
  run found no panic and no hang. (M446)
- **Per-response usage reporting no longer scans the whole journal.** The `usage`
  field of every OpenAI-compat reply was computed by a full-journal scan per
  request — O(journal) that grows unbounded over the daemon's life and that a
  client hammering the API could amplify into a CPU/IO DoS. The Governor now keeps
  a bounded in-memory per-correlation token index (recorded with the same counts
  that go into `budget.consumed`), and usage lookup consults it first, falling back
  to the journal scan on a miss — so the reported numbers are identical, the common
  "just-finished run" case is O(1), and the index is reporting-only (never billing
  or ceiling enforcement). (M443)
- **The control plane's panic guard now also covers the pre-auth parse phase.** The
  per-connection `recover` was deferred only after the request was read and parsed,
  so a panic during the bounded read or JSON unmarshal would have propagated out of
  the connection goroutine and crashed the daemon. It is now deferred at the top of
  the handler (reading the request id at panic time), containing the full
  connection lifecycle — read, parse, auth, dispatch. Latent today (the stdlib JSON
  decode doesn't panic) but a defense-in-depth gap on the daemon's pre-auth surface.
  (M439)
- **A hung scheduled run can no longer permanently stall its schedule.** Cadence
  guards against overlapping runs with an in-flight marker cleared when the firing
  returns — but the firing had no deadline, so a run that hung (a wedged provider/
  tool ignoring its own bounds) left the marker set forever and that schedule never
  fired again, silently, until a restart. Each firing now carries a backstop
  deadline (default 1 h, `AGEZT_SCHEDULE_RUN_TIMEOUT` to override; `0`/`off`
  disables): a ctx-respecting run is cancelled at the deadline, the marker clears,
  and the schedule recovers on its next slot. (M438)
- **The daemon honors a cgroup CPU quota (GOMAXPROCS auto-adapts).** The Go runtime
  is not cgroup-aware, so inside a `--cpus`-limited container or a constrained host
  it defaulted `GOMAXPROCS` to the number of *host* cores and over-scheduled against
  a fraction of a core — the "hot-loop a Pi" symptom SPEC-11 §4 calls out. At
  startup the daemon now reads the cgroup v2 (`cpu.max`) / v1 (`cpu.cfs_*`) CPU
  quota and lowers `GOMAXPROCS` to match (rounded up, clamped to host cores). It is
  a no-op off Linux, when no quota is set, when the quota meets/exceeds the host
  cores, or when `GOMAXPROCS` is set explicitly — it only ever lowers, never
  overrides the operator. Stdlib-only (no automaxprocs dependency). (M437)
- **The AWS assume-role duration env var rejects negative/malformed values.**
  `AGEZT_AWS_ASSUME_ROLE_DURATION_SECONDS` was parsed without a `>0` guard (the
  lone duration parse in the daemon wiring missing one). `kernel/creds` substitutes
  the AWS default (3600 s) only for an exact `0`, so a negative value (a typo'd
  `-3600`) flowed verbatim into the STS `DurationSeconds` and was rejected with a
  ValidationError at first credential resolution — a runtime failure of the whole
  AWS chain rather than a graceful fallback. The value now degrades to the default
  on any missing/zero/negative/malformed input. (M436)
- **The ACP-agent bridge's timeout is now real, and teardown is bounded.** The
  `acp_agent` tool wrapped each delegated session in a 5-min `context.WithTimeout`,
  but the agent was spawned with `exec.Command` (not `CommandContext`) and teardown
  ran only in a deferred `close()` after `Prompt` returned — so a silent or wedged
  external agent parked the stdout read and `Invoke` blocked indefinitely; the
  timeout never fired because nothing acted on the cancellation. `Invoke` now starts
  a watcher that tears the transport down when ctx fires (unblocking the read), and
  `close()` is idempotent (`sync.Once`) with a bounded post-kill wait so an
  un-reapable child can't pin the caller. (M434)
- **The plugin SDK caps the inbound frame read.** `plugins/sdk` read every
  newline-delimited frame from the host with `bufio.Reader.ReadBytes('\n')` and no
  size cap, so a frame with no terminating newline — a corrupted pipe or a partial
  host write — would grow a single allocation until the plugin was OOM-killed. Every
  other newline reader in the tree already caps (host side 16 MiB, mcpbridge 16 MiB,
  acp 8 MiB); the SDK was the lone gap. It now reads through a capped `readFrame`
  (16 MiB) mirroring `kernel/plugin.readFrame`, and skips stray blank lines so they
  no longer emit a spurious empty-id error frame. (M433)
- **Inbound channel HTTP servers are hardened against slow-loris.** The Slack, Discord,
  and webhook inbound servers set only `ReadHeaderTimeout`, so a client that finished the
  headers then dripped the request body one byte at a time held a handler goroutine and
  connection open indefinitely (the body-size cap bounds bytes, not time) — exhausting
  goroutines/FDs across many connections, before signature verification. They now also
  set `ReadTimeout` (bounding the full body read) and `IdleTimeout`. (`WriteTimeout` stays
  unset so a slow agent reply isn't cut off.)
- **A misbehaving MCP server can no longer crash or wedge the MCP bridge.** Two fixes
  to the JSON-RPC response path: (1) the bridge closed each pending-request channel when
  the connection died, which raced the transport read goroutine's send — a server that
  replied and then dropped its connection during teardown triggered a send-on-closed-
  channel panic that crashed the bridge; death is now signalled via a shared channel and
  the per-call channels are never closed. (2) a server sending two responses with the
  same id blocked the read goroutine forever on a full channel, wedging every later call;
  the delivery is now non-blocking (a duplicate is dropped).
- **A bad pre-serialized value no longer poisons a state namespace.** `state.Set` of an
  invalid `json.RawMessage` (e.g. a malformed plugin/tool result via the passthrough
  path) wrote the bad bytes into the in-memory map before the atomic snapshot failed,
  leaving the entry resident — which then made every subsequent `Set` to that namespace
  fail and `Get` return invalid JSON diverging from disk, for the rest of the process.
  The value is now validated before the map is touched, so a rejected write leaves the
  namespace consistent.
- **A hard-deny no longer false-positives on ordinary multi-word text.** To catch the
  space-padded fork bomb, the Edict hard-deny matcher stripped *all* whitespace from the
  command before substring-matching — which collapsed ordinary prose onto an alphabetic
  floor rule (`re boot the server` → `reboot`, `mk fs` → `mkfs`, `power off` →
  `poweroff`), permanently blocking a legitimate command with no override. It now strips
  only whitespace adjacent to punctuation, still normalising the fork bomb (its spaces
  sit next to `{ | & ;`) without ever merging two words.
- **An empty catalog sync no longer wipes the working catalog.** A `catalog sync` that
  fetched a syntactically valid but provider-less payload (`null` or `{}` — e.g. a
  proxy/CDN returning HTTP 200) parsed without error and overwrote `api.json` with an
  empty catalog, leaving the Governor with no models to route to (a self-inflicted
  outage). Sync now treats a zero-provider result as a failure, so the prior catalog is
  kept.
- **The Forge skill lifecycle is concurrency-safe and revert respects the state
  machine.** Two fixes to the self-improvement engine: (1) it did its skill
  read-modify-writes as separate store calls with no lock, so concurrent runs (each
  run calls `Activate` then `RecordOutcome`, and the control plane is concurrent) could
  lose a metric update or — worse — resurrect a just-quarantined skill to active by
  writing back a stale snapshot; a manager-level mutex now serialises every mutator.
  (2) `revert` force-set a lineage parent to active without checking the transition, so
  it could resurrect a quarantined parent (or push a draft straight to active, skipping
  the shadow gate); it now only restores a parent that may legally become active.
- **A panicking Pulse observer can no longer crash the daemon.** The autonomous Pulse
  engine ran its observers, salience scoring (incl. an optional LLM provider call), and
  briefing sinks inline on a single resident goroutine with no panic recovery — so a
  buggy observer, a panicking provider, or a misbehaving channel sink took down the
  whole daemon. Each observer poll and the digest flush are now wrapped in a recover
  backstop (the panic is journaled), matching the containment the standing-order and
  schedule engines already had.
- **A self-exiting plugin no longer leaks a zombie process.** The plugin host called
  `cmd.Wait()` only inside `Close()`, and `Close()` short-circuited once the plugin was
  marked dead — so a plugin that exited or crashed on its own (or was reloaded) was
  never reaped, accumulating one zombie per death on Linux (a crash-looping or
  repeatedly-reloaded plugin could exhaust the process table). A dedicated per-process
  waiter now owns `cmd.Wait()` and reaps the child on every death path; `Close` waits
  on (or forces) it without ever double-calling `Wait`.
- **Concurrent memory / world-model writes no longer lose updates.** The memory-lite
  and world-model managers did their read-modify-write (`Get` → compute → `Put`) as
  two separately-locked store calls, so two concurrent writers — the agent loop and
  the auto-distiller both remembering a fact, or a reinforce racing the periodic decay
  — could interleave and lose one update (a dropped reinforcement, or decay clobbering
  a just-refreshed weight). Each mutator now holds a manager-level lock across the
  whole Get→Put.
- **A panicking scheduled run can no longer crash the daemon.** A fired schedule
  (cadence) runs a governed run and then delivers its answer over a channel plugin —
  the delivery executes after the agent loop's own recover returns, on the bare fire
  goroutine. A panic there (a channel-plugin bug, a nil deref in delivery) took down
  the whole process. Fired runs are now wrapped in a recover backstop and the
  in-flight guard is always cleared, mirroring the standing-order containment.
- **Re-stating a superseded memory/world-model fact no longer resurrects it.**
  Reinforcing a record/entity (re-`Remember`/`Upsert` of content that was previously
  *superseded*) rebuilt it without its supersession link, so both the stale fact and
  its replacement became active again — and the auto-distiller, which re-extracts
  facts every task, triggered this on its own. The supersession link is now preserved
  across a reinforce, so a superseded fact stays inactive.
- **HTTP surfaces are hardened against slow-loris connection exhaustion.** The web UI,
  the OpenAI-compatible API, and the REST server were built with no HTTP timeouts, so a
  client that dribbles request-header bytes (or never finishes the request line) could
  pin a connection and goroutine indefinitely — exhausting file descriptors / memory
  and wedging the surface. All three now set `ReadHeaderTimeout` and `IdleTimeout`
  (`WriteTimeout` is deliberately left unset so long-lived SSE/streaming responses
  aren't killed mid-flight). The control-plane TCP server already had a read deadline.
- **A crash mid-write no longer corrupts the journal on the next append.** When a
  crash left a torn (newline-less) fragment at the tail of the last segment, reopening
  set the append offset to the raw file size and opened the segment `O_APPEND` — so
  the first new event was written *after* the fragment, gluing a partial record onto a
  whole one. The result was a line nothing could decode, which wedged
  `Range`/`Verify`/reopen permanently — the source-of-truth log became unreadable.
  On reopen the torn tail is now truncated to the end of the last complete line, so
  the next append begins exactly where the last committed record ended. (Readers
  already discarded the torn line; only the append path was affected.)
- **A panicking standing order can no longer crash the daemon.** A fired order runs
  its plan (provider/tool/plugin code) and then briefs over the network on a
  dedicated `go fire(...)` goroutine. The event-runner and cron loops recovered only
  on their *own* loop goroutine — not on the dispatched run — so a panic in any
  tool/plugin reached by a triggered order took down the whole process (every
  in-flight run and the control plane with it). Every dispatched order is now wrapped
  in a recover backstop, and the daemon's run also recovers-and-journals a new
  `standing.error` event (visible in `agt journal`) so the crash stays diagnosable
  instead of vanishing. This makes the package's documented no-crash guarantee true.
- **Standing-order bookkeeping maps no longer grow without bound.** The event and
  cron runners track a per-order last-fire timestamp to enforce the cooldown /
  once-per-minute dedup; entries for removed orders were never dropped, so a
  long-lived daemon with order churn leaked memory in proportion to every order id
  ever created. Both maps are now pruned to the live order set each pass.
- **`agt standing add --budget` rejects an out-of-range or non-finite amount.** A
  budget above ~$9.2e9 (or `Inf`/`NaN`) overflowed the `int64` microcents
  conversion, whose result is undefined in Go — it could land as a small or negative
  cap, silently mis-configuring the per-run spend guard. Such amounts are now
  rejected with a clear error instead of stored.
- **Standing orders never diverge from disk on a failed write.** The standing-order
  store (Chronos) mutated its in-memory order *before* the durable JSON write in
  `SetEnabled`/`Remove`; a transient write failure (full disk, permissions) left the
  running view showing a pause/removal that never reached disk and would vanish on
  reopen. Both now roll the in-memory mutation back when `save()` fails, so the live
  view and the durable file stay identical.
- **A standing order's event-trigger cooldown keys off the local clock.** The
  per-order cooldown previously compared against the *event's* timestamp, so an
  externally-sourced (webhook/mesh) event carrying a skewed or far-future timestamp
  could permanently suppress — or prematurely release — the order. It now uses the
  runner's own clock, immune to untrusted event timestamps.
- **`/metrics` is robust to an invalid metric definition.** The REST `/metrics`
  Prometheus endpoint now coerces each metric name to a valid Prometheus
  identifier and escapes `HELP` text (backslash + newline) — a name containing a
  `.`, `-`, or space, or a HELP line with a newline, would otherwise emit a line
  Prometheus can't parse, and a single malformed line breaks the **whole** scrape,
  silently dropping every other metric. (Today's names/help are all valid; this
  keeps a future metric definition from taking out observability wholesale.)
- **A malformed channel message no longer crashes the daemon.** Inbound handling
  for the Telegram, Slack, and Discord channels — which process untrusted external
  messages on long-lived goroutines — now recovers from a panic and journals it as
  a `channel.error` event (visible in `agt journal`), instead of an unrecovered
  panic taking down the whole daemon. (The webhook channel was already covered by
  the stdlib HTTP server's per-request recovery.)
- **A handler panic no longer crashes the daemon.** The control-plane's
  per-connection goroutine now recovers from a panic in any command handler and
  returns an `internal error` response to that caller, instead of an unrecovered
  panic that would take down the whole daemon — every in-flight run and channel
  with it. One malformed/edge-case request is contained to its own connection.

### Security
- **Advisory: build with Go 1.26.4+.** A `govulncheck ./...` scan flagged two
  reachable standard-library vulnerabilities — GO-2026-5039 (net/textproto error
  escaping, via the email/SMTP path) and GO-2026-5037 (crypto/x509 hostname
  parsing) — both fixed in go1.26.4. They are stdlib-only (no source defect) and the
  reachable paths are low-severity/largely not runtime-exercised, but the release
  and CI toolchain should be pinned to go1.26.4 or later; re-run `govulncheck` there
  to confirm clean. (M452)
- **Inbound channel signature verification is now fuzz-tested for forgery
  resistance.** `FuzzVerify` in the Slack (HMAC-SHA256), Discord (Ed25519), and
  webhook (HMAC-SHA256) channels asserts the authenticity gate never panics, the
  genuine signature is accepted, and — the load-bearing property — no signature
  other than the authentic one is ever accepted (no forged-command injection).
  Runs of ~2 M / ~2 M / ~3.7 M executions found no panic and no forgery. (M448)
- **The control-plane pre-auth request parse is now fuzz-tested.** `FuzzRequestParse`
  drives `readBoundedLine` + the request unmarshal — the path that runs before the
  token is checked, so any local client feeds it bytes pre-auth. It asserts the
  bounded reader never panics and never exceeds its byte cap (the pre-auth-OOM
  guard) and that unmarshalling a complete line never panics. A 40 s / 7.9 M-execution
  run found no panic. (M447)
- **The trust-ladder decision path is now fuzz-tested.** `FuzzDecide` hammers
  `edict.Decide` — which JSON-decodes and whitespace-normalizes untrusted tool
  input to match the hard-deny floor (the M173/M426 evasion surface). It asserts
  the engine never panics, the hard-deny floor is un-overridable by any trust
  ceiling, and a ceiling only ever tightens. A 45 s / 2.65 M-execution run found no
  panic and no floor bypass. (M445)
- **The secret-redaction path is now fuzz-tested.** Added the tree's first fuzz
  test (`FuzzRedact`) over the boundary that keeps credentials out of logs/the bus/
  transcripts: it asserts redaction never panics and never leaves an indexed secret
  verbatim in the output. A 45 s / 1.5 M-execution run found no panic and no leak;
  the one fuzzer-flagged case was a placeholder artifact (a secret that is a
  substring of `[REDACTED]`), soundly excluded by redacting the bare secret as the
  guard. Committed with its regression corpus. (M444)
- **Telegram Bot API responses are size-capped.** `getUpdates` and `getFile`
  decoded the response body with no bound, so a buggy/compromised/MITM'd Bot API
  endpoint could stream an unbounded body and OOM the long-poll loop — the one HTTP
  response class in the tree without the size cap the rest uniformly applies. Both
  now decode through an 8 MiB `io.LimitReader`. (M441)
- **The file tool opens its write paths with O_NOFOLLOW (Unix).** `resolve()`
  rejects out-of-root symlink targets, but for a not-yet-existing file there was a
  narrow TOCTOU between the check and the `O_CREATE` open: a concurrent writer could
  plant a symlink at the path and the open would follow it out of the workspace.
  `doWrite` and `doReplace` now pass `O_NOFOLLOW`, so the open fails rather than
  following a swapped-in symlink. Narrow (the file tool can't create symlinks
  itself, so it needs a separate concurrent writer), but it definitively closes the
  window and completes the M427 symlink-escape defense. No-op on non-Unix (POSIX
  flag; Windows symlink creation is privileged). (M440)
- **The web dashboard sets a Content-Security-Policy with a per-response nonce.**
  The operator dashboard is the highest-privilege browser surface (same-origin with
  the token-authed, state-mutating control plane) and renders untrusted agent/tool/
  transcript data. A full review found no XSS — the page is built entirely with
  text-node DOM construction, no dynamic-HTML sink — but it shipped without a CSP.
  It now sends `default-src 'none'; script-src 'nonce-…'; style-src 'nonce-…';
  connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`
  with a fresh `crypto/rand` nonce minted per request and substituted into the two
  inline tags. A nonce (not `'unsafe-inline'`) means any injected inline script from
  a future regression is refused for lacking the unpredictable value, and the
  directives block injected external loads, cross-origin exfiltration, and framing.
  Defense-in-depth on top of the existing nosniff / X-Frame-Options / no-referrer /
  constant-time-token headers. (M435)
- **The Azure provider URL escapes the deployment name and api-version.** The
  Azure OpenAI request URL was built by raw string concatenation, inserting the
  deployment/model id into the path unescaped. A value bearing a `?` terminated
  the path early and smuggled a query parameter ahead of the real `api-version`
  (and a space/`/`/`#` produced a malformed URL). Deployment names are normally
  alphanumeric and gated by the model allowlist, so exploitation needs a
  malicious/mistaken catalog entry — but it is also a correctness bug for any
  legitimately punctuated name. The id now goes through `url.PathEscape` and the
  api-version through `url.QueryEscape`; ordinary names are byte-identical. (M432)
- **The file tool's `search`/`glob` can't read outside the workspace via a symlink.**
  `read`/`write`/`replace`/`stat`/`delete` resolve symlinks and reject targets outside
  the configured root, but `search` and `glob` walked the tree and read/enumerated every
  entry without re-checking symlinks — `WalkDir` reports a symlink-to-file as a regular
  entry, so `os.ReadFile` followed it out of the workspace. A prompt-injected agent could
  point an in-root symlink at `/etc/passwd` (or `…/.aws/credentials`) and `search` to
  dump its contents — an arbitrary-file-read escape of the tool's sandbox. Both ops now
  skip any entry whose resolved target leaves the root. (Also: `search`/`replace` now cap
  the per-file size they read, so a multi-GB workspace file can't OOM the daemon.)
- **Plugin pin verification can't be bypassed by a bare-name path.** A pinned plugin
  given as a bare name (`AGEZT_PLUGINS="t=mytool"`) was hashed via `os.Open` (CWD-
  relative) but executed via `$PATH` lookup — so the pin could verify one file while a
  different one ran. A bare name is now resolved to its absolute `$PATH` location once,
  before both the hash check and exec, so the pinned bytes and the executed bytes are
  the same file.
- **Streaming LLM deltas are now secret-redacted before fan-out.** Token and
  reasoning deltas (`llm.token` / `llm.reasoning`) were published on the ephemeral
  streaming path, which skipped the redactor that the durable path applies. They never
  reached the journal, but they were fanned out unredacted to every subscriber —
  including the outbound webhook dispatcher (default `>` subject), the pulse stream,
  the OpenAI-compat relay, and the web UI — so a credential the model echoed
  mid-stream could egress in the clear. Streaming publishes now run through the same
  redactor.
- **The AWS secret access key is now redacted.** The redactor caught the AWS key *id*
  (`AKIA…`, the non-secret half) but not the 40-char secret access key. It is now
  scrubbed when it appears next to its `aws_secret_access_key=…` assignment label
  (keyed to the label so a standalone base64 string — a hash or id — is never
  mangled).
- **Outbound webhooks are egress-guarded.** The webhook dispatcher (and `agt webhook
  test`) now route deliveries through the same netguard egress guard as the `http`/
  `browser` tools, so a configured sink can no longer reach loopback, RFC1918/ULA, or
  the cloud-metadata endpoint (169.254.169.254) by default — closing a journal-
  exfiltration / internal-POST gap in the SPEC-06 egress model. Operators who
  legitimately deliver to an internal sink opt the range back in with
  `AGEZT_WEBHOOK_ALLOW_LOOPBACK=1` / `AGEZT_WEBHOOK_ALLOW_PRIVATE=1` (the latter logs a
  warning); the boot banner shows the effective `egress=` mode.
- **OpenAI tool-name sanitisation can no longer misroute a tool call.** Dotted tool
  names (`browser.read`) are sanitised to OpenAI's `^[a-zA-Z0-9_-]+$` pattern on the
  wire, but the sanitiser is many-to-one — `browser.read` and `browser_read` both
  became `browser_read`. The reverse map was built last-writer-wins, so two colliding
  tools were sent under one (duplicate) function name and a returned `tool_call`
  routed back to whichever tool overwrote the map — non-deterministically by slice
  order, running model- or attacker-controlled arguments against the **wrong** tool.
  Wire names are now computed by an injective mapping (deterministic numeric suffix on
  collision), shared by the streaming and non-streaming encoders and the
  assistant-history replay, so the reverse map is exact and every tool is sent under a
  distinct, valid name.
- **Fork-bomb hard-deny no longer evades on whitespace.** The Edict hard-deny
  floor (immutable, not raisable by trust level) stored the fork bomb as the
  no-space form `:(){:|:&};:`, but the matcher only collapsed whitespace *runs*
  to a single space — so the canonical, actually-valid fork bomb `:(){ :|:& };:`
  (and its bash-wrapped / JSON-wrapped forms) survived and was **not** denied.
  Hard-deny candidates now also include a fully whitespace-stripped form, so
  every spacing variant normalizes onto the floor rule. Space-bearing rules
  (`rm -rf /`, `dd if=`) are unaffected. Fail-closed.
- **Connection-string passwords are now redacted.** The secret-redaction layer
  (applied to logs, tool output, and journal payloads) gained a high-confidence
  detector for the password in a `scheme://user:password@host` URI — Postgres,
  MySQL, MongoDB, Redis, AMQP, and the like — which a tool dump, error message,
  or config echo could otherwise leak. Only the password is masked
  (`scheme://user:[REDACTED]@host`), preserving the scheme/user/host so an
  operator can still tell which database leaked, per SPEC-06 §4. A raw `@` inside
  the password is fully masked; a bare `host:port` with no userinfo is left
  intact (no false positives). Surfaced in `agt redact test` diagnostics.
- **Replay-window check hardened against integer overflow.** The Slack, Discord,
  and generic-webhook channels' inbound freshness check (which rejects a stale
  signed timestamp to block replay) computed the age as
  `time.Duration(delta) * time.Second`, which overflows int64 nanoseconds for a
  timestamp ~300 years off and could wrap negative — passing the `> window` check.
  It now compares in integer seconds/milliseconds, so no timestamp can bypass the
  window via overflow. (Not previously exploitable — the timestamp is signed — but
  a freshness backstop shouldn't depend on that.)
- **Web UI defensive response headers.** Every web-monitor response now carries
  `X-Frame-Options: DENY` (the dashboard has state-mutating controls — approve /
  halt / resume — so framing is denied to block clickjacking), `Referrer-Policy:
  no-referrer` (the page URL carries the auth token in `?token=`, so the referrer
  is suppressed to keep it out of any `Referer` header), and
  `X-Content-Type-Options: nosniff`.
- **Telegram bot token no longer leaks into error messages.** The Telegram API
  carries the bot token in the request URL path (`/bot<token>/…`), and Go's
  `http.Client.Do` returns errors (`*url.Error`) that embed the full URL — so a
  transport failure (DNS, refused, timeout) could carry the token into any log or
  journal that recorded the error. Telegram channel errors are now scrubbed of the
  token before they propagate.
- **Constant-time web UI token check.** The web monitor's auth-token comparison
  used a plain `==`, a timing side-channel an attacker who can reach the web UI
  could use to recover the token byte-by-byte. It now uses
  `crypto/subtle.ConstantTimeCompare` for both the `?token=` query param and the
  `Authorization: Bearer` header, matching the control-plane's gate. (The web UI
  binds to loopback by default, which limited exposure.)

### Fixed
- **Governor routing/capability/budget events are now linked to their run.** The
  Governor's per-call decision events — `routing.decision`, `provider.fallback`,
  `rate.limited`, `budget.exceeded`, `capability.rerouted`, `capability.rejected`
  — were emitted without a correlation id, so they were orphaned from the run
  that triggered them: they didn't appear in the run timeline and `agt why
  <event-id>` on one resolved nothing. They now carry the request's correlation
  (matching `budget.consumed` and the new `capability.degraded`), so the full
  routing/spend story for a run is reachable from `agt why` and the run-detail
  view.
- **Rune-safe display truncation (codebase-wide).** A shared `strutil.Ellipsis`
  helper now backs every text truncation that reaches a user or the model: the
  provider-fallback reason in `agt status`, generated-plan node snippets, and AWS
  SSO/STS/web-identity error excerpts were all converted from byte slices to
  rune-safe cuts (joining the schedule-intent, coding-diff, and browser-text fixes
  below). No multi-byte UTF-8 rune (Turkish ç/ş/ğ, CJK, emoji, …) can be split
  into invalid output anywhere.
- **Rune-safe display truncation.** Three user-facing truncations — the schedule-
  intent shortener in `agt schedule` / cadence logs, the coding tool's diff
  output, and the **browser tool's extracted page text** sent to the model —
  sliced on a byte boundary, which could split a multi-byte UTF-8 rune (e.g. a
  Turkish ç/ş/ğ, or any non-English web page) into invalid output. All now cut on
  a rune boundary, so truncated intents, diffs, and fetched web text are always
  valid UTF-8. (The journal's own answer truncation was already rune-safe.)

### Added
- **`agt standing add --scope <ent1,ent2>` grounds an order's run in what it
  watches.** A standing order's scope entities are now settable and, when the
  order fires, prefixed to the run's intent so the agent knows the subject it is
  acting on (SPEC-16 §4 scope.entities).
- **`agt standing add --budget <USD>` caps a standing order's per-run spend.**
  The per-run cost ceiling (enforced when an order fires) is now settable from the
  CLI — e.g. `--budget 0.50` — closing the gap where the ceiling was enforced but
  had no way to be configured (SPEC-16 §4 initiative.budget_per_run).
- **Standing orders brief their result to a channel.** When a standing order
  fires and its run produces an answer, the result is delivered to the order's
  configured `--channel` (telegram/slack/discord/webhook), prefixed with the
  order name, reusing the same channel allowlists + sender as scheduled-run
  notifications (SPEC-16 §4). No channel, or an empty answer, sends nothing.
- **Standing orders enforce their `max_trust` initiative ceiling.** A standing
  order with `--max-trust L2` now caps autonomous action within its runs: a
  capability that would normally auto-allow (L4) is clamped down to Ask — or, at
  `L0`, denied — so a persistent goal can fire on its own but stay bounded
  (SPEC-16 §4). The hard-deny floor and unknown-capability default-deny are
  unaffected — the ceiling can only tighten, never loosen. (Edict gains
  `DecideWithCeiling`; runs carry the ceiling via context.)
- **Standing orders now fire on a cron schedule.** A standing order with a `cron`
  trigger (`"0 8 * * *"`, `"*/15 * * * *"`, `"0 8 * * 1-5"`, …) launches its plan
  on schedule — a stdlib 5-field cron matcher (no dependency) ticked every minute,
  firing each matching order at most once per minute and journaling
  `standing.fired` (SPEC-16 §4). This is the canonical "brief me every morning"
  path.
- **Standing orders now fire on events.** A standing order with an `event`
  trigger launches its plan as a run when a journal event matches its subject
  (NATS-style wildcards, e.g. `github.>`), bounded by the order's budget ceiling
  and journaled as `standing.fired` (SPEC-16 §4). A per-order cooldown keeps an
  event burst from launching a flood of runs, and lifecycle events never
  self-trigger. (Cron triggers continue to run via the schedule engine.)
- **`agt standing` — Chronos standing orders (persistent goals).** A new command
  to define and manage standing orders (SPEC-16 §4): named, pausable rules with
  cron and/or event triggers, an initiative ceiling, and a briefing channel.
  `agt standing add --name … --cron "0 8 * * *" [--event "github.>"] [--plan …]
  [--mode inform_only|ask|act_or_ask] [--max-trust L2] [--channel telegram]`,
  plus `list`, `pause`, `resume`, `remove`, and `why <id>` (an order's life
  story — every create/pause/resume/fire/remove). Orders are persisted and every
  mutation is journaled (`standing.created` / `standing.updated` /
  `standing.removed` / `standing.fired`) and auditable. The on-disk/wire form is
  JSON, not the spec's YAML, to keep Agezt dependency-free.
- **Shadow skills auto-promote to active once they've proven out.** The final
  rung of the SPEC-05 §5.2 trust ladder: a shadow skill whose shadow-evaluations
  cross a gated win count + rate (default ≥3 helpful judgements at ≥50%) is
  promoted `shadow → active` automatically, journaling `skill.promoted` with the
  reason. On by default but **inert unless `AGEZT_SKILL_SHADOWEVAL` is feeding
  wins**; `AGEZT_SKILL_AUTOPROMOTE=off` disables it. Together with auto-shadow
  (M399), shadow-eval (M400) and auto-quarantine (the demotion side), the
  draft→shadow→active→quarantined lifecycle is now fully self-driving.
- **`AGEZT_SKILL_SHADOWEVAL=on` evaluates shadow skills against completed runs.**
  The shadow rung of the SPEC-05 §5.2 trust ladder: after a successful run, the
  shadow skills relevant to that intent are judged — by a bounded, best-effort LLM
  call — for whether they *would have helped*, recorded as `shadow_evals` /
  `shadow_wins` on the skill and journaled as `skill.shadow_evaluated`. It runs no
  tools and never executes the shadow skill, so evaluation cannot affect outcomes.
  Off by default (it spends extra provider calls). The accumulated evidence feeds
  the upcoming shadow→active auto-promotion gate.
- **`AGEZT_SKILL_AUTOSHADOW=on` auto-stages a well-formed draft skill to shadow.**
  The first rung of the SPEC-05 §5.2 trust ladder: when a freshly-authored draft
  passes a deterministic shadow-test (substantive body and a retrievable
  description/triggers), it auto-advances `draft → shadow` on creation instead of
  waiting for a manual `agt skill promote`, journaling `skill.promoted` with the
  gate reason. Off by default (staging is a step toward production, so it's
  opt-in); pairs with the on-by-default auto-quarantine demotion. `agt skill
  import` now reports the status the skill actually landed in (draft or shadow).
- **`AGEZT_CONTEXT_SUMMARIZE=1` summarises dropped tool outputs instead of
  stubbing them.** When context compaction elides an old tool output it normally
  leaves a short head-snippet stub; with this on, a bounded one-line *summary* of
  the output (from a cached, once-per-output provider call) is embedded instead,
  so the model keeps the meaning of what was dropped, not just its first
  characters. Off by default — it spends extra provider calls, so the operator
  opts in. Only active when context budgeting is on (SPEC-10 §3).
- **`AGEZT_CONTEXT_PROTECT_FIRST=<n>` shields the run's original grounding from
  compaction.** When context budgeting elides oldest-first, the earliest tool
  results — the discovery/setup outputs that grounded the run — are the first to
  go. Setting this protects the first *n* messages so that framing survives even
  as the oldest *middle* turns are dropped; the most recent turns are always kept
  too. 0 (the default) keeps the historical strictly-oldest-first behaviour
  (SPEC-10 §3).
- **`AGEZT_CONTEXT_BUDGET=auto` derives the budget from the model's context
  window.** Instead of a fixed char count, `auto` sizes the context budget at half
  the resolved model's catalog context window (~4 chars/token) — so a small-window
  model compacts sooner and a large-window model later, automatically. A model the
  catalog doesn't know leaves compaction off (no guessing). An explicit numeric
  budget still wins (SPEC-10 §3 / SPEC-16 §3 `compress_at_fraction`).
- **Context budgeting keeps long runs within a size cap (SPEC-10 §3).** With
  `AGEZT_CONTEXT_BUDGET` set, before each model call the agent loop trims its own
  assembled context to fit: it elides the *oldest tool outputs* down to short
  stubs (the system prompt and the most recent turns are always preserved) and
  journals a `context.compacted` event (how many outputs elided, chars reclaimed,
  size before/after). The model keeps acting on its recent context while a
  many-step run stops growing without bound. Off by default (full history).
- **The run-detail view flags offloaded tool outputs.** When a tool.result's
  output was offloaded to the artifact store, the web Live Monitor's run-detail
  card now shows it as `⤓ <N>B artifact <ref>…` and, expanded, the preview plus
  the ref and the exact `agt artifact get <ref>` command to recover the full
  bytes — so an operator can tell an output was offloaded (not lost) and fetch it.
- **`agt artifact get <ref>` retrieves an offloaded tool output.** When a large
  tool output was stored content-addressed (its `tool.result` carries a
  `raw_ref`), `agt artifact get <ref>` fetches the full bytes back — to stdout or,
  with `--out <file>`, to a file. The store re-verifies the bytes against the ref
  on read, so a corrupted blob is reported rather than returned; a malformed or
  unknown ref gives a clear error. Completes the SPEC-04 §3.6 round-trip
  (offload → journal `raw_ref` → retrieve).
- **Large tool outputs are offloaded to a content-addressed store, not inlined in
  the journal (SPEC-04 §3.6).** When a tool returns more than a threshold (8 KiB
  by default; `AGEZT_ARTIFACT_THRESHOLD` to tune), the agent loop stores the full
  output in `~/.agezt/artifacts/` keyed by its BLAKE3 hash and the journaled
  `tool.result` carries a short preview + a `raw_ref` + `output_bytes` instead of
  the whole blob — so the event log stays small while the output survives,
  deduplicates, and is retrievable by ref. The model still receives the complete
  output. Backed by the new `kernel/artifact` blob store.
- **Skills that repeatedly fail in production are auto-quarantined (SPEC-05 §5).**
  A run now attributes its outcome (success/failure) to the active skills it
  activated, and the Forge pulls a skill from production once it crosses a
  conservative threshold — at least 3 failures AND a ≥50% failure rate — so a
  good skill with the occasional failure is left alone. The pull is journaled
  (`skill.quarantined` with an `auto-quarantine: N/M runs failed` reason, linked
  to the failing run) and fully reversible (`agt skill promote` re-activates).
  On by default; `AGEZT_SKILL_AUTOQUARANTINE=off` disables it. Until now skill
  outcome metrics were never recorded in production and quarantine was
  operator-only.
- **The run-detail view now shows the Governor's routing/capability decisions.**
  Now that those events are linked to their run, the web Live Monitor's run-detail
  arc renders them instead of dropping them to bare kind lines: `routing.decision`
  (which provider/model served the call + fallback chain), `provider.fallback`
  (failed → next, with reason), `capability.degraded` (the silent JSON-mode
  downgrade), `capability.rerouted` / `capability.rejected` (tool-use remap/reject),
  `rate.limited`, and `budget.exceeded`. The full routing/spend story for a run is
  now readable in one place.
- **`agt journal export --scope task:<run-correlation>` — surgical per-run
  export.** Because every event is ID'd and causally linked, you can now "cut"
  a single run's (correlation's) event subgraph into a self-contained bundle
  instead of exporting the whole journal (SPEC-09 §3). A bare correlation id
  works too. The cut is intentionally non-contiguous, so `agt journal verify
  --bundle` re-verifies it offline by recomputing every event's BLAKE3 hash and
  confirming each belongs to the scope's correlation (a foreign event smuggled
  into the cut is rejected) — rather than the prev-hash continuity check used for
  a full/windowed bundle. The other SPEC-09 §3 scopes (`agent:`/`tenant:`/
  `skill:`/`memory:`) are rejected with a clear message until implemented.
- **Silent JSON-mode capability degradation is now journaled.** When a run
  requests structured-output (JSON mode) but the resolved model belongs to a
  provider family with no native JSON switch, the provider quietly falls back to
  prompt-instructed JSON — previously with no record that the native path was
  skipped. The Governor now emits a `capability.degraded` event (carrying the
  model, `json_mode`, and the reason, linked to the run) so the downgrade is
  visible in the journal and reachable from `agt why`. The request still proceeds
  unchanged — this records the degradation, it does not block or reroute it
  (SPEC-15 §2.3). Tool-use degradation was already journaled
  (`capability.rejected` / `capability.rerouted`); this completes the pattern for
  JSON mode.
- **The run-detail tool-call card now shows the policy verdict.** Edict journals
  a `policy.decision` for every tool call (allow/deny, capability, reason,
  whether it would have prompted, whether a hard-deny floor fired). The web Live
  Monitor's run-detail arc renders it as a **policy** line (`✓ allow shell ·
  would-ask — …` / `✗ HARD-DENY …`) with an expandable
  decision/capability/reason block — the SPEC-12 §4 / SPEC-07 tool-call debug
  "policy" view, alongside the isolation and input/output legs.
- **Tool isolation now shows up in the run timeline (and `agt why`).** The shell
  tool runs commands through the Warden, which journals a `warden.executed` event
  (effective vs requested isolation profile, downgrade flag, exit code) — but it
  was emitted with an empty correlation id, so the event was orphaned from its
  run: it never appeared in the run-detail view and `agt why` on it found
  nothing. The shell tool now stamps the run correlation (read from its context)
  onto the Warden Spec, so isolation events join their run. The web Live Monitor's
  run-detail card renders them as an **isolation** line (`isolation none ⚠
  downgraded from namespace …`) with an expandable requested/effective/exit/
  duration block — the SPEC-12 §4 / SPEC-07 tool-call debug "isolation" view.

### Fixed
- **`warden.executed` / `warden.profile_downgraded` events were not linked to
  their run.** They now carry the originating run's correlation id, so they show
  in the run timeline and are reachable from `agt why <event-id>`.

### Added
- **OpenAI-compatible `GET /v1/models/{id}` (retrieve model).** The OpenAI surface
  already listed models at `GET /v1/models`; it now also answers a single-model
  retrieve — what the official SDKs' `models.retrieve(id)` calls for capability
  probing. A routable id (the default model or a catalog id, the same set the list
  advertises) returns the model object; an unknown id returns a `404` with an
  OpenAI-shaped error, so a client distinguishes "unknown model" from "endpoint
  missing" (SPEC-15 §3 / SPEC-16 §1.1).
- **Import agentskills.io / ClawHub `SKILL.md` files.** `agt skill import` now
  accepts a `.md` file written to the open agentskills.io standard (YAML-ish
  frontmatter — name/description/triggers/tools_required — plus a Markdown body),
  parses it with a dependency-free frontmatter reader, and installs it as a fresh
  draft skill (content-addressed, journaled, never auto-active). The hundreds of
  existing community skills load into Agezt without rewriting — and gain
  versioning, shadow-testing, and reversibility on top (SPEC-13 §1.2). A Agezt
  `.skill.json` export bundle still imports as before (content-address verified).
- **`agt provider check --caps` advertises prompt caching.** The capability
  report (and its `--json` `prompt_cache` field) now shows whether a model
  supports prompt caching — derived from its catalog cache-read price, the same
  signal the cache-aware billing uses. Completes the SPEC-15 §1.2 advertised
  capability set (tool-use / reasoning / vision / JSON-mode / prompt-caching);
  free/local models report `no`.
- **Web UI: context inspector in run detail.** Each `llm.request` row in the
  run-detail arc now shows a compact context summary (`N ctx chars · system …,
  user …`) and expands (▸/▾) to a full per-source breakdown — answering "how big
  was the context and where did it come from" right in the Live Monitor
  (SPEC-07 / SPEC-10 §3.5). Renders the `context_by_role` field added this
  release; XSS-safe by construction (textContent only).
- **Context size is recorded on every LLM call.** The `llm.request` journal
  event now carries `context_chars` (the assembled context size) and
  `context_by_role` (a per-source breakdown: system / user / assistant / tool) —
  the SPEC-10 §3.5 context-observability foundation. An operator (or `agt why`)
  can now see how big each call's context was and where it came from — the #1
  driver of cost and "lost in the middle" quality loss. Image attachments are
  excluded (a separate modality).
- **Point-in-time restore: `agt restore --at <seq|timestamp> --to <dir>`.** The
  journal is a time machine — this replays the source home's journal up to a
  sequence or RFC3339 timestamp into a fresh `--to` home, "branching a recovered
  state" (SPEC-09 §5). Non-destructive: the source journal is opened read-only
  and untouched; the cutoff prefix is chain-verified before write and the
  resulting home is confirmed to boot. A cutoff past the head restores
  everything; a target that already has a journal is refused.
- **Anomaly auto-halts appear in the system changelog.** `agt changelog` (the
  tamper-evident system timeline, SPEC-08 §4.2) now surfaces a `system.anomaly`
  event as "anomaly auto-halt" with its reason, alongside the `halt` it triggers
  — so an operator sees *why* the daemon stopped itself, not just that it did.
- **Anomaly auto-halt: a runaway circuit breaker.** A new always-on safety
  guard (SPEC-06 §5) watches the global tool-call rate across every run, channel,
  and Pulse; if it exceeds a ceiling within a window — the signature of a runaway
  or looping agent — it auto-engages `halt` (cancelling in-flight runs, blocking
  new ones) and journals a `system.anomaly` event explaining why. This is a
  daemon-wide backstop above the per-run loop guard. On by default (>120 tool
  calls / 10s); tune with `AGEZT_ANOMALY_MAX_TOOLCALLS` (0 disables) and
  `AGEZT_ANOMALY_WINDOW`. The boot banner shows the active setting.
- **`agt why` now shows causation provenance.** Alongside the events sharing a
  correlation, `agt why <event>` renders a "caused by (provenance, root first)"
  section that walks the `causation_id` chain back to the root cause — the
  provenance graph SPEC-01 §7.1 describes. This crosses correlation boundaries
  the correlation list cannot: e.g. a Pulse initiative carries its own
  correlation but links to the originating tick (a different correlation) only
  via `causation_id`, so the tick is now reachable. The chain is also in the
  `--json` output (`causation_chain`). Read-only; the daemon omits trivial
  single-event chains.
- **Web UI: config inspector panel.** A new "Config" panel answers "what is this
  daemon actually running with?" — the resolved model, system-prompt-set flag,
  tool/plugin counts, ask-policy, base paths, and which `AGEZT_*` env vars are
  set. Privacy-safe by construction: env vars are shown by **presence only**
  (never their values), and the system prompt is a set/unset flag (never its
  text). Backed by the existing `config` control-plane command (also available as
  `agt config`); the web panel makes it visible without shelling in.
- **Web UI: full tool I/O in run detail.** The run-detail modal's event arc now
  lets you expand any `tool.invoked` / `tool.result` row (▸/▾) to reveal the full,
  untruncated tool input (pretty-printed JSON) and output (or error) — the
  actionable half of debugging a run, straight from the browser instead of dropping
  to `agt journal`. Non-tool rows are unchanged. (Assistant message *text* remains
  unshown — it is deliberately not journalled; only tool I/O is.)
- **Email channel (outbound).** Agezt can now deliver Pulse briefs and `agt send`
  messages to operator inboxes over SMTP (stdlib `net/smtp`, no new dependency).
  Enable with `AGEZT_EMAIL_SMTP_ADDR` + `AGEZT_EMAIL_FROM` (+
  `AGEZT_EMAIL_USERNAME`/`_PASSWORD` for SMTP AUTH and `AGEZT_EMAIL_RECIPIENTS` for
  the fail-closed recipient allowlist). Outbound-only — inbound email (IMAP/MX) is
  out of scope. The recipient allowlist means a misconfigured brief can't mail
  arbitrary addresses; credentials are never logged.
- **Generic webhook channel.** A vendor-neutral inbound/outbound HTTP channel
  (SPEC-04): any external system can drive an Agezt agent by POSTing a signed JSON
  message (`{channel_id, sender, text, id, ts_ms}`) and receives the agent's reply
  synchronously in the response — the generic counterpart to the Slack/Discord
  channels, no platform SDK. Enable with `AGEZT_WEBHOOK_SECRET` +
  `AGEZT_WEBHOOK_ADDR` (+ `AGEZT_WEBHOOK_CHANNELS` allowlist); set
  `AGEZT_WEBHOOK_OUTBOUND_URL` for async/proactive delivery (Pulse briefs,
  `agt send`). Security mirrors the other channels: HMAC-SHA256 signature
  (`X-Agezt-Signature`, same scheme as outbound webhooks — empty secret fails
  closed), a timestamp freshness window + id de-duplication for replay protection,
  a fail-closed allowlist of channel ids, and bounded request bodies. The agent's
  tool calls still pass through Edict.
- **`agt pulse --text` shows live content.** The human event tail can now append a
  one-line excerpt of each event's text — the streamed answer tokens and a
  reasoning model's chain of thought — so an operator can watch *what* the agent is
  producing live, not just event kinds. Off by default; the structured one-line
  format is unchanged without the flag. This rounds out reasoning visibility:
  reasoning now reaches editors (ACP), API clients (`reasoning_content`), and the
  operator's own `agt pulse`.
- **DeepSeek-R1 on Bedrock — with its reasoning.** `deepseek.r1-*` models (and
  regional profiles like `us.deepseek.r1-v1:0`) now work through Bedrock. The
  adapter renders DeepSeek's chat-template prompt and splits the model's chain of
  thought (the `<think>…</think>` block) from the answer, feeding the reasoning
  into the same pipeline as every other reasoning model — so it surfaces in
  `agt pulse`, the ACP thought-chunk relay, and the OpenAI-compatible API's
  `reasoning_content`. Token usage comes from the Bedrock response headers.
- **Amazon Nova models on Bedrock.** Agezt's Bedrock provider now speaks the Nova
  `messages-v1` body shape, so `amazon.nova-*` models (Micro / Lite / Pro /
  Premier) and their regional cross-inference profiles (`us.amazon.nova-*`, …)
  work alongside the existing Anthropic, Mistral, Cohere, Meta-Llama, and AI21
  Jamba families. Nova returns token counts inline, so the governor sees real
  spend. The legacy `amazon.titan-*` text models stay intentionally unwired (Nova
  is the current family). Chat-only — like the other non-Anthropic Bedrock
  adapters, tool use is not wired on this path.
- **Reasoning models' chain of thought is now captured.** For DeepSeek-R1 and
  other openai-compatible reasoning models that return `reasoning_content`, the
  reasoning streams live as ephemeral `llm.reasoning` events (visible in
  `agt pulse`) and its size is recorded on the `llm.response` event — previously
  it was discarded. The durable journal stays lean (the reasoning text isn't
  persisted); ordinary models are unaffected.
- **Claude extended thinking** is supported (opt-in via
  `AGEZT_ANTHROPIC_THINKING_BUDGET=<tokens>`). When enabled, the Anthropic
  provider requests extended thinking and captures Claude's chain of thought into
  the same reasoning pipeline (live `llm.reasoning` events). Off by default
  (thinking costs extra tokens).
- **Gemini thinking** is supported (opt-in via
  `AGEZT_GOOGLE_THINKING_BUDGET=<tokens>`; `-1` lets Gemini pick a dynamic
  budget). When enabled, the Google provider requests thought summaries
  (`includeThoughts`) and captures them into the same reasoning pipeline. Gemini
  reports thinking tokens separately from answer tokens but bills them as output,
  so they're folded into the run's output-token count for accurate cost. With
  this, all three major reasoning families — DeepSeek-R1, Claude, Gemini — flow
  through one pipeline. Off by default.
- **Gemini thinking on Vertex AI** is supported too (opt-in via
  `AGEZT_GOOGLE_VERTEX_THINKING_BUDGET=<tokens>`; `-1` for a dynamic budget), so
  the thinking capability now spans *both* Gemini surfaces — the Generative
  Language API and Vertex AI — with the same reasoning capture and output-token
  accounting. Separate env var because Vertex is a distinct billing/credential
  surface. Applies to native-Gemini models on Vertex; off by default.
- **Claude extended thinking on Vertex AI** is supported as well. The same
  `AGEZT_GOOGLE_VERTEX_THINKING_BUDGET` opt-in now drives extended thinking for
  `claude-*` models served through Vertex (`:rawPredict` / `:streamRawPredict`),
  with the budget clamped to Anthropic's 1024-token floor and `max_tokens` bumped
  above it — matching the direct Anthropic adapter. Claude's chain of thought is
  captured into the same reasoning pipeline. With this, *every* reasoning-capable
  provider Agezt speaks — direct Anthropic, direct Gemini, Vertex Gemini, Vertex
  Claude, and openai-compatible DeepSeek-R1 — surfaces its reasoning uniformly.
  Off by default.
- **Reasoning reaches the editor (ACP).** When Agezt runs as an ACP agent (`agt
  acp`, e.g. inside Zed), a reasoning model's chain of thought is now relayed as
  `agent_thought_chunk` session updates — distinct from the answer's
  `agent_message_chunk` — so the editor renders it in its dedicated "thinking" UI.
  Previously the reasoning was captured but dropped at the ACP boundary; only the
  answer streamed through. Non-reasoning runs are unchanged.
- **Reasoning reaches OpenAI-compatible API clients.** When you point a client at
  Agezt's OpenAI-compatible endpoint (`/v1/chat/completions`) and the model
  reasons, its chain of thought is now surfaced as `reasoning_content` — on
  `message.reasoning_content` for non-streaming responses and as
  `delta.reasoning_content` chunks when streaming — the DeepSeek-R1 convention
  many clients already render. Non-reasoning runs omit the field entirely (the
  response is byte-identical to before). With ACP above, the captured reasoning
  now reaches both of Agezt's external surfaces.
- **Reasoning on the Responses API too.** The newer `/v1/responses` surface now
  carries a reasoning model's chain of thought as a `reasoning` output item (with
  a `summary_text`), and streams it as `response.reasoning_summary_text.delta` /
  `.done` events — the Responses-API shape, distinct from the answer's
  `output_text`. Non-reasoning runs are unchanged. Reasoning now spans both
  OpenAI-compatible endpoints (Chat Completions + Responses).

### Fixed
- **Bedrock Mistral/Cohere runs now report real token spend.** Those vendors'
  response bodies carry no token counts, so the governor saw zero spend and
  under-billed them. Agezt now overlays Bedrock's authoritative
  `X-Amzn-Bedrock-Input-Token-Count` / `-Output-Token-Count` response headers onto
  the usage when the decoded body has none — so cost accounting and per-run budget
  caps work for every Bedrock vendor. Vendors that already report inline counts
  (Anthropic, Nova, Meta-Llama, AI21 Jamba) keep their richer body-derived usage.
- **Non-streaming reasoning is no longer dropped.** When a run used a provider's
  non-streaming path (no token streaming), a reasoning model's chain of thought
  was captured on the response but never published as an `llm.reasoning` event —
  so it was invisible to every consumer (`agt pulse`, the ACP thought-chunk relay,
  the OpenAI API's `reasoning_content`); only its character count survived. The
  loop now emits the reasoning as a single ephemeral event on the non-streaming
  path too, so reasoning capture is uniform whether or not the provider streams.
- **Ollama now honours the run's token cap.** `MaxTokens` is forwarded as
  Ollama's `options.num_predict`, so a local model respects the same output limit
  every cloud provider enforces — previously the cap was silently dropped on
  Ollama. Uncapped runs are unchanged.
- **Credential vault: a corrupt or tampered vault file no longer crashes the
  process.** `decryptVault` now validates the nonce length before calling
  AES-GCM `Open` — Go's GCM *panics* (rather than returning an error) on a nonce
  that isn't 12 bytes, so a vault whose `nonce` base64-decodes to the wrong length
  (disk corruption, a truncated write, or deliberate tampering) would have crashed
  the daemon/CLI instead of failing cleanly. It now returns a clear
  "vault corrupt or tampered" error. (Ciphertext and salt lengths were already
  safe — GCM errors on a short ciphertext and PBKDF2 accepts any salt.)

### Added
- **Structured output / JSON mode — provider request support.** A new
  `JSONMode` request flag makes OpenAI (and every OpenAI-compatible vendor, plus
  Azure and Mistral) send `response_format: {type: json_object}`, Ollama send
  `format: "json"`, and the Gemini family (Google direct + Vertex) send
  `generationConfig.responseMimeType: application/json` — the "reliability over
  free-form parsing" path from the spec. Now covers every provider with a native
  JSON mode. Default-off; providers without one ignore it. **Plan generation**
  (SPEC-10's canonical case) now uses it, and the **OpenAI-compatible API honours
  a client's `response_format`** (`json_object` / `json_schema` → JSON mode on
  the run), so an external client gets structured output from any capable
  provider. The **Responses API** honours it too (`text.format` or
  `response_format`). `agt provider check --caps` now reports `json mode` per
  model so operators can see which providers support it natively.
- **Ollama now supports local vision models.** Image attachments are forwarded to
  Ollama as base64 in the chat `images` array, and auto-discovery marks multimodal
  models (llava, llama3.2-vision, moondream, …) image-capable so the vision gate
  lets attachments through — local, private image understanding with no cloud
  provider. Text-only Ollama requests are unchanged.
- **`agt doctor` now confirms the AWS credential chain at preflight.** The
  production-readiness check reports which keyless/ambient layer engaged, tagging
  IRSA / SSO / assume-role with `[keyless: …]` — so a cloud deployment can be
  verified in one pass alongside the sandbox/provider/exposure checks.
- **`agt status` now shows the resolved AWS credential chain.** Which keyless /
  ambient layer engaged — IRSA/web-identity, SSO, assume-role, IMDS — is now in
  the status round-trip (`aws creds : AWS chain: …`), so an operator on EKS can
  confirm IRSA is live without grepping the boot banner. Quiet when AWS
  credentials aren't configured.
- **AWS Bedrock now supports IRSA / EKS Pod Identity (keyless web-identity
  credentials).** When the cluster injects the standard `AWS_WEB_IDENTITY_TOKEN_FILE`
  and `AWS_ROLE_ARN` env vars, Agezt automatically exchanges the projected OIDC
  token at STS (`AssumeRoleWithWebIdentity`) for temporary credentials — no static
  access key, and a pod assumes its OWN role instead of the node's IMDS role. The
  call is unsigned (the OIDC token is the proof of identity). Auto-detected, no
  config; the boot banner reports `web_identity=<role>`. This is the AWS twin of
  the Vertex GCE/GKE metadata support — keyless ambient credentials on both clouds.
- **Vertex AI now supports ambient credentials via the GCE/GKE metadata server.**
  Set `GOOGLE_VERTEX_USE_METADATA=1` (instead of `GOOGLE_APPLICATION_CREDENTIALS`)
  to authenticate from the instance metadata server on Compute Engine, GKE with
  Workload Identity, or Cloud Run — short-lived rotating tokens, no static
  service-account key file on disk (the production-recommended path). The project
  id is auto-discovered from the same metadata server when `GOOGLE_VERTEX_PROJECT`
  is unset; `GOOGLE_VERTEX_METADATA_URL` overrides the metadata base for a
  proxy/sidecar. The service-account JSON path is unchanged.
- **Regression tests lock in the provider empty-response guards.** Every provider
  decoder already rejects a response whose `choices`/`candidates` array is empty
  (a flaky proxy that truncates the body, or a Gemini safety block) rather than
  indexing `[0]` and panicking — but four of those guards (OpenAI, Google,
  Vertex-Gemini, AI21-Jamba-on-Bedrock) had no test. They now do: an end-to-end
  "empty array → clean error, never panic" test per provider, driven through the
  public `Complete` via an `httptest` server, so a future refactor that drops a
  guard fails loudly instead of regressing into a crash.
- **Anthropic prompt caching now covers the system prompt too — across the whole
  Claude family.** The direct Anthropic provider, Claude-on-Bedrock, and
  Claude-on-Vertex all send the system prompt as a cache-marked block array (not a
  bare string), adding a second cache breakpoint. Anthropic caches the prefix
  tools→system, so this caches the whole stable prefix of an agent loop (tools AND
  system), not just the tools — more of the repeated request hits the ~0.1× cache
  read rate. An empty system prompt is omitted entirely (unchanged).
- **Anthropic prompt caching is now requested — across the whole Claude family.**
  The Anthropic provider (direct), Claude-on-Bedrock, and Claude-on-Vertex all mark
  the last tool definition with `cache_control: {type: ephemeral}`, so the provider
  caches the stable tools prefix that repeats on every iteration of an agent loop.
  Previously no encoder sent a cache breakpoint, so Anthropic never cached and the
  cache-token accounting (which only *reads* the response) always saw zero — this
  is the request-side piece that makes those savings real (cache reads bill at
  ~0.1× input; surfaced by `agt cache` / the Web UI Cache panel). The provider
  silently ignores the marker when the prefix is below the minimum cacheable size,
  so it's safe to always set. (OpenAI caches automatically.)
- **Web UI: a Policy panel.** Surfaces what the Edict policy engine is doing —
  total decisions, allowed/denied (with hard-deny count), denial rate, and a
  denied-by-capability breakdown — by proxying `edict_stats`. Clicking it opens a
  modal with the recent decision log (allow / DENY / DENY(hard) per capability +
  tool + reason), via `edict_log`. The Web UI counterpart of
  `agt edict stats` / `agt edict log`; refreshes live off `policy.*` events.
- **Web UI: the Schedules panel shows each entry's next-fire time.** Every
  enabled schedule now renders `next <local date+time>` (from the same
  `next_run_unix` the `agt schedule list` CLI already shows), so an operator can
  see *when* autonomous work will next run, not just its cadence.
- **Claude-on-Bedrock cache-token accounting.** The AWS Bedrock provider
  (streaming + non-streaming) now parses `cache_read_input_tokens` /
  `cache_creation_input_tokens` from Claude's usage — fixing the same under-count
  M290 fixed for direct Anthropic (Bedrock also reports `input_tokens` excluding
  cached). Cache reads bill at the cache-read rate, creations at the cache-write
  premium. Cache-token parsing now covers OpenAI/compat, Anthropic (direct +
  Vertex + Bedrock), and Gemini (direct + Vertex).
- **Gemini cache-token accounting.** The Google (direct) and Gemini-on-Vertex
  providers (streaming + non-streaming) now parse
  `usageMetadata.cachedContentTokenCount` into `agent.Usage.CachedInputTokens`, so
  context-cached prompt tokens bill at the cache-read rate. Gemini's
  `promptTokenCount` already includes the cached subset, so the input total is
  unchanged. Extends the cache cost model to a third provider family (after
  OpenAI/compat and Anthropic).
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
