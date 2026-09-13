// SPDX-License-Identifier: MIT

package vertex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
	"github.com/agezt/agezt/plugins/providers/internal/retry"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)

const (
	// AnthropicVertexVersion is the value sent in the request body's
	// `anthropic_version` field. Vertex pins this; bumping requires
	// coordination with Google's release notes.
	AnthropicVertexVersion = "vertex-2023-10-16"
	// DefaultAnthropicMaxTokens is what we send when the request leaves
	// max_tokens at 0. Anthropic requires a non-zero max_tokens.
	DefaultAnthropicMaxTokens = 4096
	// MinAnthropicThinkingBudget is Anthropic's minimum extended-thinking
	// budget_tokens (M321), same floor as the direct Anthropic adapter (M318).
	MinAnthropicThinkingBudget = 1024
)

// isAnthropicModel reports whether the Vertex model id maps to the
// Anthropic publisher. Vertex model ids for Anthropic look like:
//
//	claude-opus-4-7@20251031
//	claude-sonnet-4-5@20250929
//	claude-3-5-sonnet-v2@20241022
//
// We match the `claude-` prefix (case-insensitive) — that's the
// only stable signal Google emits across model revisions.
func isAnthropicModel(id string) bool {
	return strings.HasPrefix(strings.ToLower(id), "claude-")
}

// ResolveAnthropicEndpoint returns the `:rawPredict` URL Complete
// will POST to for an Anthropic model. Exported for tests + custom-
// URL verification, mirroring ResolveEndpoint.
func (p *Provider) ResolveAnthropicEndpoint(model string) string {
	if p.Endpoint != "" {
		return p.Endpoint
	}
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		base = "https://" + p.Location + "-aiplatform.googleapis.com"
	}
	return base + "/" + DefaultAPIVersion +
		"/projects/" + p.Project +
		"/locations/" + p.Location +
		"/publishers/anthropic/models/" + model + ":rawPredict"
}

// ResolveAnthropicStreamEndpoint returns the `:streamRawPredict` URL
// CompleteStream will POST to for an Anthropic model.
func (p *Provider) ResolveAnthropicStreamEndpoint(model string) string {
	if p.Endpoint != "" {
		return p.Endpoint
	}
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		base = "https://" + p.Location + "-aiplatform.googleapis.com"
	}
	return base + "/" + DefaultAPIVersion +
		"/projects/" + p.Project +
		"/locations/" + p.Location +
		"/publishers/anthropic/models/" + model + ":streamRawPredict"
}

// ----- wire types (Anthropic Messages API shape, no `model` field) -----

type anthVertexRequest struct {
	AnthropicVersion string          `json:"anthropic_version"`
	MaxTokens        int             `json:"max_tokens"`
	System           any             `json:"system,omitempty"`
	Messages         []anthVxMessage `json:"messages"`
	Tools            []anthVxTool    `json:"tools,omitempty"`
	Thinking         *anthVxThinking `json:"thinking,omitempty"` // extended thinking (M321)
	Stream           bool            `json:"stream,omitempty"`
	// Per-request sampling knobs (M997). Anthropic has no seed / penalties.
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"top_p,omitempty"`
	TopK          *int     `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`
}

// applyParams copies the universal sampling knobs Anthropic understands. The
// reasoning knob is handled separately (mapped to a thinking budget) and so is
// ignored here. An unset Params leaves the request unchanged.
func (wire *anthVertexRequest) applyParams(p agent.Params) {
	if p.IsZero() {
		return
	}
	wire.Temperature = p.Temperature
	wire.TopP = p.TopP
	wire.TopK = p.TopK
	wire.StopSequences = p.Stop
}

// anthVxThinking is Anthropic-on-Vertex's extended-thinking request block
// (M321), identical to the direct Anthropic adapter's (M318).
type anthVxThinking struct {
	Type         string `json:"type"`          // "enabled"
	BudgetTokens int    `json:"budget_tokens"` // >= 1024, and < max_tokens
}

// anthVxThinkingConfig returns the thinking block + the max_tokens to use when
// an extended-thinking budget is set (M321), mirroring the direct Anthropic
// adapter's thinkingConfig: Anthropic requires max_tokens > budget_tokens and
// budget >= 1024, so the budget is clamped up and max_tokens is bumped to leave
// room for the answer. budget <= 0 → (nil, maxTok unchanged): thinking off,
// wire byte-identical. (A negative "dynamic" budget — valid for Gemini — means
// "off" here; Anthropic has no dynamic mode.)
func anthVxThinkingConfig(budget, maxTok int) (*anthVxThinking, int) {
	if budget <= 0 {
		return nil, maxTok
	}
	if budget < MinAnthropicThinkingBudget {
		budget = MinAnthropicThinkingBudget
	}
	if maxTok <= budget {
		maxTok = budget + DefaultAnthropicMaxTokens // room for the answer beyond thinking
	}
	return &anthVxThinking{Type: "enabled", BudgetTokens: budget}, maxTok
}

// anthVxSystemBlock is the array form of the system prompt, carrying a
// prompt-cache breakpoint (M302) so the stable system prompt is cached alongside
// the tools.
type anthVxSystemBlock struct {
	Type         string              `json:"type"` // "text"
	Text         string              `json:"text"`
	CacheControl *anthVxCacheControl `json:"cache_control,omitempty"`
}

// buildVxSystem returns the system field: nil when empty (omitted), else a
// one-element cache-marked block array (M302). Vertex caches the prefix
// tools→system, so this caches tools AND system.
func buildVxSystem(system string) any {
	if system == "" {
		return nil
	}
	return []anthVxSystemBlock{{Type: "text", Text: system, CacheControl: &anthVxCacheControl{Type: "ephemeral"}}}
}

type anthVxTool struct {
	Name         string              `json:"name"`
	Description  string              `json:"description,omitempty"`
	InputSchema  json.RawMessage     `json:"input_schema"`
	CacheControl *anthVxCacheControl `json:"cache_control,omitempty"`
}

// anthVxCacheControl marks a tool/content block as a prompt-cache breakpoint
// (M300). Claude-on-Vertex caches the request prefix up to and including the
// marked block; "ephemeral" is the 5-minute TTL tier.
type anthVxCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// buildVxTools mirrors the direct-Anthropic provider (M299): it marks the LAST
// tool with cache_control so Vertex caches the stable tools prefix that repeats
// every agent-loop iteration. Vertex ignores the marker when the prefix is below
// the minimum cacheable size, so it's safe to always set.
func buildVxTools(tools []agent.ToolDef, fwd map[string]string) []anthVxTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthVxTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, anthVxTool{Name: toolname.Wire(fwd, t.Name), Description: t.Description, InputSchema: t.InputSchema})
	}
	out[len(out)-1].CacheControl = &anthVxCacheControl{Type: "ephemeral"}
	return out
}

type anthVxMessage struct {
	Role    string        `json:"role"`
	Content []anthVxBlock `json:"content"`
}

type anthVxBlock struct {
	Type       string             `json:"type"`
	Text       string             `json:"text,omitempty"`
	Thinking   string             `json:"thinking,omitempty"` // type=thinking content (M321)
	ID         string             `json:"id,omitempty"`
	Name       string             `json:"name,omitempty"`
	Input      json.RawMessage    `json:"input,omitempty"`
	ToolUseID  string             `json:"tool_use_id,omitempty"`
	ResultBody string             `json:"content,omitempty"`
	IsError    bool               `json:"is_error,omitempty"`
	Source     *anthVxImageSource `json:"source,omitempty"` // type=image
}

// anthVxImageSource is the base64 payload of a type=image content block — how
// Anthropic-on-Vertex carries an image attachment (M245).
type anthVxImageSource struct {
	Type      string `json:"type"`       // always "base64"
	MediaType string `json:"media_type"` // image/png | image/jpeg | image/gif | image/webp
	Data      string `json:"data"`       // base64-encoded image bytes
}

type anthVxResponse struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Role       string        `json:"role"`
	Content    []anthVxBlock `json:"content"`
	StopReason string        `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// anthVxUsageToAgent maps Anthropic-on-Vertex split token counts to agent.Usage
// (M290), mirroring the direct-Anthropic provider: input_tokens excludes cached
// prompt tokens, so the real prompt is input + cache_read + cache_creation;
// cache reads are marked cached (cheaper rate), cache-creation as cache-write
// (the cache-write premium, M291).
func anthVxUsageToAgent(inputTokens, cacheRead, cacheCreation, outputTokens int, model string) agent.Usage {
	return agent.Usage{
		InputTokens:           inputTokens + cacheRead + cacheCreation,
		CachedInputTokens:     cacheRead,
		CacheWriteInputTokens: cacheCreation,
		OutputTokens:          outputTokens,
		Model:                 model,
	}
}

// completeAnthropic is the Anthropic-on-Vertex non-streaming path.
// Called from Complete when isAnthropicModel(model) is true.
func (p *Provider) completeAnthropic(ctx context.Context, req agent.CompletionRequest, model string) (*agent.CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultAnthropicMaxTokens
	}
	body, err := encodeAnthropicOnVertexRequest(req.System, req.Messages, req.Tools, maxTokens, p.ThinkingBudget, false, req.Params, req.ProviderOptions["vertex"])
	if err != nil {
		return nil, fmt.Errorf("vertex: encode anthropic request: %w", err)
	}
	respBytes, _, err := retry.DoHTTP(ctx, p.HTTP, func() (*http.Request, error) {
		tok, err := p.TokenSource.Token(ctx)
		if err != nil {
			return nil, fmt.Errorf("vertex: get access token: %w", err)
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ResolveAnthropicEndpoint(model), bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("vertex: build request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+tok)
		return httpReq, nil
	}, httpread.DefaultMaxResponseBytes)
	if err != nil {
		var h *retry.HTTPError
		if errors.As(err, &h) {
			return nil, &APIError{Status: h.StatusCode, Body: h.Body}
		}
		return nil, fmt.Errorf("vertex: http: %w", err)
	}
	resp, err := decodeAnthropicOnVertexResponse(respBytes, model)
	if err != nil {
		return nil, err
	}
	toolname.RestoreCalls(resp, toolname.Reverse(req.Tools))
	return resp, nil
}

// completeStreamAnthropic is the Anthropic-on-Vertex streaming path.
// Called from CompleteStream when isAnthropicModel(model) is true.
// Wire is standard Anthropic SSE (event-tagged, not the binary
// event-stream format Bedrock uses).

