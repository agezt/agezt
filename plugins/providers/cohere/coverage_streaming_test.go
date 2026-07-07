// SPDX-License-Identifier: MIT

package cohere

import (
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestCohereStreamingParseStreamTextAndUsage(t *testing.T) {
	stream := strings.Join([]string{
		`event: message-start`,
		`data: {}`,
		``,
		`event: content-start`,
		`data: {}`,
		``,
		`event: content-delta`,
		`data: {"delta":{"message":{"content":{"text":"hello "}}}}`,
		``,
		`event: content-delta`,
		`data: {"delta":{"message":{"content":{"text":"world"}}}}`,
		``,
		`event: content-end`,
		`data: {}`,
		``,
		`event: message-end`,
		`data: {"delta":{"finish_reason":"COMPLETE","usage":{"tokens":{"input_tokens":5,"output_tokens":2}}}}`,
		``,
	}, "\n")

	var texts int
	resp, err := parseStream(strings.NewReader(stream), "command-r", func(c agent.Chunk) error {
		if c.TextDelta != "" {
			texts++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.Message.Content != "hello world" {
		t.Fatalf("content = %q", resp.Message.Content)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 2 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Fatalf("stop = %v", resp.StopReason)
	}
	if texts != 2 {
		t.Fatalf("text chunks = %d", texts)
	}
}

func TestCohereStreamingParseStreamToolCall(t *testing.T) {
	stream := strings.Join([]string{
		`event: message-start`,
		`data: {}`,
		``,
		`event: tool-call-start`,
		`data: {"index":0,"delta":{"message":{"tool_calls":{"id":"c1","function":{"name":"lookup","arguments":"{\"city\":"}}}}}`,
		``,
		`event: tool-call-delta`,
		`data: {"index":0,"delta":{"message":{"tool_calls":{"function":{"arguments":"izmir"}}}}}`,
		``,
		`event: tool-call-delta`,
		`data: {"index":0,"delta":{"message":{"tool_calls":{"function":{"arguments":"}"}}}}`,
		``,
		`event: tool-call-end`,
		`data: {"index":0}`,
		``,
		`event: message-end`,
		`data: {"delta":{"finish_reason":"TOOL_CALL"}}`,
		``,
	}, "\n")

	var starts, deltas, stops int
	resp, err := parseStream(strings.NewReader(stream), "command-r", func(c agent.Chunk) error {
		if c.ToolUseStart != nil {
			starts++
		}
		if c.ToolInputJSONDelta != "" {
			deltas++
		}
		if c.ToolUseStop != "" {
			stops++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if starts != 1 || stops != 1 {
		t.Fatalf("start/stop counts = %d/%d", starts, stops)
	}
	if deltas < 1 {
		t.Fatalf("expected at least one delta, got %d", deltas)
	}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].ID != "c1" {
		t.Fatalf("tool calls = %+v", resp.Message.ToolCalls)
	}
	if resp.StopReason != agent.StopToolUse {
		t.Fatalf("stop = %v", resp.StopReason)
	}
}

func TestCohereStreamingToolCallEndWithoutStart(t *testing.T) {
	stream := strings.Join([]string{
		`event: tool-call-end`,
		`data: {"index":0}`,
		``,
		`event: message-end`,
		`data: {"delta":{"finish_reason":"COMPLETE"}}`,
		``,
	}, "\n")
	resp, err := parseStream(strings.NewReader(stream), "command-r", func(agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if len(resp.Message.ToolCalls) != 0 {
		t.Fatalf("orphan tool end should produce no tool calls: %+v", resp.Message.ToolCalls)
	}
}

func TestCohereStreamingBadFramesAndEmpty(t *testing.T) {
	stream := strings.Join([]string{
		`data: not-json`,
		``,
		`event: content-delta`,
		`data: not-json`,
		``,
		`event: content-delta`,
		`data: {"delta":{"message":{"content":{"text":"only text"}}}}`,
		``,
		`event: message-end`,
		`data: {"delta":{"finish_reason":"COMPLETE"}}`,
		``,
	}, "\n")
	resp, err := parseStream(strings.NewReader(stream), "command-r", func(agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.Message.Content != "only text" {
		t.Fatalf("content = %q", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Fatalf("stop = %v", resp.StopReason)
	}
}

func TestCohereStreamingToolPlanDeltaAndMaxTokens(t *testing.T) {
	stream := strings.Join([]string{
		`event: tool-plan-delta`,
		`data: {"delta":{"message":{"content":{"text":"ignored plan"}}}}`,
		``,
		`event: message-end`,
		`data: {"delta":{"finish_reason":"MAX_TOKENS"}}`,
		``,
	}, "\n")
	resp, err := parseStream(strings.NewReader(stream), "command-r", func(c agent.Chunk) error {
		if c.TextDelta != "" {
			t.Fatalf("unexpected text delta from tool-plan-delta: %q", c.TextDelta)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.StopReason != agent.StopMaxTokens {
		t.Fatalf("max_tokens stop = %v", resp.StopReason)
	}
}

func TestCohereStreamingValidation(t *testing.T) {
	if _, err := (&Provider{}).CompleteStream(t.Context(), agent.CompletionRequest{Model: "m"}, func(agent.Chunk) error { return nil }); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("missing key = %v", err)
	}
	if _, err := (&Provider{APIKey: "k"}).CompleteStream(t.Context(), agent.CompletionRequest{}, func(agent.Chunk) error { return nil }); !errors.Is(err, ErrNoModel) {
		t.Fatalf("missing model = %v", err)
	}
	if _, err := (&Provider{APIKey: "k"}).CompleteStream(t.Context(), agent.CompletionRequest{Model: "m"}, nil); err == nil || !strings.Contains(err.Error(), "non-nil onChunk") {
		t.Fatalf("nil onChunk = %v", err)
	}
}
