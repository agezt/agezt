// SPDX-License-Identifier: MIT

package sdk

import (
	"encoding/json"
	"testing"
)

// TestToolCall_MalformedAndEmpty covers ToolCall's second guard — the
// json.Unmarshal error path and the empty-tool path — which the primary
// TestToolCall doesn't reach (it only exercises the kind/nil guard). A
// correct-kind event whose payload is invalid JSON, or whose "tool" is empty,
// must both yield ("", false).
func TestToolCall_MalformedAndEmpty(t *testing.T) {
	// Correct kind but the payload is not valid JSON → Unmarshal fails.
	if _, ok := ToolCall(&Event{Kind: "tool.invoked", Payload: json.RawMessage(`{not json`)}); ok {
		t.Error("malformed payload should yield false")
	}
	// Correct kind, valid JSON, but the tool name is empty.
	if _, ok := ToolCall(&Event{Kind: "tool.invoked", Payload: json.RawMessage(`{"tool":""}`)}); ok {
		t.Error("empty tool name should yield false")
	}
}

// TestTokenText_Malformed covers TokenText's Unmarshal-error branch: a valid
// llm.token kind with an unparseable payload must yield ("", false).
func TestTokenText_Malformed(t *testing.T) {
	if _, ok := TokenText(&Event{Kind: "llm.token", Payload: json.RawMessage(`{bad`)}); ok {
		t.Error("malformed token payload should yield false")
	}
}
