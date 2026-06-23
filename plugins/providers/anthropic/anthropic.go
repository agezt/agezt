// SPDX-License-Identifier: MIT

// Package anthropic is the in-process Anthropic Messages-API Provider.
//
// It translates between Agezt's canonical (dialect-free) agent.Message /
// agent.ToolCall / agent.ToolDef shapes and Anthropic's content-block
// format (SPEC-15). Non-streaming for M0.5; streaming lands later.
//
// Auth: api-key via the AGEZT_ANTHROPIC_API_KEY env var (or the constructor
// argument). OAuth/subscription auth lands with the Governor in MVP
// (TASKS P1-PROV-01).
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
	"github.com/agezt/agezt/plugins/providers/internal/retry"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)

const (
	// DefaultEndpoint is the Anthropic Messages API URL.
	DefaultEndpoint = "https://api.anthropic.com/v1/messages"
	// APIVersion is the value of the anthropic-version header.
	APIVersion = "2023-06-01"
	// DefaultMaxTokens is the cap applied when CompletionRequest.MaxTokens=0.
	DefaultMaxTokens = 4096
	// DefaultTimeout caps a single HTTP request.
	DefaultTimeout = 5 * time.Minute
)

// Provider is the in-process Anthropic Provider implementation.
type Provider struct {
	APIKey string
	// Endpoint is the full Messages API URL. If empty, BaseURL is used
	// (appending /messages); if both are empty, DefaultEndpoint.
	Endpoint string
	// BaseURL is the AI-SDK-style Anthropic base URL — the value the
	// catalog/compat layer passes from models.dev's `api` field, which
	// already includes the version segment (e.g.
	// "https://api.anthropic.com/v1", or a third-party Anthropic-shaped
	// endpoint like "https://api.minimax.io/anthropic/v1"). This Provider
	// appends only "/messages", matching the @ai-sdk/anthropic convention.
	// Ignored when Endpoint is set.
	BaseURL string
	Model   string
	HTTP    *http.Client
	// ThinkingBudget, when > 0, enables Claude extended thinking (M318) with this
	// many reasoning tokens. The chain of thought is captured into
	// CompletionResponse.ReasoningContent (M317). 0 disables it (default).
	// Operator opt-in via AGEZT_ANTHROPIC_THINKING_BUDGET — thinking costs extra
	// tokens, so it's off unless asked for. Clamped up to Anthropic's 1024 minimum.
	ThinkingBudget int
}

// MinThinkingBudget is Anthropic's minimum extended-thinking budget_tokens.
const MinThinkingBudget = 1024

// New constructs a Provider with sensible defaults.
func New(apiKey string) *Provider {
	return &Provider{
		APIKey:   apiKey,
		Endpoint: DefaultEndpoint,
		HTTP:     &http.Client{Timeout: DefaultTimeout},
	}
}

// resolveEndpoint returns the URL to POST to, derived in this order:
//
//  1. explicit p.Endpoint
//  2. p.BaseURL + "/messages" (the catalog/compat path — BaseURL already
//     carries the version segment per the @ai-sdk/anthropic convention,
//     so we append only "/messages"; models.dev's `api` for third-party
//     Anthropic-shaped providers ends in "/v1", "/anthropic/v1", etc.)
//  3. DefaultEndpoint
func (p *Provider) resolveEndpoint() string {
	if p.Endpoint != "" {
		return p.Endpoint
	}
	if p.BaseURL != "" {
		base := p.BaseURL
		for len(base) > 0 && base[len(base)-1] == '/' {
			base = base[:len(base)-1]
		}
		return base + "/messages"
	}
	return DefaultEndpoint
}

// Name implements agent.Provider.
func (p *Provider) Name() string { return "anthropic" }

// ErrNoAPIKey is returned by Complete when APIKey is empty.
var ErrNoAPIKey = errors.New("anthropic: API key not set")

// ErrNoModel is returned when a completion request carries no model and the
// provider has none set. The daemon ships with no default model (owner rule), so
// the model must come from the request (AGEZT_MODEL / routing / a fallback chain).
var ErrNoModel = errors.New("anthropic: no model specified (set CompletionRequest.Model, AGEZT_MODEL, or a routing/fallback chain)")

// APIError is returned for non-2xx responses; it carries the upstream
// status and body so callers (and the journal) can record the failure.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("anthropic: status %d: %s", e.Status, e.Body)
}

// Complete implements agent.Provider. It honors ctx for HTTP cancellation
// (which is how the agent loop reacts to `agt halt`).
func (p *Provider) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if p.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		return nil, ErrNoModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}

	body, err := encodeRequest(model, req.System, req.Messages, req.Tools, maxTokens, p.ThinkingBudget, req.Params, req.ProviderOptions["anthropic"])
	if err != nil {
		return nil, fmt.Errorf("anthropic: encode request: %w", err)
	}

	endpoint := p.resolveEndpoint()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", APIVersion)
	httpReq.Header.Set("x-api-key", p.APIKey)

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}

	// Retry logic with exponential backoff for transient errors (429, 5xx)
	var respBytes []byte
	httpErr := retry.Do(ctx, retry.DefaultConfig, func() error {
		// We need to recreate the request body for each retry since bytes.Reader
		// can only be read once. Use a func to capture the body bytes.
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("anthropic-version", APIVersion)
		req.Header.Set("x-api-key", p.APIKey)

		httpResp, err := client.Do(req)
		if err != nil {
			return &retry.TransientError{Err: err}
		}
		defer httpResp.Body.Close()

		respBytes, err = httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
		if err != nil {
			return fmt.Errorf("anthropic: read body: %w", err)
		}
		if httpResp.StatusCode/100 != 2 {
			return retry.NewHTTPError(httpResp, string(respBytes))
		}
		return nil
	})
	if httpErr != nil {
		if h, ok := httpErr.(*retry.HTTPError); ok {
			return nil, &APIError{Status: h.StatusCode, Body: h.Body}
		}
		return nil, httpErr
	}
	resp, err := decodeResponse(respBytes)
	if err != nil {
		return nil, err
	}
	// Map any sanitized tool names back to their originals so the call routes to
	// the real tool (mirrors the request-side wireToolNames conformance).
	toolname.RestoreCalls(resp, toolname.Reverse(req.Tools))
	return resp, nil
}

// ----- dialect translation (canonical ↔ Anthropic) -----

// anthRequest is the wire-shape of a Messages API request. System is `any` so it
// can be the plain-string form OR the block-array form that carries a prompt-cache
// breakpoint (M301); buildAnthSystem picks the shape.
type anthRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    any           `json:"system,omitempty"`
	Messages  []anthMessage `json:"messages"`
	Tools     []anthTool    `json:"tools,omitempty"`
	Thinking  *anthThinking `json:"thinking,omitempty"` // extended thinking (M318)
	// Per-request sampling knobs (M997). Anthropic has no seed / penalties.
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"top_p,omitempty"`
	TopK          *int     `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`
}

// applyParams copies the universal sampling knobs Anthropic understands. The
// reasoning knob is handled separately (mapped to a thinking budget) and so is
// ignored here. An unset Params leaves the request unchanged.
func (wire *anthRequest) applyParams(p agent.Params) {
	if p.IsZero() {
		return
	}
	wire.Temperature = p.Temperature
	wire.TopP = p.TopP
	wire.TopK = p.TopK
	wire.StopSequences = p.Stop
}

// anthThinking is Anthropic's extended-thinking request block.
type anthThinking struct {
	Type         string `json:"type"`          // "enabled"
	BudgetTokens int    `json:"budget_tokens"` // >= 1024, and < max_tokens
}

// thinkingConfig returns the thinking block + the max_tokens to use when an
// extended-thinking budget is set. Anthropic requires max_tokens > budget_tokens
// (thinking tokens count toward the output allowance), and budget >= 1024, so we
// clamp the budget up and ensure max_tokens leaves room for the answer on top.
// budget <= 0 → (nil, maxTok unchanged): thinking off, wire byte-identical.
func thinkingConfig(budget, maxTok int) (*anthThinking, int) {
	if budget <= 0 {
		return nil, maxTok
	}
	if budget < MinThinkingBudget {
		budget = MinThinkingBudget
	}
	if maxTok <= budget {
		maxTok = budget + DefaultMaxTokens // room for the answer beyond the thinking
	}
	return &anthThinking{Type: "enabled", BudgetTokens: budget}, maxTok
}

// anthSystemBlock is one element of the array form of the system prompt — used
// when caching so the (large, stable) system prompt carries a cache_control
// breakpoint. Anthropic caches the prefix tools→system, so marking the system
// block caches tools AND system, the whole stable prefix of an agent loop.
type anthSystemBlock struct {
	Type         string            `json:"type"` // "text"
	Text         string            `json:"text"`
	CacheControl *anthCacheControl `json:"cache_control,omitempty"`
}

// buildAnthSystem returns the system field: nil when empty (omitted), else a
// one-element block array with a cache_control breakpoint (M301). Anthropic
// accepts both the string and the array form; the array form lets the stable
// system prompt be cached alongside the tools (cache reads bill at ~0.1× input).
func buildAnthSystem(system string) any {
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

// anthCacheControl marks a content/tool block as a prompt-cache breakpoint
// (M299). Anthropic caches the request prefix up to and including the marked
// block; "ephemeral" is the 5-minute TTL tier.
type anthCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// buildAnthTools converts canonical tool defs to Anthropic's wire shape and marks
// the LAST tool with cache_control (M299). Anthropic caches the prefix up to and
// including the marked block, so this caches the whole tools array — the large,
// stable part of an agent loop's request that repeats every iteration. Anthropic
// silently ignores the marker when the prefix is below the minimum cacheable size
// (so it's safe to always set), and cache reads bill at ~0.1× input (M289-291),
// turning the repeated tools into a real saving (surfaced by `agt cache`).
func buildAnthTools(tools []agent.ToolDef, fwd map[string]string) []anthTool {
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
	Role    string      `json:"role"` // "user" | "assistant"
	Content []anthBlock `json:"content"`
}

// anthBlock is a single content block. Only one of Text / ToolUse /
// ToolResult is populated per block; the wire format is a tagged union.
type anthBlock struct {
	Type       string           `json:"type"`
	Text       string           `json:"text,omitempty"`
	ID         string           `json:"id,omitempty"`          // tool_use
	Name       string           `json:"name,omitempty"`        // tool_use
	Input      json.RawMessage  `json:"input,omitempty"`       // tool_use
	ToolUseID  string           `json:"tool_use_id,omitempty"` // tool_result
	ResultBody string           `json:"content,omitempty"`     // tool_result (string form)
	IsError    bool             `json:"is_error,omitempty"`    // tool_result
	Source     *anthImageSource `json:"source,omitempty"`      // type=image
	Thinking   string           `json:"thinking,omitempty"`    // type=thinking (M318)
}

// anthImageSource is the base64 payload of a type=image content block (M241).
type anthImageSource struct {
	Type      string `json:"type"`       // always "base64"
	MediaType string `json:"media_type"` // image/png | image/jpeg | image/gif | image/webp
	Data      string `json:"data"`       // base64-encoded image bytes
}

type anthResponse struct {
	ID         string      `json:"id"`
	Type       string      `json:"type"`
	Role       string      `json:"role"`
	Content    []anthBlock `json:"content"`
	Model      string      `json:"model"`
	StopReason string      `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// anthUsageToAgent maps Anthropic's split token counts to the canonical
// agent.Usage (M290). Anthropic reports input_tokens EXCLUDING cached prompt
// tokens, with cache_read_input_tokens and cache_creation_input_tokens reported
// separately — so the real prompt size is their sum. Cache reads are marked
// cached (billed at the cheaper cache-read rate, M289) and cache-creation as
// cache-write (billed at the cache-write premium, M291). Before this, the two
// cache counts were dropped, so cached prompt tokens were billed at zero (an
// under-count when caching was on).
func anthUsageToAgent(inputTokens, cacheRead, cacheCreation, outputTokens int, model string) agent.Usage {
	return agent.Usage{
		InputTokens:           inputTokens + cacheRead + cacheCreation,
		CachedInputTokens:     cacheRead,
		CacheWriteInputTokens: cacheCreation,
		OutputTokens:          outputTokens,
		Model:                 model,
	}
}

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
