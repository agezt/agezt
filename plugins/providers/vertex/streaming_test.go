// SPDX-License-Identifier: MIT

package vertex_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/vertex"
)

// sampleVertexTextStream is a representative text-only SSE response
// from Vertex's :streamGenerateContent endpoint. Same shape as the
// Generative Language API but routed via the regional Vertex host.
const sampleVertexTextStream = `data: {"candidates":[{"content":{"parts":[{"text":"hello"}],"role":"model"}}]}

data: {"candidates":[{"content":{"parts":[{"text":" from"}],"role":"model"}}]}

data: {"candidates":[{"content":{"parts":[{"text":" vertex"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":3}}

`

const sampleVertexToolCallStream = `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"shell","args":{"command":"ls"}}}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":12}}

`

// streamingTokenSrv returns an httptest server that handles the OAuth
// token exchange by minting a static access token. Keeps streaming
// tests focused on the streaming-specific behavior, since the OAuth
// path is exhaustively covered by vertex_test.go's TokenSource tests.
func streamingTokenSrv(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.streaming",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	}))
}

func TestCompleteStream_Vertex_TextEndToEnd(t *testing.T) {
	tokSrv := streamingTokenSrv(t)
	defer tokSrv.Close()

	var sawAuth, sawAccept, sawPath string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawAccept = r.Header.Get("Accept")
		sawPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(sampleVertexTextStream))
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "test-project", "us-central1")
	p.Endpoint = apiSrv.URL + "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-1.5-flash:streamGenerateContent?alt=sse"

	var got strings.Builder
	resp, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Model:    "gemini-1.5-flash",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "Say hi"}},
	}, func(c agent.Chunk) error {
		got.WriteString(c.TextDelta)
		return nil
	})
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if got.String() != "hello from vertex" {
		t.Errorf("streamed = %q, want 'hello from vertex'", got.String())
	}
	if resp.Message.Content != "hello from vertex" {
		t.Errorf("resp.Content = %q", resp.Message.Content)
	}
	if resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if sawAuth != "Bearer ya29.streaming" {
		t.Errorf("auth header = %q, want 'Bearer ya29.streaming'", sawAuth)
	}
	if sawAccept != "text/event-stream" {
		t.Errorf("accept = %q", sawAccept)
	}
	if !strings.Contains(sawPath, ":streamGenerateContent") {
		t.Errorf("path = %q, expected :streamGenerateContent", sawPath)
	}
}

func TestCompleteStream_Vertex_ToolCallLifecycle(t *testing.T) {
	tokSrv := streamingTokenSrv(t)
	defer tokSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(sampleVertexToolCallStream))
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "test-project", "us-central1")
	p.Endpoint = apiSrv.URL + "/anything"

	var (
		gotStart   *agent.ToolCall
		gotInput   string
		gotStop    string
		startCount int
		stopCount  int
	)
	resp, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Model:    "gemini-1.5-pro",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "list files"}},
	}, func(c agent.Chunk) error {
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
		t.Fatalf("CompleteStream: %v", err)
	}
	if startCount != 1 || stopCount != 1 {
		t.Errorf("lifecycle: start=%d stop=%d, want 1/1", startCount, stopCount)
	}
	if gotStart == nil || gotStart.Name != "shell" {
		t.Errorf("ToolUseStart wrong: %+v", gotStart)
	}
	if gotStart != nil && gotStart.ID != gotStop {
		t.Errorf("start.ID=%q != stop.ID=%q", gotStart.ID, gotStop)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(gotInput), &parsed); err != nil {
		t.Errorf("ToolInputJSONDelta not valid JSON: %v (%q)", err, gotInput)
	}
	if parsed["command"] != "ls" {
		t.Errorf("args = %v", parsed)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
	if resp.StopReason != agent.StopToolUse {
		t.Errorf("stop = %q, want tool_use (must override finishReason=STOP when tools present)", resp.StopReason)
	}
}

func TestCompleteStream_Vertex_HTTPError(t *testing.T) {
	tokSrv := streamingTokenSrv(t)
	defer tokSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":{"message":"vertex permission denied"}}`))
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "test-project", "us-central1")
	p.Endpoint = apiSrv.URL + "/anything"

	_, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}, func(c agent.Chunk) error { return nil })
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*vertex.APIError)
	if !ok || apiErr.Status != 403 {
		t.Errorf("got %v, want APIError{Status:403}", err)
	}
	if !strings.Contains(apiErr.Body, "vertex permission denied") {
		t.Errorf("body lost: %s", apiErr.Body)
	}
}

func TestCompleteStream_Vertex_NoTokenSource(t *testing.T) {
	p := vertex.New(nil, "p", "us-central1")
	_, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}, func(c agent.Chunk) error { return nil })
	if err != vertex.ErrNoTokenSource {
		t.Errorf("got %v, want ErrNoTokenSource", err)
	}
}

func TestCompleteStream_Vertex_NilOnChunkRejected(t *testing.T) {
	tokSrv := streamingTokenSrv(t)
	defer tokSrv.Close()
	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "p", "us-central1")
	_, err := p.CompleteStream(context.Background(), agent.CompletionRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "non-nil onChunk") {
		t.Errorf("got %v, want nil-callback rejection", err)
	}
}

func TestResolveStreamEndpoint_Vertex(t *testing.T) {
	cases := []struct {
		name             string
		baseURL, project string
		location, model  string
		endpoint         string
		want             string
	}{
		{
			name:     "default regional host",
			project:  "test-project",
			location: "us-central1",
			model:    "gemini-1.5-flash",
			want:     "https://us-central1-aiplatform.googleapis.com/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-1.5-flash:streamGenerateContent?alt=sse",
		},
		{
			name:     "europe region",
			project:  "p",
			location: "europe-west4",
			model:    "gemini-1.5-pro",
			want:     "https://europe-west4-aiplatform.googleapis.com/v1/projects/p/locations/europe-west4/publishers/google/models/gemini-1.5-pro:streamGenerateContent?alt=sse",
		},
		{
			name:     "custom baseurl (VPC service-control alias)",
			baseURL:  "https://aiplatform-us-central1.private.example.com",
			project:  "p",
			location: "us-central1",
			model:    "gemini-1.5-flash",
			want:     "https://aiplatform-us-central1.private.example.com/v1/projects/p/locations/us-central1/publishers/google/models/gemini-1.5-flash:streamGenerateContent?alt=sse",
		},
		{
			name:     "explicit endpoint wins",
			endpoint: "http://test/anything",
			project:  "p",
			location: "us-central1",
			model:    "m",
			want:     "http://test/anything",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &vertex.Provider{
				BaseURL:  c.baseURL,
				Endpoint: c.endpoint,
				Project:  c.project,
				Location: c.location,
			}
			if got := p.ResolveStreamEndpoint(c.model); got != c.want {
				t.Errorf("\n got %q\nwant %q", got, c.want)
			}
		})
	}
}

// Compile-time guard — *Provider must satisfy StreamingProvider.
var _ agent.StreamingProvider = (*vertex.Provider)(nil)
