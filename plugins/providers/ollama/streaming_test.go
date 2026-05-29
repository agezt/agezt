// SPDX-License-Identifier: MIT

package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// sampleOllamaTextStream is a representative NDJSON streaming
// response from a local Ollama server. One JSON object per line, no
// SSE prefixes, no event tags. Final line has done:true and the
// usage counters.
const sampleOllamaTextStream = `{"model":"llama3.2","message":{"role":"assistant","content":"po"},"done":false}
{"model":"llama3.2","message":{"role":"assistant","content":"ng"},"done":false}
{"model":"llama3.2","message":{"role":"assistant","content":"!"},"done":false}
{"model":"llama3.2","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":12,"eval_count":3}
`

// sampleOllamaToolCallStream simulates a model picking a tool. The
// full tool_calls array arrives in one chunk (Ollama doesn't stream
// tool args as fragments — same as Gemini).
const sampleOllamaToolCallStream = `{"model":"llama3.2","message":{"role":"assistant","content":"","tool_calls":[{"id":"abc","function":{"name":"shell","arguments":{"command":"ls -la"}}}]},"done":false}
{"model":"llama3.2","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":30,"eval_count":17}
`

func TestParseStream_OllamaTextOnly(t *testing.T) {
	var deltas []string
	resp, err := parseStream(strings.NewReader(sampleOllamaTextStream), "llama3.2", func(c agent.Chunk) error {
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
		t.Errorf("stop = %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if resp.Usage.Model != "llama3.2" {
		t.Errorf("model = %q", resp.Usage.Model)
	}
	if len(deltas) != 3 || deltas[0] != "po" || deltas[1] != "ng" || deltas[2] != "!" {
		t.Errorf("deltas = %v, want [po ng !]", deltas)
	}
}

func TestParseStream_OllamaToolCall(t *testing.T) {
	var (
		gotStart   *agent.ToolCall
		gotInput   string
		gotStop    string
		startCount int
		stopCount  int
	)
	resp, err := parseStream(strings.NewReader(sampleOllamaToolCallStream), "llama3.2", func(c agent.Chunk) error {
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
	if startCount != 1 || stopCount != 1 {
		t.Errorf("lifecycle = start:%d stop:%d, want 1/1", startCount, stopCount)
	}
	if gotStart == nil || gotStart.Name != "shell" || gotStart.ID != "abc" {
		t.Errorf("ToolUseStart wrong: %+v", gotStart)
	}
	if gotStop != "abc" {
		t.Errorf("ToolUseStop id = %q, want abc", gotStop)
	}
	// Args arrive whole — must be valid JSON parseable back.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(gotInput), &parsed); err != nil {
		t.Errorf("ToolInputJSONDelta not valid JSON: %v (%q)", err, gotInput)
	}
	if parsed["command"] != "ls -la" {
		t.Errorf("args = %v", parsed)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
	if resp.StopReason != agent.StopToolUse {
		t.Errorf("stop = %q, want tool_use", resp.StopReason)
	}
}

func TestParseStream_OllamaSynthesizesMissingID(t *testing.T) {
	// Some Ollama versions omit per-tool IDs entirely; the parser must
	// synthesize stable ones so the loop's tool_call_id binding works.
	const noIDStream = `{"model":"x","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"shell","arguments":{"cmd":"ls"}}}]},"done":true,"done_reason":"stop"}
`
	resp, err := parseStream(strings.NewReader(noIDStream), "x", func(c agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatal("want 1 tool call")
	}
	if resp.Message.ToolCalls[0].ID == "" {
		t.Error("ID was empty — synth failed")
	}
	if !strings.HasPrefix(resp.Message.ToolCalls[0].ID, "call-") {
		t.Errorf("synth ID = %q, want 'call-N' prefix", resp.Message.ToolCalls[0].ID)
	}
}

func TestParseStream_Ollama_OnChunkAborts(t *testing.T) {
	_, err := parseStream(strings.NewReader(sampleOllamaTextStream), "x", func(c agent.Chunk) error {
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

func TestParseStream_Ollama_GarbageLineIgnored(t *testing.T) {
	const garbage = `{"model":"x","message":{"role":"assistant","content":"hi"},"done":false}
not json at all here
{"model":"x","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":1,"eval_count":1}
`
	resp, err := parseStream(strings.NewReader(garbage), "x", func(c agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("garbage shouldn't kill stream: %v", err)
	}
	if resp.Message.Content != "hi" {
		t.Errorf("content = %q", resp.Message.Content)
	}
}

func TestParseStream_OllamaLengthDoneReason(t *testing.T) {
	const lengthStream = `{"model":"x","message":{"role":"assistant","content":"truncated..."},"done":false}
{"model":"x","message":{"role":"assistant","content":""},"done":true,"done_reason":"length","prompt_eval_count":5,"eval_count":50}
`
	resp, err := parseStream(strings.NewReader(lengthStream), "x", func(c agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.StopReason != agent.StopMaxTokens {
		t.Errorf("stop = %q, want max_tokens", resp.StopReason)
	}
}

// --- end-to-end via httptest ---

func TestCompleteStream_Ollama_EndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(sampleOllamaTextStream))
	}))
	defer srv.Close()

	p := &Provider{Endpoint: srv.URL, HTTP: srv.Client()}
	var got strings.Builder
	resp, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Model:    "llama3.2",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "Say pong"}},
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
		t.Errorf("resp.Content = %q", resp.Message.Content)
	}
}

func TestCompleteStream_Ollama_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("ollama model not loaded"))
	}))
	defer srv.Close()
	p := &Provider{Endpoint: srv.URL, HTTP: srv.Client()}
	_, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}, func(c agent.Chunk) error { return nil })
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != 500 {
		t.Errorf("got %v, want APIError{Status:500}", err)
	}
}

func TestCompleteStream_Ollama_NilOnChunkRejected(t *testing.T) {
	p := &Provider{Endpoint: "http://localhost:1"}
	_, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "non-nil onChunk") {
		t.Errorf("got %v, want nil-callback rejection", err)
	}
}

// Compile-time guard — *Provider must satisfy StreamingProvider.
var _ agent.StreamingProvider = (*Provider)(nil)
