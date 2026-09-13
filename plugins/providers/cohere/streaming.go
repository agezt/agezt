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
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
	"github.com/agezt/agezt/plugins/providers/internal/retry"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
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

	body, err := encodeStreamRequest(model, req.System, req.Messages, req.Tools, req.MaxTokens, req.Params, req.ProviderOptions["cohere"])
	if err != nil {
		return nil, fmt.Errorf("cohere: encode request: %w", err)
	}

	// Stream SETUP retries transient failures (connection errors, 429/5xx)
	// before the first frame; mid-stream failures are never replayed (LD-4).
	httpResp, err := retry.DoHTTPStream(ctx, p.HTTP, func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.resolveEndpoint(), bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("cohere: build request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
		return httpReq, nil
	}, httpread.DefaultMaxResponseBytes)
	if err != nil {
		var h *retry.HTTPError
		if errors.As(err, &h) {
			return nil, &APIError{Status: h.StatusCode, Body: h.Body}
		}
		return nil, fmt.Errorf("cohere: http: %w", err)
	}
	defer httpResp.Body.Close()

	resp, err := parseStream(httpResp.Body, model, onChunk)
	if err != nil {
		return nil, err
	}
	toolname.RestoreCalls(resp, toolname.Reverse(req.Tools))
	return resp, nil
}

// encodeStreamRequest mirrors encodeRequest but flips stream=true.
func encodeStreamRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int, params agent.Params, extra json.RawMessage) ([]byte, error) {
	fwd, _ := toolname.Maps(tools)
	wire := cohereRequest{
		Model:     model,
		Stream:    true,
		MaxTokens: maxTok,
	}
	wire.applyParams(params)
	if s := strings.TrimSpace(system); s != "" {
		wire.Messages = append(wire.Messages, cohereMessage{Role: "system", Content: s})
	}
	for _, m := range msgs {
		cm, err := canonicalToCohere(m, fwd)
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
				Name:        toolname.Wire(fwd, t.Name),
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	return provopts.Merge(body, extra)
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

