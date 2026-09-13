// SPDX-License-Identifier: MIT

package vertex

// Anthropic-on-Vertex ENCODE side: encodeAnthropicOnVertexRequest +
// canonicalToAnthVx + parseImageDataURL. Carved out of anthropic.go
// during the Day 187 god-file split so the main file can stay focused
// on the Provider dispatch (Resolve*/Complete) + shared wire types,
// and the decode file can stay focused on response parsing.
// Public API unchanged.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)

func encodeAnthropicOnVertexRequest(system string, msgs []agent.Message, tools []agent.ToolDef, maxTok, thinkingBudget int, stream bool, params agent.Params, extra json.RawMessage) ([]byte, error) {
	// A per-request reasoning effort (M997) overrides the construction-time
	// thinking budget when set; otherwise the env/default budget stands.
	if b, ok := provopts.ThinkingBudget(params.ReasoningEffort, maxTok); ok {
		thinkingBudget = b
	}
	fwd, _ := toolname.Maps(tools)
	thinking, maxTok := anthVxThinkingConfig(thinkingBudget, maxTok)
	wire := anthVertexRequest{
		AnthropicVersion: AnthropicVertexVersion,
		MaxTokens:        maxTok,
		System:           buildVxSystem(system),
		Stream:           stream,
		Tools:            buildVxTools(tools, fwd),
		Thinking:         thinking,
	}
	wire.applyParams(params)
	for _, m := range msgs {
		am, err := canonicalToAnthVx(m, fwd)
		if err != nil {
			return nil, err
		}
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
// "data:<media-type>;base64,<payload>" into its media type and base64 payload,
// returning ok=false for anything else (including a legacy bare filename),
// which the caller skips. The CLI sends data: URLs (M241). Shared by the
// Anthropic-on-Vertex and Gemini-on-Vertex encoders.
func parseImageDataURL(s string) (mediaType, data string, ok bool) {
	const prefix = "data:"
	if !strings.HasPrefix(s, prefix) {
		return "", "", false
	}
	meta, payload, found := strings.Cut(s[len(prefix):], ",")
	if !found || !strings.HasSuffix(meta, ";base64") {
		return "", "", false
	}
	mt := strings.TrimSuffix(meta, ";base64")
	if mt == "" || payload == "" {
		return "", "", false
	}
	return mt, payload, true
}

func canonicalToAnthVx(m agent.Message, fwd map[string]string) (*anthVxMessage, error) {
	switch m.Role {
	case agent.RoleSystem:
		return nil, nil
	case agent.RoleUser:
		// Vision (M245): a user message may carry image attachments as RFC 2397
		// data: URLs. Emit each as a type=image block before the text block. A
		// non-data-URL entry (e.g. a legacy bare filename) is skipped.
		blocks := make([]anthVxBlock, 0, len(m.Images)+1)
		for _, img := range m.Images {
			if mt, data, ok := parseImageDataURL(img); ok {
				blocks = append(blocks, anthVxBlock{
					Type:   "image",
					Source: &anthVxImageSource{Type: "base64", MediaType: mt, Data: data},
				})
			}
		}
		blocks = append(blocks, anthVxBlock{Type: "text", Text: m.Content})
		return &anthVxMessage{Role: "user", Content: blocks}, nil
	case agent.RoleAssistant:
		var blocks []anthVxBlock
		if strings.TrimSpace(m.Content) != "" {
			blocks = append(blocks, anthVxBlock{Type: "text", Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			input := tc.Input
			if len(input) == 0 {
				input = json.RawMessage(`{}`)
			}
			blocks = append(blocks, anthVxBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  toolname.Wire(fwd, tc.Name),
				Input: input,
			})
		}
		if len(blocks) == 0 {
			blocks = []anthVxBlock{{Type: "text", Text: ""}}
		}
		return &anthVxMessage{Role: "assistant", Content: blocks}, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("vertex: role=tool requires tool_call_id")
		}
		return &anthVxMessage{
			Role: "user",
			Content: []anthVxBlock{{
				Type:       "tool_result",
				ToolUseID:  m.ToolCallID,
				ResultBody: m.Content,
			}},
		}, nil
	default:
		return nil, fmt.Errorf("vertex: unknown role %q", m.Role)
	}
}

