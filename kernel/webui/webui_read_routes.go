// SPDX-License-Identifier: MIT

package webui

// Provenance: Read-only HTTP routes: apiRoutes (parameterless GETs) + readArgsRoutes
//             (GETs with query args). Code extracted from webui.go during the Day-51
//             god-file split. Public API unchanged.

import (
	"github.com/agezt/agezt/kernel/contract/opapi"
)

var apiRoutes = map[string]string{
	"/api/status": opapi.Status,
	"/api/config": opapi.Config,
	"/api/budget": opapi.Budget,
	// Mission Control "Spend today" tile (Day 28+1). Slim shape {total:int} the
	// useSpendToday hook consumes — pairs with /api/budget which carries the
	// full breakdown.
	"/api/spend/today":             opapi.SpendToday,
	"/api/catalog":                 opapi.CatalogList,
	"/api/execution_profiles":      opapi.ExecutionProfiles,
	"/api/execution_profile_check": opapi.ExecutionProfileCheck,
	"/api/tools_catalog":           opapi.ToolList,
	"/api/policy":                  opapi.EdictStats,
	"/api/edict_show":              opapi.EdictShow,
	"/api/schedules":               opapi.ScheduleList,
	"/api/schedule/system_tasks":   opapi.ScheduleSystemTasks,
	"/api/memory/audit":            opapi.MemoryAudit,
	"/api/world":                   opapi.WorldList,
	"/api/skills":                  opapi.SkillList,
	"/api/standing":                opapi.StandingList,
	"/api/mcp":                     opapi.MCPList,
	// CLI Toolbox (M956): the install action streams, so it has its own proxy
	// (toolInstallProxy) below. The detect/updates pair is unused by the Web UI
	// (verified in TOPOLOGY-AUDIT-DAY28-VERIFY.md §2) and reachable via the CLI
	// through opapi.ToolboxDetect / opapi.ToolboxOutdated when an operator needs them.
	"/api/acp/agents": opapi.ACPAgents,
	"/api/workflows":  opapi.WorkflowList,
	// Built-in workflow template gallery (M807). Read-only.
	"/api/workflows/templates": opapi.WorkflowTemplates,
	// Open (unanswered) help requests agents have raised on the board (M849). Read-only.
	"/api/board/help": opapi.BoardHelp,
	// Personal Data Lake (M836): list collections (no args). Read-only.
	"/api/data/collections": opapi.DataCollections,
	// Council of Elders (M839): the default membership the panel convenes with. Read-only.
	"/api/council/members": opapi.CouncilMembers,
	"/api/autonomy":        opapi.AutonomyFeed,
	"/api/approvals":       opapi.Approvals,
	"/api/sandbox":         opapi.SandboxList,
	"/api/config/schema":   opapi.ConfigSchema,
	"/api/config/values":   opapi.ConfigValues,
	"/api/channels":        opapi.ChannelList,
	"/api/nodes":           opapi.NodeRegistry,
	// Build/version provenance (M971): semver + git revision, so the UI can show
	// exactly which build the daemon is running.
	"/api/version": opapi.Version,
	// Per-task model routing (M703): the effective chains + known task types.
	"/api/routing": opapi.RoutingGet,
	// Named reusable fallback chains (M963): the registry + default chain.
	"/api/chains":  opapi.ChainsGet,
	"/api/persona": opapi.PersonaGet,
	"/api/prompts": opapi.PromptsGet,
	// Pulse — the proactive heartbeat status (running/paused/beats/cadence) (M743).
	"/api/pulse": opapi.PulseStatus,
	// /api/pulse/asks and /api/pulse/asks/resolve went un-wired
	// (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2); reachable via opapi.PulseAsks /
	// opapi.PulseAskResolve when an operator needs them.
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
	"/api/journal": {opapi.JournalGrep, []string{"correlation_id", "kind", "limit"}},
	// Cursor-paginated runs (M-pending): per the audit follow-up the SPA
	// needs to page through long histories without paying for the whole
	// journal. `cursor` is the opaque "<ms>:<seq>" boundary of the previous
	// page, returned as `next_cursor` in the same shape.
	"/api/runs": {opapi.RunsList, []string{
		"limit", "cursor", "status", "intent", "model", "min_cost_mc", "max_cost_mc",
	}},
	// Cursor-paginated agents roster (M-pending): the SPA's Agents /
	// AgentPage / Roster / Board / Dashboard / Schedules / Standing /
	// IncidentPage / Overseer / Voice views all poll this. `cursor` is the
	// opaque "<CreatedMS>:<slug>" boundary of the previous page, returned
	// as `next_cursor` in the same shape. Slugs are unique and never
	// contain ':', so strings.Cut on ':' round-trips losslessly.
	"/api/agents": {opapi.AgentList, []string{"limit", "cursor"}},
	// Cursor-paginated conversation-shaped lists (M-pending follow-up):
	// /api/inbox (channel threads, newest activity first), /api/board
	// (board posts, newest ts first; topic filter preserved), /api/memory
	// (active records, newest CreatedMS first). All three use a "<ts_or_ms>:<id>"
	// cursor shape with the ID field as the tie-break when ts collides.
	"/api/inbox":  {opapi.Inbox, []string{"limit", "cursor", "channel"}},
	"/api/board":  {opapi.BoardRead, []string{"limit", "cursor", "topic"}},
	"/api/memory": {opapi.MemoryList, []string{"limit", "cursor"}},
	// Export an integrity-attested journal bundle for archival/compliance (M772):
	// Historical journal search (M618) and journal/export both went un-wired
	// (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2); reachable via opapi.JournalGrep /
	// opapi.JournalExport when an operator needs them.
	"/api/provider_log": {opapi.ProviderLog, []string{"limit", "cursor", "fallbacks"}},
	"/api/tool_log":     {opapi.ToolLog, []string{"limit", "cursor", "tool", "errors"}},
	// Read one sandbox project file's content (M686), path-confined server-side.
	"/api/sandbox_file": {opapi.SandboxFile, []string{"project", "file"}},
	// Artifact index listing (M822): browsable metadata for stored artifacts
	// (inbound images, tool outputs), optionally filtered. No bytes — the raw
	// route below serves those.
	"/api/artifacts": {opapi.ArtifactList, []string{"kind", "source", "corr"}},
	// Personal Data Lake records query (M836): one collection, filtered/sorted/paged.
	"/api/data/records": {opapi.DataRecords, []string{"collection", "search", "sort", "desc", "limit", "offset"}},
	// Agent graveyard impact (M846): what standing orders fire this agent. Read-only.
	"/api/agents/impact": {opapi.AgentImpact, []string{"ref"}},
	// Agent effective permissions: roster tool allow/deny + Edict/trust ceiling. Read-only.
	"/api/agents/permissions": {opapi.AgentPermissions, []string{"ref"}},
	// Per-agent activity timeline (M854): what the agent did, from the journal.
	// Cursor pagination (M-pending follow-up): the SPA's IncidentPage /
	// AgentPage views load this on every poll, and the journal can hold tens
	// of thousands of events. `cursor` is the opaque "<seq>" boundary of the
	// previous page; server skips entries with seq >= cursorSeq. Journal seq
	// is monotonic per kernel, so this is unique without a tie-break.
	"/api/agents/activity": {opapi.AgentActivity, []string{"ref", "limit", "cursor"}},
	// Autonomous self-repair history/state: queued/completed/failed auto-repair
	// attempts for one agent, with inflight/cooldown detail. Read-only.
	// Same `<seq>` cursor as /api/agents/activity.
	"/api/agents/repair_status": {opapi.AgentRepairStatus, []string{"ref", "limit", "cursor"}},
	// Owner/parent escalation queue for one agent: doctor-triggered help requests
	// it is currently responsible for, enriched with wake/provenance metadata.
	// Cursor pagination: `<ts_unix_ms>:<message_id>` — ts can collide across
	// board messages so the message_id is the tie-break. server skips entries
	// strictly newer-or-equal to the cursor.
	"/api/agents/escalations": {opapi.AgentEscalations, []string{"ref", "limit", "cursor"}},
	// Rated agent Config Center (distinct from daemon /api/config settings):
	// key/value entries agents can read under rating + allow/deny policy.
	"/api/configcenter/list": {opapi.ConfigCenterList, []string{"rating"}},
	// Reaper scan (M903): dead-agent + stale-artifact candidates. Read-only detection. (#53)
	"/api/reaper/scan": {opapi.ReaperScan, []string{"idle_days", "stale_days"}},
	// Skill bundle resources (M847): list a skill's reference files + scripts.
	// Read-only; the daemon path-confines reads. The single-file GET was never
	// wired (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2); reachable via opapi.SkillReadFile
	// when an operator needs it.
	"/api/skill/files": {opapi.SkillFiles, []string{"id"}},
	// Skill hygiene (M858): active skills that look idle (never/long-unused). Read-only.
	"/api/skills/hygiene": {opapi.SkillHygiene, []string{"idle_days"}},
	// Marketplace (capability packs): browse the catalogue + one pack's contents.
	// Read-only.
	"/api/market":         {opapi.MarketList, []string{"query"}},
	"/api/market/show":    {opapi.MarketShow, []string{"name", "marketplace"}},
	"/api/market/sources": {opapi.MarketSources, nil},
	"/api/policy_log":     {opapi.EdictLog, []string{"limit", "cursor", "denied"}},
	// Chat suggested-prompts (memory-derived + tool-context). Read-only: blends
	// the agent's active memory into starter/next-step chips for the chat surface.
	// tools is a comma-joined list of recently-used tool names.
	"/api/suggestions": {opapi.ChatSuggestions, []string{"session_id", "tools"}},
	// Resolved HITL approval history (M773): a timeline of past approval requests
	// joined with their granted/denied/timeout outcome. Read-only.
	"/api/approvals_log": {opapi.ApprovalsLog, []string{"limit", "cursor", "denied"}},
	// Mission Control "Needs your attention" panel (Day 28+1). Cross-cuts pending
	// approvals + recent pulse asks into one time-sorted feed. Optional window
	// ("5m"/"1h"/"24h") filters pulse asks; approvals ignore it. limit caps the
	// result (default 8, hard cap 50 enforced server-side). Read-only.
	"/api/attention":    {opapi.Attention, []string{"window", "limit"}},
	"/api/plan_history": {opapi.PlanHistory, []string{"limit", "cursor", "status"}},
	// Provider keyring list (M700): labels + active + last-4 for one provider/env.
	// Read-only — values never leave the daemon.
	"/api/provider/keys": {opapi.ProviderKeyList, []string{"provider", "env"}},
	// Forecast a schedule's next fire times (M744): id + how many. Read-only preview.
	"/api/schedule/test": {opapi.ScheduleTest, []string{"id", "count"}},
	// Schedule firing history (M976): cronjob executions as structured actions,
	// not prompt text. Read-only and filterable for the dashboard.
	"/api/schedule/fires": {opapi.ScheduleFires, []string{"limit", "cursor", "id", "status", "since_ms", "intent"}},
	// A2 Phase 2: register the six log endpoints that previously streamed full
	// slices via apiRoutes (no-args proxy). They now expose cursor pagination.
	"/api/ratelimit_log": {opapi.RateLimitLog, []string{"limit", "cursor", "since_ms"}},
	"/api/webhook_log":   {opapi.WebhookLog, []string{"limit", "cursor", "since_ms"}},
	"/api/warden_log":    {opapi.WardenLog, []string{"limit", "cursor", "since_ms"}},
	"/api/netguard_log":  {opapi.NetguardLog, []string{"limit", "cursor", "since_ms"}},
	"/api/world_log":     {opapi.WorldLog, []string{"limit", "cursor", "since_ms"}},
	"/api/memory_log":    {opapi.MemoryLog, []string{"limit", "cursor", "since_ms"}},
	// A standing order's life story (M746): every standing.* journal event for it —
	// created, paused/resumed, each firing, removed. Read-only provenance.
	"/api/standing/why": {opapi.StandingWhy, []string{"id"}},
	// One workflow's full graph (M798) — the list stays light; the canvas
	// editor fetches nodes+edges here. Read-only.
	"/api/workflows/show": {opapi.WorkflowShow, []string{"ref"}},
	// Run history (M806): the journal folded into per-run arcs so the canvas
	// can replay any past run. Read-only.
	"/api/workflows/runs": {opapi.WorkflowRuns, []string{"ref", "limit"}},
	// Dry-run a policy decision (M753): "if the agent asked to do <capability> with
	// <input>, would the edict engine allow / ask / deny it, and via which rule?".
	// Read-only — eng.Decide mutates nothing.
	"/api/edict/test": {opapi.EdictTest, []string{"capability", "input"}},
	// Trace an event's causation (M755): /api/why was never wired
	// (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2); reachable via opapi.Why when an operator
	// needs to inspect causation provenance.
}

// writeRoutes is the operator-action allowlist: the big red button (halt),
// its inverse (resume), and HITL approval resolution (decide). Each is
// POST-only (see writeProxy).
