// SPDX-License-Identifier: MIT

// Package ollama is the in-process Ollama Provider implementation.
//
// Ollama is the local-model floor (DECISIONS C2): an always-eligible
// fallback that runs offline at zero marginal cost. The provider talks
// to a local ollama server (default http://localhost:11434) over the
// /api/chat endpoint, non-streaming for M1.
//
// Tool-calling translation (SPEC-15): Ollama uses an OpenAI-flavoured
// tool-calls shape but does not return per-call IDs; this provider
// synthesises stable IDs so canonical agent.ToolCall.ID is always
// non-empty.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

const (
	// DefaultEndpoint is a local Ollama server.
	DefaultEndpoint = "http://localhost:11434/api/chat"
	// DefaultModel is what the loop uses when CompletionRequest.Model is empty.
	DefaultModel = "llama3.2"
	// DefaultTimeout caps a single request. Local models can take a while.
	DefaultTimeout = 10 * time.Minute
)

// Provider is the Ollama Provider.
type Provider struct {
	// Endpoint is the full chat URL. If empty, BaseURL is used
	// (appending /api/chat); if both are empty, DefaultEndpoint.
	Endpoint string
	// BaseURL lets the catalog/compat layer pass a bare host
	// (e.g. "http://localhost:11434") and have this Provider derive
	// the right chat path. Ignored when Endpoint is set.
	BaseURL string
	Model   string
	HTTP    *http.Client
}

// New constructs a Provider with sensible defaults.
func New() *Provider {
	return &Provider{
		Endpoint: DefaultEndpoint,
		Model:    DefaultModel,
		HTTP:     &http.Client{Timeout: DefaultTimeout},
	}
}

// resolveEndpoint returns the URL to POST to. Order:
//
//  1. explicit p.Endpoint
//  2. p.BaseURL + "/api/chat" (the catalog/compat path)
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
		return base + "/api/chat"
	}
	return DefaultEndpoint
}

// Name implements agent.Provider.
func (p *Provider) Name() string { return "ollama" }

// APIError is returned for non-2xx upstream responses.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ollama: status %d: %s", e.Status, e.Body)
}

// ErrNoEndpoint is returned when Endpoint is empty.
var ErrNoEndpoint = errors.New("ollama: endpoint not set")

// Complete implements agent.Provider.
func (p *Provider) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	endpoint := p.resolveEndpoint()
	if endpoint == "" {
		return nil, ErrNoEndpoint
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		model = DefaultModel
	}

	body, err := encodeRequest(model, req.System, req.Messages, req.Tools)
	if err != nil {
		return nil, fmt.Errorf("ollama: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: http: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("ollama: read body: %w", err)
	}
	if httpResp.StatusCode/100 != 2 {
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(respBytes)}
	}
	return decodeResponse(respBytes)
}

// ----- dialect translation -----

type ollamaRequest struct {
	Model    string          `json:"model"`
	Stream   bool            `json:"stream"`
	Messages []ollamaMessage `json:"messages"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type ollamaToolCall struct {
	ID       string           `json:"id,omitempty"`
	Function ollamaToolCallFn `json:"function"`
}

type ollamaToolCallFn struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ollamaResponse struct {
	Model      string        `json:"model"`
	Message    ollamaMessage `json:"message"`
	Done       bool          `json:"done"`
	DoneReason string        `json:"done_reason"`
	// Usage-ish fields. Names vary by version; we read what's there.
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

func encodeRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef) ([]byte, error) {
	out := ollamaRequest{
		Model:  model,
		Stream: false,
	}
	if system != "" {
		out.Messages = append(out.Messages, ollamaMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		om, err := canonicalToOllama(m)
		if err != nil {
			return nil, err
		}
		out.Messages = append(out.Messages, om)
	}
	for _, t := range tools {
		out.Tools = append(out.Tools, ollamaTool{
			Type: "function",
			Function: ollamaToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	return json.Marshal(out)
}

func canonicalToOllama(m agent.Message) (ollamaMessage, error) {
	switch m.Role {
	case agent.RoleSystem:
		return ollamaMessage{Role: "system", Content: m.Content}, nil
	case agent.RoleUser:
		return ollamaMessage{Role: "user", Content: m.Content}, nil
	case agent.RoleAssistant:
		om := ollamaMessage{Role: "assistant", Content: m.Content}
		for _, tc := range m.ToolCalls {
			args := tc.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			om.ToolCalls = append(om.ToolCalls, ollamaToolCall{
				ID: tc.ID,
				Function: ollamaToolCallFn{
					Name:      tc.Name,
					Arguments: args,
				},
			})
		}
		return om, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return ollamaMessage{}, errors.New("ollama: role=tool requires tool_call_id")
		}
		return ollamaMessage{
			Role:       "tool",
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}, nil
	default:
		return ollamaMessage{}, fmt.Errorf("ollama: unknown role %q", m.Role)
	}
}

func decodeResponse(body []byte) (*agent.CompletionResponse, error) {
	var or ollamaResponse
	if err := json.Unmarshal(body, &or); err != nil {
		return nil, fmt.Errorf("ollama: parse response: %w", err)
	}

	var toolCalls []agent.ToolCall
	for i, tc := range or.Message.ToolCalls {
		id := tc.ID
		if id == "" {
			id = "call-" + strconv.Itoa(i)
		}
		args := tc.Function.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		toolCalls = append(toolCalls, agent.ToolCall{
			ID:    id,
			Name:  tc.Function.Name,
			Input: args,
		})
	}

	// Ollama's stop reason is less standardised than Anthropic's. Use
	// tool_calls presence as a strong signal first; fall back to
	// done_reason mapping.
	var stop agent.StopReason
	switch {
	case len(toolCalls) > 0:
		stop = agent.StopToolUse
	case or.DoneReason == "length":
		stop = agent.StopMaxTokens
	default:
		stop = agent.StopEndTurn
	}

	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			Content:   or.Message.Content,
			ToolCalls: toolCalls,
		},
		StopReason: stop,
		Usage: agent.Usage{
			InputTokens:  or.PromptEvalCount,
			OutputTokens: or.EvalCount,
			Model:        or.Model,
		},
	}, nil
}
