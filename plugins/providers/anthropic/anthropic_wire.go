// SPDX-License-Identifier: MIT

// Anthropic provider: Provider type + Complete (the entry point) + endpoint
// resolution + auth + retry. The anth* wire types and helpers live in
// anthropic_dialect.go. Day-211 god-file split. Public API unchanged.
package anthropic


import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
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
