// SPDX-License-Identifier: MIT

// REST API routes: Handler + handleLive/Ready/Metrics/Health/Models/RunsRoot/streamRun/RunByID.
// Code extracted from restapi.go during the Day-66 god-file split. Public API unchanged.
package restapi


import (
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/httpserver"
	"github.com/agezt/agezt/kernel/meshctx"
	"net/http"
	"strconv"
	"strings"
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

type runRequest struct {
	Intent string `json:"intent"`
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

func (s *Server) handleRunsRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	// Mesh delegation loop guard (M209): a run handed here from a peer's remote_run
	// carries a hop count. Refuse one past the limit so a federated mesh can't recurse
	// forever, and thread the hop into the run context so this node's own remote_run
	// (if it fires) forwards hop+1 in turn. A run with no header starts the chain at 0.
	hopIn := 0
	if h := r.Header.Get(meshctx.HopHeader); h != "" {
		if n, err := strconv.Atoi(h); err == nil && n > 0 {
			hopIn = n
		}
	}
	maxHops := meshctx.MaxHopsFromEnv()
	if hopIn > maxHops {
		// Audit the refusal so a stopped federation loop is visible in the journal /
		// `agt pulse` (M210), not just to the rejected caller. Publish to the TARGET
		// tenant's bus when the request names a resolvable tenant, so that tenant sees
		// its own mesh refusals (M212); fall back to the primary bus otherwise. The 508
		// is returned regardless — a bad tenant header does not change the outcome here.
		auditBus := s.bus
		if _, tb, err := s.bind(r); err == nil && tb != nil {
			auditBus = tb
		}
		if auditBus != nil {
			_, _ = auditBus.Publish(event.Spec{
				Subject: "mesh.loop",
				Kind:    event.KindMeshLoopRefused,
				Actor:   "restapi",
				Payload: map[string]any{"hop": hopIn, "max_hops": maxHops},
			})
		}
		writeErr(w, http.StatusLoopDetected, "mesh_hop_limit",
			"mesh delegation hop limit exceeded — refusing to avoid a federation loop")
		return
	}
	r = r.WithContext(meshctx.WithHop(r.Context(), hopIn))

	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request_too_large",
				"request body exceeds the size limit")
			return
		}
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
		return
	}
	intent := strings.TrimSpace(req.Intent)
	if intent == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "intent is required")
		return
	}
	eng, b, err := s.bind(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_tenant", err.Error())
		return
	}
	model := req.Model
	if model == "" {
		model = eng.DefaultModel()
	}

	// Streaming is opt-in via the body flag or the SSE Accept header.
	if req.Stream || strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		s.streamRun(w, r, eng, b, intent, model)
		return
	}

	corr := eng.NewCorrelation()
	answer, err := eng.RunModel(r.Context(), corr, intent, model, nil, false)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"correlation_id": corr, "model": model, "status": "failed", "error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"correlation_id": corr,
		"model":          model,
		"status":         "completed",
		"answer":         answer,
	})
}

// streamRun runs the intent and relays the kernel's llm.token events as native
// SSE frames (event: token / event: done / event: error). It subscribes BEFORE
// starting the run so no early token is missed.
func (s *Server) streamRun(w http.ResponseWriter, r *http.Request, eng Engine, b *bus.Bus, intent, model string) {
	corr := eng.NewCorrelation()
	sub, err := b.Subscribe(eng.SubjectForRun(corr), 1024)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "subscribe_error", err.Error())
		return
	}
	defer sub.Cancel()

	// StartSSE applies the process-wide per-client stream cap (V-009/LD-8).
	sse, ok := httpserver.StartSSE(w, r)
	if !ok {
		return
	}
	defer sse.Close()

	send := func(eventName string, payload map[string]any) {
		_ = sse.WriteEvent(eventName, payload)
	}
	send("start", map[string]any{"correlation_id": corr, "model": model})

	type result struct {
		answer string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		ans, err := eng.RunModel(r.Context(), corr, intent, model, nil, false)
		done <- result{ans, err}
	}()

	emit := func(ev *event.Event) {
		if txt := tokenText(ev); txt != "" {
			send("token", map[string]any{"text": txt})
		}
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				send("done", map[string]any{"correlation_id": corr, "status": "completed"})
				return
			}
			emit(ev)
		case res := <-done:
			for drained := false; !drained; {
				select {
				case ev := <-sub.C:
					emit(ev)
				default:
					drained = true
				}
			}
			if res.err != nil {
				send("error", map[string]any{"correlation_id": corr, "error": res.err.Error()})
			} else {
				send("done", map[string]any{"correlation_id": corr, "status": "completed", "answer": res.answer})
			}
			return
		}
	}
}

// --- GET /api/v1/runs/{corr} ---

func (s *Server) handleRunByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	corr := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/runs/"), "/")
	if corr == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "correlation id required in path")
		return
	}
	eng, _, err := s.bind(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_tenant", err.Error())
		return
	}
	events, err := eng.EventsForCorrelation(corr)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "lookup_error", err.Error())
		return
	}
	if len(events) == 0 {
		writeErr(w, http.StatusNotFound, "not_found", "no run with correlation id "+corr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"correlation_id": corr,
		"count":          len(events),
		"events":         events,
	})
}

// --- helpers ---

// promName coerces s to Prometheus's metric-name grammar
// ([a-zA-Z_:][a-zA-Z0-9_:]*): any other byte becomes '_', and a leading digit is
// prefixed with '_'. A metric whose name contained a '.', '-', or space would
// otherwise emit a line Prometheus can't parse — and one malformed line breaks the
// WHOLE scrape, silently dropping every other metric. Today's names are all valid;
// this keeps a future bad metric definition from taking out observability wholesale.