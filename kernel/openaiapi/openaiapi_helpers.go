// SPDX-License-Identifier: MIT
//
// /v1 OpenAI-compatible endpoint helpers: intent extraction (intentFromMessages),
// rough usage estimator (estimateUsage), JSON/error envelope writers, and the
// event-payload text extractors for streamed tokens and reasoning deltas.
// Split from openaiapi_stream.go during Day 211 god-file refactor (#32).
// Public API unchanged.
package openaiapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/convo"
	"github.com/agezt/agezt/kernel/event"
)

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
