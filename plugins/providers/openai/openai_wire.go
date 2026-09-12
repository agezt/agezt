// SPDX-License-Identifier: MIT

// OpenAI provider: wire types + tiny wire-shape helpers.
// Code extracted from openai.go during the Day-96 god-file split.
// Public API unchanged.
package openai


import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)

func encodeRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int, jsonMode bool, params agent.Params, extra json.RawMessage) ([]byte, error) {
	wire := oaRequest{
		Model:          model,
		Stream:         false,
		MaxTokens:      maxTok, // 0 → omitted via omitempty
		ResponseFormat: jsonObjectFormat(jsonMode),
	}
	wire.applyParams(params)
	fwd, _ := toolname.Maps(tools)
	if strings.TrimSpace(system) != "" {
		wire.Messages = append(wire.Messages, oaMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		om, err := canonicalToOA(m, fwd)
		if err != nil {
			return nil, err
		}
		if om == nil {
			continue
		}
		wire.Messages = append(wire.Messages, *om)
	}
	for _, t := range tools {
		params := t.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		wire.Tools = append(wire.Tools, oaTool{
			Type: "function",
			Function: oaToolFnDef{
				Name:        toolname.Wire(fwd, t.Name),
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	return provopts.Merge(body, extra)
}

// canonicalToOA converts one canonical Message into one OpenAI Chat
// Completions message. Returns (nil, nil) when the system message has
// already been folded in via CompletionRequest.System. fwd maps original tool
// names to their wire names so an assistant turn's replayed tool calls carry the
// SAME (collision-safe) names as the current tool definitions; a name absent from
// fwd (a tool no longer offered) falls back to a plain sanitisation.
func canonicalToOA(m agent.Message, fwd map[string]string) (*oaMessage, error) {
	switch m.Role {
	case agent.RoleSystem:
		// System is set once via CompletionRequest.System; per-message
		// system roles are folded there.
		if strings.TrimSpace(m.Content) == "" {
			return nil, nil
		}
		return &oaMessage{Role: "system", Content: m.Content}, nil
	case agent.RoleUser:
		// Vision (M242): a user message may carry image attachments as URLs
		// (the CLI sends RFC 2397 data: URLs). When present, switch to the
		// multimodal content-parts array — text first, then one image_url part
		// per deliverable URL. A non-URL entry (e.g. a legacy bare filename)
		// has no valid url and is skipped.
		var parts []oaContentPart
		for _, img := range m.Images {
			if !isImageURL(img) {
				continue
			}
			parts = append(parts, oaContentPart{Type: "image_url", ImageURL: &oaImageURL{URL: img}})
		}
		if len(parts) == 0 {
			return &oaMessage{Role: "user", Content: oaTextOrNil(m.Content)}, nil
		}
		content := make([]oaContentPart, 0, len(parts)+1)
		if m.Content != "" {
			content = append(content, oaContentPart{Type: "text", Text: m.Content})
		}
		content = append(content, parts...)
		return &oaMessage{Role: "user", Content: content}, nil
	case agent.RoleAssistant:
		om := &oaMessage{Role: "assistant", Content: oaTextOrNil(m.Content)}
		for _, tc := range m.ToolCalls {
			args := tc.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			// OpenAI expects arguments as a JSON-encoded *string*, not
			// a nested object. We already have valid JSON bytes; cast.
			name := toolname.Wire(fwd, tc.Name)
			om.ToolCalls = append(om.ToolCalls, oaToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: oaToolCallFn{
					Name:      name,
					Arguments: string(args),
				},
			})
		}
		return om, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("openai: role=tool requires tool_call_id")
		}
		return &oaMessage{
			Role:       "tool",
			Content:    oaTextOrNil(m.Content),
			ToolCallID: m.ToolCallID,
		}, nil
	default:
		return nil, fmt.Errorf("openai: unknown role %q", m.Role)
	}
}

func decodeResponse(body []byte) (*agent.CompletionResponse, error) {
	var or oaResponse
	if err := json.Unmarshal(body, &or); err != nil {
		return nil, fmt.Errorf("openai: parse response: %w", err)
	}
	if len(or.Choices) == 0 {
		return nil, fmt.Errorf("openai: response has no choices")
	}
	choice := or.Choices[0]

	var toolCalls []agent.ToolCall
	for i, tc := range choice.Message.ToolCalls {
		id := tc.ID
		if id == "" {
			id = "call-" + strconv.Itoa(i)
		}
		// OpenAI returns arguments as a JSON-encoded string; canonical
		// shape carries the parsed RawMessage. Treat empty as "{}".
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
	switch choice.FinishReason {
	case "stop":
		stop = agent.StopEndTurn
	case "tool_calls", "function_call":
		stop = agent.StopToolUse
	case "length":
		stop = agent.StopMaxTokens
	}
	// finish_reason is sometimes absent on openai-compatible servers
	// when tool_calls are emitted; fall back to tool-calls presence.
	if len(toolCalls) > 0 && stop == agent.StopEndTurn {
		stop = agent.StopToolUse
	}

	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			Content:   oaContentText(choice.Message.Content),
			ToolCalls: toolCalls,
		},
		ReasoningContent: choice.Message.reasoningText(), // M317: DeepSeek-R1 et al.
		StopReason:       stop,
		Usage: agent.Usage{
			InputTokens:       or.Usage.PromptTokens,
			CachedInputTokens: cachedInputTokens(or.Usage.PromptTokensDetails.CachedTokens, or.Usage.PromptCacheHitTokens),
			OutputTokens:      or.Usage.CompletionTokens,
			Model:             or.Model,
		},
	}, nil
}

// cachedInputTokens folds the two wire spellings of "prompt tokens served
// from cache" into one count (M887): OpenAI's prompt_tokens_details.
// cached_tokens and DeepSeek's prompt_cache_hit_tokens. A server emitting
// both reports the same number; max() keeps a double-reporter from
// double-counting while letting either spelling stand alone.
func cachedInputTokens(openaiStyle, deepseekStyle int) int {
	return max(openaiStyle, deepseekStyle)
}
