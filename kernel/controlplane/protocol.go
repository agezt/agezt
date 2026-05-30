// SPDX-License-Identifier: MIT

// Package controlplane is the local control protocol between the agezt
// daemon and the agt CLI.
//
// Transport: TCP localhost, line-delimited JSON. The daemon binds to an
// ephemeral port; the client discovers it via two files under
// <BaseDir>/runtime/:
//
//	control.addr    text: "127.0.0.1:NNNNN\n"
//	control.token   text: hex token; clients must send it on every request
//
// Wire format — every line is one JSON object.
//
//	Request : {"id":"q1","cmd":"<name>","token":"<hex>","args":{...}}
//	Response: {"id":"q1","type":"event",  "event":{...}}        zero or more
//	Response: {"id":"q1","type":"result", "result":{...}}       exactly one
//	Response: {"id":"q1","type":"error",  "error":"reason"}     alternative final
//
// One request per connection: open, send, read until type=result|error,
// close. This keeps the protocol trivial and avoids multiplexing concerns.
//
// (Not JSON-RPC: that's the contract for the kernel↔plugin wire per
// DECISIONS B0. The control plane is process-local and uses a thinner,
// purpose-built shape so the CLI binary stays small.)
package controlplane

import "github.com/agezt/agezt/kernel/event"

// Command names supported by the control plane.
const (
	CmdVersion       = "version"
	CmdRun           = "run"
	CmdHalt          = "halt"
	CmdResume        = "resume"
	CmdWhy           = "why"
	CmdJournalVerify = "journal_verify"
	// HITL approval queue.
	CmdApprovals = "approvals" // list pending requests
	CmdDecide    = "decide"    // resolve one (args: id, decision="grant|deny", reason)
	// DAG scheduler.
	CmdPlan = "plan" // run a pre-built Plan (args: plan_json — see scheduler/PlanSpec)
	// Provider / model catalog (SPEC-15 §1; TASKS P1-CONDUIT-04).
	CmdCatalogSync     = "catalog_sync"     // (args: url? — defaults to AGEZT_CATALOG_URL or models.dev)
	CmdCatalogList     = "catalog_list"     // returns providers + models + pricing + credentials present
	CmdCatalogDiscover = "catalog_discover" // (args: endpoint? — Ollama /api/tags discovery)
	// Live reload of catalog + vault → rebuild primary provider in place
	// (M1.r). Replaces the "restart the daemon" friction printed by
	// `agt provider creds set` since M1.o.
	CmdProviderReload = "provider_reload"
	// Planner: ask the daemon's configured Provider to generate a
	// scheduler-shaped Plan JSON from a natural-language intent (M1.v).
	// Args: intent (string), optional model (string).
	// Returns: { "plan_json": "<the JSON string>", "node_count": N }.
	// The CLI can then forward plan_json to CmdPlan to execute, or
	// just print it for the operator to review.
	CmdPlanGenerate = "plan_generate"
	// Planner refinement (M1.uu): operator-driven re-plan with
	// feedback. Takes an existing plan JSON + free-text feedback,
	// returns a complete replacement plan in the same shape as
	// CmdPlanGenerate. Refinement is whole-replacement (not diff)
	// so re-validation catches any LLM mistakes the same way the
	// initial plan validators do.
	// Args: plan_json (string), feedback (string), optional model.
	// Returns: { "plan_json": "<new JSON>", "node_count": N }.
	CmdPlanRefine = "plan_refine"
	// Pulse: live operator observability (M1.u). Long-lived
	// subscription that streams bus events to the client until either
	// side closes the connection. Args: pattern (default ">"),
	// optional kinds filter ([]string). Never sends RespResult — the
	// client terminates the stream by closing the conn.
	CmdPulseSubscribe = "pulse_subscribe"

	// CmdBudget returns the governor's current spend snapshot.
	// Closes the M1.zz feedback loop: operators set per-task-type
	// daily caps but had no way to see how close they were to
	// hitting them. Returns:
	//   - utc_date         (string YYYY-MM-DD) — the day the
	//                      counters are scoped to.
	//   - spent_mc         (int)    — total spend today, microcents.
	//   - ceiling_mc       (int)    — daily ceiling (0 = unlimited).
	//   - per_task         (map[string]{spent_mc, ceiling_mc}) —
	//                      one entry per configured TaskBudget;
	//                      empty when no per-task caps configured.
	// No args.
	CmdBudget = "budget"

	// CmdToolList returns the in-process tool inventory the agent
	// loop will advertise to the model. Sister command to
	// CmdCatalogList (providers) — operators frequently want to
	// confirm a plugin actually registered its tool before
	// debugging "the model never called my tool" issues.
	// No args. Returns:
	//   - tools: [{name, description}, ...] sorted by name.
	//   - count: int
	// Source-of-tool ("in-process" vs "plugin: <name>") is not
	// distinguished at the kernel boundary — tools all satisfy
	// the same agent.Tool interface by the time they reach the
	// loop. The CLI surfaces the description so plugin tools that
	// follow the convention of prefixing their description ("[via
	// plugin foo] ...") remain self-identifying.
	CmdToolList = "tool_list"

	// CmdStatus is a one-shot daemon health overview. Existed as
	// scattered fields across CmdVersion + CmdBudget + CmdToolList;
	// this consolidates the operator-friendly "is my daemon
	// healthy, and what's it doing?" check into a single round-trip.
	// Also surfaces daemon version so the CLI can detect client/
	// daemon skew (agt version was previously client-only).
	// No args. Returns:
	//   - daemon         (string) — daemon binary version
	//   - protocol       (int)    — protocol version
	//   - uptime_seconds (int)    — seconds since Open()
	//   - halted         (bool)   — kernel halt flag
	//   - active_runs    (int)    — in-flight Run/RunPlan count
	//   - tools          (int)    — registered tool count
	//   - journal_head   (int)    — last journaled seq (0 = empty)
	CmdStatus = "status"

	// CmdPluginList enumerates the external plugins the daemon
	// spawned at startup. Sister to CmdToolList (which sees
	// tools by name but not the plugin they came from); operators
	// debugging "I configured plugin X but its tools aren't
	// available" use this to confirm the spawn actually happened
	// before chasing tool-registration issues.
	// No args. Returns:
	//   - plugins : [{prefix, path, args, tool_count,
	//                hash_pinned, allowed_tools}, ...]
	//             sorted by prefix.
	//   - count   : int
	CmdPluginList = "plugin_list"

	// CmdShutdown asks the daemon to exit gracefully. Same effect as
	// SIGTERM but reachable from any host that holds a valid control-
	// plane token — the gap that motivates this command is scripted
	// / CI workflows that need to stop the daemon without a shell on
	// the host (`pkill agezt` doesn't compose well in CI YAML; this
	// does). Handler writes `{ok:true}` first, then signals the
	// daemon's main loop to unblock and shut down after a short
	// delay so the client read completes before the process exits.
	// No args.
	CmdShutdown = "shutdown"

	// CmdJournalTail returns the last N events from the journal as a
	// one-shot historical read. Different from CmdPulseSubscribe with
	// --until: this never starts a subscription, never blocks, and
	// streams nothing — it's a synchronous snapshot for "show me what
	// just happened" use cases (postmortems, smoke tests, scrollback).
	// Args:
	//   - n : int (optional; default 20, clamped to 1..10000)
	// Returns:
	//   - events : [*event.Event, ...] in seq order, oldest→newest
	//   - count  : int — actual number returned (may be < n if the
	//              journal is shorter)
	//   - head   : int — current journal head seq (so the operator
	//              can compute "we showed events seq=(head-count+1)..head")
	CmdJournalTail = "journal_tail"

	// CmdEdictShow returns the loaded policy snapshot. Closes a
	// real visibility gap: operators set AGEZT_APPROVAL_MODE and
	// per-capability levels but had no way to confirm what the
	// engine actually loaded — `agt edict show` is the canonical
	// answer ("is shell really Ask-First in this deployment?",
	// "did my custom HardDeny rule make it in?").
	// No args. Returns:
	//   - ask_policy : string — "allow" | "deny" | "prompt"
	//   - levels     : {capability: level-name} — sorted, all caps
	//   - hard_deny  : [{name, substring, applies_to}, ...] — the
	//                  unconditional-block patterns, sorted by name
	CmdEdictShow = "edict_show"

	// CmdEdictTest dry-runs a policy decision: "if I asked to do
	// <capability> with this <input>, what would the engine say?"
	// Read-only — does not register the call as having happened,
	// does not consume approval slots, does not journal as a
	// real decision event. Useful for:
	//   - CI preflight: assert "rm -rf /" is hard-denied before
	//     running an agent loop that might receive it.
	//   - Debugging: "why was this allowed but the next one
	//     denied?" — operators can probe variations interactively.
	//   - Composing hard-deny rules: confirm a new pattern
	//     actually catches the inputs the operator expects.
	// Args:
	//   - capability : string (required) — must match a known
	//                  edict.Capability value (shell, file_read, ...)
	//   - input      : string (optional, default "") — the input
	//                  text the runtime would be checking against
	// Returns the engine.Outcome shape:
	//   - decision         : "allow" | "deny"
	//   - level            : string — TrustLevel.String() (L0..L4)
	//   - reason           : string — human explanation from engine
	//   - hard_denied      : bool
	//   - hard_deny_rule   : string — present iff hard_denied=true
	//   - would_ask        : bool — Ask-class folded by AskPolicy
	//   - requires_approval: bool — AskPrompt mode + Ask-class hit
	CmdEdictTest = "edict_test"

	// CmdStateList enumerates namespaces and (optionally) keys in
	// the kernel state store. State is normally invisible to
	// operators — agents and the scheduler write here but there's
	// no CLI path to read what's accumulated. Closing this gap
	// matters for debugging "why did the agent loop think X?" and
	// for postmortems on long-running runs.
	// Args:
	//   - namespace : string (optional) — if set, returns keys in
	//                 that namespace; otherwise returns the
	//                 sorted namespace list.
	// Returns:
	//   - namespaces : []string  (when namespace arg empty)
	//   - keys       : []string  (when namespace arg set)
	//   - namespace  : string    (echoed for context)
	CmdStateList = "state_list"

	// CmdStateGet reads a single (namespace, key) entry. Returns
	// the raw JSON value verbatim so jq pipelines can navigate it.
	// Args:
	//   - namespace : string (required)
	//   - key       : string (required)
	// Returns:
	//   - value : json.RawMessage — the stored value (any JSON shape)
	//   - found : bool — false when (ns, key) doesn't exist; value is null
	CmdStateGet = "state_get"

	// CmdJournalHead returns just the current journal head seq +
	// hash. The minimal-payload sibling of CmdJournalTail (which
	// includes events). Useful when an operator just needs to
	// remember a checkpoint to pass as `pulse --since <seq>`
	// later, or to poll for journal growth in a tight loop
	// without parsing every event.
	//
	// No args. Returns:
	//   - head  : int    — current head seq (0 on empty journal)
	//   - hash  : string — current chain-tail hash, 64-hex. On an
	//                      empty journal this is the 64-zero
	//                      genesis (which is what any first
	//                      event will use as prev_hash).
	CmdJournalHead = "journal_head"

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
	//   - runs : [{correlation_id, intent, status, started_unix_ms,
	//             completed_unix_ms, duration_ms, iters}, ...]
	//           sorted by started_unix_ms DESCENDING (newest first).
	//   - count : int
	CmdRunsList = "runs_list"

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

	// CmdMemoryList returns active (non-tombstoned, non-superseded)
	// records, newest activity first. No args.
	// Returns: { records: [...], count }
	CmdMemoryList = "memory_list"

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

	// World model (SPEC-05 §3). The journaled entity/relation graph behind
	// `agt world`; writes go through the kernel's worldmodel.Graph so every
	// node/edge mutation is auditable via `agt why`.
	//
	// CmdWorldAdd creates (or reinforces) an entity.
	// Args:
	//   - name    : string (required) — entity name
	//   - kind    : string (optional) — project|repo|person|org|account|
	//               device|channel|topic|task (default topic)
	//   - aliases : [string] (optional) — phrases that resolve to it
	//   - attrs   : {k:v} (optional) — preferences/habits/constraints
	// Returns: { id, created (bool), kind, name }
	CmdWorldAdd = "world_add"

	// CmdWorldRelate asserts a directed relation between two entities named
	// from/to (endpoints auto-created as topics if unknown).
	// Args: from (required), verb (optional; default relates_to), to (required).
	// Returns: { id, from, verb, to }
	CmdWorldRelate = "world_relate"

	// CmdWorldResolve ranks active entities matching a phrase.
	// Args: query (required), limit (optional; default 10, 1..100).
	// Returns: { results: [{entity, score}, ...], count }
	CmdWorldResolve = "world_resolve"

	// CmdWorldNeighbors lists the active edges incident to the entity that
	// best matches a phrase, with the adjacent entity for each.
	// Args: query (required). Returns: { entity, neighbors: [...], count }
	CmdWorldNeighbors = "world_neighbors"

	// CmdWorldList returns active entities (and a relation count). No args.
	// Returns: { entities: [...], count, relation_count }
	CmdWorldList = "world_list"

	// CmdWorldGet reads one entity by id (any state).
	// Args: id (required). Returns: { found, entity }
	CmdWorldGet = "world_get"

	// CmdWorldForget tombstones an entity (soft delete; reversible,
	// journaled). Args: id (required). Returns: { forgotten }
	CmdWorldForget = "world_forget"

	// Forge / skills (SPEC-05 §4–5). The journaled skill-lifecycle behind
	// `agt skill`; transitions go through the kernel's skill.Forge so every
	// promote/quarantine/revert is auditable via `agt why`.
	//
	// CmdSkillList returns all skills (any state), with an active count.
	// No args. Returns: { skills: [...], count, active_count }
	CmdSkillList = "skill_list"

	// CmdSkillGet reads one skill by id.
	// Args: id (required). Returns: { found, skill }
	CmdSkillGet = "skill_get"

	// CmdSkillHistory folds the journal for one skill's lifecycle events.
	// Args: id (required). Returns: { id, events: [...], count }
	CmdSkillHistory = "skill_history"

	// CmdSkillPromote advances draft→shadow→active (or un-quarantines).
	// Args: id (required). Returns: { id, status }
	CmdSkillPromote = "skill_promote"

	// CmdSkillQuarantine pulls an active/shadow skill from production.
	// Args: id (required), reason (optional). Returns: { id, status }
	CmdSkillQuarantine = "skill_quarantine"

	// CmdSkillRevert archives a skill and re-activates its lineage parent
	// (non-destructive — appends a reversal).
	// Args: id (required). Returns: { id, restored }
	CmdSkillRevert = "skill_revert"

	// Reflection — meta-cognition (SPEC-05 §6).
	//
	// CmdReflectRun runs one reflection pass now: folds the journal, applies
	// world-model decay, and journals the report. No args.
	// Returns: the Report (observations, entities_decayed, proposals).
	CmdReflectRun = "reflect_run"

	// CmdReflectShow returns the latest reflection report from the journal.
	// No args. Returns: { found, report }
	CmdReflectShow = "reflect_show"

	// Pulse — the proactive heart (SPEC-03). These control the resident
	// heartbeat the daemon runs in the background. When Pulse is disabled
	// (AGEZT_PULSE=off) the handlers report it rather than erroring.
	//
	// CmdPulseStatus returns the engine snapshot (running, beats,
	// observers, dial, cadence, last tick, pending digest). No args.
	CmdPulseStatus = "pulse_status"
	// CmdPulsePause suppresses new beats (in-flight processing finishes).
	// No args. Returns { paused: true }.
	CmdPulsePause = "pulse_pause"
	// CmdPulseResume re-enables beats. No args. Returns { paused: false }.
	CmdPulseResume = "pulse_resume"

	// CmdInbox returns the Unified Inbox (SPEC-07 §4): channel.inbound /
	// channel.outbound events folded into conversation threads grouped by
	// correlation_id, newest activity first.
	// Args: limit (optional; default 20, clamped 1..1000).
	// Returns: { threads: [{correlation_id, channel_kind, channel_id,
	//            messages:[{direction,sender,text,ts_unix_ms,event_id}],
	//            last_ts_unix_ms}, ...], count }
	CmdInbox = "inbox"
)

// Request is the wire shape sent by the client.
type Request struct {
	ID    string         `json:"id"`
	Cmd   string         `json:"cmd"`
	Token string         `json:"token"`
	Args  map[string]any `json:"args,omitempty"`
}

// Response types.
const (
	RespEvent  = "event"
	RespResult = "result"
	RespError  = "error"
)

// Response is the wire shape sent by the server. Exactly one of Event,
// Result, or Error is populated depending on Type.
type Response struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Event  *event.Event   `json:"event,omitempty"`
	Result map[string]any `json:"result,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// File names under <BaseDir>/runtime/.
const (
	addrFile  = "control.addr"
	tokenFile = "control.token"
)
