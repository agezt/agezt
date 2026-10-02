// SPDX-License-Identifier: MIT

package vertex

// Anthropic-on-Vertex DECODE side: decodeAnthropicOnVertexResponse.
// Carved out of anthropic.go during the Day 187 god-file split so the
// main file can stay focused on the Provider dispatch (Resolve*/
// Complete) + shared wire types, and the encode file can stay focused
// on request construction.
// Public API unchanged.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/contract/llm"
)

func decodeAnthropicOnVertexResponse(body []byte, model string) (*llm.CompletionResponse, error) {
	var ar anthVxResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("vertex: parse anthropic response: %w", err)
	}
	var (
		textParts      []string
		reasoningParts []string
		toolCalls      []llm.ToolCall
	)
	for _, b := range ar.Content {
		switch b.Type {
		case "text":
			textParts = append(textParts, b.Text)
		case "thinking":
			// Extended-thinking block (M321) → reasoning, kept out of the answer.
			reasoningParts = append(reasoningParts, b.Thinking)
		case "tool_use":
			input := b.Input
			if len(input) == 0 {
				input = json.RawMessage(`{}`)
			}
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:    b.ID,
				Name:  b.Name,
				Input: input,
			})
		}
	}
	stop := llm.StopReason(ar.StopReason)
	switch ar.StopReason {
	case "end_turn", "stop_sequence":
		stop = llm.StopEndTurn
	case "tool_use":
		stop = llm.StopToolUse
	case "max_tokens":
		stop = llm.StopMaxTokens
	}
	return &llm.CompletionResponse{
		Message: llm.Message{
			Role:      llm.RoleAssistant,
			Content:   strings.Join(textParts, ""),
			ToolCalls: toolCalls,
		},
		StopReason: stop,
		Usage: anthVxUsageToAgent(
			ar.Usage.InputTokens, ar.Usage.CacheReadInputTokens,
			ar.Usage.CacheCreationInputTokens, ar.Usage.OutputTokens, model),
		ReasoningContent: strings.Join(reasoningParts, ""),
	}, nil
}

// ----- HTTP execution -----
