// SPDX-License-Identifier: MIT
//
// /v1/responses request types + entry-point handler. Split from responses.go
// during Day 211 god-file refactor (#31).
// Public API unchanged.
package openaiapi

import (
	"encoding/json"
	"net/http"

	"github.com/agezt/agezt/kernel/ulid"
)


// This file adds POST /v1/responses — OpenAI's newer Responses API surface
// (alongside the Chat Completions API in openaiapi.go). It runs through the
// exact same governed kernel loop; only the request/response shapes differ.
//
// Mapping: the Responses `input` (a plain string or an array of message items)
// plus the top-level `instructions` collapse into one Agezt intent via the same
// intentFromMessages used for chat. Streaming maps the kernel's llm.token events
// to the Responses SSE event sequence (response.created →
// response.output_text.delta* → response.output_text.done →
// response.completed).

type responsesRequest struct {
	Model        string          `json:"model"`
	Input        json.RawMessage `json:"input"`
	Instructions string          `json:"instructions"`
	Stream       bool            `json:"stream"`
	// Structured output (M315): the Responses API carries it as
	// text.format.type; some SDKs also send a top-level response_format. We
	// honour either → JSON mode on the run.
	Text           *responsesText  `json:"text,omitempty"`
	ResponseFormat *chatRespFormat `json:"response_format,omitempty"`
}

type responsesText struct {
	Format *chatRespFormat `json:"format,omitempty"`
}

// wantsJSON reports whether the request asks for structured JSON output via
// either the Responses-API text.format or a top-level response_format.
func (r *responsesRequest) wantsJSON() bool {
	if r.ResponseFormat.wantsJSON() {
		return true
	}
	return r.Text != nil && r.Text.Format.wantsJSON()
}

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	var req responsesRequest
	if !decodeBody(w, r, &req) {
		return
	}
	intent := intentFromResponsesInput(req)
	images := imagesFromResponsesInput(req)
	if intent == "" && len(images) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", "no usable input content")
		return
	}
	if intent == "" {
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

	jsonMode := req.wantsJSON() // M315: honour text.format / response_format

	if req.Stream {
		s.streamResponses(w, r, eng, b, intent, model, images, jsonMode)
		return
	}

	corr := eng.NewCorrelation()
	// Capture the run's reasoning (M324) the same way the chat handler does, so a
	// reasoning model's chain of thought surfaces as a `reasoning` output item.
	answer, reasoning, err := s.runCapturingReasoning(r, eng, b, corr, intent, model, images, jsonMode)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	id := "resp_" + ulid.New()
	writeJSON(w, http.StatusOK, responseObject(eng, id, model, answer, reasoning, intent, corr, "completed"))
}
