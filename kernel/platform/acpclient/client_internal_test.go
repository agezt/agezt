// SPDX-License-Identifier: MIT

package acpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// --- Client error-path coverage ---

// TestClientNewSessionUnmarshalError exercises NewSession's json.Unmarshal error
// branch: the agent replies to session/new with a result that is not an object,
// so decoding into the sessionId struct fails.
func TestClientNewSessionUnmarshalError(t *testing.T) {
	// Agent replies with result being a JSON string, not an object.
	agentOut := `{"jsonrpc":"2.0","id":1,"result":"not-an-object"}` + "\n"
	c := NewClient(strings.NewReader(agentOut), &bytes.Buffer{})
	_, err := c.NewSession(context.Background(), "/cwd")
	if err == nil {
		t.Fatal("expected unmarshal error, got nil")
	}
}

// TestClientCallErrorResponse exercises call's error-response branch: the agent
// returns a JSON-RPC error object, which call must surface as a Go error.
func TestClientCallErrorResponse(t *testing.T) {
	agentOut := `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"nope"}}` + "\n"
	c := NewClient(strings.NewReader(agentOut), &bytes.Buffer{})
	err := c.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected error from agent error-response, got nil")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("expected agent error message surfaced, got %v", err)
	}
}

// TestClientCallEOF exercises call's decode-error branch when the agent closes
// its stream without replying.
func TestClientCallEOF(t *testing.T) {
	c := NewClient(strings.NewReader(""), &bytes.Buffer{})
	if err := c.Initialize(context.Background()); err == nil {
		t.Fatal("expected error when agent stream is empty, got nil")
	}
}

// TestMustRawFallback exercises mustRaw: marshaling a normal value succeeds and
// returns valid json.RawMessage (the happy branch is used pervasively; here we
// assert it round-trips).
func TestMustRawFallback(t *testing.T) {
	raw := mustRaw(map[string]any{"k": "v"})
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("mustRaw produced invalid json: %v", err)
	}
	if m["k"] != "v" {
		t.Fatalf("unexpected mustRaw output: %s", raw)
	}
}
