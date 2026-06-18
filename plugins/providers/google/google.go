// SPDX-License-Identifier: MIT

// Package google is the in-process Google Gemini Provider, talking to
// the Generative Language API at generativelanguage.googleapis.com.
//
// Wire shape (SPEC-15): the Gemini API is meaningfully different from
// OpenAI's — top-level `contents` instead of `messages`, parts arrays
// instead of strings, `model` role instead of `assistant`, tool
// declarations under `tools[0].functionDeclarations`, tool results
// folded back into a user message as `functionResponse` parts.
//
// Auth: API key passed via the `x-goog-api-key` header (preferred over
// the `?key=...` query param so the key doesn't end up in logs). The
// key comes from one of GOOGLE_API_KEY / GOOGLE_GENERATIVE_AI_API_KEY
// / GEMINI_API_KEY, resolved by plugins/providers/compat.
//
// Vertex AI (service-account OAuth, different base URL) is *not*
// covered here — catalog.FamilyGoogleVertex returns
// ErrFamilyUnsupported from compat until a Vertex adapter ships.
//
// Non-streaming for M1.i; SSE streaming lands when streaming is
// added uniformly across providers.
package google

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
)

const (
	// DefaultBaseURL is the public Generative Language API root.
	DefaultBaseURL = "https://generativelanguage.googleapis.com"
	// APIVersion is the path segment between the base URL and /models.
	APIVersion = "v1beta"
	// DefaultTimeout caps a single HTTP request.
	DefaultTimeout = 5 * time.Minute
)

// Provider is the in-process Gemini Provider.
type Provider struct {
	APIKey string
	// Endpoint, if set, is the full URL to POST to (including
	// `:generateContent`). Useful for tests with httptest.NewServer.
	// When set, BaseURL/APIVersion/Model are ignored for URL routing.
	Endpoint string
	// BaseURL lets the catalog/compat layer pass a bare provider URL
	// (e.g. "https://generativelanguage.googleapis.com") and have
	// this Provider derive the right v1beta/models/.../:generateContent
	// path. Ignored when Endpoint is set.
	BaseURL string
	Model   string
	HTTP    *http.Client
	// ThinkingBudget enables Gemini "thinking" (2.5-series models) when
	// non-zero (M319). A positive value caps the thinking tokens; -1 asks
	// Gemini for a dynamic budget. Either way Agezt also sets
	// includeThoughts so the thought summaries come back and land on the
	// response's ReasoningContent (the M317 pipeline). 0 sends no
	// thinkingConfig at all — the request wire is byte-identical to a
	// non-thinking run, and the model's own default applies.
	ThinkingBudget int
}

// New constructs a Provider with sensible defaults.
func New(apiKey string) *Provider {
	return &Provider{
		APIKey:  apiKey,
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: DefaultTimeout},
	}
}

// resolveEndpoint returns the URL to POST to, derived in this order:
//
//  1. explicit p.Endpoint
//  2. p.BaseURL + "/v1beta/models/<model>:generateContent"
//     - if BaseURL already ends with "/v1beta" (or "/v1"), don't add APIVersion
//  3. DefaultBaseURL + "/v1beta/models/<model>:generateContent"
//
// The model id is interpolated into the path — that's how Gemini
// addresses models, not via a request body field.
func (p *Provider) resolveEndpoint(model string) string {
	if p.Endpoint != "" {
		return p.Endpoint
	}
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	prefix := base
	// Only prepend /v1beta when the base URL doesn't already carry
	// an API-version segment.
	if !strings.HasSuffix(base, "/"+APIVersion) && !strings.Contains(base, "/"+APIVersion+"/") &&
		!strings.HasSuffix(base, "/v1") && !strings.Contains(base, "/v1/") {
		prefix = base + "/" + APIVersion
	}
	return prefix + "/models/" + model + ":generateContent"
}

// Name implements agent.Provider.
func (p *Provider) Name() string { return "google" }

// ErrNoAPIKey is returned by Complete when APIKey is empty.
var ErrNoAPIKey = errors.New("google: API key not set")

// ErrNoModel is returned when a completion request carries no model and the
// provider has none set. The daemon ships with no default model (owner rule), so
// the model must come from the request (AGEZT_MODEL / routing / a fallback chain).
var ErrNoModel = errors.New("google: no model specified (set CompletionRequest.Model, AGEZT_MODEL, or a routing/fallback chain)")

// APIError is returned for non-2xx responses.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("google: status %d: %s", e.Status, e.Body)
}

// Complete implements agent.Provider.
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

	body, err := encodeRequest(req.System, req.Messages, req.Tools, req.MaxTokens, req.JSONMode, p.ThinkingBudget)
	if err != nil {
		return nil, fmt.Errorf("google: encode request: %w", err)
	}

	endpoint := p.resolveEndpoint(model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("google: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", p.APIKey)

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("google: http: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("google: read body: %w", err)
	}
	if httpResp.StatusCode/100 != 2 {
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(respBytes)}
	}
	return decodeResponse(respBytes, model)
}

// ----- dialect translation (canonical ↔ Gemini generateContent) -----

type geminiRequest struct {
	Contents          []geminiContent  `json:"contents"`
	Tools             []geminiTool     `json:"tools,omitempty"`
	SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`
}

type geminiGenConfig struct {
	MaxOutputTokens  int                   `json:"maxOutputTokens,omitempty"`
	ResponseMimeType string                `json:"responseMimeType,omitempty"` // "application/json" → JSON mode (M312)
	ThinkingConfig   *geminiThinkingConfig `json:"thinkingConfig,omitempty"`   // M319
}

// geminiThinkingConfig is Gemini's per-request thinking control (M319,
// 2.5-series models). IncludeThoughts asks the API to return the thought
// summaries as parts flagged `thought:true`; ThinkingBudget caps the
// thinking tokens (-1 = dynamic). Only emitted when the operator opts in.
type geminiThinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts"`
	ThinkingBudget  int  `json:"thinkingBudget"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"` // "user" | "model"; omitted for systemInstruction
	Parts []geminiPart `json:"parts"`
}

// geminiPart is a tagged-union content block. Only one field is
// populated per part — Text, InlineData, FunctionCall, or FunctionResponse.
type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"` // M319: a thought-summary part (reasoning, not answer)
	InlineData       *geminiInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

// geminiInlineData is an inline base64 blob part — how Gemini's generateContent
// API carries an image attachment (M243).
type geminiInlineData struct {
	MimeType string `json:"mimeType"` // image/png, image/jpeg, image/gif, image/webp
	Data     string `json:"data"`     // base64-encoded image bytes
}

type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
	Index        int           `json:"index"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
	// CachedContentTokenCount is the subset of PromptTokenCount served from
	// Gemini context caching (M294-cache). Billed at the cache-read rate.
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
	// ThoughtsTokenCount is the thinking-token count (M319). Gemini reports
	// it *separately* from CandidatesTokenCount (total = prompt + candidates
	// + thoughts) but bills it at the output rate, so it's folded into
	// Usage.OutputTokens on decode.
	ThoughtsTokenCount int `json:"thoughtsTokenCount"`
}

func encodeRequest(system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int, jsonMode bool, thinkingBudget int) ([]byte, error) {
	wire := geminiRequest{}
	if s := strings.TrimSpace(system); s != "" {
		wire.SystemInstruction = &geminiContent{
			// No role on systemInstruction per Gemini spec.
			Parts: []geminiPart{{Text: s}},
		}
	}
	for _, m := range msgs {
		c, err := canonicalToGemini(m)
		if err != nil {
			return nil, err
		}
		if c == nil {
			continue
		}
		wire.Contents = append(wire.Contents, *c)
	}
	if len(tools) > 0 {
		decls := make([]geminiFunctionDecl, 0, len(tools))
		for _, t := range tools {
			params := t.InputSchema
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			decls = append(decls, geminiFunctionDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			})
		}
		wire.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}
	if maxTok > 0 || jsonMode || thinkingBudget != 0 {
		gc := &geminiGenConfig{MaxOutputTokens: maxTok}
		if jsonMode {
			gc.ResponseMimeType = "application/json"
		}
		if thinkingBudget != 0 {
			// Opt-in (M319). includeThoughts so the summaries come back and
			// land on ReasoningContent; the budget caps the thinking tokens
			// (-1 = let Gemini decide dynamically).
			gc.ThinkingConfig = &geminiThinkingConfig{
				IncludeThoughts: true,
				ThinkingBudget:  thinkingBudget,
			}
		}
		wire.GenerationConfig = gc
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

func canonicalToGemini(m agent.Message) (*geminiContent, error) {
	switch m.Role {
	case agent.RoleSystem:
		// System messages fold into systemInstruction at the request
		// level; per-message system roles are ignored here.
		return nil, nil
	case agent.RoleUser:
		// Vision (M243): a user message may carry image attachments as RFC 2397
		// data: URLs. Emit each as an inlineData part before the text part. A
		// non-data-URL entry (e.g. a legacy bare filename) has no deliverable
		// payload and is skipped.
		parts := make([]geminiPart, 0, len(m.Images)+1)
		for _, img := range m.Images {
			if mt, data, ok := parseImageDataURL(img); ok {
				parts = append(parts, geminiPart{InlineData: &geminiInlineData{MimeType: mt, Data: data}})
			}
		}
		parts = append(parts, geminiPart{Text: m.Content})
		return &geminiContent{Role: "user", Parts: parts}, nil
	case agent.RoleAssistant:
		var parts []geminiPart
		if strings.TrimSpace(m.Content) != "" {
			parts = append(parts, geminiPart{Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			args := tc.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			parts = append(parts, geminiPart{
				FunctionCall: &geminiFunctionCall{
					Name: tc.Name,
					Args: args,
				},
			})
		}
		if len(parts) == 0 {
			// Gemini rejects empty content; insert a placeholder.
			parts = []geminiPart{{Text: ""}}
		}
		return &geminiContent{Role: "model", Parts: parts}, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("google: role=tool requires tool_call_id (used as functionResponse name lookup)")
		}
		// Gemini doesn't have a separate tool role. Tool results are
		// sent back as a user-role content with functionResponse parts.
		// The canonical Message carries the function name via the
		// preceding assistant turn — but Gemini's functionResponse
		// also requires the name. We don't have it in canonical
		// shape, so we rely on the loop to supply it via Content
		// being a JSON object or by setting Name in the canonical
		// Message (future). For now: pack the content under a
		// "result" key and use the tool_call_id as the function name
		// surrogate. Real callers that need name fidelity should
		// route through the tool registry. See ADR-???; tracked in
		// SPEC-15 "tool-result name binding".
		// Build the {"result": ...} object with encoding/json, NOT strconv.Quote:
		// strconv.Quote is a GO string-literal quoter, so a control byte (NUL, ESC
		// \x1b common in terminal/ANSI tool output, etc.) becomes a Go-only \xNN
		// escape that is INVALID JSON — which makes the whole request fail to encode
		// and wedges the agent loop on Gemini for any tool output containing one. (M481)
		quoted, err := json.Marshal(m.Content)
		if err != nil {
			return nil, fmt.Errorf("google: encode tool result: %w", err)
		}
		resp := json.RawMessage(`{"result":` + string(quoted) + `}`)
		return &geminiContent{
			Role: "user",
			Parts: []geminiPart{{
				FunctionResponse: &geminiFunctionResponse{
					Name:     m.ToolCallID, // surrogate; see comment above
					Response: resp,
				},
			}},
		}, nil
	default:
		return nil, fmt.Errorf("google: unknown role %q", m.Role)
	}
}

func decodeResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var gr geminiResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, fmt.Errorf("google: parse response: %w", err)
	}
	if len(gr.Candidates) == 0 {
		return nil, fmt.Errorf("google: response has no candidates")
	}
	cand := gr.Candidates[0]

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
			// Gemini doesn't return per-call IDs; synthesize stable ones
			// (SPEC-15: canonical ToolCall.ID is always non-empty).
			toolCalls = append(toolCalls, agent.ToolCall{
				ID:    "call-" + strconv.Itoa(i),
				Name:  part.FunctionCall.Name,
				Input: args,
			})
		case part.Thought && part.Text != "":
			// A thought-summary part (M319) — reasoning, kept out of the answer.
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
	if gr.UsageMetadata != nil {
		usage.InputTokens = gr.UsageMetadata.PromptTokenCount
		usage.CachedInputTokens = gr.UsageMetadata.CachedContentTokenCount
		// Thinking tokens are billed as output but reported separately (M319);
		// fold them in so OutputTokens reflects the true billable output.
		usage.OutputTokens = gr.UsageMetadata.CandidatesTokenCount + gr.UsageMetadata.ThoughtsTokenCount
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
