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
// Security (SPEC-06): the server is bound by the operator (loopback by default),
// token-authed on every request, and READ-ONLY in v1 — it only ever issues
// read commands; halt/approve/forget from the browser are deferred.
package webui

import (
	"context"
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

// apiRoutes maps each /api path to the read-only control-plane command it
// proxies. Read-only by construction: there is no path here that mutates.
var apiRoutes = map[string]string{
	"/api/status":  controlplane.CmdStatus,
	"/api/memory":  controlplane.CmdMemoryList,
	"/api/world":   controlplane.CmdWorldList,
	"/api/skills":  controlplane.CmdSkillList,
	"/api/inbox":   controlplane.CmdInbox,
	"/api/reflect": controlplane.CmdReflectShow,
}

// Handler builds the mux. Every route is wrapped in token auth.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.auth(s.handleDashboard))
	mux.HandleFunc("/events", s.auth(s.handleEvents))
	for path, cmd := range apiRoutes {
		mux.HandleFunc(path, s.auth(s.proxy(cmd)))
	}
	return mux
}

// auth wraps a handler with token checking. The browser passes the token in the
// query string (EventSource can't set headers); API callers may use either the
// query or an Authorization: Bearer header.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return false // never serve without a configured token
	}
	if tok := r.URL.Query().Get("token"); tok == s.token {
		return true
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ") == s.token
	}
	return false
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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
