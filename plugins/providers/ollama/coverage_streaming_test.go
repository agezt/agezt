// SPDX-License-Identifier: MIT

package ollama

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestOllamaStreamingParseStreamTextAndTools(t *testing.T) {
	stream := strings.Join([]string{
		`{"model":"llama","message":{"role":"assistant","content":"hello "}}`,
		`{"model":"llama","message":{"role":"assistant","content":"world"}}`,
		`{"model":"llama","done":true,"done_reason":"stop","message":{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"lookup","arguments":{"q":1}}}]},"prompt_eval_count":3,"eval_count":2}`,
	}, "\n")

	var chunks []string
	resp, err := parseStream(strings.NewReader(stream), "llama", func(c agent.Chunk) error {
		if c.TextDelta != "" {
			chunks = append(chunks, "text:"+c.TextDelta)
		}
		if c.ToolUseStart != nil {
			chunks = append(chunks, "start:"+c.ToolUseStart.Name)
		}
		if c.ToolInputJSONDelta != "" {
			chunks = append(chunks, "args:"+c.ToolInputJSONDelta)
		}
		if c.ToolUseStop != "" {
			chunks = append(chunks, "stop:"+c.ToolUseStop)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.Message.Content != "hello world" {
		t.Fatalf("text = %q", resp.Message.Content)
	}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Name != "lookup" || string(resp.Message.ToolCalls[0].Input) != `{"q":1}` {
		t.Fatalf("tool calls = %+v", resp.Message.ToolCalls)
	}
	if resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 2 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if resp.StopReason != agent.StopToolUse {
		t.Fatalf("stop = %v", resp.StopReason)
	}
	for _, want := range []string{"text:hello ", "text:world", "start:lookup", "args:{\"q\":1}", "stop:c1"} {
		if !strings.Contains(strings.Join(chunks, ","), want) {
			t.Fatalf("chunks missing %q in %v", want, chunks)
		}
	}
}

func TestOllamaStreamingParseStreamErrorFromCallback(t *testing.T) {
	stream := `{"message":{"role":"assistant","content":"hi"}}` + "\n"
	boom := errors.New("client abort")
	_, err := parseStream(strings.NewReader(stream), "m", func(c agent.Chunk) error { return boom })
	if err == nil || !strings.Contains(err.Error(), "client abort") {
		t.Fatalf("expected callback error, got %v", err)
	}
}

func TestOllamaStreamingParseStreamToolWithoutIDAndLengthStop(t *testing.T) {
	stream := strings.Join([]string{
		`{"model":"llama","message":{"role":"assistant","tool_calls":[{"function":{"name":"lookup","arguments":{"q":1}}}]}}`,
		`{"model":"llama","done":true,"done_reason":"length"}`,
	}, "\n")
	var starts []string
	_, err := parseStream(strings.NewReader(stream), "llama", func(c agent.Chunk) error {
		if c.ToolUseStart != nil {
			starts = append(starts, c.ToolUseStart.ID+":"+c.ToolUseStart.Name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if len(starts) != 1 || starts[0] != "call-0:lookup" {
		t.Fatalf("default tool id = %v", starts)
	}
}

func TestOllamaStreamingParseStreamBadFramesAndEmptyLines(t *testing.T) {
	// Bad JSON lines are skipped; empty lines are tolerated.
	stream := strings.Join([]string{
		`not-json`,
		``,
		`{"message":{"role":"assistant","content":"hello"}}`,
		`{"done":true,"done_reason":"stop"}`,
	}, "\n")
	resp, err := parseStream(strings.NewReader(stream), "m", func(c agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.Message.Content != "hello" || resp.StopReason != agent.StopEndTurn {
		t.Fatalf("response = %+v", resp)
	}
}

func TestOllamaStreamingEncodeAndCompleteValidation(t *testing.T) {
	body, err := encodeStreamRequest("m", "", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, []agent.ToolDef{{Name: "plain"}}, 32, true, agent.Params{}, json.RawMessage(`{"options":{"num_predict":32}}`))
	if err != nil {
		t.Fatalf("encodeStreamRequest: %v", err)
	}
	text := string(body)
	for _, want := range []string{`"stream":true`, `"format":"json"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("encoded body missing %q in %s", want, text)
		}
	}

	if _, err := New().CompleteStream(t.Context(), agent.CompletionRequest{Model: ""}, func(agent.Chunk) error { return nil }); !errors.Is(err, ErrNoModel) {
		t.Fatalf("missing model stream error = %v", err)
	}
	if _, err := New().CompleteStream(t.Context(), agent.CompletionRequest{Model: "m"}, nil); err == nil || !strings.Contains(err.Error(), "non-nil onChunk") {
		t.Fatalf("nil onChunk stream error = %v", err)
	}
}
