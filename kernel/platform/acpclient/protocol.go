// SPDX-License-Identifier: MIT

package acpclient

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
)

// MaxMessageBytes bounds a single ACP JSON-RPC message. ACP is newline-
// delimited JSON (both ends write via json.Encoder, which appends a
// newline), so a bounded line scanner enforces a per-message cap that
// json.Decoder would not — one oversized message from a misbehaving peer
// can't balloon memory (M257).
const MaxMessageBytes = 8 << 20

// ProtocolVersion is the ACP version this implementation speaks.
const ProtocolVersion = 1

// --- JSON-RPC wire types ---

// Request is an incoming JSON-RPC request or notification (a notification
// carries no id).
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // absent for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is an outgoing JSON-RPC response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Notification is an outgoing JSON-RPC notification.
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// RPCError is the JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewBoundedScanner returns a line scanner whose token (one JSON-RPC message)
// is capped at MaxMessageBytes; an over-cap line surfaces as
// bufio.ErrTooLong.
func NewBoundedScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MaxMessageBytes)
	return sc
}

// ScanMessage reads the next non-blank newline-delimited message from sc into
// v, returning io.EOF at a clean end of stream.
func ScanMessage(sc *bufio.Scanner, v any) error {
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		return json.Unmarshal(line, v)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}
