// SPDX-License-Identifier: MIT

package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ersinkoc/agezt/kernel/agent"
)

// sampleGeminiTextStream is a representative text-only SSE response.
// Notes: no `event:` lines (Gemini uses SSE in OpenAI's shape), no
// `[DONE]` sentinel (stream ends when body closes), terminal chunk
// carries finishReason + usageMetadata.
const sampleGeminiTextStream = `data: {"candidates":[{"content":{"parts":[{"text":"po"}],"role":"model"},"index":0}]}

data: {"candidates":[{"content":{"parts":[{"text":"ng"}],"role":"model"},"index":0}]}

data: {"candidates":[{"content":{"parts":[{"text":"!"}],"role":"model"},"index":0,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":3,"totalTokenCount":15}}

`

// sampleGeminiToolCallStream is the function-calling shape.
// Gemini delivers tool calls whole — the entire functionCall (with
// fully parsed args) arrives in one chunk. The streaming adapter
// must still emit a clean ToolUseStart → ToolInputJSONDelta →
// ToolUseStop sequence so callers don't need provider-specific
// special-casing.
const sampleGeminiToolCallStream = `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"shell","args":{"command":"ls -la"}}}],"role":"model"},"index":0,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":30,"candidatesTokenCount":12,"totalTokenCount":42}}

`

// sampleGeminiInterleavedStream has text deltas BEFORE a tool call.
// Real Gemini responses sometimes interleave text and tool calls in
// the same candidate; the parser must handle both in order.
const sampleGeminiInterleavedStream = `data: {"candidates":[{"content":{"parts":[{"text":"I'll check that."}],"role":"model"},"index":0}]}

data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"shell","args":{"command":"ls"}}}],"role":"model"},"index":0,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":18,"totalTokenCount":68}}

`

func TestParseStream_GeminiTextOnly(t *testing.T) {
	var deltas []string
	resp, err := parseStream(strings.NewReader(sampleGeminiTextStream), "gemini-1.5-flash", func(c agent.Chunk) error {
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
		t.Errorf("usage = %+v", resp.Usage)
	}
	if resp.Usage.Model != "gemini-1.5-flash" {
		t.Errorf("model = %q", resp.Usage.Model)
	}
	if len(deltas) != 3 || deltas[0] != "po" || deltas[1] != "ng" || deltas[2] != "!" {
		t.Errorf("deltas = %v, want [po ng !]", deltas)
	}
}

func TestParseStream_GeminiToolCall(t *testing.T) {
	var (
		gotStart   *agent.ToolCall
		gotInput   string
		gotStop    string
		startCount int
		stopCount  int
	)
	resp, err := parseStream(strings.NewReader(sampleGeminiToolCallStream), "gemini-1.5-pro", func(c agent.Chunk) error {
		if c.ToolUseStart != nil {
			gotStart = c.ToolUseStart
			startCount++
		}
		if c.ToolInputJSONDelta != "" {
			gotInput = c.ToolInputJSONDelta
		}
		if c.ToolUseStop != "" {
			gotStop = c.ToolUseStop
			stopCount++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	// Lifecycle: exactly one start, one input chunk, one stop.
	if startCount != 1 || stopCount != 1 {
		t.Errorf("lifecycle counts = start:%d stop:%d, want 1/1", startCount, stopCount)
	}
	if gotStart == nil || gotStart.Name != "shell" {
		t.Errorf("ToolUseStart wrong: %+v", gotStart)
	}
	if gotStart != nil && gotStart.ID != gotStop {
		t.Errorf("start.ID=%q != stop.ID=%q (lifecycle ids must match)", gotStart.ID, gotStop)
	}
	// The full args JSON arrives as one ToolInputJSONDelta chunk.
	// Must be valid JSON parseable back to the original.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(gotInput), &parsed); err != nil {
		t.Errorf("ToolInputJSONDelta is not valid JSON: %v (got %q)", err, gotInput)
	}
	if parsed["command"] != "ls -la" {
		t.Errorf("parsed args = %v, want {command: 'ls -la'}", parsed)
	}

	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.Name != "shell" {
		t.Errorf("assembled tool name = %q", tc.Name)
	}
	if resp.StopReason != agent.StopToolUse {
		t.Errorf("stop = %q, want tool_use (must derive from tool_calls presence even when finishReason=STOP)", resp.StopReason)
	}
}

func TestParseStream_GeminiInterleaved(t *testing.T) {
	var deltas []string
	var gotTool *agent.ToolCall
	resp, err := parseStream(strings.NewReader(sampleGeminiInterleavedStream), "gemini-1.5-pro", func(c agent.Chunk) error {
		if c.TextDelta != "" {
			deltas = append(deltas, c.TextDelta)
		}
		if c.ToolUseStart != nil {
			gotTool = c.ToolUseStart
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	// Text MUST arrive before the tool call (order of emission matters
	// for live-rendering callers).
	if len(deltas) != 1 || deltas[0] != "I'll check that." {
		t.Errorf("text deltas = %v, want [I'll check that.]", deltas)
	}
	if gotTool == nil || gotTool.Name != "shell" {
		t.Errorf("expected tool 'shell', got %+v", gotTool)
	}
	if resp.Message.Content != "I'll check that." {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
}

func TestParseStream_Gemini_OnChunkAborts(t *testing.T) {
	_, err := parseStream(strings.NewReader(sampleGeminiTextStream), "x", func(c agent.Chunk) error {
		if c.TextDelta != "" {
			return &cancelErr{"aborted"}
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "aborted") {
		t.Errorf("expected abort to propagate, got %v", err)
	}
}

type cancelErr struct{ msg string }

func (e *cancelErr) Error() string { return e.msg }

func TestParseStream_Gemini_GarbageFrameIgnored(t *testing.T) {
	const garbage = `data: {"candidates":[{"content":{"parts":[{"text":"hi"}]},"index":0}]}

data: {not parseable

data: {"candidates":[{"content":{"parts":[{"text":" there"}],"role":"model"},"index":0,"finishReason":"STOP"}]}

`
	resp, err := parseStream(strings.NewReader(garbage), "x", func(c agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("garbage should not kill stream: %v", err)
	}
	if resp.Message.Content != "hi there" {
		t.Errorf("content = %q, want 'hi there'", resp.Message.Content)
	}
}

// --- end-to-end against httptest server ---

func TestCompleteStream_Gemini_EndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("missing Accept header")
		}
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Errorf("missing/wrong x-goog-api-key header: %q", r.Header.Get("x-goog-api-key"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(sampleGeminiTextStream))
	}))
	defer srv.Close()

	p := &Provider{APIKey: "test-key", Endpoint: srv.URL, HTTP: srv.Client()}
	var got strings.Builder
	resp, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Model:    "gemini-1.5-flash",
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
		t.Errorf("streamed = %q", got.String())
	}
	if resp.Message.Content != "pong!" {
		t.Errorf("resp = %q", resp.Message.Content)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestCompleteStream_Gemini_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":{"message":"API key invalid"}}`))
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
	if !ok || apiErr.Status != 403 {
		t.Errorf("got %v, want APIError{Status:403}", err)
	}
}

func TestCompleteStream_Gemini_NilOnChunkRejected(t *testing.T) {
	p := &Provider{APIKey: "k"}
	_, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "non-nil onChunk") {
		t.Errorf("got %v, want nil-callback rejection", err)
	}
}

func TestResolveStreamEndpoint(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		model   string
		want    string
	}{
		{
			name:    "default base url",
			baseURL: "",
			model:   "gemini-1.5-flash",
			want:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:streamGenerateContent?alt=sse",
		},
		{
			name:    "base url with no version segment",
			baseURL: "https://example.com",
			model:   "gemini-1.5-pro",
			want:    "https://example.com/v1beta/models/gemini-1.5-pro:streamGenerateContent?alt=sse",
		},
		{
			name:    "base url already ending in /v1beta",
			baseURL: "https://example.com/v1beta",
			model:   "gemini-1.5-pro",
			want:    "https://example.com/v1beta/models/gemini-1.5-pro:streamGenerateContent?alt=sse",
		},
		{
			name:    "base url with trailing slash",
			baseURL: "https://example.com/",
			model:   "gemini-1.5-flash",
			want:    "https://example.com/v1beta/models/gemini-1.5-flash:streamGenerateContent?alt=sse",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &Provider{BaseURL: c.baseURL}
			if got := p.resolveStreamEndpoint(c.model); got != c.want {
				t.Errorf("got %q\nwant %q", got, c.want)
			}
		})
	}
}

// Compile-time guard — *Provider must satisfy StreamingProvider.
var _ agent.StreamingProvider = (*Provider)(nil)
