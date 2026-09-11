// SPDX-License-Identifier: MIT

// Server surface: New + SetTenantResolver/Authorizer + SetTranscriber + bind + Handler + handleTranscription + handleModels + handleModelByID + modelRoutable + chatMessage text/images/inputImages + imagesFromMessages + handleChat.
// Code extracted from openaiapi.go during the Day-49 god-file split. Public API unchanged.
package openaiapi


import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/httpserver"
)


func New(eng Engine, b *bus.Bus, token string) *Server {
	return &Server{
		eng:      eng,
		bus:      b,
		verifier: kernelauth.NewStaticVerifier(token),
	}
}

// SetTenantResolver enables tenant routing: requests carrying an X-Agezt-Tenant
// header are served by the resolved per-tenant Engine + bus.
func (s *Server) SetTenantResolver(r TenantResolver) { s.resolve = r }

// SetTenantAuthorizer enables per-tenant credentials: a request targeting a
// tenant (X-Agezt-Tenant header) may authorize with that tenant's own token
// instead of the daemon admin token. The admin token still authorizes any tenant.
func (s *Server) SetTenantAuthorizer(a TenantAuthorizer) { s.tenantAuth = a }

// SetTranscriber wires the speech-to-text backend for POST
// /v1/audio/transcriptions. Without it, that route reports "not configured".
func (s *Server) SetTranscriber(t Transcriber) { s.transcriber = t }

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
	authenticator := httpserver.Authenticator{
		Verifier:        s.verifier,
		TenantAuthorize: httpserver.TenantAuthorizer(s.tenantAuth),
	}
	router := httpserver.NewRouter(authenticator, func(w http.ResponseWriter, _ *http.Request) {
		writeErr(w, http.StatusUnauthorized, "invalid_api_key", "missing or invalid API key")
	})
	jsonRoute := httpserver.RouteOpts{
		Tier:     kernelauth.TierUser,
		Method:   http.MethodPost,
		BodyMax:  maxRequestBodyBytes,
		Mutation: true,
	}
	readRoute := httpserver.RouteOpts{Tier: kernelauth.TierUser, Method: http.MethodGet}
	router.Handle("/v1/chat/completions", jsonRoute, s.handleChat)
	router.Handle("/v1/responses", jsonRoute, s.handleResponses)
	router.Handle("/v1/models", readRoute, s.handleModels)
	// OpenAI's "retrieve model" — GET /v1/models/{id}. The list route above is an
	// EXACT match, so without this subtree handler a single-model GET (what the
	// official SDKs' models.retrieve(id) issues for capability probing) would 404.
	router.Handle("/v1/models/", readRoute, s.handleModelByID)
	router.Handle("/v1/audio/transcriptions", httpserver.RouteOpts{
		Tier:     kernelauth.TierUser,
		Method:   http.MethodPost,
		BodyMax:  audioMaxBytes,
		Mutation: true,
	}, s.handleTranscription)
	return router
}

// audioMaxBytes bounds an uploaded audio body (OpenAI's own cap is 25 MiB).
const audioMaxBytes = 25 << 20

// handleTranscription implements POST /v1/audio/transcriptions — the
// OpenAI-compatible speech-to-text upload. It reads the multipart `file`, hands
// it to the configured STT backend, and returns `{"text": …}`, so any OpenAI
// audio client can upload to Agezt and get a transcript (the "HTTP upload" voice
// source, complementing `agt transcribe` and `agt listen`).
func (s *Server) handleTranscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if s.transcriber == nil {
		writeErr(w, http.StatusNotImplemented, "not_configured",
			"speech-to-text is not configured: set AGEZT_STT_API_URL + AGEZT_STT_API_KEY")
		return
	}
	if err := r.ParseMultipartForm(audioMaxBytes); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", "expected multipart/form-data with a 'file' field: "+err.Error())
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", "missing 'file' form field")
		return
	}
	defer f.Close()
	audio, err := io.ReadAll(f)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", "read upload: "+err.Error())
		return
	}
	filename := hdr.Filename
	if filename == "" {
		filename = "audio.wav"
	}
	text, err := s.transcriber.Transcribe(r.Context(), filename, audio)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "stt_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text})
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
