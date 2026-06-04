// SPDX-License-Identifier: MIT

package vertex

// Vertex AI exposes Anthropic models (`claude-*`) through a
// different publisher + endpoint suffix than its native Gemini
// models. The auth (service-account JWT → OAuth bearer) is the
// same; the body shape, version pin, and URL routing all change.
//
// Wire (per Google's Vertex AI docs as of 2026):
//
//	POST https://{region}-aiplatform.googleapis.com/v1/projects/{project}/locations/{region}/publishers/anthropic/models/{model}:rawPredict
//	Authorization: Bearer {oauth_access_token}
//	Content-Type: application/json
//	body: { "anthropic_version": "vertex-2023-10-16", "max_tokens": ..., "messages": [...], ... }
//
// Streaming uses `:streamRawPredict` and returns **standard
// Anthropic SSE** (event-tagged, the same dispatcher the direct
// Anthropic adapter speaks). Notably *not* AWS-style event-stream
// framing — only Bedrock uses that. Vertex inherits Google's
// preference for SSE.
//
// The model id encoding is one of the gotchas: Anthropic-on-Vertex
// model ids look like `claude-opus-4-7@20251031` (publisher version
// suffix). isAnthropicModel matches by prefix.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

const (
	// AnthropicVertexVersion is the value sent in the request body's
	// `anthropic_version` field. Vertex pins this; bumping requires
	// coordination with Google's release notes.
	AnthropicVertexVersion = "vertex-2023-10-16"
	// DefaultAnthropicMaxTokens is what we send when the request leaves
	// max_tokens at 0. Anthropic requires a non-zero max_tokens.
	DefaultAnthropicMaxTokens = 4096
)

// isAnthropicModel reports whether the Vertex model id maps to the
// Anthropic publisher. Vertex model ids for Anthropic look like:
//
//	claude-opus-4-7@20251031
//	claude-sonnet-4-5@20250929
//	claude-3-5-sonnet-v2@20241022
//
// We match the `claude-` prefix (case-insensitive) — that's the
// only stable signal Google emits across model revisions.
func isAnthropicModel(id string) bool {
	return strings.HasPrefix(strings.ToLower(id), "claude-")
}

// ResolveAnthropicEndpoint returns the `:rawPredict` URL Complete
// will POST to for an Anthropic model. Exported for tests + custom-
// URL verification, mirroring ResolveEndpoint.
func (p *Provider) ResolveAnthropicEndpoint(model string) string {
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
		"/publishers/anthropic/models/" + model + ":rawPredict"
}

// ResolveAnthropicStreamEndpoint returns the `:streamRawPredict` URL
// CompleteStream will POST to for an Anthropic model.
func (p *Provider) ResolveAnthropicStreamEndpoint(model string) string {
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
		"/publishers/anthropic/models/" + model + ":streamRawPredict"
}

// ----- wire types (Anthropic Messages API shape, no `model` field) -----

type anthVertexRequest struct {
	AnthropicVersion string          `json:"anthropic_version"`
	MaxTokens        int             `json:"max_tokens"`
	System           any             `json:"system,omitempty"`
	Messages         []anthVxMessage `json:"messages"`
	Tools            []anthVxTool    `json:"tools,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
}

// anthVxSystemBlock is the array form of the system prompt, carrying a
// prompt-cache breakpoint (M302) so the stable system prompt is cached alongside
// the tools.
type anthVxSystemBlock struct {
	Type         string              `json:"type"` // "text"
	Text         string              `json:"text"`
	CacheControl *anthVxCacheControl `json:"cache_control,omitempty"`
}

// buildVxSystem returns the system field: nil when empty (omitted), else a
// one-element cache-marked block array (M302). Vertex caches the prefix
// tools→system, so this caches tools AND system.
func buildVxSystem(system string) any {
	if system == "" {
		return nil
	}
	return []anthVxSystemBlock{{Type: "text", Text: system, CacheControl: &anthVxCacheControl{Type: "ephemeral"}}}
}

type anthVxTool struct {
	Name         string              `json:"name"`
	Description  string              `json:"description,omitempty"`
	InputSchema  json.RawMessage     `json:"input_schema"`
	CacheControl *anthVxCacheControl `json:"cache_control,omitempty"`
}

// anthVxCacheControl marks a tool/content block as a prompt-cache breakpoint
// (M300). Claude-on-Vertex caches the request prefix up to and including the
// marked block; "ephemeral" is the 5-minute TTL tier.
type anthVxCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// buildVxTools mirrors the direct-Anthropic provider (M299): it marks the LAST
// tool with cache_control so Vertex caches the stable tools prefix that repeats
// every agent-loop iteration. Vertex ignores the marker when the prefix is below
// the minimum cacheable size, so it's safe to always set.
func buildVxTools(tools []agent.ToolDef) []anthVxTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthVxTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, anthVxTool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	out[len(out)-1].CacheControl = &anthVxCacheControl{Type: "ephemeral"}
	return out
}

type anthVxMessage struct {
	Role    string        `json:"role"`
	Content []anthVxBlock `json:"content"`
}

type anthVxBlock struct {
	Type       string             `json:"type"`
	Text       string             `json:"text,omitempty"`
	ID         string             `json:"id,omitempty"`
	Name       string             `json:"name,omitempty"`
	Input      json.RawMessage    `json:"input,omitempty"`
	ToolUseID  string             `json:"tool_use_id,omitempty"`
	ResultBody string             `json:"content,omitempty"`
	IsError    bool               `json:"is_error,omitempty"`
	Source     *anthVxImageSource `json:"source,omitempty"` // type=image
}

// anthVxImageSource is the base64 payload of a type=image content block — how
// Anthropic-on-Vertex carries an image attachment (M245).
type anthVxImageSource struct {
	Type      string `json:"type"`       // always "base64"
	MediaType string `json:"media_type"` // image/png | image/jpeg | image/gif | image/webp
	Data      string `json:"data"`       // base64-encoded image bytes
}

type anthVxResponse struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Role       string        `json:"role"`
	Content    []anthVxBlock `json:"content"`
	StopReason string        `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// anthVxUsageToAgent maps Anthropic-on-Vertex split token counts to agent.Usage
// (M290), mirroring the direct-Anthropic provider: input_tokens excludes cached
// prompt tokens, so the real prompt is input + cache_read + cache_creation;
// cache reads are marked cached (cheaper rate), cache-creation as cache-write
// (the cache-write premium, M291).
func anthVxUsageToAgent(inputTokens, cacheRead, cacheCreation, outputTokens int, model string) agent.Usage {
	return agent.Usage{
		InputTokens:           inputTokens + cacheRead + cacheCreation,
		CachedInputTokens:     cacheRead,
		CacheWriteInputTokens: cacheCreation,
		OutputTokens:          outputTokens,
		Model:                 model,
	}
}

func encodeAnthropicOnVertexRequest(system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int, stream bool) ([]byte, error) {
	wire := anthVertexRequest{
		AnthropicVersion: AnthropicVertexVersion,
		MaxTokens:        maxTok,
		System:           buildVxSystem(system),
		Stream:           stream,
		Tools:            buildVxTools(tools),
	}
	for _, m := range msgs {
		am, err := canonicalToAnthVx(m)
		if err != nil {
			return nil, err
		}
		if am == nil {
			continue
		}
		wire.Messages = append(wire.Messages, *am)
	}
	return json.Marshal(wire)
}

// parseImageDataURL splits an RFC 2397 data: URL of the form
// "data:<media-type>;base64,<payload>" into its media type and base64 payload,
// returning ok=false for anything else (including a legacy bare filename),
// which the caller skips. The CLI sends data: URLs (M241). Shared by the
// Anthropic-on-Vertex and Gemini-on-Vertex encoders.
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

func canonicalToAnthVx(m agent.Message) (*anthVxMessage, error) {
	switch m.Role {
	case agent.RoleSystem:
		return nil, nil
	case agent.RoleUser:
		// Vision (M245): a user message may carry image attachments as RFC 2397
		// data: URLs. Emit each as a type=image block before the text block. A
		// non-data-URL entry (e.g. a legacy bare filename) is skipped.
		blocks := make([]anthVxBlock, 0, len(m.Images)+1)
		for _, img := range m.Images {
			if mt, data, ok := parseImageDataURL(img); ok {
				blocks = append(blocks, anthVxBlock{
					Type:   "image",
					Source: &anthVxImageSource{Type: "base64", MediaType: mt, Data: data},
				})
			}
		}
		blocks = append(blocks, anthVxBlock{Type: "text", Text: m.Content})
		return &anthVxMessage{Role: "user", Content: blocks}, nil
	case agent.RoleAssistant:
		var blocks []anthVxBlock
		if strings.TrimSpace(m.Content) != "" {
			blocks = append(blocks, anthVxBlock{Type: "text", Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			input := tc.Input
			if len(input) == 0 {
				input = json.RawMessage(`{}`)
			}
			blocks = append(blocks, anthVxBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Name,
				Input: input,
			})
		}
		if len(blocks) == 0 {
			blocks = []anthVxBlock{{Type: "text", Text: ""}}
		}
		return &anthVxMessage{Role: "assistant", Content: blocks}, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("vertex: role=tool requires tool_call_id")
		}
		return &anthVxMessage{
			Role: "user",
			Content: []anthVxBlock{{
				Type:       "tool_result",
				ToolUseID:  m.ToolCallID,
				ResultBody: m.Content,
			}},
		}, nil
	default:
		return nil, fmt.Errorf("vertex: unknown role %q", m.Role)
	}
}

func decodeAnthropicOnVertexResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var ar anthVxResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("vertex: parse anthropic response: %w", err)
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
		Usage: anthVxUsageToAgent(
			ar.Usage.InputTokens, ar.Usage.CacheReadInputTokens,
			ar.Usage.CacheCreationInputTokens, ar.Usage.OutputTokens, model),
	}, nil
}

// ----- HTTP execution -----

// completeAnthropic is the Anthropic-on-Vertex non-streaming path.
// Called from Complete when isAnthropicModel(model) is true.
func (p *Provider) completeAnthropic(ctx context.Context, req agent.CompletionRequest, model string) (*agent.CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultAnthropicMaxTokens
	}
	body, err := encodeAnthropicOnVertexRequest(req.System, req.Messages, req.Tools, maxTokens, false)
	if err != nil {
		return nil, fmt.Errorf("vertex: encode anthropic request: %w", err)
	}
	tok, err := p.TokenSource.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("vertex: get access token: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ResolveAnthropicEndpoint(model), bytes.NewReader(body))
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
	return decodeAnthropicOnVertexResponse(respBytes, model)
}

// completeStreamAnthropic is the Anthropic-on-Vertex streaming path.
// Called from CompleteStream when isAnthropicModel(model) is true.
// Wire is standard Anthropic SSE (event-tagged, not the binary
// event-stream format Bedrock uses).
func (p *Provider) completeStreamAnthropic(ctx context.Context, req agent.CompletionRequest, model string, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultAnthropicMaxTokens
	}
	body, err := encodeAnthropicOnVertexRequest(req.System, req.Messages, req.Tools, maxTokens, true)
	if err != nil {
		return nil, fmt.Errorf("vertex: encode anthropic request: %w", err)
	}
	tok, err := p.TokenSource.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("vertex: get access token: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ResolveAnthropicStreamEndpoint(model), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("vertex: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+tok)
	httpReq.Header.Set("Accept", "text/event-stream")

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("vertex: http: %w", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode/100 != 2 {
		raw, _ := httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(raw)}
	}
	return parseAnthropicSSE(httpResp.Body, model, onChunk)
}

// ----- Anthropic SSE dispatch (event-tagged) -----
//
// Duplicates the dispatch logic in plugins/providers/anthropic and
// plugins/providers/bedrock for the same reason both packages
// duplicate the body encode/decode: keeping Vertex independent of
// the other adapters' evolution.

type anthStreamState struct {
	textParts     strings.Builder
	openBlock     *anthOpenBlock
	finishedTools []agent.ToolCall
	inputTokens   int
	cacheRead     int // cache_read_input_tokens (M290)
	cacheCreation int // cache_creation_input_tokens (M290)
	outputTokens  int
	stopReason    string
}

type anthOpenBlock struct {
	kind     string
	toolID   string
	toolName string
	textBuf  strings.Builder
	inputBuf strings.Builder
}

func parseAnthropicSSE(body io.Reader, model string, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	st := &anthStreamState{}
	var pendingEvent string

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			pendingEvent = ""
		case strings.HasPrefix(line, ":"):
			// SSE comment / keep-alive; ignore.
		case strings.HasPrefix(line, "event:"):
			pendingEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			if err := dispatchAnthropicSSE(pendingEvent, data, st, onChunk); err != nil {
				return nil, err
			}
			if pendingEvent == "message_stop" {
				return assembleAnthropicResponse(st, model), nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("vertex: anthropic stream read: %w", err)
	}
	return assembleAnthropicResponse(st, model), nil
}

func dispatchAnthropicSSE(eventName, data string, st *anthStreamState, onChunk func(agent.Chunk) error) error {
	switch eventName {
	case "message_start":
		var f struct {
			Message struct {
				Usage struct {
					InputTokens              int `json:"input_tokens"`
					OutputTokens             int `json:"output_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return fmt.Errorf("vertex: parse message_start: %w", err)
		}
		st.inputTokens = f.Message.Usage.InputTokens
		st.cacheRead = f.Message.Usage.CacheReadInputTokens
		st.cacheCreation = f.Message.Usage.CacheCreationInputTokens
		if f.Message.Usage.OutputTokens > 0 {
			st.outputTokens = f.Message.Usage.OutputTokens
		}

	case "content_block_start":
		var f struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type  string          `json:"type"`
				Text  string          `json:"text"`
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return fmt.Errorf("vertex: parse content_block_start: %w", err)
		}
		st.openBlock = &anthOpenBlock{kind: f.ContentBlock.Type}
		switch f.ContentBlock.Type {
		case "text":
			st.openBlock.textBuf.WriteString(f.ContentBlock.Text)
			if f.ContentBlock.Text != "" {
				if err := onChunk(agent.Chunk{TextDelta: f.ContentBlock.Text}); err != nil {
					return err
				}
			}
		case "tool_use":
			st.openBlock.toolID = f.ContentBlock.ID
			st.openBlock.toolName = f.ContentBlock.Name
			start := &agent.ToolCall{
				ID:    f.ContentBlock.ID,
				Name:  f.ContentBlock.Name,
				Input: json.RawMessage(`{}`),
			}
			if err := onChunk(agent.Chunk{ToolUseStart: start}); err != nil {
				return err
			}
		}

	case "content_block_delta":
		var f struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return fmt.Errorf("vertex: parse content_block_delta: %w", err)
		}
		if st.openBlock == nil {
			return nil
		}
		switch f.Delta.Type {
		case "text_delta":
			st.openBlock.textBuf.WriteString(f.Delta.Text)
			if f.Delta.Text != "" {
				if err := onChunk(agent.Chunk{TextDelta: f.Delta.Text}); err != nil {
					return err
				}
			}
		case "input_json_delta":
			st.openBlock.inputBuf.WriteString(f.Delta.PartialJSON)
			if f.Delta.PartialJSON != "" {
				if err := onChunk(agent.Chunk{ToolInputJSONDelta: f.Delta.PartialJSON}); err != nil {
					return err
				}
			}
		}

	case "content_block_stop":
		if st.openBlock == nil {
			return nil
		}
		ob := st.openBlock
		switch ob.kind {
		case "text":
			st.textParts.WriteString(ob.textBuf.String())
		case "tool_use":
			input := strings.TrimSpace(ob.inputBuf.String())
			if input == "" {
				input = "{}"
			}
			st.finishedTools = append(st.finishedTools, agent.ToolCall{
				ID:    ob.toolID,
				Name:  ob.toolName,
				Input: json.RawMessage(input),
			})
			if err := onChunk(agent.Chunk{ToolUseStop: ob.toolID}); err != nil {
				return err
			}
		}
		st.openBlock = nil

	case "message_delta":
		var f struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return fmt.Errorf("vertex: parse message_delta: %w", err)
		}
		if f.Delta.StopReason != "" {
			st.stopReason = f.Delta.StopReason
		}
		if f.Usage.OutputTokens > 0 {
			st.outputTokens = f.Usage.OutputTokens
		}

	case "message_stop", "ping", "":
		// terminal / keepalive / no-op
	case "error":
		var f struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return fmt.Errorf("vertex: anthropic stream error frame (unparseable): %s", data)
		}
		return fmt.Errorf("vertex: anthropic stream error (%s): %s", f.Error.Type, f.Error.Message)
	}
	return nil
}

func assembleAnthropicResponse(st *anthStreamState, model string) *agent.CompletionResponse {
	stop := agent.StopReason(st.stopReason)
	switch st.stopReason {
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
			Content:   st.textParts.String(),
			ToolCalls: st.finishedTools,
		},
		StopReason: stop,
		Usage: anthVxUsageToAgent(
			st.inputTokens, st.cacheRead, st.cacheCreation, st.outputTokens, model),
	}
}
