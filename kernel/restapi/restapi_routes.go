// SPDX-License-Identifier: MIT

// REST API routes: Handler + handleLive/Ready/Metrics/Health/Models/RunsRoot/streamRun/RunByID.
// Code extracted from restapi.go during the Day-66 god-file split. Public API unchanged.
package restapi

import (
	"net/http"
	"strconv"
	"strings"

	kernelauth "github.com/agezt/agezt/kernel/auth"
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
	publicRoute := httpserver.RouteOpts{Tier: kernelauth.TierPublic, Method: "GET,HEAD"}
	userRoute := httpserver.RouteOpts{Tier: kernelauth.TierUser, Method: http.MethodGet}
	userBodyRoute := httpserver.RouteOpts{
		Tier:     kernelauth.TierUser,
		Method:   http.MethodPost,
		BodyMax:  maxRequestBodyBytes,
		Mutation: true,
	}
	adminRoute := httpserver.RouteOpts{
		Tier:   kernelauth.TierAdmin,
		Method: http.MethodGet,
		Unauthorized: func(w http.ResponseWriter, _ *http.Request) {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "this endpoint requires the daemon admin token")
		},
	}
	adminBodyRoute := adminRoute
	adminBodyRoute.Method = "GET,POST"
	adminBodyRoute.BodyMax = maxRequestBodyBytes
	adminBodyRoute.Mutation = true

	router.Handle("/healthz", publicRoute, s.handleLive)
	router.Handle("/readyz", publicRoute, s.handleReady)
	// /metrics is token-authed: unlike liveness/readiness it exposes spend and
	// activity volume (financially/operationally sensitive). Prometheus scrapes it
	// with a bearer_token.
	metricsRoute := userRoute
	metricsRoute.Method = "GET,HEAD"
	router.Handle("/metrics", metricsRoute, s.handleMetrics)
	router.Handle("/api/v1/health", userRoute, s.handleHealth)
	router.Handle("/api/v1/models", userRoute, s.handleModels)
	router.Handle("/api/v1/runs", userBodyRoute, s.handleRunsRoot)
	router.Handle("/api/v1/runs/", userRoute, s.handleRunByID)
	router.Handle("/api/v1/artifacts", userRoute, s.handleArtifacts)
	router.Handle("/api/v1/artifacts/", userRoute, s.handleArtifactBytes)
	// Mailbox (M937): the shared inter-agent message board for SDK apps —
	// send/read messages, an inbox per name, replies, ack, topics. The board is a
	// single daemon-global instance with no tenant partition, so it is gated to
	// the admin tier only — a per-tenant token must not reach it and
	// read/spoof across tenants. (V-011)
	router.Handle("/api/v1/mailbox/messages", adminBodyRoute, s.handleMailboxMessages)
	router.Handle("/api/v1/mailbox/messages/", adminBodyRoute, s.handleMailboxMessageSub)
	router.Handle("/api/v1/mailbox/inbox", adminRoute, s.handleMailboxInbox)
	router.Handle("/api/v1/mailbox/watch", adminRoute, s.handleMailboxWatch)
	router.Handle("/api/v1/mailbox/topics", adminRoute, s.handleMailboxTopics)

	// Self-update changes host-global daemon state — admin token only, never a
	// per-tenant credential. (V-011)
	router.Handle("/api/v1/update", adminRoute, s.handleUpdateCheck)
	updateApplyRoute := adminRoute
	updateApplyRoute.Method = http.MethodPost
	updateApplyRoute.BodyMax = 64 * 1024
	updateApplyRoute.Mutation = true
	router.Handle("/api/v1/update/apply", updateApplyRoute, s.handleUpdateApply)
	return router
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
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"version":       s.version,
		"default_model": s.eng.DefaultModel(),
		"model_count":   len(s.eng.ModelIDs()),
	})
}

// --- GET /api/v1/models ---

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ids := s.eng.ModelIDs()
	seen := map[string]bool{}
	out := make([]string, 0, len(ids)+1)
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	add(s.eng.DefaultModel())
	for _, id := range ids {
		add(id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"default": s.eng.DefaultModel(), "models": out})
}

// --- POST /api/v1/runs ---

