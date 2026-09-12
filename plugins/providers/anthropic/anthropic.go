// SPDX-License-Identifier: MIT

// Anthropic provider: Provider type + Complete + encode/decode roundtrip + canonicalToAnth.
// Code extracted from anthropic.go during the Day-103 god-file split.
// Public API unchanged.
package anthropic


import (
	"errors"
	"fmt"
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)

func encodeRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef, maxTok, thinkingBudget int, params agent.Params, extra json.RawMessage) ([]byte, error) {
	// A per-request reasoning effort (M997) overrides the construction-time
	// thinking budget when set; otherwise the env/default budget stands.
	if b, ok := provopts.ThinkingBudget(params.ReasoningEffort, maxTok); ok {
		thinkingBudget = b
	}
	thinking, maxTok := thinkingConfig(thinkingBudget, maxTok)
	fwd, _ := toolname.Maps(tools)
	wire := anthRequest{
		Model:     model,
		MaxTokens: maxTok,
		System:    buildAnthSystem(system),
		Tools:     buildAnthTools(tools, fwd),
		Thinking:  thinking,
	}
	wire.applyParams(params)
	for _, m := range msgs {
		am, err := canonicalToAnth(m, fwd)
		if err != nil {
			return nil, err
		}
		// Skip role=system messages here — Anthropic uses the top-level
		// "system" field, set by the caller via CompletionRequest.System.
		if am == nil {
			continue
		}
		wire.Messages = append(wire.Messages, *am)
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	return provopts.Merge(body, extra)
}

// parseImageDataURL splits an RFC 2397 data: URL of the form
// "data:<media-type>;base64,<payload>" into its media type and base64
// payload. It returns ok=false for anything that is not a base64 image data
// URL — including a legacy bare filename — which the caller skips. Only a
// vision-capable run reaches a provider with images (the M91 gate), and the
// CLI sends data URLs (M241).
func parseImageDataURL(s string) (mediaType, data string, ok bool) {
	const prefix = "data:"
	if !strings.HasPrefix(s, prefix) {
		return "", "", false
	}
	meta, payload, found := strings.Cut(s[len(prefix):], ",")
	if !found {
		return "", "", false
	}
	if !strings.HasSuffix(meta, ";base64") {
		return "", "", false
	}
	mt := strings.TrimSuffix(meta, ";base64")
	if mt == "" || payload == "" {
		return "", "", false
	}
	return mt, payload, true
}

// canonicalToAnth converts one canonical Message into one Anthropic message.
// Returns (nil, nil) when the message has no Anthropic representation
// (e.g. role=system, which Anthropic carries as a top-level field).
func canonicalToAnth(m agent.Message, fwd map[string]string) (*anthMessage, error) {
	switch m.Role {
	case agent.RoleSystem:
		return nil, nil
	case agent.RoleUser:
		// Vision (M241): a user message may carry image attachments as RFC 2397
		// data: URLs. Emit each as an Anthropic type=image block BEFORE the text
		// block (the order Anthropic recommends). A non-data-URL entry (e.g. a
		// legacy bare filename) has no deliverable payload, so it is skipped.
		blocks := make([]anthBlock, 0, len(m.Images)+1)
		for _, img := range m.Images {
			if mt, data, ok := parseImageDataURL(img); ok {
				blocks = append(blocks, anthBlock{
					Type:   "image",
					Source: &anthImageSource{Type: "base64", MediaType: mt, Data: data},
				})
			}
		}
		blocks = append(blocks, anthBlock{Type: "text", Text: m.Content})
		return &anthMessage{Role: "user", Content: blocks}, nil
	case agent.RoleAssistant:
		var blocks []anthBlock
		if strings.TrimSpace(m.Content) != "" {
			blocks = append(blocks, anthBlock{Type: "text", Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			input := tc.Input
			if len(input) == 0 {
				input = json.RawMessage(`{}`)
			}
			blocks = append(blocks, anthBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  toolname.Wire(fwd, tc.Name),
				Input: input,
			})
		}
		if len(blocks) == 0 {
			// Anthropic rejects empty assistant content; insert a placeholder.
			blocks = []anthBlock{{Type: "text", Text: ""}}
		}
		return &anthMessage{Role: "assistant", Content: blocks}, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("anthropic: role=tool requires tool_call_id")
		}
		return &anthMessage{
			Role: "user", // Anthropic routes tool results inside a user message
			Content: []anthBlock{{
				Type:       "tool_result",
				ToolUseID:  m.ToolCallID,
				ResultBody: m.Content,
			}},
		}, nil
	default:
		return nil, fmt.Errorf("anthropic: unknown role %q", m.Role)
	}
}

func decodeResponse(body []byte) (*agent.CompletionResponse, error) {
	var ar anthResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("anthropic: parse response: %w", err)
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
		case "thinking": // extended thinking (M318)
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
		ReasoningContent: strings.Join(reasoningParts, ""), // M318
		StopReason:       stop,
		Usage: anthUsageToAgent(
			ar.Usage.InputTokens, ar.Usage.CacheReadInputTokens,
			ar.Usage.CacheCreationInputTokens, ar.Usage.OutputTokens, ar.Model),
	}, nil
}
