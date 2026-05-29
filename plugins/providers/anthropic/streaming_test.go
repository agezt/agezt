// SPDX-License-Identifier: MIT

package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// sampleTextStream is a representative text-only SSE response from
// the Anthropic Messages API. Two text deltas split across frames so
// the test exercises the buffered-append path.
const sampleTextStream = `event: message_start
data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","model":"claude-3-5-haiku-20241022","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":12,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

: keep-alive ping (comment line)

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"pong"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"!"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":3}}

event: message_stop
data: {"type":"message_stop"}

`

// sampleToolUseStream is a tool_use-only response: model picks a tool
// and streams its input as fragmented JSON. Tests verify the input
// fragments concatenate back to valid JSON.
const sampleToolUseStream = `event: message_start
data: {"type":"message_start","message":{"id":"msg_02","type":"message","role":"assistant","model":"claude-3-5-haiku-20241022","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":30,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_abc","name":"shell","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"ls -la\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":17}}

event: message_stop
data: {"type":"message_stop"}

`

func TestParseStream_TextOnly(t *testing.T) {
	var chunks []agent.Chunk
	resp, err := parseStream(strings.NewReader(sampleTextStream), func(c agent.Chunk) error {
		chunks = append(chunks, c)
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if resp.Message.Content != "pong!" {
		t.Errorf("final content = %q, want %q", resp.Message.Content, "pong!")
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop_reason = %q, want end_turn", resp.StopReason)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage wrong: %+v", resp.Usage)
	}
	if resp.Usage.Model != "claude-3-5-haiku-20241022" {
		t.Errorf("usage model = %q, want claude-3-5-haiku-20241022", resp.Usage.Model)
	}

	// Two text deltas; the empty-text content_block_start should not
	// have emitted a chunk.
	var textChunks []string
	for _, c := range chunks {
		if c.TextDelta != "" {
			textChunks = append(textChunks, c.TextDelta)
		}
	}
	if len(textChunks) != 2 || textChunks[0] != "pong" || textChunks[1] != "!" {
		t.Errorf("text chunks = %v, want [pong !]", textChunks)
	}
}

func TestParseStream_ToolUse(t *testing.T) {
	var (
		gotStart    *agent.ToolCall
		gotStopID   string
		jsonFragmts []string
	)
	resp, err := parseStream(strings.NewReader(sampleToolUseStream), func(c agent.Chunk) error {
		if c.ToolUseStart != nil {
			gotStart = c.ToolUseStart
		}
		if c.ToolInputJSONDelta != "" {
			jsonFragmts = append(jsonFragmts, c.ToolInputJSONDelta)
		}
		if c.ToolUseStop != "" {
			gotStopID = c.ToolUseStop
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}

	// Stream-level callbacks
	if gotStart == nil || gotStart.ID != "toolu_abc" || gotStart.Name != "shell" {
		t.Errorf("ToolUseStart wrong: %+v", gotStart)
	}
	if strings.Join(jsonFragmts, "") != `{"command":"ls -la"}` {
		t.Errorf("JSON fragments joined = %q, want %q",
			strings.Join(jsonFragmts, ""), `{"command":"ls -la"}`)
	}
	if gotStopID != "toolu_abc" {
		t.Errorf("ToolUseStop = %q, want toolu_abc", gotStopID)
	}

	// Final assembled response
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 ToolCall, got %d", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.ID != "toolu_abc" || tc.Name != "shell" {
		t.Errorf("tool call meta wrong: %+v", tc)
	}
	if string(tc.Input) != `{"command":"ls -la"}` {
		t.Errorf("tool call Input = %s, want {\"command\":\"ls -la\"}", tc.Input)
	}
	if resp.StopReason != agent.StopToolUse {
		t.Errorf("stop = %q, want tool_use", resp.StopReason)
	}
	if resp.Usage.OutputTokens != 17 {
		t.Errorf("output tokens = %d, want 17", resp.Usage.OutputTokens)
	}
}

func TestParseStream_ErrorFrame(t *testing.T) {
	const errStream = `event: error
data: {"type":"error","error":{"type":"overloaded_error","message":"Anthropic is overloaded"}}

`
	_, err := parseStream(strings.NewReader(errStream), func(c agent.Chunk) error { return nil })
	if err == nil {
		t.Fatal("expected error from error-frame, got nil")
	}
	if !strings.Contains(err.Error(), "overloaded_error") || !strings.Contains(err.Error(), "overloaded") {
		t.Errorf("error message lost upstream details: %v", err)
	}
}

func TestParseStream_OnChunkAborts(t *testing.T) {
	// onChunk returning an error MUST abort the stream so callers can
	// stop reading from slow providers.
	const wantErr = "user cancelled"
	calls := 0
	_, err := parseStream(strings.NewReader(sampleTextStream), func(c agent.Chunk) error {
		calls++
		if c.TextDelta != "" {
			return &cancelErr{wantErr}
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("expected abort to propagate %q, got %v", wantErr, err)
	}
	// We should have aborted on the first text delta — second delta
	// must not have been dispatched.
	if calls > 2 {
		// 1 = the first delta itself, +1 tolerance if start-block emitted
		t.Errorf("onChunk called %d times after abort, want ≤2", calls)
	}
}

type cancelErr struct{ msg string }

func (c *cancelErr) Error() string { return c.msg }

// --- end-to-end CompleteStream against an httptest server -----------------

func TestCompleteStream_EndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("missing Accept: text/event-stream header")
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing or wrong x-api-key header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(sampleTextStream))
	}))
	defer srv.Close()

	p := &Provider{APIKey: "test-key", Endpoint: srv.URL, HTTP: srv.Client()}
	var collected strings.Builder
	resp, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Model:    "claude-3-5-haiku-20241022",
		System:   "Be terse.",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "Say 'pong' in one word."}},
	}, func(c agent.Chunk) error {
		collected.WriteString(c.TextDelta)
		return nil
	})
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if collected.String() != "pong!" {
		t.Errorf("streamed text = %q, want 'pong!'", collected.String())
	}
	if resp.Message.Content != "pong!" {
		t.Errorf("response Content = %q, want 'pong!'", resp.Message.Content)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage = %+v, want in=12 out=3", resp.Usage)
	}
}

func TestCompleteStream_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid x-api-key"}}`))
	}))
	defer srv.Close()
	p := &Provider{APIKey: "wrong-key", Endpoint: srv.URL, HTTP: srv.Client()}
	_, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}, func(c agent.Chunk) error { return nil })
	if err == nil {
		t.Fatal("expected APIError for 401")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("got %T, want *APIError", err)
	}
	if apiErr.Status != 401 {
		t.Errorf("status = %d, want 401", apiErr.Status)
	}
	if !strings.Contains(apiErr.Body, "invalid x-api-key") {
		t.Errorf("body lost: %s", apiErr.Body)
	}
}

func TestCompleteStream_NilOnChunkRejected(t *testing.T) {
	p := &Provider{APIKey: "k"}
	_, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "non-nil onChunk") {
		t.Errorf("expected nil-callback rejection, got %v", err)
	}
}

// Type assertion check — agent.StreamingProvider is the contract we
// promise. Compile-time guard via _ assignment is more reliable than
// a runtime test.
var _ agent.StreamingProvider = (*Provider)(nil)
