// SPDX-License-Identifier: MIT

// Bedrock streaming: AWS event-stream protocol parser + inner-event dispatcher.
// Code extracted from streaming.go during the Day-108 god-file split.
// Public API unchanged.
package bedrock


import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
	"github.com/agezt/agezt/plugins/providers/internal/retry"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
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
	body, err := encodeAnthropicOnBedrockRequest(req.System, req.Messages, req.Tools, maxTokens, req.Params, req.ProviderOptions["bedrock"])
	if err != nil {
		return nil, fmt.Errorf("bedrock: encode request: %w", err)
	}

	endpoint := resolveStreamEndpoint(p, model)
	// Stream SETUP retries transient failures (connection errors, 429/5xx)
	// before the first frame; mid-stream failures are never replayed (LD-4).
	// build runs per attempt so the SigV4 signature is re-computed fresh.
	httpResp, err := retry.DoHTTPStream(ctx, p.HTTP, func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("bedrock: build request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/vnd.amazon.eventstream")
		if err := p.applyAuth(httpReq, body); err != nil {
			return nil, err
		}
		return httpReq, nil
	}, httpread.DefaultMaxResponseBytes)
	if err != nil {
		var h *retry.HTTPError
		if errors.As(err, &h) {
			return nil, &APIError{Status: h.StatusCode, Body: h.Body}
		}
		return nil, fmt.Errorf("bedrock: http: %w", err)
	}
	defer httpResp.Body.Close()

	resp, err := parseEventStream(httpResp.Body, model, onChunk)
	if err != nil {
		return nil, err
	}
	toolname.RestoreCalls(resp, toolname.Reverse(req.Tools))
	return resp, nil
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

