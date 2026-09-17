// SPDX-License-Identifier: MIT

// bedrock_anthropic_helpers.go: usage conversion + image parsing + canonical→anth
// conversion split off from bedrock_anthropic.go during the Day 211 god-file
// refactor (#139). Public API unchanged.
package bedrock

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)

func anthBedrockUsageToAgent(inputTokens, cacheRead, cacheCreation, outputTokens int, model string) agent.Usage {
	return agent.Usage{
		InputTokens:           inputTokens + cacheRead + cacheCreation,
		CachedInputTokens:     cacheRead,
		CacheWriteInputTokens: cacheCreation,
		OutputTokens:          outputTokens,
		Model:                 model,
	}
}

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

func canonicalToAnth(m agent.Message, fwd map[string]string) (*anthMessage, error) {
	switch m.Role {
	case agent.RoleSystem:
		return nil, nil
	case agent.RoleUser:
		// Vision (M244): a user message may carry image attachments as RFC 2397
		// data: URLs. Emit each as a type=image block before the text block. A
		// non-data-URL entry (e.g. a legacy bare filename) is skipped.
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
			blocks = []anthBlock{{Type: "text", Text: ""}}
		}
		return &anthMessage{Role: "assistant", Content: blocks}, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("bedrock: role=tool requires tool_call_id")
		}
		return &anthMessage{
			Role: "user",
			Content: []anthBlock{{
				Type:       "tool_result",
				ToolUseID:  m.ToolCallID,
				ResultBody: m.Content,
			}},
		}, nil
	default:
		return nil, fmt.Errorf("bedrock: unknown role %q", m.Role)
	}
}
