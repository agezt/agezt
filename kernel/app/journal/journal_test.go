// SPDX-License-Identifier: MIT

package journal

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeJournal struct {
	seq    int64
	hash   string
	events []*event.Event
	err    error
	asked  []int
	calls  []string
}

func (f *fakeJournal) Head() (int64, string) {
	f.calls = append(f.calls, "head")
	return f.seq, f.hash
}

func (f *fakeJournal) Tail(n int) ([]*event.Event, error) {
	f.calls = append(f.calls, "tail")
	f.asked = append(f.asked, n)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.events) == 0 {
		return nil, nil
	}
	return f.events[max(len(f.events)-n, 0):], nil
}

func (f *fakeJournal) Range(fn func(*event.Event) error) error {
	f.calls = append(f.calls, "range")
	if f.err != nil {
		return f.err
	}
	for _, e := range f.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}

func decode[T any](t *testing.T, raw string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(raw, err)
	}
	return v
}

func TestHeadClampsTheEmptyJournal(t *testing.T) {
	ctx := context.Background()
	if out, _ := New(&fakeJournal{seq: -1}, nil, nil).Head(ctx, ReadInput{}); out != (HeadOutput{}) {
		t.Fatal(out)
	}
	if out, _ := New(&fakeJournal{seq: 41, hash: "abc"}, nil, nil).Head(ctx, ReadInput{}); out != (HeadOutput{Head: 41, Hash: "abc"}) {
		t.Fatal(out)
	}
	if raw, _ := json.Marshal(HeadOutput{}); string(raw) != `{"head":0,"hash":""}` {
		t.Fatal(string(raw))
	}
}

func TestTail(t *testing.T) {
	ctx := context.Background()
	var events []*event.Event
	for i := range 30 {
		events = append(events, &event.Event{Seq: int64(i), Kind: "task.received"})
	}
	for raw, want := range map[string]int{`{}`: 20, `{"n":0}`: 1, `{"n":-3}`: 1, `{"n":5.9}`: 5, `{"n":"7"}`: 20, `{"n":null}`: 20, `{"n":20000}`: 10_000} {
		j := &fakeJournal{seq: 29, events: events}
		out, err := New(j, nil, nil).Tail(ctx, decode[TailRequest](t, raw))
		if err != nil || j.asked[0] != want || out.Count != len(out.Events) || out.Head != 29 || out.Events[len(out.Events)-1].Seq != 29 {
			t.Fatal(raw, j.asked, out.Count, err)
		}
		if !reflect.DeepEqual(j.calls, []string{"head", "tail"}) {
			t.Fatal("the head checkpoint precedes the read", j.calls)
		}
	}
	empty, err := New(&fakeJournal{seq: -1}, nil, nil).Tail(ctx, TailRequest{})
	if raw, _ := json.Marshal(empty); err != nil || string(raw) != `{"events":[],"count":0,"head":0}` {
		t.Fatal(string(raw), err)
	}
	boom := errors.New("segment unreadable")
	if _, err := New(&fakeJournal{err: boom}, nil, nil).Tail(ctx, TailRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestStats(t *testing.T) {
	ctx := context.Background()
	disk := 0
	measure := func() (int, int64) { disk++; return 3, 4096 }
	j := &fakeJournal{events: []*event.Event{
		{Kind: "task.received", TSUnixMS: 500},
		{Kind: "task.received", TSUnixMS: 0},
		{Kind: "task.completed", TSUnixMS: 900},
		{Kind: "kernel.boot", TSUnixMS: 200},
	}}
	out, err := New(j, measure, nil).Stats(ctx, ReadInput{})
	want := StatsOutput{Events: 4, Segments: 3, Bytes: 4096, ByKind: map[string]int64{"task.received": 2, "task.completed": 1, "kernel.boot": 1}, OldestUnixMS: 200, NewestUnixMS: 900}
	if err != nil || !reflect.DeepEqual(out, want) || disk != 1 {
		t.Fatalf("%+v %v", out, err)
	}
	empty, _ := New(&fakeJournal{}, func() (int, int64) { return 0, 0 }, nil).Stats(ctx, ReadInput{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"events":0,"segments":0,"bytes":0,"by_kind":{},"oldest_unix_ms":0,"newest_unix_ms":0}` {
		t.Fatal(string(raw))
	}
	boom := errors.New("segment unreadable")
	disk = 0
	if _, err := New(&fakeJournal{err: boom}, measure, nil).Stats(ctx, ReadInput{}); !errors.Is(err, boom) || disk != 0 {
		t.Fatal("a failed fold measures nothing", err, disk)
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	ops, err := Operations(func(context.Context) *Service { return New(&fakeJournal{}, nil, nil) })
	if err != nil || len(ops) != 7 {
		t.Fatal(ops, err)
	}
	for i, w := range []struct {
		name    string
		tenancy opapi.Tenancy
		out     reflect.Type
		authz   opapi.Authz
	}{
		{"journal_head", opapi.Primary, reflect.TypeFor[HeadOutput](), opapi.PrimaryOnly},
		{"journal_tail", opapi.Primary, reflect.TypeFor[EventsOutput](), opapi.PrimaryOnly},
		{"journal_grep", opapi.Primary, reflect.TypeFor[EventsOutput](), opapi.PrimaryOnly},
		{"journal_export", opapi.Primary, reflect.TypeFor[ExportOutput](), opapi.PrimaryOnly},
		{"changelog", opapi.CallerTenant, reflect.TypeFor[ChangelogOutput](), opapi.PrimaryOnly},
		{"cache_stats", opapi.CallerTenant, reflect.TypeFor[CacheStatsOutput](), opapi.OwnTenant},
		{"journal_stats", opapi.CallerTenant, reflect.TypeFor[StatsOutput](), opapi.PrimaryOnly},
	} {
		spec := ops[i].Spec()
		http := opapi.HTTP{}
		if w.name == "journal_grep" {
			http = opapi.HTTP{Method: "GET", Path: "/api/journal"}
		}
		if spec.Name != w.name || !spec.ReadOnly || spec.Authz != w.authz || spec.Tenancy != w.tenancy || !spec.AllowUnknownInput || spec.Output != w.out || spec.HTTP != http {
			t.Fatal(spec)
		}
	}
	tail := EventsOutput{Events: []*event.Event{
		{ID: "a", Seq: 1, Kind: "k", Payload: json.RawMessage(`{"x":[1,"y"]}`), Tags: map[string]string{"t": "v"}},
		{ID: "b", Seq: 2, Kind: "k", Payload: json.RawMessage(`[1,2]`)},
		{ID: "c", Seq: 3, Kind: "k", Payload: json.RawMessage(`"text"`), CorrelationID: "corr"},
		{ID: "d", Seq: 4, Kind: "k"},
	}, Count: 4, Head: 4}
	raw, _ := json.Marshal(tail)
	if err := schema.ValidateJSON(ops[1].Spec().OutputSchema, raw); err != nil {
		t.Fatal("the tail schema accepts every journaled payload shape", err, string(raw))
	}
	for i, v := range map[int]any{0: HeadOutput{Head: 1}, 2: EventsOutput{Events: tail.Events}, 3: ExportOutput{Events: tail.Events, FirstSeq: 1, LastSeq: 4, Truncated: true}, 4: ChangelogOutput{Entries: []ChangelogEntry{{Kind: "k"}}, Count: 1}, 5: CacheStatsOutput{Calls: 1}, 6: StatsOutput{ByKind: map[string]int64{"k": 1}}} {
		raw, _ := json.Marshal(v)
		if err := schema.ValidateJSON(ops[i].Spec().OutputSchema, raw); err != nil {
			t.Fatal(i, err)
		}
	}
}
