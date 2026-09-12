// SPDX-License-Identifier: MIT

// Anthropic streaming: dispatchSSEFrame (per-frame inner-event dispatcher) + assembleResponse.
// Code extracted from streaming.go during the Day-135 god-file split.
// Public API unchanged.
package anthropic


import (
	"fmt"
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
)

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
