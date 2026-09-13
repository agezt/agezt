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

	"github.com/agezt/agezt/kernel/agent"
)

func decodeAnthropicOnVertexResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var ar anthVxResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("vertex: parse anthropic response: %w", err)
	}
	var (
		textParts      []string
		reasoningParts []string
		toolCalls      []agent.ToolCall
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
			toolCalls = append(toolCalls, agent.ToolCall{
				ID:    b.ID,
				Name:  b.Name,
				Input: input,
			})
		}
	}
	stop := agent.StopReason(ar.StopReason)
	switch ar.StopReason {
	case "end_turn", "stop_sequence":
		stop = agent.StopEndTurn
	case "tool_use":
		stop = agent.StopToolUse
	case "max_tokens":
		stop = agent.StopMaxTokens
	}
	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
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

