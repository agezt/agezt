// SPDX-License-Identifier: MIT

// OpenAI provider: Provider type + Complete + encode/decode roundtrip.
// Code extracted from openai.go during the Day-96 god-file split.
// Public API unchanged.
package openai

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
	// DefaultEndpoint is the real OpenAI Chat Completions URL.
	DefaultEndpoint = "https://api.openai.com/v1/chat/completions"
	// DefaultTimeout caps a single HTTP request.
	DefaultTimeout = 5 * time.Minute
)

// Provider is the in-process OpenAI Chat Completions Provider.
type Provider struct {
	APIKey string
	// Endpoint is the full Chat Completions URL. If empty, BaseURL is
	// used (appending /chat/completions or /v1/chat/completions as
	// appropriate); if both are empty, DefaultEndpoint.
	Endpoint string
	// BaseURL lets the catalog/compat layer pass a bare provider URL
	// (e.g. "https://api.groq.com/openai/v1") and have this Provider
	// derive the right Chat Completions path. Ignored when Endpoint
	// is set.
	BaseURL string
	Model   string
	HTTP    *http.Client

	// AuthHeader is the HTTP header carrying the credential. Defaults
	// to "Authorization". Azure OpenAI uses "api-key" and no scheme
	// prefix (see AuthScheme).
	AuthHeader string
	// AuthScheme is the prefix prepended to APIKey in the auth
	// header. Defaults to "Bearer " (note trailing space). Set to ""
	// for raw-value headers like Azure's api-key.
	AuthScheme string
}

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
//  2. p.BaseURL + suffix
//     - if BaseURL already ends with "/v1" (or contains "/v1/"), append "/chat/completions"
//     - otherwise append "/v1/chat/completions"
//  3. DefaultEndpoint
//
// models.dev publishes openai-compatible providers with `api` set to
// the *full v1 root* (e.g. "https://api.groq.com/openai/v1"), so the
// /v1-already-present check is the common path.
func (p *Provider) resolveEndpoint() string {
	if p.Endpoint != "" {
		return p.Endpoint
	}
	if p.BaseURL != "" {
		base := strings.TrimRight(p.BaseURL, "/")
		if strings.HasSuffix(base, "/v1") || strings.Contains(base, "/v1/") {
			return base + "/chat/completions"
		}
		return base + "/v1/chat/completions"
	}
	return DefaultEndpoint
}

// Name implements agent.Provider.
func (p *Provider) Name() string { return "openai" }

// ErrNoAPIKey is returned by Complete when APIKey is empty.
var ErrNoAPIKey = errors.New("openai: API key not set")

// ErrNoModel is returned when a completion request carries no model and the
// provider has none set. The daemon ships with no default model (owner rule), so
// the model must come from the request (AGEZT_MODEL / routing / a fallback chain).
var ErrNoModel = errors.New("openai: no model specified (set CompletionRequest.Model, AGEZT_MODEL, or a routing/fallback chain)")

// APIError is returned for non-2xx responses; it carries the upstream
// status and body so callers (and the journal) can record the failure.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("openai: status %d: %s", e.Status, e.Body)
}

// Complete implements agent.Provider. It honors ctx for HTTP
// cancellation (which is how the agent loop reacts to `agt halt`).
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

	body, err := encodeRequest(model, req.System, req.Messages, req.Tools, req.MaxTokens, req.JSONMode, req.Params, req.ProviderOptions["openai"])
	if err != nil {
		return nil, fmt.Errorf("openai: encode request: %w", err)
	}

	endpoint := p.resolveEndpoint()
	authHeader := p.AuthHeader
	if authHeader == "" {
		authHeader = "Authorization"
	}
	authScheme := p.AuthScheme
	if authScheme == "" && p.AuthHeader == "" {
		authScheme = "Bearer "
	}

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}

	// Retry logic with exponential backoff for transient errors (429, 5xx)
	var respBytes []byte
	httpErr := retry.Do(ctx, retry.DefaultConfig, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(authHeader, authScheme+p.APIKey)

		httpResp, err := client.Do(req)
		if err != nil {
			return &retry.TransientError{Err: err}
		}
		defer httpResp.Body.Close()

		respBytes, err = httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
		if err != nil {
			return fmt.Errorf("openai: read body: %w", err)
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
	toolname.RestoreCalls(resp, toolname.Reverse(req.Tools))
	return resp, nil
}

// ----- dialect translation (canonical ↔ OpenAI Chat Completions) -----

type oaRequest struct {
	Model          string            `json:"model"`
	Messages       []oaMessage       `json:"messages"`
	Tools          []oaTool          `json:"tools,omitempty"`
	MaxTokens      int               `json:"max_tokens,omitempty"`
	Stream         bool              `json:"stream"`
	ResponseFormat *oaResponseFormat `json:"response_format,omitempty"`
	// Per-request sampling knobs (M997), promoted to the top level of the wire
	// object. An unset agent.Params leaves every field nil/empty (omitempty), so
	// the request stays byte-for-byte unchanged.
	oaParams
}

// oaParams is the embeddable set of universal sampling knobs shared by the
// non-streaming and streaming request structs. OpenAI has no top_k.
type oaParams struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	Stop             []string `json:"stop,omitempty"`
	Seed             *int64   `json:"seed,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	// ReasoningEffort maps to OpenAI's reasoning_effort (o-series / gpt-5).
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

// applyParams copies the universal sampling knobs from p. Only set fields are
// carried over, so an empty Params leaves the embedded oaParams zero-valued.
func (o *oaParams) applyParams(p agent.Params) {
	if p.IsZero() {
		return
	}
	o.Temperature = p.Temperature
	o.TopP = p.TopP
	o.Stop = p.Stop
	o.Seed = p.Seed
	o.FrequencyPenalty = p.FrequencyPenalty
	o.PresencePenalty = p.PresencePenalty
	o.ReasoningEffort = provopts.NormalizeEffort(p.ReasoningEffort)
}

// oaResponseFormat carries OpenAI's structured-output request (M311). type
// "json_object" is the broadly-supported JSON mode; we don't send a schema
// (json_schema) so openai-compatible vendors that only do json_object still work.
type oaResponseFormat struct {
	Type string `json:"type"` // "json_object"
}

// jsonObjectFormat is the shared response_format for JSON mode.
func jsonObjectFormat(jsonMode bool) *oaResponseFormat {
	if !jsonMode {
		return nil
	}
	return &oaResponseFormat{Type: "json_object"}
}

type oaMessage struct {
	Role string `json:"role"`
	// Content is OpenAI's polymorphic message content: a plain string for
	// text-only messages (the common case, and the only form a response uses),
	// or a []oaContentPart array when a user message carries images (vision,
	// M242). Typed as any so one field marshals to either form; helpers
	// oaTextOrNil / oaContentText keep the string path's omitempty semantics.
	Content    any          `json:"content,omitempty"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
	Name       string       `json:"name,omitempty"`
	// ReasoningContent / Reasoning carry a reasoning model's chain of thought
	// (M317), present on responses from DeepSeek-R1 (`reasoning_content`) and some
	// other openai-compatible reasoning gateways (`reasoning`). Response-only;
	// never set on a request (omitempty keeps the request wire unchanged).
	ReasoningContent string `json:"reasoning_content,omitempty"`
	Reasoning        string `json:"reasoning,omitempty"`
}

// reasoningText returns whichever reasoning field the provider populated.
func (m oaMessage) reasoningText() string {
	if m.ReasoningContent != "" {
		return m.ReasoningContent
	}
	return m.Reasoning
}

// oaContentPart is one element of OpenAI's multimodal content array. A part is
// either {type:"text",text:...} or {type:"image_url",image_url:{url:...}} where
// url is a data: URL (OpenAI accepts those natively) or an http(s) URL (M242).
type oaContentPart struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	ImageURL *oaImageURL `json:"image_url,omitempty"`
}

type oaImageURL struct {
	URL string `json:"url"`
}

// oaTextOrNil returns nil for empty text so the "content" field is omitted
// (preserving the pre-M242 omitempty wire shape for tool-call-only assistant
// messages), and the string itself otherwise.
func oaTextOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// oaContentText extracts the string form of a decoded message content. A
// response always uses the string form; anything else yields "".
func oaContentText(v any) string {
	s, _ := v.(string)
	return s
}

// isImageURL reports whether an image attachment string is a URL OpenAI can
// fetch or decode — a data: URL (what the CLI sends, M241) or an http(s) URL.
// A bare filename or other unresolvable string is rejected so it is dropped
// rather than sent as an invalid image_url.
func isImageURL(s string) bool {
	return strings.HasPrefix(s, "data:") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "http://")
}

type oaToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function oaToolCallFn `json:"function"`
}

type oaToolCallFn struct {
	Name string `json:"name"`
	// OpenAI passes arguments as a JSON-encoded string, not a nested object.
	Arguments string `json:"arguments"`
}

type oaTool struct {
	Type     string      `json:"type"`
	Function oaToolFnDef `json:"function"`
}

type oaToolFnDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type oaResponse struct {
	ID      string     `json:"id"`
	Object  string     `json:"object"`
	Model   string     `json:"model"`
	Choices []oaChoice `json:"choices"`
	Usage   struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails struct {
			// CachedTokens is the subset of PromptTokens served from the
			// provider's prompt cache (OpenAI + compatible gateways). Billed
			// at the cache-read rate; threaded to agent.Usage.CachedInputTokens.
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		// PromptCacheHitTokens is DeepSeek's spelling of the same cache-read
		// count (M887): their context caching reports prompt_cache_hit_tokens /
		// prompt_cache_miss_tokens at the top level of usage instead of the
		// OpenAI prompt_tokens_details shape. Read as a fallback so a DeepSeek
		// run's cache hits price at the cache-read rate instead of being
		// silently billed as fresh input.
		PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
	} `json:"usage"`
}

type oaChoice struct {
	Index        int       `json:"index"`
	Message      oaMessage `json:"message"`
	FinishReason string    `json:"finish_reason"`
}

// Tool-name conformance (OpenAI's ^[a-zA-Z0-9_-]{1,64}$) lives in the shared
// plugins/providers/internal/toolname package: toolname.Maps builds the injective
// original↔wire mapping, toolname.Wire applies it on encode, and
// toolname.RestoreCalls reverses it on the response so a tool_call still routes to
// the real tool.

