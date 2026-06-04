// SPDX-License-Identifier: MIT

package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// sampleOAITextStream is a representative text-only SSE response.
// Note: no "event:" lines (OpenAI flavor), final chunk carries the
// usage block, and the literal "[DONE]" sentinel terminates.
const sampleOAITextStream = `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":"pong"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}

data: [DONE]

`

// sampleOAIToolCallStream simulates a model picking a single tool
// and streaming its arguments in fragmented JSON chunks. id + name
// appear in the first chunk for the index; arguments are streamed
// across multiple frames.
const sampleOAIToolCallStream = `data: {"id":"chatcmpl-2","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"shell","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-2","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-2","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls -la\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-2","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: {"id":"chatcmpl-2","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[],"usage":{"prompt_tokens":40,"completion_tokens":18,"total_tokens":58}}

data: [DONE]

`

// sampleOAIParallelToolStream simulates two parallel tool calls,
// one at index 0 and another at index 1. They interleave their
// argument fragments — the parser must correlate by index, not
// arrival order, and emit both in the final response.
const sampleOAIParallelToolStream = `data: {"id":"chatcmpl-3","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"file_read","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-3","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"file_list","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-3","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"a\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-3","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"dir\":\"/\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-3","object":"chat.completion.chunk","model":"gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`

func TestParseStream_OAITextOnly(t *testing.T) {
	var deltas []string
	resp, err := parseStream(strings.NewReader(sampleOAITextStream), func(c agent.Chunk) error {
		if c.TextDelta != "" {
			deltas = append(deltas, c.TextDelta)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.Message.Content != "pong!" {
		t.Errorf("content = %q, want 'pong!'", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop = %q, want end_turn", resp.StopReason)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage wrong: %+v", resp.Usage)
	}
	if resp.Usage.Model != "gpt-4o-mini" {
		t.Errorf("model = %q, want gpt-4o-mini", resp.Usage.Model)
	}
	// Two non-empty text deltas (the initial empty content delta should
	// have been skipped).
	if len(deltas) != 2 || deltas[0] != "pong" || deltas[1] != "!" {
		t.Errorf("deltas = %v, want [pong !]", deltas)
	}
}

func TestParseStream_OAIToolCall(t *testing.T) {
	var (
		gotStart    *agent.ToolCall
		jsonFragmts []string
		gotStop     string
	)
	resp, err := parseStream(strings.NewReader(sampleOAIToolCallStream), func(c agent.Chunk) error {
		if c.ToolUseStart != nil {
			gotStart = c.ToolUseStart
		}
		if c.ToolInputJSONDelta != "" {
			jsonFragmts = append(jsonFragmts, c.ToolInputJSONDelta)
		}
		if c.ToolUseStop != "" {
			gotStop = c.ToolUseStop
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if gotStart == nil || gotStart.ID != "call_abc" || gotStart.Name != "shell" {
		t.Errorf("ToolUseStart wrong: %+v", gotStart)
	}
	if strings.Join(jsonFragmts, "") != `{"command":"ls -la"}` {
		t.Errorf("fragments joined = %q, want %q",
			strings.Join(jsonFragmts, ""), `{"command":"ls -la"}`)
	}
	if gotStop != "call_abc" {
		t.Errorf("ToolUseStop id = %q, want call_abc", gotStop)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.ID != "call_abc" || tc.Name != "shell" || string(tc.Input) != `{"command":"ls -la"}` {
		t.Errorf("assembled tool call wrong: %+v input=%s", tc, tc.Input)
	}
	if resp.StopReason != agent.StopToolUse {
		t.Errorf("stop = %q, want tool_use", resp.StopReason)
	}
	if resp.Usage.InputTokens != 40 || resp.Usage.OutputTokens != 18 {
		t.Errorf("usage wrong: %+v", resp.Usage)
	}
}

func TestParseStream_OAIParallelTools(t *testing.T) {
	starts := map[string]string{} // id → name
	args := map[string]*strings.Builder{}
	resp, err := parseStream(strings.NewReader(sampleOAIParallelToolStream), func(c agent.Chunk) error {
		if c.ToolUseStart != nil {
			starts[c.ToolUseStart.ID] = c.ToolUseStart.Name
			args[c.ToolUseStart.ID] = &strings.Builder{}
		}
		// We can't disambiguate which tool a delta belongs to from
		// the chunk alone — that's a known limitation of this signal
		// when callbacks have to dispatch across parallel tools.
		// Production callers use the assembled response. We only
		// verify counts here, not per-tool delta order.
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if len(starts) != 2 || starts["call_a"] != "file_read" || starts["call_b"] != "file_list" {
		t.Errorf("starts wrong: %v", starts)
	}
	if len(resp.Message.ToolCalls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(resp.Message.ToolCalls))
	}
	// Order should match toolOrder (index 0 before index 1).
	if resp.Message.ToolCalls[0].ID != "call_a" || resp.Message.ToolCalls[1].ID != "call_b" {
		t.Errorf("tool call order wrong: %+v", resp.Message.ToolCalls)
	}
	if string(resp.Message.ToolCalls[0].Input) != `{"path":"a"}` {
		t.Errorf("tool A input = %s", resp.Message.ToolCalls[0].Input)
	}
	if string(resp.Message.ToolCalls[1].Input) != `{"dir":"/"}` {
		t.Errorf("tool B input = %s", resp.Message.ToolCalls[1].Input)
	}
}

func TestParseStream_OAI_OnChunkAborts(t *testing.T) {
	calls := 0
	_, err := parseStream(strings.NewReader(sampleOAITextStream), func(c agent.Chunk) error {
		calls++
		if c.TextDelta != "" {
			return &abortErr{"caller cancelled"}
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "caller cancelled") {
		t.Fatalf("expected abort to propagate, got %v", err)
	}
}

type abortErr struct{ msg string }

func (e *abortErr) Error() string { return e.msg }

func TestParseStream_OAI_GarbageFrameIgnored(t *testing.T) {
	// A malformed JSON frame in the middle of the stream MUST NOT
	// kill the stream — some openai-compatible vendors inject
	// keep-alive comments as invalid JSON. The good text frame
	// before [DONE] should still produce content.
	const garbageStream = `data: {"id":"x","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}

data: {garbage not json}

data: {"id":"x","model":"gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
	resp, err := parseStream(strings.NewReader(garbageStream), func(c agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("garbage frame should not kill stream: %v", err)
	}
	if resp.Message.Content != "hi" {
		t.Errorf("content = %q, want 'hi'", resp.Message.Content)
	}
}

// --- end-to-end against an httptest server ---------------------------------

func TestCompleteStream_OAI_EndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("missing Accept: text/event-stream")
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing Bearer Authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(sampleOAITextStream))
	}))
	defer srv.Close()

	p := &Provider{APIKey: "test-key", Endpoint: srv.URL, HTTP: srv.Client()}
	var got strings.Builder
	resp, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Model:    "gpt-4o-mini",
		System:   "Be terse.",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "Say 'pong' in one word."}},
	}, func(c agent.Chunk) error {
		got.WriteString(c.TextDelta)
		return nil
	})
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if got.String() != "pong!" {
		t.Errorf("streamed = %q, want 'pong!'", got.String())
	}
	if resp.Message.Content != "pong!" {
		t.Errorf("resp.Content = %q", resp.Message.Content)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestCompleteStream_OAI_AzureAuthHeader(t *testing.T) {
	// Azure-flavored: api-key header, no scheme prefix. The streaming
	// path must respect the same AuthHeader/AuthScheme convention as
	// the non-streaming path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("api-key"); got != "azure-key" {
			t.Errorf("api-key header = %q, want 'azure-key'", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization should be empty for Azure, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(sampleOAITextStream))
	}))
	defer srv.Close()
	p := &Provider{
		APIKey:     "azure-key",
		Endpoint:   srv.URL,
		HTTP:       srv.Client(),
		AuthHeader: "api-key",
		AuthScheme: "",
	}
	_, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}, func(c agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
}

func TestCompleteStream_OAI_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()
	p := &Provider{APIKey: "x", Endpoint: srv.URL, HTTP: srv.Client()}
	_, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}, func(c agent.Chunk) error { return nil })
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != 401 {
		t.Errorf("got %v, want *APIError{Status:401}", err)
	}
}

func TestCompleteStream_OAI_NilOnChunkRejected(t *testing.T) {
	p := &Provider{APIKey: "k"}
	_, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "non-nil onChunk") {
		t.Errorf("got %v, want nil-callback rejection", err)
	}
}

// Compile-time guard — *Provider must satisfy StreamingProvider.
var _ agent.StreamingProvider = (*Provider)(nil)

// reasoning stream: a DeepSeek-R1-style stream that emits reasoning_content
// deltas before the answer tokens (M317).
const sampleOAIReasoningStream = `data: {"choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Let me "},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{"reasoning_content":"think."},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{"content":"42"},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`

// TestParseStream_Reasoning (M317): reasoning_content deltas accumulate into the
// response's ReasoningContent and surface as Chunk.ReasoningDelta, separate from
// the answer text.
func TestParseStream_Reasoning(t *testing.T) {
	var reasoning, text strings.Builder
	resp, err := parseStream(strings.NewReader(sampleOAIReasoningStream), func(c agent.Chunk) error {
		reasoning.WriteString(c.ReasoningDelta)
		text.WriteString(c.TextDelta)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if reasoning.String() != "Let me think." {
		t.Errorf("streamed reasoning=%q want 'Let me think.'", reasoning.String())
	}
	if text.String() != "42" {
		t.Errorf("streamed text=%q want '42'", text.String())
	}
	if resp.ReasoningContent != "Let me think." {
		t.Errorf("resp.ReasoningContent=%q", resp.ReasoningContent)
	}
	if resp.Message.Content != "42" {
		t.Errorf("resp.Content=%q (reasoning must not leak into the answer)", resp.Message.Content)
	}
}
