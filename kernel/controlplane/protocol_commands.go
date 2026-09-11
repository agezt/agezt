// SPDX-License-Identifier: MIT

// Core control-plane commands: lifecycle (Version/Run/Halt/Resume/Why), approvals, provider/catalog/routing, pulse/budget, tools/warden, auth/plugin/journal/rate/webhook.
// Code extracted from protocol_commands.go during the Day-43 god-file split. Public API unchanged.
package controlplane


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
	// CmdProviderConnect is the catalog-aware "register a provider + key"
	// entry point. If the id already exists in the merged catalog, the
	// existing entry is preserved (no custom.json write) — the caller
	// attaches the key separately via CmdProviderKeyAdd. If the id is new,
	// a minimal partial entry (no synthetic model list) is written to
	// custom.json and the daemon is reloaded. args: id, name, npm, api,
	// env, model. The API key itself never sits on this handler.
	CmdProviderConnect = "provider_connect"
	// CmdProviderProbe checks whether a provider endpoint is reachable (args:
	// url, key) by GETting its OpenAI-compatible /models list. The connectivity
	// status behind a Connect button (esp. keyless local runtimes).
	CmdProviderProbe = "provider_probe"
	// CmdWhatsAppGatewayStatus probes a self-hosted WhatsApp gateway (WAHA/
	// Evolution) for whether its session is logged in (args: url, backend,
	// session, key). Lets the Channels wizard show connected vs scan-QR.
	CmdWhatsAppGatewayStatus = "whatsappgw_status"
	// CmdWhatsAppGatewayQR fetches the gateway's login QR (args: url, backend,
	// session, key) as a data: URL, so the wizard can render it inline to scan.
	CmdWhatsAppGatewayQR = "whatsappgw_qr"
	// Provider keyring (M700): store many API keys per provider/env and pick the
	// active one. args.provider is optional for legacy env-global keyrings. List
	// never returns values (label + active + last-4 only). Mutations reload the
	// provider in place.
	CmdProviderKeyList     = "provider_key_list"     // args: provider?, env
	CmdProviderKeyAdd      = "provider_key_add"      // args: provider?, env, label, value, active?
	CmdProviderKeyActivate = "provider_key_activate" // args: provider?, env, label
	CmdProviderKeyRemove   = "provider_key_remove"   // args: provider?, env, label
	// Per-task model routing (M703): view/edit the governor's per-task-type model
	// fallback chains. Set: args{chains} → live + persisted.
	CmdRoutingGet = "routing_get"
	CmdRoutingSet = "routing_set"
	// Named reusable fallback chains (M963): view/edit the governor's registry of
	// named model ladders + the default chain. Any "@<name>" model slot anywhere
	// (agent, task, chat) expands to the chain's models. Set: args{chains, default}
	// → live + persisted.
	CmdChainsGet = "chains_get"
	CmdChainsSet = "chains_set"
	// Default identity (M710): view/edit daemon fallback instructions for runs not
	// bound to a roster agent. Get returns the content (owner-facing);
	// Set: args{system} → live + persisted.
	CmdPersonaGet = "persona_get"
	CmdPersonaSet = "persona_set"
	// Prompt library (M713): the owner's saved chat prompts (reusable workflows).
	// Daemon-persisted; purely a Chat-composer convenience.
	CmdPromptsGet = "prompts_get"
	CmdPromptsSet = "prompts_set"
	// CmdChatSummarize condenses older chat turns into one compact briefing
	// (M925) so a long thread can keep its full context instead of silently
	// dropping the oldest turns at the history window. One bounded provider
	// call (TaskType "summarize" — per-task routing applies); no tools, no loop.
	// Args: turns ([{role,text}]), optional model. Returns: { summary, turns }.
	CmdChatSummarize = "chat_summarize"
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

	// CmdBudgetSet adjusts the governor's global daily spend ceiling at
	// runtime (M607) — the operator "ayarla" knob behind the Web UI cockpit's
	// budget control. Arg: ceiling_mc (int microcents; 0 = unlimited, negative
	// clamped to 0). Returns the post-set snapshot (same shape as CmdBudget) so
	// the caller can render the new state without a follow-up read. Mutating,
	// so it is POST-only on the Web UI write-route allowlist and audited via a
	// budget.ceiling_set event.
	CmdBudgetSet = "budget_set"

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
)
