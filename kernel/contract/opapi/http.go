// SPDX-License-Identifier: MIT

package opapi

import "sort"

// Provenance: the console's route table, moved here from the Web UI's four
// hand-written route tables (webui_read_routes.go, webui_write_routes.go) when
// the console's HTTP surface got one home (W3.82).
//
// The daemon's operation names live in this package (names*.go); the console's
// binding of those names — the path they answer at, the method, and the
// argument allowlist the console forwards — lives here too. Before this table
// the console carried four hand-written maps of the same information, and the
// application layer repeated a third of it as a "HTTP hint" on each
// operation's spec that nothing ever read.
//
// A route is a promise: this path answers this operation with these arguments.
// A promise written in two places is a promise that drifts, so it is written
// once, here, and the console derives its routes from it.

// HTTPRouteKind classifies how the console invokes an operation. The kind
// decides which proxy the console binds and where the arguments come from.
type HTTPRouteKind uint8

const (
	// HTTPRouteRead is a parameterless read: one GET, one operation, no
	// arguments forwarded.
	HTTPRouteRead HTTPRouteKind = iota
	// HTTPRouteReadArgs is a read whose arguments come from the query string.
	HTTPRouteReadArgs
	// HTTPRouteWrite is a mutation whose arguments come from the query string.
	HTTPRouteWrite
	// HTTPRouteJSON is a mutation whose arguments come from the JSON body.
	HTTPRouteJSON
	// HTTPRouteStream is a mutation whose response is a stream of frames; the
	// console binds a bespoke handler for it, but the path and the forwarded
	// keys are declared here.
	HTTPRouteStream
)

// HTTPRoute is one operation's console binding: the path the console answers
// at, the method it answers with, and the argument allowlist it forwards.
// Args is the allowlist the browser may set — a fixed list, so the browser can
// only ever invoke the operation with these arguments.
type HTTPRoute struct {
	Op     string
	Method string
	Path   string
	Kind   HTTPRouteKind
	Args   []string
}

// consoleRoutes is the console's route table, one entry per operation the
// console binds. The index below keys it by operation name.
var consoleRoutes = []HTTPRoute{
	// Parameterless read proxies: one GET, one operation, no arguments.
	{Op: Status, Method: "GET", Path: "/api/status", Kind: HTTPRouteRead},
	{Op: Config, Method: "GET", Path: "/api/config", Kind: HTTPRouteRead},
	{Op: Budget, Method: "GET", Path: "/api/budget", Kind: HTTPRouteRead},
	// Mission Control "Spend today" tile (Day 28+1). Slim shape {total:int} the
	// useSpendToday hook consumes — pairs with /api/budget which carries the
	// full breakdown.
	{Op: SpendToday, Method: "GET", Path: "/api/spend/today", Kind: HTTPRouteRead},
	{Op: CatalogList, Method: "GET", Path: "/api/catalog", Kind: HTTPRouteRead},
	{Op: ExecutionProfiles, Method: "GET", Path: "/api/execution_profiles", Kind: HTTPRouteRead},
	{Op: ExecutionProfileCheck, Method: "GET", Path: "/api/execution_profile_check", Kind: HTTPRouteRead},
	{Op: ToolList, Method: "GET", Path: "/api/tools_catalog", Kind: HTTPRouteRead},
	{Op: EdictStats, Method: "GET", Path: "/api/policy", Kind: HTTPRouteRead},
	{Op: EdictShow, Method: "GET", Path: "/api/edict_show", Kind: HTTPRouteRead},
	{Op: ScheduleList, Method: "GET", Path: "/api/schedules", Kind: HTTPRouteRead},
	{Op: ScheduleSystemTasks, Method: "GET", Path: "/api/schedule/system_tasks", Kind: HTTPRouteRead},
	{Op: MemoryAudit, Method: "GET", Path: "/api/memory/audit", Kind: HTTPRouteRead},
	{Op: WorldList, Method: "GET", Path: "/api/world", Kind: HTTPRouteRead},
	{Op: SkillList, Method: "GET", Path: "/api/skills", Kind: HTTPRouteRead},
	{Op: StandingList, Method: "GET", Path: "/api/standing", Kind: HTTPRouteRead},
	{Op: MCPList, Method: "GET", Path: "/api/mcp", Kind: HTTPRouteRead},
	// CLI Toolbox (M956): the install action streams, so it has its own proxy
	// (toolInstallProxy) below. The detect/updates pair is unused by the Web UI
	// (verified in TOPOLOGY-AUDIT-DAY28-VERIFY.md §2) and reachable via the CLI
	// through opapi.ToolboxDetect / opapi.ToolboxOutdated when an operator needs them.
	{Op: ACPAgents, Method: "GET", Path: "/api/acp/agents", Kind: HTTPRouteRead},
	{Op: WorkflowList, Method: "GET", Path: "/api/workflows", Kind: HTTPRouteReadArgs, Args: []string{"with_runs"}},
	// Built-in workflow template gallery (M807). Read-only.
	{Op: WorkflowTemplates, Method: "GET", Path: "/api/workflows/templates", Kind: HTTPRouteRead},
	// Open (unanswered) help requests agents have raised on the board (M849). Read-only.
	{Op: BoardHelp, Method: "GET", Path: "/api/board/help", Kind: HTTPRouteRead},
	// Personal Data Lake (M836): list collections (no args). Read-only.
	{Op: DataCollections, Method: "GET", Path: "/api/data/collections", Kind: HTTPRouteRead},
	// Council of Elders (M839): the default membership the panel convenes with. Read-only.
	{Op: CouncilMembers, Method: "GET", Path: "/api/council/members", Kind: HTTPRouteRead},
	{Op: AutonomyFeed, Method: "GET", Path: "/api/autonomy", Kind: HTTPRouteReadArgs, Args: []string{"limit"}},
	{Op: Approvals, Method: "GET", Path: "/api/approvals", Kind: HTTPRouteRead},
	{Op: SandboxList, Method: "GET", Path: "/api/sandbox", Kind: HTTPRouteRead},
	{Op: ConfigSchema, Method: "GET", Path: "/api/config/schema", Kind: HTTPRouteRead},
	{Op: ConfigValues, Method: "GET", Path: "/api/config/values", Kind: HTTPRouteRead},
	{Op: ChannelList, Method: "GET", Path: "/api/channels", Kind: HTTPRouteRead},
	{Op: NodeRegistry, Method: "GET", Path: "/api/nodes", Kind: HTTPRouteRead},
	// Build/version provenance (M971): semver + git revision, so the UI can show
	// exactly which build the daemon is running.
	{Op: Version, Method: "GET", Path: "/api/version", Kind: HTTPRouteRead},
	// Per-task model routing (M703): the effective chains + known task types.
	{Op: RoutingGet, Method: "GET", Path: "/api/routing", Kind: HTTPRouteRead},
	// Named reusable fallback chains (M963): the registry + default chain.
	{Op: ChainsGet, Method: "GET", Path: "/api/chains", Kind: HTTPRouteRead},
	{Op: PersonaGet, Method: "GET", Path: "/api/persona", Kind: HTTPRouteRead},
	{Op: PromptsGet, Method: "GET", Path: "/api/prompts", Kind: HTTPRouteRead},
	// Pulse — the proactive heartbeat status (running/paused/beats/cadence) (M743).
	{Op: PulseStatus, Method: "GET", Path: "/api/pulse", Kind: HTTPRouteRead},
	// Read proxies that forward an argument allowlist from the query string.
	{Op: JournalGrep, Method: "GET", Path: "/api/journal", Kind: HTTPRouteReadArgs, Args: []string{"correlation_id", "kind", "limit"}},
	// Cursor-paginated runs (M-pending): per the audit follow-up the SPA
	// needs to page through long histories without paying for the whole
	// journal. `cursor` is the opaque "<ms>:<seq>" boundary of the previous
	// page, returned as `next_cursor` in the same shape.
	{Op: RunsList, Method: "GET", Path: "/api/runs", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "status", "intent", "model", "min_cost_mc", "max_cost_mc"}},
	// Cursor-paginated agents roster (M-pending): the SPA's Agents /
	// AgentPage / Roster / Board / Dashboard / Schedules / Standing /
	// IncidentPage / Overseer / Voice views all poll this. `cursor` is the
	// opaque "<CreatedMS>:<slug>" boundary of the previous page, returned
	// as `next_cursor` in the same shape. Slugs are unique and never
	// contain ':', so strings.Cut on ':' round-trips losslessly.
	{Op: AgentList, Method: "GET", Path: "/api/agents", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor"}},
	// Cursor-paginated conversation-shaped lists (M-pending follow-up):
	// /api/inbox (channel threads, newest activity first), /api/board
	// (board posts, newest ts first; topic filter preserved), /api/memory
	// (active records, newest CreatedMS first). All three use a "<ts_or_ms>:<id>"
	// cursor shape with the ID field as the tie-break when ts collides.
	{Op: Inbox, Method: "GET", Path: "/api/inbox", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "channel"}},
	{Op: BoardRead, Method: "GET", Path: "/api/board", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "topic"}},
	{Op: MemoryList, Method: "GET", Path: "/api/memory", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor"}},
	// Export an integrity-attested journal bundle for archival/compliance (M772):
	// Historical journal search (M618) and journal/export both went un-wired
	// (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2); reachable via opapi.JournalGrep /
	// opapi.JournalExport when an operator needs them.
	{Op: ProviderLog, Method: "GET", Path: "/api/provider_log", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "fallbacks"}},
	{Op: ToolLog, Method: "GET", Path: "/api/tool_log", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "tool", "errors"}},
	// Read one sandbox project file's content (M686), path-confined server-side.
	{Op: SandboxFile, Method: "GET", Path: "/api/sandbox_file", Kind: HTTPRouteReadArgs, Args: []string{"project", "file"}},
	// Artifact index listing (M822): browsable metadata for stored artifacts
	// (inbound images, tool outputs), optionally filtered. No bytes — the raw
	// route below serves those.
	{Op: ArtifactList, Method: "GET", Path: "/api/artifacts", Kind: HTTPRouteReadArgs, Args: []string{"kind", "source", "corr"}},
	// Personal Data Lake records query (M836): one collection, filtered/sorted/paged.
	{Op: DataRecords, Method: "GET", Path: "/api/data/records", Kind: HTTPRouteReadArgs, Args: []string{"collection", "search", "sort", "desc", "limit", "offset"}},
	// Agent graveyard impact (M846): what standing orders fire this agent. Read-only.
	{Op: AgentImpact, Method: "GET", Path: "/api/agents/impact", Kind: HTTPRouteReadArgs, Args: []string{"ref"}},
	// Agent effective permissions: roster tool allow/deny + Edict/trust ceiling. Read-only.
	{Op: AgentPermissions, Method: "GET", Path: "/api/agents/permissions", Kind: HTTPRouteReadArgs, Args: []string{"ref"}},
	// Per-agent activity timeline (M854): what the agent did, from the journal.
	// Cursor pagination (M-pending follow-up): the SPA's IncidentPage /
	// AgentPage views load this on every poll, and the journal can hold tens
	// of thousands of events. `cursor` is the opaque "<seq>" boundary of the
	// previous page; server skips entries with seq >= cursorSeq. Journal seq
	// is monotonic per kernel, so this is unique without a tie-break.
	{Op: AgentActivity, Method: "GET", Path: "/api/agents/activity", Kind: HTTPRouteReadArgs, Args: []string{"ref", "limit", "cursor"}},
	// Autonomous self-repair history/state: queued/completed/failed auto-repair
	// attempts for one agent, with inflight/cooldown detail. Read-only.
	// Same `<seq>` cursor as /api/agents/activity.
	{Op: AgentRepairStatus, Method: "GET", Path: "/api/agents/repair_status", Kind: HTTPRouteReadArgs, Args: []string{"ref", "limit", "cursor"}},
	// Owner/parent escalation queue for one agent: doctor-triggered help requests
	// it is currently responsible for, enriched with wake/provenance metadata.
	// Cursor pagination: `<ts_unix_ms>:<message_id>` — ts can collide across
	// board messages so the message_id is the tie-break. server skips entries
	// strictly newer-or-equal to the cursor.
	{Op: AgentEscalations, Method: "GET", Path: "/api/agents/escalations", Kind: HTTPRouteReadArgs, Args: []string{"ref", "limit", "cursor"}},
	// Rated agent Config Center (distinct from daemon /api/config settings):
	// key/value entries agents can read under rating + allow/deny policy.
	{Op: ConfigCenterList, Method: "GET", Path: "/api/configcenter/list", Kind: HTTPRouteReadArgs, Args: []string{"rating"}},
	// Reaper scan (M903): dead-agent + stale-artifact candidates. Read-only detection. (#53)
	{Op: ReaperScan, Method: "GET", Path: "/api/reaper/scan", Kind: HTTPRouteReadArgs, Args: []string{"idle_days", "stale_days"}},
	// Skill bundle resources (M847): list a skill's reference files + scripts.
	// Read-only; the daemon path-confines reads. The single-file GET was never
	// wired (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2); reachable via opapi.SkillReadFile
	// when an operator needs it.
	{Op: SkillFiles, Method: "GET", Path: "/api/skill/files", Kind: HTTPRouteReadArgs, Args: []string{"id"}},
	// Skill hygiene (M858): active skills that look idle (never/long-unused). Read-only.
	{Op: SkillHygiene, Method: "GET", Path: "/api/skills/hygiene", Kind: HTTPRouteReadArgs, Args: []string{"idle_days"}},
	// Marketplace (capability packs): browse the catalogue + one pack's contents.
	// Read-only.
	{Op: MarketList, Method: "GET", Path: "/api/market", Kind: HTTPRouteReadArgs, Args: []string{"query"}},
	{Op: MarketShow, Method: "GET", Path: "/api/market/show", Kind: HTTPRouteReadArgs, Args: []string{"name", "marketplace"}},
	{Op: MarketSources, Method: "GET", Path: "/api/market/sources", Kind: HTTPRouteReadArgs},
	{Op: EdictLog, Method: "GET", Path: "/api/policy_log", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "denied"}},
	// Chat suggested-prompts (memory-derived + tool-context). Read-only: blends
	// the agent's active memory into starter/next-step chips for the chat surface.
	// tools is a comma-joined list of recently-used tool names.
	{Op: ChatSuggestions, Method: "GET", Path: "/api/suggestions", Kind: HTTPRouteReadArgs, Args: []string{"session_id", "tools"}},
	// Resolved HITL approval history (M773): a timeline of past approval requests
	// joined with their granted/denied/timeout outcome. Read-only.
	{Op: ApprovalsLog, Method: "GET", Path: "/api/approvals_log", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "denied"}},
	// Mission Control "Needs your attention" panel (Day 28+1). Cross-cuts pending
	// approvals + recent pulse asks into one time-sorted feed. Optional window
	// ("5m"/"1h"/"24h") filters pulse asks; approvals ignore it. limit caps the
	// result (default 8, hard cap 50 enforced server-side). Read-only.
	{Op: Attention, Method: "GET", Path: "/api/attention", Kind: HTTPRouteReadArgs, Args: []string{"window", "limit"}},
	{Op: PlanHistory, Method: "GET", Path: "/api/plan_history", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "status"}},
	// Provider keyring list (M700): labels + active + last-4 for one provider/env.
	// Read-only — values never leave the daemon.
	{Op: ProviderKeyList, Method: "GET", Path: "/api/provider/keys", Kind: HTTPRouteReadArgs, Args: []string{"provider", "env"}},
	// Forecast a schedule's next fire times (M744): id + how many. Read-only preview.
	{Op: ScheduleTest, Method: "GET", Path: "/api/schedule/test", Kind: HTTPRouteReadArgs, Args: []string{"id", "count"}},
	// Schedule firing history (M976): cronjob executions as structured actions,
	// not prompt text. Read-only and filterable for the dashboard.
	{Op: ScheduleFires, Method: "GET", Path: "/api/schedule/fires", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "id", "status", "since_ms", "intent"}},
	// A2 Phase 2: register the six log endpoints that previously streamed full
	// slices via apiRoutes (no-args proxy). They now expose cursor pagination.
	{Op: RateLimitLog, Method: "GET", Path: "/api/ratelimit_log", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "since_ms"}},
	{Op: WebhookLog, Method: "GET", Path: "/api/webhook_log", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "since_ms"}},
	{Op: WardenLog, Method: "GET", Path: "/api/warden_log", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "since_ms"}},
	{Op: NetguardLog, Method: "GET", Path: "/api/netguard_log", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "since_ms"}},
	{Op: WorldLog, Method: "GET", Path: "/api/world_log", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "since_ms"}},
	{Op: MemoryLog, Method: "GET", Path: "/api/memory_log", Kind: HTTPRouteReadArgs, Args: []string{"limit", "cursor", "since_ms"}},
	// A standing order's life story (M746): every standing.* journal event for it —
	// created, paused/resumed, each firing, removed. Read-only provenance.
	{Op: StandingWhy, Method: "GET", Path: "/api/standing/why", Kind: HTTPRouteReadArgs, Args: []string{"id"}},
	// One workflow's full graph (M798) — the list stays light; the canvas
	// editor fetches nodes+edges here. Read-only.
	{Op: WorkflowShow, Method: "GET", Path: "/api/workflows/show", Kind: HTTPRouteReadArgs, Args: []string{"ref"}},
	// Run history (M806): the journal folded into per-run arcs so the canvas
	// can replay any past run. Read-only.
	{Op: WorkflowRuns, Method: "GET", Path: "/api/workflows/runs", Kind: HTTPRouteReadArgs, Args: []string{"ref", "limit"}},
	// Dry-run a policy decision (M753): "if the agent asked to do <capability> with
	// <input>, would the edict engine allow / ask / deny it, and via which rule?".
	// Read-only — eng.Decide mutates nothing.
	{Op: EdictTest, Method: "GET", Path: "/api/edict/test", Kind: HTTPRouteReadArgs, Args: []string{"capability", "input"}},
	// Mutating proxies that forward an argument allowlist from the query string.
	{Op: Halt, Method: "POST", Path: "/api/halt", Kind: HTTPRouteWrite, Args: []string{"reason"}},
	{Op: Resume, Method: "POST", Path: "/api/resume", Kind: HTTPRouteWrite, Args: []string{"reason"}},
	// Artifact collector (M845): reap stale artifacts. POST so the browser must opt
	// in; dry_run (default true) previews, dry_run=false deletes. Goes through the
	// jsonProxy? No — the args are simple scalars, so it's a query-arg write route.
	{Op: ArtifactCollect, Method: "POST", Path: "/api/artifact/collect", Kind: HTTPRouteWrite, Args: []string{"older_than_days", "dry_run"}},
	// Personal Data Lake mutations (M836): delete a record. (Insert/update/create
	// carry structured bodies — they are jsonRoutes.) The drop-collection write
	// was never wired (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2); reachable via
	// opapi.DataDropCollection when an operator needs to drop a collection.
	{Op: DataDelete, Method: "POST", Path: "/api/data/delete", Kind: HTTPRouteWrite, Args: []string{"collection", "id"}},
	// Pulse pause/resume (M743): the proactive-heartbeat master switch. No args.
	{Op: PulsePause, Method: "POST", Path: "/api/pulse/pause", Kind: HTTPRouteWrite},
	{Op: PulseResume, Method: "POST", Path: "/api/pulse/resume", Kind: HTTPRouteWrite},
	// Trigger one on-demand heartbeat (M756): the operator's "think now". No args.
	{Op: PulseBeat, Method: "POST", Path: "/api/pulse/beat", Kind: HTTPRouteWrite},
	// Change the heartbeat interval live (M757): seconds → clamped cadence. Runtime-only.
	{Op: PulseCadence, Method: "POST", Path: "/api/pulse/cadence", Kind: HTTPRouteWrite, Args: []string{"seconds"}},
	// Change the proactivity dial live (M758): quiet|balanced|chatty. Runtime-only.
	{Op: PulseDial, Method: "POST", Path: "/api/pulse/dial", Kind: HTTPRouteWrite, Args: []string{"dial"}},
	// Flush held digest items now (M761): deliver what the pulse is holding. No args.
	{Op: PulseFlush, Method: "POST", Path: "/api/pulse/flush", Kind: HTTPRouteWrite},
	// Add a disk-space watch at runtime (M767): alert when free space on path < min_pct.
	{Op: PulseWatch, Method: "POST", Path: "/api/pulse/watch", Kind: HTTPRouteWrite, Args: []string{"path", "min_pct"}},
	// Add a command-probe watch at runtime (M768): run command each beat, alert on flip.
	{Op: PulseProbe, Method: "POST", Path: "/api/pulse/probe", Kind: HTTPRouteWrite, Args: []string{"name", "command"}},
	// Remove a runtime-added watch by observer name (M769): the inverse of watch/probe.
	{Op: PulseUnwatch, Method: "POST", Path: "/api/pulse/unwatch", Kind: HTTPRouteWrite, Args: []string{"name"}},
	// Set the quiet-hours window live (M770): "hours" is "START-END" (e.g. "22-7"); empty disables.
	{Op: PulseQuiet, Method: "POST", Path: "/api/pulse/quiet", Kind: HTTPRouteWrite, Args: []string{"hours"}},
	// Reload catalog + providers in place (M745): apply credential/catalog changes
	// without a daemon restart. No args.
	{Op: ProviderReload, Method: "POST", Path: "/api/provider/reload", Kind: HTTPRouteJSON},
	// Send an outbound message via a configured channel (M747): channel + to + text.
	// The browser sends the three fields as a JSON body, so the route reads them
	// from the body (W5.1) — a query-string route would drop them silently.
	{Op: Send, Method: "POST", Path: "/api/send", Kind: HTTPRouteJSON, Args: []string{"channel", "to", "text"}},
	{Op: CancelRun, Method: "POST", Path: "/api/cancel_run", Kind: HTTPRouteWrite, Args: []string{"correlation"}},
	// /api/budget_set went un-wired (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2);
	// reachable via opapi.BudgetSet when an operator needs to adjust the ceiling.
	{Op: RunPause, Method: "POST", Path: "/api/run/pause", Kind: HTTPRouteWrite, Args: []string{"correlation"}},
	{Op: RunResume, Method: "POST", Path: "/api/run/resume", Kind: HTTPRouteWrite, Args: []string{"correlation"}},
	{Op: RunStep, Method: "POST", Path: "/api/run/step", Kind: HTTPRouteWrite, Args: []string{"correlation"}},
	{Op: RunSteer, Method: "POST", Path: "/api/run/steer", Kind: HTTPRouteWrite, Args: []string{"correlation", "directive", "mode"}},
	{Op: Decide, Method: "POST", Path: "/api/decide", Kind: HTTPRouteWrite, Args: []string{"id", "decision", "reason"}},
	{Op: MemoryForget, Method: "POST", Path: "/api/memory/forget", Kind: HTTPRouteWrite, Args: []string{"id"}},
	// Bulk soft-delete: the Memory view's multi-select "forget N records" path.
	// ids is a JSON array; idempotent (already-tombstoned ids count as forgotten).
	{Op: MemoryBulkForget, Method: "POST", Path: "/api/memory/bulk_forget", Kind: HTTPRouteJSON, Args: []string{"ids"}},
	// Marketplace install/uninstall stream per-item progress, so they have their
	// own SSE proxies below (marketStreamProxy) rather than going through jsonProxy.
	// Remote marketplace sources (Phase 2): add/remove a source, then sync its
	// catalogue into the local cache. Write path; journaled market.*.
	{Op: MarketAddSource, Method: "POST", Path: "/api/market/source/add", Kind: HTTPRouteJSON, Args: []string{"url", "name", "pubkey"}},
	{Op: MarketRemoveSource, Method: "POST", Path: "/api/market/source/remove", Kind: HTTPRouteJSON, Args: []string{"name"}},
	{Op: MarketSync, Method: "POST", Path: "/api/market/sync", Kind: HTTPRouteJSON, Args: []string{"name"}},
	// Promote a private (agent-scoped) record into shared memory (M915) —
	// the selective-sharing valve over per-agent memory.
	{Op: MemoryPromote, Method: "POST", Path: "/api/memory/promote", Kind: HTTPRouteWrite, Args: []string{"id"}},
	// Delete a stored artifact by index id (M822); the blob is GC'd when unreferenced.
	{Op: ArtifactDelete, Method: "POST", Path: "/api/artifact/delete", Kind: HTTPRouteWrite, Args: []string{"id"}},
	// One brain-distillation pass was the read/write pair previously exposed at
	// /api/memory/consolidate and /api/reflect/run (TOPOLOGY-AUDIT-DAY28-VERIFY.md
	// §2: no app caller). The controlplane commands remain registered for CLI use;
	// the Web UI proxies are removed.
	// Operator-profile rebuild (M1000): synthesize the operator profile from
	// accumulated memory. No args; mirrors the (removed) /api/memory/consolidate.
	{Op: ProfileRebuild, Method: "POST", Path: "/api/profile/rebuild", Kind: HTTPRouteWrite},
	// Memory prune (M857): hard-remove soft-deleted records older than N days.
	// dry_run reports the prunable count first; the UI confirms before pruning.
	{Op: MemoryPrune, Method: "POST", Path: "/api/memory/prune", Kind: HTTPRouteWrite, Args: []string{"older_than_days", "dry_run"}},
	// Memory retention hygiene: dry_run reports low-value active records;
	// dry_run=false soft-forgets them.
	{Op: MemoryClean, Method: "POST", Path: "/api/memory/clean", Kind: HTTPRouteWrite, Args: []string{"dry_run"}},
	// Memory tidy (M994): collapse near-duplicate auto-distilled notes by subject.
	// dry_run reports how many would collapse; the UI confirms before tidying.
	{Op: MemoryTidy, Method: "POST", Path: "/api/memory/tidy", Kind: HTTPRouteWrite, Args: []string{"dry_run"}},
	{Op: WorldForget, Method: "POST", Path: "/api/world/forget", Kind: HTTPRouteWrite, Args: []string{"id"}},
	{Op: WorldRelate, Method: "POST", Path: "/api/world/relate", Kind: HTTPRouteWrite, Args: []string{"from", "verb", "to"}},
	{Op: SandboxDelete, Method: "POST", Path: "/api/sandbox/delete", Kind: HTTPRouteWrite, Args: []string{"project"}},
	{Op: EdictSetLevel, Method: "POST", Path: "/api/edict/set_level", Kind: HTTPRouteWrite, Args: []string{"capability", "level"}},
	{Op: EdictSetMode, Method: "POST", Path: "/api/edict/set_mode", Kind: HTTPRouteWrite, Args: []string{"mode"}},
	{Op: EdictDenyAdd, Method: "POST", Path: "/api/edict/deny_add", Kind: HTTPRouteWrite, Args: []string{"rule"}},
	{Op: EdictDenyRemove, Method: "POST", Path: "/api/edict/deny_rm", Kind: HTTPRouteWrite, Args: []string{"name"}},
	{Op: SkillPromote, Method: "POST", Path: "/api/skill/promote", Kind: HTTPRouteWrite, Args: []string{"id"}},
	{Op: SkillQuarantine, Method: "POST", Path: "/api/skill/quarantine", Kind: HTTPRouteWrite, Args: []string{"id", "reason"}},
	{Op: SkillArchive, Method: "POST", Path: "/api/skill/archive", Kind: HTTPRouteWrite, Args: []string{"id", "reason"}},
	{Op: SkillRevert, Method: "POST", Path: "/api/skill/revert", Kind: HTTPRouteWrite, Args: []string{"id"}},
	{Op: SkillShare, Method: "POST", Path: "/api/skill/share", Kind: HTTPRouteWrite, Args: []string{"id"}},
	// /api/skill/reassign was never wired (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2);
	// reachable via opapi.SkillReassign when an operator needs to reassign a skill.
	{Op: ScheduleRemove, Method: "POST", Path: "/api/schedule/remove", Kind: HTTPRouteWrite, Args: []string{"id"}},
	{Op: ScheduleRun, Method: "POST", Path: "/api/schedule/run", Kind: HTTPRouteWrite, Args: []string{"id"}},
	{Op: ScheduleEnable, Method: "POST", Path: "/api/schedule/enable", Kind: HTTPRouteWrite, Args: []string{"id", "enabled"}},
	{Op: StandingSetEnabled, Method: "POST", Path: "/api/standing/enable", Kind: HTTPRouteWrite, Args: []string{"id", "enabled"}},
	{Op: StandingRemove, Method: "POST", Path: "/api/standing/remove", Kind: HTTPRouteWrite, Args: []string{"id"}},
	// Fire a standing order now (M765), ignoring its triggers — test or run on demand.
	{Op: StandingFire, Method: "POST", Path: "/api/standing/fire", Kind: HTTPRouteWrite, Args: []string{"id"}},
	// Agent roster lifecycle (M783): pause/resume/remove a named agent (ref = id or slug).
	{Op: AgentSetEnabled, Method: "POST", Path: "/api/agents/enable", Kind: HTTPRouteWrite, Args: []string{"ref", "enabled"}},
	// Agent graveyard (M846): retire to / revive from the graveyard. POST-only.
	{Op: AgentRetire, Method: "POST", Path: "/api/agents/retire", Kind: HTTPRouteWrite, Args: []string{"ref", "reason"}},
	{Op: AgentRevive, Method: "POST", Path: "/api/agents/revive", Kind: HTTPRouteWrite, Args: []string{"ref"}},
	// Agent wake: manual trigger for a roster agent. POST-only.
	{Op: AgentWake, Method: "POST", Path: "/api/agents/wake", Kind: HTTPRouteJSON, Args: []string{"ref", "intent", "reason", "incident_id", "root_incident_id", "parent_incident_id"}},
	// MCP self-install lifecycle (M796): attach spawns the registered server
	// NOW (its tools go live for the next run); detach is the kill switch;
	// enable flips auto-attach at daemon start. ref = name or id.
	{Op: MCPAttach, Method: "POST", Path: "/api/mcp/attach", Kind: HTTPRouteWrite, Args: []string{"ref"}},
	{Op: MCPDetach, Method: "POST", Path: "/api/mcp/detach", Kind: HTTPRouteWrite, Args: []string{"ref"}},
	{Op: MCPSetEnabled, Method: "POST", Path: "/api/mcp/enable", Kind: HTTPRouteWrite, Args: []string{"ref", "enabled"}},
	{Op: MCPRemove, Method: "POST", Path: "/api/mcp/remove", Kind: HTTPRouteWrite, Args: []string{"ref"}},
	// Workflow lifecycle (M798): enable arms triggers (M799); remove deletes.
	// (Save and run carry structured bodies — they are jsonRoutes.)
	{Op: WorkflowSetEnabled, Method: "POST", Path: "/api/workflows/enable", Kind: HTTPRouteWrite, Args: []string{"ref", "enabled"}},
	{Op: WorkflowRemove, Method: "POST", Path: "/api/workflows/remove", Kind: HTTPRouteWrite, Args: []string{"ref"}},
	// /api/reflect/run was never wired (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2);
	// reachable via opapi.ReflectRun when an operator needs it.
	// Provider keyring switch/remove (M700): activate or remove a key, reloading
	// the provider in place. (Add is a jsonRoute — the value is a secret body.)
	{Op: ProviderKeyActivate, Method: "POST", Path: "/api/provider/keys/activate", Kind: HTTPRouteWrite, Args: []string{"provider", "env", "label"}},
	{Op: ProviderKeyRemove, Method: "POST", Path: "/api/provider/keys/remove", Kind: HTTPRouteWrite, Args: []string{"provider", "env", "label"}},
	// Multi-account channel: remove a labelled account (deletes its stored fields).
	{Op: ChannelAccountRemove, Method: "POST", Path: "/api/channel/account/remove", Kind: HTTPRouteJSON, Args: []string{"kind", "label"}},
	// Mutating proxies that forward an argument allowlist from the JSON request body.
	{Op: PlanGenerate, Method: "POST", Path: "/api/plan/generate", Kind: HTTPRouteJSON, Args: []string{"intent", "model"}},
	{Op: PlanRefine, Method: "POST", Path: "/api/plan/refine", Kind: HTTPRouteJSON, Args: []string{"plan_json", "feedback", "model"}},
	// Config Center write (M693): set one setting (non-secret → config store,
	// secret → vault). POST-only; only name+value are forwarded.
	{Op: ConfigSet, Method: "POST", Path: "/api/config/set", Kind: HTTPRouteJSON, Args: []string{"name", "value"}},
	// Rated agent Config Center writes: agent-readable key/value entries with
	// rating and optional allow/deny lists. Separate from daemon settings above.
	{Op: ConfigCenterSet, Method: "POST", Path: "/api/configcenter/set", Kind: HTTPRouteJSON, Args: []string{"key", "value", "rating", "description", "allowed_agents", "excluded_agents"}},
	{Op: ConfigCenterSetAccess, Method: "POST", Path: "/api/configcenter/access", Kind: HTTPRouteJSON, Args: []string{"key", "allowed_agents", "excluded_agents"}},
	{Op: ConfigCenterDelete, Method: "POST", Path: "/api/configcenter/delete", Kind: HTTPRouteJSON, Args: []string{"key"}},
	{Op: ConfigCenterSetRating, Method: "POST", Path: "/api/configcenter/rating", Kind: HTTPRouteJSON, Args: []string{"key", "rating"}},
	// Schema registry write (M695): register/unregister a skill/plugin-contributed
	// schema section. Register forwards the whole `section` object; unregister an id.
	{Op: ConfigSchemaRegister, Method: "POST", Path: "/api/config/schema/register", Kind: HTTPRouteJSON, Args: []string{"section"}},
	{Op: ConfigSchemaUnregister, Method: "POST", Path: "/api/config/schema/unregister", Kind: HTTPRouteJSON, Args: []string{"id", "force"}},
	// Models catalog sync (M699): pull models.dev/api.json server-side, save +
	// hot-reload the catalog. POST (it mutates + hits the network) with the longer
	// jsonProxy timeout; `url` optionally overrides the source. No body needed —
	// the Sync button posts {}.
	{Op: CatalogSync, Method: "POST", Path: "/api/catalog/sync", Kind: HTTPRouteJSON, Args: []string{"url"}},
	// Provider keyring add (M700): the value is a secret, so it travels in the
	// POST body (not a query arg). provider+env+label+value(+active).
	{Op: ProviderKeyAdd, Method: "POST", Path: "/api/provider/keys/add", Kind: HTTPRouteJSON, Args: []string{"provider", "env", "label", "value", "active"}},
	// Multi-account channel: set one field of an account instance. The value may be
	// a secret, so it travels in the POST body. kind+label(""=default)+name+value.
	{Op: ChannelAccountSet, Method: "POST", Path: "/api/channel/account/set", Kind: HTTPRouteJSON, Args: []string{"kind", "label", "name", "value"}},
	// Channel OAuth connect (Phase 4): start a flow (client_secret is a secret →
	// POST body) returning {authorize_url,state}; poll its status. The browser
	// redirect lands on the public /oauth/callback handler (registered separately).
	{Op: ChannelOAuthStart, Method: "POST", Path: "/api/channel/oauth/start", Kind: HTTPRouteJSON, Args: []string{"kind", "label", "client_id", "client_secret", "redirect_uri", "instance_url"}},
	{Op: ChannelOAuthStatus, Method: "POST", Path: "/api/channel/oauth/status", Kind: HTTPRouteJSON, Args: []string{"state"}},
	// "Sign in with ChatGPT" provider login: start the flow (1455 redirect
	// listener) → poll status; import a local Codex CLI login; or disconnect.
	{Op: ProviderOAuthStart, Method: "POST", Path: "/api/provider/oauth/start", Kind: HTTPRouteJSON, Args: []string{"provider"}},
	{Op: ProviderOAuthStatus, Method: "POST", Path: "/api/provider/oauth/status", Kind: HTTPRouteJSON, Args: []string{"state"}},
	{Op: ProviderOAuthImport, Method: "POST", Path: "/api/provider/oauth/import", Kind: HTTPRouteJSON, Args: []string{"path"}},
	{Op: ProviderOAuthLogout, Method: "POST", Path: "/api/provider/oauth/logout", Kind: HTTPRouteJSON},
	// Register a provider + key. Catalog-aware: if `id` already exists in the
	// merged catalog (api+local+custom) the existing entry is preserved (no
	// custom.json write) — the orphan-with-one-model bug a naive upsert
	// caused is gone. For new ids, a minimal partial entry is written to
	// custom.json and the daemon is reloaded. JSON body (id, name, npm, api,
	// env, model); the key follows on keys/add.
	{Op: ProviderConnect, Method: "POST", Path: "/api/provider/connect", Kind: HTTPRouteJSON, Args: []string{"id", "name", "npm", "api", "env", "model"}},
	// Provider reachability probe (key in body → jsonRoute): is the endpoint up?
	{Op: ProviderProbe, Method: "POST", Path: "/api/provider/probe", Kind: HTTPRouteJSON, Args: []string{"url", "key"}},
	// WhatsApp gateway connection probe (key in body → jsonRoute): is the WAHA/
	// Evolution session logged in? Lets the Channels wizard show connected vs scan-QR.
	{Op: WhatsAppGatewayStatus, Method: "POST", Path: "/api/whatsappgw/status", Kind: HTTPRouteJSON, Args: []string{"url", "backend", "session", "key"}},
	{Op: WhatsAppGatewayQR, Method: "POST", Path: "/api/whatsappgw/qr", Kind: HTTPRouteJSON, Args: []string{"url", "backend", "session", "key"}},
	// Per-task model routing (M703): replace the model chains. `chains` is an
	// object {task: [models]} too large/structured for a query arg.
	{Op: RoutingSet, Method: "POST", Path: "/api/routing/set", Kind: HTTPRouteJSON, Args: []string{"chains"}},
	// Named reusable fallback chains (M963): replace the whole registry. `chains`
	// is an object {name: [models]} and `default` an optional chain name.
	{Op: ChainsSet, Method: "POST", Path: "/api/chains/set", Kind: HTTPRouteJSON, Args: []string{"chains", "default"}},
	{Op: PersonaSet, Method: "POST", Path: "/api/persona/set", Kind: HTTPRouteJSON, Args: []string{"system"}},
	// Chat history compaction (M923): fold older turns into one briefing. The
	// turns array is far too large for a query string — JSON body only.
	{Op: ChatSummarize, Method: "POST", Path: "/api/chat/summarize", Kind: HTTPRouteJSON, Args: []string{"turns", "model"}},
	// Personal Data Lake writes (M836): insert/update carry the record object —
	// JSON bodies, not query args. The create-collection write was never wired
	// (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2); reachable via opapi.DataCreateCollection.
	{Op: DataInsert, Method: "POST", Path: "/api/data/insert", Kind: HTTPRouteJSON, Args: []string{"collection", "record"}},
	{Op: DataUpdate, Method: "POST", Path: "/api/data/update", Kind: HTTPRouteJSON, Args: []string{"collection", "id", "record"}},
	// Council of Elders ask (M839): convene the panel on a question. Long-running
	// (several model calls) but bounded by the jsonProxy timeout. POST body.
	{Op: CouncilAsk, Method: "POST", Path: "/api/council/ask", Kind: HTTPRouteJSON, Args: []string{"question", "rounds", "corr"}},
	// Council members edit (M839): replace the default council membership. members
	// is an array of {seat, model}. Applies live and persists to config store.
	{Op: CouncilSet, Method: "POST", Path: "/api/council/set", Kind: HTTPRouteJSON, Args: []string{"members"}},
	// Conductor ask (M997): run the Thinker/Worker/Verifier loop on a task.
	// The conductor nav cluster was REMOVED in Day 28 (TOPOLOGY-AUDIT-DAY28-VERIFY.md
	// §2); reachable via opapi.ConductorAsk when an operator needs it.
	// Deep-research harness ask (M1001): decompose, gather web sources,
	// synthesize a cited answer, adversarially verify each claim. Long-running
	// (several searches/fetches + model calls) but bounded by the jsonProxy
	// timeout. POST body.
	{Op: ResearchAsk, Method: "POST", Path: "/api/research/ask", Kind: HTTPRouteJSON, Args: []string{"question", "max_sub_questions", "max_sources", "verify", "max_verify_claims", "corr"}},
	{Op: PromptsSet, Method: "POST", Path: "/api/prompts/set", Kind: HTTPRouteJSON, Args: []string{"prompts"}},
	{Op: StandingAdd, Method: "POST", Path: "/api/standing/add", Kind: HTTPRouteJSON, Args: []string{"order"}},
	// Edit a standing order in place (M729): id + any subset of the human-tunable
	// fields. assure is numeric, so the JSON body preserves its type.
	{Op: StandingEdit, Method: "POST", Path: "/api/standing/edit", Kind: HTTPRouteJSON, Args: []string{"id", "name", "plan", "agent", "mode", "max_trust", "briefing_min", "assure", "cooldown_sec"}},
	// Agent roster create/edit (M783): the profile is a structured object (soul
	// text, fallback list, numeric cost ceiling) — a JSON body, not query args.
	{Op: AgentAdd, Method: "POST", Path: "/api/agents/add", Kind: HTTPRouteJSON, Args: []string{"profile"}},
	{Op: AgentEdit, Method: "POST", Path: "/api/agents/edit", Kind: HTTPRouteJSON, Args: []string{"ref", "profile"}},
	{Op: AgentCapabilities, Method: "POST", Path: "/api/agents/capabilities", Kind: HTTPRouteJSON, Args: []string{"ref", "trust_ceiling", "tool_allow", "tool_deny", "noise_policy", "config_overrides", "memory_scope", "workdir", "max_cost_mc", "max_daily_mc"}},
	{Op: AgentRemove, Method: "POST", Path: "/api/agents/remove", Kind: HTTPRouteJSON, Args: []string{"ref", "cascade"}},
	{Op: AgentTaskUpdate, Method: "POST", Path: "/api/agents/task", Kind: HTTPRouteJSON, Args: []string{"ref", "op", "id", "task", "title", "description", "scope", "status"}},
	{Op: AgentRepair, Method: "POST", Path: "/api/agents/repair", Kind: HTTPRouteJSON, Args: []string{"ref", "reason", "incident_id", "root_incident_id", "parent_incident_id"}},
	{Op: AgentResolve, Method: "POST", Path: "/api/agents/resolve", Kind: HTTPRouteJSON, Args: []string{"ref", "resolution", "summary", "delegate_to", "task_type", "task_model_chain", "incident_id", "root_incident_id", "parent_incident_id"}},
	// Inter-agent mailbox writes (M937): message text and optional payload-shaped
	// fields ride in the body; this is the operator/app path into the same board
	// agents use with the board tool.
	{Op: BoardSend, Method: "POST", Path: "/api/board/send", Kind: HTTPRouteJSON, Args: []string{"from", "to", "topic", "reply_to", "text", "help"}},
	{Op: BoardAck, Method: "POST", Path: "/api/board/ack", Kind: HTTPRouteJSON, Args: []string{"id", "by"}},
	// OKR / Workboard / Taste / Seats / Toolforge cluster was REMOVED in Day 28
	// (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2); the underlying commands remain
	// registered for CLI use and are exercised by kernel/controlplane/
	// toolforge_test.go.
	// Register an MCP server (M796): the server is a structured object
	// (command + args list) — a JSON body, not query args.
	{Op: MCPAdd, Method: "POST", Path: "/api/mcp/add", Kind: HTTPRouteJSON, Args: []string{"server"}},
	// Workflow save/run (M798): the graph (nodes+edges) and the run payload
	// are structured objects — JSON bodies, not query args.
	{Op: WorkflowSave, Method: "POST", Path: "/api/workflows/save", Kind: HTTPRouteJSON, Args: []string{"workflow"}},
	{Op: WorkflowRun, Method: "POST", Path: "/api/workflows/run", Kind: HTTPRouteJSON, Args: []string{"ref", "payload", "async"}},
	// Copilot draft (M802): description in, validated UNSAVED graph out —
	// the canvas reviews and saves explicitly.
	{Op: WorkflowDraft, Method: "POST", Path: "/api/workflows/draft", Kind: HTTPRouteJSON, Args: []string{"description", "name"}},
	// Copilot refine (M805): the current canvas graph + a change request in,
	// the revised UNSAVED graph out.
	{Op: WorkflowRefine, Method: "POST", Path: "/api/workflows/refine", Kind: HTTPRouteJSON, Args: []string{"workflow", "instruction", "ref"}},
	// Single-node test (M811): the canvas graph + a node id + mock upstream
	// data in, the node's real output out.
	{Op: WorkflowTestNode, Method: "POST", Path: "/api/workflows/test_node", Kind: HTTPRouteJSON, Args: []string{"workflow", "node", "data", "payload"}},
	// Create a schedule (M715): intent + a timing mode. Numeric timing args (e.g.
	// interval_sec, at_minutes, once_at_unix) ride the JSON body so they keep their
	// types — a query arg would stringify them.
	{Op: ScheduleAdd, Method: "POST", Path: "/api/schedule/add", Kind: HTTPRouteJSON, Args: []string{"intent", "model", "agent", "target", "workflow", "system_task", "tool", "payload", "interval_sec", "at_minutes", "days", "tz", "once_at_unix", "window_start", "window_end"}},
	// Edit an existing schedule (M728): id + any subset of intent/model and at most
	// one cadence change. Numeric timing args ride the JSON body to keep their types.
	{Op: ScheduleEdit, Method: "POST", Path: "/api/schedule/edit", Kind: HTTPRouteJSON, Args: []string{"id", "intent", "model", "agent", "target", "workflow", "system_task", "tool", "payload", "interval_sec", "at_minutes", "days", "tz", "once_at_unix", "window_start", "window_end"}},
	// Teach the agent a fact (M718): content + optional subject/type/confidence.
	// confidence is numeric, so the JSON body preserves its type.
	{Op: MemoryAdd, Method: "POST", Path: "/api/memory/add", Kind: HTTPRouteJSON, Args: []string{"content", "subject", "type", "confidence", "evidence", "half_life_ms"}},
	// Revise a fact (M731): supersede old_id with a new record (content required;
	// confidence numeric, so the JSON body preserves its type).
	{Op: MemorySupersede, Method: "POST", Path: "/api/memory/supersede", Kind: HTTPRouteJSON, Args: []string{"old_id", "content", "subject", "type", "confidence", "evidence", "half_life_ms"}},
	// Add a world-model entity (M721): name + kind (+ optional aliases/attrs, which
	// are arrays/objects, so the JSON body is needed).
	{Op: WorldAdd, Method: "POST", Path: "/api/world/add", Kind: HTTPRouteJSON, Args: []string{"name", "kind", "aliases", "attrs"}},
	// Edit a world-model entity's aliases/attrs in place (M730): id + the full
	// editable state (arrays/objects, so the JSON body is needed).
	{Op: WorldEdit, Method: "POST", Path: "/api/world/edit", Kind: HTTPRouteJSON, Args: []string{"id", "aliases", "attrs"}},
	// Author a skill from the UI (M736): name + body (required) + optional
	// description/triggers/tools_required. triggers/tools_required are arrays, so the
	// JSON body is needed. Lands as a draft (auto-staged to shadow if well-formed) —
	// never auto-active; promote via the normal lifecycle controls.
	// agent (M933) optionally scopes the authored skill to one roster agent.
	{Op: SkillImport, Method: "POST", Path: "/api/skill/import", Kind: HTTPRouteJSON, Args: []string{"name", "description", "triggers", "body", "tools_required", "agent"}},
	// Dry-run the secret redactor (M754): does the LIVE scrubber redact this text,
	// and into which categories? Read-only, but carried in the JSON BODY (not a query
	// arg) so the sensitive probe text never lands in a URL / access log. The response
	// returns only the REDACTED form + category names, never the matched secret.
	{Op: RedactTest, Method: "POST", Path: "/api/redact/test", Kind: HTTPRouteJSON, Args: []string{"text"}},

	// Streamed operations: each has its own handler, because the frames are
	// the payload — but the path and the forwarded keys live here too.
	// The streaming "run this plan" action (Flow Studio's Run button). opapi.Plan
	// streams RespEvent frames before its terminal result, so it cannot go through
	// Call (which reads a single response); it is driven with Stream.
	{Op: Plan, Method: "POST", Path: "/api/plan/run", Kind: HTTPRouteStream, Args: []string{"plan_json"}},
	// The Chat view's send button: a free-text intent through the governed loop.
	// The streamed events ARE the chat payload, so each one is forwarded inline.
	{Op: Run, Method: "POST", Path: "/api/run", Kind: HTTPRouteStream, Args: []string{"intent", "model", "history", "system", "agent", "execution_profile", "auto_approve_caps"}},
	// CLI Toolbox install (M956): runs the host package manager for the requested
	// tools and streams the per-tool progress event then a final summary.
	{Op: ToolboxInstall, Method: "POST", Path: "/api/toolbox/install", Kind: HTTPRouteStream, Args: []string{"names"}},
	// Marketplace install streams per-item progress (skill/mcp/tool) as SSE.
	{Op: MarketInstall, Method: "POST", Path: "/api/market/install", Kind: HTTPRouteStream, Args: []string{"name", "marketplace", "version"}},
	// Marketplace uninstall streams per-item progress as SSE.
	{Op: MarketUninstall, Method: "POST", Path: "/api/market/uninstall", Kind: HTTPRouteStream, Args: []string{"name"}},
}

// consoleRouteIndex keys the route table by operation name.
var consoleRouteIndex = indexConsoleRoutes(consoleRoutes)

// indexConsoleRoutes keys the route table by operation name. A duplicate
// operation is a programmer error — two routes for one operation means one of
// them is unreachable — so it panics at process start rather than shadowing.
func indexConsoleRoutes(routes []HTTPRoute) map[string]HTTPRoute {
	index := make(map[string]HTTPRoute, len(routes))
	for _, route := range routes {
		if _, duplicate := index[route.Op]; duplicate {
			panic("opapi: duplicate console route for " + route.Op)
		}
		index[route.Op] = route
	}
	return index
}

// ConsoleRoute returns the console's binding for one operation. The second
// result is false when the console does not bind that operation.
func ConsoleRoute(op string) (HTTPRoute, bool) {
	route, ok := consoleRouteIndex[op]
	return route, ok
}

// ConsoleRoutes returns the whole route table, ordered by path.
func ConsoleRoutes() []HTTPRoute {
	routes := make([]HTTPRoute, len(consoleRoutes))
	copy(routes, consoleRoutes)
	sort.Slice(routes, func(i, j int) bool { return routes[i].Path < routes[j].Path })
	return routes
}

// ConsoleArgs returns the argument allowlist the console forwards for one
// operation. It is nil for an operation the console binds without arguments.
func ConsoleArgs(op string) []string {
	route, ok := consoleRouteIndex[op]
	if !ok {
		return nil
	}
	return route.Args
}
