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
	seq  int
}

func (f *fakeStore) Post(topic, from, text string, nowMS int64) (board.Message, error) {
	return f.Send(board.Message{Topic: topic, From: from, Text: text}, nowMS)
}

func (f *fakeStore) Send(m board.Message, nowMS int64) (board.Message, error) {
	f.seq++
	m.ID = "msg-" + string(rune('0'+f.seq))
	m.TSMS = nowMS
	f.msgs = append(f.msgs, m)
	return m, nil
}

func (f *fakeStore) Broadcast(from, text string, nowMS int64) (board.Message, error) {
	return f.Send(board.Message{Topic: "broadcast", From: from, To: board.Everyone, Text: text}, nowMS)
}

func (f *fakeStore) HelpRequest(from, to, text string, nowMS int64) (board.Message, error) {
	if to == "" {
		to = board.Everyone
	}
	return f.Send(board.Message{Topic: "help", From: from, To: to, Text: text, Help: true}, nowMS)
}

func (f *fakeStore) OpenHelp(limit int) []board.Message {
	answered := map[string]bool{}
	for _, m := range f.msgs {
		if m.ReplyTo != "" {
			answered[m.ReplyTo] = true
		}
	}
	out := make([]board.Message, 0, len(f.msgs))
	for _, m := range f.msgs {
		if m.Help && !answered[m.ID] {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSMS > out[j].TSMS })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (f *fakeStore) Get(id string) (board.Message, bool) {
	for _, m := range f.msgs {
		if m.ID == id {
			return m, true
		}
	}
	return board.Message{}, false
}

func (f *fakeStore) Inbox(to string, limit int, includeAnswered bool) []board.Message {
	answered := map[string]bool{}
	if !includeAnswered {
		for _, m := range f.msgs {
			if m.ReplyTo != "" {
				answered[m.ReplyTo] = true
			}
		}
	}
	out := make([]board.Message, 0, len(f.msgs))
	for _, m := range f.msgs {
		// Directed to me, or a broadcast I didn't send (mirrors kernel/board.Inbox).
		directed := m.To == to
		broadcast := m.To == board.Everyone && m.From != to
		if (directed || broadcast) && !answered[m.ID] {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSMS > out[j].TSMS })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (f *fakeStore) Replies(id string, limit int) []board.Message {
	out := make([]board.Message, 0, 4)
	for _, m := range f.msgs {
		if m.ReplyTo == id {
			out = append(out, m)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
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

func (f *fakeStore) Ack(id, by string) (board.Message, bool, error) {
	for i := range f.msgs {
		if f.msgs[i].ID == id {
			f.msgs[i].AckedBy = append(f.msgs[i].AckedBy, by)
			return f.msgs[i], true, nil
		}
	}
	return board.Message{}, false, nil
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

// TestInferOp (M844): a board call without an explicit op infers one from the
// fields — so a workflow board node passing {topic, text} posts instead of
// failing with "op required".
func TestInferOp(t *testing.T) {
	tool := newTool(t)
	// {topic, text} with no op → post.
	out, isErr := invoke(t, tool, map[string]any{"topic": "findings", "text": "from a workflow node"})
	if isErr {
		t.Fatalf("op-less post should succeed: %v", out)
	}
	if _, ok := out["posted"]; !ok {
		t.Errorf("expected a post, got %v", out)
	}
	// {to, text} with no op → send.
	out, isErr = invoke(t, tool, map[string]any{"to": "researcher", "text": "ping"})
	if isErr {
		t.Fatalf("op-less send should succeed: %v", out)
	}
	if _, ok := out["sent"]; !ok {
		t.Errorf("expected a send, got %v", out)
	}
	// No fields at all → read (the harmless default), not an error.
	if _, isErr := invoke(t, tool, map[string]any{}); isErr {
		t.Error("op-less empty call should default to read, not error")
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
		// {"op":""} is no longer an error — it infers a harmless read (M844).
	} {
		if _, isErr := invoke(t, tool, c); !isErr {
			t.Errorf("expected error for %v", c)
		}
	}
}

func TestPost_NotifiesWithCorrelation(t *testing.T) {
	tool := newTool(t)
	var got board.Message
	var gotCorr string
	calls := 0
	tool.OnPost(func(m board.Message, corr string) {
		calls++
		got, gotCorr = m, corr
	})
	ctx := agent.WithCorrelation(context.Background(), "run-42")
	raw, _ := json.Marshal(map[string]any{"op": "post", "topic": "handoff", "from": "ci", "text": "build green"})
	if _, err := tool.Invoke(ctx, raw); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if calls != 1 || got.Topic != "handoff" || got.From != "ci" || got.Text != "build green" || gotCorr != "run-42" {
		t.Errorf("notifier got %+v corr=%q (calls=%d), want handoff/ci/build green/run-42", got, gotCorr, calls)
	}
}

func TestRead_DoesNotNotify(t *testing.T) {
	tool := newTool(t)
	calls := 0
	tool.OnPost(func(board.Message, string) { calls++ })
	invoke(t, tool, map[string]any{"op": "read"})
	invoke(t, tool, map[string]any{"op": "topics"})
	invoke(t, tool, map[string]any{"op": "inbox", "to": "x"})
	invoke(t, tool, map[string]any{"op": "replies", "id": "msg-1"})
	if calls != 0 {
		t.Errorf("only post/send/reply should notify, got %d", calls)
	}
}

// TestAskReplyRoundTrip drives the whole A2A flow (M788) through the tool:
// planner sends researcher a question → researcher's inbox shows it waiting →
// researcher replies → the reply leaves the inbox and shows up under
// op=replies for the original id, addressed back to the asker.
func TestAskReplyRoundTrip(t *testing.T) {
	tool := newTool(t)
	var notified []board.Message
	tool.OnPost(func(m board.Message, _ string) { notified = append(notified, m) })

	sent, isErr := invoke(t, tool, map[string]any{
		"op": "send", "to": "researcher", "from": "planner", "text": "what is the deploy target?"})
	if isErr {
		t.Fatalf("send errored: %v", sent)
	}
	id := sent["sent"].(map[string]any)["id"].(string)
	if id == "" {
		t.Fatal("send returned no message id")
	}

	inbox, _ := invoke(t, tool, map[string]any{"op": "inbox", "to": "researcher"})
	if inbox["count"].(float64) != 1 {
		t.Fatalf("inbox count = %v, want 1", inbox["count"])
	}
	waiting := inbox["waiting"].([]any)[0].(map[string]any)
	if waiting["from"] != "planner" || waiting["id"] != id {
		t.Fatalf("inbox message wrong: %+v", waiting)
	}

	if _, isErr := invoke(t, tool, map[string]any{
		"op": "reply", "id": id, "from": "researcher", "text": "prod-eu"}); isErr {
		t.Fatal("reply errored")
	}

	// Answered → leaves the unanswered inbox; visible with all=true.
	inbox2, _ := invoke(t, tool, map[string]any{"op": "inbox", "to": "researcher"})
	if inbox2["count"].(float64) != 0 {
		t.Errorf("answered message still waiting: %v", inbox2)
	}
	inboxAll, _ := invoke(t, tool, map[string]any{"op": "inbox", "to": "researcher", "all": true})
	if inboxAll["count"].(float64) != 1 {
		t.Errorf("all=true should include the answered message: %v", inboxAll)
	}

	// The asker reads the answer.
	replies, _ := invoke(t, tool, map[string]any{"op": "replies", "id": id})
	if replies["count"].(float64) != 1 {
		t.Fatalf("replies count = %v, want 1", replies["count"])
	}
	r := replies["replies"].([]any)[0].(map[string]any)
	if r["text"] != "prod-eu" || r["to"] != "planner" || r["reply_to"] != id {
		t.Errorf("reply wrong: %+v", r)
	}

	// Both the send and the reply notified (addressed messages journal too).
	if len(notified) != 2 || notified[0].To != "researcher" || notified[1].To != "planner" {
		t.Errorf("notifications wrong: %+v", notified)
	}
}

// TestSendReplyBadInputs: missing to/text/id and replying to a ghost id are
// clear tool errors.
func TestSendReplyBadInputs(t *testing.T) {
	tool := newTool(t)
	for _, c := range []map[string]any{
		{"op": "send", "text": "no recipient"},
		{"op": "send", "to": "x"}, // no text
		{"op": "inbox"},           // whose inbox?
		{"op": "reply", "text": "no id"},
		{"op": "reply", "id": "ghost", "text": "orphan"},
		{"op": "replies"},
	} {
		if _, isErr := invoke(t, tool, c); !isErr {
			t.Errorf("expected error for %v", c)
		}
	}
}

func TestBroadcast_DeliversToPeersAndNotifies(t *testing.T) {
	tool := newTool(t)
	var notified []board.Message
	tool.OnPost(func(m board.Message, _ string) { notified = append(notified, m) })

	out, isErr := invoke(t, tool, map[string]any{"op": "broadcast", "from": "ann", "text": "deploying"})
	if isErr {
		t.Fatalf("broadcast errored: %v", out)
	}
	if len(notified) != 1 || notified[0].To != board.Everyone {
		t.Fatalf("broadcast did not notify To=Everyone: %+v", notified)
	}
	// A peer sees it in their inbox; the sender does not.
	peer, _ := invoke(t, tool, map[string]any{"op": "inbox", "to": "worker"})
	if peer["count"].(float64) != 1 {
		t.Fatalf("peer inbox = %v, want the broadcast", peer)
	}
	self, _ := invoke(t, tool, map[string]any{"op": "inbox", "to": "ann"})
	if self["count"].(float64) != 0 {
		t.Errorf("sender saw its own broadcast: %v", self)
	}
}

func TestHelp_RaiseListAndAnswer(t *testing.T) {
	tool := newTool(t)
	// Raise a help request (broadcast).
	raised, isErr := invoke(t, tool, map[string]any{"op": "help", "from": "worker", "text": "build is red"})
	if isErr {
		t.Fatalf("help raise errored: %v", raised)
	}
	hid := raised["help_requested"].(map[string]any)["id"].(string)
	if raised["help_requested"].(map[string]any)["help"] != true {
		t.Errorf("help message not flagged: %+v", raised["help_requested"])
	}
	// op=help with no text LISTS the open requests.
	list, _ := invoke(t, tool, map[string]any{"op": "help"})
	if list["count"].(float64) != 1 {
		t.Fatalf("open_help count = %v, want 1", list["count"])
	}
	// Answering it closes the open-help list.
	if _, isErr := invoke(t, tool, map[string]any{"op": "reply", "id": hid, "from": "helper", "text": "fixed it"}); isErr {
		t.Fatal("reply errored")
	}
	list2, _ := invoke(t, tool, map[string]any{"op": "help"})
	if list2["count"].(float64) != 0 {
		t.Errorf("answered help still listed open: %v", list2)
	}
}

func TestBroadcastHelp_BadInputs(t *testing.T) {
	tool := newTool(t)
	// broadcast needs text; help-raise infers list when text is absent (not an error).
	if _, isErr := invoke(t, tool, map[string]any{"op": "broadcast", "from": "x"}); !isErr {
		t.Error("broadcast without text should error")
	}
	if _, isErr := invoke(t, tool, map[string]any{"op": "help"}); isErr {
		t.Error("op=help with no text should LIST, not error")
	}
}

func TestAck_MarksReadAndValidates(t *testing.T) {
	tool := newTool(t)

	sent, _ := invoke(t, tool, map[string]any{
		"op": "send", "to": "researcher", "from": "planner", "text": "fyi"})
	id := sent["sent"].(map[string]any)["id"].(string)

	// ack needs both id and from.
	if out, isErr := invoke(t, tool, map[string]any{"op": "ack", "from": "researcher"}); !isErr {
		t.Fatalf("ack without id should error: %v", out)
	}
	if out, isErr := invoke(t, tool, map[string]any{"op": "ack", "id": id}); !isErr {
		t.Fatalf("ack without from should error: %v", out)
	}
	if out, isErr := invoke(t, tool, map[string]any{"op": "ack", "id": "nope", "from": "researcher"}); !isErr {
		t.Fatalf("ack of unknown id should error: %v", out)
	}

	out, isErr := invoke(t, tool, map[string]any{"op": "ack", "id": id, "from": "researcher"})
	if isErr {
		t.Fatalf("ack errored: %v", out)
	}
	if out["by"] != "researcher" || out["acked"].(map[string]any)["id"] != id {
		t.Fatalf("ack result wrong: %+v", out)
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
