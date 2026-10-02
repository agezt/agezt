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

	"github.com/agezt/agezt/kernel/contract/llm"
)

func decodeResponse(body []byte, model string) (*llm.CompletionResponse, error) {
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

	var toolCalls []llm.ToolCall
	for i, tc := range cr.Message.ToolCalls {
		id := tc.ID
		if id == "" {
			id = "call-" + strconv.Itoa(i)
		}
		args := strings.TrimSpace(tc.Function.Arguments)
		if args == "" {
			args = "{}"
		}
		toolCalls = append(toolCalls, llm.ToolCall{
			ID:    id,
			Name:  tc.Function.Name,
			Input: json.RawMessage(args),
		})
	}

	stop := llm.StopEndTurn
	switch strings.ToUpper(cr.FinishReason) {
	case "COMPLETE", "STOP_SEQUENCE", "":
		stop = llm.StopEndTurn
	case "MAX_TOKENS":
		stop = llm.StopMaxTokens
	case "TOOL_CALL":
		stop = llm.StopToolUse
	}
	if len(toolCalls) > 0 && stop == llm.StopEndTurn {
		stop = llm.StopToolUse
	}

	usage := llm.Usage{Model: model}
	if cr.Usage != nil && cr.Usage.Tokens != nil {
		usage.InputTokens = cr.Usage.Tokens.InputTokens
		usage.OutputTokens = cr.Usage.Tokens.OutputTokens
	}

	return &llm.CompletionResponse{
		Message: llm.Message{
			Role:      llm.RoleAssistant,
			Content:   text,
			ToolCalls: toolCalls,
		},
		StopReason: stop,
		Usage:      usage,
	}, nil
}
