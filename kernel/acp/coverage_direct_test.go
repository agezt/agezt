// SPDX-License-Identifier: MIT

package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestReplyErrorEmptyID exercises the len(id)==0 early-return in replyError,
// which the JSON-RPC dispatch path never reaches (it guards on len(req.ID)>0),
// so we call it directly.
func TestReplyErrorEmptyID(t *testing.T) {
	var out bytes.Buffer
	s := New(&fakeRunner{}, strings.NewReader(""), &out)
	s.replyError(nil, codeInternal, "ignored")
	if out.Len() != 0 {
		t.Fatalf("expected no output for id-less error reply, got %q", out.String())
	}
}

// TestWriteMessageMarshalError exercises the json.Marshal error branch: a value
// containing a channel cannot be marshaled, so writeMessage must return without
// writing anything (and without panicking).
func TestWriteMessageMarshalError(t *testing.T) {
	var out bytes.Buffer
	s := New(&fakeRunner{}, strings.NewReader(""), &out)
	s.writeMessage(map[string]any{"bad": make(chan int)})
	if out.Len() != 0 {
		t.Fatalf("expected no output for unmarshalable message, got %q", out.String())
	}
}

// TestHandlePromptStreamsThoughtsAndChunks exercises the reasoning-chunk and
// message-chunk streaming branches of handlePrompt: the runner emits both a
// thought and a message chunk before the final answer, each producing a
// session/update notification.
func TestHandlePromptStreamsThoughtsAndChunks(t *testing.T) {
	runner := &fakeRunner{
		thoughts: []string{"thinking..."},
		chunks:   []string{"partial "},
		answer:   "done",
	}
	out := run(t, runner,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "session/new", "params": map[string]any{"cwd": "/w"}},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/prompt", "params": map[string]any{
			"sessionId": "sess-1",
			"prompt":    []map[string]any{{"type": "text", "text": "go"}},
		}},
	)
	// Expect: session/new reply, >=1 session/update notifications, prompt reply.
	var updates int
	for _, m := range out {
		if m["method"] == "session/update" {
			updates++
		}
	}
	if updates < 2 {
		t.Fatalf("expected at least 2 session/update notifications (thought+chunk), got %d in %v", updates, out)
	}
	if runner.gotIntent != "go" {
		t.Fatalf("runner got intent %q", runner.gotIntent)
	}
	if runner.gotCwd != "/w" {
		t.Fatalf("runner got cwd %q", runner.gotCwd)
	}
}

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
