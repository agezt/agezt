// SPDX-License-Identifier: MIT

package cohere

// Cohere DECODE side: decodeResponse. Carved out of cohere.go
// during the Day 191 god-file split so the main file can stay
// focused on the Provider dispatch + shared wire types, and the
// encode file can stay focused on request construction.
// Public API unchanged.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)

func decodeResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var cr cohereResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return nil, fmt.Errorf("cohere: parse response: %w", err)
	}

	// content can be a plain string or an array of typed blocks
	// ({type:"text", text}). Try string first; on failure, decode as array.
	var text string
	if len(cr.Message.Content) > 0 {
		var asString string
		if err := json.Unmarshal(cr.Message.Content, &asString); err == nil {
			text = asString
		} else {
			var blocks []cohereContentBlock
			if err := json.Unmarshal(cr.Message.Content, &blocks); err != nil {
				return nil, fmt.Errorf("cohere: message.content not string-or-blocks: %w", err)
			}
			var parts []string
			for _, b := range blocks {
				if b.Type == "text" {
					parts = append(parts, b.Text)
				}
			}
			text = strings.Join(parts, "")
		}
	}

	var toolCalls []agent.ToolCall
	for i, tc := range cr.Message.ToolCalls {
		id := tc.ID
		if id == "" {
			id = "call-" + strconv.Itoa(i)
		}
		args := strings.TrimSpace(tc.Function.Arguments)
		if args == "" {
			args = "{}"
		}
		toolCalls = append(toolCalls, agent.ToolCall{
			ID:    id,
			Name:  tc.Function.Name,
			Input: json.RawMessage(args),
		})
	}

	stop := agent.StopEndTurn
	switch strings.ToUpper(cr.FinishReason) {
	case "COMPLETE", "STOP_SEQUENCE", "":
		stop = agent.StopEndTurn
	case "MAX_TOKENS":
		stop = agent.StopMaxTokens
	case "TOOL_CALL":
		stop = agent.StopToolUse
	}
	if len(toolCalls) > 0 && stop == agent.StopEndTurn {
		stop = agent.StopToolUse
	}

	usage := agent.Usage{Model: model}
	if cr.Usage != nil && cr.Usage.Tokens != nil {
		usage.InputTokens = cr.Usage.Tokens.InputTokens
		usage.OutputTokens = cr.Usage.Tokens.OutputTokens
	}

	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			Content:   text,
			ToolCalls: toolCalls,
		},
		StopReason: stop,
		Usage:      usage,
	}, nil
}

