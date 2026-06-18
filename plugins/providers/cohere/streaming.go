// SPDX-License-Identifier: MIT

package cohere

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

// CompleteStream implements agent.StreamingProvider for Cohere v2.
// POSTs to the same /v2/chat endpoint with stream:true. Response is
// SSE with Cohere-specific typed events:
//
//	message-start      → message id, model
//	content-start      → opens a text content block
//	content-delta      → text fragment
//	content-end        → closes the text content block
//	tool-plan-delta    → Cohere-specific tool reasoning (ignored for now)
//	tool-call-start    → opens a tool call (id, name)
//	tool-call-delta    → streamed JSON fragment of the tool args
//	tool-call-end      → closes the tool call
//	message-end        → carries finish_reason + usage.tokens.{input,output}
//
// Frame format: `event: <name>\ndata: <json>\n\n` (Anthropic-like
// shape, OpenAI-like keys). The `data` payload always nests the
// actual contents under `delta.message.{content|tool_calls}`.
func (p *Provider) CompleteStream(ctx context.Context, req agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	if p.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	if onChunk == nil {
		return nil, errors.New("cohere: CompleteStream requires non-nil onChunk")
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		return nil, ErrNoModel
	}

	body, err := encodeStreamRequest(model, req.System, req.Messages, req.Tools, req.MaxTokens)
	if err != nil {
		return nil, fmt.Errorf("cohere: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.resolveEndpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("cohere: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
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

	if httpResp.StatusCode/100 != 2 {
		raw, _ := httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(raw)}
	}

	return parseStream(httpResp.Body, model, onChunk)
}

// encodeStreamRequest mirrors encodeRequest but flips stream=true.
func encodeStreamRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int) ([]byte, error) {
	wire := cohereRequest{
		Model:     model,
		Stream:    true,
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

// ----- SSE parsing -----

type streamState struct {
	textParts    strings.Builder
	openTools    map[int]*openTool
	toolOrder    []int
	model        string
	finishReason string
	inputTokens  int
	outputTokens int
}

type openTool struct {
	id      string
	name    string
	argsBuf strings.Builder
}

// parseStream consumes the Cohere v2 SSE stream. Each event has the
// `event: <name>\ndata: <json>` form; we dispatch on event name.
func parseStream(body io.Reader, model string, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	st := &streamState{model: model, openTools: map[int]*openTool{}}
	var pendingEvent string

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			pendingEvent = ""
		case strings.HasPrefix(line, ":"):
			// SSE comment / keep-alive.
		case strings.HasPrefix(line, "event:"):
			pendingEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			if err := dispatchSSEFrame(pendingEvent, data, st, onChunk); err != nil {
				return nil, err
			}
			if pendingEvent == "message-end" {
				return assembleResponse(st), nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cohere: stream read: %w", err)
	}
	return assembleResponse(st), nil
}

func dispatchSSEFrame(eventName, data string, st *streamState, onChunk func(agent.Chunk) error) error {
	switch eventName {
	case "message-start":
		// id + model echoed; nothing to capture if non-empty model
		// already came from the request.

	case "content-start":
		// Opens a text content block; nothing to emit until we see
		// content-delta frames.

	case "content-delta":
		// Shape: {"delta":{"message":{"content":{"text":"frag"}}}}
		var f struct {
			Delta struct {
				Message struct {
					Content struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"message"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return nil // tolerate
		}
		text := f.Delta.Message.Content.Text
		if text != "" {
			st.textParts.WriteString(text)
			if err := onChunk(agent.Chunk{TextDelta: text}); err != nil {
				return err
			}
		}

	case "content-end":
		// Nothing to do; the message-end frame will carry the final
		// state we need.

	case "tool-plan-delta":
		// Cohere streams a tool-planning rationale before issuing
		// tool calls. We don't surface it through the Chunk
		// interface today — it doesn't fit Chunk's lifecycle and
		// callers haven't asked for it. Could be exposed as a
		// "thought" chunk type in a future revision.

	case "tool-call-start":
		// Shape: {"index":0,"delta":{"message":{"tool_calls":{"id":"call_abc","function":{"name":"shell","arguments":""}}}}}
		var f struct {
			Index int `json:"index"`
			Delta struct {
				Message struct {
					ToolCalls struct {
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return nil
		}
		tc := f.Delta.Message.ToolCalls
		id := tc.ID
		if id == "" {
			id = "call-" + strconv.Itoa(f.Index)
		}
		ot := &openTool{id: id, name: tc.Function.Name}
		if tc.Function.Arguments != "" {
			ot.argsBuf.WriteString(tc.Function.Arguments)
		}
		st.openTools[f.Index] = ot
		st.toolOrder = append(st.toolOrder, f.Index)
		start := &agent.ToolCall{
			ID:    id,
			Name:  tc.Function.Name,
			Input: json.RawMessage(`{}`),
		}
		if err := onChunk(agent.Chunk{ToolUseStart: start}); err != nil {
			return err
		}

	case "tool-call-delta":
		// Shape: {"index":0,"delta":{"message":{"tool_calls":{"function":{"arguments":"{\"q\":"}}}}}
		var f struct {
			Index int `json:"index"`
			Delta struct {
				Message struct {
					ToolCalls struct {
						Function struct {
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return nil
		}
		args := f.Delta.Message.ToolCalls.Function.Arguments
		if args == "" {
			return nil
		}
		ot, ok := st.openTools[f.Index]
		if !ok {
			return nil
		}
		ot.argsBuf.WriteString(args)
		if err := onChunk(agent.Chunk{ToolInputJSONDelta: args}); err != nil {
			return err
		}

	case "tool-call-end":
		// Shape: {"index":0}
		var f struct {
			Index int `json:"index"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return nil
		}
		ot, ok := st.openTools[f.Index]
		if !ok {
			return nil
		}
		if err := onChunk(agent.Chunk{ToolUseStop: ot.id}); err != nil {
			return err
		}

	case "message-end":
		// Shape: {"delta":{"finish_reason":"COMPLETE","usage":{"tokens":{"input_tokens":12,"output_tokens":3}}}}
		var f struct {
			Delta struct {
				FinishReason string `json:"finish_reason"`
				Usage        struct {
					Tokens struct {
						InputTokens  int `json:"input_tokens"`
						OutputTokens int `json:"output_tokens"`
					} `json:"tokens"`
				} `json:"usage"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return nil
		}
		if f.Delta.FinishReason != "" {
			st.finishReason = f.Delta.FinishReason
		}
		if f.Delta.Usage.Tokens.InputTokens > 0 {
			st.inputTokens = f.Delta.Usage.Tokens.InputTokens
		}
		if f.Delta.Usage.Tokens.OutputTokens > 0 {
			st.outputTokens = f.Delta.Usage.Tokens.OutputTokens
		}
	}
	return nil
}

func assembleResponse(st *streamState) *agent.CompletionResponse {
	var toolCalls []agent.ToolCall
	for _, idx := range st.toolOrder {
		ot := st.openTools[idx]
		args := strings.TrimSpace(ot.argsBuf.String())
		if args == "" {
			args = "{}"
		}
		toolCalls = append(toolCalls, agent.ToolCall{
			ID:    ot.id,
			Name:  ot.name,
			Input: json.RawMessage(args),
		})
	}

	stop := agent.StopEndTurn
	switch strings.ToUpper(st.finishReason) {
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

	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			Content:   st.textParts.String(),
			ToolCalls: toolCalls,
		},
		StopReason: stop,
		Usage: agent.Usage{
			InputTokens:  st.inputTokens,
			OutputTokens: st.outputTokens,
			Model:        st.model,
		},
	}
}
