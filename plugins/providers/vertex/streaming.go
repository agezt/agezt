// SPDX-License-Identifier: MIT

package vertex

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
// Vertex `:streamGenerateContent?alt=sse` endpoint with the same
// body Complete uses, parses the SSE stream into agent.Chunk
// callbacks, and returns the same CompletionResponse shape Complete
// would.
//
// Wire shape is identical to plugins/providers/google's streaming
// path — Gemini body, untagged SSE, no `[DONE]` sentinel,
// body-close terminus. The only differences from the Generative
// Language API:
//
//   - Endpoint is regional: `{region}-aiplatform.googleapis.com`,
//     with full project/location/publisher path.
//   - Auth is OAuth Bearer (service-account JWT exchange), not API
//     key. Same TokenSource the non-streaming path uses.
//
// Parser logic is duplicated from plugins/providers/google by
// design — the package-level comment on vertex.go explains: Vertex
// evolves independently and the duplication is contained.
func (p *Provider) CompleteStream(ctx context.Context, req agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	if p.TokenSource == nil {
		return nil, ErrNoTokenSource
	}
	if onChunk == nil {
		return nil, errors.New("vertex: CompleteStream requires non-nil onChunk")
	}
	if p.Project == "" && p.Endpoint == "" {
		return nil, errors.New("vertex: Project required (or set Endpoint directly)")
	}
	if p.Location == "" && p.Endpoint == "" {
		return nil, errors.New("vertex: Location required (or set Endpoint directly)")
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		return nil, ErrNoModel
	}

	// Anthropic-on-Vertex (`claude-*` model ids) — see
	// completeAnthropic for rationale. M1.n.x.
	if isAnthropicModel(model) {
		return p.completeStreamAnthropic(ctx, req, model, onChunk)
	}

	body, err := encodeRequest(req.System, req.Messages, req.Tools, req.MaxTokens, req.JSONMode, p.ThinkingBudget)
	if err != nil {
		return nil, fmt.Errorf("vertex: encode request: %w", err)
	}

	tok, err := p.TokenSource.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("vertex: get access token: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ResolveStreamEndpoint(model), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("vertex: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
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

	if httpResp.StatusCode/100 != 2 {
		raw, _ := httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(raw)}
	}

	return parseStream(httpResp.Body, model, onChunk)
}

// ResolveStreamEndpoint mirrors ResolveEndpoint but targets
// `:streamGenerateContent` and appends `?alt=sse`. Exported so tests
// (and future operators verifying their VPC service-control config)
// can predict the URL without an actual HTTP roundtrip.
func (p *Provider) ResolveStreamEndpoint(model string) string {
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
		"/publishers/google/models/" + model + ":streamGenerateContent?alt=sse"
}

// ----- SSE parsing -----

// streamState accumulates everything needed for the final response.
// Shape matches plugins/providers/google's streamState exactly;
// only the underlying frame types (vx* vs gemini*) differ.
type streamState struct {
	textParts      strings.Builder
	reasoningParts strings.Builder // M320: thought-summary parts
	toolCalls      []agent.ToolCall
	finishReason   string
	model          string
	inputTokens    int
	cachedTokens   int // cachedContentTokenCount (M294-cache)
	outputTokens   int
	thoughtsTokens int // thoughtsTokenCount (M320), billed as output
}

// parseStream consumes the SSE response body until EOF. Each
// `data:` line is a complete partial vxResponse JSON.
func parseStream(body io.Reader, model string, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	scanner := bufio.NewScanner(body)
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
		var frame vxResponse
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			// Tolerate malformed keepalive frames (same convention as
			// the OpenAI and Google streaming parsers).
			continue
		}
		if frame.UsageMetadata != nil {
			st.inputTokens = frame.UsageMetadata.PromptTokenCount
			st.cachedTokens = frame.UsageMetadata.CachedContentTokenCount
			st.outputTokens = frame.UsageMetadata.CandidatesTokenCount
			st.thoughtsTokens = frame.UsageMetadata.ThoughtsTokenCount
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
			case part.Thought && part.Text != "":
				// Thought-summary delta (M320): reasoning, surfaced on
				// ReasoningDelta and kept out of the answer text.
				st.reasoningParts.WriteString(part.Text)
				if err := onChunk(agent.Chunk{ReasoningDelta: part.Text}); err != nil {
					return nil, err
				}
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
				// Full args in one chunk (Vertex inherits Gemini's
				// non-streaming-tool-input behavior). Synthesize the
				// full ToolUseStart→Delta→Stop lifecycle so callers
				// don't need provider-specific code.
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
		return nil, fmt.Errorf("vertex: stream read: %w", err)
	}
	return assembleResponse(st), nil
}

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
			InputTokens:       st.inputTokens,
			CachedInputTokens: st.cachedTokens,
			OutputTokens:      st.outputTokens + st.thoughtsTokens, // thinking billed as output (M320)
			Model:             st.model,
		},
		ReasoningContent: st.reasoningParts.String(),
	}
}
