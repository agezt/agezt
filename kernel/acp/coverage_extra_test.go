// SPDX-License-Identifier: MIT

package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// errWriter fails on every Write, exercising the writeMessage error path where
// the encoder returns a write error (the branch after the mutex is taken).
type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) { return 0, errors.New("boom") }

// runRaw feeds raw newline-delimited JSON text (not re-encoded) so we can send
// deliberately malformed params, and returns decoded reply lines.
func runRaw(t *testing.T, runner Runner, raw string) []map[string]any {
	t.Helper()
	in := strings.NewReader(raw)
	var out bytes.Buffer
	s := New(runner, in, &out)
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("unmarshal %q: %v", line, err)
		}
		lines = append(lines, m)
	}
	return lines
}

// TestServeDecodeErrorTooLong exercises the non-EOF decode error branch in
// Serve: an over-cap line surfaces as bufio.ErrTooLong, which Serve wraps and
// returns (rather than treating as a clean EOF).
func TestServeDecodeErrorTooLong(t *testing.T) {
	big := strings.Repeat("a", maxMessageBytes+1) + "\n"
	s := New(&fakeRunner{}, strings.NewReader(big), &bytes.Buffer{})
	err := s.Serve(context.Background())
	if err == nil {
		t.Fatal("expected decode error for over-cap message, got nil")
	}
	if !strings.Contains(err.Error(), "acp: decode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestServeContextCancelled exercises the ctx.Err() branch at the top of Serve.
func TestServeContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Provide input so the loop would otherwise proceed; the ctx check must win.
	s := New(&fakeRunner{}, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`+"\n"), &bytes.Buffer{})
	if err := s.Serve(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestHandleNewSessionInvalidParams exercises the json.Unmarshal error branch.
func TestHandleNewSessionInvalidParams(t *testing.T) {
	out := runRaw(t, &fakeRunner{},
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":123}`+"\n")
	if len(out) != 1 {
		t.Fatalf("want 1 reply, got %d: %v", len(out), out)
	}
	errObj, ok := out[0]["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error reply, got %v", out[0])
	}
	if code := errObj["code"].(float64); int(code) != codeInvalidParams {
		t.Fatalf("want code %d, got %v", codeInvalidParams, code)
	}
}

// TestHandlePromptInvalidParams exercises the json.Unmarshal error branch.
func TestHandlePromptInvalidParams(t *testing.T) {
	out := runRaw(t, &fakeRunner{},
		`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":"notanobject"}`+"\n")
	if len(out) != 1 {
		t.Fatalf("want 1 reply, got %d: %v", len(out), out)
	}
	errObj, ok := out[0]["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error reply, got %v", out[0])
	}
	if code := errObj["code"].(float64); int(code) != codeInvalidParams {
		t.Fatalf("want code %d, got %v", codeInvalidParams, code)
	}
}

// TestHandlePromptUnknownSession exercises the session-not-found branch.
func TestHandlePromptUnknownSession(t *testing.T) {
	out := runRaw(t, &fakeRunner{},
		`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":{"sessionId":"nope","prompt":[]}}`+"\n")
	if len(out) != 1 {
		t.Fatalf("want 1 reply, got %d: %v", len(out), out)
	}
	if _, ok := out[0]["error"].(map[string]any); !ok {
		t.Fatalf("expected error reply for unknown session, got %v", out[0])
	}
}

// TestHandlePromptRunnerError exercises the runner-returns-error branch inside
// handlePrompt (after a successful session/new).
func TestHandlePromptRunnerError(t *testing.T) {
	runner := &fakeRunner{err: errors.New("runner failed")}
	out := run(t, runner,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "session/new", "params": map[string]any{"cwd": "/tmp"}},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/prompt", "params": map[string]any{
			"sessionId": "sess-1",
			"prompt":    []map[string]any{{"type": "text", "text": "hi"}},
		}},
	)
	// Second reply corresponds to the prompt; it must carry an error.
	last := out[len(out)-1]
	if _, ok := last["error"].(map[string]any); !ok {
		t.Fatalf("expected error reply from failing runner, got %v", last)
	}
}

// TestNotificationNoReply exercises the reply/replyError empty-id (notification)
// branches: initialize sent without an id, and an unknown method without id.
func TestNotificationNoReply(t *testing.T) {
	// initialize with no id -> reply() returns early (len(id)==0).
	out := runRaw(t, &fakeRunner{},
		`{"jsonrpc":"2.0","method":"initialize"}`+"\n"+
			`{"jsonrpc":"2.0","method":"totally/unknown"}`+"\n"+
			`{"jsonrpc":"2.0","method":"session/cancel"}`+"\n"+
			`{"jsonrpc":"2.0","method":"$/cancelNotification"}`+"\n")
	if len(out) != 0 {
		t.Fatalf("expected no replies for id-less requests, got %v", out)
	}
}

// TestWriteMessageWriterError exercises the writer-error path of writeMessage:
// the encoder Write fails, but the server must not panic and Serve returns nil
// on clean EOF.
func TestWriteMessageWriterError(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
	s := New(&fakeRunner{}, in, errWriter{})
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("Serve with failing writer: %v", err)
	}
}
