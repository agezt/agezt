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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/httpserver"
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

// Synthesizer turns text into spoken audio — the text-to-speech backend behind
// the console Voice Mode's /api/tts route. Satisfied by *voice.Adapter (the same
// OpenAI-compatible TTS half agents use to speak replies). Optional: when nil,
// /api/tts reports "not configured" so the browser falls back to its built-in
// SpeechSynthesis voice.
type Synthesizer interface {
	Speak(ctx context.Context, text string) (audio []byte, mime string, err error)
}

// Server is the Web UI HTTP surface.
type Server struct {
	bus         *bus.Bus
	client      Caller
	token       string // main console / API bearer token
	sseToken    string // ephemeral SSE-only token for /events EventSource URL
	tokenAuth   kernelauth.Verifier
	sseAuth     kernelauth.Verifier
	dist        fs.FS       // the built SPA bundle (embed dist/, sub-rooted)
	transcriber Transcriber // optional STT backend for /api/transcribe (nil = not configured)
	synthesizer Synthesizer // optional TTS backend for /api/tts (nil = not configured)
	// passwordFn is the LIVE password source (M933), so a Setup/Config-Center
	// edit applies without a restart.
	passwordFn func() string
	// passwordStrict restores M817 compose (token AND session) instead of the
	// M933 alternative-door default (token OR session).
	passwordStrict bool
	sessions       *sessionStore
	allowedHosts   map[string]bool
	hostPolicyMu   sync.RWMutex
	// allowQueryTokensForData preserves legacy package tests that still pass
	// ?token= to /api/*; production data routes use Bearer/session, except
	// /events where EventSource cannot set Authorization.
	allowQueryTokensForData bool

	// hookRL throttles the token-free POST /hooks/ webhook (VULN-007). The path is
	// authenticated only by a per-workflow secret; without a frequency cap, anyone
	// who learns one secret could loop it and launch unbounded governed (paid) runs
	// up to the soft daily budget. Buckets are keyed by "workflow|source-IP" so one
	// abusive source on one workflow is throttled without starving distinct legit
	// senders. Lazily initialised; idle buckets are evicted.
	hookRLMu   sync.Mutex
	hookRL     map[string]*hookBucket
	hookRLOnce sync.Once
}

// SetTranscriber wires the speech-to-text backend for POST /api/transcribe
// (the chat mic button). Without it, that route reports "not configured" so the
// UI can degrade gracefully. Called once at startup, before Handler().
func (s *Server) SetTranscriber(t Transcriber) { s.transcriber = t }

// SetSynthesizer wires the text-to-speech backend for POST /api/tts (the console
// Voice Mode's spoken replies). Without it, that route reports "not configured"
// so the browser degrades to its built-in voice. Called once at startup, before
// Handler().
func (s *Server) SetSynthesizer(t Synthesizer) { s.synthesizer = t }

// SetAllowedHosts permits additional non-IP Host header values for deployments
// behind a domain or tunnel. Loopback names and IP literals are accepted by
// default; explicit hosts are for DNS names such as a reverse-proxy hostname.
//
// When ANY non-loopback host is registered (i.e. the console is reachable
// beyond localhost), password-strict mode auto-activates so that a guessed
// password alone is insufficient — the bearer token is also required (token
// AND session). The operator can override via SetPasswordStrict(false) if
// two-factor is not desired (VULN token-or-password-mode).
func (s *Server) SetAllowedHosts(hosts ...string) {
	if len(hosts) == 0 {
		return
	}
	s.hostPolicyMu.Lock()
	defer s.hostPolicyMu.Unlock()
	if s.allowedHosts == nil {
		s.allowedHosts = map[string]bool{}
	}
	for _, h := range hosts {
		if host := hostName(h); host != "" {
			s.allowedHosts[strings.ToLower(host)] = true
			// Any single explicit (non-loopback) host trips strict mode:
			// the console is reachable beyond localhost, so a guessed
			// password alone must not open data routes (VULN token-or-
			// password-mode). The operator can override via
			// SetPasswordStrict(false).
			if !strings.EqualFold(host, "localhost") {
				if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
					s.passwordStrict = true
				}
			}
		}
	}
}

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
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	sseToken := hex.EncodeToString(buf)
	return &Server{
		bus:       b,
		client:    client,
		token:     token,
		sseToken:  sseToken,
		tokenAuth: kernelauth.NewStaticVerifier(token),
		sseAuth:   kernelauth.NewStaticVerifier(sseToken),
		dist:      sub,
		sessions:  newSessionStore(),
	}
}

// apiRoutes maps each GET /api path to the read-only control-plane command it
// proxies. Read-only by construction: there is no path here that mutates.
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
func (s *Server) routeRegistry() *httpserver.Router {
	router := httpserver.NewRouter(httpserver.Authenticator{
		RequestAuthorize: func(r *http.Request, _ kernelauth.Tier) bool {
			return s.authorized(r)
		},
	}, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	publicRead := httpserver.RouteOpts{Tier: kernelauth.TierPublic, Method: http.MethodGet}
	publicMutation := httpserver.RouteOpts{Tier: kernelauth.TierPublic, Method: http.MethodPost, Mutation: true}
	protectedRead := httpserver.RouteOpts{Tier: kernelauth.TierUser, Method: http.MethodGet}
	proxyRead := protectedRead
	proxyRead.Timeout = 5 * time.Second
	protectedMutation := httpserver.RouteOpts{
		Tier:     kernelauth.TierUser,
		Method:   http.MethodPost,
		Timeout:  5 * time.Second,
		Mutation: true,
	}
	jsonProtected := protectedMutation
	jsonProtected.BodyMax = jsonBodyMax
	jsonProtected.Timeout = 120 * time.Second

	// The SPA shell loads with the token OR — when a console password is
	// configured (M933) — with no credential at all, so a token-less visitor
	// gets the password login screen instead of a bare 401. The shell carries
	// no data; every data route stays protected by route policy.
	router.Handle("/", publicRead, s.shellAuth(s.handleSPA))
	// Auth surface (M817/M933): probe + login/logout are token-FREE — they are
	// how a token-less browser gets in. authmeta leaks only "is there a
	// password gate" (no data); login is bounded by the failed-attempt lockout.
	router.Handle("/api/authmeta", publicRead, s.handleAuthMeta)
	loginPublic := publicMutation
	loginPublic.BodyMax = loginBodyLimit
	router.Handle("/api/login", loginPublic, s.handleLogin)
	router.Handle("/api/logout", publicMutation, s.handleLogout)
	// The hashed bundle and favicon are PUBLIC: the browser loads them as
	// subresources of index.html and cannot attach the ?token= (only fetch /
	// EventSource, which the SPA controls, can). They carry no secrets (compiled
	// UI code). The data surfaces — /events and every /api/* — stay token-gated,
	// so an unauthenticated visitor gets the shell but no data.
	router.Handle("/assets/", publicRead, s.handleAssets())
	router.Handle("/favicon.ico", publicRead, handleFavicon)
	router.Handle("/events", protectedRead, s.handleEvents)
	for path, cmd := range apiRoutes {
		router.Handle(path, proxyRead, s.proxy(cmd))
	}
	for path, rr := range readArgsRoutes {
		router.Handle(path, proxyRead, s.readArgsProxy(rr))
	}
	for path, wr := range writeRoutes {
		router.Handle(path, protectedMutation, s.writeProxy(wr))
	}
	for path, jr := range jsonRoutes {
		router.Handle(path, jsonProtected, s.jsonProxy(jr))
	}
	router.Handle("/api/rollback/checkpoints", protectedRead, s.handleRollbackCheckpoints)
	router.Handle("/api/rollback/apply", jsonProtected, s.handleRollbackApply)
	streamMutation := jsonProtected
	streamMutation.Timeout = planRunTimeout
	router.Handle("/api/plan/run", streamMutation, s.planRunProxy())
	router.Handle("/api/run", streamMutation, s.runStreamProxy())
	// CLI Toolbox install (M956): runs the host package manager and streams a
	// per-tool progress event then a final summary as SSE.
	router.Handle("/api/toolbox/install", streamMutation, s.toolInstallProxy())
	// Marketplace install/uninstall stream per-item progress (skill/mcp/tool) as SSE.
	router.Handle("/api/market/install", streamMutation, s.marketStreamProxy(controlplane.CmdMarketInstall, []string{"name", "marketplace", "version"}))
	router.Handle("/api/market/uninstall", streamMutation, s.marketStreamProxy(controlplane.CmdMarketUninstall, []string{"name"}))
	// SSE token endpoint (VULN query-string-token fix): returns the ephemeral
	// SSE-only token the SPA embeds in the EventSource URL (?st=...) instead of
	// reusing the main console token. Requires valid WebUI data authorization.
	sseTokenRead := protectedRead
	sseTokenRead.Method = "GET,OPTIONS"
	router.Handle("/api/sse-token", sseTokenRead, s.handleSSEToken)
	// Provider-free speech readiness probe. Unlike the STT/TTS action routes,
	// this never uploads dummy audio or synthesizes throwaway speech.
	router.Handle("/api/voice/status", protectedRead, s.handleVoiceStatus)
	transcribeProtected := protectedMutation
	transcribeProtected.Timeout = 0
	transcribeProtected.BodyMax = audioMaxBytes
	router.Handle("/api/transcribe", transcribeProtected, s.handleTranscribe)
	// Text-to-speech for the console Voice Mode (M998): POST {"text":…} and the
	// server streams synthesized audio from the configured TTS backend.
	ttsProtected := protectedMutation
	ttsProtected.Timeout = 0
	ttsProtected.BodyMax = ttsTextMaxBytes
	router.Handle("/api/tts", ttsProtected, s.handleTTS)
	// Binary artifact serving (M822): streams the raw bytes for a content ref with
	// a sanitized Content-Type, so an <img src> / download link can render stored
	// images and files. Proxies CmdArtifactGet (which re-verifies the bytes).
	artifactRead := protectedRead
	artifactRead.Timeout = 15 * time.Second
	router.Handle("/api/artifact/raw", artifactRead, s.handleArtifactRaw)
	// File Manager routes (M1017): live filesystem under the configured
	// `AGEZT_FILE_ROOT` (default `~/agezt/workspace`). Every endpoint here
	// path-traversal-guards before touching the OS — see files_route.go for
	// the chokepoint.
	router.Handle("/api/files/tree", protectedRead, s.handleFileTree)
	router.Handle("/api/files/raw", protectedRead, s.handleFileRaw)
	filesMutation := protectedMutation
	filesMutation.Timeout = 0
	filesMutation.BodyMax = defaultFileCap
	router.Handle("/api/files/mkdir", filesMutation, s.handleFileMkdir)
	router.Handle("/api/files/rename", filesMutation, s.handleFileRename)
	router.Handle("/api/files/delete", filesMutation, s.handleFileDelete)
	// Workflow webhooks (M809): the ONE deliberately console-token-free
	// path. Authentication is the per-workflow secret, verified by the
	// control plane (constant-time); all this handler can ever do is ask
	// "fire workflow <name>" — no reads, no other writes, uniform refusals.
	webhookMutation := publicMutation
	webhookMutation.BodyMax = webhookBodyCap
	webhookMutation.Timeout = 130 * time.Second
	router.Handle("/hooks/", webhookMutation, s.handleWorkflowHook)
	// Channel OAuth redirect target (Phase 4). Public (no console token): the
	// provider redirects the operator's browser here with ?code&state. Security
	// rests on the unguessable state minted by /api/channel/oauth/start.
	oauthCallback := publicRead
	oauthCallback.Mutation = true
	oauthCallback.Timeout = 30 * time.Second
	router.Handle("/oauth/callback", oauthCallback, s.handleOAuthCallback)
	return router
}

func (s *Server) Handler() http.Handler {
	return s.secure(s.routeRegistry().ServeHTTP)
}

// handleOAuthCallback receives the provider's authorization redirect
// (GET /oauth/callback?code=&state=), forwards it to the control plane to
// exchange the code and store the token, and renders a small self-closing page.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Subscribe before opening the stream so a subscribe failure is a plain
	// 500, not a broken SSE body. StartSSE bounds concurrent firehose streams
	// per client (V-009) — 429 when a client is over its generous cap.
	sub, err := s.bus.Subscribe(">", 256)
	if err != nil {
		http.Error(w, "subscribe: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer sub.Cancel()
	sse, ok := httpserver.StartSSE(w, r)
	if !ok {
		return
	}
	defer sse.Close()
	// An initial comment opens the stream so the browser's EventSource fires
	// onopen even before the first event.
	_ = sse.Comment("connected")

	ctx := r.Context()
	// A heartbeat keeps proxies from closing an idle stream.
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if err := sse.Comment("ping"); err != nil {
				return
			}
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if err := sse.WriteData(string(payload)); err != nil {
				return
			}
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

// numericQueryArgs are the route arg keys whose control-plane handlers expect
// JSON numbers (argLimit/argFloat64/scheduleArgNumber all reject strings). A
// query string only carries text, so the query-arg proxies coerce these keys
// before forwarding; a value that doesn't parse rides through as the string so
// the server's "must be a number" error still names the real problem.
var numericQueryArgs = map[string]bool{
	"limit":           true,
	"since_ms":        true,
	"min_cost_mc":     true,
	"max_cost_mc":     true,
	"offset":          true,
	"older_than_days": true,
	"count":           true,
}

func queryArgValue(k, v string) any {
	if numericQueryArgs[k] {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return v
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
				args[k] = queryArgValue(k, v)
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
				args[k] = queryArgValue(k, v)
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
	dec := json.NewDecoder(r.Body)
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
