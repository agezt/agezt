// SPDX-License-Identifier: MIT

// Package webui serves the Agezt Web UI v1 (SPEC-07): a server-rendered,
// SSE-driven dashboard over the journal/bus. It is the stdlib-first MVP cut of
// SPEC-07's React surfaces — net/http + html/template + embed, no build chain
// — faithful to §0 ("one event truth, many views; the UI never holds
// authoritative state, it subscribes and renders") and §5.2 ("Live Monitor
// driven entirely by events — the journal is the telemetry").
//
// It holds no state. Two data paths, both reusing what already exists:
//   - the live event feed subscribes to the kernel bus (the same ">" stream the
//     daemon tees to stdout);
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
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
)

//go:embed dashboard.html
var dashboardHTML []byte

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

// Server is the Web UI HTTP surface.
type Server struct {
	bus    *bus.Bus
	client Caller
	token  string
}

// New builds a Server. token gates every request; bus drives the live feed;
// client proxies read commands.
func New(b *bus.Bus, client Caller, token string) *Server {
	return &Server{bus: b, client: client, token: token}
}

// apiRoutes maps each GET /api path to the read-only control-plane command it
// proxies. Read-only by construction: there is no path here that mutates.
var apiRoutes = map[string]string{
	"/api/status":     controlplane.CmdStatus,
	"/api/config":     controlplane.CmdConfig,
	"/api/runs":       controlplane.CmdRunsList,
	"/api/stats":      controlplane.CmdRunsStats,
	"/api/budget":     controlplane.CmdBudget,
	"/api/cache":      controlplane.CmdCacheStats,
	"/api/providers":  controlplane.CmdProviderStats,
	"/api/tools":      controlplane.CmdToolStats,
	"/api/policy":     controlplane.CmdEdictStats,
	"/api/schedules":  controlplane.CmdScheduleList,
	"/api/memory":     controlplane.CmdMemoryList,
	"/api/world":      controlplane.CmdWorldList,
	"/api/skills":     controlplane.CmdSkillList,
	"/api/standing":   controlplane.CmdStandingList,
	"/api/inbox":      controlplane.CmdInbox,
	"/api/reflect":    controlplane.CmdReflectShow,
	"/api/approvals":  controlplane.CmdApprovals,
	"/api/plan_stats": controlplane.CmdPlanStats,
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
	"/api/journal":      {controlplane.CmdJournalGrep, []string{"correlation_id", "kind", "limit"}},
	"/api/provider_log": {controlplane.CmdProviderLog, []string{"limit", "fallbacks"}},
	"/api/tool_log":     {controlplane.CmdToolLog, []string{"limit", "tool", "errors"}},
	"/api/policy_log":   {controlplane.CmdEdictLog, []string{"limit", "denied"}},
	"/api/plan_history": {controlplane.CmdPlanHistory, []string{"limit", "status"}},
}

// writeRoutes is the operator-action allowlist: the big red button (halt),
// its inverse (resume), and HITL approval resolution (decide). Each is
// POST-only (see writeProxy).
var writeRoutes = map[string]writeRoute{
	"/api/halt":             {controlplane.CmdHalt, []string{"reason"}},
	"/api/resume":           {controlplane.CmdResume, []string{"reason"}},
	"/api/decide":           {controlplane.CmdDecide, []string{"id", "decision", "reason"}},
	"/api/memory/forget":    {controlplane.CmdMemoryForget, []string{"id"}},
	"/api/world/forget":     {controlplane.CmdWorldForget, []string{"id"}},
	"/api/skill/promote":    {controlplane.CmdSkillPromote, []string{"id"}},
	"/api/skill/quarantine": {controlplane.CmdSkillQuarantine, []string{"id", "reason"}},
	"/api/skill/revert":     {controlplane.CmdSkillRevert, []string{"id"}},
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
	mux.HandleFunc("/", s.auth(s.handleDashboard))
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
	return mux
}

// auth wraps a handler with token checking. The browser passes the token in the
// query string (EventSource can't set headers); API callers may use either the
// query or an Authorization: Bearer header.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		if !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// setSecurityHeaders applies defensive response headers to every web UI route
// (set before the auth check so even 401s carry them). This is a control surface:
//   - X-Frame-Options DENY — the dashboard has state-mutating controls
//     (approve/halt/resume/decide), so framing is denied to block clickjacking.
//   - Referrer-Policy no-referrer — the page URL carries the auth token in
//     `?token=`, so the referrer is suppressed to keep it out of any Referer header.
//   - X-Content-Type-Options nosniff — stop content-type sniffing/confusion.
func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
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

// handleDashboard serves the embedded single-page dashboard at "/".
//
// The page carries the only inline <script>/<style> in the surface, and it is
// the highest-privilege browser context (same-origin with the token-authed,
// state-mutating control plane). It is built entirely with textContent — no
// innerHTML sink — so it has no current XSS, but a Content-Security-Policy with
// a per-response nonce is set as defense-in-depth: `default-src 'none'` blocks
// any injected external resource, `script-src 'nonce-…'` means only the genuine
// inline block runs (an injected <script> from a future regression is refused
// because it can't carry the unpredictable nonce), and `connect-src 'self'` /
// `base-uri` / `form-action` / `frame-ancestors 'none'` close exfiltration and
// pivot avenues. The nonce is minted per request and substituted into the two
// tag placeholders so the header and the markup always agree.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	nonce := newCSPNonce()
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; "+
			"script-src 'nonce-"+nonce+"'; "+
			"style-src 'nonce-"+nonce+"'; "+
			"connect-src 'self'; "+
			"img-src 'self' data:; "+
			"base-uri 'none'; "+
			"form-action 'none'; "+
			"frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	body := strings.ReplaceAll(string(dashboardHTML), "__CSP_NONCE__", nonce)
	_, _ = w.Write([]byte(body))
}

// newCSPNonce returns a fresh, unpredictable base64 nonce for the dashboard's
// Content-Security-Policy. 16 bytes of crypto/rand is ample CSP-nonce entropy.
// rand.Read never returns an error on the platforms we target; the zero-value
// fallback only matters in the impossible error case and is still per-process
// unique enough to not weaken a page that has no XSS sink to begin with.
func newCSPNonce() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.StdEncoding.EncodeToString(b[:])
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
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
