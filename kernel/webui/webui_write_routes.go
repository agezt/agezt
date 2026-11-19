// SPDX-License-Identifier: MIT

package webui

// Provenance: Mutating HTTP routes: writeRoutes (POST commands) + jsonRoutes (POSTs
//             that proxy with body args) + planRoute. Code extracted from webui.go
//             during the Day-51 god-file split. Public API unchanged.

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

var writeRoutes = map[string]writeRoute{
	"/api/halt":   {opapi.Halt, []string{"reason"}},
	"/api/resume": {opapi.Resume, []string{"reason"}},
	// Artifact collector (M845): reap stale artifacts. POST so the browser must opt
	// in; dry_run (default true) previews, dry_run=false deletes. Goes through the
	// jsonProxy? No — the args are simple scalars, so it's a query-arg write route.
	"/api/artifact/collect": {opapi.ArtifactCollect, []string{"older_than_days", "dry_run"}},
	// Personal Data Lake mutations (M836): delete a record. (Insert/update/create
	// carry structured bodies — they are jsonRoutes.) The drop-collection write
	// was never wired (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2); reachable via
	// opapi.DataDropCollection when an operator needs to drop a collection.
	"/api/data/delete": {opapi.DataDelete, []string{"collection", "id"}},
	// Pulse pause/resume (M743): the proactive-heartbeat master switch. No args.
	"/api/pulse/pause":  {opapi.PulsePause, nil},
	"/api/pulse/resume": {opapi.PulseResume, nil},
	// Trigger one on-demand heartbeat (M756): the operator's "think now". No args.
	"/api/pulse/beat": {opapi.PulseBeat, nil},
	// Change the heartbeat interval live (M757): seconds → clamped cadence. Runtime-only.
	"/api/pulse/cadence": {opapi.PulseCadence, []string{"seconds"}},
	// Change the proactivity dial live (M758): quiet|balanced|chatty. Runtime-only.
	"/api/pulse/dial": {opapi.PulseDial, []string{"dial"}},
	// Flush held digest items now (M761): deliver what the pulse is holding. No args.
	"/api/pulse/flush": {opapi.PulseFlush, nil},
	// Add a disk-space watch at runtime (M767): alert when free space on path < min_pct.
	"/api/pulse/watch": {opapi.PulseWatch, []string{"path", "min_pct"}},
	// Add a command-probe watch at runtime (M768): run command each beat, alert on flip.
	"/api/pulse/probe": {opapi.PulseProbe, []string{"name", "command"}},
	// Remove a runtime-added watch by observer name (M769): the inverse of watch/probe.
	"/api/pulse/unwatch": {opapi.PulseUnwatch, []string{"name"}},
	// Set the quiet-hours window live (M770): "hours" is "START-END" (e.g. "22-7"); empty disables.
	"/api/pulse/quiet": {opapi.PulseQuiet, []string{"hours"}},
	// Reload catalog + providers in place (M745): apply credential/catalog changes
	// without a daemon restart. No args.
	"/api/provider/reload": {opapi.ProviderReload, nil},
	// Send an outbound message via a configured channel (M747): channel + to + text.
	"/api/send":       {opapi.Send, []string{"channel", "to", "text"}},
	"/api/cancel_run": {opapi.CancelRun, []string{"correlation"}},
	// /api/budget_set went un-wired (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2);
	// reachable via opapi.BudgetSet when an operator needs to adjust the ceiling.
	"/api/run/pause":     {opapi.RunPause, []string{"correlation"}},
	"/api/run/resume":    {opapi.RunResume, []string{"correlation"}},
	"/api/run/step":      {opapi.RunStep, []string{"correlation"}},
	"/api/run/steer":     {opapi.RunSteer, []string{"correlation", "directive", "mode"}},
	"/api/decide":        {opapi.Decide, []string{"id", "decision", "reason"}},
	"/api/memory/forget": {opapi.MemoryForget, []string{"id"}},
	// Bulk soft-delete: the Memory view's multi-select "forget N records" path.
	// ids is a JSON array; idempotent (already-tombstoned ids count as forgotten).
	"/api/memory/bulk_forget": {opapi.MemoryBulkForget, []string{"ids"}},
	// Marketplace install/uninstall stream per-item progress, so they have their
	// own SSE proxies below (marketStreamProxy) rather than going through jsonProxy.
	// Remote marketplace sources (Phase 2): add/remove a source, then sync its
	// catalogue into the local cache. Write path; journaled market.*.
	"/api/market/source/add":    {opapi.MarketAddSource, []string{"url", "name", "pubkey"}},
	"/api/market/source/remove": {opapi.MarketRemoveSource, []string{"name"}},
	"/api/market/sync":          {opapi.MarketSync, []string{"name"}},
	// Promote a private (agent-scoped) record into shared memory (M915) —
	// the selective-sharing valve over per-agent memory.
	"/api/memory/promote": {opapi.MemoryPromote, []string{"id"}},
	// Delete a stored artifact by index id (M822); the blob is GC'd when unreferenced.
	"/api/artifact/delete": {opapi.ArtifactDelete, []string{"id"}},
	// One brain-distillation pass was the read/write pair previously exposed at
	// /api/memory/consolidate and /api/reflect/run (TOPOLOGY-AUDIT-DAY28-VERIFY.md
	// §2: no app caller). The controlplane commands remain registered for CLI use;
	// the Web UI proxies are removed.
	// Operator-profile rebuild (M1000): synthesize the operator profile from
	// accumulated memory. No args; mirrors the (removed) /api/memory/consolidate.
	"/api/profile/rebuild": {opapi.ProfileRebuild, nil},
	// Memory prune (M857): hard-remove soft-deleted records older than N days.
	// dry_run reports the prunable count first; the UI confirms before pruning.
	"/api/memory/prune": {opapi.MemoryPrune, []string{"older_than_days", "dry_run"}},
	// Memory retention hygiene: dry_run reports low-value active records;
	// dry_run=false soft-forgets them.
	"/api/memory/clean": {opapi.MemoryClean, []string{"dry_run"}},
	// Memory tidy (M994): collapse near-duplicate auto-distilled notes by subject.
	// dry_run reports how many would collapse; the UI confirms before tidying.
	"/api/memory/tidy":      {opapi.MemoryTidy, []string{"dry_run"}},
	"/api/world/forget":     {opapi.WorldForget, []string{"id"}},
	"/api/world/relate":     {opapi.WorldRelate, []string{"from", "verb", "to"}},
	"/api/sandbox/delete":   {opapi.SandboxDelete, []string{"project"}},
	"/api/edict/set_level":  {opapi.EdictSetLevel, []string{"capability", "level"}},
	"/api/edict/set_mode":   {opapi.EdictSetMode, []string{"mode"}},
	"/api/edict/deny_add":   {opapi.EdictDenyAdd, []string{"rule"}},
	"/api/edict/deny_rm":    {opapi.EdictDenyRemove, []string{"name"}},
	"/api/skill/promote":    {opapi.SkillPromote, []string{"id"}},
	"/api/skill/quarantine": {opapi.SkillQuarantine, []string{"id", "reason"}},
	"/api/skill/archive":    {opapi.SkillArchive, []string{"id", "reason"}},
	"/api/skill/revert":     {opapi.SkillRevert, []string{"id"}},
	"/api/skill/share":      {opapi.SkillShare, []string{"id"}},
	// /api/skill/reassign was never wired (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2);
	// reachable via opapi.SkillReassign when an operator needs to reassign a skill.
	"/api/schedule/remove": {opapi.ScheduleRemove, []string{"id"}},
	"/api/schedule/run":    {opapi.ScheduleRun, []string{"id"}},
	"/api/schedule/enable": {opapi.ScheduleEnable, []string{"id", "enabled"}},
	"/api/standing/enable": {opapi.StandingSetEnabled, []string{"id", "enabled"}},
	"/api/standing/remove": {opapi.StandingRemove, []string{"id"}},
	// Fire a standing order now (M765), ignoring its triggers — test or run on demand.
	"/api/standing/fire": {opapi.StandingFire, []string{"id"}},
	// Agent roster lifecycle (M783): pause/resume/remove a named agent (ref = id or slug).
	"/api/agents/enable": {opapi.AgentSetEnabled, []string{"ref", "enabled"}},
	// Agent graveyard (M846): retire to / revive from the graveyard. POST-only.
	"/api/agents/retire": {opapi.AgentRetire, []string{"ref", "reason"}},
	"/api/agents/revive": {opapi.AgentRevive, []string{"ref"}},
	// Agent wake: manual trigger for a roster agent. POST-only.
	"/api/agents/wake": {opapi.AgentWake, []string{"ref", "intent", "reason", "incident_id", "root_incident_id", "parent_incident_id"}},
	// MCP self-install lifecycle (M796): attach spawns the registered server
	// NOW (its tools go live for the next run); detach is the kill switch;
	// enable flips auto-attach at daemon start. ref = name or id.
	"/api/mcp/attach": {opapi.MCPAttach, []string{"ref"}},
	"/api/mcp/detach": {opapi.MCPDetach, []string{"ref"}},
	"/api/mcp/enable": {opapi.MCPSetEnabled, []string{"ref", "enabled"}},
	"/api/mcp/remove": {opapi.MCPRemove, []string{"ref"}},
	// Workflow lifecycle (M798): enable arms triggers (M799); remove deletes.
	// (Save and run carry structured bodies — they are jsonRoutes.)
	"/api/workflows/enable": {opapi.WorkflowSetEnabled, []string{"ref", "enabled"}},
	"/api/workflows/remove": {opapi.WorkflowRemove, []string{"ref"}},
	// /api/reflect/run was never wired (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2);
	// reachable via opapi.ReflectRun when an operator needs it.
	// Provider keyring switch/remove (M700): activate or remove a key, reloading
	// the provider in place. (Add is a jsonRoute — the value is a secret body.)
	"/api/provider/keys/activate": {opapi.ProviderKeyActivate, []string{"provider", "env", "label"}},
	"/api/provider/keys/remove":   {opapi.ProviderKeyRemove, []string{"provider", "env", "label"}},
	// Multi-account channel: remove a labelled account (deletes its stored fields).
	"/api/channel/account/remove": {opapi.ChannelAccountRemove, []string{"kind", "label"}},
}

// jsonRoutes are mutating commands invoked with a JSON request BODY rather than
// query-string args, so Flow Studio can submit values too large for a URL — a
// full plan JSON, a multi-line intent. Same allowlist discipline as
// writeRoutes: POST-only, body size-capped, and only the named keys are
// forwarded (an unexpected key in the body is dropped, never reaches the
// control plane). opapi.Plan is NOT here — it streams, so it has its own route
// (planRoute / planRunProxy) that drives Stream instead of Call.
var jsonRoutes = map[string]writeRoute{
	"/api/plan/generate": {opapi.PlanGenerate, []string{"intent", "model"}},
	"/api/plan/refine":   {opapi.PlanRefine, []string{"plan_json", "feedback", "model"}},
	// Config Center write (M693): set one setting (non-secret → config store,
	// secret → vault). POST-only; only name+value are forwarded.
	"/api/config/set": {opapi.ConfigSet, []string{"name", "value"}},
	// Rated agent Config Center writes: agent-readable key/value entries with
	// rating and optional allow/deny lists. Separate from daemon settings above.
	"/api/configcenter/set":    {opapi.ConfigCenterSet, []string{"key", "value", "rating", "description", "allowed_agents", "excluded_agents"}},
	"/api/configcenter/access": {opapi.ConfigCenterSetAccess, []string{"key", "allowed_agents", "excluded_agents"}},
	"/api/configcenter/delete": {opapi.ConfigCenterDelete, []string{"key"}},
	"/api/configcenter/rating": {opapi.ConfigCenterSetRating, []string{"key", "rating"}},
	// Schema registry write (M695): register/unregister a skill/plugin-contributed
	// schema section. Register forwards the whole `section` object; unregister an id.
	"/api/config/schema/register":   {opapi.ConfigSchemaRegister, []string{"section"}},
	"/api/config/schema/unregister": {opapi.ConfigSchemaUnregister, []string{"id", "force"}},
	// Models catalog sync (M699): pull models.dev/api.json server-side, save +
	// hot-reload the catalog. POST (it mutates + hits the network) with the longer
	// jsonProxy timeout; `url` optionally overrides the source. No body needed —
	// the Sync button posts {}.
	"/api/catalog/sync": {opapi.CatalogSync, []string{"url"}},
	// Provider keyring add (M700): the value is a secret, so it travels in the
	// POST body (not a query arg). provider+env+label+value(+active).
	"/api/provider/keys/add": {opapi.ProviderKeyAdd, []string{"provider", "env", "label", "value", "active"}},
	// Multi-account channel: set one field of an account instance. The value may be
	// a secret, so it travels in the POST body. kind+label(""=default)+name+value.
	"/api/channel/account/set": {opapi.ChannelAccountSet, []string{"kind", "label", "name", "value"}},
	// Channel OAuth connect (Phase 4): start a flow (client_secret is a secret →
	// POST body) returning {authorize_url,state}; poll its status. The browser
	// redirect lands on the public /oauth/callback handler (registered separately).
	"/api/channel/oauth/start":  {opapi.ChannelOAuthStart, []string{"kind", "label", "client_id", "client_secret", "redirect_uri", "instance_url"}},
	"/api/channel/oauth/status": {opapi.ChannelOAuthStatus, []string{"state"}},
	// "Sign in with ChatGPT" provider login: start the flow (1455 redirect
	// listener) → poll status; import a local Codex CLI login; or disconnect.
	"/api/provider/oauth/start":  {opapi.ProviderOAuthStart, []string{"provider"}},
	"/api/provider/oauth/status": {opapi.ProviderOAuthStatus, []string{"state"}},
	"/api/provider/oauth/import": {opapi.ProviderOAuthImport, []string{"path"}},
	"/api/provider/oauth/logout": {opapi.ProviderOAuthLogout, []string{}},
	// Register a provider + key. Catalog-aware: if `id` already exists in the
	// merged catalog (api+local+custom) the existing entry is preserved (no
	// custom.json write) — the orphan-with-one-model bug a naive upsert
	// caused is gone. For new ids, a minimal partial entry is written to
	// custom.json and the daemon is reloaded. JSON body (id, name, npm, api,
	// env, model); the key follows on keys/add.
	"/api/provider/connect": {opapi.ProviderConnect, []string{"id", "name", "npm", "api", "env", "model"}},
	// Provider reachability probe (key in body → jsonRoute): is the endpoint up?
	"/api/provider/probe": {opapi.ProviderProbe, []string{"url", "key"}},
	// WhatsApp gateway connection probe (key in body → jsonRoute): is the WAHA/
	// Evolution session logged in? Lets the Channels wizard show connected vs scan-QR.
	"/api/whatsappgw/status": {opapi.WhatsAppGatewayStatus, []string{"url", "backend", "session", "key"}},
	"/api/whatsappgw/qr":     {opapi.WhatsAppGatewayQR, []string{"url", "backend", "session", "key"}},
	// Per-task model routing (M703): replace the model chains. `chains` is an
	// object {task: [models]} too large/structured for a query arg.
	"/api/routing/set": {opapi.RoutingSet, []string{"chains"}},
	// Named reusable fallback chains (M963): replace the whole registry. `chains`
	// is an object {name: [models]} and `default` an optional chain name.
	"/api/chains/set":  {opapi.ChainsSet, []string{"chains", "default"}},
	"/api/persona/set": {opapi.PersonaSet, []string{"system"}},
	// Chat history compaction (M923): fold older turns into one briefing. The
	// turns array is far too large for a query string — JSON body only.
	"/api/chat/summarize": {opapi.ChatSummarize, []string{"turns", "model"}},
	// Personal Data Lake writes (M836): insert/update carry the record object —
	// JSON bodies, not query args. The create-collection write was never wired
	// (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2); reachable via opapi.DataCreateCollection.
	"/api/data/insert": {opapi.DataInsert, []string{"collection", "record"}},
	"/api/data/update": {opapi.DataUpdate, []string{"collection", "id", "record"}},
	// Council of Elders ask (M839): convene the panel on a question. Long-running
	// (several model calls) but bounded by the jsonProxy timeout. POST body.
	"/api/council/ask": {opapi.CouncilAsk, []string{"question", "rounds", "corr"}},
	// Council members edit (M839): replace the default council membership. members
	// is an array of {seat, model}. Applies live and persists to config store.
	"/api/council/set": {opapi.CouncilSet, []string{"members"}},
	// Conductor ask (M997): run the Thinker/Worker/Verifier loop on a task.
	// The conductor nav cluster was REMOVED in Day 28 (TOPOLOGY-AUDIT-DAY28-VERIFY.md
	// §2); reachable via opapi.ConductorAsk when an operator needs it.
	// Deep-research harness ask (M1001): decompose, gather web sources,
	// synthesize a cited answer, adversarially verify each claim. Long-running
	// (several searches/fetches + model calls) but bounded by the jsonProxy
	// timeout. POST body.
	"/api/research/ask": {opapi.ResearchAsk, []string{"question", "max_sub_questions", "max_sources", "verify", "max_verify_claims", "corr"}},
	"/api/prompts/set":  {opapi.PromptsSet, []string{"prompts"}},
	"/api/standing/add": {opapi.StandingAdd, []string{"order"}},
	// Edit a standing order in place (M729): id + any subset of the human-tunable
	// fields. assure is numeric, so the JSON body preserves its type.
	"/api/standing/edit": {opapi.StandingEdit, []string{"id", "name", "plan", "agent", "mode", "max_trust", "briefing_min", "assure", "cooldown_sec"}},
	// Agent roster create/edit (M783): the profile is a structured object (soul
	// text, fallback list, numeric cost ceiling) — a JSON body, not query args.
	"/api/agents/add":          {opapi.AgentAdd, []string{"profile"}},
	"/api/agents/edit":         {opapi.AgentEdit, []string{"ref", "profile"}},
	"/api/agents/capabilities": {opapi.AgentCapabilities, []string{"ref", "trust_ceiling", "tool_allow", "tool_deny", "noise_policy", "config_overrides", "memory_scope", "workdir", "max_cost_mc", "max_daily_mc"}},
	"/api/agents/remove":       {opapi.AgentRemove, []string{"ref", "cascade"}},
	"/api/agents/task":         {opapi.AgentTaskUpdate, []string{"ref", "op", "id", "task", "title", "description", "scope", "status"}},
	"/api/agents/repair":       {opapi.AgentRepair, []string{"ref", "reason", "incident_id", "root_incident_id", "parent_incident_id"}},
	"/api/agents/resolve":      {opapi.AgentResolve, []string{"ref", "resolution", "summary", "delegate_to", "task_type", "task_model_chain", "incident_id", "root_incident_id", "parent_incident_id"}},
	// Inter-agent mailbox writes (M937): message text and optional payload-shaped
	// fields ride in the body; this is the operator/app path into the same board
	// agents use with the board tool.
	"/api/board/send": {opapi.BoardSend, []string{"from", "to", "topic", "reply_to", "text", "help"}},
	"/api/board/ack":  {opapi.BoardAck, []string{"id", "by"}},
	// OKR / Workboard / Taste / Seats / Toolforge cluster was REMOVED in Day 28
	// (TOPOLOGY-AUDIT-DAY28-VERIFY.md §2); the underlying commands remain
	// registered for CLI use and are exercised by kernel/controlplane/
	// toolforge_test.go.
	// Register an MCP server (M796): the server is a structured object
	// (command + args list) — a JSON body, not query args.
	"/api/mcp/add": {opapi.MCPAdd, []string{"server"}},
	// Workflow save/run (M798): the graph (nodes+edges) and the run payload
	// are structured objects — JSON bodies, not query args.
	"/api/workflows/save": {opapi.WorkflowSave, []string{"workflow"}},
	"/api/workflows/run":  {opapi.WorkflowRun, []string{"ref", "payload", "async"}},
	// Copilot draft (M802): description in, validated UNSAVED graph out —
	// the canvas reviews and saves explicitly.
	"/api/workflows/draft": {opapi.WorkflowDraft, []string{"description", "name"}},
	// Copilot refine (M805): the current canvas graph + a change request in,
	// the revised UNSAVED graph out.
	"/api/workflows/refine": {opapi.WorkflowRefine, []string{"workflow", "instruction", "ref"}},
	// Single-node test (M811): the canvas graph + a node id + mock upstream
	// data in, the node's real output out.
	"/api/workflows/test_node": {opapi.WorkflowTestNode, []string{"workflow", "node", "data", "payload"}},
	// Create a schedule (M715): intent + a timing mode. Numeric timing args (e.g.
	// interval_sec, at_minutes, once_at_unix) ride the JSON body so they keep their
	// types — a query arg would stringify them.
	"/api/schedule/add": {opapi.ScheduleAdd, []string{"intent", "model", "agent", "target", "workflow", "system_task", "tool", "payload", "interval_sec", "at_minutes", "days", "tz", "once_at_unix", "window_start", "window_end"}},
	// Edit an existing schedule (M728): id + any subset of intent/model and at most
	// one cadence change. Numeric timing args ride the JSON body to keep their types.
	"/api/schedule/edit": {opapi.ScheduleEdit, []string{"id", "intent", "model", "agent", "target", "workflow", "system_task", "tool", "payload", "interval_sec", "at_minutes", "days", "tz", "once_at_unix", "window_start", "window_end"}},
	// Teach the agent a fact (M718): content + optional subject/type/confidence.
	// confidence is numeric, so the JSON body preserves its type.
	"/api/memory/add": {opapi.MemoryAdd, []string{"content", "subject", "type", "confidence", "evidence", "half_life_ms"}},
	// Revise a fact (M731): supersede old_id with a new record (content required;
	// confidence numeric, so the JSON body preserves its type).
	"/api/memory/supersede": {opapi.MemorySupersede, []string{"old_id", "content", "subject", "type", "confidence", "evidence", "half_life_ms"}},
	// Add a world-model entity (M721): name + kind (+ optional aliases/attrs, which
	// are arrays/objects, so the JSON body is needed).
	"/api/world/add": {opapi.WorldAdd, []string{"name", "kind", "aliases", "attrs"}},
	// Edit a world-model entity's aliases/attrs in place (M730): id + the full
	// editable state (arrays/objects, so the JSON body is needed).
	"/api/world/edit": {opapi.WorldEdit, []string{"id", "aliases", "attrs"}},
	// Author a skill from the UI (M736): name + body (required) + optional
	// description/triggers/tools_required. triggers/tools_required are arrays, so the
	// JSON body is needed. Lands as a draft (auto-staged to shadow if well-formed) —
	// never auto-active; promote via the normal lifecycle controls.
	// agent (M933) optionally scopes the authored skill to one roster agent.
	"/api/skill/import": {opapi.SkillImport, []string{"name", "description", "triggers", "body", "tools_required", "agent"}},
	// Dry-run the secret redactor (M754): does the LIVE scrubber redact this text,
	// and into which categories? Read-only, but carried in the JSON BODY (not a query
	// arg) so the sensitive probe text never lands in a URL / access log. The response
	// returns only the REDACTED form + category names, never the matched secret.
	"/api/redact/test": {opapi.RedactTest, []string{"text"}},
}

// planRoute is the streaming "run this plan" action (Flow Studio's Run button).
// It forwards only plan_json from the JSON body and drives opapi.Plan to its
// terminal result via Stream (see planRunProxy / Caller).
var planRoute = writeRoute{opapi.Plan, []string{"plan_json"}}

// jsonBodyMax caps a Flow Studio request body. A generated plan is a few KiB;
// 1 MiB is far above any legitimate plan or intent and bounds memory per call.
const jsonBodyMax = 1 << 20

// planRunTimeout bounds an in-UI plan run. Plans can legitimately take minutes
// (each loop node is a full agent run), so this is generous — far longer than
// the 5s read-panel timeout. The browser sees progress live on the SSE feed
// regardless; this only bounds how long the connection is held open.
const planRunTimeout = 30 * time.Minute

// Handler builds the route registry. The shared router owns declarative auth
// tiers and request-body caps; the WebUI keeps its request-aware credential
// policy (token, password session, and the constrained EventSource exception).
// Security headers plus Host/Origin checks wrap the entire registry so they
// also cover public routes and authentication failures.
