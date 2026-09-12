// SPDX-License-Identifier: MIT

// Bedrock streaming: CompleteStream + state types + parseEventStream + assembleBedrockResponse.
// Code extracted from streaming.go during the Day-108 god-file split.
// Public API unchanged.
package bedrock


import (
	"errors"
	"fmt"
	"io"
	"strings"

	"encoding/base64"
	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
)

type bedStreamState struct {
	textParts     strings.Builder
	openBlock     *bedOpenBlock
	finishedTools []agent.ToolCall
	inputTokens   int
	cacheRead     int // cache_read_input_tokens (M296)
	cacheCreation int // cache_creation_input_tokens (M296)
	outputTokens  int
	stopReason    string
}

type bedOpenBlock struct {
	kind     string
	toolID   string
	toolName string
	textBuf  strings.Builder
	inputBuf strings.Builder
}

// chunkPayload is the JSON Bedrock emits as the payload of a "chunk"
// event. The bytes field is base64 of the actual Anthropic event JSON.
type chunkPayload struct {
	Bytes string `json:"bytes"`
}

// parseEventStream is the per-stream loop: read a frame, branch on
// :message-type, decode the inner Anthropic event, dispatch.
func parseEventStream(body io.Reader, model string, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	st := bedStreamState{}
	for {
		hdrs, payload, err := readEventStreamMessage(body)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("bedrock: %w", err)
		}

		msgType := headerValue(hdrs, ":message-type")
		switch msgType {
		case "event":
			// Normal data — wrap-decode the inner Anthropic event.
			var cp chunkPayload
			if err := json.Unmarshal(payload, &cp); err != nil {
				return nil, fmt.Errorf("bedrock: parse chunk envelope: %w", err)
			}
			inner, err := base64.StdEncoding.DecodeString(cp.Bytes)
			if err != nil {
				return nil, fmt.Errorf("bedrock: decode chunk bytes: %w", err)
			}
			stop, err := dispatchBedrockInnerEvent(inner, &st, onChunk)
			if err != nil {
				return nil, err
			}
			if stop {
				return assembleBedrockResponse(&st, model), nil
			}

		case "exception":
			// Modelled exception (validation, throttling). The
			// :exception-type header names the class; payload is JSON.
			excType := headerValue(hdrs, ":exception-type")
			return nil, fmt.Errorf("bedrock: stream exception (%s): %s", excType, string(payload))

		case "error":
			// AWS-internal error. Surface raw.
			errCode := headerValue(hdrs, ":error-code")
			errMsg := headerValue(hdrs, ":error-message")
			return nil, fmt.Errorf("bedrock: stream error (%s): %s", errCode, errMsg)

		default:
			// Unknown message-type: skip rather than fail (forward-compat).
		}
	}
	return assembleBedrockResponse(&st, model), nil
}

// dispatchBedrockInnerEvent handles the JSON Anthropic emits inside
// each "chunk" frame. Returns (stop, err) — stop=true when the
// inner event is message_stop and the outer loop should exit.
func dispatchBedrockInnerEvent(data []byte, st *bedStreamState, onChunk func(agent.Chunk) error) (bool, error) {
	// The inner event always has a "type" field discriminating the
	// payload shape. Peek it first.
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return false, fmt.Errorf("bedrock: parse inner event type: %w", err)
	}
	switch head.Type {
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
		if err := json.Unmarshal(data, &f); err != nil {
			return false, fmt.Errorf("bedrock: parse message_start: %w", err)
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
		if err := json.Unmarshal(data, &f); err != nil {
			return false, fmt.Errorf("bedrock: parse content_block_start: %w", err)
		}
		st.openBlock = &bedOpenBlock{kind: f.ContentBlock.Type}
		switch f.ContentBlock.Type {
		case "text":
			st.openBlock.textBuf.WriteString(f.ContentBlock.Text)
			if f.ContentBlock.Text != "" {
				if err := onChunk(agent.Chunk{TextDelta: f.ContentBlock.Text}); err != nil {
					return false, err
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
				return false, err
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
		if err := json.Unmarshal(data, &f); err != nil {
			return false, fmt.Errorf("bedrock: parse content_block_delta: %w", err)
		}
		if st.openBlock == nil {
			return false, nil
		}
		switch f.Delta.Type {
		case "text_delta":
			st.openBlock.textBuf.WriteString(f.Delta.Text)
			if f.Delta.Text != "" {
				if err := onChunk(agent.Chunk{TextDelta: f.Delta.Text}); err != nil {
					return false, err
				}
			}
		case "input_json_delta":
			st.openBlock.inputBuf.WriteString(f.Delta.PartialJSON)
			if f.Delta.PartialJSON != "" {
				if err := onChunk(agent.Chunk{ToolInputJSONDelta: f.Delta.PartialJSON}); err != nil {
					return false, err
				}
			}
		}

	case "content_block_stop":
		if st.openBlock == nil {
			return false, nil
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
				return false, err
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
		if err := json.Unmarshal(data, &f); err != nil {
			return false, fmt.Errorf("bedrock: parse message_delta: %w", err)
		}
		if f.Delta.StopReason != "" {
			st.stopReason = f.Delta.StopReason
		}
		if f.Usage.OutputTokens > 0 {
			st.outputTokens = f.Usage.OutputTokens
		}

	case "message_stop":
		return true, nil

	case "ping", "":
		// keep-alive; ignore.

	case "error":
		var f struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &f); err != nil {
			return false, fmt.Errorf("bedrock: inner error frame (unparseable): %s", string(data))
		}
		return false, fmt.Errorf("bedrock: stream error (%s): %s", f.Error.Type, f.Error.Message)
	}
	return false, nil
}

func assembleBedrockResponse(st *bedStreamState, model string) *agent.CompletionResponse {
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
		Usage: anthBedrockUsageToAgent(
			st.inputTokens, st.cacheRead, st.cacheCreation, st.outputTokens, model),
	}
}
