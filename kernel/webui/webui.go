// SPDX-License-Identifier: MIT

// Package webui serves the Agezt Web UI (SPEC-07, decision A4): a React 19 +
// Vite single-page app, built to static assets and go:embed-ded into the daemon
// (see embed.go) — one binary, no Node at runtime, and no Go dependency added.
// The Go side here is the thin server: it serves the embedded bundle and proxies
// the same control-plane commands + bus stream the SPA renders. Faithful to §0
// ("one event truth, many views; the UI never holds authoritative state, it
// subscribes and renders") and §5.2 ("Live Monitor driven entirely by events").
//
// It holds no state. Three data paths, all reusing what already exists:
//   - the SPA is the embedded Vite bundle (index.html + hashed /assets/*);
//   - the live event feed subscribes to the kernel bus (the same ">" stream the
//     daemon tees to stdout) over SSE at /events;
//   - every read panel proxies a control-plane command through the same Client
//     `agt` uses, so the CLI and the Web UI are guaranteed-consistent views and
//     no query logic is duplicated.
//
// Security (SPEC-06): the server is bound by the operator (loopback by
// default) and token-authed on every request. Reads are GET; the few
// mutating actions (halt, resume, approve/deny) are POST-only and pass the
// same token — a cross-site page can't forge them because it can't read the
// token, and the surface is loopback. The write set is a fixed allowlist
// (writeRoutes); there is no generic passthrough.
package webui

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/convo"
	"github.com/agezt/agezt/kernel/event"
)

// Caller is the API the dashboard proxies to — satisfied by
// *controlplane.Client. An interface keeps webui testable without a live
// daemon (a fake Caller + an in-memory bus is enough).
//
// Call handles single request→response commands (every read panel, the
// query-arg writes). Stream handles a command that emits a sequence of
// RespEvent frames before its terminal result — currently only CmdPlan, used
// by Flow Studio's "Run". The streamed events are discarded here (the browser
// already sees them live on the SSE /events firehose); Stream is driven to its
// terminal result only so the control-plane connection stays open for the
// plan's whole duration — dropping it early would cancel the run's context.
type Caller interface {
	Call(ctx context.Context, cmd string, args map[string]any) (map[string]any, error)
	Stream(ctx context.Context, cmd string, args map[string]any, onEvent func(*event.Event)) (map[string]any, error)
}

// Transcriber turns uploaded audio into text — the speech-to-text backend behind
// the chat mic button. Satisfied by *stt.Client (the same one the OpenAI-API
// surface uses). Optional: when nil, /api/transcribe reports "not configured".
type Transcriber interface {
	Transcribe(ctx context.Context, filename string, audio []byte) (string, error)
}

// Server is the Web UI HTTP surface.
type Server struct {
	bus         *bus.Bus
	client      Caller
	token       string
	dist        fs.FS       // the built SPA bundle (embed dist/, sub-rooted)
	transcriber Transcriber // optional STT backend for /api/transcribe (nil = not configured)
}

// SetTranscriber wires the speech-to-text backend for POST /api/transcribe
// (the chat mic button). Without it, that route reports "not configured" so the
// UI can degrade gracefully. Called once at startup, before Handler().
func (s *Server) SetTranscriber(t Transcriber) { s.transcriber = t }

// New builds a Server. token gates every request; bus drives the live feed;
// client proxies read commands. The embedded SPA bundle is sub-rooted so the
// FileServer serves dist/ contents at the URL root.
func New(b *bus.Bus, client Caller, token string) *Server {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// dist is embedded at compile time; a failure here is a build defect, not
		// a runtime condition. Fall back to the raw FS so the server still starts.
		sub = distFS
	}
	return &Server{bus: b, client: client, token: token, dist: sub}
}

// apiRoutes maps each GET /api path to the read-only control-plane command it
// proxies. Read-only by construction: there is no path here that mutates.
var apiRoutes = map[string]string{
	"/api/status":        controlplane.CmdStatus,
	"/api/config":        controlplane.CmdConfig,
	"/api/runs":          controlplane.CmdRunsList,
	"/api/stats":         controlplane.CmdRunsStats,
	"/api/budget":        controlplane.CmdBudget,
	"/api/cache":         controlplane.CmdCacheStats,
	"/api/providers":     controlplane.CmdProviderStats,
	"/api/catalog":       controlplane.CmdCatalogList,
	"/api/tools":         controlplane.CmdToolStats,
	"/api/tools_catalog": controlplane.CmdToolList,
	"/api/policy":        controlplane.CmdEdictStats,
	"/api/edict_show":    controlplane.CmdEdictShow,
	"/api/schedules":     controlplane.CmdScheduleList,
	"/api/memory":        controlplane.CmdMemoryList,
	"/api/world":         controlplane.CmdWorldList,
	"/api/skills":        controlplane.CmdSkillList,
	"/api/standing":      controlplane.CmdStandingList,
	"/api/agents":        controlplane.CmdAgentList,
	"/api/toolforge":     controlplane.CmdToolforgeList,
	"/api/mcp":           controlplane.CmdMCPList,
	"/api/inbox":         controlplane.CmdInbox,
	"/api/board":         controlplane.CmdBoardRead,
	"/api/autonomy":      controlplane.CmdAutonomyFeed,
	"/api/reflect":       controlplane.CmdReflectShow,
	"/api/approvals":     controlplane.CmdApprovals,
	"/api/plan_stats":    controlplane.CmdPlanStats,
	"/api/sandbox":       controlplane.CmdSandboxList,
	"/api/config/schema": controlplane.CmdConfigSchema,
	"/api/config/values": controlplane.CmdConfigValues,
	// Per-task model routing (M703): the effective chains + known task types.
	"/api/routing": controlplane.CmdRoutingGet,
	"/api/persona": controlplane.CmdPersonaGet,
	"/api/prompts": controlplane.CmdPromptsGet,
	// Pulse — the proactive heartbeat status (running/paused/beats/cadence) (M743).
	"/api/pulse": controlplane.CmdPulseStatus,
	// Journal integrity (M759): verify the tamper-evident hash chain. Returns
	// { ok: true } when intact, or errors describing the break. Read-only.
	"/api/journal/verify": controlplane.CmdJournalVerify,
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
	// Export an integrity-attested journal bundle for archival/compliance (M772):
	// every event with its hash + the chain head, re-verifiable offline. Read-only.
	"/api/journal/export": {controlplane.CmdJournalExport, []string{"since_ms"}},
	// Historical journal search (M618): the full CmdJournalGrep filter set —
	// free-text pattern plus kind/subject/actor/correlation — over all history,
	// powering the Search view. Read-only, like every readArgsRoute.
	"/api/journal_search": {controlplane.CmdJournalGrep, []string{"pattern", "kind", "subject", "actor", "correlation_id", "limit"}},
	"/api/provider_log":   {controlplane.CmdProviderLog, []string{"limit", "fallbacks"}},
	"/api/tool_log":       {controlplane.CmdToolLog, []string{"limit", "tool", "errors"}},
	// Read one sandbox project file's content (M686), path-confined server-side.
	"/api/sandbox_file": {controlplane.CmdSandboxFile, []string{"project", "file"}},
	"/api/policy_log":   {controlplane.CmdEdictLog, []string{"limit", "denied"}},
	// Resolved HITL approval history (M773): a timeline of past approval requests
	// joined with their granted/denied/timeout outcome. Read-only.
	"/api/approvals_log": {controlplane.CmdApprovalsLog, []string{"limit", "denied"}},
	"/api/plan_history":  {controlplane.CmdPlanHistory, []string{"limit", "status"}},
	// Provider keyring list (M700): labels + active + last-4 for one env var.
	// Read-only — values never leave the daemon.
	"/api/provider/keys": {controlplane.CmdProviderKeyList, []string{"env"}},
	// Forecast a schedule's next fire times (M744): id + how many. Read-only preview.
	"/api/schedule/test": {controlplane.CmdScheduleTest, []string{"id", "count"}},
	// A standing order's life story (M746): every standing.* journal event for it —
	// created, paused/resumed, each firing, removed. Read-only provenance.
	"/api/standing/why": {controlplane.CmdStandingWhy, []string{"id"}},
	// One script tool's full record incl. the code body (M795) — the list route
	// deliberately strips code; the Forge view's editor fetches it here. Read-only.
	"/api/toolforge/show": {controlplane.CmdToolforgeShow, []string{"ref"}},
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
var writeRoutes = map[string]writeRoute{
	"/api/halt":   {controlplane.CmdHalt, []string{"reason"}},
	"/api/resume": {controlplane.CmdResume, []string{"reason"}},
	// Pulse pause/resume (M743): the proactive-heartbeat master switch. No args.
	"/api/pulse/pause":  {controlplane.CmdPulsePause, nil},
	"/api/pulse/resume": {controlplane.CmdPulseResume, nil},
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
	"/api/send":             {controlplane.CmdSend, []string{"channel", "to", "text"}},
	"/api/cancel_run":       {controlplane.CmdCancelRun, []string{"correlation"}},
	"/api/budget_set":       {controlplane.CmdBudgetSet, []string{"ceiling_mc"}},
	"/api/run/pause":        {controlplane.CmdRunPause, []string{"correlation"}},
	"/api/run/resume":       {controlplane.CmdRunResume, []string{"correlation"}},
	"/api/run/step":         {controlplane.CmdRunStep, []string{"correlation"}},
	"/api/run/steer":        {controlplane.CmdRunSteer, []string{"correlation", "directive"}},
	"/api/decide":           {controlplane.CmdDecide, []string{"id", "decision", "reason"}},
	"/api/memory/forget":    {controlplane.CmdMemoryForget, []string{"id"}},
	"/api/world/forget":     {controlplane.CmdWorldForget, []string{"id"}},
	"/api/world/relate":     {controlplane.CmdWorldRelate, []string{"from", "verb", "to"}},
	"/api/sandbox/delete":   {controlplane.CmdSandboxDelete, []string{"project"}},
	"/api/edict/set_level":  {controlplane.CmdEdictSetLevel, []string{"capability", "level"}},
	"/api/edict/set_mode":   {controlplane.CmdEdictSetMode, []string{"mode"}},
	"/api/edict/deny_add":   {controlplane.CmdEdictDenyAdd, []string{"rule"}},
	"/api/edict/deny_rm":    {controlplane.CmdEdictDenyRemove, []string{"name"}},
	"/api/skill/promote":    {controlplane.CmdSkillPromote, []string{"id"}},
	"/api/skill/quarantine": {controlplane.CmdSkillQuarantine, []string{"id", "reason"}},
	"/api/skill/revert":     {controlplane.CmdSkillRevert, []string{"id"}},
	"/api/schedule/remove":  {controlplane.CmdScheduleRemove, []string{"id"}},
	"/api/schedule/run":     {controlplane.CmdScheduleRun, []string{"id"}},
	"/api/schedule/enable":  {controlplane.CmdScheduleEnable, []string{"id", "enabled"}},
	"/api/standing/enable":  {controlplane.CmdStandingSetEnabled, []string{"id", "enabled"}},
	"/api/standing/remove":  {controlplane.CmdStandingRemove, []string{"id"}},
	// Fire a standing order now (M765), ignoring its triggers — test or run on demand.
	"/api/standing/fire": {controlplane.CmdStandingFire, []string{"id"}},
	// Agent roster lifecycle (M783): pause/resume/remove a named agent (ref = id or slug).
	"/api/agents/enable": {controlplane.CmdAgentSetEnabled, []string{"ref", "enabled"}},
	"/api/agents/remove": {controlplane.CmdAgentRemove, []string{"ref"}},
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
	"/api/mcp/attach":  {controlplane.CmdMCPAttach, []string{"ref"}},
	"/api/mcp/detach":  {controlplane.CmdMCPDetach, []string{"ref"}},
	"/api/mcp/enable":  {controlplane.CmdMCPSetEnabled, []string{"ref", "enabled"}},
	"/api/mcp/remove":  {controlplane.CmdMCPRemove, []string{"ref"}},
	"/api/reflect/run": {controlplane.CmdReflectRun, nil},
	// Provider keyring switch/remove (M700): activate or remove a key, reloading
	// the provider in place. (Add is a jsonRoute — the value is a secret body.)
	"/api/provider/keys/activate": {controlplane.CmdProviderKeyActivate, []string{"env", "label"}},
	"/api/provider/keys/remove":   {controlplane.CmdProviderKeyRemove, []string{"env", "label"}},
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
	// POST body (not a query arg). env+label+value(+active).
	"/api/provider/keys/add": {controlplane.CmdProviderKeyAdd, []string{"env", "label", "value", "active"}},
	// Per-task model routing (M703): replace the model chains. `chains` is an
	// object {task: [models]} too large/structured for a query arg.
	"/api/routing/set":  {controlplane.CmdRoutingSet, []string{"chains"}},
	"/api/persona/set":  {controlplane.CmdPersonaSet, []string{"system"}},
	"/api/prompts/set":  {controlplane.CmdPromptsSet, []string{"prompts"}},
	"/api/standing/add": {controlplane.CmdStandingAdd, []string{"order"}},
	// Edit a standing order in place (M729): id + any subset of the human-tunable
	// fields. assure is numeric, so the JSON body preserves its type.
	"/api/standing/edit": {controlplane.CmdStandingEdit, []string{"id", "name", "plan", "agent", "mode", "max_trust", "briefing_min", "assure"}},
	// Agent roster create/edit (M783): the profile is a structured object (soul
	// text, fallback list, numeric cost ceiling) — a JSON body, not query args.
	"/api/agents/add":  {controlplane.CmdAgentAdd, []string{"profile"}},
	"/api/agents/edit": {controlplane.CmdAgentEdit, []string{"ref", "profile"}},
	// Script-tool forge draft/edit (M794): the tool is a structured object
	// (code body, schema text) — a JSON body, not query args.
	"/api/toolforge/draft": {controlplane.CmdToolforgeDraft, []string{"tool"}},
	"/api/toolforge/edit":  {controlplane.CmdToolforgeEdit, []string{"ref", "tool"}},
	// Register an MCP server (M796): the server is a structured object
	// (command + args list) — a JSON body, not query args.
	"/api/mcp/add": {controlplane.CmdMCPAdd, []string{"server"}},
	// Create a schedule (M715): intent + a timing mode. Numeric timing args (e.g.
	// interval_sec, at_minutes, once_at_unix) ride the JSON body so they keep their
	// types — a query arg would stringify them.
	"/api/schedule/add": {controlplane.CmdScheduleAdd, []string{"intent", "model", "interval_sec", "at_minutes", "days", "tz", "once_at_unix", "window_start", "window_end"}},
	// Edit an existing schedule (M728): id + any subset of intent/model and at most
	// one cadence change. Numeric timing args ride the JSON body to keep their types.
	"/api/schedule/edit": {controlplane.CmdScheduleEdit, []string{"id", "intent", "model", "interval_sec", "at_minutes", "days", "tz", "once_at_unix", "window_start", "window_end"}},
	// Teach the agent a fact (M718): content + optional subject/type/confidence.
	// confidence is numeric, so the JSON body preserves its type.
	"/api/memory/add": {controlplane.CmdMemoryAdd, []string{"content", "subject", "type", "confidence"}},
	// Revise a fact (M731): supersede old_id with a new record (content required;
	// confidence numeric, so the JSON body preserves its type).
	"/api/memory/supersede": {controlplane.CmdMemorySupersede, []string{"old_id", "content", "subject", "type", "confidence"}},
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
	"/api/skill/import": {controlplane.CmdSkillImport, []string{"name", "description", "triggers", "body", "tools_required"}},
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

// Handler builds the mux. Every route is wrapped in token auth.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.auth(s.handleSPA))
	// The hashed bundle and favicon are PUBLIC: the browser loads them as
	// subresources of index.html and cannot attach the ?token= (only fetch /
	// EventSource, which the SPA controls, can). They carry no secrets (compiled
	// UI code). The data surfaces — /events and every /api/* — stay token-gated,
	// so an unauthenticated visitor gets the shell but no data.
	mux.HandleFunc("/assets/", s.secure(s.handleAssets()))
	mux.HandleFunc("/favicon.ico", s.secure(handleFavicon))
	mux.HandleFunc("/events", s.auth(s.handleEvents))
	for path, cmd := range apiRoutes {
		mux.HandleFunc(path, s.auth(s.proxy(cmd)))
	}
	for path, rr := range readArgsRoutes {
		mux.HandleFunc(path, s.auth(s.readArgsProxy(rr)))
	}
	for path, wr := range writeRoutes {
		mux.HandleFunc(path, s.auth(s.writeProxy(wr)))
	}
	for path, jr := range jsonRoutes {
		mux.HandleFunc(path, s.auth(s.jsonProxy(jr)))
	}
	mux.HandleFunc("/api/plan/run", s.auth(s.planRunProxy()))
	mux.HandleFunc("/api/run", s.auth(s.runStreamProxy()))
	mux.HandleFunc("/api/transcribe", s.auth(s.handleTranscribe))
	return mux
}

// runStreamProxy is the Chat view's send button: it runs a free-text intent
// through the governed loop (controlplane.CmdRun) and streams the agent's events
// — llm tokens, tool calls, the final answer — straight to the browser as SSE, so
// the conversation renders live (like any chat UI). Unlike planRunProxy (which
// relays only a terminal result, leaning on the /events firehose), here each event
// IS the chat payload, so it's forwarded inline.
func (s *Server) runStreamProxy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		args, ok := s.decodeAllowedBody(w, r, []string{"intent", "model", "history", "system", "agent"})
		if !ok {
			return
		}
		intent, _ := args["intent"].(string)
		if strings.TrimSpace(intent) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "intent is required"})
			return
		}
		// Multi-turn continuity (M591): the Chat view sends the prior turns as
		// `history`; fold them with this turn into one transcript intent — the
		// same convo mapping the OpenAI-compatible API uses — so the governed loop
		// (single-intent by design) sees the whole conversation. `history` is
		// dropped from the args the control plane receives; CmdRun only ever sees
		// the resolved intent.
		turns := historyTurns(args["history"])
		delete(args, "history")
		if len(turns) > 0 {
			turns = append(turns, convo.Turn{Role: "user", Text: intent})
			args["intent"] = convo.TranscriptIntent(turns)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		write := func(obj any) {
			b, _ := json.Marshal(obj)
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(b)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		}
		write(map[string]any{"kind": "open"})

		ctx, cancel := context.WithTimeout(r.Context(), planRunTimeout)
		defer cancel()
		res, err := s.client.Stream(ctx, controlplane.CmdRun, args, func(ev *event.Event) {
			write(map[string]any{
				"kind":           string(ev.Kind),
				"subject":        ev.Subject,
				"payload":        ev.Payload,
				"correlation_id": ev.CorrelationID,
			})
		})
		if err != nil {
			write(map[string]any{"kind": "error", "error": err.Error()})
			return
		}
		write(map[string]any{"kind": "done", "result": res})
	}
}

// maxHistoryTurns bounds how many prior turns the Chat view can fold into one
// run's intent, so a long thread can't grow the intent without limit. The most
// recent turns are kept (the tail carries the live context).
const maxHistoryTurns = 40

// historyTurns parses the optional `history` body field — a JSON array of
// {role, text} objects (as decoded into []any of map[string]any) — into convo
// turns, skipping malformed/blank entries and keeping only the most recent
// maxHistoryTurns. Returns nil when there is no usable history (the single-shot
// path, unchanged).
func historyTurns(raw any) []convo.Turn {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	turns := make([]convo.Turn, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := m["role"].(string)
		text, _ := m["text"].(string)
		if strings.TrimSpace(role) == "" || strings.TrimSpace(text) == "" {
			continue
		}
		turns = append(turns, convo.Turn{Role: role, Text: text})
	}
	if len(turns) > maxHistoryTurns {
		turns = turns[len(turns)-maxHistoryTurns:]
	}
	return turns
}

// secure applies the defensive response headers (CSP et al.) to a handler but
// does NOT require a token. Used for public, secret-free static subresources
// (the bundle + favicon) that the browser must be able to load without a token.
func (s *Server) secure(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		next(w, r)
	}
}

// auth wraps a handler with security headers + token checking. The browser
// passes the token in the query string (EventSource can't set headers); API
// callers may use either the query or an Authorization: Bearer header.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	})
}

// setSecurityHeaders applies defensive response headers to every web UI route
// (set before the auth check so even 401s carry them). This is a control surface:
//   - X-Frame-Options DENY — the dashboard has state-mutating controls
//     (approve/halt/resume/decide), so framing is denied to block clickjacking.
//   - Referrer-Policy no-referrer — the page URL carries the auth token in
//     `?token=`, so the referrer is suppressed to keep it out of any Referer header.
//   - X-Content-Type-Options nosniff — stop content-type sniffing/confusion.
//   - Content-Security-Policy (static): the SPA loads only external, same-origin
//     hashed JS/CSS, so `script-src 'self'` admits the genuine bundle and refuses
//     any inline/injected script — STRICTER than the old per-nonce scheme (which
//     existed to allow one inline block). `style-src 'self' 'unsafe-inline'` is
//     required because React Flow / Radix inject runtime inline styles (measured
//     transforms) that can't be hashed at build time; 'unsafe-inline' on
//     style-src enables no code execution. `connect-src 'self'` confines fetch +
//     EventSource to the daemon; the rest closes framing/exfil/pivot avenues.
func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy",
		"default-src 'none'; "+
			"script-src 'self'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"connect-src 'self'; "+
			"img-src 'self' data:; "+
			"font-src 'self' data:; "+
			"base-uri 'none'; "+
			"form-action 'none'; "+
			"frame-ancestors 'none'")
}

func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return false // never serve without a configured token
	}
	if s.tokenMatch(r.URL.Query().Get("token")) {
		return true
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return s.tokenMatch(strings.TrimPrefix(h, "Bearer "))
	}
	return false
}

// tokenMatch compares a presented token against the configured one in CONSTANT
// TIME, so an attacker who can reach the web UI can't recover the token
// byte-by-byte by timing the auth check. Mirrors the control-plane's
// subtle.ConstantTimeCompare gate (server.go). Caller guarantees s.token != "".
func (s *Server) tokenMatch(presented string) bool {
	return subtle.ConstantTimeCompare([]byte(presented), []byte(s.token)) == 1
}

// handleSPA serves the embedded React single-page app. The hashed JS/CSS live
// under /assets/ (see handleAssets); this serves index.html at "/" and for any
// other non-API, non-asset path (client-side deep links like /runs), so a
// refresh on a sub-view re-loads the app rather than 404-ing. index.html is
// served no-cache so a daemon upgrade (new asset hashes) is picked up
// immediately rather than showing a stale shell that points at gone assets.
func (s *Server) handleSPA(w http.ResponseWriter, _ *http.Request) {
	body, err := fs.ReadFile(s.dist, "index.html")
	if err != nil {
		http.Error(w, "web ui bundle missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}

// handleAssets serves the content-hashed bundle under /assets/ straight from the
// embedded FS. Hashes make each file immutable, so it gets a long immutable
// cache; a missing asset 404s rather than falling through to the SPA shell. The
// Content-Type is set EXPLICITLY by extension rather than via the stdlib's
// mime.TypeByExtension, which on Windows reads the registry and can return
// text/plain for .css (browsers then refuse the stylesheet under nosniff).
func (s *Server) handleAssets() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		f, err := s.dist.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType(name))
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		if rs, ok := f.(io.ReadSeeker); ok {
			http.ServeContent(w, r, name, st.ModTime(), rs)
			return
		}
		_, _ = io.Copy(w, f)
	}
}

// handleFavicon serves the SPA's icon from the bundle if present, else 204 (so a
// browser's automatic /favicon.ico probe doesn't 404/401-noise the console).
func handleFavicon(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// contentType maps a bundle filename to a stable MIME type, independent of the
// host OS mime registry (see handleAssets).
func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".js"), strings.HasSuffix(name, ".mjs"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".json"), strings.HasSuffix(name, ".map"):
		return "application/json"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(name, ".woff"):
		return "font/woff"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

// handleEvents streams the bus as Server-Sent Events. It subscribes to the
// whole firehose and relays each event as one `data: {json}` frame, flushing
// per event, until the client disconnects (request ctx) or the bus closes.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	sub, err := s.bus.Subscribe(">", 256)
	if err != nil {
		http.Error(w, "subscribe: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer sub.Cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	// An initial comment opens the stream so the browser's EventSource fires
	// onopen even before the first event.
	_, _ = w.Write([]byte(": connected\n\n"))
	flusher.Flush()

	ctx := r.Context()
	// A heartbeat keeps proxies from closing an idle stream.
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if _, err := w.Write([]byte("data: " + string(payload) + "\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// proxy returns a handler that runs one read-only control-plane command and
// relays its JSON result verbatim.
func (s *Server) proxy(cmd string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		res, err := s.client.Call(ctx, cmd, nil)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// readArgsProxy returns a GET handler for one allowlisted READ command that
// takes query arguments. It forwards only the route's allowlisted args (so the
// browser cannot pass arbitrary parameters) and relays the JSON result. Unlike
// writeProxy it permits GET, because the command is read-only.
func (s *Server) readArgsProxy(rr writeRoute) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args := map[string]any{}
		for _, k := range rr.args {
			if v := strings.TrimSpace(r.URL.Query().Get(k)); v != "" {
				args[k] = v
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		res, err := s.client.Call(ctx, rr.cmd, args)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// writeProxy returns a handler for one allowlisted mutating command. It is
// POST-only (a GET — e.g. a prefetch or an <img> — must never halt the agent),
// copies the route's allowed args from the query string, and relays the result.
func (s *Server) writeProxy(wr writeRoute) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
			return
		}
		args := map[string]any{}
		for _, k := range wr.args {
			if v := strings.TrimSpace(r.URL.Query().Get(k)); v != "" {
				args[k] = v
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		res, err := s.client.Call(ctx, wr.cmd, args)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// decodeAllowedBody reads a JSON object from a POST body (size-capped) and
// returns only the route's allowlisted keys. It enforces POST and writes the
// error response itself; ok=false means the caller should return immediately.
// An unexpected key in the body is silently dropped — the control plane only
// ever sees the named arguments, mirroring writeProxy's query-arg allowlist.
func (s *Server) decodeAllowedBody(w http.ResponseWriter, r *http.Request, allowed []string) (map[string]any, bool) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return nil, false
	}
	var body map[string]any
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, jsonBodyMax))
	if err := dec.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body: " + err.Error()})
		return nil, false
	}
	args := map[string]any{}
	for _, k := range allowed {
		if v, ok := body[k]; ok {
			args[k] = v
		}
	}
	return args, true
}

// jsonProxy returns a handler for one allowlisted mutating command whose
// arguments arrive as a JSON object in the request BODY. Unlike writeProxy
// (query-string args), this supports large values — a full plan JSON, a
// multi-line intent. Used by Flow Studio's Generate/Refine. The timeout is
// generous because these call the LLM.
func (s *Server) jsonProxy(jr writeRoute) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args, ok := s.decodeAllowedBody(w, r, jr.args)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()
		res, err := s.client.Call(ctx, jr.cmd, args)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// planRunProxy returns the handler for Flow Studio's Run button. CmdPlan
// streams RespEvent frames before its terminal result, so it cannot go through
// Call (which reads a single response); it is driven with Stream. The streamed
// events are discarded — the browser already receives plan.*/node.* live on the
// SSE /events firehose — but Stream must run to completion so the control-plane
// connection stays open for the run's whole duration (closing it early cancels
// the run's context, killing the plan mid-flight). The terminal result
// (plan_id + node_outputs) is relayed when the plan finishes.
func (s *Server) planRunProxy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args, ok := s.decodeAllowedBody(w, r, planRoute.args)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), planRunTimeout)
		defer cancel()
		res, err := s.client.Stream(ctx, planRoute.cmd, args, func(*event.Event) {})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	// API payloads are live daemon state — never let a browser serve a stale
	// cached body after a mutation (no Cache-Control otherwise invites heuristic
	// caching, e.g. /api/routing showing an old chain after a save+reload).
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
