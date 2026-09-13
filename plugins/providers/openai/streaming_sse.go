// SPDX-License-Identifier: MIT

package openai

// OpenAI streaming SSE parser: parseStream + dispatchSSEFrame +
// assembleResponse. Carved out of streaming.go during the Day 148 god-file
// split so the entry file can focus on CompleteStream + request encoding.
// Public API unchanged.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)
func parseStream(body io.Reader, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	scanner := bufio.NewScanner(body)
	// Tool input JSON can be large; bump from default 64K to 1MB.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	st := &streamState{tools: map[int]*openTool{}}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			return assembleResponse(st), nil
		}
		if err := dispatchSSEFrame(data, st, onChunk); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("openai: stream read: %w", err)
	}
	// EOF without [DONE] — assemble what we have rather than throwing
	// away already-streamed tokens.
	return assembleResponse(st), nil
}

// dispatchSSEFrame parses one JSON chunk and updates state.
func dispatchSSEFrame(data string, st *streamState, onChunk func(agent.Chunk) error) error {
	var f struct {
		Model   string `json:"model"`
		Choices []struct {
			Index int `json:"index"`
			Delta struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"` // DeepSeek-R1 (M317)
				Reasoning        string `json:"reasoning"`         // other compat gateways
				ToolCalls        []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			// DeepSeek's spelling of the cache-read count (M887); see oaResponse.
			PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &f); err != nil {
		// Don't fail the whole stream on one unparseable frame —
		// OpenAI proxies (and some openai-compatible vendors) have
		// been seen to inject keepalive comments as malformed JSON.
		return nil
	}
	if f.Model != "" {
		st.model = f.Model
	}
	if f.Usage != nil {
		st.inputTokens = f.Usage.PromptTokens
		st.outputTokens = f.Usage.CompletionTokens
		st.cachedTokens = cachedInputTokens(f.Usage.PromptTokensDetails.CachedTokens, f.Usage.PromptCacheHitTokens)
	}

	if len(f.Choices) == 0 {
		return nil
	}
	choice := f.Choices[0]
	if choice.FinishReason != "" {
		st.finishReason = choice.FinishReason
	}

	// Reasoning delta (M317): DeepSeek-R1 streams its chain of thought in a
	// separate reasoning_content field before the answer tokens.
	if rd := choice.Delta.ReasoningContent; rd != "" {
		st.reasoningParts.WriteString(rd)
		if err := onChunk(agent.Chunk{ReasoningDelta: rd}); err != nil {
			return err
		}
	} else if rd := choice.Delta.Reasoning; rd != "" {
		st.reasoningParts.WriteString(rd)
		if err := onChunk(agent.Chunk{ReasoningDelta: rd}); err != nil {
			return err
		}
	}

	if choice.Delta.Content != "" {
		st.textParts.WriteString(choice.Delta.Content)
		if err := onChunk(agent.Chunk{TextDelta: choice.Delta.Content}); err != nil {
			return err
		}
	}

	for _, tcd := range choice.Delta.ToolCalls {
		idx := tcd.Index
		tool, exists := st.tools[idx]
		if !exists {
			tool = &openTool{}
			st.tools[idx] = tool
			st.toolOrder = append(st.toolOrder, idx)
		}
		// First chunk for a given index carries id + function.name; we
		// only adopt them if not already set so a later chunk's empty
		// strings don't clobber them.
		if tool.id == "" && tcd.ID != "" {
			tool.id = tcd.ID
		}
		if tool.name == "" && tcd.Function.Name != "" {
			tool.name = tcd.Function.Name
			// Emit ToolUseStart only once per index, on the first
			// chunk where we know the name.
			start := &agent.ToolCall{
				ID:    tool.id,
				Name:  tool.name,
				Input: json.RawMessage(`{}`),
			}
			if start.ID == "" {
				// Some openai-compatible vendors omit the id; synthesize
				// a deterministic one so the loop's tool-result message
				// can still reference it.
				start.ID = "call-" + strconv.Itoa(idx)
				tool.id = start.ID
			}
			if err := onChunk(agent.Chunk{ToolUseStart: start}); err != nil {
				return err
			}
		}
		if tcd.Function.Arguments != "" {
			tool.argsBuf.WriteString(tcd.Function.Arguments)
			if err := onChunk(agent.Chunk{ToolInputJSONDelta: tcd.Function.Arguments}); err != nil {
				return err
			}
		}
	}

	// Tool stop signal: emit a ToolUseStop for each open tool once
	// finish_reason arrives. OpenAI doesn't have an explicit
	// per-tool-stop frame; we synthesize on the terminal frame so
	// callers get a clean lifecycle.
	if choice.FinishReason != "" {
		for _, idx := range st.toolOrder {
			tool := st.tools[idx]
			id := tool.id
			if id == "" {
				id = "call-" + strconv.Itoa(idx)
			}
			if err := onChunk(agent.Chunk{ToolUseStop: id}); err != nil {
				return err
			}
		}
	}
	return nil
}

// assembleResponse converts the accumulated streamState into the same
// CompletionResponse shape Complete returns.
func assembleResponse(st *streamState) *agent.CompletionResponse {
	stop := agent.StopEndTurn
	switch st.finishReason {
	case "stop":
		stop = agent.StopEndTurn
	case "tool_calls", "function_call":
		stop = agent.StopToolUse
	case "length":
		stop = agent.StopMaxTokens
	}

	var toolCalls []agent.ToolCall
	for _, idx := range st.toolOrder {
		tool := st.tools[idx]
		args := strings.TrimSpace(tool.argsBuf.String())
		if args == "" {
			args = "{}"
		}
		id := tool.id
		if id == "" {
			id = "call-" + strconv.Itoa(idx)
		}
		toolCalls = append(toolCalls, agent.ToolCall{
			ID:    id,
			Name:  tool.name,
			Input: json.RawMessage(args),
		})
	}
	if len(toolCalls) > 0 && stop == agent.StopEndTurn {
		// finish_reason is sometimes absent on openai-compatible
		// servers when tool calls are emitted (same quirk Complete
		// works around).
		stop = agent.StopToolUse
	}

	return &agent.CompletionResponse{
		ReasoningContent: st.reasoningParts.String(), // M317
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			Content:   st.textParts.String(),
			ToolCalls: toolCalls,
		},
		StopReason: stop,
		Usage: agent.Usage{
			InputTokens:       st.inputTokens,
			CachedInputTokens: st.cachedTokens, // M887: cache hits price at the cache-read rate
			OutputTokens:      st.outputTokens,
			Model:             st.model,
		},
	}
}
