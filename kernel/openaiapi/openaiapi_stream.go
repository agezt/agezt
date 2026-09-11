// SPDX-License-Identifier: MIT

// Streaming + helpers: runCapturingReasoning + streamChat + intentFromMessages + estimateUsage + writeJSON + writeErr + tokenText + reasoningText.
// Code extracted from openaiapi.go during the Day-49 god-file split. Public API unchanged.
package openaiapi


import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/convo"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/httpserver"
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

	id := "chatcmpl-" + ulid.New()
	created := time.Now().Unix()
	sendChunk := func(delta map[string]any, finish any) {
		_ = sse.WriteJSON(map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
		})
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
			_ = sse.WriteJSON(map[string]any{
				"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
				"choices": []map[string]any{},
				"usage":   chatUsage(eng, corr, intent, full.String()),
			})
		}
		_ = sse.WriteData("[DONE]")
	}

	// Opening role chunk.
	sendChunk(map[string]any{"role": "assistant"}, nil)

	type res struct {
		ans string
		err error
	}
	done := make(chan res, 1)
	go func() {
		ans, err := eng.RunModel(r.Context(), corr, intent, model, images, jsonMode)
		done <- res{ans, err}
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
				sendContent("\n[error: " + redactErr(r.err.Error()) + "]")
			} else if full.Len() == 0 && r.ans != "" {
				// Defensive: the run produced an answer but emitted no llm.token
				// events — i.e. the provider is non-streaming (it satisfies
				// Complete but not StreamingProvider, e.g. a provider without a
				// native stream). Without this, a stream:true request to such a
				// provider would return only the role + stop chunks and silently
				// drop the answer, while the same provider via non-stream chat
				// returns it. Emit the assembled answer as one content delta so the
				// streamed and non-streamed paths agree.
				sendContent(r.ans)
			}
			endStream(finish)
			return
		}
	}
}

// intentFromMessages collapses an OpenAI message list into one Agezt intent via
// the shared convo mapping, so this API and the Web UI Chat view render prior
// turns identically. A single user turn becomes that text verbatim; multi-turn
// conversations are rendered as a labelled transcript with system guidance
// hoisted to the front (the kernel still applies its own system prompt around it).
func intentFromMessages(msgs []chatMessage) string {
	turns := make([]convo.Turn, 0, len(msgs))
	for _, m := range msgs {
		turns = append(turns, convo.Turn{Role: m.Role, Text: m.text()})
	}
	return convo.TranscriptIntent(turns)
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
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr emits an OpenAI-shaped error envelope.
func writeErr(w http.ResponseWriter, code int, typ, msg string) {
	writeJSON(w, code, map[string]any{
		"error": map[string]any{"message": redactErr(msg), "type": typ},
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
