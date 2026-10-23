// SPDX-License-Identifier: MIT

package acp

import (
	"bytes"
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
