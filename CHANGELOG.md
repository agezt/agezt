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
