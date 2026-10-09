// SPDX-License-Identifier: MIT

package runs

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

type planJournal struct {
	events []*event.Event
	err    error
}

func (j planJournal) Range(fn func(*event.Event) error) error {
	for _, e := range j.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return j.err
}

func planEvent(seq, ts int64, kind event.Kind, corr, payload string) *event.Event {
	return &event.Event{Seq: seq, TSUnixMS: ts, Kind: kind, CorrelationID: corr, Payload: json.RawMessage(payload)}
}

// planHistory: completed, failed and running plans; a terminal event naming a
// plan whose start did not; a completion without a start; a malformed start
// whose well-typed node_count must still read as zero;
// and an unrelated event.
func planHistory() planJournal {
	return planJournal{events: []*event.Event{
		planEvent(1, 1_000, event.KindPlanStarted, "plan-a", `{"plan_name":"alpha","node_count":3}`),
		planEvent(2, 1_400, event.KindPlanCompleted, "plan-a", `{"plan_name":"ignored"}`),
		planEvent(3, 2_000, event.KindPlanStarted, "plan-b", `{"node_count":2}`),
		planEvent(4, 2_900, event.KindPlanFailed, "plan-b", `{"plan_name":"beta"}`),
		planEvent(5, 3_000, event.KindPlanStarted, "plan-c", `{"plan_name":"gamma","node_count":5}`),
		planEvent(6, 4_000, event.KindPlanCompleted, "plan-orphan", `{"plan_name":"orphan"}`),
		planEvent(7, 5_000, event.KindPlanStarted, "plan-d", `{"node_count":4,"plan_name":7}`),
		planEvent(8, 4_500, event.KindPlanCompleted, "plan-d", `{}`),
		planEvent(9, 6_000, event.KindTaskReceived, "run-x", `{}`),
	}}
}

func TestPlanList(t *testing.T) {
	ctx := context.Background()
	p := NewPlans(planHistory())
	out, err := p.List(ctx, PlansRequest{})
	if err != nil || out.Count != 5 || out.NextCursor != "" {
		t.Fatalf("%+v %v", out, err)
	}
	want := []PlanRow{
		{CorrelationID: "plan-d", Status: "completed", StartedUnixMS: 5_000},
		{CorrelationID: "plan-c", PlanName: "gamma", NodeCount: 5, Status: "running", StartedUnixMS: 3_000},
		{CorrelationID: "plan-b", PlanName: "beta", NodeCount: 2, Status: "failed", StartedUnixMS: 2_000, DurationMS: 900},
		{CorrelationID: "plan-a", PlanName: "alpha", NodeCount: 3, Status: "completed", StartedUnixMS: 1_000, DurationMS: 400},
		{CorrelationID: "plan-orphan", PlanName: "orphan", Status: "completed"},
	}
	if !reflect.DeepEqual(out.Plans, want) {
		t.Fatalf("newest start first, start-less plans last, names from the terminal event only when the start had none, no duration for an end before its start:\n%+v", out.Plans)
	}
	for status, n := range map[string]int{"completed": 3, "failed": 1, "running": 1, "nope": 0} {
		got, err := p.List(ctx, decode[PlansRequest](t, `{"status":"`+status+`"}`))
		if err != nil || got.Count != n {
			t.Fatal(status, got.Count, err)
		}
	}
	paged, _ := p.List(ctx, decode[PlansRequest](t, `{"limit":2.9}`))
	if paged.Count != 2 || paged.NextCursor == "" {
		t.Fatal(paged)
	}
	next, _ := p.List(ctx, decode[PlansRequest](t, `{"limit":2,"cursor":"`+paged.NextCursor+`"}`))
	if next.Count != 2 || next.Plans[0].CorrelationID != "plan-b" {
		t.Fatal(next)
	}
	for raw, want := range map[string]int{`{"limit":0}`: 1, `{"limit":-3}`: 1, `{"limit":"5"}`: 5, `{"cursor":3}`: 5, `{"cursor":"garbage"}`: 5} {
		if got, _ := p.List(ctx, decode[PlansRequest](t, raw)); got.Count != want {
			t.Fatal(raw, got.Count)
		}
	}
	for _, raw := range []string{`{"status":3}`, `{"status":null}`, `{"status":["running"],"limit":"x"}`} {
		if _, err := p.List(ctx, decode[PlansRequest](t, raw)); err == nil || err.Error() != "args.status must be a string" {
			t.Fatal(raw, err)
		}
	}
	empty, _ := NewPlans(planJournal{}).List(ctx, PlansRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"plans":[],"count":0,"next_cursor":""}` {
		t.Fatal(string(raw))
	}
	boom := errors.New("journal unreadable")
	if _, err := NewPlans(planJournal{err: boom}).List(ctx, PlansRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

// Start-less plans share the zero start, so they order by correlation.
func TestPlanListStartlessTie(t *testing.T) {
	j := planJournal{events: []*event.Event{
		planEvent(1, 100, event.KindPlanCompleted, "plan-z", `{}`),
		planEvent(2, 200, event.KindPlanFailed, "plan-m", `{}`),
		planEvent(3, 300, event.KindPlanCompleted, "plan-a", `{}`),
	}}
	for range 20 {
		out, _ := NewPlans(j).List(context.Background(), PlansRequest{})
		if out.Plans[0].CorrelationID != "plan-a" || out.Plans[1].CorrelationID != "plan-m" || out.Plans[2].CorrelationID != "plan-z" {
			t.Fatal(out.Plans)
		}
	}
}

func TestPlanListSameMillisecond(t *testing.T) {
	j := planJournal{events: []*event.Event{
		planEvent(1, 500, event.KindPlanStarted, "plan-first", `{}`),
		planEvent(2, 500, event.KindPlanStarted, "plan-second", `{}`),
	}}
	out, _ := NewPlans(j).List(context.Background(), PlansRequest{})
	if out.Plans[0].CorrelationID != "plan-second" {
		t.Fatal("a same-millisecond tie orders by sequence, newest first", out.Plans)
	}
}

func TestPlanListLimits(t *testing.T) {
	var j planJournal
	for i := range 1_005 {
		j.events = append(j.events, planEvent(int64(i+1), int64(i+1), event.KindPlanStarted, "plan-"+string(rune('a'+i%26))+jsonInt(i), `{}`))
	}
	p := NewPlans(j)
	for raw, want := range map[string]int{`{}`: 20, `{"limit":25}`: 25, `{"limit":1000}`: 1_000, `{"limit":2000}`: 1_000} {
		out, err := p.List(context.Background(), decode[PlansRequest](t, raw))
		if err != nil || out.Count != want || out.NextCursor == "" {
			t.Fatal(raw, out.Count, err)
		}
	}
}

func jsonInt(i int) string { raw, _ := json.Marshal(i); return string(raw) }

func TestPlanStats(t *testing.T) {
	out, err := NewPlans(planHistory()).Stats(context.Background(), PlanStatsRequest{})
	want := PlanStatsOutput{Total: 5, Completed: 3, Failed: 1, Running: 1, Terminal: 4, SuccessRate: 0.75, DurationMS: Distribution{Count: 2, Avg: 650, Min: 400, Max: 900, P50: 400, P95: 900}}
	if err != nil || out != want {
		t.Fatalf("%+v %v", out, err)
	}
	empty, _ := NewPlans(planJournal{}).Stats(context.Background(), PlanStatsRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"total":0,"completed":0,"failed":0,"running":0,"terminal":0,"success_rate":0,"duration_ms":{"count":0,"avg":0,"min":0,"max":0,"p50":0,"p95":0}}` {
		t.Fatal(string(raw))
	}
	boom := errors.New("journal unreadable")
	if _, err := NewPlans(planJournal{err: boom}).Stats(context.Background(), PlanStatsRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestPlanOperations(t *testing.T) {
	if _, err := PlanOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	p := NewPlans(planHistory())
	ops, err := PlanOperations(func(context.Context) *Plans { return p })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	list, _ := p.List(context.Background(), PlansRequest{})
	stats, _ := p.Stats(context.Background(), PlanStatsRequest{})
	for i, w := range []struct {
		name string
		http opapi.HTTP
		out  any
	}{
		{"plan_history", opapi.HTTP{Method: "GET", Path: "/api/plan_history"}, list},
		{"plan_stats", opapi.HTTP{}, stats},
	} {
		spec := ops[i].Spec()
		if spec.Name != w.name || !spec.ReadOnly || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || !spec.AllowUnknownInput || spec.HTTP != w.http || spec.Output != reflect.TypeOf(w.out) {
			t.Fatal(spec)
		}
		raw, _ := json.Marshal(w.out)
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(w.name, err, string(raw))
		}
	}
}
