// SPDX-License-Identifier: MIT

// Data + research + config + runs + memory + schedule + tenant + disk/storage commands.
// Code extracted from protocol_commands.go during the Day-43 god-file split. Public API unchanged.
package controlplane


const (
	// Council of Elders (M839) — the Web UI consults the multi-model panel (M837).
	// CmdCouncilMembers returns the default membership the council will convene with
	// (so the UI shows which models speak). Returns: members — []object{seat,model}.
	CmdCouncilMembers = "council_members"
	// CmdCouncilAsk convenes the council on a question and returns the full result.
	// Args: question (req), rounds. Returns: {consensus, dissent, members, rounds, opinions}.
	CmdCouncilAsk = "council_ask"
	// CmdCouncilSet replaces the default council membership. Args: members — array of
	// {seat, model}. Applies live and persists to the config store so it survives restart.
	CmdCouncilSet = "council_set"

	// Conductor (M997) — the operator surface for the asymmetric, verify-driven
	// panel (kernel/runtime, M997). The agent reaches the same engine through the
	// `conductor` tool.
	// CmdConductorRoles returns the default role→model assignment the Conductor
	// will use when roles aren't given (so the UI/CLI can preview who fills each
	// role). Returns: {thinker, worker, verifier, available_models}.
	CmdConductorRoles = "conductor_roles"
	// CmdConductorAsk runs the full Thinker/Worker/Verifier loop and returns the
	// answer + transcript. Args: task (req), thinker, worker, verifier (model ids
	// or "@chains"), max_rounds, plan (bool), corr. Returns: {answer, passed,
	// roles, rounds, plan, steps, correlation_id}.
	CmdConductorAsk = "conductor_ask"
	// CmdResearchAsk runs the deep-research harness (M1001) and returns a cited
	// report. Args: question (req), max_sub_questions, max_sources, verify (bool,
	// default true), max_verify_claims, corr. Returns: {question, sub_questions,
	// sources, markdown, claims, confidence, cited_sources, verified, notes,
	// correlation_id}.
	CmdResearchAsk = "research_ask"

	// CmdJournalGrep is the server-side filter sibling of
	// CmdJournalTail. Today operators run `agt journal tail 10000
	// --json | jq 'select(...)'` which loads the entire tail into
	// the client just to throw most of it away — for a journal
	// with 100k+ events the client-side filter cost dominates the
	// daemon round-trip. CmdJournalGrep moves the filter into the
	// server: it walks the journal once and only ships matching
	// events back.
	//
	// Filters AND together (all must match):
	//   - pattern        : substring (case-INSENSITIVE) matched against
	//                      kind, subject, actor, correlation_id, AND
	//                      the raw payload bytes. Empty pattern = match all.
	//   - kind           : exact match on Event.Kind (e.g. "tool.invoked").
	//   - subject        : exact match on Event.Subject.
	//   - actor          : exact match on Event.Actor.
	//   - correlation_id : exact match on Event.CorrelationID.
	//   - limit          : int (optional; default 100, clamped to 1..10000) —
	//                      maximum matches returned. Walking stops once
	//                      the limit is reached (oldest→newest order).
	//
	// Returns the same shape as CmdJournalTail so renderers reuse:
	//   - events : [*event.Event, ...] in seq order
	//   - count  : int — how many matched (may equal limit)
	//   - head   : int — current journal head seq
	//
	// Privacy note: the substring search inspects payload bytes
	// for free-text "find anything that mentions X" debugging. The
	// daemon does not redact or summarise — operators get back the
	// same payload they'd see in `agt why` or `agt journal tail`.
	CmdJournalGrep = "journal_grep"

	// CmdConfig returns a snapshot of the daemon's effective config:
	// resolved paths, model name, system-prompt presence (NOT
	// content), inventory counts (tools, plugins), and which
	// AGEZT_* env vars the operator has set. Closes the "what is
	// this daemon ACTUALLY running with?" question that today
	// requires reading the kernel's startup log line by line.
	//
	// Privacy: env-var values are NOT included — only presence
	// (true/false). This matters for AGEZT_VAULT_PASSPHRASE,
	// AGEZT_TASK_BUDGETS (which sometimes embed cost ceilings
	// operators don't want to log), and AGEZT_PLUGIN_PINS (which
	// embeds blake3 digests; surfacing them is fine but the
	// blanket "presence-only" rule is simpler to reason about).
	// system_prompt is reported as a bool for the same reason —
	// the prompt body can contain proprietary instructions.
	//
	// No args. Returns:
	//   - paths       : {base, journal, state, runtime, catalog, vault}
	//   - model       : string (empty when using provider defaults)
	//   - system_prompt_set : bool (NOT the content)
	//   - tool_count  : int  — registered tools
	//   - plugin_count: int  — external plugins spawned
	//   - ask_policy  : string ("allow"|"deny"|"prompt")
	//   - env         : {VARNAME: true} — only PRESENT vars listed
	//                   (absent ones omitted to keep the map small)
	CmdConfig = "config"

	// CmdConfigCenterSet sets a config entry in the Config Center.
	// This is an admin-only operation that bypasses access control.
	// Args:
	//   - key         : string (required)
	//   - value       : string (required)
	//   - rating      : string (optional: "public", "internal", "restricted", "secret")
	//   - description : string (optional)
	// Returns:
	//   - entry: ConfigEntry
	CmdConfigCenterSet = "configcenter.set"

	// CmdConfigCenterGet retrieves a config entry from the Config Center.
	// This is an admin-only operation that bypasses access control.
	// Args:
	//   - key: string (required)
	// Returns:
	//   - entry: ConfigEntry
	CmdConfigCenterGet = "configcenter.get"

	// CmdConfigCenterList lists all config entries in the Config Center.
	// This is an admin-only operation that bypasses access control.
	// Args:
	//   - rating : string (optional: filter by rating)
	// Returns:
	//   - entries: []ConfigEntry
	CmdConfigCenterList = "configcenter.list"

	// CmdConfigCenterDelete deletes a config entry from the Config Center.
	// This is an admin-only operation.
	// Args:
	//   - key: string (required)
	// Returns:
	//   - deleted: bool
	CmdConfigCenterDelete = "configcenter.delete"

	// CmdConfigCenterSetRating sets the rating for a config entry.
	// Args:
	//   - key    : string (required)
	//   - rating : string (required: "public", "internal", "restricted", "secret")
	// Returns:
	//   - override: bool (true if manual override was applied)
	CmdConfigCenterSetRating = "configcenter.set-rating"

	// CmdConfigCenterSetAccess updates only a config entry's agent allow/deny
	// lists, preserving the value, rating, metadata, and description.
	// Args: key, allowed_agents, excluded_agents. Returns: { entry }.
	CmdConfigCenterSetAccess = "configcenter.access"

	// CmdConfigCenterAccessLog returns the access log for config entries.
	// Args:
	//   - key     : string (optional: filter by key)
	//   - agent_id: string (optional: filter by agent)
	//   - since   : string (optional: duration like "24h")
	// Returns:
	//   - logs: []AccessLogEntry
	CmdConfigCenterAccessLog = "configcenter.access-log"

	// CmdConfigCenterAudit returns the audit log for config operations.
	// Args:
	//   - since: string (optional: duration like "24h")
	// Returns:
	//   - entries: []AuditEntry
	CmdConfigCenterAudit = "configcenter.audit"

	// CmdConfigCenterHealth returns health status of the Config Center.
	// Returns:
	//   - status : string
	//   - checks : map[string]string
	//   - stats  : map[string]any
	CmdConfigCenterHealth = "configcenter.health"

	// CmdRunsList enumerates past agent runs by scanning the
	// journal for task.received / task.completed pairs. Each
	// task.received starts a "run"; the matching task.completed
	// (same correlation_id) marks success. Unmatched task.received
	// events are reported as status="running" or "abandoned"
	// depending on whether the kernel still has the correlation
	// in its active map.
	// Args:
	//   - limit : int (optional; default 20, clamped to 1..1000)
	// Returns:
	//   - runs : [{correlation_id, intent, status, reason,
	//             started_unix_ms, completed_unix_ms, duration_ms,
	//             iters, parent_correlation}, ...] sorted by
	//           started_unix_ms DESCENDING (newest first). status ∈
	//           {completed, failed, abandoned, running}; reason carries the
	//           task.failed tag (M30) when status=failed, else "".
	//           parent_correlation links a sub-agent run to the lead run that
	//           delegated it (M41), else "".
	//   - count : int
	CmdRunsList = "runs_list"

	// CmdReaperScan runs the reaper's read-only scan (#53, M903): dead-agent
	// candidates (enabled, non-retired, idle past idle_days) + stale-artifact
	// totals (older than stale_days). Detection only — the operator still retires
	// (graveyard) / collects. Args: idle_days, stale_days (both default 30).
	CmdReaperScan = "reaper_scan"

	// CmdRunsStats aggregates the entire journal into a single
	// agent-run health summary. Pure read-only fold over the same
	// task.received/completed/abandoned events CmdRunsList pairs,
	// but over ALL runs (no limit/sort window — a "last N" stat
	// would make success-rate and percentiles meaningless).
	// Args:
	//   - since_ms : int (optional) — restrict to runs that STARTED
	//                within the last since_ms (server clock). 0/absent
	//                = all-time. A windowed view answers "how have runs
	//                done in the last hour" (M33).
	// Returns:
	//   - window_ms   : int — the window width covered (0 = all-time)
	//   - total       : int — distinct correlation ids seen
	//   - completed    : int — runs with a task.completed
	//   - failed       : int — runs with a task.failed (M30)
	//   - failed_by_reason : {reason→count} — failure breakdown by M30
	//                    reason tag (error|max_iters|canceled|timeout|
	//                    unknown); empty when no failures (M36)
	//   - running      : int — received, no terminal event (in-flight)
	//   - abandoned    : int — reconciled at boot (M28), never completed
	//   - terminal     : int — completed + failed + abandoned
	//   - success_rate : float — completed / terminal (0 if terminal==0)
	//   - avg_iters    : float — mean iters over completed runs
	//   - duration_ms  : {count, avg, min, max, p50, p95} over completed
	//                    runs only (running/abandoned have no end time)
	CmdRunsStats = "runs_stats"

	// CmdCancelRun cancels a single in-flight run by correlation id (M32),
	// leaving the kernel un-halted and every other run untouched — the
	// targeted alternative to CmdHalt (which cancels ALL runs and blocks
	// new ones until resume). The cancelled run's agent loop returns
	// context.Canceled, which the M30 terminal emitter records as
	// task.failed(reason=canceled).
	// Args:
	//   - correlation : string (required) — the run's correlation id
	//   - tenant      : string (optional) — route to a tenant kernel
	// Returns:
	//   - correlation : string — echoed back
	//   - cancelled   : bool — true if a live run matched, false otherwise
	//                   (already finished / never existed / wrong id)
	CmdCancelRun = "cancel_run"

	// Live run steering (M608) — fly a running agent without cancelling it.
	// Each targets one live run by correlation id and is tenant-routable (an
	// operator may steer their own tenant's runs, like CmdCancelRun). All four
	// take {correlation, tenant?} and return {correlation, ok}; CmdRunSteer also
	// takes {directive} and returns {accepted}.
	//   - CmdRunPause  : park the agent at its next iteration boundary.
	//   - CmdRunResume : let a paused agent run freely again.
	//   - CmdRunStep   : advance exactly one iteration then re-pause.
	//   - CmdRunSteer  : inject an operator directive that the loop folds into
	//                    the next prompt (emits run.steered when it takes effect).
	CmdRunPause  = "run_pause"
	CmdRunResume = "run_resume"
	CmdRunStep   = "run_step"
	CmdRunSteer  = "run_steer"
	// CmdRunIntervene is the protocolized intervention grammar over live runs.
	// Args: {primitive: halt|abort|redirect|adjust|query, correlation, directive?,
	// lease_ms?, scope?, idempotency_key?}. Returns the transaction result.
	CmdRunIntervene = "run_intervene"

	// Memory-lite (ROADMAP §2.3). The content-addressed, journaled
	// knowledge store the agent reads as injected context. These give
	// operators a read/write path without shelling into the data dir.
	//
	// CmdMemoryAdd stores (or reinforces) a record.
	// Args:
	//   - subject    : string (optional) — entity/topic
	//   - content    : string (required) — the text to remember
	//   - type       : string (optional) — FACT|SUMMARY|RELATION|
	//                  PREFERENCE|OBSERVATION (default FACT)
	//   - confidence : number (optional, 0..1; default 1)
	//   - tags       : {k:v} (optional)
	// Returns: { id, created (bool), type, subject }
	CmdMemoryAdd = "memory_add"

	// CmdMemorySupersede revises a record (M731): stores a new record and links
	// the old one's superseded_by to it (soft update — history retained, recall
	// uses the new one). The model-correct "edit" for a content-addressed store.
	// Args: old_id (required) + the new record fields (content required, subject/
	// type/confidence/tags optional, as CmdMemoryAdd).
	// Returns: { new_id, old_id, superseded (bool), type, subject }
	CmdMemorySupersede = "memory_supersede"

	// CmdMemoryList returns active (non-tombstoned, non-superseded)
	// records, newest activity first. No args.
	// Returns: { records: [...], count }
	CmdMemoryList = "memory_list"

	// CmdMemoryLog lists recent memory operations (M85) — a timeline of the
	// journal's memory.written/forgotten/superseded events (what the agent
	// learned, forgot, replaced). Args: limit (optional), op (optional:
	// written|forgotten|superseded), since_ms (optional window). Returns:
	// { ops: [ {ts_unix_ms, op, id, type, subject} ], count }
	CmdMemoryLog = "memory_log"

	// CmdMemoryGet reads one record by id (any state).
	// Args: id (required). Returns: { found, record }
	CmdMemoryGet = "memory_get"

	// CmdMemorySearch ranks active records by keyword×confidence×recency.
	// Args: query (required), limit (optional; default 10, 1..100).
	// Returns: { results: [{record, score}, ...], count }
	CmdMemorySearch = "memory_search"

	// CmdMemoryForget tombstones a record (soft delete; reversible,
	// retained on disk and in the journal).
	// Args: id (required). Returns: { forgotten (bool) }
	CmdMemoryForget = "memory_forget"

	// CmdMemoryPromote (M915) shares a private record: clears its scope tag so
	// the record joins the shared brain every agent recalls. The selective-
	// sharing valve over per-agent memory — agents write private by default;
	// the operator promotes the few notes worth everyone knowing. Idempotent on
	// an already-shared record.
	// Args: id (required). Returns: { promoted (bool, false = unknown id), id, subject }
	CmdMemoryPromote = "memory_promote"

	// CmdMemoryConsolidate (M804) runs one brain-distillation pass: cluster
	// related records by local embedding, LLM-merge each cluster into one
	// consolidated record, supersede the originals (soft, reversible).
	// Args: none. Returns the pass report (clusters, merges, supersessions).
	CmdMemoryConsolidate = "memory_consolidate"

	// CmdProfileRebuild (M1000) runs one operator-profile distillation pass:
	// synthesize the operator's profile facets from accumulated shared memory and
	// write them as reinforced PREFERENCE records. Args: none. Returns the report
	// (input records, facets written).
	CmdProfileRebuild = "profile_rebuild"

	// CmdMemoryPrune hard-removes soft-deleted (tombstoned/superseded) records
	// older than older_than_days — reclaiming dead weight so memory can't grow
	// unbounded (M857). dry_run (default) reports hygiene + prunable count.
	// Args: older_than_days (optional; default 30), dry_run (optional; default true).
	// Returns: { dry_run, older_than_days, cutoff_ms, prunable|pruned, stats }
	CmdMemoryPrune = "memory_prune"

	// CmdMemoryTidy collapses near-duplicate auto-distilled notes by subject (the
	// backlog from before the M993 write-time gate). dry_run (default) reports how
	// many would be collapsed; dry_run=false forgets the redundant ones, keeping
	// the strongest note per subject. Args: dry_run (optional; default true).
	// Returns: { dry_run, collapsed }.
	CmdMemoryTidy = "memory_tidy"

	// CmdMemoryBulkForget soft-deletes multiple records in one operation.
	// Args: ids (required, array of string). Returns: { forgotten: N, not_found: M }.
	CmdMemoryBulkForget = "memory_bulk_forget"

	// CmdMemoryFindRelated uses embedding-based similarity to find records related
	// to a given seed record. Given the seed's id, fetches its content, embeds it,
	// and returns the top-k most semantically similar active records.
	// Args: id (required), limit (optional; default 10, max 100).
	// Returns: { results: [{record, score}, ...], count }.
	CmdMemoryFindRelated = "memory_find_related"

	// CmdMemoryAudit reports epistemic hygiene: usable vs suspended/expired
	// records and same-subject competing memories. Read-only.
	// Args: none. Returns: memory.AuditReport.
	CmdMemoryAudit = "memory_audit"

	// CmdMemoryClean reports or soft-forgets low-value long-term memories using
	// the same retention filter applied to automatic writes. Dry-run by default.
	// Args: dry_run (optional; default true). Returns: memory.CleanReport.
	CmdMemoryClean = "memory_clean"

	// CmdChatSuggestions returns context-aware suggested next prompts for the
	// chat surface, blending memory-derived starters (from the agent's active
	// memory) with tool-context suggestions. Rule-based, no LLM call. Args:
	// session_id (optional), tools (optional, comma-joined recent tool names).
	// Returns: { suggestions: [ChatSuggestion] }.
	CmdChatSuggestions = "chat_suggestions"

	// Typed schedules (autonomy). The cadence resident fires due agent,
	// workflow, system-task, or tool targets; these commands manage the
	// persistent store behind `agt schedule`.
	//
	// CmdScheduleAdd creates a typed cron job. Historical target="" schedules
	// wake an agent for a governed LLM task; target=workflow runs a workflow; target=system_task
	// runs a daemon maintenance task; target=tool invokes a registered tool with
	// payload directly. Args include cadence fields plus intent/model/agent or
	// workflow/system_task/tool/payload depending on target.
	// Returns: { id, intent, target, interval_sec, next_run_unix, ... }
	CmdScheduleAdd = "schedule_add"
	// CmdScheduleList returns all schedules.
	// Returns: { schedules: [ {id,intent,target,workflow,system_task,tool,
	//            payload,interval_sec,model,source,enabled,created_unix,
	//            last_run_unix,next_run_unix} ], count }
	CmdScheduleList = "schedule_list"
	// CmdScheduleSystemTasks returns daemon maintenance task names allowed for
	// target=system_task schedules. Read-only metadata shared by CLI/UI/tools.
	// Returns: { system_tasks: [string], count }
	CmdScheduleSystemTasks = "schedule_system_tasks"
	// CmdScheduleRemove deletes a schedule by id.
	// Args: id (required). Returns: { removed (bool) }
	CmdScheduleRemove = "schedule_rm"
	// CmdScheduleRun marks a schedule due immediately (fires on the next tick).
	// Args: id (required). Returns: { triggered (bool) }
	CmdScheduleRun = "schedule_run"
	// CmdScheduleEnable enables or disables a schedule (pause/resume without
	// deleting). Args: id (required), enabled (bool). Returns: { updated, enabled }
	CmdScheduleEnable = "schedule_enable"
	// CmdScheduleEdit changes an existing schedule in place (preserving its id),
	// applying any subset of intent/model/agent/target fields and a new cadence
	// (interval_sec | at_minutes[+days] | once_at_unix). Returns:
	// { updated (bool), id, mode, cadence, target, ... }
	CmdScheduleEdit = "schedule_edit"
	// CmdScheduleFires lists recent scheduled-run FIRINGS (M54) — the autonomy
	// analogue of CmdRunsList. Walks the journal for schedule.fired events and
	// joins each with its run's outcome (status/duration/spend/answer). Args:
	// limit (optional), id (optional — only this schedule's firings, M55).
	// Returns: { fires: [ {correlation_id, schedule_id, fired_unix_ms, intent,
	// model, status, reason, duration_ms, spent_mc, answer_preview} ], count }
	CmdScheduleFires = "schedule_fires"
	// CmdScheduleStats aggregates scheduled-run FIRINGS (M57) — the autonomy
	// analogue of CmdRunsStats. Args: id (optional, one schedule), since_ms
	// (optional window). Returns: { total, completed, failed, running,
	// abandoned, success_rate, spent_microcents, schedules (distinct that fired),
	// failed_by_reason, window_ms }
	CmdScheduleStats = "schedule_stats"

	// CmdScheduleTest previews a schedule's upcoming fire times (M120) — a
	// read-only dry-run so an operator can confirm a daily/windowed/interval
	// cadence does what they expect before relying on it (parity with
	// `agt edict test` for policy). Args: id (required), count (default 5,
	// max 100). Returns forecasts [{unix}] + the rendered cadence.
	CmdScheduleTest = "schedule_test"

	// Multi-tenant management (ROADMAP P6-MULTI). The control-plane surface
	// behind `agt tenant`; operates on the daemon's tenant.Registry. Disabled
	// (returns an error) when the daemon has no registry configured.
	//
	// CmdTenantCreate creates/opens an isolated tenant. Args: id (required).
	// Returns: { id, base_dir, created (bool), token } — token is the tenant's
	// per-tenant credential for routing on externally-exposed surfaces.
	CmdTenantCreate = "tenant_create"
	// CmdTenantList lists tenants on disk. Returns: { tenants: [{id, base_dir, open}], count }
	CmdTenantList = "tenant_list"
	// CmdTenantRelease closes a tenant's kernel, keeping its state on disk.
	// Args: id (required). Returns: { released (bool) }
	CmdTenantRelease = "tenant_release"
	// CmdTenantRemove deletes a tenant and all its state (destructive).
	// Args: id (required). Returns: { removed (bool) }
	CmdTenantRemove = "tenant_remove"
	// CmdTenantToken reveals an existing tenant's per-tenant credential.
	// Args: id (required). Returns: { id, token }
	CmdTenantToken = "tenant_token"
	// CmdChangelog is the system timeline (SPEC-08 §4.2, M133): a curated,
	// tamper-evident fold of the journal showing only MATERIAL changes to this
	// system — halt/resume, policy changes, skill lifecycle (Forge), reflection,
	// catalog/provider sync, pulse pause/resume — newest-first, each carrying its
	// event id so `agt why` can explain it. Distinct from `journal tail` (raw, all
	// kinds): the human-meaningful "what changed about my system, and when".
	// Args: limit, since_ms. Returns: { entries: [{ts_unix_ms, kind, label,
	// detail, event_id, correlation_id}], count }.
	CmdChangelog = "changelog"

	// CmdJournalStats folds the journal into size/shape stats (M132): total event
	// count, segment count, bytes on disk, a per-event-kind breakdown, and the
	// oldest/newest event timestamps (the journal's time span). The journal is
	// append-only and full-retention, so this answers "how big is it and WHAT is
	// filling it" — the input to an archival decision. Returns: { events,
	// segments, bytes, by_kind, oldest_unix_ms, newest_unix_ms }.
	CmdJournalStats = "journal_stats"

	// CmdDiskStats reports the daemon's journal size on disk and the free/total
	// bytes of the filesystem it lives on (M131) — the data behind `agt disk` and
	// the doctor disk-space check. The journal is append-only, so a full disk is
	// the classic silent outage. Returns: { base_dir, journal_bytes,
	// disk_available (bool), disk_free_bytes, disk_total_bytes, disk_free_pct }.
	CmdDiskStats = "disk_stats"

	// CmdStorageStats breaks the daemon's home directory down per subsystem
	// (M927): for every top-level subdir under ~/.agezt, its on-disk bytes and
	// file count, labelled with what lives there, plus the same free/total
	// probe disk_stats uses. disk_stats says "the disk is filling"; this says
	// WHAT is filling it — the read side of the Storage cleanup surface.
	// Returns: { base_dir, total_bytes, total_files, dirs: [{name, bytes,
	// files, label}], disk_available, disk_free_bytes?, disk_total_bytes?,
	// disk_free_pct? }.
	CmdStorageStats = "storage_stats"

	// CmdTenantStats aggregates per-tenant run activity (M126): for each tenant
	// on disk it folds that tenant's own journal into run count / completed /
	// failed / active / spend / last activity, plus grand totals — the
	// cross-tenant usage view the primary operator otherwise lacks. Primary
	// token only (a tenant sees only its own runs via `runs stats`). Returns:
	// { tenants: [{id, runs, completed, failed, active, spent_microcents,
	// last_activity_unix_ms}], count, total_runs, total_spent_microcents }
	CmdTenantStats = "tenant_stats"
)
