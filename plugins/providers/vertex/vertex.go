// SPDX-License-Identifier: MIT

// Package vertex is the in-process Google Vertex AI Provider.
//
// **Scope (M1.n):** service-account OAuth + Gemini generateContent
// body shape on the regional aiplatform.googleapis.com endpoint.
// Anthropic-on-Vertex (`@ai-sdk/google-vertex/anthropic`, which uses
// the `:rawPredict` endpoint with the Anthropic Messages body) and
// streaming land in M1.n.x.
//
// Wire (SPEC-15):
//
//	POST https://{region}-aiplatform.googleapis.com/v1/projects/{project}/locations/{region}/publishers/google/models/{model}:generateContent
//	Authorization: Bearer {oauth_access_token}
//	Content-Type: application/json
//
// Auth: see auth.go — JWT-bearer flow against the service account's
// token_uri, with a small in-package cache.
//
// Body shape is identical to plugins/providers/google (Generative
// Language API). We duplicate the encoder/decoder rather than reuse
// google's unexported helpers — Vertex evolves independently and
// the duplication is contained.
package vertex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
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

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("vertex: http: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("vertex: read body: %w", err)
	}
	if httpResp.StatusCode/100 != 2 {
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(respBytes)}
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

func encodeRequest(system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int, jsonMode bool, thinkingBudget int, params agent.Params, extra json.RawMessage) ([]byte, error) {
	// A per-request reasoning effort (M997) overrides the construction-time
	// thinking budget when set; otherwise the env/default budget stands.
	if b, ok := provopts.ThinkingBudget(params.ReasoningEffort, maxTok); ok {
		thinkingBudget = b
	}
	fwd, _ := toolname.Maps(tools)
	wire := vxRequest{}
	if s := strings.TrimSpace(system); s != "" {
		wire.SystemInstruction = &vxContent{Parts: []vxPart{{Text: s}}}
	}
	for _, m := range msgs {
		c, err := canonicalToVertex(m, fwd)
		if err != nil {
			return nil, err
		}
		if c == nil {
			continue
		}
		wire.Contents = append(wire.Contents, *c)
	}
	if len(tools) > 0 {
		decls := make([]vxFunctionDecl, 0, len(tools))
		for _, t := range tools {
			params := t.InputSchema
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			decls = append(decls, vxFunctionDecl{
				Name:        toolname.Wire(fwd, t.Name),
				Description: t.Description,
				Parameters:  params,
			})
		}
		wire.Tools = []vxTool{{FunctionDeclarations: decls}}
	}
	if maxTok > 0 || jsonMode || thinkingBudget != 0 || !params.IsZero() {
		gc := &vxGenConfig{MaxOutputTokens: maxTok}
		if jsonMode {
			gc.ResponseMimeType = "application/json"
		}
		if thinkingBudget != 0 {
			// Opt-in (M320). includeThoughts so the summaries return and land
			// on ReasoningContent; the budget caps thinking tokens (-1 = dynamic).
			gc.ThinkingConfig = &vxThinkingConfig{
				IncludeThoughts: true,
				ThinkingBudget:  thinkingBudget,
			}
		}
		gc.applyParams(params)
		wire.GenerationConfig = gc
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	return provopts.Merge(body, extra)
}

func canonicalToVertex(m agent.Message, fwd map[string]string) (*vxContent, error) {
	switch m.Role {
	case agent.RoleSystem:
		return nil, nil
	case agent.RoleUser:
		// Vision (M245): emit each image attachment (data: URL) as an inlineData
		// part before the text part; skip non-data-URL entries.
		parts := make([]vxPart, 0, len(m.Images)+1)
		for _, img := range m.Images {
			if mt, data, ok := parseImageDataURL(img); ok {
				parts = append(parts, vxPart{InlineData: &vxInlineData{MimeType: mt, Data: data}})
			}
		}
		parts = append(parts, vxPart{Text: m.Content})
		return &vxContent{Role: "user", Parts: parts}, nil
	case agent.RoleAssistant:
		var parts []vxPart
		if strings.TrimSpace(m.Content) != "" {
			parts = append(parts, vxPart{Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			args := tc.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			parts = append(parts, vxPart{
				FunctionCall: &vxFunctionCall{Name: toolname.Wire(fwd, tc.Name), Args: args},
			})
		}
		if len(parts) == 0 {
			parts = []vxPart{{Text: ""}}
		}
		return &vxContent{Role: "model", Parts: parts}, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("vertex: role=tool requires tool_call_id")
		}
		// Surrogate name binding — same caveat as plugins/providers/google.
		// Build the {"result": ...} object with encoding/json, NOT strconv.Quote:
		// strconv.Quote emits Go-only \xNN escapes for control bytes (ANSI \x1b in
		// tool output, NUL, …) that are invalid JSON, which would fail the request
		// encode and wedge the agent loop on Vertex. (M483; same class as M481)
		quoted, err := json.Marshal(m.Content)
		if err != nil {
			return nil, fmt.Errorf("vertex: encode tool result: %w", err)
		}
		resp := json.RawMessage(`{"result":` + string(quoted) + `}`)
		return &vxContent{
			Role: "user",
			Parts: []vxPart{{
				FunctionResponse: &vxFunctionResponse{
					Name:     m.ToolCallID,
					Response: resp,
				},
			}},
		}, nil
	default:
		return nil, fmt.Errorf("vertex: unknown role %q", m.Role)
	}
}

func decodeResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var vr vxResponse
	if err := json.Unmarshal(body, &vr); err != nil {
		return nil, fmt.Errorf("vertex: parse response: %w", err)
	}
	if len(vr.Candidates) == 0 {
		return nil, fmt.Errorf("vertex: response has no candidates")
	}
	cand := vr.Candidates[0]

	var (
		textParts      []string
		reasoningParts []string
		toolCalls      []agent.ToolCall
	)
	for i, part := range cand.Content.Parts {
		switch {
		case part.FunctionCall != nil:
			args := part.FunctionCall.Args
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			toolCalls = append(toolCalls, agent.ToolCall{
				ID:    "call-" + strconv.Itoa(i),
				Name:  part.FunctionCall.Name,
				Input: args,
			})
		case part.Thought && part.Text != "":
			// A thought-summary part (M320) — reasoning, kept out of the answer.
			reasoningParts = append(reasoningParts, part.Text)
		case part.Text != "":
			textParts = append(textParts, part.Text)
		}
	}

	var stop agent.StopReason
	switch {
	case len(toolCalls) > 0:
		stop = agent.StopToolUse
	default:
		switch cand.FinishReason {
		case "STOP", "":
			stop = agent.StopEndTurn
		case "MAX_TOKENS":
			stop = agent.StopMaxTokens
		default:
			stop = agent.StopEndTurn
		}
	}

	usage := agent.Usage{Model: model}
	if vr.UsageMetadata != nil {
		usage.InputTokens = vr.UsageMetadata.PromptTokenCount
		usage.CachedInputTokens = vr.UsageMetadata.CachedContentTokenCount
		// Thinking tokens are billed as output but reported separately (M320);
		// fold them in so OutputTokens reflects the true billable output.
		usage.OutputTokens = vr.UsageMetadata.CandidatesTokenCount + vr.UsageMetadata.ThoughtsTokenCount
	}

	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			Content:   strings.Join(textParts, ""),
			ToolCalls: toolCalls,
		},
		StopReason:       stop,
		Usage:            usage,
		ReasoningContent: strings.Join(reasoningParts, ""),
	}, nil
}
