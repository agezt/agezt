// SPDX-License-Identifier: MIT

package boardtool

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/board"
)

// fakeStore is an in-memory boardStore so the tool's op → store mapping is
// asserted without touching disk (the store itself is tested in kernel/board).
type fakeStore struct {
	msgs []board.Message
}

func (f *fakeStore) Post(topic, from, text string, nowMS int64) (board.Message, error) {
	m := board.Message{Topic: topic, From: from, Text: text, TSMS: nowMS}
	f.msgs = append(f.msgs, m)
	return m, nil
}

func (f *fakeStore) Read(topic string, limit int) []board.Message {
	out := make([]board.Message, 0, len(f.msgs))
	for _, m := range f.msgs {
		if topic == "" || m.Topic == topic {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSMS > out[j].TSMS })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (f *fakeStore) Topics() map[string]int {
	c := map[string]int{}
	for _, m := range f.msgs {
		c[m.Topic]++
	}
	return c
}

func newTool(t *testing.T) *Tool {
	t.Helper()
	tool := New()
	tool.bindStore(&fakeStore{})
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
	if _, isErr := invoke(t, tool, map[string]any{"op": "post", "topic": "findings", "from": "researcher", "text": "Go site is go.dev"}); isErr {
		t.Fatal("post errored")
	}
	out, isErr := invoke(t, tool, map[string]any{"op": "read", "topic": "findings"})
	if isErr {
		t.Fatal("read errored")
	}
	if out["count"].(float64) != 1 {
		t.Fatalf("read count = %v, want 1", out["count"])
	}
	m := out["messages"].([]any)[0].(map[string]any)
	if m["text"] != "Go site is go.dev" || m["from"] != "researcher" || m["topic"] != "findings" {
		t.Errorf("message folded wrong: %+v", m)
	}
}

func TestRead_NewestFirst_AndTopicFilter(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, map[string]any{"op": "post", "topic": "a", "text": "first"})
	invoke(t, tool, map[string]any{"op": "post", "topic": "b", "text": "other-topic"})
	invoke(t, tool, map[string]any{"op": "post", "topic": "a", "text": "second"})

	out, _ := invoke(t, tool, map[string]any{"op": "read", "topic": "a"})
	msgs := out["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("topic 'a' count = %d, want 2", len(msgs))
	}
	if msgs[0].(map[string]any)["text"] != "second" {
		t.Errorf("newest-first wrong: %v", msgs[0])
	}

	all, _ := invoke(t, tool, map[string]any{"op": "read"})
	if all["count"].(float64) != 3 {
		t.Errorf("unfiltered count = %v, want 3", all["count"])
	}
}

func TestReadLimitClamped(t *testing.T) {
	tool := newTool(t)
	for i := 0; i < 5; i++ {
		invoke(t, tool, map[string]any{"op": "post", "topic": "t", "text": "m"})
	}
	out, _ := invoke(t, tool, map[string]any{"op": "read", "limit": 2})
	if out["count"].(float64) != 2 {
		t.Errorf("limit not honored: %v", out["count"])
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

func TestPost_NotifiesWithCorrelation(t *testing.T) {
	tool := newTool(t)
	var got struct{ topic, from, text, corr string }
	calls := 0
	tool.OnPost(func(topic, from, text, corr string) {
		calls++
		got.topic, got.from, got.text, got.corr = topic, from, text, corr
	})
	ctx := agent.WithCorrelation(context.Background(), "run-42")
	raw, _ := json.Marshal(map[string]any{"op": "post", "topic": "handoff", "from": "ci", "text": "build green"})
	if _, err := tool.Invoke(ctx, raw); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if calls != 1 || got.topic != "handoff" || got.from != "ci" || got.text != "build green" || got.corr != "run-42" {
		t.Errorf("notifier got %+v (calls=%d), want handoff/ci/build green/run-42", got, calls)
	}
}

func TestRead_DoesNotNotify(t *testing.T) {
	tool := newTool(t)
	calls := 0
	tool.OnPost(func(_, _, _, _ string) { calls++ })
	invoke(t, tool, map[string]any{"op": "read"})
	invoke(t, tool, map[string]any{"op": "topics"})
	if calls != 0 {
		t.Errorf("only posts should notify, got %d", calls)
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
