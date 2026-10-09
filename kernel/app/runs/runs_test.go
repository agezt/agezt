// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func decode[T any](t *testing.T, raw string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(raw, err)
	}
	return v
}

func fixed(runs ...Run) func() (map[string]Run, error) {
	return func() (map[string]Run, error) {
		m := map[string]Run{}
		for _, r := range runs {
			m[r.CorrelationID] = r
		}
		return m, nil
	}
}

var clock = func() time.Time { return time.UnixMilli(10_000) }

func TestRunStatusPrecedence(t *testing.T) {
	for want, r := range map[string]Run{
		"completed": {Completed: true, Failed: true, Abandoned: true},
		"failed":    {Failed: true, Abandoned: true},
		"abandoned": {Abandoned: true},
		"running":   {},
	} {
		if r.Status() != want {
			t.Fatal(want, r.Status())
		}
	}
}

func TestListCodecs(t *testing.T) {
	called := false
	s := New(func() (map[string]Run, error) { called = true; return nil, nil }, clock)
	for raw, want := range map[string]string{
		`{"status":1}`:                          "args.status must be a string",
		`{"status":1,"intent":2}`:               "args.status must be a string",
		`{"intent":null,"model":3}`:             "args.intent must be a string",
		`{"model":["m"]}`:                       "args.model must be a string",
		`{"limit":"x","cursor":4,"model":true}`: "args.model must be a string",
	} {
		if _, err := s.List(context.Background(), decode[ListRequest](t, raw)); err == nil || err.Error() != want {
			t.Fatal(raw, err)
		}
	}
	if called {
		t.Fatal("argument errors must precede the journal fold")
	}
	boom := errors.New("journal unreadable")
	if _, err := New(func() (map[string]Run, error) { return nil, boom }, clock).List(context.Background(), ListRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestListLimitClamp(t *testing.T) {
	var runs []Run
	for i := range 1_100 {
		runs = append(runs, Run{CorrelationID: string(rune('a'+i%26)) + strings.Repeat("x", i/26), StartedUnixMS: int64(i + 1)})
	}
	s := New(fixed(runs...), clock)
	for raw, want := range map[string]int{`{}`: 20, `{"limit":0}`: 1, `{"limit":-4}`: 1, `{"limit":2.9}`: 2, `{"limit":"5"}`: 20, `{"limit":5000}`: 1_000, `{"limit":1000}`: 1_000} {
		out, err := s.List(context.Background(), decode[ListRequest](t, raw))
		if err != nil || out.Count != want || len(out.Runs) != want {
			t.Fatal(raw, out.Count, err)
		}
	}
}

func TestListFiltersSortAndPage(t *testing.T) {
	s := New(fixed(
		Run{CorrelationID: "a", Intent: "Deploy prod", StartedUnixMS: 100, StartedSeq: 1, Completed: true, CompletedUnixMS: 160, Iters: 2, SpentMicrocents: 50, Model: "Claude-Opus", Agent: "ops"},
		Run{CorrelationID: "b", Intent: "deploy staging", StartedUnixMS: 100, StartedSeq: 2, Failed: true, FailedUnixMS: 130, FailReason: "timeout", SpentMicrocents: 500, ParentCorrelation: "a"},
		Run{CorrelationID: "c", Intent: "chat", StartedUnixMS: 300, Phase: "using tool", Tool: "shell"},
		Run{CorrelationID: "d", Intent: "deploy", StartedUnixMS: 50, Abandoned: true, Phase: "thinking", Tool: "x"},
		Run{CorrelationID: "e", Intent: "deploy", Completed: true, CompletedUnixMS: 70},
		Run{CorrelationID: "f", Intent: "DEPLOY", StartedUnixMS: 80, Failed: true, FailedUnixMS: 10},
		Run{CorrelationID: "g", Intent: "idle", StartedUnixMS: 200, Phase: "thinking"},
	), clock)
	ids := func(out ListOutput) string {
		var parts []string
		for _, r := range out.Runs {
			parts = append(parts, r.CorrelationID)
		}
		return strings.Join(parts, ",")
	}
	for raw, want := range map[string]string{
		`{}`:                                     "c,g,b,a,f,d,e",
		`{"status":"failed"}`:                    "b,f",
		`{"status":"running","limit":1}`:         "c",
		`{"intent":"DEPLOY"}`:                    "b,a,f,d,e",
		`{"model":"opus"}`:                       "a",
		`{"min_cost_mc":51}`:                     "b",
		`{"max_cost_mc":100}`:                    "c,g,a,f,d,e",
		`{"min_cost_mc":-1,"max_cost_mc":0}`:     "c,g,b,a,f,d,e",
		`{"min_cost_mc":50.9,"max_cost_mc":"1"}`: "b,a",
		`{"intent":"deploy","limit":2}`:          "b,a",
	} {
		if out, err := s.List(context.Background(), decode[ListRequest](t, raw)); err != nil || ids(out) != want {
			t.Fatal(raw, ids(out), err)
		}
	}
	page, _ := s.List(context.Background(), decode[ListRequest](t, `{"limit":2}`))
	if page.NextCursor != journal.NextCursor(200, 0, 2, 2) || page.NextCursor == "" {
		t.Fatal(page.NextCursor)
	}
	next, _ := s.List(context.Background(), decode[ListRequest](t, `{"limit":2,"cursor":"`+page.NextCursor+`"}`))
	if ids(next) != "b,a" {
		t.Fatal(ids(next))
	}
	if last, _ := s.List(context.Background(), decode[ListRequest](t, `{"limit":3,"cursor":"80:0"}`)); ids(last) != "d,e" || last.NextCursor != "" {
		t.Fatal("a short page has no next cursor", ids(last), last.NextCursor)
	}
	if bad, _ := s.List(context.Background(), decode[ListRequest](t, `{"cursor":"nonsense"}`)); ids(bad) != "c,g,b,a,f,d,e" {
		t.Fatal("an unreadable cursor is the first page", ids(bad))
	}
	all, _ := s.List(context.Background(), ListRequest{})
	rows := map[string]RunRow{}
	for _, r := range all.Runs {
		rows[r.CorrelationID] = r
	}
	if rows["a"] != (RunRow{CorrelationID: "a", Intent: "Deploy prod", Status: "completed", StartedUnixMS: 100, CompletedUnixMS: 160, DurationMS: 60, Iters: 2, SpentMC: 50, Model: "Claude-Opus", Agent: "ops"}) {
		t.Fatal(rows["a"])
	}
	if r := rows["b"]; r.Status != "failed" || r.Reason != "timeout" || r.DurationMS != 30 || r.ParentCorrelation != "a" {
		t.Fatal(r)
	}
	if r := rows["c"]; r.Phase != "using tool" || r.Tool != "shell" || r.Status != "running" {
		t.Fatal(r)
	}
	if r := rows["d"]; r.Status != "abandoned" || r.Phase != "" || r.Tool != "" || r.Reason != "" {
		t.Fatal("only a running run reports its phase", r)
	}
	if rows["e"].DurationMS != 0 || rows["f"].DurationMS != 0 || rows["f"].Reason != "" {
		t.Fatal("no start or a failure before the start has no duration", rows["e"], rows["f"])
	}
	raw, _ := json.Marshal(rows["g"])
	if string(raw) != `{"correlation_id":"g","intent":"idle","status":"running","reason":"","started_unix_ms":200,"completed_unix_ms":0,"duration_ms":0,"iters":0,"parent_correlation":"","spent_mc":0,"model":"","answer_preview":"","agent":"","phase":"thinking"}` {
		t.Fatal(string(raw))
	}
	empty, _ := New(fixed(), clock).List(context.Background(), ListRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"runs":[],"count":0,"next_cursor":""}` {
		t.Fatal(string(raw))
	}
}

func TestStats(t *testing.T) {
	ctx := context.Background()
	s := New(fixed(
		Run{CorrelationID: "a", Intent: "deploy", StartedUnixMS: 9_000, Completed: true, CompletedUnixMS: 9_100, Iters: 3, SpentMicrocents: 40, Model: "opus"},
		Run{CorrelationID: "b", Intent: "deploy", StartedUnixMS: 9_500, Completed: true, CompletedUnixMS: 9_800, Iters: 2, SpentMicrocents: 10, Model: "opus", ParentCorrelation: "a"},
		Run{CorrelationID: "c", Intent: "Chat", StartedUnixMS: 9_900, Failed: true, FailReason: "timeout", ParentCorrelation: "a", Model: "haiku"},
		Run{CorrelationID: "d", Intent: "chat", StartedUnixMS: 1_000, Failed: true, ParentCorrelation: "b"},
		Run{CorrelationID: "e", Intent: "chat", StartedUnixMS: 9_990, Abandoned: true},
		Run{CorrelationID: "f", Intent: "chat"},
		Run{CorrelationID: "g", Intent: "chat", StartedUnixMS: 9_000, Completed: true, CompletedUnixMS: 8_000},
	), clock)
	all, err := s.Stats(ctx, StatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := StatsOutput{
		Total: 7, Completed: 3, Failed: 2, Running: 1, Abandoned: 1, Terminal: 6,
		SuccessRate: 0.5, AvgIters: 5.0 / 3, FailedByReason: map[string]int{"timeout": 1, "unknown": 1},
		Delegations: 3, DelegatingRuns: 2, MaxFanout: 2, SpentMicrocents: 50, DelegatedSpentMicrocents: 10,
		ByModel:         map[string]ModelSpend{"opus": {2, 50}, "haiku": {1, 0}},
		SpendMicrocents: distribution([]int64{40, 10}), DurationMS: distribution([]int64{100, 300}),
	}
	if !reflect.DeepEqual(all, want) {
		t.Fatalf("%+v\n%+v", all, want)
	}
	windowed, _ := s.Stats(ctx, decode[StatsRequest](t, `{"since_ms":1000.7,"intent":"CHAT"}`))
	if windowed.Total != 3 || windowed.WindowMS != 1000 || windowed.Failed != 1 || windowed.Abandoned != 1 || windowed.Completed != 1 || windowed.DurationMS.Count != 0 {
		t.Fatalf("window keeps started runs at/after now-since_ms and the intent scope: %+v", windowed)
	}
	if ignored, _ := s.Stats(ctx, decode[StatsRequest](t, `{"since_ms":"60000"}`)); ignored.Total != 7 || ignored.WindowMS != 0 {
		t.Fatal(ignored)
	}
	if negative, _ := s.Stats(ctx, decode[StatsRequest](t, `{"since_ms":-5}`)); negative.Total != 7 || negative.WindowMS != -5 {
		t.Fatal("a non-positive window is all-time but still echoed", negative)
	}
	if _, err := s.Stats(ctx, decode[StatsRequest](t, `{"intent":4}`)); err == nil || err.Error() != "args.intent must be a string" {
		t.Fatal(err)
	}
	boom := errors.New("journal unreadable")
	if _, err := New(func() (map[string]Run, error) { return nil, boom }, clock).Stats(ctx, decode[StatsRequest](t, `{"intent":4}`)); !errors.Is(err, boom) {
		t.Fatal("the fold runs before the intent check", err)
	}
	empty, _ := New(fixed(), clock).Stats(ctx, StatsRequest{})
	raw, _ := json.Marshal(empty)
	if string(raw) != `{"total":0,"completed":0,"failed":0,"running":0,"abandoned":0,"terminal":0,"success_rate":0,"avg_iters":0,"failed_by_reason":{},"window_ms":0,"delegations":0,"delegating_runs":0,"max_fanout":0,"spent_microcents":0,"delegated_spent_microcents":0,"by_model":{},"spend_microcents":{"count":0,"avg":0,"min":0,"max":0,"p50":0,"p95":0},"duration_ms":{"count":0,"avg":0,"min":0,"max":0,"p50":0,"p95":0}}` {
		t.Fatal(string(raw))
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	ops, err := Operations(func(context.Context) *Service { return New(fixed(), clock) })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	for i, w := range []struct {
		name string
		http opapi.HTTP
		in   reflect.Type
		out  reflect.Type
	}{
		{"runs_list", opapi.HTTP{Method: "GET", Path: "/api/runs"}, reflect.TypeFor[ListRequest](), reflect.TypeFor[ListOutput]()},
		{"runs_stats", opapi.HTTP{}, reflect.TypeFor[StatsRequest](), reflect.TypeFor[StatsOutput]()},
	} {
		spec := ops[i].Spec()
		if spec.Name != w.name || !spec.ReadOnly || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || !spec.AllowUnknownInput || spec.HTTP != w.http || spec.Input != w.in || spec.Output != w.out {
			t.Fatal(spec)
		}
	}
	list, _ := New(fixed(Run{CorrelationID: "a"}), clock).List(context.Background(), ListRequest{})
	stats, _ := New(fixed(Run{CorrelationID: "a", Model: "m", Failed: true}), clock).Stats(context.Background(), StatsRequest{})
	for i, v := range []any{list, stats} {
		raw, _ := json.Marshal(v)
		if err := schema.ValidateJSON(ops[i].Spec().OutputSchema, raw); err != nil {
			t.Fatal(i, err, string(raw))
		}
	}
}
