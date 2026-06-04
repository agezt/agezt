// SPDX-License-Identifier: MIT

// Package bedrock is the in-process AWS Bedrock Provider.
//
// **Scope (M1.m):** bearer-token auth + Anthropic body shape only.
//
//   - Auth: AWS_BEARER_TOKEN_BEDROCK (long-lived, no SigV4 needed).
//     SigV4-signed requests (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY)
//     land in M1.m.x.
//   - Body: Anthropic Messages API shape (the largest Bedrock use case
//     by usage). Other vendor body shapes — Mistral, Meta, Amazon Titan,
//     Cohere, AI21, DeepSeek — return ErrVendorUnsupported with a hint.
//
// Bedrock's HTTP wire:
//
//	POST https://bedrock-runtime.{region}.amazonaws.com/model/{modelID}/invoke
//	Authorization: Bearer {AWS_BEARER_TOKEN_BEDROCK}
//	Content-Type: application/json
//
// The model ID is interpolated into the URL path (not the request body),
// and the body carries `anthropic_version: "bedrock-2023-05-31"` instead
// of a `model` field. Otherwise the body is the same Messages-API shape
// the anthropic adapter speaks.
//
// Cross-region inference profiles (`us.anthropic.*`, `eu.anthropic.*`,
// `global.anthropic.*`, etc.) are recognised as Anthropic too — vendor
// detection looks for the `anthropic.` segment anywhere in the model id.
package bedrock

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
)

const (
	// AnthropicBedrockVersion is the value sent in the
	// `anthropic_version` body field. Bedrock pins this; updating
	// requires coordination with the AWS team's release notes.
	AnthropicBedrockVersion = "bedrock-2023-05-31"
	// DefaultMaxTokens is what we send when the request leaves it at 0.
	DefaultMaxTokens = 4096
	// DefaultTimeout caps a single HTTP request.
	DefaultTimeout = 5 * time.Minute
)

// Provider is the in-process Bedrock Provider.
type Provider struct {
	// BearerToken is the AWS_BEARER_TOKEN_BEDROCK value. Mutually
	// exclusive with SigV4Creds — set whichever the operator's
	// environment provides. If both are set, BearerToken wins
	// (it's a thinner wire-time decision; SigV4 is the fallback).
	BearerToken string
	// SigV4Creds, when set, switches Complete/CompleteStream from
	// "Authorization: Bearer ..." to AWS SigV4 signing (M1.m.x).
	// Use SetSigV4Creds() rather than touching the field directly
	// to keep room for future credential providers.
	sigV4 *SigV4Creds
	// Endpoint, if set, is the full URL to POST to (including
	// `/invoke`). Useful for tests with httptest.NewServer. When set,
	// Region/BaseURL/Model are ignored for URL routing.
	Endpoint string
	// BaseURL lets the catalog/compat layer override the
	// bedrock-runtime host (custom.json escape hatch). Empty falls
	// back to `https://bedrock-runtime.{Region}.amazonaws.com`.
	BaseURL string
	// Region is required when BaseURL/Endpoint are unset. Also used
	// for SigV4 credential scoping when SigV4Creds is set.
	Region string
	// Model is the Bedrock model id (e.g. "anthropic.claude-opus-4-7"
	// or "us.anthropic.claude-sonnet-4-5-20250929-v1:0").
	Model string
	HTTP  *http.Client
	// Now overrides time.Now for SigV4 timestamp generation. Tests
	// pin it to compare against AWS's published canonical-request
	// test vectors; production leaves it nil.
	Now func() time.Time
}

// SetSigV4Creds switches the provider from bearer-token auth to
// AWS SigV4 signing. Pass nil to revert. Region must be set on the
// Provider before any request — it's part of the SigV4 credential
// scope and a per-day per-region signing key.
func (p *Provider) SetSigV4Creds(creds *SigV4Creds) {
	p.sigV4 = creds
}

// hasAuth reports whether at least one of {BearerToken, SigV4Creds}
// is configured. Used by Complete/CompleteStream to fail fast with
// ErrNoBearerToken before encoding the request.
func (p *Provider) hasAuth() bool {
	return p.BearerToken != "" || (p.sigV4 != nil && p.sigV4.AccessKeyID != "")
}

// applyAuth attaches whichever auth header(s) are configured. Bearer
// is the simpler / preferred path; SigV4 is the fallback for
// operators on classic IAM credentials. body is the request body —
// SigV4 hashes it as part of the canonical request, so we have to
// pass the bytes through (the http.Request already has them via
// bytes.NewReader, but we keep them separately to avoid re-reading
// the Body which is a one-shot reader).
func (p *Provider) applyAuth(req *http.Request, body []byte) error {
	if p.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.BearerToken)
		return nil
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	return signRequest(req, p.Region, body, *p.sigV4, now())
}

// New constructs a Provider with sensible defaults.
func New(bearer, region string) *Provider {
	return &Provider{
		BearerToken: bearer,
		Region:      region,
		HTTP:        &http.Client{Timeout: DefaultTimeout},
	}
}

// ErrNoBearerToken is returned by Complete when neither BearerToken
// nor SigV4 credentials are configured. The message lists both
// supported paths so operators see the full menu.
var ErrNoBearerToken = errors.New("bedrock: no auth configured — set AWS_BEARER_TOKEN_BEDROCK *or* (AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY) in the vault")

// ErrVendorUnsupported is returned when the Bedrock model id maps to
// a vendor whose body shape isn't wired in this build.
var ErrVendorUnsupported = errors.New("bedrock: vendor body shape not yet supported")

// APIError is returned for non-2xx upstream responses.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("bedrock: status %d: %s", e.Status, e.Body)
}

// Name implements agent.Provider.
func (p *Provider) Name() string { return "bedrock" }

// ResolveEndpoint returns the URL Complete will POST to for a given
// model. Exported so callers (and tests) can verify the routing
// decision without an HTTP round-trip.
//
//  1. explicit p.Endpoint (used by tests / custom.json full-URL override)
//  2. p.BaseURL  + "/model/{model}/invoke"  (custom.json host override)
//  3. derived:    https://bedrock-runtime.{Region}.amazonaws.com/model/{model}/invoke
func (p *Provider) ResolveEndpoint(model string) string {
	if p.Endpoint != "" {
		return p.Endpoint
	}
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		base = "https://bedrock-runtime." + p.Region + ".amazonaws.com"
	}
	return base + "/model/" + model + "/invoke"
}

// isAnthropicModel reports whether the Bedrock model id maps to the
// Anthropic Messages-API body shape. Covers both direct ids
// (`anthropic.claude-...`) and regional cross-inference profiles
// (`us.anthropic.claude-...`, `eu.anthropic.claude-...`, etc.).
func isAnthropicModel(id string) bool {
	if strings.HasPrefix(id, "anthropic.") {
		return true
	}
	// Regional profile: prefix segment + "." + "anthropic." + ...
	if i := strings.Index(id, ".anthropic."); i >= 0 && i < len(id)-len(".anthropic.") {
		return true
	}
	return false
}

// isMistralModel reports whether the model id maps to the
// Mistral-on-Bedrock body shape (M1.tt). Covers both direct ids
// (`mistral.mistral-large-2407-v1:0`) and regional cross-inference
// profiles (`eu.mistral.*`, `us.mistral.*`).
func isMistralModel(id string) bool {
	if strings.HasPrefix(id, "mistral.") {
		return true
	}
	if i := strings.Index(id, ".mistral."); i >= 0 && i < len(id)-len(".mistral.") {
		return true
	}
	return false
}

// isCohereModel reports whether the model id maps to the
// Cohere-on-Bedrock body shape (M1.tt-2). Cohere Command R/R+
// use a `message` / `chat_history` request shape.
func isCohereModel(id string) bool {
	if strings.HasPrefix(id, "cohere.") {
		return true
	}
	if i := strings.Index(id, ".cohere."); i >= 0 && i < len(id)-len(".cohere.") {
		return true
	}
	return false
}

// isMetaLlamaModel reports whether the model id maps to the
// Meta-Llama-on-Bedrock body shape (M1.tt-3). Uses the
// prompt-template format: `<|begin_of_text|><|start_header_id|>user
// <|end_header_id|>...<|eot_id|>` etc. No tool use through the
// raw prompt template; chat-only.
func isMetaLlamaModel(id string) bool {
	if strings.HasPrefix(id, "meta.") {
		return true
	}
	if i := strings.Index(id, ".meta."); i >= 0 && i < len(id)-len(".meta.") {
		return true
	}
	return false
}

// Complete implements agent.Provider.
func (p *Provider) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if !p.hasAuth() {
		return nil, ErrNoBearerToken
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		return nil, errors.New("bedrock: model id required (must be in CompletionRequest.Model or p.Model)")
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}

	var (
		body       []byte
		err        error
		decodeResp func([]byte, string) (*agent.CompletionResponse, error)
	)
	switch {
	case isAnthropicModel(model):
		body, err = encodeAnthropicOnBedrockRequest(req.System, req.Messages, req.Tools, maxTokens)
		decodeResp = decodeAnthropicOnBedrockResponse
	case isMistralModel(model):
		body, err = encodeMistralOnBedrockRequest(req.System, req.Messages, maxTokens)
		decodeResp = decodeMistralOnBedrockResponse
	case isCohereModel(model):
		body, err = encodeCohereOnBedrockRequest(req.System, req.Messages, maxTokens)
		decodeResp = decodeCohereOnBedrockResponse
	case isMetaLlamaModel(model):
		body, err = encodeMetaLlamaOnBedrockRequest(req.System, req.Messages, maxTokens)
		decodeResp = decodeMetaLlamaOnBedrockResponse
	case isAI21JambaModel(model):
		body, err = encodeAI21JambaOnBedrockRequest(req.System, req.Messages, maxTokens)
		decodeResp = decodeAI21JambaOnBedrockResponse
	default:
		return nil, fmt.Errorf("%w: model %q is not in a supported family (anthropic.*, mistral.*, cohere.*, meta.*, ai21.jamba.*; amazon Titan and AI21 J2 are intentionally unwired)",
			ErrVendorUnsupported, model)
	}
	if err != nil {
		return nil, fmt.Errorf("bedrock: encode request: %w", err)
	}

	endpoint := p.ResolveEndpoint(model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("bedrock: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if err := p.applyAuth(httpReq, body); err != nil {
		return nil, err
	}

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("bedrock: http: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("bedrock: read body: %w", err)
	}
	if httpResp.StatusCode/100 != 2 {
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(respBytes)}
	}
	return decodeResp(respBytes, model)
}

// ----- dialect translation (canonical ↔ Anthropic-on-Bedrock body) -----
//
// The shape is identical to plugins/providers/anthropic with two
// differences:
//   - No `model` field (model id is in the URL path).
//   - `anthropic_version: "bedrock-2023-05-31"` is required.
//
// We re-encode rather than reuse plugins/providers/anthropic's
// unexported helpers; the duplication is small and keeps Bedrock
// independent of the direct-Anthropic adapter's evolution.

type anthBedrockRequest struct {
	AnthropicVersion string        `json:"anthropic_version"`
	MaxTokens        int           `json:"max_tokens"`
	System           string        `json:"system,omitempty"`
	Messages         []anthMessage `json:"messages"`
	Tools            []anthTool    `json:"tools,omitempty"`
}

type anthTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
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
func anthBedrockUsageToAgent(inputTokens, cacheRead, cacheCreation, outputTokens int, model string) agent.Usage {
	return agent.Usage{
		InputTokens:           inputTokens + cacheRead + cacheCreation,
		CachedInputTokens:     cacheRead,
		CacheWriteInputTokens: cacheCreation,
		OutputTokens:          outputTokens,
		Model:                 model,
	}
}

func encodeAnthropicOnBedrockRequest(system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int) ([]byte, error) {
	wire := anthBedrockRequest{
		AnthropicVersion: AnthropicBedrockVersion,
		MaxTokens:        maxTok,
		System:           system,
	}
	for _, t := range tools {
		wire.Tools = append(wire.Tools, anthTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	for _, m := range msgs {
		am, err := canonicalToAnth(m)
		if err != nil {
			return nil, err
		}
		if am == nil {
			continue
		}
		wire.Messages = append(wire.Messages, *am)
	}
	return json.Marshal(wire)
}

// parseImageDataURL splits an RFC 2397 data: URL of the form
// "data:<media-type>;base64,<payload>" into its media type and base64 payload,
// returning ok=false for anything else (including a legacy bare filename),
// which the caller skips. The CLI sends data: URLs (M241).
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

func canonicalToAnth(m agent.Message) (*anthMessage, error) {
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
				Name:  tc.Name,
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
