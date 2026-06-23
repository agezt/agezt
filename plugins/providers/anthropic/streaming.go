// SPDX-License-Identifier: MIT

package anthropic

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
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)

// CompleteStream implements agent.StreamingProvider. It POSTs to the
// Messages endpoint with stream=true and parses the SSE stream into
// agent.Chunk callbacks. The returned CompletionResponse matches
// what Complete would return for the same request — same usage,
// stop_reason, and assembled assistant Message.
//
// SSE event types handled (per Anthropic's Messages SSE spec):
//
//	message_start         → captures Usage.InputTokens, model id
//	content_block_start   → either text block (no chunk) or tool_use
//	                        (emits Chunk.ToolUseStart)
//	content_block_delta   → text_delta → Chunk.TextDelta
//	                        input_json_delta → Chunk.ToolInputJSONDelta
//	content_block_stop    → if streaming a tool_use → Chunk.ToolUseStop
//	message_delta         → captures stop_reason + final OutputTokens
//	message_stop          → end of stream; close
//	ping                  → keep-alive; ignored
//	error                 → returns the error to the caller
func (p *Provider) CompleteStream(ctx context.Context, req agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	if p.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	if onChunk == nil {
		return nil, errors.New("anthropic: CompleteStream requires non-nil onChunk")
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		return nil, ErrNoModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}

	body, err := encodeStreamRequest(model, req.System, req.Messages, req.Tools, maxTokens, p.ThinkingBudget, req.Params, req.ProviderOptions["anthropic"])
	if err != nil {
		return nil, fmt.Errorf("anthropic: encode request: %w", err)
	}

	endpoint := p.resolveEndpoint()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", APIVersion)
	httpReq.Header.Set("x-api-key", p.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: http: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode/100 != 2 {
		// Non-2xx — read body for the journal, surface the same
		// APIError type Complete uses so caller code paths converge.
		raw, _ := httpread.All(httpResp.Body, httpread.DefaultMaxResponseBytes)
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(raw)}
	}

	resp, err := parseStream(httpResp.Body, onChunk)
	if err != nil {
		return nil, err
	}
	// Reverse the request-side tool-name conformance on the final response so the
	// call routes to the real tool (the live chunks carry the wire name, which is
	// display-only; dispatch uses this assembled response).
	toolname.RestoreCalls(resp, toolname.Reverse(req.Tools))
	return resp, nil
}

// encodeStreamRequest mirrors encodeRequest but adds "stream": true.
// Kept separate (rather than adding a bool param to encodeRequest) so
// the non-streaming wire format stays byte-identical to what M0.5
// tests verified.
func encodeStreamRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef, maxTok, thinkingBudget int, params agent.Params, extra json.RawMessage) ([]byte, error) {
	// Same shape as anthRequest plus the Stream field.
	type streamReq struct {
		Model     string        `json:"model"`
		MaxTokens int           `json:"max_tokens"`
		System    any           `json:"system,omitempty"`
		Messages  []anthMessage `json:"messages"`
		Tools     []anthTool    `json:"tools,omitempty"`
		Stream    bool          `json:"stream"`
		Thinking  *anthThinking `json:"thinking,omitempty"` // M318
		// Per-request sampling knobs (M997).
		Temperature   *float64 `json:"temperature,omitempty"`
		TopP          *float64 `json:"top_p,omitempty"`
		TopK          *int     `json:"top_k,omitempty"`
		StopSequences []string `json:"stop_sequences,omitempty"`
	}
	if b, ok := provopts.ThinkingBudget(params.ReasoningEffort, maxTok); ok {
		thinkingBudget = b
	}
	thinking, maxTok := thinkingConfig(thinkingBudget, maxTok)
	fwd, _ := toolname.Maps(tools)
	wire := streamReq{Model: model, MaxTokens: maxTok, System: buildAnthSystem(system), Stream: true, Tools: buildAnthTools(tools, fwd), Thinking: thinking}
	if !params.IsZero() {
		wire.Temperature = params.Temperature
		wire.TopP = params.TopP
		wire.TopK = params.TopK
		wire.StopSequences = params.Stop
	}
	for _, m := range msgs {
		am, err := canonicalToAnth(m, fwd)
		if err != nil {
			return nil, err
		}
		if am == nil {
			continue
		}
		wire.Messages = append(wire.Messages, *am)
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	return provopts.Merge(body, extra)
}

// ----- SSE parsing -----

// streamState accumulates everything we need to assemble the final
// CompletionResponse as SSE frames arrive.
type streamState struct {
	textParts      strings.Builder
	reasoningParts strings.Builder // extended thinking (M318)
	openBlock      *openBlock      // currently-streaming block, if any
	finishedTools  []agent.ToolCall
	inputTokens    int
	cacheRead      int // cache_read_input_tokens (M290)
	cacheCreation  int // cache_creation_input_tokens (M290)
	outputTokens   int
	stopReason     string
	model          string
}

// openBlock tracks a content block while its deltas stream in. For
// text blocks textBuf collects the delta strings; for tool_use the
// inputBuf collects the streamed JSON fragments.
type openBlock struct {
	kind     string // "text" or "tool_use"
	toolID   string
	toolName string
	textBuf  strings.Builder
	inputBuf strings.Builder
}

// parseStream consumes an SSE response body until message_stop or
// EOF. Per-event SSE format:
//
//	event: <name>\n
//	data:  <json>\n
//	\n
//
// We pair each event line with the immediately-following data line
// and dispatch by event name. Lines outside that pattern (comments,
// retry directives) are ignored — Anthropic doesn't use them today
// but the parser must tolerate them to stay forward-compatible.
func parseStream(body io.Reader, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	scanner := bufio.NewScanner(body)
	// Anthropic content_block_delta frames for large inputs can exceed
	// the default 64K bufio limit. Bump to 1MB; a single SSE frame
	// larger than that signals an upstream pathology, not normal use.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	st := streamState{}
	var pendingEvent string

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			// Blank line terminates an event; nothing to do here as
			// we dispatch on the data line.
			pendingEvent = ""
		case strings.HasPrefix(line, ":"):
			// SSE comment / keep-alive. Ignore.
		case strings.HasPrefix(line, "event:"):
			pendingEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			if err := dispatchSSEFrame(pendingEvent, data, &st, onChunk); err != nil {
				return nil, err
			}
			if pendingEvent == "message_stop" {
				// End of stream — done.
				return assembleResponse(&st), nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("anthropic: stream read: %w", err)
	}
	// EOF without message_stop is technically malformed, but assemble
	// what we have so callers see partial output rather than a hard
	// error swallowing already-streamed tokens.
	return assembleResponse(&st), nil
}

// dispatchSSEFrame mutates streamState and fires onChunk based on the
// SSE event name. Unknown event names are ignored (forward-compat).
func dispatchSSEFrame(eventName, data string, st *streamState, onChunk func(agent.Chunk) error) error {
	switch eventName {
	case "message_start":
		var f struct {
			Message struct {
				Model string `json:"model"`
				Usage struct {
					InputTokens              int `json:"input_tokens"`
					OutputTokens             int `json:"output_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			// Tolerate a malformed structural frame: skip it and keep the stream
			// going rather than aborting and discarding already-streamed tokens
			// (matches the other providers and this parser's own EOF handling,
			// lines ~199-202). A real provider "error" event still propagates. (M451)
			return nil
		}
		st.model = f.Message.Model
		st.inputTokens = f.Message.Usage.InputTokens
		st.cacheRead = f.Message.Usage.CacheReadInputTokens
		st.cacheCreation = f.Message.Usage.CacheCreationInputTokens
		// Some streams report partial output tokens here too.
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
			return nil // tolerate a malformed frame — skip, don't abort the stream (M451)
		}
		st.openBlock = &openBlock{kind: f.ContentBlock.Type}
		switch f.ContentBlock.Type {
		case "text":
			// Anthropic may include initial text; usually empty. Track
			// in the open block so message-level concatenation works.
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
				Thinking    string `json:"thinking"` // thinking_delta (M318)
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return nil // tolerate a malformed frame — skip, don't abort the stream (M451)
		}
		if st.openBlock == nil {
			// Delta without a preceding _start — ignore rather than
			// error so a malformed prefix doesn't drop the rest.
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
		case "thinking_delta": // extended thinking (M318)
			st.openBlock.textBuf.WriteString(f.Delta.Thinking)
			if f.Delta.Thinking != "" {
				if err := onChunk(agent.Chunk{ReasoningDelta: f.Delta.Thinking}); err != nil {
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
		case "thinking": // extended thinking (M318): textBuf held the thinking deltas
			st.reasoningParts.WriteString(ob.textBuf.String())
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
			return nil // tolerate a malformed frame — skip, don't abort the stream (M451)
		}
		if f.Delta.StopReason != "" {
			st.stopReason = f.Delta.StopReason
		}
		if f.Usage.OutputTokens > 0 {
			st.outputTokens = f.Usage.OutputTokens
		}

	case "message_stop":
		// Nothing to read out of the payload; the caller checks the
		// event name and stops the loop.

	case "ping", "":
		// keep-alives + unparented data lines: ignore.

	case "error":
		var f struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &f); err != nil {
			return fmt.Errorf("anthropic: stream error frame (unparseable): %s", data)
		}
		return fmt.Errorf("anthropic: stream error (%s): %s", f.Error.Type, f.Error.Message)
	}
	return nil
}

// assembleResponse converts the accumulated streamState into the same
// CompletionResponse shape Complete returns. Done once per stream
// when message_stop arrives (or on EOF).
func assembleResponse(st *streamState) *agent.CompletionResponse {
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
		ReasoningContent: st.reasoningParts.String(), // M318
		StopReason:       stop,
		Usage: anthUsageToAgent(
			st.inputTokens, st.cacheRead, st.cacheCreation, st.outputTokens, st.model),
	}
}
