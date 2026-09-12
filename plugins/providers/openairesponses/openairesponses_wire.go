// SPDX-License-Identifier: MIT

// OpenAI Responses provider: SSE event types + parseSSE + sseError.
// Code extracted from openairesponses.go during the Day-124 god-file split.
// Public API unchanged.
package openairesponses


import (
	"bufio"
	"fmt"
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
)

type sseEvent struct {
	Type     string          `json:"type"`
	Delta    string          `json:"delta"`
	Item     json.RawMessage `json:"item"`
	Response json.RawMessage `json:"response"`
	Error    json.RawMessage `json:"error"`
}

type respItem struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Name    string `json:"name"`
	Args    string `json:"arguments"`
	CallID  string `json:"call_id"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type respObj struct {
	Status string `json:"status"`
	Usage  struct {
		InputTokens       int `json:"input_tokens"`
		OutputTokens      int `json:"output_tokens"`
		CachedInputTokens int `json:"cached_input_tokens"`
	} `json:"usage"`
	Output []respItem `json:"output"`
	Error  struct {
		Message string `json:"message"`
	} `json:"error"`
}

// parseSSE walks the event stream, assembling text + tool calls + usage.
func parseSSE(raw []byte) (*agent.CompletionResponse, error) {
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	sc.Buffer(make([]byte, 0, 1024*1024), 16<<20)

	var textParts []string
	var deltaBuf strings.Builder
	var toolCalls []agent.ToolCall
	var usage agent.Usage
	var completed bool
	var failure string

	addItem := func(it respItem) {
		switch it.Type {
		case "message":
			for _, c := range it.Content {
				if c.Type == "output_text" || c.Type == "text" {
					textParts = append(textParts, c.Text)
				}
			}
		case "function_call":
			args := it.Args
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, agent.ToolCall{ID: it.CallID, Name: it.Name, Input: json.RawMessage(args)})
		}
	}

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var ev sseEvent
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "response.output_text.delta":
			deltaBuf.WriteString(ev.Delta)
		case "response.output_item.done":
			var it respItem
			if json.Unmarshal(ev.Item, &it) == nil {
				addItem(it)
			}
		case "response.completed":
			var r respObj
			if json.Unmarshal(ev.Response, &r) == nil {
				usage = agent.Usage{
					InputTokens:       r.Usage.InputTokens,
					OutputTokens:      r.Usage.OutputTokens,
					CachedInputTokens: r.Usage.CachedInputTokens,
				}
				// Fall back to the terminal output array if no item.done arrived.
				if len(textParts) == 0 && len(toolCalls) == 0 {
					for _, it := range r.Output {
						addItem(it)
					}
				}
			}
			completed = true
		case "response.failed", "error":
			failure = sseError(ev)
		}
	}

	if failure != "" {
		return nil, fmt.Errorf("openairesponses: %s", failure)
	}
	if !completed && len(textParts) == 0 && len(toolCalls) == 0 && deltaBuf.Len() == 0 {
		return nil, fmt.Errorf("openairesponses: empty/incomplete response stream")
	}

	text := strings.Join(textParts, "")
	if text == "" {
		text = deltaBuf.String()
	}
	stop := agent.StopEndTurn
	if len(toolCalls) > 0 {
		stop = agent.StopToolUse
	}
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: text, ToolCalls: toolCalls},
		StopReason: stop,
		Usage:      usage,
	}, nil
}

func sseError(ev sseEvent) string {
	if len(ev.Response) > 0 {
		var r respObj
		if json.Unmarshal(ev.Response, &r) == nil && r.Error.Message != "" {
			return r.Error.Message
		}
	}
	if len(ev.Error) > 0 {
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(ev.Error, &e) == nil && e.Message != "" {
			return e.Message
		}
	}
	return "response failed"
}
