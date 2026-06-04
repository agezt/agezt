// SPDX-License-Identifier: MIT

package openaiapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
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

// intentFromResponsesInput collapses a Responses request into one intent by
// reusing the chat message machinery: `instructions` becomes a leading system
// message; a string `input` becomes a single user turn; an array `input` is
// parsed as message items (content parts like {type:"input_text",text} flatten
// via chatMessage.text, which reads the text field regardless of the part type).
func intentFromResponsesInput(req responsesRequest) string {
	return intentFromMessages(responsesMessages(req))
}

// responsesMessages builds the chatMessage list from a Responses request
// (instructions + input), shared by the intent and image extractors.
func responsesMessages(req responsesRequest) []chatMessage {
	var msgs []chatMessage
	if instr := strings.TrimSpace(req.Instructions); instr != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: jsonString(instr)})
	}
	if len(req.Input) > 0 {
		var asString string
		if json.Unmarshal(req.Input, &asString) == nil {
			msgs = append(msgs, chatMessage{Role: "user", Content: jsonString(asString)})
		} else {
			var items []chatMessage
			if json.Unmarshal(req.Input, &items) == nil {
				msgs = append(msgs, items...)
			}
		}
	}
	return msgs
}

// imagesFromResponsesInput collects input_image attachment URLs from the user
// items of a Responses request, so a multimodal Responses call forwards its
// images to the run (M250).
func imagesFromResponsesInput(req responsesRequest) []string {
	var urls []string
	for _, m := range responsesMessages(req) {
		if strings.EqualFold(m.Role, "user") {
			urls = append(urls, m.inputImages()...)
		}
	}
	return urls
}

// responseObject builds a Responses API result object. status is "completed"
// for a finished run; the output carries one assistant message with an
// output_text content part, and output_text mirrors it for SDK convenience.
// When reasoning is non-empty (M324), a `reasoning` output item carrying the
// chain-of-thought summary is prepended — the position and shape the OpenAI
// Responses API uses for reasoning models.
func responseObject(eng Engine, id, model, answer, reasoning, intent, corr, status string) map[string]any {
	msgID := "msg_" + ulid.New()
	output := make([]map[string]any, 0, 2)
	if reasoning != "" {
		output = append(output, map[string]any{
			"id":   "rs_" + ulid.New(),
			"type": "reasoning",
			"summary": []map[string]any{{
				"type": "summary_text",
				"text": reasoning,
			}},
		})
	}
	output = append(output, map[string]any{
		"id":   msgID,
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{{
			"type":        "output_text",
			"text":        answer,
			"annotations": []any{},
		}},
	})
	return map[string]any{
		"id":          id,
		"object":      "response",
		"created_at":  time.Now().Unix(),
		"model":       model,
		"status":      status,
		"output":      output,
		"output_text": answer, // SDK convenience accessor
		"usage":       responsesUsageFor(eng, corr, intent, answer),
		// Agezt-specific: the correlation id so callers can `agt why` the run.
		"agezt_correlation_id": corr,
	}
}

// responsesUsage uses the Responses token field names (input_tokens /
// output_tokens) rather than the chat ones.
func responsesUsage(prompt, completion string) map[string]any {
	p := len(strings.Fields(prompt))
	c := len(strings.Fields(completion))
	return map[string]any{
		"input_tokens": p, "output_tokens": c, "total_tokens": p + c,
	}
}

// responsesUsageFor returns the real provider usage (Responses field names) when
// the engine can report it, else the whitespace estimate.
func responsesUsageFor(eng Engine, corr, prompt, completion string) map[string]any {
	if ur, ok := eng.(UsageReporter); ok {
		if pt, ct, ok := ur.UsageFor(corr); ok {
			return map[string]any{"input_tokens": pt, "output_tokens": ct, "total_tokens": pt + ct}
		}
	}
	return responsesUsage(prompt, completion)
}

// streamResponses relays the run's llm.token events as the Responses SSE event
// sequence. It subscribes BEFORE starting the run so no early token is missed
// (the same no-race pattern as streamChat).
func (s *Server) streamResponses(w http.ResponseWriter, r *http.Request, eng Engine, b *bus.Bus, intent, model string, images []string, jsonMode bool) {
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

	respID := "resp_" + ulid.New()
	msgID := "msg_" + ulid.New()
	seq := 0
	send := func(eventType string, payload map[string]any) {
		payload["type"] = eventType
		payload["sequence_number"] = seq
		seq++
		b, _ := json.Marshal(payload)
		_, _ = w.Write([]byte("event: " + eventType + "\ndata: " + string(b) + "\n\n"))
		flusher.Flush()
	}

	// response.created — a skeleton response, status in_progress.
	send("response.created", map[string]any{
		"response": map[string]any{
			"id": respID, "object": "response", "model": model, "status": "in_progress",
		},
	})

	type result struct {
		answer string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		ans, err := eng.RunModel(r.Context(), corr, intent, model, images, jsonMode)
		done <- result{ans, err}
	}()

	var full strings.Builder
	emitDelta := func(txt string) {
		if txt == "" {
			return
		}
		full.WriteString(txt)
		send("response.output_text.delta", map[string]any{
			"item_id": msgID, "output_index": 0, "content_index": 0, "delta": txt,
		})
	}
	// Reasoning (M324): a reasoning model's chain of thought streams as
	// response.reasoning_summary_text.delta events (the Responses-API convention),
	// distinct from the answer's output_text deltas, and lands in the final
	// response as a `reasoning` output item via responseObject.
	reasoningID := "rs_" + ulid.New()
	var reasoningBuf strings.Builder
	emitReasoning := func(txt string) {
		if txt == "" {
			return
		}
		reasoningBuf.WriteString(txt)
		send("response.reasoning_summary_text.delta", map[string]any{
			"item_id": reasoningID, "output_index": 0, "summary_index": 0, "delta": txt,
		})
	}

	ctx := r.Context()
	finish := func(res result) {
		// Drain any tokens still queued.
		for drained := false; !drained; {
			select {
			case ev := <-sub.C:
				emitDelta(tokenText(ev))
				emitReasoning(reasoningText(ev))
			default:
				drained = true
			}
		}
		if reasoningBuf.Len() > 0 {
			send("response.reasoning_summary_text.done", map[string]any{
				"item_id": reasoningID, "output_index": 0, "summary_index": 0, "text": reasoningBuf.String(),
			})
		}
		// If nothing streamed (non-streaming provider), emit the answer once.
		if full.Len() == 0 && res.answer != "" {
			emitDelta(res.answer)
		}
		send("response.output_text.done", map[string]any{
			"item_id": msgID, "output_index": 0, "content_index": 0, "text": full.String(),
		})
		status := "completed"
		if res.err != nil {
			status = "failed"
		}
		final := responseObject(eng, respID, model, full.String(), reasoningBuf.String(), intent, corr, status)
		if res.err != nil {
			final["error"] = map[string]any{"message": res.err.Error(), "type": "upstream_error"}
			send("response.failed", map[string]any{"response": final})
		} else {
			send("response.completed", map[string]any{"response": final})
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				finish(result{answer: full.String()})
				return
			}
			emitDelta(tokenText(ev))
			emitReasoning(reasoningText(ev))
		case res := <-done:
			finish(res)
			return
		}
	}
}

// jsonString marshals s into a JSON string literal for use as chatMessage
// content (a json.RawMessage holding a quoted string).
func jsonString(s string) json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return b
}
