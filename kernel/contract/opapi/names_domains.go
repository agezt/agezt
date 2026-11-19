// SPDX-License-Identifier: MIT

package opapi

// Provenance: the Domain half of the control plane's command vocabulary,
// moved here from kernel/controlplane/protocol_commands_commands_domains.go when the
// operation vocabulary got one home (W3.79). The values are unchanged; the
// identifiers lost the `Cmd` prefix that named the transport's own alias.
//
// Every name below is declared exactly once, in this package. The application
// layer binds these constants into the operation's spec, and the transports
// refer to them by these names. A name that can be written in two places is a
// name that can drift, and a name that drifts is a route the daemon no longer
// answers to.

const (
	//
	// WorldAdd creates (or reinforces) an entity.
	// Args:
	//   - name    : string (required) — entity name
	//   - kind    : string (optional) — project|repo|person|org|account|
	//               device|channel|topic|task (default topic)
	//   - aliases : [string] (optional) — phrases that resolve to it
	//   - attrs   : {k:v} (optional) — preferences/habits/constraints
	// Returns: { id, created (bool), kind, name }
	WorldAdd = "world_add"
	// WorldEdit replaces an existing entity's aliases and attrs in place (M730)
	// — the full editable state, so an alias/attr can be removed (Upsert only
	// merges). Identity (id/kind/name) and weight are preserved.
	// Args: id (required), aliases ([string], optional), attrs ({k:v}, optional).
	// Returns: { updated (bool), id, kind, name }
	WorldEdit = "world_edit"
	// WorldRelate asserts a directed relation between two entities named
	// from/to (endpoints auto-created as topics if unknown).
	// Args: from (required), verb (optional; default relates_to), to (required).
	// Returns: { id, from, verb, to }
	WorldRelate = "world_relate"
	// WorldResolve ranks active entities matching a phrase.
	// Args: query (required), limit (optional; default 10, 1..100).
	// Returns: { results: [{entity, score}, ...], count }
	WorldResolve = "world_resolve"
	// WorldNeighbors lists the active edges incident to the entity that
	// best matches a phrase, with the adjacent entity for each.
	// Args: query (required). Returns: { entity, neighbors: [...], count }
	WorldNeighbors = "world_neighbors"
	// WorldList returns active entities (and a relation count). No args.
	// Returns: { entities: [...], count, relation_count }
	WorldList = "world_list"
	// WorldLog lists recent world-model operations (M86) — a timeline of the
	// journal's worldmodel.entity.upserted / relation.upserted / forgotten
	// events. Args: limit (optional), kind (optional: entity|relation), since_ms
	// (optional window). Returns: { ops: [ {ts_unix_ms, op, what, label} ], count }
	WorldLog = "world_log"
	// WorldGet reads one entity by id (any state).
	// Args: id (required). Returns: { found, entity }
	WorldGet = "world_get"
	// WorldForget tombstones an entity (soft delete; reversible,
	// journaled). Args: id (required). Returns: { forgotten }
	WorldForget = "world_forget"
	// Forge / skills (SPEC-05 §4–5). The journaled skill-lifecycle behind
	// `agt skill`; transitions go through the kernel's skill forge so every
	// promote/quarantine/revert is auditable via `agt why`.
	//
	// SkillList returns all skills (any state), with an active count.
	// No args. Returns: { skills: [...], count, active_count }
	SkillList = "skill_list"
	// SkillGet reads one skill by id.
	// Args: id (required). Returns: { found, skill }
	SkillGet = "skill_get"
	// SkillHistory folds the journal for one skill's lifecycle events.
	// Args: id (required). Returns: { id, events: [...], count }
	SkillHistory = "skill_history"
	// SkillPromote advances draft→shadow→active (or un-quarantines).
	// Args: id (required). Returns: { id, status }
	SkillPromote = "skill_promote"
	// SkillQuarantine pulls an active/shadow skill from production.
	// Args: id (required), reason (optional). Returns: { id, status }
	SkillQuarantine = "skill_quarantine"
	// SkillArchive retires any non-archived skill without restoring a
	// lineage parent. This is the proposal-reject path.
	// Args: id (required), reason (optional). Returns: { id, status, reason }
	SkillArchive = "skill_archive"
	// SkillRevert archives a skill and re-activates its lineage parent
	// (non-destructive — appends a reversal).
	// Args: id (required). Returns: { id, restored }
	SkillRevert = "skill_revert"
	// SkillRestore restores a skill's status from an operator checkpoint.
	// It is for rollback tooling, not normal promotion flow; the restore is
	// journaled as skill.restored. Args: id, status (required), reason (optional).
	// Returns: { id, from, status, reason }
	SkillRestore = "skill_restore"
	// SkillShare promotes a private (per-agent, M932) skill to the shared
	// pool every agent retrieves — the ownership analogue of memory_promote
	// (M915). Clears Skill.Agent. Args: id (required).
	// Returns: { shared, id, name?, from_agent? }
	SkillShare = "skill_share"
	// SkillReassign changes a skill's owning agent (M942). An empty agent
	// shares it (same as skill_share); a non-empty slug must exist in the
	// roster. Args: id (required), agent (optional, "" = shared).
	// Returns: { reassigned, id, name?, from_agent?, to_agent }
	SkillReassign = "skill_reassign"
	// SkillImport installs a skill from a portable bundle as a fresh DRAFT
	// (via the Forge, so it is content-addressed, deduped, and journaled like
	// any other authored skill). Args: name, body (required); description,
	// triggers, tools_required, agent, resources (optional; resources is an
	// object of {relative-path: text-content} — the agentskills.io bundle of
	// reference files + scripts, M847).
	// Returns: { id, name, status, created, resources }
	SkillImport = "skill_import"
	// SkillFiles lists a skill's on-disk bundle resources (M847).
	// Args: id (required). Returns: { id, name, files: [...], dir, count }
	// SkillHygiene reports active skills that look idle (never used, or not used
	// in idle_days) — the cleanup view (M858). Args: idle_days (optional; default
	// 30). Returns: { idle_days, total, active, idle:[...], idle_count }
	SkillHygiene = "skill_hygiene"
	SkillFiles   = "skill_files"
	// SkillReadFile returns one bundle resource's text content (M847).
	// Args: id, path (required). Returns: { id, name, path, content, bytes }
	SkillReadFile = "skill_read_file"
	// Standing orders (SPEC-16 §4) — durable wake-rule CRUD behind `agt
	// standing`. Add: args.order (object). SetEnabled: args{id, enabled}.
	// Remove: args.id. List: no args. Every mutation is journaled (standing.*).
	StandingList       = "standing_list"
	StandingAdd        = "standing_add"
	StandingEdit       = "standing_edit" // edit an order's mutable fields in place (M729)
	StandingSetEnabled = "standing_set_enabled"
	StandingRemove     = "standing_remove"
	StandingWhy        = "standing_why"  // fold the journal for one order's life story
	StandingFire       = "standing_fire" // fire an order now, ignoring its triggers (M765)
	// Agent roster (M783) — durable named agent profiles behind `agt agent`.
	// Add: args.profile (object). Edit: args{ref, profile}. SetEnabled:
	// args{ref, enabled}. Remove: args.ref plus optional cascade flags:
	// standing removes bound standing orders, schedules removes bound cadence
	// jobs, memory forgets private scoped memory, authored_memory forgets shared
	// memory authored by the agent, skills archives agent-owned skills, config
	// deletes identity-owned config center entries, and subagents retires
	// dependent child agents before removing the parent (it does not hard-delete
	// child profiles). List: no args. ref = id OR slug. Every mutation is
	// journaled (roster.*).
	AgentList       = "agent_list"
	AgentAdd        = "agent_add"
	AgentEdit       = "agent_edit"
	AgentSetEnabled = "agent_set_enabled"
	AgentRemove     = "agent_remove"
	// AgentTaskUpdate mutates one durable tasklist item without rewriting the
	// whole profile. Args: ref, op(add|update|remove), id (update/remove),
	// title/description/scope/status or task object. Returns: { profile, task?,
	// updated }.
	AgentTaskUpdate = "agent_task_update"
	// Agent graveyard (M846). AgentImpact reports what depends on an agent
	// (standing orders that fire it) — shown before retiring. AgentRetire moves
	// it to the graveyard (recoverable, excluded from delegation) and returns the
	// impact; AgentRevive brings it back (paused, for the operator to resume).
	// Args: ref. Returns: profile (+ impact for retire/impact).
	AgentImpact = "agent_impact"
	AgentRetire = "agent_retire"
	AgentRevive = "agent_revive"
	// AgentTombstone returns a read-only "death certificate" for one agent: its
	// identity summary, lifecycle/retirement record, and the durable resource
	// footprint it leaves behind (the same counts AgentImpact computes). It is a
	// portable archival/audit snapshot — it does NOT remove or mutate anything.
	// Args: ref. Returns: { tombstone }.
	AgentTombstone = "agent_tombstone"
	// AgentGraveyard lists retired (graveyard) agents with their retirement age,
	// newest-first by default, optionally filtered to those retired longer than
	// older_than_days — the read-only retention-eligibility view. It REPORTS only;
	// it never archives or hard-removes (that stays an explicit operator action).
	// Args: optional older_than_days. Returns: { graveyard:[...], count }.
	AgentGraveyard = "agent_graveyard"
	// AgentPermissions returns the effective tool exposure/policy picture for
	// one agent: roster allow/deny, global Edict level, trust ceiling clamp, and
	// the resulting status for each registered tool. Args: ref. Returns:
	// { slug, trust_ceiling, permissions:[...] }.
	AgentPermissions = "agent_permissions"
	// AgentCapabilities patches only the agent-local capability/config surface:
	// trust ceiling, tool allow/deny, noise policy, and config overrides. Args:
	// ref plus any subset of trust_ceiling, tool_allow, tool_deny, noise_policy,
	// config_overrides. Returns the updated profile and effective permissions.
	AgentCapabilities = "agent_capabilities"
	// AgentActivity returns a per-agent activity timeline derived from the
	// journal (M854): the runs it executed, council consults + delegations during
	// them, memory it wrote, board messages, and profile changes — newest first.
	// Args: ref (required), limit (optional). Returns: { slug, activity:[...], count, total }
	AgentActivity = "agent_activity"
	// AgentRepairStatus returns the journal-derived autonomous repair history
	// for one agent: queued/completed/failed self-repair dispatches, the current
	// inflight queue (if any), and the effective cooldown window. Args: ref
	// (required), limit (optional). Returns: { slug, history:[...], latest?,
	// inflight:[...], cooldown_sec, next_eligible_ms? }.
	AgentRepairStatus = "agent_repair_status"
	// AgentRepair triggers an operator-requested governed repair run AS the
	// target agent, asynchronously. Args: ref (required), reason (optional),
	// incident_id/root_incident_id/parent_incident_id (optional lineage).
	// Returns: { accepted, agent, correlation_id } immediately; progress is
	// journaled as agent.repair info events and the underlying run events.
	AgentRepair = "agent_repair"
	// AgentEscalations returns the owner/parent escalation queue for one
	// agent: open or recent doctor-triggered help requests assigned to it,
	// enriched with wake status and source-agent provenance. Args: ref
	// (required), limit (optional). Returns: { slug, escalations:[...], open_count }.
	AgentEscalations = "agent_escalations"
	// AgentWake explicitly wakes a named agent now, asynchronously, with an
	// operator-supplied intent or reason. Args: ref (required), intent
	// (optional), reason (optional), incident_id/root_incident_id/
	// parent_incident_id (optional lineage). Returns: { accepted, agent,
	// correlation_id } immediately; progress is journaled as agent.wake info
	// events and the underlying run events.
	AgentWake = "agent_wake"
	// AgentResolve applies an operator-selected incident resolution to one
	// agent, synchronously: paused / retired / delegated / force_chain. Args:
	// ref (required), resolution (required), summary (optional), delegate_to
	// (required when delegated), task_type/task_model_chain (required when
	// force_chain), incident lineage (optional). Returns { applied, agent,
	// resolution } and journals agent.resolve info events.
	AgentResolve = "agent_resolve"
	// Script-tool forge (M794) — agent-authored code promoted into callable
	// forge_<name> tools, behind `agt toolforge`. Draft: args.tool (object).
	// Edit: args{ref, tool}. Test: args{ref, input?} (runs the code in the
	// sandbox and records the verdict; promotion requires a pass). Promote /
	// Quarantine(+reason?) / Remove / Show: args.ref. List: no args.
	// ref = id OR name. Every mutation is journaled (scripttool.*).
	ToolforgeList       = "toolforge_list"
	ToolforgeShow       = "toolforge_show"
	ToolforgeDraft      = "toolforge_draft"
	ToolforgeEdit       = "toolforge_edit"
	ToolforgeTest       = "toolforge_test"
	ToolforgePromote    = "toolforge_promote"
	ToolforgeQuarantine = "toolforge_quarantine"
	ToolforgeRemove     = "toolforge_remove"
	// MCP self-install (M796) — runtime-attached MCP servers behind
	// `agt mcp`. Add: args.server (object {name,command,args,description}).
	// Attach/Detach/Remove: args.ref (name or id). SetEnabled:
	// args{ref,enabled} (auto-attach at daemon start). List: no args —
	// returns registrations joined with live attachment status.
	// Every mutation is journaled (mcp.*).
	MCPList       = "mcp_list"
	MCPAdd        = "mcp_add"
	MCPAttach     = "mcp_attach"
	MCPDetach     = "mcp_detach"
	MCPSetEnabled = "mcp_set_enabled"
	MCPRemove     = "mcp_remove"
	// CLI toolbox (M956) — host CLI-tool inventory + installer behind the
	// Setup → Toolbox page. Detect/Outdated: no args, read-only host probe
	// (LookPath + bounded --version; package-manager upgrade-list). Install:
	// args.names ([]string) — streams one progress event per tool then a final
	// {installed,failed,skipped} result. Runs the host package manager
	// (winget/choco/brew/apt…) at host level; every install is journaled
	// (toolbox.*).
	ToolboxDetect   = "toolbox_detect"
	ToolboxOutdated = "toolbox_outdated"
	ToolboxInstall  = "toolbox_install"
	// Marketplace: browse + install capability packs (skills + MCP servers + CLI
	// tool requirements) from the built-in Official catalogue (and, later, synced
	// remotes). Install materializes a pack into the Forge + MCP registry; every
	// install/uninstall is journaled (market.*).
	MarketList      = "market_list"
	MarketShow      = "market_show"
	MarketInstall   = "market_install"
	MarketUninstall = "market_uninstall"
	// Remote marketplace sources (Phase 2): configure + sync external catalogues
	// into the local cache (netguard-screened, keep-last-good).
	MarketSources      = "market_sources"
	MarketAddSource    = "market_add_source"
	MarketRemoveSource = "market_remove_source"
	MarketSync         = "market_sync"
	// ACPAgents discovers the Agent Client Protocol (ACP) coding agents
	// installed on the host (Gemini CLI, Claude Code's adapter, Codex, …): which
	// are present (+version+path), which are missing (+install hint), and which is
	// the configured default. Read-only. Drives the acp_agent bridge picker.
	ACPAgents = "acp_agents"
	// Workflow engine (M798) — durable typed-node graphs behind
	// `agt workflow` and the console canvas. Save: args.workflow (the whole
	// graph object; upsert by name). Run: args{ref, payload?} — executes
	// synchronously and returns {outputs, executed}. Show/Remove: args.ref.
	// SetEnabled: args{ref, enabled} (arms triggers — M799). List: no args.
	// Every mutation + every run arc is journaled (workflow.*).
	WorkflowList       = "workflow_list"
	WorkflowShow       = "workflow_show"
	WorkflowSave       = "workflow_save"
	WorkflowRestore    = "workflow_restore"
	WorkflowRemove     = "workflow_remove"
	WorkflowSetEnabled = "workflow_set_enabled"
	WorkflowRun        = "workflow_run"
	// Draft (M802): args{description, name?} — the copilot designs a
	// validated workflow from plain language; returned UNSAVED for review.
	WorkflowDraft = "workflow_draft"
	// Refine (M805): args{instruction, workflow?|ref?} — the copilot revises
	// an existing graph (the posted one, or the stored one at ref) per a
	// plain-language change request; returned UNSAVED for review.
	WorkflowRefine = "workflow_refine"
	// Runs (M806): args{ref, limit?} — fold the journal into the workflow's
	// run history (started→node…→completed|failed arcs, newest first), so
	// the console can replay any past run on the canvas. Read-only.
	WorkflowRuns = "workflow_runs"
	// Templates (M807): no args — the built-in gallery (curated, validated
	// starting points), full graphs included. Read-only; instantiation is
	// just a save under a new name.
	WorkflowTemplates = "workflow_templates"
	// Test node (M811): args{workflow (graph), node, data?, payload?} — run
	// ONE node with caller-supplied upstream data, under the full run
	// machinery (policy, reliability, metering). Returns {output, port,
	// attempts}. The journal event is flagged test:true (out of history).
	WorkflowTestNode = "workflow_test_node"
	// Webhook fire (M809): args{ref, secret, payload?} — authenticate an
	// external POST against the workflow's webhook trigger (enabled +
	// kind=webhook + constant-time secret match) and start the run ASYNC.
	// Returns {accepted, correlation_id} immediately; the journal carries
	// the arc. The webui's tokenless /hooks/<name> path is the only caller.
	WorkflowWebhook = "workflow_webhook"
	// Sandbox projects (M686) — read-only inspection of what agents BUILT with the
	// code_exec tool under <baseDir>/sandbox/projects. List: no args, returns each
	// persistent project with its files (name, bytes, modified). File: args{project,
	// file}, returns one file's content (capped), path-confined to the projects dir.
	SandboxList   = "sandbox_list"
	SandboxFile   = "sandbox_file"
	SandboxDelete = "sandbox_delete" // remove one project dir (operator cleanup); path-confined
	// Config Center (M693) — schema-driven configuration. Schema: the editable
	// surface. Values: current state (non-secret values + secret presence, never
	// secret values). Set: args{name, value} → config store (non-secret) or vault
	// (secret); provider/model apply live, the rest "restart to apply".
	ConfigSchema = "config_schema"
	ConfigValues = "config_values"
	ConfigSet    = "config_set"
	// ChannelList returns the registered communication-channel manifests
	// joined with their Config Center account fields + a configured flag — the
	// data the Channels wizard renders. Read-only.
	ChannelList = "channel_list"
	// NodeRegistry returns the local daemon plus configured AGEZT peer nodes
	// with token-redacted reachability, for companion/node-registry UX.
	NodeRegistry = "node_registry"
	// Multi-account channel management: write/remove one channel account instance's
	// fields (the default or a "#label" account). args — Set: kind,label,name,value;
	// Remove: kind,label. Listing rides ChannelList's per-channel "accounts".
	ChannelAccountSet    = "channel_account_set"
	ChannelAccountRemove = "channel_account_remove"
	// OAuth connect flow (Phase 4) for channels whose ConnectMethod is "oauth"
	// (Slack/Discord/Mastodon). Start: args{kind,label,client_id,client_secret,
	// redirect_uri[,instance_url]} → {authorize_url,state}; the browser is sent to
	// authorize_url and redirected back to the daemon's /oauth/callback, which
	// invokes Callback: args{code,state} → exchanges the code and writes the bot
	// token into the account's "#label" vault slot. Status: args{state} →
	// {status: pending|done|error}.
	ChannelOAuthStart    = "channel_oauth_start"
	ChannelOAuthCallback = "channel_oauth_callback"
	ChannelOAuthStatus   = "channel_oauth_status"
	// Provider OAuth (Phase: ChatGPT subscription) — "Sign in with ChatGPT".
	// Start: args{provider:"chatgpt"} → {authorize_url,state}; the browser
	// authorizes and is redirected to the daemon's own 127.0.0.1:1455 listener
	// (the Codex client's fixed redirect) which exchanges the code into the token
	// vault. Status: args{state} → {status}. Import: pull ~/.codex/auth.json.
	// Logout: clear the stored tokens.
	ProviderOAuthStart  = "provider_oauth_start"
	ProviderOAuthStatus = "provider_oauth_status"
	ProviderOAuthImport = "provider_oauth_import"
	ProviderOAuthLogout = "provider_oauth_logout"
	// Schema registry (M695): skills/plugins register their own config sections
	// into <baseDir>/schemas/*.json. Register: args{section}; Unregister: args{id}.
	ConfigSchemaRegister   = "config_schema_register"
	ConfigSchemaUnregister = "config_schema_unregister"
	// Reflection — meta-cognition (SPEC-05 §6).
	//
	// ReflectRun runs one reflection pass now: folds the journal, applies
	// world-model decay, and journals the report. No args.
	// Returns: the Report (observations, entities_decayed, proposals).
	ReflectRun = "reflect_run"
	// ReflectShow returns the latest reflection report from the journal.
	// No args. Returns: { found, report }
	ReflectShow = "reflect_show"
	// Pulse — the proactive heart (SPEC-03). These control the resident
	// heartbeat the daemon runs in the background. When Pulse is disabled
	// (AGEZT_PULSE=off) the handlers report it rather than erroring.
	//
	// PulseStatus returns the engine snapshot (running, beats,
	// observers, dial, cadence, last tick, pending digest). No args.
	PulseStatus = "pulse_status"
	// PulseAsks lists actionable observations awaiting an operator verdict under
	// initiative=ask (M1001). No args. Returns { asks: [...] }.
	PulseAsks = "pulse_asks"
	// PulseAskResolve settles one pending ask (M1001). Args: issue_key, approve
	// (bool). Approval re-emits the signal onto pulse.initiative.act. Returns
	// { resolved, approved, acted }.
	PulseAskResolve = "pulse_ask_resolve"
	// PulsePause suppresses new beats (in-flight processing finishes).
	// No args. Returns { paused: true }.
	PulsePause = "pulse_pause"
	// PulseResume re-enables beats. No args. Returns { paused: false }.
	PulseResume = "pulse_resume"
	// PulseBeat triggers one on-demand heartbeat ("think now"). No args.
	// Fires even when paused. Returns { triggered: true }.
	PulseBeat = "pulse_beat"
	// PulseCadence changes the heartbeat interval live. Args: seconds.
	// Clamped to a sane range; returns { cadence_ms }. Runtime-only.
	PulseCadence = "pulse_cadence"
	// PulseDial changes the proactivity dial live. Args: dial
	// (quiet|balanced|chatty). Returns { dial }. Runtime-only.
	PulseDial = "pulse_dial"
	// PulseFlush delivers held digest items now. No args. Returns { flushed }.
	PulseFlush = "pulse_flush"
	// PulseWatch adds a disk-space watch at runtime. Args: path, min_pct.
	// Returns { added, observer }.
	PulseWatch = "pulse_watch"
	// PulseProbe adds a command-probe watch at runtime. Args: name, command.
	// Returns { added, observer }.
	PulseProbe = "pulse_probe"
	// PulseUnwatch removes runtime-added watches by observer name (M769) — the
	// inverse of pulse_watch/pulse_probe. Args: name. Returns { removed }.
	PulseUnwatch = "pulse_unwatch"
	// PulseQuiet sets the quiet-hours window live (M770). Args: hours ("START-END"
	// 24h, e.g. "22-7"; empty disables). Returns { quiet } (the applied spec).
	PulseQuiet = "pulse_quiet"
	// Inbox returns the Unified Inbox (SPEC-07 §4): channel.inbound /
	// channel.outbound events folded into conversation threads grouped by
	// correlation_id, newest activity first.
	// Args: limit (optional; default 20, clamped 1..1000); channel (optional;
	// case-insensitive channel-kind filter, e.g. "telegram"|"slack"|"discord").
	// Returns: { threads: [{correlation_id, channel_kind, channel_id,
	//            messages:[{direction,sender,text,ts_unix_ms,event_id}],
	//            last_ts_unix_ms}, ...], count, channel? }
	Inbox = "inbox"
	// Send delivers an operator-initiated outbound message through a configured
	// channel (Telegram/Slack/Discord) — the manual egress complement to Pulse
	// briefs and agent replies, for scripts/CI ("deploy done → notify Slack").
	// Authenticated by the control plane (primary token), so no per-channel
	// allowlist gate. The channel's own Send journals channel.outbound.
	// Args: channel (kind, required), to (channel/chat id, required), text (required).
	// Returns: { sent: true, channel, to }
	Send = "send"
	// AutonomyFeed returns a curated, newest-first timeline of the daemon's
	// self-directed activity (schedules and standing orders firing, skill
	// lifecycle, completion checks, briefings), folded from the journal so the
	// Web UI can show the living organism acting on its own (M653). Read-only.
	// Args: limit (optional; default 60, clamped 1..200).
	// Returns: { items: [{seq, ts_unix_ms, kind, category, title, correlation_id,
	//            detail?}], count }
	AutonomyFeed = "autonomy_feed"
	// BoardRead surfaces the shared inter-agent message board (kernel/board,
	// M647) so the Web UI can show agents talking to each other. Read-only.
	// Args: topic (optional; case-insensitive exact filter); limit (optional;
	// default 50, clamped 1..500).
	// Returns: { messages: [{topic, from?, text, ts_unix_ms}], topics: {name:count},
	//            count }
	BoardRead = "board_read"
	// BoardHelp surfaces the still-open (unanswered) help requests agents have
	// raised on the board (M849), newest first — the mailbox's "who needs help"
	// view. Read-only. Args: limit (optional; default 50, clamped ..500).
	// Returns: { open_help: [{id, from?, to?, topic, text, ts_unix_ms}], count }
	BoardHelp = "board_help"
	// BoardSend leaves a message on the shared board from OUTSIDE a run (M937
	// mailbox): an SDK app or script posts to a topic, DMs an agent by name, or
	// broadcasts to every inbox — the external counterpart of the `board` tool's
	// post/send/broadcast/reply ops. The write goes through the daemon's shared
	// store instance (SetBoard) and publishes the same board.posted event, so a
	// standing order wakes exactly as if an agent had sent it.
	// Args: text (required); from (sender name, recommended so replies can find
	// you); to (recipient agent name, "*" for everyone, empty for a topic post);
	// topic (defaults to "dm" when addressed; required for a plain topic post);
	// reply_to (message id being answered — the reply goes back to the original's
	// sender on its topic, like the board tool's op=reply); help (bool — raise an
	// assistance request that stays open until answered); correlation_id (optional
	// wake/run correlation for SDK or channel bridges).
	// Returns: { sent: {id, topic, from?, to?, reply_to?, help?, text, ts_unix_ms},
	//            correlation_id? }
	BoardSend = "board_send"
	// BoardInbox lists what is waiting for a named agent/app on the board
	// (M937): messages addressed to it plus broadcasts it didn't send, newest
	// first; answered and acked messages are dropped unless all=true. Read-only.
	// Args: to (required — whose inbox); all (optional bool); limit (optional;
	// default 50, clamped 1..500).
	// Returns: { to, waiting: [msg…], count }
	BoardInbox = "board_inbox"
	// BoardAck marks a board message read for one reader (M937): it leaves
	// that reader's unanswered inbox without a reply being written. Per-reader
	// (a broadcast acked by one agent still waits for the others) and idempotent.
	// Args: id (required), by (required — the reader's name).
	// Returns: { acked: true, id, by }
	BoardAck = "board_ack"
	// BoardReplies returns the answers to a board message, oldest first
	// (conversation order) — what the asker reads back (M937). Read-only.
	// Args: id (required); limit (optional; default 50, clamped 1..500).
	// Returns: { id, replies: [msg…], count }
	BoardReplies = "board_replies"
	// BoardGet returns one board message by id (M938). The board.posted
	// event carries only metadata (no text), so a watcher that learned an id
	// from the event stream fetches the body here. Read-only.
	// Args: id (required).
	// Returns: { message: {id, topic, from?, to?, reply_to?, help?, text,
	//            ts_unix_ms} }
	BoardGet = "board_get"
)
