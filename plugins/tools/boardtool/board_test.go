// SPDX-License-Identifier: MIT

package boardtool

import (
	"context"
	"encoding/json"
	"testing"
)

func newTool(t *testing.T) *Tool {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	tool := New()
	tool.bindStore(st)
	var clock int64 = 1000
	tool.now = func() int64 { clock += 10; return clock } // monotonically increasing
	return tool
}

func invoke(t *testing.T, tool *Tool, in map[string]any) (map[string]any, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(res.Output), &out)
	return out, res.IsError
}

func TestDefinitionValid(t *testing.T) {
	d := New().Definition()
	if d.Name != "board" || !json.Valid(d.InputSchema) {
		t.Fatalf("bad definition: %+v", d)
	}
}

func TestPostThenRead_SharedAcrossCalls(t *testing.T) {
	tool := newTool(t)
	// One "agent" posts...
	if _, isErr := invoke(t, tool, map[string]any{"op": "post", "topic": "findings", "from": "researcher", "text": "Go site is go.dev"}); isErr {
		t.Fatal("post errored")
	}
	// ...another reads it back (same shared store).
	out, isErr := invoke(t, tool, map[string]any{"op": "read", "topic": "findings"})
	if isErr {
		t.Fatal("read errored")
	}
	if out["count"].(float64) != 1 {
		t.Fatalf("read count = %v, want 1", out["count"])
	}
	msgs := out["messages"].([]any)
	m := msgs[0].(map[string]any)
	if m["text"] != "Go site is go.dev" || m["from"] != "researcher" || m["topic"] != "findings" {
		t.Errorf("message folded wrong: %+v", m)
	}
}

func TestRead_NewestFirst_AndTopicFilter(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, map[string]any{"op": "post", "topic": "a", "text": "first"})
	invoke(t, tool, map[string]any{"op": "post", "topic": "b", "text": "other-topic"})
	invoke(t, tool, map[string]any{"op": "post", "topic": "a", "text": "second"})

	// Topic filter: only topic "a", newest first.
	out, _ := invoke(t, tool, map[string]any{"op": "read", "topic": "a"})
	msgs := out["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("topic 'a' count = %d, want 2", len(msgs))
	}
	if msgs[0].(map[string]any)["text"] != "second" {
		t.Errorf("newest-first wrong: %v", msgs[0])
	}

	// No filter: all three.
	all, _ := invoke(t, tool, map[string]any{"op": "read"})
	if all["count"].(float64) != 3 {
		t.Errorf("unfiltered count = %v, want 3", all["count"])
	}
}

func TestTopics(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, map[string]any{"op": "post", "topic": "x", "text": "1"})
	invoke(t, tool, map[string]any{"op": "post", "topic": "x", "text": "2"})
	invoke(t, tool, map[string]any{"op": "post", "topic": "y", "text": "3"})
	out, _ := invoke(t, tool, map[string]any{"op": "topics"})
	topics := out["topics"].(map[string]any)
	if topics["x"].(float64) != 2 || topics["y"].(float64) != 1 {
		t.Errorf("topic counts wrong: %+v", topics)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	st1, _ := Open(dir)
	if _, err := st1.Post("t", "", "durable", 100); err != nil {
		t.Fatal(err)
	}
	// Reopen → message survives.
	st2, _ := Open(dir)
	if got := st2.Read("t", 10); len(got) != 1 || got[0].Text != "durable" {
		t.Fatalf("message did not persist: %+v", got)
	}
}

func TestBadInputs(t *testing.T) {
	tool := newTool(t)
	for _, c := range []map[string]any{
		{"op": "post", "text": "no topic"},
		{"op": "post", "topic": "t"}, // no text
		{"op": "bogus"},
		{"op": ""},
	} {
		if _, isErr := invoke(t, tool, c); !isErr {
			t.Errorf("expected error for %v", c)
		}
	}
}

func TestUnboundIsSafe(t *testing.T) {
	res, err := New().Invoke(context.Background(), json.RawMessage(`{"op":"topics"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("an unbound tool should return an error result")
	}
}
