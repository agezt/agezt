// SPDX-License-Identifier: MIT

package bedrock

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ersinkoc/agezt/kernel/agent"
)

// CompleteStream implements agent.StreamingProvider for Bedrock's
// `invoke-with-response-stream` endpoint.
//
// **Wire format (the new bit):** AWS uses its own binary framing —
// `application/vnd.amazon.eventstream` — *not* SSE. Each frame is:
//
//	[ total-len   uint32 BE ]
//	[ headers-len uint32 BE ]
//	[ prelude CRC uint32 BE ]   (covers the two length fields)
//	[ headers     N bytes    ]
//	[ payload     M bytes    ]   M = total-len - 12 - headers-len - 4
//	[ message CRC uint32 BE ]   (covers the entire frame minus itself)
//
// For Bedrock streams the headers we read are:
//
//	:message-type   = "event"        (normal) or "exception" / "error"
//	:event-type     = "chunk"        (always, for the normal payload)
//	:content-type   = "application/json"
//	:exception-type = "<name>"       (only when :message-type=exception)
//
// The payload of a "chunk" event is `{"bytes": "<base64>"}`; the
// base64-decoded bytes are *the same JSON Anthropic emits inside an
// SSE `data:` line* (no `data:` prefix, no `event:` line — Bedrock
// folds that into the binary framing headers). We dispatch on the
// JSON's `type` field, same as the direct anthropic adapter.
//
// CRC validation is **not** performed (deferred). The connection is
// HTTPS so transport corruption is already ruled out, and AWS's CRC
// is IEEE-CRC32 over a non-trivial bit layout; getting it wrong
// would reject valid streams. If a future incident shows malformed
// frames in the wild, add validation behind a flag rather than as a
// hard fail.
func (p *Provider) CompleteStream(ctx context.Context, req agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	if !p.hasAuth() {
		return nil, ErrNoBearerToken
	}
	if onChunk == nil {
		return nil, errors.New("bedrock: CompleteStream requires non-nil onChunk")
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		return nil, errors.New("bedrock: model id required (must be in CompletionRequest.Model or p.Model)")
	}
	if !isAnthropicModel(model) {
		return nil, fmt.Errorf("%w: model %q is not in the anthropic.* family (streaming covers anthropic on bedrock; other vendor body shapes land alongside SigV4)",
			ErrVendorUnsupported, model)
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}

	// Same body shape as non-streaming — Bedrock's anthropic path does
	// *not* take a `stream` field in the JSON. The streaming dispatch
	// is selected by the URL suffix `/invoke-with-response-stream`.
	body, err := encodeAnthropicOnBedrockRequest(req.System, req.Messages, req.Tools, maxTokens)
	if err != nil {
		return nil, fmt.Errorf("bedrock: encode request: %w", err)
	}

	endpoint := resolveStreamEndpoint(p, model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("bedrock: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/vnd.amazon.eventstream")
	if err := p.applyAuth(httpReq, body); err != nil {
		return nil, err
	}

	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("bedrock: http: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(httpResp.Body)
		return nil, &APIError{Status: httpResp.StatusCode, Body: string(raw)}
	}

	return parseEventStream(httpResp.Body, model, onChunk)
}

// resolveStreamEndpoint mirrors ResolveEndpoint but swaps the suffix.
// Pulled out as a free function (not a method) so the resolution
// logic stays trivially readable.
func resolveStreamEndpoint(p *Provider, model string) string {
	// Honour the explicit Endpoint override (used by tests with
	// httptest.NewServer) — the test's URL is the path it wants the
	// request to land on; we don't substitute the streaming suffix.
	if p.Endpoint != "" {
		return p.Endpoint
	}
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		base = "https://bedrock-runtime." + p.Region + ".amazonaws.com"
	}
	return base + "/model/" + model + "/invoke-with-response-stream"
}

// ----- event-stream binary framing -----

// eventStreamHeader is the parsed form of an event-stream header.
// Bedrock only emits string-typed headers for the metadata we care
// about, so we model the value as a string and reject other types.
type eventStreamHeader struct {
	Name  string
	Value string
}

// readEventStreamMessage reads exactly one binary-framed message
// from r. Returns io.EOF when r drains cleanly between frames.
func readEventStreamMessage(r io.Reader) (headers []eventStreamHeader, payload []byte, err error) {
	// Prelude: 12 bytes total (3 × uint32 BE).
	var prelude [12]byte
	if _, err := io.ReadFull(r, prelude[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			// Distinguish clean end-of-stream from a truncated prelude.
			// io.ReadFull returns ErrUnexpectedEOF when *some* bytes
			// were read; we surface both as EOF for the caller's loop
			// since either way there's nothing more to read.
			return nil, nil, io.EOF
		}
		return nil, nil, fmt.Errorf("read prelude: %w", err)
	}
	totalLen := binary.BigEndian.Uint32(prelude[0:4])
	headersLen := binary.BigEndian.Uint32(prelude[4:8])
	// prelude[8:12] is the prelude CRC — not validated (see CompleteStream comment).

	if totalLen < 16 {
		// Minimum: 12 prelude + 0 headers + 0 payload + 4 message CRC.
		return nil, nil, fmt.Errorf("event-stream frame too small (total=%d)", totalLen)
	}
	if headersLen > totalLen-16 {
		return nil, nil, fmt.Errorf("event-stream frame headers-len (%d) > frame body (%d)", headersLen, totalLen-16)
	}
	// Cap to refuse pathological frames (AWS frames are <1MB in practice).
	const maxFrame = 16 * 1024 * 1024
	if totalLen > maxFrame {
		return nil, nil, fmt.Errorf("event-stream frame too large (%d > %d)", totalLen, maxFrame)
	}

	rest := make([]byte, totalLen-12)
	if _, err := io.ReadFull(r, rest); err != nil {
		return nil, nil, fmt.Errorf("read frame body: %w", err)
	}
	hdrBytes := rest[:headersLen]
	payloadEnd := len(rest) - 4 // last 4 bytes are message CRC
	payload = rest[headersLen:payloadEnd]
	// rest[payloadEnd:] is the message CRC — not validated.

	headers, err = parseEventStreamHeaders(hdrBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse headers: %w", err)
	}
	return headers, payload, nil
}

// parseEventStreamHeaders walks the header block. Each header is:
//
//	[ name-len    uint8       ]
//	[ name        N bytes     ]
//	[ value-type  uint8       ]   we only handle type 7 (string)
//	[ value-len   uint16 BE   ]
//	[ value       M bytes     ]
//
// Non-string value types are surfaced as an error rather than
// silently skipped — Bedrock isn't expected to use them for the
// headers we read, and a silent skip would mask a future spec drift
// where AWS adds a binary header we'd actually want to inspect.
func parseEventStreamHeaders(buf []byte) ([]eventStreamHeader, error) {
	const headerTypeString = 7
	var out []eventStreamHeader
	i := 0
	for i < len(buf) {
		if i+1 > len(buf) {
			return nil, errors.New("truncated header name length")
		}
		nameLen := int(buf[i])
		i++
		if i+nameLen > len(buf) {
			return nil, errors.New("truncated header name")
		}
		name := string(buf[i : i+nameLen])
		i += nameLen
		if i+1 > len(buf) {
			return nil, errors.New("truncated header value type")
		}
		valueType := buf[i]
		i++
		if valueType != headerTypeString {
			return nil, fmt.Errorf("unsupported header value type %d for %q (only string=7 expected for Bedrock event-stream headers)", valueType, name)
		}
		if i+2 > len(buf) {
			return nil, errors.New("truncated header value length")
		}
		valueLen := int(binary.BigEndian.Uint16(buf[i : i+2]))
		i += 2
		if i+valueLen > len(buf) {
			return nil, errors.New("truncated header value")
		}
		value := string(buf[i : i+valueLen])
		i += valueLen
		out = append(out, eventStreamHeader{Name: name, Value: value})
	}
	return out, nil
}

// headerValue is a small helper because event-stream headers are an
// ordered slice (preserving wire order matters for some types of
// AWS framing) but our consumers just want a map lookup.
func headerValue(hdrs []eventStreamHeader, name string) string {
	for _, h := range hdrs {
		if h.Name == name {
			return h.Value
		}
	}
	return ""
}

// ----- inner Anthropic-shaped event dispatch -----
//
// Mirrors the dispatch logic in plugins/providers/anthropic/streaming.go.
// Duplicated rather than shared so Bedrock can evolve without
// dragging the direct-Anthropic adapter along (same rationale as the
// non-streaming body encode/decode).

type bedStreamState struct {
	textParts     strings.Builder
	openBlock     *bedOpenBlock
	finishedTools []agent.ToolCall
	inputTokens   int
	outputTokens  int
	stopReason    string
}

type bedOpenBlock struct {
	kind     string
	toolID   string
	toolName string
	textBuf  strings.Builder
	inputBuf strings.Builder
}

// chunkPayload is the JSON Bedrock emits as the payload of a "chunk"
// event. The bytes field is base64 of the actual Anthropic event JSON.
type chunkPayload struct {
	Bytes string `json:"bytes"`
}

// parseEventStream is the per-stream loop: read a frame, branch on
// :message-type, decode the inner Anthropic event, dispatch.
func parseEventStream(body io.Reader, model string, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	st := bedStreamState{}
	for {
		hdrs, payload, err := readEventStreamMessage(body)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("bedrock: %w", err)
		}

		msgType := headerValue(hdrs, ":message-type")
		switch msgType {
		case "event":
			// Normal data — wrap-decode the inner Anthropic event.
			var cp chunkPayload
			if err := json.Unmarshal(payload, &cp); err != nil {
				return nil, fmt.Errorf("bedrock: parse chunk envelope: %w", err)
			}
			inner, err := base64.StdEncoding.DecodeString(cp.Bytes)
			if err != nil {
				return nil, fmt.Errorf("bedrock: decode chunk bytes: %w", err)
			}
			stop, err := dispatchBedrockInnerEvent(inner, &st, onChunk)
			if err != nil {
				return nil, err
			}
			if stop {
				return assembleBedrockResponse(&st, model), nil
			}

		case "exception":
			// Modelled exception (validation, throttling). The
			// :exception-type header names the class; payload is JSON.
			excType := headerValue(hdrs, ":exception-type")
			return nil, fmt.Errorf("bedrock: stream exception (%s): %s", excType, string(payload))

		case "error":
			// AWS-internal error. Surface raw.
			errCode := headerValue(hdrs, ":error-code")
			errMsg := headerValue(hdrs, ":error-message")
			return nil, fmt.Errorf("bedrock: stream error (%s): %s", errCode, errMsg)

		default:
			// Unknown message-type: skip rather than fail (forward-compat).
		}
	}
	return assembleBedrockResponse(&st, model), nil
}

// dispatchBedrockInnerEvent handles the JSON Anthropic emits inside
// each "chunk" frame. Returns (stop, err) — stop=true when the
// inner event is message_stop and the outer loop should exit.
func dispatchBedrockInnerEvent(data []byte, st *bedStreamState, onChunk func(agent.Chunk) error) (bool, error) {
	// The inner event always has a "type" field discriminating the
	// payload shape. Peek it first.
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return false, fmt.Errorf("bedrock: parse inner event type: %w", err)
	}
	switch head.Type {
	case "message_start":
		var f struct {
			Message struct {
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal(data, &f); err != nil {
			return false, fmt.Errorf("bedrock: parse message_start: %w", err)
		}
		st.inputTokens = f.Message.Usage.InputTokens
		if f.Message.Usage.OutputTokens > 0 {
			st.outputTokens = f.Message.Usage.OutputTokens
		}

	case "content_block_start":
		var f struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type  string          `json:"type"`
				Text  string          `json:"text"`
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal(data, &f); err != nil {
			return false, fmt.Errorf("bedrock: parse content_block_start: %w", err)
		}
		st.openBlock = &bedOpenBlock{kind: f.ContentBlock.Type}
		switch f.ContentBlock.Type {
		case "text":
			st.openBlock.textBuf.WriteString(f.ContentBlock.Text)
			if f.ContentBlock.Text != "" {
				if err := onChunk(agent.Chunk{TextDelta: f.ContentBlock.Text}); err != nil {
					return false, err
				}
			}
		case "tool_use":
			st.openBlock.toolID = f.ContentBlock.ID
			st.openBlock.toolName = f.ContentBlock.Name
			start := &agent.ToolCall{
				ID:    f.ContentBlock.ID,
				Name:  f.ContentBlock.Name,
				Input: json.RawMessage(`{}`),
			}
			if err := onChunk(agent.Chunk{ToolUseStart: start}); err != nil {
				return false, err
			}
		}

	case "content_block_delta":
		var f struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(data, &f); err != nil {
			return false, fmt.Errorf("bedrock: parse content_block_delta: %w", err)
		}
		if st.openBlock == nil {
			return false, nil
		}
		switch f.Delta.Type {
		case "text_delta":
			st.openBlock.textBuf.WriteString(f.Delta.Text)
			if f.Delta.Text != "" {
				if err := onChunk(agent.Chunk{TextDelta: f.Delta.Text}); err != nil {
					return false, err
				}
			}
		case "input_json_delta":
			st.openBlock.inputBuf.WriteString(f.Delta.PartialJSON)
			if f.Delta.PartialJSON != "" {
				if err := onChunk(agent.Chunk{ToolInputJSONDelta: f.Delta.PartialJSON}); err != nil {
					return false, err
				}
			}
		}

	case "content_block_stop":
		if st.openBlock == nil {
			return false, nil
		}
		ob := st.openBlock
		switch ob.kind {
		case "text":
			st.textParts.WriteString(ob.textBuf.String())
		case "tool_use":
			input := strings.TrimSpace(ob.inputBuf.String())
			if input == "" {
				input = "{}"
			}
			st.finishedTools = append(st.finishedTools, agent.ToolCall{
				ID:    ob.toolID,
				Name:  ob.toolName,
				Input: json.RawMessage(input),
			})
			if err := onChunk(agent.Chunk{ToolUseStop: ob.toolID}); err != nil {
				return false, err
			}
		}
		st.openBlock = nil

	case "message_delta":
		var f struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(data, &f); err != nil {
			return false, fmt.Errorf("bedrock: parse message_delta: %w", err)
		}
		if f.Delta.StopReason != "" {
			st.stopReason = f.Delta.StopReason
		}
		if f.Usage.OutputTokens > 0 {
			st.outputTokens = f.Usage.OutputTokens
		}

	case "message_stop":
		return true, nil

	case "ping", "":
		// keep-alive; ignore.

	case "error":
		var f struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &f); err != nil {
			return false, fmt.Errorf("bedrock: inner error frame (unparseable): %s", string(data))
		}
		return false, fmt.Errorf("bedrock: stream error (%s): %s", f.Error.Type, f.Error.Message)
	}
	return false, nil
}

func assembleBedrockResponse(st *bedStreamState, model string) *agent.CompletionResponse {
	stop := agent.StopReason(st.stopReason)
	switch st.stopReason {
	case "end_turn", "stop_sequence":
		stop = agent.StopEndTurn
	case "tool_use":
		stop = agent.StopToolUse
	case "max_tokens":
		stop = agent.StopMaxTokens
	}
	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			Content:   st.textParts.String(),
			ToolCalls: st.finishedTools,
		},
		StopReason: stop,
		Usage: agent.Usage{
			InputTokens:  st.inputTokens,
			OutputTokens: st.outputTokens,
			Model:        model,
		},
	}
}
