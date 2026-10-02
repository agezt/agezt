// SPDX-License-Identifier: MIT

package ollama

// Ollama DECODE side: decodeResponse. Carved out of ollama.go
// during the Day 193 god-file split so the main file can stay
// focused on the Provider dispatch + shared wire types, and the
// encode file can stay focused on request construction.
// Public API unchanged.

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/agezt/agezt/kernel/contract/llm"
)

func decodeResponse(body []byte) (*llm.CompletionResponse, error) {
	var or ollamaResponse
	if err := json.Unmarshal(body, &or); err != nil {
		return nil, fmt.Errorf("ollama: parse response: %w", err)
	}

	var toolCalls []llm.ToolCall
	for i, tc := range or.Message.ToolCalls {
		id := tc.ID
		if id == "" {
			id = "call-" + strconv.Itoa(i)
		}
		args := tc.Function.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		toolCalls = append(toolCalls, llm.ToolCall{
			ID:    id,
			Name:  tc.Function.Name,
			Input: args,
		})
	}

	// Ollama's stop reason is less standardised than Anthropic's. Use
	// tool_calls presence as a strong signal first; fall back to
	// done_reason mapping.
	var stop llm.StopReason
	switch {
	case len(toolCalls) > 0:
		stop = llm.StopToolUse
	case or.DoneReason == "length":
		stop = llm.StopMaxTokens
	default:
		stop = llm.StopEndTurn
	}

	return &llm.CompletionResponse{
		Message: llm.Message{
			Role:      llm.RoleAssistant,
			Content:   or.Message.Content,
			ToolCalls: toolCalls,
		},
		StopReason: stop,
		Usage: llm.Usage{
			InputTokens:  or.PromptEvalCount,
			OutputTokens: or.EvalCount,
			Model:        or.Model,
		},
	}, nil
}
