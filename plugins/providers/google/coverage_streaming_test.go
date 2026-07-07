// SPDX-License-Identifier: MIT

package google

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestGoogleStreamingParseStreamTextAndReasoning(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"candidates":[{"content":{"parts":[{"text":"hello "}]}}]}`,
		`data: {"candidates":[{"content":{"parts":[{"text":"world"}]}},{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"cachedContentTokenCount":1,"thoughtsTokenCount":3}}`,
	}, "\n")

	var texts int
	resp, err := parseStream(strings.NewReader(stream), "gemini", func(c agent.Chunk) error {
		if c.TextDelta != "" {
			texts++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.Message.Content != "hello world" {
		t.Fatalf("text = %q", resp.Message.Content)
	}
	if resp.Usage.InputTokens != 4 || resp.Usage.CachedInputTokens != 1 || resp.Usage.OutputTokens != 5 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Fatalf("stop = %v", resp.StopReason)
	}
	if texts != 2 {
		t.Fatalf("text chunks = %d", texts)
	}
}

func TestGoogleStreamingParseStreamReasoningPart(t *testing.T) {
	stream := `data: {"candidates":[{"content":{"parts":[{"thought":true,"text":"reason"}]}}]}`
	resp, err := parseStream(strings.NewReader(stream), "gemini", func(c agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.ReasoningContent != "reason" {
		t.Fatalf("reasoning = %q", resp.ReasoningContent)
	}
	if resp.Message.Content != "" {
		t.Fatalf("text content = %q", resp.Message.Content)
	}
}

func TestGoogleStreamingParseStreamFunctionCallWhole(t *testing.T) {
	stream := `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"q":1}}}]}},{"finishReason":"STOP"}]}`
	var starts, deltas, stops int
	resp, err := parseStream(strings.NewReader(stream), "gemini", func(c agent.Chunk) error {
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
	if starts != 1 || deltas != 1 || stops != 1 {
		t.Fatalf("chunk counts = %d/%d/%d", starts, deltas, stops)
	}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].ID != "call-0" || string(resp.Message.ToolCalls[0].Input) != `{"q":1}` {
		t.Fatalf("tool calls = %+v", resp.Message.ToolCalls)
	}
	if resp.StopReason != agent.StopToolUse {
		t.Fatalf("stop = %v", resp.StopReason)
	}
}

func TestGoogleStreamingParseStreamBadFramesAndEmpty(t *testing.T) {
	stream := strings.Join([]string{
		`data: not-json`,
		``,
		`data: {"candidates":[]}`,
		`data: {"candidates":[{"finishReason":"MAX_TOKENS","content":{"parts":[]}}],"usageMetadata":{"promptTokenCount":1}}`,
	}, "\n")
	resp, err := parseStream(strings.NewReader(stream), "gemini", func(agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.StopReason != agent.StopMaxTokens {
		t.Fatalf("max_tokens stop = %v", resp.StopReason)
	}
	if resp.Usage.InputTokens != 1 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
}

func TestGoogleStreamingResolveAndValidation(t *testing.T) {
	p := New("k")
	if got := p.resolveStreamEndpoint("gemini"); !strings.Contains(got, ":streamGenerateContent") || !strings.Contains(got, "alt=sse") {
		t.Fatalf("stream endpoint = %q", got)
	}
	p.Endpoint = "https://override/stream"
	if got := p.resolveStreamEndpoint("gemini"); got != "https://override/stream" {
		t.Fatalf("explicit stream endpoint = %q", got)
	}

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

func TestGoogleStreamingEncodeAndCompleteValidation(t *testing.T) {
	body, err := encodeRequest("system", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, []agent.ToolDef{{Name: "plain"}}, 0, true, -1, agent.Params{}, json.RawMessage(`{"safetySettings":[{"category":"test"}]}`))
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	text := string(body)
	for _, want := range []string{`"responseMimeType":"application/json"`, "systemInstruction"} {
		if !strings.Contains(text, want) {
			t.Fatalf("encoded body missing %q in %s", want, text)
		}
	}
}
