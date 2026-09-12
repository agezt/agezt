// SPDX-License-Identifier: MIT

// Vertex provider: wire types + applyParams.
// Code extracted from vertex.go during the Day-110 god-file split.
// Public API unchanged.
package vertex


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
	"github.com/agezt/agezt/plugins/providers/internal/retry"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)


const (
	// DefaultAPIVersion is the Vertex AI REST API version path segment.
	DefaultAPIVersion = "v1"
	// DefaultTimeout caps a single HTTP request.
	DefaultTimeout = 5 * time.Minute
)

// Provider is the in-process Vertex AI Provider.
type Provider struct {
	// TokenSource mints OAuth access tokens — a service-account
	// (*TokenSource) or the GCE/GKE metadata server
	// (*MetadataTokenSource). Required.
	TokenSource TokenMinter
	// Project is the GCP project id (numeric or alias).
	Project string
	// Location is the Vertex region (e.g. "us-central1", "europe-west4").
	Location string
	// Endpoint, if set, is the full URL to POST to (tests use this).
	// When set, BaseURL / Project / Location are ignored for routing.
	Endpoint string
	// BaseURL lets the catalog/compat layer pass a custom host
	// (e.g. for a VPC service-control regional alias). Empty falls
	// back to "https://{Location}-aiplatform.googleapis.com".
	BaseURL string
	Model   string
	HTTP    *http.Client
	// ThinkingBudget enables reasoning on Vertex when non-zero. The two
	// publishers interpret it per their own rules at encode time:
	//   - native Gemini (M320): thinkingConfig.thinkingBudget — a positive cap,
	//     or -1 for a dynamic budget; includeThoughts returns the summaries.
	//   - Anthropic / claude-* (M321): thinking.budget_tokens — clamped up to
	//     Anthropic's 1024 floor, with max_tokens bumped above it; a negative
	//     ("dynamic", Gemini-only) value means off here.
	// Either way the chain of thought lands on ReasoningContent (M317). 0 sends
	// no thinking config at all — wire byte-identical to a non-thinking run.
	ThinkingBudget int
}

// New constructs a Provider. ts may be nil at construction (set
// later); Complete will error if it's still nil. ts is any TokenMinter —
// a service-account *TokenSource or a *MetadataTokenSource.
func New(ts TokenMinter, project, location string) *Provider {
	return &Provider{
		TokenSource: ts,
		Project:     project,
		Location:    location,
		HTTP:        &http.Client{Timeout: DefaultTimeout},
	}
}

// Name implements agent.Provider.
func (p *Provider) Name() string { return "google-vertex" }

// ErrNoTokenSource is returned by Complete when TokenSource is nil.
var ErrNoTokenSource = errors.New("vertex: TokenSource not configured")

// ErrNoModel is returned when a completion request carries no model and the
// provider has none set. The daemon ships with no default model (owner rule), so
// the model must come from the request (AGEZT_MODEL / routing / a fallback chain).
var ErrNoModel = errors.New("vertex: no model specified (set CompletionRequest.Model, AGEZT_MODEL, or a routing/fallback chain)")

// APIError is returned for non-2xx upstream responses.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("vertex: status %d: %s", e.Status, e.Body)
}

// ResolveEndpoint returns the URL Complete will POST to for the given
// model. Exported for tests + custom-URL verification.
//
//  1. explicit p.Endpoint
//  2. p.BaseURL  + "/v1/projects/{project}/locations/{location}/publishers/google/models/{model}:generateContent"
//  3. https://{location}-aiplatform.googleapis.com + (2)'s suffix
func (p *Provider) ResolveEndpoint(model string) string {
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
		"/publishers/google/models/" + model + ":generateContent"
}

// Complete implements agent.Provider.
func (p *Provider) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if p.TokenSource == nil {
		return nil, ErrNoTokenSource
	}
	if p.Project == "" && p.Endpoint == "" {
		return nil, errors.New("vertex: Project required (or set Endpoint directly)")
	}
	if p.Location == "" && p.Endpoint == "" {
		return nil, errors.New("vertex: Location required (or set Endpoint directly)")
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		return nil, ErrNoModel
	}

	// Anthropic-on-Vertex (`claude-*` model ids) speaks a different
	// publisher (anthropic), endpoint suffix (:rawPredict), and body
	// shape (Anthropic Messages API) than native Gemini. Branch
	// before encoding so the wire matches Vertex's per-publisher
	// dispatch. M1.n.x.
	if isAnthropicModel(model) {
		return p.completeAnthropic(ctx, req, model)
	}

	body, err := encodeRequest(req.System, req.Messages, req.Tools, req.MaxTokens, req.JSONMode, p.ThinkingBudget, req.Params, req.ProviderOptions["vertex"])
	if err != nil {
		return nil, fmt.Errorf("vertex: encode request: %w", err)
	}

	// build runs per retry attempt so a token refreshed mid-backoff (short-lived
	// OAuth access tokens) is picked up fresh each try.
	respBytes, _, err := retry.DoHTTP(ctx, p.HTTP, func() (*http.Request, error) {
		tok, err := p.TokenSource.Token(ctx)
		if err != nil {
			return nil, fmt.Errorf("vertex: get access token: %w", err)
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ResolveEndpoint(model), bytes.NewReader(body))
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
	resp, err := decodeResponse(respBytes, model)
	if err != nil {
		return nil, err
	}
	toolname.RestoreCalls(resp, toolname.Reverse(req.Tools))
	return resp, nil
}

// ----- dialect translation (canonical ↔ Vertex generateContent) -----
//
// Identical shape to plugins/providers/google. Duplicated rather
// than shared via an internal package; Vertex evolves independently.

type vxRequest struct {
	Contents          []vxContent  `json:"contents"`
	Tools             []vxTool     `json:"tools,omitempty"`
	SystemInstruction *vxContent   `json:"systemInstruction,omitempty"`
	GenerationConfig  *vxGenConfig `json:"generationConfig,omitempty"`
}

type vxGenConfig struct {
	MaxOutputTokens  int               `json:"maxOutputTokens,omitempty"`
	ResponseMimeType string            `json:"responseMimeType,omitempty"` // "application/json" → JSON mode (M312)
	ThinkingConfig   *vxThinkingConfig `json:"thinkingConfig,omitempty"`   // M320
	// Per-request sampling knobs (M997). Gemini-on-Vertex nests these inside
	// generationConfig (NOT top-level), and has no seed / penalties. An unset
	// agent.Params leaves every field nil/empty (omitempty), so the request
	// stays byte-for-byte unchanged.
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"topP,omitempty"`
	TopK          *int     `json:"topK,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

// applyParams copies the universal sampling knobs Gemini understands into the
// generationConfig. Reasoning is handled separately (mapped to a thinking
// budget), so it is ignored here. An unset Params leaves the config unchanged.
func (gc *vxGenConfig) applyParams(p agent.Params) {
	if p.IsZero() {
		return
	}
	gc.Temperature = p.Temperature
	gc.TopP = p.TopP
	gc.TopK = p.TopK
	gc.StopSequences = p.Stop
}

// vxThinkingConfig is Gemini-on-Vertex's per-request thinking control (M320,
// 2.5-series). Mirrors plugins/providers/google's geminiThinkingConfig:
// IncludeThoughts asks Vertex to return thought summaries as parts flagged
// `thought:true`; ThinkingBudget caps the thinking tokens (-1 = dynamic).
type vxThinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts"`
	ThinkingBudget  int  `json:"thinkingBudget"`
}

type vxContent struct {
	Role  string   `json:"role,omitempty"` // "user" | "model"; absent for systemInstruction
	Parts []vxPart `json:"parts"`
}

type vxPart struct {
	Text             string              `json:"text,omitempty"`
	Thought          bool                `json:"thought,omitempty"` // M320: a thought-summary part (reasoning, not answer)
	InlineData       *vxInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *vxFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *vxFunctionResponse `json:"functionResponse,omitempty"`
}

// vxInlineData is an inline base64 blob part — how Gemini-on-Vertex's
// generateContent API carries an image attachment (M245).
type vxInlineData struct {
	MimeType string `json:"mimeType"` // image/png, image/jpeg, image/gif, image/webp
	Data     string `json:"data"`     // base64-encoded image bytes
}

type vxFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type vxFunctionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type vxTool struct {
	FunctionDeclarations []vxFunctionDecl `json:"functionDeclarations"`
}

type vxFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type vxResponse struct {
	Candidates    []vxCandidate    `json:"candidates"`
	UsageMetadata *vxUsageMetadata `json:"usageMetadata,omitempty"`
}

type vxCandidate struct {
	Content      vxContent `json:"content"`
	FinishReason string    `json:"finishReason"`
}

type vxUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"` // Gemini context cache (M294-cache)
	// ThoughtsTokenCount is the thinking-token count (M320), reported
	// separately from CandidatesTokenCount but billed at the output rate;
	// folded into Usage.OutputTokens on decode.
	ThoughtsTokenCount int `json:"thoughtsTokenCount"`
}

