// SPDX-License-Identifier: MIT

package anthropic

import (
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestAnthropicStreamingTextAndThinking(t *testing.T) {
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"message":{"model":"claude-x","usage":{"input_tokens":7,"output_tokens":0,"cache_read_input_tokens":2,"cache_creation_input_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"index":0,"content_block":{"type":"text","text":"hello "}}`,
		``,
		`event: content_block_delta`,
		`data: {"index":0,"delta":{"type":"text_delta","text":"world"}}`,
		``,
		`event: content_block_stop`,
		`data: {"index":0}`,
		``,
		`event: content_block_start`,
		`data: {"index":1,"content_block":{"type":"thinking","text":"reasoning "}}`,
		``,
		`event: content_block_delta`,
		`data: {"index":1,"delta":{"type":"thinking_delta","thinking":"fragment"}}`,
		``,
		`event: content_block_stop`,
		`data: {"index":1}`,
		``,
		`event: message_delta`,
		`data: {"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
		``,
		`event: message_stop`,
		`data: {}`,
		``,
	}, "\n")

	var texts, reasons int
	resp, err := parseStream(strings.NewReader(stream), func(c agent.Chunk) error {
		if c.TextDelta != "" {
			texts++
		}
		if c.ReasoningDelta != "" {
			reasons++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.Message.Content != "hello world" {
		t.Fatalf("text = %q", resp.Message.Content)
	}
	if resp.ReasoningContent != "fragment" {
		t.Fatalf("reasoning = %q", resp.ReasoningContent)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Fatalf("stop = %v", resp.StopReason)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.CachedInputTokens != 2 || resp.Usage.CacheWriteInputTokens != 1 || resp.Usage.OutputTokens != 5 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if texts == 0 || reasons == 0 {
		t.Fatalf("chunks text=%d reason=%d", texts, reasons)
	}
}

func TestAnthropicStreamingToolUseLifecycle(t *testing.T) {
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"message":{"model":"claude-x","usage":{"input_tokens":3}}}`,
		``,
		`event: content_block_start`,
		`data: {"index":0,"content_block":{"type":"tool_use","id":"call-1","name":"lookup"}}`,
		``,
		`event: content_block_delta`,
		`data: {"index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`,
		``,
		`event: content_block_delta`,
		`data: {"index":0,"delta":{"type":"input_json_delta","partial_json":"\"izmir\"}"}}`,
		``,
		`event: content_block_stop`,
		`data: {"index":0}`,
		``,
		`event: message_delta`,
		`data: {"delta":{"stop_reason":"tool_use"}}`,
		``,
		`event: message_stop`,
		`data: {}`,
		``,
	}, "\n")

	var starts, deltas, stops int
	resp, err := parseStream(strings.NewReader(stream), func(c agent.Chunk) error {
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
	if starts != 1 || deltas != 2 || stops != 1 {
		t.Fatalf("chunk counts = %d/%d/%d", starts, deltas, stops)
	}
	if len(resp.Message.ToolCalls) != 1 || string(resp.Message.ToolCalls[0].Input) != `{"q":"izmir"}` {
		t.Fatalf("tool calls = %+v", resp.Message.ToolCalls)
	}
	if resp.StopReason != agent.StopToolUse {
		t.Fatalf("stop = %v", resp.StopReason)
	}
}

func TestAnthropicStreamingErrorAndPingFrames(t *testing.T) {
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"message":{"model":"x","usage":{"input_tokens":1}}}`,
		``,
		`: keep-alive`,
		`event: ping`,
		`data: {}`,
		``,
		`event: error`,
		`data: {"error":{"type":"rate_limit","message":"too fast"}}`,
		``,
	}, "\n")
	_, err := parseStream(strings.NewReader(stream), func(agent.Chunk) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "rate_limit") || !strings.Contains(err.Error(), "too fast") {
		t.Fatalf("expected stream error, got %v", err)
	}
}

func TestAnthropicStreamingErrorFrameUnparseable(t *testing.T) {
	stream := "event: error\ndata: not-json\n\n"
	_, err := parseStream(strings.NewReader(stream), func(agent.Chunk) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "unparseable") {
		t.Fatalf("expected unparseable error, got %v", err)
	}
}

func TestAnthropicStreamingCallbackErrorAbortsStream(t *testing.T) {
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"message":{"model":"x","usage":{"input_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"index":0,"content_block":{"type":"text","text":"hi"}}`,
		``,
	}, "\n")
	boom := errors.New("client abort")
	_, err := parseStream(strings.NewReader(stream), func(agent.Chunk) error { return boom })
	if err == nil || !strings.Contains(err.Error(), "client abort") {
		t.Fatalf("expected callback error, got %v", err)
	}
}

func TestAnthropicStreamingEOFFallback(t *testing.T) {
	// Missing message_stop, but we still assemble what we have.
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"message":{"model":"x","usage":{"input_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"index":0,"content_block":{"type":"text","text":"partial"}}`,
		``,
		`event: content_block_stop`,
		`data: {"index":0}`,
		``,
	}, "\n")
	resp, err := parseStream(strings.NewReader(stream), func(agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("parseStream EOF: %v", err)
	}
	if resp.Message.Content != "partial" {
		t.Fatalf("EOF assemble content = %q", resp.Message.Content)
	}
}

func TestAnthropicStreamingTolerantFrameErrors(t *testing.T) {
	// Malformed frames are tolerated (M451); unknown delta types ignored.
	stream := strings.Join([]string{
		`event: message_start`,
		`data: not-json`,
		``,
		`event: content_block_delta`,
		`data: not-json`,
		``,
		`event: content_block_delta`,
		`data: {"index":0,"delta":{"type":"unknown_delta","text":"ignored"}}`,
		``,
		`event: message_stop`,
		`data: {}`,
		``,
	}, "\n")
	if _, err := parseStream(strings.NewReader(stream), func(agent.Chunk) error { return nil }); err != nil {
		t.Fatalf("malformed frames should not error: %v", err)
	}
}

func TestAnthropicStreamingAssembleStopVariants(t *testing.T) {
	// Hit the assemble switch for max_tokens + tool_use fallback when finish_reason is end_turn but tools present.
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"message":{"model":"x","usage":{"input_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"index":0,"content_block":{"type":"tool_use","id":"c1","name":"lookup"}}`,
		``,
		`event: content_block_delta`,
		`data: {"index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
		``,
		`event: content_block_stop`,
		`data: {"index":0}`,
		``,
		`event: message_delta`,
		`data: {"delta":{"stop_reason":"max_tokens"}}`,
		``,
		`event: message_stop`,
		`data: {}`,
		``,
	}, "\n")
	resp, err := parseStream(strings.NewReader(stream), func(agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	// stop_reason="max_tokens" wins over the tool_use fallback.
	if resp.StopReason != agent.StopMaxTokens {
		t.Fatalf("max_tokens stop = %v", resp.StopReason)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d", len(resp.Message.ToolCalls))
	}
}

func TestAnthropicStreamingValidation(t *testing.T) {
	if _, err := (&Provider{}).CompleteStream(t.Context(), agent.CompletionRequest{Model: "m"}, func(agent.Chunk) error { return nil }); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("missing key stream = %v", err)
	}
	if _, err := (&Provider{APIKey: "k"}).CompleteStream(t.Context(), agent.CompletionRequest{}, func(agent.Chunk) error { return nil }); !errors.Is(err, ErrNoModel) {
		t.Fatalf("missing model stream = %v", err)
	}
	if _, err := (&Provider{APIKey: "k"}).CompleteStream(t.Context(), agent.CompletionRequest{Model: "m"}, nil); err == nil || !strings.Contains(err.Error(), "non-nil onChunk") {
		t.Fatalf("nil onChunk = %v", err)
	}
}
