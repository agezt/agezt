// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

type ownedInboxJournal struct {
	events []*event.Event
	err    error
	calls  int
}

func (j *ownedInboxJournal) Range(fn func(*event.Event) error) error {
	j.calls++
	if j.err != nil {
		return j.err
	}
	for _, e := range j.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}
func ownedInboxEvent(id, corr string, ts int64, kind event.Kind, payload string) *event.Event {
	return &event.Event{ID: id, CorrelationID: corr, TSUnixMS: ts, Kind: kind, Payload: json.RawMessage(payload)}
}
func inboxThreads(t *testing.T, result InboxOutput) []*InboxThread {
	t.Helper()
	if result.Threads == nil {
		t.Fatal("threads must be non-null array", result)
	}
	return result.Threads
}

func inboxLimit(value int) *int { return &value }

func TestChannelInboxGroupingFallbackFillOrderingAndExactFields(t *testing.T) {
	j := &ownedInboxJournal{events: []*event.Event{
		ownedInboxEvent("in", "shared", 10, event.KindChannelInbound, `{"sender":"alice","text":"in"}`),
		ownedInboxEvent("out", "shared", 30, event.KindChannelOutbound, `{"channel_kind":"Slack","channel_id":"42","text":"out"}`),
		ownedInboxEvent("third", "shared", 20, event.KindChannelInbound, `{"channel_kind":"ignored","channel_id":"ignored","text":"third"}`),
		ownedInboxEvent("z", "z", 30, event.KindChannelInbound, `{"channel_kind":"telegram","channel_id":"7","text":"z"}`),
		ownedInboxEvent("a", "a", 30, event.KindChannelOutbound, `{"channel_kind":"email","text":"a"}`),
		ownedInboxEvent("solo1", "", 5, event.KindChannelInbound, `bad-json`),
		ownedInboxEvent("solo2", "", 5, event.KindChannelOutbound, `{"channel_kind":5,"text":"partial"}`),
		ownedInboxEvent("ignored", "other", 100, event.KindOpInvoked, `{"text":"ignored"}`),
	}}
	before, _ := json.Marshal(j.events)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := NewInbox(j).List(ctx, InboxInput{})
	if err != nil || j.calls != 1 || result.Count != 5 || result.Total != 5 || len(inboxJSONView(t, result)) != 3 {
		t.Fatal(result, err, j.calls)
	}
	threads := inboxThreads(t, result)
	if threads[0].CorrelationID != "z" || threads[1].CorrelationID != "shared" || threads[2].CorrelationID != "a" || threads[3].Messages[0].EventID != "solo1" || threads[4].Messages[0].EventID != "solo2" {
		t.Fatal(threads)
	}
	want := &InboxThread{CorrelationID: "shared", ChannelKind: "Slack", ChannelID: "42", LastTSUnixMS: 30, Messages: []InboxMessage{{Direction: "in", Sender: "alice", Text: "in", TSUnixMS: 10, EventID: "in"}, {Direction: "out", Text: "out", TSUnixMS: 30, EventID: "out"}, {Direction: "in", Text: "third", TSUnixMS: 20, EventID: "third"}}}
	if !reflect.DeepEqual(threads[1], want) || threads[4].Messages[0].Text != "partial" {
		t.Fatal(threads[1], threads[4])
	}
	after, _ := json.Marshal(j.events)
	if string(before) != string(after) {
		t.Fatal("journal events mutated")
	}
	result, err = NewInbox(j).List(context.Background(), InboxInput{Channel: " SLACK "})
	if err != nil || result.Channel != "slack" || result.Total != 1 || result.Count != 1 || len(inboxThreads(t, result)) != 1 {
		t.Fatal(result, err)
	}
	result, err = NewInbox(j).List(context.Background(), InboxInput{Channel: "missing"})
	if err != nil || result.Count != 0 || result.Total != 0 || len(inboxThreads(t, result)) != 0 {
		t.Fatal(result, err)
	}
}

func TestChannelInboxDefaultClampAndCursorPresentation(t *testing.T) {
	j := &ownedInboxJournal{}
	for i := 0; i < 1005; i++ {
		j.events = append(j.events, ownedInboxEvent(fmt.Sprint(i), fmt.Sprintf("c%04d", i), int64(i), event.KindChannelInbound, `{}`))
	}
	for _, tc := range []struct {
		limit  *int
		count  int
		cursor string
	}{{nil, 20, "985:c0985"}, {inboxLimit(0), 1, "1004:c1004"}, {inboxLimit(-5), 1, "1004:c1004"}, {inboxLimit(1), 1, "1004:c1004"}, {inboxLimit(1001), 1000, "5:c0005"}, {inboxLimit(1000), 1000, "5:c0005"}} {
		result, err := NewInbox(j).List(context.Background(), InboxInput{Limit: tc.limit})
		if err != nil || result.Count != tc.count || result.Total != 1005 || result.NextCursor != tc.cursor || len(inboxThreads(t, result)) != tc.count {
			t.Fatal(tc, result, err)
		}
	}
	result, err := NewInbox(j).List(context.Background(), InboxInput{Cursor: "5:c0005", Limit: inboxLimit(1000)})
	if err != nil || result.Count != 5 || result.Total != 1005 || len(inboxJSONView(t, result)) != 3 {
		t.Fatal(result, err)
	}
	empty, err := NewInbox(&ownedInboxJournal{}).List(context.Background(), InboxInput{})
	if err != nil || !reflect.DeepEqual(empty, InboxOutput{Threads: []*InboxThread{}, Count: 0, Total: 0}) {
		t.Fatal(empty, err)
	}
}

func TestChannelInboxCursorTieBoundaryMalformedAndLargeIntegerWire(t *testing.T) {
	const huge = int64(9223372036854775807)
	j := &ownedInboxJournal{events: []*event.Event{ownedInboxEvent("z", "z", 100, event.KindChannelInbound, `{}`), ownedInboxEvent("b", "b:part", 100, event.KindChannelInbound, `{}`), ownedInboxEvent("a", "a", 100, event.KindChannelInbound, `{}`), ownedInboxEvent("old", "old", 99, event.KindChannelInbound, `{}`)}}
	for _, tc := range []struct {
		cursor string
		want   []string
	}{{"100:b:part", []string{"a", "old"}}, {"100", []string{"old"}}, {"100:", []string{"old"}}, {"99:old", []string{}}, {"bad", []string{"z", "b:part", "a", "old"}}, {"9223372036854775808:z", []string{"z", "b:part", "a", "old"}}, {" 100:z", []string{"z", "b:part", "a", "old"}}} {
		result, err := NewInbox(j).List(context.Background(), InboxInput{Cursor: tc.cursor})
		if err != nil || result.Total != 4 {
			t.Fatal(tc, result, err)
		}
		got := []string{}
		for _, thread := range inboxThreads(t, result) {
			got = append(got, thread.CorrelationID)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatal(tc, got)
		}
	}
	j.events = []*event.Event{ownedInboxEvent("huge", "huge", huge, event.KindChannelInbound, `{}`), ownedInboxEvent("other", "other", 1, event.KindChannelInbound, `{}`)}
	result, err := NewInbox(j).List(context.Background(), InboxInput{Limit: inboxLimit(1)})
	raw, _ := json.Marshal(result)
	if err != nil || result.NextCursor != "9223372036854775807:huge" || !strings.Contains(string(raw), `"last_ts_unix_ms":9223372036854775807`) || !strings.Contains(string(raw), `"ts_unix_ms":9223372036854775807`) {
		t.Fatal(result, string(raw), err)
	}
}

func TestChannelInboxJournalErrorPrecedesDelayedCursorCodecError(t *testing.T) {
	rangeError := errors.New("owned range failure")
	cursorError := errors.New("args.cursor must be a string")
	j := &ownedInboxJournal{err: rangeError}
	result, err := NewInbox(j).List(context.Background(), InboxInput{CursorError: cursorError})
	if !reflect.DeepEqual(result, InboxOutput{}) || !errors.Is(err, rangeError) || j.calls != 1 {
		t.Fatal(result, err, j.calls)
	}
	j.err = nil
	result, err = NewInbox(j).List(context.Background(), InboxInput{CursorError: cursorError})
	if !reflect.DeepEqual(result, InboxOutput{}) || !errors.Is(err, cursorError) || j.calls != 2 {
		t.Fatal(result, err, j.calls)
	}
}

func inboxJSONView(t *testing.T, result InboxOutput) map[string]any {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
