// SPDX-License-Identifier: MIT

// Package cohere is the in-process Cohere v2 chat Provider.
//
// Wire shape: POST /v2/chat with Bearer auth. The request shape is
// close to OpenAI (messages array, tools, stream:false) but the
// response carries `message.content` as a typed-block array
// ({type:"text", text}) rather than a single string, and usage is
// nested under `usage.tokens.{input,output}`. Tool calls follow an
// OpenAI-like shape ({id, type:"function", function:{name, arguments}}).
//
// finish_reason values:
//
//	COMPLETE / STOP_SEQUENCE → StopEndTurn
//	MAX_TOKENS               → StopMaxTokens
//	TOOL_CALL                → StopToolUse
//
// Auth: API key via COHERE_API_KEY (catalog-resolved by compat).
//
// Non-streaming for M1.k; SSE arrives when streaming is added
// uniformly across providers.
package cohere

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
	// DefaultBaseURL is the public Cohere API root.
	DefaultBaseURL = "https://api.cohere.com"
	// DefaultTimeout caps a single HTTP request.
	DefaultTimeout = 5 * time.Minute
)

// Provider is the in-process Cohere v2 Provider.
type Provider struct {
	APIKey string
	// Endpoint is the full chat URL. If empty, BaseURL is used
	// (appending /v2/chat); if both are empty, DefaultBaseURL.
	Endpoint string
	// BaseURL lets the catalog/compat layer pass a bare provider URL
	// (e.g. "https://api.cohere.com") and have this Provider derive
	// the right v2/chat path. Ignored when Endpoint is set.
	BaseURL string
	Model   string
	HTTP    *http.Client
}

// New constructs a Provider with sensible defaults.
func New(apiKey string) *Provider {
	return &Provider{
		APIKey:  apiKey,
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: DefaultTimeout},
	}
}

// resolveEndpoint returns the URL to POST to.
//
//  1. explicit p.Endpoint
//  2. p.BaseURL + "/v2/chat"  — skipped if BaseURL already ends with /v2
//  3. DefaultBaseURL + "/v2/chat"
func (p *Provider) resolveEndpoint() string {
	if p.Endpoint != "" {
		return p.Endpoint
	}
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	if strings.HasSuffix(base, "/v2") || strings.Contains(base, "/v2/") {
		return base + "/chat"
	}
	return base + "/v2/chat"
}

// Name implements agent.Provider.
func (p *Provider) Name() string { return "cohere" }

// ErrNoAPIKey is returned by Complete when APIKey is empty.
var ErrNoAPIKey = errors.New("cohere: API key not set")

// ErrNoModel is returned when a completion request carries no model and the
// provider has none set. The daemon ships with no default model (owner rule), so
// the model must come from the request (AGEZT_MODEL / routing / a fallback chain).
var ErrNoModel = errors.New("cohere: no model specified (set CompletionRequest.Model, AGEZT_MODEL, or a routing/fallback chain)")

// APIError is returned for non-2xx responses.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("cohere: status %d: %s", e.Status, e.Body)
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

	body, err := encodeRequest(model, req.System, req.Messages, req.Tools, req.MaxTokens)
	if err != nil {
		return nil, fmt.Errorf("cohere: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.resolveEndpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("cohere: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cohere: http: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("cohere: read body: %w", err)
	}
	if httpResp.StatusCode/100 != 2 {
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(respBytes)}
	}
	return decodeResponse(respBytes, model)
}

// ----- dialect translation (canonical ↔ Cohere v2 chat) -----

type cohereRequest struct {
	Model     string          `json:"model"`
	Messages  []cohereMessage `json:"messages"`
	Tools     []cohereTool    `json:"tools,omitempty"`
	Stream    bool            `json:"stream"`
	MaxTokens int             `json:"max_tokens,omitempty"`
}

// cohereMessage uses a permissive `content` shape: string when
// outgoing (we always send strings), array-of-blocks when incoming.
// We model both via json.RawMessage and inspect at decode time.
type cohereMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []cohereToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolPlan   string           `json:"tool_plan,omitempty"`
}

type cohereToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function cohereToolCallFn `json:"function"`
}

type cohereToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type cohereTool struct {
	Type     string          `json:"type"`
	Function cohereToolFnDef `json:"function"`
}

type cohereToolFnDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

// cohereResponse models the v2 response. message.content is a typed
// block array on the wire; we tolerate either string or array via
// json.RawMessage + custom inspection.
type cohereResponse struct {
	ID           string            `json:"id"`
	FinishReason string            `json:"finish_reason"`
	Message      cohereRespMessage `json:"message"`
	Usage        *cohereUsage      `json:"usage,omitempty"`
}

type cohereRespMessage struct {
	Role      string           `json:"role"`
	Content   json.RawMessage  `json:"content"`
	ToolCalls []cohereToolCall `json:"tool_calls,omitempty"`
	ToolPlan  string           `json:"tool_plan,omitempty"`
}

type cohereContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type cohereUsage struct {
	Tokens *cohereTokens `json:"tokens,omitempty"`
}

type cohereTokens struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func encodeRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int) ([]byte, error) {
	wire := cohereRequest{
		Model:     model,
		Stream:    false,
		MaxTokens: maxTok,
	}
	if s := strings.TrimSpace(system); s != "" {
		wire.Messages = append(wire.Messages, cohereMessage{Role: "system", Content: s})
	}
	for _, m := range msgs {
		cm, err := canonicalToCohere(m)
		if err != nil {
			return nil, err
		}
		if cm == nil {
			continue
		}
		wire.Messages = append(wire.Messages, *cm)
	}
	for _, t := range tools {
		params := t.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		wire.Tools = append(wire.Tools, cohereTool{
			Type: "function",
			Function: cohereToolFnDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return json.Marshal(wire)
}

func canonicalToCohere(m agent.Message) (*cohereMessage, error) {
	switch m.Role {
	case agent.RoleSystem:
		if strings.TrimSpace(m.Content) == "" {
			return nil, nil
		}
		return &cohereMessage{Role: "system", Content: m.Content}, nil
	case agent.RoleUser:
		return &cohereMessage{Role: "user", Content: m.Content}, nil
	case agent.RoleAssistant:
		cm := &cohereMessage{Role: "assistant", Content: m.Content}
		for _, tc := range m.ToolCalls {
			args := tc.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			cm.ToolCalls = append(cm.ToolCalls, cohereToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: cohereToolCallFn{
					Name:      tc.Name,
					Arguments: string(args),
				},
			})
		}
		return cm, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("cohere: role=tool requires tool_call_id")
		}
		return &cohereMessage{
			Role:       "tool",
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}, nil
	default:
		return nil, fmt.Errorf("cohere: unknown role %q", m.Role)
	}
}

func decodeResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var cr cohereResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return nil, fmt.Errorf("cohere: parse response: %w", err)
	}

	// content can be a plain string or an array of typed blocks
	// ({type:"text", text}). Try string first; on failure, decode as array.
	var text string
	if len(cr.Message.Content) > 0 {
		var asString string
		if err := json.Unmarshal(cr.Message.Content, &asString); err == nil {
			text = asString
		} else {
			var blocks []cohereContentBlock
			if err := json.Unmarshal(cr.Message.Content, &blocks); err != nil {
				return nil, fmt.Errorf("cohere: message.content not string-or-blocks: %w", err)
			}
			var parts []string
			for _, b := range blocks {
				if b.Type == "text" {
					parts = append(parts, b.Text)
				}
			}
			text = strings.Join(parts, "")
		}
	}

	var toolCalls []agent.ToolCall
	for i, tc := range cr.Message.ToolCalls {
		id := tc.ID
		if id == "" {
			id = "call-" + strconv.Itoa(i)
		}
		args := strings.TrimSpace(tc.Function.Arguments)
		if args == "" {
			args = "{}"
		}
		toolCalls = append(toolCalls, agent.ToolCall{
			ID:    id,
			Name:  tc.Function.Name,
			Input: json.RawMessage(args),
		})
	}

	stop := agent.StopEndTurn
	switch strings.ToUpper(cr.FinishReason) {
	case "COMPLETE", "STOP_SEQUENCE", "":
		stop = agent.StopEndTurn
	case "MAX_TOKENS":
		stop = agent.StopMaxTokens
	case "TOOL_CALL":
		stop = agent.StopToolUse
	}
	if len(toolCalls) > 0 && stop == agent.StopEndTurn {
		stop = agent.StopToolUse
	}

	usage := agent.Usage{Model: model}
	if cr.Usage != nil && cr.Usage.Tokens != nil {
		usage.InputTokens = cr.Usage.Tokens.InputTokens
		usage.OutputTokens = cr.Usage.Tokens.OutputTokens
	}

	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			Content:   text,
			ToolCalls: toolCalls,
		},
		StopReason: stop,
		Usage:      usage,
	}, nil
}
