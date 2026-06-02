// SPDX-License-Identifier: MIT

// Package anthropic is the in-process Anthropic Messages-API Provider.
//
// It translates between Agezt's canonical (dialect-free) agent.Message /
// agent.ToolCall / agent.ToolDef shapes and Anthropic's content-block
// format (SPEC-15). Non-streaming for M0.5; streaming lands later.
//
// Auth: api-key via the AGEZT_ANTHROPIC_API_KEY env var (or the constructor
// argument). OAuth/subscription auth lands with the Governor in MVP
// (TASKS P1-PROV-01).
package anthropic

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
	// DefaultEndpoint is the Anthropic Messages API URL.
	DefaultEndpoint = "https://api.anthropic.com/v1/messages"
	// DefaultModel is what the loop uses when CompletionRequest.Model is empty.
	// claude-sonnet-4-6 is the latest Sonnet at the project's knowledge cutoff.
	DefaultModel = "claude-sonnet-4-6"
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
		model = DefaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}

	body, err := encodeRequest(model, req.System, req.Messages, req.Tools, maxTokens)
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
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: http: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("anthropic: read body: %w", err)
	}
	if httpResp.StatusCode/100 != 2 {
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(respBytes)}
	}
	return decodeResponse(respBytes)
}

// ----- dialect translation (canonical ↔ Anthropic) -----

// anthRequest is the wire-shape of a Messages API request.
type anthRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Messages  []anthMessage `json:"messages"`
	Tools     []anthTool    `json:"tools,omitempty"`
}

type anthTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthMessage struct {
	Role    string      `json:"role"` // "user" | "assistant"
	Content []anthBlock `json:"content"`
}

// anthBlock is a single content block. Only one of Text / ToolUse /
// ToolResult is populated per block; the wire format is a tagged union.
type anthBlock struct {
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	ID         string          `json:"id,omitempty"`          // tool_use
	Name       string          `json:"name,omitempty"`        // tool_use
	Input      json.RawMessage `json:"input,omitempty"`       // tool_use
	ToolUseID  string          `json:"tool_use_id,omitempty"` // tool_result
	ResultBody string          `json:"content,omitempty"`     // tool_result (string form)
	IsError    bool            `json:"is_error,omitempty"`    // tool_result
}

type anthResponse struct {
	ID         string      `json:"id"`
	Type       string      `json:"type"`
	Role       string      `json:"role"`
	Content    []anthBlock `json:"content"`
	Model      string      `json:"model"`
	StopReason string      `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func encodeRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int) ([]byte, error) {
	wire := anthRequest{
		Model:     model,
		MaxTokens: maxTok,
		System:    system,
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
		// Skip role=system messages here — Anthropic uses the top-level
		// "system" field, set by the caller via CompletionRequest.System.
		if am == nil {
			continue
		}
		wire.Messages = append(wire.Messages, *am)
	}
	return json.Marshal(wire)
}

// canonicalToAnth converts one canonical Message into one Anthropic message.
// Returns (nil, nil) when the message has no Anthropic representation
// (e.g. role=system, which Anthropic carries as a top-level field).
func canonicalToAnth(m agent.Message) (*anthMessage, error) {
	switch m.Role {
	case agent.RoleSystem:
		return nil, nil
	case agent.RoleUser:
		return &anthMessage{
			Role:    "user",
			Content: []anthBlock{{Type: "text", Text: m.Content}},
		}, nil
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
			// Anthropic rejects empty assistant content; insert a placeholder.
			blocks = []anthBlock{{Type: "text", Text: ""}}
		}
		return &anthMessage{Role: "assistant", Content: blocks}, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("anthropic: role=tool requires tool_call_id")
		}
		return &anthMessage{
			Role: "user", // Anthropic routes tool results inside a user message
			Content: []anthBlock{{
				Type:       "tool_result",
				ToolUseID:  m.ToolCallID,
				ResultBody: m.Content,
			}},
		}, nil
	default:
		return nil, fmt.Errorf("anthropic: unknown role %q", m.Role)
	}
}

func decodeResponse(body []byte) (*agent.CompletionResponse, error) {
	var ar anthResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("anthropic: parse response: %w", err)
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
		Usage: agent.Usage{
			InputTokens:  ar.Usage.InputTokens,
			OutputTokens: ar.Usage.OutputTokens,
			Model:        ar.Model,
		},
	}, nil
}
