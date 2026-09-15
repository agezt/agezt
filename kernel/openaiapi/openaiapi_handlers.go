// SPDX-License-Identifier: MIT
//
// Server handlers: handleTranscription + handleModels + handleModelByID +
// modelRoutable. Split from openaiapi_server.go during Day 211 god-file
// refactor (#35). Public API unchanged.
package openaiapi

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

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
