// SPDX-License-Identifier: MIT

package restapi

// Provenance: REST API routes: Handler +
//             handleLive/Ready/Metrics/Health/Models/RunsRoot/streamRun/RunByID.
//             Code extracted from restapi.go during the Day-66 god-file split.
//             Public API unchanged.

import (
	"net/http"
	"strconv"
	"strings"

	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/contract/restapiapi"
	"github.com/agezt/agezt/kernel/httpserver"
)

func (s *Server) Handler() http.Handler {
	authenticator := httpserver.Authenticator{
		Verifier:        s.verifier,
		TenantAuthorize: httpserver.TenantAuthorizer(s.tenantAuth),
	}
	router := httpserver.NewRouter(authenticator, func(w http.ResponseWriter, _ *http.Request) {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token")
	})

	// The route table lives in kernel/contract/restapiapi — the one home of
	// the `/api/v1` surface (W5.2). The daemon registers FROM it, and
	// tools/sdkgen generates the SDK catalogs from the same table, so a path
	// exists in exactly one place. The handler map below is the only thing
	// this file still owns: which function serves which route.
	handlers := map[string]http.HandlerFunc{
		restapiapi.ProbeHealthz:       s.handleLive,
		restapiapi.ProbeReadyz:        s.handleReady,
		restapiapi.ProbeMetrics:       s.handleMetrics,
		restapiapi.RouteHealth:        s.handleHealth,
		restapiapi.RouteModels:        s.handleModels,
		restapiapi.RouteRuns:          s.handleRunsRoot,
		restapiapi.RouteRunByID:       s.handleRunByID,
		restapiapi.RouteArtifacts:     s.handleArtifacts,
		restapiapi.RouteArtifactBytes: s.handleArtifactBytes,
		restapiapi.RouteMailbox:       s.handleMailboxMessages,
		restapiapi.RouteMailboxSub:    s.handleMailboxMessageSub,
		restapiapi.RouteMailboxInbox:  s.handleMailboxInbox,
		restapiapi.RouteMailboxWatch:  s.handleMailboxWatch,
		restapiapi.RouteMailboxTopics: s.handleMailboxTopics,
		restapiapi.RouteUpdate:        s.handleUpdateCheck,
		restapiapi.RouteUpdateApply:   s.handleUpdateApply,
	}
	for _, r := range restapiapi.Routes() {
		h, ok := handlers[r.ID]
		if !ok {
			// A route in the contract without a handler would answer 404 on a
			// path the SDK was told to call: fail loudly at boot instead.
			panic("restapi: contract route has no handler: " + r.ID)
		}
		delete(handlers, r.ID)
		router.Handle(r.Path, routeOpts(r), h)
	}
	if len(handlers) != 0 {
		// A handler without a contract route is the same drift in the other
		// direction: the code serves a path the contract does not name.
		for id := range handlers {
			panic("restapi: handler has no contract route: " + id)
		}
	}
	return router
}

// routeOpts turns a contract route into the router's transport policy.
//
// The tier decides the authorization level; the methods decide the verbs the
// router answers; a mutating route gets the surface's body cap (or the
// route's own smaller cap) and is marked as a mutation so the transport
// audits it.
func routeOpts(r restapiapi.Route) httpserver.RouteOpts {
	verbs := make([]string, 0, len(r.Methods)+1)
	for _, m := range r.Methods {
		verbs = append(verbs, m.Verb)
	}
	if r.Head {
		verbs = append(verbs, http.MethodHead)
	}
	opts := httpserver.RouteOpts{
		Tier:   authTier(r.Tier),
		Method: strings.Join(verbs, ","),
	}
	if r.Mutation {
		opts.Mutation = true
		opts.BodyMax = maxRequestBodyBytes
		if r.BodyMax > 0 {
			opts.BodyMax = r.BodyMax
		}
	}
	if r.Tier == restapiapi.TierAdmin {
		// The admin tier says more than "not the user tier": the caller needs
		// to know it needs the DAEMON's token, not merely a valid one.
		opts.Unauthorized = func(w http.ResponseWriter, _ *http.Request) {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "this endpoint requires the daemon admin token")
		}
	}
	return opts
}

// authTier maps a contract tier onto the kernel's authorization tier.
func authTier(t restapiapi.Tier) kernelauth.Tier {
	switch t {
	case restapiapi.TierPublic:
		return kernelauth.TierPublic
	case restapiapi.TierAdmin:
		return kernelauth.TierAdmin
	default:
		return kernelauth.TierUser
	}
}

// --- GET /healthz (unauthenticated liveness) ---
//
// 200 as long as the HTTP server can answer — i.e. the process is alive and
// serving. No kernel state, no sensitive fields. HEAD is supported for monitors
// that probe with it.
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// --- GET /readyz (unauthenticated readiness) ---
//
// 200 {"status":"ready"} when the daemon can serve work; 503
// {"status":"not_ready","reason":...} otherwise (e.g. halted), so a load
// balancer / readiness probe pulls it out of rotation while it's halted but the
// process stays live.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ready, reason := true, ""
	if s.readiness != nil {
		ready, reason = s.readiness()
	}
	if ready {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "reason": reason})
}

// --- GET /metrics (token-authed, Prometheus text exposition) ---

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	var b strings.Builder
	if s.metrics != nil {
		for _, m := range s.metrics() {
			name := promName("agezt_" + m.Name)
			if m.Help != "" {
				b.WriteString("# HELP " + name + " " + promHelp(m.Help) + "\n")
			}
			typ := m.Type
			if typ == "" {
				typ = "gauge"
			}
			b.WriteString("# TYPE " + name + " " + typ + "\n")
			// 'f' (not 'g') so large gauges like spend render as plain integers
			// (1500000000), not scientific notation — both are valid Prometheus,
			// but plain reads better in dashboards and ad-hoc curls.
			b.WriteString(name + " " + strconv.FormatFloat(m.Value, 'f', -1, 64) + "\n")
		}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

// --- GET /api/v1/health ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	// Answer from the engine the request is bound to: a tenant request used to
	// be answered from the primary kernel.
	eng, _, err := s.bind(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_tenant", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"version":       s.version,
		"default_model": eng.DefaultModel(),
		"model_count":   len(eng.ModelIDs()),
	})
}

// --- GET /api/v1/models ---

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	eng, _, err := s.bind(r) // the tenant's own model set, not the primary's
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_tenant", err.Error())
		return
	}
	ids := eng.ModelIDs()
	seen := map[string]bool{}
	out := make([]string, 0, len(ids)+1)
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	add(eng.DefaultModel())
	for _, id := range ids {
		add(id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"default": eng.DefaultModel(), "models": out})
}

// --- POST /api/v1/runs ---
