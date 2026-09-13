// SPDX-License-Identifier: MIT

package restapi

// Run handlers: runRequest type + handleRunsRoot + streamRun +
// handleRunByID. Carved out of restapi_routes.go during the Day
// 197 god-file split so the main file can stay focused on the
// HTTP router + health/probe handlers and the runs file can stay
// focused on the run-create/stream/get handlers.
// Public API unchanged.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/httpserver"
	"github.com/agezt/agezt/kernel/meshctx"
)

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
