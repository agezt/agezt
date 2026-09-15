// SPDX-License-Identifier: MIT
//
// /v1/responses intent extraction, response object builders, usage helpers,
// and the jsonString RawMessage helper. Split from responses.go during
// Day 211 god-file refactor (#31). Public API unchanged.
package openaiapi

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/ulid"
)

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

// jsonString marshals s into a JSON string literal for use as chatMessage
// content (a json.RawMessage holding a quoted string).
func jsonString(s string) json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return b
}
