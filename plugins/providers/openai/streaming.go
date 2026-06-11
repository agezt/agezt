// SPDX-License-Identifier: MIT

package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

// CompleteStream implements agent.StreamingProvider. It POSTs to the
// Chat Completions endpoint with stream=true (and
// stream_options.include_usage=true so the final chunk carries the
// usage block) and parses the SSE stream into agent.Chunk callbacks.
// The returned CompletionResponse matches Complete for the same request.
//
// SSE shape (different from Anthropic):
//
//   - No "event:" prefix lines — every frame is plain "data: {json}\n\n".
//   - Stream terminated by the literal sentinel "data: [DONE]\n\n".
//   - Each frame has choices[0].delta with optional content and/or
//     tool_calls arrays. Tool calls in stream are indexed (multiple
//     parallel calls allowed); the id + function.name appear only in
//     the first chunk per index, subsequent chunks carry only the
//     function.arguments fragment.
//   - usage block appears in the final pre-[DONE] chunk when
//     stream_options.include_usage=true is set. We always send it so
//     callers can do Governor-grade cost accounting.
//
// Inherits the same family coverage as Complete: real OpenAI,
// openai-compatible (Groq / Cerebras / SambaNova / Together /
// DeepInfra / Perplexity / Fireworks / xai / OpenRouter), Mistral,
// and Azure OpenAI. The auth header (Authorization vs api-key) and
// auth scheme (Bearer vs raw) come from the same AuthHeader /
// AuthScheme fields the non-streaming path uses.
func (p *Provider) CompleteStream(ctx context.Context, req agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	if p.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	if onChunk == nil {
		return nil, errors.New("openai: CompleteStream requires non-nil onChunk")
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		model = DefaultModel
	}

	body, err := encodeStreamRequest(model, req.System, req.Messages, req.Tools, req.MaxTokens, req.JSONMode)
	if err != nil {
		return nil, fmt.Errorf("openai: encode request: %w", err)
	}

	endpoint := p.resolveEndpoint()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	authHeader := p.AuthHeader
	if authHeader == "" {
		authHeader = "Authorization"
	}
	authScheme := p.AuthScheme
	if authScheme == "" && p.AuthHeader == "" {
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

	if httpResp.StatusCode/100 != 2 {
		raw, _ := httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(raw)}
	}

	resp, err := parseStream(httpResp.Body, onChunk)
	if err != nil {
		return nil, err
	}
	restoreToolCallNames(resp, reverseToolNames(req.Tools))
	return resp, nil
}

// encodeStreamRequest mirrors encodeRequest but flips stream=true
// and adds stream_options.include_usage=true. Kept separate so the
// non-streaming wire format stays byte-identical to what existing
// tests verified.
func encodeStreamRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int, jsonMode bool) ([]byte, error) {
	type streamOptions struct {
		IncludeUsage bool `json:"include_usage"`
	}
	type streamReq struct {
		Model          string            `json:"model"`
		Messages       []oaMessage       `json:"messages"`
		Tools          []oaTool          `json:"tools,omitempty"`
		MaxTokens      int               `json:"max_tokens,omitempty"`
		Stream         bool              `json:"stream"`
		StreamOptions  *streamOptions    `json:"stream_options,omitempty"`
		ResponseFormat *oaResponseFormat `json:"response_format,omitempty"`
	}
	wire := streamReq{
		Model:          model,
		Stream:         true,
		MaxTokens:      maxTok,
		StreamOptions:  &streamOptions{IncludeUsage: true},
		ResponseFormat: jsonObjectFormat(jsonMode),
	}
	fwd, _ := wireToolNames(tools)
	if strings.TrimSpace(system) != "" {
		wire.Messages = append(wire.Messages, oaMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		om, err := canonicalToOA(m, fwd)
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
				Name:        fwd[t.Name],
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return json.Marshal(wire)
}

// ----- SSE parsing -----

// streamState accumulates everything we need to assemble the final
// CompletionResponse as SSE frames arrive. Tool calls are tracked by
// index because the wire format uses index to correlate fragments
// across chunks (id and name only appear in the first chunk per
// index).
type streamState struct {
	textParts      strings.Builder
	reasoningParts strings.Builder   // M317: accumulated reasoning (DeepSeek-R1 et al.)
	tools          map[int]*openTool // index → open tool call
	toolOrder      []int             // index order (preserves emit order for the response)
	finishReason   string
	model          string
	inputTokens    int
	outputTokens   int
	cachedTokens   int // prompt tokens served from the provider's cache (M887)
}

type openTool struct {
	id      string
	name    string
	argsBuf strings.Builder
}

// parseStream consumes the SSE response body until "data: [DONE]" or
// EOF. Each non-blank, non-data line is ignored (forward-compat —
// future OpenAI changes that add comment lines, retry hints, etc.
// won't break the parser).
func parseStream(body io.Reader, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	scanner := bufio.NewScanner(body)
	// Tool input JSON can be large; bump from default 64K to 1MB.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	st := &streamState{tools: map[int]*openTool{}}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			return assembleResponse(st), nil
		}
		if err := dispatchSSEFrame(data, st, onChunk); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("openai: stream read: %w", err)
	}
	// EOF without [DONE] — assemble what we have rather than throwing
	// away already-streamed tokens.
	return assembleResponse(st), nil
}

// dispatchSSEFrame parses one JSON chunk and updates state.
func dispatchSSEFrame(data string, st *streamState, onChunk func(agent.Chunk) error) error {
	var f struct {
		Model   string `json:"model"`
		Choices []struct {
			Index int `json:"index"`
			Delta struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"` // DeepSeek-R1 (M317)
				Reasoning        string `json:"reasoning"`         // other compat gateways
				ToolCalls        []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			// DeepSeek's spelling of the cache-read count (M887); see oaResponse.
			PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &f); err != nil {
		// Don't fail the whole stream on one unparseable frame —
		// OpenAI proxies (and some openai-compatible vendors) have
		// been seen to inject keepalive comments as malformed JSON.
		return nil
	}
	if f.Model != "" {
		st.model = f.Model
	}
	if f.Usage != nil {
		st.inputTokens = f.Usage.PromptTokens
		st.outputTokens = f.Usage.CompletionTokens
		st.cachedTokens = cachedInputTokens(f.Usage.PromptTokensDetails.CachedTokens, f.Usage.PromptCacheHitTokens)
	}

	if len(f.Choices) == 0 {
		return nil
	}
	choice := f.Choices[0]
	if choice.FinishReason != "" {
		st.finishReason = choice.FinishReason
	}

	// Reasoning delta (M317): DeepSeek-R1 streams its chain of thought in a
	// separate reasoning_content field before the answer tokens.
	if rd := choice.Delta.ReasoningContent; rd != "" {
		st.reasoningParts.WriteString(rd)
		if err := onChunk(agent.Chunk{ReasoningDelta: rd}); err != nil {
			return err
		}
	} else if rd := choice.Delta.Reasoning; rd != "" {
		st.reasoningParts.WriteString(rd)
		if err := onChunk(agent.Chunk{ReasoningDelta: rd}); err != nil {
			return err
		}
	}

	if choice.Delta.Content != "" {
		st.textParts.WriteString(choice.Delta.Content)
		if err := onChunk(agent.Chunk{TextDelta: choice.Delta.Content}); err != nil {
			return err
		}
	}

	for _, tcd := range choice.Delta.ToolCalls {
		idx := tcd.Index
		tool, exists := st.tools[idx]
		if !exists {
			tool = &openTool{}
			st.tools[idx] = tool
			st.toolOrder = append(st.toolOrder, idx)
		}
		// First chunk for a given index carries id + function.name; we
		// only adopt them if not already set so a later chunk's empty
		// strings don't clobber them.
		if tool.id == "" && tcd.ID != "" {
			tool.id = tcd.ID
		}
		if tool.name == "" && tcd.Function.Name != "" {
			tool.name = tcd.Function.Name
			// Emit ToolUseStart only once per index, on the first
			// chunk where we know the name.
			start := &agent.ToolCall{
				ID:    tool.id,
				Name:  tool.name,
				Input: json.RawMessage(`{}`),
			}
			if start.ID == "" {
				// Some openai-compatible vendors omit the id; synthesize
				// a deterministic one so the loop's tool-result message
				// can still reference it.
				start.ID = "call-" + strconv.Itoa(idx)
				tool.id = start.ID
			}
			if err := onChunk(agent.Chunk{ToolUseStart: start}); err != nil {
				return err
			}
		}
		if tcd.Function.Arguments != "" {
			tool.argsBuf.WriteString(tcd.Function.Arguments)
			if err := onChunk(agent.Chunk{ToolInputJSONDelta: tcd.Function.Arguments}); err != nil {
				return err
			}
		}
	}

	// Tool stop signal: emit a ToolUseStop for each open tool once
	// finish_reason arrives. OpenAI doesn't have an explicit
	// per-tool-stop frame; we synthesize on the terminal frame so
	// callers get a clean lifecycle.
	if choice.FinishReason != "" {
		for _, idx := range st.toolOrder {
			tool := st.tools[idx]
			id := tool.id
			if id == "" {
				id = "call-" + strconv.Itoa(idx)
			}
			if err := onChunk(agent.Chunk{ToolUseStop: id}); err != nil {
				return err
			}
		}
	}
	return nil
}

// assembleResponse converts the accumulated streamState into the same
// CompletionResponse shape Complete returns.
func assembleResponse(st *streamState) *agent.CompletionResponse {
	stop := agent.StopEndTurn
	switch st.finishReason {
	case "stop":
		stop = agent.StopEndTurn
	case "tool_calls", "function_call":
		stop = agent.StopToolUse
	case "length":
		stop = agent.StopMaxTokens
	}

	var toolCalls []agent.ToolCall
	for _, idx := range st.toolOrder {
		tool := st.tools[idx]
		args := strings.TrimSpace(tool.argsBuf.String())
		if args == "" {
			args = "{}"
		}
		id := tool.id
		if id == "" {
			id = "call-" + strconv.Itoa(idx)
		}
		toolCalls = append(toolCalls, agent.ToolCall{
			ID:    id,
			Name:  tool.name,
			Input: json.RawMessage(args),
		})
	}
	if len(toolCalls) > 0 && stop == agent.StopEndTurn {
		// finish_reason is sometimes absent on openai-compatible
		// servers when tool calls are emitted (same quirk Complete
		// works around).
		stop = agent.StopToolUse
	}

	return &agent.CompletionResponse{
		ReasoningContent: st.reasoningParts.String(), // M317
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			Content:   st.textParts.String(),
			ToolCalls: toolCalls,
		},
		StopReason: stop,
		Usage: agent.Usage{
			InputTokens:       st.inputTokens,
			CachedInputTokens: st.cachedTokens, // M887: cache hits price at the cache-read rate
			OutputTokens:      st.outputTokens,
			Model:             st.model,
		},
	}
}
