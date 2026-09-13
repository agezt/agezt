// SPDX-License-Identifier: MIT

// Package bedrock: AWS Bedrock Provider — consts + Provider struct + auth +
// New + APIError + Name + ResolveEndpoint + Complete + headerTokenCount.
// The vendor-detection predicates (isAnthropicModel + isMistralModel +
// isCohereModel + isMetaLlamaModel) moved to bedrock_models.go.
// Day-211 god-file split. Public API unchanged.
package bedrock


import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
	"github.com/agezt/agezt/plugins/providers/internal/retry"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
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

	// A single dialect key carries per-request provider extras for every Bedrock
	// model family (M997); the encoders overlay it after marshalling their wire
	// body. agent.Params (sampling knobs) is threaded the same way.
	extra := req.ProviderOptions["bedrock"]
	var (
		body       []byte
		err        error
		decodeResp func([]byte, string) (*agent.CompletionResponse, error)
	)
	switch {
	case isAnthropicModel(model):
		body, err = encodeAnthropicOnBedrockRequest(req.System, req.Messages, req.Tools, maxTokens, req.Params, extra)
		decodeResp = decodeAnthropicOnBedrockResponse
	case isMistralModel(model):
		body, err = encodeMistralOnBedrockRequest(req.System, req.Messages, maxTokens, req.Params, extra)
		decodeResp = decodeMistralOnBedrockResponse
	case isCohereModel(model):
		body, err = encodeCohereOnBedrockRequest(req.System, req.Messages, maxTokens, req.Params, extra)
		decodeResp = decodeCohereOnBedrockResponse
	case isMetaLlamaModel(model):
		body, err = encodeMetaLlamaOnBedrockRequest(req.System, req.Messages, maxTokens, req.Params, extra)
		decodeResp = decodeMetaLlamaOnBedrockResponse
	case isAI21JambaModel(model):
		body, err = encodeAI21JambaOnBedrockRequest(req.System, req.Messages, maxTokens, req.Params, extra)
		decodeResp = decodeAI21JambaOnBedrockResponse
	case isAmazonNovaModel(model):
		body, err = encodeNovaOnBedrockRequest(req.System, req.Messages, maxTokens, req.Params, extra)
		decodeResp = decodeNovaOnBedrockResponse
	case isDeepSeekModel(model):
		body, err = encodeDeepSeekOnBedrockRequest(req.System, req.Messages, maxTokens, req.Params, extra)
		decodeResp = decodeDeepSeekOnBedrockResponse
	default:
		return nil, fmt.Errorf("%w: model %q is not in a supported family (anthropic.*, mistral.*, cohere.*, meta.*, ai21.jamba.*, amazon.nova.*, deepseek.*; legacy amazon Titan and AI21 J2 are intentionally unwired)",
			ErrVendorUnsupported, model)
	}
	if err != nil {
		return nil, fmt.Errorf("bedrock: encode request: %w", err)
	}

	endpoint := p.ResolveEndpoint(model)
	// build runs per retry attempt so the SigV4 signature (date-scoped) is
	// re-computed fresh for each try.
	respBytes, respHeader, err := retry.DoHTTP(ctx, p.HTTP, func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("bedrock: build request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if err := p.applyAuth(httpReq, body); err != nil {
			return nil, err
		}
		return httpReq, nil
	}, httpread.DefaultMaxResponseBytes)
	if err != nil {
		var h *retry.HTTPError
		if errors.As(err, &h) {
			return nil, &APIError{Status: h.StatusCode, Body: h.Body}
		}
		return nil, fmt.Errorf("bedrock: http: %w", err)
	}
	resp, err := decodeResp(respBytes, model)
	if err != nil {
		return nil, err
	}
	toolname.RestoreCalls(resp, toolname.Reverse(req.Tools))
	// Bedrock reports the authoritative token counts in response headers for every
	// InvokeModel call (M327). Vendors whose body carries no inline usage (Mistral,
	// Cohere) would otherwise report zero spend to the governor — under-billing the
	// run. Fill from the headers when the decoded usage is empty; vendors with
	// inline counts (Anthropic, Nova, Meta-Llama, AI21 Jamba) keep their richer
	// body-derived usage (e.g. Anthropic's cache-read/write breakdown).
	if resp.Usage.InputTokens == 0 && resp.Usage.OutputTokens == 0 {
		resp.Usage.InputTokens = headerTokenCount(respHeader, "X-Amzn-Bedrock-Input-Token-Count")
		resp.Usage.OutputTokens = headerTokenCount(respHeader, "X-Amzn-Bedrock-Output-Token-Count")
		if resp.Usage.Model == "" {
			resp.Usage.Model = model
		}
	}
	return resp, nil
}

// headerTokenCount reads a non-negative integer token count from a Bedrock
// response header, returning 0 for a missing or unparseable value (so a quirky
// proxy that strips or mangles the header degrades to "unknown spend" rather than
// an error).
func headerTokenCount(h http.Header, name string) int {
	v := strings.TrimSpace(h.Get(name))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
