// SPDX-License-Identifier: MIT

package vertex_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/vertex"
)

// ---- model id detection ----

func TestResolveAnthropicEndpoint_RoutesToAnthropicPublisher(t *testing.T) {
	p := vertex.New(nil, "my-proj", "us-east5")
	want := "https://us-east5-aiplatform.googleapis.com/v1/projects/my-proj/locations/us-east5/publishers/anthropic/models/claude-opus-4-7@20251031:rawPredict"
	if got := p.ResolveAnthropicEndpoint("claude-opus-4-7@20251031"); got != want {
		t.Errorf("ResolveAnthropicEndpoint =\n  %q\nwant\n  %q", got, want)
	}
}

func TestResolveAnthropicStreamEndpoint_RoutesToStreamRawPredict(t *testing.T) {
	p := vertex.New(nil, "my-proj", "us-east5")
	want := "https://us-east5-aiplatform.googleapis.com/v1/projects/my-proj/locations/us-east5/publishers/anthropic/models/claude-sonnet-4-5@20250929:streamRawPredict"
	if got := p.ResolveAnthropicStreamEndpoint("claude-sonnet-4-5@20250929"); got != want {
		t.Errorf("ResolveAnthropicStreamEndpoint =\n  %q\nwant\n  %q", got, want)
	}
}

// ---- non-streaming Anthropic-on-Vertex happy path ----

func TestComplete_AnthropicModelRoutesToRawPredict(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.anth",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	}))
	defer tokenSrv.Close()

	var seen struct {
		path string
		body map[string]any
	}
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &seen.body)
		// Reply in Anthropic Messages-API shape.
		_, _ = io.WriteString(w, `{
			"id": "msg_01",
			"type": "message",
			"role": "assistant",
			"content": [{"type":"text","text":"hi from vertex anthropic"}],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 4, "output_tokens": 7}
		}`)
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "test-project", "us-east5")
	// Pin endpoint so the request lands on apiSrv but the test
	// otherwise exercises the routing/encode/decode logic.
	p.Endpoint = apiSrv.URL + "/v1/projects/test-project/locations/us-east5/publishers/anthropic/models/claude-opus-4-7@20251031:rawPredict"

	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "claude-opus-4-7@20251031",
		System:   "be terse",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
		Tools: []agent.ToolDef{
			{Name: "first", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "last", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi from vertex anthropic" {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop = %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 7 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	// Anthropic body invariants:
	if seen.body["anthropic_version"] != "vertex-2023-10-16" {
		t.Errorf("anthropic_version = %v, want vertex-2023-10-16", seen.body["anthropic_version"])
	}
	if _, ok := seen.body["model"]; ok {
		t.Errorf("body must NOT contain `model` field (model goes in URL); got: %v", seen.body)
	}
	// M302: system is sent as a cache-marked block array (not a bare string).
	if sysArr, _ := seen.body["system"].([]any); len(sysArr) == 1 {
		sb, _ := sysArr[0].(map[string]any)
		if sb["text"] != "be terse" {
			t.Errorf("system text = %v, want 'be terse'", sb["text"])
		}
		if cc, _ := sb["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
			t.Errorf("system block missing cache_control: %v", sb)
		}
	} else {
		t.Errorf("system = %v, want a 1-element block array", seen.body["system"])
	}
	if seen.body["max_tokens"] == nil {
		t.Errorf("body missing max_tokens (should default to %d)", vertex.DefaultAnthropicMaxTokens)
	}
	// Verify the URL routed through the anthropic publisher path.
	if !strings.Contains(seen.path, "publishers/anthropic/") {
		t.Errorf("path = %q, want publishers/anthropic/", seen.path)
	}
	// M300: the last tool must carry cache_control: ephemeral (caches the tools
	// prefix); earlier tools must not.
	if toolsArr, _ := seen.body["tools"].([]any); len(toolsArr) == 2 {
		if first, _ := toolsArr[0].(map[string]any); first["cache_control"] != nil {
			t.Errorf("first tool must NOT carry cache_control")
		}
		last, _ := toolsArr[1].(map[string]any)
		if cc, _ := last["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
			t.Errorf("last tool cache_control = %v want {type: ephemeral}", last["cache_control"])
		}
	} else {
		t.Errorf("tools = %v, want 2", seen.body["tools"])
	}
	if !strings.Contains(seen.path, ":rawPredict") {
		t.Errorf("path = %q, want :rawPredict suffix", seen.path)
	}
}

func TestComplete_AnthropicToolCallRoundTrip(t *testing.T) {
	// Verifies that an assistant message containing a ToolCall round-
	// trips through canonicalToAnthVx → wire → Anthropic response →
	// back to a CompletionResponse with the tool call surfaced.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "x", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{
			"id":"msg_2","type":"message","role":"assistant",
			"content":[
				{"type":"text","text":"calling tool"},
				{"type":"tool_use","id":"tu_1","name":"weather","input":{"city":"Istanbul"}}
			],
			"stop_reason":"tool_use",
			"usage":{"input_tokens":12,"output_tokens":9}
		}`)
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "tp", "us-east5")
	p.Endpoint = apiSrv.URL + "/x"

	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model: "claude-opus-4-7@20251031",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "weather?"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "calling tool" {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.ID != "tu_1" || tc.Name != "weather" {
		t.Errorf("tool = %+v", tc)
	}
	if !bytes.Contains(tc.Input, []byte(`"Istanbul"`)) {
		t.Errorf("tool input = %s, missing Istanbul", string(tc.Input))
	}
	if resp.StopReason != agent.StopToolUse {
		t.Errorf("stop = %q, want %q", resp.StopReason, agent.StopToolUse)
	}
}

func TestComplete_GeminiModelStillRoutesToGenerateContent(t *testing.T) {
	// Regression guard: adding the Anthropic branch must not divert
	// `gemini-*` model ids. They must still hit the existing
	// :generateContent endpoint with the Gemini body shape.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "x", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	var seenPath string
	var seenBody map[string]any
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&seenBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content":      map[string]any{"role": "model", "parts": []map[string]any{{"text": "gemini"}}},
				"finishReason": "STOP",
			}},
		})
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "tp", "us-central1")
	p.Endpoint = apiSrv.URL + "/v1/projects/tp/locations/us-central1/publishers/google/models/gemini-1.5-flash:generateContent"

	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gemini-1.5-flash",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "gemini" {
		t.Errorf("content = %q (gemini decode lost)", resp.Message.Content)
	}
	// Gemini body uses `contents` array, NOT `messages`.
	if _, ok := seenBody["contents"]; !ok {
		t.Errorf("gemini body should have `contents`; got %v", seenBody)
	}
	if _, ok := seenBody["messages"]; ok {
		t.Errorf("gemini body must NOT have `messages` (that's the Anthropic shape); got %v", seenBody)
	}
	if !strings.Contains(seenPath, ":generateContent") {
		t.Errorf("path = %q, want :generateContent suffix", seenPath)
	}
}

// ---- streaming ----

// buildAnthSSE assembles a tiny event-tagged Anthropic SSE stream.
func buildAnthSSE(frames ...[2]string) string {
	var sb strings.Builder
	for _, f := range frames {
		sb.WriteString("event: ")
		sb.WriteString(f[0])
		sb.WriteString("\ndata: ")
		sb.WriteString(f[1])
		sb.WriteString("\n\n")
	}
	return sb.String()
}

func TestCompleteStream_AnthropicAssemblesText(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "y", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	stream := buildAnthSSE(
		[2]string{"message_start", `{"message":{"id":"m1","model":"claude-opus-4-7","usage":{"input_tokens":4,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"index":0,"delta":{"type":"text_delta","text":"Merhaba"}}`},
		[2]string{"content_block_delta", `{"index":0,"delta":{"type":"text_delta","text":", dunya"}}`},
		[2]string{"content_block_stop", `{"index":0}`},
		[2]string{"message_delta", `{"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9}}`},
		[2]string{"message_stop", `{}`},
	)

	var seenPath, seenAccept string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, stream)
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "tp", "us-east5")
	p.Endpoint = apiSrv.URL + "/v1/projects/tp/locations/us-east5/publishers/anthropic/models/claude-opus-4-7@20251031:streamRawPredict"

	var got strings.Builder
	resp, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "claude-opus-4-7@20251031"},
		func(c agent.Chunk) error {
			got.WriteString(c.TextDelta)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if got.String() != "Merhaba, dunya" {
		t.Errorf("streamed = %q, want %q", got.String(), "Merhaba, dunya")
	}
	if resp.Message.Content != "Merhaba, dunya" {
		t.Errorf("assembled = %q", resp.Message.Content)
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop = %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 9 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if seenAccept != "text/event-stream" {
		t.Errorf("Accept = %q, want text/event-stream", seenAccept)
	}
	if !strings.Contains(seenPath, "publishers/anthropic/") || !strings.Contains(seenPath, ":streamRawPredict") {
		t.Errorf("path = %q, want anthropic publisher + :streamRawPredict suffix", seenPath)
	}
}

func TestCompleteStream_AnthropicToolUse(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "x", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	stream := buildAnthSSE(
		[2]string{"message_start", `{"message":{"id":"m1","model":"claude-opus-4-7","usage":{"input_tokens":3,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"index":0,"content_block":{"type":"tool_use","id":"tu_1","name":"weather"}}`},
		[2]string{"content_block_delta", `{"index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\""}}`},
		[2]string{"content_block_delta", `{"index":0,"delta":{"type":"input_json_delta","partial_json":"Ankara\"}"}}`},
		[2]string{"content_block_stop", `{"index":0}`},
		[2]string{"message_delta", `{"delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`},
		[2]string{"message_stop", `{}`},
	)

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, stream)
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "tp", "us-east5")
	p.Endpoint = apiSrv.URL + "/x"

	var (
		starts int
		stops  int
		input  strings.Builder
	)
	resp, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "claude-sonnet-4-5@20250929"},
		func(c agent.Chunk) error {
			if c.ToolUseStart != nil {
				starts++
			}
			if c.ToolUseStop != "" {
				stops++
			}
			if c.ToolInputJSONDelta != "" {
				input.WriteString(c.ToolInputJSONDelta)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if starts != 1 || stops != 1 {
		t.Errorf("ToolUseStart=%d Stop=%d, want 1/1", starts, stops)
	}
	if input.String() != `{"city":"Ankara"}` {
		t.Errorf("streamed input = %q, want %q", input.String(), `{"city":"Ankara"}`)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len=%d", len(resp.Message.ToolCalls))
	}
	if string(resp.Message.ToolCalls[0].Input) != `{"city":"Ankara"}` {
		t.Errorf("assembled tool input = %s", string(resp.Message.ToolCalls[0].Input))
	}
	if resp.StopReason != agent.StopToolUse {
		t.Errorf("stop = %q", resp.StopReason)
	}
}

func TestCompleteStream_GeminiModelStillUsesGeminiStreamPath(t *testing.T) {
	// Regression guard symmetric to TestComplete_GeminiModelStillRoutesToGenerateContent:
	// adding the Anthropic streaming branch must not divert gemini-* ids.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "x", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	var seenPath string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		// Gemini SSE frame.
		_, _ = io.WriteString(w, "data: "+`{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP"}]}`+"\n\n")
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "tp", "us-central1")
	p.Endpoint = apiSrv.URL + "/v1/projects/tp/locations/us-central1/publishers/google/models/gemini-1.5-flash:streamGenerateContent?alt=sse"

	var got strings.Builder
	_, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "gemini-1.5-flash"},
		func(c agent.Chunk) error {
			got.WriteString(c.TextDelta)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if got.String() != "hi" {
		t.Errorf("streamed = %q (gemini decode lost)", got.String())
	}
	if !strings.Contains(seenPath, "publishers/google/") {
		t.Errorf("path = %q, gemini should route to publishers/google/", seenPath)
	}
}

func TestCompleteStream_AnthropicSurfacesAPIError(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "x", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":429,"message":"slow down"}}`)
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "tp", "us-east5")
	p.Endpoint = apiSrv.URL + "/x"

	_, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "claude-opus-4-7"},
		func(agent.Chunk) error { return nil },
	)
	apiErr, ok := err.(*vertex.APIError)
	if !ok {
		t.Fatalf("err = %T (%v), want *vertex.APIError", err, err)
	}
	if apiErr.Status != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", apiErr.Status)
	}
}
