// SPDX-License-Identifier: MIT

package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
	"github.com/agezt/agezt/plugins/providers/internal/retry"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
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
		return nil, ErrNoModel
	}

	body, err := encodeStreamRequest(model, req.System, req.Messages, req.Tools, req.MaxTokens, req.JSONMode, req.Params, req.ProviderOptions["openai"])
	if err != nil {
		return nil, fmt.Errorf("openai: encode request: %w", err)
	}

	endpoint := p.resolveEndpoint()
	// Stream SETUP retries transient failures (connection errors, 429/5xx)
	// before the first frame; mid-stream failures are never replayed (LD-4).
	httpResp, err := retry.DoHTTPStream(ctx, p.HTTP, func() (*http.Request, error) {
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
		return httpReq, nil
	}, httpread.DefaultMaxResponseBytes)
	if err != nil {
		var h *retry.HTTPError
		if errors.As(err, &h) {
			return nil, &APIError{Status: h.StatusCode, Body: h.Body}
		}
		return nil, fmt.Errorf("openai: http: %w", err)
	}
	defer httpResp.Body.Close()

	resp, err := parseStream(httpResp.Body, onChunk)
	if err != nil {
		return nil, err
	}
	toolname.RestoreCalls(resp, toolname.Reverse(req.Tools))
	return resp, nil
}

// encodeStreamRequest mirrors encodeRequest but flips stream=true
// and adds stream_options.include_usage=true. Kept separate so the
// non-streaming wire format stays byte-identical to what existing
// tests verified.
func encodeStreamRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int, jsonMode bool, params agent.Params, extra json.RawMessage) ([]byte, error) {
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
		oaParams
	}
	wire := streamReq{
		Model:          model,
		Stream:         true,
		MaxTokens:      maxTok,
		StreamOptions:  &streamOptions{IncludeUsage: true},
		ResponseFormat: jsonObjectFormat(jsonMode),
	}
	wire.applyParams(params)
	fwd, _ := toolname.Maps(tools)
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
