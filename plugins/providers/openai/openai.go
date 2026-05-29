// SPDX-License-Identifier: MIT

// Package openai is the in-process OpenAI Chat Completions Provider.
//
// One adapter covers two compatibility families: FamilyOpenAI (the
// real api.openai.com) and FamilyOpenAICompatible (Groq, DeepSeek,
// Together, OpenRouter, xAI, Fireworks, Cerebras, SambaNova, …). All
// of those expose the same /v1/chat/completions wire shape with
// Bearer-token auth; the only difference is the base URL and the env
// var holding the key, both of which come from the catalog.
//
// Non-streaming for M1.h; streaming lands when SSE-aware Providers
// arrive across the board.
//
// Auth: Bearer <key> via the constructor or the BaseURL/Endpoint
// pair set by plugins/providers/compat.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

const (
	// DefaultEndpoint is the real OpenAI Chat Completions URL.
	DefaultEndpoint = "https://api.openai.com/v1/chat/completions"
	// DefaultModel is what the loop uses when CompletionRequest.Model is empty.
	DefaultModel = "gpt-4o-mini"
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
		Model:    DefaultModel,
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
		model = DefaultModel
	}

	body, err := encodeRequest(model, req.System, req.Messages, req.Tools, req.MaxTokens)
	if err != nil {
		return nil, fmt.Errorf("openai: encode request: %w", err)
	}

	endpoint := p.resolveEndpoint()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	authHeader := p.AuthHeader
	if authHeader == "" {
		authHeader = "Authorization"
	}
	authScheme := p.AuthScheme
	if authScheme == "" && p.AuthHeader == "" {
		// Only default the scheme when the header is also defaulted,
		// so an explicit empty-scheme caller (Azure) isn't silently
		// promoted back to Bearer.
		authScheme = "Bearer "
	}
	httpReq.Header.Set(authHeader, authScheme+p.APIKey)

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: http: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read body: %w", err)
	}
	if httpResp.StatusCode/100 != 2 {
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(respBytes)}
	}
	return decodeResponse(respBytes)
}

// ----- dialect translation (canonical ↔ OpenAI Chat Completions) -----

type oaRequest struct {
	Model     string      `json:"model"`
	Messages  []oaMessage `json:"messages"`
	Tools     []oaTool    `json:"tools,omitempty"`
	MaxTokens int         `json:"max_tokens,omitempty"`
	Stream    bool        `json:"stream"`
}

type oaMessage struct {
	Role       string       `json:"role"`
	Content    string       `json:"content,omitempty"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
	Name       string       `json:"name,omitempty"`
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
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type oaChoice struct {
	Index        int       `json:"index"`
	Message      oaMessage `json:"message"`
	FinishReason string    `json:"finish_reason"`
}

func encodeRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int) ([]byte, error) {
	wire := oaRequest{
		Model:     model,
		Stream:    false,
		MaxTokens: maxTok, // 0 → omitted via omitempty
	}
	if strings.TrimSpace(system) != "" {
		wire.Messages = append(wire.Messages, oaMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		om, err := canonicalToOA(m)
		if err != nil {
			return nil, err
		}
		if om == nil {
			continue
		}
		wire.Messages = append(wire.Messages, *om)
	}
	for _, t := range tools {
		params := t.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		wire.Tools = append(wire.Tools, oaTool{
			Type: "function",
			Function: oaToolFnDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return json.Marshal(wire)
}

// canonicalToOA converts one canonical Message into one OpenAI Chat
// Completions message. Returns (nil, nil) when the system message has
// already been folded in via CompletionRequest.System.
func canonicalToOA(m agent.Message) (*oaMessage, error) {
	switch m.Role {
	case agent.RoleSystem:
		// System is set once via CompletionRequest.System; per-message
		// system roles are folded there.
		if strings.TrimSpace(m.Content) == "" {
			return nil, nil
		}
		return &oaMessage{Role: "system", Content: m.Content}, nil
	case agent.RoleUser:
		return &oaMessage{Role: "user", Content: m.Content}, nil
	case agent.RoleAssistant:
		om := &oaMessage{Role: "assistant", Content: m.Content}
		for _, tc := range m.ToolCalls {
			args := tc.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			// OpenAI expects arguments as a JSON-encoded *string*, not
			// a nested object. We already have valid JSON bytes; cast.
			om.ToolCalls = append(om.ToolCalls, oaToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: oaToolCallFn{
					Name:      tc.Name,
					Arguments: string(args),
				},
			})
		}
		return om, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("openai: role=tool requires tool_call_id")
		}
		return &oaMessage{
			Role:       "tool",
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}, nil
	default:
		return nil, fmt.Errorf("openai: unknown role %q", m.Role)
	}
}

func decodeResponse(body []byte) (*agent.CompletionResponse, error) {
	var or oaResponse
	if err := json.Unmarshal(body, &or); err != nil {
		return nil, fmt.Errorf("openai: parse response: %w", err)
	}
	if len(or.Choices) == 0 {
		return nil, fmt.Errorf("openai: response has no choices")
	}
	choice := or.Choices[0]

	var toolCalls []agent.ToolCall
	for i, tc := range choice.Message.ToolCalls {
		id := tc.ID
		if id == "" {
			id = "call-" + strconv.Itoa(i)
		}
		// OpenAI returns arguments as a JSON-encoded string; canonical
		// shape carries the parsed RawMessage. Treat empty as "{}".
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
	switch choice.FinishReason {
	case "stop":
		stop = agent.StopEndTurn
	case "tool_calls", "function_call":
		stop = agent.StopToolUse
	case "length":
		stop = agent.StopMaxTokens
	}
	// finish_reason is sometimes absent on openai-compatible servers
	// when tool_calls are emitted; fall back to tool-calls presence.
	if len(toolCalls) > 0 && stop == agent.StopEndTurn {
		stop = agent.StopToolUse
	}

	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			Content:   choice.Message.Content,
			ToolCalls: toolCalls,
		},
		StopReason: stop,
		Usage: agent.Usage{
			InputTokens:  or.Usage.PromptTokens,
			OutputTokens: or.Usage.CompletionTokens,
			Model:        or.Model,
		},
	}, nil
}
