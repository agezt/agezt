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
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/meshctx"
)

// maxRequestBodyBytes caps an HTTP request body (M198). The API surfaces are
// network-exposed and token-authed, but a token holder (or a compromised/buggy
// client) must not be able to OOM the daemon with a giant JSON body. 16 MiB is
// far above any legitimate run/chat request.
const maxRequestBodyBytes = 16 << 20

// Engine is the slice of the kernel this server drives. It is satisfied
// structurally by the daemon's kernel adapter (the same one kernel/openaiapi
// uses, plus EventsForCorrelation for run inspection).
type Engine interface {
	NewCorrelation() string
	SubjectForRun(corr string) string
	RunModel(ctx context.Context, corr, intent, model string, images []string, jsonMode bool) (string, error)
	DefaultModel() string
	ModelIDs() []string
	// EventsForCorrelation returns the journaled events of a run, in order.
	// Empty (not an error) when the correlation is unknown.
	EventsForCorrelation(corr string) ([]*event.Event, error)
}

// TenantResolver maps a tenant id to the Engine + bus that serve it. The daemon
// injects one (backed by the tenant registry) when multi-tenancy is enabled.
type TenantResolver func(tenant string) (Engine, *bus.Bus, error)

// TenantAuthorizer reports whether presented is the per-tenant credential of
// tenant id. The daemon injects one backed by the registry; it lets a scoped
// per-tenant token authorize requests against ONLY its own tenant, while the
// daemon admin token continues to authorize any tenant.
type TenantAuthorizer func(tenant, presented string) bool

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

	// tenantAuth, when set, validates a per-tenant token against the tenant named
	// in the X-Agezt-Tenant header. Nil means only the admin token authorizes.
	tenantAuth TenantAuthorizer

	// readiness, when set, reports whether the daemon can serve work right now
	// (e.g. not halted) for the unauthenticated /readyz probe. Nil → always ready
	// (the server answering at all proves liveness). Injected by the daemon so
	// this package needs no kernel halt-state coupling.
	readiness func() (ready bool, reason string)

	// metrics, when set, supplies the gauges exposed at /metrics in Prometheus
	// text format. Injected by the daemon (it has the kernel + governor); this
	// package only formats. Nil → /metrics reports no samples.
	metrics func() []Metric
}

// Metric is one Prometheus sample exposed at /metrics. Name is the suffix after
// the `agezt_` prefix (e.g. "active_runs" → `agezt_active_runs`).
type Metric struct {
	Name  string
	Help  string
	Type  string // "gauge" or "counter"
	Value float64
}

// New builds a Server. token gates every request; bus drives streaming;
// version is reported by /health.
func New(eng Engine, b *bus.Bus, token, version string) *Server {
	return &Server{eng: eng, bus: b, token: token, version: version}
}

// SetTenantResolver enables tenant routing: requests carrying an X-Agezt-Tenant
// header are served by the resolved per-tenant Engine + bus.
func (s *Server) SetTenantResolver(r TenantResolver) { s.resolve = r }

// SetTenantAuthorizer enables per-tenant credentials: a request targeting a
// tenant (X-Agezt-Tenant header) may authorize with that tenant's own token
// instead of the daemon admin token. The admin token still authorizes any tenant.
func (s *Server) SetTenantAuthorizer(a TenantAuthorizer) { s.tenantAuth = a }

// SetReadiness injects the readiness probe behind the unauthenticated /readyz
// endpoint: it returns (false, reason) when the daemon can't serve work (e.g.
// halted). When unset, /readyz reports ready.
func (s *Server) SetReadiness(fn func() (ready bool, reason string)) { s.readiness = fn }

// SetMetrics injects the gauge source for /metrics (Prometheus text format).
func (s *Server) SetMetrics(fn func() []Metric) { s.metrics = fn }

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

// Handler builds the mux. The /api/v1/* routes are token-authed; the /healthz
// and /readyz probes are intentionally UNAUTHENTICATED so deployment tooling
// (systemd watchdog, container/k8s liveness+readiness probes, load balancers,
// uptime monitors) can check the daemon without a credential. They expose only
// liveness/readiness — never version, model, or any run data (that stays behind
// the authed /api/v1/health).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleLive)
	mux.HandleFunc("/readyz", s.handleReady)
	// /metrics is token-authed: unlike liveness/readiness it exposes spend and
	// activity volume (financially/operationally sensitive). Prometheus scrapes it
	// with a bearer_token.
	mux.HandleFunc("/metrics", s.auth(s.handleMetrics))
	mux.HandleFunc("/api/v1/health", s.auth(s.handleHealth))
	mux.HandleFunc("/api/v1/models", s.auth(s.handleModels))
	mux.HandleFunc("/api/v1/runs", s.auth(s.handleRunsRoot))
	mux.HandleFunc("/api/v1/runs/", s.auth(s.handleRunByID))
	return mux
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
				b.WriteString("# HELP " + name + " " + m.Help + "\n")
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
	presented := bearerToken(r)
	if presented == "" {
		return false
	}
	// The daemon admin token authorizes the primary and any tenant.
	if s.token != "" && subtle.ConstantTimeCompare([]byte(presented), []byte(s.token)) == 1 {
		return true
	}
	// Otherwise a per-tenant token authorizes ONLY its own tenant, and only when
	// the request actually targets that tenant via the header.
	if s.tenantAuth != nil {
		if id := strings.TrimSpace(r.Header.Get("X-Agezt-Tenant")); id != "" {
			return s.tenantAuth(id, presented)
		}
	}
	return false
}

// bearerToken extracts the presented token from the Authorization: Bearer
// header, falling back to the ?token= query param (browser/EventSource convenience).
func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return r.URL.Query().Get("token")
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
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
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
func promName(s string) string {
	if s == "" {
		return "_"
	}
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c == ':':
			// valid anywhere
		case c >= '0' && c <= '9':
			// valid except as the first character (handled below)
		default:
			b[i] = '_'
		}
	}
	if b[0] >= '0' && b[0] <= '9' {
		return "_" + string(b) // a leading digit isn't allowed; prefix '_'
	}
	return string(b)
}

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
