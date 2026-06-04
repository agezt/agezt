// SPDX-License-Identifier: MIT

package bedrock

// Mistral-on-Bedrock body shape (M1.tt). Mistral models on
// Bedrock use the *chat-format* request shape introduced with
// `mistral-large-2407-v1:0`:
//
//	{
//	  "messages": [
//	    {"role": "system",    "content": "..."},
//	    {"role": "user",      "content": "..."},
//	    {"role": "assistant", "content": "..."}
//	  ],
//	  "max_tokens":  N,
//	  "temperature": ...
//	}
//
// Response shape:
//
//	{
//	  "choices": [{
//	    "message": {"role":"assistant", "content":"..."},
//	    "finish_reason": "stop|length"
//	  }]
//	}
//
// **Tools are NOT supported (yet).** The 2024-11+ Bedrock Mistral
// chat shape technically accepts a `tools` array, but tool-use
// round-trips through Bedrock have a different message-content
// shape than what the agent loop emits today. Out of scope for
// M1.tt; tool-using calls against Mistral models will still get
// the chat-only path (the model can answer, but can't invoke
// tools). Operators wanting full tool use should stick with the
// anthropic.* models on Bedrock.

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agezt/agezt/kernel/agent"
)

type mistralBedrockRequest struct {
	Messages    []mistralMessage `json:"messages"`
	MaxTokens   int              `json:"max_tokens"`
	Temperature float64          `json:"temperature,omitempty"`
}

type mistralMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type mistralBedrockResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	// Bedrock attaches token counts in response headers
	// (`X-Amzn-Bedrock-Input-Token-Count` / `-Output-Token-Count`)
	// rather than inline in the Mistral body. The body decoder leaves
	// Usage at zero; Complete overlays the header counts afterward
	// (M327), so the governor sees real spend.
}

// encodeMistralOnBedrockRequest converts a canonical
// agent.CompletionRequest into the Mistral chat body. System
// prompts are converted to a leading system-role message (Bedrock
// Mistral honours that role; the older `prompt`-string shape did
// not). Tool definitions are dropped — see the file doc-comment.
func encodeMistralOnBedrockRequest(system string, msgs []agent.Message, maxTok int) ([]byte, error) {
	out := mistralBedrockRequest{MaxTokens: maxTok}
	if system != "" {
		out.Messages = append(out.Messages, mistralMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		role := string(m.Role)
		// Canonical agent roles map cleanly to Mistral chat roles
		// (user/assistant/system). Tool-result messages don't have
		// a Mistral equivalent in this shape; surface them as user
		// messages so the model at least sees the content (even
		// though it can't tell they came from a tool round-trip).
		switch role {
		case "user", "assistant", "system":
			// ok
		default:
			role = "user"
		}
		out.Messages = append(out.Messages, mistralMessage{Role: role, Content: m.Content})
	}
	if len(out.Messages) == 0 {
		return nil, errors.New("bedrock-mistral: at least one message required")
	}
	return json.Marshal(out)
}

// decodeMistralOnBedrockResponse converts the Mistral choices array
// into a canonical CompletionResponse. The model id is echoed back
// in Usage so downstream tracking (governor cost accounting, audit
// events) sees the same id the caller specified.
func decodeMistralOnBedrockResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var wire mistralBedrockResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("bedrock-mistral: parse response: %w", err)
	}
	if len(wire.Choices) == 0 {
		return nil, errors.New("bedrock-mistral: response has no choices")
	}
	ch := wire.Choices[0]
	stop := agent.StopEndTurn
	if ch.FinishReason == "length" {
		stop = agent.StopMaxTokens
	}
	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:    agent.Role(ch.Message.Role),
			Content: ch.Message.Content,
		},
		StopReason: stop,
		Usage:      agent.Usage{Model: model},
	}, nil
}
