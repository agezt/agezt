// SPDX-License-Identifier: MIT

package vertex

// Anthropic-on-Vertex STREAMING path: completeStreamAnthropic +
// parseAnthropicSSE + dispatchAnthropicSSE + assembleAnthropicResponse +
// anthStreamState + anthOpenBlock types. Carved out of anthropic.go
// during the Day 162 god-file split so the main file can focus on
// request encoding + Complete.
// Public API unchanged.

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
	"github.com/agezt/agezt/plugins/providers/internal/retry"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)
func (p *Provider) completeStreamAnthropic(ctx context.Context, req agent.CompletionRequest, model string, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultAnthropicMaxTokens
	}
	body, err := encodeAnthropicOnVertexRequest(req.System, req.Messages, req.Tools, maxTokens, p.ThinkingBudget, true, req.Params, req.ProviderOptions["vertex"])
	if err != nil {
		return nil, fmt.Errorf("vertex: encode anthropic request: %w", err)
	}
	// Stream SETUP retries transient failures before the first frame (LD-4);
	// build runs per attempt so a refreshed OAuth token is picked up fresh.
	httpResp, err := retry.DoHTTPStream(ctx, p.HTTP, func() (*http.Request, error) {
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
		return httpReq, nil
	}, httpread.DefaultMaxResponseBytes)
	if err != nil {
		var h *retry.HTTPError
		if errors.As(err, &h) {
			return nil, &APIError{Status: h.StatusCode, Body: h.Body}
		}
		return nil, fmt.Errorf("vertex: http: %w", err)
	}
	defer httpResp.Body.Close()
	resp, err := parseAnthropicSSE(httpResp.Body, model, onChunk)
	if err != nil {
		return nil, err
	}
	toolname.RestoreCalls(resp, toolname.Reverse(req.Tools))
	return resp, nil
}

// ----- Anthropic SSE dispatch (event-tagged) -----
//
// Duplicates the dispatch logic in plugins/providers/anthropic and
// plugins/providers/bedrock for the same reason both packages
// duplicate the body encode/decode: keeping Vertex independent of
// the other adapters' evolution.

type anthStreamState struct {
	textParts      strings.Builder
	reasoningParts strings.Builder // extended-thinking blocks (M321)
	openBlock      *anthOpenBlock
	finishedTools  []agent.ToolCall
	inputTokens    int
	cacheRead      int // cache_read_input_tokens (M290)
	cacheCreation  int // cache_creation_input_tokens (M290)
	outputTokens   int
	stopReason     string
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
				Type     string          `json:"type"`
				Text     string          `json:"text"`
				Thinking string          `json:"thinking"`
				ID       string          `json:"id"`
				Name     string          `json:"name"`
				Input    json.RawMessage `json:"input"`
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
		case "thinking":
			// Extended-thinking block (M321): accumulate into textBuf (routed
			// to reasoningParts at block_stop) and surface as ReasoningDelta.
			st.openBlock.textBuf.WriteString(f.ContentBlock.Thinking)
			if f.ContentBlock.Thinking != "" {
				if err := onChunk(agent.Chunk{ReasoningDelta: f.ContentBlock.Thinking}); err != nil {
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
				Thinking    string `json:"thinking"`
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
		case "thinking_delta":
			// Extended-thinking delta (M321): accumulate + surface as ReasoningDelta.
			st.openBlock.textBuf.WriteString(f.Delta.Thinking)
			if f.Delta.Thinking != "" {
				if err := onChunk(agent.Chunk{ReasoningDelta: f.Delta.Thinking}); err != nil {
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
		case "thinking":
			st.reasoningParts.WriteString(ob.textBuf.String()) // M321
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
		ReasoningContent: st.reasoningParts.String(), // M321
	}
}
