// SPDX-License-Identifier: MIT

package google

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

	"github.com/ersinkoc/agezt/kernel/agent"
)

// CompleteStream implements agent.StreamingProvider. It POSTs to the
// `:streamGenerateContent?alt=sse` endpoint and parses the SSE stream
// into agent.Chunk callbacks. The returned CompletionResponse matches
// what Complete would return for the same request.
//
// Wire shape notes (different from both Anthropic and OpenAI):
//
//   - URL ends in `:streamGenerateContent` instead of `:generateContent`,
//     and carries `?alt=sse` to get SSE framing (without it, Gemini
//     returns a chunked-JSON array which is harder to parse incrementally).
//   - SSE frames are plain `data: {json}\n\n` with no `event:` lines
//     (same as OpenAI).
//   - No `[DONE]` sentinel — the stream ends when the HTTP body closes.
//   - Each JSON is a partial `geminiResponse` (same shape as Complete's
//     final response). Text deltas live in `candidates[0].content.parts[i].text`;
//     tool calls arrive *whole* in `parts[i].functionCall` (Gemini doesn't
//     stream tool arguments as fragments — the entire functionCall lands
//     in one chunk).
//   - `usageMetadata` and `finishReason` appear in the terminal chunk.
//
// Because Gemini delivers tool calls whole, the agent.Chunk lifecycle
// for a tool call is: emit ToolUseStart → emit a single
// ToolInputJSONDelta carrying the full marshaled args → emit
// ToolUseStop. Callers that synthesize per-tool UI from chunks get a
// consistent start→deltas→stop story regardless of whether the
// upstream provider truly streamed the input.
func (p *Provider) CompleteStream(ctx context.Context, req agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	if p.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	if onChunk == nil {
		return nil, errors.New("google: CompleteStream requires non-nil onChunk")
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		model = DefaultModel
	}

	body, err := encodeRequest(req.System, req.Messages, req.Tools, req.MaxTokens)
	if err != nil {
		return nil, fmt.Errorf("google: encode request: %w", err)
	}

	endpoint := p.resolveStreamEndpoint(model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("google: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("x-goog-api-key", p.APIKey)

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("google: http: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(httpResp.Body)
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(raw)}
	}

	return parseStream(httpResp.Body, model, onChunk)
}

// resolveStreamEndpoint mirrors resolveEndpoint but targets
// `:streamGenerateContent` and appends `?alt=sse`. Kept separate so
// the non-streaming endpoint URL stays exactly what existing tests
// verified.
func (p *Provider) resolveStreamEndpoint(model string) string {
	if p.Endpoint != "" {
		// When tests pin Endpoint directly, honor it as-is — they're
		// pointing at a mock server, not the real Gemini URL builder.
		return p.Endpoint
	}
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	prefix := base
	if !strings.HasSuffix(base, "/"+APIVersion) && !strings.Contains(base, "/"+APIVersion+"/") &&
		!strings.HasSuffix(base, "/v1") && !strings.Contains(base, "/v1/") {
		prefix = base + "/" + APIVersion
	}
	return prefix + "/models/" + model + ":streamGenerateContent?alt=sse"
}

// ----- SSE parsing -----

// streamState accumulates everything needed for the final response.
type streamState struct {
	textParts    strings.Builder
	toolCalls    []agent.ToolCall
	finishReason string
	model        string
	inputTokens  int
	outputTokens int
}

// parseStream consumes the SSE response body until EOF. Each
// `data:` line is a complete partial geminiResponse JSON.
func parseStream(body io.Reader, model string, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	scanner := bufio.NewScanner(body)
	// Tool arguments + system instructions can produce large frames;
	// bump from default 64K to 1MB to match the Anthropic / OpenAI
	// streaming parsers.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	st := &streamState{model: model}
	toolIdx := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var frame geminiResponse
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			// Tolerate malformed keepalive frames the way the OpenAI
			// parser does — some proxies inject them.
			continue
		}
		if frame.UsageMetadata != nil {
			st.inputTokens = frame.UsageMetadata.PromptTokenCount
			st.outputTokens = frame.UsageMetadata.CandidatesTokenCount
		}
		if len(frame.Candidates) == 0 {
			continue
		}
		cand := frame.Candidates[0]
		if cand.FinishReason != "" {
			st.finishReason = cand.FinishReason
		}
		for _, part := range cand.Content.Parts {
			switch {
			case part.Text != "":
				st.textParts.WriteString(part.Text)
				if err := onChunk(agent.Chunk{TextDelta: part.Text}); err != nil {
					return nil, err
				}
			case part.FunctionCall != nil:
				args := part.FunctionCall.Args
				if len(args) == 0 {
					args = json.RawMessage(`{}`)
				}
				// Gemini doesn't emit per-call IDs; synthesize stable
				// ones (same convention as the non-streaming path).
				callID := "call-" + strconv.Itoa(toolIdx)
				toolIdx++
				start := &agent.ToolCall{
					ID:    callID,
					Name:  part.FunctionCall.Name,
					Input: args,
				}
				if err := onChunk(agent.Chunk{ToolUseStart: start}); err != nil {
					return nil, err
				}
				// The full args arrive in one chunk, so we emit them as
				// a single ToolInputJSONDelta — keeps the chunk
				// lifecycle consistent with adapters that genuinely
				// stream (Anthropic/OpenAI).
				if err := onChunk(agent.Chunk{ToolInputJSONDelta: string(args)}); err != nil {
					return nil, err
				}
				if err := onChunk(agent.Chunk{ToolUseStop: callID}); err != nil {
					return nil, err
				}
				st.toolCalls = append(st.toolCalls, agent.ToolCall{
					ID:    callID,
					Name:  part.FunctionCall.Name,
					Input: args,
				})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("google: stream read: %w", err)
	}
	return assembleResponse(st), nil
}

// assembleResponse converts the accumulated streamState into the same
// CompletionResponse shape Complete returns.
func assembleResponse(st *streamState) *agent.CompletionResponse {
	var stop agent.StopReason
	switch {
	case len(st.toolCalls) > 0:
		stop = agent.StopToolUse
	default:
		switch st.finishReason {
		case "STOP", "":
			stop = agent.StopEndTurn
		case "MAX_TOKENS":
			stop = agent.StopMaxTokens
		default:
			stop = agent.StopEndTurn
		}
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
