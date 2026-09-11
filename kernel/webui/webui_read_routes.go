// SPDX-License-Identifier: MIT

// Read-only HTTP routes: apiRoutes (parameterless GETs) + readArgsRoutes (GETs with query args).
// Code extracted from webui.go during the Day-51 god-file split. Public API unchanged.
package webui


import (
	"github.com/agezt/agezt/kernel/controlplane"
)


var apiRoutes = map[string]string{
	"/api/status":                  controlplane.CmdStatus,
	"/api/config":                  controlplane.CmdConfig,
	"/api/stats":                   controlplane.CmdRunsStats,
	"/api/budget":                  controlplane.CmdBudget,
	"/api/cache":                   controlplane.CmdCacheStats,
	"/api/providers":               controlplane.CmdProviderStats,
	"/api/catalog":                 controlplane.CmdCatalogList,
	"/api/tools":                   controlplane.CmdToolStats,
	"/api/execution_profiles":      controlplane.CmdExecutionProfiles,
	"/api/execution_profile_check": controlplane.CmdExecutionProfileCheck,
	"/api/tools_catalog":           controlplane.CmdToolList,
	"/api/policy":                  controlplane.CmdEdictStats,
	"/api/edict_show":              controlplane.CmdEdictShow,
	"/api/schedules":               controlplane.CmdScheduleList,
	"/api/schedule/system_tasks":   controlplane.CmdScheduleSystemTasks,
	"/api/memory/audit":            controlplane.CmdMemoryAudit,
	"/api/world":                   controlplane.CmdWorldList,
	"/api/skills":                  controlplane.CmdSkillList,
	"/api/standing":                controlplane.CmdStandingList,
	"/api/toolforge":               controlplane.CmdToolforgeList,
	"/api/mcp":                     controlplane.CmdMCPList,
	// CLI Toolbox (M956): host tool inventory + upgradable set. Read-only host
	// probes (LookPath + bounded --version; package-manager upgrade-list). The
	// install action streams, so it has its own proxy (toolInstallProxy) below.
	"/api/toolbox":         controlplane.CmdToolboxDetect,
	"/api/toolbox/updates": controlplane.CmdToolboxOutdated,
	"/api/acp/agents":      controlplane.CmdACPAgents,
	"/api/workflows":       controlplane.CmdWorkflowList,
	// Built-in workflow template gallery (M807). Read-only.
	"/api/workflows/templates": controlplane.CmdWorkflowTemplates,
	// Open (unanswered) help requests agents have raised on the board (M849). Read-only.
	"/api/board/help": controlplane.CmdBoardHelp,
	"/api/workboard":  controlplane.CmdWorkboardList,
	// OKR spine (Phase 2): list objectives with live rollup (no args). Read-only.
	"/api/okr": controlplane.CmdOKRList,
	// Taste overlay (Phase 3): list curated exemplars (no args). Read-only.
	"/api/taste": controlplane.CmdTasteList,
	// Execution seats (Phase 4): the built-in seat catalog (no args). Read-only.
	"/api/seats": controlplane.CmdSeatList,
	// Personal Data Lake (M836): list collections (no args). Read-only.
	"/api/data/collections": controlplane.CmdDataCollections,
	// Council of Elders (M839): the default membership the panel convenes with. Read-only.
	"/api/council/members": controlplane.CmdCouncilMembers,
	// Conductor (M997): the default role→model assignment the panel will use. Read-only.
	"/api/conductor/roles": controlplane.CmdConductorRoles,
	"/api/autonomy":        controlplane.CmdAutonomyFeed,
	"/api/reflect":         controlplane.CmdReflectShow,
	"/api/approvals":       controlplane.CmdApprovals,
	"/api/plan_stats":      controlplane.CmdPlanStats,
	"/api/sandbox":         controlplane.CmdSandboxList,
	"/api/config/schema":   controlplane.CmdConfigSchema,
	"/api/config/values":   controlplane.CmdConfigValues,
	"/api/channels":        controlplane.CmdChannelList,
	"/api/nodes":           controlplane.CmdNodeRegistry,
	// Build/version provenance (M971): semver + git revision, so the UI can show
	// exactly which build the daemon is running.
	"/api/version": controlplane.CmdVersion,
	// Per-task model routing (M703): the effective chains + known task types.
	"/api/routing": controlplane.CmdRoutingGet,
	// Named reusable fallback chains (M963): the registry + default chain.
	"/api/chains":  controlplane.CmdChainsGet,
	"/api/persona": controlplane.CmdPersonaGet,
	"/api/prompts": controlplane.CmdPromptsGet,
	// Pulse — the proactive heartbeat status (running/paused/beats/cadence) (M743).
	"/api/pulse": controlplane.CmdPulseStatus,
	// Pulse asks — actionable observations awaiting an operator verdict under
	// initiative=ask (M1001). Read-only; the resolve action is a write route below.
	"/api/pulse/asks": controlplane.CmdPulseAsks,
	// Journal integrity (M759): verify the tamper-evident hash chain. Returns
	// { ok: true } when intact, or errors describing the break. Read-only.
	"/api/journal/verify": controlplane.CmdJournalVerify,
	// Per-subsystem home-dir disk breakdown (M927): what under ~/.agezt is
	// taking the space, plus the filesystem free/total. Read-only — the
	// collectors (artifact collect, memory prune) reclaim via their own routes.
	"/api/storage": controlplane.CmdStorageStats,
}

// writeRoute is a mutating control-plane command exposed over POST. args lists
// the query-param names copied into the call — a fixed allowlist, so the
// browser can only ever invoke these specific commands with these arguments.
type writeRoute struct {
	cmd  string
	args []string
}

// readArgsRoutes are READ-only commands that take query arguments (unlike
// apiRoutes, which proxy a parameterless read). They are served over GET — they
// never mutate — and only the allowlisted args are forwarded. Used by the run
// detail view, which fetches one run's events by correlation_id.
var readArgsRoutes = map[string]writeRoute{
	"/api/journal": {controlplane.CmdJournalGrep, []string{"correlation_id", "kind", "limit"}},
	// Cursor-paginated runs (M-pending): per the audit follow-up the SPA
	// needs to page through long histories without paying for the whole
	// journal. `cursor` is the opaque "<ms>:<seq>" boundary of the previous
	// page, returned as `next_cursor` in the same shape.
	"/api/runs": {controlplane.CmdRunsList, []string{
		"limit", "cursor", "status", "intent", "model", "min_cost_mc", "max_cost_mc",
	}},
	// Cursor-paginated agents roster (M-pending): the SPA's Agents /
	// AgentPage / Roster / Board / Dashboard / Schedules / Standing /
	// IncidentPage / Overseer / Voice views all poll this. `cursor` is the
	// opaque "<CreatedMS>:<slug>" boundary of the previous page, returned
	// as `next_cursor` in the same shape. Slugs are unique and never
	// contain ':', so strings.Cut on ':' round-trips losslessly.
	"/api/agents": {controlplane.CmdAgentList, []string{"limit", "cursor"}},
	// Cursor-paginated conversation-shaped lists (M-pending follow-up):
	// /api/inbox (channel threads, newest activity first), /api/board
	// (board posts, newest ts first; topic filter preserved), /api/memory
	// (active records, newest CreatedMS first). All three use a "<ts_or_ms>:<id>"
	// cursor shape with the ID field as the tie-break when ts collides.
	"/api/inbox":  {controlplane.CmdInbox, []string{"limit", "cursor", "channel"}},
	"/api/board":  {controlplane.CmdBoardRead, []string{"limit", "cursor", "topic"}},
	"/api/memory": {controlplane.CmdMemoryList, []string{"limit", "cursor"}},
	// Export an integrity-attested journal bundle for archival/compliance (M772):
	// every event with its hash + the chain head, re-verifiable offline. Read-only.
	"/api/journal/export": {controlplane.CmdJournalExport, []string{"since_ms"}},
	// Historical journal search (M618): the full CmdJournalGrep filter set —
	// free-text pattern plus kind/subject/actor/correlation — over all history,
	// powering the Search view. Read-only, like every readArgsRoute.
	"/api/journal_search": {controlplane.CmdJournalGrep, []string{"pattern", "kind", "subject", "actor", "correlation_id", "limit"}},
	"/api/provider_log":   {controlplane.CmdProviderLog, []string{"limit", "cursor", "fallbacks"}},
	"/api/tool_log":       {controlplane.CmdToolLog, []string{"limit", "cursor", "tool", "errors"}},
	"/api/execution_profile": {controlplane.CmdExecutionProfileShow, []string{
		"id",
	}},
	// Read one sandbox project file's content (M686), path-confined server-side.
	"/api/sandbox_file": {controlplane.CmdSandboxFile, []string{"project", "file"}},
	// Artifact index listing (M822): browsable metadata for stored artifacts
	// (inbound images, tool outputs), optionally filtered. No bytes — the raw
	// route below serves those.
	"/api/artifacts": {controlplane.CmdArtifactList, []string{"kind", "source", "corr"}},
	// Personal Data Lake records query (M836): one collection, filtered/sorted/paged.
	"/api/data/records": {controlplane.CmdDataRecords, []string{"collection", "search", "sort", "desc", "limit", "offset"}},
	// Agent graveyard impact (M846): what standing orders fire this agent. Read-only.
	"/api/agents/impact": {controlplane.CmdAgentImpact, []string{"ref"}},
	// Agent effective permissions: roster tool allow/deny + Edict/trust ceiling. Read-only.
	"/api/agents/permissions": {controlplane.CmdAgentPermissions, []string{"ref"}},
	// Per-agent activity timeline (M854): what the agent did, from the journal.
	// Cursor pagination (M-pending follow-up): the SPA's IncidentPage /
	// AgentPage views load this on every poll, and the journal can hold tens
	// of thousands of events. `cursor` is the opaque "<seq>" boundary of the
	// previous page; server skips entries with seq >= cursorSeq. Journal seq
	// is monotonic per kernel, so this is unique without a tie-break.
	"/api/agents/activity": {controlplane.CmdAgentActivity, []string{"ref", "limit", "cursor"}},
	// Autonomous self-repair history/state: queued/completed/failed auto-repair
	// attempts for one agent, with inflight/cooldown detail. Read-only.
	// Same `<seq>` cursor as /api/agents/activity.
	"/api/agents/repair_status": {controlplane.CmdAgentRepairStatus, []string{"ref", "limit", "cursor"}},
	// Owner/parent escalation queue for one agent: doctor-triggered help requests
	// it is currently responsible for, enriched with wake/provenance metadata.
	// Cursor pagination: `<ts_unix_ms>:<message_id>` — ts can collide across
	// board messages so the message_id is the tie-break. server skips entries
	// strictly newer-or-equal to the cursor.
	"/api/agents/escalations": {controlplane.CmdAgentEscalations, []string{"ref", "limit", "cursor"}},
	// Rated agent Config Center (distinct from daemon /api/config settings):
	// key/value entries agents can read under rating + allow/deny policy.
	"/api/configcenter/list": {controlplane.CmdConfigCenterList, []string{"rating"}},
	"/api/configcenter/get":  {controlplane.CmdConfigCenterGet, []string{"key"}},
	// Reaper scan (M903): dead-agent + stale-artifact candidates. Read-only detection. (#53)
	"/api/reaper/scan": {controlplane.CmdReaperScan, []string{"idle_days", "stale_days"}},
	"/api/workboard/lanes": {controlplane.CmdWorkboardLanes, []string{
		"status", "tenant", "limit", "include_archived",
	}},
	"/api/workboard/watch": {controlplane.CmdWorkboardWatch, []string{"id", "run_id", "limit"}},
	"/api/okr/show":        {controlplane.CmdOKRShow, []string{"id"}},
	// Skill bundle resources (M847): list a skill's reference files + scripts, and
	// read one resource's content. Both read-only; the daemon path-confines reads.
	"/api/skill/files": {controlplane.CmdSkillFiles, []string{"id"}},
	"/api/skill/file":  {controlplane.CmdSkillReadFile, []string{"id", "path"}},
	// Skill hygiene (M858): active skills that look idle (never/long-unused). Read-only.
	"/api/skills/hygiene": {controlplane.CmdSkillHygiene, []string{"idle_days"}},
	// Marketplace (capability packs): browse the catalogue + one pack's contents.
	// Read-only.
	"/api/market":         {controlplane.CmdMarketList, []string{"query"}},
	"/api/market/show":    {controlplane.CmdMarketShow, []string{"name", "marketplace"}},
	"/api/market/sources": {controlplane.CmdMarketSources, nil},
	"/api/policy_log":     {controlplane.CmdEdictLog, []string{"limit", "cursor", "denied"}},
	// Chat suggested-prompts (memory-derived + tool-context). Read-only: blends
	// the agent's active memory into starter/next-step chips for the chat surface.
	// tools is a comma-joined list of recently-used tool names.
	"/api/suggestions": {controlplane.CmdChatSuggestions, []string{"session_id", "tools"}},
	// Resolved HITL approval history (M773): a timeline of past approval requests
	// joined with their granted/denied/timeout outcome. Read-only.
	"/api/approvals_log": {controlplane.CmdApprovalsLog, []string{"limit", "cursor", "denied"}},
	"/api/plan_history":  {controlplane.CmdPlanHistory, []string{"limit", "cursor", "status"}},
	// Provider keyring list (M700): labels + active + last-4 for one provider/env.
	// Read-only — values never leave the daemon.
	"/api/provider/keys": {controlplane.CmdProviderKeyList, []string{"provider", "env"}},
	// Forecast a schedule's next fire times (M744): id + how many. Read-only preview.
	"/api/schedule/test": {controlplane.CmdScheduleTest, []string{"id", "count"}},
	// Schedule firing history (M976): cronjob executions as structured actions,
	// not prompt text. Read-only and filterable for the dashboard.
	"/api/schedule/fires": {controlplane.CmdScheduleFires, []string{"limit", "cursor", "id", "status", "since_ms", "intent"}},
	// A2 Phase 2: register the six log endpoints that previously streamed full
	// slices via apiRoutes (no-args proxy). They now expose cursor pagination.
	"/api/ratelimit_log": {controlplane.CmdRateLimitLog, []string{"limit", "cursor", "since_ms"}},
	"/api/webhook_log":   {controlplane.CmdWebhookLog, []string{"limit", "cursor", "since_ms"}},
	"/api/warden_log":    {controlplane.CmdWardenLog, []string{"limit", "cursor", "since_ms"}},
	"/api/netguard_log":  {controlplane.CmdNetguardLog, []string{"limit", "cursor", "since_ms"}},
	"/api/world_log":     {controlplane.CmdWorldLog, []string{"limit", "cursor", "since_ms"}},
	"/api/memory_log":    {controlplane.CmdMemoryLog, []string{"limit", "cursor", "since_ms"}},
	// A standing order's life story (M746): every standing.* journal event for it —
	// created, paused/resumed, each firing, removed. Read-only provenance.
	"/api/standing/why": {controlplane.CmdStandingWhy, []string{"id"}},
	// One script tool's full record incl. the code body (M795) — the list route
	// deliberately strips code; the Forge view's editor fetches it here. Read-only.
	"/api/toolforge/show": {controlplane.CmdToolforgeShow, []string{"ref"}},
	// One workflow's full graph (M798) — the list stays light; the canvas
	// editor fetches nodes+edges here. Read-only.
	"/api/workflows/show": {controlplane.CmdWorkflowShow, []string{"ref"}},
	// Run history (M806): the journal folded into per-run arcs so the canvas
	// can replay any past run. Read-only.
	"/api/workflows/runs": {controlplane.CmdWorkflowRuns, []string{"ref", "limit"}},
	// Dry-run a policy decision (M753): "if the agent asked to do <capability> with
	// <input>, would the edict engine allow / ask / deny it, and via which rule?".
	// Read-only — eng.Decide mutates nothing.
	"/api/edict/test": {controlplane.CmdEdictTest, []string{"capability", "input"}},
	// Trace an event's causation (M755): the chain of journal events linked by
	// causation_id from the root cause down to this one — crossing correlation
	// boundaries (e.g. a heartbeat tick → the initiative it spawned → the run). Plus
	// the correlation group and a sub-agent's parent backlink. Read-only provenance.
	"/api/why": {controlplane.CmdWhy, []string{"event_id"}},
}

// writeRoutes is the operator-action allowlist: the big red button (halt),
// its inverse (resume), and HITL approval resolution (decide). Each is
// POST-only (see writeProxy).