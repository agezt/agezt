// SPDX-License-Identifier: MIT

package bedrock

// Meta-Llama-on-Bedrock body shape (M1.tt-3). Llama 3 / 3.1 / 3.2
// on Bedrock use the prompt-template format — there's no
// structured `messages` array; the caller assembles the special
// tokens into a single `prompt` string and Bedrock relays it
// verbatim to the model:
//
//	{
//	  "prompt":       "<|begin_of_text|>...<|eot_id|>",
//	  "max_gen_len":  N,
//	  "temperature": ...
//	}
//
// Response:
//
//	{
//	  "generation": "<assistant text>",
//	  "stop_reason": "stop|length",
//	  "prompt_token_count":     N,
//	  "generation_token_count": M
//	}
//
// **No tool use.** The Llama prompt template doesn't have a
// generic tool-use protocol that maps cleanly to agezt's
// canonical tool round-trips. Chat-only.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ersinkoc/agezt/kernel/agent"
)

type llamaBedrockRequest struct {
	Prompt    string `json:"prompt"`
	MaxGenLen int    `json:"max_gen_len"`
}

type llamaBedrockResponse struct {
	Generation           string `json:"generation"`
	StopReason           string `json:"stop_reason"`
	PromptTokenCount     int    `json:"prompt_token_count"`
	GenerationTokenCount int    `json:"generation_token_count"`
}

// llama3Template assembles the system + messages into the Llama
// 3.x chat prompt template. Each turn is wrapped in
// `<|start_header_id|>ROLE<|end_header_id|>\n\nBODY<|eot_id|>`.
// The prompt ends with an open assistant header so the model
// generates the next response.
//
// Older Llama 2 used a different template (`<s>[INST] ... [/INST]`);
// since 3.x is what's broadly available on Bedrock today we
// target only that. Operators on Llama 2 should pin to the older
// model id and accept that the v2 template isn't covered.
func llama3Template(system string, msgs []agent.Message) string {
	var sb strings.Builder
	sb.WriteString("<|begin_of_text|>")
	if system != "" {
		sb.WriteString("<|start_header_id|>system<|end_header_id|>\n\n")
		sb.WriteString(system)
		sb.WriteString("<|eot_id|>")
	}
	for _, m := range msgs {
		role := llamaRole(m.Role)
		sb.WriteString("<|start_header_id|>")
		sb.WriteString(role)
		sb.WriteString("<|end_header_id|>\n\n")
		sb.WriteString(m.Content)
		sb.WriteString("<|eot_id|>")
	}
	// Open assistant header so the model continues.
	sb.WriteString("<|start_header_id|>assistant<|end_header_id|>\n\n")
	return sb.String()
}

func llamaRole(r agent.Role) string {
	switch r {
	case agent.RoleAssistant:
		return "assistant"
	case agent.RoleUser:
		return "user"
	case agent.RoleSystem:
		return "system"
	}
	return "user"
}

func encodeMetaLlamaOnBedrockRequest(system string, msgs []agent.Message, maxTok int) ([]byte, error) {
	if len(msgs) == 0 {
		return nil, errors.New("bedrock-llama: at least one message required")
	}
	out := llamaBedrockRequest{
		Prompt:    llama3Template(system, msgs),
		MaxGenLen: maxTok,
	}
	return json.Marshal(out)
}

func decodeMetaLlamaOnBedrockResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var wire llamaBedrockResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("bedrock-llama: parse response: %w", err)
	}
	if wire.Generation == "" {
		return nil, errors.New("bedrock-llama: response generation empty")
	}
	stop := agent.StopEndTurn
	if strings.EqualFold(wire.StopReason, "length") {
		stop = agent.StopMaxTokens
	}
	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: wire.Generation,
		},
		StopReason: stop,
		Usage: agent.Usage{
			Model:        model,
			InputTokens:  wire.PromptTokenCount,
			OutputTokens: wire.GenerationTokenCount,
		},
	}, nil
}
