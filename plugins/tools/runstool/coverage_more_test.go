// SPDX-License-Identifier: MIT

package runstool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

type fakeHist struct {
	events []*event.Event
	err    error
}

func (f fakeHist) Tail(int) ([]*event.Event, error) { return f.events, f.err }

func makeEvent(kind, corr string, ts int64, payload any) *event.Event {
	if payload == nil {
		return &event.Event{Kind: event.Kind(kind), CorrelationID: corr, TSUnixMS: ts}
	}
	raw, _ := json.Marshal(payload)
	return &event.Event{Kind: event.Kind(kind), CorrelationID: corr, TSUnixMS: ts, Payload: raw}
}

func TestRunstoolCoverageBindAndLimitHelpers(t *testing.T) {
	tool := New()
	if tool.hist != nil {
		t.Fatal("unbound tool should have nil hist")
	}

	// Bind with nil is a no-op (preserves nil).
	tool.Bind(nil)
	if tool.hist != nil {
		t.Fatal("Bind(nil) should not set hist")
	}

	// Bind with a real hist sets it.
	h := fakeHist{}
	tool.Bind(h)
	if tool.hist == nil {
		t.Fatal("Bind(fakeHist) should set hist")
	}

	// limitOf clamps to default / max.
	cases := map[int]int{
		0:    DefaultLimit,
		-5:   DefaultLimit,
		1:    1,
		25:   25,
		100:  MaxLimit,
		5000: MaxLimit,
	}
	for in, want := range cases {
		if got := limitOf(in); got != want {
			t.Fatalf("limitOf(%d) = %d, want %d", in, got, want)
		}
	}

	// clip returns the slice unchanged for nil and short slices.
	if got := clip(nil, 5); len(got) != 0 {
		t.Fatalf("clip(nil) = %v, want empty", got)
	}
	if got := clip([]runRec{{Corr: "a"}, {Corr: "b"}}, 5); len(got) != 2 {
		t.Fatalf("clip short = %v", got)
	}
	if got := clip([]runRec{{Corr: "a"}, {Corr: "b"}, {Corr: "c"}}, 2); len(got) != 2 || got[0].Corr != "a" || got[1].Corr != "b" {
		t.Fatalf("clip long = %+v", got)
	}
}

func TestRunstoolCoverageOKJSONMarshalFail(t *testing.T) {
	// okJSON with an unmarshalable value surfaces the errResult fallback.
	r := okJSON(make(chan int))
	if !r.IsError || !strings.Contains(r.Output, "marshal:") {
		t.Fatalf("okJSON marshal fail = %+v", r)
	}
}

func TestRunstoolCoverageInvokeBranches(t *testing.T) {
	// Unbound tool.
	tool := New()
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"recent"}`))
	if err != nil {
		t.Fatalf("Invoke unbound: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("unbound result = %+v", res)
	}

	// Bound with a journal read error.
	tool.Bind(fakeHist{err: errors.New("disk full")})
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"recent"}`))
	if err != nil {
		t.Fatalf("Invoke read err: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "read journal") {
		t.Fatalf("read err = %+v", res)
	}

	// Bound with empty events: recent, stats, search, unknown, empty op.
	tool.Bind(fakeHist{events: nil})
	cases := map[string]string{
		`{"op":""}`:                   "op required",
		`{"op":"unknown"}`:            "unknown op",
		`{"op":"recent"}`:             `"runs":`,
		`{"op":"stats"}`:              `"total": 0`,
		`{"op":"search"}`:             `op=search needs`,
		`{"op":"search","query":"x"}`: `"query": "x"`,
	}
	for input, want := range cases {
		res, err := tool.Invoke(context.Background(), json.RawMessage(input))
		if err != nil {
			t.Fatalf("Invoke %q: %v", input, err)
		}
		if !strings.Contains(res.Output, want) {
			t.Fatalf("Invoke %q output = %q, want substring %q", input, res.Output, want)
		}
	}

	// Bound with a sample event: stats reflects one completed run.
	tool.Bind(fakeHist{events: []*event.Event{
		makeEvent("task.received", "run-1", 100, map[string]any{"intent": "do x"}),
		makeEvent("task.completed", "run-1", 200, nil),
	}})
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"stats"}`))
	if err != nil {
		t.Fatalf("Invoke stats: %v", err)
	}
	for _, want := range []string{`"total": 1`, `"completed": 1`, `"failed": 0`} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("stats output = %q, want %q", res.Output, want)
		}
	}
}
