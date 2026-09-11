// SPDX-License-Identifier: MIT

// Mutating HTTP routes: writeRoutes (POST commands) + jsonRoutes (POSTs that proxy with body args) + planRoute.
// Code extracted from webui.go during the Day-51 god-file split. Public API unchanged.
package webui


import (
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
)


var writeRoutes = map[string]writeRoute{
	"/api/halt":   {controlplane.CmdHalt, []string{"reason"}},
	"/api/resume": {controlplane.CmdResume, []string{"reason"}},
	// Artifact collector (M845): reap stale artifacts. POST so the browser must opt
	// in; dry_run (default true) previews, dry_run=false deletes. Goes through the
	// jsonProxy? No — the args are simple scalars, so it's a query-arg write route.
	"/api/artifact/collect": {controlplane.CmdArtifactCollect, []string{"older_than_days", "dry_run"}},
	// Personal Data Lake mutations (M836): delete a record / drop a user collection.
	// (Insert/update/create carry structured bodies — they are jsonRoutes.)
	"/api/data/delete": {controlplane.CmdDataDelete, []string{"collection", "id"}},
	"/api/data/drop":   {controlplane.CmdDataDropCollection, []string{"name"}},
	// Pulse pause/resume (M743): the proactive-heartbeat master switch. No args.
	"/api/pulse/pause":  {controlplane.CmdPulsePause, nil},
	"/api/pulse/resume": {controlplane.CmdPulseResume, nil},
	// Resolve a pending ask (M1001): approve (re-emit onto pulse.initiative.act) or
	// reject one of the actionable observations the heartbeat raised under ask-mode.
	"/api/pulse/asks/resolve": {controlplane.CmdPulseAskResolve, []string{"issue_key", "approve"}},
	// Trigger one on-demand heartbeat (M756): the operator's "think now". No args.
	"/api/pulse/beat": {controlplane.CmdPulseBeat, nil},
	// Change the heartbeat interval live (M757): seconds → clamped cadence. Runtime-only.
	"/api/pulse/cadence": {controlplane.CmdPulseCadence, []string{"seconds"}},
	// Change the proactivity dial live (M758): quiet|balanced|chatty. Runtime-only.
	"/api/pulse/dial": {controlplane.CmdPulseDial, []string{"dial"}},
	// Flush held digest items now (M761): deliver what the pulse is holding. No args.
	"/api/pulse/flush": {controlplane.CmdPulseFlush, nil},
	// Add a disk-space watch at runtime (M767): alert when free space on path < min_pct.
	"/api/pulse/watch": {controlplane.CmdPulseWatch, []string{"path", "min_pct"}},
	// Add a command-probe watch at runtime (M768): run command each beat, alert on flip.
	"/api/pulse/probe": {controlplane.CmdPulseProbe, []string{"name", "command"}},
	// Remove a runtime-added watch by observer name (M769): the inverse of watch/probe.
	"/api/pulse/unwatch": {controlplane.CmdPulseUnwatch, []string{"name"}},
	// Set the quiet-hours window live (M770): "hours" is "START-END" (e.g. "22-7"); empty disables.
	"/api/pulse/quiet": {controlplane.CmdPulseQuiet, []string{"hours"}},
	// Reload catalog + providers in place (M745): apply credential/catalog changes
	// without a daemon restart. No args.
	"/api/provider/reload": {controlplane.CmdProviderReload, nil},
	// Send an outbound message via a configured channel (M747): channel + to + text.
	"/api/send":          {controlplane.CmdSend, []string{"channel", "to", "text"}},
	"/api/cancel_run":    {controlplane.CmdCancelRun, []string{"correlation"}},
	"/api/budget_set":    {controlplane.CmdBudgetSet, []string{"ceiling_mc"}},
	"/api/run/pause":     {controlplane.CmdRunPause, []string{"correlation"}},
	"/api/run/resume":    {controlplane.CmdRunResume, []string{"correlation"}},
	"/api/run/step":      {controlplane.CmdRunStep, []string{"correlation"}},
	"/api/run/steer":     {controlplane.CmdRunSteer, []string{"correlation", "directive", "mode"}},
	"/api/decide":        {controlplane.CmdDecide, []string{"id", "decision", "reason"}},
	"/api/memory/forget": {controlplane.CmdMemoryForget, []string{"id"}},
	// Bulk soft-delete: the Memory view's multi-select "forget N records" path.
	// ids is a JSON array; idempotent (already-tombstoned ids count as forgotten).
	"/api/memory/bulk_forget": {controlplane.CmdMemoryBulkForget, []string{"ids"}},
	// Marketplace install/uninstall stream per-item progress, so they have their
	// own SSE proxies below (marketStreamProxy) rather than going through jsonProxy.
	// Remote marketplace sources (Phase 2): add/remove a source, then sync its
	// catalogue into the local cache. Write path; journaled market.*.
	"/api/market/source/add":    {controlplane.CmdMarketAddSource, []string{"url", "name", "pubkey"}},
	"/api/market/source/remove": {controlplane.CmdMarketRemoveSource, []string{"name"}},
	"/api/market/sync":          {controlplane.CmdMarketSync, []string{"name"}},
	// Promote a private (agent-scoped) record into shared memory (M915) —
	// the selective-sharing valve over per-agent memory.
	"/api/memory/promote": {controlplane.CmdMemoryPromote, []string{"id"}},
	// Delete a stored artifact by index id (M822); the blob is GC'd when unreferenced.
	"/api/artifact/delete": {controlplane.CmdArtifactDelete, []string{"id"}},
	// One brain-distillation pass (M804): merge related records, supersede
	// the originals. No args; mirrors /api/reflect/run.
	"/api/memory/consolidate": {controlplane.CmdMemoryConsolidate, nil},
	// Operator-profile rebuild (M1000): synthesize the operator profile from
	// accumulated memory. No args; mirrors /api/memory/consolidate.
	"/api/profile/rebuild": {controlplane.CmdProfileRebuild, nil},
	// Memory prune (M857): hard-remove soft-deleted records older than N days.
	// dry_run reports the prunable count first; the UI confirms before pruning.
	"/api/memory/prune": {controlplane.CmdMemoryPrune, []string{"older_than_days", "dry_run"}},
	// Memory retention hygiene: dry_run reports low-value active records;
	// dry_run=false soft-forgets them.
	"/api/memory/clean": {controlplane.CmdMemoryClean, []string{"dry_run"}},
	// Memory tidy (M994): collapse near-duplicate auto-distilled notes by subject.
	// dry_run reports how many would collapse; the UI confirms before tidying.
	"/api/memory/tidy":      {controlplane.CmdMemoryTidy, []string{"dry_run"}},
	"/api/world/forget":     {controlplane.CmdWorldForget, []string{"id"}},
	"/api/world/relate":     {controlplane.CmdWorldRelate, []string{"from", "verb", "to"}},
	"/api/sandbox/delete":   {controlplane.CmdSandboxDelete, []string{"project"}},
	"/api/edict/set_level":  {controlplane.CmdEdictSetLevel, []string{"capability", "level"}},
	"/api/edict/set_mode":   {controlplane.CmdEdictSetMode, []string{"mode"}},
	"/api/edict/deny_add":   {controlplane.CmdEdictDenyAdd, []string{"rule"}},
	"/api/edict/deny_rm":    {controlplane.CmdEdictDenyRemove, []string{"name"}},
	"/api/skill/promote":    {controlplane.CmdSkillPromote, []string{"id"}},
	"/api/skill/quarantine": {controlplane.CmdSkillQuarantine, []string{"id", "reason"}},
	"/api/skill/archive":    {controlplane.CmdSkillArchive, []string{"id", "reason"}},
	"/api/skill/revert":     {controlplane.CmdSkillRevert, []string{"id"}},
	"/api/skill/share":      {controlplane.CmdSkillShare, []string{"id"}},
	"/api/skill/reassign":   {controlplane.CmdSkillReassign, []string{"id", "agent"}},
	"/api/schedule/remove":  {controlplane.CmdScheduleRemove, []string{"id"}},
	"/api/schedule/run":     {controlplane.CmdScheduleRun, []string{"id"}},
	"/api/schedule/enable":  {controlplane.CmdScheduleEnable, []string{"id", "enabled"}},
	"/api/standing/enable":  {controlplane.CmdStandingSetEnabled, []string{"id", "enabled"}},
	"/api/standing/remove":  {controlplane.CmdStandingRemove, []string{"id"}},
	// Fire a standing order now (M765), ignoring its triggers — test or run on demand.
	"/api/standing/fire": {controlplane.CmdStandingFire, []string{"id"}},
	// Agent roster lifecycle (M783): pause/resume/remove a named agent (ref = id or slug).
	"/api/agents/enable": {controlplane.CmdAgentSetEnabled, []string{"ref", "enabled"}},
	// Agent graveyard (M846): retire to / revive from the graveyard. POST-only.
	"/api/agents/retire": {controlplane.CmdAgentRetire, []string{"ref", "reason"}},
	"/api/agents/revive": {controlplane.CmdAgentRevive, []string{"ref"}},
	// Agent wake: manual trigger for a roster agent. POST-only.
	"/api/agents/wake": {controlplane.CmdAgentWake, []string{"ref", "intent", "reason", "incident_id", "root_incident_id", "parent_incident_id"}},
	// Script-tool forge lifecycle (M794): test runs the code in the sandbox and
	// records the verdict; promote/quarantine move a TESTED tool in/out of
	// production; remove deletes it. ref = id or name.
	"/api/toolforge/test":       {controlplane.CmdToolforgeTest, []string{"ref", "input"}},
	"/api/toolforge/promote":    {controlplane.CmdToolforgePromote, []string{"ref"}},
	"/api/toolforge/quarantine": {controlplane.CmdToolforgeQuarantine, []string{"ref", "reason"}},
	"/api/toolforge/remove":     {controlplane.CmdToolforgeRemove, []string{"ref"}},
	// MCP self-install lifecycle (M796): attach spawns the registered server
	// NOW (its tools go live for the next run); detach is the kill switch;
	// enable flips auto-attach at daemon start. ref = name or id.
	"/api/mcp/attach": {controlplane.CmdMCPAttach, []string{"ref"}},
	"/api/mcp/detach": {controlplane.CmdMCPDetach, []string{"ref"}},
	"/api/mcp/enable": {controlplane.CmdMCPSetEnabled, []string{"ref", "enabled"}},
	"/api/mcp/remove": {controlplane.CmdMCPRemove, []string{"ref"}},
	// Workflow lifecycle (M798): enable arms triggers (M799); remove deletes.
	// (Save and run carry structured bodies — they are jsonRoutes.)
	"/api/workflows/enable": {controlplane.CmdWorkflowSetEnabled, []string{"ref", "enabled"}},
	"/api/workflows/remove": {controlplane.CmdWorkflowRemove, []string{"ref"}},
	"/api/reflect/run":      {controlplane.CmdReflectRun, nil},
	// Provider keyring switch/remove (M700): activate or remove a key, reloading
	// the provider in place. (Add is a jsonRoute — the value is a secret body.)
	"/api/provider/keys/activate": {controlplane.CmdProviderKeyActivate, []string{"provider", "env", "label"}},
	"/api/provider/keys/remove":   {controlplane.CmdProviderKeyRemove, []string{"provider", "env", "label"}},
	// Multi-account channel: remove a labelled account (deletes its stored fields).
	"/api/channel/account/remove": {controlplane.CmdChannelAccountRemove, []string{"kind", "label"}},
}

// jsonRoutes are mutating commands invoked with a JSON request BODY rather than
// query-string args, so Flow Studio can submit values too large for a URL — a
// full plan JSON, a multi-line intent. Same allowlist discipline as
// writeRoutes: POST-only, body size-capped, and only the named keys are
// forwarded (an unexpected key in the body is dropped, never reaches the
// control plane). CmdPlan is NOT here — it streams, so it has its own route
// (planRoute / planRunProxy) that drives Stream instead of Call.
var jsonRoutes = map[string]writeRoute{
	"/api/plan/generate": {controlplane.CmdPlanGenerate, []string{"intent", "model"}},
	"/api/plan/refine":   {controlplane.CmdPlanRefine, []string{"plan_json", "feedback", "model"}},
	// Config Center write (M693): set one setting (non-secret → config store,
	// secret → vault). POST-only; only name+value are forwarded.
	"/api/config/set": {controlplane.CmdConfigSet, []string{"name", "value"}},
	// Rated agent Config Center writes: agent-readable key/value entries with
	// rating and optional allow/deny lists. Separate from daemon settings above.
	"/api/configcenter/set":    {controlplane.CmdConfigCenterSet, []string{"key", "value", "rating", "description", "allowed_agents", "excluded_agents"}},
	"/api/configcenter/access": {controlplane.CmdConfigCenterSetAccess, []string{"key", "allowed_agents", "excluded_agents"}},
	"/api/configcenter/delete": {controlplane.CmdConfigCenterDelete, []string{"key"}},
	"/api/configcenter/rating": {controlplane.CmdConfigCenterSetRating, []string{"key", "rating"}},
	// Schema registry write (M695): register/unregister a skill/plugin-contributed
	// schema section. Register forwards the whole `section` object; unregister an id.
	"/api/config/schema/register":   {controlplane.CmdConfigSchemaRegister, []string{"section"}},
	"/api/config/schema/unregister": {controlplane.CmdConfigSchemaUnregister, []string{"id", "force"}},
	// Models catalog sync (M699): pull models.dev/api.json server-side, save +
	// hot-reload the catalog. POST (it mutates + hits the network) with the longer
	// jsonProxy timeout; `url` optionally overrides the source. No body needed —
	// the Sync button posts {}.
	"/api/catalog/sync": {controlplane.CmdCatalogSync, []string{"url"}},
	// Provider keyring add (M700): the value is a secret, so it travels in the
	// POST body (not a query arg). provider+env+label+value(+active).
	"/api/provider/keys/add": {controlplane.CmdProviderKeyAdd, []string{"provider", "env", "label", "value", "active"}},
	// Multi-account channel: set one field of an account instance. The value may be
	// a secret, so it travels in the POST body. kind+label(""=default)+name+value.
	"/api/channel/account/set": {controlplane.CmdChannelAccountSet, []string{"kind", "label", "name", "value"}},
	// Channel OAuth connect (Phase 4): start a flow (client_secret is a secret →
	// POST body) returning {authorize_url,state}; poll its status. The browser
	// redirect lands on the public /oauth/callback handler (registered separately).
	"/api/channel/oauth/start":  {controlplane.CmdChannelOAuthStart, []string{"kind", "label", "client_id", "client_secret", "redirect_uri", "instance_url"}},
	"/api/channel/oauth/status": {controlplane.CmdChannelOAuthStatus, []string{"state"}},
	// "Sign in with ChatGPT" provider login: start the flow (1455 redirect
	// listener) → poll status; import a local Codex CLI login; or disconnect.
	"/api/provider/oauth/start":  {controlplane.CmdProviderOAuthStart, []string{"provider"}},
	"/api/provider/oauth/status": {controlplane.CmdProviderOAuthStatus, []string{"state"}},
	"/api/provider/oauth/import": {controlplane.CmdProviderOAuthImport, []string{"path"}},
	"/api/provider/oauth/logout": {controlplane.CmdProviderOAuthLogout, []string{}},
	// Register a provider + key. Catalog-aware: if `id` already exists in the
	// merged catalog (api+local+custom) the existing entry is preserved (no
	// custom.json write) — the orphan-with-one-model bug a naive upsert
	// caused is gone. For new ids, a minimal partial entry is written to
	// custom.json and the daemon is reloaded. JSON body (id, name, npm, api,
	// env, model); the key follows on keys/add.
	"/api/provider/connect": {controlplane.CmdProviderConnect, []string{"id", "name", "npm", "api", "env", "model"}},
	// Provider reachability probe (key in body → jsonRoute): is the endpoint up?
	"/api/provider/probe": {controlplane.CmdProviderProbe, []string{"url", "key"}},
	// WhatsApp gateway connection probe (key in body → jsonRoute): is the WAHA/
	// Evolution session logged in? Lets the Channels wizard show connected vs scan-QR.
	"/api/whatsappgw/status": {controlplane.CmdWhatsAppGatewayStatus, []string{"url", "backend", "session", "key"}},
	"/api/whatsappgw/qr":     {controlplane.CmdWhatsAppGatewayQR, []string{"url", "backend", "session", "key"}},
	// Per-task model routing (M703): replace the model chains. `chains` is an
	// object {task: [models]} too large/structured for a query arg.
	"/api/routing/set": {controlplane.CmdRoutingSet, []string{"chains"}},
	// Named reusable fallback chains (M963): replace the whole registry. `chains`
	// is an object {name: [models]} and `default` an optional chain name.
	"/api/chains/set":  {controlplane.CmdChainsSet, []string{"chains", "default"}},
	"/api/persona/set": {controlplane.CmdPersonaSet, []string{"system"}},
	// Chat history compaction (M923): fold older turns into one briefing. The
	// turns array is far too large for a query string — JSON body only.
	"/api/chat/summarize": {controlplane.CmdChatSummarize, []string{"turns", "model"}},
	// Personal Data Lake writes (M836): insert/update carry the record object;
	// create carries the full collection schema — JSON bodies, not query args.
	"/api/data/insert":     {controlplane.CmdDataInsert, []string{"collection", "record"}},
	"/api/data/update":     {controlplane.CmdDataUpdate, []string{"collection", "id", "record"}},
	"/api/data/collection": {controlplane.CmdDataCreateCollection, []string{"collection"}},
	// Council of Elders ask (M839): convene the panel on a question. Long-running
	// (several model calls) but bounded by the jsonProxy timeout. POST body.
	"/api/council/ask": {controlplane.CmdCouncilAsk, []string{"question", "rounds", "corr"}},
	// Council members edit (M839): replace the default council membership. members
	// is an array of {seat, model}. Applies live and persists to config store.
	"/api/council/set": {controlplane.CmdCouncilSet, []string{"members"}},
	// Conductor ask (M997): run the Thinker/Worker/Verifier loop on a task.
	// Long-running (several model calls + possibly a sandbox run) but bounded by
	// the jsonProxy timeout. POST body.
	"/api/conductor/ask": {controlplane.CmdConductorAsk, []string{"task", "thinker", "worker", "verifier", "max_rounds", "plan", "corr"}},
	// Deep-research harness ask (M1001): decompose, gather web sources,
	// synthesize a cited answer, adversarially verify each claim. Long-running
	// (several searches/fetches + model calls) but bounded by the jsonProxy
	// timeout. POST body.
	"/api/research/ask": {controlplane.CmdResearchAsk, []string{"question", "max_sub_questions", "max_sources", "verify", "max_verify_claims", "corr"}},
	"/api/prompts/set":  {controlplane.CmdPromptsSet, []string{"prompts"}},
	"/api/standing/add": {controlplane.CmdStandingAdd, []string{"order"}},
	// Edit a standing order in place (M729): id + any subset of the human-tunable
	// fields. assure is numeric, so the JSON body preserves its type.
	"/api/standing/edit": {controlplane.CmdStandingEdit, []string{"id", "name", "plan", "agent", "mode", "max_trust", "briefing_min", "assure", "cooldown_sec"}},
	// Agent roster create/edit (M783): the profile is a structured object (soul
	// text, fallback list, numeric cost ceiling) — a JSON body, not query args.
	"/api/agents/add":          {controlplane.CmdAgentAdd, []string{"profile"}},
	"/api/agents/edit":         {controlplane.CmdAgentEdit, []string{"ref", "profile"}},
	"/api/agents/capabilities": {controlplane.CmdAgentCapabilities, []string{"ref", "trust_ceiling", "tool_allow", "tool_deny", "noise_policy", "config_overrides", "memory_scope", "workdir", "max_cost_mc", "max_daily_mc"}},
	"/api/agents/remove":       {controlplane.CmdAgentRemove, []string{"ref", "cascade"}},
	"/api/agents/task":         {controlplane.CmdAgentTaskUpdate, []string{"ref", "op", "id", "task", "title", "description", "scope", "status"}},
	"/api/agents/repair":       {controlplane.CmdAgentRepair, []string{"ref", "reason", "incident_id", "root_incident_id", "parent_incident_id"}},
	"/api/agents/resolve":      {controlplane.CmdAgentResolve, []string{"ref", "resolution", "summary", "delegate_to", "task_type", "task_model_chain", "incident_id", "root_incident_id", "parent_incident_id"}},
	// Inter-agent mailbox writes (M937): message text and optional payload-shaped
	// fields ride in the body; this is the operator/app path into the same board
	// agents use with the board tool.
	"/api/board/send": {controlplane.CmdBoardSend, []string{"from", "to", "topic", "reply_to", "text", "help"}},
	"/api/board/ack":  {controlplane.CmdBoardAck, []string{"id", "by"}},
	// Workboard operator actions: the dedicated task-detail UI uses body-shaped
	// calls so long comments/reasons/intents never ride in query strings.
	"/api/workboard/create":   {controlplane.CmdWorkboardCreate, []string{"title", "description", "assignee", "priority", "criteria", "seat"}},
	"/api/workboard/comment":  {controlplane.CmdWorkboardComment, []string{"id", "author", "body"}},
	"/api/workboard/block":    {controlplane.CmdWorkboardBlock, []string{"id", "actor", "reason"}},
	"/api/workboard/fail":     {controlplane.CmdWorkboardFail, []string{"id", "actor", "reason"}},
	"/api/workboard/unblock":  {controlplane.CmdWorkboardUnblock, []string{"id", "actor"}},
	"/api/workboard/complete": {controlplane.CmdWorkboardComplete, []string{"id", "actor"}},
	"/api/workboard/prove":    {controlplane.CmdWorkboardProve, []string{"id", "actor", "answer"}},
	"/api/workboard/seat":     {controlplane.CmdWorkboardSeat, []string{"id", "seat"}},
	"/api/workboard/policy":   {controlplane.CmdWorkboardPolicy, []string{"id", "actor", "max_attempts", "escalate_to", "clear"}},
	// OKR spine (Phase 2): operator + agent actions on objectives.
	"/api/okr/create":    {controlplane.CmdOKRCreate, []string{"title", "description", "owner", "tenant"}},
	"/api/okr/keyresult": {controlplane.CmdOKRKeyResult, []string{"id", "title", "target"}},
	"/api/okr/link":      {controlplane.CmdOKRLink, []string{"id", "key_result", "task"}},
	"/api/okr/unlink":    {controlplane.CmdOKRUnlink, []string{"id", "key_result", "task"}},
	"/api/okr/archive":   {controlplane.CmdOKRArchive, []string{"id"}},
	// Taste overlay (Phase 3): curate exemplars from the console.
	"/api/taste/create":       {controlplane.CmdTasteCreate, []string{"title", "body", "scope", "tags"}},
	"/api/taste/delete":       {controlplane.CmdTasteDelete, []string{"id"}},
	"/api/seats/create":       {controlplane.CmdSeatCreate, []string{"id", "name", "description", "execution_profile", "model_chain", "tools", "restrict_tools"}},
	"/api/seats/delete":       {controlplane.CmdSeatDelete, []string{"id"}},
	"/api/workboard/dispatch": {controlplane.CmdWorkboardDispatch, []string{"id", "agent", "intent", "reason"}},
	// Script-tool forge draft/edit (M794): the tool is a structured object
	// (code body, schema text) — a JSON body, not query args.
	"/api/toolforge/draft": {controlplane.CmdToolforgeDraft, []string{"tool"}},
	"/api/toolforge/edit":  {controlplane.CmdToolforgeEdit, []string{"ref", "tool"}},
	// Register an MCP server (M796): the server is a structured object
	// (command + args list) — a JSON body, not query args.
	"/api/mcp/add": {controlplane.CmdMCPAdd, []string{"server"}},
	// Workflow save/run (M798): the graph (nodes+edges) and the run payload
	// are structured objects — JSON bodies, not query args.
	"/api/workflows/save": {controlplane.CmdWorkflowSave, []string{"workflow"}},
	"/api/workflows/run":  {controlplane.CmdWorkflowRun, []string{"ref", "payload", "async"}},
	// Copilot draft (M802): description in, validated UNSAVED graph out —
	// the canvas reviews and saves explicitly.
	"/api/workflows/draft": {controlplane.CmdWorkflowDraft, []string{"description", "name"}},
	// Copilot refine (M805): the current canvas graph + a change request in,
	// the revised UNSAVED graph out.
	"/api/workflows/refine": {controlplane.CmdWorkflowRefine, []string{"workflow", "instruction", "ref"}},
	// Single-node test (M811): the canvas graph + a node id + mock upstream
	// data in, the node's real output out.
	"/api/workflows/test_node": {controlplane.CmdWorkflowTestNode, []string{"workflow", "node", "data", "payload"}},
	// Create a schedule (M715): intent + a timing mode. Numeric timing args (e.g.
	// interval_sec, at_minutes, once_at_unix) ride the JSON body so they keep their
	// types — a query arg would stringify them.
	"/api/schedule/add": {controlplane.CmdScheduleAdd, []string{"intent", "model", "agent", "target", "workflow", "system_task", "tool", "payload", "interval_sec", "at_minutes", "days", "tz", "once_at_unix", "window_start", "window_end"}},
	// Edit an existing schedule (M728): id + any subset of intent/model and at most
	// one cadence change. Numeric timing args ride the JSON body to keep their types.
	"/api/schedule/edit": {controlplane.CmdScheduleEdit, []string{"id", "intent", "model", "agent", "target", "workflow", "system_task", "tool", "payload", "interval_sec", "at_minutes", "days", "tz", "once_at_unix", "window_start", "window_end"}},
	// Teach the agent a fact (M718): content + optional subject/type/confidence.
	// confidence is numeric, so the JSON body preserves its type.
	"/api/memory/add": {controlplane.CmdMemoryAdd, []string{"content", "subject", "type", "confidence", "evidence", "half_life_ms"}},
	// Revise a fact (M731): supersede old_id with a new record (content required;
	// confidence numeric, so the JSON body preserves its type).
	"/api/memory/supersede": {controlplane.CmdMemorySupersede, []string{"old_id", "content", "subject", "type", "confidence", "evidence", "half_life_ms"}},
	// Add a world-model entity (M721): name + kind (+ optional aliases/attrs, which
	// are arrays/objects, so the JSON body is needed).
	"/api/world/add": {controlplane.CmdWorldAdd, []string{"name", "kind", "aliases", "attrs"}},
	// Edit a world-model entity's aliases/attrs in place (M730): id + the full
	// editable state (arrays/objects, so the JSON body is needed).
	"/api/world/edit": {controlplane.CmdWorldEdit, []string{"id", "aliases", "attrs"}},
	// Author a skill from the UI (M736): name + body (required) + optional
	// description/triggers/tools_required. triggers/tools_required are arrays, so the
	// JSON body is needed. Lands as a draft (auto-staged to shadow if well-formed) —
	// never auto-active; promote via the normal lifecycle controls.
	// agent (M933) optionally scopes the authored skill to one roster agent.
	"/api/skill/import": {controlplane.CmdSkillImport, []string{"name", "description", "triggers", "body", "tools_required", "agent"}},
	// Dry-run the secret redactor (M754): does the LIVE scrubber redact this text,
	// and into which categories? Read-only, but carried in the JSON BODY (not a query
	// arg) so the sensitive probe text never lands in a URL / access log. The response
	// returns only the REDACTED form + category names, never the matched secret.
	"/api/redact/test": {controlplane.CmdRedactTest, []string{"text"}},
}

// planRoute is the streaming "run this plan" action (Flow Studio's Run button).
// It forwards only plan_json from the JSON body and drives CmdPlan to its
// terminal result via Stream (see planRunProxy / Caller).
var planRoute = writeRoute{controlplane.CmdPlan, []string{"plan_json"}}

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