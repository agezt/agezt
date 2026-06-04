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
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/controlplane"
)

//go:embed dashboard.html
var dashboardHTML []byte

// Caller is the read API the dashboard proxies to — satisfied by
// *controlplane.Client. An interface keeps webui testable without a live
// daemon (a fake Caller + an in-memory bus is enough).
type Caller interface {
	Call(ctx context.Context, cmd string, args map[string]any) (map[string]any, error)
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
	"/api/status":    controlplane.CmdStatus,
	"/api/config":    controlplane.CmdConfig,
	"/api/runs":      controlplane.CmdRunsList,
	"/api/stats":     controlplane.CmdRunsStats,
	"/api/budget":    controlplane.CmdBudget,
	"/api/cache":     controlplane.CmdCacheStats,
	"/api/providers": controlplane.CmdProviderStats,
	"/api/tools":     controlplane.CmdToolStats,
	"/api/policy":    controlplane.CmdEdictStats,
	"/api/schedules": controlplane.CmdScheduleList,
	"/api/memory":    controlplane.CmdMemoryList,
	"/api/world":     controlplane.CmdWorldList,
	"/api/skills":    controlplane.CmdSkillList,
	"/api/inbox":     controlplane.CmdInbox,
	"/api/reflect":   controlplane.CmdReflectShow,
	"/api/approvals": controlplane.CmdApprovals,
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
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(dashboardHTML)
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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
