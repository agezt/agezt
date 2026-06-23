// SPDX-License-Identifier: MIT

package bedrock

// Amazon-Nova-on-Bedrock body shape (M326). Amazon's flagship model
// family (Nova Micro / Lite / Pro / Premier) uses the Converse-style
// "messages-v1" InvokeModel schema — distinct from the legacy
// `amazon.titan-*` text body, which stays intentionally unwired:
//
//	{
//	  "schemaVersion": "messages-v1",
//	  "system":   [{"text": "..."}],
//	  "messages": [
//	    {"role": "user",      "content": [{"text": "..."}]},
//	    {"role": "assistant", "content": [{"text": "..."}]}
//	  ],
//	  "inferenceConfig": {"maxTokens": N}
//	}
//
// Response shape:
//
//	{
//	  "output": {"message": {"role": "assistant", "content": [{"text": "..."}]}},
//	  "stopReason": "end_turn" | "max_tokens" | ...,
//	  "usage": {"inputTokens": N, "outputTokens": N, "totalTokens": N}
//	}
//
// The system prompt is a top-level array (not a message role); Nova
// accepts only user/assistant message roles. **Tools are NOT wired
// here** — Nova supports toolConfig, but tool round-trips need the
// content-block shape the agent loop doesn't emit for this path yet
// (same boundary as the Mistral/Llama adapters). Chat-only; operators
// needing tool use on Bedrock should use the anthropic.* models.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
)

type novaBedrockRequest struct {
	SchemaVersion   string              `json:"schemaVersion"` // "messages-v1"
	System          []novaTextBlock     `json:"system,omitempty"`
	Messages        []novaMessage       `json:"messages"`
	InferenceConfig novaInferenceConfig `json:"inferenceConfig"`
}

type novaTextBlock struct {
	Text string `json:"text"`
}

type novaMessage struct {
	Role    string          `json:"role"` // "user" | "assistant"
	Content []novaTextBlock `json:"content"`
}

type novaInferenceConfig struct {
	MaxTokens int `json:"maxTokens"`
	// Per-request sampling knobs (M997). Nova nests sampling under
	// inferenceConfig with camelCase keys (temperature/topP/topK/stopSequences);
	// no seed or penalties. Nil-able so an unset Params is a no-op.
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"topP,omitempty"`
	TopK          *int     `json:"topK,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

// applyParams maps the universal sampling knobs onto Nova's inferenceConfig
// fields. An unset Params leaves the request unchanged; seed/penalties/
// ReasoningEffort have no Nova equivalent.
func (c *novaInferenceConfig) applyParams(p agent.Params) {
	if p.IsZero() {
		return
	}
	c.Temperature = p.Temperature
	c.TopP = p.TopP
	c.TopK = p.TopK
	c.StopSequences = p.Stop
}

type novaBedrockResponse struct {
	Output struct {
		Message struct {
			Role    string          `json:"role"`
			Content []novaTextBlock `json:"content"`
		} `json:"message"`
	} `json:"output"`
	StopReason string `json:"stopReason"`
	Usage      struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
		TotalTokens  int `json:"totalTokens"`
	} `json:"usage"`
}

// encodeNovaOnBedrockRequest converts a canonical request into the Nova
// "messages-v1" body. The system prompt becomes the top-level `system`
// array (Nova has no system message role); user/assistant roles map
// through, and any other canonical role (system-as-message, tool) folds
// into a user message so the model still sees the content — Nova rejects
// unknown roles and empty content, so empty messages are skipped.
func encodeNovaOnBedrockRequest(system string, msgs []agent.Message, maxTok int, params agent.Params, extra json.RawMessage) ([]byte, error) {
	out := novaBedrockRequest{
		SchemaVersion:   "messages-v1",
		InferenceConfig: novaInferenceConfig{MaxTokens: maxTok},
	}
	out.InferenceConfig.applyParams(params)
	if s := strings.TrimSpace(system); s != "" {
		out.System = []novaTextBlock{{Text: system}}
	}
	for _, m := range msgs {
		text := m.Content
		if strings.TrimSpace(text) == "" {
			// Nova rejects empty content blocks; drop empty turns rather than
			// send a body the API will 400.
			continue
		}
		role := string(m.Role)
		switch role {
		case "user", "assistant":
			// ok — Nova's two message roles
		case "system":
			// A per-message system prompt has no Nova message role; fold it into
			// the top-level system array so the guidance still reaches the model.
			out.System = append(out.System, novaTextBlock{Text: text})
			continue
		default:
			// Tool results (and anything else) surface as user content so the
			// model sees them, even without a tool round-trip shape.
			role = "user"
		}
		out.Messages = append(out.Messages, novaMessage{
			Role:    role,
			Content: []novaTextBlock{{Text: text}},
		})
	}
	if len(out.Messages) == 0 {
		return nil, errors.New("bedrock-nova: at least one non-empty message required")
	}
	body, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return provopts.Merge(body, extra)
}

// decodeNovaOnBedrockResponse converts the Nova output message into a
// canonical CompletionResponse. Unlike the Mistral adapter, Nova returns
// token counts inline (usage.inputTokens/outputTokens), so the governor
// sees real spend.
func decodeNovaOnBedrockResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var wire novaBedrockResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("bedrock-nova: parse response: %w", err)
	}
	var sb strings.Builder
	for _, c := range wire.Output.Message.Content {
		sb.WriteString(c.Text)
	}
	if sb.Len() == 0 {
		return nil, errors.New("bedrock-nova: response has no output text")
	}
	stop := agent.StopEndTurn
	switch wire.StopReason {
	case "max_tokens":
		stop = agent.StopMaxTokens
	case "end_turn", "stop_sequence", "":
		stop = agent.StopEndTurn
	}
	role := wire.Output.Message.Role
	if role == "" {
		role = string(agent.RoleAssistant)
	}
	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:    agent.Role(role),
			Content: sb.String(),
		},
		StopReason: stop,
		Usage: agent.Usage{
			InputTokens:  wire.Usage.InputTokens,
			OutputTokens: wire.Usage.OutputTokens,
			Model:        model,
		},
	}, nil
}

// isAmazonNovaModel reports whether the model id maps to the Amazon Nova
// "messages-v1" body shape (M326). Matches `amazon.nova-*` and regional
// cross-inference profiles (`us.amazon.nova-*`, `eu.amazon.nova-*`),
// deliberately NOT the legacy `amazon.titan-*` text models.
func isAmazonNovaModel(id string) bool {
	if strings.HasPrefix(id, "amazon.nova") {
		return true
	}
	return strings.Contains(id, ".amazon.nova")
}
