// SPDX-License-Identifier: MIT

package openai

import (
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestOpenAIStreamingParseStreamTextAndReasoning(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"x","model":"gpt-x","choices":[{"index":0,"delta":{"role":"assistant","content":"hello "}}]}`,
		`data: {"id":"x","model":"gpt-x","choices":[{"index":0,"delta":{"content":"world","reasoning_content":"because"}}]}`,
		`data: {"id":"x","model":"gpt-x","choices":[{"index":0,"delta":{}}],"usage":{"prompt_tokens":4,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":1}}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	var chunks []string
	resp, err := parseStream(strings.NewReader(stream), func(c agent.Chunk) error {
		if c.TextDelta != "" {
			chunks = append(chunks, "text:"+c.TextDelta)
		}
		if c.ReasoningDelta != "" {
			chunks = append(chunks, "reasoning:"+c.ReasoningDelta)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.Message.Content != "hello world" {
		t.Fatalf("text content = %q", resp.Message.Content)
	}
	if resp.ReasoningContent != "because" {
		t.Fatalf("reasoning = %q", resp.ReasoningContent)
	}
	if resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 2 || resp.Usage.CachedInputTokens != 1 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if !strings.Contains(strings.Join(chunks, ","), "text:hello ") || !strings.Contains(strings.Join(chunks, ","), "reasoning:because") {
		t.Fatalf("chunks = %v", chunks)
	}
}

func TestOpenAIStreamingParseStreamToolCallAndStopSignals(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{\"city\":"}}]}}]}`,
		`data: {"id":"x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"izmir\"}"}}]}}]}`,
		`data: {"id":"x","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
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
	if resp.StopReason != agent.StopToolUse || len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("response = %+v", resp)
	}
	if string(resp.Message.ToolCalls[0].Input) != `{"city":"izmir"}` {
		t.Fatalf("args = %s", resp.Message.ToolCalls[0].Input)
	}
}

func TestOpenAIStreamingParseStreamErrorFromCallback(t *testing.T) {
	stream := `data: {"choices":[{"index":0,"delta":{"content":"hello"}}]}` + "\n"
	boom := errors.New("client abort")
	_, err := parseStream(strings.NewReader(stream), func(c agent.Chunk) error { return boom })
	if err == nil || !strings.Contains(err.Error(), "client abort") {
		t.Fatalf("expected callback error, got %v", err)
	}
}

func TestOpenAIStreamingParseStreamMissingDoneAndBadFrames(t *testing.T) {
	// Bad frames are tolerated; EOF without [DONE] still assembles a response.
	stream := strings.Join([]string{
		`data: not-json`,
		`data: {"choices":[{"index":0,"delta":{"content":"only text"}}]}`,
		``, // empty line ignored
		// EOF (no [DONE])
	}, "\n")
	resp, err := parseStream(strings.NewReader(stream), func(c agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("parseStream EOF: %v", err)
	}
	if resp.Message.Content != "only text" {
		t.Fatalf("EOF assemble content = %q", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Fatalf("EOF stop = %v", resp.StopReason)
	}
}

func TestOpenAIStreamingParseStreamStopVariants(t *testing.T) {
	cases := map[string]agent.StopReason{
		"stop":          agent.StopEndTurn,
		"length":        agent.StopMaxTokens,
		"tool_calls":    agent.StopToolUse,
		"function_call": agent.StopToolUse,
		"unsupported":   agent.StopEndTurn,
	}
	for finish, want := range cases {
		t.Run(finish, func(t *testing.T) {
			stream := `data: {"choices":[{"index":0,"delta":{},"finish_reason":"` + finish + `"}]}` + "\n" + `data: [DONE]`
			resp, err := parseStream(strings.NewReader(stream), func(c agent.Chunk) error { return nil })
			if err != nil {
				t.Fatalf("parseStream: %v", err)
			}
			if resp.StopReason != want {
				t.Fatalf("finish_reason=%q stop=%v want %v", finish, resp.StopReason, want)
			}
		})
	}
}

func TestOpenAIStreamingAssembleStopFallbackForTools(t *testing.T) {
	// finish_reason missing but tool calls present — fall back to StopToolUse.
	stream := `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup","arguments":"{}"}}]}}]}` + "\n" + `data: [DONE]`
	resp, err := parseStream(strings.NewReader(stream), func(c agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.StopReason != agent.StopToolUse {
		t.Fatalf("tool fallback stop = %v", resp.StopReason)
	}
}

func TestOpenAIProviderIdentityAndErrors(t *testing.T) {
	if (&Provider{}).Name() != "openai" {
		t.Fatalf("Name = %q", (&Provider{}).Name())
	}
	if got := (&APIError{Status: 429, Body: "slow"}).Error(); !strings.Contains(got, "429") || !strings.Contains(got, "slow") {
		t.Fatalf("APIError = %q", got)
	}
	p := New("k")
	p.Endpoint = "https://example.com/chat"
	if got := p.resolveEndpoint(); got != "https://example.com/chat" {
		t.Fatalf("explicit endpoint = %q", got)
	}
	p.Endpoint = ""
	if got := (&Provider{BaseURL: "https://proxy.example/api/v1/custom"}).resolveEndpoint(); got != "https://proxy.example/api/v1/custom/chat/completions" {
		t.Fatalf("embedded /v1 endpoint = %q", got)
	}
	if !isImageURL("https://x/y.png") || isImageURL("plain.png") {
		t.Fatal("isImageURL mismatch")
	}
	if got := oaTextOrNil(""); got != nil {
		t.Fatalf("oaTextOrNil empty = %#v", got)
	}
	if got := oaTextOrNil("hi"); got != "hi" {
		t.Fatalf("oaTextOrNil hi = %#v", got)
	}
}

func TestOpenAICompleteValidation(t *testing.T) {
	if _, err := (&Provider{}).Complete(t.Context(), agent.CompletionRequest{Model: "m"}); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("missing key = %v", err)
	}
	if _, err := (&Provider{APIKey: "k"}).Complete(t.Context(), agent.CompletionRequest{}); !errors.Is(err, ErrNoModel) {
		t.Fatalf("missing model = %v", err)
	}
}
