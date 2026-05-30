// SPDX-License-Identifier: MIT

// Package restapi serves Agezt's first-party REST surface (ROADMAP P7-API-02,
// the inbound half; outbound is kernel/webhook). Where kernel/openaiapi mimics
// OpenAI's wire shapes for drop-in client compatibility, this surface speaks
// native Agezt semantics: runs are correlation-first, the streaming form emits
// the kernel's own event arc, and a submitted run is inspectable afterwards by
// its correlation id.
//
// Every run goes through the same kernel tool-loop as `agt run` — Edict, the
// journal, and the budget all apply; this is not a governance backdoor.
//
// Routes (all under /api/v1, all token-authed):
//
//	GET  /api/v1/health            — liveness + version + model summary
//	GET  /api/v1/models            — the model ids the daemon can route
//	POST /api/v1/runs              — submit an intent; sync JSON or SSE stream
//	GET  /api/v1/runs/{corr}       — the journaled event arc of a past run
//
// Security (SPEC-06): loopback-bound by the operator, token-authed on every
// request (Authorization: Bearer <token>, or ?token= for convenience). Empty
// token fails closed.
package restapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

// Engine is the slice of the kernel this server drives. It is satisfied
// structurally by the daemon's kernel adapter (the same one kernel/openaiapi
// uses, plus EventsForCorrelation for run inspection).
type Engine interface {
	NewCorrelation() string
	SubjectForRun(corr string) string
	RunModel(ctx context.Context, corr, intent, model string) (string, error)
	DefaultModel() string
	ModelIDs() []string
	// EventsForCorrelation returns the journaled events of a run, in order.
	// Empty (not an error) when the correlation is unknown.
	EventsForCorrelation(corr string) ([]*event.Event, error)
}

// TenantResolver maps a tenant id to the Engine + bus that serve it. The daemon
// injects one (backed by the tenant registry) when multi-tenancy is enabled.
type TenantResolver func(tenant string) (Engine, *bus.Bus, error)

// Server is the native REST surface.
type Server struct {
	eng     Engine
	bus     *bus.Bus
	token   string
	version string

	// resolve, when set, maps the X-Agezt-Tenant request header to a per-tenant
	// Engine + bus. Nil (or an empty header) means the primary engine/bus —
	// the unchanged single-tenant path.
	resolve TenantResolver
}

// New builds a Server. token gates every request; bus drives streaming;
// version is reported by /health.
func New(eng Engine, b *bus.Bus, token, version string) *Server {
	return &Server{eng: eng, bus: b, token: token, version: version}
}

// SetTenantResolver enables tenant routing: requests carrying an X-Agezt-Tenant
// header are served by the resolved per-tenant Engine + bus.
func (s *Server) SetTenantResolver(r TenantResolver) { s.resolve = r }

// bind resolves the Engine + bus for a request: the per-tenant pair when an
// X-Agezt-Tenant header is present and a resolver is configured, else the
// primary engine/bus.
func (s *Server) bind(r *http.Request) (Engine, *bus.Bus, error) {
	tenant := strings.TrimSpace(r.Header.Get("X-Agezt-Tenant"))
	if tenant == "" || s.resolve == nil {
		return s.eng, s.bus, nil
	}
	return s.resolve(tenant)
}

// Handler builds the mux; every route is token-authed.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", s.auth(s.handleHealth))
	mux.HandleFunc("/api/v1/models", s.auth(s.handleModels))
	mux.HandleFunc("/api/v1/runs", s.auth(s.handleRunsRoot))
	mux.HandleFunc("/api/v1/runs/", s.auth(s.handleRunByID))
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token")
			return
		}
		next(w, r)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return false
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ") == s.token
	}
	return r.URL.Query().Get("token") == s.token
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
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	answer, err := eng.RunModel(r.Context(), corr, intent, model)
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
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "stream_unsupported", "streaming unsupported")
		return
	}
	corr := eng.NewCorrelation()
	sub, err := b.Subscribe(eng.SubjectForRun(corr), 1024)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "subscribe_error", err.Error())
		return
	}
	defer sub.Cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	send := func(eventName string, payload map[string]any) {
		data, _ := json.Marshal(payload)
		_, _ = w.Write([]byte("event: " + eventName + "\ndata: " + string(data) + "\n\n"))
		flusher.Flush()
	}
	send("start", map[string]any{"correlation_id": corr, "model": model})

	type result struct {
		answer string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		ans, err := eng.RunModel(r.Context(), corr, intent, model)
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

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", allow+" required")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, typ, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]any{"type": typ, "message": msg}})
}

// tokenText returns the streamed text delta carried by an llm.token event, or
// "" for any other event.
func tokenText(ev *event.Event) string {
	if ev == nil || ev.Kind != event.KindLLMToken || len(ev.Payload) == 0 {
		return ""
	}
	var p struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(ev.Payload, &p) != nil {
		return ""
	}
	return p.Text
}
