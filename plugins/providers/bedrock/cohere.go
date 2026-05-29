// SPDX-License-Identifier: MIT

package bedrock

// Cohere-on-Bedrock body shape (M1.tt-2). Cohere Command R / R+
// models use a `message` + `chat_history` request shape distinct
// from both Anthropic Messages and Mistral chat:
//
//	{
//	  "message": "<latest user turn>",
//	  "chat_history": [
//	    {"role": "USER",      "message": "..."},
//	    {"role": "CHATBOT",   "message": "..."}
//	  ],
//	  "preamble": "<system prompt>",
//	  "max_tokens": N
//	}
//
// Response shape:
//
//	{
//	  "text": "<assistant response>",
//	  "finish_reason": "COMPLETE|MAX_TOKENS|...",
//	  "generation_id": "..."
//	}
//
// Note the uppercase role values (USER / CHATBOT) and the
// `preamble` key for system prompts. These differ from every
// other vendor we wire — that's why each vendor needs its own
// encode/decode pair rather than a generic shim.
//
// **Tool use NOT wired (yet).** Cohere R+ supports a `tools` +
// `tool_results` shape but it differs from both Anthropic and
// Mistral's; tool-using calls fall back to chat-only behaviour
// (the model can answer but can't invoke tools through agezt).
// Operators wanting tool use should stick with anthropic.* on
// Bedrock.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ersinkoc/agezt/kernel/agent"
)

type cohereBedrockRequest struct {
	Message     string               `json:"message"`
	ChatHistory []cohereHistoryEntry `json:"chat_history,omitempty"`
	Preamble    string               `json:"preamble,omitempty"`
	MaxTokens   int                  `json:"max_tokens"`
}

type cohereHistoryEntry struct {
	Role    string `json:"role"`
	Message string `json:"message"`
}

type cohereBedrockResponse struct {
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason"`
	GenerationID string `json:"generation_id,omitempty"`
}

// encodeCohereOnBedrockRequest converts canonical messages to the
// Cohere chat shape. Splits the message list at the last user
// turn — everything before is `chat_history`, the last user turn
// becomes the standalone `message` field.
func encodeCohereOnBedrockRequest(system string, msgs []agent.Message, maxTok int) ([]byte, error) {
	if len(msgs) == 0 {
		return nil, errors.New("bedrock-cohere: at least one message required")
	}
	// Find the last user message — that's the prompt; earlier
	// turns form chat_history.
	lastUserIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == agent.RoleUser {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx < 0 {
		return nil, errors.New("bedrock-cohere: messages must include at least one user turn")
	}
	out := cohereBedrockRequest{
		Message:   msgs[lastUserIdx].Content,
		Preamble:  system,
		MaxTokens: maxTok,
	}
	for i := range lastUserIdx {
		m := msgs[i]
		out.ChatHistory = append(out.ChatHistory, cohereHistoryEntry{
			Role:    cohereRole(m.Role),
			Message: m.Content,
		})
	}
	return json.Marshal(out)
}

// cohereRole maps canonical agent roles to Cohere's uppercase
// scheme. Unknown roles fold to USER so the model at least sees
// the content.
func cohereRole(r agent.Role) string {
	switch r {
	case agent.RoleAssistant:
		return "CHATBOT"
	case agent.RoleUser:
		return "USER"
	case agent.RoleSystem:
		// Cohere uses `preamble` for system; a system message
		// appearing mid-history is unusual but we fold it to USER
		// rather than dropping it.
		return "USER"
	}
	return "USER"
}

func decodeCohereOnBedrockResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var wire cohereBedrockResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("bedrock-cohere: parse response: %w", err)
	}
	if wire.Text == "" {
		return nil, errors.New("bedrock-cohere: response has empty text")
	}
	stop := agent.StopEndTurn
	if strings.EqualFold(wire.FinishReason, "MAX_TOKENS") {
		stop = agent.StopMaxTokens
	}
	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: wire.Text,
		},
		StopReason: stop,
		Usage:      agent.Usage{Model: model},
	}, nil
}
