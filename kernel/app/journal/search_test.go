// SPDX-License-Identifier: MIT

package journal

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

// countingJournal records how many events the walk visited.
type countingJournal struct {
	fakeJournal
	visited int
}

func (c *countingJournal) Range(fn func(*event.Event) error) error {
	c.calls = append(c.calls, "range")
	for _, e := range c.events {
		c.visited++
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}

func searchEvents() []*event.Event {
	return []*event.Event{
		{Seq: 0, TSUnixMS: 1_000, Kind: "kernel.boot", Subject: "kernel", Actor: "kernel"},
		{Seq: 1, TSUnixMS: 2_000, Kind: "task.received", Subject: "agent.a.task", Actor: "agent-a", CorrelationID: "c1", Payload: json.RawMessage(`{"intent":"Deploy PROD"}`)},
		{Seq: 2, TSUnixMS: 3_000, Kind: "tool.invoked", Subject: "agent.a.tool", Actor: "agent-a", CorrelationID: "c1", Payload: json.RawMessage(`{"tool":"shell","cmd":"rm -rf /tmp/x"}`)},
		{Seq: 3, TSUnixMS: 4_000, Kind: "task.received", Subject: "agent.b.task", Actor: "agent-b", CorrelationID: "c2"},
		{Seq: 4, TSUnixMS: 5_000, Kind: "task.completed", Subject: "agent.a.task", Actor: "agent-a", CorrelationID: "c1"},
	}
}

func seqs(events []*event.Event) string {
	var parts []string
	for _, e := range events {
		parts = append(parts, string(rune('0'+e.Seq)))
	}
	return strings.Join(parts, ",")
}

func TestGrepCodecsAndOrder(t *testing.T) {
	for raw, want := range map[string]string{
		`{"pattern":1}`:                     "args.pattern must be a string",
		`{"pattern":1,"kind":2}`:            "args.pattern must be a string",
		`{"kind":null,"subject":3}`:         "args.kind must be a string",
		`{"subject":[]}`:                    "args.subject must be a string",
		`{"actor":true,"correlation_id":4}`: "args.actor must be a string",
		`{"correlation_id":{},"limit":"x"}`: "args.correlation_id must be a string",
	} {
		j := &fakeJournal{}
		if _, err := New(j, nil, nil).Grep(context.Background(), decode[GrepRequest](t, raw)); err == nil || err.Error() != want || j.calls != nil {
			t.Fatal(raw, err, j.calls)
		}
	}
}

func TestGrep(t *testing.T) {
	ctx := context.Background()
	for raw, want := range map[string]string{
		`{}`:                                          "0,1,2,3,4",
		`{"kind":"task.received"}`:                    "1,3",
		`{"kind":"TASK.RECEIVED"}`:                    "",
		`{"subject":"agent.a"}`:                       "",
		`{"actor":"agent"}`:                           "",
		`{"correlation_id":"c"}`:                      "",
		`{"subject":"agent.a.task"}`:                  "1,4",
		`{"actor":"agent-a","kind":"task.completed"}`: "4",
		`{"correlation_id":"c1"}`:                     "1,2,4",
		`{"pattern":"PROD"}`:                          "1",
		`{"pattern":"RM -RF"}`:                        "2",
		`{"pattern":"AGENT-B"}`:                       "3",
		`{"pattern":"c2"}`:                            "3",
		`{"pattern":"kernel.BOOT"}`:                   "0",
		`{"pattern":"agent.a","correlation_id":"c1"}`: "1,2,4",
		`{"limit":2}`:                                 "0,1",
		`{"limit":0}`:                                 "0",
		`{"limit":-1}`:                                "0",
		`{"limit":2.8}`:                               "0,1",
		`{"limit":"2"}`:                               "0,1,2,3,4",
	} {
		j := &countingJournal{fakeJournal: fakeJournal{seq: 4, events: searchEvents()}}
		out, err := New(j, nil, nil).Grep(ctx, decode[GrepRequest](t, raw))
		if err != nil || seqs(out.Events) != want || out.Count != len(out.Events) || out.Head != 4 {
			t.Fatal(raw, seqs(out.Events), err)
		}
		if !reflect.DeepEqual(j.calls, []string{"head", "range"}) {
			t.Fatal("the head checkpoint precedes the walk", j.calls)
		}
	}
	j := &countingJournal{fakeJournal: fakeJournal{events: searchEvents()}}
	if _, err := New(j, nil, nil).Grep(ctx, decode[GrepRequest](t, `{"actor":"agent-a","limit":1}`)); err != nil || j.visited != 2 {
		t.Fatal("the walk stops at the limit", j.visited, err)
	}
	var many []*event.Event
	for i := range 12_000 {
		many = append(many, &event.Event{Seq: int64(i)})
	}
	if out, _ := New(&fakeJournal{events: many}, nil, nil).Grep(ctx, decode[GrepRequest](t, `{"limit":50000}`)); out.Count != 10_000 {
		t.Fatal(out.Count)
	}
	if out, _ := New(&fakeJournal{events: many}, nil, nil).Grep(ctx, GrepRequest{}); out.Count != 100 {
		t.Fatal(out.Count)
	}
	empty, _ := New(&fakeJournal{seq: -1}, nil, nil).Grep(ctx, GrepRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"events":[],"count":0,"head":0}` {
		t.Fatal(string(raw))
	}
	boom := errors.New("segment unreadable")
	if _, err := New(&fakeJournal{err: boom}, nil, nil).Grep(ctx, GrepRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestExport(t *testing.T) {
	ctx := context.Background()
	clock := func() time.Time { return time.UnixMilli(6_000) }
	run := func(raw string) ExportOutput {
		t.Helper()
		out, err := New(&fakeJournal{seq: 4, hash: "h4", events: searchEvents()}, nil, clock).Export(ctx, decode[ExportRequest](t, raw))
		if err != nil {
			t.Fatal(raw, err)
		}
		return out
	}
	if out := run(`{}`); seqs(out.Events) != "0,1,2,3,4" || out.Count != 5 || out.FirstSeq != 0 || out.LastSeq != 4 || out.HeadSeq != 4 || out.HeadHash != "h4" || out.Truncated || out.Correlation != "" {
		t.Fatalf("%+v", out)
	}
	if out := run(`{"since_ms":3000}`); seqs(out.Events) != "2,3,4" || out.FirstSeq != 2 {
		t.Fatalf("since keeps events stamped at or after now-since: %+v", out)
	}
	if out := run(`{"since_ms":3000.9}`); seqs(out.Events) != "2,3,4" {
		t.Fatal(seqs(out.Events))
	}
	for _, raw := range []string{`{"since_ms":0}`, `{"since_ms":-5}`, `{"since_ms":"3000"}`, `{"since_ms":0.4}`} {
		if out := run(raw); out.Count != 5 {
			t.Fatal("a non-positive or non-numeric window exports everything", raw, out.Count)
		}
	}
	if out := run(`{"correlation":"c1","since_ms":3500}`); seqs(out.Events) != "2,4" || out.Correlation != "c1" || out.FirstSeq != 2 || out.LastSeq != 4 {
		t.Fatalf("%+v", out)
	}
	if out := run(`{"correlation":"ghost"}`); out.Count != 0 || out.FirstSeq != -1 || out.LastSeq != -1 {
		t.Fatalf("an empty scope reports -1 bounds: %+v", out)
	}
	empty, _ := New(&fakeJournal{seq: -1}, nil, clock).Export(ctx, ExportRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"events":[],"count":0,"first_seq":-1,"last_seq":-1,"head_seq":0,"head_hash":"","truncated":false,"correlation":""}` {
		t.Fatal(string(raw))
	}
	for raw, want := range map[string]string{`{"correlation":3}`: "args.correlation must be a string", `{"correlation":null,"since_ms":"x"}`: "args.correlation must be a string"} {
		j := &fakeJournal{}
		if _, err := New(j, nil, clock).Export(ctx, decode[ExportRequest](t, raw)); err == nil || err.Error() != want || j.calls != nil {
			t.Fatal(raw, err, j.calls)
		}
	}
	capped := func(n int, events []*event.Event) (ExportOutput, int) {
		j := &countingJournal{fakeJournal: fakeJournal{events: events}}
		s := New(j, nil, clock)
		s.exportCap = n
		out, err := s.Export(ctx, ExportRequest{})
		if err != nil {
			t.Fatal(err)
		}
		return out, j.visited
	}
	if out, visited := capped(3, searchEvents()); !out.Truncated || out.Count != 3 || out.LastSeq != 2 || visited != 4 {
		t.Fatalf("the cap truncates at the first event past it: %+v visited=%d", out, visited)
	}
	if out, _ := capped(5, searchEvents()); out.Truncated || out.Count != 5 {
		t.Fatalf("exactly the cap is complete: %+v", out)
	}
	if New(&fakeJournal{}, nil, clock).exportCap != MaxExportN || MaxExportN != 200_000 {
		t.Fatal("export cap")
	}
	boom := errors.New("segment unreadable")
	if _, err := New(&fakeJournal{err: boom}, nil, clock).Export(ctx, ExportRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}
