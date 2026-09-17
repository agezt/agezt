// SPDX-License-Identifier: MIT

package bedrock

// Anthropic-on-Bedrock wire types + encoders/decoders: anthBedrockRequest +
// anthThinking + anthSystemBlock + anthTool + anthCacheControl +
// anthMessage + anthBlock + anthImageSource + anthBedrockResponse +
// applyParams + thinkingConfig + buildBedrockSystem + buildBedrockTools
// + anthBedrockUsageToAgent + encodeAnthropicOnBedrockRequest +
// parseImageDataURL + canonicalToAnth + decodeAnthropicOnBedrockResponse.
// Carved out of bedrock.go during the Day 171 god-file split so the main
// file can focus on Provider struct + auth + Complete + model detection.
// Public API unchanged.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)

type anthBedrockRequest struct {
	AnthropicVersion string        `json:"anthropic_version"`
	MaxTokens        int           `json:"max_tokens"`
	System           any           `json:"system,omitempty"`
	Messages         []anthMessage `json:"messages"`
	Tools            []anthTool    `json:"tools,omitempty"`
	Thinking         *anthThinking `json:"thinking,omitempty"` // extended thinking via ReasoningEffort (M997)
	// Per-request sampling knobs (M997). Anthropic-on-Bedrock has no seed /
	// penalties — same surface as the direct-Anthropic adapter.
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"top_p,omitempty"`
	TopK          *int     `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`
}

// applyParams copies the universal sampling knobs Anthropic-on-Bedrock
// understands. ReasoningEffort is handled separately (mapped to a thinking
// budget); an unset Params leaves the request unchanged.
func (wire *anthBedrockRequest) applyParams(p agent.Params) {
	if p.IsZero() {
		return
	}
	wire.Temperature = p.Temperature
	wire.TopP = p.TopP
	wire.TopK = p.TopK
	wire.StopSequences = p.Stop
}

// MinThinkingBudget is Anthropic's minimum extended-thinking budget_tokens.
const MinThinkingBudget = 1024

// anthThinking is Anthropic-on-Bedrock's extended-thinking request block.
type anthThinking struct {
	Type         string `json:"type"`          // "enabled"
	BudgetTokens int    `json:"budget_tokens"` // >= 1024, and < max_tokens
}

// thinkingConfig returns the thinking block + the max_tokens to use when an
// extended-thinking budget is set. Mirrors the direct-Anthropic adapter:
// Anthropic requires max_tokens > budget_tokens and budget >= 1024, so we clamp
// the budget up and ensure max_tokens leaves room for the answer. budget <= 0
// returns (nil, maxTok unchanged) so the wire stays byte-identical.
func thinkingConfig(budget, maxTok int) (*anthThinking, int) {
	if budget <= 0 {
		return nil, maxTok
	}
	if budget < MinThinkingBudget {
		budget = MinThinkingBudget
	}
	if maxTok <= budget {
		maxTok = budget + DefaultMaxTokens
	}
	return &anthThinking{Type: "enabled", BudgetTokens: budget}, maxTok
}

// anthSystemBlock is the array form of the system prompt, carrying a prompt-cache
// breakpoint (M302) so the stable system prompt is cached alongside the tools.
type anthSystemBlock struct {
	Type         string            `json:"type"` // "text"
	Text         string            `json:"text"`
	CacheControl *anthCacheControl `json:"cache_control,omitempty"`
}

// buildBedrockSystem returns the system field: nil when empty (omitted), else a
// one-element cache-marked block array (M302). Bedrock caches the prefix
// tools→system, so this caches tools AND system, the whole stable agent-loop
// prefix.
func buildBedrockSystem(system string) any {
	if system == "" {
		return nil
	}
	return []anthSystemBlock{{Type: "text", Text: system, CacheControl: &anthCacheControl{Type: "ephemeral"}}}
}

type anthTool struct {
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	InputSchema  json.RawMessage   `json:"input_schema"`
	CacheControl *anthCacheControl `json:"cache_control,omitempty"`
}

// anthCacheControl marks a tool/content block as a prompt-cache breakpoint
// (M300). Claude-on-Bedrock caches the request prefix up to and including the
// marked block; "ephemeral" is the 5-minute TTL tier.
type anthCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// buildBedrockTools mirrors the direct-Anthropic provider (M299): it marks the
// LAST tool with cache_control so Bedrock caches the stable tools prefix that
// repeats every agent-loop iteration. Bedrock ignores the marker when the prefix
// is below the minimum cacheable size, so it's safe to always set; cache reads
// bill at ~0.1× input (M289-291/M296).
func buildBedrockTools(tools []agent.ToolDef, fwd map[string]string) []anthTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, anthTool{Name: toolname.Wire(fwd, t.Name), Description: t.Description, InputSchema: t.InputSchema})
	}
	out[len(out)-1].CacheControl = &anthCacheControl{Type: "ephemeral"}
	return out
}

type anthMessage struct {
	Role    string      `json:"role"`
	Content []anthBlock `json:"content"`
}

type anthBlock struct {
	Type       string           `json:"type"`
	Text       string           `json:"text,omitempty"`
	ID         string           `json:"id,omitempty"`
	Name       string           `json:"name,omitempty"`
	Input      json.RawMessage  `json:"input,omitempty"`
	ToolUseID  string           `json:"tool_use_id,omitempty"`
	ResultBody string           `json:"content,omitempty"`
	IsError    bool             `json:"is_error,omitempty"`
	Source     *anthImageSource `json:"source,omitempty"` // type=image
}

// anthImageSource is the base64 payload of a type=image content block — how
// Anthropic-on-Bedrock carries an image attachment (M244).
type anthImageSource struct {
	Type      string `json:"type"`       // always "base64"
	MediaType string `json:"media_type"` // image/png | image/jpeg | image/gif | image/webp
	Data      string `json:"data"`       // base64-encoded image bytes
}

type anthBedrockResponse struct {
	ID         string      `json:"id"`
	Type       string      `json:"type"`
	Role       string      `json:"role"`
	Content    []anthBlock `json:"content"`
	StopReason string      `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// anthBedrockUsageToAgent maps Claude-on-Bedrock's split token counts to the
// canonical agent.Usage (M296), mirroring the direct-Anthropic provider:
// input_tokens excludes cached prompt tokens, so the real prompt is
// input + cache_read + cache_creation; cache reads are marked cached (cheaper
// rate, M289), cache-creation as cache-write (the premium, M291).
func encodeAnthropicOnBedrockRequest(system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int, params agent.Params, extra json.RawMessage) ([]byte, error) {
	// A per-request reasoning effort (M997) maps to an extended-thinking budget,
	// exactly like the direct-Anthropic adapter. Empty effort → no thinking block.
	thinking, maxTok := thinkingConfig(func() int {
		b, _ := provopts.ThinkingBudget(params.ReasoningEffort, maxTok)
		return b
	}(), maxTok)
	fwd, _ := toolname.Maps(tools)
	wire := anthBedrockRequest{
		AnthropicVersion: AnthropicBedrockVersion,
		MaxTokens:        maxTok,
		System:           buildBedrockSystem(system),
		Tools:            buildBedrockTools(tools, fwd),
		Thinking:         thinking,
	}
	wire.applyParams(params)
	for _, m := range msgs {
		am, err := canonicalToAnth(m, fwd)
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
// which the caller skips. The CLI sends data: URLs (M241).
func decodeAnthropicOnBedrockResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var ar anthBedrockResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("bedrock: parse response: %w", err)
	}
	var (
		textParts []string
		toolCalls []agent.ToolCall
	)
	for _, b := range ar.Content {
		switch b.Type {
		case "text":
			textParts = append(textParts, b.Text)
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
		Usage: anthBedrockUsageToAgent(
			ar.Usage.InputTokens, ar.Usage.CacheReadInputTokens,
			ar.Usage.CacheCreationInputTokens, ar.Usage.OutputTokens, model),
	}, nil
}
