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
	// CmdApprovalsLog lists resolved + pending HITL approvals (M87) — a timeline
	// of approval.requested joined with the terminal granted/denied/timeout.
	// Args: limit, denied (bool), since_ms. Returns: { approvals: [ {ts_unix_ms,
	// approval_id, capability, tool, reason, status, resolved_by} ], count }
	CmdApprovalsLog = "approvals_log"
	// CmdApprovalsStats aggregates HITL approvals (M88) — total / granted /
	// denied / timeout / pending, grant rate, denied-by-capability. Args:
	// since_ms (optional window).
	CmdApprovalsStats = "approvals_stats"
	CmdDecide         = "decide" // resolve one (args: id, decision="grant|deny", reason)
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
	// CmdProviderLog lists recent provider-routing activity (M89) — a timeline of
	// routing.decision + provider.fallback events (which provider handled calls,
	// when the primary fell back). Args: limit, fallbacks (bool — only fallbacks),
	// since_ms. Returns: { events: [ {ts_unix_ms, kind, ...} ], count }
	CmdProviderLog = "provider_log"
	// CmdProviderStats aggregates provider routing (M90) — routed calls, fallback
	// count + rate, calls-by-primary, fallbacks-by-failed-provider. Args: since_ms.
	CmdProviderStats = "provider_stats"
	// CmdProviderRejections lists capability-gating events (M92) —
	// capability.rejected (M25 tool_call / M91 vision) + capability.rerouted (M40
	// down-route). Args: limit, since_ms. Returns: { rejections: [...], count }
	CmdProviderRejections = "provider_rejections"
	// Planner: ask the daemon's configured Provider to generate a
	// scheduler-shaped Plan JSON from a natural-language intent (M1.v).
	// Args: intent (string), optional model (string).
	// Returns: { "plan_json": "<the JSON string>", "node_count": N }.
	// The CLI can then forward plan_json to CmdPlan to execute, or
	// just print it for the operator to review.
	// CmdPlanHistory lists recent plan executions (M83) — the plan analogue of
	// CmdRunsList. Folds plan.started joined with plan.completed/plan.failed.
	// Args: limit (optional), status (optional: completed|failed|running).
	// Returns: { plans: [ {correlation_id, plan_name, node_count, status,
	// started_unix_ms, duration_ms} ], count }
	CmdPlanHistory = "plan_history"
	// CmdPlanStats aggregates plan executions (M84) — the plan analogue of
	// CmdRunsStats. Returns: { total, completed, failed, running, terminal,
	// success_rate, duration_ms: {count,avg,min,max,p50,p95} }
	CmdPlanStats    = "plan_stats"
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
	// CmdWardenLog lists recent sandboxed executions (M96) — a timeline of the
	// journal's warden.executed / profile_downgraded / limit_exceeded events
	// (the OS-sandbox audit). Args: limit, issues (bool — only downgrades/limit
	// breaches), since_ms. Returns: { executions: [...], count }
	CmdWardenLog = "warden_log"
	// CmdWardenStats aggregates sandboxed executions (M97) — total, downgraded
	// count + rate, timed-out, limit breaches, by-effective-profile. Args:
	// since_ms (optional window).
	CmdWardenStats = "warden_stats"
	// CmdWhoami reports the authenticated principal (M62) — whether the request
	// used the primary (admin) token or a tenant's own token, and which tenant.
	// Args: tenant (required for a tenant token, pinned by handleConn). Returns:
	//   - identity (string) — "primary" | "tenant"
	//   - primary  (bool)   — true for the admin token
	//   - tenant   (string) — the tenant id (empty for primary)
	CmdWhoami = "whoami"

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

	// CmdRedactTest checks whether the LIVE secret redactor would scrub a
	// candidate string before it could reach the hash-chained journal (M104) —
	// the "is my secret actually protected?" confidence check. The daemon never
	// echoes the raw input back: it returns only the redacted form plus which
	// built-in pattern categories matched, so the response is safe to display.
	//
	// Args:
	//   - text : string — the candidate to test.
	// Returns:
	//   - enabled      : bool — whether redaction is on (off → nothing scrubbed).
	//   - would_redact : bool — the redactor changed the input.
	//   - redacted     : string — the scrubbed form (safe to print).
	//   - categories   : []string — built-in pattern labels that matched.
	//   - literal_hit  : bool — a configured literal secret matched (no pattern).
	CmdRedactTest = "redact_test"

	// CmdRateLimitLog lists recent throttle events (M106) — a timeline of
	// rate.limited events (the governor refused a call because the per-minute
	// call cap was hit). Tenant-routed. Args: limit, since_ms. Returns events
	// [{ts_unix_ms, used, limit_per_min}] + count.
	CmdRateLimitLog = "ratelimit_log"

	// CmdRateLimitStats aggregates throttle events (M106) — total throttled,
	// the configured limit, and the worst observed `used` overshoot. Answers
	// "is this tenant/primary hitting its call-rate cap?". Tenant-routed.
	CmdRateLimitStats = "ratelimit_stats"

	// CmdNetguardLog lists egress connections the guard refused (M109) — a
	// timeline of netguard.blocked events (a tool tried to reach an internal /
	// metadata address). Args: limit, since_ms. Returns blocks
	// [{ts_unix_ms, ip, reason, tool}] + count. An audit trail for SSRF /
	// prompt-injection / exfiltration attempts. Tenant-routed.
	CmdNetguardLog = "netguard_log"

	// CmdWebhookLog lists recent outbound webhook deliveries (M112) — a timeline
	// of webhook.delivered / webhook.failed events. `--failed` keeps only the
	// failures. Args: limit, since_ms, failed. Returns deliveries
	// [{ts_unix_ms, status, url, event_kind, attempts, ok, error}] + count.
	// Webhook delivery was previously only reachable via `journal grep webhook`.
	CmdWebhookLog = "webhook_log"

	// CmdWebhookStats aggregates webhook deliveries (M112) — total, delivered,
	// failed, failure_rate, and a per-URL breakdown. Answers "are my
	// notifications getting through?". Tenant-routed.
	CmdWebhookStats = "webhook_stats"

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
	// CmdEdictOverlay returns the NET durable policy overlay (M94) — every
	// runtime policy.changed folded via ProjectPolicyChanges (the boot replay).
	// No args. Returns: { levels: {cap: level}, deny_rules: [...], mode,
	// empty, changes_folded }
	CmdEdictOverlay = "edict_overlay"
	// CmdEdictCompact collapses the durable policy overlay into a snapshot (M95)
	// so boot replays {snapshot + post-snapshot changes} instead of all history.
	// No args. Returns: { folded, compacted, through_seq, empty }
	CmdEdictCompact = "edict_compact"

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

	// CmdEdictDenyList enumerates the hard-deny rules currently loaded,
	// each tagged with whether it is removable at runtime. Same rows as
	// CmdEdictShow's hard_deny block, plus a `removable` flag: built-in
	// and AGEZT_EDICT_DENY (operator[N]) rules are the immutable floor;
	// only runtime[N] rules added via CmdEdictDenyAdd can be removed.
	// No args. Returns:
	//   - rules : [{name, substring, applies_to, removable}, ...]
	CmdEdictDenyList = "edict_deny_list"

	// CmdEdictDenyAdd appends a hard-deny rule at runtime — no restart.
	// The deny floor is security-critical, so the change is journaled as
	// a policy.changed event. Args:
	//   - rule : string (required) — one rule in AGEZT_EDICT_DENY syntax,
	//            "substring" (all caps) or "<capability>:substring"
	//            (scoped). Must parse to exactly one rule.
	// Returns: {name, substring, applies_to, count} — name is the
	// engine-assigned "runtime[N]" handle for a later remove.
	CmdEdictDenyAdd = "edict_deny_add"

	// CmdEdictDenyRemove removes a runtime-added hard-deny rule by name
	// and journals a policy.changed event. Refuses to remove a built-in
	// or operator[N] floor rule (error, not a silent no-op). Args:
	//   - name : string (required) — the "runtime[N]" handle.
	// Returns: {removed: bool, count}.
	CmdEdictDenyRemove = "edict_deny_rm"

	// CmdEdictSetLevel changes a capability's trust level at runtime — no
	// restart. The companion to CmdEdictDeny* for the other policy layer:
	// the trust ladder (L0 deny .. L4 allow). Loosening is safe by
	// construction — the hard-deny floor still fires regardless of level,
	// so even shell=L4 cannot pass `rm -rf /`. The change is journaled as
	// a policy.changed event. Args:
	//   - capability : string (required) — must be a known capability
	//                  (shell, file.read, http.post, ...). Unknown is an
	//                  error, never a silent default-deny entry.
	//   - level      : string (required) — "L0".."L4" or a word alias
	//                  (deny/ask/askfirst/askscoped/allow).
	// Returns: {capability, from, to} — the previous and new level labels.
	CmdEdictSetLevel = "edict_set_level"

	// CmdEdictSetMode changes the engine-wide approval mode (how Ask-class
	// levels L1..L3 are folded) at runtime — no restart. The third runtime
	// policy knob alongside CmdEdictDeny* and CmdEdictSetLevel. The hard-deny
	// floor is unaffected (it fires before AskPolicy), so even `allow` mode
	// can't relax a hard-deny. Journaled as a policy.changed event. Args:
	//   - mode : string (required) — "allow" | "deny" | "prompt".
	// Returns: {from, to} — the previous and new mode labels.
	CmdEdictSetMode = "edict_set_mode"
	// CmdEdictLog lists recent policy decisions (M63) — a read-only audit of the
	// journal's policy.decision events (every tool-call gating). Args: limit
	// (optional), denied (optional bool — only denials). Returns: { decisions:
	// [ {ts_unix_ms, actor, correlation_id, tool, capability, allow, reason,
	// hard_denied} ], count }
	CmdEdictLog = "edict_log"
	// CmdEdictStats aggregates policy decisions (M64) — the security-dashboard
	// analogue of CmdRunsStats. Args: since_ms (optional window). Returns:
	//   { total, allowed, denied, hard_denied, denial_rate,
	//     denied_by_capability: {cap → count}, window_ms }
	CmdEdictStats = "edict_stats"
	// CmdToolLog lists recent tool invocations (M66) — a read-only audit of the
	// journal's tool.invoked + tool.result events (what the agent actually ran).
	// The execution analogue of CmdEdictLog (which audits the policy gating of
	// those same calls). Args: limit (optional), errors (optional bool — only
	// failed calls), tool (optional name filter), since_ms (optional window).
	// Returns: { invocations: [ {ts_unix_ms, actor, correlation_id, tool,
	// call_id, input, output, error} ], count }
	CmdToolLog = "tool_log"
	// CmdToolStats aggregates tool invocations (M67) — the execution-dashboard
	// analogue of CmdEdictStats. Args: tool (optional name filter), since_ms
	// (optional window). Returns: { total, errored, error_rate, by_tool: {tool →
	// {calls, errors}}, tools, window_ms }
	CmdToolStats = "tool_stats"
	// CmdCacheStats aggregates prompt-cache usage + savings (M293) by folding
	// budget.consumed events. Args: since_ms (optional window). Returns:
	// { cached_input_tokens, cache_write_input_tokens, saved_microcents, calls,
	// window_ms } where saved_microcents is the difference between the no-cache
	// baseline (every input token at the full input rate) and the recorded
	// cache-aware cost, summed per call.
	CmdCacheStats = "cache_stats"

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

	// CmdJournalExport returns a complete, integrity-attested slice of
	// the journal for archival / compliance / disaster-recovery (M101).
	// Unlike CmdJournalTail (count-windowed) and CmdJournalGrep
	// (predicate-filtered for triage), export streams EVERY event —
	// optionally only those at/after a since_ms cutoff — with its hash
	// and prev_hash intact, plus the chain head at export time, so the
	// resulting bundle can be re-verified OFFLINE via
	// `agt journal verify --bundle <file>` (recompute each event's
	// BLAKE3 hash + check prev-hash continuity).
	//
	// Args:
	//   - since_ms : int (optional) — only events with ts >= now-since_ms.
	// Returns:
	//   - events    : []event — full events, ascending seq.
	//   - count     : int     — len(events).
	//   - first_seq / last_seq : int — seq bounds of the slice (-1 if empty).
	//   - head_seq / head_hash : int / string — chain head at export time.
	//   - truncated : bool    — true if the export hit the size cap.
	CmdJournalExport = "journal_export"

	// CmdArtifactGet fetches a content-addressed artifact (SPEC-04 §3.6) — the
	// full bytes of a tool output the agent loop offloaded out of the journal
	// (the tool.result event carries a raw_ref). The store re-verifies the bytes
	// against the ref on read, so a corrupted blob is rejected.
	//
	// Args:
	//   - ref : string — the 64-hex BLAKE3 content address (from a raw_ref).
	// Returns:
	//   - ref   : string — echoed.
	//   - size  : int    — byte length.
	//   - data  : string — base64-encoded bytes.
	CmdArtifactGet = "artifact_get"

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

	// Scheduled intents (autonomy). The cadence resident fires due schedules
	// through the governed loop; these commands manage the persistent store
	// behind `agt schedule`.
	//
	// CmdScheduleAdd creates a recurring schedule.
	// Args: intent (required), interval_sec (required, >= 1), model (optional).
	// Returns: { id, intent, interval_sec, next_run_unix }
	CmdScheduleAdd = "schedule_add"
	// CmdScheduleList returns all schedules.
	// Returns: { schedules: [ {id,intent,interval_sec,model,source,enabled,
	//            created_unix,last_run_unix,next_run_unix} ], count }
	CmdScheduleList = "schedule_list"
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
	// applying any of: intent, model, and a new cadence (interval_sec |
	// at_minutes[+days] | once_at_unix). Args: id (required) + whichever fields
	// change. Returns: { updated (bool), id, mode, cadence }
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

	// CmdTenantStats aggregates per-tenant run activity (M126): for each tenant
	// on disk it folds that tenant's own journal into run count / completed /
	// failed / active / spend / last activity, plus grand totals — the
	// cross-tenant usage view the primary operator otherwise lacks. Primary
	// token only (a tenant sees only its own runs via `runs stats`). Returns:
	// { tenants: [{id, runs, completed, failed, active, spent_microcents,
	// last_activity_unix_ms}], count, total_runs, total_spent_microcents }
	CmdTenantStats = "tenant_stats"

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

	// CmdWorldLog lists recent world-model operations (M86) — a timeline of the
	// journal's worldmodel.entity.upserted / relation.upserted / forgotten
	// events. Args: limit (optional), kind (optional: entity|relation), since_ms
	// (optional window). Returns: { ops: [ {ts_unix_ms, op, what, label} ], count }
	CmdWorldLog = "world_log"

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

	// CmdSkillImport installs a skill from a portable bundle as a fresh DRAFT
	// (via the Forge, so it is content-addressed, deduped, and journaled like
	// any other authored skill). Args: name, body (required); description,
	// triggers, tools_required (optional). Returns: { id, name, status, created }
	CmdSkillImport = "skill_import"

	// Chronos standing orders (SPEC-16 §4) — persistent-goal CRUD behind
	// `agt standing`. Add: args.order (object). SetEnabled: args{id, enabled}.
	// Remove: args.id. List: no args. Every mutation is journaled (standing.*).
	CmdStandingList       = "standing_list"
	CmdStandingAdd        = "standing_add"
	CmdStandingSetEnabled = "standing_set_enabled"
	CmdStandingRemove     = "standing_remove"
	CmdStandingWhy        = "standing_why" // fold the journal for one order's life story

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
	// Args: limit (optional; default 20, clamped 1..1000); channel (optional;
	// case-insensitive channel-kind filter, e.g. "telegram"|"slack"|"discord").
	// Returns: { threads: [{correlation_id, channel_kind, channel_id,
	//            messages:[{direction,sender,text,ts_unix_ms,event_id}],
	//            last_ts_unix_ms}, ...], count, channel? }
	CmdInbox = "inbox"

	// CmdSend delivers an operator-initiated outbound message through a configured
	// channel (Telegram/Slack/Discord) — the manual egress complement to Pulse
	// briefs and agent replies, for scripts/CI ("deploy done → notify Slack").
	// Authenticated by the control plane (primary token), so no per-channel
	// allowlist gate. The channel's own Send journals channel.outbound.
	// Args: channel (kind, required), to (channel/chat id, required), text (required).
	// Returns: { sent: true, channel, to }
	CmdSend = "send"
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
