// SPDX-License-Identifier: MIT

package ollama

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

// CompleteStream implements agent.StreamingProvider for Ollama.
// Unlike Anthropic / OpenAI / Google, Ollama doesn't use SSE — its
// streaming format is JSON-lines (NDJSON): one complete JSON object
// per `\n`-delimited line, no `data:` prefix, no event tags. The
// final line has `done: true` and carries the usage counters.
//
// Tool calls in Ollama's streaming arrive whole (the entire
// `tool_calls` array in one chunk, same convention as Gemini), so
// we synthesize the full ToolUseStart → ToolInputJSONDelta →
// ToolUseStop lifecycle to keep the chunk contract consistent
// across providers.
func (p *Provider) CompleteStream(ctx context.Context, req agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	endpoint := p.resolveEndpoint()
	if endpoint == "" {
		return nil, ErrNoEndpoint
	}
	if onChunk == nil {
		return nil, errors.New("ollama: CompleteStream requires non-nil onChunk")
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		model = DefaultModel
	}

	body, err := encodeStreamRequest(model, req.System, req.Messages, req.Tools)
	if err != nil {
		return nil, fmt.Errorf("ollama: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Ollama doesn't require an Accept header for its NDJSON stream,
	// but setting it makes the protocol intent explicit in logs/proxies.
	httpReq.Header.Set("Accept", "application/x-ndjson")

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: http: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode/100 != 2 {
		raw, _ := httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(raw)}
	}

	return parseStream(httpResp.Body, model, onChunk)
}

// encodeStreamRequest mirrors encodeRequest but flips stream=true.
// Kept separate so the non-streaming wire format stays byte-identical
// to what existing tests verified.
func encodeStreamRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef) ([]byte, error) {
	out := ollamaRequest{
		Model:  model,
		Stream: true,
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

// ----- NDJSON parsing -----

type streamState struct {
	textParts    strings.Builder
	toolCalls    []agent.ToolCall
	model        string
	doneReason   string
	inputTokens  int
	outputTokens int
}

// parseStream consumes the NDJSON response body line-by-line until
// EOF or a chunk with `done: true`. Empty lines are skipped (some
// proxies introduce them).
func parseStream(body io.Reader, model string, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	scanner := bufio.NewScanner(body)
	// Large tool inputs can blow the default 64K line limit, same as
	// the SSE parsers in the other adapters.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	st := &streamState{model: model}
	toolIdx := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var frame ollamaResponse
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			// Same forgiving stance as the other parsers — a single bad
			// frame shouldn't kill the stream.
			continue
		}
		if frame.Model != "" {
			st.model = frame.Model
		}

		// Text fragment in this chunk's message.
		if frame.Message.Content != "" {
			st.textParts.WriteString(frame.Message.Content)
			if err := onChunk(agent.Chunk{TextDelta: frame.Message.Content}); err != nil {
				return nil, err
			}
		}

		// Tool calls in stream — arrive whole. Synthesize the lifecycle.
		for _, tc := range frame.Message.ToolCalls {
			callID := tc.ID
			if callID == "" {
				callID = "call-" + strconv.Itoa(toolIdx)
			}
			toolIdx++
			args := tc.Function.Arguments
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			start := &agent.ToolCall{
				ID:    callID,
				Name:  tc.Function.Name,
				Input: args,
			}
			if err := onChunk(agent.Chunk{ToolUseStart: start}); err != nil {
				return nil, err
			}
			if err := onChunk(agent.Chunk{ToolInputJSONDelta: string(args)}); err != nil {
				return nil, err
			}
			if err := onChunk(agent.Chunk{ToolUseStop: callID}); err != nil {
				return nil, err
			}
			st.toolCalls = append(st.toolCalls, agent.ToolCall{
				ID:    callID,
				Name:  tc.Function.Name,
				Input: args,
			})
		}

		if frame.Done {
			if frame.DoneReason != "" {
				st.doneReason = frame.DoneReason
			}
			if frame.PromptEvalCount > 0 {
				st.inputTokens = frame.PromptEvalCount
			}
			if frame.EvalCount > 0 {
				st.outputTokens = frame.EvalCount
			}
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("ollama: stream read: %w", err)
	}
	return assembleResponse(st), nil
}

func assembleResponse(st *streamState) *agent.CompletionResponse {
	var stop agent.StopReason
	switch {
	case len(st.toolCalls) > 0:
		stop = agent.StopToolUse
	case st.doneReason == "length":
		stop = agent.StopMaxTokens
	default:
		stop = agent.StopEndTurn
	}
	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			Content:   st.textParts.String(),
			ToolCalls: st.toolCalls,
		},
		StopReason: stop,
		Usage: agent.Usage{
			InputTokens:  st.inputTokens,
			OutputTokens: st.outputTokens,
			Model:        st.model,
		},
	}
}
