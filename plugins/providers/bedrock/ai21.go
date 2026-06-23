// SPDX-License-Identifier: MIT

package bedrock

// AI21-Jamba-on-Bedrock body shape (M1.tt-4). AI21's current
// Bedrock SKU is the Jamba family (jamba-1.5-large, jamba-1.5-mini,
// jamba-instruct), which exposes an **OpenAI-compatible chat
// completion** request shape — quite different from AI21's older
// J2 models:
//
//	{
//	  "messages": [
//	    {"role": "system", "content": "..."},
//	    {"role": "user",   "content": "..."}
//	  ],
//	  "max_tokens": N,
//	  "temperature": 0.7
//	}
//
// Response:
//
//	{
//	  "id": "...",
//	  "choices": [{
//	    "index": 0,
//	    "message": {"role": "assistant", "content": "..."},
//	    "finish_reason": "stop"
//	  }],
//	  "usage": {"prompt_tokens": N, "completion_tokens": M, "total_tokens": T}
//	}
//
// **No tool use.** Jamba on Bedrock does support tool calling via
// the OpenAI tool-call shape, but agezt's canonical tool round-trip
// is Anthropic-shaped; bridging that into OpenAI-flavoured
// tool_calls/tool_call_id reuses the openai provider plumbing that
// already exists in plugins/providers/openai. For now AI21 stays
// chat-only on Bedrock to match the policy applied to Mistral,
// Cohere, and Meta-Llama.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
)

type ai21JambaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ai21JambaRequest struct {
	Messages  []ai21JambaMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
	// Per-request sampling knobs (M997). AI21 Jamba's OpenAI-compatible chat
	// shape accepts temperature/top_p/stop; it has no top_k, seed, or penalties
	// on Bedrock. Nil-able so an unset Params is a no-op.
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// applyParams copies the sampling knobs Jamba understands; an unset Params
// leaves the request unchanged. top_k/seed/penalties/ReasoningEffort are not in
// the Bedrock-Jamba shape and are ignored.
func (wire *ai21JambaRequest) applyParams(p agent.Params) {
	if p.IsZero() {
		return
	}
	wire.Temperature = p.Temperature
	wire.TopP = p.TopP
	wire.Stop = p.Stop
}

type ai21JambaChoice struct {
	Index        int              `json:"index"`
	Message      ai21JambaMessage `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

type ai21JambaUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ai21JambaResponse struct {
	ID      string            `json:"id"`
	Choices []ai21JambaChoice `json:"choices"`
	Usage   ai21JambaUsage    `json:"usage"`
}

func ai21JambaRole(r agent.Role) string {
	switch r {
	case agent.RoleAssistant:
		return "assistant"
	case agent.RoleSystem:
		return "system"
	case agent.RoleTool:
		// AI21 has no tool-result role; surface tool output as a
		// user-role turn so the model still sees it. Bedrock's
		// shape would reject "tool" role.
		return "user"
	}
	return "user"
}

func encodeAI21JambaOnBedrockRequest(system string, msgs []agent.Message, maxTok int, params agent.Params, extra json.RawMessage) ([]byte, error) {
	if len(msgs) == 0 {
		return nil, errors.New("bedrock-ai21: at least one message required")
	}
	out := ai21JambaRequest{MaxTokens: maxTok}
	out.applyParams(params)
	if system != "" {
		out.Messages = append(out.Messages, ai21JambaMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		out.Messages = append(out.Messages, ai21JambaMessage{
			Role:    ai21JambaRole(m.Role),
			Content: m.Content,
		})
	}
	body, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return provopts.Merge(body, extra)
}

func decodeAI21JambaOnBedrockResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var wire ai21JambaResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("bedrock-ai21: parse response: %w", err)
	}
	if len(wire.Choices) == 0 {
		return nil, errors.New("bedrock-ai21: response has no choices")
	}
	choice := wire.Choices[0]
	if choice.Message.Content == "" {
		return nil, errors.New("bedrock-ai21: choice has empty content")
	}
	stop := agent.StopEndTurn
	// OpenAI-style finish_reason: "stop" | "length" | "tool_calls" | "content_filter"
	switch strings.ToLower(choice.FinishReason) {
	case "length":
		stop = agent.StopMaxTokens
	}
	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: choice.Message.Content,
		},
		StopReason: stop,
		Usage: agent.Usage{
			Model:        model,
			InputTokens:  wire.Usage.PromptTokens,
			OutputTokens: wire.Usage.CompletionTokens,
		},
	}, nil
}

// isAI21JambaModel reports whether the Bedrock model id maps to the
// AI21 Jamba body shape (M1.tt-4). Covers direct ids
// (`ai21.jamba-*`) and regional cross-inference profiles.
//
// **Why "jamba" and not bare "ai21".** AI21 also has legacy J2
// models on Bedrock (`ai21.j2-mid-v1`, `ai21.j2-ultra-v1`) which
// use a completely different request shape (`prompt` + `maxTokens`
// camelCase). The Jamba shape doesn't fit those; rather than ship
// half a J2 implementation, we explicitly route only Jamba ids
// here and let J2 fall into the unsupported-vendor error. The J2
// SKU is being deprecated by AI21, so this is unlikely to matter.
func isAI21JambaModel(id string) bool {
	// Direct ids: ai21.jamba-1-5-large-v1:0, ai21.jamba-instruct-v1:0, etc.
	if strings.HasPrefix(id, "ai21.jamba") {
		return true
	}
	// Regional cross-inference profile: us.ai21.jamba-..., eu.ai21.jamba-...
	return strings.Contains(id, ".ai21.jamba")
}
