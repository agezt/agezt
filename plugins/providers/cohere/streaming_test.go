// SPDX-License-Identifier: MIT

package cohere

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ersinkoc/agezt/kernel/agent"
)

// sampleCohereTextStream is a representative Cohere v2/chat SSE
// response. Frames are `event: <name>\ndata: <json>\n\n`. Text
// fragments live in delta.message.content.text; usage + finish
// reason arrive in the message-end frame.
const sampleCohereTextStream = `event: message-start
data: {"id":"msg_01","type":"message-start","delta":{"message":{"role":"assistant","content":[],"tool_calls":[]}}}

event: content-start
data: {"index":0,"type":"content-start","delta":{"message":{"content":{"type":"text","text":""}}}}

event: content-delta
data: {"index":0,"type":"content-delta","delta":{"message":{"content":{"text":"pong"}}}}

event: content-delta
data: {"index":0,"type":"content-delta","delta":{"message":{"content":{"text":"!"}}}}

event: content-end
data: {"index":0,"type":"content-end"}

event: message-end
data: {"type":"message-end","delta":{"finish_reason":"COMPLETE","usage":{"tokens":{"input_tokens":12,"output_tokens":3}}}}

`

// sampleCohereToolCallStream simulates a model picking a tool. Args
// stream as fragments across multiple tool-call-delta events, then
// tool-call-end closes the lifecycle.
const sampleCohereToolCallStream = `event: message-start
data: {"id":"msg_02","type":"message-start","delta":{"message":{"role":"assistant","content":[],"tool_calls":[]}}}

event: tool-call-start
data: {"index":0,"type":"tool-call-start","delta":{"message":{"tool_calls":{"id":"call_abc","function":{"name":"shell","arguments":""}}}}}

event: tool-call-delta
data: {"index":0,"type":"tool-call-delta","delta":{"message":{"tool_calls":{"function":{"arguments":"{\"command\":"}}}}}

event: tool-call-delta
data: {"index":0,"type":"tool-call-delta","delta":{"message":{"tool_calls":{"function":{"arguments":"\"ls -la\"}"}}}}}

event: tool-call-end
data: {"index":0,"type":"tool-call-end"}

event: message-end
data: {"type":"message-end","delta":{"finish_reason":"TOOL_CALL","usage":{"tokens":{"input_tokens":40,"output_tokens":18}}}}

`

func TestParseStream_CohereTextOnly(t *testing.T) {
	var deltas []string
	resp, err := parseStream(strings.NewReader(sampleCohereTextStream), "command-r-plus", func(c agent.Chunk) error {
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
	if len(deltas) != 2 || deltas[0] != "pong" || deltas[1] != "!" {
		t.Errorf("deltas = %v, want [pong !]", deltas)
	}
}

func TestParseStream_CohereToolCall(t *testing.T) {
	var (
		gotStart    *agent.ToolCall
		jsonFragmts []string
		gotStop     string
	)
	resp, err := parseStream(strings.NewReader(sampleCohereToolCallStream), "command-r-plus", func(c agent.Chunk) error {
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
	if joined := strings.Join(jsonFragmts, ""); joined != `{"command":"ls -la"}` {
		t.Errorf("fragments joined = %q, want %q", joined, `{"command":"ls -la"}`)
	}
	if gotStop != "call_abc" {
		t.Errorf("ToolUseStop = %q, want call_abc", gotStop)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if string(tc.Input) != `{"command":"ls -la"}` {
		t.Errorf("assembled args = %s", tc.Input)
	}
	if resp.StopReason != agent.StopToolUse {
		t.Errorf("stop = %q, want tool_use", resp.StopReason)
	}
}

func TestParseStream_Cohere_OnChunkAborts(t *testing.T) {
	_, err := parseStream(strings.NewReader(sampleCohereTextStream), "x", func(c agent.Chunk) error {
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

func TestParseStream_Cohere_MalformedFrameTolerated(t *testing.T) {
	const stream = `event: message-start
data: {"id":"x","delta":{"message":{"role":"assistant"}}}

event: content-delta
data: not json at all

event: content-delta
data: {"index":0,"delta":{"message":{"content":{"text":"hi"}}}}

event: message-end
data: {"delta":{"finish_reason":"COMPLETE","usage":{"tokens":{"input_tokens":1,"output_tokens":1}}}}

`
	resp, err := parseStream(strings.NewReader(stream), "x", func(c agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("malformed frame killed stream: %v", err)
	}
	if resp.Message.Content != "hi" {
		t.Errorf("content = %q, want 'hi'", resp.Message.Content)
	}
}

// --- end-to-end via httptest ---

func TestCompleteStream_Cohere_EndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("missing Accept header")
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("wrong Authorization: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(sampleCohereTextStream))
	}))
	defer srv.Close()

	p := &Provider{APIKey: "test-key", Endpoint: srv.URL, HTTP: srv.Client()}
	var got strings.Builder
	resp, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Model:    "command-r-plus",
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
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestCompleteStream_Cohere_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"message":"invalid API key"}`))
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
		t.Errorf("got %v, want APIError{Status:401}", err)
	}
}

func TestCompleteStream_Cohere_NilOnChunkRejected(t *testing.T) {
	p := &Provider{APIKey: "k"}
	_, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "non-nil onChunk") {
		t.Errorf("got %v, want nil-callback rejection", err)
	}
}

func TestCompleteStream_Cohere_AssembledInputIsValidJSON(t *testing.T) {
	// Sanity: the multi-frame tool args from sampleCohereToolCallStream
	// reassemble to JSON that round-trips through encoding/json.
	resp, err := parseStream(strings.NewReader(sampleCohereToolCallStream), "x", func(c agent.Chunk) error { return nil })
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatal("no tool call")
	}
	var parsed map[string]any
	if err := json.Unmarshal(resp.Message.ToolCalls[0].Input, &parsed); err != nil {
		t.Errorf("assembled args don't parse: %v", err)
	}
}

// Compile-time guard — *Provider must satisfy StreamingProvider.
var _ agent.StreamingProvider = (*Provider)(nil)
