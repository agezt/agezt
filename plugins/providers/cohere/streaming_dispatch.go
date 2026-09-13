// SPDX-License-Identifier: MIT

package cohere

// Cohere SSE frame dispatcher: dispatchSSEFrame. Carved out of
// streaming.go during the Day 199 god-file split so the main file
// can stay focused on CompleteStream + encodeStreamRequest +
// streamState/openTool types + parseStream + assembleResponse.
// Public API unchanged.

import (
	"encoding/json"
	"strconv"

	"github.com/agezt/agezt/kernel/agent"
)

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
