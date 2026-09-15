// SPDX-License-Identifier: MIT
//
// /v1/chat/completions entry-point handler. Split from openaiapi_stream.go
// during Day 211 god-file refactor (#32).
// Public API unchanged.
package openaiapi

import (
	"net/http"
	"time"

	"github.com/agezt/agezt/kernel/ulid"
)

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
