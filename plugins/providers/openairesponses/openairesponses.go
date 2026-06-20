// SPDX-License-Identifier: MIT

// Package openairesponses is the provider adapter for "Sign in with ChatGPT":
// it speaks the OpenAI Responses API against the ChatGPT backend
// (https://chatgpt.com/backend-api/codex/responses) using a subscription OAuth
// access token, the same wire Codex CLI uses. It translates AGEZT's
// chat-shaped CompletionRequest to/from Responses items, streams the SSE reply,
// and assembles a single non-streaming CompletionResponse.
//
// This is an UNOFFICIAL, undocumented backend (it requires Codex's own system
// instructions, embedded below) and may break or violate OpenAI's terms — see
// kernel/chatgptauth. The instructions.md file is Codex's prompt, vendored from
// the Apache-2.0 openai/codex repo so the backend accepts our requests.
package openairesponses

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"crypto/rand"
	_ "embed"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/netguard"
)

//go:embed instructions.md
var codexInstructions string

const (
	// DefaultBaseURL is the ChatGPT subscription backend (NOT api.openai.com).
	DefaultBaseURL = "https://chatgpt.com/backend-api/codex"
	betaHeader     = "responses=experimental"
	originator     = "codex_cli_rs"
)

// httpClientFor builds the HTTP client. Defaults to an SSRF-guarded client
// (the backend is public); tests override it to reach an httptest server.
var httpClientFor = func(timeout time.Duration) *http.Client {
	return netguard.New().HTTPClient(timeout)
}

// TokenFunc returns a valid access token + ChatGPT account id; force requests a
// refresh (used after a 401). Backed by chatgptauth.Manager in production.
type TokenFunc func(ctx context.Context, force bool) (access, accountID string, err error)

// Provider implements agent.Provider over the ChatGPT Responses backend.
type Provider struct {
	ID              string
	Model           string
	BaseURL         string
	Token           TokenFunc
	ReasoningEffort string // "" omits the reasoning field; default "medium"
	newSession      func() string
}

// New builds a provider with id (catalog name), default model, and a token source.
func New(id, model string, token TokenFunc) *Provider {
	return &Provider{ID: id, Model: model, BaseURL: DefaultBaseURL, Token: token, ReasoningEffort: "medium"}
}

func (p *Provider) Name() string { return p.ID }

func (p *Provider) base() string {
	if p.BaseURL != "" {
		return strings.TrimRight(p.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (p *Provider) session() string {
	if p.newSession != nil {
		return p.newSession()
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Complete sends one request, retrying once after a forced token refresh on 401.
func (p *Provider) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if p.Token == nil {
		return nil, fmt.Errorf("openairesponses: no token source")
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		return nil, fmt.Errorf("openairesponses: no model")
	}
	body, err := p.buildBody(req, model)
	if err != nil {
		return nil, err
	}

	resp, status, err := p.send(ctx, body, false)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized {
		// Reactive refresh-and-retry once.
		resp, status, err = p.send(ctx, body, true)
		if err != nil {
			return nil, err
		}
	}
	if status < 200 || status >= 300 {
		msg := strings.TrimSpace(string(resp))
		if len(msg) > 600 {
			msg = msg[:600]
		}
		return nil, fmt.Errorf("openairesponses: backend status %d: %s", status, msg)
	}
	return parseSSE(resp)
}

// send performs one POST, returning the raw body bytes + status. When force, it
// asks the token source to refresh first.
func (p *Provider) send(ctx context.Context, body []byte, force bool) ([]byte, int, error) {
	access, accountID, err := p.Token(ctx, force)
	if err != nil {
		return nil, 0, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base()+"/responses", strings.NewReader(string(body)))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+access)
	if accountID != "" {
		httpReq.Header.Set("chatgpt-account-id", accountID)
	}
	httpReq.Header.Set("OpenAI-Beta", betaHeader)
	httpReq.Header.Set("originator", originator)
	httpReq.Header.Set("session_id", p.session())
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	client := p.client()
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	return raw, resp.StatusCode, nil
}

func (p *Provider) client() *http.Client {
	return httpClientFor(120 * time.Second)
}

// --- request translation ---

type toolDef struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      bool            `json:"strict"`
}

type reasoningField struct {
	Effort string `json:"effort"`
}

type reqBody struct {
	Model             string          `json:"model"`
	Instructions      string          `json:"instructions"`
	Input             []any           `json:"input"`
	Tools             []toolDef       `json:"tools,omitempty"`
	ToolChoice        string          `json:"tool_choice,omitempty"`
	ParallelToolCalls bool            `json:"parallel_tool_calls"`
	Reasoning         *reasoningField `json:"reasoning,omitempty"`
	Store             bool            `json:"store"`
	Stream            bool            `json:"stream"`
}

func (p *Provider) buildBody(req agent.CompletionRequest, model string) ([]byte, error) {
	instructions := codexInstructions
	if s := strings.TrimSpace(req.System); s != "" {
		instructions = codexInstructions + "\n\n" + s
	}
	b := reqBody{
		Model:        model,
		Instructions: instructions,
		Input:        toInput(req.Messages),
		Tools:        toTools(req.Tools),
		Store:        false,
		Stream:       true,
	}
	if len(b.Tools) > 0 {
		b.ToolChoice = "auto"
	}
	if p.ReasoningEffort != "" {
		b.Reasoning = &reasoningField{Effort: p.ReasoningEffort}
	}
	return json.Marshal(b)
}

func contentText(kind, text string) map[string]any {
	return map[string]any{"type": kind, "text": text}
}

// toInput maps AGEZT messages to Responses input items.
func toInput(msgs []agent.Message) []any {
	out := make([]any, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case agent.RoleUser:
			out = append(out, map[string]any{
				"type": "message", "role": "user",
				"content": []any{contentText("input_text", m.Content)},
			})
		case agent.RoleSystem:
			out = append(out, map[string]any{
				"type": "message", "role": "developer",
				"content": []any{contentText("input_text", m.Content)},
			})
		case agent.RoleAssistant:
			if strings.TrimSpace(m.Content) != "" {
				out = append(out, map[string]any{
					"type": "message", "role": "assistant",
					"content": []any{contentText("output_text", m.Content)},
				})
			}
			for _, tc := range m.ToolCalls {
				args := string(tc.Input)
				if strings.TrimSpace(args) == "" {
					args = "{}"
				}
				out = append(out, map[string]any{
					"type": "function_call", "name": tc.Name,
					"arguments": args, "call_id": tc.ID,
				})
			}
		case agent.RoleTool:
			out = append(out, map[string]any{
				"type": "function_call_output", "call_id": m.ToolCallID, "output": m.Content,
			})
		}
	}
	return out
}

func toTools(defs []agent.ToolDef) []toolDef {
	if len(defs) == 0 {
		return nil
	}
	out := make([]toolDef, 0, len(defs))
	for _, d := range defs {
		params := d.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object"}`)
		}
		out = append(out, toolDef{
			Type: "function", Name: d.Name, Description: d.Description, Parameters: params,
		})
	}
	return out
}

// --- SSE response parsing ---

type sseEvent struct {
	Type     string          `json:"type"`
	Delta    string          `json:"delta"`
	Item     json.RawMessage `json:"item"`
	Response json.RawMessage `json:"response"`
	Error    json.RawMessage `json:"error"`
}

type respItem struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Name    string `json:"name"`
	Args    string `json:"arguments"`
	CallID  string `json:"call_id"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type respObj struct {
	Status string `json:"status"`
	Usage  struct {
		InputTokens       int `json:"input_tokens"`
		OutputTokens      int `json:"output_tokens"`
		CachedInputTokens int `json:"cached_input_tokens"`
	} `json:"usage"`
	Output []respItem `json:"output"`
	Error  struct {
		Message string `json:"message"`
	} `json:"error"`
}

// parseSSE walks the event stream, assembling text + tool calls + usage.
func parseSSE(raw []byte) (*agent.CompletionResponse, error) {
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	sc.Buffer(make([]byte, 0, 1024*1024), 16<<20)

	var textParts []string
	var deltaBuf strings.Builder
	var toolCalls []agent.ToolCall
	var usage agent.Usage
	var completed bool
	var failure string

	addItem := func(it respItem) {
		switch it.Type {
		case "message":
			for _, c := range it.Content {
				if c.Type == "output_text" || c.Type == "text" {
					textParts = append(textParts, c.Text)
				}
			}
		case "function_call":
			args := it.Args
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, agent.ToolCall{ID: it.CallID, Name: it.Name, Input: json.RawMessage(args)})
		}
	}

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var ev sseEvent
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "response.output_text.delta":
			deltaBuf.WriteString(ev.Delta)
		case "response.output_item.done":
			var it respItem
			if json.Unmarshal(ev.Item, &it) == nil {
				addItem(it)
			}
		case "response.completed":
			var r respObj
			if json.Unmarshal(ev.Response, &r) == nil {
				usage = agent.Usage{
					InputTokens:       r.Usage.InputTokens,
					OutputTokens:      r.Usage.OutputTokens,
					CachedInputTokens: r.Usage.CachedInputTokens,
				}
				// Fall back to the terminal output array if no item.done arrived.
				if len(textParts) == 0 && len(toolCalls) == 0 {
					for _, it := range r.Output {
						addItem(it)
					}
				}
			}
			completed = true
		case "response.failed", "error":
			failure = sseError(ev)
		}
	}

	if failure != "" {
		return nil, fmt.Errorf("openairesponses: %s", failure)
	}
	if !completed && len(textParts) == 0 && len(toolCalls) == 0 && deltaBuf.Len() == 0 {
		return nil, fmt.Errorf("openairesponses: empty/incomplete response stream")
	}

	text := strings.Join(textParts, "")
	if text == "" {
		text = deltaBuf.String()
	}
	stop := agent.StopEndTurn
	if len(toolCalls) > 0 {
		stop = agent.StopToolUse
	}
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: text, ToolCalls: toolCalls},
		StopReason: stop,
		Usage:      usage,
	}, nil
}

func sseError(ev sseEvent) string {
	if len(ev.Response) > 0 {
		var r respObj
		if json.Unmarshal(ev.Response, &r) == nil && r.Error.Message != "" {
			return r.Error.Message
		}
	}
	if len(ev.Error) > 0 {
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(ev.Error, &e) == nil && e.Message != "" {
			return e.Message
		}
	}
	return "response failed"
}
