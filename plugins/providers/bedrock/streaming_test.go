// SPDX-License-Identifier: MIT

package bedrock_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ersinkoc/agezt/kernel/agent"
	"github.com/ersinkoc/agezt/plugins/providers/bedrock"
)

// ---- event-stream framing helpers (synthesize what AWS would send) ----

// buildEventStreamFrame assembles one binary-framed message. CRCs are
// written as zeros — bedrock.parseEventStream doesn't validate them
// (documented as a deferral in streaming.go), so test fixtures don't
// have to compute IEEE-CRC32 on a polynomial-sensitive bit layout.
func buildEventStreamFrame(t *testing.T, headers map[string]string, payload []byte) []byte {
	t.Helper()
	// Encode headers in deterministic order so tests are stable.
	// AWS doesn't require a specific header order on the wire, but
	// stable order makes hex dumps in test failures easier to read.
	var hdr bytes.Buffer
	keys := sortedKeys(headers)
	for _, k := range keys {
		v := headers[k]
		if len(k) > 255 {
			t.Fatalf("header name %q too long for tests", k)
		}
		hdr.WriteByte(byte(len(k)))
		hdr.WriteString(k)
		hdr.WriteByte(7) // type 7 = string
		var vlen [2]byte
		binary.BigEndian.PutUint16(vlen[:], uint16(len(v)))
		hdr.Write(vlen[:])
		hdr.WriteString(v)
	}
	hdrBytes := hdr.Bytes()
	totalLen := uint32(12 + len(hdrBytes) + len(payload) + 4)

	var out bytes.Buffer
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], totalLen)
	out.Write(u32[:])
	binary.BigEndian.PutUint32(u32[:], uint32(len(hdrBytes)))
	out.Write(u32[:])
	// Prelude CRC (4 bytes of zero — not validated).
	out.Write(make([]byte, 4))
	out.Write(hdrBytes)
	out.Write(payload)
	// Message CRC (4 bytes of zero — not validated).
	out.Write(make([]byte, 4))
	return out.Bytes()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// avoid pulling in sort: tiny manual insertion sort
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}

// chunkFrame wraps an Anthropic-shaped inner event JSON in the
// Bedrock chunk envelope + binary framing.
func chunkFrame(t *testing.T, innerJSON string) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"bytes": base64.StdEncoding.EncodeToString([]byte(innerJSON)),
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return buildEventStreamFrame(t, map[string]string{
		":message-type": "event",
		":event-type":   "chunk",
		":content-type": "application/json",
	}, payload)
}

// buildTextStream is a typical Anthropic streaming session: start,
// text block with two deltas, stop, message_delta with usage, stop.
func buildTextStream(t *testing.T) []byte {
	t.Helper()
	frames := [][]byte{
		chunkFrame(t, `{"type":"message_start","message":{"id":"m1","model":"claude-opus-4-7","usage":{"input_tokens":7,"output_tokens":0}}}`),
		chunkFrame(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		chunkFrame(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`),
		chunkFrame(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", world"}}`),
		chunkFrame(t, `{"type":"content_block_stop","index":0}`),
		chunkFrame(t, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":12}}`),
		chunkFrame(t, `{"type":"message_stop"}`),
	}
	var buf bytes.Buffer
	for _, f := range frames {
		buf.Write(f)
	}
	return buf.Bytes()
}

// ---- tests ----

func TestCompleteStream_AssemblesTextResponse(t *testing.T) {
	stream := buildTextStream(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/invoke-with-response-stream") {
			t.Errorf("expected /invoke-with-response-stream suffix, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-bearer" {
			t.Errorf("missing bearer: %q", r.Header.Get("Authorization"))
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.amazon.eventstream" {
			t.Errorf("Accept = %q, want application/vnd.amazon.eventstream", got)
		}
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(stream)
	}))
	defer srv.Close()

	p := &bedrock.Provider{
		BearerToken: "test-bearer",
		Endpoint:    srv.URL + "/model/anthropic.claude-opus-4-7/invoke-with-response-stream",
		HTTP:        srv.Client(),
	}

	var got strings.Builder
	resp, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "anthropic.claude-opus-4-7"},
		func(c agent.Chunk) error {
			got.WriteString(c.TextDelta)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if got.String() != "Hello, world" {
		t.Errorf("streamed text = %q, want %q", got.String(), "Hello, world")
	}
	if resp.Message.Content != "Hello, world" {
		t.Errorf("assembled text = %q, want %q", resp.Message.Content, "Hello, world")
	}
	if resp.StopReason != agent.StopEndTurn {
		t.Errorf("stop = %q, want %q", resp.StopReason, agent.StopEndTurn)
	}
	if resp.Usage.InputTokens != 7 || resp.Usage.OutputTokens != 12 {
		t.Errorf("usage = %+v, want in=7 out=12", resp.Usage)
	}
	if resp.Usage.Model != "anthropic.claude-opus-4-7" {
		t.Errorf("model = %q, want anthropic.claude-opus-4-7", resp.Usage.Model)
	}
}

func TestCompleteStream_AssemblesToolCall(t *testing.T) {
	// Same shape, but a tool_use block streams the input JSON across
	// two input_json_delta chunks before content_block_stop.
	frames := [][]byte{
		chunkFrame(t, `{"type":"message_start","message":{"id":"m1","model":"anthropic.claude-opus-4-7","usage":{"input_tokens":5,"output_tokens":0}}}`),
		chunkFrame(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"weather"}}`),
		chunkFrame(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\""}}`),
		chunkFrame(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"Istanbul\"}"}}`),
		chunkFrame(t, `{"type":"content_block_stop","index":0}`),
		chunkFrame(t, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`),
		chunkFrame(t, `{"type":"message_stop"}`),
	}
	var stream bytes.Buffer
	for _, f := range frames {
		stream.Write(f)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(stream.Bytes())
	}))
	defer srv.Close()

	p := &bedrock.Provider{
		BearerToken: "test-bearer",
		Endpoint:    srv.URL + "/model/anthropic.claude-opus-4-7/invoke-with-response-stream",
		HTTP:        srv.Client(),
	}

	var (
		gotStarts int
		gotStops  int
		gotJSON   strings.Builder
	)
	resp, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "anthropic.claude-opus-4-7"},
		func(c agent.Chunk) error {
			if c.ToolUseStart != nil {
				gotStarts++
				if c.ToolUseStart.ID != "tool_1" || c.ToolUseStart.Name != "weather" {
					t.Errorf("ToolUseStart = %+v", c.ToolUseStart)
				}
			}
			if c.ToolInputJSONDelta != "" {
				gotJSON.WriteString(c.ToolInputJSONDelta)
			}
			if c.ToolUseStop != "" {
				gotStops++
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if gotStarts != 1 {
		t.Errorf("ToolUseStart count = %d, want 1", gotStarts)
	}
	if gotStops != 1 {
		t.Errorf("ToolUseStop count = %d, want 1", gotStops)
	}
	if gotJSON.String() != `{"city":"Istanbul"}` {
		t.Errorf("streamed JSON = %q, want %q", gotJSON.String(), `{"city":"Istanbul"}`)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len=%d want 1", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.ID != "tool_1" || tc.Name != "weather" {
		t.Errorf("tool call = %+v", tc)
	}
	if string(tc.Input) != `{"city":"Istanbul"}` {
		t.Errorf("tool input = %s, want {\"city\":\"Istanbul\"}", string(tc.Input))
	}
	if resp.StopReason != agent.StopToolUse {
		t.Errorf("stop = %q, want %q", resp.StopReason, agent.StopToolUse)
	}
}

func TestCompleteStream_RejectsNonAnthropicModel(t *testing.T) {
	p := bedrock.New("test", "us-east-1")
	_, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "meta.llama3-70b-instruct-v1:0"},
		func(agent.Chunk) error { return nil },
	)
	if err == nil {
		t.Fatal("expected error for non-anthropic model")
	}
	if !strings.Contains(err.Error(), "not in the anthropic.* family") {
		t.Errorf("error = %v, want vendor-unsupported message", err)
	}
}

func TestCompleteStream_RejectsMissingBearer(t *testing.T) {
	p := &bedrock.Provider{Region: "us-east-1"}
	_, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "anthropic.claude-opus-4-7"},
		func(agent.Chunk) error { return nil },
	)
	if err != bedrock.ErrNoBearerToken {
		t.Errorf("err = %v, want ErrNoBearerToken", err)
	}
}

func TestCompleteStream_RejectsNilOnChunk(t *testing.T) {
	p := &bedrock.Provider{BearerToken: "x", Region: "us-east-1"}
	_, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "anthropic.claude-opus-4-7"},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "non-nil onChunk") {
		t.Errorf("err = %v, want nil-onChunk message", err)
	}
}

func TestCompleteStream_SurfacesAPIErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message":"bad request"}`)
	}))
	defer srv.Close()

	p := &bedrock.Provider{
		BearerToken: "x",
		Endpoint:    srv.URL + "/model/anthropic.claude-opus-4-7/invoke-with-response-stream",
		HTTP:        srv.Client(),
	}
	_, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "anthropic.claude-opus-4-7"},
		func(agent.Chunk) error { return nil },
	)
	apiErr, ok := err.(*bedrock.APIError)
	if !ok {
		t.Fatalf("err = %T (%v), want *bedrock.APIError", err, err)
	}
	if apiErr.Status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", apiErr.Status)
	}
}

func TestCompleteStream_SurfacesExceptionFrame(t *testing.T) {
	// AWS-modelled exception: :message-type=exception with
	// :exception-type naming the class. The payload is JSON detail.
	frame := buildEventStreamFrame(t, map[string]string{
		":message-type":   "exception",
		":exception-type": "ThrottlingException",
		":content-type":   "application/json",
	}, []byte(`{"message":"slow down"}`))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(frame)
	}))
	defer srv.Close()

	p := &bedrock.Provider{
		BearerToken: "x",
		Endpoint:    srv.URL + "/model/anthropic.claude-opus-4-7/invoke-with-response-stream",
		HTTP:        srv.Client(),
	}
	_, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "anthropic.claude-opus-4-7"},
		func(agent.Chunk) error { return nil },
	)
	if err == nil {
		t.Fatal("expected error from exception frame")
	}
	if !strings.Contains(err.Error(), "ThrottlingException") || !strings.Contains(err.Error(), "slow down") {
		t.Errorf("err = %v, want ThrottlingException + slow down", err)
	}
}

func TestCompleteStream_ReturnsPartialOnEOF(t *testing.T) {
	// Truncated stream — message_start + delta + EOF (no
	// message_stop). The parser should not error; the caller sees
	// whatever text already streamed.
	frames := [][]byte{
		chunkFrame(t, `{"type":"message_start","message":{"id":"m1","model":"anthropic.claude-opus-4-7","usage":{"input_tokens":3,"output_tokens":0}}}`),
		chunkFrame(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		chunkFrame(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`),
	}
	var stream bytes.Buffer
	for _, f := range frames {
		stream.Write(f)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(stream.Bytes())
	}))
	defer srv.Close()

	p := &bedrock.Provider{
		BearerToken: "x",
		Endpoint:    srv.URL + "/model/anthropic.claude-opus-4-7/invoke-with-response-stream",
		HTTP:        srv.Client(),
	}
	var got strings.Builder
	resp, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "anthropic.claude-opus-4-7"},
		func(c agent.Chunk) error {
			got.WriteString(c.TextDelta)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("expected no error on EOF mid-stream, got %v", err)
	}
	if got.String() != "partial" {
		t.Errorf("streamed text = %q, want %q", got.String(), "partial")
	}
	// Inner text block never closed → assembled Content stays empty
	// (the open-block buffer is flushed only on content_block_stop).
	// This is the same behaviour as the direct Anthropic adapter.
	if resp.Message.Content != "" {
		t.Errorf("assembled content = %q, want empty (block never closed)", resp.Message.Content)
	}
}

func TestCompleteStream_RejectsNonStringHeader(t *testing.T) {
	// Hand-craft a frame where the value type is 8 (timestamp) instead
	// of 7 (string). The parser must surface that as an error rather
	// than silently skipping — a header we'd normally read (like
	// :event-type) could be hidden behind a type drift.
	var hdr bytes.Buffer
	name := ":custom"
	hdr.WriteByte(byte(len(name)))
	hdr.WriteString(name)
	hdr.WriteByte(8)                          // type 8 = timestamp
	hdr.Write(make([]byte, 8))                // 8-byte body
	hdrBytes := hdr.Bytes()
	payload := []byte("{}")
	totalLen := uint32(12 + len(hdrBytes) + len(payload) + 4)
	var out bytes.Buffer
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], totalLen)
	out.Write(u32[:])
	binary.BigEndian.PutUint32(u32[:], uint32(len(hdrBytes)))
	out.Write(u32[:])
	out.Write(make([]byte, 4))
	out.Write(hdrBytes)
	out.Write(payload)
	out.Write(make([]byte, 4))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out.Bytes())
	}))
	defer srv.Close()

	p := &bedrock.Provider{
		BearerToken: "x",
		Endpoint:    srv.URL + "/model/anthropic.claude-opus-4-7/invoke-with-response-stream",
		HTTP:        srv.Client(),
	}
	_, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "anthropic.claude-opus-4-7"},
		func(agent.Chunk) error { return nil },
	)
	if err == nil {
		t.Fatal("expected error for unsupported header type")
	}
	if !strings.Contains(err.Error(), "unsupported header value type") {
		t.Errorf("err = %v, want unsupported-header-type message", err)
	}
}

func TestCompleteStream_OnChunkErrorPropagates(t *testing.T) {
	stream := buildTextStream(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(stream)
	}))
	defer srv.Close()

	p := &bedrock.Provider{
		BearerToken: "x",
		Endpoint:    srv.URL + "/model/anthropic.claude-opus-4-7/invoke-with-response-stream",
		HTTP:        srv.Client(),
	}
	want := io.ErrClosedPipe // a sentinel — could be anything
	_, err := p.CompleteStream(
		context.Background(),
		agent.CompletionRequest{Model: "anthropic.claude-opus-4-7"},
		func(c agent.Chunk) error {
			if c.TextDelta != "" {
				return want
			}
			return nil
		},
	)
	if err == nil {
		t.Fatal("expected onChunk error to propagate")
	}
	// The error is wrapped only when it comes from a JSON parse step;
	// onChunk errors propagate verbatim through dispatchBedrockInnerEvent.
	if err != want && !strings.Contains(err.Error(), want.Error()) {
		t.Errorf("err = %v, want %v (or wrapped)", err, want)
	}
}
