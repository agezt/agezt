// SPDX-License-Identifier: MIT

// Package openaiapi serves an OpenAI-compatible HTTP surface (ROADMAP P7-API-01,
// SPEC-15 §3): POST /v1/chat/completions, POST /v1/responses, GET /v1/models and
// GET /v1/models/{id}, so any OpenAI client, SDK, or IDE can drive Agezt as if it
// were OpenAI. Every request runs
// through the same kernel tool-loop as `agt run` — so it passes through Edict,
// the journal, and the budget exactly like any other run. It is NOT a
// governance backdoor (P7-API-02 DoD).
//
// The mapping is deliberate and lossy-by-design: OpenAI `messages[]` collapse
// into one Agezt intent (Agezt is an agent, not a raw completion endpoint —
// the configured provider/model and system prompt are the kernel's, not the
// caller's). The caller's `model` field is echoed back but routing is the
// Governor's job. Streaming maps the kernel's llm.token events to OpenAI
// chat.completion.chunk SSE frames.
//
// Security (SPEC-06): loopback-bound by the operator, token-authed on every
// request (Authorization: Bearer <token>, or ?token= for convenience). Empty
// token fails closed.
package openaiapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

// maxRequestBodyBytes caps an HTTP request body (M198). The OpenAI-compatible
// surface is network-exposed and token-authed, but a token holder (or a
// compromised/buggy client) must not be able to OOM the daemon with a giant JSON
// body. 16 MiB is far above any legitimate chat/responses request.
const maxRequestBodyBytes = 16 << 20

// decodeBody reads and decodes a SIZE-BOUNDED JSON request body (M198). On
// failure it writes the appropriate error (413 over the limit, else 400) and
// returns false, so callers just `if !decodeBody(...) { return }`.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "invalid_request_error",
				"request body exceeds the size limit")
		} else {
			writeErr(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body: "+err.Error())
		}
		return false
	}
	return true
}

// Engine is the slice of the kernel this server drives. An interface keeps the
// package testable with a fake (a canned RunWith that publishes token events on
// a real in-memory bus exercises the SSE path without a daemon).
type Engine interface {
	NewCorrelation() string
	SubjectForRun(corr string) string
	// RunModel runs the intent under the given correlation, honouring the
	// requested model (empty → the kernel's configured default). images carries
	// any attachment URLs parsed from a multimodal request (data: URLs or
	// http(s) URLs); nil for a text-only run. jsonMode requests structured JSON
	// output (M314), set from the request's response_format.
	RunModel(ctx context.Context, corr, intent, model string, images []string, jsonMode bool) (string, error)
	DefaultModel() string
	ModelIDs() []string
}

// UsageReporter is an optional Engine capability: report the REAL provider token
// usage for a completed run, summed across the run's LLM calls (folded from the
// journal's budget.consumed events). When an Engine implements it, the API's
// usage block reflects provider truth instead of the whitespace estimate. ok is
// false when no usage was recorded (a free/local/mock model, or the run had no
// priced call) — callers fall back to estimateUsage.
type UsageReporter interface {
	UsageFor(corr string) (promptTokens, completionTokens int, ok bool)
}

// chatUsage returns the usage block for a chat completion: the real provider
// usage when the engine can report it, else the whitespace estimate.
func chatUsage(eng Engine, corr, intent, answer string) map[string]any {
	if ur, ok := eng.(UsageReporter); ok {
		if pt, ct, ok := ur.UsageFor(corr); ok {
			return map[string]any{"prompt_tokens": pt, "completion_tokens": ct, "total_tokens": pt + ct}
		}
	}
	return estimateUsage(intent, answer)
}

// TenantResolver maps a tenant id to the Engine + bus that serve it. The daemon
// injects one (backed by the tenant registry) when multi-tenancy is enabled.
type TenantResolver func(tenant string) (Engine, *bus.Bus, error)

// TenantAuthorizer reports whether presented is the per-tenant credential of
// tenant id. It lets a scoped per-tenant token authorize requests against ONLY
// its own tenant, while the daemon admin token authorizes any tenant.
type TenantAuthorizer func(tenant, presented string) bool

// Server is the OpenAI-compatible HTTP surface.
type Server struct {
	eng   Engine
	bus   *bus.Bus
	token string

	// resolve, when set, maps the X-Agezt-Tenant request header to a per-tenant
	// Engine + bus. Nil (or an empty header) means the primary engine/bus —
	// the unchanged single-tenant path.
	resolve TenantResolver

	// tenantAuth, when set, validates a per-tenant token against the tenant named
	// in the X-Agezt-Tenant header. Nil means only the admin token authorizes.
	tenantAuth TenantAuthorizer
}

// New builds a Server. token gates every request; bus drives streaming.
func New(eng Engine, b *bus.Bus, token string) *Server {
	return &Server{eng: eng, bus: b, token: token}
}

// SetTenantResolver enables tenant routing: requests carrying an X-Agezt-Tenant
// header are served by the resolved per-tenant Engine + bus.
func (s *Server) SetTenantResolver(r TenantResolver) { s.resolve = r }

// SetTenantAuthorizer enables per-tenant credentials: a request targeting a
// tenant (X-Agezt-Tenant header) may authorize with that tenant's own token
// instead of the daemon admin token. The admin token still authorizes any tenant.
func (s *Server) SetTenantAuthorizer(a TenantAuthorizer) { s.tenantAuth = a }

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
	mux.HandleFunc("/v1/chat/completions", s.auth(s.handleChat))
	mux.HandleFunc("/v1/responses", s.auth(s.handleResponses))
	mux.HandleFunc("/v1/models", s.auth(s.handleModels))
	// OpenAI's "retrieve model" — GET /v1/models/{id}. The list route above is an
	// EXACT match, so without this subtree handler a single-model GET (what the
	// official SDKs' models.retrieve(id) issues for capability probing) would 404.
	mux.HandleFunc("/v1/models/", s.auth(s.handleModelByID))
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeErr(w, http.StatusUnauthorized, "invalid_api_key", "missing or invalid API key")
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
// header, falling back to the ?token= query param (client convenience).
func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return r.URL.Query().Get("token")
}

// --- /v1/models ---

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	eng, _, err := s.bind(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ids := eng.ModelIDs()
	seen := map[string]bool{}
	data := make([]map[string]any, 0, len(ids)+1)
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		data = append(data, map[string]any{
			"id": id, "object": "model", "created": 0, "owned_by": "agezt",
		})
	}
	add(eng.DefaultModel())
	for _, id := range ids {
		add(id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// --- GET /v1/models/{id} (OpenAI "retrieve model") ---
//
// Answers the OpenAI shape: the model object when {id} is one the engine can
// route (the same set /v1/models lists — default model + catalog ids), else a
// 404 with an OpenAI-shaped error so a client distinguishes "unknown model"
// from "route missing". The id may itself contain slashes (provider-prefixed
// ids like "anthropic/claude-..."), so everything after the prefix is the id.
func (s *Server) handleModelByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/models/"), "/")
	if decoded, err := url.PathUnescape(id); err == nil {
		id = decoded
	}
	if id == "" {
		writeErr(w, http.StatusNotFound, "invalid_request_error", "a model id is required: GET /v1/models/{id}")
		return
	}
	eng, _, err := s.bind(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if !modelRoutable(eng, id) {
		writeErr(w, http.StatusNotFound, "invalid_request_error",
			fmt.Sprintf("model %q does not exist or is not routable", id))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "object": "model", "created": 0, "owned_by": "agezt",
	})
}

// modelRoutable reports whether id is in the set /v1/models advertises: the
// engine's default model plus the catalog ids it can route. Kept in lockstep
// with handleModels so a retrieve never disagrees with the list.
func modelRoutable(eng Engine, id string) bool {
	if id == eng.DefaultModel() {
		return true
	}
	for _, m := range eng.ModelIDs() {
		if m == id {
			return true
		}
	}
	return false
}

// --- /v1/chat/completions ---

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Stream         bool            `json:"stream"`
	StreamOptions  *streamOptions  `json:"stream_options,omitempty"`
	ResponseFormat *chatRespFormat `json:"response_format,omitempty"`
}

// chatRespFormat is OpenAI's response_format request object. We honour
// json_object and json_schema (both mean "structured JSON") by switching the
// run to JSON mode (M314); "text" (or absent) is the default free-form output.
type chatRespFormat struct {
	Type string `json:"type"` // "text" | "json_object" | "json_schema"
}

// wantsJSON reports whether a response_format asks for structured JSON output.
func (f *chatRespFormat) wantsJSON() bool {
	return f != nil && (f.Type == "json_object" || f.Type == "json_schema")
}

// streamOptions mirrors OpenAI's stream_options. IncludeUsage requests a final
// usage-only chunk at the end of a stream (M237) — cost-tracking clients and the
// OpenAI SDK rely on it when set.
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// text flattens OpenAI message content, which is either a plain string or an
// array of typed parts ([{type:"text", text:"..."}]). Non-text parts (images)
// are ignored — Agezt's intent is text.
func (m chatMessage) text() string {
	if len(m.Content) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return strings.TrimSpace(s)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(m.Content, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Text != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(p.Text)
			}
		}
		return strings.TrimSpace(b.String())
	}
	return ""
}

// images extracts image attachment URLs from a message's content parts. OpenAI
// Chat Completions carries an image as {type:"image_url", image_url:{url:...}};
// the url is a data: URL or an http(s) URL. Returns nil for string content or
// a part list with no images.
func (m chatMessage) images() []string {
	if len(m.Content) == 0 {
		return nil
	}
	var parts []struct {
		Type     string `json:"type"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if json.Unmarshal(m.Content, &parts) != nil {
		return nil
	}
	var urls []string
	for _, p := range parts {
		if p.Type == "image_url" && p.ImageURL.URL != "" {
			urls = append(urls, p.ImageURL.URL)
		}
	}
	return urls
}

// inputImages extracts Responses-API `input_image` part URLs. There the
// image_url field is a bare string (the documented Responses shape); some SDKs
// send the Chat-style {url} object, so both are tolerated (M250). This is
// distinct from images(), which reads Chat Completions' `image_url` parts.
func (m chatMessage) inputImages() []string {
	if len(m.Content) == 0 {
		return nil
	}
	var parts []struct {
		Type     string          `json:"type"`
		ImageURL json.RawMessage `json:"image_url"`
	}
	if json.Unmarshal(m.Content, &parts) != nil {
		return nil
	}
	var urls []string
	for _, p := range parts {
		if p.Type != "input_image" || len(p.ImageURL) == 0 {
			continue
		}
		var s string
		if json.Unmarshal(p.ImageURL, &s) == nil && s != "" {
			urls = append(urls, s)
			continue
		}
		var o struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(p.ImageURL, &o) == nil && o.URL != "" {
			urls = append(urls, o.URL)
		}
	}
	return urls
}

// imagesFromMessages collects image attachment URLs from the user messages so a
// multimodal chat completion forwards its images to the run; Agezt's providers
// turn each into the model's native image input (M246). The kernel still gates
// the model's vision capability at the provider call.
func imagesFromMessages(msgs []chatMessage) []string {
	var urls []string
	for _, m := range msgs {
		if strings.EqualFold(m.Role, "user") {
			urls = append(urls, m.images()...)
		}
	}
	return urls
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	var req chatRequest
	if !decodeBody(w, r, &req) {
		return
	}
	intent := intentFromMessages(req.Messages)
	images := imagesFromMessages(req.Messages)
	if intent == "" && len(images) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", "no usable message content")
		return
	}
	if intent == "" {
		// Image-only request (image_url parts, no text). Give the model a
		// minimal instruction so the run has an intent.
		intent = "Describe the attached image(s)."
	}
	eng, b, err := s.bind(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	model := req.Model
	if model == "" {
		model = eng.DefaultModel()
	}

	jsonMode := req.ResponseFormat.wantsJSON() // M314: honour response_format

	if req.Stream {
		includeUsage := req.StreamOptions != nil && req.StreamOptions.IncludeUsage
		s.streamChat(w, r, eng, b, intent, model, images, includeUsage, jsonMode)
		return
	}

	corr := eng.NewCorrelation()
	// Capture the run's reasoning (M323) the same way streamChat relays tokens:
	// subscribe BEFORE the run so no early delta is missed, run in a goroutine,
	// and accumulate llm.reasoning text live (the events are ephemeral, so a long
	// chain of thought can exceed the buffer if only drained afterward). A failed
	// subscription degrades to a plain run — reasoning is a bonus, never required.
	answer, reasoning, err := s.runCapturingReasoning(r, eng, b, corr, intent, model, images, jsonMode)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	message := map[string]any{"role": "assistant", "content": answer}
	if reasoning != "" {
		// DeepSeek-R1 convention: the chain of thought rides alongside the answer
		// as `reasoning_content`, which compatible clients already render.
		message["reasoning_content"] = reasoning
	}
	id := "chatcmpl-" + ulid.New()
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "object": "chat.completion", "created": time.Now().Unix(),
		"model": model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       message,
			"finish_reason": "stop",
		}},
		"usage": chatUsage(eng, corr, intent, answer),
		// Agezt-specific: the correlation id so callers can `agt why` the run.
		"agezt_correlation_id": corr,
	})
}

// runCapturingReasoning runs the model and, in parallel, accumulates the run's
// llm.reasoning deltas (M323) so the non-streaming chat response can carry
// `reasoning_content`. Mirrors streamChat's no-race subscribe-then-run pattern;
// a subscription failure degrades gracefully to a plain run (empty reasoning).
func (s *Server) runCapturingReasoning(r *http.Request, eng Engine, b *bus.Bus, corr, intent, model string, images []string, jsonMode bool) (answer, reasoning string, err error) {
	type runRes struct {
		answer string
		err    error
	}
	done := make(chan runRes, 1)

	sub, suberr := b.Subscribe(eng.SubjectForRun(corr), 1024)
	if suberr == nil {
		defer sub.Cancel()
	}
	go func() {
		a, e := eng.RunModel(r.Context(), corr, intent, model, images, jsonMode)
		done <- runRes{a, e}
	}()

	if suberr != nil {
		rr := <-done // no subscription → run without reasoning capture
		return rr.answer, "", rr.err
	}

	var rb strings.Builder
	var rr runRes
capture:
	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				rr = <-done
				break capture
			}
			if rt := reasoningText(ev); rt != "" {
				rb.WriteString(rt)
			}
		case rr = <-done:
			// Drain any reasoning still queued before responding.
			for drained := false; !drained; {
				select {
				case ev := <-sub.C:
					if rt := reasoningText(ev); rt != "" {
						rb.WriteString(rt)
					}
				default:
					drained = true
				}
			}
			break capture
		}
	}
	return rr.answer, rb.String(), rr.err
}

// streamChat runs the intent and relays the kernel's llm.token events as
// OpenAI chat.completion.chunk SSE frames. It subscribes to the run subject
// BEFORE starting the run so no early token is missed (the same no-race pattern
// the control plane's handleRun uses).
func (s *Server) streamChat(w http.ResponseWriter, r *http.Request, eng Engine, b *bus.Bus, intent, model string, images []string, includeUsage, jsonMode bool) {
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

	id := "chatcmpl-" + ulid.New()
	created := time.Now().Unix()
	sendChunk := func(delta map[string]any, finish any) {
		frame := map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
		}
		b, _ := json.Marshal(frame)
		_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
		flusher.Flush()
	}

	var full strings.Builder // accumulates the answer for the optional usage chunk
	sendContent := func(txt string) {
		full.WriteString(txt)
		sendChunk(map[string]any{"content": txt}, nil)
	}
	// sendReasoning relays a reasoning delta as a `reasoning_content` delta (M323),
	// the DeepSeek-R1 streaming convention. It does NOT feed `full` — reasoning is
	// not the answer and must not pollute the usage chunk's content estimate.
	sendReasoning := func(txt string) {
		sendChunk(map[string]any{"reasoning_content": txt}, nil)
	}
	// endStream writes the terminal finish chunk, then — if the client requested
	// stream_options.include_usage — a usage-only chunk (choices:[] + usage, the
	// OpenAI shape), then the [DONE] terminator (M237).
	endStream := func(finish string) {
		sendChunk(map[string]any{}, finish)
		if includeUsage {
			frame := map[string]any{
				"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
				"choices": []map[string]any{},
				"usage":   chatUsage(eng, corr, intent, full.String()),
			}
			b, _ := json.Marshal(frame)
			_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}

	// Opening role chunk.
	sendChunk(map[string]any{"role": "assistant"}, nil)

	type res struct {
		err error
	}
	done := make(chan res, 1)
	go func() {
		_, err := eng.RunModel(r.Context(), corr, intent, model, images, jsonMode)
		done <- res{err}
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				endStream("stop")
				return
			}
			if txt := tokenText(ev); txt != "" {
				sendContent(txt)
			}
			if rt := reasoningText(ev); rt != "" {
				sendReasoning(rt)
			}
		case r := <-done:
			// Drain any tokens still queued, then close the stream.
			for drained := false; !drained; {
				select {
				case ev := <-sub.C:
					if txt := tokenText(ev); txt != "" {
						sendContent(txt)
					}
					if rt := reasoningText(ev); rt != "" {
						sendReasoning(rt)
					}
				default:
					drained = true
				}
			}
			finish := "stop"
			if r.err != nil {
				finish = "error"
				sendContent("\n[error: " + r.err.Error() + "]")
			}
			endStream(finish)
			return
		}
	}
}

// intentFromMessages collapses an OpenAI message list into one Agezt intent.
// A single user turn becomes that text verbatim; multi-turn conversations are
// rendered as a labelled transcript. System messages are surfaced as leading
// guidance (the kernel still applies its own system prompt around this).
func intentFromMessages(msgs []chatMessage) string {
	var systems, convo []string
	soleUser := ""
	userTurns := 0
	for _, m := range msgs {
		t := m.text()
		if t == "" {
			continue
		}
		switch strings.ToLower(m.Role) {
		case "system", "developer":
			systems = append(systems, t)
		case "user":
			userTurns++
			soleUser = t
			convo = append(convo, "User: "+t)
		case "assistant":
			convo = append(convo, "Assistant: "+t)
		default:
			convo = append(convo, t)
		}
	}
	var b strings.Builder
	if len(systems) > 0 {
		b.WriteString(strings.Join(systems, "\n"))
		b.WriteString("\n\n")
	}
	// Single user turn → clean intent (no transcript labels).
	if userTurns == 1 && len(convo) == 1 {
		b.WriteString(soleUser)
		return strings.TrimSpace(b.String())
	}
	b.WriteString(strings.Join(convo, "\n"))
	return strings.TrimSpace(b.String())
}

// estimateUsage gives a rough whitespace-token count so clients that read the
// usage block get plausible numbers. It is an estimate, not provider truth
// (SPEC-15 §7.4 reconciles to provider usage for billing elsewhere).
func estimateUsage(prompt, completion string) map[string]any {
	p := len(strings.Fields(prompt))
	c := len(strings.Fields(completion))
	return map[string]any{
		"prompt_tokens": p, "completion_tokens": c, "total_tokens": p + c,
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr emits an OpenAI-shaped error envelope.
func writeErr(w http.ResponseWriter, code int, typ, msg string) {
	writeJSON(w, code, map[string]any{
		"error": map[string]any{"message": msg, "type": typ},
	})
}

// tokenText returns the streamed text delta carried by an llm.token event, or
// "" for any other event (or nil). The kernel publishes assistant token deltas
// as KindLLMToken with a {"text": "..."} payload (agent.go).
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

// reasoningText returns the streamed reasoning delta carried by an llm.reasoning
// event, or "" for any other event (M323). The kernel publishes a reasoning
// model's chain of thought as KindLLMReasoning with a {"text": "..."} payload
// (agent.go, M317). Exposed to OpenAI-compatible clients as `reasoning_content`,
// the DeepSeek-R1 convention many such clients already understand.
func reasoningText(ev *event.Event) string {
	if ev == nil || ev.Kind != event.KindLLMReasoning || len(ev.Payload) == 0 {
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
