// SPDX-License-Identifier: MIT

package ollama

// Ollama ENCODE side: buildOptions + encodeRequest + ollamaImageData
// + canonicalToOllama. Carved out of ollama.go during the Day 193
// god-file split so the main file can stay focused on the Provider
// dispatch (New/Name/Complete) + shared wire types, and the decode
// file can stay focused on response parsing.
// Public API unchanged.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
)

func encodeRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef, maxTokens int, jsonMode bool, params agent.Params, extra json.RawMessage) ([]byte, error) {
	out := ollamaRequest{
		Model:  model,
		Stream: false,
	}
	if jsonMode {
		out.Format = "json" // Ollama's native JSON-mode switch (M311)
	}
	out.Options = buildOptions(maxTokens, params)
	if system != "" {
		out.Messages = append(out.Messages, ollamaMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		om, err := canonicalToOllama(m)
		if err != nil {
			return nil, err
		}
		out.Messages = append(out.Messages, om)
	}
	for _, t := range tools {
		out.Tools = append(out.Tools, ollamaTool{
			Type: "function",
			Function: ollamaToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	body, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return provopts.Merge(body, extra)
}

// ollamaImageData extracts the raw base64 payload from an RFC 2397 data: URL
// ("data:<media-type>;base64,<payload>"). Ollama's chat API wants the bare
// base64 bytes — no data: prefix, no media type — so we drop the envelope.
// Returns ok=false for anything that isn't a base64 image data URL (e.g. an
// http(s) URL Ollama can't fetch, or a legacy bare filename), which the caller
// skips. Mirrors the parseImageDataURL helpers in the cloud providers.
func ollamaImageData(s string) (string, bool) {
	const prefix = "data:"
	if !strings.HasPrefix(s, prefix) {
		return "", false
	}
	meta, payload, found := strings.Cut(s[len(prefix):], ",")
	if !found || !strings.HasSuffix(meta, ";base64") || payload == "" {
		return "", false
	}
	return payload, true
}

func canonicalToOllama(m agent.Message) (ollamaMessage, error) {
	switch m.Role {
	case agent.RoleSystem:
		return ollamaMessage{Role: "system", Content: m.Content}, nil
	case agent.RoleUser:
		om := ollamaMessage{Role: "user", Content: m.Content}
		// Vision (M309): a user message may carry image attachments as RFC 2397
		// data: URLs (what the CLI sends, M241). Ollama's chat API takes raw
		// base64 image data in an `images` array — it sniffs the format itself,
		// no media type — so extract the payload from each data: URL. Entries
		// without a deliverable base64 payload (an http URL Ollama can't fetch,
		// or a legacy bare filename) are skipped. Local vision models — llava,
		// llama3.2-vision, moondream — read these.
		for _, img := range m.Images {
			if data, ok := ollamaImageData(img); ok {
				om.Images = append(om.Images, data)
			}
		}
		return om, nil
	case agent.RoleAssistant:
		om := ollamaMessage{Role: "assistant", Content: m.Content}
		for _, tc := range m.ToolCalls {
			args := tc.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			om.ToolCalls = append(om.ToolCalls, ollamaToolCall{
				ID: tc.ID,
				Function: ollamaToolCallFn{
					Name:      tc.Name,
					Arguments: args,
				},
			})
		}
		return om, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return ollamaMessage{}, errors.New("ollama: role=tool requires tool_call_id")
		}
		return ollamaMessage{
			Role:       "tool",
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}, nil
	default:
		return ollamaMessage{}, fmt.Errorf("ollama: unknown role %q", m.Role)
	}
}

