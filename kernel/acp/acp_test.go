// SPDX-License-Identifier: MIT

package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
)

// fakeRunner records the intent and optionally streams chunks before returning.
type fakeRunner struct {
	thoughts  []string // reasoning chunks streamed before the answer (M322)
	chunks    []string
	answer    string
	err       error
	gotIntent string
	gotCwd    string
}

func (f *fakeRunner) Prompt(_ context.Context, cwd, intent string, onChunk func(ChunkKind, string)) (string, error) {
	f.gotIntent = intent
	f.gotCwd = cwd
	for _, th := range f.thoughts {
		onChunk(ChunkThought, th)
	}
	for _, c := range f.chunks {
		onChunk(ChunkMessage, c)
	}
	return f.answer, f.err
}

// run feeds a sequence of JSON-RPC messages and returns the decoded reply lines.
func run(t *testing.T, runner Runner, messages ...any) []map[string]any {
	t.Helper()
	var inBuf bytes.Buffer
	enc := json.NewEncoder(&inBuf)
	for _, m := range messages {
		if err := enc.Encode(m); err != nil {
			t.Fatal(err)
		}
	}
	var outBuf bytes.Buffer
	s := New(runner, &inBuf, &outBuf)
	if err := s.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(outBuf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("unmarshal out line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestInitialize(t *testing.T) {
	out := run(t, &fakeRunner{}, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(out))
	}
	res, _ := out[0]["result"].(map[string]any)
	if res["protocolVersion"].(float64) != ProtocolVersion {
		t.Errorf("protocolVersion = %v", res["protocolVersion"])
	}
	info, _ := res["agentInfo"].(map[string]any)
	if info["name"] != brand.Binary {
		t.Errorf("agentInfo.name = %v, want %q", info["name"], brand.Binary)
	}
	// agentInfo.version must report the real product version, not a stale literal.
	if info["version"] != brand.Version {
		t.Errorf("agentInfo.version = %v, want %q (brand.Version)", info["version"], brand.Version)
	}
}

func TestNewSessionReturnsID(t *testing.T) {
	out := run(t, &fakeRunner{}, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "session/new",
		"params": map[string]any{"cwd": "/work", "mcpServers": []any{}},
	})
	res, _ := out[0]["result"].(map[string]any)
	if sid, _ := res["sessionId"].(string); sid == "" {
		t.Errorf("sessionId empty: %v", res)
	}
}

func TestPromptStreamsChunksThenStop(t *testing.T) {
	runner := &fakeRunner{chunks: []string{"hello", " world"}, answer: "hello world"}
	out := run(t, runner,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "session/new",
			"params": map[string]any{"cwd": "/proj", "mcpServers": []any{}}},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/prompt",
			"params": map[string]any{
				"sessionId": "sess-1",
				"prompt":    []any{map[string]any{"type": "text", "text": "hi there"}},
			}},
	)
	// Expect: session/new reply, two session/update notifications, prompt reply.
	var updates, replies int
	var stop string
	for _, m := range out {
		if m["method"] == "session/update" {
			updates++
			p, _ := m["params"].(map[string]any)
			up, _ := p["update"].(map[string]any)
			if up["sessionUpdate"] != "agent_message_chunk" {
				t.Errorf("unexpected update type %v", up["sessionUpdate"])
			}
		}
		if r, ok := m["result"].(map[string]any); ok {
			replies++
			if sr, ok := r["stopReason"].(string); ok {
				stop = sr
			}
		}
	}
	if updates != 2 {
		t.Errorf("expected 2 chunk updates, got %d", updates)
	}
	if stop != "end_turn" {
		t.Errorf("stopReason = %q", stop)
	}
	if runner.gotIntent != "hi there" || runner.gotCwd != "/proj" {
		t.Errorf("runner saw intent=%q cwd=%q", runner.gotIntent, runner.gotCwd)
	}
}

// A reasoning model's thought chunks must be delivered as agent_thought_chunk
// session updates (M322), distinct from the answer's agent_message_chunk, so the
// editor renders them in its "thinking" UI.
func TestPromptStreamsThoughtChunks(t *testing.T) {
	runner := &fakeRunner{
		thoughts: []string{"hmm, ", "let me see"},
		chunks:   []string{"42"},
		answer:   "42",
	}
	out := run(t, runner,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "session/new",
			"params": map[string]any{"cwd": "/proj", "mcpServers": []any{}}},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/prompt",
			"params": map[string]any{
				"sessionId": "sess-1",
				"prompt":    []any{map[string]any{"type": "text", "text": "6*7?"}},
			}},
	)
	var thoughts, messages int
	var thoughtText, msgText string
	for _, m := range out {
		if m["method"] != "session/update" {
			continue
		}
		p, _ := m["params"].(map[string]any)
		up, _ := p["update"].(map[string]any)
		content, _ := up["content"].(map[string]any)
		text, _ := content["text"].(string)
		switch up["sessionUpdate"] {
		case "agent_thought_chunk":
			thoughts++
			thoughtText += text
		case "agent_message_chunk":
			messages++
			msgText += text
		default:
			t.Errorf("unexpected update type %v", up["sessionUpdate"])
		}
	}
	if thoughts != 2 || thoughtText != "hmm, let me see" {
		t.Errorf("thought chunks: n=%d text=%q", thoughts, thoughtText)
	}
	if messages != 1 || msgText != "42" {
		t.Errorf("message chunks: n=%d text=%q", messages, msgText)
	}
}

func TestPromptNonStreamingEmitsAnswerAsOneChunk(t *testing.T) {
	runner := &fakeRunner{answer: "the whole answer"} // no chunks streamed
	out := run(t, runner,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "session/new",
			"params": map[string]any{"cwd": "/x", "mcpServers": []any{}}},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/prompt",
			"params": map[string]any{
				"sessionId": "sess-1",
				"prompt":    []any{map[string]any{"type": "text", "text": "q"}},
			}},
	)
	var sawAnswer bool
	for _, m := range out {
		if m["method"] == "session/update" {
			p, _ := m["params"].(map[string]any)
			up, _ := p["update"].(map[string]any)
			c, _ := up["content"].(map[string]any)
			if c["text"] == "the whole answer" {
				sawAnswer = true
			}
		}
	}
	if !sawAnswer {
		t.Error("non-streaming answer should be emitted as a single chunk")
	}
}

func TestUnknownSessionIsError(t *testing.T) {
	out := run(t, &fakeRunner{answer: "x"},
		map[string]any{"jsonrpc": "2.0", "id": 5, "method": "session/prompt",
			"params": map[string]any{
				"sessionId": "nope",
				"prompt":    []any{map[string]any{"type": "text", "text": "q"}},
			}},
	)
	if out[0]["error"] == nil {
		t.Errorf("prompt to unknown session should error, got %v", out[0])
	}
}

func TestUnknownMethodIsError(t *testing.T) {
	out := run(t, &fakeRunner{}, map[string]any{
		"jsonrpc": "2.0", "id": 9, "method": "does/notexist",
	})
	e, _ := out[0]["error"].(map[string]any)
	if e == nil || e["code"].(float64) != codeMethodNotFound {
		t.Errorf("expected method-not-found, got %v", out[0])
	}
}

func TestNotificationGetsNoReply(t *testing.T) {
	// A cancel notification (no id) must not produce a reply line.
	out := run(t, &fakeRunner{}, map[string]any{
		"jsonrpc": "2.0", "method": "session/cancel",
		"params": map[string]any{"sessionId": "sess-1"},
	})
	if len(out) != 0 {
		t.Errorf("notification should yield no reply, got %v", out)
	}
}

// TestFlattenPrompt_BlockSelection pins how a multi-block prompt is flattened into one
// intent. Every other test sends a single {"type":"text"} block, so three properties of
// flattenPrompt were unpinned and survived mutation (M514): (1) blocks join with a
// newline; (2) a block with the type field OMITTED but text present is treated as text
// (lenient parse for clients that don't set type); (3) a non-text typed block (e.g.
// image) is ignored even when it carries a text field, and an empty-type/empty-text block
// adds nothing. The branch `b.Type == "" && b.Text != ""` is the load-bearing condition:
// `== → !=`, `!= → ==`, and `&& → ||` all changed which blocks were selected undetected.
func TestFlattenPrompt_BlockSelection(t *testing.T) {
	got := flattenPrompt([]contentBlock{
		{Type: "text", Text: "one"},
		{Type: "", Text: "two"},          // type omitted but text present → included
		{Type: "image", Text: "ignored"}, // non-text typed block → excluded despite text
		{Type: "", Text: ""},             // empty type AND empty text → contributes nothing
		{Type: "text", Text: "three"},
	})
	if want := "one\ntwo\nthree"; got != want {
		t.Errorf("flattenPrompt = %q, want %q", got, want)
	}
}
