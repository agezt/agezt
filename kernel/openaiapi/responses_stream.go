// SPDX-License-Identifier: MIT
//
// /v1/responses SSE streaming handler. Split from responses.go during
// Day 211 god-file refactor (#31). Public API unchanged.
package openaiapi

import (
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/httpserver"
	"github.com/agezt/agezt/kernel/ulid"
)

// streamResponses relays the run's llm.token events as the Responses SSE event
// sequence. It subscribes BEFORE starting the run so no early token is missed
// (the same no-race pattern as streamChat).
func (s *Server) streamResponses(w http.ResponseWriter, r *http.Request, eng Engine, b *bus.Bus, intent, model string, images []string, jsonMode bool) {
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

	respID := "resp_" + ulid.New()
	msgID := "msg_" + ulid.New()
	seq := 0
	send := func(eventType string, payload map[string]any) {
		payload["type"] = eventType
		payload["sequence_number"] = seq
		seq++
		_ = sse.WriteEvent(eventType, payload)
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
			final["error"] = map[string]any{"message": redactErr(res.err.Error()), "type": "upstream_error"}
			send("response.failed", map[string]any{"response": final})
		} else {
			send("response.completed", map[string]any{"response": final})
		}
		_ = sse.WriteData("[DONE]")
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
